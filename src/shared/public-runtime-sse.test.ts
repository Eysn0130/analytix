import { describe, expect, it } from 'vitest'
import { GeneralTerminalDeliveryBatchV1Schema, generalTerminalFailureMessageV1 } from '../../packages/runtime/src/contracts/events'
import { isPublicSseIpcPayload, PublicRuntimeEventFilter } from './public-runtime-content'
import {
  isClosedPublicRuntimeSseEvent,
  isStrictPublicRuntimeSseEndPayload,
  isStrictPublicRuntimeSseErrorPayload,
  isStrictPublicRuntimeSseIpcPayload,
  projectPublicRuntimeSseBlock,
  takePublicRuntimeSseBlock
} from './public-runtime-sse'

function frame(id: string, event: string, data: Record<string, unknown>): string {
  return `id: ${id}\nevent: ${event}\ndata: ${JSON.stringify(data)}`
}

function heartbeat(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    seq: 7,
    kind: 'heartbeat',
    timestamp: '2026-07-14T00:00:00.000Z',
    threadId: 'thread-strict',
    ...overrides
  }
}

function projectionRevoked(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    schemaVersion: 1,
    kind: 'public_projection_revoked',
    threadId: 'thread-strict',
    historyAuthority: 'case_boundary_only_v1',
    code: 'case_public_authority_rejected',
    action: 'purge_case_projection',
    terminal: true,
    ...overrides
  }
}

function autoresearchAudit(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    seq: 9,
    kind: 'autoresearch_state_audit',
    timestamp: '2026-07-20T00:00:00Z',
    threadId: 'thread-strict',
    turnId: 'turn-strict',
    schemaVersion: 1,
    changeId: 'autoresearch-state-audit',
    runtimeContract: 'analytix-go-runtime',
    upstreamSource: 'reasonix-absorbed',
    goalMode: 'research',
    fileCount: 5,
    requirementCount: 2,
    completedRequirementCount: 1,
    staleRequirementCount: 1,
    staleDirectionCount: 1,
    complete: false,
    pivotRequired: true,
    result: 'pivot_required',
    unknownRequirementAccepted: false,
    findingsWrittenForUnknownRequirement: false,
    writesReasonixFile: false,
    writesAgentsFile: false,
    stablePrefixContainsState: false,
    toolSchemaContainsState: false,
    topLevelAutoResearchRouteExposed: false,
    usesReasonixPublicProtocol: false,
    usesReasonixConfigRoot: false,
    changesRendererContract: false,
    changesProductIdentity: false,
    ...overrides
  }
}

