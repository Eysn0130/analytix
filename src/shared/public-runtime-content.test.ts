import { describe, expect, it } from 'vitest'
import goCaseSourcePublicProjectionJSON from '../../packages/runtime/src/conformance/fixtures/go-case-source-public-projection-v1.json'
import {
  TYPED_LOCAL_DATA_SURFACE_KINDS_V1
} from '../../packages/runtime/src/contracts/typed-local-data-surface'
import {
  containsPrivateRuntimeDiagnosticContent,
  containsPrivateReasoningContent,
  isPublicSseIpcPayload,
  PublicRuntimeEventFilter,
  sanitizePublicAssistantText,
  sanitizePublicSerializedText,
  sanitizePublicRuntimeValue,
  stripASCIIControlCharacters
} from './public-runtime-content'
import { projectPublicRuntimeSseBlock } from './public-runtime-sse'

function closedTerminalCacheDiagnostics(): Record<string, unknown> {
  const complete = (value: number) => ({
    complete: true,
    knownObservationCount: 1,
    observationCount: 1,
    value
  })
  return {
    prefixHash: '1'.repeat(64),
    prefixChanged: false,
    prefixChangeReasons: [],
    toolSourceChanged: false,
    toolSourceChangeReasons: [],
    cacheTelemetrySupported: true,
    cacheTelemetryPresent: true,
    providerNativeCacheTelemetry: true,
    cacheBaselineObserved: false,
    cacheHitTokens: 0,
    cacheMissTokens: 0,
    cacheTelemetrySource: 'provider_usage',
    providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
    providerAttemptTelemetryValid: true,
    providerLogicalCallCount: 1,
    providerAttemptCount: 1,
    providerAttemptStatuses: {
      succeeded: 0,
      failed: 1,
      cancelled: 0,
      timedOut: 0,
      streamAborted: 0
    },
    providerAttemptInputTokens: complete(0),
    providerAttemptOutputTokens: complete(0),
    providerAttemptCacheHitTokens: complete(0),
    providerAttemptCacheMissTokens: complete(0),
    providerAttemptCacheRate: {
      known: false,
      numerator: 0,
      denominator: 0
    },
    contextEpochStateValid: true,
    contextEpoch: 1,
    contextEpochDigest: '2'.repeat(64),
    contextEpochRegistryDigest: '3'.repeat(64),
    contextEpochChangeReasons: [],
    contextEpochImpact: {
      stablePrefix: false,
      dynamicContext: true,
      turnTail: false,
      diagnosticsOnly: false
    }
  }
}

