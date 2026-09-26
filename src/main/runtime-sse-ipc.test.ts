import type { IpcMain, WebContents } from 'electron'
import { createHash, generateKeyPairSync, sign, type KeyObject } from 'node:crypto'
import { EventEmitter } from 'node:events'
import { readFileSync } from 'node:fs'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AcceptedFinalDeliveryBatchV1Schema,
  GENERAL_PROVIDER_FINAL_QUARANTINED_TEXT
} from '../../packages/runtime/src/contracts/events'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../shared/app-settings'
import { registerRuntimeSseIpc } from './runtime-sse-ipc'
import type { ThreadTraceEventPayload } from '../shared/thread-trace'
import {
  acceptedFinalPublicationEventId,
  acceptedFinalPublicationPayloadDigest
} from './accepted-final-publication'

type IpcHandler = (event: { sender: WebContents }, args?: unknown) => Promise<unknown>

const FORBIDDEN_PUBLIC_SSE_ROUTE =
  /^\/v1\/(?:reasonix(?:\/|$)|runtime\/go(?:\/|$)|workflows?(?:\/|$)|create-loop(?:\/|$)|subagents?(?:\/|$)|autoresearch(?:\/|$)|mcp-indexer(?:\/|$))/

function settingsForPort(port: number): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: {
      ...defaultAnalytixRuntimeSettings(port),
      runtimeToken: 'runtime-token'
    },
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

function registerHarness(
  fetchImpl: typeof fetch,
  overrides: {
    ensureRuntime?: (settings: AppSettingsV1) => Promise<AppSettingsV1 | void>
    store?: { load: () => Promise<AppSettingsV1> }
    authorityPin?: {
      keyId: string
      publicKey: string
      runtimePid: number
      runtimeUrl: string
      generation: number
    }
    recordThreadTrace?: (event: ThreadTraceEventPayload) => void
  } = {}
) {
  const handlers = new Map<string, IpcHandler>()
  const logError = vi.fn()
  const ipcMain = {
    handle: vi.fn((channel: string, handler: IpcHandler) => {
      handlers.set(channel, handler)
    })
  } as unknown as IpcMain
  vi.stubGlobal('fetch', fetchImpl)
  const ensureRuntime = overrides.ensureRuntime ?? vi.fn(async () => undefined)
  const store = overrides.store ?? { load: vi.fn(async () => settingsForPort(9988)) }
  registerRuntimeSseIpc({
    ipcMain,
    store: store as never,
    ensureRuntime,
    logError,
    ...(overrides.recordThreadTrace ? { recordThreadTrace: overrides.recordThreadTrace } : {}),
    ...(overrides.authorityPin
      ? {
          resolveFinalPublicationAuthorityPin: () => overrides.authorityPin ?? null,
          isFinalPublicationAuthorityPinCurrent: (pin) => pin === overrides.authorityPin
        }
      : {})
  })
  return { handlers, logError, ensureRuntime, store }
}

function makeSender(isDestroyed = false, id = 1) {
  const send = vi.fn()
  const destroyed = vi.fn(() => isDestroyed)
  return {
    id,
    send,
    isDestroyed: destroyed
  } as unknown as WebContents & { send: typeof send; isDestroyed: typeof destroyed }
}

function makeEventEmitterSender(id: number) {
  const sender = new EventEmitter() as EventEmitter & {
    id: number
    send: ReturnType<typeof vi.fn>
    isDestroyed: ReturnType<typeof vi.fn>
  }
  sender.id = id
  sender.send = vi.fn()
  sender.isDestroyed = vi.fn(() => false)
  return sender as unknown as WebContents & typeof sender
}

function runtimeEvent(seq: number, _label: string, threadId: string): Record<string, unknown> {
  return {
    seq,
    kind: 'tool_progress',
    timestamp: `2026-07-14T00:00:${String(seq).padStart(2, '0')}.000Z`,
    threadId,
    turnId: `turn-${threadId}`,
    itemId: `item-${seq}`,
    toolName: 'test_tool',
    callId: `call-${seq}`,
    status: 'running'
  }
}

function eventFrame(seq: number, label: string, threadId: string): string {
  return [
    `id: ${seq}`,
    'event: tool_progress',
    `data: ${JSON.stringify(runtimeEvent(seq, label, threadId))}`,
    '',
    ''
  ].join('\n')
}

function generalTerminalDeliveryBatch(
  threadId: string,
  turnId: string,
  firstSeq: number,
  options: {
    includeTerminalItem?: boolean
    usageSource?: string
    childRunId?: string
  } = {}
): Record<string, unknown> {
  const timestamp = '2026-07-20T03:00:00Z'
  const slots = options.includeTerminalItem
    ? ['terminal-item', 'usage', 'terminal']
    : ['usage', 'terminal']
  const lastSeq = firstSeq + slots.length - 1
  const domainDigest = (domain: string, body: string): string => createHash('sha256')
    .update(Buffer.from(`${domain}\0`))
    .update(body, 'utf8')
    .digest('hex')
  const sha256 = (body: string): string => createHash('sha256').update(body).digest('hex')
  const generalTerminalCommitId = sha256(`general-terminal:${threadId}:${turnId}`)
  const events: Array<Record<string, unknown>> = []
  if (options.includeTerminalItem) {
    events.push({
      kind: 'item_completed', seq: firstSeq, timestamp, threadId, turnId,
      itemId: 'item-general-terminal',
      item: {
        id: 'item-general-terminal', turnId, threadId, role: 'assistant', status: 'completed',
        createdAt: timestamp, finishedAt: timestamp, kind: 'assistant_text',
        text: GENERAL_PROVIDER_FINAL_QUARANTINED_TEXT
      }
    })
  }
  events.push(
    {
      kind: 'usage', seq: firstSeq + events.length, timestamp, threadId, turnId, model: 'gpt-5',
      usage: {
        promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
        cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
        cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
        priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0, turns: 1
      },
      cacheDiagnostics: {},
      ...(options.usageSource ? { usageSource: options.usageSource } : {}),
      ...(options.childRunId ? { childRunId: options.childRunId } : {}),
      usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: lastSeq, timestamp, threadId, turnId,
      status: 'completed', terminalReason: 'success'
    }
  )
  const eventManifest = slots.map((slot, index) => ({
    slot,
    eventId: domainDigest('analytix/general-terminal-event/v1', `${generalTerminalCommitId}\0${slot}`),
    payloadDigest: sha256(`opaque-general-terminal-payload-${index}`)
  }))
  const manifestJSON = `[${eventManifest.map((entry) => `{${[
    `"slot":${goCanonicalJson(entry.slot)}`,
    `"eventId":${goCanonicalJson(entry.eventId)}`,
    `"payloadDigest":${goCanonicalJson(entry.payloadDigest)}`
  ].join(',')}}`).join(',')}]`
  const batch: Record<string, unknown> = {
    schemaVersion: 1,
    purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch',
    batchDigest: '',
    threadId,
    turnId,
    seq: lastSeq,
    firstSeq,
    lastSeq,
    timestamp,
    generalTerminalCommitId,
    generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: sha256('opaque-general-terminal-cas-authority'),
    eventManifestDigest: domainDigest('analytix/general-terminal-delivery-manifest/v1', manifestJSON),
    projectedEventsDigest: domainDigest(
      'analytix/general-terminal-projected-events/v1',
      goCanonicalJson(events)
    ),
    transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    events,
    eventManifest
  }
  const scalar = (key: string): string => `${goCanonicalJson(key)}:${goCanonicalJson(batch[key])}`
  const batchJSON = `{${[
    'schemaVersion', 'purpose', 'kind'
  ].map(scalar).concat([
    `${goCanonicalJson('batchDigest')}:${goCanonicalJson('')}`
  ], [
    'threadId', 'turnId', 'seq', 'firstSeq', 'lastSeq', 'timestamp',
    'generalTerminalCommitId', 'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest',
    'eventManifestDigest', 'projectedEventsDigest', 'transportAuthority', 'evidenceAuthority',
    'citationAuthority', 'factAnswerAllowed'
  ].map(scalar), [
    `${goCanonicalJson('events')}:${goCanonicalJson(events)}`,
    `${goCanonicalJson('eventManifest')}:${manifestJSON}`
  ]).join(',')}}`
  batch.batchDigest = domainDigest('analytix/general-terminal-delivery-batch/v1', batchJSON)
  return batch
}

function terminalErrorFrame(seq: number, payload: Record<string, unknown>): string {
  return [
    `id: ${seq}`,
    'event: error',
    `data: ${JSON.stringify({
      seq,
      kind: 'error',
      timestamp: '2026-07-14T00:00:00.000Z',
      terminal: true,
      ...payload
    })}`,
    '',
    ''
  ].join('\n')
}

function publicProjectionRevokedFrame(threadId: string): string {
  return [
    'event: public_projection_revoked',
    `data: ${JSON.stringify({
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId,
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected',
      action: 'purge_case_projection',
      terminal: true
    })}`,
    '',
    ''
  ].join('\n')
}

function payloadFrame(seq: number, event: string, payload: Record<string, unknown>): string {
  return [
    `id: ${seq}`,
    `event: ${event}`,
    `data: ${JSON.stringify({ seq, kind: event, timestamp: '2026-07-14T00:00:00.000Z', ...payload })}`,
    '',
    ''
  ].join('\n')
}

function strictFrame(id: number, event: string, data: Record<string, unknown>): string {
  return [
    `id: ${id}`,
    `event: ${event}`,
    `data: ${JSON.stringify(data)}`,
    '',
    ''
  ].join('\n')
}

