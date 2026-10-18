import { describe, expect, it, vi } from 'vitest'
import { GeneralTerminalDeliveryBatchV1Schema, generalTerminalFailureMessageV1 } from '../../packages/runtime/src/contracts/events'
import { ToolCallTurnItem } from '../../packages/runtime/src/contracts/items'
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

  it.each([
    '日期：2026-07-01；数量：42；金额：1234.56 元；参考编号：REF-17。',
    '日期：2026-07-01；数量：42；金额：1234.56 元；参考编号：550e8400-e29b-41d4-a716-44665544a000。',
    '请解释 DOM 元素的父子关系。'
  ])('admits a typed ordinary answer in a public SSE batch: %s', (answer) => {
    const batch = typedGeneralTerminalBatch(answer)
    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict', new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 13, event: batch })
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'stream-strict', events: [batch] })).toBe(true)
  })

  it.each([
    '张某与李某是父子。',
    '甲公司支付给乙公司2645.72元。',
    '甲公司支付乙公司2645.72元。',
    '甲公司支付乙公司2万元。',
    '张某与李某存在父子关系。',
    '甲公司取得2026年收益￥2万元。',
    '甲公司取得2026年收益2万元。',
    '甲公司支付\n2645.72 元'
  ])('withholds a typed case assertion from public SSE: %s', (text) => {
    const batch = typedGeneralTerminalBatch(text)
    expect(projectPublicRuntimeSseBlock(
      frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict', new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
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

  it('preserves closed cache and attempt-cost diagnostics in the terminal carrier', () => {
    const unknown = { complete: false, knownObservationCount: 0, observationCount: 2 }
    const diagnostics = {
      dynamicStateCheck: 'not_checked', toolSchemaEstimator: 'utf8_bytes_div4',
      responseModelObservation: 'matches_resolved', modelInputFirstDifference: 'history',
      modelInputComparable: true, modelInputComparablePrefixBytes: 123,
      providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1', providerAttemptTelemetryValid: true,
      providerLogicalCallCount: 1, providerAttemptCount: 2,
      providerAttemptStatuses: { succeeded: 1, failed: 1, cancelled: 0, timedOut: 0, streamAborted: 0 },
      providerAttemptInputTokens: unknown, providerAttemptOutputTokens: unknown,
      providerAttemptCacheHitTokens: unknown, providerAttemptCacheMissTokens: unknown,
      providerAttemptCacheRate: { known: false, numerator: 0, denominator: 0 },
      providerCostEstimateComplete: false, providerCostKnownAttemptCount: 1,
      providerKnownCostUsdNanos: 12, providerKnownCostCnyNanos: 0
    }
    const batch = generalTerminalBatch()
    const usage = (batch.events as Array<Record<string, unknown>>)[1]
    usage.cacheDiagnostics = diagnostics
    const parsed = GeneralTerminalDeliveryBatchV1Schema.parse(batch)
    expect(parsed.events[1]).toEqual(usage) // No stripping fields covered by the host digest.
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'stream-cache', events: [batch] })).toBe(true)
    for (const mutation of [
      { dynamicStateCheck: 'checked_safe' }, { toolSchemaEstimator: 'actual_tokens' },
      { responseModelObservation: 'PRIVATE_MODEL_PROSE' }, { modelInputFirstDifference: 'PRIVATE_INPUT' },
      { modelInputComparablePrefixBytes: -1 }, { providerCostEstimateComplete: true },
      { providerCostKnownAttemptCount: 3 }, { providerKnownCostUsdNanos: 0.1 },
      { providerKnownCostCnyNanos: undefined }, { providerCostKnownAttemptCount: 0 }
    ]) {
      usage.cacheDiagnostics = { ...diagnostics, ...mutation }
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'stream-cache', events: [batch] })).toBe(false)
    }
  })

  it('binds new terminal cost coverage to physical attempt counts while retaining old signed shape', () => {
    const legacy = generalTerminalBatch()
    expect(GeneralTerminalDeliveryBatchV1Schema.safeParse(legacy).success).toBe(true)
    const batch = generalTerminalBatch()
    const usageEvent = (batch.events as Array<Record<string, unknown>>)[1]
    const usage = usageEvent.usage as Record<string, unknown>
    usage.costEstimateStatus = 'partial'
    usage.costKnownCurrencies = ['USD']
    usageEvent.cacheDiagnostics = {
      providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
      providerAttemptTelemetryValid: true,
      providerLogicalCallCount: 1,
      providerAttemptCount: 2,
      providerAttemptStatuses: { succeeded: 1, failed: 1, cancelled: 0, timedOut: 0, streamAborted: 0 },
      providerAttemptInputTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
      providerAttemptOutputTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
      providerAttemptCacheHitTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
      providerAttemptCacheMissTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
      providerAttemptCacheRate: { known: false, numerator: 0, denominator: 0 },
      providerCostEstimateComplete: false,
      providerCostKnownAttemptCount: 1,
      providerKnownCostUsdAttemptCount: 1,
      providerKnownCostCnyAttemptCount: 0,
      providerKnownCostUsdNanos: 12,
      providerKnownCostCnyNanos: 0
    }
    expect(GeneralTerminalDeliveryBatchV1Schema.safeParse(batch).success).toBe(true)
    for (const mutation of [
      { costKnownCurrencies: ['CNY'] },
      { costKnownCurrencies: ['USD', 'USD'] },
      { costKnownCurrencies: undefined },
      { costEstimateStatus: 'complete' },
      { costUsd: 12 / 1e9 }
    ]) {
      usageEvent.usage = { ...usage, ...mutation }
      expect(GeneralTerminalDeliveryBatchV1Schema.safeParse(batch).success).toBe(false)
    }
    usageEvent.usage = usage
    usageEvent.cacheDiagnostics = { ...(usageEvent.cacheDiagnostics as Record<string, unknown>), providerKnownCostUsdAttemptCount: 0 }
    expect(GeneralTerminalDeliveryBatchV1Schema.safeParse(batch).success).toBe(false)
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

  describe('initial user skill IDs across SSE and IPC', () => {
    const timestamp = '2026-10-18T00:00:00Z'
    const eventWithSkills = (activeSkillIds: unknown) => ({
      kind: 'item_created', seq: 22, timestamp, threadId: 'thread-strict',
      turnId: 'turn-user', itemId: 'item-user',
      item: { id: 'item-user', threadId: 'thread-strict', turnId: 'turn-user',
        role: 'user', status: 'completed', createdAt: timestamp, kind: 'user_message',
        text: '/synthetic-skill review this', activeSkillIds }
    })

    it.each([
      ['synthetic-skill'],
      ['a', 'office.docx', 'my_skill', '7-skill', 'z'.repeat(128)],
      Array.from({ length: 16 }, (_, index) => `skill-${index}`)
    ].flatMap(ids => ['item_created', 'item_completed'].map(kind => ({ ids, kind }))))(
      'preserves the exact admitted initial item through decoder and IPC (%#)', ({ ids, kind }) => {
        const event = { ...eventWithSkills(ids), kind }
        const diagnostics = vi.fn()
        expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
        expect(projectPublicRuntimeSseBlock(frame('22', kind, event), 'thread-strict',
          new PublicRuntimeEventFilter(), diagnostics)).toEqual({ status: 'emit', seq: 22, event })
        expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'initial-skills', events: [event] })).toBe(true)
        expect(diagnostics).not.toHaveBeenCalled()
      }
    )

    it.each([
      null, 'synthetic-skill', {}, [], [null], [1], [true], [['review']],
      [{ body: 'PRIVATE_SKILL_BODY_SENTINEL' }], ['Review'], [' review'], ['review '],
      ['review\n'], ['review\r\n'], ['review\u2028'], ['review/entry'], ['-review'], ['review_'], ['review.'], ['réview'],
      ['a'.repeat(129)], ['review', 'review'],
      Array.from({ length: 17 }, (_, index) => `skill-${index}`), new Array(1)
    ].map(ids => ({ ids })))('rejects malformed, duplicate, or over-budget skill IDs before filtering (%#)', ({ ids }) => {
      const event = eventWithSkills(ids)
      const diagnostics = vi.fn()
      const filter = new PublicRuntimeEventFilter()
      const push = vi.spyOn(filter, 'push')
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
      const decision = projectPublicRuntimeSseBlock(frame('22', event.kind, event), 'thread-strict', filter, diagnostics)
      expect(decision).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'initial-skills', events: [event] })).toBe(false)
      expect(push).not.toHaveBeenCalled()
      expect(diagnostics).toHaveBeenCalledExactlyOnceWith({
        validation: 'input', stage: 'item_contract', reason: 'invalid_field', fieldPaths: ['item.activeSkillIds']
      })
      expect(JSON.stringify({ decision, diagnostics: diagnostics.mock.calls })).not.toContain('PRIVATE_SKILL_BODY_SENTINEL')
    })

    it('rejects a present undefined value at the object and IPC boundaries', () => {
      const event = eventWithSkills(undefined)
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'initial-skills', events: [event] })).toBe(false)
    })

    it('rejects loaded IDs on a steering item', () => {
      const event = eventWithSkills(['synthetic-skill'])
      Object.assign(event.item, { delivery: 'steer', clientUserMessageId: '00000000-0000-4000-8000-000000000001' })
      expect(projectPublicRuntimeSseBlock(frame('22', event.kind, event), 'thread-strict',
        new PublicRuntimeEventFilter())).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'initial-skills', events: [event] })).toBe(false)
    })

    it('keeps the field unavailable to other item kinds', () => {
      const event = { ...eventWithSkills(['synthetic-skill']), item: {
        id: 'item-user', threadId: 'thread-strict', turnId: 'turn-user', role: 'tool',
        status: 'running', createdAt: timestamp, kind: 'tool_call', toolName: 'read_file',
        callId: 'call_host_' + 'a'.repeat(64), toolKind: 'tool_call',
        arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only',
          messageKey: 'tool_arguments_withheld', privatePayloadWithheld: true,
          factAnswerAllowed: false, evidenceAuthority: false }
      } }
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
      Object.assign(event.item, { activeSkillIds: ['synthetic-skill'] })
      expect(projectPublicRuntimeSseBlock(frame('22', event.kind, event), 'thread-strict',
        new PublicRuntimeEventFilter())).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'initial-skills', events: [event] })).toBe(false)
    })

    it.each(['event-root', 'item-meta'])('keeps arbitrary metadata closed at %s', owner => {
      const event = eventWithSkills(['synthetic-skill'])
      if (owner === 'event-root') Object.assign(event, { activeSkillIds: ['synthetic-skill'] })
      else Object.assign(event.item, { meta: { activeSkillIds: ['synthetic-skill'] } })
      expect(projectPublicRuntimeSseBlock(frame('22', event.kind, event), 'thread-strict',
        new PublicRuntimeEventFilter())).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'initial-skills', events: [event] })).toBe(false)
    })
  })

  describe('typed user file references across the SSE boundary', () => {
    const reference = { path: '/synthetic/workspace/queue-fixture.txt', relativePath: 'queue-fixture.txt',
      name: 'queue-fixture.txt', kind: 'file' }
    const eventWithReferences = (fileReferences: unknown) => ({
      kind: 'item_created', seq: 22, timestamp: '2026-10-14T00:38:04Z',
      threadId: 'thread-strict', turnId: 'turn-user', itemId: 'item-user',
      trace: { sse_sent_at: 1791938335000, sse_live_emitted_at: 1791938335000 },
      item: { id: 'item-user', threadId: 'thread-strict', turnId: 'turn-user', role: 'user',
        status: 'completed', createdAt: '2026-10-14T00:38:04Z', kind: 'user_message',
        text: 'Synthetic queued message.', fileReferences }
    })
    it.each([[], [reference], [{ ...reference, kind: 'directory' }], [{
      path: reference.path, relativePath: reference.relativePath, name: reference.name
    }]].map(references => ({ references })))('preserves exact closed file references through decoder and IPC (%j)', ({ references }) => {
      const event = eventWithReferences(references)
      const diagnostics = vi.fn()
      expect(projectPublicRuntimeSseBlock(frame('22', 'item_created', event), 'thread-strict',
        new PublicRuntimeEventFilter(), diagnostics)).toEqual({ status: 'emit', seq: 22, event })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'typed-references', events: [event] })).toBe(true)
      expect(diagnostics).not.toHaveBeenCalled()
    })
    it.each([
      null, reference, [null], [{ ...reference, path: '' }], [{ ...reference, relativePath: '' }],
      [{ ...reference, name: '' }], [{ ...reference, path: 1 }], [{ ...reference, kind: 'unknown' }],
      [{ ...reference, content: 'PRIVATE_REFERENCE_BODY_SENTINEL' }],
      [{ ...reference, selector: { authority: 'PRIVATE_REFERENCE_AUTHORITY_SENTINEL' } }],
      Array.from({ length: 4097 }, () => reference)
    ].map(references => ({ references })))('rejects invalid or open reference metadata without exposing it (%#)', ({ references }) => {
      const event = eventWithReferences(references)
      const diagnostics = vi.fn()
      const decision = projectPublicRuntimeSseBlock(frame('22', 'item_created', event), 'thread-strict',
        new PublicRuntimeEventFilter(), diagnostics)
      expect(decision).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'typed-references', events: [event] })).toBe(false)
      expect(JSON.stringify({ decision, diagnostics: diagnostics.mock.calls })).not.toContain('PRIVATE_REFERENCE_')
    })
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

  it('admits only numeric host reasoning latency across the desktop SSE boundary', () => {
    for (const firstReasoningLatencyMs of [0, 17, 'PRIVATE_REASONING', { text: 'PRIVATE' }, -1, 1.5, Infinity]) {
      const event = {
        seq: 11, kind: 'pipeline_stage', timestamp: '2026-07-14T00:00:00.000Z',
        threadId: 'thread-reasoning-timing', turnId: 'turn-reasoning-timing',
        stage: 'response_received', label: 'Response Received',
        details: { firstReasoningLatencyMs }
      }
      const valid = typeof firstReasoningLatencyMs === 'number' &&
        Number.isSafeInteger(firstReasoningLatencyMs) && firstReasoningLatencyMs >= 0
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'stream-timing', events: [event] })).toBe(valid)
      expect(projectPublicRuntimeSseBlock(
        frame('11', 'pipeline_stage', event), event.threadId, new PublicRuntimeEventFilter()
      )?.status).toBe(valid ? 'emit' : 'invalid')
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

  it('replays a host user-input item whose question ID has a numeric digest run', () => {
    const inputId = `input_${'123456789012'}${'a'.repeat(52)}`
    const item = { ...base, kind: 'user_input', role: 'system', inputId,
      prompt: 'Choose a path', questions: [
        { id: `${inputId}_1`, header: 'Path', question: 'Choose a path', options: [] }
      ] }
    const event = { kind: 'item_created', seq: 170, timestamp: item.createdAt,
      threadId: item.threadId, turnId: item.turnId, itemId: item.id, item }
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame('170', 'item_created', event), item.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: 170 })
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

  it('keeps an exact host question ID when its opaque digest contains a numeric run', () => {
    const inputId = `input_${'123456789012'}${'a'.repeat(52)}`
    const event = { ...base, kind: 'user_input_requested', inputId,
      status: 'pending', prompt: 'Choose a path', questions: [
        { id: `${inputId}_1`, header: 'Path', question: 'Choose a path', options: [] }
      ] }
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame('171', event.kind, event), event.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: 171 })

    const forged = { ...event, questions: [{ ...event.questions[0], id: `other_${'123456789012'}` }] }
    expect(isClosedPublicRuntimeSseEvent(forged)).toBe(false)
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