function generalTerminalBatch(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const timestamp = '2026-07-20T03:00:00Z'
  const events = [
    {
      kind: 'item_completed', seq: 11, timestamp, threadId: 'thread-strict', turnId: 'turn-general',
      itemId: 'item-general',
      item: {
        id: 'item-general', turnId: 'turn-general', threadId: 'thread-strict', role: 'assistant',
        status: 'completed', createdAt: timestamp, finishedAt: timestamp,
        kind: 'assistant_text',
        text: '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。'
      }
    },
    {
      kind: 'usage', seq: 12, timestamp, threadId: 'thread-strict', turnId: 'turn-general',
      model: 'gpt-5',
      usage: {
        promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
        cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
        cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
        priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0, turns: 1
      },
      cacheDiagnostics: {},
      usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: 13, timestamp, threadId: 'thread-strict', turnId: 'turn-general',
      status: 'completed', terminalReason: 'success'
    }
  ]
  return {
    schemaVersion: 1,
    purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch',
    batchDigest: 'a'.repeat(64),
    threadId: 'thread-strict',
    turnId: 'turn-general',
    seq: 13,
    firstSeq: 11,
    lastSeq: 13,
    timestamp,
    generalTerminalCommitId: 'b'.repeat(64),
    generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: 'c'.repeat(64),
    eventManifestDigest: 'd'.repeat(64),
    projectedEventsDigest: 'e'.repeat(64),
    transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    events,
    eventManifest: [
      { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
    ],
    ...overrides
  }
}

function typedGeneralTerminalBatch(
  text = '已完成代码修改并通过相关测试。'
): Record<string, unknown> {
  const batch = generalTerminalBatch()
  const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
  item.text = text
  item.ordinaryResult = {
    schemaVersion: 1,
    purpose: 'analytix.ordinary-result/v1',
    projectionVersion: 'analytix.ordinary-output-projection/v1',
    logicalEffect: 'ordinary',
    ordinaryWork: true,
    candidateOrigin: 'provider_ordinary_only',
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    text,
    textSha256: '6'.repeat(64),
    resultDigest: '7'.repeat(64)
  }
  return batch
}

function toolNotAdvertisedDiagnostic(): Record<string, unknown> {
  return {
    rejectedToolNormalizedNameSha256: '8'.repeat(63) + 'a',
    rejectedToolCategory: 'known_builtin_not_advertised',
    promptRoute: 'tool_agent',
    loopStep: 1,
    advertisedToolCount: 7,
    advertisedToolManifestHash: '9'.repeat(63) + 'b',
    advertisedNameSetSortedHash: 'a'.repeat(64),
    providerRequestToolManifestHash: 'b'.repeat(64),
    runToolStepManifestHash: 'b'.repeat(64),
    providerRequestRunToolStepManifestSame: true
  }
}

function toolNotAdvertisedTerminalBatch(): Record<string, unknown> {
  const batch = generalTerminalBatch()
  const events = batch.events as Array<Record<string, unknown>>
  const timestamp = batch.timestamp as string
  const message = 'The provider requested a tool that was not advertised for this turn.'
  events[0].item = {
    id: 'item-general', turnId: 'turn-general', threadId: 'thread-strict', role: 'system',
    status: 'failed', createdAt: timestamp, finishedAt: timestamp, kind: 'error',
    code: 'tool_not_advertised', message, severity: 'error',
    details: toolNotAdvertisedDiagnostic()
  }
  events[1].usageFinalStatus = 'failed'
  events[2] = {
    kind: 'turn_failed', seq: 13, timestamp, threadId: 'thread-strict', turnId: 'turn-general',
    itemId: 'item-general', status: 'failed', terminalReason: 'tool_failure',
    code: 'tool_not_advertised', message, error: message, severity: 'error',
    details: toolNotAdvertisedDiagnostic()
  }
  return batch
}

describe('public runtime SSE boundary', () => {
  it('emits only a closed event with exact id, event kind, sequence, and thread bindings', () => {
    const decision = projectPublicRuntimeSseBlock(
      frame('7', 'heartbeat', heartbeat()),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )

    expect(decision).toEqual({ status: 'emit', seq: 7, event: heartbeat() })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat()]
    })).toBe(true)
  })

  it('AcceptedFinalTraceRequiresCompleteClosedShape', () => {
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat({ trace: {} })]
    })).toBe(false)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat({ trace: { sse_sent_at: 1 } })]
    })).toBe(false)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat({
        trace: {
          sse_sent_at: 1,
          sse_live_emitted_at: 2
        }
      })]
    })).toBe(true)
  })

  it('admits one closed non-authoritative general terminal batch and rejects detached slots', () => {
    const batch = generalTerminalBatch()
    expect(projectPublicRuntimeSseBlock(
      frame('13', 'general_terminal_batch', batch),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 13, event: batch })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [batch]
    })).toBe(true)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat(), batch]
    })).toBe(false)

    const currentGoUsageWithoutFinalStatus = generalTerminalBatch()
    delete (currentGoUsageWithoutFinalStatus.events as Array<Record<string, unknown>>)[1].usageFinalStatus
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [currentGoUsageWithoutFinalStatus]
    })).toBe(true)

    const nested = batch.events as Array<Record<string, unknown>>
    for (const event of nested) {
      expect(projectPublicRuntimeSseBlock(
        frame(String(event.seq), String(event.kind), event),
        'thread-strict',
        new PublicRuntimeEventFilter()
      )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    }
  })

  it.each(['approval_denied', 'input_cancelled'])('admits canonical completed %s boundary failure', (reason) => {
    const batch = generalTerminalBatch()
    const terminal = (batch.events as Array<Record<string, unknown>>)[2]
    terminal.terminalReason = reason
    terminal.code = reason
    terminal.message = generalTerminalFailureMessageV1(reason)
    terminal.severity = 'warning'
    expect(GeneralTerminalDeliveryBatchV1Schema.safeParse(batch).success).toBe(true)

    const forged = generalTerminalBatch()
    const forgedTerminal = (forged.events as Array<Record<string, unknown>>)[2]
    forgedTerminal.code = reason
    forgedTerminal.message = generalTerminalFailureMessageV1(reason)
    forgedTerminal.severity = 'warning'
    expect(GeneralTerminalDeliveryBatchV1Schema.safeParse(forged).success).toBe(false)
  })

  it('admits the closed typed ordinary result on the atomic general-terminal carrier', () => {
    const batch = typedGeneralTerminalBatch()
    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 13, event: batch })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [batch]
    })).toBe(true)
  })

  it('admits only exact hash-and-enum diagnostics for an unadvertised-tool terminal', () => {
    const batch = toolNotAdvertisedTerminalBatch()
    const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(batch)
    if (!parsed.success) throw new Error(parsed.error.message)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [batch]
    })).toBe(true)
    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 13, event: batch })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [batch]
    })).toBe(true)

    for (const mutate of [
      (details: Record<string, unknown>) => { details.toolName = 'read_file' },
      (details: Record<string, unknown>) => { delete details.advertisedToolManifestHash },
      (details: Record<string, unknown>) => { details.rejectedToolCategory = 'provider_text' },
      (details: Record<string, unknown>) => { details.providerRequestRunToolStepManifestSame = false }
    ]) {
      const candidate = JSON.parse(JSON.stringify(batch)) as Record<string, unknown>
      const events = candidate.events as Array<Record<string, unknown>>
      mutate(events[2].details as Record<string, unknown>)
      expect(isStrictPublicRuntimeSseIpcPayload({
        streamId: 'stream-strict',
        events: [candidate]
      })).toBe(false)
    }
  })

  it.each([
    ['mismatched item text', (batch: Record<string, unknown>) => {
      const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
      item.text = '这是另一个普通结果。'
    }],
    ['unknown slot field', (batch: Record<string, unknown>) => {
      const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
      ;(item.ordinaryResult as Record<string, unknown>).publicationAuthority = true
    }],
    ['internal entity reference', (batch: Record<string, unknown>) => {
      const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
      const text = `已完成对象整理：cer1_${'a'.repeat(64)}`
      item.text = text
      ;(item.ordinaryResult as Record<string, unknown>).text = text
    }],
    ['private source-row reference', (batch: Record<string, unknown>) => {
      const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
      const text = `已完成来源整理：srow1_${'a'.repeat(64)}`
      item.text = text
      ;(item.ordinaryResult as Record<string, unknown>).text = text
    }],
    ['protected case fact', (batch: Record<string, unknown>) => {
      const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
      const text = '该案涉案金额为 2,645,472 元。'
      item.text = text
      ;(item.ordinaryResult as Record<string, unknown>).text = text
    }]
  ])('rejects a typed ordinary terminal with %s', (_label, mutate) => {
    const batch = typedGeneralTerminalBatch()
    mutate(batch)
    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [batch]
    })).toBe(false)
  })

  it.each([
    ['fabricated amount', '该案涉案金额为 2,645,472 元。'],
    ['fabricated account', '资金已流入银行账号 6222020200001234567。']
  ])('rejects ordinary terminal assistant text containing %s', (_label, text) => {
    const batch = generalTerminalBatch()
    const itemEvent = (batch.events as Array<Record<string, unknown>>)[0]
    ;(itemEvent.item as Record<string, unknown>).text = text

    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [batch]
    })).toBe(false)
  })

  it.each([
    ['fact authority', { factAnswerAllowed: true }],
    ['evidence authority', { evidenceAuthority: true }],
    ['citation authority', { citationAuthority: true }],
    ['unknown field', { publicationAuthority: { forged: true } }],
    ['outer sequence', { seq: 12 }]
  ])('rejects a general terminal batch with %s', (_label, mutation) => {
    const batch = generalTerminalBatch(mutation)
    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it('rejects non-contiguous and cross-turn general terminal nested events', () => {
    const nonContiguous = generalTerminalBatch()
    ;(nonContiguous.events as Array<Record<string, unknown>>)[1].seq = 13
    const crossTurn = generalTerminalBatch()
    ;(crossTurn.events as Array<Record<string, unknown>>)[1].turnId = 'turn-other'
    for (const batch of [nonContiguous, crossTurn]) {
      expect(isStrictPublicRuntimeSseIpcPayload({
        streamId: 'stream-strict',
        events: [batch]
      })).toBe(false)
    }
  })

  it('rejects detached terminal usage even when its numeric fields are otherwise closed', () => {
    const usage = {
      seq: 8,
      kind: 'usage',
      timestamp: '2026-07-18T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-strict',
      model: 'deepseek-chat',
      usage: {
        promptTokens: 10,
        completionTokens: 2,
        reasoningTokens: 1,
        totalTokens: 12,
        cacheHitRate: null,
        turns: 1
      }
    }
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [usage]
    })).toBe(false)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [{
        ...usage,
        usage: { ...usage.usage, reasoningTokens: 'PRIVATE_REASONING_SENTINEL' }
      }]
    })).toBe(false)
  })

  it('admits only count-consistent AutoResearch metadata and rejects operational paths', () => {
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [autoresearchAudit()]
    })).toBe(true)
    for (const event of [
      autoresearchAudit({ progressPath: '.analytix/private/progress.json' }),
      autoresearchAudit({ staleRequirementCount: 0 }),
      autoresearchAudit({ complete: true }),
      autoresearchAudit({ result: 'complete', complete: true })
    ]) {
      expect(isStrictPublicRuntimeSseIpcPayload({
        streamId: 'stream-strict',
        events: [event]
      })).toBe(false)
    }
  })

  it('admits the closed authority-revocation control frame without a durable id or sequence', () => {
    const raw = `event: public_projection_revoked\ndata: ${JSON.stringify(projectionRevoked())}`
    const decision = projectPublicRuntimeSseBlock(
      raw,
      'thread-strict',
      new PublicRuntimeEventFilter()
    )

    expect(decision).toEqual({ status: 'revoke', event: projectionRevoked() })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [projectionRevoked()]
    })).toBe(true)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat(), projectionRevoked()]
    })).toBe(false)
    expect(projectPublicRuntimeSseBlock(
      `id: 7\n${raw}`,
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'malformed_frame' })
    expect(projectPublicRuntimeSseBlock(
      `event: public_projection_revoked\ndata: ${JSON.stringify(projectionRevoked({ seq: 7 }))}`,
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(projectPublicRuntimeSseBlock(
      `event: public_projection_revoked\ndata: ${JSON.stringify(projectionRevoked({ threadId: ' thread-strict' }))}`,
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it.each([
    ['event kind', frame('7', 'heartbeat', heartbeat({ kind: 'tool_progress' })), 'event_kind_mismatch'],
    ['sequence', frame('7', 'heartbeat', heartbeat({ seq: 8 })), 'event_sequence_mismatch'],
    ['thread', frame('7', 'heartbeat', heartbeat({ threadId: 'thread-other' })), 'event_thread_mismatch'],
    ['unknown kind', frame('7', 'private_dump', heartbeat({ kind: 'private_dump' })), 'unsupported_event_kind'],
    ['extra public field', frame('7', 'heartbeat', heartbeat({ privatePayload: 'secret' })), 'invalid_public_projection'],
    ['non-canonical id', frame('07', 'heartbeat', heartbeat()), 'malformed_frame'],
    ['missing header', `data: ${JSON.stringify(heartbeat())}`, 'malformed_frame']
  ])('rejects a mismatched %s frame', (_label, raw, reason) => {
    expect(projectPublicRuntimeSseBlock(
      raw,
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason })
  })

  it('distinguishes restricted content from malformed input without emitting it', () => {
    const decision = projectPublicRuntimeSseBlock(
      frame('8', 'assistant_reasoning_delta', {
        seq: 8,
        kind: 'assistant_reasoning_delta',
        timestamp: '2026-07-14T00:00:00.000Z',
        threadId: 'thread-strict',
        text: 'PRIVATE_REASONING_SENTINEL'
      }),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )

    expect(decision).toEqual({ status: 'withheld', seq: 8, reason: 'restricted_content' })
    expect(JSON.stringify(decision)).not.toContain('PRIVATE_REASONING_SENTINEL')
  })

  it('withholds clean-looking assistant draft deltas at the desktop boundary', () => {
    const decision = projectPublicRuntimeSseBlock(
      frame('8', 'assistant_text_delta', {
        seq: 8,
        kind: 'assistant_text_delta',
        timestamp: '2026-07-14T00:00:00.000Z',
        threadId: 'thread-strict',
        text: 'unaccepted draft'
      }),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )

    expect(decision).toEqual({ status: 'withheld', seq: 8, reason: 'restricted_content' })
    expect(JSON.stringify(decision)).not.toContain('unaccepted draft')
  })

  it('rejects raw error prose and a closed host error when detached from its terminal batch', () => {
    const raw = {
      seq: 9,
      kind: 'item_completed',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-error',
      itemId: 'item-error',
      item: {
        id: 'item-error',
        turnId: 'turn-error',
        threadId: 'thread-strict',
        role: 'system',
        status: 'failed',
        createdAt: '2026-07-14T00:00:00.000Z',
        finishedAt: '2026-07-14T00:00:01.000Z',
        kind: 'error',
        code: 'provider_request_failed',
        message: 'PRIVATE_PROVIDER_RESPONSE_SENTINEL',
        severity: 'error'
      }
    }
    const decision = projectPublicRuntimeSseBlock(
      frame('9', 'item_completed', raw),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )

    expect(decision).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })

    const closed = {
      ...raw,
      item: {
        ...raw.item,
        message: 'Runtime request failed (provider_request_failed).'
      }
    }
    const closedDecision = projectPublicRuntimeSseBlock(
      frame('9', 'item_completed', closed),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )

    expect(closedDecision).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(JSON.stringify(closedDecision)).not.toContain('PRIVATE_PROVIDER_RESPONSE_SENTINEL')
  })

  it('admits user-origin text only in a typed user-message item', () => {
    const event = {
      seq: 10,
      kind: 'item_completed',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-user',
      itemId: 'item-user',
      item: {
        id: 'item-user',
        turnId: 'turn-user',
        threadId: 'thread-strict',
        role: 'user',
        status: 'completed',
        createdAt: '2026-07-14T00:00:00.000Z',
        finishedAt: '2026-07-14T00:00:01.000Z',
        kind: 'user_message',
        text: 'user-authored marker-free text'
      }
    }
    expect(projectPublicRuntimeSseBlock(
      frame('10', 'item_completed', event),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 10, event })

    expect(projectPublicRuntimeSseBlock(
      frame('10', 'pipeline_stage', {
        ...event,
        kind: 'pipeline_stage',
        itemId: undefined,
        item: undefined,
        stage: 'response_received',
        label: 'Response Received',
        message: 'user-authored marker-free text'
      }),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it('admits the closed invalid-tool-arguments recovery guard without tool arguments', () => {
    const event = {
      seq: 11,
      kind: 'pipeline_stage',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-strict',
      stage: 'loop_guard',
      label: 'Loop guard',
      details: {
        visibleRecovery: true,
        toolName: 'bash',
        guardKind: 'invalid_tool_arguments',
        stormCount: 1
      }
    }
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [event]
    })).toBe(true)
    expect(projectPublicRuntimeSseBlock(
      frame('11', 'pipeline_stage', event),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 11, event })

    const unknownGuard = {
      ...event,
      details: { ...event.details, guardKind: 'unknown_guard' }
    }
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [unknownGuard]
    })).toBe(false)
    expect(projectPublicRuntimeSseBlock(
      frame('11', 'pipeline_stage', unknownGuard),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it('admits only closed Provider failure-stage attribution', () => {
    const event = {
      seq: 12,
      kind: 'pipeline_stage',
      timestamp: '2026-07-14T00:00:00.000Z',
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
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-provider-stage',
      events: [event]
    })).toBe(true)
    expect(projectPublicRuntimeSseBlock(
      frame('12', 'pipeline_stage', event),
      'thread-provider-stage',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 12, event })

    for (const providerError of [
      { failureStage: 'guessed', dispatchState: 'not_sent', attempt: 1 },
      { failureStage: 'telemetry_begin', dispatchState: 'maybe_sent', attempt: 1 },
      { failureStage: 'telemetry_begin', dispatchState: 'not_sent', attempt: 1, body: 'PRIVATE' }
    ]) {
      expect(isStrictPublicRuntimeSseIpcPayload({
        streamId: 'stream-provider-stage',
        events: [{ ...event, details: { ...event.details, providerError } }]
      })).toBe(false)
    }
  })

  it.each([
    ['root message', { message: 'MARKER_FREE_PROVIDER_ERROR_7F3C' }],
    ['root summary', { summary: 'MARKER_FREE_MCP_RESPONSE_9A2D' }],
    ['root reason', { reason: 'MARKER_FREE_JOB_FAILURE_4C8E' }],
    ['unbound label', { label: 'MARKER_FREE_PROVIDER_LABEL_2B6A' }],
    ['nested message', { details: { message: 'MARKER_FREE_MCP_RESPONSE_9A2D' } }],
    ['unmarked provider error', { details: { providerError: { status: 503, retryable: true } } }],
    ['provider error prose', {
      details: { providerError: { kind: 'server', status: 503, message: 'MARKER_FREE_PROVIDER_ERROR_7F3C' } }
    }],
    ['filesystem path', { summary: '/Users/sun/private/case.csv' }],
    ['case fact', { message: '账户 6222020202020202020 收款 2645472 元' }],
    ['unknown nested key', { details: { providerError: { kind: 'server', status: 503, body: 'opaque' } } }]
  ])('rejects provider/tool/MCP/job text carrier: %s', (_label, extra) => {
    const event = {
      seq: 11,
      kind: 'pipeline_stage',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-strict',
      stage: 'response_received',
      label: 'Response Received',
      ...extra
    }
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [event]
    })).toBe(false)
    expect(projectPublicRuntimeSseBlock(
      frame('11', 'pipeline_stage', event),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it('rejects a raw MCP response instead of converting it into a public tool-result projection', () => {
    const event = {
      seq: 13,
      kind: 'item_completed',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-mcp',
      itemId: 'item-mcp',
      item: {
        id: 'item-mcp',
        turnId: 'turn-mcp',
        threadId: 'thread-strict',
        role: 'tool',
        status: 'completed',
        createdAt: '2026-07-14T00:00:00.000Z',
        finishedAt: '2026-07-14T00:00:01.000Z',
        kind: 'tool_result',
        toolName: 'mcp__funds__query',
        callId: 'call-mcp',
        toolKind: 'tool_call',
        isError: false,
        output: {
          content: [{ type: 'text', text: 'MARKER_FREE_MCP_RESPONSE_9A2D' }],
          isError: false
        }
      }
    }
    expect(projectPublicRuntimeSseBlock(
      frame('13', 'item_completed', event),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it('treats comment-only frames as transport metadata and splits LF and CRLF blocks exactly', () => {
    const filter = new PublicRuntimeEventFilter()
    expect(projectPublicRuntimeSseBlock(': connected', 'thread-strict', filter)).toBeNull()
    expect(takePublicRuntimeSseBlock(': connected\n\nrest')).toEqual({
      block: ': connected',
      rest: 'rest'
    })
    expect(takePublicRuntimeSseBlock(': connected\r\n\r\nrest')).toEqual({
      block: ': connected',
      rest: 'rest'
    })
  })

  it('rejects open IPC envelopes and raw private event keys at the preload boundary', () => {
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat()],
      privatePayload: 'secret'
    })).toBe(false)
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [heartbeat({ privatePayload: 'secret' })]
    })).toBe(false)
    expect(isStrictPublicRuntimeSseEndPayload({ streamId: 'stream-strict' })).toBe(true)
    expect(isStrictPublicRuntimeSseEndPayload({
      streamId: 'stream-strict',
      privatePayload: 'secret'
    })).toBe(false)
    expect(isStrictPublicRuntimeSseErrorPayload({
      streamId: 'stream-strict',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).'
    })).toBe(true)
    expect(isStrictPublicRuntimeSseErrorPayload({
      streamId: 'stream-strict',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'general_terminal_batch_invalid'
    })).toBe(true)
    expect(isStrictPublicRuntimeSseErrorPayload({
      streamId: 'stream-strict',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'raw_provider_body'
    })).toBe(false)
    expect(isStrictPublicRuntimeSseErrorPayload({
      streamId: 'stream-strict',
      code: 'sse_event_rejected',
      message: 'PRIVATE_UPSTREAM_ERROR'
    })).toBe(false)
  })

  it.each([
    'Authorization: Bearer opaque-sse-secret-123',
    'api_key=opaque-sse-secret-123',
    'account number: 6222020202020202020'
  ])('rejects secret or PII content in an otherwise closed progress event: %s', (message) => {
    const event = {
      seq: 12,
      kind: 'tool_progress',
      timestamp: '2026-07-18T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-strict',
      toolName: 'read_file',
      callId: 'call-safe',
      status: 'running',
      message
    }
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [event]
    })).toBe(false)
    expect(projectPublicRuntimeSseBlock(
      frame('12', 'tool_progress', event),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it.each([
    {
      seq: 12,
      kind: 'tool_call_ready',
      timestamp: '2026-07-18T00:00:00.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-strict',
      itemId: 'item_tool_host_v1_4c72',
      toolName: 'read',
      callId: 'call_host_4c72',
      readyCount: 4
    },
    {
      seq: 13,
      kind: 'tool_progress',
      timestamp: '2026-07-18T00:00:01.000Z',
      threadId: 'thread-strict',
      turnId: 'turn-strict',
      itemId: 'item_tool_host_v1_4c72',
      toolName: 'read',
      callId: 'call_host_4c72',
      status: 'running'
    }
  ])('accepts the host tool item identity on $kind events', (event) => {
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-strict',
      events: [event]
    })).toBe(true)
    expect(projectPublicRuntimeSseBlock(
      frame(String(event.seq), event.kind, event),
      'thread-strict',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', event, seq: event.seq })
  })
})

