import { describe, expect, it } from 'vitest'
import { RuntimeUsageResponseSchema } from './usage.js'

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
})