describe('closed Host retry lifecycle wire', () => {
  const scheduled = { schemaVersion: 1, source: 'provider_retry', phase: 'scheduled',
    logicalSequence: 7, outerAttempt: 2, delayMs: 0, deadlineUnixMs: 1700000000000 }
  const wire = (retry: unknown = scheduled, overrides: Record<string, unknown> = {}) => ({
    kind: 'pipeline_stage', seq: 41, threadId: 'thread-strict', turnId: 'turn-retry',
    timestamp: '2026-10-07T00:00:00.123456789Z', stage: 'provider_retrying',
    label: 'Retrying provider stream', attempt: 2, maxAttempt: 3, details: { retry }, ...overrides
  })
  it.each(['provider_retry', 'transport_reconnect'])('admits all actual %s phases including zero delay and past deadline', source => {
    for (const phase of ['scheduled', 'started', 'cancelled']) {
      const retry = { schemaVersion: 1, source, phase, logicalSequence: 7, outerAttempt: 2,
        ...(source === 'transport_reconnect' ? { physicalAttempt: 3 } : {}),
        ...(phase === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1700000000000 } : {}),
        ...(phase === 'cancelled' ? { cancelReason: 'attempt_rejected' } : {}) }
      const event = wire(retry, { attempt: source === 'transport_reconnect' ? 3 : 2 })
      expect(projectPublicRuntimeSseBlock(frame('41', 'pipeline_stage', event), 'thread-strict',
        new PublicRuntimeEventFilter())).toEqual({ status: 'emit', seq: 41, event })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'retry-stream', events: [event] })).toBe(true)
    }
  })
  it('preserves realistic retry epochs without public-content rejection diagnostics', () => {
    const event = wire()
    const diagnostics: unknown[] = []
    expect(projectPublicRuntimeSseBlock(frame('41', 'pipeline_stage', event), 'thread-strict',
      new PublicRuntimeEventFilter(), diagnostic => diagnostics.push(diagnostic)))
      .toEqual({ status: 'emit', seq: 41, event })
    expect(diagnostics).toEqual([])
  })
  it.each([
    null, {}, { ...scheduled, privateText: 'DO_NOT_EXPOSE' }, { ...scheduled, schemaVersion: 2 },
    { ...scheduled, source: 'visibleRecovery' }, { ...scheduled, phase: 'unknown' },
    { ...scheduled, logicalSequence: 0 }, { ...scheduled, logicalSequence: 1.5 },
    { ...scheduled, logicalSequence: 1e15 + 1 }, { ...scheduled, outerAttempt: 1e15 + 1 },
    { ...scheduled, source: 'transport_reconnect', physicalAttempt: 1e15 + 1 },
    { ...scheduled, delayMs: 1e15 + 1 }, { ...scheduled, deadlineUnixMs: 1e15 + 1 },
    { ...scheduled, logicalSequence: Number.MAX_SAFE_INTEGER + 1 }, { ...scheduled, logicalSequence: null },
    { ...scheduled, outerAttempt: 0 }, { ...scheduled, outerAttempt: -1 },
    { ...scheduled, physicalAttempt: 3 }, { ...scheduled, source: 'transport_reconnect' },
    { ...scheduled, source: 'transport_reconnect', physicalAttempt: 0 },
    { ...scheduled, source: 'transport_reconnect', physicalAttempt: 1.5 },
    { ...scheduled, delayMs: -1 }, { ...scheduled, delayMs: null }, { ...scheduled, delayMs: 0.1 },
    { ...scheduled, delayMs: Number.MAX_SAFE_INTEGER + 1 }, { ...scheduled, delayMs: undefined },
    { ...scheduled, deadlineUnixMs: 0 }, { ...scheduled, deadlineUnixMs: null },
    { ...scheduled, deadlineUnixMs: Number.MAX_SAFE_INTEGER + 1 }, { ...scheduled, deadlineUnixMs: undefined },
    { ...scheduled, cancelReason: 'context_cancelled' }, { ...scheduled, phase: 'started' },
    { ...scheduled, phase: 'cancelled', cancelReason: 'deadline_exceeded' },
    { schemaVersion: 1, source: 'provider_retry', phase: 'cancelled', logicalSequence: 1, outerAttempt: 2 },
    { schemaVersion: 1, source: 'provider_retry', phase: 'started', logicalSequence: 1, outerAttempt: 2, cancelReason: 'attempt_rejected' },
    { schemaVersion: 1, source: 'provider_retry', phase: 'cancelled', logicalSequence: 1, outerAttempt: 2, cancelReason: 'private reason' },
    ...[null, ['context_cancelled'], { toString: 'context_cancelled' }].map(cancelReason => ({
      schemaVersion: 1, source: 'provider_retry', phase: 'cancelled', logicalSequence: 1, outerAttempt: 2, cancelReason
    }))
  ])('rejects incomplete, open or impossible retry facts: %j', retry => {
    const event = wire(retry)
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
    expect(projectPublicRuntimeSseBlock(frame('41', 'pipeline_stage', event), 'thread-strict',
      new PublicRuntimeEventFilter())).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'retry-stream', events: [event] })).toBe(false)
  })
  it('retains stage, message, id, sequence, thread and privacy barriers', () => {
    for (const event of [
      wire(scheduled, { stage: 'response_received', label: 'Response Received' }),
      wire(scheduled, { message: 'user-controlled message' }),
      wire(scheduled, { details: { retry: scheduled, visibleRecovery: true } }),
      wire(scheduled, { details: { retry: scheduled, privatePayload: 'secret' } }),
      wire(scheduled, { timestamp: null }), wire(scheduled, { turnId: null }),
      wire(scheduled, { attempt: 1 }), wire(scheduled, { attempt: 0 }),
      wire(scheduled, { attempt: undefined }), wire(scheduled, { maxAttempt: undefined }),
      wire(scheduled, { maxAttempt: 1 }), wire(scheduled, { maxAttempt: 1e15 + 1 })
    ]) expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
    expect(projectPublicRuntimeSseBlock(frame('40', 'pipeline_stage', wire()), 'thread-strict',
      new PublicRuntimeEventFilter())).toMatchObject({ status: 'invalid', reason: 'event_sequence_mismatch' })
    expect(projectPublicRuntimeSseBlock(frame('41', 'pipeline_stage', wire()), 'different-thread',
      new PublicRuntimeEventFilter())).toMatchObject({ status: 'invalid', reason: 'event_thread_mismatch' })
    const legacy = wire(undefined, { details: { visibleRecovery: true, recoveryKind: 'interrupted_stream' } })
    expect(isClosedPublicRuntimeSseEvent(legacy)).toBe(true)
    expect(isClosedPublicRuntimeSseEvent(wire(undefined, { details: {} }))).toBe(true)
  })
  it('admits only a scoped length observation at response_received', () => {
    const details = { stopReason: 'length', streamCompleted: false, logicalSequence: 7 }
    expect(isClosedPublicRuntimeSseEvent(wire(undefined, { stage: 'response_received', label: 'Response Received', details: { ...details, streamCompleted: true } }))).toBe(true)
    expect(isClosedPublicRuntimeSseEvent(wire(undefined, { stage: 'response_received', label: 'Response Received', details }))).toBe(true)
    expect(isClosedPublicRuntimeSseEvent(wire(undefined, { stage: 'response_received', label: 'Response Received', details: { ...details, toolCallCount: 0 } }))).toBe(true)
    for (const invalid of [{ ...details, logicalSequence: 1e15 + 1 }, { ...details, toolCallCount: 1e15 + 1 },
      { ...details, toolCallCount: null }, { ...details, toolCallCount: -1 }, { ...details, toolCallCount: 0.1 },
      { ...details, streamCompleted: 'true' }, { ...details, stopReason: 'stop' },
      { ...details, logicalSequence: 0 }, { ...details, logicalSequence: 1.5 },
      { ...details, logicalSequence: null }, { ...details, streamCompleted: null }]) {
      expect(isClosedPublicRuntimeSseEvent(wire(undefined, { stage: 'response_received', label: 'Response Received', details: invalid }))).toBe(false)
    }
    expect(isClosedPublicRuntimeSseEvent(wire(undefined, { details }))).toBe(false)
  })


})

