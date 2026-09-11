import { describe, expect, it } from 'vitest'
import { spawn, spawnSync, type ChildProcessByStdio } from 'node:child_process'
import { createHash } from 'node:crypto'
import { createServer, type Server } from 'node:http'
import { readFileSync, readdirSync } from 'node:fs'
import { chmod, mkdir, mkdtemp, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Readable } from 'node:stream'
import {
  buildGoRuntimeKernelConformanceManifest,
  GO_RUNTIME_KERNEL_COMPONENTS
} from '../src/conformance/go-runtime-kernel-conformance.js'
import {
  ApprovalUserInputRouteContract,
  GoG3ProviderStreamingUsageCacheContract,
  GoG4ToolsApprovalUserInputMcpContract,
  GoG5FullLoopContract,
  GoMinimalAgentLoopContract,
  McpToolLifecycleContract,
  ProviderCacheContract,
  TaskJobOrchestrationContract
} from '../src/conformance/runtime-parity-fixtures.js'
import { evaluateOfflineCacheCurveGuard } from '../src/cache/offline-cache-curve-guard.js'
import { RuntimeInfoResponse } from '../src/contracts/runtime-info.js'
import { RuntimeToolsResponse } from '../src/contracts/runtime-tools.js'
import { PublicRuntimeErrorResponse } from '../src/contracts/errors.js'
import {
  providerRegistryProviderResponseSchemaV1,
  providerRegistrySnapshotResponseSchemaV1
} from '../src/contracts/provider-registry.js'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { buildRouter } from '../src/server-test-support/routes/index.js'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import { DurableTaskJobManager, FileTaskJobStore } from '../src/delegation-test-support/job-manager.js'
import { AutoResearchProjectStore } from '../src/research/autoresearch-store.js'
import { runLiveLocalIndexerContract } from '../src/conformance/mcp-live-local-indexer.js'
import { buildAuditableCheckpointRewindPlan } from '../src/domain/checkpoint-rewind-contract.js'
import {
  AUTO_MODEL_ROUTER_FINGERPRINT,
  AUTO_MODEL_ROUTER_MODEL,
  AUTO_MODEL_ROUTER_TIMEOUT_MS,
  buildAutoModelRouterFingerprint,
  parseAutoRouteRecommendation,
  recentAutoRouterContext
} from '../src/shared/auto-model-router.js'
import { resolvePlanModeToolSpecs } from '../src/loop-test-support/agent-loop.js'
import { effectiveHistoryAfterLatestCompaction } from '../src/shared/compaction-history.js'
import { repairModelHistoryItems } from '../src/domain/model-history-repair.js'
import {
  makeAssistantTextItem,
  makePublicToolCallArgumentsProjection,
  makePublicToolResultWithheldProjection,
  makeToolResultItem,
  makeUserItem
} from '../src/domain/item.js'
import type { CheckpointMetadata } from '../src/contracts/checkpoints.js'
import type { RuntimeEvent } from '../src/contracts/events.js'
import type { TurnItem } from '../src/contracts/items.js'
import type { ThreadRecord } from '../src/contracts/threads.js'
import type { Turn } from '../src/contracts/turns.js'
import type { ServerRuntime } from '../src/server-test-support/routes/server-runtime.js'
import { buildHarness, readJson, readSseEvents } from './http-server-test-harness.js'
import { findGoBinary, goTestEnv } from './go-test-toolchain.js'
import { PublicRuntimeEventFilter } from '../../../src/shared/public-runtime-content.js'
import {
  projectPublicRuntimeSseBlock,
  takePublicRuntimeSseBlock
} from '../../../src/shared/public-runtime-sse.js'
import { verifiedGeneralTerminalDeliveryBatchV1 } from '../../../src/main/general-terminal-publication.js'

type ContractResponse = {
  status: number
  body: Record<string, unknown>
}

type GoG1ShadowContract = {
  runtimeToken: string
  startedAt: string
  productBoundary: Record<string, boolean>
  health: ContractResponse
  runtimeInfoUnauthorized: ContractResponse
  runtimeInfo: { status: number; schemaVersion: number; bodyHash: string; forbiddenTopLevelKeys: string[] }
  runtimeToolsUnauthorized: ContractResponse
  runtimeTools: { status: number; schemaVersion: number; bodyHash: string; forbiddenTopLevelKeys: string[]; forbiddenJSONTokens: string[] }
}

const g1ContractUrl = new URL('../src/conformance/fixtures/go-g1-shadow-contract.json', import.meta.url)
const g2ContractUrl = new URL('../src/conformance/fixtures/go-g2-route-replay-contract.json', import.meta.url)
const g3ProviderContractUrl = new URL('../src/conformance/fixtures/go-g3-provider-streaming-usage-cache-contract.json', import.meta.url)
const g4ContractUrl = new URL('../src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-contract.json', import.meta.url)
const g5ContractUrl = new URL('../src/conformance/fixtures/go-g5-full-loop-contract.json', import.meta.url)
const minimalLoopContractUrl = new URL('../src/conformance/fixtures/go-minimal-agent-loop-contract.json', import.meta.url)
const taskJobContractUrl = new URL('../src/conformance/fixtures/task-job-orchestration-contract.json', import.meta.url)
const providerCacheContractUrl = new URL('../src/conformance/fixtures/provider-cache-contract.json', import.meta.url)
const mcpLifecycleContractUrl = new URL('../src/conformance/fixtures/mcp-tool-lifecycle-contract.json', import.meta.url)
const approvalUserInputContractUrl = new URL('../src/conformance/fixtures/approval-user-input-route-contract.json', import.meta.url)
const repoRootPath = fileURLToPath(new URL('../../../', import.meta.url))
const runtimeGoDir = join(repoRootPath, 'packages', 'runtime-go')

function loadGoG1ShadowContract(): GoG1ShadowContract {
  return JSON.parse(readFileSync(g1ContractUrl, 'utf8')) as GoG1ShadowContract
}

function exactPublicContractHash(value: unknown): string {
  return createHash('sha256').update(JSON.stringify(value)).digest('hex')
}

type GoG2RouteReplayContract = {
  runtimeToken: string
  productBoundary: Record<string, boolean>
  routes: GoG2RouteReplayCase[]
}

type GoG2RouteReplayCase = {
  id: string
  setup: G2Setup
  method: 'GET' | 'PATCH' | 'POST'
  path: string
  auth: 'none' | 'runtime-token'
  responseKind: 'json' | 'sse'
  body?: unknown
  response: {
    status: number
    body?: unknown
  }
  sseFrames?: string[]
}

type G2Setup =
  | 'thread-list'
  | 'thread-archive'
  | 'thread-search-archive'
  | 'thread-read'
  | 'thread-fork'
  | 'session-resume'
  | 'events'

function loadGoG2RouteReplayContract(): GoG2RouteReplayContract {
  return JSON.parse(readFileSync(g2ContractUrl, 'utf8')) as GoG2RouteReplayContract
}

function loadGoG3ProviderContract() {
  return GoG3ProviderStreamingUsageCacheContract.parse(JSON.parse(readFileSync(g3ProviderContractUrl, 'utf8')))
}

function loadGoG4ToolsContract() {
  return GoG4ToolsApprovalUserInputMcpContract.parse(JSON.parse(readFileSync(g4ContractUrl, 'utf8')))
}

function loadGoG5FullLoopContract() {
  return GoG5FullLoopContract.parse(JSON.parse(readFileSync(g5ContractUrl, 'utf8')))
}

function loadGoMinimalAgentLoopContract() {
  return GoMinimalAgentLoopContract.parse(JSON.parse(readFileSync(minimalLoopContractUrl, 'utf8')))
}

function loadTaskJobOrchestrationContract() {
  return TaskJobOrchestrationContract.parse(JSON.parse(readFileSync(taskJobContractUrl, 'utf8')))
}

function loadProviderCacheContract() {
  return ProviderCacheContract.parse(JSON.parse(readFileSync(providerCacheContractUrl, 'utf8')))
}

function loadApprovalUserInputRouteContract() {
  return ApprovalUserInputRouteContract.parse(JSON.parse(readFileSync(approvalUserInputContractUrl, 'utf8')))
}

function loadMcpToolLifecycleContract() {
  return McpToolLifecycleContract.parse(JSON.parse(readFileSync(mcpLifecycleContractUrl, 'utf8')))
}

type GoG5FullLoop = ReturnType<typeof loadGoG5FullLoopContract>

function combinedStepCancelCacheExpected(g5: GoG5FullLoop) {
  const cancelResults = [
    {
      callId: 'call_combo_completed',
      status: 'completed',
      code: '',
      output: 'completed before combo cancel'
    },
    {
      callId: 'call_combo_running',
      status: 'aborted',
      code: g5.controlReplay.combined.cancelledResultCode,
      output: 'partial combo output'
    },
    {
      callId: 'call_combo_unstarted',
      status: 'aborted',
      code: g5.controlReplay.combined.cancelledResultCode,
      output: 'cancelled before tool execution'
    }
  ]

  return {
    sameTurnRouterCalls: g5.controlExecutableCases.combined.autoRouteCache.sameTurnRouterCalls,
    mainModelSteps: g5.controlExecutableCases.combined.autoRouteCache.mainModelSteps,
    maxModelSteps: g5.controlExecutableCases.combined.stepLimit.maxModelSteps,
    nextTurnRouterCalls: g5.controlExecutableCases.combined.autoRouteCache.nextTurnRouterCalls,
    routeCacheReusedUntilStepLimit: true,
    nextTurnReroutes: true,
    stepLimitErrorCode: g5.controlReplay.combined.stepLimitErrorCode,
    dynamicControlStateInStablePrefix: g5.controlReplay.combined.dynamicControlStateInStablePrefix,
    cancelledResultCode: g5.controlReplay.combined.cancelledResultCode,
    classifierStateInStablePrefix: false,
    stepLimitDynamicInStablePrefix: g5.controlReplay.stepLimits.dynamicLimitInStablePrefix,
    acceptedToolCallCount: g5.controlExecutableCases.combined.cancel.acceptedToolCalls.length,
    cancelResultCount: cancelResults.length,
    completedResultCount: 1,
    abortedResultCount: 2,
    cancelResults,
    completedResultPreserved: true,
    unstartedResultStatus: g5.controlReplay.cancel.unstartedResultStatus
  }
}

function planStepCancelCacheExpected(g5: GoG5FullLoop) {
  const planCase = g5.controlExecutableCases.planStepCancelCache
  return {
    step0ReadOnlyPlusPlan:
      planCase.step0MustAdvertise.every((toolName) =>
        g5.controlExecutableCases.planner.expectedStep0Advertised.includes(toolName)
      ) &&
      planCase.step0MustNotAdvertise.every((toolName) =>
        !g5.controlExecutableCases.planner.expectedStep0Advertised.includes(toolName)
      ),
    followUpOnlyCreatePlan: planCase.followUpRequiredToolName === 'create_plan' &&
      planCase.followUpTools.length === 1 &&
      planCase.followUpTools[0] === 'create_plan',
    cancelledStepDoesNotAdvanceCacheBaseline: planCase.abortedRunStatus === 'aborted' &&
      planCase.prefixChanged === false &&
      planCase.prefixChangeReasons.length === 0,
    nextPlanReusesOriginalCacheBaseline: planCase.retryRunStatus === 'failed' &&
      planCase.retryMustAdvertise.every((toolName) =>
        g5.controlExecutableCases.planner.expectedStep0Advertised.includes(toolName)
      ) &&
      planCase.prefixChanged === false,
    usageEventCount: planCase.usageEventCount,
    allPrefixChangedFalse: planCase.prefixChanged === false,
    cacheTelemetryPreserved: planCase.provider === 'deepseek' &&
      planCase.endpointFormat === 'chat_completions' &&
      planCase.cacheHitTokens === 80 &&
      planCase.cacheMissTokens === 20,
    forbiddenShellExcluded: planCase.step0MustNotAdvertise.includes('bash'),
    usesReasonixProtocol: planCase.productBoundary.reasonixControllerProtocol,
    topLevelRouteExposed: planCase.productBoundary.topLevelRouteExposed,
    defaultGoBackendEnabled: planCase.productBoundary.defaultGoBackendEnabled
  }
}

function planCancelStateResetExpected(g5: GoG5FullLoop) {
  const reset = g5.controlExecutableCases.planCancelStateReset
  return {
    cancelledPlanDoesNotLeakMode: reset.previousMode === 'plan' &&
      reset.abortedRunStatus === 'aborted' &&
      reset.normalModeInstructionPresent === false &&
      reset.autoModeInstructionPresent === false,
    normalTurnHidesCreatePlan: reset.normalMustNotAdvertise.includes(reset.planToolName),
    normalTurnHasNoPlanRequirement: reset.normalRequiredToolNamePresent === false,
    autoTurnReroutesAfterCancel: reset.autoRequestedModel === 'auto' &&
      reset.autoRouterCalls === 1 &&
      reset.autoRouterTurnIdSuffix === '_auto_router',
    autoRouterRequestIsolated: reset.autoRouterToolCount === 0 &&
      reset.autoRouterPrefixItemCount === 0 &&
      reset.autoRouterModeInstructionPresent === false,
    autoRecommendationCurrent: reset.autoRealModel === 'deepseek-v4-pro' &&
      reset.autoReasoningEffort === 'max',
    autoTurnHidesCreatePlan: reset.autoMustNotAdvertise.includes(reset.planToolName),
    autoTurnHasNoPlanRequirement: reset.autoRequiredToolNamePresent === false,
    stablePrefixClean: reset.stablePrefixContainsPlanState === false &&
      reset.stablePrefixContainsClassifierState === false,
    usesReasonixProtocol: reset.productBoundary.reasonixControllerProtocol,
    topLevelRouteExposed: reset.productBoundary.topLevelRouteExposed,
    defaultGoBackendEnabled: reset.productBoundary.defaultGoBackendEnabled
  }
}

function promptQuestionReplay(questions: Array<{
  header: string
  id: string
  options: Array<{ label: string }>
}>) {
  return questions.map((question) => ({
    header: question.header,
    id: question.id,
    optionLabels: question.options.map((option) => option.label)
  }))
}

function namedToolSpecs(toolNames: string[]) {
  return toolNames.map((name) => ({
    name,
    description: `${name} test tool`,
    inputSchema: {}
  }))
}

function autoRouterRecommendationParsingCases() {
  return [
    { id: 'pro-max-json', raw: '{"model":"pro","thinking":"max"}' },
    { id: 'flash-no-thinking-noise', raw: 'noise {"model":"v4-flash"} tail' },
    { id: 'auto-rejected', raw: '{"model":"auto"}' },
    { id: 'malformed-rejected', raw: 'not json' }
  ].map((item) => {
    const parsed = parseAutoRouteRecommendation(item.raw)
    return {
      ...item,
      accepted: parsed !== null,
      expectedModel: parsed?.model ?? null,
      expectedReasoningEffort: parsed?.reasoningEffort ?? null
    }
  })
}

function autoRouterContextBoundaryCase() {
  const currentTurnId = 'turn_3'
  const items: TurnItem[] = [
    makeUserItem({ id: 'u1', threadId: 'thr_1', turnId: 'turn_1', text: 'hello' }),
    makeAssistantTextItem({ id: 'a1', threadId: 'thr_1', turnId: 'turn_1', text: 'hi', status: 'completed' }),
    makeToolResultItem({
      id: 'r1',
      threadId: 'thr_1',
      turnId: 'turn_2',
      callId: 'call_1',
      toolName: 'read',
      output: 'file content'
    }),
    makeUserItem({ id: 'u2', threadId: 'thr_1', turnId: currentTurnId, text: 'latest' })
  ]
  const recentContext = recentAutoRouterContext(items, currentTurnId)
  const includedRows = [
    'user: hello',
    'assistant: hi',
    'tool: [tool result: read] status=completed disclosure=metadata_only'
  ]
  const excludedLatestText = 'latest'
  return {
    currentTurnId,
    includedRows,
    excludedLatestText,
    activeTurnExcluded: !recentContext.includes(excludedLatestText),
    toolResultSummarized:
      recentContext.includes('tool: [tool result: read] status=completed disclosure=metadata_only') &&
      !recentContext.includes('file content')
  }
}

function repoSource(relativePath: string) {
  return readFileSync(new URL(`../../../${relativePath}`, import.meta.url), 'utf8')
}

type GoTestChildProcess = ChildProcessByStdio<null, Readable, Readable>

type OwnedProcessExit = {
  code: number | null
  signal: NodeJS.Signals | null
  spawnError: Error | null
}

type OwnedProcessStopResult = {
  pid: number | null
  termination: 'already_exited' | 'sigterm' | 'sigkill'
  signals: NodeJS.Signals[]
  exit: OwnedProcessExit
}

type OwnedProcessHandle = {
  process: GoTestChildProcess
  pid: number | null
  processGroupId: number | null
  exit: Promise<OwnedProcessExit>
  exitStatus: OwnedProcessExit | null
  stopPromise: Promise<OwnedProcessStopResult> | null
  stopResult: OwnedProcessStopResult | null
}

type GoRuntimeServerHandle = {
  url: string
  runtimeToken: string
  ownedProcess: OwnedProcessHandle
  stderr: () => string
}

type GoRuntimeServerBuild = {
  executablePath: string
  cleanup: () => Promise<void>
}

const GO_RUNTIME_SERVER_TEST_TOKEN = 'go-runtime-conformance-token'
const GO_RUNTIME_SERVER_BUILD_TIMEOUT_MS = 180_000
const GO_RUNTIME_SERVER_STOP_GRACE_MS = 2_000
const GO_RUNTIME_SERVER_KILL_WAIT_MS = 2_000

type ScriptedProviderHandle = {
  url: string
  close: () => Promise<void>
}

async function startScriptedProvider(): Promise<ScriptedProviderHandle> {
  const server: Server = createServer((req, res) => {
    if (req.method !== 'POST') {
      res.statusCode = 404
      res.end('not found')
      return
    }
    res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
    if (req.url === '/v1/messages') {
      res.end([
        'event: message_start',
        'data: {"type":"message_start","message":{"usage":{"input_tokens":50,"cache_creation_input_tokens":200,"cache_read_input_tokens":1000}}}',
        'event: content_block_delta',
        'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Cedar lanterns glow quietly."}}',
        'event: message_delta',
        'data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}',
        'event: message_stop',
        'data: {"type":"message_stop"}',
        ''
      ].join('\n\n'))
      return
    }
    res.end([
      'data: {"choices":[{"delta":{"content":"Go runtime ok"},"finish_reason":"stop"}]}',
      'data: {"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":40,"total_tokens":1040,"prompt_cache_hit_tokens":700,"prompt_cache_miss_tokens":300,"completion_tokens_details":{"reasoning_tokens":12}}}',
      'data: [DONE]',
      ''
    ].join('\n\n'))
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('missing scripted provider address')
  return {
    url: `http://127.0.0.1:${address.port}`,
    close: () => new Promise<void>((resolve) => server.close(() => resolve()))
  }
}

async function buildGoRuntimeServerBinary(): Promise<GoRuntimeServerBuild> {
  const go = await findGoBinary()
  const buildDir = await mkdtemp(join(tmpdir(), 'analytix-go-runtime-server-build-'))
  await chmod(buildDir, 0o700)
  const executablePath = join(
    buildDir,
    process.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server'
  )
  const buildResult = spawnSync(go, [
    'build',
    '-o',
    executablePath,
    './cmd/runtime-server'
  ], {
    cwd: runtimeGoDir,
    env: goTestEnv(),
    stdio: ['ignore', 'pipe', 'pipe'],
    timeout: GO_RUNTIME_SERVER_BUILD_TIMEOUT_MS
  })
  const errorCode = (buildResult.error as NodeJS.ErrnoException | undefined)?.code
  const failureClass = errorCode === 'ETIMEDOUT'
    ? 'timeout'
    : buildResult.error
      ? 'spawn_error'
      : buildResult.signal
        ? 'signal'
        : buildResult.status !== 0
          ? 'exit_status'
          : null
  if (failureClass !== null) {
    await rm(buildDir, { recursive: true, force: true })
    throw new Error(`Go runtime server build failed (${failureClass}).`)
  }
  let cleaned = false
  return {
    executablePath,
    cleanup: async () => {
      if (cleaned) return
      cleaned = true
      await rm(buildDir, { recursive: true, force: true })
    }
  }
}

async function startGoRuntimeServer(
  durableRoot: string,
  options: {
    runtimeServerBinary?: string
    runtimeDurableRoot?: boolean
    providerBaseURL?: string
  } = {}
): Promise<GoRuntimeServerHandle> {
  const runtimeServerBinary = options.runtimeServerBinary
  const command = runtimeServerBinary ?? await findGoBinary()
  const args = [
    ...(runtimeServerBinary === undefined ? ['run', './cmd/runtime-server'] : []),
    '--addr',
    '127.0.0.1:0',
    '--data-dir',
    durableRoot,
    '--provider-id',
    'deepseek',
    '--model',
    'deepseek-chat',
    '--endpoint-format',
    'chat_completions'
  ]
  if (options.providerBaseURL) args.push('--base-url', options.providerBaseURL)
  if (options.runtimeDurableRoot) {
    args.push('--runtime-durable-root', durableRoot)
  } else {
    args.push('--durable-temp-dir', durableRoot)
  }
  const child = spawn(command, args, {
    cwd: runtimeGoDir,
    env: goTestEnv({
      ANALYTIX_API_KEY: 'test-provider-key',
      ANALYTIX_RUNTIME_TOKEN: GO_RUNTIME_SERVER_TEST_TOKEN
    }),
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: process.platform !== 'win32'
  })
  const ownedProcess = ownSpawnedProcess(child, process.platform !== 'win32')
  let stderr = ''
  try {
    child.stdout.setEncoding('utf8')
    child.stderr.setEncoding('utf8')
    child.stderr.on('data', (chunk) => {
      stderr += String(chunk)
    })
    const ready = await waitForRuntimeServerReady(ownedProcess, () => stderr)
    return {
      url: ready.url,
      runtimeToken: GO_RUNTIME_SERVER_TEST_TOKEN,
      ownedProcess,
      stderr: () => stderr
    }
  } catch (error) {
    return rethrowAfterOwnedProcessCleanup(ownedProcess, error)
  }
}

function stopGoRuntimeServer(server: GoRuntimeServerHandle): Promise<OwnedProcessStopResult> {
  return stopOwnedProcess(server.ownedProcess)
}

function killGoRuntimeServer(server: GoRuntimeServerHandle): Promise<OwnedProcessStopResult> {
  return killOwnedProcess(server.ownedProcess)
}

function ownSpawnedProcess(child: GoTestChildProcess, detached: boolean): OwnedProcessHandle {
  const pid = typeof child.pid === 'number' && child.pid > 0 ? child.pid : null
  let resolveExit!: (exit: OwnedProcessExit) => void
  const owned: OwnedProcessHandle = {
    process: child,
    pid,
    processGroupId: detached && pid !== null ? pid : null,
    exit: new Promise<OwnedProcessExit>((resolve) => {
      resolveExit = resolve
    }),
    exitStatus: null,
    stopPromise: null,
    stopResult: null
  }
  const settle = (exit: OwnedProcessExit) => {
    if (owned.exitStatus !== null) return
    owned.exitStatus = exit
    resolveExit(exit)
  }
  child.once('exit', (code, signal) => {
    settle({ code, signal, spawnError: null })
  })
  child.once('error', (error) => {
    if (owned.pid === null) {
      settle({ code: null, signal: null, spawnError: error })
    }
  })
  if (child.exitCode !== null || child.signalCode !== null) {
    settle({ code: child.exitCode, signal: child.signalCode, spawnError: null })
  }
  return owned
}

function stopOwnedProcess(
  owned: OwnedProcessHandle,
  options: { graceMs?: number; killWaitMs?: number } = {}
): Promise<OwnedProcessStopResult> {
  if (owned.stopPromise === null) {
    owned.stopPromise = terminateOwnedProcess(owned, {
      graceful: true,
      graceMs: options.graceMs ?? GO_RUNTIME_SERVER_STOP_GRACE_MS,
      killWaitMs: options.killWaitMs ?? GO_RUNTIME_SERVER_KILL_WAIT_MS
    }).then((result) => {
      owned.stopResult = result
      return result
    })
  }
  return owned.stopPromise
}

function killOwnedProcess(
  owned: OwnedProcessHandle,
  options: { killWaitMs?: number } = {}
): Promise<OwnedProcessStopResult> {
  if (owned.stopPromise === null) {
    owned.stopPromise = terminateOwnedProcess(owned, {
      graceful: false,
      graceMs: 0,
      killWaitMs: options.killWaitMs ?? GO_RUNTIME_SERVER_KILL_WAIT_MS
    }).then((result) => {
      owned.stopResult = result
      return result
    })
  }
  return owned.stopPromise
}

async function terminateOwnedProcess(
  owned: OwnedProcessHandle,
  options: { graceful: boolean; graceMs: number; killWaitMs: number }
): Promise<OwnedProcessStopResult> {
  const alreadyExited = await waitForOwnedProcessExit(owned, 0)
  if (alreadyExited !== null) {
    return ownedStopResult(owned, [], alreadyExited)
  }

  const signals: NodeJS.Signals[] = []
  if (options.graceful) {
    if (signalOwnedProcess(owned, 'SIGTERM')) signals.push('SIGTERM')
    const gracefulExit = await waitForOwnedProcessExit(owned, options.graceMs)
    if (gracefulExit !== null) {
      return ownedStopResult(owned, signals, gracefulExit)
    }
  }

  if (signalOwnedProcess(owned, 'SIGKILL')) signals.push('SIGKILL')
  const forcedExit = await waitForOwnedProcessExit(owned, options.killWaitMs)
  if (forcedExit === null) {
    throw new Error(
      `Owned process ${owned.pid ?? 'without-pid'} did not fully exit after SIGKILL within ${options.killWaitMs}ms`
    )
  }
  return ownedStopResult(owned, signals, forcedExit)
}

function ownedStopResult(
  owned: OwnedProcessHandle,
  signals: NodeJS.Signals[],
  exit: OwnedProcessExit
): OwnedProcessStopResult {
  const lastSignal = signals.at(-1)
  return {
    pid: owned.pid,
    termination: lastSignal === 'SIGKILL'
      ? 'sigkill'
      : lastSignal === 'SIGTERM'
        ? 'sigterm'
        : 'already_exited',
    signals,
    exit
  }
}

function signalOwnedProcess(owned: OwnedProcessHandle, signal: NodeJS.Signals): boolean {
  if (ownedProcessExitIfComplete(owned) !== null) return false
  if (owned.processGroupId !== null) {
    try {
      process.kill(-owned.processGroupId, signal)
      return true
    } catch (error) {
      if (isNoSuchProcess(error)) return false
      throw error
    }
  }
  if (owned.pid === null) {
    if (owned.exitStatus !== null) return false
    throw new Error('Owned process has no captured PID and has not reported exit')
  }
  const signalled = owned.process.kill(signal)
  if (!signalled && owned.exitStatus === null) {
    throw new Error(`Owned process ${owned.pid} rejected ${signal}`)
  }
  return signalled
}

function ownedProcessExitIfComplete(owned: OwnedProcessHandle): OwnedProcessExit | null {
  if (owned.exitStatus === null) return null
  if (owned.processGroupId !== null && ownedProcessGroupExists(owned.processGroupId)) return null
  return owned.exitStatus
}

function ownedProcessGroupExists(processGroupId: number): boolean {
  try {
    process.kill(-processGroupId, 0)
    return true
  } catch (error) {
    if (isNoSuchProcess(error)) return false
    if (isPermissionDenied(error)) return true
    throw error
  }
}

function isNoSuchProcess(error: unknown): boolean {
  return error instanceof Error && 'code' in error && error.code === 'ESRCH'
}

function isPermissionDenied(error: unknown): boolean {
  return error instanceof Error && 'code' in error && error.code === 'EPERM'
}

async function waitForOwnedProcessExit(
  owned: OwnedProcessHandle,
  timeoutMs: number
): Promise<OwnedProcessExit | null> {
  const deadline = Date.now() + Math.max(0, timeoutMs)
  while (true) {
    const complete = ownedProcessExitIfComplete(owned)
    if (complete !== null) return complete
    const remaining = deadline - Date.now()
    if (remaining <= 0) return null
    if (owned.exitStatus === null) {
      await Promise.race([
        owned.exit,
        waitForTimer(Math.min(remaining, 25))
      ])
    } else {
      await waitForTimer(Math.min(remaining, 25))
    }
  }
}

function waitForTimer(timeoutMs: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, timeoutMs)
  })
}

async function rethrowAfterOwnedProcessCleanup(
  owned: OwnedProcessHandle,
  startupError: unknown,
  options: { graceMs?: number; killWaitMs?: number } = {}
): Promise<never> {
  try {
    await stopOwnedProcess(owned, options)
  } catch (cleanupError) {
    throw new AggregateError(
      [startupError, cleanupError],
      `Owned process startup failed and cleanup did not complete for PID ${owned.pid ?? 'unavailable'}`
    )
  }
  throw startupError
}

async function waitForRuntimeServerReady(
  owned: OwnedProcessHandle,
  stderr: () => string,
  timeoutMs = 60_000
): Promise<{ url: string; runtimeTokenConfigured: boolean }> {
  return new Promise((resolve, reject) => {
    let stdout = ''
    let settled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    const child = owned.process
    const finish = (
      outcome: { kind: 'resolve'; value: { url: string; runtimeTokenConfigured: boolean } } |
        { kind: 'reject'; error: Error }
    ) => {
      if (settled) return
      settled = true
      if (timer !== null) clearTimeout(timer)
      child.stdout.off('data', onStdout)
      if (outcome.kind === 'resolve') {
        resolve(outcome.value)
      } else {
        reject(outcome.error)
      }
    }
    const onStdout = (chunk: string | Buffer) => {
      stdout += String(chunk)
      const line = stdout.split('\n').find((item) => item.startsWith('ANALYTIX_RUNTIME_SERVER_READY '))
      if (!line) return
      try {
        const parsed = JSON.parse(line.slice('ANALYTIX_RUNTIME_SERVER_READY '.length)) as {
          url: string
          runtimeTokenConfigured: boolean
        }
        if (parsed.runtimeTokenConfigured !== true) {
          finish({
            kind: 'reject',
            error: new Error('Go runtime server did not confirm token configuration.')
          })
          return
        }
        finish({ kind: 'resolve', value: parsed })
      } catch (error) {
        finish({
          kind: 'reject',
          error: new Error('Go runtime server emitted an invalid ready payload.', { cause: error })
        })
      }
    }
    child.stdout.on('data', onStdout)
    timer = setTimeout(() => {
      finish({
        kind: 'reject',
        error: new Error(`Go runtime server did not become ready. stderr:\n${stderr()}`)
      })
    }, timeoutMs)
    void owned.exit.then((exit) => {
      const detail = exit.spawnError
        ? `spawn error ${exit.spawnError.message}`
        : `code ${exit.code} and signal ${exit.signal}`
      finish({
        kind: 'reject',
        error: new Error(`Go runtime server exited before ready with ${detail}. stderr:\n${stderr()}`)
      })
    })
  })
}

type OwnedProcessFixture = {
  ownedProcess: OwnedProcessHandle
  stderr: () => string
}

const OWNED_PROCESS_FIXTURE_READY_LINE =
  'ANALYTIX_RUNTIME_SERVER_READY {"url":"http://127.0.0.1:1","runtimeTokenConfigured":true}\n'

function spawnOwnedProcessFixture(source: string): OwnedProcessFixture {
  const child = spawn(process.execPath, ['-e', source], {
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: process.platform !== 'win32'
  })
  const ownedProcess = ownSpawnedProcess(child, process.platform !== 'win32')
  child.stdout.setEncoding('utf8')
  child.stderr.setEncoding('utf8')
  let stderr = ''
  child.stderr.on('data', (chunk) => {
    stderr += String(chunk)
  })
  return { ownedProcess, stderr: () => stderr }
}

async function waitForOwnedProcessFixtureReady(
  fixture: OwnedProcessFixture,
  timeoutMs: number
): Promise<{ url: string; runtimeTokenConfigured: boolean }> {
  try {
    return await waitForRuntimeServerReady(fixture.ownedProcess, fixture.stderr, timeoutMs)
  } catch (error) {
    return rethrowAfterOwnedProcessCleanup(fixture.ownedProcess, error, {
      graceMs: 100,
      killWaitMs: 2_000
    })
  }
}

async function goRuntimeServerJSON(
  server: GoRuntimeServerHandle,
  path: string,
  init: RequestInit = {}
): Promise<{ status: number; body: Record<string, unknown> }> {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${server.runtimeToken}`)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(`${server.url}${path}`, { ...init, headers })
  const text = await response.text()
  return { status: response.status, body: text.trim() ? recordValue(JSON.parse(text) as unknown) : {} }
}

async function goRuntimeServerText(
  server: GoRuntimeServerHandle,
  path: string,
  init: RequestInit = {}
): Promise<{ status: number; text: string }> {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${server.runtimeToken}`)
  const response = await fetch(`${server.url}${path}`, { ...init, headers })
  return { status: response.status, text: await response.text() }
}

function runtimeServerSsePayloads(text: string, threadId: string): Record<string, unknown>[] {
  const filter = new PublicRuntimeEventFilter()
  const payloads: Record<string, unknown>[] = []
  let buffer = text
  let previousSeq = 0
  while (true) {
    const next = takePublicRuntimeSseBlock(buffer)
    if (next === null) break
    buffer = next.rest
    const decision = projectPublicRuntimeSseBlock(next.block, threadId, filter)
    if (decision === null) continue
    if (decision.status !== 'emit') {
      throw new Error(`runtime SSE replay is not public and strict: ${decision.status}`)
    }
    const eventKind = typeof decision.event.kind === 'string' ? decision.event.kind : ''
    if (eventKind === 'general_terminal_batch' &&
        verifiedGeneralTerminalDeliveryBatchV1(decision.event) === null) {
      throw new Error('Go runtime general-terminal batch failed the complete Electron digest verifier')
    }
    if (eventKind === 'accepted_final_batch' || eventKind === 'general_terminal_batch') {
      const firstSeq = decision.event.firstSeq
      const lastSeq = decision.event.lastSeq
      if (typeof firstSeq !== 'number' || typeof lastSeq !== 'number' ||
          firstSeq !== previousSeq + 1 || lastSeq !== decision.seq) {
        throw new Error(`runtime SSE atomic replay range is not contiguous: ${previousSeq} -> ${String(firstSeq)}..${String(lastSeq)}`)
      }
    } else if (decision.seq !== previousSeq + 1) {
      throw new Error(`runtime SSE replay sequence is not contiguous: ${previousSeq} -> ${decision.seq}`)
    }
    previousSeq = decision.seq
    payloads.push(decision.event)
  }
  if (buffer.trim() !== '') throw new Error('runtime SSE replay ended with a truncated frame')
  return payloads
}

function repoProductionSources(relativeRoot: string) {
  const files: string[] = []
  const visit = (relativeDir: string) => {
    for (const entry of readdirSync(new URL(`../../../${relativeDir}/`, import.meta.url), { withFileTypes: true })) {
      const relativePath = `${relativeDir}/${entry.name}`
      if (entry.isDirectory()) {
        visit(relativePath)
        continue
      }
      if (!/\.(ts|tsx)$/.test(entry.name)) continue
      if (/\.(test|spec)\.(ts|tsx)$/.test(entry.name)) continue
      files.push(relativePath)
    }
  }
  visit(relativeRoot)
  return files.sort()
}

function sortedUnique(values: string[]) {
  return [...new Set(values)].sort()
}

function packageRuntimeIdentityControlCase() {
  const sourceFiles = {
    rootPackage: 'package.json' as const,
    runtimePackage: 'packages/runtime/package.json' as const,
    electronBuilderConfig: 'electron-builder.config.cjs' as const,
    electronBuilderStandardWinConfig: 'electron-builder.standard-win.cjs' as const,
    appIdentity: 'src/main/app-identity.ts' as const,
    mainIndex: 'src/main/index.ts' as const,
    resolveAnalytixBinary: 'src/main/resolve-analytix-binary.ts' as const,
    runtimeServeEntry: 'packages/runtime/src/cli/serve-entry.ts' as const,
    runtimeServe: 'packages/runtime/src/cli/serve.ts' as const,
    afterPack: 'scripts/after-pack.cjs' as const,
    packagingConfigTest: 'src/main/packaging-config.test.ts' as const,
    releaseWorkflow: '.github/workflows/release.yml' as const
  }
  const rootPackage = JSON.parse(repoSource(sourceFiles.rootPackage)) as {
    name: string
    productName: string
  }
  const runtimePackage = JSON.parse(repoSource(sourceFiles.runtimePackage)) as {
    name: string
    bin: Record<string, string>
    scripts: Record<string, string>
  }
  const builderConfigSource = repoSource(sourceFiles.electronBuilderConfig)
  const standardWinConfigSource = repoSource(sourceFiles.electronBuilderStandardWinConfig)
  const appIdentitySource = repoSource(sourceFiles.appIdentity)
  const mainIndexSource = repoSource(sourceFiles.mainIndex)
  const resolveAnalytixBinarySource = repoSource(sourceFiles.resolveAnalytixBinary)
  const runtimeServeEntrySource = repoSource(sourceFiles.runtimeServeEntry)
  const runtimeServeSource = repoSource(sourceFiles.runtimeServe)
  const afterPackSource = repoSource(sourceFiles.afterPack)
  const packagingConfigTestSource = repoSource(sourceFiles.packagingConfigTest)
  const releaseWorkflowSource = repoSource(sourceFiles.releaseWorkflow)
  const releaseIdentity = {
    rootPackageName: rootPackage.name,
    rootProductName: rootPackage.productName,
    runtimePackageName: runtimePackage.name,
    runtimeBinName: Object.keys(runtimePackage.bin)[0],
    runtimeBinPath: runtimePackage.bin.analytix,
    runtimeServeScript: runtimePackage.scripts.serve,
    appId: builderConfigSource.includes("appId: 'com.analytix.desktop'")
      ? 'com.analytix.desktop'
      : '',
    builderProductName: builderConfigSource.includes("productName: 'Analytix'")
      ? 'Analytix'
      : '',
    builderExecutableName: builderConfigSource.includes("executableName: 'analytix'")
      ? 'analytix'
      : '',
    artifactNamePrefix: builderConfigSource.includes('artifactName: `analytix-')
      ? 'analytix-'
      : '',
    nsisShortcutName: standardWinConfigSource.includes("shortcutName: 'Analytix灵鉴'")
      ? 'Analytix灵鉴'
      : '',
    nsisUninstallDisplayName: standardWinConfigSource.includes("uninstallDisplayName: 'Analytix灵鉴'")
      ? 'Analytix灵鉴'
      : '',
    appProductName: appIdentitySource.includes("export const APP_PRODUCT_NAME = 'Analytix'")
      ? 'Analytix'
      : '',
    appUserDataDirectoryName: appIdentitySource.includes("export const APP_USER_DATA_DIRECTORY_NAME = 'analytix'")
      ? 'analytix'
      : '',
    windowsAppUserModelId: mainIndexSource.includes("const APP_USER_MODEL_ID = 'com.analytix.desktop'")
      ? 'com.analytix.desktop'
      : ''
  }
  const runtimeCli = {
    bundledEntryCandidate: resolveAnalytixBinarySource.includes("'packages/runtime/dist/cli/serve-entry.js'")
      ? 'packages/runtime/dist/cli/serve-entry.js'
      : '',
    afterPackRequiredPath: afterPackSource.includes("'packages/runtime/dist/cli/serve-entry.js'")
      ? 'packages/runtime/dist/cli/serve-entry.js'
      : '',
    readyPrefix: runtimeServeEntrySource.includes("ANALYTIX_READY_PREFIX = 'ANALYTIX_READY '")
      ? 'ANALYTIX_READY '
      : '',
    serveUsagePrefix: runtimeServeSource.includes('analytix serve [options]')
      ? 'analytix serve [options]'
      : '',
    supportedCommand: runtimeServeEntrySource.includes("if (command === 'serve')")
      ? 'serve'
      : '',
    unknownCommandMessage: runtimeServeEntrySource.includes("Only 'analytix serve' is supported.")
      ? "Only 'analytix serve' is supported."
      : ''
  }
  const forbiddenIdentityFields = ['kun', 'reasonix', 'deepseek'] as const
  const identityFieldValues = Object.values(releaseIdentity)
    .concat(Object.values(runtimeCli))
    .map((value) => value.toLowerCase())
  const forbiddenIdentityFieldsPresent = forbiddenIdentityFields.filter((field) =>
    identityFieldValues.some((value) => value.includes(field))
  )
  const evidence = {
    packagingTestPinsReleaseIdentity: packagingConfigTestSource.includes('pins release identity and bundled runtime CLI to analytix') &&
      packagingConfigTestSource.includes("expect(rootPackage.name).toBe('analytix')") &&
      packagingConfigTestSource.includes("expect(rootPackage.productName).toBe('Analytix')") &&
      packagingConfigTestSource.includes("expect(defaultConfig.executableName).toBe('analytix')") &&
      packagingConfigTestSource.includes("expect(runtimePackage.bin).toEqual({") &&
      packagingConfigTestSource.includes("analytix: './dist/cli/serve-entry.js'"),
    resolveUsesBundledServeEntry: resolveAnalytixBinarySource.includes('DIST_ENTRY_CANDIDATES') &&
      resolveAnalytixBinarySource.includes("'packages/runtime/dist/cli/serve-entry.js'") &&
      resolveAnalytixBinarySource.includes('Build the full `analytix serve` argv'),
    serveEntryOnlySupportsServe: runtimeServeEntrySource.includes("if (command === 'serve')") &&
      runtimeServeEntrySource.includes("Only 'analytix serve' is supported.") &&
      !runtimeServeEntrySource.includes("command === 'kun'") &&
      !runtimeServeEntrySource.includes("command === 'reasonix'"),
    serveUsageMentionsAnalytixServe: runtimeServeSource.includes('analytix serve [options]'),
    afterPackRequiresServeEntry: afterPackSource.includes('ANALYTIX_RUNTIME_REQUIRED_PATHS') &&
      afterPackSource.includes("'packages/runtime/dist/cli/serve-entry.js'"),
    releaseWorkflowUsesAnalytixEnv: releaseWorkflowSource.includes('ANALYTIX_APP_VERSION') &&
      releaseWorkflowSource.includes('ANALYTIX_UPDATE_CHANNEL') &&
      !releaseWorkflowSource.includes('KUN_') &&
      !releaseWorkflowSource.includes('REASONIX_')
  }
  return {
    sourceContractId: 'package-runtime-cli-identity-v1' as const,
    sourceFiles,
    releaseIdentity,
    runtimeCli,
    forbiddenIdentityFields: [...forbiddenIdentityFields],
    forbiddenIdentityFieldsPresent,
    evidence,
    productBoundary: {
      usesReasonixProtocol: false as const,
      kunIdentityExposed: false as const,
      defaultGoBackendEnabled: false as const
    },
    expected: {
      rootPackageNameAnalytix: releaseIdentity.rootPackageName === 'analytix',
      rootProductNameAnalytix: releaseIdentity.rootProductName === 'Analytix',
      runtimePackageNameAnalytixRuntime: releaseIdentity.runtimePackageName === 'analytix-runtime',
      runtimeBinAnalytixServeEntry: releaseIdentity.runtimeBinName === 'analytix' &&
        releaseIdentity.runtimeBinPath === './dist/cli/serve-entry.js',
      runtimeServeScriptUsesServeEntry: releaseIdentity.runtimeServeScript === 'node ./dist/cli/serve-entry.js',
      builderAppIdAnalytix: releaseIdentity.appId === 'com.analytix.desktop',
      builderProductNameAnalytix: releaseIdentity.builderProductName === 'Analytix',
      builderExecutableNameLowercaseAnalytix: releaseIdentity.builderExecutableName === 'analytix',
      builderArtifactNameAnalytix: releaseIdentity.artifactNamePrefix === 'analytix-',
      nsisNamesAnalytix: releaseIdentity.nsisShortcutName === 'Analytix灵鉴' &&
        releaseIdentity.nsisUninstallDisplayName === 'Analytix灵鉴',
      appProductNameAnalytix: releaseIdentity.appProductName === 'Analytix',
      appUserDataDirectoryLowercaseAnalytix: releaseIdentity.appUserDataDirectoryName === 'analytix',
      windowsAppUserModelIdAnalytix: releaseIdentity.windowsAppUserModelId === 'com.analytix.desktop',
      resolveBundledServeEntry: evidence.resolveUsesBundledServeEntry &&
        runtimeCli.bundledEntryCandidate === 'packages/runtime/dist/cli/serve-entry.js',
      serveUsageAnalytixServe: evidence.serveUsageMentionsAnalytixServe &&
        runtimeCli.serveUsagePrefix === 'analytix serve [options]',
      serveEntryAllowsOnlyAnalytixServe: evidence.serveEntryOnlySupportsServe &&
        runtimeCli.supportedCommand === 'serve' &&
        runtimeCli.unknownCommandMessage === "Only 'analytix serve' is supported.",
      readyHandshakeAnalytix: runtimeCli.readyPrefix === 'ANALYTIX_READY ' &&
        runtimeServeEntrySource.includes("service: 'analytix'") &&
        runtimeServeEntrySource.includes("mode: 'serve'"),
      afterPackRequiresServeEntry: evidence.afterPackRequiresServeEntry &&
        runtimeCli.afterPackRequiredPath === runtimeCli.bundledEntryCandidate,
      releaseEnvAnalytixPrefixed: evidence.releaseWorkflowUsesAnalytixEnv,
      usesReasonixProtocol: false as const,
      kunIdentityExposed: forbiddenIdentityFieldsPresent.length > 0,
      defaultGoBackendEnabled: false as const
    }
  }
}

function runtimeHttpRouteSovereigntyControlCase() {
  const sourceFiles = {
    serverRoutesIndex: 'packages/runtime/src/server-test-support/routes/index.ts' as const,
    router: 'packages/runtime/src/server-test-support/router.ts' as const,
    httpServer: 'packages/runtime/src/server-test-support/http-server.ts' as const,
    sharedEndpoints: 'src/shared/analytix-endpoints.ts' as const,
    sharedEndpointTest: 'src/shared/analytix-endpoints.test.ts' as const,
    httpServerTest: 'packages/runtime/tests/http-server.test.ts' as const
  }
  const serverRoutesIndexSource = repoSource(sourceFiles.serverRoutesIndex)
  const routerSource = repoSource(sourceFiles.router)
  const httpServerSource = repoSource(sourceFiles.httpServer)
  const sharedEndpointsSource = repoSource(sourceFiles.sharedEndpoints)
  const sharedEndpointTestSource = repoSource(sourceFiles.sharedEndpointTest)
  const httpServerTestSource = repoSource(sourceFiles.httpServerTest)
  const routes = Array.from(
    serverRoutesIndexSource.matchAll(/router\.add\('([^']+)',\s*'([^']+)'/g)
  ).map((match) => ({
    method: match[1] as 'GET' | 'POST' | 'PATCH' | 'DELETE',
    path: match[2]
  }))
  const routeKeys = routes.map((route) => `${route.method} ${route.path}`)
  const authGuardCount = Array.from(
    serverRoutesIndexSource.matchAll(/if \(!authorize\(request, runtime\)\) return ERRORS\.unauthorized\(\)/g)
  ).length
  const unauthenticatedRoutes = routeKeys.filter((route) => route === 'GET /health')
  const protectedRouteKeys = routeKeys.filter((route) => route !== 'GET /health')
  const sseRoutes = routeKeys.filter((route) => route === 'GET /v1/threads/:id/events')
  const internalTaskJobRoutes = [
    'POST /v1/runtime/task-jobs/wait',
    'POST /v1/runtime/task-jobs/output',
    'POST /v1/runtime/task-jobs/kill'
  ].filter((route) => routeKeys.includes(route))
  const compatibilityOnlyRoutes = routeKeys.filter((route) => route === 'POST /v1/user-input/:id')
  const forbiddenRouteTokens = [
    '/v1/reasonix',
    '/v1/kun',
    '/v1/deepseek',
    '/v1/runtime/go',
    '/v1/workflow',
    '/v1/create-loop',
    '/v1/subagent',
    '/v1/subagents',
    '/v1/autoresearch',
    '/v1/auto-research',
    '/v1/mcp-indexer',
    '/session-api',
    '/api/session'
  ]
  const forbiddenRouteTokensPresent = forbiddenRouteTokens.filter((token) =>
    routeKeys.some((route) => route.includes(token))
  )
  const forbiddenProtocolTokens = [
    '/v1/reasonix',
    '/v1/kun',
    '/v1/deepseek',
    '/v1/runtime/go',
    '/session-api',
    '/api/session'
  ].filter((token) => forbiddenRouteTokens.includes(token))
  const forbiddenHiddenSurfaceTokens = [
    '/v1/workflow',
    '/v1/create-loop',
    '/v1/subagent',
    '/v1/subagents',
    '/v1/autoresearch',
    '/v1/auto-research',
    '/v1/mcp-indexer'
  ].filter((token) => forbiddenRouteTokens.includes(token))
  const threadLifecycleRoutes = [
    'GET /v1/threads',
    'POST /v1/threads',
    'GET /v1/threads/:id',
    'PATCH /v1/threads/:id',
    'DELETE /v1/threads/:id',
    'POST /v1/threads/:id/fork',
    'POST /v1/threads/:id/rewind',
    'POST /v1/sessions/:id/resume-thread'
  ]
  const approvalUserInputRoutes = [
    'POST /v1/approvals/:id',
    'POST /v1/user-inputs/:id'
  ]
  const sensitiveRouteKeys = [
    'GET /v1/threads/:id/events',
    'POST /v1/runtime/task-jobs/wait',
    'POST /v1/approvals/:id',
    'POST /v1/user-inputs/:id',
    'POST /v1/sessions/:id/resume-thread'
  ].filter((route) => routeKeys.includes(route))
  const authMatrix = {
    healthRoute: 'GET /health' as const,
    healthStatus: 200 as const,
    unauthorizedStatus: 401 as const,
    unauthorizedBody: {
      code: 'unauthorized' as const,
      message: 'Runtime authentication is required.' as const
    },
    protectedRouteCount: protectedRouteKeys.length,
    protectedRouteKeys,
    sensitiveRouteKeys
  }
  const forbiddenDispatchMatrix = {
    authMode: 'valid-runtime-token' as const,
    status: 404 as const,
    body: {
      code: 'not_found' as const,
      message: 'route not found' as const
    },
    tokenCount: forbiddenRouteTokens.length,
    tokens: forbiddenRouteTokens,
    protocolTokens: forbiddenProtocolTokens,
    hiddenSurfaceTokens: forbiddenHiddenSurfaceTokens
  }
  const sharedEndpointSourceId = 'id/with space?x=1#frag'
  const sharedEndpointTurnId = 'turn/with space?x=1#frag'
  const sharedEndpointTaskId = 'task/with space?x=1#frag'
  const sharedEndpointCheckpointId = 'axcp/with space?x=1#frag'
  const sharedEndpointCaseProjectId = 'case/with space?x=1#frag'
  const encodedSourceId = encodeURIComponent(sharedEndpointSourceId)
  const encodedTurnId = encodeURIComponent(sharedEndpointTurnId)
  const encodedTaskId = encodeURIComponent(sharedEndpointTaskId)
  const encodedCheckpointId = encodeURIComponent(sharedEndpointCheckpointId)
  const encodedCaseProjectId = encodeURIComponent(sharedEndpointCaseProjectId)
  const sharedEndpointBuilderCases = [
    { name: 'analytixThreadPath', template: '/v1/threads/{id}', outputPath: `/v1/threads/${encodedSourceId}`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadSummaryPath', template: '/v1/threads/{id}/summary', outputPath: `/v1/threads/${encodedSourceId}/summary`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadSummaryTaskOutputPath', template: '/v1/threads/{id}/summary/tasks/{task}/output', outputPath: `/v1/threads/${encodedSourceId}/summary/tasks/${encodedTaskId}/output`, encodedSegments: [encodedSourceId, encodedTaskId] },
    { name: 'analytixThreadSummaryTaskKillPath', template: '/v1/threads/{id}/summary/tasks/{task}/kill', outputPath: `/v1/threads/${encodedSourceId}/summary/tasks/${encodedTaskId}/kill`, encodedSegments: [encodedSourceId, encodedTaskId] },
    { name: 'analytixThreadSummaryTaskRestartPath', template: '/v1/threads/{id}/summary/tasks/{task}/restart', outputPath: `/v1/threads/${encodedSourceId}/summary/tasks/${encodedTaskId}/restart`, encodedSegments: [encodedSourceId, encodedTaskId] },
    { name: 'analytixThreadForkPath', template: '/v1/threads/{id}/fork', outputPath: `/v1/threads/${encodedSourceId}/fork`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadGoalPath', template: '/v1/threads/{id}/goal', outputPath: `/v1/threads/${encodedSourceId}/goal`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadTodosPath', template: '/v1/threads/{id}/todos', outputPath: `/v1/threads/${encodedSourceId}/todos`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadCompactPath', template: '/v1/threads/{id}/compact', outputPath: `/v1/threads/${encodedSourceId}/compact`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadReviewPath', template: '/v1/threads/{id}/review', outputPath: `/v1/threads/${encodedSourceId}/review`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadTurnsPath', template: '/v1/threads/{id}/turns', outputPath: `/v1/threads/${encodedSourceId}/turns`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadRewindPath', template: '/v1/threads/{id}/rewind', outputPath: `/v1/threads/${encodedSourceId}/rewind`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadSteerPath', template: '/v1/threads/{id}/turns/{turn}/steer', outputPath: `/v1/threads/${encodedSourceId}/turns/${encodedTurnId}/steer`, encodedSegments: [encodedSourceId, encodedTurnId] },
    { name: 'analytixThreadInterruptPath', template: '/v1/threads/{id}/turns/{turn}/interrupt', outputPath: `/v1/threads/${encodedSourceId}/turns/${encodedTurnId}/interrupt`, encodedSegments: [encodedSourceId, encodedTurnId] },
    { name: 'analytixThreadEventsPath', template: '/v1/threads/{id}/events', outputPath: `/v1/threads/${encodedSourceId}/events`, encodedSegments: [encodedSourceId] },
    { name: 'analytixThreadCheckpointRewindPlanPath', template: '/v1/threads/{id}/checkpoints/{checkpoint}/rewind-plan', outputPath: `/v1/threads/${encodedSourceId}/checkpoints/${encodedCheckpointId}/rewind-plan`, encodedSegments: [encodedSourceId, encodedCheckpointId] },
    { name: 'analytixThreadCheckpointRewindApplyPath', template: '/v1/threads/{id}/checkpoints/{checkpoint}/rewind-apply', outputPath: `/v1/threads/${encodedSourceId}/checkpoints/${encodedCheckpointId}/rewind-apply`, encodedSegments: [encodedSourceId, encodedCheckpointId] },
    { name: 'analytixApprovalPath', template: '/v1/approvals/{id}', outputPath: `/v1/approvals/${encodedSourceId}`, encodedSegments: [encodedSourceId] },
    { name: 'analytixUserInputPath', template: '/v1/user-inputs/{id}', outputPath: `/v1/user-inputs/${encodedSourceId}`, encodedSegments: [encodedSourceId] },
    { name: 'analytixSessionResumePath', template: '/v1/sessions/{id}/resume-thread', outputPath: `/v1/sessions/${encodedSourceId}/resume-thread`, encodedSegments: [encodedSourceId] },
    { name: 'analytixAttachmentPath', template: '/v1/attachments/{id}', outputPath: `/v1/attachments/${encodedSourceId}`, encodedSegments: [encodedSourceId] },
    { name: 'analytixAttachmentContentPath', template: '/v1/attachments/{id}/content', outputPath: `/v1/attachments/${encodedSourceId}/content`, encodedSegments: [encodedSourceId] },
    { name: 'analytixMemoryRecordPath', template: '/v1/memory/{id}', outputPath: `/v1/memory/${encodedSourceId}`, encodedSegments: [encodedSourceId] },
    { name: 'analytixCaseProjectThreadsPath', template: '/v1/case-projects/{id}/threads', outputPath: `/v1/case-projects/${encodedCaseProjectId}/threads`, encodedSegments: [encodedCaseProjectId] },
    { name: 'analytixCaseProjectDetailPath', template: '/v1/case-projects/{id}/detail', outputPath: `/v1/case-projects/${encodedCaseProjectId}/detail`, encodedSegments: [encodedCaseProjectId] }
  ] as const
  const sharedEndpointForbiddenTokens = [
    'reasonix',
    '/kun',
    '/deepseek',
    '/runtime/go',
    'workflow',
    'create-loop',
    'subagent',
    'autoresearch',
    'mcp-indexer',
    'session-api'
  ]
  const exportedEndpointStrings = Array.from(
    sharedEndpointsSource.matchAll(/export const [A-Z0-9_]+(?:_PATH|_TEMPLATE)\s*=\s*'([^']+)'/g)
  ).map((match) => match[1])
  const sharedEndpointBuilderMatrix = {
    sourceId: sharedEndpointSourceId,
    turnId: sharedEndpointTurnId,
    taskId: sharedEndpointTaskId,
    checkpointId: sharedEndpointCheckpointId,
    encodedSourceId,
    encodedTurnId,
    encodedTaskId,
    encodedCheckpointId,
    encodedCaseProjectId,
    builderCases: sharedEndpointBuilderCases,
    exportedEndpointStrings,
    forbiddenTokens: sharedEndpointForbiddenTokens,
    forbiddenTokensPresent: sharedEndpointForbiddenTokens.filter((token) =>
      exportedEndpointStrings.some((value) => value.includes(token))
    ),
    canonicalUserInputTemplate: '/v1/user-inputs/{id}' as const,
    singularUserInputTemplateExported: exportedEndpointStrings.includes('/v1/user-input/{id}'),
    sensitiveBuilderNames: [
      'analytixThreadSummaryPath',
      'analytixThreadSummaryTaskOutputPath',
      'analytixThreadSummaryTaskKillPath',
      'analytixThreadSummaryTaskRestartPath',
      'analytixThreadEventsPath',
      'analytixThreadRewindPath',
      'analytixApprovalPath',
      'analytixUserInputPath',
      'analytixSessionResumePath',
      'analytixThreadCheckpointRewindPlanPath',
      'analytixThreadCheckpointRewindApplyPath'
    ] as const,
    unitTestEvidencePresent: sharedEndpointTestSource.includes('URL-encodes route ids') &&
      sharedEndpointTestSource.includes('analytixThreadSummaryTaskRestartPath') &&
      sharedEndpointTestSource.includes('analytixCaseProjectThreadsPath') &&
      sharedEndpointTestSource.includes("not.toContain('/v1/user-input/{id}')")
  }
  const evidence = {
    routerFirstMatch: routerSource.includes('The first matching route wins') &&
      routerSource.includes('for (const route of this.routes)'),
    structuredNotFound: httpServerSource.includes("code: 'not_found'") &&
      httpServerSource.includes("message: 'route not found'") &&
      httpServerTestSource.includes('returns 200 on /health without auth'),
    healthNoAuth: routeKeys[0] === 'GET /health' &&
      serverRoutesIndexSource.includes("router.add('GET', '/health', () => healthJsonResponse())"),
    v1AuthGuardsCoverRoutes: authGuardCount === routes.length - unauthenticatedRoutes.length,
    sseUsesBuildEventStreamResponse: serverRoutesIndexSource.includes("router.add('GET', '/v1/threads/:id/events'") &&
      serverRoutesIndexSource.includes('return buildEventStreamResponse({') &&
      sharedEndpointsSource.includes("ANALYTIX_THREAD_EVENTS_TEMPLATE = '/v1/threads/{id}/events'"),
    taskJobRoutesUseInternalHandlers: internalTaskJobRoutes.length === 3 &&
      serverRoutesIndexSource.includes('return waitTaskJobs(runtime.taskJobs, request)') &&
      serverRoutesIndexSource.includes('return readTaskJobOutput(runtime.taskJobs, request)') &&
      serverRoutesIndexSource.includes('return killTaskJob(runtime.taskJobs, request)'),
    singularUserInputCompatibility: compatibilityOnlyRoutes.length === 1 &&
      routeKeys.includes('POST /v1/user-inputs/:id') &&
      serverRoutesIndexSource.includes("router.add('POST', '/v1/user-input/:id'"),
    sharedThreadEventsTemplatePresent: sharedEndpointsSource.includes("ANALYTIX_THREAD_EVENTS_TEMPLATE = '/v1/threads/{id}/events'"),
    sharedSessionResumeTemplatePresent: sharedEndpointsSource.includes("ANALYTIX_SESSION_RESUME_TEMPLATE = '/v1/sessions/{id}/resume-thread'"),
    sharedApprovalUserInputTemplatesPresent: sharedEndpointsSource.includes("ANALYTIX_APPROVAL_TEMPLATE = '/v1/approvals/{id}'") &&
      sharedEndpointsSource.includes("ANALYTIX_USER_INPUT_TEMPLATE = '/v1/user-inputs/{id}'")
  }
  return {
    sourceContractId: 'runtime-http-route-sovereignty-v1' as const,
    sourceFiles,
    routeCount: routes.length,
    routes,
    unauthenticatedRoutes,
    authenticatedRouteCount: authGuardCount,
    authMatrix,
    sseRoutes,
    internalTaskJobRoutes,
    compatibilityOnlyRoutes,
    forbiddenRouteTokens,
    forbiddenRouteTokensPresent,
    forbiddenDispatchMatrix,
    sharedEndpointBuilderMatrix,
    evidence,
    productBoundary: {
      usesReasonixProtocol: false as const,
      topLevelHiddenEntryExposed: false as const,
      defaultGoBackendEnabled: false as const
    },
    expected: {
      routeCountExact: routes.length === 52,
      onlyHealthUnauthenticated: unauthenticatedRoutes.length === 1 &&
        authGuardCount === routes.length - 1,
      allRuntimeRoutesAnalytixOwned: routeKeys.every((route) =>
        route === 'GET /health' || route.includes(' /v1/')
      ),
      healthUnauthenticatedStatusOk: authMatrix.healthRoute === 'GET /health' &&
        authMatrix.healthStatus === 200,
      allAuthenticatedRoutesRejectMissingAuth: authMatrix.unauthorizedStatus === 401 &&
        authMatrix.protectedRouteCount === authGuardCount &&
        authMatrix.protectedRouteKeys.length === authGuardCount,
      unauthorizedBodyShapeStable: authMatrix.unauthorizedBody.code === 'unauthorized' &&
        authMatrix.unauthorizedBody.message === 'Runtime authentication is required.',
      authMatrixCoversAllRegisteredRoutes: authMatrix.protectedRouteCount + unauthenticatedRoutes.length === routes.length &&
        authMatrix.protectedRouteKeys.every((route) => routeKeys.includes(route)),
      sensitiveRoutesProtected: sensitiveRouteKeys.length === 5 &&
        sensitiveRouteKeys.every((route) => authMatrix.protectedRouteKeys.includes(route)),
      forbiddenDispatchReturnsStructuredNotFound: forbiddenDispatchMatrix.status === 404 &&
        forbiddenDispatchMatrix.body.code === 'not_found' &&
        forbiddenDispatchMatrix.body.message === 'route not found',
      forbiddenDispatchCoversAllTokens: forbiddenDispatchMatrix.tokenCount === forbiddenRouteTokens.length &&
        forbiddenDispatchMatrix.tokens.length === forbiddenRouteTokens.length &&
        forbiddenDispatchMatrix.tokens.every((token) => forbiddenRouteTokens.includes(token)),
      forbiddenProtocolTokensRejected: forbiddenProtocolTokens.length === 6 &&
        forbiddenProtocolTokens.every((token) => forbiddenDispatchMatrix.tokens.includes(token)),
      forbiddenHiddenSurfaceTokensRejected: forbiddenHiddenSurfaceTokens.length === 7 &&
        forbiddenHiddenSurfaceTokens.every((token) => forbiddenDispatchMatrix.tokens.includes(token)),
      forbiddenDispatchUsesRuntimeAuth: forbiddenDispatchMatrix.authMode === 'valid-runtime-token',
      sharedEndpointBuilderCaseCountExact: sharedEndpointBuilderCases.length === 25,
      sharedEndpointBuildersEncodeRouteIds: sharedEndpointBuilderCases.every((entry) =>
        entry.encodedSegments.every((segment) => entry.outputPath.includes(segment)) &&
        !entry.outputPath.includes('with space') &&
        !entry.outputPath.includes('?x=') &&
        !entry.outputPath.includes('#frag')
      ),
      sharedEndpointTemplatesAnalytixOwned: sharedEndpointBuilderMatrix.forbiddenTokensPresent.length === 0 &&
        exportedEndpointStrings.every((value) => value === '/health' || value.startsWith('/v1/')),
      sharedEndpointCanonicalUserInputPlural: sharedEndpointBuilderMatrix.canonicalUserInputTemplate === '/v1/user-inputs/{id}' &&
        !sharedEndpointBuilderMatrix.singularUserInputTemplateExported,
      sharedEndpointSensitiveBuildersPresent: sharedEndpointBuilderMatrix.sensitiveBuilderNames.every((name) =>
        sharedEndpointBuilderCases.some((entry) => entry.name === name)
      ),
      sharedEndpointBuilderUnitEvidencePresent: sharedEndpointBuilderMatrix.unitTestEvidencePresent,
      sseRouteAnalytixThreadEvents: evidence.sseUsesBuildEventStreamResponse &&
        sseRoutes.length === 1,
      threadLifecycleRoutesPresent: threadLifecycleRoutes.every((route) => routeKeys.includes(route)),
      approvalUserInputRoutesPresent: approvalUserInputRoutes.every((route) => routeKeys.includes(route)),
      taskJobRoutesInternalOnly: evidence.taskJobRoutesUseInternalHandlers,
      notFoundStructured: evidence.structuredNotFound,
      noForbiddenRoutes: forbiddenRouteTokensPresent.length === 0,
      singularUserInputCompatibilityOnly: evidence.singularUserInputCompatibility,
      usesReasonixProtocol: false as const,
      topLevelHiddenEntryExposed: false as const,
      defaultGoBackendEnabled: false as const
    }
  }
}

function requestForRegisteredRoute(route: { method: string; path: string }): Request {
  const replacementValues: Record<string, string> = {
    id: 'thr_auth_matrix',
    checkpointId: 'axcp_auth_matrix',
    turnId: 'turn_auth_matrix'
  }
  const path = route.path.replace(/:([A-Za-z][A-Za-z0-9]*)/g, (_match, key: string) =>
    replacementValues[key] ?? `${key}_auth_matrix`
  )
  return new Request(`http://localhost${path}`, { method: route.method })
}

function authorizedRuntimeRequest(path: string, method = 'GET'): Request {
  return new Request(`http://localhost${path}`, {
    method,
    headers: { authorization: 'Bearer tok-1' }
  })
}

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

async function waitFor(assertion: () => Promise<boolean> | boolean): Promise<void> {
  const deadline = Date.now() + 1000
  while (Date.now() < deadline) {
    if (await assertion()) return
    await new Promise((resolve) => setTimeout(resolve, 5))
  }
  throw new Error('condition was not met')
}

async function putG5CommandThread(
  h: ReturnType<typeof buildHarness>,
  thread: ThreadRecord,
  input: { cwd: string; command: string; callId: string }
): Promise<void> {
  const now = h.nowIso()
  await h.threadStore.upsert({
    ...thread,
    turns: [{
      id: 'turn_g5_restart',
      threadId: thread.id,
      status: 'completed',
      prompt: 'restart command',
      steering: [],
      createdAt: now,
      attachmentIds: [],
      activeSkillIds: [],
      injectedMemoryIds: [],
      items: [
        {
          id: `item_call_${thread.id}`,
          threadId: thread.id,
          turnId: 'turn_g5_restart',
          role: 'assistant',
          kind: 'tool_call',
          toolName: 'bash',
          toolKind: 'command_execution',
          callId: input.callId,
          status: 'completed',
          arguments: makePublicToolCallArgumentsProjection(),
          createdAt: now
        },
        {
          id: `item_result_${thread.id}`,
          threadId: thread.id,
          turnId: 'turn_g5_restart',
          role: 'tool',
          kind: 'tool_result',
          toolName: 'bash',
          toolKind: 'command_execution',
          callId: input.callId,
          status: 'completed',
          isError: false,
          output: makePublicToolResultWithheldProjection({ status: 'completed' }),
          createdAt: now,
          finishedAt: now
        }
      ]
    }]
  })
}

function desktopSovereigntyControlCase() {
  const sourceFiles = {
    preload: 'src/preload/index.ts' as const,
    windowTypes: 'src/preload/index.d.ts' as const,
    sharedApi: 'src/shared/analytix-api.ts' as const,
    settingsStoreTest: 'src/main/settings-store.test.ts' as const,
    runtimeNormalizer: 'src/shared/app-settings-runtime.ts' as const,
    productSovereigntyScan: 'scripts/scan-product-sovereignty.cjs' as const,
    rendererSourceRoot: 'src/renderer/src' as const,
    rendererRuntimeClient: 'src/renderer/src/agent/runtime-client.ts' as const,
    rendererRuntimeClientTest: 'src/renderer/src/agent/runtime-client.test.ts' as const,
    rendererProvider: 'src/renderer/src/agent/analytix-runtime.ts' as const,
    rendererProviderTest: 'src/renderer/src/agent/analytix-runtime.test.ts' as const,
    rendererAgentTypes: 'src/renderer/src/agent/types.ts' as const,
    rendererSideActions: 'src/renderer/src/store/chat-store-side-actions.ts' as const,
    rendererSideActionsTest: 'src/renderer/src/store/chat-store-side-actions.test.ts' as const,
    rendererThreadUsage: 'src/renderer/src/hooks/use-thread-usage.ts' as const,
    rendererThreadUsageTest: 'src/renderer/src/hooks/use-thread-usage.test.ts' as const,
    rendererDailyUsage: 'src/renderer/src/hooks/use-daily-usage.ts' as const,
    rendererDailyUsageTest: 'src/renderer/src/hooks/use-daily-usage.test.ts' as const,
    rendererModelUsage: 'src/renderer/src/hooks/use-model-usage.ts' as const,
    rendererModelUsageTest: 'src/renderer/src/hooks/use-model-usage.test.ts' as const,
    rendererSettingsAgents: 'src/renderer/src/components/settings-section-agents.tsx' as const,
    rendererLlmDebug: 'src/renderer/src/components/settings-section-llm-debug.tsx' as const,
    rendererKeyboardShortcutSettings: 'src/renderer/src/lib/keyboard-shortcut-settings.ts' as const,
    rendererVoiceDictation: 'src/renderer/src/components/chat/use-voice-dictation.ts' as const,
    rendererInitialUsageHeatmap: 'src/renderer/src/components/chat/InitialSessionUsageHeatmap.tsx' as const,
    preloadRuntimeRequestTest: 'src/preload/preload-runtime-request.test.ts' as const,
    preloadSseBridgeTest: 'src/preload/preload-sse-bridge.test.ts' as const
  }
  const preloadSource = repoSource(sourceFiles.preload)
  const windowTypeSource = repoSource(sourceFiles.windowTypes)
  const sharedApiSource = repoSource(sourceFiles.sharedApi)
  const settingsStoreTestSource = repoSource(sourceFiles.settingsStoreTest)
  const runtimeNormalizerSource = repoSource(sourceFiles.runtimeNormalizer)
  const productSovereigntyScanSource = repoSource(sourceFiles.productSovereigntyScan)
  const rendererRuntimeClientSource = repoSource(sourceFiles.rendererRuntimeClient)
  const rendererRuntimeClientTestSource = repoSource(sourceFiles.rendererRuntimeClientTest)
  const rendererProviderSource = repoSource(sourceFiles.rendererProvider)
  const rendererProviderTestSource = repoSource(sourceFiles.rendererProviderTest)
  const rendererAgentTypesSource = repoSource(sourceFiles.rendererAgentTypes)
  const rendererSideActionsSource = repoSource(sourceFiles.rendererSideActions)
  const rendererSideActionsTestSource = repoSource(sourceFiles.rendererSideActionsTest)
  const rendererThreadUsageSource = repoSource(sourceFiles.rendererThreadUsage)
  const rendererThreadUsageTestSource = repoSource(sourceFiles.rendererThreadUsageTest)
  const rendererDailyUsageSource = repoSource(sourceFiles.rendererDailyUsage)
  const rendererDailyUsageTestSource = repoSource(sourceFiles.rendererDailyUsageTest)
  const rendererModelUsageSource = repoSource(sourceFiles.rendererModelUsage)
  const rendererModelUsageTestSource = repoSource(sourceFiles.rendererModelUsageTest)
  const rendererSettingsAgentsSource = repoSource(sourceFiles.rendererSettingsAgents)
  const rendererLlmDebugSource = repoSource(sourceFiles.rendererLlmDebug)
  const rendererKeyboardShortcutSettingsSource = repoSource(sourceFiles.rendererKeyboardShortcutSettings)
  const rendererVoiceDictationSource = repoSource(sourceFiles.rendererVoiceDictation)
  const rendererInitialUsageHeatmapSource = repoSource(sourceFiles.rendererInitialUsageHeatmap)
  const preloadRuntimeRequestTestSource = repoSource(sourceFiles.preloadRuntimeRequestTest)
  const preloadSseBridgeTestSource = repoSource(sourceFiles.preloadSseBridgeTest)
  const rendererSources = repoProductionSources(sourceFiles.rendererSourceRoot)
    .map((relativePath) => repoSource(relativePath))
  const rendererSource = rendererSources.join('\n')
  const exposedBridgeNames = Array.from(
    preloadSource.matchAll(/contextBridge\.exposeInMainWorld\(\s*(['"`])([^'"`]+)\1/g)
  ).map((match) => match[2])
  const windowInterface = windowTypeSource.match(/interface Window\s*{([\s\S]*?)^\s*}/m)
  const windowTypeProperties = Array.from(
    windowInterface?.[1].matchAll(/^\s*([A-Za-z_$][\w$]*)\??\s*:/gm) ?? []
  ).map((match) => match[1])
  const facade = sharedApiSource.match(/export type AnalytixDomainFacade = \{([\s\S]*?)^\}/m)
  const facadeDomains = Array.from(
    facade?.[1].matchAll(/^\s*([A-Za-z_$][\w$]*)\s*:/gm) ?? []
  ).map((match) => match[1])
  const forbiddenBridgeAliases = [
    'analytixGui',
    'deepseek',
    'deepseekGui',
    'kun',
    'reasonix'
  ] as const
  const forbiddenRuntimeIpcChannels = [
    'reasonix:request',
    'kun:request',
    'runtime:go:request',
    'workflow:request',
    'reasonix:sse',
    'kun:sse',
    'runtime:go:sse',
    'workflow:sse'
  ] as const
  const forbiddenExportedApiNames = [
    'AnalytixGuiApi',
    'DeepSeekApi',
    'KunApi',
    'KunGuiApi',
    'ReasonixApi',
    'ReasonixSessionAPI'
  ] as const
  const forbiddenSettingsKeys = [
    'agent',
    'agentProvider',
    'agents',
    'autoPlan',
    'auto_plan',
    'deepseek',
    'reasonix'
  ] as const
  const allowedNamedRuntimeApis = [
    'acceptedSlotDisplay',
    'cancelFundsCSVImport',
    'cleaningDiffPreview',
    'confirmFundsCSVSnapshot',
    'directSourcePreview',
    'fetchUpstreamModels',
    'getAnalytixConfigFile',
    'importMappingPreview',
    'onRuntimeStatus',
    'openAnalytixConfigDir',
    'probeModelCapabilities',
    'restartRuntime',
    'revokeCleaningDiffPreview',
    'runDeterministicFundsCleaning',
    'stageFundsCSVSnapshot',
    'statusFundsCSVImport',
    'setAnalytixConfigFile'
  ] as const
  const allowedNamedSettingsApis = [
    'saveSettingsSilent',
    'setSettings'
  ] as const
  const forbiddenRuntimeIpcChannelsExposed = forbiddenRuntimeIpcChannels.filter((channel) =>
    preloadSource.includes(channel)
  )
  const publicApiTypesAnalytixOwned = sharedApiSource.includes('export type AnalytixApi') &&
    !forbiddenExportedApiNames.some((name) =>
      new RegExp(`\\bexport\\s+(?:type|interface)\\s+${name}\\b`).test(sharedApiSource)
    )
  const runtimeRequestUsesAnalytixIpc =
    preloadSource.includes("runtimeRequest: (path, method, body) =>") &&
    preloadSource.includes("ipcRenderer.invoke('runtime:request', { path, method, body })") &&
    !forbiddenRuntimeIpcChannelsExposed.some((channel) => channel.endsWith(':request'))
  const runtimeSseUsesAnalytixIpc =
    preloadSource.includes("startSse: (threadId, sinceSeq, streamId) =>") &&
    preloadSource.includes("ipcRenderer.invoke('runtime:sse:start', { threadId, sinceSeq, streamId })") &&
    preloadSource.includes("stopSse: (streamId) => ipcRenderer.invoke('runtime:sse:stop', streamId)") &&
    !forbiddenRuntimeIpcChannelsExposed.some((channel) => channel.endsWith(':sse'))
  const directBridgeAccesses = Array.from(
    rendererSource.matchAll(/window\.analytix(?:\?\.|\.)(runtime|settings)(?:\?\.|\.)([A-Za-z0-9_]+)/g)
  ).map((match) => ({ domain: match[1], method: match[2] }))
  const directRuntimeApis = sortedUnique(
    directBridgeAccesses
      .filter((access) => access.domain === 'runtime')
      .map((access) => access.method)
  )
  const directSettingsApis = sortedUnique(
    directBridgeAccesses
      .filter((access) => access.domain === 'settings')
      .map((access) => access.method)
  )
  const directRuntimeViolations = directRuntimeApis.filter((method) =>
    !allowedNamedRuntimeApis.includes(method as typeof allowedNamedRuntimeApis[number])
  )
  const directSettingsViolations = directSettingsApis.filter((method) =>
    !allowedNamedSettingsApis.includes(method as typeof allowedNamedSettingsApis[number])
  )
  const genericRuntimeBypassCount = Array.from(
    rendererSource.matchAll(/window\.analytix(?:\?\.|\.)(?:runtime)(?:\?\.|\.)runtimeRequest/g)
  ).length
  const settingsReadBypassCount = Array.from(
    rendererSource.matchAll(/window\.analytix(?:\?\.|\.)(?:settings)(?:\?\.|\.)getSettings/g)
  ).length
  const optionalChainPatternCovered =
    productSovereigntyScanSource.includes('(?:\\\\?\\\\.|\\\\.)runtime') &&
    productSovereigntyScanSource.includes('(?:\\\\?\\\\.|\\\\.)settings') &&
    productSovereigntyScanSource.includes('directRuntimeRequestBridgePattern') &&
    productSovereigntyScanSource.includes('directSettingsGetBridgePattern')
  const rendererEndpointThreadId = 'thr/route?x=1#frag'
  const rendererEndpointTurnId = 'turn/route?x=1#frag'
  const rendererEndpointApprovalId = 'appr/route?x=1#frag'
  const rendererEndpointInputId = 'input/route?x=1#frag'
  const rendererEndpointSessionId = 'sess/route?x=1#frag'
  const encodedRendererThread = encodeURIComponent(rendererEndpointThreadId)
  const encodedRendererTurn = encodeURIComponent(rendererEndpointTurnId)
  const encodedRendererApproval = encodeURIComponent(rendererEndpointApprovalId)
  const encodedRendererInput = encodeURIComponent(rendererEndpointInputId)
  const encodedRendererSession = encodeURIComponent(rendererEndpointSessionId)
  const rendererRuntimeClientThreadId = 'thr/with space?x=1#frag'
  const rendererRuntimeClientTurnId = 'turn/with space?x=1#frag'
  const encodedRendererRuntimeClientThread = encodeURIComponent(rendererRuntimeClientThreadId)
  const encodedRendererRuntimeClientTurn = encodeURIComponent(rendererRuntimeClientTurnId)
  const rendererRuntimeClientBridgeMatrix = {
    sourceIds: {
      threadId: rendererRuntimeClientThreadId,
      turnId: rendererRuntimeClientTurnId
    },
    encodedIds: {
      threadId: encodedRendererRuntimeClientThread,
      turnId: encodedRendererRuntimeClientTurn
    },
    runtimeRequestCalls: [
      { path: `/v1/threads/${encodedRendererRuntimeClientThread}/turns/${encodedRendererRuntimeClientTurn}/steer`, argumentCount: 1 as const },
      { path: `/v1/threads/${encodedRendererRuntimeClientThread}/turns/${encodedRendererRuntimeClientTurn}/steer`, method: 'POST' as const, argumentCount: 2 as const },
      { path: `/v1/threads/${encodedRendererRuntimeClientThread}/turns/${encodedRendererRuntimeClientTurn}/steer`, method: 'POST' as const, body: '{"action":"step"}', argumentCount: 3 as const }
    ],
    sseStartCall: {
      threadId: rendererRuntimeClientThreadId,
      sinceSeq: 7 as const,
      streamId: 'stream-renderer' as const
    },
    sseStopCall: {
      streamId: 'stream-renderer' as const
    },
    listenerApis: ['onSseEvent', 'onSseEnd', 'onSseError'] as const,
    sourceUsesAnalytixRuntime: rendererRuntimeClientSource.includes('return window.analytix.runtime') &&
      !rendererRuntimeClientSource.includes('window.kun') &&
      !rendererRuntimeClientSource.includes('window.reasonix'),
    sourcePassesRuntimeRequestArgumentsUnchanged: rendererRuntimeClientSource.includes('runtimeRequest(path: string, method?: string, body?: string)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().runtimeRequest(path)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().runtimeRequest(path, method)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().runtimeRequest(path, method, body)'),
    sourcePassesRestartUnchanged: rendererRuntimeClientSource.includes('restartRuntime(): Promise<void>') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().restartRuntime()'),
    sourcePassesSseControlsUnchanged: rendererRuntimeClientSource.includes('startSse(threadId: string, sinceSeq: number, streamId?: string)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().startSse(threadId, sinceSeq, streamId)') &&
      rendererRuntimeClientSource.includes('stopSse(streamId: string): Promise<boolean>') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().stopSse(streamId)'),
    sourcePassesSseListenersUnchanged: rendererRuntimeClientSource.includes('onSseEvent(handler: (payload: SseEventPayload) => void)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().onSseEvent(handler)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().onSseEnd(handler)') &&
      rendererRuntimeClientSource.includes('return this.runtimeApi().onSseError(handler)'),
    runtimeRequestUnitEvidencePresent: rendererRuntimeClientTestSource.includes('passes runtime requests through window.analytix without rewriting arguments') &&
      rendererRuntimeClientTestSource.includes('await expect(rendererRuntimeClient.runtimeRequest(path)).resolves.toEqual(response)') &&
      rendererRuntimeClientTestSource.includes("expect(runtimeRequest).toHaveBeenNthCalledWith(3, path, 'POST', '{\"action\":\"step\"}')"),
    restartUnitEvidencePresent: rendererRuntimeClientTestSource.includes('forwards explicit runtime restarts through the preload bridge') &&
      rendererRuntimeClientTestSource.includes('expect(restartRuntime).toHaveBeenCalledTimes(1)'),
    sseUnitEvidencePresent: rendererRuntimeClientTestSource.includes('passes SSE control and listener handlers through window.analytix unchanged') &&
      rendererRuntimeClientTestSource.includes("expect(startSse).toHaveBeenCalledWith('thr/with space?x=1#frag', 7, 'stream-renderer')") &&
      rendererRuntimeClientTestSource.includes('expect(onSseError).toHaveBeenCalledWith(errorHandler)'),
    legacyAliasUnitEvidencePresent: rendererRuntimeClientTestSource.includes('legacy bridge alias should not be read') &&
      rendererRuntimeClientTestSource.includes('get kun()') &&
      rendererRuntimeClientTestSource.includes('get reasonix()')
  }
  const rendererSettingsBridgeMatrix = {
    patch: {
      workspaceRoot: '/tmp/next' as const,
      runtimeModel: 'deepseek-reasoner' as const,
      approvalPolicy: 'never' as const
    },
    cacheExpectations: {
      getSettingsCallsForDoubleRead: 1 as const,
      getSettingsCallsAfterSetSettings: 1 as const,
      setSettingsCallsAfterWrite: 1 as const
    },
    sourceUsesAnalytixSettings: rendererRuntimeClientSource.includes('return window.analytix.settings') &&
      !rendererRuntimeClientSource.includes('window.kun') &&
      !rendererRuntimeClientSource.includes('window.reasonix'),
    sourceCachesSettingsReads: rendererRuntimeClientSource.includes('desktopQueryCache.fetchQuery({') &&
      rendererRuntimeClientSource.includes('queryKey: SETTINGS_QUERY_KEY') &&
      rendererRuntimeClientSource.includes('staleTimeMs: SETTINGS_STALE_TIME_MS') &&
      rendererRuntimeClientSource.includes('fetcher: () => this.settingsApi().getSettings()'),
    sourceRefreshesCacheAfterSetSettings: rendererRuntimeClientSource.includes('async setSettings(partial: AppSettingsPatch)') &&
      rendererRuntimeClientSource.includes('const settings = await this.settingsApi().setSettings(partial)') &&
      rendererRuntimeClientSource.includes('desktopQueryCache.setQueryData(SETTINGS_QUERY_KEY, settings, SETTINGS_STALE_TIME_MS)') &&
      rendererRuntimeClientSource.includes('desktopQueryCache.broadcastInvalidation(SETTINGS_QUERY_KEY)'),
    cacheUnitEvidencePresent: rendererRuntimeClientTestSource.includes('caches settings reads until invalidated') &&
      rendererRuntimeClientTestSource.includes('expect(getSettings).toHaveBeenCalledTimes(1)'),
    refreshUnitEvidencePresent: rendererRuntimeClientTestSource.includes('refreshes the cache after setSettings') &&
      rendererRuntimeClientTestSource.includes('expect(setSettings).toHaveBeenCalledTimes(1)'),
    topLevelRuntimePatchUnitEvidencePresent: rendererRuntimeClientTestSource.includes('passes top-level runtime settings patches through window.analytix.settings without legacy aliases') &&
      rendererRuntimeClientTestSource.includes("model: 'deepseek-reasoner'") &&
      rendererRuntimeClientTestSource.includes("approvalPolicy: 'never'") &&
      rendererRuntimeClientTestSource.includes('expect(setSettings).toHaveBeenCalledWith(patch)'),
    legacyAliasUnitEvidencePresent: rendererRuntimeClientTestSource.includes('legacy bridge alias should not be read') &&
      rendererRuntimeClientTestSource.includes('get kun()') &&
      rendererRuntimeClientTestSource.includes('get reasonix()')
  }
  const rendererProviderEndpointMatrix = {
    rootPaths: {
      health: '/health' as const,
      threads: '/v1/threads' as const
    },
    rootConstantsUsed: rendererProviderSource.includes('ANALYTIX_HEALTH_PATH') &&
      rendererProviderSource.includes('ANALYTIX_THREADS_PATH') &&
      rendererProviderSource.includes('runtimeRequest(ANALYTIX_HEALTH_PATH') &&
      rendererProviderSource.includes('runtimeRequest(`${ANALYTIX_THREADS_PATH}?limit=1`') &&
      rendererProviderSource.includes('runtimeRequest(`${ANALYTIX_THREADS_PATH}${query}`') &&
      rendererProviderSource.includes('runtimeRequest(\n      ANALYTIX_THREADS_PATH,'),
    sourceIds: {
      threadId: rendererEndpointThreadId,
      turnId: rendererEndpointTurnId,
      approvalId: rendererEndpointApprovalId,
      inputId: rendererEndpointInputId,
      sessionId: rendererEndpointSessionId
    },
    encodedIds: {
      threadId: encodedRendererThread,
      turnId: encodedRendererTurn,
      approvalId: encodedRendererApproval,
      inputId: encodedRendererInput,
      sessionId: encodedRendererSession
    },
    encodedRuntimeRequestPaths: [
      `/v1/threads/${encodedRendererThread}/turns`,
      `/v1/threads/${encodedRendererThread}/turns/${encodedRendererTurn}/steer`,
      `/v1/threads/${encodedRendererThread}/turns/${encodedRendererTurn}/interrupt`,
      `/v1/threads/${encodedRendererThread}/compact`,
      `/v1/approvals/${encodedRendererApproval}`,
      `/v1/user-inputs/${encodedRendererInput}`,
      `/v1/threads/${encodedRendererThread}/fork`,
      `/v1/sessions/${encodedRendererSession}/resume-thread`
    ],
    sensitivePathKinds: [
      'turns',
      'steer',
      'interrupt',
      'compact',
      'approval',
      'user-input',
      'fork',
      'resume-thread'
    ] as const,
    usesRendererRuntimeClientFacade: rendererProviderSource.includes('rendererRuntimeClient.runtimeRequest') &&
      !rendererProviderSource.includes('window.analytix.runtime.runtimeRequest'),
    pathOwnershipGuardPresent: rendererProviderTestSource.includes('expectRuntimeRequestPathsStayAnalytixOwned(paths)') &&
      rendererProviderTestSource.includes('FORBIDDEN_PUBLIC_RUNTIME_ROUTE'),
    unitTestEvidencePresent: rendererProviderTestSource.includes('URL-encodes dynamic runtime route ids before calling the bridge') &&
      rendererProviderTestSource.includes('encodedThread') &&
      rendererProviderTestSource.includes('encodedSession')
  }
  const rendererProviderAliasGuardMatrix = {
    helperName: 'installDsGui' as const,
    forbiddenAliases: ['kun', 'reasonix'] as const,
    guardedTestNames: [
      'keeps renderer runtime requests on analytix-owned HTTP routes',
      'uses Analytix thread lifecycle endpoints for list, archive, restore, rename, workspace, and delete',
      'calls Analytix fork and user-input compatibility endpoints',
      'URL-encodes dynamic runtime route ids before calling the bridge',
      'resumes a session through the Analytix HTTP runtime'
    ] as const,
    coverage: {
      routeOwnership: true as const,
      threadLifecycle: true as const,
      approvalUserInput: true as const,
      forkResume: true as const,
      dynamicRouteEncoding: true as const
    },
    sourceInstallsThrowingAliases: rendererProviderTestSource.includes('get kun()') &&
      rendererProviderTestSource.includes('get reasonix()') &&
      rendererProviderTestSource.includes('legacy runtime provider alias should not be read'),
    sourceUsesAnalytixOnly: rendererProviderSource.includes('rendererRuntimeClient.runtimeRequest') &&
      !rendererProviderSource.includes('window.kun') &&
      !rendererProviderSource.includes('window.reasonix') &&
      !rendererProviderSource.includes('window.analytix.runtime.runtimeRequest'),
    routeOwnershipEvidencePresent: rendererProviderTestSource.includes('keeps renderer runtime requests on analytix-owned HTTP routes') &&
      rendererProviderTestSource.includes('expectRuntimeRequestPathsStayAnalytixOwned(paths)') &&
      rendererProviderTestSource.includes('FORBIDDEN_PUBLIC_RUNTIME_ROUTE'),
    lifecycleEvidencePresent: rendererProviderTestSource.includes('uses Analytix thread lifecycle endpoints for list, archive, restore, rename, workspace, and delete') &&
      rendererProviderTestSource.includes('/v1/threads/thr_archived') &&
      rendererProviderTestSource.includes("JSON.stringify({ relation: 'primary' })"),
    approvalUserInputEvidencePresent: rendererProviderTestSource.includes('submitApprovalDecision') &&
      rendererProviderTestSource.includes('submitUserInputResponse') &&
      rendererProviderTestSource.includes('cancelUserInput') &&
      rendererProviderTestSource.includes('/v1/approvals/appr_surface') &&
      rendererProviderTestSource.includes('/v1/user-inputs/input_surface'),
    forkResumeEvidencePresent: rendererProviderTestSource.includes('calls Analytix fork and user-input compatibility endpoints') &&
      rendererProviderTestSource.includes('forkThread') &&
      rendererProviderTestSource.includes('resumeSession') &&
      rendererProviderTestSource.includes('/resume-thread'),
    dynamicEncodingEvidencePresent: rendererProviderTestSource.includes('URL-encodes dynamic runtime route ids before calling the bridge') &&
      rendererProviderTestSource.includes('encodedThread') &&
      rendererProviderTestSource.includes('encodedSession') &&
      rendererProviderTestSource.includes("expect(path).not.toContain(threadId)"),
    forbiddenRouteGuardPresent: rendererProviderTestSource.includes('FORBIDDEN_PUBLIC_RUNTIME_ROUTE') &&
      rendererProviderTestSource.includes('expect(path).not.toMatch(FORBIDDEN_PUBLIC_RUNTIME_ROUTE)')
  }
  const rendererProviderFacadeSealMatrix = {
    facade: 'rendererRuntimeClient.runtimeRequest' as const,
    forbiddenDirectBridge: 'window.analytix.runtime.runtimeRequest' as const,
    sealedMethods: ['archiveThread', 'updateThreadRelation'] as const,
    sourceUsesRuntimeClient: rendererProviderSource.includes('rendererRuntimeClient.runtimeRequest'),
    sourceRejectsDirectBridgeBypass: !rendererProviderSource.includes('window.analytix.runtime.runtimeRequest'),
    archiveRestoreSourceEvidencePresent: rendererProviderSource.includes('async archiveThread(threadId: string, archived: boolean)') &&
      rendererProviderSource.includes("JSON.stringify({ status: archived ? 'archived' : 'idle' })") &&
      rendererProviderSource.includes('archive thread failed'),
    relationSourceEvidencePresent: rendererProviderSource.includes("updateThreadRelation(threadId: string, relation: NonNullable<NormalizedThread['relation']>)") &&
      rendererProviderSource.includes('JSON.stringify({ relation })') &&
      rendererProviderSource.includes('update thread relation failed'),
    lifecycleUnitEvidencePresent: rendererProviderTestSource.includes('uses Analytix thread lifecycle endpoints for list, archive, restore, rename, workspace, and delete') &&
      rendererProviderTestSource.includes("JSON.stringify({ status: 'archived' })") &&
      rendererProviderTestSource.includes("JSON.stringify({ status: 'idle' })") &&
      rendererProviderTestSource.includes("JSON.stringify({ relation: 'primary' })"),
    scanGuardPresent: productSovereigntyScanSource.includes('renderer direct runtime bridge bypass scan') &&
      productSovereigntyScanSource.includes('directRuntimeRequestBridgePattern') &&
      productSovereigntyScanSource.includes('window\\\\.analytix(?:\\\\?\\\\.|\\\\.)runtime(?:\\\\?\\\\.|\\\\.)runtimeRequest') &&
      productSovereigntyScanSource.includes('src/renderer/src'),
    providerFacadeTokenScanPresent: productSovereigntyScanSource.includes('post-881 renderer runtime provider facade contract token scan') &&
      productSovereigntyScanSource.includes('archiveThread\\\\(threadId: string, archived: boolean\\\\)') &&
      productSovereigntyScanSource.includes("updateThreadRelation\\\\(threadId: string, relation: NonNullable<NormalizedThread\\\\['relation'\\\\]>\\\\)") &&
      productSovereigntyScanSource.includes('rendererRuntimeClient\\\\.runtimeRequest')
  }
  const sideConversationRelationContractMatrix = {
    providerMethod: 'updateThreadRelation' as const,
    storeAction: 'promoteSideConversation' as const,
    relation: 'primary' as const,
    providerContractOptional: rendererAgentTypesSource.includes("updateThreadRelation?(threadId: string, relation: NonNullable<NormalizedThread['relation']>): Promise<void>"),
    providerImplementationUsesRuntimeClient: rendererProviderFacadeSealMatrix.relationSourceEvidencePresent &&
      rendererProviderFacadeSealMatrix.sourceRejectsDirectBridgeBypass,
    storeUsesProviderContract: rendererSideActionsSource.includes("typeof provider.updateThreadRelation !== 'function'") &&
      rendererSideActionsSource.includes("await provider.updateThreadRelation(sideId, 'primary')"),
    storeRefreshesAndCloses: rendererSideActionsSource.includes('await ctx.get().refreshThreads()') &&
      rendererSideActionsSource.includes('await actions.closeSideConversation(sideId)'),
    storeRejectsDirectRuntimeBridge: !rendererSideActionsSource.includes('window.analytix.runtime.runtimeRequest'),
    unitEvidencePresent: rendererSideActionsTestSource.includes('promoteSideConversation clears the relation through the provider and refreshes the thread list') &&
      rendererSideActionsTestSource.includes("expect(provider.relationMock).toHaveBeenCalledWith(id, 'primary')") &&
      rendererSideActionsTestSource.includes('expect(provider.refreshThreadsMock).toHaveBeenCalledTimes(1)') &&
      rendererSideActionsTestSource.includes('expect(state.sideConversations[id]).toBeUndefined()'),
    scanGuardPresent: productSovereigntyScanSource.includes('post-881 side conversation provider relation contract token scan') &&
      productSovereigntyScanSource.includes("updateThreadRelation\\\\(sideId, 'primary'\\\\)") &&
      productSovereigntyScanSource.includes('relationMock') &&
      productSovereigntyScanSource.includes('chat-store-side-actions.ts')
  }
  const rendererUsageFacadeSources = [
    rendererThreadUsageSource,
    rendererDailyUsageSource,
    rendererModelUsageSource,
    rendererSettingsAgentsSource,
    rendererLlmDebugSource
  ].join('\n')
  const rendererUsageRuntimeClientFacadeMatrix = {
    facade: 'rendererRuntimeClient.runtimeRequest' as const,
    forbiddenDirectBridge: 'window.analytix.runtime.runtimeRequest' as const,
    usageLoaders: ['loadThreadUsage', 'loadDailyUsage', 'loadModelUsage'] as const,
    diagnosticsLoaders: ['loadTokenEconomySavingsSummary', 'LlmDebugSettingsSection'] as const,
    threadUsageSourceUsesRuntimeClient: rendererThreadUsageSource.includes('loadThreadUsage(threadId: string)') &&
      rendererThreadUsageSource.includes("rendererRuntimeClient.runtimeRequest(`/v1/usage?${params.toString()}`, 'GET')"),
    dailyUsageSourceUsesRuntimeClient: rendererDailyUsageSource.includes('loadDailyUsage(range: DailyUsageRange)') &&
      rendererDailyUsageSource.includes("rendererRuntimeClient.runtimeRequest(buildDailyUsagePath(range), 'GET')"),
    modelUsageSourceUsesRuntimeClient: rendererModelUsageSource.includes('loadModelUsage(range: DailyUsageRange)') &&
      rendererModelUsageSource.includes("rendererRuntimeClient.runtimeRequest(buildModelUsagePath(range), 'GET')"),
    tokenEconomySourceUsesRuntimeClient: rendererSettingsAgentsSource.includes('loadTokenEconomySavingsSummary()') &&
      rendererSettingsAgentsSource.includes("rendererRuntimeClient.runtimeRequest('/v1/usage?group_by=thread', 'GET')"),
    llmDebugSourceUsesRuntimeClient: rendererLlmDebugSource.includes('LlmDebugSettingsSection') &&
      rendererLlmDebugSource.includes("rendererRuntimeClient.runtimeRequest('/v1/debug/llm-rounds', 'GET')"),
    sourceRejectsDirectBridgeBypass: !rendererUsageFacadeSources.includes('window.analytix.runtime.runtimeRequest'),
    usageUnitEvidencePresent: rendererThreadUsageTestSource.includes('requests only the selected thread usage bucket') &&
      rendererThreadUsageTestSource.includes("expect(runtimeRequest).toHaveBeenCalledWith(threadUsagePath('thr_native_cache'), 'GET')") &&
      rendererDailyUsageTestSource.includes('loads daily usage from the runtime request bridge') &&
      rendererDailyUsageTestSource.includes('/v1/usage?group_by=day&from=2026-05-01&to=2026-05-01&timezone=UTC') &&
      rendererModelUsageTestSource.includes('loads model usage from the runtime request bridge') &&
      rendererModelUsageTestSource.includes('/v1/usage?group_by=model&from=2026-05-01&to=2026-05-01&timezone=UTC'),
    scanGuardPresent: productSovereigntyScanSource.includes('renderer direct runtime bridge bypass scan') &&
      productSovereigntyScanSource.includes('directRuntimeRequestBridgePattern') &&
      productSovereigntyScanSource.includes('window\\\\.analytix(?:\\\\?\\\\.|\\\\.)runtime(?:\\\\?\\\\.|\\\\.)runtimeRequest') &&
      productSovereigntyScanSource.includes('src/renderer/src'),
    usageFacadeTokenScanPresent: productSovereigntyScanSource.includes('post-881 renderer usage runtime client facade contract token scan') &&
      productSovereigntyScanSource.includes('loadThreadUsage\\\\(threadId: string\\\\)') &&
      productSovereigntyScanSource.includes('loadDailyUsage\\\\(range: DailyUsageRange\\\\)') &&
      productSovereigntyScanSource.includes('loadModelUsage\\\\(range: DailyUsageRange\\\\)') &&
      productSovereigntyScanSource.includes('loadTokenEconomySavingsSummary\\\\(\\\\)') &&
      productSovereigntyScanSource.includes('rendererRuntimeClient\\\\.runtimeRequest')
  }
  const rendererSettingsReadFacadeSources = [
    rendererKeyboardShortcutSettingsSource,
    rendererVoiceDictationSource,
    rendererInitialUsageHeatmapSource
  ].join('\n')
  const rendererSettingsReadFacadeMatrix = {
    facade: 'rendererRuntimeClient.getSettings' as const,
    forbiddenDirectBridge: 'window.analytix.settings.getSettings' as const,
    settingsReaders: ['useKeyboardShortcutSettings', 'useSpeechToTextEnabled', 'InitialSessionUsageHeatmapView'] as const,
    keyboardShortcutSourceUsesSettingsClient: rendererKeyboardShortcutSettingsSource.includes('useKeyboardShortcutSettings()') &&
      rendererKeyboardShortcutSettingsSource.includes('rendererRuntimeClient.getSettings().then(apply)'),
    speechToTextSourceUsesSettingsClient: rendererVoiceDictationSource.includes('useSpeechToTextEnabled()') &&
      rendererVoiceDictationSource.includes('rendererRuntimeClient.getSettings().then(apply)'),
    usageHeatmapSourceUsesSettingsClient: rendererInitialUsageHeatmapSource.includes('rendererRuntimeClient.getSettings()') &&
      rendererInitialUsageHeatmapSource.includes('setModelLabel(settings.runtime.model.trim())'),
    settingsChangedEventPreserved: rendererKeyboardShortcutSettingsSource.includes("export const SETTINGS_CHANGED_EVENT = 'analytix:settings-changed'") &&
      rendererKeyboardShortcutSettingsSource.includes('window.addEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)') &&
      rendererVoiceDictationSource.includes('window.addEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)'),
    sourceRejectsDirectSettingsBypass: !rendererSettingsReadFacadeSources.includes('window.analytix.settings.getSettings'),
    scanGuardPresent: productSovereigntyScanSource.includes('renderer direct settings read bypass scan') &&
      productSovereigntyScanSource.includes('directSettingsGetBridgePattern') &&
      productSovereigntyScanSource.includes('window\\\\.analytix(?:\\\\?\\\\.|\\\\.)settings(?:\\\\?\\\\.|\\\\.)getSettings') &&
      productSovereigntyScanSource.includes('src/renderer/src'),
    settingsReadFacadeTokenScanPresent: productSovereigntyScanSource.includes('post-881 renderer settings read facade contract token scan') &&
      productSovereigntyScanSource.includes('useKeyboardShortcutSettings\\\\(\\\\)') &&
      productSovereigntyScanSource.includes('useSpeechToTextEnabled\\\\(\\\\)') &&
      !productSovereigntyScanSource.includes('useSpeechToTextSettings\\\\(\\\\)') &&
      productSovereigntyScanSource.includes('setModelLabel\\\\(settings\\\\.runtime\\\\.model\\\\.trim\\\\(\\\\)\\\\)') &&
      productSovereigntyScanSource.includes('rendererRuntimeClient\\\\.getSettings')
  }
  const preloadRuntimeThreadId = 'thr/with space?x=1#frag'
  const preloadRuntimeTurnId = 'turn/with space?x=1#frag'
  const preloadRuntimeInputId = 'input/with space?x=1#frag'
  const encodedPreloadThread = encodeURIComponent(preloadRuntimeThreadId)
  const encodedPreloadTurn = encodeURIComponent(preloadRuntimeTurnId)
  const encodedPreloadInput = encodeURIComponent(preloadRuntimeInputId)
  const preloadRuntimeRequestBridgeMatrix = {
    channel: 'runtime:request' as const,
    restartChannel: 'runtime:restart' as const,
    sourceIds: {
      threadId: preloadRuntimeThreadId,
      turnId: preloadRuntimeTurnId,
      inputId: preloadRuntimeInputId
    },
    encodedIds: {
      threadId: encodedPreloadThread,
      turnId: encodedPreloadTurn,
      inputId: encodedPreloadInput
    },
    runtimeFacadeCall: {
      path: `/v1/threads/${encodedPreloadThread}/turns/${encodedPreloadTurn}/steer`,
      method: 'POST' as const,
      body: '{"action":"step"}'
    },
    diagnosticsFacadeCall: {
      path: `/v1/user-inputs/${encodedPreloadInput}`,
      method: 'POST' as const,
      body: '{}'
    },
    sourcePassesArgumentsUnchanged: preloadSource.includes("runtimeRequest: (path, method, body) =>") &&
      preloadSource.includes("ipcRenderer.invoke('runtime:request', { path, method, body })"),
    sourcePassesRestartUnchanged: preloadSource.includes("restartRuntime: () => ipcRenderer.invoke('runtime:restart')"),
    runtimeFacadeUnitEvidencePresent: preloadRuntimeRequestTestSource.includes('passes runtime request paths through the analytix runtime facade unchanged') &&
      preloadRuntimeRequestTestSource.includes("api.runtime.runtimeRequest(path, 'POST', '{\"action\":\"step\"}')") &&
      preloadRuntimeRequestTestSource.includes("expect(electronMock.invoke).toHaveBeenCalledWith('runtime:request'"),
    restartUnitEvidencePresent: preloadRuntimeRequestTestSource.includes('routes explicit runtime restarts through the analytix runtime IPC channel') &&
      preloadRuntimeRequestTestSource.includes("expect(electronMock.invoke).toHaveBeenCalledWith('runtime:restart')"),
    diagnosticsFacadeUnitEvidencePresent: preloadRuntimeRequestTestSource.includes('keeps diagnostics runtime requests on the same analytix IPC channel') &&
      preloadRuntimeRequestTestSource.includes("api.diagnostics.runtimeRequest(path, 'POST', '{}')") &&
      preloadRuntimeRequestTestSource.includes("expect(electronMock.invoke).toHaveBeenCalledWith('runtime:request'"),
    exposesOnlyAnalytixApi: preloadRuntimeRequestTestSource.includes("electronMock.exposedApi.get('analytix')") &&
      preloadRuntimeRequestTestSource.includes("exposeInMainWorld).toHaveBeenCalledWith('analytix'")
  }
  const preloadSseBridgeMatrix = {
    channels: {
      start: 'runtime:sse:start' as const,
      stop: 'runtime:sse:stop' as const,
      event: 'runtime:sse-event' as const,
      end: 'runtime:sse-end' as const,
      error: 'runtime:sse-error' as const
    },
    startCall: {
      threadId: preloadRuntimeThreadId,
      sinceSeq: 42 as const,
      streamId: 'stream-provided' as const
    },
    stopCall: {
      streamId: 'stream-provided' as const
    },
    eventPayload: {
      streamId: 'stream-a' as const,
      seq: 1 as const,
      kind: 'thread_event' as const
    },
    endPayload: {
      streamId: 'stream-a' as const
    },
    errorPayload: {
      streamId: 'stream-a' as const,
      status: 404 as const
    },
    sourcePassesStartStopUnchanged: preloadSource.includes("startSse: (threadId, sinceSeq, streamId) =>") &&
      preloadSource.includes("ipcRenderer.invoke('runtime:sse:start', { threadId, sinceSeq, streamId })") &&
      preloadSource.includes("stopSse: (streamId) => ipcRenderer.invoke('runtime:sse:stop', streamId)"),
    sourcePayloadOnlyWrappers: preloadSource.includes('payload: Parameters<typeof handler>[0]') &&
      preloadSource.includes(') => handler(payload)') &&
      preloadSource.includes("ipcRenderer.on('runtime:sse-event', wrapped)") &&
      preloadSource.includes("ipcRenderer.on('runtime:sse-end', wrapped)") &&
      preloadSource.includes("ipcRenderer.on('runtime:sse-error', wrapped)"),
    sourceCleanupUsesSameWrapper: preloadSource.includes("ipcRenderer.removeListener('runtime:sse-event', wrapped)") &&
      preloadSource.includes("ipcRenderer.removeListener('runtime:sse-end', wrapped)") &&
      preloadSource.includes("ipcRenderer.removeListener('runtime:sse-error', wrapped)"),
    startStopUnitEvidencePresent: preloadSseBridgeTestSource.includes('passes SSE start and stop requests through the analytix runtime facade unchanged') &&
      preloadSseBridgeTestSource.includes("expect(electronMock.invoke).toHaveBeenNthCalledWith(1, 'runtime:sse:start'") &&
      preloadSseBridgeTestSource.includes("expect(electronMock.invoke).toHaveBeenNthCalledWith(2, 'runtime:sse:stop', 'stream-provided')"),
    payloadUnitEvidencePresent: preloadSseBridgeTestSource.includes('forwards SSE event, end, and error payloads without exposing Electron events') &&
      preloadSseBridgeTestSource.includes("sender: 'electron-event'") &&
      preloadSseBridgeTestSource.includes("expect(eventHandler).toHaveBeenCalledWith({") &&
      preloadSseBridgeTestSource.includes("expect(electronMock.removeListener).toHaveBeenCalledWith('runtime:sse-error', errorWrapper)"),
    exposesOnlyAnalytixApi: preloadSseBridgeTestSource.includes("electronMock.exposedApi.get('analytix')") &&
      preloadSseBridgeTestSource.includes("exposeInMainWorld).toHaveBeenCalledWith('analytix'")
  }
  const evidenceIds = [
    exposedBridgeNames.length === 1 && exposedBridgeNames[0] === 'analytix'
      ? 'preload-sandbox-single-analytix-bridge'
      : '',
    windowTypeProperties.length === 1 && windowTypeProperties[0] === 'analytix'
      ? 'window-type-analytix-only'
      : '',
    facadeDomains.length > 0 && !facadeDomains.some((domain) =>
      ['claw', 'kun', 'reasonix'].includes(domain)
    )
      ? 'shared-api-owned-facade'
      : '',
    publicApiTypesAnalytixOwned
      ? 'shared-api-no-upstream-public-types'
      : '',
    runtimeRequestUsesAnalytixIpc
      ? 'runtime-request-analytix-ipc'
      : '',
    runtimeSseUsesAnalytixIpc
      ? 'runtime-sse-analytix-ipc'
      : '',
    settingsStoreTestSource.includes('drops Reasonix auto-plan config shapes')
      ? 'settings-drop-reasonix-auto-plan'
      : '',
    settingsStoreTestSource.includes("'agentProvider' in persisted") &&
      settingsStoreTestSource.includes("'agents' in persisted")
      ? 'settings-drop-legacy-agent-envelope'
      : '',
    settingsStoreTestSource.includes('round-trips runtime and provider endpoint formats without legacy settings envelopes') &&
      runtimeNormalizerSource.includes('function projectAnalytixRuntimeFields') &&
      runtimeNormalizerSource.includes('Object.hasOwn(canonical, key)')
      ? 'settings-endpoint-format-top-level-runtime'
      : '',
    genericRuntimeBypassCount === 0
      ? 'renderer-generic-runtime-bypass-absent'
      : '',
    settingsReadBypassCount === 0
      ? 'renderer-settings-read-bypass-absent'
      : '',
    optionalChainPatternCovered
      ? 'renderer-optional-chain-bridge-scan'
      : '',
    rendererProviderEndpointMatrix.rootConstantsUsed
      ? 'renderer-provider-shared-root-paths'
      : '',
    rendererProviderEndpointMatrix.usesRendererRuntimeClientFacade
      ? 'renderer-provider-runtime-client-facade'
      : '',
    rendererProviderEndpointMatrix.unitTestEvidencePresent
      ? 'renderer-provider-endpoint-builder-unit-evidence'
      : '',
    rendererProviderAliasGuardMatrix.sourceInstallsThrowingAliases &&
      rendererProviderAliasGuardMatrix.routeOwnershipEvidencePresent
      ? 'renderer-provider-alias-guard-evidence'
      : '',
    rendererProviderAliasGuardMatrix.lifecycleEvidencePresent &&
      rendererProviderAliasGuardMatrix.approvalUserInputEvidencePresent
      ? 'renderer-provider-lifecycle-alias-evidence'
      : '',
    rendererProviderAliasGuardMatrix.forkResumeEvidencePresent &&
      rendererProviderAliasGuardMatrix.dynamicEncodingEvidencePresent
      ? 'renderer-provider-dynamic-route-alias-evidence'
      : '',
    rendererProviderFacadeSealMatrix.sourceUsesRuntimeClient &&
      rendererProviderFacadeSealMatrix.sourceRejectsDirectBridgeBypass
      ? 'renderer-provider-facade-source-evidence'
      : '',
    rendererProviderFacadeSealMatrix.archiveRestoreSourceEvidencePresent &&
      rendererProviderFacadeSealMatrix.relationSourceEvidencePresent &&
      rendererProviderFacadeSealMatrix.lifecycleUnitEvidencePresent
      ? 'renderer-provider-facade-lifecycle-evidence'
      : '',
    rendererProviderFacadeSealMatrix.scanGuardPresent &&
      rendererProviderFacadeSealMatrix.providerFacadeTokenScanPresent
      ? 'renderer-provider-direct-bypass-scan-evidence'
      : '',
    sideConversationRelationContractMatrix.providerContractOptional &&
      sideConversationRelationContractMatrix.providerImplementationUsesRuntimeClient
      ? 'side-conversation-relation-provider-contract'
      : '',
    sideConversationRelationContractMatrix.storeUsesProviderContract &&
      sideConversationRelationContractMatrix.storeRefreshesAndCloses &&
      sideConversationRelationContractMatrix.unitEvidencePresent
      ? 'side-conversation-relation-store-evidence'
      : '',
    sideConversationRelationContractMatrix.storeRejectsDirectRuntimeBridge &&
      sideConversationRelationContractMatrix.scanGuardPresent
      ? 'side-conversation-direct-bypass-scan-evidence'
      : '',
    rendererUsageRuntimeClientFacadeMatrix.threadUsageSourceUsesRuntimeClient &&
      rendererUsageRuntimeClientFacadeMatrix.dailyUsageSourceUsesRuntimeClient &&
      rendererUsageRuntimeClientFacadeMatrix.modelUsageSourceUsesRuntimeClient &&
      rendererUsageRuntimeClientFacadeMatrix.sourceRejectsDirectBridgeBypass
      ? 'renderer-usage-runtime-client-source-evidence'
      : '',
    rendererUsageRuntimeClientFacadeMatrix.tokenEconomySourceUsesRuntimeClient &&
      rendererUsageRuntimeClientFacadeMatrix.llmDebugSourceUsesRuntimeClient &&
      rendererUsageRuntimeClientFacadeMatrix.sourceRejectsDirectBridgeBypass
      ? 'renderer-usage-runtime-client-settings-diagnostics-evidence'
      : '',
    rendererUsageRuntimeClientFacadeMatrix.usageUnitEvidencePresent
      ? 'renderer-usage-runtime-client-unit-evidence'
      : '',
    rendererUsageRuntimeClientFacadeMatrix.scanGuardPresent &&
      rendererUsageRuntimeClientFacadeMatrix.usageFacadeTokenScanPresent
      ? 'renderer-usage-direct-bypass-scan-evidence'
      : '',
    rendererSettingsReadFacadeMatrix.keyboardShortcutSourceUsesSettingsClient &&
      rendererSettingsReadFacadeMatrix.speechToTextSourceUsesSettingsClient &&
      rendererSettingsReadFacadeMatrix.usageHeatmapSourceUsesSettingsClient &&
      rendererSettingsReadFacadeMatrix.sourceRejectsDirectSettingsBypass
      ? 'renderer-settings-read-facade-source-evidence'
      : '',
    rendererSettingsReadFacadeMatrix.settingsChangedEventPreserved
      ? 'renderer-settings-read-event-sync-evidence'
      : '',
    rendererSettingsReadFacadeMatrix.scanGuardPresent &&
      rendererSettingsReadFacadeMatrix.settingsReadFacadeTokenScanPresent
      ? 'renderer-settings-read-direct-bypass-scan-evidence'
      : '',
    rendererRuntimeClientBridgeMatrix.runtimeRequestUnitEvidencePresent
      ? 'renderer-runtime-client-request-evidence'
      : '',
    rendererRuntimeClientBridgeMatrix.restartUnitEvidencePresent
      ? 'renderer-runtime-client-restart-evidence'
      : '',
    rendererRuntimeClientBridgeMatrix.sseUnitEvidencePresent
      ? 'renderer-runtime-client-sse-evidence'
      : '',
    rendererSettingsBridgeMatrix.cacheUnitEvidencePresent
      ? 'renderer-settings-cache-evidence'
      : '',
    rendererSettingsBridgeMatrix.topLevelRuntimePatchUnitEvidencePresent
      ? 'renderer-settings-runtime-patch-evidence'
      : '',
    rendererSettingsBridgeMatrix.legacyAliasUnitEvidencePresent
      ? 'renderer-settings-legacy-alias-evidence'
      : '',
    preloadRuntimeRequestBridgeMatrix.runtimeFacadeUnitEvidencePresent
      ? 'preload-runtime-request-path-method-body-evidence'
      : '',
    preloadRuntimeRequestBridgeMatrix.restartUnitEvidencePresent
      ? 'preload-runtime-restart-evidence'
      : '',
    preloadRuntimeRequestBridgeMatrix.diagnosticsFacadeUnitEvidencePresent
      ? 'preload-diagnostics-runtime-request-same-channel'
      : '',
    preloadSseBridgeMatrix.startStopUnitEvidencePresent
      ? 'preload-sse-start-stop-evidence'
      : '',
    preloadSseBridgeMatrix.payloadUnitEvidencePresent
      ? 'preload-sse-payload-only-listeners'
      : ''
  ].filter(Boolean)
  const deprecatedBridgeAliasExposed = forbiddenBridgeAliases.some((alias) =>
    exposedBridgeNames.includes(alias) ||
    windowTypeProperties.includes(alias) ||
    facadeDomains.includes(alias)
  )

  return {
    sourceFiles,
    bridgeName: 'analytix' as const,
    runtimeIpcChannels: {
      request: 'runtime:request' as const,
      sseStart: 'runtime:sse:start' as const,
      sseStop: 'runtime:sse:stop' as const,
      sseEvent: 'runtime:sse-event' as const,
      sseEnd: 'runtime:sse-end' as const,
      sseError: 'runtime:sse-error' as const
    },
    exposedBridgeNames,
    windowTypeProperties,
    facadeDomains,
    forbiddenBridgeAliases: [...forbiddenBridgeAliases],
    forbiddenRuntimeIpcChannels: [...forbiddenRuntimeIpcChannels],
    forbiddenRuntimeIpcChannelsExposed,
    forbiddenSettingsKeys: [...forbiddenSettingsKeys],
    rendererBridgeAllowList: {
      allowedNamedRuntimeApis: [...allowedNamedRuntimeApis],
      allowedNamedSettingsApis: [...allowedNamedSettingsApis],
      directRuntimeApis,
      directSettingsApis,
      directRuntimeViolations,
      directSettingsViolations,
      genericRuntimeBypassCount,
      settingsReadBypassCount,
      optionalChainPatternCovered
    },
    rendererRuntimeClientBridgeMatrix,
    rendererSettingsBridgeMatrix,
    rendererProviderEndpointMatrix,
    rendererProviderAliasGuardMatrix,
    rendererProviderFacadeSealMatrix,
    sideConversationRelationContractMatrix,
    rendererUsageRuntimeClientFacadeMatrix,
    rendererSettingsReadFacadeMatrix,
    preloadRuntimeRequestBridgeMatrix,
    preloadSseBridgeMatrix,
    evidenceIds,
    expected: {
      exposesOnlyAnalytixBridge: exposedBridgeNames.length === 1 && exposedBridgeNames[0] === 'analytix',
      windowTypeOnlyAnalytix: windowTypeProperties.length === 1 && windowTypeProperties[0] === 'analytix',
      facadeDomainsAnalytixOwned: !facadeDomains.some((domain) =>
        forbiddenBridgeAliases.includes(domain as typeof forbiddenBridgeAliases[number])
      ),
      publicApiTypesAnalytixOwned,
      runtimeRequestUsesAnalytixIpc,
      runtimeSseUsesAnalytixIpc,
      rendererNamedRuntimeApisAllowListed: directRuntimeViolations.length === 0,
      rendererNamedSettingsApisAllowListed: directSettingsViolations.length === 0,
      rendererGenericRuntimeBypassAbsent: genericRuntimeBypassCount === 0,
      rendererSettingsReadBypassAbsent: settingsReadBypassCount === 0,
      optionalChainBridgeAccessScanned: optionalChainPatternCovered,
      rendererProviderUsesSharedRootPaths: rendererProviderEndpointMatrix.rootConstantsUsed,
      rendererProviderEncodesDynamicRouteIds: rendererProviderEndpointMatrix.encodedRuntimeRequestPaths.length === 8 &&
        rendererProviderEndpointMatrix.encodedRuntimeRequestPaths.every((path) =>
          path.includes('/v1/') &&
          !Object.values(rendererProviderEndpointMatrix.sourceIds).some((id) => path.includes(id))
        ),
      rendererProviderRuntimePathsAnalytixOwned: rendererProviderEndpointMatrix.pathOwnershipGuardPresent &&
        rendererProviderEndpointMatrix.encodedRuntimeRequestPaths.every((path) =>
          path.startsWith('/v1/') &&
          ![
            '/v1/reasonix',
            '/v1/runtime/go',
            '/v1/workflow',
            '/v1/create-loop',
            '/v1/subagent',
            '/v1/autoresearch',
            '/v1/mcp-indexer'
          ].some((token) => path.includes(token))
        ),
      rendererProviderUsesRuntimeClientFacade: rendererProviderEndpointMatrix.usesRendererRuntimeClientFacade,
      rendererProviderEndpointBuilderUnitEvidencePresent: rendererProviderEndpointMatrix.unitTestEvidencePresent,
      rendererProviderAliasGuardInstallsThrowingAliases: rendererProviderAliasGuardMatrix.sourceInstallsThrowingAliases &&
        rendererProviderAliasGuardMatrix.sourceUsesAnalytixOnly &&
        rendererProviderAliasGuardMatrix.forbiddenAliases.length === 2 &&
        rendererProviderAliasGuardMatrix.forbiddenAliases.includes('kun') &&
        rendererProviderAliasGuardMatrix.forbiddenAliases.includes('reasonix'),
      rendererProviderAliasGuardCoversRuntimeRoutes: rendererProviderAliasGuardMatrix.coverage.routeOwnership &&
        rendererProviderAliasGuardMatrix.routeOwnershipEvidencePresent,
      rendererProviderAliasGuardCoversLifecycleAndGates: rendererProviderAliasGuardMatrix.coverage.threadLifecycle &&
        rendererProviderAliasGuardMatrix.coverage.approvalUserInput &&
        rendererProviderAliasGuardMatrix.lifecycleEvidencePresent &&
        rendererProviderAliasGuardMatrix.approvalUserInputEvidencePresent,
      rendererProviderAliasGuardCoversForkResumeAndEncoding: rendererProviderAliasGuardMatrix.coverage.forkResume &&
        rendererProviderAliasGuardMatrix.coverage.dynamicRouteEncoding &&
        rendererProviderAliasGuardMatrix.forkResumeEvidencePresent &&
        rendererProviderAliasGuardMatrix.dynamicEncodingEvidencePresent,
      rendererProviderAliasGuardRejectsForbiddenRoutes: rendererProviderAliasGuardMatrix.forbiddenRouteGuardPresent &&
        rendererProviderEndpointMatrix.pathOwnershipGuardPresent,
      rendererProviderAliasGuardUnitEvidencePresent: rendererProviderAliasGuardMatrix.guardedTestNames.length === 5 &&
        rendererProviderAliasGuardMatrix.sourceInstallsThrowingAliases &&
        rendererProviderAliasGuardMatrix.sourceUsesAnalytixOnly &&
        rendererProviderAliasGuardMatrix.routeOwnershipEvidencePresent &&
        rendererProviderAliasGuardMatrix.lifecycleEvidencePresent &&
        rendererProviderAliasGuardMatrix.approvalUserInputEvidencePresent &&
        rendererProviderAliasGuardMatrix.forkResumeEvidencePresent &&
        rendererProviderAliasGuardMatrix.dynamicEncodingEvidencePresent &&
        rendererProviderAliasGuardMatrix.forbiddenRouteGuardPresent,
      rendererProviderFacadeSealUsesRuntimeClient: rendererProviderFacadeSealMatrix.sourceUsesRuntimeClient &&
        rendererProviderFacadeSealMatrix.facade === 'rendererRuntimeClient.runtimeRequest',
      rendererProviderFacadeSealRejectsDirectBridge: rendererProviderFacadeSealMatrix.sourceRejectsDirectBridgeBypass &&
        rendererProviderFacadeSealMatrix.forbiddenDirectBridge === 'window.analytix.runtime.runtimeRequest',
      rendererProviderFacadeSealCoversArchiveRestore: rendererProviderFacadeSealMatrix.archiveRestoreSourceEvidencePresent &&
        rendererProviderFacadeSealMatrix.lifecycleUnitEvidencePresent,
      rendererProviderFacadeSealCoversRelationPatch: rendererProviderFacadeSealMatrix.relationSourceEvidencePresent &&
        rendererProviderFacadeSealMatrix.lifecycleUnitEvidencePresent,
      rendererProviderFacadeSealScanGuardPresent: rendererProviderFacadeSealMatrix.scanGuardPresent &&
        rendererProviderFacadeSealMatrix.providerFacadeTokenScanPresent,
      rendererProviderFacadeSealUnitEvidencePresent: rendererProviderFacadeSealMatrix.sealedMethods.length === 2 &&
        rendererProviderFacadeSealMatrix.sealedMethods.includes('archiveThread') &&
        rendererProviderFacadeSealMatrix.sealedMethods.includes('updateThreadRelation') &&
        rendererProviderFacadeSealMatrix.sourceUsesRuntimeClient &&
        rendererProviderFacadeSealMatrix.sourceRejectsDirectBridgeBypass &&
        rendererProviderFacadeSealMatrix.archiveRestoreSourceEvidencePresent &&
        rendererProviderFacadeSealMatrix.relationSourceEvidencePresent &&
        rendererProviderFacadeSealMatrix.lifecycleUnitEvidencePresent &&
        rendererProviderFacadeSealMatrix.scanGuardPresent &&
        rendererProviderFacadeSealMatrix.providerFacadeTokenScanPresent,
      sideConversationRelationContractOptionalProvider: sideConversationRelationContractMatrix.providerContractOptional &&
        sideConversationRelationContractMatrix.providerMethod === 'updateThreadRelation',
      sideConversationRelationPromotesThroughProvider: sideConversationRelationContractMatrix.storeUsesProviderContract &&
        sideConversationRelationContractMatrix.relation === 'primary',
      sideConversationRelationRefreshesAndCloses: sideConversationRelationContractMatrix.storeRefreshesAndCloses,
      sideConversationRelationRejectsDirectBridge: sideConversationRelationContractMatrix.storeRejectsDirectRuntimeBridge,
      sideConversationRelationScanGuardPresent: sideConversationRelationContractMatrix.scanGuardPresent,
      sideConversationRelationUnitEvidencePresent: sideConversationRelationContractMatrix.storeAction === 'promoteSideConversation' &&
        sideConversationRelationContractMatrix.providerContractOptional &&
        sideConversationRelationContractMatrix.providerImplementationUsesRuntimeClient &&
        sideConversationRelationContractMatrix.storeUsesProviderContract &&
        sideConversationRelationContractMatrix.storeRefreshesAndCloses &&
        sideConversationRelationContractMatrix.storeRejectsDirectRuntimeBridge &&
        sideConversationRelationContractMatrix.unitEvidencePresent &&
        sideConversationRelationContractMatrix.scanGuardPresent,
      rendererUsageRuntimeClientCoversThreadUsage: rendererUsageRuntimeClientFacadeMatrix.facade === 'rendererRuntimeClient.runtimeRequest' &&
        rendererUsageRuntimeClientFacadeMatrix.usageLoaders.includes('loadThreadUsage') &&
        rendererUsageRuntimeClientFacadeMatrix.threadUsageSourceUsesRuntimeClient &&
        rendererUsageRuntimeClientFacadeMatrix.usageUnitEvidencePresent,
      rendererUsageRuntimeClientCoversDailyUsage: rendererUsageRuntimeClientFacadeMatrix.facade === 'rendererRuntimeClient.runtimeRequest' &&
        rendererUsageRuntimeClientFacadeMatrix.usageLoaders.includes('loadDailyUsage') &&
        rendererUsageRuntimeClientFacadeMatrix.dailyUsageSourceUsesRuntimeClient &&
        rendererUsageRuntimeClientFacadeMatrix.usageUnitEvidencePresent,
      rendererUsageRuntimeClientCoversModelUsage: rendererUsageRuntimeClientFacadeMatrix.facade === 'rendererRuntimeClient.runtimeRequest' &&
        rendererUsageRuntimeClientFacadeMatrix.usageLoaders.includes('loadModelUsage') &&
        rendererUsageRuntimeClientFacadeMatrix.modelUsageSourceUsesRuntimeClient &&
        rendererUsageRuntimeClientFacadeMatrix.usageUnitEvidencePresent,
      rendererUsageRuntimeClientCoversSettingsDiagnostics: rendererUsageRuntimeClientFacadeMatrix.diagnosticsLoaders.includes('loadTokenEconomySavingsSummary') &&
        rendererUsageRuntimeClientFacadeMatrix.diagnosticsLoaders.includes('LlmDebugSettingsSection') &&
        rendererUsageRuntimeClientFacadeMatrix.tokenEconomySourceUsesRuntimeClient &&
        rendererUsageRuntimeClientFacadeMatrix.llmDebugSourceUsesRuntimeClient,
      rendererUsageRuntimeClientRejectsDirectBridge: rendererUsageRuntimeClientFacadeMatrix.forbiddenDirectBridge === 'window.analytix.runtime.runtimeRequest' &&
        rendererUsageRuntimeClientFacadeMatrix.sourceRejectsDirectBridgeBypass,
      rendererUsageRuntimeClientScanGuardPresent: rendererUsageRuntimeClientFacadeMatrix.scanGuardPresent &&
        rendererUsageRuntimeClientFacadeMatrix.usageFacadeTokenScanPresent,
      rendererUsageRuntimeClientUnitEvidencePresent: rendererUsageRuntimeClientFacadeMatrix.usageLoaders.length === 3 &&
        rendererUsageRuntimeClientFacadeMatrix.usageUnitEvidencePresent &&
        rendererUsageRuntimeClientFacadeMatrix.scanGuardPresent &&
        rendererUsageRuntimeClientFacadeMatrix.usageFacadeTokenScanPresent,
      rendererSettingsReadFacadeCoversKeyboardShortcuts: rendererSettingsReadFacadeMatrix.facade === 'rendererRuntimeClient.getSettings' &&
        rendererSettingsReadFacadeMatrix.settingsReaders.includes('useKeyboardShortcutSettings') &&
        rendererSettingsReadFacadeMatrix.keyboardShortcutSourceUsesSettingsClient,
      rendererSettingsReadFacadeCoversSpeechToText: rendererSettingsReadFacadeMatrix.facade === 'rendererRuntimeClient.getSettings' &&
        rendererSettingsReadFacadeMatrix.settingsReaders.includes('useSpeechToTextEnabled') &&
        rendererSettingsReadFacadeMatrix.speechToTextSourceUsesSettingsClient,
      rendererSettingsReadFacadeCoversUsageModelLabel: rendererSettingsReadFacadeMatrix.facade === 'rendererRuntimeClient.getSettings' &&
        rendererSettingsReadFacadeMatrix.settingsReaders.includes('InitialSessionUsageHeatmapView') &&
        rendererSettingsReadFacadeMatrix.usageHeatmapSourceUsesSettingsClient,
      rendererSettingsReadFacadePreservesSettingsChangedEvent: rendererSettingsReadFacadeMatrix.settingsChangedEventPreserved,
      rendererSettingsReadFacadeRejectsDirectBridge: rendererSettingsReadFacadeMatrix.forbiddenDirectBridge === 'window.analytix.settings.getSettings' &&
        rendererSettingsReadFacadeMatrix.sourceRejectsDirectSettingsBypass,
      rendererSettingsReadFacadeScanGuardPresent: rendererSettingsReadFacadeMatrix.scanGuardPresent &&
        rendererSettingsReadFacadeMatrix.settingsReadFacadeTokenScanPresent,
      rendererRuntimeClientRuntimeRequestPreservesArguments: rendererRuntimeClientBridgeMatrix.sourceUsesAnalytixRuntime &&
        rendererRuntimeClientBridgeMatrix.sourcePassesRuntimeRequestArgumentsUnchanged &&
        rendererRuntimeClientBridgeMatrix.runtimeRequestUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.runtimeRequestCalls.length === 3 &&
        rendererRuntimeClientBridgeMatrix.runtimeRequestCalls.every((request) =>
          request.path.startsWith('/v1/') &&
          request.path.includes(encodedRendererRuntimeClientThread) &&
          request.path.includes(encodedRendererRuntimeClientTurn) &&
          !request.path.includes(rendererRuntimeClientThreadId) &&
          !request.path.includes(rendererRuntimeClientTurnId)
        ),
      rendererRuntimeClientRestartUsesRuntimeApi: rendererRuntimeClientBridgeMatrix.sourceUsesAnalytixRuntime &&
        rendererRuntimeClientBridgeMatrix.sourcePassesRestartUnchanged &&
        rendererRuntimeClientBridgeMatrix.restartUnitEvidencePresent,
      rendererRuntimeClientSseControlsPreserveArguments: rendererRuntimeClientBridgeMatrix.sourceUsesAnalytixRuntime &&
        rendererRuntimeClientBridgeMatrix.sourcePassesSseControlsUnchanged &&
        rendererRuntimeClientBridgeMatrix.sseUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.sseStartCall.threadId === rendererRuntimeClientThreadId &&
        rendererRuntimeClientBridgeMatrix.sseStartCall.sinceSeq === 7 &&
        rendererRuntimeClientBridgeMatrix.sseStartCall.streamId === 'stream-renderer' &&
        rendererRuntimeClientBridgeMatrix.sseStopCall.streamId === 'stream-renderer',
      rendererRuntimeClientSseListenersPreserveHandlers: rendererRuntimeClientBridgeMatrix.sourcePassesSseListenersUnchanged &&
        rendererRuntimeClientBridgeMatrix.sseUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.listenerApis.length === 3,
      rendererRuntimeClientLegacyAliasesUnread: rendererRuntimeClientBridgeMatrix.legacyAliasUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.sourceUsesAnalytixRuntime,
      rendererRuntimeClientUnitEvidencePresent: rendererRuntimeClientBridgeMatrix.runtimeRequestUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.restartUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.sseUnitEvidencePresent &&
        rendererRuntimeClientBridgeMatrix.legacyAliasUnitEvidencePresent,
      rendererSettingsBridgeUsesAnalytixSettingsApi: rendererSettingsBridgeMatrix.sourceUsesAnalytixSettings,
      rendererSettingsBridgeCachesReads: rendererSettingsBridgeMatrix.sourceUsesAnalytixSettings &&
        rendererSettingsBridgeMatrix.sourceCachesSettingsReads &&
        rendererSettingsBridgeMatrix.cacheUnitEvidencePresent &&
        rendererSettingsBridgeMatrix.cacheExpectations.getSettingsCallsForDoubleRead === 1,
      rendererSettingsBridgeRefreshesCacheAfterWrite: rendererSettingsBridgeMatrix.sourceUsesAnalytixSettings &&
        rendererSettingsBridgeMatrix.sourceRefreshesCacheAfterSetSettings &&
        rendererSettingsBridgeMatrix.refreshUnitEvidencePresent &&
        rendererSettingsBridgeMatrix.cacheExpectations.getSettingsCallsAfterSetSettings === 1 &&
        rendererSettingsBridgeMatrix.cacheExpectations.setSettingsCallsAfterWrite === 1,
      rendererSettingsBridgePreservesTopLevelRuntimePatch: rendererSettingsBridgeMatrix.sourceUsesAnalytixSettings &&
        rendererSettingsBridgeMatrix.topLevelRuntimePatchUnitEvidencePresent &&
        rendererSettingsBridgeMatrix.patch.runtimeModel === 'deepseek-reasoner' &&
        rendererSettingsBridgeMatrix.patch.approvalPolicy === 'never',
      rendererSettingsBridgeLegacyAliasesUnread: rendererSettingsBridgeMatrix.sourceUsesAnalytixSettings &&
        rendererSettingsBridgeMatrix.legacyAliasUnitEvidencePresent,
      rendererSettingsBridgeUnitEvidencePresent: rendererSettingsBridgeMatrix.cacheUnitEvidencePresent &&
        rendererSettingsBridgeMatrix.refreshUnitEvidencePresent &&
        rendererSettingsBridgeMatrix.topLevelRuntimePatchUnitEvidencePresent &&
        rendererSettingsBridgeMatrix.legacyAliasUnitEvidencePresent,
      preloadRuntimeRequestPreservesPathMethodBody: preloadRuntimeRequestBridgeMatrix.sourcePassesArgumentsUnchanged &&
        preloadRuntimeRequestBridgeMatrix.runtimeFacadeUnitEvidencePresent &&
        preloadRuntimeRequestBridgeMatrix.runtimeFacadeCall.path.startsWith('/v1/') &&
        preloadRuntimeRequestBridgeMatrix.runtimeFacadeCall.path.includes(encodedPreloadThread) &&
        preloadRuntimeRequestBridgeMatrix.runtimeFacadeCall.path.includes(encodedPreloadTurn) &&
        !Object.values(preloadRuntimeRequestBridgeMatrix.sourceIds).some((id) =>
          preloadRuntimeRequestBridgeMatrix.runtimeFacadeCall.path.includes(id)
        ),
      preloadDiagnosticsRuntimeRequestUsesSameChannel: preloadRuntimeRequestBridgeMatrix.channel === 'runtime:request' &&
        preloadRuntimeRequestBridgeMatrix.diagnosticsFacadeUnitEvidencePresent &&
        preloadRuntimeRequestBridgeMatrix.diagnosticsFacadeCall.path.includes(encodedPreloadInput),
      preloadRuntimeRestartUsesAnalytixIpc: preloadRuntimeRequestBridgeMatrix.restartChannel === 'runtime:restart' &&
        preloadRuntimeRequestBridgeMatrix.sourcePassesRestartUnchanged &&
        preloadRuntimeRequestBridgeMatrix.restartUnitEvidencePresent,
      preloadRuntimeRequestExposesOnlyAnalytixApi: preloadRuntimeRequestBridgeMatrix.exposesOnlyAnalytixApi,
      preloadRuntimeRequestUnitEvidencePresent: preloadRuntimeRequestBridgeMatrix.runtimeFacadeUnitEvidencePresent &&
        preloadRuntimeRequestBridgeMatrix.restartUnitEvidencePresent &&
        preloadRuntimeRequestBridgeMatrix.diagnosticsFacadeUnitEvidencePresent,
      preloadSseStartStopPreservesArguments: preloadSseBridgeMatrix.sourcePassesStartStopUnchanged &&
        preloadSseBridgeMatrix.startStopUnitEvidencePresent &&
        preloadSseBridgeMatrix.channels.start === 'runtime:sse:start' &&
        preloadSseBridgeMatrix.channels.stop === 'runtime:sse:stop' &&
        preloadSseBridgeMatrix.startCall.threadId === preloadRuntimeThreadId &&
        preloadSseBridgeMatrix.startCall.sinceSeq === 42 &&
        preloadSseBridgeMatrix.startCall.streamId === 'stream-provided' &&
        preloadSseBridgeMatrix.stopCall.streamId === 'stream-provided',
      preloadSsePayloadListenersOmitElectronEvent: preloadSseBridgeMatrix.sourcePayloadOnlyWrappers &&
        preloadSseBridgeMatrix.payloadUnitEvidencePresent &&
        preloadSseBridgeMatrix.eventPayload.streamId === 'stream-a' &&
        preloadSseBridgeMatrix.eventPayload.seq === 1 &&
        preloadSseBridgeMatrix.endPayload.streamId === 'stream-a' &&
        preloadSseBridgeMatrix.errorPayload.status === 404,
      preloadSseListenerCleanupUsesSameWrapper: preloadSseBridgeMatrix.sourceCleanupUsesSameWrapper &&
        preloadSseBridgeMatrix.payloadUnitEvidencePresent,
      preloadSseBridgeUnitEvidencePresent: preloadSseBridgeMatrix.startStopUnitEvidencePresent &&
        preloadSseBridgeMatrix.payloadUnitEvidencePresent &&
        preloadSseBridgeMatrix.exposesOnlyAnalytixApi,
      dropsReasonixAutoPlanConfig: evidenceIds.includes('settings-drop-reasonix-auto-plan'),
      stripsLegacyRuntimeSettings: evidenceIds.includes('settings-drop-legacy-agent-envelope'),
      writesTopLevelRuntimeSettings: evidenceIds.includes('settings-endpoint-format-top-level-runtime'),
      deprecatedBridgeAliasExposed,
      forbiddenRuntimeIpcExposed: forbiddenRuntimeIpcChannelsExposed.length > 0,
      deprecatedSettingsFallbackWritten: !(evidenceIds.includes('settings-drop-reasonix-auto-plan') &&
        evidenceIds.includes('settings-drop-legacy-agent-envelope') &&
        evidenceIds.includes('settings-endpoint-format-top-level-runtime')),
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    }
  }
}

function desktopMainIpcBoundaryControlCase() {
  const sourceFiles = {
    appIpcSchemas: 'src/main/ipc/app-ipc-schemas.ts' as const,
    appIpcSchemasTest: 'src/main/ipc/app-ipc-schemas.test.ts' as const,
    registerAppIpcHandlers: 'src/main/ipc/register-app-ipc-handlers.ts' as const,
    registerAppIpcHandlersTest: 'src/main/ipc/register-app-ipc-handlers.test.ts' as const,
    runtimeSseIpc: 'src/main/runtime-sse-ipc.ts' as const,
    runtimeSseIpcTest: 'src/main/runtime-sse-ipc.test.ts' as const,
    runtimeAdapter: 'src/main/runtime/analytix-adapter.ts' as const,
    runtimeAdapterTest: 'src/main/runtime/analytix-adapter.test.ts' as const
  }
  const appIpcSchemasSource = repoSource(sourceFiles.appIpcSchemas)
  const appIpcSchemasTestSource = repoSource(sourceFiles.appIpcSchemasTest)
  const registerAppIpcHandlersSource = repoSource(sourceFiles.registerAppIpcHandlers)
  const registerAppIpcHandlersTestSource = repoSource(sourceFiles.registerAppIpcHandlersTest)
  const runtimeSseIpcSource = repoSource(sourceFiles.runtimeSseIpc)
  const runtimeSseIpcTestSource = repoSource(sourceFiles.runtimeSseIpcTest)
  const runtimeAdapterSource = repoSource(sourceFiles.runtimeAdapter)
  const runtimeAdapterTestSource = repoSource(sourceFiles.runtimeAdapterTest)
  const runtimeRequestSchemaSource = appIpcSchemasSource.slice(
    appIpcSchemasSource.indexOf('export const runtimeRequestPayloadSchema'),
    appIpcSchemasSource.indexOf('const localeSchema')
  )
  const sseStartSchemaSource = appIpcSchemasSource.slice(
    appIpcSchemasSource.indexOf('export const sseStartPayloadSchema'),
    appIpcSchemasSource.indexOf('export const streamIdSchema')
  )
  const runtimeRequestHandlerSource = registerAppIpcHandlersSource.slice(
    registerAppIpcHandlersSource.indexOf("ipcMain.handle('runtime:request'"),
    registerAppIpcHandlersSource.indexOf("ipcMain.handle('runtime:restart'")
  )
  const sseStopHandlerStart = runtimeSseIpcSource.indexOf("ipcMain.handle('runtime:sse:stop'")
  const sseStopHandlerSource = runtimeSseIpcSource.slice(
    sseStopHandlerStart,
    runtimeSseIpcSource.indexOf('\n}', sseStopHandlerStart)
  )
  const allowedAnalytixRouteExamples = [
    '/v1/runtime/info',
    '/v1/runtime/tools',
    '/v1/threads?limit=1',
    '/v1/threads/thr_1/goal',
    '/v1/debug/llm-rounds'
  ]
  const forbiddenRuntimeRoutes = [
    '/v1/reasonix/session?thread_id=thr_1',
    '/v1/runtime/go/threads',
    '/v1/workflow',
    '/v1/workflows',
    '/v1/create-loop',
    '/v1/subagents',
    '/v1/autoresearch',
    '/v1/mcp-indexer'
  ] as const
  const handlerParseIndex = runtimeRequestHandlerSource.indexOf(
    "parseIpcPayload('runtime:request', runtimeRequestPayloadSchema, payload)"
  )
  const handlerRuntimeCallIndex = runtimeRequestHandlerSource.indexOf(
    'runtimeRequest(request.path, request.method, request.body)'
  )
  const runtimeRequestEvidence = {
    schemaStrict: runtimeRequestSchemaSource.includes('.strict()'),
    refinesAllowedSurface: runtimeRequestSchemaSource.includes('.refine((payload) => isAllowedRuntimeRequest(payload)'),
    normalizesRelativePath: runtimeRequestSchemaSource.includes("value.startsWith('/') ? value : `/${value}`") &&
      appIpcSchemasTestSource.includes('normalizes runtime request paths'),
    handlerParsesBeforeRuntimeCall: handlerParseIndex >= 0 &&
      handlerRuntimeCallIndex >= 0 &&
      handlerParseIndex < handlerRuntimeCallIndex,
    forbiddenHandlerNoExecute: registerAppIpcHandlersTestSource.includes('rejects forbidden runtime request routes at the IPC handler boundary') &&
      registerAppIpcHandlersTestSource.includes('expect(runtimeRequest).not.toHaveBeenCalled()')
  }
  const sseEvidence = {
    startSchemaStrict: sseStartSchemaSource.includes('.strict()'),
    rejectsReasonixSessionId: appIpcSchemasTestSource.includes('reasonixSessionId') &&
      appIpcSchemasTestSource.includes('Unrecognized key'),
    usesAnalytixThreadEventsPath: runtimeSseIpcSource.includes('analytixThreadEventsPath(request.threadId)') &&
      runtimeSseIpcTestSource.includes("expect(seenUrls[0].pathname).toBe('/v1/threads/thread-generated/events')"),
    stopParsesStreamId: sseStopHandlerSource.includes('streamIdSchema.parse(streamId)'),
    stopMatchesStreamIdOnly: runtimeSseIpcTestSource.includes('stops only the matching SSE stream id') &&
      runtimeSseIpcTestSource.includes("'wrong-stream'") &&
      runtimeSseIpcTestSource.includes("'stream-a'"),
    reconnectLastEventId: runtimeSseIpcTestSource.includes('reconnects with the highest renderer-acked since_seq') &&
      runtimeSseIpcTestSource.includes("'Last-Event-ID'"),
    batchesEventsAt100ms: runtimeSseIpcSource.includes('SSE_PENDING_EVENT_THROTTLE_MS') &&
      runtimeSseIpcTestSource.includes('throttles pending runtime events into a 100ms IPC batch') &&
      runtimeSseIpcTestSource.includes('advanceTimersByTimeAsync(99)') &&
      runtimeSseIpcTestSource.includes('advanceTimersByTimeAsync(1)'),
    flushesFirstEventAndBatchesAt16ms: runtimeSseIpcSource.includes('SSE_PENDING_EVENT_BATCH_MS = 16') &&
      runtimeSseIpcTestSource.includes('flushes the first runtime event immediately and batches following events per frame') &&
      runtimeSseIpcTestSource.includes('advanceTimersByTimeAsync(15)') &&
      runtimeSseIpcTestSource.includes('advanceTimersByTimeAsync(1)')
  }
  const runtimeRequestAllowsAnalytixRoutes = allowedAnalytixRouteExamples.every((route) =>
    appIpcSchemasTestSource.includes(route)
  )
  const runtimeRequestRejectsForbiddenRoutes = forbiddenRuntimeRoutes.every((route) =>
    appIpcSchemasTestSource.includes(route) ||
      appIpcSchemasTestSource.includes(route.replace(/^\//, ''))
  )
  const mainIpcEndpointThreadId = 'thr/with space?x=1#frag'
  const mainIpcEndpointTurnId = 'turn/with space?x=1#frag'
  const mainIpcEndpointCheckpointId = 'axcp/with space?x=1#frag'
  const mainIpcEndpointApprovalId = 'appr/with space?x=1#frag'
  const mainIpcEndpointInputId = 'input/with space?x=1#frag'
  const mainIpcEndpointSessionId = 'sess/with space?x=1#frag'
  const mainIpcEndpointAttachmentId = 'att/with space?x=1#frag'
  const mainIpcEndpointMemoryId = 'mem/with space?x=1#frag'
  const encodedMainIpcThread = encodeURIComponent(mainIpcEndpointThreadId)
  const encodedMainIpcTurn = encodeURIComponent(mainIpcEndpointTurnId)
  const encodedMainIpcCheckpoint = encodeURIComponent(mainIpcEndpointCheckpointId)
  const encodedMainIpcApproval = encodeURIComponent(mainIpcEndpointApprovalId)
  const encodedMainIpcInput = encodeURIComponent(mainIpcEndpointInputId)
  const encodedMainIpcSession = encodeURIComponent(mainIpcEndpointSessionId)
  const encodedMainIpcAttachment = encodeURIComponent(mainIpcEndpointAttachmentId)
  const encodedMainIpcMemory = encodeURIComponent(mainIpcEndpointMemoryId)
  const mainIpcSharedTemplateNames = [
    'ANALYTIX_THREAD_TEMPLATE',
    'ANALYTIX_THREAD_FORK_TEMPLATE',
    'ANALYTIX_THREAD_REWIND_TEMPLATE',
    'ANALYTIX_THREAD_TURNS_TEMPLATE',
    'ANALYTIX_THREAD_STEER_TEMPLATE',
    'ANALYTIX_THREAD_INTERRUPT_TEMPLATE',
    'ANALYTIX_THREAD_CHECKPOINT_REWIND_PLAN_TEMPLATE',
    'ANALYTIX_THREAD_CHECKPOINT_REWIND_APPLY_TEMPLATE',
    'ANALYTIX_APPROVAL_TEMPLATE',
    'ANALYTIX_USER_INPUT_TEMPLATE',
    'ANALYTIX_SESSION_RESUME_TEMPLATE',
    'ANALYTIX_ATTACHMENT_CONTENT_TEMPLATE',
    'ANALYTIX_MEMORY_RECORD_TEMPLATE'
  ] as const
  const endpointBuilderAllowListMatrix = {
    sourceIds: {
      threadId: mainIpcEndpointThreadId,
      turnId: mainIpcEndpointTurnId,
      checkpointId: mainIpcEndpointCheckpointId,
      approvalId: mainIpcEndpointApprovalId,
      inputId: mainIpcEndpointInputId,
      sessionId: mainIpcEndpointSessionId,
      attachmentId: mainIpcEndpointAttachmentId,
      memoryId: mainIpcEndpointMemoryId
    },
    encodedIds: {
      threadId: encodedMainIpcThread,
      turnId: encodedMainIpcTurn,
      checkpointId: encodedMainIpcCheckpoint,
      approvalId: encodedMainIpcApproval,
      inputId: encodedMainIpcInput,
      sessionId: encodedMainIpcSession,
      attachmentId: encodedMainIpcAttachment,
      memoryId: encodedMainIpcMemory
    },
    acceptedSharedBuilderRequests: [
      { name: 'thread-read' as const, path: `/v1/threads/${encodedMainIpcThread}`, method: 'GET' as const },
      { name: 'thread-patch' as const, path: `/v1/threads/${encodedMainIpcThread}`, method: 'PATCH' as const },
      { name: 'thread-fork' as const, path: `/v1/threads/${encodedMainIpcThread}/fork`, method: 'POST' as const },
      { name: 'thread-rewind' as const, path: `/v1/threads/${encodedMainIpcThread}/rewind`, method: 'POST' as const },
      { name: 'thread-turns' as const, path: `/v1/threads/${encodedMainIpcThread}/turns`, method: 'POST' as const },
      { name: 'thread-steer' as const, path: `/v1/threads/${encodedMainIpcThread}/turns/${encodedMainIpcTurn}/steer`, method: 'POST' as const },
      { name: 'thread-interrupt' as const, path: `/v1/threads/${encodedMainIpcThread}/turns/${encodedMainIpcTurn}/interrupt`, method: 'POST' as const },
      { name: 'checkpoint-rewind-plan' as const, path: `/v1/threads/${encodedMainIpcThread}/checkpoints/${encodedMainIpcCheckpoint}/rewind-plan`, method: 'POST' as const },
      { name: 'checkpoint-rewind-apply' as const, path: `/v1/threads/${encodedMainIpcThread}/checkpoints/${encodedMainIpcCheckpoint}/rewind-apply`, method: 'POST' as const },
      { name: 'approval-submit' as const, path: `/v1/approvals/${encodedMainIpcApproval}`, method: 'POST' as const },
      { name: 'user-input-submit' as const, path: `/v1/user-inputs/${encodedMainIpcInput}`, method: 'POST' as const },
      { name: 'session-resume' as const, path: `/v1/sessions/${encodedMainIpcSession}/resume-thread`, method: 'POST' as const },
      { name: 'attachment-content' as const, path: `/v1/attachments/${encodedMainIpcAttachment}/content`, method: 'GET' as const },
      { name: 'memory-record' as const, path: `/v1/memory/${encodedMainIpcMemory}`, method: 'PATCH' as const }
    ],
    rejectedRawDynamicRequests: [
      { path: '/v1/threads/thr/raw/turns', method: 'POST' as const },
      { path: '/v1/threads/thr/raw/rewind', method: 'POST' as const },
      { path: '/v1/threads/thr/raw/turns/turn/raw/steer', method: 'POST' as const },
      { path: '/v1/threads/thr/raw/checkpoints/axcp/raw/rewind-plan', method: 'POST' as const },
      { path: '/v1/approvals/appr/raw', method: 'POST' as const },
      { path: '/v1/user-input/input_raw', method: 'POST' as const },
      { path: '/v1/sessions/sess/raw/resume-thread', method: 'POST' as const },
      { path: '/v1/memory/mem/raw', method: 'PATCH' as const },
      { path: '/v1/attachments/att/raw/content', method: 'GET' as const }
    ],
    sharedTemplatesCompiled: mainIpcSharedTemplateNames,
    schemaUsesSharedTemplates: mainIpcSharedTemplateNames.every((templateName) =>
      appIpcSchemasSource.includes(`compileEndpoint(${templateName}`)
    ),
    unitTestEvidencePresent: appIpcSchemasTestSource.includes('accepts shared endpoint builder output for encoded dynamic ids') &&
      appIpcSchemasTestSource.includes('analytixSessionResumePath(sessionId)') &&
      appIpcSchemasTestSource.includes('analytixAttachmentContentPath(attachmentId)') &&
      appIpcSchemasTestSource.includes('analytixMemoryRecordPath(memoryId)'),
    rawRouteRejectionEvidencePresent: appIpcSchemasTestSource.includes('rejects unencoded dynamic route ids and singular user-input compatibility paths') &&
      appIpcSchemasTestSource.includes('/v1/user-input/input_raw') &&
      appIpcSchemasTestSource.includes('/v1/attachments/att/raw/content')
  }
  const runtimeAdapterHandoffMatrix = {
    handlerChannel: 'runtime:request' as const,
    adapterFunction: 'runtimeRequest' as const,
    acceptedAdapterCalls: [
      { name: 'thread-steer' as const, path: `/v1/threads/${encodedMainIpcThread}/turns/${encodedMainIpcTurn}/steer`, method: 'POST' as const, body: '{"action":"step"}' },
      { name: 'thread-rewind' as const, path: `/v1/threads/${encodedMainIpcThread}/rewind`, method: 'POST' as const, body: '{"turnId":"turn_2"}' },
      { name: 'checkpoint-rewind-plan' as const, path: `/v1/threads/${encodedMainIpcThread}/checkpoints/${encodedMainIpcCheckpoint}/rewind-plan`, method: 'POST' as const, body: '{"scope":"combined"}' },
      { name: 'approval-submit' as const, path: `/v1/approvals/${encodedMainIpcApproval}`, method: 'POST' as const, body: '{}' },
      { name: 'user-input-submit' as const, path: `/v1/user-inputs/${encodedMainIpcInput}`, method: 'POST' as const, body: '{}' },
      { name: 'session-resume' as const, path: `/v1/sessions/${encodedMainIpcSession}/resume-thread`, method: 'POST' as const, body: '{}' },
      { name: 'attachment-content' as const, path: `/v1/attachments/${encodedMainIpcAttachment}/content`, method: 'GET' as const },
      { name: 'memory-record' as const, path: `/v1/memory/${encodedMainIpcMemory}`, method: 'PATCH' as const, body: '{}' }
    ],
    rejectedBeforeAdapterCalls: [
      { path: '/v1/threads/thr/raw/rewind', method: 'POST' as const, body: '{}' },
      { path: '/v1/threads/thr/raw/turns/turn/raw/steer', method: 'POST' as const, body: '{}' },
      { path: '/v1/threads/thr/raw/checkpoints/axcp/raw/rewind-plan', method: 'POST' as const, body: '{}' },
      { path: '/v1/approvals/appr/raw', method: 'POST' as const, body: '{}' },
      { path: '/v1/user-input/input_raw', method: 'POST' as const, body: '{}' },
      { path: '/v1/sessions/sess/raw/resume-thread', method: 'POST' as const, body: '{}' },
      { path: '/v1/attachments/att/raw/content', method: 'GET' as const }
    ],
    parseBeforeAdapterCall: handlerParseIndex >= 0 &&
      handlerRuntimeCallIndex >= 0 &&
      handlerParseIndex < handlerRuntimeCallIndex,
    encodedPathEvidencePresent: registerAppIpcHandlersTestSource.includes('passes encoded shared endpoint builder paths to the runtime adapter unchanged') &&
      registerAppIpcHandlersTestSource.includes("expect(request.path).toContain('%2F')") &&
      registerAppIpcHandlersTestSource.includes('toHaveBeenNthCalledWith'),
    methodAndBodyEvidencePresent: registerAppIpcHandlersTestSource.includes("'body' in request ? request.body : undefined") &&
      registerAppIpcHandlersTestSource.includes('request.method'),
    rejectsBeforeAdapterEvidencePresent: registerAppIpcHandlersTestSource.includes('rejects raw dynamic and singular compatibility paths before runtime adapter handoff') &&
      registerAppIpcHandlersTestSource.includes('expect(runtimeRequest).not.toHaveBeenCalled()')
  }
  const runtimeHostHandoffPath = `/v1/threads/${encodedMainIpcThread}/turns/${encodedMainIpcTurn}/steer?dry_run=true&timezone=Asia%2FShanghai`
  const runtimeHostHandoffMatrix = {
    adapterFunction: 'runtimeRequestViaHost' as const,
    baseUrlFunction: 'getRuntimeBaseUrlForSettings' as const,
    sourceIds: {
      threadId: mainIpcEndpointThreadId,
      turnId: mainIpcEndpointTurnId
    },
    encodedIds: {
      threadId: encodedMainIpcThread,
      turnId: encodedMainIpcTurn
    },
    request: {
      path: runtimeHostHandoffPath,
      method: 'POST' as const,
      body: '{"action":"step"}',
      authHeader: 'Bearer usage-token' as const,
      contentType: 'application/json' as const,
      customHeaderName: 'X-Analytix-Evidence' as const,
      customHeaderValue: 'runtime-host-path' as const
    },
    sourceEvidence: {
      usesEnsuredSettings: runtimeAdapterSource.includes('const ensuredSettings = await ensureRuntime(settings)') &&
        runtimeAdapterSource.includes('const requestSettings = ensuredSettings ?? settings'),
      joinsBaseWithNormalizedPath: runtimeAdapterSource.includes('const base = getRuntimeBaseUrlForSettings(requestSettings)') &&
        runtimeAdapterSource.includes("pathAndQuery.startsWith('/') ? pathAndQuery : `/${pathAndQuery}`") &&
        runtimeAdapterSource.includes('const url = `${base}${pathNorm}`'),
      setsBearerAuth: runtimeAdapterSource.includes('runtimeAuthHeaders(requestSettings)') &&
        runtimeAdapterSource.includes("headers.set('Authorization', `Bearer ${runtimeToken}`)"),
      forwardsCustomHeaders: runtimeAdapterSource.includes('for (const [key, value] of Object.entries(init.headers ?? {}))') &&
        runtimeAdapterSource.includes('hdrs.set(key, value)'),
      defaultsJsonContentType: runtimeAdapterSource.includes("hdrs.set('Content-Type', 'application/json')"),
      forwardsMethodAndBody: runtimeAdapterSource.includes('method: init.method ??') &&
        runtimeAdapterSource.includes('body: init.body')
    },
    unitTestEvidencePresent: runtimeAdapterTestSource.includes('preserves encoded shared endpoint paths when forwarding to the runtime host') &&
      runtimeAdapterTestSource.includes("expect(seenUrl).toBe(path)") &&
      runtimeAdapterTestSource.includes("expect(seenAuthorization).toBe('Bearer usage-token')") &&
      runtimeAdapterTestSource.includes("expect(seenCustomHeader).toBe('runtime-host-path')") &&
      runtimeAdapterTestSource.includes("expect(seenBody).toBe('{\"action\":\"step\"}')"),
    ensureRuntimePortEvidencePresent: runtimeAdapterTestSource.includes('uses settings returned by ensureRuntime when the managed port changes') &&
      runtimeAdapterTestSource.includes('async () => settingsForPort(port)') &&
      runtimeAdapterTestSource.includes("expect(seenUrl).toBe('/v1/threads?limit=1')")
  }
  const mainSseThreadId = 'thr/with space?x=1#frag'
  const encodedMainSseThread = encodeURIComponent(mainSseThreadId)
  const mainSseHostEncodingMatrix = {
    handlerChannel: 'runtime:sse:start' as const,
    eventChannel: 'runtime:sse-error' as const,
    sourceThreadId: mainSseThreadId,
    encodedThreadId: encodedMainSseThread,
    request: {
      path: `/v1/threads/${encodedMainSseThread}/events`,
      sinceSeq: 9 as const,
      lastEventId: '9' as const,
      accept: 'text/event-stream' as const,
      authorization: 'Bearer runtime-token' as const,
      streamId: 'stream-encoded' as const
    },
    errorPayload: {
      streamId: 'stream-encoded' as const,
      status: 404 as const
    },
    sourceBuildsAnalytixEventsPath: runtimeSseIpcSource.includes('analytixThreadEventsPath(request.threadId)') &&
      runtimeSseIpcSource.includes('url.searchParams.set') &&
      runtimeSseIpcSource.includes("requestHeaders['Last-Event-ID'] = String(state.ackedSinceSeq)") &&
      runtimeSseIpcSource.includes("const headers: Record<string, string> = { Accept: 'text/event-stream' }") &&
      runtimeSseIpcSource.includes('runtimeAuthHeaders(s).forEach'),
    encodedUnitEvidencePresent: runtimeSseIpcTestSource.includes('URL-encodes SSE thread ids before fetching the analytix runtime host') &&
      runtimeSseIpcTestSource.includes("expect(seenUrls[0].pathname).toBe('/v1/threads/thr%2Fwith%20space%3Fx%3D1%23frag/events')") &&
      runtimeSseIpcTestSource.includes("expect(seenUrls[0].searchParams.get('since_seq')).toBe('9')") &&
      runtimeSseIpcTestSource.includes("expect(seenLastEventId).toEqual(['9'])"),
    headerUnitEvidencePresent: runtimeSseIpcTestSource.includes("expect(requestHeader(init?.headers, 'Authorization')).toBe('Bearer runtime-token')") &&
      runtimeSseIpcTestSource.includes("expect(requestHeader(init?.headers, 'Accept')).toBe('text/event-stream')"),
    errorUnitEvidencePresent: runtimeSseIpcTestSource.includes("expect(result).toEqual({ streamId: 'stream-encoded' })") &&
      runtimeSseIpcTestSource.includes("expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {") &&
      runtimeSseIpcTestSource.includes("streamId: 'stream-encoded'") &&
      runtimeSseIpcTestSource.includes('status: 404'),
    forbiddenRouteGuardPresent: runtimeSseIpcTestSource.includes('FORBIDDEN_PUBLIC_SSE_ROUTE') &&
      runtimeSseIpcTestSource.includes('not.toMatch(FORBIDDEN_PUBLIC_SSE_ROUTE)')
  }
  return {
    sourceContractId: 'desktop-main-ipc-boundary-v1' as const,
    sourceFiles,
    runtimeRequest: {
      handlerChannel: 'runtime:request' as const,
      allowedAnalytixRouteExamples,
      forbiddenRuntimeRoutes: [...forbiddenRuntimeRoutes],
      evidence: runtimeRequestEvidence
    },
    sse: {
      startChannel: 'runtime:sse:start' as const,
      stopChannel: 'runtime:sse:stop' as const,
      eventChannel: 'runtime:sse-event' as const,
      errorChannel: 'runtime:sse-error' as const,
      endChannel: 'runtime:sse-end' as const,
      routePathTemplate: '/v1/threads/{id}/events' as const,
      batchMs: 16 as const,
      evidence: sseEvidence
    },
    endpointBuilderAllowListMatrix,
    runtimeAdapterHandoffMatrix,
    runtimeHostHandoffMatrix,
    mainSseHostEncodingMatrix,
    productBoundary: {
      usesReasonixProtocol: false as const,
      rendererRouteExposed: false as const,
      defaultGoBackendEnabled: false as const
    },
    expected: {
      runtimeRequestSchemaStrict: runtimeRequestEvidence.schemaStrict,
      runtimeRequestAllowsAnalytixRoutes,
      runtimeRequestRejectsForbiddenRoutes,
      runtimeRequestHandlerRejectsBeforeRuntimeCall: runtimeRequestEvidence.handlerParsesBeforeRuntimeCall &&
        runtimeRequestEvidence.forbiddenHandlerNoExecute,
      mainIpcEndpointBuilderAcceptsSharedPaths: endpointBuilderAllowListMatrix.acceptedSharedBuilderRequests.length === 14 &&
        endpointBuilderAllowListMatrix.unitTestEvidencePresent &&
        endpointBuilderAllowListMatrix.acceptedSharedBuilderRequests.every((request) =>
          request.path.startsWith('/v1/') &&
          !Object.values(endpointBuilderAllowListMatrix.sourceIds).some((id) => request.path.includes(id)) &&
          Object.values(endpointBuilderAllowListMatrix.encodedIds).some((id) => request.path.includes(id))
        ),
      mainIpcEndpointBuilderRejectsRawDynamicRoutes: endpointBuilderAllowListMatrix.rawRouteRejectionEvidencePresent &&
        endpointBuilderAllowListMatrix.rejectedRawDynamicRequests.length === 9,
      mainIpcEndpointBuilderUsesSharedTemplates: endpointBuilderAllowListMatrix.schemaUsesSharedTemplates,
      mainIpcEndpointBuilderUnitEvidencePresent: endpointBuilderAllowListMatrix.unitTestEvidencePresent,
      mainIpcEndpointBuilderRejectsSingularUserInput: endpointBuilderAllowListMatrix.rawRouteRejectionEvidencePresent &&
        endpointBuilderAllowListMatrix.rejectedRawDynamicRequests.some((request) =>
          request.path === '/v1/user-input/input_raw'
        ),
      mainIpcRuntimeAdapterPreservesEncodedPaths: runtimeAdapterHandoffMatrix.encodedPathEvidencePresent &&
        runtimeAdapterHandoffMatrix.acceptedAdapterCalls.length === 8 &&
        runtimeAdapterHandoffMatrix.acceptedAdapterCalls.every((request) =>
          request.path.startsWith('/v1/') &&
          !Object.values(endpointBuilderAllowListMatrix.sourceIds).some((id) => request.path.includes(id)) &&
          Object.values(endpointBuilderAllowListMatrix.encodedIds).some((id) => request.path.includes(id))
        ),
      mainIpcRuntimeAdapterPreservesMethodAndBody: runtimeAdapterHandoffMatrix.methodAndBodyEvidencePresent &&
        runtimeAdapterHandoffMatrix.acceptedAdapterCalls.every((request) =>
          request.method === 'GET'
            ? !('body' in request)
            : 'body' in request && request.body.length > 0
        ),
      mainIpcRuntimeAdapterRejectsRawDynamicRoutes: runtimeAdapterHandoffMatrix.rejectsBeforeAdapterEvidencePresent &&
        runtimeAdapterHandoffMatrix.rejectedBeforeAdapterCalls.length === 7,
      mainIpcRuntimeAdapterRejectsBeforeCall: runtimeAdapterHandoffMatrix.parseBeforeAdapterCall &&
        runtimeAdapterHandoffMatrix.rejectsBeforeAdapterEvidencePresent,
      mainIpcRuntimeAdapterUnitEvidencePresent: runtimeAdapterHandoffMatrix.encodedPathEvidencePresent &&
        runtimeAdapterHandoffMatrix.methodAndBodyEvidencePresent &&
        runtimeAdapterHandoffMatrix.rejectsBeforeAdapterEvidencePresent,
      mainRuntimeHostHandoffPreservesEncodedPathAndQuery: runtimeHostHandoffMatrix.unitTestEvidencePresent &&
        runtimeHostHandoffMatrix.sourceEvidence.joinsBaseWithNormalizedPath &&
        runtimeHostHandoffMatrix.request.path.startsWith('/v1/') &&
        runtimeHostHandoffMatrix.request.path.includes(encodedMainIpcThread) &&
        runtimeHostHandoffMatrix.request.path.includes(encodedMainIpcTurn) &&
        runtimeHostHandoffMatrix.request.path.includes('timezone=Asia%2FShanghai') &&
        !runtimeHostHandoffMatrix.request.path.includes(mainIpcEndpointThreadId) &&
        !runtimeHostHandoffMatrix.request.path.includes(mainIpcEndpointTurnId),
      mainRuntimeHostHandoffPreservesMethodHeadersBody: runtimeHostHandoffMatrix.unitTestEvidencePresent &&
        runtimeHostHandoffMatrix.sourceEvidence.setsBearerAuth &&
        runtimeHostHandoffMatrix.sourceEvidence.forwardsCustomHeaders &&
        runtimeHostHandoffMatrix.sourceEvidence.defaultsJsonContentType &&
        runtimeHostHandoffMatrix.sourceEvidence.forwardsMethodAndBody &&
        runtimeHostHandoffMatrix.request.method === 'POST' &&
        runtimeHostHandoffMatrix.request.body === '{"action":"step"}' &&
        runtimeHostHandoffMatrix.request.authHeader === 'Bearer usage-token' &&
        runtimeHostHandoffMatrix.request.contentType === 'application/json' &&
        runtimeHostHandoffMatrix.request.customHeaderValue === 'runtime-host-path',
      mainRuntimeHostHandoffUsesEnsuredSettings: runtimeHostHandoffMatrix.sourceEvidence.usesEnsuredSettings &&
        runtimeHostHandoffMatrix.ensureRuntimePortEvidencePresent,
      mainRuntimeHostHandoffUnitEvidencePresent: runtimeHostHandoffMatrix.unitTestEvidencePresent &&
        runtimeHostHandoffMatrix.ensureRuntimePortEvidencePresent,
      mainSseHostEncodesThreadIdAndCursor: mainSseHostEncodingMatrix.sourceBuildsAnalytixEventsPath &&
        mainSseHostEncodingMatrix.encodedUnitEvidencePresent &&
        mainSseHostEncodingMatrix.request.path.startsWith('/v1/threads/') &&
        mainSseHostEncodingMatrix.request.path.endsWith('/events') &&
        mainSseHostEncodingMatrix.request.path.includes(encodedMainSseThread) &&
        !mainSseHostEncodingMatrix.request.path.includes(mainSseThreadId) &&
        mainSseHostEncodingMatrix.request.sinceSeq === 9 &&
        mainSseHostEncodingMatrix.request.lastEventId === '9',
      mainSseHostPreservesHeadersAndStreamId: mainSseHostEncodingMatrix.headerUnitEvidencePresent &&
        mainSseHostEncodingMatrix.errorUnitEvidencePresent &&
        mainSseHostEncodingMatrix.request.accept === 'text/event-stream' &&
        mainSseHostEncodingMatrix.request.authorization === 'Bearer runtime-token' &&
        mainSseHostEncodingMatrix.request.streamId === 'stream-encoded' &&
        mainSseHostEncodingMatrix.errorPayload.streamId === 'stream-encoded' &&
        mainSseHostEncodingMatrix.errorPayload.status === 404,
      mainSseHostRejectsForbiddenRouteTokens: mainSseHostEncodingMatrix.forbiddenRouteGuardPresent &&
        ![
          '/v1/reasonix',
          '/v1/runtime/go',
          '/v1/workflow',
          '/v1/create-loop',
          '/v1/subagents',
          '/v1/autoresearch',
          '/v1/mcp-indexer'
        ].some((token) => mainSseHostEncodingMatrix.request.path.includes(token)),
      mainSseHostEncodingUnitEvidencePresent: mainSseHostEncodingMatrix.encodedUnitEvidencePresent &&
        mainSseHostEncodingMatrix.headerUnitEvidencePresent &&
        mainSseHostEncodingMatrix.errorUnitEvidencePresent,
      sseStartSchemaStrict: sseEvidence.startSchemaStrict,
      sseRejectsReasonixSessionPayload: sseEvidence.rejectsReasonixSessionId,
      sseUsesAnalytixThreadEventsRoute: sseEvidence.usesAnalytixThreadEventsPath,
      sseStopParsesStreamId: sseEvidence.stopParsesStreamId,
      sseStopMatchesStreamIdOnly: sseEvidence.stopMatchesStreamIdOnly,
      sseReconnectCursorPreserved: sseEvidence.reconnectLastEventId,
      sseBatchesEventsAt100ms: sseEvidence.batchesEventsAt100ms,
      sseFlushesFirstEventAndBatchesAt16ms: sseEvidence.flushesFirstEventAndBatchesAt16ms,
      usesReasonixProtocol: false as const,
      rendererRouteExposed: false as const,
      defaultGoBackendEnabled: false as const
    }
  }
}

function rendererRouteSurfaceSovereigntyControlCase() {
  const sourceFiles = {
    workbench: 'src/renderer/src/components/Workbench.tsx' as const,
    workbenchRouteSurfaceTest: 'src/renderer/src/components/Workbench.route-surface.test.ts' as const,
    chatStoreTypes: 'src/renderer/src/store/chat-store-types.ts' as const,
    sidebar: 'src/renderer/src/components/chat/Sidebar.tsx' as const,
    pluginMarketplaceView: 'src/renderer/src/components/PluginMarketplaceView.tsx' as const,
    browserAnalytixBridge: 'src/renderer/src/lib/browser-analytix-bridge.ts' as const,
    workflowCreateLoopView: 'src/renderer/src/components/workflow/WorkflowCreateLoopView.tsx' as const,
    createLoopRuntime: 'src/renderer/src/workflow/create-loop-runtime.ts' as const,
    shellNavigationControls: 'src/renderer/src/components/shell/ShellNavigationControls.tsx' as const,
    workbenchShell: 'src/renderer/src/components/workbench/WorkbenchShell.tsx' as const,
    baseShellCss: 'src/renderer/src/styles/base-shell.css' as const
  }
  const workbenchSource = repoSource(sourceFiles.workbench)
  const routeSurfaceTestSource = repoSource(sourceFiles.workbenchRouteSurfaceTest)
  const chatStoreTypesSource = repoSource(sourceFiles.chatStoreTypes)
  const sidebarSource = repoSource(sourceFiles.sidebar)
  const pluginMarketplaceViewSource = repoSource(sourceFiles.pluginMarketplaceView)
  const browserAnalytixBridgeSource = repoSource(sourceFiles.browserAnalytixBridge)
  const workflowCreateLoopViewSource = repoSource(sourceFiles.workflowCreateLoopView)
  const createLoopRuntimeSource = repoSource(sourceFiles.createLoopRuntime)
  const shellNavigationControlsSource = repoSource(sourceFiles.shellNavigationControls)
  const workbenchShellSource = repoSource(sourceFiles.workbenchShell)
  const baseShellCssSource = repoSource(sourceFiles.baseShellCss)
  const topLevelRouteSurfaceSource = [
    workbenchSource,
    pluginMarketplaceViewSource,
    sidebarSource,
    chatStoreTypesSource,
    shellNavigationControlsSource
  ].join('\n')
  const appRoutes = ['chat', 'write', 'settings', 'plugins', 'claw', 'schedule'] as const
  const forbiddenTopLevelRouteTokens = [
    "'workflow'",
    '"workflow"',
    "'create-loop'",
    '"create-loop"',
    "'createLoop'",
    '"createLoop"',
    "'subagent'",
    '"subagent"',
    "'subagents'",
    '"subagents"',
    "'auto-research'",
    '"auto-research"',
    "'autoResearch'",
    '"autoResearch"',
    "'mcp-indexer'",
    '"mcp-indexer"',
    "'mcpIndexer'",
    '"mcpIndexer"'
  ]
  const forbiddenEntrypointSymbols = [
    'WorkflowCreateLoopView',
    'AutoResearch',
    'MCPIndexer',
    'McpIndexer',
    'openWorkflow',
    'openCreateLoop',
    'openSubagent',
    'openSubagents',
    'openAutoResearch',
    'openMCPIndexer',
    'openMcpIndexer'
  ]
  const quarantinedWorkflowSymbols = [
    'WorkflowCreateLoopView',
    'runCreateLoopWorkflow',
    'findPendingWorkflowGate',
    'analytix-create-loop'
  ]
  const forbiddenBrowserPreviewBridgeAliases = [
    'window.kun',
    'window.reasonix',
    'window.deepseek',
    'window.analytixGui',
    'kunGui',
    'reasonixGui'
  ]
  const forbiddenTopLevelRouteTokensPresent = forbiddenTopLevelRouteTokens.filter((token) =>
    topLevelRouteSurfaceSource.includes(token)
  )
  const forbiddenEntrypointSymbolsPresent = forbiddenEntrypointSymbols.filter((symbol) =>
    topLevelRouteSurfaceSource.includes(symbol)
  )
  const quarantinedWorkflowEntrySurfaceHits = quarantinedWorkflowSymbols.filter((symbol) =>
    topLevelRouteSurfaceSource.includes(symbol)
  )
  const appRouteUnionKunCompatible = chatStoreTypesSource.includes(
    "export type AppRoute = 'chat' | 'write' | 'settings' | 'plugins' | 'claw' | 'schedule'"
  ) && appRoutes.every((route) => routeSurfaceTestSource.includes(route))
  const evidence = {
    dormantWorkflowCodeExists: workflowCreateLoopViewSource.includes('WorkflowCreateLoopView') &&
      createLoopRuntimeSource.includes('runCreateLoopWorkflow'),
    browserPreviewInstallsWindowAnalytix: browserAnalytixBridgeSource.includes('window.analytix = createBrowserAnalytixApi') &&
      browserAnalytixBridgeSource.includes("document.documentElement.dataset.bridge = 'browser-preview'"),
    browserPreviewForbiddenAliasesAbsent: forbiddenBrowserPreviewBridgeAliases.every((alias) =>
      !browserAnalytixBridgeSource.includes(alias)
    ),
    pluginMarketplaceReceivesLeftSidebarCollapsed: workbenchSource.includes('<PluginMarketplaceView leftSidebarCollapsed={leftSidebarCollapsed} />') &&
      pluginMarketplaceViewSource.includes('leftSidebarCollapsed?: boolean') &&
      pluginMarketplaceViewSource.includes("data-left-sidebar-collapsed={leftSidebarCollapsed ? 'true' : 'false'}"),
    pluginMarketplaceTabsBeforeContent: pluginMarketplaceViewSource.indexOf('ds-plugin-marketplace-tabs') >= 0 &&
      pluginMarketplaceViewSource.indexOf('mx-auto w-full max-w-[960px]') >= 0 &&
      pluginMarketplaceViewSource.indexOf('ds-plugin-marketplace-tabs') <
        pluginMarketplaceViewSource.indexOf('mx-auto w-full max-w-[960px]'),
    shellNavigationControlsNoDrag: workbenchSource.includes('ShellNavigationControls') &&
      shellNavigationControlsSource.includes('ds-shell-navigation-controls ds-no-drag') &&
      workbenchShellSource.includes('ds-workbench-shell ds-no-drag') &&
      workbenchShellSource.includes('ds-no-drag ds-stage-surface') &&
      !workbenchShellSource.includes('ds-drag ds-stage-surface') &&
      !workbenchShellSource.includes('ds-workbench-shell ds-drag'),
    nativeSafeInsetCssPresent: baseShellCssSource.includes('--ds-window-controls-safe-block: calc(50px / var(--ds-ui-scale))') &&
      baseShellCssSource.includes('--ds-window-controls-safe-inset: calc(') &&
      baseShellCssSource.includes('.ds-shell-navigation-controls') &&
      baseShellCssSource.includes('.ds-plugin-marketplace-tabs[data-left-sidebar-collapsed')
  }
  return {
    sourceContractId: 'renderer-route-surface-sovereignty-v1' as const,
    sourceFiles,
    appRoutes: [...appRoutes],
    forbiddenTopLevelRouteTokens,
    forbiddenTopLevelRouteTokensPresent,
    forbiddenEntrypointSymbols,
    forbiddenEntrypointSymbolsPresent,
    quarantinedWorkflowSymbols,
    quarantinedWorkflowEntrySurfaceHits,
    evidence,
    productBoundary: {
      usesReasonixProtocol: false as const,
      topLevelHiddenEntryExposed: false as const,
      defaultGoBackendEnabled: false as const
    },
    expected: {
      appRouteUnionKunCompatible,
      noForbiddenTopLevelRouteTokens: forbiddenTopLevelRouteTokensPresent.length === 0,
      noForbiddenEntrypointSymbols: forbiddenEntrypointSymbolsPresent.length === 0,
      dormantWorkflowCodeQuarantined: evidence.dormantWorkflowCodeExists &&
        quarantinedWorkflowEntrySurfaceHits.length === 0,
      browserPreviewBridgeAnalytixOnly: evidence.browserPreviewInstallsWindowAnalytix &&
        evidence.browserPreviewForbiddenAliasesAbsent,
      pluginMarketplaceSafeAreaPropagates: evidence.pluginMarketplaceReceivesLeftSidebarCollapsed &&
        evidence.pluginMarketplaceTabsBeforeContent,
      shellNavigationNoDrag: evidence.shellNavigationControlsNoDrag,
      nativeControlsSafeInset: evidence.nativeSafeInsetCssPresent,
      usesReasonixProtocol: false as const,
      topLevelHiddenEntryExposed: false as const,
      defaultGoBackendEnabled: false as const
    }
  }
}

function goalPersistenceOffLockControlCase() {
  const sourceFiles = {
    threadService: 'packages/runtime/src/services-test-support/thread-service.ts' as const,
    threadServiceTest: 'packages/runtime/tests/thread-service.test.ts' as const
  }
  const threadServiceSource = repoSource(sourceFiles.threadService)
  const threadServiceTestSource = repoSource(sourceFiles.threadServiceTest)
  const setGoalSource = threadServiceSource.slice(
    threadServiceSource.indexOf('async setGoal('),
    threadServiceSource.indexOf('async appendGoalEvidence(')
  )
  const clearGoalSource = threadServiceSource.slice(
    threadServiceSource.indexOf('async clearGoal('),
    threadServiceSource.indexOf('async getTodos(')
  )
  const forbiddenLockSubstrings = [
    'controllerLock',
    'statusLock',
    'approvalLock',
    'withControllerLock',
    'withStatusLock',
    'withApprovalLock',
    'Mutex',
    'mutex',
    'lock.acquire'
  ] as const
  const forbiddenLockSubstringsPresent = forbiddenLockSubstrings
    .filter((item) => threadServiceSource.includes(item))
  const setGoalOrder = [
    setGoalSource.includes('touchThread({ ...current, goal }') ? 'touchThread' : '',
    setGoalSource.includes("await this.persistGoalThread(updated, 'set')") ? 'persistGoalThread:set' : '',
    setGoalSource.includes("kind: 'goal_updated'") ? 'goal_updated' : ''
  ].filter(Boolean)
  const clearGoalOrder = [
    clearGoalSource.includes('touchThread({ ...current }') ? 'touchThread' : '',
    clearGoalSource.includes("await this.persistGoalThread(updated, 'clear')") ? 'persistGoalThread:clear' : '',
    clearGoalSource.includes("kind: 'goal_cleared'") ? 'goal_cleared' : ''
  ].filter(Boolean)
  const persistGoalThreadSource = threadServiceSource.slice(
    threadServiceSource.indexOf('private async persistGoalThread('),
    threadServiceSource.indexOf('function cloneTurnForThread(')
  )
  const warningCallStart = persistGoalThreadSource.indexOf('console.warn(')
  const warningCallEnd = warningCallStart >= 0
    ? persistGoalThreadSource.indexOf('\n', warningCallStart)
    : -1
  const warningCallSource = warningCallStart >= 0
    ? persistGoalThreadSource
      .slice(warningCallStart, warningCallEnd >= 0 ? warningCallEnd : undefined)
      .trim()
    : ''
  const warningEvent = '[analytix] event=ANALYTIX_GOAL_PERSISTENCE_FAILED'
  const threadId = 'thr_goal_persist__/private/thread-pii-13900000000'
  const originalError = 'disk full: /private/error-pii-13900000001'
  const actionMarker = 'during set'
  const fixedWarningCall = `console.warn('${warningEvent}')`
  const warningEvidence = {
    warningEvent: warningCallSource === fixedWarningCall ? warningEvent : warningCallSource,
    threadId,
    originalError,
    actionMarker,
    logsPersistenceFailure:
      warningCallSource.startsWith('console.warn(') &&
      threadServiceTestSource.includes('toHaveBeenCalledTimes(1)'),
    surfacesOriginalError: threadServiceTestSource.includes('rejects.toBe(error)'),
    warningIsExactValueFreeEvent:
      warningCallSource === fixedWarningCall &&
      threadServiceTestSource.includes(`toHaveBeenCalledWith('${warningEvent}')`) &&
      threadServiceTestSource.includes('not.toContain(threadId)') &&
      threadServiceTestSource.includes('not.toContain(errorSentinel)') &&
      threadServiceTestSource.includes("not.toContain('during set')")
  }
  const expected = {
    setGoalPersistsBeforeEvent:
      setGoalOrder.join('>') === 'touchThread>persistGoalThread:set>goal_updated',
    clearGoalPersistsBeforeEvent:
      clearGoalOrder.join('>') === 'touchThread>persistGoalThread:clear>goal_cleared',
    noForbiddenControllerLock: forbiddenLockSubstringsPresent.length === 0,
    goalWritesOutsideSharedStatusApprovalLock: forbiddenLockSubstringsPresent.length === 0,
    persistenceFailureWarns: warningEvidence.logsPersistenceFailure,
    persistenceFailureSurfaces: warningEvidence.surfacesOriginalError,
    warningIsExactValueFreeEvent: warningEvidence.warningIsExactValueFreeEvent,
    warningIncludesThreadId: warningEvidence.warningEvent.includes(warningEvidence.threadId),
    warningIncludesOriginalError: warningEvidence.warningEvent.includes(warningEvidence.originalError),
    warningIncludesAction: warningEvidence.warningEvent.includes(warningEvidence.actionMarker),
    usesReasonixProtocol: false,
    statusApprovalLockShared: false,
    defaultGoBackendEnabled: false,
    rendererVisibleGoRoute: false
  }
  return {
    sourceContractId: 'thread-service-goal-persistence-v1' as const,
    sourceFiles,
    setGoalOrder,
    clearGoalOrder,
    forbiddenLockSubstrings: [...forbiddenLockSubstrings],
    forbiddenLockSubstringsPresent,
    warningEvidence,
    productBoundary: {
      usesReasonixProtocol: false,
      statusApprovalLockShared: false,
      defaultGoBackendEnabled: false,
      rendererVisibleGoRoute: false
    },
    expected
  }
}

function toolResultFileImageBoundaryControlCase() {
  const sourceFiles = {
    toolResultImage: 'packages/runtime/src/shared/tool-result-image.ts' as const,
    toolResultImageTest: 'packages/runtime/src/loop/tool-result-image.test.ts' as const,
    attachmentStoreTest: 'packages/runtime/tests/attachment-store.test.ts' as const,
    rendererMapperTest: 'src/renderer/src/agent/analytix-mapper.test.ts' as const
  }
  const imageSource = repoSource(sourceFiles.toolResultImage)
  const imageTestSource = repoSource(sourceFiles.toolResultImageTest)
  const attachmentTestSource = repoSource(sourceFiles.attachmentStoreTest)
  const mapperTestSource = repoSource(sourceFiles.rendererMapperTest)
  const inlineImageKinds = [
    imageSource.includes("'image'") ? 'image' : '',
    imageSource.includes("'computer_screenshot'") ? 'computer_screenshot' : ''
  ].filter(Boolean)
  const evictedPayloadMarkers = {
    payload: 'HUGE_BASE64_PAYLOAD',
    preservedMetadata: [
      imageTestSource.includes("toContain('computer_screenshot')") ? 'computer_screenshot' : '',
      imageTestSource.includes("toContain('1280')") ? '1280' : ''
    ].filter(Boolean)
  }
  const capPolicy = {
    historyImageCount: ['IMG_A', 'IMG_B', 'IMG_C', 'IMG_D']
      .filter((marker) => imageTestSource.includes(marker)).length,
    maxKept: imageTestSource.includes('capToolResultImages(history, 2)') ? 2 : 0,
    evictedCount:
      imageTestSource.includes('capped[0]') &&
      imageTestSource.includes('capped[1]') ? 2 : 0,
    newestImageDataBase64: imageTestSource.includes("'IMG_D'") ? 'IMG_D' : ''
  }
  const attachmentFallback = {
    localFilePath: '/tmp/picked/shot.png',
    textFallbackPrefix: '[Attached image as base64 text]',
    mimeType: 'image/webp',
    dimensions: '1280x720',
    fallbackBase64: 'YWJj',
    deepseekV4TextFallback:
      attachmentTestSource.includes("'deepseek-v4-pro'") &&
      attachmentTestSource.includes('attachmentTextFallbacks?.[0]')
  }
  const generatedFiles = {
    toolName: 'generate_image',
    attachmentId: 'att_abc',
    generatedRelativePath: '.analytix-images/img-1.png',
    speechRelativePath: '.analytix-audio/speech.mp3'
  }
  const expected = {
    inlineImageKindsPreserved:
      inlineImageKinds.join('>') === 'image>computer_screenshot',
    evictedBase64Omitted:
      imageTestSource.includes("not.toContain('HUGE_BASE64_PAYLOAD')") &&
      imageSource.includes("key === 'data_base64'"),
    evictedMetadataPreserved:
      evictedPayloadMarkers.preservedMetadata.join('>') === 'computer_screenshot>1280',
    onlyNewestImagesInline:
      capPolicy.historyImageCount === 4 &&
      capPolicy.maxKept === 2 &&
      capPolicy.evictedCount === 2 &&
      capPolicy.newestImageDataBase64 === 'IMG_D',
    attachmentLocalFilePathPreserved:
      attachmentTestSource.includes("localFilePath: '/tmp/picked/shot.png'") &&
      attachmentTestSource.includes('localFilePath: \'/tmp/picked/shot.png\''),
    textFallbackCarriesFilePath:
      attachmentTestSource.includes('FilePath: /tmp/picked/shot.png') &&
      attachmentTestSource.includes('[Attached image as base64 text]') &&
      attachmentTestSource.includes('MIME: image/webp') &&
      attachmentTestSource.includes('Dimensions: 1280x720') &&
      attachmentTestSource.includes('```base64\\nYWJj\\n```'),
    deepseekV4TextFallback: attachmentFallback.deepseekV4TextFallback,
    generatedFileMetaLifted:
      mapperTestSource.includes("toolName: 'generate_image'") &&
      mapperTestSource.includes("relativePath: '.analytix-images/img-1.png'") &&
      mapperTestSource.includes('meta?.generatedFiles'),
    toolAttachmentMetaLifted:
      mapperTestSource.includes("id: 'att_abc'") &&
      mapperTestSource.includes('meta?.attachments'),
    usesReasonixProtocol: false,
    topLevelRouteExposed: false,
    defaultGoBackendEnabled: false
  }
  return {
    sourceContractId: 'tool-result-file-image-boundary-v1' as const,
    sourceFiles,
    inlineImageKinds,
    evictedPayloadMarkers,
    capPolicy,
    attachmentFallback,
    generatedFiles,
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false,
      defaultGoBackendEnabled: false
    },
    expected
  }
}

function eventJsonlReplayBoundaryControlCase() {
  const sourceFiles = {
    fileSessionStore: 'packages/runtime/src/adapters/file/file-session-store.ts' as const,
    loopTest: 'packages/runtime/tests/loop.test.ts' as const,
    runtimeEventRecorderTest: 'packages/runtime/tests/runtime-event-recorder.test.ts' as const,
    fileSessionStoreTest: 'packages/runtime/tests/file-session-store.test.ts' as const
  }
  const fileSessionStoreSource = repoSource(sourceFiles.fileSessionStore)
  const loopTestSource = repoSource(sourceFiles.loopTest)
  const recorderTestSource = repoSource(sourceFiles.runtimeEventRecorderTest)
  const fileSessionStoreTestSource = repoSource(sourceFiles.fileSessionStoreTest)
  const jsonlFiles = [
    fileSessionStoreSource.includes("'events.jsonl'") ? 'events.jsonl' : '',
    fileSessionStoreSource.includes("'messages.jsonl'") ? 'messages.jsonl' : ''
  ].filter(Boolean)
  const recorderEvidence = {
    persistsBeforePublish:
      recorderTestSource.includes("order).toEqual(['persist', 'publish'])"),
    concurrentSeqsUnique:
      recorderTestSource.includes('never stamps the same seq twice') &&
      recorderTestSource.includes('Math.min(...seqs)).toBeGreaterThan(100)'),
    persistedHighWaterReadOnce:
      recorderTestSource.includes('reads the persisted high-water mark only once per thread') &&
      recorderTestSource.includes('toHaveBeenCalledTimes(1)')
  }
  const replayEvidence = {
    appendNewlineTerminated:
      fileSessionStoreSource.includes('appendFile(path, `${JSON.stringify(event)}\\n`,') &&
      loopTestSource.includes("content.endsWith('\\n')"),
    loadEventsSinceFiltersAndSorts:
      fileSessionStoreSource.includes('event.seq > sinceSeq') &&
      fileSessionStoreSource.includes('sort((a, b) => a.seq - b.seq)'),
    highestSeqUsesMax:
      fileSessionStoreSource.includes('Math.max(max, event.seq)'),
    malformedJsonlLineSkipped:
      loopTestSource.includes('survives a malformed JSONL line') &&
      loopTestSource.includes('expect(events).toHaveLength(1)')
  }
  const usageCompactionEvidence = {
    compactedSeqs: loopTestSource.includes('toEqual([1, 3, 5, 6, 7])')
      ? [1, 3, 5, 6, 7]
      : [],
    highestSeq: loopTestSource.includes("highestSeq('thr_usage_compact')).toBe(7)") ? 7 : 0,
    failureKeepsAppendedSeqs:
      fileSessionStoreTestSource.includes('toEqual([1, 2, 3])') ? [1, 2, 3] : [],
    warningPrefix: fileSessionStoreSource.includes('[analytix] usage event compaction failed')
      ? '[analytix] usage event compaction failed'
      : ''
  }
  const expected = {
    appendNewlineTerminated: replayEvidence.appendNewlineTerminated,
    loadEventsSinceFiltersAndSorts: replayEvidence.loadEventsSinceFiltersAndSorts,
    highestSeqPreservesMax: replayEvidence.highestSeqUsesMax,
    malformedJsonlLineSkipped: replayEvidence.malformedJsonlLineSkipped,
    persistsBeforePublish: recorderEvidence.persistsBeforePublish,
    concurrentSeqsUnique: recorderEvidence.concurrentSeqsUnique,
    persistedHighWaterReadOnce: recorderEvidence.persistedHighWaterReadOnce,
    usageCompactionKeepsCarryover:
      usageCompactionEvidence.compactedSeqs.join(',') === '1,3,5,6,7' &&
      usageCompactionEvidence.highestSeq === 7,
    compactionFailureKeepsAppendOnlyLog:
      usageCompactionEvidence.failureKeepsAppendedSeqs.join(',') === '1,2,3' &&
      usageCompactionEvidence.warningPrefix === '[analytix] usage event compaction failed',
    usesReasonixProtocol: false,
    topLevelRouteExposed: false,
    defaultGoBackendEnabled: false
  }
  return {
    sourceContractId: 'event-jsonl-replay-boundary-v1' as const,
    sourceFiles,
    jsonlFiles,
    recorderEvidence,
    replayEvidence,
    usageCompactionEvidence,
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false,
      defaultGoBackendEnabled: false
    },
    expected
  }
}

function mcpMalformedSchemaBoundaryControlCase() {
  const sourceFiles = {
    mcpToolProvider: 'packages/runtime/src/tool-test-support/tool/mcp-tool-provider.ts' as const,
    mcpToolProviderTest: 'packages/runtime/tests/mcp-tool-provider.test.ts' as const
  }
  const providerSource = repoSource(sourceFiles.mcpToolProvider)
  const providerTestSource = repoSource(sourceFiles.mcpToolProviderTest)
  const malformedTools = {
    nonObjectSchemaToolName: 'mcp_github_bad_schema',
    malformedRequiredToolName: 'mcp_github_bad_required',
    rawNonObjectSchemaKind: providerTestSource.includes("inputSchema: ['not', 'an', 'object']")
      ? 'array'
      : '',
    rawRequiredMixedCount:
      providerTestSource.includes("required: ['query', 1, false]") ? 3 : 0
  }
  const normalizedSchemas = {
    badSchema: {
      type: 'object',
      propertiesEmpty:
        providerTestSource.includes('properties: {}') &&
        providerTestSource.includes("tool.name === 'mcp_github_bad_schema'"),
      additionalProperties: providerTestSource.includes('additionalProperties: true')
    },
    badRequired: {
      type: 'object',
      required: providerTestSource.includes("required: ['query']") ? ['query'] : [],
      propertiesDropped:
        providerTestSource.includes("tool.name === 'mcp_github_bad_required'") &&
        !providerTestSource
          .slice(
            providerTestSource.indexOf("tool.name === 'mcp_github_bad_required'"),
            providerTestSource.indexOf('})', providerTestSource.indexOf("tool.name === 'mcp_github_bad_required'")) + 2
          )
          .includes('properties:')
    }
  }
  const normalizerEvidence = {
    nonRecordDefaults:
      providerSource.includes('if (!isRecord(schema)) return defaultMcpInputSchema()'),
    nonObjectTypeDefaults:
      providerSource.includes("typeof normalized.type === 'string'") &&
      providerSource.includes("normalized.type !== 'object'") &&
      providerSource.includes('return defaultMcpInputSchema()'),
    propertiesMustBeRecord:
      providerSource.includes('normalized.properties !== undefined') &&
      providerSource.includes('!isRecord(normalized.properties)') &&
      providerSource.includes('delete normalized.properties'),
    requiredFiltersStrings:
      providerSource.includes('normalized.required.filter') &&
      providerSource.includes("typeof value === 'string'"),
    outputSchemaRecordOnly:
      providerSource.includes('return isRecord(schema) ? schema : undefined')
  }
  const expected = {
    nonObjectSchemaDefaults:
      malformedTools.rawNonObjectSchemaKind === 'array' &&
      normalizedSchemas.badSchema.propertiesEmpty &&
      normalizedSchemas.badSchema.additionalProperties,
    propertiesArrayDropped: normalizedSchemas.badRequired.propertiesDropped,
    requiredNonStringsDropped:
      malformedTools.rawRequiredMixedCount === 3 &&
      normalizedSchemas.badRequired.required.join(',') === 'query',
    advertisedToolNamesPreserved:
      providerTestSource.includes('mcp_github_bad_schema') &&
      providerTestSource.includes('mcp_github_bad_required'),
    modelCatalogSchemaSafe:
      normalizerEvidence.nonRecordDefaults &&
      normalizerEvidence.propertiesMustBeRecord &&
      normalizerEvidence.requiredFiltersStrings,
    outputSchemaNonRecordOmitted: normalizerEvidence.outputSchemaRecordOnly,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false,
    defaultGoBackendEnabled: false
  }
  return {
    sourceContractId: 'mcp-malformed-schema-boundary-v1' as const,
    sourceFiles,
    malformedTools,
    normalizedSchemas,
    normalizerEvidence,
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false,
      defaultGoBackendEnabled: false
    },
    expected
  }
}

function approvalUserInputReplayOutput(
  approvalUserInput: ReturnType<typeof ApprovalUserInputRouteContract.parse>
) {
  const approvalResponseBody = approvalUserInput.approval.expectedResponse.body
  if (!approvalResponseBody) {
    throw new Error('approval expectedResponse.body is required')
  }
  return {
    approvalRoute: {
      itemId: approvalUserInput.approval.itemId,
      toolName: approvalUserInput.approval.toolName,
      summary: approvalUserInput.approval.summary,
      requestBody: approvalUserInput.approval.decisionRequest,
      responseStatus: approvalUserInput.approval.expectedResponse.status,
      responseBody: approvalResponseBody,
      pendingBefore: approvalUserInput.approval.pendingBefore,
      pendingAfter: approvalUserInput.approval.pendingAfter
    },
    approvalId: approvalUserInput.approval.id,
    approvalDecision: approvalUserInput.approval.decisionRequest.decision,
    approvalStatus: approvalResponseBody.status,
    approvalPendingAfter: approvalUserInput.approval.pendingAfter,
    secondApprovalDecisionStatus: approvalUserInput.approval.secondDecisionStatus,
    replaySinceSeq: approvalUserInput.replay.sinceSeq,
    replayKindsInOrder: approvalUserInput.replay.expectedKindsInOrder,
    submittedInputId: approvalUserInput.submittedUserInput.id,
    submittedStatus: approvalUserInput.submittedUserInput.expectedResolution.status,
    userInputSubmitRoute: {
      threadId: approvalUserInput.submittedUserInput.threadId,
      itemId: approvalUserInput.submittedUserInput.itemId,
      prompt: approvalUserInput.submittedUserInput.prompt,
      questions: promptQuestionReplay(approvalUserInput.submittedUserInput.questions),
      requestBody: approvalUserInput.submittedUserInput.resolveRequest,
      responseStatus: approvalUserInput.submittedUserInput.expectedResponse.status,
      responseBody: approvalUserInput.submittedUserInput.expectedResponse.body,
      resolvedEvent: approvalUserInput.submittedUserInput.resolvedEvent,
      pendingBefore: approvalUserInput.submittedUserInput.pendingBefore,
      pendingAfter: approvalUserInput.submittedUserInput.pendingAfter
    },
    answerCount: approvalUserInput.submittedUserInput.expectedResolution.answers.length,
    httpEchoesAnswers: approvalUserInput.submittedUserInput.expectedResponse.body?.answers !== undefined,
    resolvedEventKind: approvalUserInput.submittedUserInput.resolvedEvent.kind,
    resolvedEventIncludesAnswers: approvalUserInput.submittedUserInput.resolvedEvent.includesAnswers,
    cancelledInputId: approvalUserInput.userInput.id,
    cancelledStatus: approvalUserInput.userInput.expectedResponse.body?.status,
    userInputCancelRoute: {
      itemId: approvalUserInput.userInput.itemId,
      prompt: approvalUserInput.userInput.prompt,
      questions: promptQuestionReplay(approvalUserInput.userInput.questions),
      requestBody: approvalUserInput.userInput.resolveRequest,
      responseStatus: approvalUserInput.userInput.expectedResponse.status,
      responseBody: approvalUserInput.userInput.expectedResponse.body,
      secondResolveStatus: approvalUserInput.userInput.secondResolveStatus,
      pendingBefore: approvalUserInput.userInput.pendingBefore,
      pendingAfter: approvalUserInput.userInput.pendingAfter
    },
    lateResolveRejected: approvalUserInput.userInput.secondResolveStatus === 404,
    pendingAfterSubmit: approvalUserInput.submittedUserInput.pendingAfter,
    pendingAfterCancel: approvalUserInput.userInput.pendingAfter,
    abortApprovalId: approvalUserInput.abortCleanup.approvalId,
    abortApprovalStatus: approvalUserInput.abortCleanup.expectedApprovalStatus,
    abortUserInputId: approvalUserInput.abortCleanup.userInputId,
    abortUserInputStatus: approvalUserInput.abortCleanup.expectedUserInputStatus,
    lateApprovalDecisionStatus: approvalUserInput.abortCleanup.lateApprovalDecisionStatus,
    lateUserInputResolveStatus: approvalUserInput.abortCleanup.lateUserInputResolveStatus,
    pendingAfterAbortCleanup: 0,
    abortReplayKinds: approvalUserInput.abortCleanup.expectedReplayKinds
  }
}

function approvalUserInputRouteReplayControlCase(
  approvalUserInput: ReturnType<typeof ApprovalUserInputRouteContract.parse>
) {
  const { id, approval, replay, userInput, submittedUserInput, abortCleanup } = approvalUserInput
  return {
    sourceContractId: approvalUserInput.id,
    contract: { id, approval, replay, userInput, submittedUserInput, abortCleanup },
    expected: approvalUserInputReplayOutput(approvalUserInput)
  }
}

function approvalUserInputInventoryInput(
  approvalUserInput: ReturnType<typeof ApprovalUserInputRouteContract.parse>
) {
  return {
    approvalId: approvalUserInput.approval.id,
    approvalPendingAfter: approvalUserInput.approval.pendingAfter,
    submittedInputId: approvalUserInput.submittedUserInput.id,
    cancelledInputId: approvalUserInput.userInput.id,
    abortApprovalId: approvalUserInput.abortCleanup.approvalId,
    abortUserInputId: approvalUserInput.abortCleanup.userInputId,
    replayKindsInOrder: approvalUserInput.replay.expectedKindsInOrder,
    abortReplayKinds: approvalUserInput.abortCleanup.expectedReplayKinds,
    answerCount: approvalUserInput.submittedUserInput.expectedResolution.answers.length,
    httpEchoesAnswers: approvalUserInput.submittedUserInput.expectedResponse.body?.answers !== undefined,
    resolvedEventIncludesAnswers: approvalUserInput.submittedUserInput.resolvedEvent.includesAnswers,
    lateApprovalDecisionStatus: approvalUserInput.abortCleanup.lateApprovalDecisionStatus,
    lateUserInputResolveStatus: approvalUserInput.abortCleanup.lateUserInputResolveStatus,
    pendingAfterSubmit: approvalUserInput.submittedUserInput.pendingAfter,
    pendingAfterCancel: approvalUserInput.userInput.pendingAfter,
    pendingAfterAbortCleanup: 0
  }
}

function approvalUserInputInventoryOutput(
  approvalUserInput: ReturnType<typeof ApprovalUserInputRouteContract.parse>
) {
  const input = approvalUserInputInventoryInput(approvalUserInput)
  return {
    gateIds: [
      input.approvalId,
      input.submittedInputId,
      input.cancelledInputId,
      input.abortApprovalId,
      input.abortUserInputId
    ],
    approvalIds: [
      input.approvalId,
      input.abortApprovalId
    ],
    userInputIds: [
      input.submittedInputId,
      input.cancelledInputId,
      input.abortUserInputId
    ],
    routeKinds: [
      'approval-decision',
      'user-input-submit',
      'user-input-cancel',
      'abort-cleanup'
    ],
    replayKindsInOrder: input.replayKindsInOrder,
    abortReplayKinds: input.abortReplayKinds,
    answerCount: input.answerCount,
    httpEchoesAnswers: input.httpEchoesAnswers,
    resolvedEventIncludesAnswers: input.resolvedEventIncludesAnswers,
    lateApprovalDecisionStatus: input.lateApprovalDecisionStatus,
    lateUserInputResolveStatus: input.lateUserInputResolveStatus,
    pendingAfterAll: input.approvalPendingAfter +
      input.pendingAfterSubmit +
      input.pendingAfterCancel +
      input.pendingAfterAbortCleanup,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function approvalUserInputInventoryControlCase(
  approvalUserInput: ReturnType<typeof ApprovalUserInputRouteContract.parse>
) {
  return {
    sourceContractId: approvalUserInput.id,
    input: approvalUserInputInventoryInput(approvalUserInput),
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: approvalUserInputInventoryOutput(approvalUserInput)
  }
}

function g5ProductBoundaryOutput(g5: ReturnType<typeof GoG5FullLoopContract.parse>) {
  return {
    ...g5.productBoundary,
    electronMainConnected: false,
    defaultGoBackendEnabled: false,
    usesReasonixProtocol: g5.productBoundary.reasonixPublicProtocolAllowed,
    rendererRouteExposed: g5.productBoundary.rendererVisibleGoRoutesAllowed
  }
}

function g5ProductBoundaryControlCase(g5: ReturnType<typeof GoG5FullLoopContract.parse>) {
  return {
    sourceContractId: g5.id,
    productBoundary: g5.productBoundary,
    electronMainConnected: false,
    defaultGoBackendEnabled: false,
    expected: g5ProductBoundaryOutput(g5)
  }
}

function taskJobToolContractBoundaryOutput(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  return {
    taskToolName: taskJob.toolContracts.task.name,
    parallelToolName: taskJob.toolContracts.parallelTasks.name,
    taskInternalRuntimeOnly: taskJob.toolContracts.task.internalRuntimeOnly,
    parallelInternalRuntimeOnly: taskJob.toolContracts.parallelTasks.internalRuntimeOnly,
    requiresPermissionGate: taskJob.toolContracts.task.requiresPermissionGate,
    mayAppendParentGoalEvidence: taskJob.toolContracts.task.mayAppendParentGoalEvidence,
    requiresDependencyValidation: taskJob.toolContracts.parallelTasks.requiresDependencyValidation,
    requiresPlannerReadOnlyToolset: taskJob.toolContracts.parallelTasks.requiresPlannerReadOnlyToolset,
    routes: [
      taskJob.routeContract.wait,
      taskJob.routeContract.output,
      taskJob.routeContract.kill
    ],
    protectedRoutes: taskJob.routeContract.authMatrix.protectedRoutes,
    unauthorizedStatus: taskJob.routeContract.authMatrix.unauthorizedStatus,
    forbiddenTopLevelRoutes: taskJob.routeContract.forbiddenTopLevelRoutes,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function taskJobToolContractBoundaryControlCase(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  return {
    sourceContractId: taskJob.id,
    task: taskJob.toolContracts.task,
    parallelTasks: taskJob.toolContracts.parallelTasks,
    routes: [
      taskJob.routeContract.wait,
      taskJob.routeContract.output,
      taskJob.routeContract.kill
    ],
    protectedRoutes: taskJob.routeContract.authMatrix.protectedRoutes,
    unauthorizedStatus: taskJob.routeContract.authMatrix.unauthorizedStatus,
    forbiddenTopLevelRoutes: taskJob.routeContract.forbiddenTopLevelRoutes,
    expected: taskJobToolContractBoundaryOutput(taskJob)
  }
}

function taskJobTranscriptIdentityOutput(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  return {
    sourceId: taskJob.transcript.sourceId,
    continueTargetId: taskJob.transcript.continueTargetId,
    forkTargetId: taskJob.transcript.forkTargetId,
    incompatibleError: taskJob.transcript.incompatibleError,
    sameTranscriptIdentityRequired: taskJob.transcript.sameIdentityRequired,
    continuePreservesTarget: taskJob.transcript.continuePreservesTarget,
    forkCreatesDistinctTarget: taskJob.transcript.forkCreatesDistinctTarget,
    continueTargetMatchesSource: taskJob.transcript.continueTargetId === taskJob.transcript.sourceId,
    forkTargetDistinctFromSource: taskJob.transcript.forkTargetId !== taskJob.transcript.sourceId,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function taskJobTranscriptIdentityControlCase(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  return {
    sourceContractId: taskJob.id,
    transcript: taskJob.transcript,
    expected: taskJobTranscriptIdentityOutput(taskJob)
  }
}

function taskJobNestedSseMetadataOutput(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  const fields = taskJob.nestedEvent.nestedSseMetadataFields
  return {
    parentCallId: taskJob.nestedEvent.parentCallId,
    childRunId: taskJob.nestedEvent.childRunId,
    nestedSseMetadataFields: fields,
    includesParentCallId: fields.includes('parentCallId'),
    includesChildRunId: fields.includes('childRunId'),
    includesEvidenceLedgered: fields.includes(taskJob.parentGoalEvidence.evidenceLedgeredEventKey),
    includesEvidenceLedgerError: fields.includes(taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey),
    parentChildDistinct: taskJob.nestedEvent.parentCallId !== taskJob.nestedEvent.childRunId,
    requiresActiveGoal: taskJob.parentGoalEvidence.requiresActiveGoal,
    evidenceLedgeredEventKey: taskJob.parentGoalEvidence.evidenceLedgeredEventKey,
    evidenceLedgerErrorEventKey: taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function taskJobNestedSseMetadataControlCase(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  return {
    sourceContractId: taskJob.id,
    nestedEvent: taskJob.nestedEvent,
    parentGoalEvidence: taskJob.parentGoalEvidence,
    expected: taskJobNestedSseMetadataOutput(taskJob)
  }
}

function taskJobPlannerToolsetInventoryOutput(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  const taskToolNames = [
    taskJob.toolContracts.task.name,
    taskJob.toolContracts.parallelTasks.name
  ]
  return {
    readOnlyToolset: taskJob.permissions.plannerReadOnlyToolset,
    forbiddenToolset: taskJob.permissions.plannerForbiddenToolset,
    readOnlyToolCount: taskJob.permissions.plannerReadOnlyToolset.length,
    forbiddenToolCount: taskJob.permissions.plannerForbiddenToolset.length,
    readOnlyExcludesTaskTools: taskToolNames.every(
      (toolName) => !taskJob.permissions.plannerReadOnlyToolset.includes(toolName)
    ),
    forbiddenMatchesTaskTools: taskToolNames.every(
      (toolName) => taskJob.permissions.plannerForbiddenToolset.includes(toolName)
    ),
    plannerPolicy: taskJob.plannerExecutor.plannerPolicy,
    executorPolicy: taskJob.plannerExecutor.executorPolicy,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function taskJobPlannerToolsetInventoryControlCase(taskJob: ReturnType<typeof TaskJobOrchestrationContract.parse>) {
  return {
    readOnlyToolset: taskJob.permissions.plannerReadOnlyToolset,
    forbiddenToolset: taskJob.permissions.plannerForbiddenToolset,
    taskToolName: taskJob.toolContracts.task.name,
    parallelToolName: taskJob.toolContracts.parallelTasks.name,
    plannerPolicy: taskJob.plannerExecutor.plannerPolicy,
    executorPolicy: taskJob.plannerExecutor.executorPolicy,
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: taskJobPlannerToolsetInventoryOutput(taskJob)
  }
}

function mcpSearchRefreshDriftOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  const refreshDrift = mcp.searchMetaTools.refreshDrift
  return {
    serverId: refreshDrift.serverId,
    initialToolNames: refreshDrift.initialToolNames,
    expandedToolNames: refreshDrift.expandedToolNames,
    totalIndexed: refreshDrift.expectedTotalIndexed,
    catalogDrift: refreshDrift.expectedCatalogDrift,
    topLevelRouteExposed: false
  }
}

function mcpSearchRefreshDriftControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    refreshDrift: mcp.searchMetaTools.refreshDrift,
    expected: mcpSearchRefreshDriftOutput(mcp)
  }
}

function mcpCoreLifecycleOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    providerId: mcp.providerId,
    connectToolNames: mcp.connect.toolNames,
    connectAvailable: mcp.connect.diagnostic.available,
    connectToolCount: mcp.connect.diagnostic.toolCount,
    disconnectReason: mcp.disconnect.reason,
    disconnectToolNames: mcp.disconnect.toolNames,
    disconnectAvailable: mcp.disconnect.diagnostic.available,
    disconnectToolCount: mcp.disconnect.diagnostic.toolCount,
    reloadToolNames: mcp.reload.toolNames,
    schemaOrderStable: mcp.reload.schemaOrderStable,
    cancelErrorSubstring: mcp.cancel.errorSubstring,
    cancelExecuted: mcp.cancel.executed,
    errorCode: mcp.error.code,
    errorApproved: mcp.error.approved,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function mcpCoreLifecycleControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  const { connect, disconnect, reload, cancel, error } = mcp
  return {
    sourceContractId: mcp.id,
    providerId: mcp.providerId,
    coreLifecycle: { connect, disconnect, reload, cancel, error },
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: mcpCoreLifecycleOutput(mcp)
  }
}

function mcpBackgroundReconnectOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  const reconnect = mcp.backgroundReconnect
  return {
    failedServerIds: reconnect.failedServerIds,
    suspendedProviderId: reconnect.suspendedProviderId,
    suspendedReason: reconnect.suspendedReason,
    connectedServerIds: reconnect.expectedConnectedServerIds,
    errorServerIds: reconnect.expectedErrorServerIds,
    attemptsPerFailedServer: reconnect.attemptsPerFailedServer,
    retryAllFailedServers:
      reconnect.failedServerIds.length ===
      reconnect.expectedConnectedServerIds.length + reconnect.expectedErrorServerIds.length,
    requiresRuntimeRestart: reconnect.requiresRuntimeRestart
  }
}

function mcpBackgroundReconnectControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    backgroundReconnect: mcp.backgroundReconnect,
    expected: mcpBackgroundReconnectOutput(mcp)
  }
}

function mcpCallReconnectOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  const reconnect = mcp.callReconnect
  return {
    serverId: reconnect.serverId,
    toolName: reconnect.toolName,
    normalizedToolName: reconnect.normalizedToolName,
    transportErrorRetried: reconnect.retryOnTransportError &&
      reconnect.staleConnection.factoryAttempts === reconnect.maxAttempts,
    protocolErrorRetried: reconnect.retryOnProtocolError,
    maxAttempts: reconnect.maxAttempts,
    staleFactoryAttempts: reconnect.staleConnection.factoryAttempts,
    staleCloseCount: reconnect.staleConnection.closeCount,
    staleResultInstance: reconnect.staleConnection.resultInstance,
    staleCallSucceeded: !reconnect.staleConnection.isError,
    protocolFactoryAttempts: reconnect.protocolFailure.factoryAttempts,
    protocolCloseCount: reconnect.protocolFailure.closeCount,
    protocolErrorCode: reconnect.protocolFailure.code,
    protocolCallReturnedError: reconnect.protocolFailure.isError,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function mcpCallReconnectControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    callReconnect: mcp.callReconnect,
    expected: mcpCallReconnectOutput(mcp)
  }
}

function mcpKnownOverrideDiagnosticRows(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return mcp.knownOverrideVariants.map((variant) => ({
    serverId: variant.serverId,
    knownOverride: variant.diagnostic.knownOverride,
    effectiveCwd: variant.diagnostic.effectiveCwd,
    lowPriority: variant.diagnostic.lowPriority,
    backgroundStart: variant.diagnostic.backgroundStart,
    workspaceRoot: variant.workspaceRoot,
    ...(variant.explicitCwd ? { explicitCwd: variant.explicitCwd } : {}),
    ...(variant.daemonIdleTimeoutMs ? { daemonIdleTimeoutMs: variant.daemonIdleTimeoutMs } : {})
  }))
}

function mcpKnownOverrideDiagnosticsOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    diagnostics: mcpKnownOverrideDiagnosticRows(mcp),
    variantCount: mcp.knownOverrideVariants.length,
    knownOverrideKinds: mcp.knownOverrideVariants.map((variant) => variant.diagnostic.knownOverride),
    workspaceRoots: mcp.knownOverrideVariants.map((variant) => variant.workspaceRoot),
    explicitCwdServerIds: mcp.knownOverrideVariants
      .filter((variant) => Boolean(variant.explicitCwd))
      .map((variant) => variant.serverId),
    daemonIdleTimeoutServerIds: mcp.knownOverrideVariants
      .filter((variant) => Boolean(variant.daemonIdleTimeoutMs))
      .map((variant) => variant.serverId),
    allLowPriority: mcp.knownOverrideVariants.every((variant) => variant.diagnostic.lowPriority),
    allBackgroundStart: mcp.knownOverrideVariants.every((variant) => variant.diagnostic.backgroundStart),
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function mcpKnownOverrideDiagnosticsControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    knownOverrideVariants: mcp.knownOverrideVariants,
    expected: mcpKnownOverrideDiagnosticsOutput(mcp)
  }
}

function mcpLiveLocalIndexerOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  const liveLocalMcp = runLiveLocalIndexerContract(mcp.liveLocalIndexer)

  return {
    serverId: mcp.liveLocalIndexer.serverId,
    cwd: mcp.liveLocalIndexer.cwd,
    lowPriority: mcp.liveLocalIndexer.lowPriority,
    backgroundStart: mcp.liveLocalIndexer.backgroundStart,
    retryServerIds: mcp.liveLocalIndexer.failedServerIds,
    attemptsPerFailedServer: mcp.liveLocalIndexer.attemptsPerFailedServer,
    retryAttempts: Object.fromEntries(mcp.liveLocalIndexer.failedServerIds.map((serverId) => [
      serverId,
      mcp.liveLocalIndexer.attemptsPerFailedServer
    ])),
    initialPaths: mcp.liveLocalIndexer.initialFiles.map((file) => file.path),
    resumePaths: mcp.liveLocalIndexer.resumeFiles.map((file) => file.path),
    activePaths: liveLocalMcp.activePaths,
    tombstoneCount: liveLocalMcp.tombstoneCount,
    restartedFromSnapshot: liveLocalMcp.restartedFromSnapshot,
    lateTombstonePath: mcp.liveLocalIndexer.lateTombstonePath,
    secretSafeDiagnostic: liveLocalMcp.secretSafeDiagnostic,
    leaksSecret: liveLocalMcp.secretSafeDiagnostic.includes(mcp.liveLocalIndexer.secretDiagnostic),
    executionErrorToolName: mcp.liveLocalIndexer.executionError.toolName,
    executionErrorIsError: mcp.liveLocalIndexer.executionError.isError,
    executionErrorSafe: mcp.liveLocalIndexer.executionError.secretSafeError,
    executionErrorLeaksSecret: mcp.liveLocalIndexer.executionError.secretSafeError.includes(
      mcp.liveLocalIndexer.secretDiagnostic
    ),
    topLevelRouteExposed: false,
    usesReasonixProtocol: false
  }
}

function mcpLiveLocalIndexerControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    liveLocalIndexer: mcp.liveLocalIndexer,
    expected: mcpLiveLocalIndexerOutput(mcp)
  }
}

function mcpApprovalAnnotationOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    serverId: mcp.approvalAnnotations.serverId,
    toolName: mcp.approvalAnnotations.toolName,
    normalizedToolName: mcp.approvalAnnotations.normalizedToolName,
    destructiveHint: mcp.approvalAnnotations.annotations.destructiveHint,
    openWorldHint: mcp.approvalAnnotations.annotations.openWorldHint,
    approvalId: mcp.approvalAnnotations.approvalId,
    decision: mcp.approvalAnnotations.decision,
    resultKind: mcp.approvalAnnotations.resultKind,
    executed: mcp.approvalAnnotations.executed,
    deniedNoExecute: !mcp.approvalAnnotations.executed
  }
}

function mcpApprovalAnnotationControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    approvalAnnotations: mcp.approvalAnnotations,
    expected: mcpApprovalAnnotationOutput(mcp)
  }
}

function mcpSearchMetaToolsOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    toolNames: mcp.searchMetaTools.toolNames,
    toolCount: mcp.searchMetaTools.toolNames.length,
    refreshToolAdvertised: mcp.searchMetaTools.toolNames.includes('mcp_refresh_catalog'),
    trustedWorkspace: mcp.searchMetaTools.trustedWorkspace,
    untrustedWorkspace: mcp.searchMetaTools.untrustedWorkspace,
    query: mcp.searchMetaTools.query,
    trustedToolId: mcp.searchMetaTools.trustedToolId,
    untrustedSearchedTools: mcp.searchMetaTools.untrustedSearchedTools,
    unknownToolError: mcp.searchMetaTools.unknownToolError,
    callPolicy: mcp.searchMetaTools.callPolicy,
    deniedCallExecuted: mcp.searchMetaTools.deniedCallExecuted,
    deniedNoExecute: !mcp.searchMetaTools.deniedCallExecuted,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function mcpSearchMetaToolsControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    searchMetaTools: mcp.searchMetaTools,
    expected: mcpSearchMetaToolsOutput(mcp)
  }
}

function mcpSearchWorkspaceBoundaryContract(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    trustedWorkspace: mcp.searchMetaTools.trustedWorkspace,
    untrustedWorkspace: mcp.searchMetaTools.untrustedWorkspace,
    query: mcp.searchMetaTools.query,
    trustedToolId: mcp.searchMetaTools.trustedToolId,
    untrustedSearchedTools: mcp.searchMetaTools.untrustedSearchedTools,
    unknownToolError: mcp.searchMetaTools.unknownToolError,
    callPolicy: mcp.searchMetaTools.callPolicy,
    deniedCallExecuted: mcp.searchMetaTools.deniedCallExecuted
  }
}

function mcpSearchWorkspaceBoundaryOutput(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  const boundary = mcpSearchWorkspaceBoundaryContract(mcp)
  return {
    trustedWorkspace: boundary.trustedWorkspace,
    untrustedWorkspace: boundary.untrustedWorkspace,
    query: boundary.query,
    trustedToolId: boundary.trustedToolId,
    untrustedSearchedTools: boundary.untrustedSearchedTools,
    unknownToolError: boundary.unknownToolError,
    callPolicy: boundary.callPolicy,
    deniedNoExecute: !boundary.deniedCallExecuted
  }
}

function mcpSearchWorkspaceBoundaryControlCase(mcp: ReturnType<typeof McpToolLifecycleContract.parse>) {
  return {
    sourceContractId: mcp.id,
    searchWorkspaceBoundary: mcpSearchWorkspaceBoundaryContract(mcp),
    expected: mcpSearchWorkspaceBoundaryOutput(mcp)
  }
}

type ParsedProviderUsage = {
  promptTokens?: number
  completionTokens?: number
  reasoningTokens?: number
  totalTokens?: number
  cacheHitTokens?: number
  cacheMissTokens?: number
  cacheHitRate?: number | null
}

function parsedProviderUsageFromRawPayload(testCase: ProviderUsageCase): ParsedProviderUsage {
  const usage = usageBody(testCase)
  const endpointFormat = testCase.endpointFormat ?? 'chat_completions'
  if (endpointFormat === 'responses') {
    const inputTokens = numberValue(usage, 'input_tokens')
    const outputTokens = numberValue(usage, 'output_tokens')
    const inputDetails = recordValue(usage.input_tokens_details)
    const outputDetails = recordValue(usage.output_tokens_details)
    const cachedTokens = numberValue(inputDetails, 'cached_tokens')
    return {
      promptTokens: inputTokens,
      completionTokens: outputTokens,
      reasoningTokens: numberValue(outputDetails, 'reasoning_tokens'),
      totalTokens: numberValue(usage, 'total_tokens'),
      cacheHitTokens: cachedTokens,
      cacheMissTokens: inputTokens - cachedTokens,
      cacheHitRate: cachedTokens / inputTokens
    }
  }
  if (endpointFormat === 'messages') {
    const inputTokens = numberValue(usage, 'input_tokens')
    const outputTokens = numberValue(usage, 'output_tokens')
    const cacheRead = numberValue(usage, 'cache_read_input_tokens')
    const cacheCreation = numberValue(usage, 'cache_creation_input_tokens')
    const promptTokens = inputTokens + cacheRead + cacheCreation
    const cacheMissTokens = inputTokens + cacheCreation
    return {
      promptTokens,
      completionTokens: outputTokens,
      totalTokens: promptTokens + outputTokens,
      cacheHitTokens: cacheRead,
      cacheMissTokens,
      cacheHitRate: cacheRead / (cacheRead + cacheMissTokens)
    }
  }
  const promptTokens = numberValue(usage, 'prompt_tokens')
  const promptDetails = recordValue(usage.prompt_tokens_details)
  const completionDetails = recordValue(usage.completion_tokens_details)
  const nativeHitTokens = numberValue(usage, 'prompt_cache_hit_tokens')
  const nativeMissTokens = numberValue(usage, 'prompt_cache_miss_tokens')
  const cachedTokens = numberValue(promptDetails, 'cached_tokens')
  const hasNativeCacheTelemetry = nativeHitTokens > 0 || nativeMissTokens > 0
  const hasOpenAICompatibleCacheTelemetry = cachedTokens > 0
  const parsed: ParsedProviderUsage = {
    promptTokens,
    completionTokens: numberValue(usage, 'completion_tokens'),
    reasoningTokens: numberValue(completionDetails, 'reasoning_tokens') || undefined,
    totalTokens: numberValue(usage, 'total_tokens')
  }
  if (hasNativeCacheTelemetry) {
    parsed.cacheHitTokens = nativeHitTokens
    parsed.cacheMissTokens = nativeMissTokens
    parsed.cacheHitRate = nativeHitTokens / (nativeHitTokens + nativeMissTokens)
  } else if (hasOpenAICompatibleCacheTelemetry) {
    parsed.cacheHitTokens = cachedTokens
    parsed.cacheMissTokens = promptTokens - cachedTokens
    parsed.cacheHitRate = cachedTokens / promptTokens
  } else {
    parsed.cacheHitRate = null
  }
  return parsed
}

function usageNumberMatches(left: number | null | undefined, right: number | null | undefined): boolean {
  return left === right
}

function parsedUsageMatchesExpected(testCase: ProviderUsageCase): boolean {
  const parsed = parsedProviderUsageFromRawPayload(testCase)
  const expected = testCase.expectedUsage
  return usageNumberMatches(parsed.promptTokens, expected.promptTokens) &&
    usageNumberMatches(parsed.completionTokens, expected.completionTokens) &&
    usageNumberMatches(parsed.reasoningTokens, expected.reasoningTokens) &&
    usageNumberMatches(parsed.totalTokens, expected.totalTokens) &&
    usageNumberMatches(parsed.cacheHitTokens, expected.cacheHitTokens) &&
    usageNumberMatches(parsed.cacheMissTokens, expected.cacheMissTokens) &&
    usageNumberMatches(parsed.cacheHitRate, expected.cacheHitRate)
}

function providerCacheAccounting(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const parsedCases = providerCache.providerUsageCases.map((item) => ({
    ...item,
    parsedUsage: parsedProviderUsageFromRawPayload(item)
  }))
  const supported = parsedCases
    .filter((item) => item.parsedUsage.cacheHitTokens !== undefined && item.parsedUsage.cacheMissTokens !== undefined)
  const totalCacheHitTokens = supported.reduce((sum, item) => sum + (item.parsedUsage.cacheHitTokens ?? 0), 0)
  const totalCacheMissTokens = supported.reduce((sum, item) => sum + (item.parsedUsage.cacheMissTokens ?? 0), 0)
  return {
    rawPayloadParsedCaseIds: parsedCases.map((item) => item.id),
    rawTelemetrySupportedCaseIds: supported.map((item) => item.id),
    rawMatchesExpectedUsageCaseIds: providerCache.providerUsageCases
      .filter((item) => parsedUsageMatchesExpected(item))
      .map((item) => item.id),
    telemetrySupportedCaseIds: supported.map((item) => item.id),
    unsupportedUnknownCaseIds: parsedCases
      .filter((item) => item.parsedUsage.cacheHitRate === null)
      .map((item) => item.id),
    deepseekCaseIds: supported
      .filter((item) => item.baseUrl.includes('deepseek'))
      .map((item) => item.id),
    openaiCacheCaseIds: supported
      .filter((item) => item.endpointFormat === 'responses')
      .map((item) => item.id),
    anthropicCacheCaseIds: supported
      .filter((item) => item.endpointFormat === 'messages')
      .map((item) => item.id),
    totalCacheHitTokens,
    totalCacheMissTokens,
    aggregateCacheHitRate: totalCacheHitTokens / (totalCacheHitTokens + totalCacheMissTokens),
    unsupportedProvidersCountedAsMisses: false
  }
}

function providerUsageAccountingCases(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return providerCache.providerUsageCases.map((item) => ({
    id: item.id,
    ...(item.endpointFormat === undefined ? {} : { endpointFormat: item.endpointFormat }),
    baseUrl: item.baseUrl,
    responseBody: item.responseBody,
    expectedUsage: item.expectedUsage
  }))
}

type ProviderUsageCase = ReturnType<typeof ProviderCacheContract.parse>['providerUsageCases'][number]

function providerUsageCaseById(
  providerCache: ReturnType<typeof ProviderCacheContract.parse>,
  id: string
): ProviderUsageCase {
  const found = providerCache.providerUsageCases.find((item) => item.id === id)
  if (!found) throw new Error(`Missing provider usage case ${id}`)
  return found
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function numberValue(record: Record<string, unknown>, field: string): number {
  const value = record[field]
  return typeof value === 'number' ? value : 0
}

function usageBody(testCase: ProviderUsageCase): Record<string, unknown> {
  return recordValue(testCase.responseBody.usage)
}

function providerUsageParserReplay(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const telemetrySupported = providerCache.providerUsageCases
    .filter((item) => item.expectedUsage.cacheHitTokens !== undefined && item.expectedUsage.cacheMissTokens !== undefined)
  const unsupported = providerUsageCaseById(providerCache, 'unsupported-openai-compatible')
  const deepseekNative = providerUsageCaseById(providerCache, 'deepseek-native-cache-precedence')
  const deepseekUsage = usageBody(deepseekNative)
  const deepseekPromptDetails = recordValue(deepseekUsage.prompt_tokens_details)
  const responses = providerUsageCaseById(providerCache, 'openai-responses-cached-tokens')
  const responsesUsage = usageBody(responses)
  const responsesInputDetails = recordValue(responsesUsage.input_tokens_details)
  const responsesOutputDetails = recordValue(responsesUsage.output_tokens_details)
  const anthropic = providerUsageCaseById(providerCache, 'anthropic-cache-fields')
  const anthropicUsage = usageBody(anthropic)

  return {
    caseCount: providerCache.providerUsageCases.length,
    telemetrySupportedCaseIds: telemetrySupported.map((item) => item.id),
    unsupportedAbsentFields: {
      caseId: unsupported.id,
      absentFields: unsupported.expectedUsage.absent ?? [],
      cacheHitRateKnown: unsupported.expectedUsage.cacheHitRate !== null
    },
    deepseekNativePrecedence: {
      caseId: deepseekNative.id,
      promptCacheHitTokens: numberValue(deepseekUsage, 'prompt_cache_hit_tokens'),
      promptCacheMissTokens: numberValue(deepseekUsage, 'prompt_cache_miss_tokens'),
      promptTokensDetailsCachedTokens: numberValue(deepseekPromptDetails, 'cached_tokens'),
      expectedCacheHitTokens: deepseekNative.expectedUsage.cacheHitTokens,
      expectedCacheMissTokens: deepseekNative.expectedUsage.cacheMissTokens,
      nativeCacheFieldsWin:
        deepseekNative.expectedUsage.cacheHitTokens === numberValue(deepseekUsage, 'prompt_cache_hit_tokens') &&
        deepseekNative.expectedUsage.cacheHitTokens !== numberValue(deepseekPromptDetails, 'cached_tokens'),
      cacheHitRate: deepseekNative.expectedUsage.cacheHitRate
    },
    openaiResponsesCachedTokens: {
      caseId: responses.id,
      inputTokens: numberValue(responsesUsage, 'input_tokens'),
      cachedTokens: numberValue(responsesInputDetails, 'cached_tokens'),
      expectedCacheHitTokens: responses.expectedUsage.cacheHitTokens,
      expectedCacheMissTokens: responses.expectedUsage.cacheMissTokens,
      reasoningTokens: numberValue(responsesOutputDetails, 'reasoning_tokens'),
      cachedTokensUsed: responses.expectedUsage.cacheHitTokens === numberValue(responsesInputDetails, 'cached_tokens')
    },
    anthropicCacheFields: {
      caseId: anthropic.id,
      inputTokens: numberValue(anthropicUsage, 'input_tokens'),
      cacheReadInputTokens: numberValue(anthropicUsage, 'cache_read_input_tokens'),
      cacheCreationInputTokens: numberValue(anthropicUsage, 'cache_creation_input_tokens'),
      expectedPromptTokens: anthropic.expectedUsage.promptTokens,
      expectedCacheHitTokens: anthropic.expectedUsage.cacheHitTokens,
      expectedCacheMissTokens: anthropic.expectedUsage.cacheMissTokens,
      cacheFieldsIncludedInPrompt:
        anthropic.expectedUsage.promptTokens ===
        numberValue(anthropicUsage, 'input_tokens') +
        numberValue(anthropicUsage, 'cache_read_input_tokens') +
        numberValue(anthropicUsage, 'cache_creation_input_tokens')
    }
  }
}

function providerReleaseGuard(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const guard = evaluateOfflineCacheCurveGuard(providerCache.releaseGuard)
  return {
    ...guard,
    maxLowTailCases: providerCache.releaseGuard.maxLowTailCases,
    thresholdPercent: providerCache.releaseGuard.thresholdPercent,
    tailWindow: providerCache.releaseGuard.tailWindow
  }
}

function requestShapeEndpointFormats(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return Array.from(new Set(providerCache.requestShapeCases.map((item) => item.endpointFormat)))
}

type ProviderCacheCoverageCase =
  | ReturnType<typeof ProviderCacheContract.parse>['providerUsageCases'][number]
  | ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]

function providerFamilyForCoverageCase(testCase: ProviderCacheCoverageCase): string {
  const endpointFormat = testCase.endpointFormat ?? 'chat_completions'
  if (endpointFormat === 'custom_endpoint') return 'custom-full-endpoint'
  if (testCase.baseUrl.includes('deepseek')) return 'deepseek'
  if (endpointFormat === 'responses') return 'openai-responses'
  if (endpointFormat === 'messages') return 'anthropic-messages'
  return 'openai-compatible'
}

function uniqueInOrder(values: string[]): string[] {
  return Array.from(new Set(values))
}

function countRequiredBodyField(
  cases: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'],
  field: string
): number {
  return cases.filter((item) => item.requiredBodyFields.includes(field)).length
}

function countForbiddenBodyField(
  cases: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'],
  field: string
): number {
  return cases.filter((item) => item.forbiddenBodyFields.includes(field)).length
}

function appendProviderEndpointPath(baseUrl: string, versionedPath: string): string {
  const trimmed = baseUrl.replace(/\/+$/, '')
  if (trimmed.endsWith('/v1') && versionedPath.startsWith('/v1/')) {
    return `${trimmed}${versionedPath.slice('/v1'.length)}`
  }
  return `${trimmed}${versionedPath}`
}

function derivedProviderRequestUrl(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string {
  if (testCase.endpointFormat === 'custom_endpoint') return testCase.baseUrl
  if (testCase.endpointFormat === 'responses') {
    return appendProviderEndpointPath(testCase.baseUrl, '/v1/responses')
  }
  if (testCase.endpointFormat === 'messages') {
    return appendProviderEndpointPath(testCase.baseUrl, '/v1/messages')
  }
  return appendProviderEndpointPath(testCase.baseUrl, '/v1/chat/completions')
}

function derivedProviderToolShape(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string {
  if (testCase.endpointFormat === 'responses' || testCase.baseUrl.endsWith('/responses')) {
    return 'responses-function'
  }
  if (testCase.endpointFormat === 'messages' || testCase.baseUrl.endsWith('/messages')) {
    return 'anthropic-input-schema'
  }
  return 'openai-function'
}

function derivedProviderRequiredHeaders(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  if (derivedProviderToolShape(testCase) === 'anthropic-input-schema') {
    return ['Authorization', 'x-api-key', 'anthropic-version']
  }
  return ['Authorization']
}

function derivedProviderForbiddenHeaders(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  if (derivedProviderToolShape(testCase) === 'anthropic-input-schema') return []
  return ['x-api-key', 'anthropic-version']
}

function derivedProviderRequiredBodyFields(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  const toolShape = derivedProviderToolShape(testCase)
  if (toolShape === 'responses-function') {
    return ['model', 'stream', 'input', 'tools', 'max_output_tokens']
  }
  if (toolShape === 'anthropic-input-schema') {
    return ['model', 'stream', 'system', 'messages', 'tools', 'max_tokens']
  }
  const fields = ['model', 'stream', 'messages', 'tools']
  if (testCase.baseUrl.includes('deepseek')) fields.push('thinking')
  fields.push('reasoning_effort')
  return fields
}

function derivedProviderForbiddenBodyFields(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  const toolShape = derivedProviderToolShape(testCase)
  if (toolShape === 'responses-function') return ['messages', 'system', 'thinking']
  if (toolShape === 'anthropic-input-schema') return ['input', 'max_output_tokens', 'thinking']
  const fields = ['input', 'system', 'max_output_tokens']
  if (!testCase.baseUrl.includes('deepseek')) fields.push('thinking')
  return fields
}

function sameStringArray(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((item, index) => item === right[index])
}

function providerRequestShapeSummary(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const cases = providerCache.requestShapeCases
  return {
    caseCount: cases.length,
    exactUrlCount: cases.filter((item) => item.expectedUrl.length > 0).length,
    derivedUrlMatchCaseIds: cases
      .filter((item) => derivedProviderRequestUrl(item) === item.expectedUrl)
      .map((item) => item.id),
    headerShapeMatchCaseIds: cases
      .filter((item) =>
        sameStringArray(derivedProviderRequiredHeaders(item), item.requiredHeaders) &&
        sameStringArray(derivedProviderForbiddenHeaders(item), item.forbiddenHeaders)
      )
      .map((item) => item.id),
    bodyShapeMatchCaseIds: cases
      .filter((item) =>
        sameStringArray(derivedProviderRequiredBodyFields(item), item.requiredBodyFields) &&
        sameStringArray(derivedProviderForbiddenBodyFields(item), item.forbiddenBodyFields)
      )
      .map((item) => item.id),
    toolShapeMatchCaseIds: cases
      .filter((item) => derivedProviderToolShape(item) === item.expectedToolShape)
      .map((item) => item.id),
    matrix: cases,
    endpointFormats: uniqueInOrder(cases.map((item) => item.endpointFormat)),
    fullEndpointCaseIds: cases
      .filter((item) => item.endpointFormat === 'custom_endpoint')
      .map((item) => item.id),
    toolShapes: uniqueInOrder(cases.map((item) => item.expectedToolShape)),
    customFullEndpointExactUrlCaseIds: cases
      .filter((item) =>
        item.endpointFormat === 'custom_endpoint' &&
        derivedProviderRequestUrl(item) === item.baseUrl &&
        item.expectedUrl === item.baseUrl
      )
      .map((item) => item.id),
    customFullEndpointAppendedPathCount: cases
      .filter((item) => item.endpointFormat === 'custom_endpoint' && item.expectedUrl !== item.baseUrl)
      .length,
    requiredBodyFieldFamilies: {
      messagesFieldCaseCount: countRequiredBodyField(cases, 'messages'),
      inputFieldCaseCount: countRequiredBodyField(cases, 'input'),
      systemFieldCaseCount: countRequiredBodyField(cases, 'system'),
      thinkingFieldCaseCount: countRequiredBodyField(cases, 'thinking'),
      maxOutputTokensCaseCount: countRequiredBodyField(cases, 'max_output_tokens'),
      maxTokensCaseCount: countRequiredBodyField(cases, 'max_tokens')
    },
    forbiddenBodyFieldFamilies: {
      thinkingForbiddenCaseCount: countForbiddenBodyField(cases, 'thinking'),
      systemForbiddenCaseCount: countForbiddenBodyField(cases, 'system'),
      inputForbiddenCaseCount: countForbiddenBodyField(cases, 'input'),
      maxOutputTokensForbiddenCaseCount: countForbiddenBodyField(cases, 'max_output_tokens')
    }
  }
}

function providerLiveLocalHttpContractSummary(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const contract = providerCache.liveLocalHttpContract
  return {
    fixtureOnly: contract.fixtureOnly,
    transport: contract.transport,
    usesLiveCredentials: contract.usesLiveCredentials,
    preservesOriginalProviderBaseUrl: contract.preservesOriginalProviderBaseUrl,
    usageCaseCount: providerCache.providerUsageCases.length,
    requestShapeCaseCount: providerCache.requestShapeCases.length,
    expectedPostCount: providerCache.providerUsageCases.length + providerCache.requestShapeCases.length,
    coveredEndpointFormats: requestShapeEndpointFormats(providerCache),
    coveredProviderFamilies: contract.coveredProviderFamilies,
    mayClaimLiveSuperiority: contract.mayClaimLiveSuperiority
  }
}

function providerLiveLocalHttpContractControlCase(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    sourceContractId: providerCache.id,
    contract: providerCache.liveLocalHttpContract,
    providerUsageCaseIds: providerCache.providerUsageCases.map((item) => item.id),
    requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
    requestShapeEndpointFormats: requestShapeEndpointFormats(providerCache),
    expected: providerLiveLocalHttpContractSummary(providerCache)
  }
}

function providerCacheCoverageFloor(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const requiredProviderFamilies = providerCache.liveLocalHttpContract.coveredProviderFamilies
  const coveredProviderFamilies = uniqueInOrder([
    ...providerCache.requestShapeCases,
    ...providerCache.providerUsageCases
  ].map(providerFamilyForCoverageCase))
  const telemetrySupportedCaseIds = providerCache.providerUsageCases
    .filter((item) => (
      item.expectedUsage.cacheHitTokens !== undefined &&
      item.expectedUsage.cacheMissTokens !== undefined
    ))
    .map((item) => item.id)
  const customFullEndpointCaseIds = providerCache.requestShapeCases
    .filter((item) => item.endpointFormat === 'custom_endpoint')
    .map((item) => item.id)
  const customFullEndpointTelemetryCaseIds = telemetrySupportedCaseIds
    .filter((id) => customFullEndpointCaseIds.includes(id))
  return {
    requiredProviderFamilies,
    coveredProviderFamilies,
    providerFamilyCoverageComplete:
      JSON.stringify(requiredProviderFamilies) === JSON.stringify(coveredProviderFamilies),
    providerUsageCaseIds: providerCache.providerUsageCases.map((item) => item.id),
    requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
    requestShapeEndpointFormats: requestShapeEndpointFormats(providerCache),
    telemetrySupportedCaseIds,
    unsupportedUnknownCaseIds: providerCache.providerUsageCases
      .filter((item) => item.expectedUsage.cacheHitRate === null)
      .map((item) => item.id),
    customFullEndpointCaseIds,
    customFullEndpointExactUrlCaseIds: providerCache.requestShapeCases
      .filter((item) => item.endpointFormat === 'custom_endpoint' && item.expectedUrl === item.baseUrl)
      .map((item) => item.id),
    customFullEndpointToolShapes: providerCache.requestShapeCases
      .filter((item) => item.endpointFormat === 'custom_endpoint')
      .map((item) => item.expectedToolShape),
    customFullEndpointTelemetryCaseIds,
    customFullEndpointRequestShapeOnly:
      customFullEndpointCaseIds.length > 0 && customFullEndpointTelemetryCaseIds.length === 0,
    telemetrySupportedExcludesCustomFullEndpoints: customFullEndpointTelemetryCaseIds.length === 0,
    customProviderCacheTelemetryClaimAllowed: false,
    deepseekUsageCaseIds: providerCache.providerUsageCases
      .filter((item) => item.baseUrl.includes('deepseek'))
      .map((item) => item.id),
    deepseekRequestShapeCaseIds: providerCache.requestShapeCases
      .filter((item) => item.baseUrl.includes('deepseek'))
      .map((item) => item.id),
    openaiCompatibleChatRequestShapeCaseIds: providerCache.requestShapeCases
      .filter((item) => (
        item.endpointFormat === 'chat_completions' &&
        !item.baseUrl.includes('deepseek')
      ))
      .map((item) => item.id),
    openaiResponsesUsageCaseIds: providerCache.providerUsageCases
      .filter((item) => item.endpointFormat === 'responses')
      .map((item) => item.id),
    openaiResponsesRequestShapeCaseIds: providerCache.requestShapeCases
      .filter((item) => item.endpointFormat === 'responses')
      .map((item) => item.id),
    anthropicMessagesUsageCaseIds: providerCache.providerUsageCases
      .filter((item) => item.endpointFormat === 'messages')
      .map((item) => item.id),
    anthropicMessagesRequestShapeCaseIds: providerCache.requestShapeCases
      .filter((item) => item.endpointFormat === 'messages')
      .map((item) => item.id),
    liveCredentialsUsed: providerCache.liveLocalHttpContract.usesLiveCredentials,
    mayClaimLiveSuperiority:
      providerCache.liveCredentialPolicy.mayClaimSuperiority ||
      providerCache.liveLocalHttpContract.mayClaimLiveSuperiority,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function providerCacheCoverageFloorControlCase(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    sourceContractId: providerCache.id,
    providerUsageCases: providerUsageAccountingCases(providerCache),
    requestShapeCases: providerCache.requestShapeCases.map((item) => ({
      id: item.id,
      endpointFormat: item.endpointFormat,
      baseUrl: item.baseUrl,
      expectedUrl: item.expectedUrl,
      expectedToolShape: item.expectedToolShape
    })),
    liveLocalHttpContract: providerCache.liveLocalHttpContract,
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: providerCacheCoverageFloor(providerCache)
  }
}

function providerOfflineParitySeal(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const releaseGuard = providerReleaseGuard(providerCache)
  return {
    fixtureOnly: providerCache.liveCredentialPolicy.fixtureOnly,
    mayClaimLiveSuperiority: providerCache.liveCredentialPolicy.mayClaimSuperiority,
    stablePrefixEquivalent:
      providerCache.stablePrefix.firstShape.prefixHash === providerCache.stablePrefix.equivalentShape.prefixHash,
    prefixItemsHashStable:
      providerCache.stablePrefix.firstShape.prefixItemsHash === providerCache.stablePrefix.equivalentShape.prefixItemsHash,
    toolsHashStable:
      providerCache.stablePrefix.firstShape.toolsHash === providerCache.stablePrefix.equivalentShape.toolsHash,
    stablePrefixHash: providerCache.stablePrefix.firstShape.prefixHash,
    equivalentPrefixHash: providerCache.stablePrefix.equivalentShape.prefixHash,
    deepseekProviderId: providerCache.stablePrefix.firstShape.providerId,
    deepseekEndpointFormat: providerCache.stablePrefix.firstShape.endpointFormat,
    deepseekModel: providerCache.stablePrefix.firstShape.model,
    deepseekStableCacheHitTokens: providerCache.stablePrefix.usage.cacheHitTokens,
    deepseekStableCacheMissTokens: providerCache.stablePrefix.usage.cacheMissTokens,
    deepseekStableCacheHitRate: providerCache.stablePrefix.usage.cacheHitRate,
    diagnosticsPrefixChanged: providerCache.stablePrefix.expectedDiagnostics.prefixChanged,
    diagnosticsTelemetrySupported: providerCache.stablePrefix.expectedDiagnostics.cacheTelemetrySupported,
    providerUsageCaseCount: providerCache.providerUsageCases.length,
    requestShapeCaseCount: providerCache.requestShapeCases.length,
    requestShapeEndpointFormats: requestShapeEndpointFormats(providerCache),
    releaseGuardStatus: releaseGuard.status,
    releaseGuardThresholdPercent: providerCache.releaseGuard.thresholdPercent,
    releaseGuardTailWindow: providerCache.releaseGuard.tailWindow
  }
}

function providerOfflineParitySealControlCase(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    sourceContractId: providerCache.id,
    stablePrefix: providerCache.stablePrefix,
    providerUsageCaseIds: providerCache.providerUsageCases.map((item) => item.id),
    requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
    requestShapeEndpointFormats: requestShapeEndpointFormats(providerCache),
    releaseGuard: {
      status: providerReleaseGuard(providerCache).status,
      thresholdPercent: providerCache.releaseGuard.thresholdPercent,
      tailWindow: providerCache.releaseGuard.tailWindow
    },
    liveCredentialPolicy: providerCache.liveCredentialPolicy,
    expected: providerOfflineParitySeal(providerCache)
  }
}

function providerCachePrivacy(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const diagnostics = JSON.stringify(providerCache.stablePrefix.expectedDiagnostics)
  return {
    fixtureOnly: providerCache.liveCredentialPolicy.fixtureOnly,
    diagnosticCheckedFieldCount: Object.keys(providerCache.stablePrefix.expectedDiagnostics).length,
    forbiddenDiagnosticsSubstringCount: providerCache.privacy.forbiddenDiagnosticsSubstrings.length,
    diagnosticsLeakForbiddenSubstrings: providerCache.privacy.forbiddenDiagnosticsSubstrings
      .some((item) => diagnostics.includes(item)),
    liveCredentialsUsed: providerCache.liveLocalHttpContract.usesLiveCredentials,
    mayClaimLiveSuperiority:
      providerCache.liveCredentialPolicy.mayClaimSuperiority ||
      providerCache.liveLocalHttpContract.mayClaimLiveSuperiority,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function providerCachePrivacyControlCase(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    sourceContractId: providerCache.id,
    privacy: providerCache.privacy,
    diagnostics: providerCache.stablePrefix.expectedDiagnostics,
    liveCredentialPolicy: providerCache.liveCredentialPolicy,
    liveLocalHttpContract: providerCache.liveLocalHttpContract,
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: providerCachePrivacy(providerCache)
  }
}

function providerCacheInventory(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    stablePrefixHash: providerCache.stablePrefix.firstShape.prefixHash,
    toolsHash: providerCache.stablePrefix.firstShape.toolsHash,
    providerUsageCaseIds: providerCache.providerUsageCases.map((item) => item.id),
    requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
    providerUsageCaseCount: providerCache.providerUsageCases.length,
    requestShapeCaseCount: providerCache.requestShapeCases.length,
    stablePrefixEquivalent:
      providerCache.stablePrefix.firstShape.prefixHash === providerCache.stablePrefix.equivalentShape.prefixHash,
    toolsHashStable:
      providerCache.stablePrefix.firstShape.toolsHash === providerCache.stablePrefix.equivalentShape.toolsHash,
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function providerCacheInventoryControlCase(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    sourceContractId: providerCache.id,
    stablePrefix: providerCache.stablePrefix,
    providerUsageCaseIds: providerCache.providerUsageCases.map((item) => item.id),
    requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: providerCacheInventory(providerCache)
  }
}

function providerDriftAttribution(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const drift = providerCache.driftAttribution
  return {
    previousPrefixHash: drift.previousShape.prefixHash,
    currentPrefixHash: drift.currentShape.prefixHash,
    systemHashStable: drift.previousShape.systemHash === drift.currentShape.systemHash,
    prefixItemsHashStable: drift.previousShape.prefixItemsHash === drift.currentShape.prefixItemsHash,
    toolsHashChanged: drift.previousShape.toolsHash !== drift.currentShape.toolsHash,
    providerChanged: drift.previousShape.providerId !== drift.currentShape.providerId,
    modelChanged: drift.previousShape.model !== drift.currentShape.model,
    endpointFormatChanged: drift.previousShape.endpointFormat !== drift.currentShape.endpointFormat,
    expectedReasons: drift.expectedReasons,
    telemetrySupported: drift.expectedTelemetrySupported,
    cacheHitRateKnown: drift.usage.cacheHitRate !== null
  }
}

function providerDriftAttributionControlCase(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  return {
    sourceContractId: providerCache.id,
    driftAttribution: providerCache.driftAttribution,
    expected: providerDriftAttribution(providerCache)
  }
}

function routeById(routes: GoG2RouteReplayCase[], id: string): GoG2RouteReplayCase {
  const route = routes.find((item) => item.id === id)
  if (!route) {
    throw new Error(`Missing G2 route fixture ${id}`)
  }
  return route
}

function responseBody(route: GoG2RouteReplayCase): Record<string, unknown> {
  return route.response.body as Record<string, unknown>
}

function threadCount(route: GoG2RouteReplayCase): number {
  const threads = responseBody(route).threads
  return Array.isArray(threads) ? threads.length : 0
}

function firstThreadStatus(route: GoG2RouteReplayCase): string {
  const threads = responseBody(route).threads
  return Array.isArray(threads) ? String((threads[0] as Record<string, unknown>).status) : ''
}

function turnCount(route: GoG2RouteReplayCase): number {
  const turns = responseBody(route).turns
  return Array.isArray(turns) ? turns.length : 0
}

function sseEventNames(frames: string[]): string[] {
  return frames.map((frame) => {
    const match = /^event:\s*(.+)$/m.exec(frame)
    return match?.[1] ?? ''
  })
}

function sseDataPayload(frame: string): Record<string, unknown> {
  const match = /^data:\s*(.+)$/m.exec(frame)
  return match ? JSON.parse(match[1]) as Record<string, unknown> : {}
}

function usageFromSseFrames(frames: string[]): Record<string, unknown> {
  const usageFrame = frames.find((frame) => /^event:\s*usage$/m.test(frame))
  const payload = usageFrame ? sseDataPayload(usageFrame) : {}
  return (payload.usage ?? {}) as Record<string, unknown>
}

function usageNumber(usage: Record<string, unknown>, field: string): number {
  const value = usage[field]
  return typeof value === 'number' ? value : 0
}

function g3ProviderStreamingReplay(g3: ReturnType<typeof GoG3ProviderStreamingUsageCacheContract.parse>) {
  const expectedUsageCase = g3.providerUsageMatrix.find((item) => item.id === g3.streaming.expectedUsageCaseId)
  if (!expectedUsageCase) {
    throw new Error(`Missing G3 provider usage case ${g3.streaming.expectedUsageCaseId}`)
  }
  const usage = usageFromSseFrames(g3.streaming.sseFrames)
  const usageEventMatchesExpectedCase = [
    'promptTokens',
    'completionTokens',
    'reasoningTokens',
    'totalTokens',
    'cacheHitTokens',
    'cacheMissTokens',
    'cacheHitRate'
  ].every((field) => {
    const expected = expectedUsageCase.expectedUsage[field as keyof typeof expectedUsageCase.expectedUsage]
    return expected === undefined || usage[field] === expected
  })

  return {
    sourceContractId: g3.id,
    threadId: g3.streaming.threadId,
    sinceSeq: g3.streaming.sinceSeq,
    sseFrameCount: g3.streaming.sseFrames.length,
    streamingKinds: sseEventNames(g3.streaming.sseFrames),
    expectedKindsInOrder: g3.streaming.expectedKindsInOrder,
    expectedUsageCaseId: g3.streaming.expectedUsageCaseId,
    usageEventMatchesExpectedCase,
    usagePromptTokens: usageNumber(usage, 'promptTokens'),
    usageCompletionTokens: usageNumber(usage, 'completionTokens'),
    usageReasoningTokens: usageNumber(usage, 'reasoningTokens'),
    usageTotalTokens: usageNumber(usage, 'totalTokens'),
    usageCacheHitTokens: usageNumber(usage, 'cacheHitTokens'),
    usageCacheMissTokens: usageNumber(usage, 'cacheMissTokens'),
    usageCacheHitRate: usageNumber(usage, 'cacheHitRate'),
    cacheTelemetrySupported:
      expectedUsageCase.expectedUsage.cacheHitTokens !== undefined &&
      expectedUsageCase.expectedUsage.cacheMissTokens !== undefined,
    productBoundary: g3.productBoundary
  }
}

function g3ProviderStreamingControlCase(g3: ReturnType<typeof GoG3ProviderStreamingUsageCacheContract.parse>) {
  return {
    sourceContractId: g3.id,
    providerUsageMatrix: g3.providerUsageMatrix,
    streaming: g3.streaming,
    productBoundary: g3.productBoundary,
    expected: g3ProviderStreamingReplay(g3)
  }
}

function g2RouteStatusReplay(routes: GoG2RouteReplayCase[]) {
  const archive = routeById(routes, 'thread-archive-patch')
  const archivedOnly = routeById(routes, 'thread-list-archived-only')
  const searchArchived = routeById(routes, 'thread-search-archived')
  const read = routeById(routes, 'thread-read-detail')
  const update = routeById(routes, 'thread-update-title-workspace')
  const fork = routeById(routes, 'thread-fork-side')
  const resume = routeById(routes, 'session-resume-thread')
  const replay = routeById(routes, 'events-since-seq')
  const caughtUp = routeById(routes, 'events-caught-up-since-seq')
  const unauthorizedSse = routeById(routes, 'events-unauthorized-since-seq')
  return {
    routeCount: routes.length,
    jsonRouteCount: routes.filter((route) => route.responseKind === 'json').length,
    sseRouteCount: routes.filter((route) => route.responseKind === 'sse').length,
    statusCodes: {
      ok: routes.filter((route) => route.response.status === 200).length,
      created: routes.filter((route) => route.response.status === 201).length,
      unauthorized: routes.filter((route) => route.response.status === 401).length
    },
    auth: {
      protectedRouteId: unauthorizedSse.id,
      protectedPath: unauthorizedSse.path,
      protectedStatus: unauthorizedSse.response.status,
      protectedBodyCode: String(responseBody(unauthorizedSse).code),
      sseFrameCount: unauthorizedSse.sseFrames?.length ?? 0
    },
    archive: {
      patchStatus: archive.response.status,
      archivedStatus: String(responseBody(archive).status),
      archivedOnlyCount: threadCount(archivedOnly),
      searchArchivedCount: threadCount(searchArchived),
      searchFirstStatus: firstThreadStatus(searchArchived)
    },
    readUpdate: {
      readStatus: read.response.status,
      readLatestSeq: responseBody(read).latestSeq,
      readTurnCount: turnCount(read),
      updateStatus: update.response.status,
      updatedWorkspace: String(responseBody(update).workspace)
    },
    fork: {
      status: fork.response.status,
      relation: String(responseBody(fork).relation),
      parentThreadId: String(responseBody(fork).parentThreadId),
      forkedFromTurnCount: responseBody(fork).forkedFromTurnCount,
      forkedTurnCount: turnCount(fork)
    },
    resume: {
      status: resume.response.status,
      sessionId: String(responseBody(resume).session_id),
      messageCount: responseBody(resume).message_count,
      summary: String(responseBody(resume).summary)
    },
    sse: {
      replayStatus: replay.response.status,
      replayFrameCount: replay.sseFrames?.length ?? 0,
      caughtUpStatus: caughtUp.response.status,
      caughtUpFrameCount: caughtUp.sseFrames?.length ?? 0,
      replayEventNames: sseEventNames(replay.sseFrames ?? [])
    }
  }
}

const G2_NOW = '2026-06-21T00:10:00.000Z'
const G2_T0 = '2026-06-21T00:00:00.000Z'
const G2_T1 = '2026-06-21T00:01:00.000Z'
const G2_T2 = '2026-06-21T00:02:00.000Z'

type G2Harness = {
  router: ReturnType<typeof buildRouter>
  threadStore: InMemoryThreadStore
  sessionStore: InMemorySessionStore
}

function buildG2Harness(): G2Harness {
  const eventBus = new InMemoryEventBus()
  const threadStore = new InMemoryThreadStore()
  const sessionStore = new InMemorySessionStore()
  const ids = new SequentialIdGenerator()
  const nowIso = () => G2_NOW
  const allocateSeq = (threadId: string) => eventBus.allocateSeq(threadId)
  const events = new RuntimeEventRecorder({ eventBus, sessionStore, allocateSeq, nowIso })
  const threadService = new ThreadService({ threadStore, sessionStore, events, ids, nowIso })
  const runtime: ServerRuntime = {
    threadService,
    turnService: {} as ServerRuntime['turnService'],
    usageService: {} as ServerRuntime['usageService'],
    checkpointRewindService: {} as ServerRuntime['checkpointRewindService'],
    eventBus,
    sessionStore,
    events,
    approvalGate: {} as ServerRuntime['approvalGate'],
    userInputGate: {} as ServerRuntime['userInputGate'],
    workspaceInspector: {} as ServerRuntime['workspaceInspector'],
    remoteEntryControl: {} as ServerRuntime['remoteEntryControl'],
    runTurn: () => undefined,
    runtimeToken: 'tok-1',
    insecure: false,
    allocateSeq,
    nowIso,
    info: () => ({}) as ReturnType<ServerRuntime['info']>
  }
  return { router: buildRouter(runtime), threadStore, sessionStore }
}

async function seedG2Setup(h: G2Harness, setup: G2Setup): Promise<void> {
  switch (setup) {
    case 'thread-list':
      await seedThreadList(h, false)
      return
    case 'thread-archive':
      await h.threadStore.upsert(baseThread({
        id: 'thr_g2_beta',
        title: 'Beta Archive',
        workspace: '/tmp/beta',
        updatedAt: G2_T2
      }))
      return
    case 'thread-search-archive':
      await seedThreadList(h, true)
      return
    case 'thread-read':
      await seedReadThread(h)
      return
    case 'thread-fork':
      await h.threadStore.upsert(threadWithUserTurn({
        id: 'thr_g2_parent',
        title: 'Parent Thread',
        workspace: '/tmp/parent',
        turnId: 'turn_g2_parent',
        itemId: 'item_g2_parent_user',
        prompt: 'Fork me'
      }))
      return
    case 'session-resume':
      await h.threadStore.upsert(threadWithUserTurn({
        id: 'thr_g2_source',
        title: 'Source Thread',
        workspace: '/tmp/source',
        turnId: 'turn_g2_source',
        itemId: 'item_g2_source_user',
        prompt: 'Resume me'
      }))
      return
    case 'events':
      await seedEvents(h)
      return
  }
}

async function seedThreadList(h: G2Harness, archivedBeta: boolean): Promise<void> {
  await h.threadStore.upsert(baseThread({
    id: 'thr_g2_alpha',
    title: 'Alpha Project',
    workspace: '/tmp/alpha',
    updatedAt: G2_T1
  }))
  await h.threadStore.upsert(baseThread({
    id: 'thr_g2_beta',
    title: 'Beta Archive',
    workspace: '/tmp/beta',
    status: archivedBeta ? 'archived' : 'idle',
    updatedAt: archivedBeta ? G2_NOW : G2_T2
  }))
}

async function seedReadThread(h: G2Harness): Promise<void> {
  const thread = threadWithReadTurn()
  await h.threadStore.upsert(thread)
  for (const item of thread.turns[0]?.items ?? []) {
    await h.sessionStore.appendItem(thread.id, item)
  }
  await h.sessionStore.appendEvent(thread.id, {
    kind: 'thread_created',
    seq: 1,
    timestamp: G2_T0,
    threadId: thread.id,
    title: thread.title
  })
  await h.sessionStore.appendEvent(thread.id, {
    kind: 'turn_completed',
    seq: 2,
    timestamp: G2_T2,
    threadId: thread.id,
    turnId: 'turn_g2_read',
    status: 'completed'
  })
}

async function seedEvents(h: G2Harness): Promise<void> {
  const item = userItem({
    id: 'item_g2_events_user',
    threadId: 'thr_g2_events',
    turnId: 'turn_g2_events',
    text: 'Replay this',
    createdAt: '2026-06-21T00:04:00.000Z'
  })
  const events: RuntimeEvent[] = [
    {
      kind: 'thread_created',
      seq: 1,
      timestamp: '2026-06-21T00:04:00.000Z',
      threadId: 'thr_g2_events',
      title: 'Replay Thread'
    },
    {
      kind: 'turn_started',
      seq: 2,
      timestamp: '2026-06-21T00:04:01.000Z',
      threadId: 'thr_g2_events',
      turnId: 'turn_g2_events'
    },
    {
      kind: 'item_created',
      seq: 3,
      timestamp: '2026-06-21T00:04:02.000Z',
      threadId: 'thr_g2_events',
      turnId: 'turn_g2_events',
      itemId: item.id,
      item
    }
  ]
  for (const event of events) {
    await h.sessionStore.appendEvent(event.threadId, event)
  }
}

function baseThread(input: {
  id: string
  title: string
  workspace: string
  status?: ThreadRecord['status']
  updatedAt?: string
  turns?: Turn[]
}): ThreadRecord {
  return {
    id: input.id,
    title: input.title,
    workspace: input.workspace,
    model: 'deepseek-chat',
    mode: 'agent',
    status: input.status ?? 'idle',
    executionPolicyVersion: 2,
    approvalPolicy: 'on-request',
    sandboxMode: 'workspace-write',
    relation: 'primary',
    createdAt: G2_T0,
    updatedAt: input.updatedAt ?? G2_T0,
    turns: input.turns ?? []
  }
}

function threadWithReadTurn(): ThreadRecord {
  const user = userItem({
    id: 'item_g2_read_user',
    threadId: 'thr_g2_read',
    turnId: 'turn_g2_read',
    text: 'Read me',
    createdAt: G2_T1
  })
  const assistant: TurnItem = {
    id: 'item_g2_read_assistant',
    turnId: 'turn_g2_read',
    threadId: 'thr_g2_read',
    role: 'assistant',
    status: 'completed',
    createdAt: G2_T2,
    finishedAt: G2_T2,
    kind: 'assistant_text',
    text: 'Read response'
  }
  return baseThread({
    id: 'thr_g2_read',
    title: 'Read Thread',
    workspace: '/tmp/read',
    updatedAt: G2_T2,
    turns: [turn({
      id: 'turn_g2_read',
      threadId: 'thr_g2_read',
      prompt: 'Read me',
      items: [user, assistant]
    })]
  })
}

function threadWithUserTurn(input: {
  id: string
  title: string
  workspace: string
  turnId: string
  itemId: string
  prompt: string
}): ThreadRecord {
  const item = userItem({
    id: input.itemId,
    threadId: input.id,
    turnId: input.turnId,
    text: input.prompt,
    createdAt: G2_T1
  })
  return baseThread({
    id: input.id,
    title: input.title,
    workspace: input.workspace,
    updatedAt: G2_T2,
    turns: [turn({
      id: input.turnId,
      threadId: input.id,
      prompt: input.prompt,
      items: [item]
    })]
  })
}

function turn(input: {
  id: string
  threadId: string
  prompt: string
  items: TurnItem[]
}): Turn {
  return {
    id: input.id,
    threadId: input.threadId,
    status: 'completed',
    prompt: input.prompt,
    steering: [],
    createdAt: G2_T1,
    startedAt: G2_T1,
    finishedAt: G2_T2,
    items: input.items,
    attachmentIds: [],
    activeSkillIds: [],
    injectedMemoryIds: []
  }
}

function userItem(input: {
  id: string
  threadId: string
  turnId: string
  text: string
  createdAt: string
}): TurnItem {
  return {
    id: input.id,
    turnId: input.turnId,
    threadId: input.threadId,
    role: 'user',
    status: 'completed',
    createdAt: input.createdAt,
    finishedAt: input.createdAt,
    kind: 'user_message',
    text: input.text
  }
}

function g2RouteStatusControlCase(g2: GoG2RouteReplayContract) {
  return {
    sourceContractId: 'go-g2-route-replay-contract-v1',
    routes: g2.routes,
    expected: g2RouteStatusReplay(g2.routes)
  }
}

function g2RouteInventory(routes: GoG2RouteReplayCase[]) {
  return {
    routeIds: routes.map((route) => route.id),
    jsonRouteIds: routes.filter((route) => route.responseKind === 'json').map((route) => route.id),
    sseRouteIds: routes.filter((route) => route.responseKind === 'sse').map((route) => route.id),
    eventRouteIds: routes.filter((route) => route.setup === 'events').map((route) => route.id),
    resumeRouteIds: routes.filter((route) => route.setup === 'session-resume').map((route) => route.id),
    forkRouteIds: routes.filter((route) => route.setup === 'thread-fork').map((route) => route.id),
    archiveRouteIds: routes
      .filter((route) => route.setup === 'thread-archive' || route.setup === 'thread-search-archive')
      .map((route) => route.id),
    searchRouteIds: routes.filter((route) => route.setup === 'thread-search-archive').map((route) => route.id),
    readUpdateRouteIds: routes.filter((route) => route.setup === 'thread-read').map((route) => route.id),
    runtimeTokenRouteCount: routes.filter((route) => route.auth === 'runtime-token').length,
    unauthorizedRouteIds: routes.filter((route) => route.auth === 'none').map((route) => route.id),
    usesReasonixProtocol: false,
    topLevelRouteExposed: false
  }
}

function g2RouteInventoryControlCase(g2: GoG2RouteReplayContract) {
  return {
    sourceContractId: 'go-g2-route-replay-contract-v1',
    routes: g2.routes.map((route) => ({
      id: route.id,
      setup: route.setup,
      path: route.path,
      auth: route.auth,
      responseKind: route.responseKind
    })),
    productBoundary: {
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    },
    expected: g2RouteInventory(g2.routes)
  }
}

function stableJson(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map((item) => stableJson(item)).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record).sort().map((key) =>
    `${JSON.stringify(key)}:${stableJson(record[key])}`
  ).join(',')}}`
}

function shortHash(value: string): string {
  return createHash('sha256').update(value).digest('hex').slice(0, 16)
}

function bodyShape(route: GoG2RouteReplayCase): 'none' | 'object' | 'array' {
  const body = route.response.body
  if (body === undefined) return 'none'
  if (Array.isArray(body)) return 'array'
  return typeof body === 'object' && body !== null ? 'object' : 'none'
}

function requestBodyHash(route: GoG2RouteReplayCase): string {
  return route.body === undefined ? '' : shortHash(stableJson(route.body))
}

function responseBodyHash(route: GoG2RouteReplayCase): string {
  return route.response.body === undefined ? '' : shortHash(stableJson(route.response.body))
}

function sseFramesHash(route: GoG2RouteReplayCase): string {
  const frames = route.sseFrames ?? []
  return frames.length === 0 ? '' : shortHash(frames.join('\n\n'))
}

function g2RouteReplayMatrix(routes: GoG2RouteReplayCase[]) {
  return routes.map((route) => ({
    id: route.id,
    method: route.method,
    path: route.path,
    setup: route.setup,
    auth: route.auth,
    responseKind: route.responseKind,
    status: route.response.status,
    requestBodyHash: requestBodyHash(route),
    responseBodyShape: bodyShape(route),
    responseBodyHash: responseBodyHash(route),
    sseFrameCount: route.sseFrames?.length ?? 0,
    sseEventNames: sseEventNames(route.sseFrames ?? []),
    sseFramesHash: sseFramesHash(route)
  }))
}

function routeMatrixById(
  matrix: ReturnType<typeof g2RouteReplayMatrix>,
  id: string
) {
  const route = matrix.find((item) => item.id === id)
  if (!route) {
    throw new Error(`Missing G2 route matrix row ${id}`)
  }
  return route
}

function g2SessionRouteReplay(
  matrix: ReturnType<typeof g2RouteReplayMatrix>,
  productBoundary = {
    usesReasonixProtocol: false,
    topLevelRouteExposed: false,
    rendererVisibleGoRoute: false,
    defaultGoBackend: false
  }
) {
  const replay = routeMatrixById(matrix, 'events-since-seq')
  const caughtUp = routeMatrixById(matrix, 'events-caught-up-since-seq')
  const unauthorized = routeMatrixById(matrix, 'events-unauthorized-since-seq')
  const archive = routeMatrixById(matrix, 'thread-archive-patch')
  const search = routeMatrixById(matrix, 'thread-search-archived')
  const fork = routeMatrixById(matrix, 'thread-fork-side')
  const resume = routeMatrixById(matrix, 'session-resume-thread')
  return {
    routeCount: matrix.length,
    matrix,
    exactJsonBodyRouteCount: matrix.filter((route) => route.responseKind === 'json' && route.responseBodyHash).length,
    exactSseRouteCount: matrix.filter((route) => route.responseKind === 'sse').length,
    runtimeTokenRouteCount: matrix.filter((route) => route.auth === 'runtime-token').length,
    unauthorizedRouteIds: matrix.filter((route) => route.auth === 'none').map((route) => route.id),
    archiveResponseHash: archive.responseBodyHash,
    searchResponseHash: search.responseBodyHash,
    forkResponseHash: fork.responseBodyHash,
    resumeResponseHash: resume.responseBodyHash,
    replaySseHash: replay.sseFramesHash,
    caughtUpSseHash: caughtUp.sseFramesHash,
    unauthorizedBodyHash: unauthorized.responseBodyHash,
    everyRouteHasMethodPathStatus: matrix.every((route) => route.method && route.path && route.status > 0),
    jsonRoutesHaveBodyHash: matrix
      .filter((route) => route.responseKind === 'json')
      .every((route) => route.responseBodyHash.length === 16),
    sseRoutesHaveExactFrames: matrix
      .filter((route) => route.responseKind === 'sse')
      .every((route) => route.sseFrameCount === 0 || route.sseFramesHash.length === 16),
    usesReasonixProtocol: productBoundary.usesReasonixProtocol,
    topLevelRouteExposed: productBoundary.topLevelRouteExposed,
    rendererVisibleGoRoute: productBoundary.rendererVisibleGoRoute,
    defaultGoBackend: productBoundary.defaultGoBackend
  }
}

function g2SessionRouteReplayControlCase(g2: GoG2RouteReplayContract) {
  const routes = g2RouteReplayMatrix(g2.routes)
  const productBoundary = {
    usesReasonixProtocol: false,
    topLevelRouteExposed: false,
    rendererVisibleGoRoute: false,
    defaultGoBackend: false
  }
  return {
    sourceContractId: 'go-g2-route-replay-contract-v1',
    routes,
    productBoundary,
    expected: g2SessionRouteReplay(routes, productBoundary)
  }
}

function requestFromRoute(route: GoG2RouteReplayCase, token: string): Request {
  const headers = new Headers()
  if (route.auth === 'runtime-token') {
    headers.set('authorization', `Bearer ${token}`)
  }
  const init: RequestInit = { method: route.method, headers }
  if (route.body !== undefined) {
    headers.set('content-type', 'application/json')
    init.body = JSON.stringify(route.body)
  }
  return new Request(`http://localhost${route.path}`, init)
}

describe('owned Go runtime server lifecycle helper', () => {
  it('confirms a ready process and its detached group have exited normally before stop returns', async () => {
    const fixture = spawnOwnedProcessFixture(`
process.stdout.write(${JSON.stringify(OWNED_PROCESS_FIXTURE_READY_LINE)})
setTimeout(() => process.exit(0), 50)
`)

    await waitForOwnedProcessFixtureReady(fixture, 2_000)
    await fixture.ownedProcess.exit
    const stopped = await stopOwnedProcess(fixture.ownedProcess, {
      graceMs: 100,
      killWaitMs: 2_000
    })

    expect(stopped).toEqual({
      pid: fixture.ownedProcess.pid,
      termination: 'already_exited',
      signals: [],
      exit: { code: 0, signal: null, spawnError: null }
    })
  })

  it('cleans an owned process that exits before readiness without signalling a replacement PID', async () => {
    const fixture = spawnOwnedProcessFixture(`
process.stderr.write('fixture early exit\\n')
process.exit(23)
`)

    await expect(waitForOwnedProcessFixtureReady(fixture, 2_000))
      .rejects.toThrow(/exited before ready with code 23 and signal null/)
    expect(fixture.ownedProcess.stopResult).toEqual({
      pid: fixture.ownedProcess.pid,
      termination: 'already_exited',
      signals: [],
      exit: { code: 23, signal: null, spawnError: null }
    })
  })

  it('times out a never-ready process and completes startup cleanup through SIGTERM', async () => {
    const fixture = spawnOwnedProcessFixture(`
process.on('SIGTERM', () => process.exit(0))
setInterval(() => {}, 1_000)
`)

    await expect(waitForOwnedProcessFixtureReady(fixture, 100))
      .rejects.toThrow(/did not become ready/)
    expect(fixture.ownedProcess.stopResult).toEqual({
      pid: fixture.ownedProcess.pid,
      termination: 'sigterm',
      signals: ['SIGTERM'],
      exit: { code: 0, signal: null, spawnError: null }
    })
  })

  it('cleans an owned process after an invalid readiness payload', async () => {
    const fixture = spawnOwnedProcessFixture(`
process.on('SIGTERM', () => process.exit(0))
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY {invalid}\\n')
setInterval(() => {}, 1_000)
`)

    await expect(waitForOwnedProcessFixtureReady(fixture, 2_000))
      .rejects.toThrow(/invalid ready payload/)
    expect(fixture.ownedProcess.stopResult).toEqual({
      pid: fixture.ownedProcess.pid,
      termination: 'sigterm',
      signals: ['SIGTERM'],
      exit: { code: 0, signal: null, spawnError: null }
    })
  })

  it('escalates an owned detached group that ignores SIGTERM and confirms SIGKILL exit', async () => {
    const ignoredDescendant = process.platform === 'win32'
      ? ''
      : `
const { spawn } = require('node:child_process')
const descendant = spawn(process.execPath, ['-e', ${JSON.stringify(`
process.on('SIGTERM', () => {})
process.stdout.write('ready\\n')
setInterval(() => {}, 1_000)
`)}], { stdio: ['ignore', 'pipe', 'ignore'] })
descendant.stdout.once('data', () => {
  process.stdout.write(${JSON.stringify(OWNED_PROCESS_FIXTURE_READY_LINE)})
})
`
    const fixture = spawnOwnedProcessFixture(`
process.on('SIGTERM', () => {})
${ignoredDescendant || `process.stdout.write(${JSON.stringify(OWNED_PROCESS_FIXTURE_READY_LINE)})`}
setInterval(() => {}, 1_000)
`)

    await waitForOwnedProcessFixtureReady(fixture, 2_000)
    const stopped = await stopOwnedProcess(fixture.ownedProcess, {
      graceMs: 100,
      killWaitMs: 2_000
    })

    expect(stopped).toEqual({
      pid: fixture.ownedProcess.pid,
      termination: 'sigkill',
      signals: ['SIGTERM', 'SIGKILL'],
      exit: { code: null, signal: 'SIGKILL', spawnError: null }
    })
  })

  it('shares one deterministic stop across concurrent and repeated callers', async () => {
    const fixture = spawnOwnedProcessFixture(`
process.on('SIGTERM', () => setTimeout(() => process.exit(0), 25))
process.stdout.write(${JSON.stringify(OWNED_PROCESS_FIXTURE_READY_LINE)})
setInterval(() => {}, 1_000)
`)

    await waitForOwnedProcessFixtureReady(fixture, 2_000)
    const firstStop = stopOwnedProcess(fixture.ownedProcess, {
      graceMs: 500,
      killWaitMs: 2_000
    })
    const secondStop = stopOwnedProcess(fixture.ownedProcess, {
      graceMs: 1,
      killWaitMs: 1
    })
    expect(secondStop).toBe(firstStop)

    const [first, second] = await Promise.all([firstStop, secondStop])
    const third = await stopOwnedProcess(fixture.ownedProcess)
    expect(second).toBe(first)
    expect(third).toBe(first)
    expect(first).toEqual({
      pid: fixture.ownedProcess.pid,
      termination: 'sigterm',
      signals: ['SIGTERM'],
      exit: { code: 0, signal: null, spawnError: null }
    })
  })
})

type D0242ProviderFixtureId =
  | 'deepseek'
  | 'openai-compatible-live-local'
  | 'anthropic-compatible-live-local'
  | 'custom-endpoint-live-local'

type D0242ProviderFixtureDefinition = {
  id: D0242ProviderFixtureId
  kind: 'openai-compatible' | 'anthropic-compatible' | 'custom-endpoint'
  family: 'deepseek' | 'openai-compatible' | 'anthropic-compatible' | 'custom_endpoint'
  endpointFormat: 'chat_completions' | 'messages' | 'custom_endpoint'
  endpoint: string
  model: 'deepseek-chat' | 'gpt-compatible' | 'claude-compatible' | 'custom-compatible'
  selectedRoutes: readonly ['primary']
}

type D0242ProviderRegistrySnapshot = {
  registryRevision: string
  registryIncarnation: string
  providersById: Map<string, Record<string, unknown>>
}

type D0242ProviderRegistryFixtureContext = {
  providers: readonly D0242ProviderFixtureDefinition[]
  credentialBytesByProviderId: Map<D0242ProviderFixtureId, Buffer>
  credentialBase64ByProviderId: Map<D0242ProviderFixtureId, string>
  forbiddenValues: string[]
  connectedProviderIds: D0242ProviderFixtureId[]
  selectedProviderId: D0242ProviderFixtureId | ''
  dispose: () => void
}

type D0242ProviderRegistryFixturePhase =
  | 'k1'
  | 'initial_get'
  | 'connect'
  | 'select'
  | 'readback'

function failD0242ProviderRegistryFixture(
  phase: D0242ProviderRegistryFixturePhase,
  providerId: D0242ProviderFixtureId | 'none',
  resultClass: string
): never {
  throw new Error(`D-0242 Provider Registry fixture failed (${phase}/${providerId}/${resultClass}).`)
}

async function prepareD0242SyntheticProviderRegistryMasterKey(dataDir: string): Promise<void> {
  const privateDir = join(dataDir, 'private')
  const providerSecretsDir = join(privateDir, 'provider-secrets')
  const masterKeyDir = join(providerSecretsDir, 'master-key')
  const masterKey = Buffer.alloc(32, 0x6a)
  try {
    for (const directory of [dataDir, privateDir, providerSecretsDir, masterKeyDir]) {
      await mkdir(directory, { recursive: true, mode: 0o700 })
      await chmod(directory, 0o700)
    }
    const masterKeyPath = join(masterKeyDir, 'master.key')
    await writeFile(masterKeyPath, masterKey, { flag: 'wx', mode: 0o600 })
    await chmod(masterKeyPath, 0o600)
    if (process.platform === 'darwin') {
      const authorityPath = join(masterKeyDir, 'authority.v1')
      await writeFile(authorityPath, 'analytix-master-key-authority:v1:fallback\n', {
        flag: 'wx',
        mode: 0o600
      })
      await chmod(authorityPath, 0o600)
    }
  } catch {
    await rm(dataDir, { recursive: true, force: true }).catch(() => undefined)
    failD0242ProviderRegistryFixture('k1', 'none', 'setup_failed')
  } finally {
    masterKey.fill(0)
  }
}

function d0242ProviderFixtureDefinitions(
  endpoint: string
): readonly D0242ProviderFixtureDefinition[] {
  return [
    {
      id: 'openai-compatible-live-local',
      kind: 'openai-compatible',
      family: 'openai-compatible',
      endpointFormat: 'chat_completions',
      endpoint,
      model: 'gpt-compatible',
      selectedRoutes: ['primary']
    },
    {
      id: 'deepseek',
      kind: 'openai-compatible',
      family: 'deepseek',
      endpointFormat: 'chat_completions',
      endpoint,
      model: 'deepseek-chat',
      selectedRoutes: ['primary']
    },
    {
      id: 'anthropic-compatible-live-local',
      kind: 'anthropic-compatible',
      family: 'anthropic-compatible',
      endpointFormat: 'messages',
      endpoint,
      model: 'claude-compatible',
      selectedRoutes: ['primary']
    },
    {
      id: 'custom-endpoint-live-local',
      kind: 'custom-endpoint',
      family: 'custom_endpoint',
      endpointFormat: 'custom_endpoint',
      endpoint,
      model: 'custom-compatible',
      selectedRoutes: ['primary']
    }
  ]
}

function createD0242ProviderRegistryFixtureContext(
  endpoint: string
): D0242ProviderRegistryFixtureContext {
  const providers = d0242ProviderFixtureDefinitions(endpoint)
  const credentialBytesByProviderId = new Map<D0242ProviderFixtureId, Buffer>([
    ['openai-compatible-live-local', Buffer.from('test-openai-provider-key', 'utf8')],
    ['deepseek', Buffer.from('test-deepseek-provider-key', 'utf8')],
    ['anthropic-compatible-live-local', Buffer.from('test-anthropic-provider-key', 'utf8')],
    ['custom-endpoint-live-local', Buffer.alloc(32, 0x63)]
  ])
  const credentialBase64ByProviderId = new Map<D0242ProviderFixtureId, string>()
  const forbiddenValues: string[] = []
  for (const [providerId, credential] of credentialBytesByProviderId) {
    const credentialText = credential.toString('utf8')
    const credentialBase64 = credential.toString('base64')
    credentialBase64ByProviderId.set(providerId, credentialBase64)
    forbiddenValues.push(credentialText, credentialBase64)
  }
  forbiddenValues.push(
    'credentialRef',
    'ciphertext',
    'nonce',
    'masterKey',
    'master-key',
    'valueBase64',
    'rawBody',
    'raw_body'
  )
  const context: D0242ProviderRegistryFixtureContext = {
    providers,
    credentialBytesByProviderId,
    credentialBase64ByProviderId,
    forbiddenValues,
    connectedProviderIds: [],
    selectedProviderId: '',
    dispose: () => {
      for (const credential of credentialBytesByProviderId.values()) credential.fill(0)
      credentialBytesByProviderId.clear()
      credentialBase64ByProviderId.clear()
      forbiddenValues.fill('')
      context.connectedProviderIds.splice(0)
      context.selectedProviderId = ''
    }
  }
  return context
}

function d0242ProviderRegistryProjectionContainsPrivateCredential(
  projection: Record<string, unknown>,
  forbiddenValues: readonly string[]
): boolean {
  const encoded = JSON.stringify(projection)
  if (forbiddenValues.some((value) => value !== '' && encoded.includes(value))) return true
  const allowedCredentialMetadata = new Set([
    'credentialConfigured',
    'credentialPurpose',
    'credentialAuthorityBound'
  ])
  const visit = (value: unknown): boolean => {
    if (Array.isArray(value)) return value.some(visit)
    if (value === null || typeof value !== 'object') return false
    return Object.entries(value).some(([key, nested]) => {
      const lowered = key.toLowerCase()
      if (lowered === 'credential' || lowered.includes('secret')) return true
      if (lowered.includes('credential') && !allowedCredentialMetadata.has(key)) return true
      return visit(nested)
    })
  }
  return visit(projection)
}

function d0242ProviderRegistryProjectionMatches(
  provider: unknown,
  expected: D0242ProviderFixtureDefinition
): boolean {
  const projection = recordValue(provider)
  return projection.id === expected.id &&
    projection.kind === expected.kind &&
    projection.endpoint === expected.endpoint &&
    projection.selectedModel === expected.model &&
    Array.isArray(projection.models) &&
    projection.models.length === 1 &&
    projection.models[0] === expected.model &&
    Array.isArray(projection.selectedRoutes) &&
    projection.selectedRoutes.length === 1 &&
    projection.selectedRoutes[0] === expected.selectedRoutes[0] &&
    projection.credentialConfigured === true
}

function d0242ProviderFixtureById(
  context: D0242ProviderRegistryFixtureContext,
  providerId: D0242ProviderFixtureId
): D0242ProviderFixtureDefinition {
  const provider = context.providers.find((candidate) => candidate.id === providerId)
  if (provider === undefined) {
    failD0242ProviderRegistryFixture('readback', providerId, 'provider_definition_missing')
  }
  return provider
}

async function readD0242ProviderRegistrySnapshot(
  server: GoRuntimeServerHandle,
  context: D0242ProviderRegistryFixtureContext,
  phase: D0242ProviderRegistryFixturePhase,
  ownerProviderId: D0242ProviderFixtureId | 'none'
): Promise<D0242ProviderRegistrySnapshot> {
  let response: Awaited<ReturnType<typeof goRuntimeServerJSON>>
  try {
    response = await goRuntimeServerJSON(server, '/v1/provider-registry')
  } catch {
    failD0242ProviderRegistryFixture(phase, ownerProviderId, 'request_failed')
  }
  if (response.status !== 200) {
    failD0242ProviderRegistryFixture(phase, ownerProviderId, 'http_status_mismatch')
  }
  if (d0242ProviderRegistryProjectionContainsPrivateCredential(
    response.body,
    context.forbiddenValues
  )) {
    failD0242ProviderRegistryFixture(phase, ownerProviderId, 'private_projection')
  }
  const parsed = providerRegistrySnapshotResponseSchemaV1.safeParse(response.body)
  if (!parsed.success) {
    failD0242ProviderRegistryFixture(phase, ownerProviderId, 'public_projection_invalid')
  }
  const providersById = new Map(parsed.data.providers.map((provider) => [
    provider.id,
    recordValue(provider)
  ] as const))
  if (parsed.data.providers.length !== context.connectedProviderIds.length ||
      providersById.size !== context.connectedProviderIds.length ||
      (parsed.data.selectedProviderId ?? '') !== context.selectedProviderId) {
    failD0242ProviderRegistryFixture(phase, ownerProviderId, 'winner_mismatch')
  }
  for (const providerId of context.connectedProviderIds) {
    if (!d0242ProviderRegistryProjectionMatches(
      providersById.get(providerId),
      d0242ProviderFixtureById(context, providerId)
    )) {
      failD0242ProviderRegistryFixture(phase, providerId, 'provider_mismatch')
    }
  }
  return {
    registryRevision: parsed.data.registryRevision,
    registryIncarnation: parsed.data.registryIncarnation,
    providersById
  }
}

function assertD0242ProviderRegistryMutation(
  body: Record<string, unknown>,
  context: D0242ProviderRegistryFixtureContext,
  provider: D0242ProviderFixtureDefinition,
  phase: 'connect' | 'select'
): void {
  if (d0242ProviderRegistryProjectionContainsPrivateCredential(body, context.forbiddenValues)) {
    failD0242ProviderRegistryFixture(phase, provider.id, 'private_projection')
  }
  const parsed = providerRegistryProviderResponseSchemaV1.safeParse(body)
  if (!parsed.success) {
    failD0242ProviderRegistryFixture(phase, provider.id, 'public_projection_invalid')
  }
  if (!d0242ProviderRegistryProjectionMatches(parsed.data.provider, provider)) {
    failD0242ProviderRegistryFixture(phase, provider.id, 'provider_mismatch')
  }
}

async function connectD0242ProviderRegistryProvider(
  server: GoRuntimeServerHandle,
  context: D0242ProviderRegistryFixtureContext,
  providerId: D0242ProviderFixtureId
): Promise<void> {
  const provider = d0242ProviderFixtureById(context, providerId)
  const before = await readD0242ProviderRegistrySnapshot(server, context, 'readback', providerId)
  const credentialBase64 = context.credentialBase64ByProviderId.get(providerId)
  if (!credentialBase64 || context.connectedProviderIds.includes(providerId)) {
    failD0242ProviderRegistryFixture('connect', providerId, 'credential_or_state_invalid')
  }
  let body = JSON.stringify({
    schemaVersion: 1,
    expected: {
      registryRevision: before.registryRevision,
      registryIncarnation: before.registryIncarnation,
      providerRevision: '0',
      providerGeneration: '0',
      providerIncarnation: '',
      providerCredentialPurpose: ''
    },
    provider: {
      id: provider.id,
      kind: provider.kind,
      endpoint: provider.endpoint,
      proxy: '',
      models: [provider.model],
      mediaModels: [],
      selectedModel: provider.model,
      selectedMediaModel: '',
      selectedRoutes: [...provider.selectedRoutes]
    },
    credential: {
      kind: 'set',
      purpose: 'provider-api-key',
      valueBase64: credentialBase64
    }
  })
  try {
    let response: Awaited<ReturnType<typeof goRuntimeServerJSON>>
    try {
      response = await goRuntimeServerJSON(server, '/v1/provider-registry', {
        method: 'POST',
        body
      })
    } catch {
      failD0242ProviderRegistryFixture('connect', providerId, 'request_failed')
    }
    if (response.status !== 200) {
      failD0242ProviderRegistryFixture('connect', providerId, 'http_status_mismatch')
    }
    assertD0242ProviderRegistryMutation(response.body, context, provider, 'connect')
    context.connectedProviderIds.push(providerId)
    if (context.selectedProviderId === '') context.selectedProviderId = providerId
    await readD0242ProviderRegistrySnapshot(server, context, 'readback', providerId)
  } finally {
    body = ''
  }
}

async function selectD0242ProviderRegistryWinner(
  server: GoRuntimeServerHandle,
  context: D0242ProviderRegistryFixtureContext,
  providerId: D0242ProviderFixtureId
): Promise<void> {
  const provider = d0242ProviderFixtureById(context, providerId)
  const before = await readD0242ProviderRegistrySnapshot(server, context, 'readback', providerId)
  const current = before.providersById.get(providerId)
  if (current === undefined) {
    failD0242ProviderRegistryFixture('select', providerId, 'provider_missing')
  }
  const providerRevision = String(current.revision || '')
  const providerGeneration = String(current.generation || '')
  const providerIncarnation = String(current.incarnation || '')
  const providerCredentialPurpose = String(current.credentialPurpose || '')
  if (!providerRevision || !providerGeneration || !providerIncarnation ||
      !providerCredentialPurpose) {
    failD0242ProviderRegistryFixture('select', providerId, 'cas_missing')
  }
  let body = JSON.stringify({
    schemaVersion: 1,
    expected: {
      registryRevision: before.registryRevision,
      registryIncarnation: before.registryIncarnation,
      providerRevision,
      providerGeneration,
      providerIncarnation,
      providerCredentialPurpose
    }
  })
  try {
    let response: Awaited<ReturnType<typeof goRuntimeServerJSON>>
    try {
      response = await goRuntimeServerJSON(
        server,
        `/v1/provider-registry/providers/${encodeURIComponent(providerId)}/select`,
        { method: 'POST', body }
      )
    } catch {
      failD0242ProviderRegistryFixture('select', providerId, 'request_failed')
    }
    if (response.status !== 200) {
      failD0242ProviderRegistryFixture('select', providerId, 'http_status_mismatch')
    }
    assertD0242ProviderRegistryMutation(response.body, context, provider, 'select')
    context.selectedProviderId = providerId
    await readD0242ProviderRegistrySnapshot(server, context, 'readback', providerId)
  } finally {
    body = ''
  }
}

async function establishD0242ExplicitProviderRegistryWinner(
  server: GoRuntimeServerHandle,
  context: D0242ProviderRegistryFixtureContext
): Promise<void> {
  await readD0242ProviderRegistrySnapshot(server, context, 'initial_get', 'none')
  for (const provider of context.providers) {
    await connectD0242ProviderRegistryProvider(server, context, provider.id)
  }
  await selectD0242ProviderRegistryWinner(server, context, 'deepseek')
}

describe('Go runtime kernel conformance manifest', () => {
  it('runs the D-0242 Go runtime server contract subset behind the internal candidate boundary', async () => {
    const durableTempDir = await mkdtemp(join(tmpdir(), 'analytix-go-runtime-server-'))
    await prepareD0242SyntheticProviderRegistryMasterKey(durableTempDir)
    let runtimeBuild: GoRuntimeServerBuild | null = null
    let provider: ScriptedProviderHandle | null = null
    let server: GoRuntimeServerHandle | null = null
    let registryFixture: D0242ProviderRegistryFixtureContext | null = null
    try {
      runtimeBuild = await buildGoRuntimeServerBinary()
      provider = await startScriptedProvider()
      server = await startGoRuntimeServer(durableTempDir, {
        runtimeServerBinary: runtimeBuild.executablePath,
        providerBaseURL: provider.url
      })
      registryFixture = createD0242ProviderRegistryFixtureContext(provider.url)
      await establishD0242ExplicitProviderRegistryWinner(server, registryFixture)

      const health = await fetch(`${server.url}/health`)
      expect(health.status).toBe(200)
      expect(await health.json()).toEqual({ status: 'ok', service: 'analytix', mode: 'serve' })

      const unauthorized = await fetch(`${server.url}/v1/runtime/info`)
      expect(unauthorized.status).toBe(401)
      expect(await unauthorized.json()).toEqual({
        code: 'unauthorized',
        message: 'Runtime authentication is required.'
      })

      const info = await goRuntimeServerJSON(server, '/v1/runtime/info')
      expect(info.status).toBe(200)
	  const publicInfo = RuntimeInfoResponse.parse(info.body)
	  expect(publicInfo).toEqual(expect.objectContaining({
		schemaVersion: 2,
		status: 'ready',
		listenerScope: 'loopback',
		executionPolicy: {
		  approvalPolicy: 'on-request',
		  sandboxMode: 'workspace-write'
		},
		provider: expect.objectContaining({ model: 'deepseek-chat' }),
		storage: { configured: true, available: true }
	  }))
	  for (const forbidden of ['host', 'dataDir', 'model', 'approvalPolicy', 'sandboxMode', 'providers']) {
		expect(publicInfo).not.toHaveProperty(forbidden)
	  }
      const capabilities = recordValue(info.body.capabilities)
      expect(recordValue(capabilities.mcp).available).toBe(false)
      expect(recordValue(capabilities.mcp).configuredServers).toBe(0)
      expect(recordValue(capabilities.mcp).connectedServers).toBe(0)
      expect(recordValue(capabilities.mcp).toolCount).toBe(0)
      expect(recordValue(recordValue(capabilities.mcp).search).indexedToolCount).toBe(0)
      expect(recordValue(recordValue(capabilities.mcp).search).advertisedToolCount).toBe(0)
      expect(recordValue(capabilities.mcp)).not.toHaveProperty('contractProof')
      expect(recordValue(capabilities.subagents).available).toBe(true)
      expect(recordValue(capabilities.subagents).internalLineageAvailable).toBe(true)
      expect(recordValue(capabilities.subagents).profilesAvailable).toBe(true)
      expect(recordValue(capabilities.subagents).durableChildRunStore).toBe(true)
      expect(recordValue(capabilities.subagents).parallelExecutionAvailable).toBe(true)
      expect(recordValue(capabilities.subagents).taskToolAvailable).toBe(true)
      expect(recordValue(capabilities.subagents).parallelTasksToolAvailable).toBe(true)
	  expect(recordValue(capabilities.subagents)).not.toHaveProperty('topLevelRouteExposed')
      expect(recordValue(capabilities.attachments).available).toBe(true)
      expect(recordValue(capabilities.memory).available).toBe(true)
	  expect(capabilities).not.toHaveProperty('upstreamAbsorption')

      const tools = await goRuntimeServerJSON(server, '/v1/runtime/tools')
      expect(tools.status).toBe(200)
	  const toolDiagnostics = RuntimeToolsResponse.parse(tools.body)
      expect(Array.isArray(toolDiagnostics.mcpServers) ? toolDiagnostics.mcpServers : []).toHaveLength(0)
      expect(recordValue(toolDiagnostics.mcpSearch)).toEqual(expect.objectContaining({
        active: false,
        available: false,
        indexedToolCount: 0,
        advertisedToolCount: 0
      }))
	  expect(toolDiagnostics.toolContracts.count).toBeGreaterThan(0)
	  expect(toolDiagnostics.toolContracts.catalogHash).toMatch(/^[a-f0-9]{64}$/)
      const commandDiagnostics = Array.isArray(toolDiagnostics.commands)
        ? toolDiagnostics.commands.map((entry) => recordValue(entry))
        : []
      expect(commandDiagnostics).toEqual(expect.arrayContaining([
        expect.objectContaining({
		  binary: 'go',
		  found: true,
		  status: 'available'
        })
      ]))
	  expect(JSON.stringify(commandDiagnostics)).not.toContain('output')
	  expect(JSON.stringify(commandDiagnostics)).not.toContain('/Users/')
      expect(toolDiagnostics).not.toHaveProperty('mcpLocalProof')
      expect(recordValue(toolDiagnostics.subagents)).toEqual(expect.objectContaining({
        enabled: true,
        available: true,
        internalLineageAvailable: true,
        parallelExecutionAvailable: true,
        taskToolAvailable: true,
		parallelTasksToolAvailable: true
      }))
	  for (const forbidden of ['providers', 'webProviders', 'mcpPrompts', 'mcpResources']) {
		expect(toolDiagnostics).not.toHaveProperty(forbidden)
	  }
	  const serializedDiagnostics = JSON.stringify(toolDiagnostics)
	  for (const forbidden of ['rootDir', 'lastError', 'lastActivations', 'toolNames', 'output']) {
		expect(serializedDiagnostics).not.toContain(forbidden)
	  }

      const created = await goRuntimeServerJSON(server, '/v1/threads', {
        method: 'POST',
        body: JSON.stringify({
          title: 'D-0242 runtime server contract',
          workspace: durableTempDir,
          model: 'deepseek-chat'
        })
      })
      expect(created.status).toBe(201)

      const threads = await goRuntimeServerJSON(server, '/v1/threads?limit=1')
      expect(threads.status).toBe(200)
      const threadList = Array.isArray(threads.body.threads) ? threads.body.threads : []
      expect(threadList).toHaveLength(1)
      const threadIdValue = recordValue(threadList[0]).id
      expect(typeof threadIdValue).toBe('string')
      if (typeof threadIdValue !== 'string') throw new Error('Go runtime thread id is not a string')
      const threadId = threadIdValue
      const encodedThreadId = encodeURIComponent(threadId)

      const thread = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}`)
      expect(thread.status).toBe(200)
      expect(thread.body).toEqual(expect.objectContaining({
        id: threadId,
        latestSeq: expect.any(Number)
      }))

      const rejectedStatus = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}`, {
        method: 'PATCH',
        body: JSON.stringify({ status: 'archived' })
      })
      expect(rejectedStatus.status).toBe(400)
	  expect(rejectedStatus.body).toEqual({
		code: 'validation_error',
		message: 'The request did not satisfy the runtime contract.'
	  })
	  expect(JSON.stringify(rejectedStatus.body)).not.toContain('runtime-owned')

      const patched = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}`, {
        method: 'PATCH',
        body: JSON.stringify({ title: 'D-0242 Runtime Server Canary', workspace: durableTempDir })
      })
      expect(patched.status, JSON.stringify(patched.body)).toBe(200)
      expect(patched.body).toEqual(expect.objectContaining({
        title: 'D-0242 Runtime Server Canary',
        workspace: durableTempDir
      }))
      expect(patched.body.status).not.toBe('archived')

      const fork = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}/fork`, {
        method: 'POST',
        body: JSON.stringify({ relation: 'side', title: 'D-0242 Side' })
      })
      expect(fork.status).toBe(201)
      expect(fork.body).toEqual(expect.objectContaining({
        relation: 'side',
        parentThreadId: threadId
      }))

      const resume = await goRuntimeServerJSON(server, `/v1/sessions/${encodedThreadId}/resume-thread`, {
        method: 'POST',
        body: JSON.stringify({ workspace: durableTempDir, model: 'deepseek-chat', mode: 'agent' })
      })
      expect(resume.status).toBe(201)
      expect(resume.body).toEqual(expect.objectContaining({
        session_id: threadId,
        thread_id: expect.any(String),
        message_count: expect.any(Number)
      }))

      const turn = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}/turns`, {
        method: 'POST',
        body: JSON.stringify({ prompt: '/goal --research Run the D-0242 runtime server contract test.', model: 'deepseek-chat' })
      })
      expect(turn.status, JSON.stringify(turn.body)).toBe(202)
      expect(turn.body).toEqual(expect.objectContaining({
        threadId,
        turnId: expect.any(String),
        userMessageItemId: expect.any(String)
      }))

      const runtimeUsage = await goRuntimeServerJSON(server, '/v1/usage')
      expect(runtimeUsage.status).toBe(200)
      expect(recordValue(runtimeUsage.body.total)).toEqual(expect.objectContaining({
        promptTokens: 1000,
        completionTokens: 40,
        reasoningTokens: 12,
        totalTokens: 1040,
        cacheHitTokens: 700,
        cacheMissTokens: 300
      }))
      const threadUsage = await goRuntimeServerJSON(server, `/v1/usage?group_by=thread&thread_id=${encodedThreadId}`)
      expect(threadUsage.status).toBe(200)
      const threadUsageBuckets = Array.isArray(threadUsage.body.buckets) ? threadUsage.body.buckets.map(recordValue) : []
      expect(threadUsageBuckets).toEqual([
        expect.objectContaining({
          thread_id: threadId,
          input_tokens: 1000,
          output_tokens: 40,
          reasoning_tokens: 12,
          cached_tokens: 700,
          cache_hit_tokens: 700,
          cache_miss_tokens: 300,
          total_tokens: 1040,
          provider: 'deepseek',
          last_turn_cache_hit_rate: 0.7
        })
      ])
      const missingDayWindow = await goRuntimeServerJSON(server, '/v1/usage?group_by=day&timezone=UTC')
      expect(missingDayWindow.status).toBe(400)
	  expect(missingDayWindow.body).toEqual({
        code: 'validation_error',
		message: 'The request did not satisfy the runtime contract.'
	  })
	  expect(JSON.stringify(missingDayWindow.body)).not.toContain('requires from and to')
      const dayUsage = await goRuntimeServerJSON(server, '/v1/usage?group_by=day&window=today&timezone=UTC')
      expect(dayUsage.status).toBe(200)
      expect(recordValue(dayUsage.body.totals)).toEqual(expect.objectContaining({
        total_tokens: 1040,
        thread_count: 1
      }))
      const usageFrom = String(dayUsage.body.from)
      const usageTo = String(dayUsage.body.to)
      expect(usageFrom).toMatch(/^\d{4}-\d{2}-\d{2}$/)
      expect(usageTo).toMatch(/^\d{4}-\d{2}-\d{2}$/)
      const modelUsage = await goRuntimeServerJSON(server, `/v1/usage?group_by=model&from=${usageFrom}&to=${usageTo}&timezone=UTC`)
      expect(modelUsage.status).toBe(200)
      const modelUsageBuckets = Array.isArray(modelUsage.body.buckets) ? modelUsage.body.buckets.map(recordValue) : []
      expect(modelUsageBuckets).toEqual([
        expect.objectContaining({
          model: 'deepseek-chat',
          provider: 'deepseek',
          total_tokens: 1040
        })
      ])
      const threadWithUsage = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}`)
      expect(threadWithUsage.status).toBe(200)
      expect(recordValue(threadWithUsage.body.usage)).toEqual(expect.objectContaining({
        totalTokens: 1040,
        cacheHitTokens: 700,
        cacheMissTokens: 300
      }))

      const replay = await goRuntimeServerText(server, `/v1/threads/${encodedThreadId}/events?since_seq=0`)
      expect(replay.status).toBe(200)
      for (const token of [
        'event: turn_started',
        'event: item_created',
        'event: autoresearch_state_audit',
        '"fileCount":5',
        '"result":"pivot_required"',
        '"stablePrefixContainsState":false',
        '"toolSchemaContainsState":false',
        '"topLevelAutoResearchRouteExposed":false',
        'event: general_terminal_batch',
        '"factAnswerAllowed":false',
        '"evidenceAuthority":false',
        '"citationAuthority":false',
        '"kind":"item_completed"',
        '"kind":"usage"',
        '"cacheHitTokens":700',
        '"kind":"turn_completed"'
      ]) {
        expect(replay.text).toContain(token)
      }
      const replayPayloads = runtimeServerSsePayloads(replay.text, threadId)
      const terminalBatches = replayPayloads.filter((payload) =>
        payload.kind === 'general_terminal_batch' &&
        payload.threadId === threadId &&
        payload.turnId === turn.body.turnId
      )
      expect(terminalBatches).toHaveLength(1)
      expect(terminalBatches[0]).toMatchObject({
        factAnswerAllowed: false,
        evidenceAuthority: false,
        citationAuthority: false
      })
      const verifiedGoTerminalBatch = verifiedGeneralTerminalDeliveryBatchV1(terminalBatches[0])
      expect(verifiedGoTerminalBatch).not.toBeNull()
      if (verifiedGoTerminalBatch === null) {
        throw new Error('Go runtime typed ordinary terminal batch was not verified')
      }
      expect(verifiedGoTerminalBatch.batch).toEqual(terminalBatches[0])
      expect(verifiedGoTerminalBatch.batch).toMatchObject({
        factAnswerAllowed: false,
        evidenceAuthority: false,
        citationAuthority: false
      })
      const digestMutation = structuredClone(terminalBatches[0])
      digestMutation.batchDigest = '0'.repeat(64)
      expect(verifiedGeneralTerminalDeliveryBatchV1(digestMutation)).toBeNull()
      const projectedTerminalEvents = verifiedGoTerminalBatch.events.map(recordValue)
      const atomicAssistantFinals = projectedTerminalEvents.filter((payload) => {
        const item = recordValue(payload.item)
        return payload.kind === 'item_completed' &&
          payload.threadId === threadId &&
          payload.turnId === turn.body.turnId &&
          payload.itemId === `item_${String(turn.body.turnId)}_assistant` &&
          item.id === payload.itemId &&
          item.threadId === threadId &&
          item.turnId === turn.body.turnId &&
          item.kind === 'assistant_text' &&
          item.role === 'assistant' &&
          item.status === 'completed' &&
          item.text === 'Go runtime ok'
      })
      expect(atomicAssistantFinals).toHaveLength(1)
      const atomicAssistantItem = recordValue(atomicAssistantFinals[0].item)
      const ordinaryResult = recordValue(atomicAssistantItem.ordinaryResult)
      expect(ordinaryResult).toMatchObject({
        schemaVersion: 1,
        purpose: 'analytix.ordinary-result/v1',
        projectionVersion: 'analytix.ordinary-output-projection/v1',
        logicalEffect: 'ordinary',
        ordinaryWork: true,
        candidateOrigin: 'provider_ordinary_only',
        evidenceAuthority: false,
        citationAuthority: false,
        factAnswerAllowed: false,
        text: 'Go runtime ok'
      })
      expect(atomicAssistantItem.text).toBe(ordinaryResult.text)
      expect(ordinaryResult.textSha256).toBe(
        createHash('sha256').update('Go runtime ok', 'utf8').digest('hex')
      )
      expect(ordinaryResult.resultDigest).toMatch(/^[0-9a-f]{64}$/)
      const targetTerminal = projectedTerminalEvents.filter((payload) =>
        payload.kind === 'turn_completed' &&
        payload.threadId === threadId &&
        payload.turnId === turn.body.turnId &&
        payload.status === 'completed'
      )
      expect(targetTerminal).toHaveLength(1)
      expect(projectedTerminalEvents.indexOf(targetTerminal[0]))
        .toBeGreaterThan(projectedTerminalEvents.indexOf(atomicAssistantFinals[0]))
      expect(replayPayloads.some((payload) => payload.kind === 'assistant_text_delta')).toBe(false)
      const ordinaryTextToken = JSON.stringify('Go runtime ok')
      expect(replayPayloads.filter((payload) => JSON.stringify(payload).includes(ordinaryTextToken)))
        .toEqual(terminalBatches)
      for (const forbidden of [
        'event: assistant_text_delta',
        'event: approval_requested',
        'event: user_input_requested',
        'event: approval_resolved',
        'event: user_input_resolved',
        'event: tool_catalog_changed',
        'event: mcp_lifecycle_audit',
        'event: goal_evidence_audit',
        '"answers"',
        '"mcp__analytix_local_56366d__search_issues_244952"',
        'Ship',
        'Choose direction',
        'local-fake-key',
        'D0244'
      ]) {
        expect(replay.text).not.toContain(forbidden)
      }

      const providerTurns = [
        {
          providerId: 'openai-compatible-live-local',
          model: 'gpt-compatible',
          family: 'openai-compatible',
          endpointFormat: 'chat_completions',
          text: 'openai compatible hello',
          cacheTelemetrySupported: true
        },
        {
          providerId: 'anthropic-compatible-live-local',
          model: 'claude-compatible',
          family: 'anthropic-compatible',
          endpointFormat: 'messages',
          text: 'anthropic compatible hello',
          cacheTelemetrySupported: true
        },
        {
          providerId: 'custom-endpoint-live-local',
          model: 'custom-compatible',
          family: 'custom_endpoint',
          endpointFormat: 'custom_endpoint',
          text: 'custom endpoint hello',
          cacheTelemetrySupported: true
        }
      ] as const
      for (const item of providerTurns) {
        await selectD0242ProviderRegistryWinner(server, registryFixture, item.providerId)
        const providerTurn = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}/turns`, {
          method: 'POST',
          body: JSON.stringify({
            prompt: `Run runtime provider ${item.family}.`,
            providerId: item.providerId,
            model: item.model
          })
        })
        expect(providerTurn.status).toBe(202)
        expect(JSON.stringify(providerTurn.body)).not.toContain(item.text)
      }

      const multiModelReplay = await goRuntimeServerText(server, `/v1/threads/${encodedThreadId}/events?since_seq=0`)
      const usageDiagnosticsByModel = new Map<string, Record<string, unknown>>()
      for (const payload of runtimeServerSsePayloads(multiModelReplay.text, threadId)) {
        const projected = payload.kind === 'general_terminal_batch' && Array.isArray(payload.events)
          ? payload.events.map(recordValue)
          : [payload]
        for (const event of projected) {
          if (event.kind !== 'usage') continue
          const diagnostics = recordValue(event.cacheDiagnostics)
          if (typeof event.model === 'string') usageDiagnosticsByModel.set(event.model, diagnostics)
        }
      }
      for (const item of providerTurns) {
        expect(multiModelReplay.text).not.toContain(item.text)
        const diagnostics = usageDiagnosticsByModel.get(item.model)
        expect(diagnostics).toEqual(expect.objectContaining({
          cacheTelemetrySupported: item.cacheTelemetrySupported,
          providerNativeCacheTelemetry: true,
          cacheHitTokens: expect.any(Number),
          cacheMissTokens: expect.any(Number)
        }))
        expect(recordValue(diagnostics)).not.toHaveProperty('provider')
        expect(recordValue(diagnostics)).not.toHaveProperty('providerId')
        expect(recordValue(diagnostics)).not.toHaveProperty('model')
        expect(recordValue(diagnostics)).not.toHaveProperty('endpointFormat')
        expect(Number(recordValue(diagnostics).cacheHitTokens)).toBeGreaterThanOrEqual(0)
        expect(Number(recordValue(diagnostics).cacheMissTokens)).toBeGreaterThanOrEqual(0)
      }
      const deepseekDiagnostics = usageDiagnosticsByModel.get('deepseek-chat')
      expect(deepseekDiagnostics).toEqual(expect.objectContaining({
        cacheTelemetrySupported: true,
        cacheHitTokens: 700,
        cacheMissTokens: 300
      }))

      const hiddenGoRoute = await goRuntimeServerJSON(server, '/v1/runtime/go')
      const hiddenBoundaryRoute = await goRuntimeServerJSON(server, '/v1/internal/go-production-candidate/boundary')
      const hiddenAutoResearchRoute = await goRuntimeServerJSON(server, '/v1/autoresearch')
      expect(hiddenGoRoute).toEqual(expect.objectContaining({ status: 404 }))
      expect(hiddenBoundaryRoute).toEqual(expect.objectContaining({ status: 404 }))
      expect(hiddenAutoResearchRoute).toEqual(expect.objectContaining({ status: 404 }))
      expect(hiddenGoRoute.body).toEqual(expect.objectContaining({ code: 'not_found' }))
      expect(hiddenBoundaryRoute.body).toEqual(expect.objectContaining({ code: 'not_found' }))
      expect(hiddenAutoResearchRoute.body).toEqual(expect.objectContaining({ code: 'not_found' }))
    } finally {
      try {
        if (server !== null) await stopGoRuntimeServer(server)
      } finally {
        try {
          if (provider !== null) await provider.close()
        } finally {
          try {
            if (registryFixture !== null) registryFixture.dispose()
          } finally {
            try {
              if (runtimeBuild !== null) await runtimeBuild.cleanup()
            } finally {
              await rm(durableTempDir, { recursive: true, force: true })
            }
          }
        }
      }
    }
  }, 300_000)

  it('runs the runtime-server crash/restart durable root and rollback scaffold drill', async () => {
    const durableRoot = await mkdtemp(join(tmpdir(), 'analytix-go-runtime-candidate-drill-'))
    await prepareD0242SyntheticProviderRegistryMasterKey(durableRoot)
    let runtimeBuild: GoRuntimeServerBuild | null = null
    let provider: ScriptedProviderHandle | null = null
    let server: GoRuntimeServerHandle | null = null
    let registryFixture: D0242ProviderRegistryFixtureContext | null = null
    try {
      runtimeBuild = await buildGoRuntimeServerBinary()
      provider = await startScriptedProvider()
      server = await startGoRuntimeServer(durableRoot, {
        runtimeServerBinary: runtimeBuild.executablePath,
        runtimeDurableRoot: true,
        providerBaseURL: provider.url
      })
      registryFixture = createD0242ProviderRegistryFixtureContext(provider.url)
      await establishD0242ExplicitProviderRegistryWinner(server, registryFixture)

      const created = await goRuntimeServerJSON(server, '/v1/threads', {
        method: 'POST',
        body: JSON.stringify({
          title: 'Runtime crash restart drill',
          workspace: durableRoot,
          model: 'deepseek-chat'
        })
      })
      expect(created.status).toBe(201)
      const threadId = String(recordValue(created.body).id)
      expect(threadId).toBeTruthy()
      const encodedThreadId = encodeURIComponent(threadId)
      const researchWorkspace = join(durableRoot, 'workspace')
      await mkdir(researchWorkspace, { recursive: true })
      const patchWorkspace = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}`, {
        method: 'PATCH',
        body: JSON.stringify({ workspace: researchWorkspace })
      })
      expect(patchWorkspace.status).toBe(200)

      const turn = await goRuntimeServerJSON(server, `/v1/threads/${encodedThreadId}/turns`, {
        method: 'POST',
          body: JSON.stringify({ prompt: '/goal --research Run the runtime crash restart drill.', model: 'deepseek-chat' })
      })
      expect(turn.status, JSON.stringify(turn.body)).toBe(202)
      expect(turn.body.turnId).toBe('turn_1')

      const before = await goRuntimeServerText(server, `/v1/threads/${encodedThreadId}/events?since_seq=0`)
      expect(before.text).toContain('event: autoresearch_state_audit')
      expect(before.text).toContain('"result":"pivot_required"')
      expect(before.text).toContain('event: general_terminal_batch')
      expect(before.text).toContain('"kind":"turn_completed"')
      expect(before.text).toContain('"kind":"usage"')
      expect(before.text).toContain('"factAnswerAllowed":false')
      const persistedEvents = await readFile(join(durableRoot, 'threads', threadId, 'events.jsonl'), 'utf8')
      expect(persistedEvents).toContain('"kind":"turn_started"')
      expect(persistedEvents.endsWith('\n')).toBe(true)
      for (const forbidden of [
        'event: approval_requested',
        'event: user_input_requested',
        'event: mcp_lifecycle_audit',
        'event: goal_evidence_audit',
        'Ship',
        'Choose direction',
        'mcp__analytix_local',
        'local-fake-key',
        'D0244'
      ]) {
        expect(before.text).not.toContain(forbidden)
      }

      await killGoRuntimeServer(server)
      server = null

      const restarted = await startGoRuntimeServer(durableRoot, {
        runtimeServerBinary: runtimeBuild.executablePath,
        runtimeDurableRoot: true,
        providerBaseURL: provider.url
      })
      try {
        const thread = await goRuntimeServerJSON(restarted, `/v1/threads/${encodedThreadId}`)
        expect(thread.status).toBe(200)
        expect(typeof thread.body.latestSeq).toBe('number')
        expect(Number(thread.body.latestSeq)).toBeGreaterThanOrEqual(1)

        const replay = await goRuntimeServerText(restarted, `/v1/threads/${encodedThreadId}/events?since_seq=0`)
        expect(replay.text).toContain('event: turn_started')
        expect(replay.text).toContain('event: autoresearch_state_audit')
        expect(replay.text).not.toContain('"stateRelativePath"')
        expect(replay.text).toContain('"stablePrefixContainsState":false')
        expect(replay.text).toContain('"topLevelAutoResearchRouteExposed":false')
        expect(replay.text).toContain('event: general_terminal_batch')
        expect(replay.text).toContain('"kind":"usage"')
        expect(replay.text).toContain('"kind":"turn_completed"')
        expect(replay.text).toContain('"factAnswerAllowed":false')
        for (const forbidden of [
          'event: approval_requested',
          'event: user_input_requested',
          'event: approval_resolved',
          'event: user_input_resolved',
          'event: mcp_lifecycle_audit',
          'event: goal_evidence_audit',
          'Ship',
          'Choose direction',
          'mcp__analytix_local',
          'local-fake-key',
          'D0244'
        ]) {
          expect(replay.text).not.toContain(forbidden)
        }

        const secondTurn = await goRuntimeServerJSON(restarted, `/v1/threads/${encodedThreadId}/turns`, {
          method: 'POST',
          body: JSON.stringify({ prompt: 'Continue after runtime restart.', model: 'deepseek-chat' })
        })
        expect(secondTurn.status).toBe(202)
        expect(secondTurn.body.turnId).toBe('turn_2')
      } finally {
        await stopGoRuntimeServer(restarted)
      }

      const adapterSource = repoSource('src/main/runtime/analytix-adapter.ts')
      expect(adapterSource).not.toContain('ensureGoBackendOrRollback')
      expect(adapterSource).toContain('TypeScript runtime fallback is retired')
      expect(adapterSource).toContain('go-runtime-candidate-g6-readiness')
      expect(adapterSource).toContain('ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS')
      expect(adapterSource).toContain('ANALYTIX_RUNTIME_MCP_MATRIX_STATUS')
    } finally {
      try {
        if (server) await stopGoRuntimeServer(server)
      } finally {
        try {
          if (provider !== null) await provider.close()
        } finally {
          try {
            if (registryFixture !== null) registryFixture.dispose()
          } finally {
            try {
              if (runtimeBuild !== null) await runtimeBuild.cleanup()
            } finally {
              await rm(durableRoot, { recursive: true, force: true })
            }
          }
        }
      }
    }
  }, 300_000)

  it('maps every Batch 5 kernel component to TypeScript contract evidence', () => {
    const manifest = buildGoRuntimeKernelConformanceManifest()
    const contract = loadGoG1ShadowContract()

    expect(manifest).toMatchObject({
      schemaVersion: 1,
      mode: 'shadow-conformance-only',
      productBoundary: contract.productBoundary
    })
    expect(manifest.components.map((item) => item.component).sort()).toEqual(
      [...GO_RUNTIME_KERNEL_COMPONENTS].sort()
    )
    expect(new Set(manifest.components.map((item) => item.component)).size)
      .toBe(GO_RUNTIME_KERNEL_COMPONENTS.length)
    for (const component of manifest.components) {
      expect(component.goStages.length).toBeGreaterThan(0)
      expect(component.tsAuthority.length).toBeGreaterThan(0)
      expect(component.tsContractTests.length).toBeGreaterThan(0)
      expect(component.contractRequiredBeforeGo.length).toBeGreaterThan(0)
      expect(component.status).not.toBe('go-pending')
    }
    expect(manifest.g5ContractInventory).toEqual([
      {
        id: 'go-g5-full-loop-contract-v1',
        stage: 'G5',
        fixture: 'packages/runtime/src/conformance/fixtures/go-g5-full-loop-contract.json',
        tsOwned: true,
        shadowOnly: true
      }
    ])
  })

  it('keeps G5 full-loop inventory TS-owned and shadow-only', () => {
    const manifest = buildGoRuntimeKernelConformanceManifest()
    const contract = loadGoG5FullLoopContract()
    const jobManager = manifest.components.find((component) => component.component === 'Job Manager')
    const provider = manifest.components.find((component) => component.component === 'Provider Registry')
    const mcp = manifest.components.find((component) => component.component === 'MCP Client')

    expect(contract.productBoundary).toEqual(manifest.productBoundary)
    expect(contract.expectedOutput).toEqual({
      stage: 'G5',
      mode: 'shadow-conformance-only',
      sourceContractIds: contract.sourceContractIds,
      tsOwnedContractInventory: true,
      electronMainConnected: false,
      defaultGoBackendEnabled: false,
      fullLoopTests: contract.contractInventory.fullLoop,
      jobManager: {
        tests: contract.contractInventory.jobManager.tests,
        routes: contract.contractInventory.jobManager.routeContract,
        requiredBehaviors: contract.contractInventory.jobManager.requiredBehaviors
      },
      cacheCompactionTests: contract.contractInventory.cacheCompaction,
      resumeInterruptTests: contract.contractInventory.resumeInterrupt,
      mcpIndexerTests: contract.contractInventory.mcpIndexer,
      remainingBlockers: contract.remainingBlockers,
      productBoundary: manifest.productBoundary
    })
    expect(contract.contractInventory.jobManager.routeContract).toEqual([
      '/v1/runtime/task-jobs/wait',
      '/v1/runtime/task-jobs/output',
      '/v1/runtime/task-jobs/kill'
    ])
    expect(contract.contractInventory.jobManager.tests).toContain(
      'packages/runtime/tests/task-job-orchestration-contract.test.ts'
    )
    expect(jobManager?.tsAuthority).toEqual(expect.arrayContaining([
      'packages/runtime/src/delegation-test-support/job-manager.ts',
      'packages/runtime/src/server-test-support/routes/task-jobs.ts'
    ]))
    expect(jobManager?.contractRequiredBeforeGo.join('\n')).toContain(
      'first-class task/parallel/background/planner contract'
    )
    expect(provider?.contractRequiredBeforeGo.join('\n')).toContain('offline cache curve guard')
    expect(mcp?.contractRequiredBeforeGo.join('\n')).toContain('retry-all failed startup servers')
    expect(contract.remainingBlockers).toEqual(expect.arrayContaining([
      'No Go runtime replaces analytix serve.',
      'Renderer must not observe Go-specific routes or Reasonix public protocol.'
    ]))
  })

  it('ties G5 shadow slices to task, cache, session, and MCP source fixtures', async () => {
    const g5 = loadGoG5FullLoopContract()
    const taskJob = loadTaskJobOrchestrationContract()
    const providerCache = loadProviderCacheContract()
    const providerStreaming = loadGoG3ProviderContract()
    const g2 = loadGoG2RouteReplayContract()
    const approvalUserInput = loadApprovalUserInputRouteContract()
    const g4 = loadGoG4ToolsContract()
    const mcp = loadMcpToolLifecycleContract()
    const liveLocalMcp = runLiveLocalIndexerContract(mcp.liveLocalIndexer)

    expect(g5.shadowSlicesExpectedOutput).toEqual({
      stage: 'G5',
      mode: 'shadow-conformance-only',
      sourceContractIds: [
        taskJob.id,
        providerCache.id,
        providerStreaming.id,
        'go-g2-route-replay-contract-v1',
        approvalUserInput.id,
        mcp.id
      ],
      jobReplay: {
        taskToolName: taskJob.toolContracts.task.name,
        parallelToolName: taskJob.toolContracts.parallelTasks.name,
        toolContractBoundary: {
          task: {
            name: taskJob.toolContracts.task.name,
            promptField: taskJob.toolContracts.task.promptField,
            backgroundField: taskJob.toolContracts.task.backgroundField,
            continueField: taskJob.toolContracts.task.continueField,
            forkField: taskJob.toolContracts.task.forkField,
            internalRuntimeOnly: taskJob.toolContracts.task.internalRuntimeOnly,
            requiresPermissionGate: taskJob.toolContracts.task.requiresPermissionGate,
            mayAppendParentGoalEvidence: taskJob.toolContracts.task.mayAppendParentGoalEvidence
          },
          parallelTasks: {
            name: taskJob.toolContracts.parallelTasks.name,
            tasksField: taskJob.toolContracts.parallelTasks.tasksField,
            dependencyField: taskJob.toolContracts.parallelTasks.dependencyField,
            internalRuntimeOnly: taskJob.toolContracts.parallelTasks.internalRuntimeOnly,
            requiresDependencyValidation: taskJob.toolContracts.parallelTasks.requiresDependencyValidation,
            requiresPlannerReadOnlyToolset: taskJob.toolContracts.parallelTasks.requiresPlannerReadOnlyToolset
          },
          usesReasonixProtocol: false,
          topLevelRouteExposed: false
        },
        routes: [
          taskJob.routeContract.wait,
          taskJob.routeContract.output,
          taskJob.routeContract.kill
        ],
        routeBoundary: {
          protectedRoutes: taskJob.routeContract.authMatrix.protectedRoutes,
          unauthorizedStatus: taskJob.routeContract.authMatrix.unauthorizedStatus,
          forbiddenTopLevelRoutes: taskJob.routeContract.forbiddenTopLevelRoutes
        },
        routeExecutable: {
          unauthorizedStatus: taskJob.routeExecutable.unauthorizedStatus,
          outputStatus: taskJob.routeExecutable.output.status,
          outputJobStatus: taskJob.routeExecutable.output.jobStatus,
          outputNextOffset: taskJob.routeExecutable.output.nextOffset,
          outputReplayOffset: taskJob.routeExecutable.output.replayOffset,
          waitStatus: taskJob.routeExecutable.wait.status,
          waitJobStatus: taskJob.routeExecutable.wait.jobStatus,
          killStatus: taskJob.routeExecutable.kill.status,
          killJobStatus: taskJob.routeExecutable.kill.jobStatus,
          missingOutputStatus: taskJob.routeExecutable.missingOutput.status,
          rehydratedOutputStatus: taskJob.routeExecutable.rehydrated.outputStatus,
          rehydratedWaitStatus: taskJob.routeExecutable.rehydrated.waitStatus,
          rehydratedKillStatus: taskJob.routeExecutable.rehydrated.killStatus,
          rehydratedCompletedStatus: taskJob.routeExecutable.rehydrated.completedStatus,
          rehydratedCompletedNextOffset: taskJob.routeExecutable.rehydrated.completedNextOffset,
          rehydratedKilledStatus: taskJob.routeExecutable.rehydrated.killedStatus
        },
        lifecycle: {
          foreground: {
            kind: taskJob.foreground.kind,
            status: taskJob.foreground.status,
            result: taskJob.foreground.result
          },
          background: {
            kind: taskJob.background.kind,
            statusAcrossTurn: taskJob.background.statusAcrossTurn,
            output: taskJob.background.output,
            finalStatus: taskJob.background.finalStatus,
            finalResult: taskJob.background.finalResult
          },
          waitOutputKill: {
            killedStatus: taskJob.waitOutputKill.killedStatus,
            killedError: taskJob.waitOutputKill.killedError
          }
        },
        dependencyOrder: taskJob.parallel.validOrder,
        plannerReadOnlyToolset: taskJob.permissions.plannerReadOnlyToolset,
        plannerForbiddenToolset: taskJob.permissions.plannerForbiddenToolset,
        nestedSseMetadataFields: taskJob.nestedEvent.nestedSseMetadataFields,
        parentGoalEvidence: {
          requiresActiveGoal: taskJob.parentGoalEvidence.requiresActiveGoal,
          evidenceLedgeredEventKey: taskJob.parentGoalEvidence.evidenceLedgeredEventKey,
          evidenceLedgerErrorEventKey: taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey,
          usesReasonixProtocol: false,
          topLevelRouteExposed: false
        },
        sameTranscriptIdentityRequired: taskJob.transcript.sameIdentityRequired,
        continuePreservesTarget: taskJob.transcript.continuePreservesTarget,
        forkCreatesDistinctTarget: taskJob.transcript.forkCreatesDistinctTarget,
        plannerExecutor: {
          plannerKind: taskJob.plannerExecutor.plannerKind,
          plannerPolicy: taskJob.plannerExecutor.plannerPolicy,
          executorPolicy: taskJob.plannerExecutor.executorPolicy,
          failureStatus: taskJob.plannerExecutor.failureStatus,
          cancelledStatus: taskJob.plannerExecutor.cancelledStatus,
          skippedReason: taskJob.plannerExecutor.skippedReason,
          cancelReason: taskJob.plannerExecutor.cancelReason,
          outputOffsetJobCount: taskJob.plannerExecutor.outputOffsetJobCount,
          requiresFailurePropagation: taskJob.plannerExecutor.requiresFailurePropagation,
          requiresCancellationPropagation: taskJob.plannerExecutor.requiresCancellationPropagation,
          requiresTranscriptPropagation: taskJob.plannerExecutor.requiresTranscriptPropagation,
          transcriptPropagationMode: taskJob.plannerExecutor.transcriptPropagationMode,
          transcriptPropagationJobCount: taskJob.plannerExecutor.transcriptPropagationJobCount
        },
        durableRunnerRestart: {
          runningJobId: taskJob.durableRunner.restartDrill.runningJobId,
          queuedJobId: taskJob.durableRunner.restartDrill.queuedJobId,
          rehydratedCount: taskJob.durableRunner.restartDrill.rehydratedCount,
          waitStatus: taskJob.durableRunner.restartDrill.waitStatus,
          killStatus: taskJob.durableRunner.restartDrill.killStatus,
          outputAfterRestart: taskJob.durableRunner.restartDrill.outputAfterRestart
        },
        approvalDenyNoExecute: {
          deniedToolNames: taskJob.approvalDenyNoExecute.deniedToolNames,
          approvalIds: taskJob.approvalDenyNoExecute.approvalIds,
          createsDurableJobs: taskJob.approvalDenyNoExecute.createsDurableJobs,
          createsChildRuns: taskJob.approvalDenyNoExecute.createsChildRuns
        },
        parallelValidation: {
          dependencyField: taskJob.parallel.dependencyField,
          validOrder: taskJob.parallel.validOrder,
          invalidCaseIds: [
            'single_task',
            'duplicate_id',
            'self_dependency',
            'cycle',
            'unknown_dependency'
          ],
          singleTaskError: taskJob.parallel.singleTaskError,
          duplicateIdError: taskJob.parallel.duplicateIdError,
          selfDependencyError: taskJob.parallel.selfDependencyError,
          cycleError: taskJob.parallel.cycleError,
          unknownDependencyError: taskJob.parallel.unknownDependencyError
        }
      },
      cacheReplay: {
        stablePrefixHash: providerCache.stablePrefix.firstShape.prefixHash,
        toolsHash: providerCache.stablePrefix.firstShape.toolsHash,
        providerUsageCaseIds: providerCache.providerUsageCases.map((item) => item.id),
        requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
        requestShapeReplay: providerRequestShapeSummary(providerCache),
        releaseGuardStatuses: providerCache.releaseGuard.cases.map((item) => item.expectedStatus),
        releaseGuard: providerReleaseGuard(providerCache),
        cacheAccounting: providerCacheAccounting(providerCache),
        usageParserReplay: providerUsageParserReplay(providerCache),
        liveLocalHttpContract: providerLiveLocalHttpContractSummary(providerCache),
        offlineParitySeal: providerOfflineParitySeal(providerCache),
        providerCachePrivacy: providerCachePrivacy(providerCache),
        driftAttribution: providerDriftAttribution(providerCache),
        forbiddenDiagnosticsSubstrings: providerCache.privacy.forbiddenDiagnosticsSubstrings,
        liveSuperiorityClaimAllowed: providerCache.liveCredentialPolicy.mayClaimSuperiority
      },
      providerStreamingReplay: g3ProviderStreamingReplay(providerStreaming),
      sessionReplay: {
        routeIds: g2.routes.map((route) => route.id),
        resumeRouteIds: g2.routes.filter((route) => route.id.includes('resume')).map((route) => route.id),
        forkRouteIds: g2.routes.filter((route) => route.id.includes('fork')).map((route) => route.id),
        sseRouteIds: g2.routes.filter((route) => route.responseKind === 'sse').map((route) => route.id),
        routeStatusReplay: g2RouteStatusReplay(g2.routes)
      },
      approvalUserInputReplay: approvalUserInputReplayOutput(approvalUserInput),
      controlReplay: g5.controlReplay,
      mcpReplay: {
        providerId: mcp.providerId,
        lifecycle: mcpCoreLifecycleOutput(mcp),
        retryFailedServerIds: mcp.backgroundReconnect.failedServerIds,
        expectedConnectedServerIds: mcp.backgroundReconnect.expectedConnectedServerIds,
        backgroundReconnect: mcpBackgroundReconnectOutput(mcp),
        knownOverride: mcp.knownOverride.diagnostic.knownOverride,
        knownOverrideDiagnostics: mcp.knownOverrideVariants.map((variant) => ({
          serverId: variant.serverId,
          knownOverride: variant.diagnostic.knownOverride,
          effectiveCwd: variant.diagnostic.effectiveCwd,
          lowPriority: variant.diagnostic.lowPriority,
          backgroundStart: variant.diagnostic.backgroundStart,
          workspaceRoot: variant.workspaceRoot,
          ...(variant.explicitCwd ? { explicitCwd: variant.explicitCwd } : {}),
          ...(variant.daemonIdleTimeoutMs ? { daemonIdleTimeoutMs: variant.daemonIdleTimeoutMs } : {})
        })),
        liveLocalActivePaths: mcp.liveLocalIndexer.expectedOutput.activePaths,
        lateTombstonePath: mcp.liveLocalIndexer.lateTombstonePath,
        secretSafeDiagnostic: mcp.liveLocalIndexer.expectedOutput.secretSafeDiagnostic,
        liveLocalIndexer: {
          serverId: mcp.liveLocalIndexer.expectedOutput.serverId,
          cwd: mcp.liveLocalIndexer.expectedOutput.cwd,
          lowPriority: mcp.liveLocalIndexer.expectedOutput.lowPriority,
          backgroundStart: mcp.liveLocalIndexer.expectedOutput.backgroundStart,
          retryAttempts: mcp.liveLocalIndexer.expectedOutput.retryAttempts,
          activePaths: liveLocalMcp.activePaths,
          tombstoneCount: liveLocalMcp.tombstoneCount,
          restartedFromSnapshot: liveLocalMcp.restartedFromSnapshot,
          lateTombstonePath: mcp.liveLocalIndexer.lateTombstonePath,
          secretSafeDiagnostic: liveLocalMcp.secretSafeDiagnostic,
          leaksSecret: liveLocalMcp.secretSafeDiagnostic.includes(mcp.liveLocalIndexer.secretDiagnostic),
          topLevelRouteExposed: false
        },
        approvalAnnotations: mcpApprovalAnnotationOutput(mcp),
        searchMetaToolNames: mcp.searchMetaTools.toolNames,
        searchTrustedToolId: mcp.searchMetaTools.trustedToolId,
        searchUnknownToolError: mcp.searchMetaTools.unknownToolError,
        searchUntrustedSearchedTools: mcp.searchMetaTools.untrustedSearchedTools,
        searchCallPolicy: mcp.searchMetaTools.callPolicy,
        searchCallDeniedNoExecute: !mcp.searchMetaTools.deniedCallExecuted,
        searchRefreshDrift: mcpSearchRefreshDriftOutput(mcp),
        searchWorkspaceBoundary: mcpSearchWorkspaceBoundaryOutput(mcp)
      },
      productBoundary: g5.productBoundary
    })
    expect(g5.controlReplay).toMatchObject({
      cancel: {
        toolResultsPairedByCallId: true,
        preservesCompletedBatchResults: true,
        cancelledResultCode: 'tool_call_cancelled'
      },
      taskJobs: {
        parentSignalKillsRunningJobs: true,
        preservesOutputOffsets: true,
        staleRestartReconcileFailsQueuedAndRunning: true
      },
      approvalDeny: {
        deniedTaskToolsReturnApprovalItems: true,
        createsDurableJobs: false,
        createsChildRuns: false
      },
      userInput: {
        submittedAnswersEchoedByHttp: true,
        resolvedEventOmitsAnswers: true,
        cancelledResolutionStatus: 'cancelled',
        lateResolveRejected: true
      },
      abortCleanup: {
        approvalStatus: approvalUserInput.abortCleanup.expectedApprovalStatus,
        userInputStatus: approvalUserInput.abortCleanup.expectedUserInputStatus,
        lateApprovalDecisionStatus: approvalUserInput.abortCleanup.lateApprovalDecisionStatus,
        lateUserInputResolveStatus: approvalUserInput.abortCleanup.lateUserInputResolveStatus,
        pendingAfterCleanup: 0,
        replayKinds: approvalUserInput.abortCleanup.expectedReplayKinds,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      resumePendingGates: {
        sourceApprovalStatus: 'pending',
        sourceUserInputStatus: 'pending',
        resumedApprovalStatus: 'expired',
        resumedUserInputStatus: 'cancelled',
        answersCopiedToResume: false,
        topLevelRouteExposed: false
      },
      autoResearch: {
        directionTrackingRecorded: true,
        directionRequiresActiveResearchGoal: true,
        recordResearchDirectionToolPresent: true
      },
      mcpLifecycle: {
        retryAllFailedServers: true,
        tombstonesPersistAcrossRestart: true,
        diagnosticsRedacted: true,
        searchMetaToolsAdvertised: true,
        searchUntrustedWorkspaceHidden: true,
        searchCallDeniedNoExecute: true,
        topLevelRouteExposed: false
      },
      checkpointRewind: {
        checkpointIdPrefix: 'axcp_',
        planIdPrefix: 'axrp_',
        applyIdPrefix: 'axra_',
        rescueIdPrefix: 'axrr_',
        pathEscapeBlocked: true,
        conversationAuditAppendOnly: true,
        requiresConfirmationPhrase: true,
        topLevelRouteExposed: false
      },
      remoteEntry: {
        exposesOnlyLifecycleTurnsApprovals: true,
        forbiddenControlPlanesOmitted: true,
        rejectsPolicyOverrides: true,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      stepLimits: {
        defaultMaxModelSteps: 64,
        userGlobalOverride: true,
        sessionOverride: true,
        turnOverride: true,
        plannerOverride: true,
        headlessOverride: true,
        dynamicLimitInStablePrefix: false,
        errorCode: 'turn_step_limit_exceeded'
      },
      planner: {
        readOnlyPlusCreatePlan: true,
        forbiddenTaskToolsRejected: true,
        rejectionCode: 'tool_dispatch_rejected',
        createPlanToolName: 'create_plan'
      },
      combined: {
        autoRouteCacheReusedUntilStepLimit: true,
        cancelledResultsPreservedWithStepCache: true,
        dynamicControlStateInStablePrefix: false,
        stepLimitErrorCode: 'turn_step_limit_exceeded',
        cancelledResultCode: 'tool_call_cancelled'
      }
    })
    expect(g5.controlExecutableCases.productBoundary).toEqual(
      g5ProductBoundaryControlCase(g5)
    )
    expect(g5.controlExecutableCases.packageRuntimeIdentity).toEqual(
      packageRuntimeIdentityControlCase()
    )
    expect(g5.controlExecutableCases.runtimeHttpRouteSovereignty).toEqual(
      runtimeHttpRouteSovereigntyControlCase()
    )
    expect(g5.controlExecutableCases.desktopSovereignty).toEqual(
      desktopSovereigntyControlCase()
    )
    expect(g5.controlExecutableCases.desktopMainIpcBoundary).toEqual(
      desktopMainIpcBoundaryControlCase()
    )
    expect(g5.controlExecutableCases.rendererRouteSurfaceSovereignty).toEqual(
      rendererRouteSurfaceSovereigntyControlCase()
    )
    expect(g5.controlExecutableCases.goalPersistenceOffLock).toEqual(
      goalPersistenceOffLockControlCase()
    )
    expect(g5.controlExecutableCases.toolResultFileImageBoundary).toEqual(
      toolResultFileImageBoundaryControlCase()
    )
    expect(g5.controlExecutableCases.eventJsonlReplayBoundary).toEqual(
      eventJsonlReplayBoundaryControlCase()
    )
    expect(g5.controlExecutableCases.mcpMalformedSchemaBoundary).toEqual(
      mcpMalformedSchemaBoundaryControlCase()
    )
    expect(g5.controlExecutableCases.cancel.cancelledResultCode)
      .toBe(g5.controlReplay.cancel.cancelledResultCode)
    expect(g5.controlExecutableCases.cancel.scheduledAfterCancel).toBe(0)
    expect(g5.controlExecutableCases.taskJobs.skippedUnstartedReason)
      .toBe(g5.controlReplay.taskJobs.skippedUnstartedReason)
    expect(g5.controlExecutableCases.taskJobs.staleReconcile).toEqual({
      jobs: [
        {
          id: taskJob.durableRunner.staleReconcile.runningJobId,
          state: 'running',
          output: '',
          offset: 0
        },
        {
          id: taskJob.durableRunner.staleReconcile.queuedJobId,
          state: 'queued',
          output: '',
          offset: 0
        }
      ],
      reason: taskJob.durableRunner.staleReconcile.reason,
      expectedJobs: [
        {
          id: taskJob.durableRunner.staleReconcile.runningJobId,
          status: taskJob.durableRunner.staleReconcile.expectedStatus,
          output: '',
          offset: 0,
          reason: taskJob.durableRunner.staleReconcile.reason
        },
        {
          id: taskJob.durableRunner.staleReconcile.queuedJobId,
          status: taskJob.durableRunner.staleReconcile.expectedStatus,
          output: '',
          offset: 0,
          reason: taskJob.durableRunner.staleReconcile.reason
        }
      ]
    })
    expect(g5.controlExecutableCases.taskJobs.toolContractBoundary).toEqual(
      taskJobToolContractBoundaryControlCase(taskJob)
    )
    expect(g5.controlExecutableCases.taskJobs.transcriptIdentity).toEqual(
      taskJobTranscriptIdentityControlCase(taskJob)
    )
    expect(g5.controlExecutableCases.taskJobs.nestedSseMetadata).toEqual(
      taskJobNestedSseMetadataControlCase(taskJob)
    )
    expect(g5.controlExecutableCases.taskJobs.plannerToolsetInventory).toEqual(
      taskJobPlannerToolsetInventoryControlCase(taskJob)
    )
    expect(g5.controlExecutableCases.taskJobs.restartDrill).toEqual({
      jobs: [
        {
          id: taskJob.durableRunner.restartDrill.runningJobId,
          state: 'running',
          output: taskJob.durableRunner.restartDrill.outputBeforeRestart,
          offset: 1
        },
        {
          id: taskJob.durableRunner.restartDrill.queuedJobId,
          state: 'queued',
          output: '',
          offset: 0
        }
      ],
      outputAfterRestart: taskJob.durableRunner.restartDrill.outputAfterRestart,
      waitStatus: taskJob.durableRunner.restartDrill.waitStatus,
      killStatus: taskJob.durableRunner.restartDrill.killStatus,
      killError: taskJob.durableRunner.restartDrill.killError,
      expected: {
        rehydratedCount: taskJob.durableRunner.restartDrill.rehydratedCount,
        completed: {
          id: taskJob.durableRunner.restartDrill.runningJobId,
          status: taskJob.durableRunner.restartDrill.waitStatus,
          output: `${taskJob.durableRunner.restartDrill.outputBeforeRestart}${taskJob.durableRunner.restartDrill.outputAfterRestart}`,
          nextOffset: 2
        },
        killed: {
          id: taskJob.durableRunner.restartDrill.queuedJobId,
          status: taskJob.durableRunner.restartDrill.killStatus,
          error: taskJob.durableRunner.restartDrill.killError
        }
      }
    })
    expect(g5.controlExecutableCases.taskJobs.routeExecutable).toEqual({
      unauthorizedStatus: taskJob.routeExecutable.unauthorizedStatus,
      output: taskJob.routeExecutable.output,
      wait: taskJob.routeExecutable.wait,
      kill: taskJob.routeExecutable.kill,
      missingOutput: taskJob.routeExecutable.missingOutput,
      rehydrated: taskJob.routeExecutable.rehydrated,
      productBoundary: {
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      expected: {
        unauthorizedStatus: taskJob.routeExecutable.unauthorizedStatus,
        outputStatus: taskJob.routeExecutable.output.status,
        outputJobStatus: taskJob.routeExecutable.output.jobStatus,
        outputNextOffset: taskJob.routeExecutable.output.nextOffset,
        outputReplayOffset: taskJob.routeExecutable.output.replayOffset,
        waitStatus: taskJob.routeExecutable.wait.status,
        waitJobStatus: taskJob.routeExecutable.wait.jobStatus,
        killStatus: taskJob.routeExecutable.kill.status,
        killJobStatus: taskJob.routeExecutable.kill.jobStatus,
        missingOutputStatus: taskJob.routeExecutable.missingOutput.status,
        rehydratedOutputStatus: taskJob.routeExecutable.rehydrated.outputStatus,
        rehydratedWaitStatus: taskJob.routeExecutable.rehydrated.waitStatus,
        rehydratedKillStatus: taskJob.routeExecutable.rehydrated.killStatus,
        rehydratedCompletedStatus: taskJob.routeExecutable.rehydrated.completedStatus,
        rehydratedCompletedNextOffset: taskJob.routeExecutable.rehydrated.completedNextOffset,
        rehydratedKilledStatus: taskJob.routeExecutable.rehydrated.killedStatus,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.taskJobs.lifecycle).toEqual({
      foreground: taskJob.foreground,
      background: taskJob.background,
      waitOutputKill: taskJob.waitOutputKill,
      productBoundary: {
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      expected: {
        foregroundKind: taskJob.foreground.kind,
        foregroundStatus: taskJob.foreground.status,
        foregroundResult: taskJob.foreground.result,
        backgroundKind: taskJob.background.kind,
        backgroundStatusAcrossTurn: taskJob.background.statusAcrossTurn,
        backgroundOutput: taskJob.background.output,
        backgroundFinalStatus: taskJob.background.finalStatus,
        backgroundFinalResult: taskJob.background.finalResult,
        waitOutputKillStatus: taskJob.waitOutputKill.killedStatus,
        waitOutputKillError: taskJob.waitOutputKill.killedError,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.taskJobs.plannerExecutor).toEqual({
      plannerKind: taskJob.plannerExecutor.plannerKind,
      plannerPolicy: taskJob.plannerExecutor.plannerPolicy,
      executorPolicy: taskJob.plannerExecutor.executorPolicy,
      failureStatus: taskJob.plannerExecutor.failureStatus,
      cancelledStatus: taskJob.plannerExecutor.cancelledStatus,
      skippedReason: taskJob.plannerExecutor.skippedReason,
      cancelReason: taskJob.plannerExecutor.cancelReason,
      outputOffsetJobCount: taskJob.plannerExecutor.outputOffsetJobCount,
      requiresFailurePropagation: taskJob.plannerExecutor.requiresFailurePropagation,
      requiresCancellationPropagation: taskJob.plannerExecutor.requiresCancellationPropagation,
      requiresTranscriptPropagation: taskJob.plannerExecutor.requiresTranscriptPropagation,
      transcriptPropagationMode: taskJob.plannerExecutor.transcriptPropagationMode,
      transcriptPropagationJobCount: taskJob.plannerExecutor.transcriptPropagationJobCount,
      productBoundary: {
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      expected: {
        plannerKind: taskJob.plannerExecutor.plannerKind,
        plannerPolicy: taskJob.plannerExecutor.plannerPolicy,
        executorPolicy: taskJob.plannerExecutor.executorPolicy,
        failureStatus: taskJob.plannerExecutor.failureStatus,
        cancelledStatus: taskJob.plannerExecutor.cancelledStatus,
        skippedReason: taskJob.plannerExecutor.skippedReason,
        cancelReason: taskJob.plannerExecutor.cancelReason,
        outputOffsetJobCount: taskJob.plannerExecutor.outputOffsetJobCount,
        failurePropagates: true,
        cancellationPropagates: true,
        transcriptPropagates: true,
        transcriptPropagationMode: taskJob.plannerExecutor.transcriptPropagationMode,
        transcriptPropagationJobCount: taskJob.plannerExecutor.transcriptPropagationJobCount,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.taskJobs.parentGoalEvidence).toEqual({
      requiresActiveGoal: taskJob.parentGoalEvidence.requiresActiveGoal,
      ledgeredEventKey: taskJob.parentGoalEvidence.evidenceLedgeredEventKey,
      ledgerErrorEventKey: taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey,
      activeGoalEventMetadata: {
        [taskJob.parentGoalEvidence.evidenceLedgeredEventKey]: true,
        [taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey]: ''
      },
      missingGoalEventMetadata: {
        [taskJob.parentGoalEvidence.evidenceLedgeredEventKey]: false,
        [taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey]: 'missing active goal'
      },
      productBoundary: {
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      expected: {
        requiresActiveGoal: taskJob.parentGoalEvidence.requiresActiveGoal,
        ledgeredEventKey: taskJob.parentGoalEvidence.evidenceLedgeredEventKey,
        ledgerErrorEventKey: taskJob.parentGoalEvidence.evidenceLedgerErrorEventKey,
        ledgeredWhenActiveGoal: true,
        errorsWithoutActiveGoal: true,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.taskJobs.parallelValidation).toEqual({
      dependencyField: taskJob.parallel.dependencyField,
      validPlan: [
        { id: 'a' },
        { id: 'b', depends_on: ['a'] },
        { id: 'c', depends_on: ['b'] }
      ],
      invalidPlans: [
        {
          caseId: 'single_task',
          tasks: [{ id: 'a' }]
        },
        {
          caseId: 'duplicate_id',
          tasks: [{ id: 'a' }, { id: 'a' }]
        },
        {
          caseId: 'self_dependency',
          tasks: [{ id: 'a', depends_on: ['a'] }, { id: 'b' }]
        },
        {
          caseId: 'cycle',
          tasks: [
            { id: 'a', depends_on: ['c'] },
            { id: 'b', depends_on: ['a'] },
            { id: 'c', depends_on: ['b'] }
          ]
        },
        {
          caseId: 'unknown_dependency',
          tasks: [{ id: 'a', depends_on: ['missing'] }, { id: 'b' }]
        }
      ],
      productBoundary: {
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      },
      expected: {
        dependencyField: taskJob.parallel.dependencyField,
        validOrder: taskJob.parallel.validOrder,
        invalidResults: [
          {
            caseId: 'single_task',
            error: taskJob.parallel.singleTaskError
          },
          {
            caseId: 'duplicate_id',
            error: `${taskJob.parallel.duplicateIdError}: a`
          },
          {
            caseId: 'self_dependency',
            error: `${taskJob.parallel.selfDependencyError}: a`
          },
          {
            caseId: 'cycle',
            error: `${taskJob.parallel.cycleError} includes: a`
          },
          {
            caseId: 'unknown_dependency',
            error: `${taskJob.parallel.unknownDependencyError}: missing`
          }
        ],
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.approvalDeny).toEqual({
      attemptedToolNames: taskJob.approvalDenyNoExecute.deniedToolNames,
      approvalIds: taskJob.approvalDenyNoExecute.approvalIds,
      expected: {
        deniedToolNames: taskJob.approvalDenyNoExecute.deniedToolNames,
        approvalIds: taskJob.approvalDenyNoExecute.approvalIds,
        approvalItemCount: taskJob.approvalDenyNoExecute.approvalIds.length,
        createsDurableJobs: taskJob.approvalDenyNoExecute.createsDurableJobs,
        createsChildRuns: taskJob.approvalDenyNoExecute.createsChildRuns
      }
    })
    expect(g5.controlExecutableCases.userInput).toEqual({
      submitted: {
        id: approvalUserInput.submittedUserInput.id,
        itemId: approvalUserInput.submittedUserInput.itemId,
        answers: approvalUserInput.submittedUserInput.expectedResolution.answers,
        resolvedEventKind: approvalUserInput.submittedUserInput.resolvedEvent.kind,
        resolvedEventIncludesAnswers: approvalUserInput.submittedUserInput.resolvedEvent.includesAnswers,
        pendingBefore: approvalUserInput.submittedUserInput.pendingBefore,
        pendingAfter: approvalUserInput.submittedUserInput.pendingAfter
      },
      cancelled: {
        id: approvalUserInput.userInput.id,
        status: approvalUserInput.userInput.expectedResponse.body?.status,
        secondResolveStatus: approvalUserInput.userInput.secondResolveStatus,
        pendingBefore: approvalUserInput.userInput.pendingBefore,
        pendingAfter: approvalUserInput.userInput.pendingAfter
      },
      structuredChoiceValidation: g4.userInput.structuredChoiceValidation,
      expected: {
        submittedInputId: approvalUserInput.submittedUserInput.id,
        submittedStatus: approvalUserInput.submittedUserInput.expectedResolution.status,
        answerCount: approvalUserInput.submittedUserInput.expectedResolution.answers.length,
        httpEchoesAnswers: approvalUserInput.submittedUserInput.expectedResponse.body?.answers !== undefined,
        resolvedEventKind: approvalUserInput.submittedUserInput.resolvedEvent.kind,
        resolvedEventIncludesAnswers: approvalUserInput.submittedUserInput.resolvedEvent.includesAnswers,
        cancelledInputId: approvalUserInput.userInput.id,
        cancelledStatus: approvalUserInput.userInput.expectedResponse.body?.status,
        secondResolveStatus: approvalUserInput.userInput.secondResolveStatus,
        pendingAfterSubmit: approvalUserInput.submittedUserInput.pendingAfter,
        pendingAfterCancel: approvalUserInput.userInput.pendingAfter,
        structuredChoiceValidation: {
          maxQuestions: g4.userInput.structuredChoiceValidation.maxQuestions,
          minOptionsWhenProvided: g4.userInput.structuredChoiceValidation.minOptionsWhenProvided,
          maxOptionsWhenProvided: g4.userInput.structuredChoiceValidation.maxOptionsWhenProvided,
          dedupeLabelsCaseInsensitive: g4.userInput.structuredChoiceValidation.dedupeLabelsCaseInsensitive,
          invalidResultCode: g4.userInput.structuredChoiceValidation.invalidResultCode,
          invalidCases: g4.userInput.structuredChoiceValidation.invalidCases,
          invalidCaseCount: g4.userInput.structuredChoiceValidation.invalidCases.length,
          rejectsTooManyQuestions: g4.userInput.structuredChoiceValidation.invalidCases.includes('too_many_questions'),
          rejectsSingleOption: g4.userInput.structuredChoiceValidation.invalidCases.includes('single_option'),
          rejectsTooManyOptions: g4.userInput.structuredChoiceValidation.invalidCases.includes('too_many_options'),
          rejectsDuplicateLabelsCaseInsensitive: g4.userInput.structuredChoiceValidation.invalidCases.includes('duplicate_labels_case_insensitive'),
          opensGateOnInvalid: g4.userInput.structuredChoiceValidation.opensGateOnInvalid,
          usesReasonixProtocol: false,
          topLevelRouteExposed: false
        }
      }
    })
    expect(g5.controlExecutableCases.approvalUserInputRouteReplay).toEqual(
      approvalUserInputRouteReplayControlCase(approvalUserInput)
    )
    expect(g5.controlExecutableCases.approvalUserInputInventory).toEqual(
      approvalUserInputInventoryControlCase(approvalUserInput)
    )
    expect(g5.controlExecutableCases.abortCleanup).toEqual({
      approvalId: approvalUserInput.abortCleanup.approvalId,
      userInputId: approvalUserInput.abortCleanup.userInputId,
      expectedApprovalStatus: approvalUserInput.abortCleanup.expectedApprovalStatus,
      expectedUserInputStatus: approvalUserInput.abortCleanup.expectedUserInputStatus,
      lateApprovalDecisionStatus: approvalUserInput.abortCleanup.lateApprovalDecisionStatus,
      lateUserInputResolveStatus: approvalUserInput.abortCleanup.lateUserInputResolveStatus,
      replayKinds: approvalUserInput.abortCleanup.expectedReplayKinds,
      expected: {
        approvalId: approvalUserInput.abortCleanup.approvalId,
        approvalStatus: approvalUserInput.abortCleanup.expectedApprovalStatus,
        userInputId: approvalUserInput.abortCleanup.userInputId,
        userInputStatus: approvalUserInput.abortCleanup.expectedUserInputStatus,
        lateApprovalDecisionStatus: approvalUserInput.abortCleanup.lateApprovalDecisionStatus,
        lateUserInputResolveStatus: approvalUserInput.abortCleanup.lateUserInputResolveStatus,
        pendingAfterCleanup: 0,
        replayKinds: approvalUserInput.abortCleanup.expectedReplayKinds,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.resumePendingGates).toEqual({
      sourceThreadId: approvalUserInput.resumePendingGates.sourceThreadId,
      approvalId: approvalUserInput.resumePendingGates.approvalId,
      userInputId: approvalUserInput.resumePendingGates.userInputId,
      sourceStatuses: approvalUserInput.resumePendingGates.expectedSourceStatuses,
      resumedStatuses: approvalUserInput.resumePendingGates.expectedResumedStatuses,
      expected: {
        sourceApprovalStatus: approvalUserInput.resumePendingGates.expectedSourceStatuses.approval,
        sourceUserInputStatus: approvalUserInput.resumePendingGates.expectedSourceStatuses.userInput,
        resumedApprovalStatus: approvalUserInput.resumePendingGates.expectedResumedStatuses.approval,
        resumedUserInputStatus: approvalUserInput.resumePendingGates.expectedResumedStatuses.userInput,
        pendingAfterResume: 0,
        answersCopiedToResume: false,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    const autoResearchStoreSource = repoSource('packages/runtime/src/research/autoresearch-store.ts')
    const goalToolsSource = repoSource('packages/runtime/src/tool-test-support/tool/goal-tools.ts')
    const threadServiceSource = repoSource('packages/runtime/src/services-test-support/thread-service.ts')
    const goalToolsTestSource = repoSource('packages/runtime/tests/goal-tools.test.ts')
    const autoResearchDirectionToolPresent = goalToolsSource.includes(
      "RECORD_RESEARCH_DIRECTION_TOOL_NAME = 'record_research_direction'"
    ) && goalToolsSource.includes('directions_tried.json and iteration_log.jsonl')
    const autoResearchRequiresActiveGoal = threadServiceSource.includes(
      'no active research goal'
    ) && goalToolsTestSource.includes(
      'rejects research direction records without an active research goal'
    )
    expect(g5.controlExecutableCases.autoResearch.expected).toEqual({
      stateRelativePath: g5.controlExecutableCases.autoResearch.expectedStateRelativePath,
      fileCount: g5.controlExecutableCases.autoResearch.expectedFiles.length,
      writesReasonixFile: false,
      writesAgentsFile: false,
      unknownRequirementAccepted: false,
      findingsWrittenForUnknownRequirement: false,
      directionTrackingFileWritten: autoResearchStoreSource.includes("kind: 'direction_recorded'") &&
        g5.controlExecutableCases.autoResearch.expectedFiles.includes('directions_tried.json'),
      iterationLogRecordsDirection: autoResearchStoreSource.includes('appendJsonl(paths.iterationLogAbsolutePath') &&
        g5.controlExecutableCases.autoResearch.expectedFiles.includes('iteration_log.jsonl'),
      recordResearchDirectionToolPresent: autoResearchDirectionToolPresent,
      recordDirectionRequiresActiveResearchGoal: autoResearchRequiresActiveGoal,
      stablePrefixContainsState: false,
      toolSchemaContainsState: false,
      topLevelRouteExposed: false
    })
    expect(g5.controlExecutableCases.autoResearch.recordDirectionToolName)
      .toBe('record_research_direction')
    expect(g5.controlExecutableCases.autoResearch.directionRequiresActiveResearchGoal)
      .toBe(autoResearchRequiresActiveGoal)
    const workspace = await mkdtemp(join(tmpdir(), 'analytix-g5-autoresearch-'))
    try {
      const store = new AutoResearchProjectStore({ nowIso: () => '2026-06-22T00:00:00.000Z' })
      const state = await store.createOrResume({
        workspace,
        threadId: g5.controlExecutableCases.autoResearch.threadId,
        objective: g5.controlExecutableCases.autoResearch.objective,
        requirements: g5.controlExecutableCases.autoResearch.requirements
      })

      expect(state.descriptor.stateRelativePath)
        .toBe(g5.controlExecutableCases.autoResearch.expectedStateRelativePath)
      for (const fileName of g5.controlExecutableCases.autoResearch.expectedFiles) {
        await expect(stat(join(workspace, state.descriptor.stateRelativePath, fileName))).resolves.toBeTruthy()
      }
      await expect(stat(join(workspace, 'REASONIX.md'))).rejects.toThrow()
      await expect(stat(join(workspace, 'AGENTS.md'))).rejects.toThrow()
      const direction = await store.recordDirection({
        workspace,
        threadId: g5.controlExecutableCases.autoResearch.threadId,
        direction: g5.controlExecutableCases.autoResearch.direction,
        outcome: g5.controlExecutableCases.autoResearch.directionOutcome,
        summary: g5.controlExecutableCases.autoResearch.directionSummary
      })
      expect(direction).toMatchObject({
        direction: g5.controlExecutableCases.autoResearch.direction,
        outcome: g5.controlExecutableCases.autoResearch.directionOutcome,
        summary: g5.controlExecutableCases.autoResearch.directionSummary
      })
      const directions = JSON.parse(
        await readFile(join(workspace, state.descriptor.directionsTriedPath), 'utf8')
      ) as { directions: Array<{ direction: string; outcome: string; summary?: string }> }
      expect(directions.directions).toEqual([
        expect.objectContaining({
          direction: g5.controlExecutableCases.autoResearch.direction,
          outcome: g5.controlExecutableCases.autoResearch.directionOutcome,
          summary: g5.controlExecutableCases.autoResearch.directionSummary
        })
      ])
      const iterationLog = await readFile(join(workspace, state.descriptor.iterationLogPath), 'utf8')
      expect(iterationLog).toContain('"kind":"direction_recorded"')
      expect(iterationLog).toContain(g5.controlExecutableCases.autoResearch.direction)
      await expect(store.recordEvidence({
        workspace,
        threadId: g5.controlExecutableCases.autoResearch.threadId,
        requirementId: g5.controlExecutableCases.autoResearch.unknownRequirementId,
        step: 'Wrong requirement',
        evidence: ['must not write']
      })).rejects.toThrow(/unknown research requirement/)
      await expect(readFile(join(workspace, state.descriptor.findingsPath), 'utf8')).resolves.toBe('')
    } finally {
      await rm(workspace, { recursive: true, force: true })
    }
    expect(g5.controlExecutableCases.mcpLifecycle).toEqual({
      providerId: mcp.providerId,
      failedServerIds: mcp.backgroundReconnect.failedServerIds,
      expectedConnectedServerIds: mcp.backgroundReconnect.expectedConnectedServerIds,
      expectedErrorServerIds: mcp.backgroundReconnect.expectedErrorServerIds,
      attemptsPerFailedServer: mcp.backgroundReconnect.attemptsPerFailedServer,
      requiresRuntimeRestart: mcp.backgroundReconnect.requiresRuntimeRestart,
      liveLocal: {
        serverId: mcp.liveLocalIndexer.serverId,
        cwd: mcp.liveLocalIndexer.cwd,
        lowPriority: mcp.liveLocalIndexer.lowPriority,
        backgroundStart: mcp.liveLocalIndexer.backgroundStart,
        initialPaths: mcp.liveLocalIndexer.initialFiles.map((file) => file.path),
        lateTombstonePath: mcp.liveLocalIndexer.lateTombstonePath,
        resumePaths: mcp.liveLocalIndexer.resumeFiles.map((file) => file.path),
        secretDiagnostic: mcp.liveLocalIndexer.secretDiagnostic,
        replacement: mcp.diagnosticsRedaction.replacement
      },
      expected: {
        providerId: mcp.providerId,
        retryAttempts: Object.fromEntries(mcp.backgroundReconnect.failedServerIds.map((serverId) => [
          serverId,
          mcp.backgroundReconnect.attemptsPerFailedServer
        ])),
        connectedServerIds: mcp.backgroundReconnect.expectedConnectedServerIds,
        errorServerIds: mcp.backgroundReconnect.expectedErrorServerIds,
        requiresRuntimeRestart: mcp.backgroundReconnect.requiresRuntimeRestart,
        activePaths: liveLocalMcp.activePaths,
        tombstoneCount: liveLocalMcp.tombstoneCount,
        restartedFromSnapshot: liveLocalMcp.restartedFromSnapshot,
        secretSafeDiagnostic: liveLocalMcp.secretSafeDiagnostic,
        leaksSecret: liveLocalMcp.secretSafeDiagnostic.includes(mcp.liveLocalIndexer.secretDiagnostic),
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.mcpCoreLifecycle).toEqual(
      mcpCoreLifecycleControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpBackgroundReconnect).toEqual(
      mcpBackgroundReconnectControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpCallReconnect).toEqual(
      mcpCallReconnectControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpKnownOverrideDiagnostics).toEqual(
      mcpKnownOverrideDiagnosticsControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpLiveLocalIndexer).toEqual(
      mcpLiveLocalIndexerControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpApprovalAnnotations).toEqual(
      mcpApprovalAnnotationControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpSearchMetaTools).toEqual(
      mcpSearchMetaToolsControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpSearchRefreshDrift).toEqual(
      mcpSearchRefreshDriftControlCase(mcp)
    )
    expect(g5.controlExecutableCases.mcpSearchWorkspaceBoundary).toEqual(
      mcpSearchWorkspaceBoundaryControlCase(mcp)
    )
    const checkpointRewind = g5.controlExecutableCases.checkpointRewind
    const checkpoint: CheckpointMetadata = {
      schemaVersion: 1,
      checkpointId: checkpointRewind.checkpointId,
      threadId: checkpointRewind.threadId,
      turnId: checkpointRewind.turnId,
      workspace: checkpointRewind.workspace,
      createdAt: checkpointRewind.createdAt,
      status: 'captured',
      changedFiles: checkpointRewind.changedFiles
    }
    const checkpointEvents: RuntimeEvent[] = [
      {
        kind: 'thread_created',
        seq: 1,
        timestamp: checkpointRewind.createdAt,
        threadId: checkpoint.threadId,
        title: 'G5 checkpoint rewind'
      },
      {
        kind: 'turn_started',
        seq: 2,
        timestamp: checkpointRewind.createdAt,
        threadId: checkpoint.threadId,
        turnId: 'turn-before-checkpoint'
      },
      {
        kind: 'turn_completed',
        seq: 3,
        timestamp: checkpointRewind.createdAt,
        threadId: checkpoint.threadId,
        turnId: 'turn-before-checkpoint',
        status: 'completed'
      },
      {
        kind: 'checkpoint_captured',
        seq: 4,
        timestamp: checkpointRewind.createdAt,
        threadId: checkpoint.threadId,
        turnId: checkpoint.turnId,
        checkpoint
      },
      {
        kind: 'turn_started',
        seq: 5,
        timestamp: checkpointRewind.createdAt,
        threadId: checkpoint.threadId,
        turnId: checkpoint.turnId
      },
      {
        kind: 'turn_completed',
        seq: 6,
        timestamp: checkpointRewind.createdAt,
        threadId: checkpoint.threadId,
        turnId: checkpoint.turnId,
        status: 'completed'
      }
    ]
    const checkpointPlan = buildAuditableCheckpointRewindPlan({
      events: checkpointEvents,
      checkpointId: checkpoint.checkpointId,
      scope: 'combined',
      createdAt: checkpointRewind.createdAt,
      pathRisks: new Map(checkpointRewind.pathRisks.map((risk) => [
        risk.relativePath,
        { exists: risk.exists, isSymlink: risk.isSymlink }
      ]))
    })
    expect(checkpointPlan.ok).toBe(true)
    if (!checkpointPlan.ok) return
    expect(checkpointPlan.plan.planId).toMatch(/^axrp_/)
    expect(checkpointRewind.applyId).toMatch(/^axra_/)
    expect(checkpointRewind.rescueId).toMatch(/^axrr_/)
    expect(checkpointRewind.expected).toEqual({
      checkpointIdPrefix: 'axcp_',
      planIdPrefix: 'axrp_',
      applyIdPrefix: 'axra_',
      rescueIdPrefix: 'axrr_',
      readyFileCount: checkpointPlan.plan.files.filter((file) => file.status === 'ready').length,
      blockedFileCount: checkpointPlan.plan.files.filter((file) => file.status === 'blocked').length,
      symlinkBlocked: checkpointPlan.plan.files.some((file) => file.relativePath === 'src/link.ts' && file.status === 'blocked'),
      pathEscapeBlocked: checkpointPlan.plan.files.some((file) => file.relativePath === '../outside.ts' && file.status === 'blocked') &&
        checkpointPlan.plan.files.some((file) => file.relativePath === '/tmp/outside.ts' && file.status === 'blocked'),
      legalDotDotFilenameReady: checkpointPlan.plan.files.some((file) => file.relativePath === '..name' && file.status === 'ready'),
      requiresExplicitConfirmation: checkpointRewind.confirmation.confirmed &&
        checkpointRewind.confirmation.destructive &&
        checkpointRewind.confirmation.phrase === 'APPLY_CHECKPOINT_REWIND',
      conversationAuditAppendOnly: checkpointPlan.plan.conversation?.status === 'ready',
      rewritesTranscript: false,
      usesGitRefs: false,
      eventKinds: checkpointRewind.eventKinds,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    expect(g5.controlExecutableCases.remoteEntry).toEqual({
      expectedPortKeys: approvalUserInput.remoteEntry.expectedPortKeys,
      forbiddenPortKeys: approvalUserInput.remoteEntry.forbiddenPortKeys,
      rejectedOverrideKeys: Object.keys(approvalUserInput.remoteEntry.rejectedOverride),
      expected: {
        exposedPortKeys: approvalUserInput.remoteEntry.expectedPortKeys,
        forbiddenPortKeysAbsent: true,
        rejectedOverrideAccepted: false,
        goalAccess: false,
        checkpointAccess: false,
        memoryAccess: false,
        storageAccess: false,
        toolHostAccess: false,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    const historyRepair = g5.controlExecutableCases.historyRepair
    const repairedHistory = repairModelHistoryItems(historyRepair.items as unknown as TurnItem[])
    const repairedIds = repairedHistory.map((item) => item.id)
    const repairedIdSet = new Set(repairedIds)
    const droppedIds = historyRepair.items
      .map((item) => item.id)
      .filter((id) => !repairedIdSet.has(id))
    expect(historyRepair.expected).toEqual({
      repairedIds,
      droppedIds,
      keptCallIds: repairedHistory
        .filter((item) => item.kind === 'tool_call')
        .map((item) => item.callId),
      keptResultCallIds: repairedHistory
        .filter((item) => item.kind === 'tool_result')
        .map((item) => item.callId),
      orphanResultDropped: droppedIds.includes('orphan_result'),
      missingResultCallDropped: droppedIds.includes('missing_call'),
      duplicateResultDropped: droppedIds.includes('result_b_duplicate'),
      bridgeTextPreserved: repairedIdSet.has('assistant_bridge') && repairedIdSet.has('assistant_text'),
      stablePrefixContainsRepairState: false,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    const compactionBoundary = g5.controlExecutableCases.compactionBoundary
    const effectiveHistory = effectiveHistoryAfterLatestCompaction(
      compactionBoundary.items as unknown as TurnItem[]
    )
    const effectiveIds = effectiveHistory.map((item) => item.id)
    const effectiveIdSet = new Set(effectiveIds)
    const compactionDroppedIds = compactionBoundary.items
      .map((item) => item.id)
      .filter((id) => !effectiveIdSet.has(id))
    expect(compactionBoundary.expected).toEqual({
      effectiveIds,
      droppedIds: compactionDroppedIds,
      latestCompactionId: effectiveIds[0],
      latestCompactionFirst: effectiveHistory[0]?.kind === 'compaction',
      latestCompactionPreserved: effectiveIdSet.has('latest_compaction'),
      olderCompactionDropped: compactionDroppedIds.includes('older_compaction'),
      noopCompactionDropped: compactionDroppedIds.includes('noop_compaction'),
      postCompactionUserPreserved: effectiveIdSet.has('post_user_continue'),
      preCompactionUserDropped: compactionDroppedIds.includes('pre_latest_user'),
      stablePrefixContainsCompactionState: false,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    const stepLimits = g5.controlExecutableCases.stepLimits
    const expectedStepLimitMatrix = [
      {
        scope: 'default',
        configured: stepLimits.defaultMaxModelSteps,
        fallback: 0,
        effective: stepLimits.defaultMaxModelSteps,
        source: 'default',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'userGlobal',
        configured: stepLimits.userGlobalMaxModelSteps,
        fallback: stepLimits.defaultMaxModelSteps,
        effective: stepLimits.userGlobalMaxModelSteps,
        source: 'user',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'session',
        configured: stepLimits.sessionMaxModelSteps,
        fallback: stepLimits.userGlobalMaxModelSteps,
        effective: stepLimits.sessionMaxModelSteps,
        source: 'session',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'turn',
        configured: stepLimits.turnMaxModelSteps,
        fallback: stepLimits.sessionMaxModelSteps,
        effective: stepLimits.turnMaxModelSteps,
        source: 'turn',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'planner',
        configured: stepLimits.plannerMaxModelSteps,
        fallback: stepLimits.userGlobalMaxModelSteps,
        effective: stepLimits.plannerMaxModelSteps,
        source: 'planner',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'headless',
        configured: stepLimits.headlessMaxModelSteps,
        fallback: stepLimits.userGlobalMaxModelSteps,
        effective: stepLimits.headlessMaxModelSteps,
        source: 'headless',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'zeroDefault',
        configured: stepLimits.zeroDefaultMaxModelSteps,
        fallback: stepLimits.defaultMaxModelSteps,
        effective: stepLimits.defaultMaxModelSteps,
        source: 'default',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'delegate',
        configured: stepLimits.delegateParentMaxModelSteps,
        fallback: 5,
        effective: 6,
        source: 'parent-half',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: false
      },
      {
        scope: 'delegateFloor',
        configured: stepLimits.delegateFloorParentMaxModelSteps,
        fallback: 5,
        effective: 5,
        source: 'parent-half',
        dynamicLimitInStablePrefix: false,
        disablesGuard: false,
        delegateMinFloorApplied: true
      }
    ]
    expect(stepLimits.expected).toEqual({
      default: g5.controlReplay.stepLimits.defaultMaxModelSteps,
      userGlobal: stepLimits.userGlobalMaxModelSteps,
      session: stepLimits.sessionMaxModelSteps,
      turn: stepLimits.turnMaxModelSteps,
      planner: stepLimits.plannerMaxModelSteps,
      headless: stepLimits.headlessMaxModelSteps,
      zeroDefault: stepLimits.defaultMaxModelSteps,
      delegateInherited: 6,
      delegateFloorInherited: 5,
      dynamicLimitInStablePrefix: g5.controlReplay.stepLimits.dynamicLimitInStablePrefix,
      errorCode: g5.controlReplay.stepLimits.errorCode,
      matrix: expectedStepLimitMatrix
    })
    const planner = g5.controlExecutableCases.planner
    const plannerGateTools = ['user_input', 'request_user_input']
    const plannerCapabilityAdvertised = planner.availableToolset.filter((toolName) =>
      toolName === planner.planToolName ||
      planner.readOnlyToolset.includes(toolName) ||
      plannerGateTools.includes(toolName)
    )
    const plannerStep0Advertised = resolvePlanModeToolSpecs(
      namedToolSpecs(plannerCapabilityAdvertised),
      {
        planTurnActive: true,
        createPlanSatisfied: false,
        stepIndex: 0
      }
    ).map((tool) => tool.name)
    const plannerStep1Advertised = resolvePlanModeToolSpecs(
      namedToolSpecs(plannerCapabilityAdvertised),
      {
        planTurnActive: true,
        createPlanSatisfied: false,
        stepIndex: 1
      }
    ).map((tool) => tool.name)
    const plannerRejectedCalls = planner.blockedToolset.map((toolName) => ({
      toolName,
      status: 'failed' as const,
      code: g5.controlReplay.planner.rejectionCode,
      executed: false as const
    }))
    expect(planner).toEqual({
      availableToolset: [
        ...taskJob.permissions.plannerReadOnlyToolset,
        'user_input',
        'request_user_input',
        ...taskJob.permissions.plannerForbiddenToolset,
        'bash',
        'edit',
        'write',
        'echo',
        'create_plan'
      ],
      readOnlyToolset: taskJob.permissions.plannerReadOnlyToolset,
      forbiddenToolset: taskJob.permissions.plannerForbiddenToolset,
      blockedToolset: [
        ...taskJob.permissions.plannerForbiddenToolset,
        'bash',
        'edit',
        'write',
        'echo'
      ],
      planToolName: 'create_plan',
      forgedToolName: taskJob.toolContracts.task.name,
      normalModeAdvertised: [
        ...taskJob.permissions.plannerReadOnlyToolset,
        ...taskJob.permissions.plannerForbiddenToolset,
        'bash',
        'edit',
        'write',
        'echo'
      ],
      expectedCapabilityAdvertised: plannerCapabilityAdvertised,
      expectedStep0Advertised: plannerStep0Advertised,
      expectedStep1Advertised: plannerStep1Advertised,
      expectedRejectedCall: plannerRejectedCalls[0],
      expectedRejectedCalls: plannerRejectedCalls,
      expected: {
        normalModeHidesPlanTool: !planner.normalModeAdvertised.includes(planner.planToolName),
        capabilityGateAdvertised: plannerCapabilityAdvertised,
        step0Advertised: plannerStep0Advertised,
        step1Advertised: plannerStep1Advertised,
        step0ExcludesBlockedTools: planner.blockedToolset.every((toolName) =>
          !plannerStep0Advertised.includes(toolName)
        ),
        step1OnlyCreatePlan: plannerStep1Advertised.length === 1 &&
          plannerStep1Advertised[0] === planner.planToolName,
        rejectedToolNames: planner.blockedToolset,
        rejectedCallCount: plannerRejectedCalls.length,
        allForgedCallsRejected: plannerRejectedCalls.every((call) =>
          call.status === 'failed' &&
          call.code === g5.controlReplay.planner.rejectionCode &&
          call.executed === false
        ),
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.autoRouterClassifier).toEqual({
      routerModel: AUTO_MODEL_ROUTER_MODEL,
      defaultTimeoutMs: AUTO_MODEL_ROUTER_TIMEOUT_MS,
      fingerprint: AUTO_MODEL_ROUTER_FINGERPRINT,
      isolatedRequest: {
        turnIdSuffix: '_auto_router',
        stream: false,
        maxTokens: 96,
        temperature: 0,
        responseFormat: 'json_object',
        reasoningEffort: 'off',
        prefixItemCount: 0,
        toolCount: 0,
        carriesContextInstructions: false
      },
      fingerprintDrift: {
        routerModelChangeInvalidates:
          buildAutoModelRouterFingerprint({ routerModel: 'deepseek-v4-pro' }) !== AUTO_MODEL_ROUTER_FINGERPRINT,
        systemPromptChangeInvalidates:
          buildAutoModelRouterFingerprint({ systemPrompt: 'different classifier prompt' }) !== AUTO_MODEL_ROUTER_FINGERPRINT,
        timeoutChangeInvalidates:
          buildAutoModelRouterFingerprint({ timeoutMs: 8_000 }) !== AUTO_MODEL_ROUTER_FINGERPRINT,
        maxTokensChangeInvalidates:
          buildAutoModelRouterFingerprint({ maxTokens: 128 }) !== AUTO_MODEL_ROUTER_FINGERPRINT,
        temperatureChangeInvalidates:
          buildAutoModelRouterFingerprint({ temperature: 0.1 }) !== AUTO_MODEL_ROUTER_FINGERPRINT,
        reasoningEffortChangeInvalidates:
          buildAutoModelRouterFingerprint({ reasoningEffort: 'high' }) !== AUTO_MODEL_ROUTER_FINGERPRINT,
        defaultFlashAliasStable:
          buildAutoModelRouterFingerprint({ routerModel: AUTO_MODEL_ROUTER_MODEL }) === AUTO_MODEL_ROUTER_FINGERPRINT,
        responseFormatDefaultStable:
          buildAutoModelRouterFingerprint({ responseFormat: undefined }) === AUTO_MODEL_ROUTER_FINGERPRINT
      },
      timeoutFallback: {
        timeoutMs: 1,
        timeoutFingerprint: buildAutoModelRouterFingerprint({ timeoutMs: 1 }),
        fallbackModel: 'deepseek-v4-pro',
        fallbackReasoningEffort: 'max',
        fallbackSource: 'heuristic',
        abortsClassifier: true
      },
      recommendationParsing: autoRouterRecommendationParsingCases(),
      contextBoundary: autoRouterContextBoundaryCase(),
      productBoundary: {
        publicAutoPlanSetting: false,
        projectAutoPlanOverride: false,
        reasonixControllerProtocol: false,
        topLevelRouteExposed: false
      },
      expected: {
        fingerprintCurrent: true,
        contractDriftInvalidatesCache: true,
        requestIsolated: true,
        timeoutFallsBackToHeuristic: true,
        abortsTimedOutClassifier: true,
        acceptedRecommendationCount: 2,
        rejectedRecommendationCount: 2,
        proMaxRecommendationAccepted: true,
        autoRecommendationRejected: true,
        malformedRecommendationRejected: true,
        activeTurnExcludedFromRecentContext: true,
        recentContextPreservesToolSummary: true,
        stablePrefixContainsClassifierState: false,
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      }
    })
    expect(g5.controlExecutableCases.combined.expected).toEqual(
      combinedStepCancelCacheExpected(g5)
    )
    const loopTestSource = repoSource('packages/runtime/tests/loop.test.ts')
    expect(loopTestSource).toContain('does not promote an aborted plan follow-up step to the cache prefix baseline')
    expect(loopTestSource).toContain("requests[1]?.requiredToolName).toBe(CREATE_PLAN_TOOL_NAME)")
    expect(loopTestSource).toContain("prefixChanged: false")
    expect(loopTestSource).toContain("cacheHitTokens: 80")
    expect(g5.controlExecutableCases.planStepCancelCache.expected).toEqual(
      planStepCancelCacheExpected(g5)
    )
    expect(g5.controlExecutableCases.planStepCancelCache.followUpTools).toEqual(['create_plan'])
    expect(g5.controlExecutableCases.planStepCancelCache.productBoundary).toEqual({
      publicAutoPlanSetting: false,
      reasonixControllerProtocol: false,
      topLevelRouteExposed: false,
      defaultGoBackendEnabled: false
    })
    expect(loopTestSource).toContain('does not leak cancelled plan mode into later normal or auto-routed turns')
    expect(loopTestSource).toContain("normalRequest?.modeInstruction).toBeUndefined()")
    expect(loopTestSource).toContain("routerRequests).toHaveLength(1)")
    expect(loopTestSource).toContain("autoRequest?.requiredToolName).toBeUndefined()")
    expect(g5.controlExecutableCases.planCancelStateReset.expected).toEqual(
      planCancelStateResetExpected(g5)
    )
    expect(g5.controlExecutableCases.planCancelStateReset.productBoundary).toEqual({
      publicAutoPlanSetting: false,
      projectAutoPlanOverride: false,
      reasonixControllerProtocol: false,
      topLevelRouteExposed: false,
      defaultGoBackendEnabled: false
    })
    expect(g5.controlExecutableCases.combined.autoRouteCache.classifierStateInStablePrefix).toBe(false)
    expect(g5.controlExecutableCases.combined.stepLimit.dynamicLimitInStablePrefix)
      .toBe(g5.controlReplay.stepLimits.dynamicLimitInStablePrefix)
    expect(g5.controlExecutableCases.combined.stepLimit.maxModelSteps)
      .toBe(g5.controlExecutableCases.combined.autoRouteCache.mainModelSteps)
    expect(g5.controlExecutableCases.combined.cancel.cancelledResultCode)
      .toBe(g5.controlReplay.cancel.cancelledResultCode)
    expect(g5.controlExecutableCases.providerCacheReleaseGuard).toEqual({
      ...providerCache.releaseGuard,
      expected: providerReleaseGuard(providerCache)
    })
    expect(g5.controlExecutableCases.providerUsageParser).toEqual({
      cases: providerCache.providerUsageCases,
      expected: providerUsageParserReplay(providerCache)
    })
    expect(g5.controlExecutableCases.providerRequestShape).toEqual({
      cases: providerCache.requestShapeCases,
      expected: providerRequestShapeSummary(providerCache)
    })
    expect(g5.controlExecutableCases.providerLiveLocalHttpContract).toEqual(
      providerLiveLocalHttpContractControlCase(providerCache)
    )
    expect(g5.controlExecutableCases.providerCacheCoverageFloor).toEqual(
      providerCacheCoverageFloorControlCase(providerCache)
    )
    expect(g5.controlExecutableCases.providerDriftAttribution).toEqual(
      providerDriftAttributionControlCase(providerCache)
    )
    expect(g5.controlExecutableCases.providerCacheAccounting).toEqual({
      cases: providerUsageAccountingCases(providerCache),
      expected: providerCacheAccounting(providerCache)
    })
    expect(g5.controlExecutableCases.providerOfflineParitySeal).toEqual(
      providerOfflineParitySealControlCase(providerCache)
    )
    expect(g5.controlExecutableCases.providerCachePrivacy).toEqual(
      providerCachePrivacyControlCase(providerCache)
    )
    expect(g5.controlExecutableCases.providerCacheInventory).toEqual(
      providerCacheInventoryControlCase(providerCache)
    )
    expect(g5.controlExecutableCases.providerStreaming).toEqual(
      g3ProviderStreamingControlCase(providerStreaming)
    )
    expect(g5.controlExecutableCases.sessionRouteStatus).toEqual(
      g2RouteStatusControlCase(g2)
    )
    expect(g5.controlExecutableCases.sessionRouteInventory).toEqual(
      g2RouteInventoryControlCase(g2)
    )
    expect(g5.controlExecutableCases.sessionRouteReplay).toEqual(
      g2SessionRouteReplayControlCase(g2)
    )
  })

  it('keeps the G5 combined step/cancel/cache trace focused and shadow-only', () => {
    const g5 = loadGoG5FullLoopContract()
    const combined = g5.controlExecutableCases.combined

    expect(combined.expected).toEqual(combinedStepCancelCacheExpected(g5))
    expect(combined.autoRouteCache).toEqual({
      sameTurnRouterCalls: 1,
      mainModelSteps: 2,
      nextTurnRouterCalls: 1,
      classifierStateInStablePrefix: false
    })
    expect(combined.stepLimit).toEqual({
      maxModelSteps: combined.autoRouteCache.mainModelSteps,
      errorCode: 'turn_step_limit_exceeded',
      dynamicLimitInStablePrefix: false
    })
    expect(combined.cancel.acceptedToolCalls.map((call) => call.state)).toEqual([
      'completed',
      'running',
      'unstarted'
    ])
    expect(combined.expected.cancelResults.map((result) => result.status)).toEqual([
      'completed',
      'aborted',
      'aborted'
    ])
    expect(combined.expected.routeCacheReusedUntilStepLimit).toBe(true)
    expect(combined.expected.nextTurnReroutes).toBe(true)
    expect(combined.expected.completedResultPreserved).toBe(true)
    expect(combined.expected.classifierStateInStablePrefix).toBe(false)
    expect(combined.expected.stepLimitDynamicInStablePrefix).toBe(false)
    expect(g5.expectedOutput.defaultGoBackendEnabled).toBe(false)
    expect(g5.productBoundary.reasonixPublicProtocolAllowed).toBe(false)
    expect(g5.productBoundary.defaultGoBackendAllowed).toBe(false)
    expect(g5.productBoundary.rendererVisibleGoRoutesAllowed).toBe(false)
  })

  it('keeps approval and user-input gates focused, private, and shadow-only', () => {
    const g5 = loadGoG5FullLoopContract()
    const approvalUserInput = loadApprovalUserInputRouteContract()
    const taskJob = loadTaskJobOrchestrationContract()
    const g4 = loadGoG4ToolsContract()

    expect(g5.controlExecutableCases.approvalDeny.expected).toEqual({
      deniedToolNames: taskJob.approvalDenyNoExecute.deniedToolNames,
      approvalIds: taskJob.approvalDenyNoExecute.approvalIds,
      approvalItemCount: taskJob.approvalDenyNoExecute.approvalIds.length,
      createsDurableJobs: false,
      createsChildRuns: false
    })
    expect(g5.controlExecutableCases.userInput.expected).toMatchObject({
      submittedInputId: approvalUserInput.submittedUserInput.id,
      submittedStatus: 'submitted',
      answerCount: approvalUserInput.submittedUserInput.expectedResolution.answers.length,
      httpEchoesAnswers: true,
      resolvedEventKind: 'user_input_resolved',
      resolvedEventIncludesAnswers: false,
      cancelledInputId: approvalUserInput.userInput.id,
      cancelledStatus: 'cancelled',
      secondResolveStatus: 404,
      pendingAfterSubmit: 0,
      pendingAfterCancel: 0
    })
    expect(g5.controlExecutableCases.userInput.expected.structuredChoiceValidation).toEqual({
      maxQuestions: g4.userInput.structuredChoiceValidation.maxQuestions,
      minOptionsWhenProvided: g4.userInput.structuredChoiceValidation.minOptionsWhenProvided,
      maxOptionsWhenProvided: g4.userInput.structuredChoiceValidation.maxOptionsWhenProvided,
      dedupeLabelsCaseInsensitive: true,
      invalidResultCode: 'invalid_user_input_request',
      invalidCases: g4.userInput.structuredChoiceValidation.invalidCases,
      invalidCaseCount: g4.userInput.structuredChoiceValidation.invalidCases.length,
      rejectsTooManyQuestions: true,
      rejectsSingleOption: true,
      rejectsTooManyOptions: true,
      rejectsDuplicateLabelsCaseInsensitive: true,
      opensGateOnInvalid: false,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    expect(g5.controlExecutableCases.approvalUserInputRouteReplay).toEqual(
      approvalUserInputRouteReplayControlCase(approvalUserInput)
    )
    expect(g5.controlExecutableCases.approvalUserInputInventory).toEqual(
      approvalUserInputInventoryControlCase(approvalUserInput)
    )
    expect(g5.controlExecutableCases.abortCleanup.expected).toEqual({
      approvalId: approvalUserInput.abortCleanup.approvalId,
      approvalStatus: 'expired',
      userInputId: approvalUserInput.abortCleanup.userInputId,
      userInputStatus: 'cancelled',
      lateApprovalDecisionStatus: 409,
      lateUserInputResolveStatus: 404,
      pendingAfterCleanup: 0,
      replayKinds: approvalUserInput.abortCleanup.expectedReplayKinds,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    expect(g5.controlExecutableCases.resumePendingGates.expected).toEqual({
      sourceApprovalStatus: 'pending',
      sourceUserInputStatus: 'pending',
      resumedApprovalStatus: 'expired',
      resumedUserInputStatus: 'cancelled',
      pendingAfterResume: 0,
      answersCopiedToResume: false,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    expect(g5.expectedOutput.defaultGoBackendEnabled).toBe(false)
    expect(g5.productBoundary.reasonixPublicProtocolAllowed).toBe(false)
    expect(g5.productBoundary.rendererVisibleGoRoutesAllowed).toBe(false)
  })

  it('keeps session routes exact, private, and shadow-only', () => {
    const g5 = loadGoG5FullLoopContract()
    const g2 = loadGoG2RouteReplayContract()
    const routeStatus = g5.controlExecutableCases.sessionRouteStatus
    const routeInventory = g5.controlExecutableCases.sessionRouteInventory
    const sessionRouteReplay = g5.controlExecutableCases.sessionRouteReplay

    expect(routeStatus).toEqual(g2RouteStatusControlCase(g2))
    expect(routeInventory).toEqual(g2RouteInventoryControlCase(g2))
    expect(sessionRouteReplay).toEqual(g2SessionRouteReplayControlCase(g2))
    expect(routeStatus.expected.archive).toEqual({
      patchStatus: 200,
      archivedStatus: 'archived',
      archivedOnlyCount: 1,
      searchArchivedCount: 1,
      searchFirstStatus: 'archived'
    })
    expect(routeStatus.expected.fork).toMatchObject({
      status: 201,
      relation: 'side',
      parentThreadId: 'thr_g2_parent',
      forkedTurnCount: 1
    })
    expect(routeStatus.expected.resume).toEqual({
      status: 201,
      sessionId: 'thr_g2_source',
      messageCount: 1,
      summary: 'Source Thread resumed'
    })
    expect(routeStatus.expected.sse).toEqual({
      replayStatus: 200,
      replayFrameCount: 2,
      caughtUpStatus: 200,
      caughtUpFrameCount: 0,
      replayEventNames: ['turn_started', 'item_created']
    })
    expect(routeInventory.expected).toMatchObject({
      routeIds: [
        'thread-list-default',
        'thread-archive-patch',
        'thread-list-archived-only',
        'thread-search-archived',
        'thread-read-detail',
        'thread-update-title-workspace',
        'thread-fork-side',
        'session-resume-thread',
        'events-since-seq',
        'events-caught-up-since-seq',
        'events-unauthorized-since-seq'
      ],
      resumeRouteIds: ['session-resume-thread'],
      forkRouteIds: ['thread-fork-side'],
      archiveRouteIds: [
        'thread-archive-patch',
        'thread-list-archived-only',
        'thread-search-archived'
      ],
      searchRouteIds: [
        'thread-list-archived-only',
        'thread-search-archived'
      ],
      eventRouteIds: [
        'events-since-seq',
        'events-caught-up-since-seq',
        'events-unauthorized-since-seq'
      ],
      runtimeTokenRouteCount: 10,
      unauthorizedRouteIds: ['events-unauthorized-since-seq'],
      usesReasonixProtocol: false,
      topLevelRouteExposed: false
    })
    expect(sessionRouteReplay.expected).toMatchObject({
      routeCount: 11,
      exactJsonBodyRouteCount: 9,
      exactSseRouteCount: 2,
      runtimeTokenRouteCount: 10,
      unauthorizedRouteIds: ['events-unauthorized-since-seq'],
      archiveResponseHash: 'e0d0b8a04a6f162d',
      searchResponseHash: '27cf96765460646f',
      forkResponseHash: 'bdf16e24ca626c77',
      resumeResponseHash: '503a4dd53f682b29',
      replaySseHash: 'b194787b8ea18143',
      caughtUpSseHash: '',
      unauthorizedBodyHash: '764cacdad1a8e5a3',
      everyRouteHasMethodPathStatus: true,
      jsonRoutesHaveBodyHash: true,
      sseRoutesHaveExactFrames: true,
      usesReasonixProtocol: false,
      topLevelRouteExposed: false,
      rendererVisibleGoRoute: false,
      defaultGoBackend: false
    })
    expect(sessionRouteReplay.expected.matrix.map((route) => ({
      id: route.id,
      method: route.method,
      path: route.path,
      status: route.status,
      responseKind: route.responseKind
    }))).toEqual([
      {
        id: 'thread-list-default',
        method: 'GET',
        path: '/v1/threads',
        status: 200,
        responseKind: 'json'
      },
      {
        id: 'thread-archive-patch',
        method: 'PATCH',
        path: '/v1/threads/thr_g2_beta',
        status: 200,
        responseKind: 'json'
      },
      {
        id: 'thread-list-archived-only',
        method: 'GET',
        path: '/v1/threads?archived_only=true',
        status: 200,
        responseKind: 'json'
      },
      {
        id: 'thread-search-archived',
        method: 'GET',
        path: '/v1/threads?include_archived=true&search=archive',
        status: 200,
        responseKind: 'json'
      },
      {
        id: 'thread-read-detail',
        method: 'GET',
        path: '/v1/threads/thr_g2_read',
        status: 200,
        responseKind: 'json'
      },
      {
        id: 'thread-update-title-workspace',
        method: 'PATCH',
        path: '/v1/threads/thr_g2_read',
        status: 200,
        responseKind: 'json'
      },
      {
        id: 'thread-fork-side',
        method: 'POST',
        path: '/v1/threads/thr_g2_parent/fork',
        status: 201,
        responseKind: 'json'
      },
      {
        id: 'session-resume-thread',
        method: 'POST',
        path: '/v1/sessions/thr_g2_source/resume-thread',
        status: 201,
        responseKind: 'json'
      },
      {
        id: 'events-since-seq',
        method: 'GET',
        path: '/v1/threads/thr_g2_events/events?since_seq=1',
        status: 200,
        responseKind: 'sse'
      },
      {
        id: 'events-caught-up-since-seq',
        method: 'GET',
        path: '/v1/threads/thr_g2_events/events?since_seq=3',
        status: 200,
        responseKind: 'sse'
      },
      {
        id: 'events-unauthorized-since-seq',
        method: 'GET',
        path: '/v1/threads/thr_g2_events/events?since_seq=0',
        status: 401,
        responseKind: 'json'
      }
    ])
    expect(g5.expectedOutput.defaultGoBackendEnabled).toBe(false)
    expect(g5.productBoundary.reasonixPublicProtocolAllowed).toBe(false)
    expect(g5.productBoundary.rendererVisibleGoRoutesAllowed).toBe(false)
    expect(g5.productBoundary.defaultGoBackendAllowed).toBe(false)
  })

  it('keeps G2 route replay conformance tied to the shared contract fixture', () => {
    const manifest = buildGoRuntimeKernelConformanceManifest()
    const contract = loadGoG2RouteReplayContract()

    expect(manifest.productBoundary).toEqual(contract.productBoundary)
    expect(manifest.g2ShadowRoutes.map(({ id, method, path, auth }) => ({ id, method, path, auth })))
      .toEqual(contract.routes.map(({ id, method, path, auth }) => ({ id, method, path, auth })))
  })

  it('requires auth on every registered /v1 runtime route from the G5 route-sovereignty fixture', async () => {
    const g5 = loadGoG5FullLoopContract()
    const h = buildHarness()
    const routeCase = g5.controlExecutableCases.runtimeHttpRouteSovereignty
    const unauthorizedRoutes: string[] = []

    for (const route of routeCase.routes) {
      const response = await dispatchRequest(h.router, requestForRegisteredRoute(route))
      const routeKey = `${route.method} ${route.path}`

      if (routeKey === 'GET /health') {
        expect(response.status, routeKey).toBe(routeCase.authMatrix.healthStatus)
        continue
      }

      unauthorizedRoutes.push(routeKey)
      expect(response.status, routeKey).toBe(routeCase.authMatrix.unauthorizedStatus)
      expect(await readJson(response), routeKey).toEqual(routeCase.authMatrix.unauthorizedBody)
    }

    expect(unauthorizedRoutes).toHaveLength(routeCase.authenticatedRouteCount)
    expect(unauthorizedRoutes).toHaveLength(routeCase.authMatrix.protectedRouteCount)
    expect(unauthorizedRoutes).toEqual(routeCase.authMatrix.protectedRouteKeys)
    expect(unauthorizedRoutes).toEqual(
      routeCase.routes
        .map((route) => `${route.method} ${route.path}`)
        .filter((route) => route !== 'GET /health')
    )
    expect(unauthorizedRoutes).toEqual(
      expect.arrayContaining(routeCase.authMatrix.sensitiveRouteKeys)
    )
  })

  it('executes thread summary task routes from the G5 task-job fixture', async () => {
    const g5 = loadGoG5FullLoopContract()
    const routeCase = g5.controlExecutableCases.taskJobs.summaryRouteExecutable
    const dir = await mkdtemp(join(tmpdir(), 'analytix-g5-summary-routes-'))
    let tick = 0
    let release: ReturnType<typeof deferred<string>> | undefined

    try {
      const h = buildHarness({
        nowIso: () => `2026-06-30T00:00:${String(tick++).padStart(2, '0')}.000Z`
      })
      const generatedIds = new Map<string, string[]>([
        ['task', [routeCase.seeded.runningJobId, routeCase.seeded.restartJobId]],
        ['parallel_task', [routeCase.seeded.completedJobId]]
      ])
      const manager = new DurableTaskJobManager({
        store: new FileTaskJobStore(dir),
        nowIso: h.nowIso,
        idGenerator: (kind) => generatedIds.get(kind)?.shift() ?? `${kind}_unexpected_g5_summary`
      })
      h.runtime.taskJobs = manager
      await h.threadService.create({
        workspace: dir,
        model: 'deepseek-chat',
        mode: 'agent'
      }, { id: routeCase.seeded.parentThreadId })
      await h.threadService.create({
        workspace: dir,
        model: 'deepseek-chat',
        mode: 'agent'
      }, { id: routeCase.seeded.otherThreadId })
      const restartThread = await h.threadService.create({
        workspace: dir,
        model: 'deepseek-chat',
        mode: 'agent',
        approvalPolicy: 'auto',
        sandboxMode: 'workspace-write'
      }, { id: routeCase.seeded.restartThreadId })
      await putG5CommandThread(h, restartThread, {
        cwd: dir,
        command: 'printf restarted',
        callId: routeCase.seeded.commandTaskId.slice('command:'.length)
      })

      release = deferred<string>()
      const active = await manager.startBackground({
        kind: 'task',
        parentThreadId: routeCase.seeded.parentThreadId,
        parentTurnId: 'turn_g5_summary',
        label: 'G5 summary route task',
        childRunId: routeCase.seeded.childRunId
      }, async ({ appendOutput }) => {
        await appendOutput(routeCase.expected.output)
        return release!.promise
      })
      expect(active.id).toBe(routeCase.seeded.runningJobId)
      await waitFor(async () =>
        (await manager.output(active.id, { offset: 0 }))?.output === routeCase.expected.output
      )

      const completed = await manager.startForeground({
        kind: 'parallel_task',
        parentThreadId: routeCase.seeded.parentThreadId,
        parentTurnId: 'turn_g5_summary',
        label: 'G5 summary route parallel task',
        dependencies: ['seed'],
        childRunId: routeCase.seeded.parallelChildRunId
      }, async () => 'parallel route result')
      expect(completed.id).toBe(routeCase.seeded.completedJobId)

      await h.runtime.events.record({
        kind: 'turn_started',
        threadId: routeCase.seeded.parentThreadId,
        turnId: 'turn_g5_summary',
        text: 'summary route child task',
        child: {
          parentThreadId: routeCase.seeded.parentThreadId,
          parentTurnId: 'turn_g5_summary',
          childId: 'child_g5_summary_task',
          childRunId: routeCase.seeded.childRunId,
          childThreadId: routeCase.seeded.childThreadId,
          childStatus: 'running'
        }
      })
      await h.runtime.events.record({
        kind: 'turn_completed',
        threadId: routeCase.seeded.parentThreadId,
        turnId: 'turn_g5_summary',
        text: 'summary route parallel task',
        child: {
          parentThreadId: routeCase.seeded.parentThreadId,
          parentTurnId: 'turn_g5_summary',
          childId: 'child_g5_summary_parallel',
          childRunId: routeCase.seeded.parallelChildRunId,
          childThreadId: routeCase.seeded.parallelChildThreadId,
          childStatus: 'completed'
        }
      })

      const summary = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(`/v1/threads/${routeCase.seeded.parentThreadId}/summary`)
      )
      expect(summary.status).toBe(routeCase.expected.summaryStatus)
      const summaryBody = await readJson(summary) as {
        tasks: Array<{
          id: string
          status: string
          terminal: boolean
          outputWithheld: boolean
          factAnswerAllowed: boolean
          evidenceAuthority: boolean
          canReadOutput: boolean
        }>
        subagents: Array<{
          childRunId?: string
          childThreadId?: string
          status: string
          canOpenThread: boolean
          canKill: boolean
        }>
      }
      expect(summaryBody.tasks).toHaveLength(routeCase.expected.taskCount)
      expect(summaryBody.tasks).toEqual(expect.arrayContaining([
        {
          schemaVersion: 1,
          id: `taskjob:${routeCase.seeded.runningJobId}`,
          kind: 'task',
          status: routeCase.expected.runningTaskStatus,
          background: false,
          terminal: routeCase.expected.runningTaskTerminal,
          outputWithheld: routeCase.expected.taskOutputWithheld,
          outputTrustStatus: 'untrusted_child_output',
          factAnswerAllowed: routeCase.expected.taskFactAnswerAllowed,
          evidenceAuthority: routeCase.expected.taskEvidenceAuthority,
          canReadOutput: routeCase.expected.taskCanReadOutput,
          canContinueParent: false
        },
        {
          schemaVersion: 1,
          id: `taskjob:${routeCase.seeded.completedJobId}`,
          status: routeCase.expected.completedTaskStatus,
          kind: 'parallel_task',
          background: true,
          terminal: routeCase.expected.completedTaskTerminal,
          outputWithheld: routeCase.expected.taskOutputWithheld,
          outputTrustStatus: 'untrusted_child_output',
          factAnswerAllowed: routeCase.expected.taskFactAnswerAllowed,
          evidenceAuthority: routeCase.expected.taskEvidenceAuthority,
          canReadOutput: routeCase.expected.taskCanReadOutput,
          canContinueParent: false
        }
      ]))
      expect(summaryBody.subagents).toHaveLength(routeCase.expected.subagentCount)
      expect(summaryBody.subagents).toEqual(expect.arrayContaining([
        expect.objectContaining({
          childRunId: routeCase.seeded.childRunId,
          childThreadId: routeCase.seeded.childThreadId,
          status: routeCase.expected.runningSubagentStatus,
          canOpenThread: routeCase.expected.subagentCanOpenThread,
          canKill: routeCase.expected.subagentCanKill
        }),
        expect.objectContaining({
          childRunId: routeCase.seeded.parallelChildRunId,
          childThreadId: routeCase.seeded.parallelChildThreadId,
          status: routeCase.expected.completedSubagentStatus,
          canOpenThread: routeCase.expected.subagentCanOpenThread,
          canKill: routeCase.expected.subagentCanKill
        })
      ]))

      const taskId = encodeURIComponent(`taskjob:${routeCase.seeded.runningJobId}`)
      const output = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(`/v1/threads/${routeCase.seeded.parentThreadId}/summary/tasks/${taskId}/output?offset=0`)
      )
      expect(output.status).toBe(routeCase.expected.outputStatus)
      // The G5 fixture's output bytes remain an internal execution workload;
      // the public route is a closed metadata-only security boundary.
      expect(await readJson(output)).toEqual({
        schemaVersion: 1,
        availability: 'withheld',
        taskId: `taskjob:${routeCase.seeded.runningJobId}`,
        status: 'running',
        reasonCode: 'security_bound_child_output',
        outputWithheld: true,
        outputTrustStatus: 'untrusted_child_output',
        factAnswerAllowed: false,
        evidenceAuthority: false,
        canReadOutput: false,
        canContinueParent: false
      })

      const crossThreadOutput = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(`/v1/threads/${routeCase.seeded.otherThreadId}/summary/tasks/${taskId}/output?offset=0`)
      )
      expect(crossThreadOutput.status).toBe(routeCase.expected.crossThreadOutputStatus)
      expect(await readJson(crossThreadOutput)).toMatchObject({ code: 'forbidden' })

      const crossThreadKill = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(`/v1/threads/${routeCase.seeded.otherThreadId}/summary/tasks/${taskId}/kill`, 'POST')
      )
      expect(crossThreadKill.status).toBe(routeCase.expected.crossThreadKillStatus)
      expect(await readJson(crossThreadKill)).toMatchObject({ code: 'forbidden' })

      const kill = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(`/v1/threads/${routeCase.seeded.parentThreadId}/summary/tasks/${taskId}/kill`, 'POST')
      )
      expect(kill.status).toBe(routeCase.expected.killStatus)
      expect(await readJson(kill)).toEqual({
        task: {
          schemaVersion: 1,
          id: `taskjob:${routeCase.seeded.runningJobId}`,
          kind: 'task',
          status: routeCase.expected.killedTaskStatus,
          background: false,
          terminal: routeCase.expected.killedTaskTerminal,
          outputWithheld: routeCase.expected.taskOutputWithheld,
          outputTrustStatus: 'untrusted_child_output',
          factAnswerAllowed: routeCase.expected.taskFactAnswerAllowed,
          evidenceAuthority: routeCase.expected.taskEvidenceAuthority,
          canReadOutput: routeCase.expected.taskCanReadOutput,
          canContinueParent: false
        }
      })
      const restartSummary = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(`/v1/threads/${routeCase.seeded.restartThreadId}/summary`)
      )
      expect(restartSummary.status).toBe(routeCase.expected.summaryStatus)
      const restartSummaryBody = await readJson(restartSummary) as { tasks: unknown[] }
      expect(restartSummaryBody.tasks).toEqual([{
        schemaVersion: 1,
        id: routeCase.seeded.commandTaskId,
        kind: 'command',
        status: routeCase.expected.commandTaskStatus,
        background: false,
        active: false,
        terminal: routeCase.expected.commandTaskTerminal,
        outputWithheld: true,
        outputTrustStatus: 'private_tool_output',
        factAnswerAllowed: false,
        evidenceAuthority: false,
        canReadOutput: false,
        canContinueParent: false
      }])

      const restart = await dispatchRequest(
        h.router,
        authorizedRuntimeRequest(
          `/v1/threads/${routeCase.seeded.restartThreadId}/summary/tasks/${encodeURIComponent(routeCase.seeded.commandTaskId)}/restart`,
          'POST'
        )
      )
      expect(restart.status).toBe(routeCase.expected.restartStatus)
      expect(await readJson(restart)).toEqual({
        code: routeCase.expected.restartErrorCode,
        message: routeCase.expected.restartMessage
      })
      expect(routeCase.productBoundary).toEqual({
        usesReasonixProtocol: false,
        topLevelRouteExposed: false
      })
    } finally {
      release?.resolve('done')
      await rm(dir, { recursive: true, force: true, maxRetries: 5, retryDelay: 10 })
    }
  })

  it('returns structured not_found for forbidden public route tokens from the G5 route-sovereignty fixture', async () => {
    const g5 = loadGoG5FullLoopContract()
    const h = buildHarness()
    const routeCase = g5.controlExecutableCases.runtimeHttpRouteSovereignty
    const forbiddenPaths = routeCase.forbiddenDispatchMatrix.tokens

    for (const path of forbiddenPaths) {
      const response = await dispatchRequest(
        h.router,
        new Request(`http://localhost${path}`, {
          headers: { authorization: 'Bearer tok-1' }
        })
      )

      expect(response.status, path).toBe(routeCase.forbiddenDispatchMatrix.status)
      expect(await readJson(response), path).toEqual(routeCase.forbiddenDispatchMatrix.body)
    }

    expect(forbiddenPaths).toHaveLength(routeCase.forbiddenDispatchMatrix.tokenCount)
    expect(forbiddenPaths).toEqual(routeCase.forbiddenRouteTokens)
    expect(forbiddenPaths).toEqual(
      expect.arrayContaining(routeCase.forbiddenDispatchMatrix.protocolTokens)
    )
    expect(forbiddenPaths).toEqual(
      expect.arrayContaining(routeCase.forbiddenDispatchMatrix.hiddenSurfaceTokens)
    )
  })

  it('keeps G1 shadow route conformance tied to existing analytix HTTP contracts', async () => {
    const manifest = buildGoRuntimeKernelConformanceManifest()
    const contract = loadGoG1ShadowContract()
    const h = buildHarness()

    expect(manifest.g1ShadowRoutes).toEqual([
      expect.objectContaining({ id: 'health', path: '/health', auth: 'none' }),
      expect.objectContaining({ id: 'runtime-info', path: '/v1/runtime/info', auth: 'runtime-token' }),
      expect.objectContaining({ id: 'runtime-tools', path: '/v1/runtime/tools', auth: 'runtime-token' })
    ])

    const health = await dispatchRequest(h.router, new Request('http://localhost/health'))
    expect(health.status).toBe(contract.health.status)
    expect(await readJson(health)).toEqual(contract.health.body)

    const unauthorizedInfo = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/info')
    )
    expect(unauthorizedInfo.status).toBe(contract.runtimeInfoUnauthorized.status)
    const unauthorizedInfoBody = await readJson(unauthorizedInfo)
    expect(PublicRuntimeErrorResponse.parse(unauthorizedInfoBody)).toEqual(contract.runtimeInfoUnauthorized.body)

    const info = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/info', {
        headers: { authorization: `Bearer ${contract.runtimeToken}` }
      })
    )
    expect(info.status).toBe(contract.runtimeInfo.status)
    const infoBody = await readJson(info)
    const parsedInfo = RuntimeInfoResponse.parse(infoBody)
    expect(parsedInfo.schemaVersion).toBe(contract.runtimeInfo.schemaVersion)
    const normalizedInfo = { ...parsedInfo, startedAt: contract.startedAt }
    expect(exactPublicContractHash(normalizedInfo)).toBe(contract.runtimeInfo.bodyHash)
    for (const key of contract.runtimeInfo.forbiddenTopLevelKeys) {
      expect(parsedInfo).not.toHaveProperty(key)
    }

    const unauthorizedTools = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools')
    )
    expect(unauthorizedTools.status).toBe(contract.runtimeToolsUnauthorized.status)
    const unauthorizedToolsBody = await readJson(unauthorizedTools)
    expect(PublicRuntimeErrorResponse.parse(unauthorizedToolsBody)).toEqual(contract.runtimeToolsUnauthorized.body)

    const tools = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools', {
        headers: { authorization: `Bearer ${contract.runtimeToken}` }
      })
    )
    expect(tools.status).toBe(contract.runtimeTools.status)
    const parsedTools = RuntimeToolsResponse.parse(await readJson(tools))
    expect(parsedTools.schemaVersion).toBe(contract.runtimeTools.schemaVersion)
    expect(exactPublicContractHash(parsedTools)).toBe(contract.runtimeTools.bodyHash)
    for (const key of contract.runtimeTools.forbiddenTopLevelKeys) {
      expect(parsedTools).not.toHaveProperty(key)
    }
    const serializedTools = JSON.stringify(parsedTools)
    for (const token of contract.runtimeTools.forbiddenJSONTokens) {
      expect(serializedTools).not.toContain(token)
    }
  })

  it('matches the G2 route replay fixture against the TypeScript HTTP/SSE contracts', async () => {
    const contract = loadGoG2RouteReplayContract()

    for (const route of contract.routes) {
      const h = buildG2Harness()
      await seedG2Setup(h, route.setup)
      const response = await dispatchRequest(h.router, requestFromRoute(route, contract.runtimeToken))

      expect(response.status, route.id).toBe(route.response.status)
      if (route.responseKind === 'sse') {
        expect(response.headers.get('content-type'), route.id).toContain('text/event-stream')
        expect(await readSseEvents(response), route.id).toEqual(route.sseFrames ?? [])
      } else {
        expect(await readJson(response), route.id).toEqual(route.response.body)
      }
    }
  })

  it('keeps the Go live-local sidecar prototype test-only and isolated for G2/G3/G4 replay', () => {
    const g5 = loadGoG5FullLoopContract()
    const source = repoSource('packages/runtime-go/contract_shims_test.go')
    const harnessSource = repoSource('packages/runtime-go/internal/conformance/livelocal/harness.go')
    const boundarySource = repoSource('packages/runtime-go/internal/contracts/boundary.go')
    const storeSource = repoSource('packages/runtime-go/internal/conformance/livelocal/store.go')
    const providerSource = repoSource('packages/runtime-go/internal/provider/provider_contract_replay_handler.go')
    const g4Source = repoSource('packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go')
    const durableSource = repoSource('packages/runtime-go/internal/conformance/livelocal/durable_handler.go')
    const durableStoreSource = [
      repoSource('packages/runtime-go/internal/server/durable_store.go'),
      repoSource('packages/runtime-go/internal/server/durable_core.go'),
      repoSource('packages/runtime-go/internal/server/durable_event_log.go'),
      repoSource('packages/runtime-go/internal/server/durable_goals_todos.go'),
      repoSource('packages/runtime-go/internal/server/durable_threads.go'),
      repoSource('packages/runtime-go/internal/server/durable_turns.go'),
      repoSource('packages/runtime-go/internal/server/durable_recovery.go'),
      repoSource('packages/runtime-go/internal/adapters/outbound/eventlog/store.go'),
    ].join('\n')
    const durableTestSource = repoSource('packages/runtime-go/durable_replay_contract_test.go')
    const durableCommandSource = repoSource('packages/runtime-go/cmd/contract-sidecar/main.go')
    const durableSidecarTestSource = repoSource('packages/runtime/tests/go-durable-sidecar-conformance.test.ts')
    const durableContractSource = repoSource('packages/runtime/src/conformance/fixtures/go-durable-sidecar-contract.json')
    const testSource = repoSource('packages/runtime-go/shadow_test.go')

    expect(source).toContain('NewLiveLocalSidecarHandler')
    expect(source).toContain('NewLiveLocalSidecarHarness')
    expect(harnessSource).toMatch(/ProviderContract\s+provider\.G3ProviderConformanceContract/)
    expect(harnessSource).toMatch(/G4Contract\s+mcp\.G4ToolsConformanceContract/)
    expect(harnessSource).toMatch(/ApprovalUserInputContract\s+agent\.ApprovalUserInputRouteContract/)
    expect(harnessSource).toMatch(/MCPToolLifecycleContract\s+mcp\.MCPToolLifecycleContract/)
    expect(harnessSource).toMatch(/DurableTempDir\s+string/)
    expect(boundarySource).toMatch(/TestConformanceOnly:\s+true/)
    expect(boundarySource).toMatch(/ReadOnlyRouteReplayOnly:\s+false/)
    expect(boundarySource).toMatch(/IsolatedMutableG2LifecyclePrototype:\s+true/)
    expect(boundarySource).toMatch(/IsolatedInMemoryStoreOnly:\s+true/)
    expect(boundarySource).toMatch(/FixtureBackedProviderG3Prototype:\s+true/)
    expect(boundarySource).toMatch(/FixtureBackedProviderOnly:\s+true/)
    expect(boundarySource).toMatch(/IsolatedG4ManagerPrototype:\s+true/)
    expect(boundarySource).toMatch(/FixtureBackedG4ManagerOnly:\s+true/)
    expect(boundarySource).toMatch(/TempDurableStorePrototype:\s+false/)
    expect(boundarySource).toMatch(/TempDurableStoreOnly:\s+false/)
    expect(boundarySource).toMatch(/ExternalNetworkAllowed:\s+false/)
    expect(boundarySource).toMatch(/ProviderCredentialsAllowed:\s+false/)
    expect(boundarySource).toMatch(/APIKeyReadAllowed:\s+false/)
    expect(boundarySource).toMatch(/ProviderLiveCallsAllowed:\s+false/)
    expect(boundarySource).toMatch(/ToolExecutionAllowed:\s+false/)
    expect(boundarySource).toMatch(/ApprovalExecutionAllowed:\s+false/)
    expect(boundarySource).toMatch(/MCPConnectionAllowed:\s+false/)
    expect(boundarySource).toMatch(/MCPCredentialsAllowed:\s+false/)
    expect(boundarySource).toMatch(/CredentialReadAllowed:\s+false/)
    expect(boundarySource).toMatch(/FileMutationAllowed:\s+false/)
    expect(boundarySource).toMatch(/EventsJSONLMutationAllowed:\s+false/)
    expect(boundarySource).toMatch(/RealWorkspaceMutationAllowed:\s+false/)
    expect(boundarySource).toMatch(/ElectronMainConnected:\s+false/)
    expect(boundarySource).toMatch(/DefaultGoBackendEnabled:\s+false/)
    expect(boundarySource).toMatch(/RendererVisibleGoRoutesAllowed:\s+false/)
    expect(boundarySource).toMatch(/ReasonixPublicProtocolAllowed:\s+false/)
    expect(storeSource).toContain('LiveLocalSidecarSnapshot')
    expect(storeSource).toContain('MutatingG2Routes')
    expect(storeSource).toContain('IsMutatingG2Route')
    expect(storeSource).toContain('EventsJSONLWriteAttempts')
    expect(storeSource).toContain('RealWorkspaceWriteAttempts')
    expect(storeSource).toContain('http.MethodPatch')
    expect(storeSource).toContain('http.MethodPost')
    expect(storeSource).toContain('StubProviderUsageReplays')
    expect(storeSource).toContain('StubProviderShapeReplays')
    expect(storeSource).toContain('StubProviderStreamReplays')
    expect(storeSource).toContain('StubProviderCacheReplays')
    expect(storeSource).toContain('StubG4ApprovalReplays')
    expect(storeSource).toContain('StubG4UserInputReplays')
    expect(storeSource).toContain('StubG4MCPReplays')
    expect(storeSource).toContain('StubG4ValidationReplays')
    expect(storeSource).toContain('TempDurableStoreEnabled')
    expect(storeSource).toContain('ToolExecutionAttempts')
    expect(storeSource).toContain('MCPConnectionAttempts')
    expect(storeSource).toContain('CredentialReadAttempts')
    expect(providerSource).toContain('/v1/conformance/g3/provider')
    expect(providerSource).toContain('ParsedProviderUsageFromRawPayload')
    expect(providerSource).toContain('DerivedG3ProviderRequestURL')
    expect(providerSource).toContain('writeSSE(w, http.StatusOK, h.contract.Streaming.SSEFrames)')
    expect(providerSource).toContain('BuildG3ProviderCacheAccounting')
    expect(providerSource).toContain('BuildProviderDriftAttribution')
    expect(providerSource).toContain('"externalNetworkUsed"')
    expect(providerSource).toContain('"providerCredentialsUsed"')
    expect(providerSource).toContain('"apiKeyRead"')
    expect(providerSource).not.toMatch(/http\.DefaultClient|os\.Getenv|LookupEnv|ANALYTIX_API_KEY|OPENAI_API_KEY|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY/)
    expect(g4Source).toContain('/v1/conformance/g4/manager')
    expect(g4Source).toContain('handleApprovalDecision')
    expect(g4Source).toContain('handleUserInputValidation')
    expect(g4Source).toContain('handleMCPCallReconnect')
    expect(g4Source).toContain('handleMCPDiagnostics')
    expect(g4Source).toContain('"mcpConnectionAllowed"')
    expect(g4Source).toContain('"credentialReadAllowed"')
    expect(g4Source).toContain('"toolExecutionAllowed"')
    expect(g4Source).not.toMatch(/http\.DefaultClient|os\.Getenv|LookupEnv|ANALYTIX_API_KEY|OPENAI_API_KEY|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY|events\.jsonl/)
    expect(durableSource).toContain('/v1/conformance/durable')
    expect(durableSource).toContain('tempDurableEventSessionStoreEnabled')
    expect(durableSource).toContain('Last-Event-ID')
    expect(durableSource).toContain('since_seq')
    expect(durableSource).toContain('WriteDurableSSE')
    expect(durableSource).toContain('"reasonixPublicProtocolAllowed"')
    expect(durableSource).toContain('"defaultGoBackendEnabled"')
    expect(durableSource).toContain('"realWorkspaceMutationAllowed"')
    expect(durableSource).not.toMatch(/os\.Getenv|LookupEnv|ANALYTIX_API_KEY|OPENAI_API_KEY|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY|http\.DefaultClient/)
    expect(durableStoreSource).toContain('events.jsonl')
    expect(durableStoreSource).toContain('candidateDigest := sha256.New()')
    expect(durableStoreSource).toContain('candidateWriter := io.MultiWriter(temp, candidateDigest)')
    expect(durableStoreSource).toContain('json.NewEncoder(candidateWriter)')
    expect(durableStoreSource).toContain('encoder.Encode(event)')
    expect(durableStoreSource).toContain('NewlineTerminated')
    expect(durableStoreSource).toContain('LoadEventsSince')
    expect(durableStoreSource).toContain('sort.SliceStable')
    expect(durableStoreSource).toContain('highestSeq')
    expect(durableStoreSource).toContain('DurableJSONLDiagnostic')
    expect(durableStoreSource).toContain('persist')
    expect(durableStoreSource).toContain('publish')
    expect(durableStoreSource).toContain('isInsideTempDir')
    expect(durableStoreSource).not.toMatch(/os\.Getenv|LookupEnv|ANALYTIX_API_KEY|OPENAI_API_KEY|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY|http\.DefaultClient/)
    expect(durableTestSource).toContain('TestTempDurableStoreAppendReplayMalformedAndConcurrency')
    expect(durableTestSource).toContain('TestLiveLocalSidecarTempDurableRoutesMatchContract')
    expect(durableCommandSource).toContain('ANALYTIX_SIDECAR_READY')
    expect(durableCommandSource).toContain('DurableTempDir')
    expect(durableSidecarTestSource).toContain('go-durable-sidecar-contract.json')
    expect(durableSidecarTestSource).toContain('startGoSidecar')
    expect(durableSidecarTestSource).toContain('/v1/conformance/durable')
    expect(durableSidecarTestSource).toContain('last-event-id')
    expect(durableContractSource).toContain('"stage": "D-0236"')
    expect(durableContractSource).toContain('"deepseek"')
    expect(durableContractSource).toContain('"openai"')
    expect(durableContractSource).toContain('"anthropic"')
    expect(testSource).toContain('TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptContract')
    expect(testSource).toContain('TestLiveLocalSidecarMutationsDoNotTouchFilesystem')
    expect(testSource).toContain('TestLiveLocalSidecarG3ProviderUsageAndCacheReplayMatchesTypeScriptContract')
    expect(testSource).toContain('TestLiveLocalSidecarG3ProviderRequestShapeReplayMatchesTypeScriptContract')
    expect(testSource).toContain('TestLiveLocalSidecarG3ProviderStreamingReplayMatchesTypeScriptContract')
    expect(testSource).toContain('TestLiveLocalSidecarG4ApprovalUserInputReplayMatchesTypeScriptContract')
    expect(testSource).toContain('TestLiveLocalSidecarG4MCPReplayMatchesTypeScriptContract')
    expect(testSource).toContain('httptest.NewServer(harness)')
    expect(testSource).toContain('events.jsonl')
    expect(testSource).toContain('/v1/runtime/go')
    expect(testSource).toContain('/v1/conformance/g3/provider/stream')
    expect(testSource).toContain('/v1/conformance/g4/manager/mcp/diagnostics')
    expect(testSource).toContain('g5.ControlExecutableCases.ProductBoundary.Expected')
    expect(g5.controlExecutableCases.productBoundary.expected.electronMainConnected).toBe(false)
    expect(g5.controlExecutableCases.productBoundary.expected.defaultGoBackendEnabled).toBe(false)
    expect(g5.controlExecutableCases.productBoundary.expected.rendererVisibleGoRoutesAllowed).toBe(false)
  })

  it('keeps the D-0237 Go minimal agent loop contract-only and fixture backed', () => {
    const loop = loadGoMinimalAgentLoopContract()
    const providerCache = loadProviderCacheContract()
    const g4 = loadGoG4ToolsContract()
    const approvalUserInput = loadApprovalUserInputRouteContract()
    const mcp = loadMcpToolLifecycleContract()
    const source = repoSource('packages/runtime-go/contract_shims_test.go')
    const harnessSource = repoSource('packages/runtime-go/internal/conformance/livelocal/harness.go')
    const boundarySource = repoSource('packages/runtime-go/internal/contracts/boundary.go')
    const loopSource = repoSource('packages/runtime-go/internal/conformance/livelocal/loop_handler.go')
    const loopTestSource = repoSource('packages/runtime-go/loop_replay_contract_test.go')
    const sidecarSource = repoSource('packages/runtime-go/cmd/contract-sidecar/main.go')
    const sidecarConformanceSource = repoSource('packages/runtime/tests/go-minimal-agent-loop-conformance.test.ts')
    const loopContractSource = repoSource('packages/runtime/src/conformance/fixtures/go-minimal-agent-loop-contract.json')

    expect(loop.mode).toBe('conformance-only-minimal-agent-loop')
    expect(loop.productBoundary).toMatchObject({
      rendererPreloadMainBridgeUnchanged: true,
      analytixServeContractUnchanged: true,
      reasonixPublicProtocolAllowed: false,
      defaultGoBackendAllowed: false,
      rendererVisibleGoRoutesAllowed: false,
      tempDurableStorePrototype: true,
      minimalAgentLoopPrototype: true,
      fixtureBackedLoopOnly: true
    })
    expect(loop.stablePrefix.prefixHash).toBe(providerCache.stablePrefix.firstShape.prefixHash)
    expect(loop.stablePrefix.systemHash).toBe(providerCache.stablePrefix.firstShape.systemHash)
    expect(loop.stablePrefix.dynamicStateInStablePrefix).toBe(false)
    expect(loop.stablePrefix.toolNames).toEqual(g4.toolCatalog.advertisedToolNames)
    expect(loop.modelRequestShape.caseId).toBe('deepseek-chat-request-shape')
    expect(loop.modelRequestShape.externalNetworkUsed).toBe(false)
    expect(loop.modelRequestShape.apiKeyRead).toBe(false)
    expect(loop.providerCacheTelemetry.expectedProviders).toEqual(['anthropic', 'deepseek', 'openai'])
    expect(loop.providerCacheTelemetry.cacheTelemetryLost).toBe(false)
    expect(loop.providerCacheTelemetry.unsupportedProvidersCountedAsMisses).toBe(false)
    expect(loop.approvalDenied.approvalId).toBe(approvalUserInput.approval.id)
    expect(loop.approvalDenied.mustNotExecuteDeniedTool).toBe(true)
    expect(loop.userInputGates.submittedInputId).toBe(approvalUserInput.submittedUserInput.id)
    expect(loop.userInputGates.cancelledInputId).toBe(approvalUserInput.userInput.id)
    expect(loop.userInputGates.submittedAnswersPersistedInEvents).toBe(false)
    expect(loop.mcpToolCatalog.providerId).toBe(mcp.providerId)
    expect(loop.mcpToolCatalog.toolNames).toEqual(g4.mcp.searchMetaTools.toolNames)
    expect(loop.mcpToolCatalog.mcpConnectionUsed).toBe(false)
    expect(loop.control.stepLimit.stepLimitHit).toBe(true)
    expect(loop.control.stepLimit.dynamicStateInStablePrefix).toBe(false)
    expect(loop.control.cancel.recordsNoToolExecution).toBe(true)
    expect(loop.control.resume.recoveredStateMustMatch).toBe(true)
    expect(loop.expected.sideEffects).toEqual({
      providerCallAttempts: 0,
      toolExecutionAttempts: 0,
      approvalExecutionAttempts: 0,
      mcpConnectionAttempts: 0,
      credentialReadAttempts: 0,
      fileMutationAttempts: 0,
      realWorkspaceWriteAttempts: 0
    })

    expect(harnessSource).toMatch(/LoopContract\s+agent\.GoMinimalAgentLoopContract/)
    expect(source).toContain('LiveLocalSidecarLoopProductBoundary')
    expect(boundarySource).toContain('MinimalAgentLoopPrototype')
    expect(loopSource).toContain('/v1/conformance/loop')
    expect(loopSource).toContain('NewLiveLocalLoopHandler')
    expect(loopSource).toContain('recordDrafts')
    expect(loopSource).toContain('store.RecordEvent')
    expect(loopSource).toContain('WriteDurableSSE')
    expect(loopSource).toContain('AllPersistBeforePublish')
    expect(loopSource).toContain('"providerLiveCallsAllowed"')
    expect(loopSource).toContain('"toolExecutionAllowed"')
    expect(loopSource).toContain('"approvalExecutionAllowed"')
    expect(loopSource).toContain('"mcpConnectionAllowed"')
    expect(loopSource).not.toMatch(/http\.DefaultClient|os\.Getenv|LookupEnv|ANALYTIX_API_KEY|OPENAI_API_KEY|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY/)
    expect(loopTestSource).toContain('TestLiveLocalMinimalAgentLoopRoutesMatchContract')
    expect(sidecarSource).toContain('go-minimal-agent-loop-contract.json')
    expect(sidecarSource).toContain('ProviderContract')
    expect(sidecarSource).toContain('G4Contract')
    expect(sidecarSource).toContain('MCPToolLifecycleContract')
    expect(sidecarConformanceSource).toContain('startGoSidecar')
    expect(sidecarConformanceSource).toContain('/v1/conformance/loop/run')
    expect(sidecarConformanceSource).toContain('/v1/conformance/loop/threads/')
    expect(loopContractSource).toContain('"stage": "D-0237"')
    expect(loopContractSource).toContain('"deepseek"')
    expect(loopContractSource).toContain('"openai"')
    expect(loopContractSource).toContain('"anthropic"')
  })
})