function acceptedFinalPair() {
  const signatureDomain = Buffer.from('analytix.final-answer-authority/v1\0')
  const publicViewDomain = Buffer.from('analytix.accepted-final-public-view/v2\0')
  const authority = generateKeyPairSync('ed25519')
  const publicKeyDER = authority.publicKey.export({ format: 'der', type: 'spki' }) as Buffer
  const publicKey = publicKeyDER.subarray(publicKeyDER.length - 32)
  const sha256 = (value: string | Buffer) => createHash('sha256').update(value).digest('hex')
  const goJSONStringify = (value: unknown) => {
    const encoded = JSON.stringify(value)
    if (encoded === undefined) throw new Error('value is not JSON serializable')
    return encoded
      .replace(/</g, '\\u003c')
      .replace(/>/g, '\\u003e')
      .replace(/&/g, '\\u0026')
      .replace(/\u2028/g, '\\u2028')
      .replace(/\u2029/g, '\\u2029')
  }
  const acceptedAt = '2026-07-11T01:02:03Z'
  const publicView = {
    schemaVersion: 2,
    publicationState: 'accepted',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-case-3',
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: '8'.repeat(64),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: acceptedAt,
    acceptedAt
  }
  const publicViewDigest = sha256(Buffer.concat([
    publicViewDomain,
    Buffer.from(goJSONStringify(publicView))
  ]))
  const renderedTextSha256 = sha256('host boundary')
  const unsignedRecord = {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: sha256(publicKey),
    authorityPublicKey: publicKey.toString('base64url'),
    threadId: 'thread-case',
    turnId: 'turn-case',
    envelopeDigest: publicView.envelopeDigest,
    contextDigest: publicView.contextDigest,
    contextEpoch: publicView.contextEpoch,
    datasetSnapshotId: publicView.datasetSnapshotId,
    variant: publicView.variant,
    terminalReason: publicView.terminalReason,
    renderedTextSha256,
    registrySequence: 0,
    registryStateDigest: 'e'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest,
    privateRecordDigest: 'f'.repeat(64),
    acceptedAt,
    authoritySignature: '',
    recordDigest: ''
  }
  const signingDigest = createHash('sha256').update(goJSONStringify(unsignedRecord)).digest()
  const authoritySignature = sign(
    null,
    Buffer.concat([signatureDomain, signingDigest]),
    authority.privateKey
  ).toString('base64url')
  const signedRecord = { ...unsignedRecord, authoritySignature }
  const acceptedFinal = {
    ...signedRecord,
    recordDigest: sha256(goJSONStringify(signedRecord))
  }
  const { schemaVersion, publicationState, ...publicViewFields } = publicView
  const acceptedFinalView = {
    schemaVersion,
    publicationState,
    acceptedFinalDigest: acceptedFinal.recordDigest,
    publicViewDigest,
    ...publicViewFields
  }
  return {
    acceptedFinal,
    acceptedFinalView,
    privateKey: authority.privateKey,
    pin: {
      keyId: acceptedFinal.authorityKeyId,
      publicKey: acceptedFinal.authorityPublicKey,
      runtimePid: 1001,
      runtimeUrl: 'http://127.0.0.1:9988',
      generation: 1
    }
  }
}

function goCanonicalJson(value: unknown): string {
  if (value === null || typeof value !== 'object') {
    const encoded = JSON.stringify(value)
    if (encoded === undefined) throw new Error('value is not JSON serializable')
    return encoded
      .replace(/</g, '\\u003c')
      .replace(/>/g, '\\u003e')
      .replace(/&/g, '\\u0026')
      .replace(/\u2028/g, '\\u2028')
      .replace(/\u2029/g, '\\u2029')
  }
  if (Array.isArray(value)) return `[${value.map(goCanonicalJson).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record).sort().map((key) => `${goCanonicalJson(key)}:${goCanonicalJson(record[key])}`).join(',')}}`
}

function orderedDeliverySeal(value: Record<string, unknown>): Record<string, unknown> {
  return {
    schemaVersion: value.schemaVersion,
    purpose: value.purpose,
    sealId: value.sealId,
    threadId: value.threadId,
    turnId: value.turnId,
    publicationCommitId: value.publicationCommitId,
    acceptedFinalDispositionDigest: value.acceptedFinalDispositionDigest,
    terminalDispositionId: value.terminalDispositionId,
    eventManifestDigest: value.eventManifestDigest,
    sequencedEventsDigest: value.sequencedEventsDigest,
    batchId: value.batchId,
    firstSeq: value.firstSeq,
    lastSeq: value.lastSeq,
    timestamp: value.timestamp,
    authorityAlgorithm: value.authorityAlgorithm,
    authorityKeyId: value.authorityKeyId,
    authorityPublicKey: value.authorityPublicKey,
    authoritySignature: value.authoritySignature
  }
}

function acceptedFinalDeliveryBatch() {
  const fixture = acceptedFinalPair()
  const { acceptedFinal } = fixture
  const core = acceptedFinal.publicView
  const acceptedFinalView = {
    schemaVersion: 3,
    acceptedFinalDigest: acceptedFinal.recordDigest,
    publicationState: 'accepted',
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
  const item = {
    id: 'item-case-final',
    turnId: acceptedFinal.turnId,
    threadId: acceptedFinal.threadId,
    role: 'assistant',
    status: 'completed',
    createdAt: acceptedFinal.acceptedAt,
    finishedAt: acceptedFinal.acceptedAt,
    kind: 'assistant_text',
    text: 'host boundary',
    acceptedFinalView
  }
  const events: Array<Record<string, unknown>> = [
    {
      kind: 'item_completed', seq: 1, timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId, turnId: acceptedFinal.turnId, itemId: item.id, item
    },
    {
      kind: 'usage', seq: 2, timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId, turnId: acceptedFinal.turnId, model: '',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0, cacheHitRate: null, turns: 0 },
      cacheDiagnostics: {}, usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: 3, timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId, turnId: acceptedFinal.turnId,
      status: 'completed', terminalReason: acceptedFinal.terminalReason
    }
  ]
  const slots = ['assistant-final', 'usage', 'terminal']
  events.forEach((event, index) => {
    const slot = slots[index]
    event.acceptedFinalDigest = acceptedFinal.recordDigest
    event.publicationCommitId = acceptedFinal.recordDigest
    event.publicationEventId = acceptedFinalPublicationEventId(acceptedFinal.recordDigest, slot)
    event.publicationSlot = slot
    event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event)
  })
  const manifest = events.map((event) => ({
    slot: event.publicationSlot,
    eventId: event.publicationEventId,
    payloadDigest: event.publicationPayloadDigest
  }))
  const eventManifestDigest = createHash('sha256').update(goCanonicalJson(manifest)).digest('hex')
  const batchCore = {
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    threadId: acceptedFinal.threadId,
    turnId: acceptedFinal.turnId,
    firstSeq: 1,
    lastSeq: 3,
    publicationCommitId: acceptedFinal.recordDigest,
    eventManifestDigest
  }
  const batchId = createHash('sha256').update(JSON.stringify(batchCore)).digest('hex')
  const sealBase: Record<string, unknown> = {
    schemaVersion: 'accepted-final-delivery-seal.v1',
    purpose: 'analytix.accepted-final-delivery-seal/v1',
    sealId: '',
    threadId: acceptedFinal.threadId,
    turnId: acceptedFinal.turnId,
    publicationCommitId: acceptedFinal.recordDigest,
    acceptedFinalDispositionDigest: '4'.repeat(64),
    terminalDispositionId: '5'.repeat(64),
    eventManifestDigest,
    sequencedEventsDigest: createHash('sha256').update(goCanonicalJson(events)).digest('hex'),
    batchId,
    firstSeq: 1,
    lastSeq: 3,
    timestamp: acceptedFinal.acceptedAt,
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: fixture.pin.keyId,
    authorityPublicKey: fixture.pin.publicKey,
    authoritySignature: ''
  }
  sealBase.sealId = createHash('sha256').update(Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-id/v1\0'),
    Buffer.from(JSON.stringify(orderedDeliverySeal(sealBase)))
  ])).digest('hex')
  const signingDigest = createHash('sha256').update(JSON.stringify(orderedDeliverySeal(sealBase))).digest()
  sealBase.authoritySignature = sign(null, Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-signature/v1\0'),
    signingDigest
  ]), fixture.privateKey).toString('base64url')
  const batch = {
    schemaVersion: 2,
    purpose: batchCore.purpose,
    kind: 'accepted_final_batch',
    batchId,
    threadId: batchCore.threadId,
    turnId: batchCore.turnId,
    seq: 3,
    firstSeq: 1,
    lastSeq: 3,
    timestamp: acceptedFinal.acceptedAt,
    publicationCommitId: acceptedFinal.recordDigest,
    eventManifestDigest,
    publicationAuthority: orderedDeliverySeal(sealBase),
    events
  }
  return {
    batch,
    pin: fixture.pin,
    privateKey: fixture.privateKey,
    privateRecord: acceptedFinal,
    privateView: fixture.acceptedFinalView
  }
}