describe('modern Core user-input replay binding', () => {
  const inputId = `input_${'123456789012'}${'a'.repeat(52)}`
  const question = { id: `${inputId}_1`, header: 'Path', question: 'Choose', multiSelect: true,
    options: [{ id: `${inputId}_1_option_1`, label: 'Fast', description: '' }] }
  const event = { kind: 'user_input_requested', seq: 173, timestamp: '2026-10-08T00:00:00Z',
    threadId: 'thread-strict', turnId: 'turn-strict', inputId, status: 'pending', prompt: 'Choose', questions: [question] }
  it('replays exact question/option IDs with numeric opaque digest runs', () => {
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame('173', event.kind, event), event.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: 173 })
  })
  it.each([
    { questions: [{ ...question, id: 'other' }] },
    { questions: [{ ...question, options: [{ ...question.options[0], id: `${inputId}_2_option_1` }] }] },
    { questions: [{ ...question, options: [{ label: 'Fast', description: '' }] }] },
    { questions: [{ ...question, multiSelect: undefined }] },
    { questions: [question, { id: 'legacy', header: 'Other', question: 'Other', options: [] }] }
  ])('rejects detached, incomplete or downgraded modern identities', ({ questions }) => {
    expect(isClosedPublicRuntimeSseEvent({ ...event, questions })).toBe(false)
  })
})