describe('public gate item replay', () => {
  const base = {
    id: 'item-gate', threadId: 'thread-strict', turnId: 'turn-strict',
    status: 'pending', createdAt: '2026-09-23T00:00:00Z'
  }
  const items = [
    { ...base, kind: 'approval', role: 'tool', approvalId: 'approval_123456789012',
      toolName: 'write_file', summary: 'Approve write_file' },
    { ...base, kind: 'user_input', role: 'system', inputId: 'input_a1b2c3d4e5f6',
      prompt: 'Choose a path', questions: [{ id: 'input_a1b2c3d4e5f6_1', header: 'Path',
        question: 'Choose a path', options: [] }] }
  ]

  it.each(items)('accepts the public $kind item produced by Core', (item) => {
    const event = { kind: 'item_created', seq: 170, timestamp: item.createdAt,
      threadId: item.threadId, turnId: item.turnId, itemId: item.id, item }
    expect(isPublicSseIpcPayload({ streamId: 'gate-stream', events: [event] })).toBe(true)
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'gate-stream', events: [event] })).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame('170', 'item_created', event), item.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: 170 })
    expect(projectPublicRuntimeSseBlock(frame('170', 'item_created', {
      ...event, item: { ...item, continuationReceiptId: 'private' }
    }), item.threadId, new PublicRuntimeEventFilter())).toEqual({
      status: 'invalid', reason: 'invalid_public_projection'
    })
  })
})