const acceptedFinalPublicViewCore = {
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

const acceptedFinalRecord = {
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
  publicView: acceptedFinalPublicViewCore,
  publicViewDigest: '7'.repeat(64),
  privateRecordDigest: 'f'.repeat(64),
  acceptedAt: '2026-07-11T01:02:03Z',
  authoritySignature: 'A'.repeat(86),
  recordDigest: '9'.repeat(64)
}

const acceptedFinalView = {
  schemaVersion: 3,
  acceptedFinalDigest: acceptedFinalRecord.recordDigest,
  publicationState: 'accepted',
  variant: acceptedFinalPublicViewCore.variant,
  terminalReason: acceptedFinalPublicViewCore.terminalReason,
  blockerCode: acceptedFinalPublicViewCore.blockerCode,
  coverageStatus: acceptedFinalPublicViewCore.coverageStatus,
  checkedScopeDigest: acceptedFinalPublicViewCore.checkedScopeDigest,
  missingScopeCount: acceptedFinalPublicViewCore.missingScopeCount,
  claimCount: acceptedFinalPublicViewCore.claimCount,
  claimTypes: acceptedFinalPublicViewCore.claimTypes,
  receiptMetadata: acceptedFinalPublicViewCore.receiptMetadata,
  noHitWording: acceptedFinalPublicViewCore.noHitWording,
  acceptedAt: acceptedFinalPublicViewCore.acceptedAt
}

function toolResultItem(output: unknown, extra: Record<string, unknown> = {}) {
  return {
    id: 'item-tool-result',
    turnId: 'turn-tool-result',
    threadId: 'thread-tool-result',
    role: 'tool',
    status: 'completed',
    createdAt: '2026-07-14T00:00:00.000Z',
    finishedAt: '2026-07-14T00:00:01.000Z',
    kind: 'tool_result',
    toolName: 'read',
    callId: 'call-read',
    toolKind: 'tool_call',
    output,
    isError: false,
    ...extra
  }
}

function toolCallItem(argumentsValue: unknown, extra: Record<string, unknown> = {}) {
  return {
    id: 'item-tool-call',
    turnId: 'turn-tool-call',
    threadId: 'thread-tool-call',
    role: 'tool',
    status: 'running',
    createdAt: '2026-07-14T00:00:00.000Z',
    kind: 'tool_call',
    toolName: 'read',
    callId: 'call-read',
    toolKind: 'tool_call',
    arguments: argumentsValue,
    ...extra
  }
}

describe('public runtime content', () => {
  it('preserves host question IDs with numeric digest runs in thread detail', () => {
    const inputId = `input_${'123456789012'}${'a'.repeat(52)}`
    const item = {
      kind: 'user_input', role: 'system', status: 'pending', inputId, prompt: 'Choose a path',
      questions: [{ id: `${inputId}_1`, header: 'Path', question: 'Choose a path', options: [] }]
    }
    const thread = { id: 'thread-gate', turns: [{ id: 'turn-gate', items: [item] }] }
    expect(sanitizePublicRuntimeValue(thread)).toEqual(thread)
    expect(sanitizePublicRuntimeValue({ ...thread, turns: [{ id: 'turn-gate', items: [{
      ...item, questions: [{ ...item.questions[0], question: 'account=6222020202020202020' }]
    }] }] })).toBeUndefined()
  })

  it('withholds marker-free raw provider, tool, and job diagnostic fields', () => {
    const sentinel = 'SOL_RAW_JOB_ERROR_SENTINEL_7F3C'
    const value = {
      child: { evidenceLedgerError: sentinel },
      details: {
        deliveryError: sentinel,
        autoContinueError: sentinel,
        outputPreview: sentinel
      }
    }

    expect(containsPrivateRuntimeDiagnosticContent(value)).toBe(true)
    expect(sanitizePublicRuntimeValue(value)).toBeUndefined()
    expect(containsPrivateRuntimeDiagnosticContent({
      failureCode: 'delivery_failed',
      reasonCode: 'parent_turn_not_latest',
      outputBytes: sentinel.length
    })).toBe(false)
  })

  it('does not erase a typed turn failure kind when the event carries code and message fields', () => {
    expect(sanitizePublicRuntimeValue({
      kind: 'turn_failed',
      seq: 3,
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-failed',
      turnId: 'turn-failed',
      code: 'provider_request_failed',
      message: 'safe host status'
    })).toEqual({
      kind: 'turn_failed',
      seq: 3,
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-failed',
      turnId: 'turn-failed',
      code: 'provider_request_failed',
      message: 'safe host status'
    })
  })

  it('removes ASCII controls without dropping Unicode text', () => {
    expect(stripASCIIControlCharacters('safe\u0000\u001f\u007f中文\n')).toBe('safe中文')
  })

  it.each(['think', 'analysis', 'reasoning'])(
    'withholds a whole assistant value for complete, unterminated, or orphaned %s markup',
    (tag) => {
      expect(sanitizePublicAssistantText(`before<${tag}>private</${tag}>after`)).toBe('')
      expect(sanitizePublicAssistantText(`before<${tag}>private`)).toBe('')
      expect(sanitizePublicAssistantText(`private</${tag}>after`)).toBe('')
    }
  )

  it('withholds partial reasoning tag prefixes across every provider tag family', () => {
    expect(sanitizePublicAssistantText('public</thi')).toBe('')
    expect(sanitizePublicAssistantText('public<anal')).toBe('')
    expect(sanitizePublicAssistantText('public</reas')).toBe('')
  })

  it('withholds restricted evidence and closed internal references from assistant text', () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const sourceRowRef = `srow1_${'b'.repeat(64)}`
    const canonicalEvidenceV3 = JSON.stringify({
      canonicalEvidence: {
        schemaVersion: 3,
        purpose: 'analytix.canonical-evidence/v3',
        facts: []
      }
    })

    expect(sanitizePublicAssistantText(`prefix ${canonicalEvidenceV3} suffix`)).toBe('')
    expect(sanitizePublicAssistantText(authorityRef)).toBe('')
    expect(sanitizePublicAssistantText(`prefix ${sourceRowRef} suffix`)).toBe('')
  })

  it('preserves clean assistant bytes without treating a different tag name as reasoning', () => {
    expect(sanitizePublicAssistantText('  public\nbytes  ')).toBe('  public\nbytes  ')
    expect(sanitizePublicAssistantText('<thinker>safe</thinker>')).toBe('<thinker>safe</thinker>')
    expect(sanitizePublicAssistantText('ordinary trailing <')).toBe('ordinary trailing <')
    expect(sanitizePublicAssistantText('acct:1 card:1')).toBe('acct:1 card:1')
    expect(sanitizePublicAssistantText('ab'.repeat(32))).toBe('ab'.repeat(32))
  })

  it('drops private kinds and fields recursively but keeps numeric usage', () => {
    const value = {
      kind: 'turn_completed',
      usage: { reasoningTokens: 12 },
      reasoning_content: 'PRIVATE_FIELD',
      turns: [{
        role: 'assistant',
        kind: 'assistant_text',
        text: '<think>PRIVATE_TAG</think>PUBLIC',
        nested: [{ kind: 'assistant_reasoning', text: 'PRIVATE_ITEM' }]
      }]
    }
    const sanitized = sanitizePublicRuntimeValue(value)
    expect(sanitized).toEqual({
      kind: 'turn_completed',
      usage: { reasoningTokens: 12 },
      turns: [{
        role: 'assistant',
        kind: 'assistant_text',
        text: '',
        nested: []
      }]
    })
    expect(containsPrivateReasoningContent(value)).toBe(true)
    expect(containsPrivateReasoningContent(sanitized)).toBe(false)
  })

  it('rejects type-confused reasoning metadata instead of treating its key as public authority', () => {
    const invalidValues = [
      { kind: 'usage', usage: { reasoningTokens: 'PRIVATE_REASONING_SENTINEL' } },
      { kind: 'usage', usage: { reasoningTokens: { value: 'PRIVATE_REASONING_SENTINEL' } } },
      { kind: 'usage', usage: { reasoningTokens: 1.5 } },
      { kind: 'usage', usage: { reasoningTokens: Number.MAX_SAFE_INTEGER + 1 } },
      { kind: 'turn_started', reasoningEffort: 'PRIVATE_REASONING_SENTINEL' },
      { kind: 'usage', reasoningDurationMs: 4 },
      { kind: 'compaction', reasoningExcluded: 'PRIVATE_REASONING_SENTINEL' },
      { kind: 'compaction', reasoningExclusionProof: `sha256:${'a'.repeat(64)}PRIVATE_REASONING_SENTINEL` }
    ]

    for (const value of invalidValues) {
      expect(sanitizePublicRuntimeValue(value)).toBeUndefined()
      expect(containsPrivateReasoningContent(value)).toBe(true)
    }
    expect(isPublicSseIpcPayload({
      streamId: 'stream-reasoning-smuggle',
      events: [{
        kind: 'usage', seq: 1, timestamp: '2026-07-18T00:00:00.000Z', threadId: 'thread-1',
        usage: {
          promptTokens: 1,
          completionTokens: 1,
          reasoningTokens: 'PRIVATE_REASONING_SENTINEL',
          totalTokens: 2,
          cacheHitRate: null,
          turns: 1
        }
      }]
    })).toBe(false)
  })

  it('preserves only typed bounded reasoning metadata', () => {
    const value = {
      kind: 'usage',
      reasoningEffort: 'high',
      usage: { reasoningTokens: 12 },
      reasoningContent: 'PRIVATE_REASONING_SENTINEL'
    }
    expect(sanitizePublicRuntimeValue(value)).toEqual({
      kind: 'usage',
      reasoningEffort: 'high',
      usage: { reasoningTokens: 12 }
    })
    expect(containsPrivateReasoningContent({ reasoningTokens: 12, reasoningEffort: 'high' })).toBe(false)
    expect(sanitizePublicRuntimeValue({ reasoningEffort: 'auto' })).toEqual({ reasoningEffort: 'auto' })
  })

  it('preserves the closed model reasoning capability but rejects content-shaped objects', () => {
    const capability = {
      supportedEfforts: ['off', 'high', 'max'],
      defaultEffort: 'high',
      requestProtocol: 'deepseek-chat-completions'
    }
    expect(sanitizePublicRuntimeValue({ reasoning: capability })).toEqual({ reasoning: capability })
    expect(containsPrivateReasoningContent({ reasoning: capability })).toBe(false)

    for (const reasoning of [
      { content: 'PRIVATE_REASONING_SENTINEL' },
      { supportedEfforts: ['high'], defaultEffort: 'high', requestProtocol: 'invalid' },
      'PRIVATE_REASONING_SENTINEL'
    ]) {
      expect(sanitizePublicRuntimeValue({ reasoning })).toBeUndefined()
      expect(containsPrivateReasoningContent({ reasoning })).toBe(true)
    }
  })

  it('migrates the exact legacy empty reasoning effort to an absent field', () => {
    expect(sanitizePublicRuntimeValue({
      id: 'thread-legacy',
      reasoningEffort: '',
      turns: [{ id: 'turn-legacy', reasoningEffort: '' }]
    })).toEqual({
      id: 'thread-legacy',
      turns: [{ id: 'turn-legacy' }]
    })
  })

  it('withholds internal general-terminal outbox authority recursively', () => {
    const sanitized = sanitizePublicRuntimeValue({
      id: 'thread-1',
      generalTerminalPublication: { commitId: 'GENERAL_OUTBOX_SENTINEL' },
      turns: [{
        id: 'turn-1',
        generalTerminalCASBinding: { bindingDigest: 'GENERAL_BINDING_SENTINEL' },
        items: [{
          kind: 'assistant_text',
          text: 'public answer',
          generalTerminalCASBinding: { bindingDigest: 'GENERAL_ITEM_BINDING_SENTINEL' }
        }]
      }],
      generalTerminalCommitId: 'GENERAL_EVENT_SENTINEL'
    })
    expect(sanitized).toEqual({
      id: 'thread-1',
      turns: [{
        id: 'turn-1',
        items: [{ kind: 'assistant_text', text: 'public answer' }]
      }]
    })
    expect(JSON.stringify(sanitized)).not.toContain('GENERAL_')
  })

  it('withholds neutral-wrapped private terminal contracts by semantic identity', () => {
    const sanitized = sanitizePublicRuntimeValue({
      kind: 'thread_snapshot',
      details: {
        archive: {
          schemaVersion: 'general-terminal-publication-archive.v1',
          purpose: 'analytix.general-terminal-publication-archive/v1',
          archiveDigest: 'NEUTRAL_ARCHIVE_DIGEST_SENTINEL'
        }
      }
    })
    expect(JSON.stringify(sanitized)).not.toMatch(
      /general-terminal-publication-archive|NEUTRAL_ARCHIVE_DIGEST_SENTINEL/
    )
  })

  it('withholds restricted evidence and source-exact values from every ordinary runtime projection', () => {
    const exactAccount = '0012-3456789012345678'
    const digest = 'a'.repeat(64)
    const privateValue = {
      kind: 'review_completed',
      review: {
        output: {
          schemaVersion: 2,
          purpose: 'analytix.source-field-binding/v2',
          sourceExactValue: exactAccount,
          sourceExactValueSha256: 'b'.repeat(64),
          bindingDigest: digest
        }
      }
    }
    expect(sanitizePublicRuntimeValue(privateValue)).toBeUndefined()
    expect(sanitizePublicSerializedText(
      `[runtime] detail: ${JSON.stringify(privateValue.review.output)}`
    )).toBe('')
    expect(new PublicRuntimeEventFilter().push(privateValue)).toBeNull()
    expect(isPublicSseIpcPayload({ streamId: 'stream-1', events: [privateValue] })).toBe(false)

    const publicValue = { kind: 'publication_metric', reportSha256: digest, datasetSnapshotId: 'snapshot-v2' }
    expect(sanitizePublicRuntimeValue(publicValue)).toEqual(publicValue)
  })

  it.each(TYPED_LOCAL_DATA_SURFACE_KINDS_V1)(
    'withholds PII-neutral %s responses from object serialized and SSE projections',
    (kind) => {
      const canary = `NEUTRAL_TYPED_LOCAL_CANARY_${kind.toUpperCase()}`
      const response = {
        schemaVersion: 1,
        kind,
        nested: { displayValue: canary }
      }
      const wrapped = { kind: 'runtime_progress', payload: { response } }
      const serialized = `prefix ${JSON.stringify(response)} suffix`

      expect(sanitizePublicSerializedText(canary)).toBe(canary)
      expect(sanitizePublicRuntimeValue(response)).toBeUndefined()
      expect(sanitizePublicRuntimeValue(wrapped)).toBeUndefined()
      expect(sanitizePublicSerializedText(serialized)).toBe('')
      expect(new PublicRuntimeEventFilter().push(wrapped)).toBeNull()
      expect(isPublicSseIpcPayload({ streamId: 'stream-typed-local', events: [wrapped] })).toBe(false)
    }
  )

  it('allows ordinary prose and incomplete objects that only mention typed-local family words', () => {
    const prose = 'Please discuss typed-local-data-surface/v1 and direct_source_preview naming.'
    expect(sanitizePublicSerializedText(prose)).toBe(prose)
    expect(sanitizePublicRuntimeValue({ kind: 'runtime_progress', message: prose })).toEqual({
      kind: 'runtime_progress',
      message: prose
    })
    expect(sanitizePublicRuntimeValue({ schemaVersion: 1, message: 'ordinary' })).toEqual({
      schemaVersion: 1,
      message: 'ordinary'
    })
    expect(sanitizePublicRuntimeValue({ kind: 'direct_source_preview', message: 'ordinary' })).toEqual({
      kind: 'direct_source_preview',
      message: 'ordinary'
    })
    for (const value of [
      { schemaVersion: '1', kind: 'direct_source_preview', message: 'ordinary' },
      { schemaVersion: 2, kind: 'direct_source_preview', message: 'ordinary' },
      { schemaVersion: 1, kind: 'unknown_preview', message: 'ordinary' }
    ]) {
      expect(sanitizePublicRuntimeValue(value)).toEqual(value)
    }
  })

  it('withholds embedded V3 JSON and internal authority refs from serialized value event and SSE projections', () => {
    const privateV3 = JSON.stringify({
      schemaVersion: 3,
      purpose: 'analytix.canonical-evidence/v3',
      piiClassification: 'none',
      facts: [],
      acceptedSlotSourceBindings: [{
        sourceRecordId: `srow1_${'a'.repeat(64)}`,
        sourceFileId: 'opaque-source-file',
        sourceRowNumber: 7,
        field: 'account',
        bindingDigest: 'b'.repeat(64)
      }],
      acceptedSlotSourceBindingSetDigest: 'c'.repeat(64)
    })
    const embeddedV3 = `prefix ${privateV3} suffix`
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const sourceRowRef = `srow1_${'b'.repeat(64)}`

    for (const privateText of [
      embeddedV3,
      authorityRef,
      `prefix ${authorityRef} suffix`,
      sourceRowRef,
      `prefix ${sourceRowRef} suffix`
    ]) {
      const event = { kind: 'runtime_progress', message: privateText }
      expect(sanitizePublicSerializedText(privateText)).toBe('')
      expect(sanitizePublicRuntimeValue(privateText)).toBeUndefined()
      expect(sanitizePublicRuntimeValue(event)).toBeUndefined()
      expect(new PublicRuntimeEventFilter().push(event)).toBeNull()
      expect(isPublicSseIpcPayload({ streamId: 'stream-private', events: [event] })).toBe(false)
    }

    const ordinaryJSON = 'prefix {"kind":"ordinary","message":"literal } and ]"} suffix'
    const publicValue = {
      kind: 'runtime_progress',
      message: ordinaryJSON,
      accountAlias: 'acct:1',
      cardAlias: 'card:1'
    }
    expect(sanitizePublicSerializedText(ordinaryJSON)).toBe(ordinaryJSON)
    expect(sanitizePublicSerializedText('ab'.repeat(32))).toBe('ab'.repeat(32))
    expect(sanitizePublicRuntimeValue(publicValue)).toEqual(publicValue)
    expect(new PublicRuntimeEventFilter().push(publicValue)).toEqual(publicValue)
    expect(isPublicSseIpcPayload({ streamId: 'stream-public', events: [publicValue] })).toBe(true)
    expect(sanitizePublicRuntimeValue({
      kind: 'error',
      code: 'runtime_failed',
      message: `failed near ${authorityRef}`
    })).toEqual({ code: 'runtime_error', message: 'Runtime request failed.' })
  })

  it('rejects standalone V3 items and item events without a sealed Batch V2 group', () => {
    const validItem = {
      id: 'item-case-final',
      turnId: acceptedFinalRecord.turnId,
      threadId: acceptedFinalRecord.threadId,
      kind: 'assistant_text',
      role: 'assistant',
      status: 'completed',
      createdAt: acceptedFinalRecord.acceptedAt,
      finishedAt: acceptedFinalRecord.acceptedAt,
      text: 'host boundary',
      acceptedFinalView
    }
    expect(sanitizePublicRuntimeValue(validItem)).toBeUndefined()

    const invalidThread = sanitizePublicRuntimeValue({
      kind: 'thread_snapshot',
      acceptedFinal: acceptedFinalRecord,
      acceptedFinalView: { ...acceptedFinalView, rawReceiptId: 'receipt-private' },
      items: [validItem, {
        ...validItem,
        acceptedFinalView: { ...acceptedFinalView, contextDigest: '7'.repeat(64) }
      }]
    })
    expect(invalidThread).toBeUndefined()
    expect(JSON.stringify(invalidThread) ?? '').not.toContain('receipt-private')

    const filter = new PublicRuntimeEventFilter()
    const event = {
      kind: 'item_completed',
      seq: 3,
      timestamp: acceptedFinalRecord.acceptedAt,
      threadId: acceptedFinalRecord.threadId,
      turnId: acceptedFinalRecord.turnId,
      itemId: validItem.id,
      item: validItem,
      acceptedFinalDigest: acceptedFinalRecord.recordDigest,
      publicationCommitId: acceptedFinalRecord.recordDigest,
      publicationEventId: '7'.repeat(64),
      publicationSlot: 'assistant-final',
      publicationPayloadDigest: '6'.repeat(64)
    }
    expect(filter.push({
      ...event,
      item: { ...validItem, acceptedFinalView: { ...acceptedFinalView, providerSaysSafe: true } }
    })).toBeNull()
    expect(filter.push({
      ...event,
      kind: 'assistant_text_delta'
    })).toBeNull()
    expect(filter.push(event)).toBeNull()
    expect(isPublicSseIpcPayload({ streamId: 'stream-standalone-v3', events: [event] })).toBe(false)
  })

  it('preserves a sealed four-slot accepted-final error item byte-for-byte', () => {
    const record = structuredClone(acceptedFinalRecord) as Record<string, any>
    record.terminalReason = 'approval_denied'
    record.publicView.terminalReason = 'approval_denied'
    const view = {
      schemaVersion: 3,
      acceptedFinalDigest: record.recordDigest,
      publicationState: 'accepted',
      variant: record.publicView.variant,
      terminalReason: record.publicView.terminalReason,
      blockerCode: record.publicView.blockerCode,
      coverageStatus: record.publicView.coverageStatus,
      checkedScopeDigest: record.publicView.checkedScopeDigest,
      missingScopeCount: record.publicView.missingScopeCount,
      claimCount: record.publicView.claimCount,
      claimTypes: record.publicView.claimTypes,
      receiptMetadata: record.publicView.receiptMetadata,
      noHitWording: record.publicView.noHitWording,
      acceptedAt: record.publicView.acceptedAt
    }
    const timestamp = record.acceptedAt as string
    const item = {
      id: 'item-case-final', turnId: record.turnId, threadId: record.threadId,
      role: 'assistant', status: 'completed', createdAt: timestamp, finishedAt: timestamp,
      kind: 'assistant_text', text: 'host boundary', acceptedFinalView: view
    }
    const errorItem = {
      id: `item_${record.turnId}_case_terminal`, turnId: record.turnId, threadId: record.threadId,
      role: 'system', status: 'completed', createdAt: timestamp, finishedAt: timestamp,
      kind: 'error', code: 'case_terminal_approval_denied',
      message: '案件分析已结束；仅发布通过宿主证据门的固定边界答复。',
      severity: 'warning', acceptedFinalDigest: record.recordDigest
    }
    const common = {
      timestamp, threadId: record.threadId, turnId: record.turnId,
      acceptedFinalDigest: record.recordDigest, publicationCommitId: record.recordDigest
    }
    const events = [
      {
        ...common, kind: 'item_completed', seq: 1, itemId: item.id, item,
        publicationEventId: '1'.repeat(64), publicationSlot: 'assistant-final',
        publicationPayloadDigest: '2'.repeat(64)
      },
      {
        ...common, kind: 'item_completed', seq: 2, itemId: errorItem.id, item: errorItem,
        publicationEventId: '3'.repeat(64), publicationSlot: 'terminal-error-item',
        publicationPayloadDigest: '4'.repeat(64)
      },
      {
        ...common, kind: 'usage', seq: 3, model: '', effort: 'auto',
        usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0, cacheHitRate: null, turns: 0 },
        cacheDiagnostics: closedTerminalCacheDiagnostics(), usageFinalStatus: 'completed',
        publicationEventId: '5'.repeat(64), publicationSlot: 'usage',
        publicationPayloadDigest: '6'.repeat(64)
      },
      {
        ...common, kind: 'turn_completed', seq: 4, status: 'completed', terminalReason: 'approval_denied',
        code: errorItem.code, itemId: errorItem.id,
        publicationEventId: '7'.repeat(64), publicationSlot: 'terminal',
        publicationPayloadDigest: '8'.repeat(64)
      }
    ]
    const batch = {
      schemaVersion: 2, purpose: 'analytix.accepted-final-delivery-batch/v2',
      kind: 'accepted_final_batch', batchId: '9'.repeat(64), threadId: record.threadId,
      turnId: record.turnId, seq: 4, firstSeq: 1, lastSeq: 4, timestamp,
      publicationCommitId: record.recordDigest, eventManifestDigest: 'a'.repeat(64),
      publicationAuthority: {
        schemaVersion: 'accepted-final-delivery-seal.v1',
        purpose: 'analytix.accepted-final-delivery-seal/v1', sealId: 'b'.repeat(64),
        threadId: record.threadId, turnId: record.turnId, publicationCommitId: record.recordDigest,
        acceptedFinalDispositionDigest: 'c'.repeat(64), terminalDispositionId: 'd'.repeat(64),
        eventManifestDigest: 'a'.repeat(64), sequencedEventsDigest: 'e'.repeat(64),
        batchId: '9'.repeat(64), firstSeq: 1, lastSeq: 4, timestamp,
        authorityAlgorithm: 'Ed25519', authorityKeyId: record.authorityKeyId,
        authorityPublicKey: record.authorityPublicKey, authoritySignature: record.authoritySignature
      },
      events
    }

    const standalone = sanitizePublicRuntimeValue(errorItem) as Record<string, unknown>
    expect(standalone).not.toEqual(errorItem)
    expect(standalone.acceptedFinalDigest).toBeUndefined()

    const admitted = new PublicRuntimeEventFilter().push(batch)
    expect(admitted).toBe(batch)
    expect(projectPublicRuntimeSseBlock(
      `id: 4\nevent: accepted_final_batch\ndata: ${JSON.stringify(batch)}`,
      record.threadId,
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 4, event: batch })
    expect((admitted?.events as Array<Record<string, any>>)[1].item).toEqual(errorItem)
    expect(JSON.stringify(admitted)).toContain('案件分析已结束；仅发布通过宿主证据门的固定边界答复。')
    expect(JSON.stringify(admitted)).toContain(record.recordDigest)

    for (const effort of ['', ' high ', 'HIGH', 'SOL_PRIVATE_REASONING_SENTINEL_7F3C']) {
      const invalidEffortBatch = structuredClone(batch) as Record<string, any>
      invalidEffortBatch.events[2].effort = effort
      expect(new PublicRuntimeEventFilter().push(invalidEffortBatch)).toBeNull()
    }

    for (const unsafeText of [
      'Authorization: Bearer opaque-final-secret-123',
      'account=6222020202020202020'
    ]) {
      const unsafeBatch = structuredClone(batch) as Record<string, any>
      unsafeBatch.events[0].item.text = unsafeText
      expect(new PublicRuntimeEventFilter().push(unsafeBatch)).toBeNull()
    }

    for (const mutateDiagnostics of [
      (diagnostics: Record<string, any>) => { diagnostics.providerAttemptCount = 2 },
      (diagnostics: Record<string, any>) => { diagnostics.providerAttemptCacheRate.known = 'false' },
      (diagnostics: Record<string, any>) => { diagnostics.rawProviderPayload = 'private' }
    ]) {
      const invalidDiagnosticsBatch = structuredClone(batch) as Record<string, any>
      mutateDiagnostics(invalidDiagnosticsBatch.events[2].cacheDiagnostics)
      expect(projectPublicRuntimeSseBlock(
        `id: 4\nevent: accepted_final_batch\ndata: ${JSON.stringify(invalidDiagnosticsBatch)}`,
        record.threadId,
        new PublicRuntimeEventFilter()
      )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    }
  })

  it('rejects detached private accepted-final strong markers recursively while retaining generic metadata names', () => {
    const strongMarkers = [
      'acceptedFinal',
      'factFinalWitnessAdmission',
      'publicationSnapshotProof',
      'publicationSnapshotProofDigest'
    ] as const
    for (const marker of strongMarkers) {
      const event = {
        kind: 'runtime_status',
        seq: 1,
        nested: { detached: { [marker]: 'PRIVATE_ACCEPTED_FINAL_CANARY' } }
      }
      expect(sanitizePublicRuntimeValue(event)).toBeUndefined()
      expect(new PublicRuntimeEventFilter().push(event)).toBeNull()
      expect(isPublicSseIpcPayload({ streamId: `stream-${marker}`, events: [event] })).toBe(false)
    }

    const generic = {
      kind: 'runtime_status',
      seq: 1,
      nested: {
        envelope: { state: 'closed' },
        registryHead: { state: 'healthy' },
        publicationIntent: { state: 'queued' },
        storeDigest: 'a'.repeat(64)
      }
    }
    expect(sanitizePublicRuntimeValue(generic)).toEqual(generic)
    expect(new PublicRuntimeEventFilter().push(generic)).toEqual(generic)
    expect(isPublicSseIpcPayload({ streamId: 'stream-generic-metadata', events: [generic] })).toBe(true)
  })

  it('withholds every assistant draft delta instead of incrementally repairing it', () => {
    const filter = new PublicRuntimeEventFilter()
    expect(filter.push({ kind: 'assistant_text_delta', turnId: 't', itemId: 'i', text: 'a<thi' }))
      .toBeNull()
    expect(filter.push({ kind: 'assistant_text_delta', turnId: 't', itemId: 'i', text: 'nk>PRIVATE' }))
      .toBeNull()
    expect(filter.push({ kind: 'assistant_text_delta', turnId: 't', itemId: 'i', text: '</think>b' }))
      .toBeNull()
    expect(filter.push({ kind: 'assistant_text_delta', turnId: 't', itemId: 'i', text: 'clean draft' }))
      .toBeNull()
    expect(filter.push({ kind: 'assistant_reasoning_delta', turnId: 't', text: 'PRIVATE' }))
      .toBeNull()
  })

  it.each([
    ['item.text', (text: string) => ({ item: { id: 'i', text } })],
    ['text', (text: string) => ({ text })],
    ['content', (text: string) => ({ content: text })],
    ['delta string', (text: string) => ({ delta: text })],
    ['delta.text', (text: string) => ({ delta: { text } })],
    ['delta.content', (text: string) => ({ delta: { content: text } })]
  ])('withholds assistant draft deltas from %s event payloads', (_label, shape) => {
    const filter = new PublicRuntimeEventFilter()
    const first = filter.push({
      kind: 'assistant_text_delta',
      turnId: 'turn-shape',
      itemId: 'item-shape',
      ...shape('public<thi')
    })
    const hidden = filter.push({
      kind: 'assistant_text_delta',
      turnId: 'turn-shape',
      itemId: 'item-shape',
      ...shape('nk>PRIVATE')
    })
    const last = filter.push({
      kind: 'assistant_text_delta',
      turnId: 'turn-shape',
      itemId: 'item-shape',
      ...shape('</think>safe')
    })
    expect(first).toBeNull()
    expect(hidden).toBeNull()
    expect(last).toBeNull()
  })

  it('withholds deltas even when carrier shape and identifier casing change', () => {
    const filter = new PublicRuntimeEventFilter()
    expect(filter.push({
      kind: 'assistant_text_delta',
      turn_id: 'turn-switch',
      item_id: 'item-switch',
      content: 'public<thi'
    })).toBeNull()
    expect(filter.push({
      kind: 'assistant_text_delta',
      turnId: 'turn-switch',
      itemId: 'item-switch',
      delta: { text: 'nk>PRIVATE' }
    })).toBeNull()
    expect(filter.push({
      kind: 'assistant_text_delta',
      item: { id: 'item-switch', turnId: 'turn-switch', text: '</think>safe' }
    })).toBeNull()
  })

  it('projects runtime errors to a fixed message and whitelisted diagnostics', () => {
    const sanitized = sanitizePublicRuntimeValue({
      kind: 'error',
      code: 'provider_request_failed',
      message: '<think>PRIVATE_REASONING</think>raw provider body',
      details: {
        providerId: 'deepseek',
        status: 500,
        retryable: false,
        baseUrl: 'https://secret.invalid',
        requestBody: 'PRIVATE_REQUEST',
        responseBody: 'PRIVATE_RESPONSE'
      }
    })
    expect(sanitized).toEqual({
      kind: 'error',
      code: 'provider_request_failed',
      message: 'Runtime request failed (provider_request_failed).',
      details: { providerId: 'deepseek', status: 500, retryable: false }
    })
    expect(JSON.stringify(sanitized)).not.toMatch(/PRIVATE|secret\.invalid|requestBody|responseBody/)
  })

  it('preserves closed unadvertised-tool hashes and enums without retaining the raw name', () => {
    const diagnostic = {
      rejectedToolNormalizedNameSha256: '8'.repeat(64),
      rejectedToolCategory: 'known_alias_not_advertised',
      promptRoute: 'tool_agent',
      loopStep: 1,
      advertisedToolCount: 7,
      advertisedToolManifestHash: '9'.repeat(64),
      advertisedNameSetSortedHash: 'a'.repeat(64),
      providerRequestToolManifestHash: 'b'.repeat(64),
      runToolStepManifestHash: 'b'.repeat(64),
      providerRequestRunToolStepManifestSame: true
    }
    const sanitized = sanitizePublicRuntimeValue({
      code: 'tool_not_advertised',
      message: 'raw host envelope',
      toolName: 'read_file',
      details: diagnostic
    })
    expect(sanitized).toEqual({
      code: 'tool_not_advertised',
      message: 'Runtime request failed (tool_not_advertised).',
      details: diagnostic
    })
    expect(JSON.stringify(sanitized)).not.toContain('read_file')
  })

  it.each([
    ['wrong code', { code: 'turn_failed' }],
    ['extra key', { details: { extra: 'hostile' } }],
    ['bad category', { details: { rejectedToolCategory: 'provider_supplied' } }],
    ['bad digest', { details: { advertisedToolManifestHash: 'Authorization: Bearer secret-token' } }],
    ['mismatched relation', {
      details: {
        runToolStepManifestHash: 'c'.repeat(64),
        providerRequestRunToolStepManifestSame: true
      }
    }]
  ])('drops malformed unadvertised-tool diagnostics: %s', (_name, override) => {
    const diagnostic = {
      rejectedToolNormalizedNameSha256: '8'.repeat(64),
      rejectedToolCategory: 'known_alias_not_advertised',
      promptRoute: 'tool_agent',
      loopStep: 1,
      advertisedToolCount: 7,
      advertisedToolManifestHash: '9'.repeat(64),
      advertisedNameSetSortedHash: 'a'.repeat(64),
      providerRequestToolManifestHash: 'b'.repeat(64),
      runToolStepManifestHash: 'b'.repeat(64),
      providerRequestRunToolStepManifestSame: true,
      ...('details' in override ? override.details : {})
    }
    const code = 'code' in override ? override.code : 'tool_not_advertised'
    const sanitized = sanitizePublicRuntimeValue({
      code,
      message: 'raw host envelope',
      details: diagnostic
    })
    expect(sanitized).toEqual({
      code,
      message: `Runtime request failed (${code}).`
    })
    expect(JSON.stringify(sanitized)).not.toMatch(/secret-token|hostile|provider_supplied/)
  })

  it('drops fallback diagnostic metadata containing secrets', () => {
    expect(sanitizePublicRuntimeValue({
      kind: 'error',
      code: 'provider_request_failed',
      details: { providerId: 'Authorization: Bearer secret-token', status: 500 }
    })).toEqual({
      kind: 'error',
      code: 'runtime_error',
      message: 'Runtime request failed.'
    })
  })

  it.each([
    ['secret-shaped', 'sk-abcdefgh'],
    ['PII-shaped', '6222020202020202020']
  ])('fails closed on %s runtime error codes', (_name, code) => {
    expect(sanitizePublicRuntimeValue({
      kind: 'error',
      code,
      message: 'raw host envelope'
    })).toEqual({
      kind: 'error',
      code: 'runtime_error',
      message: 'Runtime request failed.'
    })
  })

  it('does not reinterpret user-authored think text as provider reasoning', () => {
    const value = {
      role: 'user',
      kind: 'user_message',
      text: '请解释字面量 <think>example</think>'
    }
    expect(sanitizePublicRuntimeValue(value)).toEqual(value)
  })

  it('fails closed on unprojected secret and PII values in ordinary public content', () => {
    for (const value of [
      'account=6222020202020202020',
      { kind: 'thread_updated', title: 'Authorization: Basic dXNlcjpwYXNz' },
      { kind: 'user_message', text: 'account=6222020202020202020' },
      { kind: 'contact_status', email: 'victim@example.com' },
      { kind: 'subject_status', personName: '张三' },
      { kind: 'location_status', address: '北京市朝阳区建国路' }
    ]) {
      expect(sanitizePublicRuntimeValue(value)).toBeUndefined()
    }
  })

  it('filters private reasoning from every non-user runtime content carrier', () => {
    const value = {
      kind: 'thread_summary',
      compaction: {
        kind: 'compaction',
        summary: '<think>PRIVATE_COMPACTION</think>public summary',
        pinnedConstraints: ['reasoning: "PRIVATE_PIN"', 'public constraint']
      },
      task: {
        output: '<think>PRIVATE_OUTPUT</think>public output',
        outputSnippet: 'thinking = "PRIVATE_SNIPPET"\npublic snippet'
      },
      tool: toolResultItem({ arbitraryColumn: '<think>PRIVATE_TOOL</think>public value' }),
      review: { kind: 'review', reviewText: '<think>PRIVATE_REVIEW</think>public review' }
    }
    const sanitized = sanitizePublicRuntimeValue(value)
    expect(JSON.stringify(sanitized)).not.toMatch(
      /PRIVATE_(?:COMPACTION|PIN|OUTPUT|SNIPPET|TOOL|REVIEW)|<\/?think|\breasoning\s*[:=]|\bthinking\s*[:=]/i
    )
    expect(sanitized).toMatchObject({
      compaction: { summary: '', pinnedConstraints: ['', 'public constraint'] },
      task: { output: '', outputSnippet: '' },
      tool: {
        id: 'item-tool-result',
        turnId: 'turn-tool-result',
        threadId: 'thread-tool-result',
        role: 'tool',
        status: 'completed',
        createdAt: '2026-07-14T00:00:00.000Z',
        finishedAt: '2026-07-14T00:00:01.000Z',
        kind: 'tool_result',
        toolName: 'read',
        callId: 'call-read',
        toolKind: 'tool_call',
        isError: false,
        output: {
          schemaVersion: 1,
          projectionKind: 'withheld',
          disclosure: 'metadata_only',
          status: 'unknown',
          code: 'legacy_output_withheld',
          messageKey: 'legacy_output_withheld',
          privatePayloadWithheld: true,
          factAnswerAllowed: false,
          evidenceAuthority: false
        }
      },
      review: { reviewText: '' }
    })
    expect(containsPrivateReasoningContent(value)).toBe(true)
    expect(containsPrivateReasoningContent(sanitized)).toBe(false)
  })

  it('preserves only strict public tool projections and withholds hostile raw output', () => {
    const projection = {
      schemaVersion: 1,
      projectionKind: 'host_status',
      disclosure: 'metadata_only',
      status: 'completed',
      code: 'tool_completed',
      messageKey: 'tool_completed',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    }
    expect(sanitizePublicRuntimeValue(toolResultItem(projection)))
      .toEqual(toolResultItem(projection))

    const sentinel = 'PRIVATE_TOOL_RESULT_SENTINEL'
    const sanitized = sanitizePublicRuntimeValue({
      kind: 'item_completed',
      item: toolResultItem({
          content: sentinel,
          dataUrl: `data:image/png;base64,${sentinel}`,
          previewUrl: `http://127.0.0.1:4173/${sentinel}`,
          attachments: [{ id: 'att-hostile', localFilePath: `/tmp/${sentinel}` }],
          citations: [{ sourceId: sentinel }],
          safeToAnswer: true
        }, {
          summary: sentinel,
          dataUrl: `data:image/png;base64,${sentinel}`,
          details: { account: sentinel }
        })
    })
    expect(JSON.stringify(sanitized)).not.toContain(sentinel)
    expect(sanitized).toMatchObject({
      item: {
        output: {
          projectionKind: 'withheld',
          messageKey: 'legacy_output_withheld',
          privatePayloadWithheld: true,
          factAnswerAllowed: false,
          evidenceAuthority: false
        }
      }
    })
  })

  it('preserves the exact Go case-source projections instead of legacy withholding', () => {
    const fixture = goCaseSourcePublicProjectionJSON as {
      projections: Record<'completed' | 'failed', Record<string, unknown>>
    }
    const completed = toolResultItem(fixture.projections.completed)
    const failed = toolResultItem(fixture.projections.failed, { status: 'failed', isError: true })
    expect(sanitizePublicRuntimeValue(completed)).toEqual(completed)
    expect(sanitizePublicRuntimeValue(failed)).toEqual(failed)
    expect(JSON.stringify([completed, failed])).not.toContain('legacy_output_withheld')

    const proofDigest = 'a'.repeat(64)
    const privateDurable = toolResultItem(fixture.projections.completed, {
      contextDigest: 'b'.repeat(64),
      contextEpoch: 7,
      executionGrantId: 'c'.repeat(64),
      caseSourceBindingProof: {
        version: 1,
        purpose: 'analytix.case-source-result-binding/v1',
        digest: proofDigest
      }
    })
    const failedClosed = sanitizePublicRuntimeValue(privateDurable)
    const failedClosedBody = JSON.stringify([failedClosed])
    expect(failedClosedBody).not.toMatch(/caseSourceBindingProof|analytix\.case-source-result-binding|contextDigest|contextEpoch|executionGrantId/)
    expect(failedClosedBody).not.toContain(proofDigest)
    expect(failedClosed).toBeUndefined()
  })

  it('withholds provider tool-call arguments across the public tree and SSE boundary', () => {
    const sentinel = 'PRIVATE_TOOL_ARGUMENT_SENTINEL_6222020202020202020'
    const rawEvent = {
      kind: 'item_created',
      item: toolCallItem({ account: sentinel, path: `/private/${sentinel}` }, {
        summary: sentinel,
        executionGrant: { argsHash: sentinel }
      })
    }
    const sanitized = sanitizePublicRuntimeValue(rawEvent)
    expect(JSON.stringify(sanitized)).not.toContain(sentinel)
    expect(sanitized).toMatchObject({
      item: {
        kind: 'tool_call',
        arguments: {
          schemaVersion: 1,
          projectionKind: 'withheld',
          disclosure: 'metadata_only',
          messageKey: 'tool_arguments_withheld',
          privatePayloadWithheld: true,
          factAnswerAllowed: false,
          evidenceAuthority: false
        }
      }
    })
    expect(isPublicSseIpcPayload({ streamId: 'stream-tool-call', events: [sanitized] })).toBe(true)
    expect(isPublicSseIpcPayload({ streamId: 'stream-tool-call', events: [rawEvent] })).toBe(false)
  })

  it('requires strict tool-result items at the preload SSE envelope boundary', () => {
    const projection = {
      schemaVersion: 1,
      projectionKind: 'host_status',
      disclosure: 'metadata_only',
      status: 'completed',
      code: 'tool_completed',
      messageKey: 'tool_completed',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    }
    expect(isPublicSseIpcPayload({
      streamId: 'stream-tool-result',
      events: [{ kind: 'item_completed', item: toolResultItem(projection) }]
    })).toBe(true)
    expect(isPublicSseIpcPayload({
      streamId: 'stream-tool-result',
      events: [{
        kind: 'item_completed',
        item: toolResultItem({ content: 'PRIVATE_PRELOAD_TOOL_SENTINEL' }, {
          summary: 'PRIVATE_PRELOAD_TOOL_SENTINEL'
        })
      }]
    })).toBe(false)
  })

  it('filters top-level tool output strings but preserves explicit user literals', () => {
    expect(sanitizePublicRuntimeValue('<think>PRIVATE</think>PUBLIC')).toBe('')
    expect(sanitizePublicRuntimeValue({
      kind: 'user_message',
      text: '<think>literal user text</think>',
      attachments: [{ name: '<think>literal filename</think>' }]
    })).toEqual({
      kind: 'user_message',
      text: '<think>literal user text</think>',
      attachments: [{ name: '<think>literal filename</think>' }]
    })
  })

  it('withholds serialized values containing private reasoning fields', () => {
    expect(sanitizePublicSerializedText('public\nreasoning_content: "PRIVATE\nFIELD"\nafter'))
      .toBe('')
    expect(sanitizePublicSerializedText('public\nassistant_reasoning = "PRIVATE\nstill private'))
      .toBe('')
    expect(sanitizePublicSerializedText('public\nreasoning: "PRIVATE"\nafter')).toBe('')
    expect(sanitizePublicSerializedText('public\nthinking = PRIVATE\nafter')).toBe('')
    expect(sanitizePublicSerializedText('  public\nbytes  ')).toBe('  public\nbytes  ')
  })
})