describe('timed question public boundary', () => {
  const inputId = 'input_' + 'a'.repeat(64), timedWait = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
  const requested = { kind: 'user_input_requested', seq: 180, timestamp: '2026-10-08T00:00:00Z', threadId: 'thread-strict', turnId: 'turn-strict', inputId,
    status: 'pending', prompt: 'Safe projection', questions: [{ id: inputId + '_1', header: 'Safe header', question: 'Safe question', multiSelect: false, options: [] }], timedWait }
  it('admits bounded timing metadata and a real continued resolution', () => {
    expect(isClosedPublicRuntimeSseEvent(requested)).toBe(true)
    expect(isClosedPublicRuntimeSseEvent({ kind: 'user_input_resolved', seq: 181, timestamp: requested.timestamp, threadId: requested.threadId, turnId: requested.turnId, inputId, status: 'continued' })).toBe(true)
  })
  it.each([{ callId: 'private-call' }, { lateReply: { state: 'queued' } }, { submission: { answers: [] } }, { timedWait: { ...timedWait, remainingMs: 20 } }])('rejects private or open timed fields %j', fields => {
    expect(isClosedPublicRuntimeSseEvent({ ...requested, ...fields })).toBe(false)
  })
})

const nativeWitnessFs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
const nativeWitnessProcess = await vi.importActual<{ env: Record<string, string | undefined> }>('node:process')

