import { describe, expect, it } from 'vitest'
import { LogicalStepStartedDetailsV1Schema, LogicalStepClosedDetailsV1Schema, PipelineStageEvent, RuntimeEvent } from './events.js'
import { RuntimeUsageResponseSchema, UsageSnapshotSchema, ProviderBillingV1Schema, ConversationMetricsV1Schema } from './usage.js'

const completeZeroUsd = {
  promptTokens: 1,
  completionTokens: 1,
  totalTokens: 2,
  cacheHitRate: null,
  turns: 1,
  priceConfigured: true,
  costUsd: 0,
  costCny: 0,
  costEstimateStatus: 'complete',
  costKnownCurrencies: ['USD']
}

describe('usage cost coverage contract', () => {
  it('keeps a known zero currency and rejects inconsistent coverage at the public response seam', () => {
    const response = { total: completeZeroUsd, perThread: [], bySource: [] }
    expect(RuntimeUsageResponseSchema.safeParse(response).success).toBe(true)
    for (const mutation of [
      { costKnownCurrencies: ['CNY', 'USD'] },
      { costKnownCurrencies: ['USD', 'USD'] },
      { costKnownCurrencies: undefined },
      { priceConfigured: false },
      { costEstimateStatus: 'partial' },
      { costEstimateStatus: 'none', costKnownCurrencies: [], priceConfigured: false },
      { costUsd: 0.01, costKnownCurrencies: ['CNY'] }
    ]) {
      expect(RuntimeUsageResponseSchema.safeParse({ ...response, total: { ...completeZeroUsd, ...mutation } }).success).toBe(false)
    }
  })

  it('accepts old response fields but does not infer a currency from priceConfigured', () => {
    const legacy = { ...completeZeroUsd }
    Reflect.deleteProperty(legacy, 'costEstimateStatus')
    Reflect.deleteProperty(legacy, 'costKnownCurrencies')
    expect(RuntimeUsageResponseSchema.safeParse({ total: legacy, perThread: [], bySource: [] }).success).toBe(true)
  })

  it('accepts host-only zero usage as unknown and rejects an empty status enum', () => {
    const hostOnly = {
      promptTokens: 0, completionTokens: 0, reasoningTokens: 0, totalTokens: 0,
      cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
      cacheMissReasons: [], cacheSuggestions: [], turns: 1,
      costUsd: 0, costCny: 0, priceConfigured: false,
      cacheSavingsUsd: 0, cacheSavingsCny: 0, tokenEconomySavingsTokens: 0,
      costEstimateStatus: 'unknown', costKnownCurrencies: []
    }
    expect(UsageSnapshotSchema.strict().safeParse(hostOnly).success).toBe(true)
    expect(UsageSnapshotSchema.strict().safeParse({ ...hostOnly, costEstimateStatus: '' }).success).toBe(false)
  })
})

function reportedCounter(value: number, known: number, attempts: number, complete: boolean) {
  return { reportedValue: value, knownAttemptCount: known, attemptCount: attempts, complete }
}

