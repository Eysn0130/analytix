import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'

const electronMock = vi.hoisted(() => {
  const exposedApi = new Map<string, unknown>()
  return {
    exposedApi,
    exposeInMainWorld: vi.fn((name: string, api: unknown) => {
      exposedApi.set(name, api)
    }),
    invoke: vi.fn(),
    on: vi.fn(),
    removeListener: vi.fn(),
    getPathForFile: vi.fn()
  }
})

vi.mock('electron', () => ({
  contextBridge: {
    exposeInMainWorld: electronMock.exposeInMainWorld
  },
  ipcRenderer: {
    invoke: electronMock.invoke,
    on: electronMock.on,
    removeListener: electronMock.removeListener
  },
  webUtils: {
    getPathForFile: electronMock.getPathForFile
  }
}))

async function loadPreloadApi(): Promise<AnalytixApi> {
  vi.resetModules()
  electronMock.exposedApi.clear()
  await import('./index')
  const api = electronMock.exposedApi.get('analytix')
  expect(electronMock.exposeInMainWorld).toHaveBeenCalledWith('analytix', api)
  return api as AnalytixApi
}

function acceptedFinalProjection() {
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
    envelopeIssuedAt: '2026-07-11T01:02:03Z',
    acceptedAt: '2026-07-11T01:02:03Z'
  }
  const record = {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: 'a'.repeat(64),
    authorityPublicKey: 'A'.repeat(43),
    threadId: 'thread-case',
    turnId: 'turn-case',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-case-3',
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    renderedTextSha256: 'd'.repeat(64),
    registrySequence: 0,
    registryStateDigest: 'e'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest: '7'.repeat(64),
    privateRecordDigest: 'f'.repeat(64),
    acceptedAt: '2026-07-11T01:02:03Z',
    authoritySignature: 'A'.repeat(86),
    recordDigest: '9'.repeat(64)
  }
  const expandedView = {
    schemaVersion: 3,
    acceptedFinalDigest: record.recordDigest,
    publicationState: 'accepted',
    variant: publicView.variant,
    terminalReason: publicView.terminalReason,
    blockerCode: publicView.blockerCode,
    coverageStatus: publicView.coverageStatus,
    checkedScopeDigest: publicView.checkedScopeDigest,
    missingScopeCount: publicView.missingScopeCount,
    claimCount: publicView.claimCount,
    claimTypes: publicView.claimTypes,
    receiptMetadata: publicView.receiptMetadata,
    noHitWording: publicView.noHitWording,
    acceptedAt: publicView.acceptedAt
  }
  return { record, view: expandedView }
}

function acceptedFinalBatchProjection() {
  const { record, view } = acceptedFinalProjection()
  const item = {
    id: 'item-case-final',
    turnId: record.turnId,
    threadId: record.threadId,
    role: 'assistant',
    status: 'completed',
    createdAt: record.acceptedAt,
    finishedAt: record.acceptedAt,
    kind: 'assistant_text',
    text: 'host boundary',
    acceptedFinalView: view
  }
  const events = [
    {
      kind: 'item_completed', seq: 3, timestamp: record.acceptedAt,
      threadId: record.threadId, turnId: record.turnId, itemId: item.id, item,
      acceptedFinalDigest: record.recordDigest, publicationCommitId: record.recordDigest,
      publicationEventId: '1'.repeat(64), publicationSlot: 'assistant-final',
      publicationPayloadDigest: '2'.repeat(64)
    },
    {
      kind: 'usage', seq: 4, timestamp: record.acceptedAt,
      threadId: record.threadId, turnId: record.turnId, model: 'deepseek-chat',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0, cacheHitRate: null, turns: 0 },
      cacheDiagnostics: {}, usageFinalStatus: 'completed',
      acceptedFinalDigest: record.recordDigest, publicationCommitId: record.recordDigest,
      publicationEventId: '3'.repeat(64), publicationSlot: 'usage',
      publicationPayloadDigest: '4'.repeat(64)
    },
    {
      kind: 'turn_completed', seq: 5, timestamp: record.acceptedAt,
      threadId: record.threadId, turnId: record.turnId, status: 'completed',
      terminalReason: record.terminalReason,
      acceptedFinalDigest: record.recordDigest, publicationCommitId: record.recordDigest,
      publicationEventId: '5'.repeat(64), publicationSlot: 'terminal',
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
    threadId: record.threadId,
    turnId: record.turnId,
    seq: 5,
    firstSeq: 3,
    lastSeq: 5,
    timestamp: record.acceptedAt,
    publicationCommitId: record.recordDigest,
    eventManifestDigest,
    publicationAuthority: {
      schemaVersion: 'accepted-final-delivery-seal.v1',
      purpose: 'analytix.accepted-final-delivery-seal/v1',
      sealId: '9'.repeat(64),
      threadId: record.threadId,
      turnId: record.turnId,
      publicationCommitId: record.recordDigest,
      acceptedFinalDispositionDigest: 'a'.repeat(64),
      terminalDispositionId: 'b'.repeat(64),
      eventManifestDigest,
      sequencedEventsDigest: 'c'.repeat(64),
      batchId,
      firstSeq: 3,
      lastSeq: 5,
      timestamp: record.acceptedAt,
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: record.authorityKeyId,
      authorityPublicKey: record.authorityPublicKey,
      authoritySignature: record.authoritySignature
    },
    events
  }
}

