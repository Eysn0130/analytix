import { createHash, generateKeyPairSync, sign } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import type {
  LarkChannel,
  MarkdownStreamController,
  SendOptions,
  SendResult
} from '@larksuiteoapi/node-sdk'
import {
  FEISHU_PUBLIC_PROJECTION_BOUNDARY,
  FeishuStreamer,
  type SseSubscriber
} from './feishu-streamer'

type StreamInput = { markdown: (controller: MarkdownStreamController) => Promise<void> }

function makeBridge(): {
  bridge: LarkChannel
  controller: MarkdownStreamController
  messageId: string
} {
  const messageId = 'om_stream_1'
  const controller: MarkdownStreamController = {
    append: vi.fn(async () => undefined),
    setContent: vi.fn(async () => undefined),
    get messageId() {
      return messageId
    }
  }
  const bridge = {
    stream: vi.fn(async (_to: string, input: StreamInput, _opts: SendOptions): Promise<SendResult> => {
      await input.markdown(controller)
      return { messageId }
    })
  } as unknown as LarkChannel
  return { bridge, controller, messageId }
}

function makeSubscriber(
  events: Array<Record<string, unknown>>,
  onEvent: (event: Record<string, unknown>) => void
): SseSubscriber {
  return (signal) => {
    let closed = false
    signal.addEventListener('abort', () => {
      closed = true
    }, { once: true })
    queueMicrotask(() => {
      for (const event of events) {
        if (closed) return
        onEvent(event)
      }
    })
    return {
      close: () => {
        closed = true
      }
    }
  }
}

const TEST_ACCEPTED_AT = '2026-07-15T01:02:03Z'
const PROVIDER_FAILURE_ERROR_ITEM_SENTINEL = 'PROVIDER_FAILURE_ERROR_ITEM_SENTINEL'
const RESTART_ERROR_ITEM_SENTINEL = 'RESTART_ERROR_ITEM_SENTINEL'
const TEST_SIGNATURE_DOMAIN = Buffer.from('analytix.final-answer-authority/v1\0')
const TEST_PUBLIC_VIEW_DOMAIN = Buffer.from('analytix.accepted-final-public-view/v2\0')
const TEST_EVENT_DOMAIN = 'analytix.accepted-final-event/v1\0'
const TEST_AUTHORITY = generateKeyPairSync('ed25519')
const TEST_PUBLIC_KEY_DER = TEST_AUTHORITY.publicKey.export({ format: 'der', type: 'spki' }) as Buffer
const TEST_PUBLIC_KEY = TEST_PUBLIC_KEY_DER.subarray(TEST_PUBLIC_KEY_DER.length - 32)
const TEST_MANAGED_AUTHORITY_PIN = {
  keyId: createHash('sha256').update(TEST_PUBLIC_KEY).digest('hex'),
  publicKey: TEST_PUBLIC_KEY.toString('base64url'),
  runtimePid: 4242,
  runtimeUrl: 'http://127.0.0.1:4242',
  generation: 7
} as const

function currentAuthorityOptions(current: () => boolean = () => true) {
  return {
    finalPublicationAuthorityPin: TEST_MANAGED_AUTHORITY_PIN,
    isFinalPublicationAuthorityPinCurrent: () => current()
  }
}

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function publicViewDigest(publicView: Record<string, unknown>): string {
  return sha256(Buffer.concat([TEST_PUBLIC_VIEW_DOMAIN, Buffer.from(goJSONStringify(publicView))]))
}