describe('conversation billing coverage contract', () => {
  const partial = {
    inventoryComplete: true, closed: false, attemptCount: 2, settledAttemptCount: 1,
    inputTokens: reportedCounter(100, 1, 2, false),
    outputTokens: reportedCounter(10, 1, 2, false),
    uncachedInputTokens: reportedCounter(15, 1, 2, false),
    cacheReadTokens: reportedCounter(80, 1, 2, false),
    cacheWriteTokens: reportedCounter(5, 1, 2, false),
    reasoningTokens: reportedCounter(0, 1, 2, false),
    totalTokens: reportedCounter(110, 1, 2, false)
  }

  it('retains the settled subtotal while a started attempt is pending', () => {
    expect(ProviderBillingV1Schema.parse(partial).totalTokens).toEqual(reportedCounter(110, 1, 2, false))
    expect(ProviderBillingV1Schema.safeParse({ ...partial, totalTokens: { ...partial.totalTokens, complete: true } }).success).toBe(false)
    expect(ProviderBillingV1Schema.safeParse({ ...partial, closed: true }).success).toBe(false)
  })

  it('preserves explicit reasoning zero and rejects cross-attempt total coverage', () => {
    expect(ProviderBillingV1Schema.parse(partial).reasoningTokens.knownAttemptCount).toBe(1)
    expect(ProviderBillingV1Schema.safeParse({ ...partial, totalTokens: reportedCounter(110, 2, 2, true) }).success).toBe(false)
    expect(ProviderBillingV1Schema.safeParse({ ...partial, reasoningTokens: reportedCounter(1, 0, 2, false) }).success).toBe(false)
  })

  it('requires an exact closure for a complete empty inventory', () => {
    const empty = Object.fromEntries(Object.entries(partial).map(([key, value]) =>
      [key, typeof value === 'object' ? reportedCounter(0, 0, 0, true) : value]))
    Object.assign(empty, { attemptCount: 0, settledAttemptCount: 0, closed: true })
    expect(ProviderBillingV1Schema.safeParse(empty).success).toBe(true)
    expect(ProviderBillingV1Schema.safeParse({ ...empty, closed: false }).success).toBe(false)
  })
})

describe('conversation timing pairing', () => {
  it('keeps historical step count unknown and validates paired decode samples', () => {
    const metrics = { turnCount: 1, stepCount: null,
      llmMs: { reportedValue: 4000, knownStepCount: 1, complete: false },
      toolMs: { reportedValue: 2000, knownCallCount: 1, callCount: 2, complete: false },
      ttftMs: 200, ttftSteps: 1, decodeMs: 3800, decodeTokens: 30, decodeSteps: 1 }
    expect(ConversationMetricsV1Schema.parse(metrics).stepCount).toBeNull()
    expect(ConversationMetricsV1Schema.safeParse({ ...metrics, decodeSteps: 0 }).success).toBe(false)
    expect(ConversationMetricsV1Schema.safeParse({ ...metrics, llmMs: { ...metrics.llmMs, complete: true } }).success).toBe(false)
  })
})


describe('reported presence is independent of final qualification', () => {
 it('accepts all-reported provisional output but rejects a fabricated complete total', () => {
  const counters = Object.fromEntries(['inputTokens','outputTokens','uncachedInputTokens','cacheReadTokens','cacheWriteTokens','reasoningTokens','totalTokens'].map(key => [key, reportedCounter(0, 1, 1, false)]))
  const provisional = { ...counters, inventoryComplete:true, closed:true, attemptCount:1, settledAttemptCount:1 }
  expect(ProviderBillingV1Schema.safeParse(provisional).success).toBe(true)
  expect(ProviderBillingV1Schema.safeParse({...provisional,totalTokens:reportedCounter(0,1,1,true)}).success).toBe(false)
 })
})