type NativeHookupEventWitness = {
  rawBuilder: Record<string, unknown>
  projected: Record<string, unknown>
  expected: Record<string, unknown>
  promotion: Record<string, unknown>
}

function nativeHookupEventWitness(): NativeHookupEventWitness {
  const witnessPath = nativeWitnessProcess.env.ANALYTIX_NATIVE_HOOKUP_EVENT_WITNESS
  if (witnessPath === undefined) {
    const timestamp = '2026-10-09T00:00:00.123456789Z'
    const expected = {
      kind: 'turn_steered', seq: 41, timestamp, threadId: 'synthetic-native-hookup-thread',
      turnId: 'synthetic-native-hookup-turn', clientUserMessageId: 'synthetic-native-hookup-message',
      admittedSeq: 40, model: 'synthetic-model', providerId: 'synthetic-provider',
      approvalPolicy: 'on-request', sandboxMode: 'workspace-write', mode: 'agent',
      disableUserInput: false, maxModelSteps: 0
    }
    return {
      rawBuilder: { ...expected, text: 'SYNTHETIC_NATIVE_HOOKUP_RAW_BUILDER_BODY' },
      projected: { ...expected },
      expected,
      promotion: {
        kind: 'item_created', seq: 42, timestamp, threadId: expected.threadId,
        turnId: expected.turnId, itemId: 'synthetic-native-hookup-item', item: {
          id: 'synthetic-native-hookup-item', threadId: expected.threadId, turnId: expected.turnId,
          role: 'user', kind: 'user_message', status: 'completed', createdAt: timestamp,
          finishedAt: timestamp, delivery: 'steer', clientUserMessageId: expected.clientUserMessageId,
          text: 'Synthetic marker-free late reply'
        }
      }
    }
  }
  // A configured exporter is required to supply every original object. Never
  // repair a producer witness by filling metadata or removing its extra text.
  if (witnessPath.trim().length === 0) throw new Error('Native hookup witness path is empty')
  const value: unknown = JSON.parse(nativeWitnessFs.readFileSync(witnessPath, 'utf8'))
  const record = (candidate: unknown): candidate is Record<string, unknown> =>
    candidate !== null && typeof candidate === 'object' && !Array.isArray(candidate)
  if (!record(value) || !record(value.rawBuilder) || !record(value.projected) ||
      !record(value.expected) || !record(value.promotion)) {
    throw new Error('Native hookup witness requires rawBuilder/projected/expected/promotion objects')
  }
  for (const event of [value.projected, value.expected, value.promotion]) {
    if (!Number.isSafeInteger(event.seq) || typeof event.threadId !== 'string' ||
        typeof event.turnId !== 'string' || typeof event.timestamp !== 'string') {
      throw new Error('Native hookup witness is missing an original event binding')
    }
  }
  if (value.rawBuilder.kind !== 'turn_steered' || value.projected.kind !== 'turn_steered' ||
      value.expected.kind !== 'turn_steered' || 'text' in value.expected ||
      value.promotion.kind !== 'item_created' || !record(value.promotion.item) ||
      value.promotion.item.kind !== 'user_message' || value.promotion.item.status !== 'completed' ||
      value.promotion.threadId !== value.expected.threadId) {
    throw new Error('Native hookup witness has an unexpected event purpose or owner')
  }
  return {
    rawBuilder: value.rawBuilder, projected: value.projected,
    expected: value.expected, promotion: value.promotion
  }
}


describe('native steering producer public SSE witness', () => {
  const witness = nativeHookupEventWitness()
  const owner = witness.expected.threadId as string
  const decode = (event: Record<string, unknown>, frameId = String(event.seq)) =>
    projectPublicRuntimeSseBlock(frame(frameId, String(event.kind), event), owner,
      new PublicRuntimeEventFilter())

  it('TARGET: delivers the original Go projected steering metadata without adding or removing fields', () => {
    // With the unfixed Go producer, projected still has extra text: this must
    // stay RED rather than normalizing the actual witness in the test.
    const decision = decode(witness.projected)
    expect(decision?.status).toBe('emit')
    expect(decision).toEqual({ status: 'emit', seq: witness.expected.seq, event: witness.expected })
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup', events: [witness.projected] })).toBe(true)
    expect(witness.projected).toEqual(witness.expected)
  })

  it('keeps raw builder text outside the closed public steering contract', () => {
    expect(typeof witness.rawBuilder.text).toBe('string')
    expect(isClosedPublicRuntimeSseEvent(witness.rawBuilder)).toBe(false)
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup', events: [witness.rawBuilder] })).toBe(false)
  })

  it('delivers a real completed user-message promotion as item_created', () => {
    expect(decode(witness.promotion)).toEqual({
      status: 'emit', seq: witness.promotion.seq, event: witness.promotion
    })
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup', events: [witness.promotion] })).toBe(true)
  })

  it.each([
    ['text', { text: 'SYNTHETIC_NATIVE_HOOKUP_FORBIDDEN_BODY' }],
    ['unknown field', { unexpectedField: true }],
    ['illegal status', { status: 'queued' }]
  ] as const)('rejects steering %s without using a generic sanitizer to repair it', (_name, fields) => {
    const event = { ...witness.expected, ...fields }
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup', events: [event] })).toBe(false)
    expect(decode(event)).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
  })

  it('binds steering transport to the actual subscribed thread and exact sequence', () => {
    expect(decode({ ...witness.expected, threadId: `${owner}-foreign` })).toEqual({
      status: 'invalid', reason: 'event_thread_mismatch'
    })
    expect(decode(witness.expected, String(Number(witness.expected.seq) + 1))).toEqual({
      status: 'invalid', reason: 'event_sequence_mismatch'
    })
    const invalidSequence = { ...witness.expected, seq: -1 }
    expect(decode(invalidSequence, String(witness.expected.seq))).toEqual({ status: 'invalid', reason: 'event_sequence_mismatch' })
    expect(decode(invalidSequence)).toEqual({ status: 'invalid', reason: 'malformed_frame' })
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup', events: [invalidSequence] })).toBe(false)
  })

  it.each([
    { status: 'queued' },
    { threadId: 'synthetic-foreign-promotion-thread' },
    { id: 'synthetic-foreign-promotion-item' },
    { unexpectedField: true }
  ])('rejects an unbound or open promotion item %j', fields => {
    const event = { ...witness.promotion, item: { ...(witness.promotion.item as Record<string, unknown>), ...fields } }
    expect(decode(event)).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup', events: [event] })).toBe(false)
  })
})


