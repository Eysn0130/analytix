import { buildRouter } from '../src/server-test-support/routes/index.js'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemoryApprovalGate } from '../src/adapters/in-memory-approval-gate.js'
import { InMemoryUserInputGate } from '../src/adapters/in-memory-user-input-gate.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { LocalToolHost, defaultLocalTools } from '../src/tool-test-support/tool/local-tool-host.js'
import { LocalWorkspaceInspector } from '../src/adapters/workspace/local-workspace-inspector.js'
import { TurnService } from '../src/services-test-support/turn-service.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'
import { UsageService } from '../src/services-test-support/usage-service.js'
import { CheckpointRewindService } from '../src/services-test-support/checkpoint-rewind-service.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { createRemoteEntryControlPort } from '../src/services-test-support/remote-entry-control-port.js'
import { InflightTracker } from '../src/loop-test-support/inflight-tracker.js'
import { SteeringQueue } from '../src/loop-test-support/steering-queue.js'
import { ContextCompactor } from '../src/shared/context-compactor.js'
import { createImmutablePrefix } from '../src/cache/immutable-prefix.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import type { ServerRuntime } from '../src/server-test-support/routes/server-runtime.js'
import { AgentLoop } from '../src/loop-test-support/agent-loop.js'
import type { ModelClient, ModelRequest, ModelStreamChunk } from '../src/ports/model-client.js'
import { createApprovalRequest } from '../src/domain/approval.js'
import { encodeSseEvent } from '../src/server-test-support/sse.js'
import type { UsageSnapshot } from '../src/contracts/usage.js'
import { buildRuntimeCapabilityManifest } from '../src/contracts/capabilities.js'
import type { RuntimeCapabilityState } from '../src/contracts/capabilities.js'
import type { RuntimeInfoResponse } from '../src/contracts/runtime-info.js'
import { modelCapabilitiesForModel } from '../src/shared/model-context-profile.js'

function makeModel(chunks: ModelStreamChunk[]): ModelClient {
  return {
    provider: 'fake',
    model: 'fake',
    async *stream(): AsyncIterable<ModelStreamChunk> {
      for (const chunk of chunks) yield chunk
    }
  }
}

export function usageSnapshot(overrides: Partial<UsageSnapshot> = {}): UsageSnapshot {
  const promptTokens = overrides.promptTokens ?? 10
  const completionTokens = overrides.completionTokens ?? 5
  const snapshot: UsageSnapshot = {
    promptTokens,
    completionTokens,
    totalTokens: overrides.totalTokens ?? promptTokens + completionTokens,
    cachedTokens: overrides.cachedTokens ?? 2,
    cacheHitTokens: overrides.cacheHitTokens ?? 2,
    cacheMissTokens: overrides.cacheMissTokens ?? Math.max(promptTokens - 2, 0),
    cacheHitRate: 'cacheHitRate' in overrides ? overrides.cacheHitRate ?? null : (promptTokens > 0 ? 2 / promptTokens : null),
    turns: overrides.turns ?? 1
  }
  if (overrides.costUsd !== undefined) snapshot.costUsd = overrides.costUsd
  if (overrides.costCny !== undefined) snapshot.costCny = overrides.costCny
  if (overrides.cacheSavingsUsd !== undefined) snapshot.cacheSavingsUsd = overrides.cacheSavingsUsd
  if (overrides.cacheSavingsCny !== undefined) snapshot.cacheSavingsCny = overrides.cacheSavingsCny
  if (overrides.tokenEconomySavingsTokens !== undefined) {
    snapshot.tokenEconomySavingsTokens = overrides.tokenEconomySavingsTokens
  }
  if (overrides.tokenEconomySavingsUsd !== undefined) {
    snapshot.tokenEconomySavingsUsd = overrides.tokenEconomySavingsUsd
  }
  if (overrides.tokenEconomySavingsCny !== undefined) {
    snapshot.tokenEconomySavingsCny = overrides.tokenEconomySavingsCny
  }
  if (overrides.hasError !== undefined) snapshot.hasError = overrides.hasError
  if (overrides.reasoningTokens !== undefined) snapshot.reasoningTokens = overrides.reasoningTokens
  if (overrides.cacheableTokenHitRate !== undefined) {
    snapshot.cacheableTokenHitRate = overrides.cacheableTokenHitRate
  }
  if (overrides.totalInputTokenHitRate !== undefined) {
    snapshot.totalInputTokenHitRate = overrides.totalInputTokenHitRate
  }
  if (overrides.cacheMissReasons !== undefined) {
    snapshot.cacheMissReasons = [...overrides.cacheMissReasons]
  }
  if (overrides.cacheSuggestions !== undefined) {
    snapshot.cacheSuggestions = [...overrides.cacheSuggestions]
  }
  return snapshot
}