function resignAcceptedFinalDeliveryBatch(
  batch: Record<string, any>,
  privateKey: KeyObject
): void {
  const events = batch.events as Array<Record<string, unknown>>
  for (const event of events) {
    event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event)
  }
  const manifest = events.map((event) => ({
    slot: event.publicationSlot,
    eventId: event.publicationEventId,
    payloadDigest: event.publicationPayloadDigest
  }))
  batch.eventManifestDigest = createHash('sha256').update(goCanonicalJson(manifest)).digest('hex')
  batch.batchId = createHash('sha256').update(JSON.stringify({
    purpose: batch.purpose,
    threadId: batch.threadId,
    turnId: batch.turnId,
    firstSeq: batch.firstSeq,
    lastSeq: batch.lastSeq,
    publicationCommitId: batch.publicationCommitId,
    eventManifestDigest: batch.eventManifestDigest
  })).digest('hex')
  const seal = {
    ...batch.publicationAuthority,
    sealId: '',
    eventManifestDigest: batch.eventManifestDigest,
    sequencedEventsDigest: createHash('sha256').update(goCanonicalJson(events)).digest('hex'),
    batchId: batch.batchId,
    authoritySignature: ''
  }
  seal.sealId = createHash('sha256').update(Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-id/v1\0'),
    Buffer.from(JSON.stringify(orderedDeliverySeal(seal)))
  ])).digest('hex')
  const signingDigest = createHash('sha256').update(JSON.stringify(orderedDeliverySeal(seal))).digest()
  seal.authoritySignature = sign(null, Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-signature/v1\0'),
    signingDigest
  ]), privateKey).toString('base64url')
  batch.publicationAuthority = orderedDeliverySeal(seal)
}

function streamFromFrames(frames: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const frame of frames) {
        controller.enqueue(encoder.encode(frame))
      }
      controller.close()
    }
  })
}

function controlledStream(frames: string[]) {
  const encoder = new TextEncoder()
  let streamController: ReadableStreamDefaultController<Uint8Array> | undefined
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      streamController = controller
      for (const frame of frames) {
        controller.enqueue(encoder.encode(frame))
      }
    }
  })
  return {
    stream,
    close() {
      try {
        streamController?.close()
      } catch {
        // A rejected SSE frame may already have cancelled the reader.
      }
    }
  }
}

function responseFor(frames: string[]): Response {
  return new Response(streamFromFrames(frames), {
    status: 200,
    headers: { 'content-type': 'text/event-stream' }
  })
}

async function flushAsync(rounds = 20): Promise<void> {
  for (let i = 0; i < rounds; i += 1) {
    await Promise.resolve()
  }
}

async function waitForCondition(
  predicate: () => boolean,
  message: string | (() => string) = 'condition was not met'
): Promise<void> {
  for (let i = 0; i < 100; i += 1) {
    if (predicate()) return
    await new Promise<void>((resolve) => setTimeout(resolve, 0))
  }
  expect(predicate(), typeof message === 'function' ? message() : message).toBe(true)
}

function sseEventCalls(sender: { send: ReturnType<typeof vi.fn> }) {
  return sender.send.mock.calls.filter(([channel]) => channel === 'runtime:sse-event')
}

function requestHeader(headers: RequestInit['headers'] | undefined, name: string): string | undefined {
  if (!headers) return undefined
  if (headers instanceof Headers) {
    return headers.get(name) ?? undefined
  }
  const record = headers as Record<string, string>
  return record[name] ?? record[name.toLowerCase()] ?? record[name.toUpperCase()]
}