function goJSONStringify(value: unknown): string {
  const encoded = JSON.stringify(value)
  if (encoded === undefined) throw new Error('value is not JSON serializable')
  return encoded
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function goCanonicalJSON(value: unknown): string {
  if (value === null || typeof value !== 'object') return goJSONStringify(value)
  if (Array.isArray(value)) return `[${value.map(goCanonicalJSON).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record).sort().map((key) => `${goJSONStringify(key)}:${goCanonicalJSON(record[key])}`).join(',')}}`
}

function publicationEventId(recordDigest: string, slot: string): string {
  return sha256(`${TEST_EVENT_DOMAIN}${recordDigest}\0${slot}`)
}

function withPublicationMetadata(
  draft: Record<string, unknown>,
  recordDigest: string,
  slot: string
): Record<string, unknown> {
  const event: Record<string, unknown> = {
    ...draft,
    publicationCommitId: recordDigest,
    publicationEventId: publicationEventId(recordDigest, slot),
    publicationSlot: slot,
    acceptedFinalDigest: recordDigest,
    timestamp: TEST_ACCEPTED_AT
  }
  const canonical = { ...event }
  delete canonical.seq
  event.publicationPayloadDigest = sha256(goCanonicalJSON(canonical))
  return event
}

function atomicAssistantFinal(
  text: string,
  overrides: Partial<Record<string, unknown>> = {}
): Record<string, unknown> {
  const threadId = 'thread_1'
  const turnId = 'turn_1'
  const itemId = `item_${turnId}_assistant`
  return {
    kind: 'item_completed',
    seq: 2,
    timestamp: TEST_ACCEPTED_AT,
    threadId,
    turnId,
    itemId,
    item: {
      id: itemId,
      turnId,
      threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: TEST_ACCEPTED_AT,
      finishedAt: TEST_ACCEPTED_AT,
      kind: 'assistant_text',
      text
    },
    ...overrides
  }
}

function terminalEvent(kind: 'turn_completed' | 'turn_failed' | 'turn_aborted'): Record<string, unknown> {
  return {
    kind,
    seq: 3,
    timestamp: TEST_ACCEPTED_AT,
    threadId: 'thread_1',
    turnId: 'turn_1',
    status: kind.slice('turn_'.length)
  }
}

function acceptedFinalEvents(input: {
  text: string
  terminalReason: 'success' | 'source_unavailable' | 'provider_failure' | 'restart'
}): { item: Record<string, unknown>; terminal: Record<string, unknown> } {
  const { text, terminalReason } = input
  const threadId = 'thread_1'
  const turnId = 'turn_1'
  const acceptedAt = TEST_ACCEPTED_AT
  const generalGuidance = terminalReason === 'success'
  const sourceUnavailable = terminalReason === 'source_unavailable'
  const variant = generalGuidance
    ? 'GeneralGuidanceAnswer'
    : sourceUnavailable ? 'SourceUnavailableAnswer' : 'NeedsEvidenceAnswer'
  const publicView = {
    schemaVersion: 2,
    publicationState: 'accepted',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-case-3',
    variant,
    terminalReason,
    blockerCode: generalGuidance ? '' : sourceUnavailable ? 'current_case_source_unavailable' : 'provider_failure',
    coverageStatus: generalGuidance ? 'guidance_only' : sourceUnavailable ? 'unavailable' : 'unverified',
    checkedScopeDigest: '',
    missingScopeCount: generalGuidance || sourceUnavailable ? 0 : 1,
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
  const signedPublicViewDigest = publicViewDigest(publicView)
  const unsignedRecord = {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: sha256(TEST_PUBLIC_KEY),
    authorityPublicKey: TEST_PUBLIC_KEY.toString('base64url'),
    threadId,
    turnId,
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-case-3',
    variant,
    terminalReason,
    renderedTextSha256: sha256(text),
    registrySequence: 0,
    registryStateDigest: 'e'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest: signedPublicViewDigest,
    privateRecordDigest: 'f'.repeat(64),
    acceptedAt,
    authoritySignature: '',
    recordDigest: ''
  }
  const signingDigest = createHash('sha256').update(goJSONStringify(unsignedRecord)).digest()
  const authoritySignature = sign(
    null,
    Buffer.concat([TEST_SIGNATURE_DOMAIN, signingDigest]),
    TEST_AUTHORITY.privateKey
  ).toString('base64url')
  const signedRecord = { ...unsignedRecord, authoritySignature }
  const record = { ...signedRecord, recordDigest: sha256(goJSONStringify(signedRecord)) }
  const { schemaVersion, publicationState, ...publicViewFields } = publicView
  const view = {
    schemaVersion,
    publicationState,
    acceptedFinalDigest: record.recordDigest,
    publicViewDigest: signedPublicViewDigest,
    ...publicViewFields
  }
  const itemId = `item_${turnId}_assistant`
  const item = withPublicationMetadata({
    kind: 'item_completed',
    seq: 2,
    timestamp: acceptedAt,
    threadId,
    turnId,
    itemId,
    item: {
      id: itemId,
      turnId,
      threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: acceptedAt,
      finishedAt: acceptedAt,
      kind: 'assistant_text',
      text,
      acceptedFinal: record,
      acceptedFinalView: view
    }
  }, record.recordDigest, 'assistant-final')
  const status = generalGuidance || sourceUnavailable ? 'completed' : terminalReason === 'restart' ? 'aborted' : 'failed'
  const terminalErrorItemId = `item_${turnId}_terminal_error`
  const terminalCode = `case_terminal_${terminalReason}`
  const terminalErrorMessage = status === 'failed'
    ? '案件分析未完成；未经核验的案件事实未发布。'
    : status === 'aborted'
      ? '案件分析已终止；未经核验的案件事实未发布。'
      : '案件分析已结束；仅发布通过宿主证据门的固定边界答复。'
  const terminal = withPublicationMetadata({
    kind: `turn_${status}`,
    seq: 3,
    threadId,
    turnId,
    status,
    terminalReason,
    ...(status === 'completed' ? {} : { code: terminalCode, itemId: terminalErrorItemId }),
    ...(status === 'failed'
      ? { error: terminalErrorMessage, message: terminalErrorMessage }
      : {}),
    ...(status === 'aborted'
      ? { discard: true, cancelled: false, cancelledPendingGates: 0 }
      : {})
  }, record.recordDigest, 'terminal')
  return {
    item,
    terminal
  }
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

function acceptedFinalDeliveryBatch(
  accepted: ReturnType<typeof acceptedFinalEvents>
): Record<string, unknown> {
  const item = structuredClone(accepted.item)
  const terminal = structuredClone(accepted.terminal)
  const publicItem = item.item as Record<string, unknown>
  const record = publicItem.acceptedFinal as Record<string, unknown>
  const publicView = record.publicView as Record<string, unknown>
  const publicationCommitId = record.recordDigest as string
  publicItem.acceptedFinalView = {
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
  delete publicItem.acceptedFinal
  delete item.publicationPayloadDigest
  const publicItemPayload = { ...item }
  delete publicItemPayload.seq
  item.publicationPayloadDigest = sha256(goCanonicalJSON(publicItemPayload))
  item.seq = 2
  const hasTerminalError = terminal.status !== 'completed'
  const terminalError = hasTerminalError
    ? withPublicationMetadata({
        kind: 'item_completed',
        seq: 3,
        threadId: 'thread_1',
        turnId: 'turn_1',
        itemId: terminal.itemId,
        item: {
          id: terminal.itemId,
          turnId: 'turn_1',
          threadId: 'thread_1',
          role: 'system',
          status: terminal.status,
          createdAt: TEST_ACCEPTED_AT,
          finishedAt: TEST_ACCEPTED_AT,
          kind: 'error',
          code: terminal.code,
          message: terminal.status === 'failed'
            ? terminal.message
            : '案件分析已终止；未经核验的案件事实未发布。',
          severity: terminal.status === 'failed' ? 'error' : 'warning',
          acceptedFinalDigest: publicationCommitId
        }
      }, publicationCommitId, 'terminal-error-item')
    : null
  const usageSeq = hasTerminalError ? 4 : 3
  const lastSeq = hasTerminalError ? 5 : 4
  terminal.seq = terminal.seq === item.seq ? 2 : lastSeq
  const usage = withPublicationMetadata({
    kind: 'usage',
    seq: usageSeq,
    threadId: 'thread_1',
    turnId: 'turn_1',
    model: 'deepseek-chat',
    usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2, cacheHitRate: null, turns: 1 },
    cacheDiagnostics: {},
    usageFinalStatus: terminal.status
  }, publicationCommitId, 'usage')
  const events = [item, ...(terminalError ? [terminalError] : []), usage, terminal]
  const manifest = events.map((event) => ({
    slot: event.publicationSlot,
    eventId: event.publicationEventId,
    payloadDigest: event.publicationPayloadDigest
  }))
  const eventManifestDigest = sha256(goCanonicalJSON(manifest))
  const batchCore = {
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    threadId: 'thread_1',
    turnId: 'turn_1',
    firstSeq: 2,
    lastSeq,
    publicationCommitId,
    eventManifestDigest
  }
  const batchId = sha256(JSON.stringify(batchCore))
  const seal: Record<string, unknown> = {
    schemaVersion: 'accepted-final-delivery-seal.v1',
    purpose: 'analytix.accepted-final-delivery-seal/v1',
    sealId: '',
    threadId: 'thread_1',
    turnId: 'turn_1',
    publicationCommitId,
    acceptedFinalDispositionDigest: '4'.repeat(64),
    terminalDispositionId: '5'.repeat(64),
    eventManifestDigest,
    sequencedEventsDigest: sha256(goCanonicalJSON(events)),
    batchId,
    firstSeq: 2,
    lastSeq,
    timestamp: TEST_ACCEPTED_AT,
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: TEST_MANAGED_AUTHORITY_PIN.keyId,
    authorityPublicKey: TEST_MANAGED_AUTHORITY_PIN.publicKey,
    authoritySignature: ''
  }
  seal.sealId = sha256(Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-id/v1\0'),
    Buffer.from(JSON.stringify(orderedDeliverySeal(seal)))
  ]))
  const signingDigest = createHash('sha256')
    .update(JSON.stringify(orderedDeliverySeal(seal)))
    .digest()
  seal.authoritySignature = sign(null, Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-signature/v1\0'),
    signingDigest
  ]), TEST_AUTHORITY.privateKey).toString('base64url')
  return {
    schemaVersion: 2,
    ...batchCore,
    kind: 'accepted_final_batch',
    batchId,
    seq: lastSeq,
    timestamp: TEST_ACCEPTED_AT,
    publicationAuthority: seal,
    events
  }
}