type Harness = {
  runtime: ServerRuntime
  approvalGate: InMemoryApprovalGate
  userInputGate: InMemoryUserInputGate
  router: ReturnType<typeof buildRouter>
  threadService: ThreadService
  turnService: TurnService
  bus: InMemoryEventBus
  sessionStore: InMemorySessionStore
  loop: AgentLoop
  threadStore: InMemoryThreadStore
  inflight: InflightTracker
  steering: SteeringQueue
  nowIso: () => string
}

type HarnessOptions = {
  nowIso?: () => string
  model?: ModelClient
}

export function buildHarness(options: HarnessOptions = {}): Harness {
  const bus = new InMemoryEventBus()
  const approvalGate = new InMemoryApprovalGate()
  const userInputGate = new InMemoryUserInputGate()
  const threadStore = new InMemoryThreadStore()
  const sessionStore = new InMemorySessionStore()
  const inflight = new InflightTracker()
  const steering = new SteeringQueue()
  const compactor = new ContextCompactor()
  const toolHost = new LocalToolHost({ tools: defaultLocalTools })
  const usage = new UsageService()
  const prefix = createImmutablePrefix({ systemPrompt: 'be brief' })
  const nowIso = options.nowIso ?? (() => new Date().toISOString())
  const allocateSeq = (threadId: string) => bus.allocateSeq(threadId)
  const events = new RuntimeEventRecorder({ eventBus: bus, sessionStore, allocateSeq, nowIso })
  const ids = new SequentialIdGenerator()
  const turnService = new TurnService({
    threadStore,
    sessionStore,
    events,
    inflight,
    steering,
    compactor,
    ids,
    nowIso
  })
  const threadService = new ThreadService({ threadStore, sessionStore, events, ids, nowIso })
  const checkpointRewindService = new CheckpointRewindService({ threadService, sessionStore, events, nowIso })
  const model = options.model ?? makeModel([{ kind: 'completed', stopReason: 'stop' }])
  const loop = new AgentLoop({
    threadStore,
    sessionStore,
    approvalGate,
    userInputGate,
    model,
    toolHost,
    usage,
    events,
    turns: turnService,
    inflight,
    steering,
    compactor,
    prefix,
    ids,
    nowIso
  })
  const startedAt = nowIso()
  const modelId = 'deepseek-chat'
  const capabilities = buildRuntimeCapabilityManifest({
    model: modelCapabilitiesForModel(modelId)
  })
  const publicCapabilityState = (state: RuntimeCapabilityState) => ({
    status: state.available ? 'available' as const : state.enabled ? 'unavailable' as const : 'disabled' as const,
    enabled: state.enabled,
    available: state.available,
    reasonCode: state.available ? 'available' as const : state.enabled ? 'unavailable' as const : 'disabled_by_config' as const
  })
  const runtimeInfo = (): RuntimeInfoResponse => ({
    schemaVersion: 2,
    status: 'ready',
    listenerScope: 'loopback',
    port: 0,
    startedAt,
    insecure: false,
    storage: { configured: true, available: true },
    executionPolicy: {
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    },
    provider: {
      id: 'deepseek',
      model: modelId,
      family: 'deepseek',
      endpointFormat: 'chat_completions',
      available: false,
      apiKeyConfigured: false,
      baseUrlConfigured: true,
      cacheTelemetrySupported: true,
      supportsImageInput: capabilities.model.supportsImageInput ?? false,
      contextWindowTokens: capabilities.model.contextWindowTokens
    },
    networkProxy: {
      mode: 'auto',
      configured: false,
      source: 'unknown',
      valid: false,
      credentialsMasked: false
    },
    capabilities: {
      contractVersion: 1,
      model: {
        ...capabilities.model,
        supportsImageInput: capabilities.model.supportsImageInput ?? false
      },
      cli: {
        serve: publicCapabilityState(capabilities.cli.serve),
        run: publicCapabilityState(capabilities.cli.run),
        chat: publicCapabilityState(capabilities.cli.chat),
        exec: publicCapabilityState(capabilities.cli.exec)
      },
      mcp: {
        ...publicCapabilityState(capabilities.mcp),
        configuredServers: capabilities.mcp.configuredServers,
        connectedServers: capabilities.mcp.connectedServers,
        toolCount: capabilities.mcp.toolCount,
        promptCount: capabilities.mcp.promptCount,
        resourceCount: capabilities.mcp.resourceCount,
        catalog: {
          status: capabilities.mcp.catalog.status === 'disabled' ? 'disabled' : 'unavailable',
          reasonCode: capabilities.mcp.catalog.status === 'disabled' ? 'disabled_by_config' : 'unavailable',
          toolCount: capabilities.mcp.catalog.toolCount,
          advertisedToolCount: capabilities.mcp.catalog.advertisedToolCount ?? 0,
          promptCount: capabilities.mcp.catalog.promptCount,
          resourceCount: capabilities.mcp.catalog.resourceCount,
          catalogDrift: capabilities.mcp.catalog.catalogDrift ?? false
        },
        search: {
          enabled: capabilities.mcp.search.enabled,
          mode: capabilities.mcp.search.mode,
          active: capabilities.mcp.search.active,
          available: capabilities.mcp.available,
          reasonCode: capabilities.mcp.available ? 'available' : capabilities.mcp.enabled ? 'unavailable' : 'disabled_by_config',
          indexedToolCount: capabilities.mcp.search.indexedToolCount,
          advertisedToolCount: capabilities.mcp.search.advertisedToolCount,
          autoThresholdToolCount: capabilities.mcp.search.autoThresholdToolCount ?? 0,
          topKDefault: capabilities.mcp.search.topKDefault ?? 0,
          topKMax: capabilities.mcp.search.topKMax ?? 0,
          minScore: capabilities.mcp.search.minScore ?? 0,
          catalogDrift: capabilities.mcp.search.catalogDrift ?? false
        }
      },
      web: {
        ...publicCapabilityState(capabilities.web),
        fetch: publicCapabilityState(capabilities.web.fetch),
        search: publicCapabilityState(capabilities.web.search),
        provider: capabilities.web.provider ?? 'unknown',
        fetchEnabled: capabilities.web.fetchEnabled ?? false,
        searchEnabled: capabilities.web.searchEnabled ?? false
      },
      skills: {
        ...publicCapabilityState(capabilities.skills),
        configuredRoots: capabilities.skills.configuredRoots,
        discoveredSkills: capabilities.skills.discoveredSkills
      },
      subagents: {
        ...publicCapabilityState(capabilities.subagents),
        maxParallel: capabilities.subagents.maxParallel,
        maxChildRuns: capabilities.subagents.maxChildRuns,
        defaultToolPolicy: capabilities.subagents.defaultToolPolicy,
        profileCount: capabilities.subagents.profiles.length,
        internalLineageAvailable: capabilities.subagents.internalLineageAvailable ?? false,
        profilesAvailable: capabilities.subagents.profilesAvailable ?? false,
        durableChildRunStore: capabilities.subagents.durableChildRunStore ?? false,
        parallelExecutionAvailable: capabilities.subagents.parallelExecutionAvailable ?? false,
        taskToolAvailable: capabilities.subagents.taskToolAvailable ?? false,
        parallelTasksToolAvailable: capabilities.subagents.parallelTasksToolAvailable ?? false,
        backgroundTaskJobsAvailable: capabilities.subagents.backgroundTaskJobsAvailable ?? false,
        backgroundShellAvailable: capabilities.subagents.backgroundShellAvailable ?? false,
        backgroundSubagentJobsAvailable: capabilities.subagents.backgroundSubagentJobsAvailable ?? false,
        modelJobToolsAvailable: capabilities.subagents.modelJobToolsAvailable ?? false,
        taskJobThreadScopeSupported: capabilities.subagents.taskJobThreadScopeSupported ?? false
      },
      attachments: {
        ...publicCapabilityState(capabilities.attachments),
        maxImageBytes: capabilities.attachments.maxImageBytes,
        maxImageDimension: capabilities.attachments.maxImageDimension,
        allowedMimeTypes: capabilities.attachments.allowedMimeTypes,
        allowedDocumentMimeTypes: capabilities.attachments.allowedDocumentMimeTypes,
        maxDocumentBytes: capabilities.attachments.maxDocumentBytes,
        maxDocumentTextChars: capabilities.attachments.maxDocumentTextChars,
        textFallbackMaxBase64Bytes: capabilities.attachments.textFallbackMaxBase64Bytes,
        textFallbackMaxImageDimension: capabilities.attachments.textFallbackMaxImageDimension,
        textFallbackPreferredMimeType: capabilities.attachments.textFallbackPreferredMimeType
      },
      memory: {
        ...publicCapabilityState(capabilities.memory),
        mode: 'manual',
        storeOnly: true,
        modelInjection: false,
        automaticCapture: false,
        scopes: capabilities.memory.scopes,
        maxInjectedRecords: capabilities.memory.maxInjectedRecords
      },
      imageGen: { ...publicCapabilityState(capabilities.imageGen), ...(capabilities.imageGen.model ? { model: capabilities.imageGen.model } : {}) },
      speechGen: { ...publicCapabilityState(capabilities.speechGen), ...(capabilities.speechGen.model ? { model: capabilities.speechGen.model } : {}) },
      musicGen: { ...publicCapabilityState(capabilities.musicGen), ...(capabilities.musicGen.model ? { model: capabilities.musicGen.model } : {}) },
      videoGen: { ...publicCapabilityState(capabilities.videoGen), ...(capabilities.videoGen.model ? { model: capabilities.videoGen.model } : {}) },
      computerUse: {
        ...publicCapabilityState(capabilities.computerUse),
        mode: capabilities.computerUse.mode,
        ...(capabilities.computerUse.backendId ? { backendId: capabilities.computerUse.backendId } : {}),
        ...(capabilities.computerUse.preferredBackendId ? { preferredBackendId: capabilities.computerUse.preferredBackendId } : {})
      },
      visionBridge: {
        ...publicCapabilityState(capabilities.visionBridge),
        mode: capabilities.visionBridge.mode,
        ...(capabilities.visionBridge.model ? { model: capabilities.visionBridge.model } : {}),
        ...(capabilities.visionBridge.providerId ? { providerId: capabilities.visionBridge.providerId } : {}),
        maxScreenshotsPerTurn: capabilities.visionBridge.maxScreenshotsPerTurn,
        maxImagesPerTurn: capabilities.visionBridge.maxImagesPerTurn ?? 0,
        maxImageBytes: capabilities.visionBridge.maxImageBytes ?? 0,
        maxImageDimension: capabilities.visionBridge.maxImageDimension ?? 0,
        semanticProbeStatus: capabilities.visionBridge.semanticProbeStatus
      }
    }
  })
  const runTurn = (threadId: string, turnId: string) => {
    void loop.runTurn(threadId, turnId)
  }
  const remoteEntryControl = createRemoteEntryControlPort({
    info: runtimeInfo,
    turns: turnService,
    approvals: approvalGate,
    userInputs: userInputGate,
    events,
    runTurn
  })
  const runtime: ServerRuntime = {
    threadService,
    turnService,
    usageService: usage,
    checkpointRewindService,
    eventBus: bus,
    sessionStore,
    events,
    approvalGate,
    userInputGate,
    workspaceInspector: new LocalWorkspaceInspector(),
    remoteEntryControl,
    runTurn,
    runtimeToken: 'tok-1',
    insecure: false,
    allocateSeq,
    nowIso,
    info: runtimeInfo
  }
  return {
    runtime,
    approvalGate,
    userInputGate,
    router: buildRouter(runtime),
    threadService,
    turnService,
    bus,
    sessionStore,
    loop,
    threadStore,
    inflight,
    steering,
    nowIso
  }
}

export async function readJson(response: Response): Promise<unknown> {
  return JSON.parse(await response.text())
}

export async function readSseEvents(response: Response, options: { idleMs?: number } = {}): Promise<string[]> {
  const reader = response.body?.getReader()
  if (!reader) return []
  const decoder = new TextDecoder()
  let buffer = ''
  const events: string[] = []
  let lastChunkAt = Date.now()
  const idleMs = options.idleMs ?? 50
  while (true) {
    const timeout = new Promise<{ value: undefined; done: true }>((resolve) =>
      setTimeout(() => resolve({ value: undefined, done: true }), idleMs)
    )
    const next = await Promise.race([reader.read(), timeout])
    if (!next || next.done) break
    if (next.value) {
      lastChunkAt = Date.now()
      buffer += decoder.decode(next.value, { stream: true })
      let boundary: number
      while ((boundary = buffer.indexOf('\n\n')) >= 0) {
        const frame = buffer.slice(0, boundary)
        if (!frame.startsWith(':')) events.push(frame)
        buffer = buffer.slice(boundary + 2)
      }
    } else if (Date.now() - lastChunkAt > idleMs) {
      break
    }
  }
  try {
    reader.releaseLock()
  } catch {
    // ignore
  }
  return events
}