describe('preload SSE bridge', () => {
  beforeEach(() => {
    electronMock.exposedApi.clear()
    electronMock.exposeInMainWorld.mockClear()
    electronMock.invoke.mockReset()
    electronMock.on.mockReset()
    electronMock.removeListener.mockReset()
    electronMock.getPathForFile.mockReset()
  })

  it('passes SSE start and stop requests through the analytix runtime facade unchanged', async () => {
    electronMock.invoke
      .mockResolvedValueOnce({ streamId: 'stream-provided' })
      .mockResolvedValueOnce(true)
    const api = await loadPreloadApi()
    const threadId = 'thr/with space?x=1#frag'

    await expect(api.runtime.startSse(threadId, 42, 'stream-provided')).resolves.toEqual({
      streamId: 'stream-provided'
    })
    await expect(api.runtime.stopSse('stream-provided')).resolves.toBe(true)

    expect(electronMock.invoke).toHaveBeenNthCalledWith(1, 'runtime:sse:start', {
      threadId,
      sinceSeq: 42,
      streamId: 'stream-provided'
    })
    expect(electronMock.invoke).toHaveBeenNthCalledWith(2, 'runtime:sse:stop', 'stream-provided')
  })

  it('keeps ordinary ACKs two-field and binds accepted-final ACKs to the exact batch identity', async () => {
    electronMock.invoke.mockResolvedValue(true)
    const api = await loadPreloadApi()
    const acceptedFinal = {
      batchId: '7'.repeat(64),
      threadId: 'thread-case',
      turnId: 'turn-case',
      publicationCommitId: '9'.repeat(64)
    }

    await expect(api.runtime.ackSseEvent('stream-ack', 4)).resolves.toBe(true)
    await expect(api.runtime.ackSseEvent('stream-ack', 5, acceptedFinal)).resolves.toBe(true)

    expect(electronMock.invoke).toHaveBeenNthCalledWith(1, 'runtime:sse:ack', {
      streamId: 'stream-ack', seq: 4
    })
    expect(electronMock.invoke).toHaveBeenNthCalledWith(2, 'runtime:sse:ack', {
      streamId: 'stream-ack', seq: 5, ...acceptedFinal
    })
  })

  it('forwards SSE event, end, and error payloads without exposing Electron events', async () => {
    const api = await loadPreloadApi()
    const eventHandler = vi.fn()
    const endHandler = vi.fn()
    const errorHandler = vi.fn()

    const offEvent = api.runtime.onSseEvent(eventHandler)
    const eventWrapper = electronMock.on.mock.calls.find(([channel]) => channel === 'runtime:sse-event')?.[1]
    const heartbeat = {
      seq: 1,
      kind: 'heartbeat',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-a'
    }
    eventWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      events: [heartbeat]
    })
    const revocation = {
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId: 'thread-a',
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected',
      action: 'purge_case_projection',
      terminal: true
    }
    eventWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      events: [revocation]
    })
    eventWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      events: [heartbeat, revocation]
    })
    offEvent()

    const offEnd = api.runtime.onSseEnd(endHandler)
    const endWrapper = electronMock.on.mock.calls.find(([channel]) => channel === 'runtime:sse-end')?.[1]
    endWrapper?.({ sender: 'electron-event' }, { streamId: 'stream-a' })
    endWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      privatePayload: 'PRIVATE_END_SENTINEL'
    })
    offEnd()

    const offError = api.runtime.onSseError(errorHandler)
    const errorWrapper = electronMock.on.mock.calls.find(([channel]) => channel === 'runtime:sse-error')?.[1]
    errorWrapper?.({ sender: 'electron-event' }, { streamId: 'stream-a', status: 404 })
    errorWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'general_terminal_batch_invalid'
    })
    errorWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'raw_provider_body'
    })
    errorWrapper?.({ sender: 'electron-event' }, {
      streamId: 'stream-a',
      code: 'sse_stream_error',
      message: 'PRIVATE_ERROR_SENTINEL'
    })
    offError()

    expect(eventHandler).toHaveBeenCalledWith({
      streamId: 'stream-a',
      events: [heartbeat]
    })
    expect(eventHandler).toHaveBeenCalledWith({
      streamId: 'stream-a',
      events: [revocation]
    })
    expect(eventHandler).toHaveBeenCalledTimes(2)
    expect(endHandler).toHaveBeenCalledWith({ streamId: 'stream-a' })
    expect(endHandler).toHaveBeenCalledTimes(1)
    expect(errorHandler).toHaveBeenCalledWith({ streamId: 'stream-a', status: 404 })
    expect(errorHandler).toHaveBeenCalledWith({
      streamId: 'stream-a',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'general_terminal_batch_invalid'
    })
    expect(errorHandler).toHaveBeenCalledTimes(2)
    expect(electronMock.removeListener).toHaveBeenCalledWith('runtime:sse-event', eventWrapper)
    expect(electronMock.removeListener).toHaveBeenCalledWith('runtime:sse-end', endWrapper)
    expect(electronMock.removeListener).toHaveBeenCalledWith('runtime:sse-error', errorWrapper)
  })

  it('admits typed user text but rejects open provider, tool, MCP, job, path, and case-fact carriers', async () => {
    const api = await loadPreloadApi()
    const handler = vi.fn()
    api.runtime.onSseEvent(handler)
    const wrapped = electronMock.on.mock.calls.find(([channel]) => channel === 'runtime:sse-event')?.[1]
    const pipeline = {
      seq: 2,
      kind: 'pipeline_stage',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-open',
      turnId: 'turn-open',
      stage: 'response_received',
      label: 'Response Received'
    }
    const invalidEvents = [
      { ...pipeline, message: 'MARKER_FREE_PROVIDER_ERROR_7F3C' },
      { ...pipeline, summary: 'MARKER_FREE_MCP_RESPONSE_9A2D' },
      { ...pipeline, reason: 'MARKER_FREE_JOB_FAILURE_4C8E' },
      { ...pipeline, label: 'MARKER_FREE_PROVIDER_LABEL_2B6A' },
      { ...pipeline, details: { message: 'MARKER_FREE_MCP_RESPONSE_9A2D' } },
      { ...pipeline, details: { providerError: { status: 503, retryable: true } } },
      { ...pipeline, summary: '/Users/sun/private/case.csv' },
      { ...pipeline, message: '账户 6222020202020202020 收款 2645472 元' },
      {
        seq: 3,
        kind: 'item_completed',
        timestamp: '2026-07-14T00:00:00.000Z',
        threadId: 'thread-open',
        turnId: 'turn-open',
        itemId: 'item-mcp',
        item: {
          id: 'item-mcp',
          turnId: 'turn-open',
          threadId: 'thread-open',
          role: 'tool',
          status: 'completed',
          createdAt: '2026-07-14T00:00:00.000Z',
          finishedAt: '2026-07-14T00:00:01.000Z',
          kind: 'tool_result',
          toolName: 'mcp__funds__query',
          callId: 'call-mcp',
          toolKind: 'tool_call',
          isError: false,
          output: { content: [{ type: 'text', text: 'MARKER_FREE_MCP_RESPONSE_9A2D' }] }
        }
      }
    ]
    for (const event of invalidEvents) {
      wrapped?.({}, { streamId: 'stream-open', events: [event] })
    }

    const typedUserEvent = {
      seq: 4,
      kind: 'item_completed',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-open',
      turnId: 'turn-open',
      itemId: 'item-user',
      item: {
        id: 'item-user',
        turnId: 'turn-open',
        threadId: 'thread-open',
        role: 'user',
        status: 'completed',
        createdAt: '2026-07-14T00:00:00.000Z',
        finishedAt: '2026-07-14T00:00:01.000Z',
        kind: 'user_message',
        text: 'typed user-origin text'
      }
    }
    wrapped?.({}, { streamId: 'stream-open', events: [typedUserEvent] })

    const providerStageEvent = {
      seq: 5,
      kind: 'pipeline_stage',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-open',
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
    wrapped?.({}, { streamId: 'stream-open', events: [providerStageEvent] })

    expect(handler).toHaveBeenCalledTimes(2)
    expect(handler).toHaveBeenCalledWith({ streamId: 'stream-open', events: [typedUserEvent] })
    expect(handler).toHaveBeenCalledWith({ streamId: 'stream-open', events: [providerStageEvent] })
    expect(JSON.stringify(handler.mock.calls)).not.toContain('MARKER_FREE_')
    expect(JSON.stringify(handler.mock.calls)).not.toContain('/Users/sun/private')
    expect(JSON.stringify(handler.mock.calls)).not.toContain('6222020202020202020')
  })

  it('rejects standalone accepted-final SSE projections at the preload boundary', async () => {
    const api = await loadPreloadApi()
    const handler = vi.fn()
    api.runtime.onSseEvent(handler)
    const wrapped = electronMock.on.mock.calls.find(([channel]) => channel === 'runtime:sse-event')?.[1]
    const { record, view } = acceptedFinalProjection()
    const item = {
      id: 'item-case-final',
      turnId: record.turnId,
      threadId: record.threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: record.acceptedAt,
      finishedAt: record.acceptedAt,
      kind: 'assistant_text',
      text: 'host boundary',
      acceptedFinal: record,
      acceptedFinalView: view
    }
    const event = {
      kind: 'item_completed',
      seq: 3,
      timestamp: record.acceptedAt,
      threadId: record.threadId,
      turnId: record.turnId,
      itemId: item.id,
      item,
      acceptedFinalDigest: record.recordDigest,
      publicationCommitId: record.recordDigest,
      publicationEventId: '7'.repeat(64),
      publicationSlot: 'assistant-final',
      publicationPayloadDigest: '6'.repeat(64)
    }

    wrapped?.({}, { streamId: 'stream-accepted', events: [event] })
    wrapped?.({}, {
      streamId: 'stream-accepted',
      events: [{
        ...event,
        item: { ...event.item, acceptedFinalView: { ...view, rawReceiptId: 'receipt-private' } }
      }]
    })
    wrapped?.({}, { streamId: 'stream-accepted', events: [event], providerSaysSafe: true })
    wrapped?.({}, {
      streamId: 'stream-accepted',
      events: [{
        kind: 'heartbeat',
        seq: 4,
        timestamp: record.acceptedAt,
        threadId: record.threadId,
        privatePayload: 'PRELOAD_PRIVATE_SENTINEL'
      }]
    })

    expect(handler).not.toHaveBeenCalled()
    expect(JSON.stringify(handler.mock.calls)).not.toContain('receipt-private')
    expect(JSON.stringify(handler.mock.calls)).not.toContain('PRELOAD_PRIVATE_SENTINEL')
  })

  it('forwards one complete accepted-final batch and rejects detached or mixed batches', async () => {
    const api = await loadPreloadApi()
    const handler = vi.fn()
    api.runtime.onSseEvent(handler)
    const wrapped = electronMock.on.mock.calls.find(([channel]) => channel === 'runtime:sse-event')?.[1]
    const batch = acceptedFinalBatchProjection()

    wrapped?.({}, { streamId: 'stream-accepted-batch', events: [batch] })
    wrapped?.({}, {
      streamId: 'stream-accepted-batch',
      events: [{
        ...batch,
        publicationAuthority: { ...batch.publicationAuthority, batchId: 'd'.repeat(64) }
      }]
    })
    wrapped?.({}, {
      streamId: 'stream-accepted-batch',
      events: [batch, {
        kind: 'heartbeat', seq: 6, timestamp: batch.timestamp, threadId: batch.threadId
      }]
    })
    wrapped?.({}, {
      streamId: 'stream-accepted-batch',
      events: [{ ...batch, publicationAuthority: { ...batch.publicationAuthority, privateKey: 'secret' } }]
    })

    expect(handler).toHaveBeenCalledTimes(1)
    expect(handler).toHaveBeenCalledWith({ streamId: 'stream-accepted-batch', events: [batch] })
    expect(JSON.stringify(handler.mock.calls)).not.toContain('secret')
  })
})