beforeEach(() => {
  vi.restoreAllMocks()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('registerRuntimeSseIpc', () => {
  it('generates stream ids and starts on the analytix-owned SSE route', async () => {
    const seenUrls: URL[] = []
    const seenLastEventId: Array<string | undefined> = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      seenUrls.push(url)
      seenLastEventId.push(requestHeader(init?.headers, 'Last-Event-ID'))
      expect(requestHeader(init?.headers, 'Authorization')).toBe('Bearer runtime-token')
      return new Response('missing', { status: 404 })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    const result = await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-generated', sinceSeq: 5 })
    const streamId = (result as { streamId: string }).streamId

    await waitForCondition(() => fetchMock.mock.calls.length === 1)
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')
    )

    expect(streamId).toEqual(expect.any(String))
    expect(streamId.length).toBeGreaterThan(0)
    expect(seenUrls[0].pathname).toBe('/v1/threads/thread-generated/events')
    expect(seenUrls[0].pathname).not.toMatch(FORBIDDEN_PUBLIC_SSE_ROUTE)
    expect(seenUrls[0].searchParams.get('since_seq')).toBe('5')
    expect(seenUrls[0].searchParams.get('live')).toBe('1')
    expect(seenLastEventId).toEqual(['5'])
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId,
      status: 404
    })
  })

  it('returns a stream id and retries transient runtime ensure failures before opening SSE', async () => {
    vi.useFakeTimers()
    const controlled = controlledStream([eventFrame(1, 'ready', 'thread-retry')])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const ensureRuntime = vi.fn()
      .mockRejectedValueOnce(new Error(JSON.stringify({
        code: 'fetch_failed',
        message: 'The operation was aborted due to timeout'
      })))
      .mockResolvedValueOnce(settingsForPort(9988))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { ensureRuntime })
    const sender = makeSender()

    const result = await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-retry', sinceSeq: 0, streamId: 'stream-retry' })

    expect(result).toEqual({ streamId: 'stream-retry' })
    await flushAsync()
    expect(ensureRuntime).toHaveBeenCalledTimes(1)
    expect(fetchMock).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(750)
    await flushAsync()

    expect(ensureRuntime).toHaveBeenCalledTimes(2)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-retry', events: [runtimeEvent(1, 'ready', 'thread-retry')] }
    ])
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-retry')
    controlled.close()
    await flushAsync()
  })

  it('URL-encodes SSE thread ids before fetching the analytix runtime host', async () => {
    const seenUrls: URL[] = []
    const seenLastEventId: Array<string | undefined> = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      seenUrls.push(url)
      seenLastEventId.push(requestHeader(init?.headers, 'Last-Event-ID'))
      expect(requestHeader(init?.headers, 'Authorization')).toBe('Bearer runtime-token')
      expect(requestHeader(init?.headers, 'Accept')).toBe('text/event-stream')
      return new Response('missing', { status: 404 })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()
    const threadId = 'thr/with space?x=1#frag'

    const result = await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId, sinceSeq: 9, streamId: 'stream-encoded' })

    await waitForCondition(() => fetchMock.mock.calls.length === 1)
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')
    )

    expect(result).toEqual({ streamId: 'stream-encoded' })
    expect(seenUrls[0].pathname).toBe('/v1/threads/thr%2Fwith%20space%3Fx%3D1%23frag/events')
    expect(seenUrls[0].pathname).not.toMatch(FORBIDDEN_PUBLIC_SSE_ROUTE)
    expect(seenUrls[0].searchParams.get('since_seq')).toBe('9')
    expect(seenUrls[0].searchParams.get('live')).toBe('1')
    expect(seenLastEventId).toEqual(['9'])
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId: 'stream-encoded',
      status: 404
    })
  })

  it('stops only the matching SSE stream id while a start request is pending', async () => {
    let fetchSignal: AbortSignal | undefined
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      fetchSignal = init?.signal as AbortSignal | undefined
      return await new Promise<Response>((_resolve, reject) => {
        fetchSignal?.addEventListener('abort', () => {
          reject(new Error('aborted'))
        }, { once: true })
      })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    const result = await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-stop', sinceSeq: 0, streamId: 'stream-a' })
    await waitForCondition(() => fetchSignal !== undefined)

    expect(result).toEqual({ streamId: 'stream-a' })
    await handlers.get('runtime:sse:stop')?.({ sender }, 'wrong-stream')
    await flushAsync()
    expect(fetchSignal?.aborted).toBe(false)

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-a')
    await waitForCondition(() => fetchSignal?.aborted === true)
  })

  it('binds start, ack, and stop to one WebContents owner and caps ACK at the delivered cursor', async () => {
    const controlled = controlledStream([eventFrame(1, 'one', 'thread-owner')])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const owner = makeSender(false, 101)
    const other = makeSender(false, 101)

    await handlers.get('runtime:sse:start')?.({ sender: owner }, {
      threadId: 'thread-owner',
      sinceSeq: 0,
      streamId: 'stream-owner'
    })
    await waitForCondition(() => sseEventCalls(owner).length === 1)

    await expect(handlers.get('runtime:sse:ack')?.({ sender: other }, {
      streamId: 'stream-owner',
      seq: 1
    })).resolves.toBe(false)
    await expect(handlers.get('runtime:sse:ack')?.({ sender: owner }, {
      streamId: 'stream-owner',
      seq: 2
    })).resolves.toBe(false)
    await expect(handlers.get('runtime:sse:stop')?.({ sender: other }, 'stream-owner')).resolves.toBe(false)
    await expect(handlers.get('runtime:sse:start')?.({ sender: other }, {
      threadId: 'thread-owner',
      sinceSeq: 0,
      streamId: 'stream-owner'
    })).rejects.toThrow('sse_stream_owner_mismatch')
    await expect(handlers.get('runtime:sse:ack')?.({ sender: owner }, {
      streamId: 'stream-owner',
      seq: 1
    })).resolves.toBe(true)
    await expect(handlers.get('runtime:sse:stop')?.({ sender: owner }, 'stream-owner')).resolves.toBe(true)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    controlled.close()
    await flushAsync()
  })

  it('cancels an active reader and releases stream ownership when WebContents is destroyed', async () => {
    const encoder = new TextEncoder()
    const cancelReader = vi.fn()
    const liveStream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(eventFrame(1, 'live', 'thread-owner-destroyed')))
      },
      cancel() {
        cancelReader()
      }
    })
    const fetchMock = vi.fn(async () => {
      if (fetchMock.mock.calls.length === 1) {
        return new Response(liveStream, {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return new Response('gone', { status: 404 })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const owner = makeEventEmitterSender(401)

    await handlers.get('runtime:sse:start')?.({ sender: owner }, {
      threadId: 'thread-owner-destroyed',
      sinceSeq: 0,
      streamId: 'stream-owner-destroyed'
    })
    await waitForCondition(() => sseEventCalls(owner).length === 1)
    owner.emit('destroyed')
    await waitForCondition(() => cancelReader.mock.calls.length === 1)

    const replacement = makeEventEmitterSender(402)
    await expect(handlers.get('runtime:sse:start')?.({ sender: replacement }, {
      threadId: 'thread-owner-destroyed',
      sinceSeq: 0,
      streamId: 'stream-owner-destroyed'
    })).resolves.toEqual({ streamId: 'stream-owner-destroyed' })
    await handlers.get('runtime:sse:stop')?.({ sender: replacement }, 'stream-owner-destroyed')
  })

  it('rejects header-kind, id-seq, thread, and event-schema mismatches without emitting or reconnecting', async () => {
    const candidates = [
      strictFrame(1, 'heartbeat', {
        kind: 'tool_progress', seq: 1, timestamp: '2026-07-14T00:00:00.000Z', threadId: 'thread-strict'
      }),
      strictFrame(2, 'heartbeat', {
        kind: 'heartbeat', seq: 3, timestamp: '2026-07-14T00:00:00.000Z', threadId: 'thread-strict'
      }),
      strictFrame(3, 'heartbeat', {
        kind: 'heartbeat', seq: 3, timestamp: '2026-07-14T00:00:00.000Z', threadId: 'other-thread'
      }),
      strictFrame(4, 'unknown_private_event', {
        kind: 'unknown_private_event', seq: 4, timestamp: '2026-07-14T00:00:00.000Z',
        threadId: 'thread-strict', privatePayload: 'must-not-cross-ipc'
      })
    ]
    const fetchMock = vi.fn(async () => responseFor([candidates[fetchMock.mock.calls.length - 1] ?? '']))
    const { handlers } = registerHarness(fetchMock as typeof fetch)

    for (let index = 0; index < candidates.length; index += 1) {
      const sender = makeSender(false, 300 + index)
      await handlers.get('runtime:sse:start')?.({ sender }, {
        threadId: 'thread-strict',
        sinceSeq: 0,
        streamId: `stream-strict-${index}`
      })
      await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
      expect(sseEventCalls(sender)).toHaveLength(0)
      expect(JSON.stringify(sender.send.mock.calls)).not.toContain('must-not-cross-ipc')
    }

    expect(fetchMock).toHaveBeenCalledTimes(candidates.length)
  })

  it('reconnects with the highest renderer-acked since_seq after stream disconnects', async () => {
    const seenSinceSeq: string[] = []
    const seenLastEventId: Array<string | undefined> = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      seenSinceSeq.push(url.searchParams.get('since_seq') ?? '')
      seenLastEventId.push(requestHeader(init?.headers, 'Last-Event-ID'))
      expect(requestHeader(init?.headers, 'Authorization')).toBe('Bearer runtime-token')
      if (fetchMock.mock.calls.length === 1) {
        return responseFor([
          eventFrame(1, 'one', 'thread-a'),
          eventFrame(2, 'two', 'thread-a')
        ])
      }
      if (fetchMock.mock.calls.length === 2) {
        return responseFor([eventFrame(3, 'three', 'thread-a')])
      }
      return new Response('gone', { status: 404 })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-a', sinceSeq: 0, streamId: 'stream-a' })

    await waitForCondition(() => sseEventCalls(sender).length === 2)
    await handlers.get('runtime:sse:ack')?.({ sender }, { streamId: 'stream-a', seq: 2 })
    await waitForCondition(() => sseEventCalls(sender).length === 3)
    await handlers.get('runtime:sse:ack')?.({ sender }, { streamId: 'stream-a', seq: 3 })
    await waitForCondition(
      () => fetchMock.mock.calls.length === 3,
      () => `fetch calls: ${fetchMock.mock.calls.length}; since_seq: ${JSON.stringify(seenSinceSeq)}; sends: ${JSON.stringify(sender.send.mock.calls)}`
    )
    await waitForCondition(() => sseEventCalls(sender).length === 3)

    expect(seenSinceSeq).toEqual(['0', '2', '3'])
    expect(seenLastEventId).toEqual([undefined, '2', '3'])
    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-a', events: [runtimeEvent(1, 'one', 'thread-a')] },
      { streamId: 'stream-a', events: [runtimeEvent(2, 'two', 'thread-a')] },
      { streamId: 'stream-a', events: [runtimeEvent(3, 'three', 'thread-a')] }
    ])
  })

  it('reconnects from the previous acked cursor when a flushed batch is not acked', async () => {
    const seenSinceSeq: string[] = []
    const seenLastEventId: Array<string | undefined> = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      seenSinceSeq.push(url.searchParams.get('since_seq') ?? '')
      seenLastEventId.push(requestHeader(init?.headers, 'Last-Event-ID'))
      if (fetchMock.mock.calls.length === 1) {
        return responseFor([eventFrame(1, 'one', 'thread-unacked')])
      }
      return new Response('gone', { status: 404 })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-unacked', sinceSeq: 0, streamId: 'stream-unacked' })

    await waitForCondition(() => sseEventCalls(sender).length === 1)
    await new Promise<void>((resolve) => setTimeout(resolve, 600))
    await waitForCondition(() => fetchMock.mock.calls.length === 2)

    expect(seenSinceSeq).toEqual(['0', '0'])
    expect(seenLastEventId).toEqual([undefined, undefined])
    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-unacked', events: [runtimeEvent(1, 'one', 'thread-unacked')] }
    ])
  })

  it('does not reconnect after a terminal runtime error event closes the stream', async () => {
    const seenSinceSeq: string[] = []
    const seenLastEventId: Array<string | undefined> = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      seenSinceSeq.push(url.searchParams.get('since_seq') ?? '')
      seenLastEventId.push(requestHeader(init?.headers, 'Last-Event-ID'))
      return responseFor([terminalErrorFrame(5, {
        threadId: 'thread-terminal',
        code: 'sse_setup_error',
        message: 'SSE setup failed',
        severity: 'error',
        details: { phase: 'highest_seq' }
      })])
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-terminal', sinceSeq: 5, streamId: 'stream-terminal' })

    await waitForCondition(() => sseEventCalls(sender).length === 1)
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end')
    )

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(seenSinceSeq).toEqual(['5'])
    expect(seenLastEventId).toEqual(['5'])
    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      {
        streamId: 'stream-terminal',
        events: [{
          seq: 5,
          kind: 'error',
          timestamp: '2026-07-14T00:00:00.000Z',
          terminal: true,
          threadId: 'thread-terminal',
          code: 'sse_setup_error',
          message: 'Runtime request failed (sse_setup_error).',
          severity: 'error',
          details: { phase: 'highest_seq' }
        }]
      }
    ])
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-end', { streamId: 'stream-terminal' })
  })

  it('flushes the first runtime event immediately and batches following events per frame', async () => {
    vi.useFakeTimers()
    const controlled = controlledStream([
      eventFrame(1, 'one', 'thread-b'),
      eventFrame(2, 'two', 'thread-b')
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-b', sinceSeq: 0, streamId: 'stream-b' })
    await flushAsync()

    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-b', events: [runtimeEvent(1, 'one', 'thread-b')] }
    ])
    await vi.advanceTimersByTimeAsync(15)
    expect(sseEventCalls(sender)).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)

    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-b', events: [runtimeEvent(1, 'one', 'thread-b')] },
      { streamId: 'stream-b', events: [runtimeEvent(2, 'two', 'thread-b')] }
    ])

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-b')
    controlled.close()
    await flushAsync()
  })

  it('discards a pending durable batch when public projection authority is revoked', async () => {
    const controlled = controlledStream([
      eventFrame(1, 'one', 'thread-revoked'),
      eventFrame(2, 'must-be-discarded', 'thread-revoked'),
      publicProjectionRevokedFrame('thread-revoked')
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: 'thread-revoked',
      sinceSeq: 0,
      streamId: 'stream-revoked'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end'))

    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-revoked', events: [runtimeEvent(1, 'one', 'thread-revoked')] },
      {
        streamId: 'stream-revoked',
        events: [{
          schemaVersion: 1,
          kind: 'public_projection_revoked',
          threadId: 'thread-revoked',
          historyAuthority: 'case_boundary_only_v1',
          code: 'case_public_authority_rejected',
          action: 'purge_case_projection',
          terminal: true
        }]
      }
    ])
    expect(JSON.stringify(sseEventCalls(sender))).not.toContain('must-be-discarded')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('fails closed on private reasoning without advancing or exposing the rejected event', async () => {
    const controlled = controlledStream([
      payloadFrame(1, 'assistant_reasoning_delta', {
        threadId: 'thread-private-reasoning',
        text: 'PRIVATE_REASONING_SENTINEL'
      }),
      payloadFrame(2, 'item_completed', {
        threadId: 'thread-private-reasoning',
        turnId: 'turn-private-reasoning',
        itemId: 'item-private-reasoning',
        item: {
          id: 'item-private-reasoning',
          threadId: 'thread-private-reasoning',
          turnId: 'turn-private-reasoning',
          kind: 'assistant_reasoning',
          text: 'ARCHIVED_REASONING_SENTINEL'
        }
      }),
      payloadFrame(3, 'assistant_text_delta', {
        threadId: 'thread-private-reasoning',
        turnId: 'turn-private-reasoning',
        itemId: 'item-public',
        item: {
          id: 'item-public',
          threadId: 'thread-private-reasoning',
          turnId: 'turn-private-reasoning',
          role: 'assistant',
          status: 'running',
          createdAt: '2026-07-14T00:00:00.000Z',
          kind: 'assistant_text',
          text: 'public answer'
        }
      })
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: 'thread-private-reasoning',
      sinceSeq: 0,
      streamId: 'stream-private-reasoning'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))

    const serialized = JSON.stringify(sseEventCalls(sender))
    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(serialized).not.toContain('PRIVATE_REASONING_SENTINEL')
    expect(serialized).not.toContain('ARCHIVED_REASONING_SENTINEL')
    expect(serialized).not.toContain('assistant_reasoning')
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId: 'stream-private-reasoning',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'invalid_public_projection'
    })
  })

  it.each([
    ['message', { message: 'MARKER_FREE_PROVIDER_ERROR_7F3C' }],
    ['summary', { summary: 'MARKER_FREE_MCP_RESPONSE_9A2D' }],
    ['reason', { reason: 'MARKER_FREE_JOB_FAILURE_4C8E' }],
    ['label', { label: 'MARKER_FREE_PROVIDER_LABEL_2B6A' }],
    ['details.message', { details: { message: 'MARKER_FREE_MCP_RESPONSE_9A2D' } }],
    ['unmarked provider error', { details: { providerError: { status: 503, retryable: true } } }],
    ['path', { summary: '/Users/sun/private/case.csv' }],
    ['case fact', { message: '账户 6222020202020202020 收款 2645472 元' }]
  ])('rejects an open %s carrier before main sends SSE IPC', async (label, extra) => {
    const suffix = label.replace(/[^a-z]+/gi, '-').toLowerCase()
    const threadId = `thread-open-${suffix}`
    const streamId = `stream-open-${suffix}`
    const fetchMock = vi.fn(async () => responseFor([
      payloadFrame(1, 'pipeline_stage', {
        threadId,
        turnId: `turn-open-${suffix}`,
        stage: 'response_received',
        label: 'Response Received',
        ...extra
      })
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId,
      sinceSeq: 0,
      streamId
    })
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')
    )

    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId,
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'invalid_public_projection'
    })
  })

  it('delivers closed Provider failure-stage attribution across main SSE IPC', async () => {
    const event = {
      threadId: 'thread-provider-stage',
      turnId: 'turn-provider-stage',
      stage: 'provider_error',
      label: 'Provider stream failed',
      details: {
        reasonCode: 'provider_error',
        providerError: {
          failureStage: 'telemetry_begin',
          dispatchState: 'not_sent',
          attempt: 1
        }
      }
    }
    const fetchMock = vi.fn(async () => responseFor([
      payloadFrame(1, 'pipeline_stage', event)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: event.threadId,
      sinceSeq: 0,
      streamId: 'stream-provider-stage'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([{
      streamId: 'stream-provider-stage',
      events: [{
        seq: 1,
        kind: 'pipeline_stage',
        timestamp: '2026-07-14T00:00:00.000Z',
        ...event
      }]
    }])
  })

  it('delivers a canonical create_plan status across the Electron SSE boundary', async () => {
    const planResult = {
      threadId: 'thread-plan-status',
      turnId: 'turn-plan-status',
      itemId: 'tool-result-plan-status',
      item: {
        id: 'tool-result-plan-status',
        turnId: 'turn-plan-status',
        threadId: 'thread-plan-status',
        role: 'tool',
        status: 'completed',
        createdAt: '2026-08-01T00:00:00.000Z',
        finishedAt: '2026-08-01T00:00:01.000Z',
        kind: 'tool_result',
        toolName: 'create_plan',
        callId: 'host-call-plan-status',
        toolKind: 'file_change',
        isError: false,
        output: {
          schemaVersion: 1,
          projectionKind: 'plan_status',
          disclosure: 'metadata_only',
          status: 'completed',
          code: 'plan_updated',
          messageKey: 'plan_updated',
          privatePayloadWithheld: true,
          factAnswerAllowed: false,
          evidenceAuthority: false,
          plan: {
            planId: 'plan-status',
            relativePath: '.analytixsdd/plan/status.md',
            operation: 'draft',
            contentHash: 'a'.repeat(64),
            byteSize: 128,
            savedAt: '2026-08-01T00:00:01.000Z'
          }
        }
      }
    }
    const controlled = controlledStream([
      payloadFrame(47, 'tool_call_finished', planResult)
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: 'thread-plan-status',
      sinceSeq: 46,
      streamId: 'stream-plan-status'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([{
      streamId: 'stream-plan-status',
      events: [{
        seq: 47,
        kind: 'tool_call_finished',
        timestamp: '2026-07-14T00:00:00.000Z',
        ...planResult
      }]
    }])
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-plan-status')
    controlled.close()
    await flushAsync()
  })

  it('rejects a hostile tool-result payload before SSE crosses Electron IPC', async () => {
    const sentinel = 'PRIVATE_SSE_TOOL_RESULT_SENTINEL'
    const controlled = controlledStream([
      payloadFrame(1, 'item_completed', {
        threadId: 'thread-tool-result',
        turnId: 'turn-tool-result',
        itemId: 'item-tool-result',
        item: {
          id: 'item-tool-result',
          turnId: 'turn-tool-result',
          threadId: 'thread-tool-result',
          role: 'tool',
          status: 'completed',
          createdAt: '2026-07-14T00:00:00.000Z',
          finishedAt: '2026-07-14T00:00:01.000Z',
          kind: 'tool_result',
          toolName: 'mcp__hostile__query',
          callId: 'call-hostile',
          toolKind: 'tool_call',
          isError: false,
          summary: sentinel,
          dataUrl: `data:image/png;base64,${sentinel}`,
          output: {
            code: '6222020202020202020',
            content: sentinel,
            citations: [{ sourceId: sentinel }],
            attachments: [{ id: 'att-hostile', localFilePath: `/tmp/${sentinel}` }],
            previewUrl: `http://127.0.0.1:4173/${sentinel}`
          }
        }
      })
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: 'thread-tool-result',
      sinceSeq: 0,
      streamId: 'stream-tool-result'
    })
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')
    )

    const serialized = JSON.stringify(sseEventCalls(sender))
    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(serialized).not.toContain(sentinel)
    expect(serialized).not.toContain('6222020202020202020')
    expect(serialized).not.toContain('data:image')
    expect(serialized).not.toContain('127.0.0.1:4173')
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId: 'stream-tool-result',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'invalid_public_projection'
    })

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-tool-result')
    controlled.close()
    await flushAsync()
  })

  it('rejects a standalone signed v5 accepted-final event as an atomic-prefix violation', async () => {
    const { acceptedFinal, acceptedFinalView } = acceptedFinalPair()
    const item = {
      id: 'item-case-final',
      turnId: acceptedFinal.turnId,
      threadId: acceptedFinal.threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: acceptedFinal.acceptedAt,
      finishedAt: acceptedFinal.acceptedAt,
      kind: 'assistant_text',
      text: 'host boundary',
      acceptedFinal,
      acceptedFinalView
    }
    const event: Record<string, unknown> = {
      kind: 'item_completed',
      seq: 1,
      timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      itemId: item.id,
      item,
      acceptedFinalDigest: acceptedFinal.recordDigest,
      publicationCommitId: acceptedFinal.recordDigest,
      publicationEventId: acceptedFinalPublicationEventId(acceptedFinal.recordDigest, 'assistant-final'),
      publicationSlot: 'assistant-final'
    }
    event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event)
    const controlled = controlledStream([
      strictFrame(1, 'item_completed', event)
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: acceptedFinal.threadId,
      sinceSeq: 0,
      streamId: 'stream-v5-accepted'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-v5-accepted')
    controlled.close()
    await flushAsync()
  })

  it.each([false, true])('delivers one pinned sealed accepted-final batch and only acknowledges its last sequence (trace throws: %s)', async (traceThrows) => {
    const { batch, pin } = acceptedFinalDeliveryBatch()
    const recordThreadTrace = vi.fn((_event: ThreadTraceEventPayload) => {
      if (traceThrows) throw new Error('synthetic diagnostic failure')
    })
    const controlled = controlledStream([
      strictFrame(batch.lastSeq, 'accepted_final_batch', batch)
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: pin, recordThreadTrace })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-sealed-accepted'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    expect(sseEventCalls(sender)[0]?.[1]).toEqual({
      streamId: 'stream-sealed-accepted',
      events: [batch]
    })
    expect(recordThreadTrace.mock.calls.map(([event]) => event.name)).toEqual([
      'thread.terminal.verified', 'thread.terminal.ipc_sent'
    ])
    expect(recordThreadTrace.mock.calls[0]?.[0]?.data).toMatchObject({
      lastSeq: batch.lastSeq, acceptedFinal: true
    })
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-sealed-accepted', seq: batch.firstSeq
    })).toBe(false)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-sealed-accepted', seq: batch.lastSeq
    })).toBe(false)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-sealed-accepted',
      seq: batch.lastSeq,
      batchId: batch.batchId,
      threadId: batch.threadId,
      turnId: batch.turnId,
      publicationCommitId: '0'.repeat(64)
    })).toBe(false)
    await new Promise((resolve) => setTimeout(resolve, 550))
    expect(sender.send).not.toHaveBeenCalledWith('runtime:sse-end', expect.anything())
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-sealed-accepted',
      seq: batch.lastSeq,
      batchId: batch.batchId,
      threadId: batch.threadId,
      turnId: batch.turnId,
      publicationCommitId: batch.publicationCommitId
    })).toBe(true)

    controlled.close()
    await flushAsync()
  })

  it('rejects the frozen V1 audit batch from the current generic SSE consumer', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const historicalV1 = {
      ...structuredClone(fixture.batch),
      schemaVersion: 1,
      purpose: 'analytix.accepted-final-delivery-batch/v1'
    }
    const historicalAssistant = historicalV1.events[0].item as Record<string, unknown>
    historicalAssistant.acceptedFinal = fixture.privateRecord
    historicalAssistant.acceptedFinalView = fixture.privateView
    expect(AcceptedFinalDeliveryBatchV1Schema.safeParse(historicalV1).success).toBe(true)
    const historicalBefore = JSON.stringify(historicalV1)
    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(historicalV1.lastSeq, 'accepted_final_batch', historicalV1),
      strictFrame(fixture.batch.lastSeq, 'accepted_final_batch', fixture.batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: fixture.pin })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: historicalV1.threadId,
      sinceSeq: 0,
      streamId: 'stream-historical-v1-rejected'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(JSON.stringify(historicalV1)).toBe(historicalBefore)
    expect(historicalV1).toMatchObject({
      schemaVersion: 1,
      purpose: 'analytix.accepted-final-delivery-batch/v1'
    })
    expect(fixture.batch).toMatchObject({
      schemaVersion: 2,
      purpose: 'analytix.accepted-final-delivery-batch/v2'
    })
  })

  it('rejects a V1 wrapper carrying a V3 assistant event without advancing the generic SSE cursor', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const v1WrapperWithV3Event = {
      ...structuredClone(fixture.batch),
      schemaVersion: 1,
      purpose: 'analytix.accepted-final-delivery-batch/v1'
    }
    const assistant = v1WrapperWithV3Event.events[0].item as Record<string, any>
    expect(assistant.acceptedFinal).toBeUndefined()
    expect(assistant.acceptedFinalView.schemaVersion).toBe(3)
    expect(AcceptedFinalDeliveryBatchV1Schema.safeParse(v1WrapperWithV3Event).success).toBe(false)

    const before = JSON.stringify(v1WrapperWithV3Event)
    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(v1WrapperWithV3Event.lastSeq, 'accepted_final_batch', v1WrapperWithV3Event),
      strictFrame(fixture.batch.lastSeq, 'accepted_final_batch', fixture.batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: fixture.pin })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: v1WrapperWithV3Event.threadId,
      sinceSeq: 0,
      streamId: 'stream-v1-wrapper-v3-event-rejected'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(JSON.stringify(v1WrapperWithV3Event)).toBe(before)
  })

  it.each(['separate', 'coalesced'])('delivers a sealed V3 generic accepted-final batch without a private V5 record (%s)', async (chunking) => {
    const { batch, pin, privateRecord } = acceptedFinalDeliveryBatch()
    const frames = [
      strictFrame(batch.lastSeq, 'accepted_final_batch', batch),
      eventFrame(batch.lastSeq + 1, 'later-history', batch.threadId),
      eventFrame(batch.lastSeq + 2, 'later-history', batch.threadId)
    ]
    const controlled = controlledStream(chunking === 'coalesced' ? [frames.join('')] : frames)
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: pin })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-v3-generic-accepted'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    const delivered = sseEventCalls(sender)[0]?.[1]
    expect(delivered).toEqual({ streamId: 'stream-v3-generic-accepted', events: [batch] })
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    const body = JSON.stringify(delivered)
    const acceptedFinalView = (batch.events[0].item as Record<string, any>).acceptedFinalView as Record<string, any>
    const publicSetDigest = acceptedFinalView.receiptMetadata.setDigest as string
    expect(Object.keys(acceptedFinalView).sort()).toEqual([
      'schemaVersion', 'acceptedFinalDigest', 'publicationState', 'variant', 'terminalReason',
      'blockerCode', 'coverageStatus', 'checkedScopeDigest', 'missingScopeCount', 'claimCount',
      'claimTypes', 'receiptMetadata', 'noHitWording', 'acceptedAt'
    ].sort())
    expect(Object.keys(acceptedFinalView.receiptMetadata).sort()).toEqual([
      'projection', 'count', 'setDigest', 'citations'
    ].sort())
    const viewBody = JSON.stringify(acceptedFinalView)
    for (const forbiddenProperty of [
      'publicViewDigest', 'envelopeDigest', 'contextDigest', 'contextEpoch', 'datasetSnapshotId',
      'envelopeIssuedAt', 'threadId', 'turnId', 'renderedTextSha256', 'acceptedFinal',
      'factFinalWitnessAdmission', 'publicationSnapshotProof', 'publicationSnapshotProofDigest',
      'securityContext', 'publicationIntent', 'storeDigest', 'registryHead'
    ]) {
      expect(viewBody).not.toContain(`"${forbiddenProperty}"`)
    }
    for (const forbiddenValue of [
      privateRecord.publicViewDigest,
      privateRecord.envelopeDigest,
      privateRecord.contextDigest,
      privateRecord.datasetSnapshotId,
      privateRecord.threadId,
      privateRecord.turnId,
      privateRecord.renderedTextSha256,
      privateRecord.privateRecordDigest
    ]) {
      expect(viewBody).not.toContain(forbiddenValue)
    }
    expect(body).toContain('"schemaVersion":3')
    expect(body).toContain(`"setDigest":"${publicSetDigest}"`)
    expect(body).not.toContain('"acceptedFinal":')
    expect(body).toContain('"publicationState":"accepted"')
    expect(body).not.toContain('"publicViewDigest":')
    expect(body).not.toContain('"renderedTextSha256":')
    expect(body).not.toContain('"envelopeIssuedAt":')
    expect(body).not.toContain('"contextDigest":')
    expect(body).not.toContain('"datasetSnapshotId":')

    await handlers.get('runtime:sse:stop')?.({ sender }, 'stream-v3-generic-accepted')
    controlled.close()
    await flushAsync()
  })

  it('rejects a complete Go-issued V5+V2 accepted final injected into generic SSE replay', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const batch = structuredClone(fixture.batch) as Record<string, any>
    const privateRecord = JSON.parse(readFileSync(new URL(
      '../../packages/runtime-go/internal/domain/evidence/testdata/accepted-final-v5-fact-witness-admission-v2.json',
      import.meta.url
    ), 'utf8')) as Record<string, any>
    expect(privateRecord.factFinalWitnessAdmission).toMatchObject({
      schemaVersion: 2,
      purpose: 'analytix.fact-final-witness-admission/v2'
    })
    expect(privateRecord.publicationSnapshotProofDigest).toMatch(/^[a-f0-9]{64}$/)
    batch.events[0].item.acceptedFinal = privateRecord

    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(batch.lastSeq, 'accepted_final_batch', batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: fixture.pin })
    const sender = makeSender()
    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-private-v5-v2-rejected'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)
    const sent = JSON.stringify(sender.send.mock.calls)
    expect(sent).not.toContain(privateRecord.factFinalWitnessAdmission.admissionDigest)
    expect(sent).not.toContain(privateRecord.publicationSnapshotProofDigest)
  })

  it('rejects a V3 view rewrite even when unsigned payload, manifest, and batch digests are recomputed', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const batch = structuredClone(fixture.batch) as Record<string, any>
    batch.events[0].item.acceptedFinalView.receiptMetadata.setDigest = 'f'.repeat(64)
    for (const event of batch.events as Array<Record<string, unknown>>) {
      event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event)
    }
    const manifest = (batch.events as Array<Record<string, unknown>>).map((event) => ({
      slot: event.publicationSlot,
      eventId: event.publicationEventId,
      payloadDigest: event.publicationPayloadDigest
    }))
    batch.eventManifestDigest = createHash('sha256').update(goCanonicalJson(manifest)).digest('hex')
    batch.batchId = createHash('sha256').update(JSON.stringify({
      purpose: batch.purpose,
      threadId: batch.threadId,
      turnId: batch.turnId,
      firstSeq: batch.firstSeq,
      lastSeq: batch.lastSeq,
      publicationCommitId: batch.publicationCommitId,
      eventManifestDigest: batch.eventManifestDigest
    })).digest('hex')

    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(batch.lastSeq, 'accepted_final_batch', batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: fixture.pin })
    const sender = makeSender()
    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-v3-rewritten-without-seal'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)
  })

  it('refreshes the accepted-final authority pin after a managed runtime reconnect', async () => {
    const firstRuntime = acceptedFinalDeliveryBatch()
    const freshRuntime = acceptedFinalDeliveryBatch()
    const freshPin = {
      ...freshRuntime.pin,
      runtimePid: firstRuntime.pin.runtimePid + 1,
      runtimeUrl: 'http://127.0.0.1:9989',
      generation: firstRuntime.pin.generation + 1
    }
    const nextSettings = settingsForPort(9989)
    nextSettings.runtime.runtimeToken = 'runtime-token-next'
    const store = {
      load: vi.fn()
        .mockResolvedValueOnce(settingsForPort(9988))
        .mockResolvedValue(nextSettings)
    }
    const overrides = { authorityPin: firstRuntime.pin, store }
    const seenOrigins: string[] = []
    const seenAuthorization: Array<string | undefined> = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      seenOrigins.push(new URL(String(input)).origin)
      seenAuthorization.push(requestHeader(init?.headers, 'Authorization'))
      if (fetchMock.mock.calls.length === 1) {
        // The first connection was authenticated with the old process pin.
        // Its EOF represents the managed runtime restart.
        overrides.authorityPin = freshPin
        return responseFor([])
      }
      return responseFor([
        strictFrame(freshRuntime.batch.lastSeq, 'accepted_final_batch', freshRuntime.batch)
      ])
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch, overrides)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: freshRuntime.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-restarted-accepted'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(seenOrigins).toEqual(['http://127.0.0.1:9988', 'http://127.0.0.1:9989'])
    expect(seenAuthorization).toEqual(['Bearer runtime-token', 'Bearer runtime-token-next'])
    expect(sseEventCalls(sender)[0]?.[1]).toEqual({
      streamId: 'stream-restarted-accepted',
      events: [freshRuntime.batch]
    })
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-restarted-accepted',
      seq: freshRuntime.batch.lastSeq,
      batchId: freshRuntime.batch.batchId,
      threadId: freshRuntime.batch.threadId,
      turnId: freshRuntime.batch.turnId,
      publicationCommitId: freshRuntime.batch.publicationCommitId
    })).toBe(true)
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end')
    )
  })

  it('retries a recoverable runtime ensure failure after a long-lived managed reconnect', async () => {
    vi.useFakeTimers()
    const firstRuntime = acceptedFinalDeliveryBatch()
    const freshRuntime = acceptedFinalDeliveryBatch()
    const freshPin = {
      ...freshRuntime.pin,
      runtimePid: firstRuntime.pin.runtimePid + 1,
      runtimeUrl: 'http://127.0.0.1:9989',
      generation: firstRuntime.pin.generation + 1
    }
    const initialSettings = settingsForPort(9988)
    const nextSettings = settingsForPort(9989)
    nextSettings.runtime.runtimeToken = 'runtime-token-next'
    const store = {
      load: vi.fn()
        .mockResolvedValueOnce(initialSettings)
        .mockResolvedValue(nextSettings)
    }
    const ensureRuntime = vi.fn()
      .mockResolvedValueOnce(initialSettings)
      .mockRejectedValueOnce(new Error(JSON.stringify({
        code: 'runtime_unavailable',
        message: 'Analytix runtime unavailable'
      })))
      .mockResolvedValueOnce(nextSettings)
    const overrides = { authorityPin: firstRuntime.pin, store, ensureRuntime }
    const firstConnection = controlledStream([])
    const seenOrigins: string[] = []
    const seenAuthorization: Array<string | undefined> = []
    const seenSinceSeq: string[] = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      seenOrigins.push(url.origin)
      seenAuthorization.push(requestHeader(init?.headers, 'Authorization'))
      seenSinceSeq.push(url.searchParams.get('since_seq') ?? '')
      if (fetchMock.mock.calls.length === 1) {
        return new Response(firstConnection.stream, {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return responseFor([
        strictFrame(freshRuntime.batch.lastSeq, 'accepted_final_batch', freshRuntime.batch)
      ])
    })
    const { handlers, logError } = registerHarness(fetchMock as typeof fetch, overrides)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: freshRuntime.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-long-lived-reconnect'
    })
    await flushAsync(50)
    expect(fetchMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(46_000)
    overrides.authorityPin = freshPin
    firstConnection.close()
    await flushAsync(50)

    expect(ensureRuntime).toHaveBeenCalledTimes(2)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end')).toBe(false)

    await vi.advanceTimersByTimeAsync(750)
    await flushAsync(80)

    expect(ensureRuntime).toHaveBeenCalledTimes(3)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(seenOrigins).toEqual(['http://127.0.0.1:9988', 'http://127.0.0.1:9989'])
    expect(seenAuthorization).toEqual(['Bearer runtime-token', 'Bearer runtime-token-next'])
    expect(seenSinceSeq).toEqual(['0', '0'])
    expect(sseEventCalls(sender)).toHaveLength(1)
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end')).toBe(false)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-long-lived-reconnect',
      seq: freshRuntime.batch.lastSeq,
      batchId: freshRuntime.batch.batchId,
      threadId: freshRuntime.batch.threadId,
      turnId: freshRuntime.batch.turnId,
      publicationCommitId: freshRuntime.batch.publicationCommitId
    })).toBe(true)
    await flushAsync(30)
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end')).toBe(true)
    const serializedLogs = JSON.stringify(logError.mock.calls)
    expect(serializedLogs).not.toContain('sse_worker_error')
    expect(serializedLogs).not.toContain('Analytix runtime unavailable')
  })

  it('surfaces a non-recoverable reconnect ensure error without crashing the worker', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const firstConnection = controlledStream([])
    const initialSettings = settingsForPort(9988)
    const store = { load: vi.fn(async () => initialSettings) }
    const ensureRuntime = vi.fn()
      .mockResolvedValueOnce(initialSettings)
      .mockRejectedValueOnce(new Error(JSON.stringify({
        code: 'missing_api_key',
        message: 'PRIVATE_MISSING_KEY_DETAIL'
      })))
    const fetchMock = vi.fn(async () => new Response(firstConnection.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers, logError } = registerHarness(fetchMock as typeof fetch, {
      authorityPin: fixture.pin,
      store,
      ensureRuntime
    })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: fixture.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-nonrecoverable-reconnect'
    })
    await waitForCondition(() => fetchMock.mock.calls.length === 1)
    firstConnection.close()
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')
    )

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId: 'stream-nonrecoverable-reconnect',
      code: 'missing_api_key',
      message: 'Runtime request failed (missing_api_key).'
    })
    expect(logError).toHaveBeenCalledWith(
      'sse',
      'SSE runtime ensure failed',
      {
        code: 'missing_api_key',
        phase: 'reconnect'
      }
    )
    const serialized = JSON.stringify({ messages: sender.send.mock.calls, logs: logError.mock.calls })
    expect(serialized).not.toContain('PRIVATE_MISSING_KEY_DETAIL')
    expect(serialized).not.toContain('sse_worker_error')
  })

  it('replays from the prior cursor when a connection pin becomes stale before terminal delivery', async () => {
    const staleRuntime = acceptedFinalDeliveryBatch()
    const freshRuntime = acceptedFinalDeliveryBatch()
    const freshPin = {
      ...freshRuntime.pin,
      runtimePid: staleRuntime.pin.runtimePid + 1,
      generation: staleRuntime.pin.generation + 1
    }
    const overrides = { authorityPin: staleRuntime.pin }
    const encoder = new TextEncoder()
    let staleController: ReadableStreamDefaultController<Uint8Array> | undefined
    const staleStream = new ReadableStream<Uint8Array>({
      start(controller) {
        staleController = controller
      }
    })
    const seenSinceSeq: string[] = []
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenSinceSeq.push(new URL(String(input)).searchParams.get('since_seq') ?? '')
      if (fetchMock.mock.calls.length === 1) {
        return new Response(staleStream, {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return responseFor([
        strictFrame(freshRuntime.batch.lastSeq, 'accepted_final_batch', freshRuntime.batch)
      ])
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch, overrides)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: staleRuntime.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-stale-connection-pin'
    })
    await waitForCondition(() => fetchMock.mock.calls.length === 1)
    overrides.authorityPin = freshPin
    staleController?.enqueue(encoder.encode(
      strictFrame(staleRuntime.batch.lastSeq, 'accepted_final_batch', staleRuntime.batch)
    ))
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(seenSinceSeq).toEqual(['0', '0'])
    expect(sseEventCalls(sender)[0]?.[1]).toEqual({
      streamId: 'stream-stale-connection-pin',
      events: [freshRuntime.batch]
    })
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-stale-connection-pin',
      seq: freshRuntime.batch.lastSeq,
      batchId: freshRuntime.batch.batchId,
      threadId: freshRuntime.batch.threadId,
      turnId: freshRuntime.batch.turnId,
      publicationCommitId: freshRuntime.batch.publicationCommitId
    })).toBe(true)
  })

  it.each(['separate', 'coalesced'])('delivers an ordinary general terminal batch alone and waits for its atomic renderer ACK (%s)', async (chunking) => {
    const threadId = 'thread-general-terminal'
    const batch = generalTerminalDeliveryBatch(threadId, 'turn-general-terminal', 2)
    const frames = [
      eventFrame(1, 'progress', threadId),
      strictFrame(batch.lastSeq as number, 'general_terminal_batch', batch),
      eventFrame(4, 'must-not-follow-terminal', threadId),
      eventFrame(5, 'later-history-must-not-follow-terminal', threadId)
    ]
    const controlled = controlledStream(chunking === 'coalesced' ? [frames.join('')] : frames)
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId,
      sinceSeq: 0,
      streamId: 'stream-general-terminal'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 2)

    expect(sseEventCalls(sender).map(([, payload]) => payload)).toEqual([
      { streamId: 'stream-general-terminal', events: [runtimeEvent(1, 'progress', threadId)] },
      { streamId: 'stream-general-terminal', events: [batch] }
    ])
    expect(JSON.stringify(sseEventCalls(sender))).not.toContain('must-not-follow-terminal')
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    expect(sender.send).not.toHaveBeenCalledWith('runtime:sse-end', expect.anything())
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-general-terminal',
      seq: batch.lastSeq
    })).toBe(true)
    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-end')
    )
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-general-terminal', seq: batch.lastSeq
    })).toBe(false)
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)

    controlled.close()
    await flushAsync()
  })

  it('delivers a Go-compatible three-slot child terminal batch with bounded child metadata', async () => {
    const threadId = 'thread-child-general-terminal'
    const batch = generalTerminalDeliveryBatch(threadId, 'turn-child-general-terminal', 11, {
      includeTerminalItem: true,
      usageSource: 'subagent',
      childRunId: 'child-run-1'
    })
    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(batch.lastSeq as number, 'general_terminal_batch', batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId,
      sinceSeq: 10,
      streamId: 'stream-child-general-terminal'
    })
    await waitForCondition(() => sseEventCalls(sender).length === 1)

    expect(sseEventCalls(sender)[0]?.[1]).toEqual({
      streamId: 'stream-child-general-terminal',
      events: [batch]
    })
    expect((batch.events as Array<Record<string, unknown>>)[1]).toMatchObject({
      kind: 'usage',
      usageSource: 'subagent',
      childRunId: 'child-run-1'
    })
    expect(sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')).toBe(false)
    expect(await handlers.get('runtime:sse:ack')?.({ sender }, {
      streamId: 'stream-child-general-terminal',
      seq: batch.lastSeq
    })).toBe(true)
  })

  it('rejects a shape-valid general terminal batch whose public integrity digests are forged', async () => {
    const threadId = 'thread-general-terminal-forged'
    const batch = generalTerminalDeliveryBatch(threadId, 'turn-general-terminal-forged', 2)
    batch.batchDigest = 'a'.repeat(64)
    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(batch.lastSeq as number, 'general_terminal_batch', batch)
    ]))
    const { handlers, logError } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId,
      sinceSeq: 0,
      streamId: 'stream-general-terminal-forged'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))

    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(logError).toHaveBeenCalledWith(
      'sse',
      'SSE event rejected',
      expect.objectContaining({ reasonCode: 'general_terminal_batch_integrity_invalid' })
    )
    expect(sender.send).toHaveBeenCalledWith('runtime:sse-error', {
      streamId: 'stream-general-terminal-forged',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'general_terminal_batch_integrity_invalid'
    })
  })

  it('releases a required accepted-final ACK waiter when its WebContents owner is destroyed', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const controlled = controlledStream([
      strictFrame(fixture.batch.lastSeq, 'accepted_final_batch', fixture.batch)
    ])
    const fetchMock = vi.fn(async () => {
      if (fetchMock.mock.calls.length === 1) {
        return new Response(controlled.stream, {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return new Response('gone', { status: 404 })
    })
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: fixture.pin })
    const owner = makeEventEmitterSender(501)

    await handlers.get('runtime:sse:start')?.({ sender: owner }, {
      threadId: fixture.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-accepted-owner-destroyed'
    })
    await waitForCondition(() => sseEventCalls(owner).length === 1)
    owner.emit('destroyed')
    await flushAsync()
    expect(owner.send).not.toHaveBeenCalledWith('runtime:sse-end', expect.anything())

    const replacement = makeEventEmitterSender(502)
    await expect(handlers.get('runtime:sse:start')?.({ sender: replacement }, {
      threadId: fixture.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-accepted-owner-destroyed'
    })).resolves.toEqual({ streamId: 'stream-accepted-owner-destroyed' })
    await handlers.get('runtime:sse:stop')?.({ sender: replacement }, 'stream-accepted-owner-destroyed')
    controlled.close()
  })

  it('rejects a rehashed and re-signed accepted-final batch with detached terminal semantics', async () => {
    const fixture = acceptedFinalDeliveryBatch()
    const batch = structuredClone(fixture.batch) as Record<string, any>
    batch.events[1].usageFinalStatus = 'failed'
    resignAcceptedFinalDeliveryBatch(batch, fixture.privateKey)
    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(batch.lastSeq, 'accepted_final_batch', batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: fixture.pin })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-semantically-detached-accepted'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)
  })

  it('rejects a foreign self-signed accepted-final batch even when all ordinary hashes are coherent', async () => {
    const trusted = acceptedFinalDeliveryBatch()
    const foreign = acceptedFinalDeliveryBatch()
    const fetchMock = vi.fn(async () => responseFor([
      strictFrame(foreign.batch.lastSeq, 'accepted_final_batch', foreign.batch)
    ]))
    const { handlers } = registerHarness(fetchMock as typeof fetch, { authorityPin: trusted.pin })
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: foreign.batch.threadId,
      sinceSeq: 0,
      streamId: 'stream-foreign-accepted'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))
    expect(sseEventCalls(sender)).toHaveLength(0)
  })

  it('rejects a malformed accepted-final projection without skipping to a later sequence', async () => {
    const { acceptedFinal, acceptedFinalView } = acceptedFinalPair()
    const validItem = {
      id: 'item-case-final',
      turnId: acceptedFinal.turnId,
      threadId: acceptedFinal.threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: acceptedFinal.acceptedAt,
      finishedAt: acceptedFinal.acceptedAt,
      kind: 'assistant_text',
      text: 'host boundary',
      acceptedFinal,
      acceptedFinalView
    }
    const eventMetadata = {
      timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      itemId: validItem.id,
      acceptedFinalDigest: acceptedFinal.recordDigest,
      publicationCommitId: acceptedFinal.recordDigest,
      publicationEventId: '7'.repeat(64),
      publicationSlot: 'assistant-final',
      publicationPayloadDigest: '6'.repeat(64)
    }
    const controlled = controlledStream([
      payloadFrame(1, 'item_completed', {
        ...eventMetadata,
        item: { ...validItem, text: 'must not pass', acceptedFinalView: { ...acceptedFinalView, rawReceiptId: 'receipt-private' } }
      }),
      payloadFrame(2, 'item_completed', {
        ...eventMetadata,
        item: validItem
      })
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: 'thread-case',
      sinceSeq: 0,
      streamId: 'stream-case'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))

    const serialized = JSON.stringify(sseEventCalls(sender))
    expect(sseEventCalls(sender)).toHaveLength(0)
    expect(serialized).not.toContain('host boundary')
    expect(serialized).not.toContain('must not pass')
    expect(serialized).not.toContain('receipt-private')
  })

  it('never emits an assistant draft delta before a fragmented think block fails closed', async () => {
    const assistantItem = (text: string) => ({
      id: 'item-1',
      turnId: 'turn-1',
      threadId: 'thread-fragmented-reasoning',
      role: 'assistant',
      status: 'running',
      createdAt: '2026-07-14T00:00:00.000Z',
      kind: 'assistant_text',
      text
    })
    const controlled = controlledStream([
      payloadFrame(1, 'assistant_text_delta', {
        threadId: 'thread-fragmented-reasoning',
        turnId: 'turn-1',
        itemId: 'item-1',
        item: assistantItem('public<thi')
      }),
      payloadFrame(2, 'assistant_text_delta', {
        threadId: 'thread-fragmented-reasoning',
        turnId: 'turn-1',
        itemId: 'item-1',
        item: assistantItem('nk>PRIVATE_REASONING')
      }),
      payloadFrame(3, 'assistant_text_delta', {
        threadId: 'thread-fragmented-reasoning',
        turnId: 'turn-1',
        itemId: 'item-1',
        item: assistantItem('</th')
      }),
      payloadFrame(4, 'assistant_text_delta', {
        threadId: 'thread-fragmented-reasoning',
        turnId: 'turn-1',
        itemId: 'item-1',
        item: assistantItem('ink>safe')
      })
    ])
    const fetchMock = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    const { handlers } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({ sender }, {
      threadId: 'thread-fragmented-reasoning',
      sinceSeq: 0,
      streamId: 'stream-fragmented-reasoning'
    })
    await waitForCondition(() => sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error'))

    const serialized = JSON.stringify(sseEventCalls(sender))
    expect(serialized).not.toContain('public')
    expect(serialized).not.toContain('safe')
    expect(serialized).not.toContain('PRIVATE_REASONING')
    expect(serialized).not.toContain('<thi')
  })

  it('redacts secrets before forwarding SSE errors to renderer and logs', async () => {
    const fetchMock = vi.fn(async () => {
      throw new Error('upstream rejected Authorization: Bearer sk-liveSecretValue1234567890')
    })
    const { handlers, logError } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender()

    await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-secret', sinceSeq: 0, streamId: 'stream-secret' })

    await waitForCondition(() =>
      sender.send.mock.calls.some(([channel]) => channel === 'runtime:sse-error')
    )
    const errorCall = sender.send.mock.calls.find(([channel]) => channel === 'runtime:sse-error')
    const logCall = logError.mock.calls.find(([scope]) => scope === 'sse')
    const serialized = JSON.stringify({ errorCall, logCall })

    expect(errorCall).toEqual([
      'runtime:sse-error',
      {
        streamId: 'stream-secret',
        code: 'sse_stream_error',
        message: 'Runtime request failed (sse_stream_error).'
      }
    ])
    expect(serialized).not.toContain('sk-liveSecretValue1234567890')
    expect(serialized).not.toContain('Bearer sk-')
  })

  it('stops safely when the renderer is destroyed before forwarding events', async () => {
    const fetchMock = vi.fn(async () => responseFor([eventFrame(1, 'one', 'thread-c')]))
    const { handlers, logError } = registerHarness(fetchMock as typeof fetch)
    const sender = makeSender(true)

    await handlers.get('runtime:sse:start')?.({
      sender
    }, { threadId: 'thread-c', sinceSeq: 0, streamId: 'stream-c' })
    await waitForCondition(() => fetchMock.mock.calls.length === 1)
    await flushAsync()

    expect(sender.send).not.toHaveBeenCalled()
    expect(logError).not.toHaveBeenCalledWith(
      'sse',
      expect.stringContaining('SSE worker crashed'),
      expect.anything()
    )
  })
})