describe('continued Host question SSE correlation', () => {
  const inputId = `input_${'123456789012'}${'a'.repeat(52)}`
  const timestamp = '2026-10-08T00:00:00Z'
  const question = { id: `${inputId}_1`, header: 'Path', question: 'Choose', multiSelect: false,
    options: [{ id: `${inputId}_1_option_1`, label: 'Fast', description: '' }] }
  const resolved = { kind: 'user_input_resolved', seq: 183, timestamp,
    threadId: 'thread-strict', turnId: 'turn-strict', inputId, status: 'continued',
    prompt: 'Choose', questions: [question] }
  const item = { id: 'item-continued-question', threadId: resolved.threadId, turnId: resolved.turnId,
    kind: 'user_input', role: 'system', status: 'continued', createdAt: timestamp, finishedAt: timestamp,
    inputId, prompt: 'Choose', questions: [question], timedWait: { timeoutSeconds: 1, deadlineAt: timestamp } }
  const completed = { kind: 'item_completed', seq: 184, timestamp, threadId: item.threadId,
    turnId: item.turnId, itemId: item.id, item }

  it.each(['submitted', 'cancelled', 'continued'])('delivers %s resolution without reclassifying Host IDs as prose', status => {
    const event = { ...resolved, status }
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'continued-question', events: [event] })).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame(String(event.seq), event.kind, event), event.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event, seq: event.seq })
  })

  it('delivers the continued item_completed through both shared IPC guards', () => {
    expect(isPublicSseIpcPayload({ streamId: 'continued-question', events: [completed] })).toBe(true)
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'continued-question', events: [completed] })).toBe(true)
    expect(projectPublicRuntimeSseBlock(frame(String(completed.seq), completed.kind, completed), item.threadId,
      new PublicRuntimeEventFilter())).toEqual({ status: 'emit', event: completed, seq: completed.seq })
  })

  it('keeps detached IDs, wrong roles and display PII outside continued SSE', () => {
    for (const unsafe of [
      { ...completed, item: { ...item, role: 'user' } },
      { ...completed, item: { ...item, questions: [{ ...question, id: `${inputId}_2` }] } },
      { ...resolved, questions: [{ ...question, question: 'account=6222020202020202020' }] },
      { ...resolved, questions: [{ ...question, options: [{ ...question.options[0], description: 'phone=13800138000' }] }] },
      { ...resolved, kind: 'user_input_requested' }
    ]) {
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'continued-question', events: [unsafe] })).toBe(false)
      expect(projectPublicRuntimeSseBlock(frame(String(unsafe.seq), unsafe.kind, unsafe), resolved.threadId,
        new PublicRuntimeEventFilter())).not.toMatchObject({ status: 'emit' })
    }
  })
})