describe('public gate request event replay', () => {
  const base = { seq: 171, timestamp: '2026-09-23T00:00:00Z',
    threadId: 'thread-strict', turnId: 'turn-strict', itemId: 'item-gate' }
  const events = [
    { ...base, kind: 'approval_requested', approvalId: 'approval_a1b2c3d4e5f6',
      toolName: 'write_file', status: 'pending', approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write', summary: 'Approve write_file' },
    { ...base, kind: 'user_input_requested', inputId: 'input_a1b2c3d4e5f6',
      status: 'pending', prompt: 'Choose a path', questions: [
        { id: 'input_a1b2c3d4e5f6_1', header: 'Path', question: 'Choose a path', options: [] }
      ] }
  ]

  it.each(events)('accepts the public $kind event produced by Core', (event) => {
    expect(isPublicSseIpcPayload({ streamId: 'gate-stream', events: [event] })).toBe(true)
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame('171', event.kind, event), event.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: 171 })
    expect(projectPublicRuntimeSseBlock(frame('171', event.kind, {
      ...event, continuationReceiptId: 'private'
    }), event.threadId, new PublicRuntimeEventFilter())).toEqual({
      status: 'invalid', reason: 'invalid_public_projection'
    })
  })
})

function publicChildLedger(stage = 'background_job_delivery_pending', status = 'pending', callId?: string) {
  const delivery = stage.startsWith('background_job_delivery_')
  const autoContinue = stage.startsWith('background_job_auto_continue_')
  const jobStatus = delivery || autoContinue ? 'completed' : status
  return {
    id: 'item-child-ledger', threadId: 'thread-strict', turnId: 'turn-strict',
    kind: 'tool_progress', role: 'tool', status: status === 'dead_letter' ? 'failed' : 'completed',
    createdAt: '2026-09-11T00:00:00Z', finishedAt: '2026-09-11T00:00:00Z',
    toolName: delivery ? 'background_delivery' : autoContinue ? 'background_auto_continue' : 'subagent',
    ...(callId === undefined ? {} : { callId }),
    summary: 'child output withheld', message: 'child output withheld',
    arguments: {
      runtimeStatus: 'tool_progress', stage, status,
      diagnostics: {
        kind: 'subagent_task', id: 'job-child', jobId: 'job-child', childRunId: 'job-child',
        status: jobStatus, childStatus: jobStatus, terminal: jobStatus !== 'running',
        ...(delivery ? { deliveryStatus: status } : {}),
        ...(autoContinue ? { autoContinueStatus: status } : {}),
        outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
        factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false
      }
    }
  }
}

