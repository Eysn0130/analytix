import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  getModelProviderPreset,
  modelProviderPresetProfile,
  modelProviderTokenPlanProfile,
  type AppSettingsV1
} from '@shared/app-settings'
import { AnalytixRuntimeProvider } from './analytix-runtime'
import { getProvider, resetProviderCacheForTests } from './registry'
import { rendererRuntimeClient } from './runtime-client'
import type { AcceptedFinalProjectionBatch, ThreadEventSink } from './types'
import {
  markPublicProjectionRevoked,
  resetPublicProjectionRevocationsForTests
} from '../lib/public-projection-revocation'

function settings(): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

function diagnosticsState(enabled: boolean, available: boolean) {
  return available
    ? { status: 'available', enabled: true, available: true, reasonCode: 'available' }
    : enabled
      ? { status: 'unavailable', enabled: true, available: false, reasonCode: 'unavailable' }
      : { status: 'disabled', enabled: false, available: false, reasonCode: 'disabled_by_config' }
}

function runtimeInfoV2Fixture(): Record<string, unknown> {
  const available = diagnosticsState(true, true)
  const disabled = diagnosticsState(false, false)
  const search = {
    enabled: false,
    mode: 'auto',
    active: false,
    available: false,
    reasonCode: 'disabled_by_config',
    indexedToolCount: 0,
    advertisedToolCount: 0,
    autoThresholdToolCount: 24,
    topKDefault: 8,
    topKMax: 24,
    minScore: 0,
    catalogDrift: false
  }
  return {
    schemaVersion: 2,
    status: 'ready',
    listenerScope: 'loopback',
    port: 17878,
    startedAt: '2024-01-01T00:00:00.000Z',
    insecure: false,
    storage: { configured: true, available: true },
    executionPolicy: { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' },
    provider: {
      id: 'deepseek',
      model: 'deepseek-chat',
      family: 'deepseek',
      endpointFormat: 'chat_completions',
      available: true,
      apiKeyConfigured: true,
      baseUrlConfigured: true,
      cacheTelemetrySupported: true,
      supportsImageInput: true,
      reasoningEffort: 'high'
    },
    networkProxy: { mode: 'off', configured: false, source: 'unknown', valid: true, credentialsMasked: true },
    capabilities: {
      contractVersion: 1,
      model: {
        id: 'deepseek-chat',
        providerId: 'deepseek',
        family: 'deepseek',
        inputModalities: ['text', 'image'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        supportsImageInput: true,
        messageParts: ['text', 'image_url'],
        reasoning: {
          supportedEfforts: ['off', 'high', 'max'],
          defaultEffort: 'high',
          requestProtocol: 'deepseek-chat-completions'
        },
        endpointFormat: 'chat_completions'
      },
      cli: { serve: available, run: available, chat: available, exec: available },
      mcp: {
        ...disabled,
        configuredServers: 0,
        connectedServers: 0,
        toolCount: 0,
        promptCount: 0,
        resourceCount: 0,
        catalog: {
          status: 'disabled', reasonCode: 'disabled_by_config', toolCount: 0, advertisedToolCount: 0,
          promptCount: 0, resourceCount: 0, catalogDrift: false
        },
        search
      },
      web: { ...disabled, fetch: disabled, search: disabled, provider: 'none', fetchEnabled: false, searchEnabled: false },
      skills: { ...disabled, configuredRoots: 0, discoveredSkills: 0 },
      subagents: {
        ...disabled,
        maxParallel: 0,
        maxChildRuns: 0,
        defaultToolPolicy: 'readOnly',
        profileCount: 0,
        internalLineageAvailable: false,
        profilesAvailable: false,
        durableChildRunStore: false,
        parallelExecutionAvailable: false,
        taskToolAvailable: false,
        parallelTasksToolAvailable: false,
        backgroundTaskJobsAvailable: false,
        backgroundShellAvailable: false,
        backgroundSubagentJobsAvailable: false,
        modelJobToolsAvailable: false,
        taskJobThreadScopeSupported: false
      },
      attachments: {
        ...available,
        maxImageBytes: 5242880,
        maxImageDimension: 4096,
        allowedMimeTypes: ['image/png'],
        allowedDocumentMimeTypes: ['text/plain'],
        maxDocumentBytes: 5242880,
        maxDocumentTextChars: 100000,
        textFallbackMaxBase64Bytes: 524288,
        textFallbackMaxImageDimension: 1280,
        textFallbackPreferredMimeType: 'image/webp'
      },
      memory: { ...disabled, mode: 'manual', storeOnly: true, modelInjection: false, automaticCapture: false, scopes: ['user'], maxInjectedRecords: 0 },
      imageGen: disabled,
      speechGen: disabled,
      musicGen: disabled,
      videoGen: disabled,
      computerUse: { ...disabled, mode: 'off' },
      visionBridge: {
        ...disabled,
        mode: 'off',
        maxScreenshotsPerTurn: 0,
        maxImagesPerTurn: 0,
        maxImageBytes: 0,
        maxImageDimension: 0,
        semanticProbeStatus: 'unknown'
      }
    }
  }
}

function runtimeToolsV2Fixture(): Record<string, unknown> {
  const disabled = diagnosticsState(false, false)
  return {
    schemaVersion: 2,
    providerCount: 1,
    toolContracts: { count: 0, catalogHash: 'a'.repeat(64) },
    mcpServers: [],
    mcpSearch: {
      enabled: false,
      mode: 'auto',
      active: false,
      available: false,
      reasonCode: 'disabled_by_config',
      indexedToolCount: 0,
      advertisedToolCount: 0,
      autoThresholdToolCount: 24,
      topKDefault: 8,
      topKMax: 24,
      minScore: 0,
      catalogDrift: false
    },
    mcpPromptCount: 0,
    mcpResourceCount: 0,
    commands: [],
    networkProxy: { mode: 'off', configured: false, source: 'unknown', valid: true, credentialsMasked: true },
    webProviderCount: 0,
    skills: { enabled: false, available: false, reasonCode: 'disabled_by_config', configuredRootCount: 0, skillCount: 0, validationErrorCount: 0 },
    attachments: {
      enabled: true,
      count: 0,
      totalBytes: 0,
      maxImageBytes: 5242880,
      maxImageDimension: 4096,
      allowedMimeTypes: ['image/png'],
      allowedDocumentMimeTypes: ['text/plain'],
      maxDocumentBytes: 5242880,
      maxDocumentTextChars: 100000
    },
    memory: { enabled: false, activeCount: 0, tombstoneCount: 0 },
    subagents: {
      ...disabled,
      active: 0,
      queued: 0,
      profileCount: 0,
      maxParallel: 0,
      maxChildRuns: 0,
      defaultToolPolicy: 'readOnly',
      internalLineageAvailable: false,
      parallelExecutionAvailable: false,
      taskToolAvailable: false,
      parallelTasksToolAvailable: false,
      backgroundTaskJobsAvailable: false,
      backgroundShellAvailable: false,
      backgroundSubagentJobsAvailable: false
    }
  }
}

async function ordinaryHistoryItem() {
  const sha256 = async (text: string) => Array.from(new Uint8Array(
    await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
  ), (byte) => byte.toString(16).padStart(2, '0')).join('')
  const text = 'Updated the parser and the focused tests pass.'
  const ordinaryResult = {
    schemaVersion: 1 as const,
    purpose: 'analytix.ordinary-result/v1' as const,
    projectionVersion: 'analytix.ordinary-output-projection/v1' as const,
    logicalEffect: 'ordinary' as const,
    ordinaryWork: true as const,
    candidateOrigin: 'provider_ordinary_only' as const,
    evidenceAuthority: false as const,
    citationAuthority: false as const,
    factAnswerAllowed: false as const,
    text,
    textSha256: await sha256(text),
    resultDigest: ''
  }
  ordinaryResult.resultDigest = await sha256('analytix/ordinary-result/v1\0' + JSON.stringify(ordinaryResult))
  return {
    id: 'item_ordinary', turnId: 'turn_ordinary', threadId: 'thr_case',
    role: 'assistant', status: 'completed', kind: 'assistant_text',
    createdAt: '2026-07-11T00:00:02Z', finishedAt: '2026-07-11T00:00:02Z',
    text, ordinaryResult
  }
}

function acceptedCaseFinalRecord() {
  const publicView = {
    schemaVersion: 2 as const,
    publicationState: 'accepted' as const,
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 1,
    datasetSnapshotId: 'snapshot-case-1',
    variant: 'SourceUnavailableAnswer' as const,
    terminalReason: 'source_unavailable' as const,
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable' as const,
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only' as const,
      count: 0,
      setDigest: '8'.repeat(64),
      citations: []
    },
    noHitWording: '' as const,
    envelopeIssuedAt: '2026-07-11T00:00:01Z',
    acceptedAt: '2026-07-11T00:00:01Z'
  }
  return {
    schemaVersion: 5 as const,
    authorityPurpose: 'analytix.case-final/v1' as const,
    authorityAlgorithm: 'Ed25519' as const,
    authorityKeyId: 'a'.repeat(64),
    authorityPublicKey: 'p'.repeat(43),
    threadId: 'thr_case',
    turnId: 'turn_case',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 1,
    datasetSnapshotId: 'snapshot-case-1',
    variant: 'SourceUnavailableAnswer' as const,
    terminalReason: 'source_unavailable' as const,
    renderedTextSha256: 'ac111a8b3a3867f57f8d4448ebf59df5bccb7a5972e85a6451a091ddd3be1d2e',
    registrySequence: 1,
    registryStateDigest: 'd'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1' as const,
    finalGateVersion: 'analytix.final-evidence-gate/v4' as const,
    verifierVersion: 'analytix.claim-verifier-policy/v1' as const,
    publicView,
    publicViewDigest: '7'.repeat(64),
    privateRecordDigest: 'e'.repeat(64),
    acceptedAt: '2026-07-11T00:00:01Z',
    authoritySignature: 's'.repeat(86),
    recordDigest: 'f'.repeat(64)
  }
}

function acceptedCaseFinalView(record = acceptedCaseFinalRecord()) {
  return {
    ...record.publicView,
    acceptedFinalDigest: record.recordDigest,
    publicViewDigest: record.publicViewDigest
  }
}

function acceptedCaseFinalViewV3(record = acceptedCaseFinalRecord()) {
  const core = record.publicView
  return {
    schemaVersion: 3 as const,
    acceptedFinalDigest: record.recordDigest,
    publicationState: 'accepted' as const,
    variant: core.variant,
    terminalReason: core.terminalReason,
    blockerCode: core.blockerCode,
    coverageStatus: core.coverageStatus,
    checkedScopeDigest: core.checkedScopeDigest,
    missingScopeCount: core.missingScopeCount,
    claimCount: core.claimCount,
    claimTypes: core.claimTypes,
    receiptMetadata: core.receiptMetadata,
    noHitWording: core.noHitWording,
    acceptedAt: core.acceptedAt
  }
}

function acceptedCaseThreadResponse(
  acceptedFinal = acceptedCaseFinalRecord(),
  acceptedFinalView: Record<string, unknown> = acceptedCaseFinalView(acceptedFinal),
  itemAcceptedFinalView: Record<string, unknown> = acceptedFinalView
) {
  return {
    id: 'thr_case',
    title: 'Case',
    workspace: '/tmp/case',
    model: 'deepseek-chat',
    mode: 'agent',
    status: 'idle',
    historyAuthority: 'case_boundary_only_v1',
    createdAt: '2026-07-11T00:00:00.000Z',
    updatedAt: '2026-07-11T00:00:01.000Z',
    latestSeq: 12,
    turns: [{
      id: 'turn_case',
      threadId: 'thr_case',
      status: 'completed',
      prompt: 'case question',
      createdAt: '2026-07-11T00:00:00.000Z',
      finishedAt: acceptedFinal.acceptedAt,
      acceptedFinal,
      acceptedFinalView,
      items: [{
        id: 'item_user',
        turnId: 'turn_case',
        threadId: 'thr_case',
        role: 'user',
        status: 'completed',
        createdAt: '2026-07-11T00:00:00.000Z',
        kind: 'user_message',
        text: 'case question'
      }, {
        id: 'item_final',
        turnId: 'turn_case',
        threadId: 'thr_case',
        role: 'assistant',
        status: 'completed',
        createdAt: acceptedFinal.acceptedAt,
        finishedAt: acceptedFinal.acceptedAt,
        kind: 'assistant_text',
        text: 'verified final',
        acceptedFinal,
        acceptedFinalView: itemAcceptedFinalView
      }]
    }]
  }
}

function acceptedFinalDeliveryBatchForRenderer() {
  const acceptedFinal = acceptedCaseFinalRecord()
  const acceptedFinalView = acceptedCaseFinalViewV3(acceptedFinal)
  const firstSeq = 20
  const lastSeq = 22
  const timestamp = acceptedFinal.acceptedAt
  const publicationCommitId = acceptedFinal.recordDigest
  const events = [
    {
      kind: 'item_completed',
      seq: firstSeq,
      timestamp,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      itemId: 'item_final',
      item: {
        id: 'item_final',
        turnId: acceptedFinal.turnId,
        threadId: acceptedFinal.threadId,
        role: 'assistant',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'assistant_text',
        text: 'verified final',
        acceptedFinalView
      },
      acceptedFinalDigest: publicationCommitId,
      publicationCommitId,
      publicationEventId: '1'.repeat(64),
      publicationSlot: 'assistant-final',
      publicationPayloadDigest: '2'.repeat(64)
    },
    {
      kind: 'usage',
      seq: firstSeq + 1,
      timestamp,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      model: 'deepseek-chat',
      usage: {
        promptTokens: 10,
        completionTokens: 3,
        totalTokens: 13,
        cacheHitRate: 0.5,
        turns: 1
      },
      cacheDiagnostics: {},
      usageFinalStatus: 'completed',
      acceptedFinalDigest: publicationCommitId,
      publicationCommitId,
      publicationEventId: '3'.repeat(64),
      publicationSlot: 'usage',
      publicationPayloadDigest: '4'.repeat(64)
    },
    {
      kind: 'turn_completed',
      seq: lastSeq,
      timestamp,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      status: 'completed',
      terminalReason: acceptedFinal.terminalReason,
      acceptedFinalDigest: publicationCommitId,
      publicationCommitId,
      publicationEventId: '5'.repeat(64),
      publicationSlot: 'terminal',
      publicationPayloadDigest: '6'.repeat(64)
    }
  ]
  const batchId = '7'.repeat(64)
  const eventManifestDigest = '8'.repeat(64)
  return {
    schemaVersion: 2,
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    kind: 'accepted_final_batch',
    batchId,
    threadId: acceptedFinal.threadId,
    turnId: acceptedFinal.turnId,
    seq: lastSeq,
    firstSeq,
    lastSeq,
    timestamp,
    publicationCommitId,
    eventManifestDigest,
    publicationAuthority: {
      schemaVersion: 'accepted-final-delivery-seal.v1',
      purpose: 'analytix.accepted-final-delivery-seal/v1',
      sealId: '9'.repeat(64),
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      publicationCommitId,
      acceptedFinalDispositionDigest: 'a'.repeat(64),
      terminalDispositionId: 'b'.repeat(64),
      eventManifestDigest,
      sequencedEventsDigest: 'c'.repeat(64),
      batchId,
      firstSeq,
      lastSeq,
      timestamp,
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: acceptedFinal.authorityKeyId,
      authorityPublicKey: acceptedFinal.authorityPublicKey,
      authoritySignature: acceptedFinal.authoritySignature
    },
    events
  }
}

function secondAcceptedFinalDeliveryBatchForRenderer() {
  const batch = structuredClone(acceptedFinalDeliveryBatchForRenderer())
  const turnId = 'turn_case_second'
  const itemId = 'item_final_second'
  const acceptedFinalDigest = 'e'.repeat(64)
  const timestamp = '2026-07-11T00:00:02Z'
  const firstSeq = batch.lastSeq + 1
  batch.batchId = 'd'.repeat(64)
  batch.turnId = turnId
  batch.firstSeq = firstSeq
  batch.lastSeq = firstSeq + batch.events.length - 1
  batch.seq = batch.lastSeq
  batch.timestamp = timestamp
  batch.publicationCommitId = acceptedFinalDigest
  batch.eventManifestDigest = 'c'.repeat(64)
  batch.publicationAuthority = {
    ...batch.publicationAuthority,
    sealId: 'b'.repeat(64),
    turnId,
    publicationCommitId: acceptedFinalDigest,
    eventManifestDigest: batch.eventManifestDigest,
    sequencedEventsDigest: 'a'.repeat(64),
    batchId: batch.batchId,
    firstSeq: batch.firstSeq,
    lastSeq: batch.lastSeq,
    timestamp
  }
  for (const [index, event] of batch.events.entries()) {
    event.seq = firstSeq + index
    event.timestamp = timestamp
    event.turnId = turnId
    event.acceptedFinalDigest = acceptedFinalDigest
    event.publicationCommitId = acceptedFinalDigest
  }
  const assistantEvent = batch.events[0] as any
  if (assistantEvent.kind !== 'item_completed' || assistantEvent.item.kind !== 'assistant_text') {
    throw new Error('accepted-final renderer fixture assistant is invalid')
  }
  assistantEvent.itemId = itemId
  assistantEvent.item = {
    ...assistantEvent.item,
    id: itemId,
    turnId,
    createdAt: timestamp,
    finishedAt: timestamp,
    text: 'second verified final',
    acceptedFinalView: {
      ...assistantEvent.item.acceptedFinalView,
      acceptedFinalDigest,
      acceptedAt: timestamp
    }
  }
  return batch
}

function generalTerminalDeliveryBatchForRenderer() {
  const timestamp = '2026-07-20T03:00:00Z'
  const events = [
    {
      kind: 'item_completed', seq: 20, timestamp, threadId: 'thr_general', turnId: 'turn_general',
      itemId: 'item_general', item: {
        id: 'item_general', turnId: 'turn_general', threadId: 'thr_general', role: 'assistant',
        status: 'completed', createdAt: timestamp, finishedAt: timestamp,
        kind: 'assistant_text',
        text: '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。'
      }
    },
    {
      kind: 'usage', seq: 21, timestamp, threadId: 'thr_general', turnId: 'turn_general',
      model: 'gpt-5', usage: {
        promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
        cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
        cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
        priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0, turns: 1
      }, cacheDiagnostics: {}, usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: 22, timestamp, threadId: 'thr_general', turnId: 'turn_general',
      status: 'completed', terminalReason: 'success'
    }
  ]
  return {
    schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch', batchDigest: 'a'.repeat(64),
    threadId: 'thr_general', turnId: 'turn_general', seq: 22, firstSeq: 20, lastSeq: 22,
    timestamp, generalTerminalCommitId: 'b'.repeat(64),
    generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: 'c'.repeat(64), eventManifestDigest: 'd'.repeat(64),
    projectedEventsDigest: 'e'.repeat(64), transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
    events,
    eventManifest: [
      { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
    ]
  }
}

function summaryResponse(threadId = 'thr_1'): Record<string, unknown> {
  return {
    threadId,
    generatedAt: '2026-06-30T00:00:00.000Z',
    latestSeq: 9,
    subagents: [{
      schemaVersion: 1,
      id: 'run:run_surface',
      key: 'run:run_surface',
      parentThreadId: threadId,
      childRunId: 'run_surface',
      childThreadId: 'thr_child_surface',
      title: 'Surface child',
      status: 'active',
      rawStatus: 'running',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canContinueParent: false,
      canReadOutput: false,
      canOpenThread: true,
      canKill: false,
      canRestart: false,
      evidenceLedgered: true,
      cacheHitRate: 0.5,
      updatedAt: '2026-06-30T00:00:00.000Z'
    }],
    tasks: [{
      schemaVersion: 1,
      id: 'taskjob:task_surface',
      kind: 'task',
      status: 'running',
      background: false,
      terminal: false,
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }],
    outputs: [],
    sources: [],
    sideChats: [],
    backgroundProcesses: []
  }
}

function withheldTaskOutput(taskId: string): Record<string, unknown> {
  return {
    schemaVersion: 1,
    availability: 'withheld',
    taskId,
    status: 'running',
    reasonCode: 'security_bound_child_output',
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false
  }
}

function taskMutation(taskId: string, status: 'killed' | 'queued' = 'killed'): Record<string, unknown> {
  return {
    task: {
      schemaVersion: 1,
      id: taskId,
      kind: 'task',
      status,
      background: false,
      active: status === 'queued',
      terminal: status === 'killed',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
  }
}

type RuntimeBridgeTestOverrides = Partial<{
  getSettings: Window['analytix']['settings']['getSettings']
  runtimeRequest: Window['analytix']['runtime']['runtimeRequest']
  startSse: Window['analytix']['runtime']['startSse']
  ackSseEvent: Window['analytix']['runtime']['ackSseEvent']
  stopSse: Window['analytix']['runtime']['stopSse']
  onSseEvent: Window['analytix']['runtime']['onSseEvent']
  onSseEnd: Window['analytix']['runtime']['onSseEnd']
  onSseError: Window['analytix']['runtime']['onSseError']
}>

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const FORBIDDEN_PUBLIC_RUNTIME_ROUTE =
  /^\/v1\/(?:reasonix(?:\/|$)|runtime\/go(?:\/|$)|workflows?(?:\/|$)|create-loop(?:\/|$)|subagents?(?:\/|$)|autoresearch(?:\/|$)|mcp-indexer(?:\/|$))/

function installDsGui(overrides: RuntimeBridgeTestOverrides): void {
  const getSettings = overrides.getSettings ?? vi.fn(async () => settings())
  const runtimeRequest = overrides.runtimeRequest ?? vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
  const startSse = overrides.startSse ?? vi.fn(async (_threadId: string, _sinceSeq: number, streamId?: string) => ({
    streamId: streamId ?? 'stream-1'
  }))
  const ackSseEvent = overrides.ackSseEvent ?? vi.fn(async () => true)
  const stopSse = overrides.stopSse ?? vi.fn(async () => true)
  const onSseEvent = overrides.onSseEvent ?? vi.fn(() => () => undefined)
  const onSseEnd = overrides.onSseEnd ?? vi.fn(() => () => undefined)
  const onSseError = overrides.onSseError ?? vi.fn(() => () => undefined)
  vi.stubGlobal('window', {
    analytix: {
      settings: { getSettings },
      runtime: { runtimeRequest, startSse, ackSseEvent, stopSse, onSseEvent, onSseEnd, onSseError }
    } as unknown as Window['analytix'],
    get kun() {
      throw new Error('legacy runtime provider alias should not be read')
    },
    get reasonix() {
      throw new Error('legacy runtime provider alias should not be read')
    }
  })
}

function expectRuntimeRequestPathsStayAnalytixOwned(paths: string[]): void {
  expect(paths.length).toBeGreaterThan(0)
  for (const path of paths) {
    expect(path).not.toMatch(FORBIDDEN_PUBLIC_RUNTIME_ROUTE)
    expect(path === '/health' || path.startsWith('/v1/')).toBe(true)
  }
}

afterEach(() => {
  rendererRuntimeClient.invalidateSettings()
  resetPublicProjectionRevocationsForTests()
  vi.unstubAllGlobals()
})

describe('AnalytixRuntimeProvider', () => {
  it('reports the analytix id and Analytix display name', () => {
    const provider = new AnalytixRuntimeProvider()
    expect(provider.id).toBe('analytix')
    expect(provider.displayName).toBe('Analytix')
  })

  it('exposes the local HTTP/SSE capabilities', () => {
    const provider = new AnalytixRuntimeProvider()
    const caps = provider.getCapabilities()
    expect(caps.stream).toBe(true)
    expect(caps.interrupt).toBe(true)
    expect(caps.approvals).toBe(true)
  })

  it('reports invalid runtime JSON responses with a stable error message', async () => {
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: '{not-json'
      }))
    })
    const provider = new AnalytixRuntimeProvider()

    await expect(provider.listThreads()).rejects.toThrow(
      'Runtime response failed schema validation.'
    )
  })

  it('rejects marker-free arbitrary task output before it reaches renderer consumers', async () => {
    const privateOutputSentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          availability: 'available',
          taskId: 'taskjob:job-private-output',
          status: 'completed',
          output: privateOutputSentinel,
          offset: 0,
          nextOffset: privateOutputSentinel.length,
          outputBytes: privateOutputSentinel.length,
          truncated: false
        })
      }))
    })
    const provider = new AnalytixRuntimeProvider()

    const error = await provider.getThreadSummaryTaskOutput('thr_1', 'taskjob:job-private-output')
      .then(() => null, (reason: unknown) => reason)

    expect(error).toBeInstanceOf(Error)
    expect(String(error)).toContain('Runtime response failed schema validation.')
    expect(String(error)).not.toContain(privateOutputSentinel)
  })

  it('rejects a validly shaped summary returned for another thread', async () => {
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify(summaryResponse('thr_foreign'))
      }))
    })
    const provider = new AnalytixRuntimeProvider()

    await expect(provider.getThreadSummary('thr_expected')).rejects.toThrow(
      'Runtime response failed schema validation.'
    )
  })

  it.each(['output', 'kill', 'restart'] as const)(
    'rejects a validly shaped %s response returned for another task',
    async (operation) => {
      installDsGui({
        runtimeRequest: vi.fn(async () => ({
          ok: true,
          status: 200,
          body: JSON.stringify(
            operation === 'output'
              ? withheldTaskOutput('taskjob:foreign')
              : taskMutation('taskjob:foreign', operation === 'restart' ? 'queued' : 'killed')
          )
        }))
      })
      const provider = new AnalytixRuntimeProvider()
      const request = operation === 'output'
        ? provider.getThreadSummaryTaskOutput('thr_expected', 'taskjob:expected')
        : operation === 'kill'
          ? provider.killThreadSummaryTask('thr_expected', 'taskjob:expected')
          : provider.restartThreadSummaryTask('thr_expected', 'taskjob:expected')

      await expect(request).rejects.toThrow('Runtime response failed schema validation.')
    }
  )

  it.each(['kill', 'restart'] as const)(
    'does not issue a %s request after public projection authority is revoked',
    async (operation) => {
      const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
      installDsGui({ runtimeRequest })
      markPublicProjectionRevoked('thr_revoked')
      const provider = new AnalytixRuntimeProvider()
      const request = operation === 'kill'
        ? provider.killThreadSummaryTask('thr_revoked', 'taskjob:expected')
        : provider.restartThreadSummaryTask('thr_revoked', 'taskjob:expected')

      await expect(request).rejects.toThrow('Public case projection authority was revoked.')
      expect(runtimeRequest).not.toHaveBeenCalled()
    }
  )

  it.each(['kill', 'restart'] as const)(
    'rejects a %s result when authority is revoked while the request is in flight',
    async (operation) => {
      const pending = deferred<{ ok: boolean; status: number; body: string }>()
      const runtimeRequest = vi.fn(() => pending.promise)
      installDsGui({ runtimeRequest })
      const provider = new AnalytixRuntimeProvider()
      const request = operation === 'kill'
        ? provider.killThreadSummaryTask('thr_revoke_race', 'taskjob:expected')
        : provider.restartThreadSummaryTask('thr_revoke_race', 'taskjob:expected')
      await vi.waitFor(() => expect(runtimeRequest).toHaveBeenCalledTimes(1))
      markPublicProjectionRevoked('thr_revoke_race')
      pending.resolve({
        ok: true,
        status: 200,
        body: JSON.stringify(taskMutation(
          'taskjob:expected',
          operation === 'restart' ? 'queued' : 'killed'
        ))
      })

      await expect(request).rejects.toThrow('Public case projection authority was revoked.')
    }
  )

  it('keeps renderer runtime requests on analytix-owned HTTP routes', async () => {
    const threadBody = {
      id: 'thr_surface',
      title: 'Surface',
      workspace: '/tmp/workspace',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      createdAt: 't0',
      updatedAt: 't1'
    }
    const runtimeRequest = vi.fn(async (path: string, method?: string) => {
      if (path.startsWith('/v1/threads?')) {
        return { ok: true, status: 200, body: JSON.stringify({ threads: [] }) }
      }
      if (path === '/v1/threads' && method === 'POST') {
        return { ok: true, status: 201, body: JSON.stringify(threadBody) }
      }
      if (path.endsWith('/turns')) {
        return {
          ok: true,
          status: 202,
          body: JSON.stringify({ threadId: 'thr_surface', turnId: 'turn_surface', userMessageItemId: 'item_user' })
        }
      }
      if (path.endsWith('/goal') && method === 'POST') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            goal: {
              objective: 'Keep routes owned',
              status: 'active',
              createdAt: 't0',
              updatedAt: 't1'
            }
          })
        }
      }
      if (path.endsWith('/goal') && method === 'DELETE') {
        return { ok: true, status: 200, body: JSON.stringify({ cleared: true }) }
      }
      if (path.endsWith('/todos') && method === 'POST') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            todos: {
              items: [{
                id: 'todo_1',
                content: 'prove route surface',
                status: 'pending'
              }]
            }
          })
        }
      }
      if (path.endsWith('/todos') && method === 'DELETE') {
        return { ok: true, status: 200, body: JSON.stringify({ cleared: true }) }
      }
      if (path.endsWith('/fork')) {
        return { ok: true, status: 201, body: JSON.stringify({ ...threadBody, id: 'thr_fork', forkedFromThreadId: 'thr_surface' }) }
      }
      if (path.includes('/resume-thread')) {
        return { ok: true, status: 201, body: JSON.stringify({ thread_id: 'thr_resumed', session_id: 'sess_surface' }) }
      }
      if (path === '/v1/threads/thr_surface/summary') {
        return { ok: true, status: 200, body: JSON.stringify(summaryResponse('thr_surface')) }
      }
      if (path === '/v1/threads/thr_surface/summary/tasks/taskjob%3Atask_surface/output?offset=1&limit=3') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            availability: 'withheld',
            taskId: 'taskjob:task_surface',
            status: 'running',
            reasonCode: 'security_bound_child_output',
            outputWithheld: true,
            outputTrustStatus: 'untrusted_child_output',
            factAnswerAllowed: false,
            evidenceAuthority: false,
            canReadOutput: false,
            canContinueParent: false
          })
        }
      }
      if (path === '/v1/threads/thr_surface/summary/tasks/taskjob%3Atask_surface/kill' ||
        path === '/v1/threads/thr_surface/summary/tasks/taskjob%3Atask_surface/restart') {
        const killed = path.endsWith('/kill')
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            task: {
              schemaVersion: 1,
              id: 'taskjob:task_surface',
              kind: 'task',
              status: killed ? 'killed' : 'queued',
              background: false,
              terminal: killed,
              outputWithheld: true,
              outputTrustStatus: 'untrusted_child_output',
              factAnswerAllowed: false,
              evidenceAuthority: false,
              canReadOutput: false,
              canContinueParent: false
            }
          })
        }
      }
      return { ok: true, status: 200, body: '{}' }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.connect()
    await provider.listThreads({ limit: 3, search: 'owned surface' })
    await provider.listThreads({ includeSide: true })
    await provider.createThread({ workspace: '/tmp/workspace', title: 'Surface' })
    await provider.sendUserMessage('thr_surface', 'hello')
    await provider.steerUserMessage({
      threadId: 'thr_surface',
      turnId: 'turn_surface',
      text: 'steer'
    })
    await provider.interruptTurn('thr_surface', 'turn_surface', { discard: true })
    await provider.compactThread('thr_surface', 'currentness')
    await provider.setThreadGoal('thr_surface', { objective: 'Keep routes owned' })
    await provider.clearThreadGoal('thr_surface')
    await provider.setThreadTodos('thr_surface', [{ id: 'todo_1', content: 'prove route surface', status: 'pending' }])
    await provider.clearThreadTodos('thr_surface')
    await provider.submitApprovalDecision('appr_surface', 'deny')
    await provider.submitUserInputResponse('input_surface', [{ id: 'choice', label: 'Yes', value: 'yes' }])
    await provider.cancelUserInput('input_cancel')
    await provider.forkThread('thr_surface')
    await provider.resumeSession('sess_surface', { mode: 'plan' })
    const summary = await provider.getThreadSummary('thr_surface')
    const output = await provider.getThreadSummaryTaskOutput('thr_surface', 'taskjob:task_surface', { offset: 1, limit: 3 })
    const killed = await provider.killThreadSummaryTask('thr_surface', 'taskjob:task_surface')
    const restarted = await provider.restartThreadSummaryTask('thr_surface', 'taskjob:task_surface')

    expect(summary).toMatchObject({
      threadId: 'thr_surface',
      latestSeq: 9,
      subagents: [expect.objectContaining({
        childThreadId: 'thr_child_surface',
        canOpenThread: true,
        evidenceLedgered: true,
        cacheHitRate: 0.5
      })],
      tasks: [expect.objectContaining({
        id: 'taskjob:task_surface',
        kind: 'task',
        status: 'running',
        outputWithheld: true,
        canReadOutput: false
      })]
    })
    expect(output).toEqual({
      schemaVersion: 1,
      availability: 'withheld',
      taskId: 'taskjob:task_surface',
      status: 'running',
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    })
    expect(killed.task).toMatchObject({ id: 'taskjob:task_surface', status: 'killed', terminal: true })
    expect(restarted.task).toMatchObject({ id: 'taskjob:task_surface', status: 'queued', terminal: false })

    const paths = runtimeRequest.mock.calls.map(([path]) => path)
    expectRuntimeRequestPathsStayAnalytixOwned(paths)
    expect(paths).toEqual(expect.arrayContaining([
      '/health',
      '/v1/threads?limit=1',
      '/v1/threads?limit=3&search=owned+surface',
      '/v1/threads?limit=50&include=side',
      '/v1/threads',
      '/v1/threads/thr_surface/turns',
      '/v1/threads/thr_surface/turns/turn_surface/steer',
      '/v1/threads/thr_surface/turns/turn_surface/interrupt',
      '/v1/threads/thr_surface/compact',
      '/v1/threads/thr_surface/goal',
      '/v1/threads/thr_surface/todos',
      '/v1/approvals/appr_surface',
      '/v1/user-inputs/input_surface',
      '/v1/user-inputs/input_cancel',
      '/v1/threads/thr_surface/fork',
      '/v1/sessions/sess_surface/resume-thread',
      '/v1/threads/thr_surface/summary',
      '/v1/threads/thr_surface/summary/tasks/taskjob%3Atask_surface/output?offset=1&limit=3',
      '/v1/threads/thr_surface/summary/tasks/taskjob%3Atask_surface/kill',
      '/v1/threads/thr_surface/summary/tasks/taskjob%3Atask_surface/restart'
    ]))
  })

  it('uses Analytix thread lifecycle endpoints for list, archive, restore, rename, workspace, and delete', async () => {
    const runtimeRequest = vi.fn(async (path: string, _method?: string, _body?: string) => {
      if (path === '/v1/threads?limit=25&search=archive+match&include_archived=true&archived_only=true') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            threads: [{
              id: 'thr_archived',
              title: 'Archived match',
              workspace: '/tmp/workspace',
              model: 'deepseek-chat',
              mode: 'agent',
              status: 'archived',
              createdAt: 't0',
              updatedAt: 't1',
              preview: 'archive match'
            }]
          })
        }
      }
      return { ok: true, status: 200, body: '{}' }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const threads = await provider.listThreads({
      limit: 25,
      search: 'archive match',
      includeArchived: true,
      archivedOnly: true
    })
    await provider.archiveThread('thr_archived', true)
    await provider.archiveThread('thr_archived', false)
    await provider.renameThread('thr_archived', 'Renamed')
    await provider.updateThreadWorkspace('thr_archived', '/tmp/next-workspace')
    await provider.updateThreadRelation('thr_archived', 'primary')
    await provider.deleteThread('thr_archived')

    expect(threads).toEqual([
      expect.objectContaining({
        id: 'thr_archived',
        archived: true,
        preview: 'archive match'
      })
    ])
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      '/v1/threads?limit=25&search=archive+match&include_archived=true&archived_only=true',
      'GET'
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      '/v1/threads/thr_archived',
      'PATCH',
      JSON.stringify({ status: 'archived' })
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      '/v1/threads/thr_archived',
      'PATCH',
      JSON.stringify({ status: 'idle' })
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      4,
      '/v1/threads/thr_archived',
      'PATCH',
      JSON.stringify({ title: 'Renamed' })
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      5,
      '/v1/threads/thr_archived',
      'PATCH',
      JSON.stringify({ workspace: '/tmp/next-workspace' })
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      6,
      '/v1/threads/thr_archived',
      'PATCH',
      JSON.stringify({ relation: 'primary' })
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      7,
      '/v1/threads/thr_archived',
      'DELETE'
    )
  })

  it('maps Analytix thread items into chat blocks', async () => {
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify({
          id: 'thr_1',
          title: 'Demo',
          workspace: '/tmp',
          model: 'deepseek-chat',
          mode: 'agent',
          status: 'idle',
          createdAt: 't0',
          updatedAt: 't1',
          latestSeq: 9,
          usage: {
            promptTokens: 1000,
            completionTokens: 40,
            reasoningTokens: 12,
            totalTokens: 1040,
            cacheHitTokens: 700,
            cacheMissTokens: 300,
            cacheHitRate: 0.7,
            turns: 1
          },
          turns: [
            {
              id: 'turn_1',
              threadId: 'thr_1',
              status: 'completed',
              prompt: 'hi',
              workspaceCheckpointId: 'gcp_turn_1',
              createdAt: 't0',
              items: [
                {
                  id: 'item_user',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'user',
                  status: 'completed',
                  createdAt: 't0',
                  kind: 'user_message',
                  text: 'hi'
                },
                {
                  id: 'item_answer',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'assistant',
                  status: 'completed',
                  createdAt: 't1',
                  kind: 'assistant_text',
                  text: 'hello'
                }
              ]
            }
          ]
        })
      }))
    })
    const provider = new AnalytixRuntimeProvider()
    const detail = await provider.getThreadDetail('thr_1')
    expect(detail.thread).toMatchObject({ id: 'thr_1', title: 'Demo', workspace: '/tmp', model: 'deepseek-chat' })
    expect(detail.blocks.map((block) => block.kind)).toEqual(['user', 'assistant'])
    expect(detail.blocks[0]).toMatchObject({
      kind: 'user',
      meta: expect.objectContaining({ workspaceCheckpointId: 'gcp_turn_1' })
    })
    expect(detail.latestSeq).toBe(9)
    expect(detail.latestTurnId).toBe('turn_1')
    expect(detail.latestUserMessageId).toBe('item_user')
    expect(detail.usage).toMatchObject({
      inputTokens: 1000,
      outputTokens: 40,
      reasoningTokens: 12,
      cachedTokens: 700,
      cacheMissTokens: 300,
      totalTokens: 1040,
      cacheHitRate: 0.7,
      turns: 1
    })
  })

  it('rejects a thread detail belonging to a different requested identity', async () => {
    installDsGui({ runtimeRequest: vi.fn(async () => ({ ok: true, status: 200,
      body: JSON.stringify({ id: 'wrong-thread', title: 'Wrong', turns: [] }) })) })
    await expect(new AnalytixRuntimeProvider().getThreadDetail('requested-thread')).rejects.toThrow('identity mismatch')
  })

  it('drops case assistant drafts unless the host GET carries a matching V3 generic final', async () => {
    const acceptedFinal = acceptedCaseFinalRecord()
    const acceptedFinalView = acceptedCaseFinalViewV3(acceptedFinal)
    const acceptedFinalDelivery = acceptedFinalDeliveryBatchForRenderer()
    const runtimeRequest = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        body: JSON.stringify({
          id: 'thr_case',
          title: 'Case',
          workspace: '/tmp/case',
          model: 'deepseek-chat',
          mode: 'agent',
          status: 'idle',
          historyAuthority: 'case_boundary_only_v1',
          createdAt: '2026-07-11T00:00:00.000Z',
          updatedAt: '2026-07-11T00:00:01.000Z',
          latestSeq: 10,
          turns: [{
            id: 'turn_case',
            threadId: 'thr_case',
            status: 'completed',
            prompt: 'case question',
            createdAt: '2026-07-11T00:00:00.000Z',
            finishedAt: '2026-07-11T00:00:01.000Z',
            items: [
              {
                id: 'item_user',
                turnId: 'turn_case',
                threadId: 'thr_case',
                role: 'user',
                status: 'completed',
                createdAt: '2026-07-11T00:00:00.000Z',
                kind: 'user_message',
                text: 'case question'
              },
              {
                id: 'item_draft',
                turnId: 'turn_case',
                threadId: 'thr_case',
                role: 'assistant',
                status: 'completed',
                createdAt: '2026-07-11T00:00:01.000Z',
                kind: 'assistant_text',
                text: 'fabricated case fact'
              }
            ]
          }]
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        body: JSON.stringify({
          id: 'thr_case',
          title: 'Case',
          workspace: '/tmp/case',
          model: 'deepseek-chat',
          mode: 'agent',
          status: 'idle',
          historyAuthority: 'case_boundary_only_v1',
          createdAt: '2026-07-11T00:00:00.000Z',
          updatedAt: '2026-07-11T00:00:01.000Z',
          latestSeq: acceptedFinalDelivery.lastSeq,
          acceptedFinalDelivery,
          acceptedFinalDeliveries: [acceptedFinalDelivery],
          turns: [{
            id: 'turn_case',
            threadId: 'thr_case',
            status: 'completed',
            prompt: 'case question',
            createdAt: '2026-07-11T00:00:00.000Z',
            finishedAt: acceptedFinal.acceptedAt,
            acceptedFinalView,
            items: [
              {
                id: 'item_user',
                turnId: 'turn_case',
                threadId: 'thr_case',
                role: 'user',
                status: 'completed',
                createdAt: '2026-07-11T00:00:00.000Z',
                kind: 'user_message',
                text: 'case question'
              },
              {
                id: 'item_final',
                turnId: 'turn_case',
                threadId: 'thr_case',
                role: 'assistant',
                status: 'completed',
                createdAt: acceptedFinal.acceptedAt,
                finishedAt: acceptedFinal.acceptedAt,
                kind: 'assistant_text',
                text: 'verified final',
                acceptedFinalView
              }
            ]
          }]
        })
      })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const draft = await provider.getThreadDetail('thr_case')
    expect(draft.blocks.map((block) => block.kind)).toEqual(['user'])
    expect(draft.latestTurnAcceptedFinalDigest).toBeUndefined()

    const accepted = await provider.getThreadDetail('thr_case')
    expect(accepted.blocks).toEqual([
      expect.objectContaining({ kind: 'user', id: 'item_user' }),
      expect.objectContaining({
        kind: 'assistant',
        id: 'item_final',
        text: 'verified final',
        acceptedFinalView: expect.objectContaining({ blockerCode: 'current_case_source_unavailable' })
      })
    ])
    expect(accepted.latestTurnAcceptedFinalDigest).toBe(acceptedFinal.recordDigest)
  })

  it('retains typed ordinary history alongside SourceUnavailableAnswer after resume', async () => {
    const record = acceptedCaseFinalRecord()
    const view = acceptedCaseFinalViewV3(record)
    const response = acceptedCaseThreadResponse(record, view, view) as Record<string, any>
    delete response.turns[0].acceptedFinal
    delete response.turns[0].items[1].acceptedFinal
    const delivery = acceptedFinalDeliveryBatchForRenderer()
    response.acceptedFinalDeliveries = [delivery]
    response.latestSeq = 30
    const ordinary = await ordinaryHistoryItem()
    response.turns.push({
      id: ordinary.turnId, threadId: ordinary.threadId, status: 'completed',
      createdAt: ordinary.createdAt, finishedAt: ordinary.finishedAt,
      items: [ordinary]
    })
    const runtimeRequest = vi.fn(async (path: string) => ({
      ok: true, status: 200,
      body: JSON.stringify(path.endsWith('/resume-thread')
        ? { thread_id: 'thr_case', session_id: 'session_case' } : response)
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()
    const before = await provider.getThreadDetail('thr_case')
    const resumed = await provider.resumeSession('session_case')
    const after = await provider.getThreadDetail(resumed.threadId)
    expect(after).toEqual(before)
    expect(after.blocks).toEqual([
      expect.objectContaining({ kind: 'user', id: 'item_user' }),
      expect.objectContaining({
        kind: 'assistant', id: 'item_final', meta: { turnId: 'turn_case' },
        acceptedFinalView: expect.objectContaining({ variant: 'SourceUnavailableAnswer' }),
        acceptedFinalProjectionReceipt: expect.objectContaining({ batchId: delivery.batchId })
      }),
      expect.objectContaining({
        kind: 'assistant', id: ordinary.id, text: ordinary.text,
        meta: { turnId: ordinary.turnId }
      })
    ])
    const ordinaryBlock = after.blocks.at(-1)
    expect(ordinaryBlock).not.toHaveProperty('acceptedFinalView')
    expect(ordinaryBlock).not.toHaveProperty('acceptedFinalProjectionReceipt')
    expect(after.latestTurnAcceptedFinalDigest).toBeUndefined()
    expect(after.latestTurnId).toBe(ordinary.turnId)
    expect(after.latestSeq).toBe(30)
  })

  it.each([
    ['missing slot', (item: any) => { delete item.ordinaryResult }],
    ['text mismatch', (item: any) => { item.text = 'changed text' }],
    ['mixed authority', (item: any) => { item.acceptedFinalView = acceptedCaseFinalViewV3() }],
    ['private authority', (item: any) => { item.acceptedFinal = acceptedCaseFinalRecord() }],
    ['malformed slot', (item: any) => { item.ordinaryResult.evidenceAuthority = true }],
    ['unknown slot field', (item: any) => { item.ordinaryResult.extra = true }],
    ['wrong turn identity', (item: any) => { item.turnId = 'another_turn' }],
    ['wrong thread identity', (item: any) => { item.threadId = 'another_thread' }]
  ])('rejects case ordinary history with %s', async (_name, mutate) => {
    const item = await ordinaryHistoryItem()
    mutate(item)
    installDsGui({ runtimeRequest: vi.fn().mockResolvedValue({
      ok: true, status: 200, body: JSON.stringify({
        id: 'thr_case', historyAuthority: 'case_boundary_only_v1', latestSeq: 3,
        turns: [{ id: 'turn_ordinary', threadId: 'thr_case', status: 'completed', items: [item] }]
      })
    }) })
    const detail = await new AnalytixRuntimeProvider().getThreadDetail('thr_case')
    expect(detail.blocks).toEqual([])
    expect(detail.latestTurnAcceptedFinalDigest).toBeUndefined()
  })

  it('hydrates a strict V3 accepted final only from its verified delivery group', async () => {
    const record = acceptedCaseFinalRecord()
    const view = acceptedCaseFinalViewV3(record)
    const response = acceptedCaseThreadResponse(record, view, view) as Record<string, any>
    const acceptedFinalDelivery = acceptedFinalDeliveryBatchForRenderer()
    const turn = response.turns[0] as Record<string, any>
    delete turn.acceptedFinal
    const assistant = turn.items.find((item: Record<string, unknown>) => item.kind === 'assistant_text')
    delete assistant.acceptedFinal
    const sealedResponse = structuredClone(response)
    sealedResponse.latestSeq = acceptedFinalDelivery.lastSeq
    sealedResponse.acceptedFinalDelivery = acceptedFinalDelivery
    sealedResponse.acceptedFinalDeliveries = [acceptedFinalDelivery]
    installDsGui({
      runtimeRequest: vi.fn()
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          body: JSON.stringify(response)
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          body: JSON.stringify(sealedResponse)
        })
    })
    const provider = new AnalytixRuntimeProvider()

    const standalone = await provider.getThreadDetail('thr_case')
    expect(standalone.blocks.map((block) => block.kind)).toEqual(['user'])
    expect(standalone.latestTurnAcceptedFinalDigest).toBeUndefined()

    const accepted = await provider.getThreadDetail('thr_case')
    expect(accepted.blocks).toEqual([
      expect.objectContaining({ kind: 'user', id: 'item_user' }),
      expect.objectContaining({
        kind: 'assistant',
        id: 'item_final',
        text: 'verified final',
        acceptedFinalView: expect.objectContaining({
          schemaVersion: 3,
          acceptedFinalDigest: record.recordDigest,
          publicationState: 'accepted',
          blockerCode: 'current_case_source_unavailable'
        })
      })
    ])
    expect(accepted.latestTurnAcceptedFinalDigest).toBe(record.recordDigest)
    const renderedAssistant = accepted.blocks.find((block) => block.kind === 'assistant') as any
    expect(renderedAssistant.acceptedFinalProjectionReceipt).toEqual(expect.objectContaining({
      schemaVersion: 1,
      batchId: acceptedFinalDelivery.batchId,
      publicationCommitId: record.recordDigest,
      lastSeq: acceptedFinalDelivery.lastSeq
    }))
    expect(renderedAssistant.acceptedFinalProjectionTerminal).toEqual(expect.objectContaining({
      acceptedFinalDigest: record.recordDigest,
      terminalReason: record.terminalReason
    }))
    const renderedView = renderedAssistant.acceptedFinalView
    expect(Object.keys(renderedView).sort()).toEqual([
      'schemaVersion', 'acceptedFinalDigest', 'publicationState', 'variant', 'terminalReason',
      'blockerCode', 'coverageStatus', 'checkedScopeDigest', 'missingScopeCount', 'claimCount',
      'claimTypes', 'receiptMetadata', 'noHitWording', 'acceptedAt'
    ].sort())
    const renderedBody = JSON.stringify(accepted)
    expect(renderedBody).not.toContain('"acceptedFinal":')
    for (const forbiddenProperty of [
      'publicViewDigest', 'envelopeDigest', 'contextDigest', 'contextEpoch', 'datasetSnapshotId',
      'envelopeIssuedAt', 'renderedTextSha256', 'factFinalWitnessAdmission',
      'publicationSnapshotProof', 'publicationSnapshotProofDigest', 'publicationIntent',
      'storeDigest', 'registryHead'
    ]) {
      expect(JSON.stringify(renderedView)).not.toContain(`"${forbiddenProperty}"`)
    }
  })

  it('rejects duplicate delivery groups instead of authorizing a standalone V3 view', async () => {
    const record = acceptedCaseFinalRecord()
    const view = acceptedCaseFinalViewV3(record)
    const response = acceptedCaseThreadResponse(record, view, view) as Record<string, any>
    const delivery = acceptedFinalDeliveryBatchForRenderer()
    const turn = response.turns[0] as Record<string, any>
    delete turn.acceptedFinal
    delete turn.items.find((item: Record<string, unknown>) => item.kind === 'assistant_text').acceptedFinal
    response.latestSeq = delivery.lastSeq
    response.acceptedFinalDeliveries = [delivery, structuredClone(delivery)]
    installDsGui({
      runtimeRequest: vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        body: JSON.stringify(response)
      })
    })
    const provider = new AnalytixRuntimeProvider()

    const detail = await provider.getThreadDetail('thr_case')
    expect(detail.blocks.map((block) => block.kind)).toEqual(['user'])
    expect(detail.latestTurnAcceptedFinalDigest).toBeUndefined()
  })

  it('hydrates two historical V3 finals only from their own ordered delivery groups', async () => {
    const record = acceptedCaseFinalRecord()
    const firstView = acceptedCaseFinalViewV3(record)
    const firstDelivery = acceptedFinalDeliveryBatchForRenderer()
    const secondDelivery = secondAcceptedFinalDeliveryBatchForRenderer()
    const response = acceptedCaseThreadResponse(record, firstView, firstView) as Record<string, any>
    const firstTurn = response.turns[0] as Record<string, any>
    firstTurn.factHistoryState = 'retained_snapshot'
    delete firstTurn.acceptedFinal
    delete firstTurn.items.find((item: Record<string, unknown>) => item.kind === 'assistant_text').acceptedFinal
    const secondAssistantEvent = secondDelivery.events[0] as any
    if (secondAssistantEvent.kind !== 'item_completed' ||
        secondAssistantEvent.item.kind !== 'assistant_text') {
      throw new Error('second accepted-final fixture is invalid')
    }
    response.turns.push({
      id: secondDelivery.turnId,
      threadId: secondDelivery.threadId,
      status: 'completed',
      prompt: 'second case question',
      createdAt: secondDelivery.timestamp,
      finishedAt: secondDelivery.timestamp,
      acceptedFinalView: secondAssistantEvent.item.acceptedFinalView,
      items: [{
        id: 'item_user_second',
        turnId: secondDelivery.turnId,
        threadId: secondDelivery.threadId,
        role: 'user',
        status: 'completed',
        createdAt: secondDelivery.timestamp,
        kind: 'user_message',
        text: 'second case question'
      }, secondAssistantEvent.item]
    })
    response.latestSeq = secondDelivery.lastSeq
    response.acceptedFinalDeliveries = [firstDelivery, secondDelivery]
    const swapped = structuredClone(response)
    swapped.acceptedFinalDeliveries.reverse()
    installDsGui({
      runtimeRequest: vi.fn()
        .mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(response) })
        .mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(swapped) })
    })
    const provider = new AnalytixRuntimeProvider()

    const accepted = await provider.getThreadDetail('thr_case')
    expect(accepted.blocks.map((block) => `${block.kind}:${block.id}`)).toEqual([
      'user:item_user',
      'assistant:item_final',
      'user:item_user_second',
      'assistant:item_final_second'
    ])
    const assistants = accepted.blocks.filter((block) => block.kind === 'assistant')
    expect(assistants[0].meta?.factHistoryState).toBe('retained_snapshot')
    expect(assistants[1].meta?.factHistoryState).toBeUndefined()
    expect(assistants.map((block) => block.acceptedFinalProjectionReceipt?.batchId)).toEqual([
      firstDelivery.batchId,
      secondDelivery.batchId
    ])
    expect(accepted.latestTurnAcceptedFinalDigest).toBe(secondDelivery.publicationCommitId)

    const rejected = await provider.getThreadDetail('thr_case')
    expect(rejected.blocks.map((block) => block.kind)).toEqual(['user', 'user'])
    expect(rejected.latestTurnAcceptedFinalDigest).toBeUndefined()
  })

  it('fails closed on unknown or mismatched accepted-final public view metadata', async () => {
    const acceptedFinal = acceptedCaseFinalRecord()
    const acceptedFinalView = acceptedCaseFinalView(acceptedFinal)
    const unknownFieldView = { ...acceptedFinalView, rawReceiptId: 'receipt-private' }
    const mismatchedItemView = { ...acceptedFinalView, contextDigest: '7'.repeat(64) }
    installDsGui({
      runtimeRequest: vi.fn()
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          body: JSON.stringify(acceptedCaseThreadResponse(acceptedFinal, unknownFieldView))
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          body: JSON.stringify(acceptedCaseThreadResponse(acceptedFinal, acceptedFinalView, mismatchedItemView))
        })
    })
    const provider = new AnalytixRuntimeProvider()

    const unknown = await provider.getThreadDetail('thr_case')
    expect(unknown.blocks.map((block) => block.kind)).toEqual(['user'])
    expect(unknown.latestTurnAcceptedFinalDigest).toBeUndefined()

    const mismatched = await provider.getThreadDetail('thr_case')
    expect(mismatched.blocks.map((block) => block.kind)).toEqual(['user'])
    expect(mismatched.latestTurnAcceptedFinalDigest).toBeUndefined()
  })

  it('uses pending gate ids to hydrate only live approval and user-input blocks', async () => {
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify({
          id: 'thr_1',
          title: 'Gates',
          workspace: '/tmp',
          model: 'deepseek-chat',
          mode: 'agent',
          status: 'running',
          createdAt: 't0',
          updatedAt: 't1',
          latestSeq: 12,
          pendingApprovalIds: ['appr_live'],
          pendingUserInputIds: [],
          turns: [
            {
              id: 'turn_1',
              threadId: 'thr_1',
              status: 'waiting',
              prompt: 'need gates',
              createdAt: 't0',
              items: [
                {
                  id: 'item_appr_live',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'tool',
                  status: 'allowed',
                  createdAt: 't0',
                  kind: 'approval',
                  approvalId: 'appr_live',
                  toolName: 'shell',
                  summary: 'Approve shell'
                },
                {
                  id: 'item_appr_stale',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'tool',
                  status: 'pending',
                  createdAt: 't1',
                  kind: 'approval',
                  approvalId: 'appr_stale',
                  toolName: 'edit',
                  summary: 'Approve edit'
                },
                {
                  id: 'item_input_stale',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'system',
                  status: 'pending',
                  createdAt: 't2',
                  kind: 'user_input',
                  inputId: 'input_stale',
                  prompt: 'Pick',
                  questions: []
                }
              ]
            }
          ]
        })
      }))
    })
    const provider = new AnalytixRuntimeProvider()
    const detail = await provider.getThreadDetail('thr_1')
    expect(detail.pendingApprovalIds).toEqual(['appr_live'])
    expect(detail.pendingUserInputIds).toEqual([])
    expect(detail.blocks).toEqual([
      expect.objectContaining({ kind: 'approval', approvalId: 'appr_live', status: 'pending' }),
      expect.objectContaining({ kind: 'approval', approvalId: 'appr_stale', status: 'error' }),
      expect.objectContaining({ kind: 'user_input', requestId: 'input_stale', status: 'cancelled' })
    ])
  })

  it('coalesces tool_call and tool_result pairs into one tool block on thread load', async () => {
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify({
          id: 'thr_1',
          title: 'Demo',
          workspace: '/tmp',
          model: 'deepseek-chat',
          mode: 'agent',
          status: 'idle',
          createdAt: 't0',
          updatedAt: 't1',
          latestSeq: 9,
          turns: [
            {
              id: 'turn_1',
              threadId: 'thr_1',
              status: 'completed',
              prompt: 'run echo',
              createdAt: 't0',
              items: [
                {
                  id: 'item_call',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'tool',
                  status: 'pending',
                  createdAt: 't0',
                  kind: 'tool_call',
                  toolName: 'echo',
                  callId: 'call_1',
                  arguments: { text: 'hi' }
                },
                {
                  id: 'item_result',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'tool',
                  status: 'completed',
                  createdAt: 't1',
                  kind: 'tool_result',
                  toolName: 'echo',
                  callId: 'call_1',
                  output: { echoed: 'hi' }
                }
              ]
            }
          ]
        })
      }))
    })
    const provider = new AnalytixRuntimeProvider()
    const detail = await provider.getThreadDetail('thr_1')
    expect(detail.blocks).toHaveLength(1)
    expect(detail.blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_1',
      status: 'success'
    })
  })

  it('drops persisted reasoning while preserving public turn timing on thread load', async () => {
    installDsGui({
      runtimeRequest: vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify({
          id: 'thr_1',
          title: 'Demo',
          workspace: '/tmp',
          model: 'deepseek-chat',
          mode: 'agent',
          status: 'idle',
          createdAt: '2026-01-02T03:04:00.000Z',
          updatedAt: '2026-01-02T03:04:08.000Z',
          latestSeq: 12,
          turns: [
            {
              id: 'turn_1',
              threadId: 'thr_1',
              status: 'completed',
              prompt: 'inspect',
              createdAt: '2026-01-02T03:04:00.000Z',
              startedAt: '2026-01-02T03:04:00.000Z',
              finishedAt: '2026-01-02T03:04:08.000Z',
              items: [
                {
                  id: 'item_user',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'user',
                  status: 'completed',
                  createdAt: '2026-01-02T03:04:00.000Z',
                  kind: 'user_message',
                  text: 'inspect'
                },
                {
                  id: 'item_call',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'tool',
                  status: 'pending',
                  createdAt: '2026-01-02T03:04:02.000Z',
                  kind: 'tool_call',
                  toolName: 'list',
                  callId: 'call_1',
                  reasoningContent: 'I need to inspect the workspace first.',
                  arguments: { path: '/tmp' }
                },
                {
                  id: 'item_result',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'tool',
                  status: 'completed',
                  createdAt: '2026-01-02T03:04:03.000Z',
                  kind: 'tool_result',
                  toolName: 'list',
                  callId: 'call_1',
                  output: { files: ['a.txt'] }
                },
                {
                  id: 'item_final_reasoning',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'assistant',
                  status: 'completed',
                  createdAt: '2026-01-02T03:04:04.000Z',
                  finishedAt: '2026-01-02T03:04:07.000Z',
                  kind: 'assistant_reasoning',
                  text: 'Now I can answer.'
                },
                {
                  id: 'item_answer',
                  turnId: 'turn_1',
                  threadId: 'thr_1',
                  role: 'assistant',
                  status: 'completed',
                  createdAt: '2026-01-02T03:04:08.000Z',
                  kind: 'assistant_text',
                  text: 'There is a.txt.'
                }
              ]
            }
          ]
        })
      }))
    })
    const provider = new AnalytixRuntimeProvider()
    const detail = await provider.getThreadDetail('thr_1')

	expect(detail.blocks.map((block) => block.kind)).toEqual([
		'user',
		'tool',
		'assistant'
	])
	expect(detail.turnDurationByUserId?.item_user).toBe(8_000)
  })

  it('posts Analytix turn requests and returns the deterministic user item id', async () => {
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, _body?: string) => ({
      ok: true,
      status: 202,
      body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_abc', userMessageItemId: 'item_user_real' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()
    const result = await provider.sendUserMessage('thr_1', 'hello')
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns',
      'POST',
      JSON.stringify({
        prompt: 'hello',
        async: true,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write'
      })
    )
    expect(result.userMessageItemId).toBe('item_user_real')
  })

  it('posts Analytix steer requests with admission metadata and parses the response', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        ok: true,
        threadId: 'thr_1',
        turnId: 'turn_abc',
        itemId: 'item_client_steer',
        clientUserMessageId: 'client_steer',
        admittedSeq: 12
      })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const result = await provider.steerUserMessage({
      threadId: 'thr_1',
      turnId: 'turn_abc',
      text: 'add this context',
      displayText: 'Add this context',
      clientUserMessageId: 'client_steer',
      expectedTurnId: 'turn_abc',
      attachmentIds: ['att_1'],
      fileReferences: [{
        path: '/workspace/analytix/src/App.tsx',
        relativePath: 'src/App.tsx',
        name: 'App.tsx',
        kind: 'file'
      }]
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns/turn_abc/steer',
      'POST',
      JSON.stringify({
        text: 'add this context',
        expectedTurnId: 'turn_abc',
        delivery: 'steer',
        displayText: 'Add this context',
        clientUserMessageId: 'client_steer',
        attachmentIds: ['att_1'],
        fileReferences: [{
          path: '/workspace/analytix/src/App.tsx',
          relativePath: 'src/App.tsx',
          name: 'App.tsx',
          kind: 'file'
        }]
      })
    )
    expect(result).toEqual({
      threadId: 'thr_1',
      turnId: 'turn_abc',
      itemId: 'item_client_steer',
      clientUserMessageId: 'client_steer',
      admittedSeq: 12
    })
  })

  it('passes providerId through create, turn, and review requests', async () => {
    const runtimeRequest = vi.fn(async (path: string, _method?: string, _body?: string) => {
      if (path === '/v1/threads') {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({
            id: 'thr_1',
            title: 'Provider thread',
            workspace: '/tmp/workspace',
            model: 'glm-5',
            providerId: 'zai-coding-plan',
            mode: 'agent',
            status: 'idle',
            createdAt: '2026-06-20T00:00:00.000Z',
            updatedAt: '2026-06-20T00:00:00.000Z'
          })
        }
      }
      if (path === '/v1/threads/thr_1/review') {
        return {
          ok: true,
          status: 202,
          body: JSON.stringify({
            threadId: 'thr_1',
            turnId: 'turn_review',
            userMessageItemId: 'item_user_review',
            reviewItemId: 'item_review'
          })
        }
      }
      return {
        ok: true,
        status: 202,
        body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_1', userMessageItemId: 'item_user' })
      }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const thread = await provider.createThread({
      workspace: '/tmp/workspace',
      title: 'Provider thread',
      model: 'glm-5',
      providerId: 'zai-coding-plan'
    })
    await provider.sendUserMessage('thr_1', 'hello', { model: 'glm-5', providerId: 'zai-coding-plan' })
    await provider.reviewThread('thr_1', { kind: 'uncommittedChanges' }, {
      model: 'glm-5',
      providerId: 'zai-coding-plan'
    })

    expect(thread.providerId).toBe('zai-coding-plan')
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      '/v1/threads',
      'POST',
      expect.stringContaining('"providerId":"zai-coding-plan"')
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      '/v1/threads/thr_1/turns',
      'POST',
      expect.stringContaining('"providerId":"zai-coding-plan"')
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      '/v1/threads/thr_1/review',
      'POST',
      expect.stringContaining('"providerId":"zai-coding-plan"')
    )
  })

  it('preserves an explicit Registry model when legacy Settings list older models', async () => {
    const runtimeRequest = vi.fn(async (path: string, _method?: string, _body?: string) => {
      if (path === '/v1/threads') {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({
            id: 'thr_1',
            title: 'Provider thread',
            workspace: '/tmp/workspace',
            model: 'deepseek-flash',
            providerId: 'deepseek',
            mode: 'agent',
            status: 'idle',
            createdAt: '2026-06-20T00:00:00.000Z',
            updatedAt: '2026-06-20T00:00:00.000Z'
          })
        }
      }
      if (path === '/v1/threads/thr_1/review') {
        return {
          ok: true,
          status: 202,
          body: JSON.stringify({
            threadId: 'thr_1',
            turnId: 'turn_review',
            userMessageItemId: 'item_user_review',
            reviewItemId: 'item_review'
          })
        }
      }
      return {
        ok: true,
        status: 202,
        body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_1', userMessageItemId: 'item_user' })
      }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const thread = await provider.createThread({
      workspace: '/tmp/workspace',
      title: 'Provider thread',
      model: 'deepseek-flash',
      providerId: 'deepseek'
    })
    await provider.sendUserMessage('thr_1', 'hello', { model: 'deepseek-flash', providerId: 'deepseek' })
    await provider.reviewThread('thr_1', { kind: 'uncommittedChanges' }, {
      model: 'deepseek-flash',
      providerId: 'deepseek'
    })

    expect(thread.providerId).toBe('deepseek')
    for (const call of runtimeRequest.mock.calls) {
      expect(JSON.parse(call[2] ?? '{}')).toMatchObject({
        providerId: 'deepseek', model: 'deepseek-flash'
      })
    }
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      '/v1/threads',
      'POST',
      expect.stringContaining('"providerId":"deepseek"')
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2,
      '/v1/threads/thr_1/turns',
      'POST',
      expect.stringContaining('"providerId":"deepseek"')
    )
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      3,
      '/v1/threads/thr_1/review',
      'POST',
      expect.stringContaining('"providerId":"deepseek"')
    )
  })

  it('infers the active Xiaomi provider for MiMo plan-mode create and turn requests', async () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiTokenPlan = modelProviderTokenPlanProfile(xiaomi!)
    expect(xiaomiTokenPlan).not.toBeNull()
    const getSettings = vi.fn(async (): Promise<AppSettingsV1> => {
      const base = settings()
      return {
        ...base,
        provider: {
          ...base.provider,
          activeProviderId: 'xiaomi-token-plan',
          providers: [
            ...base.provider.providers,
            xiaomiTokenPlan!
          ]
        },
        runtime: {
          ...base.runtime,
          providerId: '',
          model: 'deepseek-v4-pro'
        }
      }
    })
    const runtimeRequest = vi.fn(async (path: string, _method?: string, _body?: string) => {
      if (path === '/v1/threads') {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({
            id: 'thr_mimo',
            title: 'MiMo plan',
            workspace: '/tmp/workspace',
            model: 'mimo-v2.5-pro',
            providerId: 'xiaomi-token-plan',
            mode: 'plan',
            status: 'idle',
            createdAt: '2026-06-20T00:00:00.000Z',
            updatedAt: '2026-06-20T00:00:00.000Z'
          })
        }
      }
      return {
        ok: true,
        status: 202,
        body: JSON.stringify({ threadId: 'thr_mimo', turnId: 'turn_mimo', userMessageItemId: 'item_user_mimo' })
      }
    })
    installDsGui({ getSettings, runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.createThread({
      workspace: '/tmp/workspace',
      title: 'MiMo plan',
      mode: 'plan',
      model: 'mimo-v2.5-pro'
    })
    await provider.sendUserMessage('thr_mimo', 'draft the plan', {
      mode: 'plan',
      model: 'mimo-v2.5-pro'
    })
    await provider.sendUserMessage('thr_mimo', 'draft the plan with legacy model id', {
      mode: 'plan',
      model: 'mimo-v2.5-pro-ultraspeed'
    })

    const createBody = JSON.parse(runtimeRequest.mock.calls[0]?.[2] as string) as Record<string, unknown>
    const turnBody = JSON.parse(runtimeRequest.mock.calls[1]?.[2] as string) as Record<string, unknown>
    const legacyTurnBody = JSON.parse(runtimeRequest.mock.calls[2]?.[2] as string) as Record<string, unknown>
    expect(createBody).toMatchObject({
      mode: 'plan',
      model: 'mimo-v2.5-pro',
      providerId: 'xiaomi-token-plan'
    })
    expect(turnBody).toMatchObject({
      mode: 'plan',
      model: 'mimo-v2.5-pro',
      providerId: 'xiaomi-token-plan'
    })
    expect(legacyTurnBody).toMatchObject({
      mode: 'plan',
      model: 'mimo-v2.5-pro',
      providerId: 'xiaomi-token-plan'
    })
    expect(JSON.stringify([createBody, turnBody])).not.toContain('deepseek')
  })

  it('uses the top-level Xiaomi runtime provider when the saved model is the MiMo ultraspeed alias', async () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiProfile = modelProviderPresetProfile(xiaomi!)
    const getSettings = vi.fn(async (): Promise<AppSettingsV1> => {
      const base = settings()
      return {
        ...base,
        provider: {
          ...base.provider,
          activeProviderId: '',
          providers: [
            ...base.provider.providers,
            xiaomiProfile
          ]
        },
        runtime: {
          ...base.runtime,
          providerId: 'xiaomi',
          model: 'mimo-v2.5-pro-ultraspeed'
        }
      }
    })
    const runtimeRequest = vi.fn(async (path: string, _method?: string, _body?: string) => {
      if (path === '/v1/threads') {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({
            id: 'thr_mimo_xiaomi',
            title: 'MiMo plan',
            workspace: '/tmp/workspace',
            model: 'mimo-v2.5-pro',
            providerId: 'xiaomi',
            mode: 'plan',
            status: 'idle',
            createdAt: '2026-06-20T00:00:00.000Z',
            updatedAt: '2026-06-20T00:00:00.000Z'
          })
        }
      }
      return {
        ok: true,
        status: 202,
        body: JSON.stringify({
          threadId: 'thr_mimo_xiaomi',
          turnId: 'turn_mimo_xiaomi',
          userMessageItemId: 'item_user_mimo_xiaomi'
        })
      }
    })
    installDsGui({ getSettings, runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.createThread({
      workspace: '/tmp/workspace',
      title: 'MiMo plan',
      mode: 'plan'
    })
    await provider.sendUserMessage('thr_mimo_xiaomi', 'draft the plan', {
      mode: 'plan',
      model: 'mimo-v2.5-pro-ultraspeed'
    })

    const createBody = JSON.parse(runtimeRequest.mock.calls[0]?.[2] as string) as Record<string, unknown>
    const turnBody = JSON.parse(runtimeRequest.mock.calls[1]?.[2] as string) as Record<string, unknown>
    expect(createBody).toMatchObject({
      mode: 'plan',
      model: 'mimo-v2.5-pro',
      providerId: 'xiaomi'
    })
    expect(turnBody).toMatchObject({
      mode: 'plan',
      model: 'mimo-v2.5-pro',
      providerId: 'xiaomi'
    })
    expect(JSON.stringify([createBody, turnBody])).not.toContain('deepseek')
    expect(JSON.stringify([createBody, turnBody])).not.toContain('mimo-v2.5-pro-ultraspeed')
  })

  it('normalizes MiMo provider and model aliases for review requests', async () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiTokenPlan = modelProviderTokenPlanProfile(xiaomi!)
    expect(xiaomiTokenPlan).not.toBeNull()
    const getSettings = vi.fn(async (): Promise<AppSettingsV1> => {
      const base = settings()
      return {
        ...base,
        provider: {
          ...base.provider,
          activeProviderId: '',
          providers: [
            ...base.provider.providers,
            xiaomiTokenPlan!
          ]
        },
        runtime: {
          ...base.runtime,
          providerId: 'xiaomi-token-plan',
          model: 'mimo-v2.5-pro-ultraspeed'
        }
      }
    })
    let capturedReviewBody = ''
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      capturedReviewBody = body ?? ''
      return {
        ok: true,
        status: 202,
        body: JSON.stringify({
          threadId: 'thr_mimo',
          turnId: 'turn_review_mimo',
          userMessageItemId: 'item_user_review_mimo',
          reviewItemId: 'item_review_mimo'
        })
      }
    })
    installDsGui({ getSettings, runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.reviewThread('thr_mimo', { kind: 'uncommittedChanges' }, {
      model: 'mimo-v2.5-pro-ultraspeed'
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_mimo/review',
      'POST',
      JSON.stringify({
        target: { kind: 'uncommittedChanges' },
        model: 'mimo-v2.5-pro',
        providerId: 'xiaomi-token-plan'
      })
    )
    const body = JSON.parse(capturedReviewBody) as Record<string, unknown>
    expect(JSON.stringify(body)).not.toContain('deepseek')
    expect(JSON.stringify(body)).not.toContain('mimo-v2.5-pro-ultraspeed')
  })

  it('creates checkpoint rewind plans through the Analytix HTTP runtime', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        plan: {
          schemaVersion: 1,
          planId: 'axrp_123',
          checkpointId: 'axcp_123',
          threadId: 'thr_1',
          planDigest: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
          workspace: '/tmp/workspace',
          createdAt: '2026-06-20T10:00:00.000Z',
          scope: 'combined',
          applyMode: 'plan_only',
          destructive: false,
          checkpoint: {
            schemaVersion: 1,
            checkpointId: 'axcp_123',
            threadId: 'thr_1',
            turnId: 'turn_2',
            workspace: '/tmp/workspace',
            createdAt: '2026-06-20T10:00:00.000Z',
            status: 'captured',
            changedFiles: []
          },
          files: [],
          summary: {
            fileCount: 0,
            readyFileCount: 0,
            manualReviewFileCount: 0,
            blockedFileCount: 0,
            retainedEventCount: 0,
            removedEventCount: 0,
            removedTurnCount: 0,
            containsRawPrompt: false,
            containsFullFileContent: false,
            containsSecretValue: false
          }
        }
      })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const plan = await provider.planCheckpointRewind('thr_1', 'axcp_123')

    expect(plan).toMatchObject({ planId: 'axrp_123', applyMode: 'plan_only', destructive: false })
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/checkpoints/axcp_123/rewind-plan',
      'POST',
      JSON.stringify({ scope: 'combined' })
    )
  })

  it('applies checkpoint rewind plans through the Analytix HTTP runtime with confirmation', async () => {
    const plan = {
      schemaVersion: 1 as const,
      planId: 'axrp_123',
      checkpointId: 'axcp_123',
      threadId: 'thr_1',
      planDigest: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      workspace: '/tmp/workspace',
      createdAt: '2026-06-20T10:00:00.000Z',
      scope: 'conversation' as const,
      applyMode: 'plan_only' as const,
      destructive: false as const,
      checkpoint: {
        schemaVersion: 1 as const,
        checkpointId: 'axcp_123',
        threadId: 'thr_1',
        turnId: 'turn_2',
        workspace: '/tmp/workspace',
        createdAt: '2026-06-20T10:00:00.000Z',
        status: 'captured' as const,
        changedFiles: []
      },
      files: [],
      conversation: {
        status: 'ready' as const,
        boundaryTurnId: 'turn_2',
        retainedEventCount: 5,
        removedEventCount: 3,
        removedTurnIds: ['turn_2'],
        projection: {
          latestSeq: 5,
          turnCount: 1,
          itemCount: 1,
          checkpointCount: 1
        }
      },
      summary: {
        fileCount: 0,
        readyFileCount: 0,
        manualReviewFileCount: 0,
        blockedFileCount: 0,
        retainedEventCount: 5,
        removedEventCount: 3,
        removedTurnCount: 1,
        containsRawPrompt: false as const,
        containsFullFileContent: false as const,
        containsSecretValue: false as const
      }
    }
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        apply: {
          schemaVersion: 1,
          applyId: 'axra_123',
          planId: 'axrp_123',
          checkpointId: 'axcp_123',
          threadId: 'thr_1',
          workspace: '/tmp/workspace',
          createdAt: '2026-06-20T10:00:00.000Z',
          scope: 'conversation',
          status: 'applied',
          destructive: true,
          files: [],
          conversation: {
            status: 'audit_recorded',
            boundaryTurnId: 'turn_2',
            retainedEventCount: 5,
            removedEventCount: 3,
            removedTurnIds: ['turn_2'],
            auditEventSeq: 9
          },
          summary: {
            fileAppliedCount: 0,
            fileNoopCount: 0,
            fileManualReviewCount: 0,
            fileBlockedCount: 0,
            fileFailedCount: 0
          },
          auditEventSeq: 9
        }
      })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const apply = await provider.applyCheckpointRewind('thr_1', 'axcp_123', plan)

    expect(apply).toMatchObject({ applyId: 'axra_123', status: 'applied' })
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/checkpoints/axcp_123/rewind-apply',
      'POST',
      JSON.stringify({
        plan,
        confirmation: {
          confirmed: true,
          destructive: true,
          phrase: 'APPLY_CHECKPOINT_REWIND'
        }
      })
    )
  })

  it('posts attachment ids with Analytix turn requests when provided', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 202,
      body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_img', userMessageItemId: 'item_user_img' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.sendUserMessage('thr_1', 'describe this', { attachmentIds: ['att_1'] })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns',
      'POST',
      JSON.stringify({
        prompt: 'describe this',
        async: true,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write',
        attachmentIds: ['att_1']
      })
    )
  })

  it('posts structured file references with Analytix turn requests when provided', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 202,
      body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_file', userMessageItemId: 'item_user_file' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.sendUserMessage('thr_1', 'inspect these files', {
      fileReferences: [{
        path: '/workspace/analytix/src/App.tsx',
        relativePath: 'src/App.tsx',
        name: 'App.tsx',
        kind: 'file'
      }, {
        path: '/workspace/analytix/src',
        relativePath: 'src',
        name: 'src',
        kind: 'directory'
      }]
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns',
      'POST',
      JSON.stringify({
        prompt: 'inspect these files',
        async: true,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write',
        fileReferences: [
          {
            path: '/workspace/analytix/src/App.tsx',
            relativePath: 'src/App.tsx',
            name: 'App.tsx',
            kind: 'file'
          },
          {
            path: '/workspace/analytix/src',
            relativePath: 'src',
            name: 'src',
            kind: 'directory'
          }
        ]
      })
    )
  })

  it('posts explicit reasoning effort with Analytix turn requests', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 202,
      body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_reason', userMessageItemId: 'item_user_reason' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.sendUserMessage('thr_1', 'think harder', {
      model: 'auto',
      reasoningEffort: 'max'
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns',
      'POST',
      JSON.stringify({
        prompt: 'think harder',
        async: true,
        model: 'auto',
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write',
        reasoningEffort: 'max'
      })
    )
  })

  it('rejects invalid reasoning effort before the runtime HTTP request', async () => {
    const runtimeRequest = vi.fn(async () => ({ ok: true, status: 202, body: '{}' }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()
    const sentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'

    for (const reasoningEffort of [' max ', 'MAX', sentinel]) {
      await expect(provider.sendUserMessage('thr_1', 'do not send', {
        reasoningEffort
      } as unknown as { reasoningEffort: 'max' })).rejects.toThrow('reasoning effort is invalid')
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('posts the raise-only case risk intent selected by the host case surface', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 202,
      body: JSON.stringify({ threadId: 'thr_case', turnId: 'turn_case', userMessageItemId: 'item_user_case' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.sendUserMessage('thr_case', 'inspect current evidence', { riskIntent: 'case' })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_case/turns',
      'POST',
      JSON.stringify({
        prompt: 'inspect current evidence',
        async: true,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write',
        riskIntent: 'case'
      })
    )
  })

  it('posts GUI plan context with Analytix plan turn requests', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 202,
      body: JSON.stringify({ threadId: 'thr_1', turnId: 'turn_plan', userMessageItemId: 'item_user_plan' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.sendUserMessage('thr_1', 'refine the plan', {
      mode: 'plan',
      displayText: 'Generate implementation plan',
      guiPlan: {
        operation: 'refine',
        workspaceRoot: '/workspace/analytix',
        relativePath: '.analytixsdd/plan/auth.md',
        planId: '/workspace/analytix:.analytixsdd/plan/auth.md',
        sourceRequest: 'Add auth',
        title: 'auth'
      }
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns',
      'POST',
      JSON.stringify({
        prompt: 'refine the plan',
        async: true,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write',
        displayText: 'Generate implementation plan',
        mode: 'plan',
        guiPlan: {
          operation: 'refine',
          workspaceRoot: '/workspace/analytix',
          relativePath: '.analytixsdd/plan/auth.md',
          planId: '/workspace/analytix:.analytixsdd/plan/auth.md',
          sourceRequest: 'Add auth',
          title: 'auth'
        }
      })
    )
  })

  it('posts interrupt requests with the discard option when requested', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: '{}'
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.interruptTurn('thr_1', 'turn_1', { discard: true })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_1/turns/turn_1/interrupt',
      'POST',
      JSON.stringify({ discard: true })
    )
  })

  it('loads runtime diagnostics and uploads image attachments through Analytix endpoints', async () => {
    const runtimeRequest = vi.fn(async (path: string) => {
      if (path === '/v1/runtime/info') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify(runtimeInfoV2Fixture())
        }
      }
      if (path === '/v1/runtime/tools') {
        return { ok: true, status: 200, body: JSON.stringify(runtimeToolsV2Fixture()) }
      }
      if (path === '/v1/skills') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 2,
            enabled: true,
            available: true,
            reasonCode: 'available',
            configuredRootCount: 1,
            skillCount: 1,
            validationErrorCount: 0,
            skills: [{
              id: 'review',
              name: 'Review',
              scope: 'project',
              legacy: false
            }]
          })
        }
      }
      if (path === '/v1/attachments') {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({
            attachment: {
              id: 'att_1',
              name: 'shot.png',
              mimeType: 'image/png',
              byteSize: 3,
              kind: 'image',
              scope: 'thread',
              createdAt: 't0',
              updatedAt: 't0'
            }
          })
        }
      }
      if (path === '/v1/attachments/att_1/content?thread_id=thr_1&workspace=%2Ftmp%2Fworkspace') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            attachment: {
              id: 'att_1',
              name: 'shot.png',
              mimeType: 'image/png',
              byteSize: 3,
              kind: 'image',
              scope: 'thread',
              createdAt: 't0',
              updatedAt: 't0'
            },
            dataBase64: 'abc'
          })
        }
      }
      return { ok: true, status: 200, body: '{}' }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await expect(provider.getRuntimeInfo()).resolves.toMatchObject({
      capabilities: { attachments: { available: true } }
    })
    await expect(provider.getToolDiagnostics()).resolves.toMatchObject({
      providerCount: 1,
      schemaVersion: 2
    })
    await expect(provider.listSkills()).resolves.toMatchObject({
      validationErrorCount: 0,
      skills: [expect.objectContaining({
        id: 'review',
        name: 'Review',
        scope: 'project',
        legacy: false
      })]
    })
    await expect(provider.uploadAttachment({
      name: 'shot.png',
      mimeType: 'image/png',
      dataBase64: 'abc',
      textFallback: {
        dataBase64: 'xyz',
        mimeType: 'image/webp',
        byteSize: 2,
        width: 1,
        height: 1,
        wasCompressed: true
      },
      threadId: 'thr_1',
      workspace: '/tmp/workspace'
    })).resolves.toMatchObject({ id: 'att_1', name: 'shot.png', kind: 'image', scope: 'thread' })
    await expect(provider.getAttachmentContent('att_1', {
      threadId: 'thr_1',
      workspace: '/tmp/workspace'
    })).resolves.toMatchObject({
      attachment: { id: 'att_1', mimeType: 'image/png' },
      dataBase64: 'abc'
    })
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/attachments',
      'POST',
      JSON.stringify({
        name: 'shot.png',
        mimeType: 'image/png',
        dataBase64: 'abc',
        textFallback: {
          dataBase64: 'xyz',
          mimeType: 'image/webp',
          byteSize: 2,
          width: 1,
          height: 1,
          wasCompressed: true
        },
        threadId: 'thr_1',
        workspace: '/tmp/workspace'
      })
    )
  })

  it.each([0, 1])('preserves incomplete skill discovery diagnostics with %i usable skills', async (skillCount) => {
    const response = { schemaVersion: 2, enabled: true, available: skillCount > 0,
      reasonCode: skillCount > 0 ? 'available' : 'unavailable', configuredRootCount: 0,
      skillCount, validationErrorCount: 1,
      skills: skillCount ? [{ id: 'analytix-documents', name: 'Analytix Documents', scope: 'global', legacy: false }] : [] }
    const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(response) }))
    installDsGui({ runtimeRequest })
    await expect(new AnalytixRuntimeProvider().listSkills()).resolves.toEqual(response)
  })

  it('rejects legacy or private runtime skill catalog responses', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        skills: [{
          id: 'review',
          name: 'Review',
          root: '/Users/private/skills'
        }]
      })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await expect(provider.listSkills()).rejects.toThrow('runtime_response_schema_invalid')
  })

  it('lists, disables, and deletes memory records through Analytix endpoints', async () => {
    const runtimeRequest = vi.fn(async (path: string, method?: string, body?: string) => {
      if (path === '/v1/memory?workspace=%2Ftmp%2Fworkspace&include_deleted=false') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            memories: [{
              id: 'mem_1',
              content: 'Use pnpm',
              scope: 'workspace',
              workspace: '/tmp/workspace',
              tags: ['tooling'],
              confidence: 0.9,
              createdAt: 't0',
              updatedAt: 't0'
            }]
          })
        }
      }
      if (path === '/v1/memory/mem_1' && method === 'PATCH') {
        expect(body).toBe(JSON.stringify({ disabled: true }))
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            memory: {
              id: 'mem_1',
              content: 'Use pnpm',
              scope: 'workspace',
              disabledAt: 't1',
              createdAt: 't0',
              updatedAt: 't1'
            }
          })
        }
      }
      if (path === '/v1/memory/mem_1' && method === 'DELETE') {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            memory: {
              id: 'mem_1',
              content: 'Use pnpm',
              scope: 'workspace',
              deletedAt: 't2',
              createdAt: 't0',
              updatedAt: 't2'
            }
          })
        }
      }
      return { ok: true, status: 200, body: '{}' }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await expect(provider.listMemories({ workspace: '/tmp/workspace', includeDeleted: false })).resolves.toHaveLength(1)
    await expect(provider.updateMemory('mem_1', { disabled: true })).resolves.toMatchObject({
      id: 'mem_1',
      disabledAt: 't1'
    })
    await expect(provider.deleteMemory('mem_1')).resolves.toMatchObject({
      id: 'mem_1',
      deletedAt: 't2'
    })
  })

  it('calls Analytix fork and user-input compatibility endpoints', async () => {
    const runtimeRequest = vi.fn(async (path: string) => ({
      ok: true,
      status: 200,
      body: path.includes('/fork')
        ? JSON.stringify({
            id: 'thr_fork',
            title: 'Forked',
            workspace: '/tmp/workspace',
            model: 'deepseek-chat',
            mode: 'agent',
            status: 'idle',
            forkedFromThreadId: 'thr_parent',
            createdAt: 't0',
            updatedAt: 't1'
          })
        : '{}'
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const forked = await provider.forkThread('thr_parent')
    await provider.forkThread('thr_parent', { relation: 'fork', turnId: 'turn_parent_1' })
    await provider.submitUserInputResponse('input_1', [{ id: 'choice', label: 'Yes', value: 'yes' }])
    await provider.cancelUserInput('input_2')

    expect(forked).toMatchObject({ id: 'thr_fork', forkedFromThreadId: 'thr_parent' })
    expect(runtimeRequest).toHaveBeenCalledWith('/v1/threads/thr_parent/fork', 'POST')
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/threads/thr_parent/fork',
      'POST',
      JSON.stringify({ relation: 'fork', turnId: 'turn_parent_1' })
    )
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/user-inputs/input_1',
      'POST',
      JSON.stringify({ answers: [{ id: 'choice', label: 'Yes', value: 'yes' }] })
    )
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/user-inputs/input_2',
      'POST',
      JSON.stringify({ cancelled: true })
    )
  })

  it('URL-encodes dynamic runtime route ids before calling the bridge', async () => {
    const threadId = 'thr:route'
    const taskId = 'taskjob:task-route'
    const turnId = 'turn/route?x=1#frag'
    const approvalId = 'appr/route?x=1#frag'
    const inputId = 'input/route?x=1#frag'
    const sessionId = 'sess/route?x=1#frag'
    const encodedThread = encodeURIComponent(threadId)
    const encodedTask = encodeURIComponent(taskId)
    const encodedTurn = encodeURIComponent(turnId)
    const encodedApproval = encodeURIComponent(approvalId)
    const encodedInput = encodeURIComponent(inputId)
    const encodedSession = encodeURIComponent(sessionId)
    const runtimeRequest = vi.fn(async (path: string) => {
      if (path.endsWith('/turns')) {
        return {
          ok: true,
          status: 202,
          body: JSON.stringify({ threadId, turnId: 'turn_created' })
        }
      }
      if (path.endsWith('/fork')) {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({
            id: 'thr_fork',
            title: 'Forked',
            workspace: '/tmp/workspace',
            model: 'deepseek-chat',
            mode: 'agent',
            status: 'idle',
            forkedFromThreadId: threadId,
            createdAt: 't0',
            updatedAt: 't1'
          })
        }
      }
      if (path.endsWith('/resume-thread')) {
        return {
          ok: true,
          status: 201,
          body: JSON.stringify({ thread_id: 'thr_resumed', session_id: sessionId })
        }
      }
      if (path.endsWith('/rewind')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            threadId,
            turnId,
            removedTurns: 1,
            remainingTurns: 0,
            removedTurnIds: [turnId]
          })
        }
      }
      if (path === `/v1/threads/${encodedThread}/summary`) {
        return { ok: true, status: 200, body: JSON.stringify(summaryResponse(threadId)) }
      }
      if (path === `/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/output?offset=2`) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            availability: 'withheld',
            taskId,
            status: 'running',
            reasonCode: 'security_bound_child_output',
            outputWithheld: true,
            outputTrustStatus: 'untrusted_child_output',
            factAnswerAllowed: false,
            evidenceAuthority: false,
            canReadOutput: false,
            canContinueParent: false
          })
        }
      }
      if (path === `/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/kill` ||
        path === `/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/restart`) {
        const killed = path.endsWith('/kill')
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            task: {
              schemaVersion: 1,
              id: taskId,
              kind: 'task',
              status: killed ? 'killed' : 'queued',
              background: false,
              terminal: killed,
              outputWithheld: true,
              outputTrustStatus: 'untrusted_child_output',
              factAnswerAllowed: false,
              evidenceAuthority: false,
              canReadOutput: false,
              canContinueParent: false
            }
          })
        }
      }
      return { ok: true, status: 200, body: '{}' }
    })
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.sendUserMessage(threadId, 'hello')
    await provider.rewindThread(threadId, turnId)
    await provider.steerUserMessage({ threadId, turnId, text: 'steer' })
    await provider.interruptTurn(threadId, turnId)
    await provider.compactThread(threadId)
    await provider.submitApprovalDecision(approvalId, 'allow')
    await provider.submitUserInputResponse(inputId, [])
    await provider.cancelUserInput(inputId)
    await provider.forkThread(threadId)
    await provider.resumeSession(sessionId)
    await provider.getThreadSummary(threadId)
    await provider.getThreadSummaryTaskOutput(threadId, taskId, { offset: 2 })
    await provider.killThreadSummaryTask(threadId, taskId)
    await provider.restartThreadSummaryTask(threadId, taskId)

    const paths = runtimeRequest.mock.calls.map(([path]) => path)
    expect(paths).toEqual(expect.arrayContaining([
      `/v1/threads/${encodedThread}/turns`,
      `/v1/threads/${encodedThread}/rewind`,
      `/v1/threads/${encodedThread}/turns/${encodedTurn}/steer`,
      `/v1/threads/${encodedThread}/turns/${encodedTurn}/interrupt`,
      `/v1/threads/${encodedThread}/compact`,
      `/v1/approvals/${encodedApproval}`,
      `/v1/user-inputs/${encodedInput}`,
      `/v1/threads/${encodedThread}/fork`,
      `/v1/sessions/${encodedSession}/resume-thread`,
      `/v1/threads/${encodedThread}/summary`,
      `/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/output?offset=2`,
      `/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/kill`,
      `/v1/threads/${encodedThread}/summary/tasks/${encodedTask}/restart`
    ]))
    expectRuntimeRequestPathsStayAnalytixOwned(paths)
    for (const path of paths) {
      expect(path).not.toContain(threadId)
      expect(path).not.toContain(taskId)
      expect(path).not.toContain(turnId)
      expect(path).not.toContain(approvalId)
      expect(path).not.toContain(inputId)
      expect(path).not.toContain(sessionId)
    }
  })

  it('resumes a session through the Analytix HTTP runtime', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 201,
      body: JSON.stringify({ thread_id: 'thr_resumed', session_id: 'sess_1' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    const result = await provider.resumeSession('sess_1', { mode: 'plan' })

    expect(result).toEqual({ threadId: 'thr_resumed', sessionId: 'sess_1' })
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/sessions/sess_1/resume-thread',
      'POST',
      JSON.stringify({
        workspace: '/tmp/workspace',
        model: defaultAnalytixRuntimeSettings().model,
        providerId: 'deepseek',
        mode: 'plan'
      })
    )
  })

  it('passes providerId through resume session requests', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 201,
      body: JSON.stringify({ thread_id: 'thr_resumed', session_id: 'sess_1' })
    }))
    installDsGui({ runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.resumeSession('sess_1', {
      model: 'glm-5',
      providerId: 'zai-coding-plan',
      mode: 'agent'
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/sessions/sess_1/resume-thread',
      'POST',
      JSON.stringify({
        workspace: '/tmp/workspace',
        model: 'glm-5',
        providerId: 'zai-coding-plan',
        mode: 'agent'
      })
    )
  })

  it('normalizes MiMo runtime model aliases when resuming plan sessions', async () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiProfile = modelProviderTokenPlanProfile(xiaomi!)
    expect(xiaomiProfile).not.toBeNull()
    const getSettings = vi.fn(async (): Promise<AppSettingsV1> => {
      const base = settings()
      return {
        ...base,
        provider: {
          ...base.provider,
          activeProviderId: '',
          providers: [
            ...base.provider.providers,
            xiaomiProfile!
          ]
        },
        runtime: {
          ...base.runtime,
          providerId: 'xiaomi-token-plan',
          model: 'mimo-v2.5-pro-ultraspeed'
        }
      }
    })
    let capturedResumeBody = ''
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      capturedResumeBody = body ?? ''
      return {
        ok: true,
        status: 201,
        body: JSON.stringify({ thread_id: 'thr_resumed', session_id: 'sess_mimo' })
      }
    })
    installDsGui({ getSettings, runtimeRequest })
    const provider = new AnalytixRuntimeProvider()

    await provider.resumeSession('sess_mimo', { mode: 'plan' })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/sessions/sess_mimo/resume-thread',
      'POST',
      JSON.stringify({
        workspace: '/tmp/workspace',
        model: 'mimo-v2.5-pro',
        providerId: 'xiaomi-token-plan',
        mode: 'plan'
      })
    )
    const body = JSON.parse(capturedResumeBody) as Record<string, unknown>
    expect(JSON.stringify(body)).not.toContain('deepseek')
    expect(JSON.stringify(body)).not.toContain('mimo-v2.5-pro-ultraspeed')
  })

  it('ignores assistant draft deltas while advancing the renderer cursor', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
    const stopSse = vi.fn(async () => true)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-1',
          events: [
            {
              kind: 'assistant_text_delta',
              seq: 3,
              item: {
                id: 'item_text',
                turnId: 'turn_1',
                threadId: 'thr_1',
                role: 'assistant',
                status: 'running',
                createdAt: 't1',
                kind: 'assistant_text',
                text: 'he'
              }
            }
          ]
        })
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const ac = new AbortController()
    const ackSseEvent = vi.fn(async () => true)
    const sink: ThreadEventSink = {
      onSeq: vi.fn(() => ac.abort()),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      ackSseEvent,
      runtimeRequest,
      startSse,
      stopSse
    })
    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 2, sink, ac.signal)
    const streamId = startSse.mock.calls[0]?.[2]
    expect(startSse).toHaveBeenCalledTimes(1)
    expect(startSse).toHaveBeenCalledWith('thr_1', 2, streamId)
    expect(typeof streamId).toBe('string')
    expect(streamId?.length).toBeGreaterThan(0)
    expect(runtimeRequest).not.toHaveBeenCalled()
    expect(stopSse).toHaveBeenCalledWith(streamId)
    expect(sink.onSeq).toHaveBeenCalledWith(3)
    expect(ackSseEvent).toHaveBeenCalledWith(streamId, 3)
    expect(sink.onDeltas).not.toHaveBeenCalled()
  })

  it('dispatches public projection revocation as a control and never ACKs it', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const onPublicProjectionRevoked = vi.fn(async () => undefined)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-revoked',
          events: [{
            schemaVersion: 1,
            kind: 'public_projection_revoked',
            threadId: 'thr_1',
            historyAuthority: 'case_boundary_only_v1',
            code: 'case_public_authority_rejected',
            action: 'purge_case_projection',
            terminal: true
          }]
        })
      })
      return { streamId: streamId ?? 'stream-revoked' }
    })
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onPublicProjectionRevoked,
      onTurnComplete: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      ackSseEvent,
      startSse,
      stopSse
    })

    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 9, sink, new AbortController().signal)

    const streamId = startSse.mock.calls[0]?.[2]
    expect(onPublicProjectionRevoked).toHaveBeenCalledTimes(1)
    expect(sink.onSeq).not.toHaveBeenCalled()
    expect(sink.onDeltas).not.toHaveBeenCalled()
    expect(ackSseEvent).not.toHaveBeenCalled()
    expect(stopSse).toHaveBeenCalledWith(streamId)
  })

  it('commits one accepted-final batch before ACKing only its terminal sequence', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    let onEnd: ((payload: { streamId: string }) => void) | null = null
    let releaseCommit!: () => void
    let observedCommit!: () => void
    const commitStarted = new Promise<void>((resolve) => { observedCommit = resolve })
    const commitGate = new Promise<void>((resolve) => { releaseCommit = resolve })
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        const sid = streamId ?? 'stream-accepted-final'
        onData?.({ streamId: sid, events: [acceptedFinalDeliveryBatchForRenderer()] })
        queueMicrotask(() => onEnd?.({ streamId: sid }))
      })
      return { streamId: streamId ?? 'stream-accepted-final' }
    })
    const onAcceptedFinalBatch = vi.fn(async (batch: AcceptedFinalProjectionBatch) => {
      observedCommit()
      await commitGate
      return batch.receipt
    })
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onAcceptedFinalBatch,
      onTurnComplete: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      onSseEnd: vi.fn((handler) => {
        onEnd = handler
        return () => undefined
      }),
      ackSseEvent,
      startSse,
      stopSse
    })

    const provider = new AnalytixRuntimeProvider()
    const subscription = provider.subscribeThreadEvents('thr_case', 19, sink, new AbortController().signal)
    await commitStarted
    expect(onAcceptedFinalBatch).toHaveBeenCalledTimes(1)
    expect(sink.onSeq).not.toHaveBeenCalled()
    expect(ackSseEvent).not.toHaveBeenCalled()

    releaseCommit()
    await subscription
    const streamId = startSse.mock.calls[0]?.[2]
    expect(ackSseEvent).toHaveBeenCalledTimes(1)
    expect(ackSseEvent).toHaveBeenCalledWith(streamId, 22, {
      batchId: '7'.repeat(64),
      threadId: 'thr_case',
      turnId: 'turn_case',
      publicationCommitId: 'f'.repeat(64)
    })
    expect(sink.onUsage).toBeUndefined()
    expect(sink.onTurnComplete).not.toHaveBeenCalled()
    expect(sink.onError).not.toHaveBeenCalled()
  })

  it('commits one general terminal batch before ACKing its terminal sequence without authority', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    let onEnd: ((payload: { streamId: string }) => void) | null = null
    let releaseCommit!: () => void
    let observedCommit!: () => void
    const commitStarted = new Promise<void>((resolve) => { observedCommit = resolve })
    const commitGate = new Promise<void>((resolve) => { releaseCommit = resolve })
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        const sid = streamId ?? 'stream-general-terminal'
        onData?.({ streamId: sid, events: [generalTerminalDeliveryBatchForRenderer()] })
        queueMicrotask(() => onEnd?.({ streamId: sid }))
      })
      return { streamId: streamId ?? 'stream-general-terminal' }
    })
    const onGeneralTerminalBatch = vi.fn(async () => {
      observedCommit()
      await commitGate
    })
    const sink: ThreadEventSink = {
      onSeq: vi.fn(), onDeltas: vi.fn(), onUserMessage: vi.fn(), onTool: vi.fn(),
      onCompaction: vi.fn(), onApproval: vi.fn(), onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(), onGoal: vi.fn(), onTodos: vi.fn(),
      onGeneralTerminalBatch, onTurnComplete: vi.fn(), onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      onSseEnd: vi.fn((handler) => {
        onEnd = handler
        return () => undefined
      }),
      ackSseEvent,
      startSse,
      stopSse
    })

    const provider = new AnalytixRuntimeProvider()
    const subscription = provider.subscribeThreadEvents('thr_general', 19, sink, new AbortController().signal)
    await commitStarted
    expect(onGeneralTerminalBatch).toHaveBeenCalledTimes(1)
    expect(sink.onSeq).not.toHaveBeenCalled()
    expect(ackSseEvent).not.toHaveBeenCalled()

    releaseCommit()
    await subscription
    const streamId = startSse.mock.calls[0]?.[2]
    expect(sink.onSeq).toHaveBeenCalledWith(22)
    expect(ackSseEvent).toHaveBeenCalledWith(streamId, 22)
    expect(sink.onTurnComplete).not.toHaveBeenCalled()
    expect(sink.onError).not.toHaveBeenCalled()
  })

  it('does not ACK an accepted-final batch when the store returns a mismatched receipt', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-mismatched-receipt',
          events: [acceptedFinalDeliveryBatchForRenderer()]
        })
      })
      return { streamId: streamId ?? 'stream-mismatched-receipt' }
    })
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onAcceptedFinalBatch: vi.fn(async (batch) => ({
        ...batch.receipt,
        batchId: '0'.repeat(64)
      })),
      onTurnComplete: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      ackSseEvent,
      startSse,
      stopSse
    })

    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_case', 19, sink, new AbortController().signal)

    expect(sink.onAcceptedFinalBatch).toHaveBeenCalledTimes(1)
    expect(ackSseEvent).not.toHaveBeenCalled()
    expect(stopSse).toHaveBeenCalled()
  })

  it('does not ACK an accepted-final batch when its owner aborts during store commit', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const controller = new AbortController()
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-aborted-receipt',
          events: [acceptedFinalDeliveryBatchForRenderer()]
        })
      })
      return { streamId: streamId ?? 'stream-aborted-receipt' }
    })
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onAcceptedFinalBatch: vi.fn(async (batch) => {
        controller.abort()
        return batch.receipt
      }),
      onTurnComplete: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      ackSseEvent,
      startSse,
      stopSse
    })

    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_case', 19, sink, controller.signal)

    expect(sink.onAcceptedFinalBatch).toHaveBeenCalledTimes(1)
    expect(ackSseEvent).not.toHaveBeenCalled()
    expect(stopSse).toHaveBeenCalled()
  })

  it('does not ack renderer SSE batches when dispatch fails before applying them', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    let onEnd: ((payload: { streamId: string }) => void) | null = null
    const ackSseEvent = vi.fn(async () => true)
    const stopSse = vi.fn(async () => true)
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        const sid = streamId ?? 'stream-1'
        onData?.({
          streamId: sid,
          events: [
            {
              kind: 'tool_progress',
              seq: 4,
              turnId: 'turn_1',
              toolName: 'read',
              callId: 'call_read',
              summary: 'reading',
              status: 'running'
            }
          ]
        })
        queueMicrotask(() => onEnd?.({ streamId: sid }))
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onTool: vi.fn(() => {
        throw new Error('projection failed before apply')
      }),
      onUserMessage: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      onSseEnd: vi.fn((handler) => {
        onEnd = handler
        return () => undefined
      }),
      ackSseEvent,
      startSse,
      stopSse
    })

    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 2, sink, new AbortController().signal)

    expect(sink.onTool).toHaveBeenCalled()
    expect(sink.onSeq).not.toHaveBeenCalled()
    expect(ackSseEvent).not.toHaveBeenCalled()
    expect(stopSse).toHaveBeenCalled()
  })

  it('settles renderer SSE transport errors as visible terminal runtime errors', async () => {
    let onError: ((payload: { streamId: string; message?: string; status?: number }) => void) | null = null
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onError?.({ streamId: streamId ?? 'stream-1', message: 'SSE setup failed', status: 500 })
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const stopSse = vi.fn(async () => true)
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(),
      onRuntimeError: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseError: vi.fn((handler) => {
        onError = handler
        return () => undefined
      }),
      startSse,
      stopSse
    })
    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 2, sink, new AbortController().signal)
    const streamId = startSse.mock.calls[0]?.[2]

    expect(stopSse).toHaveBeenCalledWith(streamId)
    expect(sink.onRuntimeError).toHaveBeenCalledWith(expect.objectContaining({
      itemId: expect.stringContaining('sse_stream_error'),
      message: 'SSE setup failed',
      code: 'sse_stream_error',
      details: { status: 500 },
      severity: 'error'
    }))
    expect(sink.onError).toHaveBeenCalledWith(expect.any(Error), { terminal: false })
    expect(JSON.parse((sink.onError as ReturnType<typeof vi.fn>).mock.calls[0][0].message)).toMatchObject({
      code: 'sse_stream_error',
      message: 'SSE setup failed',
      details: { status: 500 },
      severity: 'error'
    })
  })

  it('preserves a controlled main-process SSE error code at the renderer boundary', async () => {
    let onError: ((payload: { streamId: string; message?: string; status?: number; code?: string }) => void) | null = null
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onError?.({
          streamId: streamId ?? 'stream-1',
          code: 'sse_event_rejected',
          message: 'Runtime request failed (sse_event_rejected).'
        })
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const stopSse = vi.fn(async () => true)
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(),
      onRuntimeError: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseError: vi.fn((handler) => {
        onError = handler
        return () => undefined
      }),
      startSse,
      stopSse
    })
    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 2, sink, new AbortController().signal)
    const streamId = startSse.mock.calls[0]?.[2]

    expect(stopSse).toHaveBeenCalledWith(streamId)
    expect(sink.onRuntimeError).toHaveBeenCalledWith(expect.objectContaining({
      itemId: expect.stringContaining('sse_event_rejected'),
      message: 'Runtime request failed (sse_event_rejected).',
      code: 'sse_event_rejected',
      severity: 'error'
    }))
    expect(sink.onRuntimeError).not.toHaveBeenCalledWith(expect.objectContaining({
      details: expect.objectContaining({ status: expect.anything() })
    }))
    expect(JSON.parse((sink.onError as ReturnType<typeof vi.fn>).mock.calls[0][0].message)).toMatchObject({
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      severity: 'error'
    })
  })

  it('settles structured SSE setup error events even when seq equals the subscription cursor', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    let onEnd: ((payload: { streamId: string }) => void) | null = null
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-1',
          events: [{
            kind: 'error',
            seq: 5,
            timestamp: '2026-07-01T00:00:00.000Z',
            threadId: 'thr_1',
            code: 'sse_setup_error',
            message: 'SSE setup failed',
            severity: 'error',
            terminal: true,
            details: { phase: 'highest_seq' }
          }]
        })
        onEnd?.({ streamId: streamId ?? 'stream-1' })
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const stopSse = vi.fn(async () => true)
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(),
      onRuntimeError: vi.fn(),
      onError: vi.fn()
    }
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      onSseEnd: vi.fn((handler) => {
        onEnd = handler
        return () => undefined
      }),
      startSse,
      stopSse
    })
    const provider = new AnalytixRuntimeProvider()

    await provider.subscribeThreadEvents('thr_1', 5, sink, new AbortController().signal)

    const streamId = startSse.mock.calls[0]?.[2]
    expect(startSse).toHaveBeenCalledWith('thr_1', 5, streamId)
    expect(sink.onSeq).toHaveBeenCalledWith(5)
    expect(sink.onRuntimeError).toHaveBeenCalledWith(expect.objectContaining({
      itemId: 'runtime_error_thr_1',
      message: 'SSE setup failed',
      code: 'sse_setup_error',
      details: { phase: 'highest_seq' },
      severity: 'error'
    }))
    expect(sink.onError).toHaveBeenCalledWith(expect.any(Error), {
      terminal: true,
      threadId: 'thr_1',
      turnId: undefined,
      seq: 5,
      status: 'failed'
    })
    expect(JSON.parse((sink.onError as ReturnType<typeof vi.fn>).mock.calls[0][0].message)).toMatchObject({
      code: 'sse_setup_error',
      message: 'SSE setup failed',
      details: { phase: 'highest_seq' },
      severity: 'error'
    })
    expect(stopSse).toHaveBeenCalledWith(streamId)
  })

  it('advances the renderer SSE cursor to the max seq in each delivered batch', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const ac = new AbortController()
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(() => ac.abort()),
      onError: vi.fn()
    }
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-1',
          events: [
            { kind: 'pipeline_stage', seq: 4, stage: 'provider_ready', label: 'Provider ready' },
            { kind: 'heartbeat', seq: 6 },
            { kind: 'turn_completed', seq: 7 }
          ]
        })
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const stopSse = vi.fn(async () => true)
    installDsGui({
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      startSse,
      stopSse
    })
    const provider = new AnalytixRuntimeProvider()

    await provider.subscribeThreadEvents('thr_1', 3, sink, ac.signal)

    const streamId = startSse.mock.calls[0]?.[2]
    expect(startSse).toHaveBeenCalledWith('thr_1', 3, streamId)
    expect(sink.onSeq).toHaveBeenCalledWith(7)
    expect(sink.onDeltas).not.toHaveBeenCalled()
    expect(sink.onTurnComplete).toHaveBeenCalled()
    expect(stopSse).toHaveBeenCalledWith(streamId)
  })

  it('serializes renderer SSE batches when an earlier batch awaits approval policy', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const approvalSettings = deferred<AppSettingsV1>()
    const order: string[] = []
    const ac = new AbortController()
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(() => {
        order.push('approval')
      }),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(() => {
        order.push('complete')
        ac.abort()
      }),
      onError: vi.fn()
    }
    const startSse = vi.fn(async (_threadId, _sinceSeq, streamId) => {
      queueMicrotask(() => {
        onData?.({
          streamId: streamId ?? 'stream-1',
          events: [{
            kind: 'approval_requested',
            seq: 4,
            approvalId: 'appr_ordered',
            summary: 'Need approval before completion'
          }]
        })
        onData?.({
          streamId: streamId ?? 'stream-1',
          events: [{ kind: 'turn_completed', seq: 5 }]
        })
        queueMicrotask(() => {
          approvalSettings.resolve({
            ...settings(),
            runtime: { ...defaultAnalytixRuntimeSettings(), approvalPolicy: 'on-request' }
          })
        })
      })
      return { streamId: streamId ?? 'stream-1' }
    })
    const stopSse = vi.fn(async () => true)
    installDsGui({
      getSettings: vi.fn(() => approvalSettings.promise),
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      startSse,
      stopSse
    })
    const provider = new AnalytixRuntimeProvider()

    await provider.subscribeThreadEvents('thr_1', 0, sink, ac.signal)

    expect(order).toEqual(['approval', 'complete'])
    expect(sink.onSeq).toHaveBeenCalledWith(4)
    expect(sink.onSeq).toHaveBeenCalledWith(5)
    expect(sink.onApproval).toHaveBeenCalledWith({
      approvalId: 'appr_ordered',
      summary: 'Need approval before completion',
      toolName: undefined
    })
    expect(sink.onTurnComplete).toHaveBeenCalled()
  })

  it('maps snake_case approval request ids before rendering the approval card', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const getSettings = vi.fn(async (): Promise<AppSettingsV1> => ({
      ...settings(),
      runtime: { ...defaultAnalytixRuntimeSettings(), approvalPolicy: 'auto' }
    }))
    const ac = new AbortController()
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(() => ac.abort()),
      onError: vi.fn()
    }
    installDsGui({
      getSettings,
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      startSse: vi.fn(async (_threadId, _sinceSeq, streamId) => {
        queueMicrotask(() => {
          onData?.({
            streamId: streamId ?? 'stream-1',
            events: [
              {
                kind: 'approval_requested',
                seq: 4,
                approval_id: 'appr_snake',
                item_id: 'item_approval_snake',
                turn_id: 'turn_snake',
                tool_name: 'shell',
                approval_policy: 'on-request',
                summary: 'Need snake approval'
              },
              { kind: 'turn_completed', seq: 5 }
            ]
          })
        })
        return { streamId: streamId ?? 'stream-1' }
      })
    })
    const provider = new AnalytixRuntimeProvider()

    await provider.subscribeThreadEvents('thr_1', 0, sink, ac.signal)

    expect(getSettings).not.toHaveBeenCalled()
    expect(sink.onApproval).toHaveBeenCalledWith({
      approvalId: 'appr_snake',
      summary: 'Need snake approval',
      toolName: 'shell',
      turnId: 'turn_snake',
      meta: { turnId: 'turn_snake' }
    })
  })

  it('auto-approves approval requests when policy is auto', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
    const ac = new AbortController()
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(() => ac.abort()),
      onError: vi.fn()
    }
    const autoSettings: AppSettingsV1 = {
      ...settings(),
      runtime: { ...defaultAnalytixRuntimeSettings(), approvalPolicy: 'auto' }
    }
    installDsGui({
      getSettings: vi.fn(async () => autoSettings),
      runtimeRequest,
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      startSse: vi.fn(async (_threadId, _sinceSeq, streamId) => {
        queueMicrotask(() => {
          onData?.({
            streamId: streamId ?? 'stream-1',
            events: [
              { kind: 'approval_requested', seq: 4, approvalId: 'appr_auto', summary: 'Need approval' },
              { kind: 'turn_completed', seq: 5 }
            ]
          })
        })
        return { streamId: streamId ?? 'stream-1' }
      })
    })
    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 0, sink, ac.signal)
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/approvals/appr_auto',
      'POST',
      JSON.stringify({ decision: 'allow' })
    )
    expect(sink.onApproval).not.toHaveBeenCalled()
  })

  it('uses the approval policy from runtime events before falling back to settings', async () => {
    let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
    const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
    const getSettings = vi.fn(async (): Promise<AppSettingsV1> => ({
      ...settings(),
      runtime: { ...defaultAnalytixRuntimeSettings(), approvalPolicy: 'on-request' }
    }))
    const ac = new AbortController()
    const sink: ThreadEventSink = {
      onSeq: vi.fn(),
      onDeltas: vi.fn(),
      onUserMessage: vi.fn(),
      onTool: vi.fn(),
      onCompaction: vi.fn(),
      onApproval: vi.fn(),
      onUserInput: vi.fn(),
      onUserInputStatus: vi.fn(),
      onGoal: vi.fn(),
      onTodos: vi.fn(),
      onTurnComplete: vi.fn(() => ac.abort()),
      onError: vi.fn()
    }
    installDsGui({
      getSettings,
      runtimeRequest,
      onSseEvent: vi.fn((handler) => {
        onData = handler
        return () => undefined
      }),
      startSse: vi.fn(async (_threadId, _sinceSeq, streamId) => {
        queueMicrotask(() => {
          onData?.({
            streamId: streamId ?? 'stream-1',
            events: [
              {
                kind: 'approval_requested',
                seq: 4,
                approvalId: 'appr_event_auto',
                approvalPolicy: 'auto',
                summary: 'Need approval'
              },
              { kind: 'turn_completed', seq: 5 }
            ]
          })
        })
        return { streamId: streamId ?? 'stream-1' }
      })
    })
    const provider = new AnalytixRuntimeProvider()
    await provider.subscribeThreadEvents('thr_1', 0, sink, ac.signal)
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/approvals/appr_event_auto',
      'POST',
      JSON.stringify({ decision: 'allow' })
    )
    expect(getSettings).not.toHaveBeenCalled()
    expect(sink.onApproval).not.toHaveBeenCalled()
  })

  it('renders approval cards for suggest and untrusted policies', async () => {
    for (const policy of ['suggest', 'untrusted'] as const) {
      let onData: ((payload: { streamId: string; events: unknown[] }) => void) | null = null
      const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
      const ac = new AbortController()
      const sink: ThreadEventSink = {
        onSeq: vi.fn(),
        onDeltas: vi.fn(),
        onUserMessage: vi.fn(),
        onTool: vi.fn(),
        onCompaction: vi.fn(),
        onApproval: vi.fn(),
        onUserInput: vi.fn(),
        onUserInputStatus: vi.fn(),
        onGoal: vi.fn(),
        onTodos: vi.fn(),
        onTurnComplete: vi.fn(() => ac.abort()),
        onError: vi.fn()
      }
      const policySettings: AppSettingsV1 = {
        ...settings(),
        runtime: { ...defaultAnalytixRuntimeSettings(), approvalPolicy: policy }
      }
      installDsGui({
        getSettings: vi.fn(async () => policySettings),
        runtimeRequest,
        onSseEvent: vi.fn((handler) => {
          onData = handler
          return () => undefined
        }),
        startSse: vi.fn(async (_threadId, _sinceSeq, streamId) => {
          queueMicrotask(() => {
            onData?.({
              streamId: streamId ?? 'stream-1',
              events: [
                {
                  kind: 'approval_requested',
                  seq: 6,
                  approvalId: `appr_${policy}`,
                  summary: `${policy} approval`
                },
                { kind: 'turn_completed', seq: 7 }
              ]
            })
          })
          return { streamId: streamId ?? 'stream-1' }
        })
      })
      const provider = new AnalytixRuntimeProvider()
      await provider.subscribeThreadEvents('thr_1', 0, sink, ac.signal)
      expect(sink.onApproval).toHaveBeenCalledWith({
        approvalId: `appr_${policy}`,
        summary: `${policy} approval`,
        toolName: undefined
      })
      expect(runtimeRequest).not.toHaveBeenCalledWith(
        `/v1/approvals/appr_${policy}`,
        'POST',
        expect.any(String)
      )
    }
  })
})

describe('registry', () => {
  it('returns a cached provider for the analytix id', () => {
    resetProviderCacheForTests()
    const first = getProvider()
    const second = getProvider()
    expect(first).toBe(second)
  })

})