describe('first-validation SSE diagnostics', () => {
  function toolCallEvent(): Record<string, unknown> {
    return {
      ...heartbeat(), kind: 'item_created', turnId: 'turn-diagnostic', itemId: 'item-diagnostic',
      item: {
        id: 'item-diagnostic', threadId: 'thread-strict', turnId: 'turn-diagnostic',
        role: 'tool', status: 'running', createdAt: '2026-07-14T00:00:00.000Z',
        kind: 'tool_call', toolName: 'read_file', callId: 'call-diagnostic', toolKind: 'tool_call',
        arguments: {
          schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only',
          messageKey: 'tool_arguments_withheld', privatePayloadWithheld: true,
          factAnswerAllowed: false, evidenceAuthority: false
        }
      }
    }
  }

  it('records the original failed item parse once before touching the stateful filter', () => {
    const event = toolCallEvent()
    const item = event.item as Record<string, unknown>
    const args = item.arguments as Record<string, unknown>
    args.privatePayloadWithheld = false
    const parse = vi.spyOn(ToolCallTurnItem, 'safeParse')
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    const diagnostic = vi.fn()
    try {
      expect(projectPublicRuntimeSseBlock(frame('7', 'item_created', event), 'thread-strict', filter, diagnostic))
        .toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(parse).toHaveBeenCalledTimes(1)
      expect(push).not.toHaveBeenCalled()
      expect(diagnostic).toHaveBeenCalledExactlyOnceWith({
        validation: 'input', stage: 'item_schema', reason: 'schema_invalid_value',
        fieldPaths: ['item.arguments.privatePayloadWithheld']
      })
    } finally {
      parse.mockRestore()
    }
  })

  it.each([
    ['event', 'event_contract', '$'], ['item', 'item_schema', 'item'],
    ['arguments', 'item_schema', 'item.arguments']
  ])('reports only a known container for unknown %s fields', (owner, stage, fieldPath) => {
    const event = toolCallEvent()
    const item = event.item as Record<string, unknown>
    const target = owner === 'event' ? event : owner === 'item' ? item : item.arguments as Record<string, unknown>
    // Deterministic synthetic bearer fixture; never use a real credential.
    const syntheticPrivateBearer = 'Bearer ' + 'x'.repeat(23)
    target.PRIVATE_UNKNOWN_KEY_SENTINEL = {
      text: 'PRIVATE_BODY_SENTINEL', arguments: '/tmp/PRIVATE_PATH_SENTINEL',
      credential: syntheticPrivateBearer
    }
    const diagnostic = vi.fn()
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    const decision = projectPublicRuntimeSseBlock(frame('7', 'item_created', event), 'thread-strict', filter, diagnostic)
    expect(decision).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith({
      validation: 'input', stage, reason: 'unknown_fields', fieldPaths: [fieldPath]
    })
    expect(push).not.toHaveBeenCalled()
    expect(JSON.stringify({ decision, diagnostics: diagnostic.mock.calls })).not.toContain('PRIVATE_')
  })

  it('truncates unknown path segments, normalizes indexes, and never reads issue contents', () => {
    const issue = (path: PropertyKey[]) => Object.defineProperties({ code: 'unrecognized_keys', path },
      Object.fromEntries(['keys', 'message', 'input', 'errors'].map((key) => [key, {
        get() { throw new Error('PRIVATE_ISSUE_CONTENT_SENTINEL') }
      }])))
    const parse = vi.spyOn(ToolCallTurnItem, 'safeParse').mockReturnValueOnce({
      success: false,
      error: { issues: [
        issue(['arguments', 'PRIVATE_PATH_SEGMENT', 'status']),
        issue(['arguments.schemaVersion']),
        issue(['questions', 987654321, 'id']),
        ...Array.from({ length: 9 }, () => issue(['PRIVATE_EXCESS_SEGMENT'])),
        Object.defineProperty({ code: 'unrecognized_keys' }, 'path', {
          get() { throw new Error('PRIVATE_OVER_BUDGET_ISSUE') }
        })
      ] }
    } as never)
    const diagnostic = vi.fn()
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    try {
      expect(projectPublicRuntimeSseBlock(frame('7', 'item_created', toolCallEvent()), 'thread-strict', filter, diagnostic))
        .toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(parse).toHaveBeenCalledTimes(1)
      expect(push).not.toHaveBeenCalled()
      expect(diagnostic).toHaveBeenCalledExactlyOnceWith({
        validation: 'input', stage: 'item_schema', reason: 'unknown_fields',
        fieldPaths: ['item.arguments', 'item', 'item.questions.[].id']
      })
      const metadata = diagnostic.mock.calls[0][0]
      expect(Object.isFrozen(metadata)).toBe(true)
      expect(Object.isFrozen(metadata.fieldPaths)).toBe(true)
      const serialized = JSON.stringify(metadata)
      expect(serialized).not.toContain('PRIVATE_')
      expect(serialized).not.toContain('987654321')
      expect(serialized).not.toContain('item.arguments.status')
    } finally {
      parse.mockRestore()
    }
  })

  it('keeps the first binding guard ahead of subsequent timestamp and schema failures', () => {
    const event = toolCallEvent()
    const item = event.item as Record<string, unknown>
    item.threadId = 'PRIVATE_DETACHED_THREAD'
    item.createdAt = 'PRIVATE_TIMESTAMP_VALUE'
    const parse = vi.spyOn(ToolCallTurnItem, 'safeParse')
    const diagnostic = vi.fn()
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    try {
      expect(projectPublicRuntimeSseBlock(frame('7', 'item_created', event), 'thread-strict', filter, diagnostic))
        .toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
      expect(parse).not.toHaveBeenCalled()
      expect(push).not.toHaveBeenCalled()
      expect(diagnostic).toHaveBeenCalledExactlyOnceWith({
        validation: 'input', stage: 'item_binding', reason: 'binding_mismatch', fieldPaths: ['item.threadId']
      })
    } finally {
      parse.mockRestore()
    }
  })

  it('preserves valid event and seq while calling the filter only once', () => {
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    const diagnostic = vi.fn()
    const event = heartbeat()
    expect(projectPublicRuntimeSseBlock(frame('7', 'heartbeat', event), 'thread-strict', filter, diagnostic))
      .toEqual({ status: 'emit', event, seq: 7 })
    expect(push).toHaveBeenCalledExactlyOnceWith(event)
    expect(diagnostic).not.toHaveBeenCalled()
  })

  it.each([
    ['filter rejection', null, 'projection_filter', 'projection_rejected', '$'],
    ['projected binding', heartbeat({ seq: 8 }), 'projected_binding', 'binding_mismatch', 'seq'],
    ['projected contract', heartbeat({ PRIVATE_EXTRA_FIELD: 'PRIVATE_PROJECTED_BODY' }),
      'event_contract', 'unknown_fields', '$']
  ] as const)('uses the single original filter result for %s', (_label, projected, stage, reason, fieldPath) => {
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push').mockReturnValueOnce(projected)
    const diagnostic = vi.fn()
    expect(projectPublicRuntimeSseBlock(frame('7', 'heartbeat', heartbeat()), 'thread-strict', filter, diagnostic))
      .toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(push).toHaveBeenCalledTimes(1)
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith({
      validation: 'projected', stage, reason, fieldPaths: [fieldPath]
    })
    expect(JSON.stringify(diagnostic.mock.calls)).not.toContain('PRIVATE_')
  })

  it('isolates observer failures and leaves the next valid decision unchanged', () => {
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    const observer = vi.fn(() => { throw new Error('PRIVATE_OBSERVER_ERROR') })
    expect(projectPublicRuntimeSseBlock(frame('7', 'heartbeat', heartbeat({ PRIVATE_EXTRA: 'PRIVATE_BODY' })),
      'thread-strict', filter, observer)).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(observer).toHaveBeenCalledTimes(1)
    expect(push).not.toHaveBeenCalled()
    const next = heartbeat({ seq: 8 })
    expect(projectPublicRuntimeSseBlock(frame('8', 'heartbeat', next), 'thread-strict', filter, observer))
      .toEqual({ status: 'emit', event: next, seq: 8 })
    expect(push).toHaveBeenCalledTimes(1)
    expect(observer).toHaveBeenCalledTimes(1)
  })

  it('preserves private withheld, revocation, and atomic terminal decisions', () => {
    const filter = new PublicRuntimeEventFilter()
    const push = vi.spyOn(filter, 'push')
    const diagnostic = vi.fn()
    expect(projectPublicRuntimeSseBlock(frame('8', 'assistant_reasoning_delta', {
      ...heartbeat({ seq: 8, kind: 'assistant_reasoning_delta' }), text: 'PRIVATE_REASONING'
    }), 'thread-strict', filter, diagnostic)).toEqual({ status: 'withheld', seq: 8, reason: 'restricted_content' })
    expect(push).not.toHaveBeenCalled()
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith({
      validation: 'input', stage: 'public_content', reason: 'restricted_content', fieldPaths: ['kind']
    })
    diagnostic.mockClear()
    const revoked = projectionRevoked()
    expect(projectPublicRuntimeSseBlock(`event: public_projection_revoked\ndata: ${JSON.stringify(revoked)}`,
      'thread-strict', filter, diagnostic)).toEqual({ status: 'revoke', event: revoked })
    expect(push).not.toHaveBeenCalled()
    const batch = generalTerminalBatch()
    expect(projectPublicRuntimeSseBlock(frame(String(batch.seq), 'general_terminal_batch', batch),
      'thread-strict', filter, diagnostic)).toEqual({ status: 'emit', seq: batch.seq, event: batch })
    expect(push).toHaveBeenCalledTimes(1)
    expect(diagnostic).not.toHaveBeenCalled()
  })
})