describe('logical-step metrics wire', () => {
  const logicalStepId = '11111111-1111-4111-8111-111111111111'
  const start = { schemaVersion: 1, logicalStepId }
  const close = { ...start, assistantMessageAssembled: true, llmMs: 40, ttftMs: 10, decodeMs: 30, decodeTokens: 7 }
  const event = (stage: 'logical_step_started' | 'logical_step_assembled' | 'logical_step_closed', details: unknown) => ({
    kind: 'pipeline_stage', seq: 1, timestamp: '2026-10-12T00:00:00Z', threadId: 'thread-logical', turnId: 'turn-logical',
    stage, label: stage === 'logical_step_started' ? 'Logical step started' :
      stage === 'logical_step_assembled' ? 'Logical step assembled' : 'Logical step closed', details })
  it('admits immutable starts and closes, including zero and an unassembled close without samples', () => {
    expect(LogicalStepStartedDetailsV1Schema.parse(start)).toEqual(start)
    for (const details of [close, { ...close, llmMs: 0, ttftMs: 0, decodeMs: 0, decodeTokens: 0 },
      { ...start, assistantMessageAssembled: false }, { ...start, assistantMessageAssembled: true }]) {
      expect(LogicalStepClosedDetailsV1Schema.parse(details)).toEqual(details)
      expect(PipelineStageEvent.parse(event('logical_step_closed', details)).details).toEqual(details)
      expect(RuntimeEvent.parse(event('logical_step_closed', details)).kind).toBe('pipeline_stage')
    }
    expect(RuntimeEvent.parse(event('logical_step_started', start)).kind).toBe('pipeline_stage')
    expect(RuntimeEvent.parse(event('logical_step_assembled', start))).toEqual(event('logical_step_assembled', start))
  })
  it('rejects incomplete identities and impossible timing instead of synthesizing samples', () => {
    for (const details of [{ ...close, schemaVersion: 2 }, { ...close, logicalStepId: '11111111-1111-4111-C111-111111111111' },
      { ...close, assistantMessageAssembled: undefined }, { ...close, assistantMessageAssembled: false },
      { ...close, llmMs: null }, { ...close, llmMs: -1 }, { ...close, llmMs: Number.MAX_SAFE_INTEGER + 1 },
      { ...close, ttftMs: 41 }, { ...close, llmMs: undefined }, { ...close, decodeTokens: undefined },
      { ...close, decodeMs: undefined }, { ...close, ttftMs: undefined }, { ...close, decodeMs: 29 },
      { ...close, privatePayload: 'unadmitted' }]) {
      expect(LogicalStepClosedDetailsV1Schema.safeParse(details).success).toBe(false)
      expect(RuntimeEvent.safeParse(event('logical_step_closed', details)).success).toBe(false)
    }
    expect(LogicalStepStartedDetailsV1Schema.safeParse({ ...start, assistantMessageAssembled: true }).success).toBe(false)
    for (const patch of [{ label: 'wrong' }, { turnId: undefined }, { attempt: 1 }, { itemId: 'item-extra' }]) {
      expect(PipelineStageEvent.safeParse({ ...event('logical_step_started', start), ...patch }).success).toBe(false)
    }
  })
  it('rejects extra assembly fields and envelopes without opening legacy pipeline replay', () => {
    for (const details of [{}, { ...start, schemaVersion: 2 }, { ...start, logicalStepId: 'invalid' },
      { ...start, assistantMessageAssembled: true }, { ...start, llmMs: 0 },
      { ...start, body: 'PRIVATE_BODY' }, { ...start, StepBindingHMAC: 'PRIVATE_HMAC' }]) {
      expect(RuntimeEvent.safeParse(event('logical_step_assembled', details)).success).toBe(false)
    }
    for (const patch of [{ label: 'wrong' }, { timestamp: 'invalid' }, { turnId: undefined }, { itemId: 'extra' }, { child: {} },
      { attempt: 1 }, { maxAttempt: 2 }, { trace: {} }, { body: 'PRIVATE_BODY' }]) {
      expect(RuntimeEvent.safeParse({ ...event('logical_step_assembled', start), ...patch }).success).toBe(false)
    }
    const legacy = { ...event('logical_step_started', start), stage: 'pre_send', label: 'Pre-Send',
      details: {}, trace: { provider_request_sent_at: 1 } }
    expect(RuntimeEvent.parse(legacy)).not.toHaveProperty('trace')
  })
  it('keeps legacy/mixed counts explicitly unknown alongside independent proven samples', () => {
    const mixed = { turnCount: null, stepCount: null,
      llmMs: { reportedValue: 40, knownStepCount: 1, complete: false },
      toolMs: { reportedValue: 10, knownCallCount: 1, callCount: 1, complete: true },
      ttftMs: 10, ttftSteps: 1, decodeMs: 30, decodeTokens: 7, decodeSteps: 1 }
    expect(ConversationMetricsV1Schema.parse(mixed)).toEqual(mixed)
    const { turnCount: _turnCount, ...missing } = mixed
    expect(ConversationMetricsV1Schema.safeParse(missing).success).toBe(false)
    expect(ConversationMetricsV1Schema.safeParse({ ...mixed, turnCount: -1 }).success).toBe(false)
  })
})