function childLedgerEvent(item = publicChildLedger()) {
  return { kind: 'item_created', seq: 170, timestamp: item.createdAt,
    threadId: item.threadId, turnId: item.turnId, itemId: item.id, item }
}

describe('public child ledger contract', () => {
  const hostCallId = `call_host_${'a'.repeat(64)}`
  it.each([
    ['background_job_delivery_pending', 'pending', undefined],
    ['background_job_delivery_unknown', 'unknown', undefined], // isolates absent callId
    ['background_job_delivery_pending', 'pending', hostCallId], // isolates status family
    ['background_job_delivery_delivered', 'delivered', undefined],
    ['background_job_delivery_retry', 'retry', undefined],
    ['background_job_delivery_skipped', 'skipped', undefined],
    ['background_job_delivery_dead_letter', 'dead_letter', undefined],
    ['subagent_running', 'running', hostCallId],
    ['background_job_completed', 'completed', hostCallId],
    ['background_job_auto_continue_started', 'started', undefined]
  ])('accepts closed %s with status %s and callId %s', (stage, status, callId) => {
    const event = childLedgerEvent(publicChildLedger(stage, status, callId))
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'ledger-stream', events: [event] })).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame('170', 'item_created', event), event.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: 170 })
  })

  it.each([
    ['callId', 'foreign-provider-call'], ['callId', 'call_host_short'],
    ['arguments.status', 'completed'], ['arguments.stage', 'subagent_pending'],
    ['arguments.diagnostics.deliveryStatus', 'delivered'],
    ['arguments.diagnostics.jobId', undefined], ['arguments.diagnostics.id', 'detached-child'],
    ['arguments.diagnostics.childStatus', 'running'], ['arguments.diagnostics.terminal', false],
    ['arguments.diagnostics.evidenceAuthority', true], ['arguments.diagnostics.factAnswerAllowed', true],
    ['arguments.diagnostics.canReadOutput', true], ['arguments.diagnostics.canContinueParent', true],
    ['arguments.diagnostics.output', 'raw child output'], ['arguments.stdout', 'private output'],
    ['extra', true], ['role', 'assistant'], ['createdAt', 'not-a-time'],
    ['status', 'running'], ['finishedAt', '2026-09-10T00:00:00Z'],
    ['arguments.diagnostics.parentThreadId', 'foreign-thread']
  ])('rejects invalid closed child field %s', (path, value) => {
    const event = childLedgerEvent()
    let target = event.item as unknown as Record<string, unknown>
    const parts = path.split('.')
    for (const key of parts.slice(0, -1)) target = target[key] as Record<string, unknown>
    if (value === undefined) {
      delete target[parts[parts.length - 1]]
      delete target.childRunId
    } else target[parts[parts.length - 1]] = value
    expect(projectPublicRuntimeSseBlock(frame('170', 'item_created', event), event.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it.each(['threadId', 'turnId', 'itemId'])('rejects detached event %s', (field) => {
    const event = { ...childLedgerEvent(), [field]: 'foreign-binding' }
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'ledger-stream', events: [event] })).toBe(false)
  })

  it('requires a host callId outside the closed background ledger families', () => {
    const event = childLedgerEvent(publicChildLedger('subagent_running', 'running'))
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'ledger-stream', events: [event] })).toBe(false)
  })
})