describe('logical-step strict public SSE', () => {
  const id = { schemaVersion: 1, logicalStepId: '11111111-1111-4111-8111-111111111111' }
  const close = { ...id, assistantMessageAssembled: true, llmMs: 40, ttftMs: 10, decodeMs: 30, decodeTokens: 7 }
  const wire = (stage = 'logical_step_closed', details: unknown = close, overrides: Record<string, unknown> = {}) => ({
    kind: 'pipeline_stage', seq: 62, timestamp: '2026-10-12T00:00:00Z', threadId: 'thread-strict', turnId: 'turn-logical',
    stage, label: stage === 'logical_step_started' ? 'Logical step started' :
      stage === 'logical_step_assembled' ? 'Logical step assembled' : 'Logical step closed', details, ...overrides })
  it('emits each actual stage exactly, including zero timing and failed/aborted unassembled closes', () => {
    for (const event of [wire('logical_step_started', id), wire('logical_step_assembled', id), wire(), wire('logical_step_closed', { ...id, assistantMessageAssembled: false }),
      wire('logical_step_closed', { ...id, assistantMessageAssembled: true }),
      wire('logical_step_closed', { ...close, llmMs: 0, ttftMs: 0, decodeMs: 0, decodeTokens: 0 }),
      wire('logical_step_closed', { ...close, llmMs: Number.MAX_SAFE_INTEGER, ttftMs: 0,
        decodeMs: Number.MAX_SAFE_INTEGER, decodeTokens: Number.MAX_SAFE_INTEGER })]) {
      expect(projectPublicRuntimeSseBlock(frame('62', 'pipeline_stage', event), 'thread-strict', new PublicRuntimeEventFilter()))
        .toEqual({ status: 'emit', seq: 62, event })
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'logical-stream', events: [event] })).toBe(true)
    }
  })
  it('rejects open or impossible details, absent fields, mismatched labels and trace', () => {
    for (const details of [null, {}, { ...close, schemaVersion: 2 }, { ...close, logicalStepId: '11111111-1111-1111-8111-111111111111' },
      { ...close, logicalStepId: 'AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA' }, { ...close, assistantMessageAssembled: null },
      { ...close, assistantMessageAssembled: false }, { ...close, llmMs: null }, { ...close, llmMs: -1 },
      { ...close, llmMs: 0.5 }, { ...close, llmMs: Number.MAX_SAFE_INTEGER + 1 }, { ...close, ttftMs: 41 },
      { ...close, decodeTokens: undefined }, { ...close, decodeMs: undefined }, { ...close, ttftMs: undefined },
      { ...close, llmMs: undefined }, { ...close, decodeMs: 29 }, { ...close, extra: 'unadmitted' }]) {
      expect(isClosedPublicRuntimeSseEvent(wire('logical_step_closed', details))).toBe(false)
    }
    for (const event of [wire('logical_step_started', { ...id, llmMs: 0 }), wire(undefined, undefined, { details: undefined }),
      wire(undefined, undefined, { label: 'Wrong label' }), wire(undefined, undefined, { turnId: undefined }),
      wire(undefined, undefined, { attempt: 1 }), wire(undefined, undefined, { maxAttempt: 2 }),
      wire(undefined, undefined, { trace: { sse_sent_at: 1, sse_live_emitted_at: 1 } }),
      wire(undefined, undefined, { itemId: 'item-extra' }), wire(undefined, undefined, { extra: 0 })]) {
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'logical-stream', events: [event] })).toBe(false)
    }
  })
  it('keeps assembly identity-only and rejects unadmitted metadata at the transport seam', () => {
    for (const details of [{}, { ...id, schemaVersion: 2 }, { ...id, logicalStepId: 'invalid' },
      { ...id, assistantMessageAssembled: true }, { ...id, llmMs: 0 },
      { ...id, body: 'PRIVATE_BODY' }, { ...id, StepBindingHMAC: 'PRIVATE_HMAC' }]) {
      expect(isClosedPublicRuntimeSseEvent(wire('logical_step_assembled', details))).toBe(false)
    }
    for (const patch of [{ label: 'wrong' }, { turnId: undefined }, { itemId: 'extra' }, { child: {} },
      { attempt: 1 }, { maxAttempt: 2 }, { trace: {} }, { body: 'PRIVATE_BODY' }]) {
      const event = wire('logical_step_assembled', id, patch)
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'logical-stream', events: [event] })).toBe(false)
      const decision = projectPublicRuntimeSseBlock(frame('62', 'pipeline_stage', event), 'thread-strict', new PublicRuntimeEventFilter())
      expect(decision).not.toBeNull()
      if (decision === null) throw new Error('malformed assembly must produce an explicit rejection')
      expect(decision.status).toBe('invalid')
    }
  })
  it('admits literal logical metrics version only on post_start while preserving maxModelSteps', () => {
    const post = { ...wire(), stage: 'post_start', label: 'Post-Start', details: { maxModelSteps: 5, logicalStepMetricsVersion: 1 } }
    expect(projectPublicRuntimeSseBlock(frame('62', 'pipeline_stage', post), 'thread-strict', new PublicRuntimeEventFilter()))
      .toEqual({ status: 'emit', seq: 62, event: post })
    for (const version of [null, 0, 2, '1', true]) {
      expect(isClosedPublicRuntimeSseEvent({ ...post, details: { maxModelSteps: 5, logicalStepMetricsVersion: version } })).toBe(false)
    }
    expect(isClosedPublicRuntimeSseEvent({ ...post, stage: 'response_received', label: 'Response Received' })).toBe(false)
    expect(isClosedPublicRuntimeSseEvent({ ...post, details: { maxModelSteps: 5 } })).toBe(true)
  })
})