describe('FeishuStreamer', () => {
  it('starts no Feishu stream effect when the live credential session is no longer current', async () => {
    const { bridge } = makeBridge()
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_stale',
      turnId: 'turn_stale',
      threadId: 'thread_stale',
      replyOptions: {},
      logger: vi.fn(),
      ...currentAuthorityOptions(),
      isBridgeCredentialAuthorityCurrent: async () => false
    })
    await expect(streamer.start({ subscribe: () => ({ close: vi.fn() }) }))
      .rejects.toThrow('Feishu account credential is unavailable.')
    expect((bridge as unknown as { stream: ReturnType<typeof vi.fn> }).stream).not.toHaveBeenCalled()
  })

  it('publishes one atomic assistant final only after the matching terminal event', async () => {
    const { bridge, controller, messageId } = makeBridge()
    const accepted = acceptedFinalEvents({ text: '你好', terminalReason: 'success' })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: { replyTo: 'om_in_1' },
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber(
      [
        { kind: 'assistant_text_delta', threadId: 'thread_1', turnId: 'turn_1', item: { text: 'DRAFT_SENTINEL' } },
        { kind: 'assistant_reasoning_delta', turnId: 'turn_1', item: { text: 'thinking' } },
        acceptedFinalDeliveryBatch(accepted)
      ],
      (event) => streamer.onSseEvent(event)
    )

    const result = await streamer.start({ subscribe })

    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith('你好')
    expect(result).toEqual({ ok: true, messageId, finalText: '你好', fellBack: false })
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain('DRAFT_SENTINEL')
  })

  it('buffers a synchronous atomic final delivered before the Feishu producer starts', async () => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({ text: 'atomic-before-producer', terminalReason: 'success' })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const close = vi.fn()
    const subscribe: SseSubscriber = () => {
      streamer.onSseEvent(acceptedFinalDeliveryBatch(accepted))
      return { close }
    }

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: true,
      finalText: 'atomic-before-producer'
    })
    expect(close).toHaveBeenCalledOnce()
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledOnce()
    expect(controller.setContent).toHaveBeenCalledWith('atomic-before-producer')
  })

  it.each([
    '金额为 4,200,000 元',
    '银行卡号 6222020202020202020',
    '设备 MAC 为 AA:BB:CC:DD:EE:FF',
    '张三是李四的舅舅',
    '投标报价为 8,800,000 元'
  ])('rejects an unaccepted ordinary assistant final before external delivery: %s', async (text) => {
    const { bridge, controller } = makeBridge()
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber(
      [atomicAssistantFinal(text), terminalEvent('turn_completed')],
      (event) => streamer.onSseEvent(event)
    )

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    const calls = JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls)
    expect(calls).toContain(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(calls).not.toContain(text)
  })

  it('never publishes reasoning from an atomic assistant item', async () => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({
      text: '公开<think>PRIVATE_REASONING</think>结论',
      terminalReason: 'success'
    })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber(
      [
        { kind: 'assistant_reasoning_delta', turnId: 'turn_1', item: { text: 'DELTA_REASONING' } },
        acceptedFinalDeliveryBatch(accepted)
      ],
      (event) => streamer.onSseEvent(event)
    )

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain('PRIVATE_REASONING')
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain('DELTA_REASONING')
  })

  it('preserves an accepted host boundary on a completed source-unavailable terminal', async () => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({
      text: 'HOST_ACCEPTED_SOURCE_BOUNDARY',
      terminalReason: 'source_unavailable'
    })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber([acceptedFinalDeliveryBatch(accepted)], (event) => streamer.onSseEvent(event))

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: true,
      finalText: 'HOST_ACCEPTED_SOURCE_BOUNDARY'
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith('HOST_ACCEPTED_SOURCE_BOUNDARY')
    expect(controller.setContent).not.toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
  })

  it('preserves an accepted host boundary while reporting a failed terminal', async () => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({
      text: 'HOST_ACCEPTED_FAILURE_BOUNDARY',
      terminalReason: 'provider_failure'
    })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber([acceptedFinalDeliveryBatch(accepted)], (event) => streamer.onSseEvent(event))

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: 'HOST_ACCEPTED_FAILURE_BOUNDARY'
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith('HOST_ACCEPTED_FAILURE_BOUNDARY')
    expect(controller.setContent).not.toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain(PROVIDER_FAILURE_ERROR_ITEM_SENTINEL)
  })

  it('preserves an accepted host boundary while reporting an aborted terminal', async () => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({
      text: 'HOST_ACCEPTED_RESTART_BOUNDARY',
      terminalReason: 'restart'
    })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber([acceptedFinalDeliveryBatch(accepted)], (event) => streamer.onSseEvent(event))

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: 'HOST_ACCEPTED_RESTART_BOUNDARY'
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith('HOST_ACCEPTED_RESTART_BOUNDARY')
    expect(controller.setContent).not.toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain(RESTART_ERROR_ITEM_SENTINEL)
  })

  it.each([
    ['sealed text', (batch: Record<string, any>) => {
      const item = batch.events[0].item as Record<string, unknown>
      item.text = 'TAMPERED_ACCEPTED_TEXT'
    }],
    ['private reasoning', (batch: Record<string, any>) => {
      batch.events[0].item.text = 'public<think>ACCEPTED_PRIVATE_REASONING</think>'
    }],
    ['terminal digest', (batch: Record<string, any>) => {
      batch.events.at(-1).acceptedFinalDigest = '1'.repeat(64)
    }],
    ['terminal reason', (batch: Record<string, any>) => {
      batch.events.at(-1).terminalReason = 'semantic_failure'
    }],
    ['public claim shape', (batch: Record<string, any>) => {
      batch.events[0].item.acceptedFinalView.claimCount = 1
    }],
    ['view digest', (batch: Record<string, any>) => {
      batch.events[0].item.acceptedFinalView.acceptedFinalDigest = '1'.repeat(64)
    }],
    ['item publication event id', (batch: Record<string, any>) => {
      batch.events[0].publicationEventId = '2'.repeat(64)
    }],
    ['terminal publication payload', (batch: Record<string, any>) => {
      batch.events.at(-1).publicationPayloadDigest = '3'.repeat(64)
    }],
    ['terminal sequence', (batch: Record<string, any>) => {
      batch.events.at(-1).seq = batch.events[0].seq
    }]
  ])('fails closed when accepted final %s is mismatched', async (_label, mutate) => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({
      text: 'ACCEPTED_CANDIDATE_SENTINEL',
      terminalReason: 'provider_failure'
    })
    const batch = acceptedFinalDeliveryBatch(accepted) as Record<string, any>
    mutate(batch)
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber([batch], (event) => streamer.onSseEvent(event))

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain('ACCEPTED_CANDIDATE_SENTINEL')
  })

  it('replaces an accepted final when authority is revoked while setContent is in flight', async () => {
    let releaseFirstSetContent: (() => void) | undefined
    const firstSetContent = new Promise<void>((resolve) => {
      releaseFirstSetContent = resolve
    })
    const setContent = vi.fn()
      .mockImplementationOnce(() => firstSetContent)
      .mockResolvedValueOnce(undefined)
    const controller: MarkdownStreamController = {
      append: vi.fn(async () => undefined),
      setContent,
      get messageId() {
        return 'om_stream_revoked'
      }
    }
    const bridge = {
      stream: vi.fn(async (_to: string, input: StreamInput): Promise<SendResult> => {
        await input.markdown(controller)
        return { messageId: 'om_stream_revoked' }
      })
    } as unknown as LarkChannel
    const accepted = acceptedFinalEvents({ text: 'ACCEPTED_BEFORE_REVOKE', terminalReason: 'success' })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const resultPromise = streamer.start({
      subscribe: makeSubscriber([acceptedFinalDeliveryBatch(accepted)], (event) => streamer.onSseEvent(event))
    })
    await vi.waitFor(() => expect(setContent).toHaveBeenCalledWith('ACCEPTED_BEFORE_REVOKE'))
    streamer.onSseEvent({
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId: 'thread_1',
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected',
      action: 'purge_case_projection',
      terminal: true
    })
    releaseFirstSetContent?.()

    await expect(resultPromise).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(setContent).toHaveBeenNthCalledWith(1, 'ACCEPTED_BEFORE_REVOKE')
    expect(setContent).toHaveBeenNthCalledWith(2, FEISHU_PUBLIC_PROJECTION_BOUNDARY)
  })

  it('replaces an accepted final when the frozen runtime generation changes in flight', async () => {
    let current = true
    let releaseFirstSetContent: (() => void) | undefined
    const firstSetContent = new Promise<void>((resolve) => {
      releaseFirstSetContent = resolve
    })
    const setContent = vi.fn()
      .mockImplementationOnce(() => firstSetContent)
      .mockResolvedValueOnce(undefined)
    const controller: MarkdownStreamController = {
      append: vi.fn(async () => undefined),
      setContent,
      get messageId() {
        return 'om_stream_generation_changed'
      }
    }
    const bridge = {
      stream: vi.fn(async (_to: string, input: StreamInput): Promise<SendResult> => {
        await input.markdown(controller)
        return { messageId: 'om_stream_generation_changed' }
      })
    } as unknown as LarkChannel
    const accepted = acceptedFinalEvents({ text: 'OLD_GENERATION_ACCEPTED_FINAL', terminalReason: 'success' })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(() => current),
      logger: vi.fn()
    })
    const resultPromise = streamer.start({
      subscribe: makeSubscriber([acceptedFinalDeliveryBatch(accepted)], (event) => streamer.onSseEvent(event))
    })

    await vi.waitFor(() => expect(setContent).toHaveBeenCalledWith('OLD_GENERATION_ACCEPTED_FINAL'))
    current = false
    releaseFirstSetContent?.()

    await expect(resultPromise).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(setContent).toHaveBeenNthCalledWith(2, FEISHU_PUBLIC_PROJECTION_BOUNDARY)
  })

  it('rejects an otherwise valid batch when no managed runtime generation is pinned', async () => {
    const { bridge, controller } = makeBridge()
    const accepted = acceptedFinalEvents({ text: 'UNPINNED_ACCEPTED_FINAL', terminalReason: 'success' })
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      logger: vi.fn()
    })

    await expect(streamer.start({
      subscribe: makeSubscriber([acceptedFinalDeliveryBatch(accepted)], (event) => streamer.onSseEvent(event))
    })).resolves.toMatchObject({ ok: false, finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY })
    expect(controller.setContent).toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain('UNPINNED_ACCEPTED_FINAL')
  })

  it('rejects legacy assistant-process items instead of treating them as an atomic final', async () => {
    const { bridge, controller } = makeBridge()
    const legacy = atomicAssistantFinal('LEGACY_PROCESS_SENTINEL')
    legacy.itemId = 'item_turn_1_assistant_process_1'
    const legacyItem = legacy.item as Record<string, unknown>
    legacyItem.id = 'item_turn_1_assistant_process_1'
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe = makeSubscriber([legacy, terminalEvent('turn_completed')], (event) => streamer.onSseEvent(event))

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(JSON.stringify((controller.setContent as ReturnType<typeof vi.fn>).mock.calls))
      .not.toContain('LEGACY_PROCESS_SENTINEL')
  })

  it('replaces a failed atomic setContent with the fixed boundary', async () => {
    const setContent = vi.fn()
      .mockRejectedValueOnce(new Error('rate_limited'))
      .mockResolvedValueOnce(undefined)
    const bridge = {
      stream: vi.fn(async (_to: string, input: StreamInput): Promise<SendResult> => {
        const controller: MarkdownStreamController = {
          append: vi.fn(async () => undefined),
          setContent,
          get messageId() {
            return 'om_stream_2'
          }
        }
        await input.markdown(controller)
        return { messageId: 'om_stream_2' }
      })
    } as unknown as LarkChannel
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const accepted = acceptedFinalEvents({ text: 'atomic-final', terminalReason: 'success' })
    const subscribe = makeSubscriber(
      [acceptedFinalDeliveryBatch(accepted)],
      (event) => streamer.onSseEvent(event)
    )

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(setContent).toHaveBeenNthCalledWith(1, 'atomic-final')
    expect(setContent).toHaveBeenNthCalledWith(2, FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(setContent).toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
  })

  it('handles authority revocation before the producer starts and emits no stale text', async () => {
    const { bridge, controller } = makeBridge()
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })
    const subscribe: SseSubscriber = () => {
      streamer.onSseEvent({
        schemaVersion: 1,
        kind: 'public_projection_revoked',
        threadId: 'thread_1',
        historyAuthority: 'case_boundary_only_v1',
        code: 'case_public_authority_rejected',
        action: 'purge_case_projection',
        terminal: true
      })
      return { close: vi.fn() }
    }

    await expect(streamer.start({ subscribe })).resolves.toMatchObject({
      ok: false,
      finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY
    })
    expect(controller.append).not.toHaveBeenCalled()
    expect(controller.setContent).toHaveBeenCalledWith(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
    expect(streamer.getAccumulatedText()).toBe('')
  })

  it('rejects start() when subscribe throws synchronously', async () => {
    const { bridge, controller } = makeBridge()
    const streamer = new FeishuStreamer({
      bridge,
      chatId: 'oc_chat_1',
      turnId: 'turn_1',
      threadId: 'thread_1',
      replyOptions: {},
      ...currentAuthorityOptions(),
      logger: vi.fn()
    })

    await expect(streamer.start({ subscribe: () => { throw new Error('sse_unavailable') } })).rejects.toThrow(
      'sse_unavailable'
    )
    expect(controller.append).not.toHaveBeenCalled()
  })
})
