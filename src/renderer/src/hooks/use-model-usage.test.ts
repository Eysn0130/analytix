import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildModelUsagePath,
  loadModelUsage,
  normalizeModelUsageResponse
} from './use-model-usage'
import { formatCost } from './use-thread-usage'
import { USAGE_REQUEST_TIMEOUT_MS } from './usage-response'

type RuntimeRequest = (path: string, method?: string) => Promise<{ ok: boolean; status: number; body: string }>

function setRuntimeRequest(runtimeRequest: RuntimeRequest): void {
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: {
      analytix: {
        runtime: { runtimeRequest }
      }
    }
  })
}

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  Reflect.deleteProperty(globalThis, 'window')
})

describe('model usage helpers', () => {
  it('builds an encoded model usage request path', () => {
    expect(
      buildModelUsagePath({
        from: '2026-05-01',
        to: '2026-05-31',
        timezone: 'Asia/Shanghai'
      })
    ).toBe('/v1/usage?group_by=model&from=2026-05-01&to=2026-05-31&timezone=Asia%2FShanghai')
  })

  it('normalizes model buckets and daily chart buckets', () => {
    const normalized = normalizeModelUsageResponse({
      group_by: 'model',
      from: '2026-05-01',
      to: '2026-05-02',
      timezone: 'UTC',
      buckets: [
        {
          model: 'Opus 4.8',
          provider: 'anthropic',
          input_tokens: 100,
          output_tokens: 30,
          total_tokens: 130,
          price_configured: true,
          cache_savings_usd: 0.006,
          cache_savings_cny: 0.0432,
          token_economy_savings_tokens: 2048,
          token_economy_savings_usd: 0.0009,
          token_economy_savings_cny: 0.0063,
          turns: 2,
          thread_count: 1,
          cache_hit_rate: 0.5
        }
      ],
      days: [
        { date: '2026-05-01', total_tokens: 130, turns: 2, priceConfigured: true },
        { date: '2026-05-02', total_tokens: 0, turns: 0 }
      ],
      totals: { total_tokens: 130, turns: 2, days: 2, active_days: 1, thread_count: 1, price_configured: true }
    })

    expect(normalized.buckets[0]).toMatchObject({
      model: 'Opus 4.8',
      provider: 'anthropic',
      inputTokens: 100,
      outputTokens: 30,
      totalTokens: 130,
      priceConfigured: true,
      tokenEconomySavingsTokens: 2048,
      turns: 2,
      threadCount: 1,
      cacheHitRate: 0.5
    })
    expect(normalized.days.map((bucket) => bucket.date)).toEqual(['2026-05-01', '2026-05-02'])
    expect(normalized.days[0]?.priceConfigured).toBe(true)
    expect(normalized.totals.priceConfigured).toBe(true)
    expect(normalized.totals.activeDays).toBe(1)
  })

  it('keeps missing model pricing distinct from explicit configured zero cost', () => {
    const unpriced = normalizeModelUsageResponse({
      group_by: 'model',
      buckets: [
        {
          model: 'custom-model',
          provider: 'custom',
          total_tokens: 130,
          cost_usd: 0,
          cost_cny: 0,
          price_configured: false,
          turns: 2
        }
      ],
      days: [
        {
          date: '2026-05-01',
          total_tokens: 130,
          cost_usd: 0,
          cost_cny: 0,
          price_configured: false,
          turns: 2
        }
      ],
      totals: {
        total_tokens: 130,
        cost_usd: 0,
        cost_cny: 0,
        price_configured: false,
        turns: 2,
        days: 1,
        active_days: 1
      }
    })
    expect(formatCost(unpriced.buckets[0]?.costUsd, 'en', unpriced.buckets[0]?.costCny, unpriced.buckets[0]?.priceConfigured)).toBe('Cost unavailable')
    expect(formatCost(unpriced.days[0]?.costUsd, 'zh-CN', unpriced.days[0]?.costCny, unpriced.days[0]?.priceConfigured)).toBe('费用未知')
    expect(formatCost(unpriced.totals.costUsd, 'en', unpriced.totals.costCny, unpriced.totals.priceConfigured)).toBe('Cost unavailable')

    const freeConfigured = normalizeModelUsageResponse({
      group_by: 'model',
      buckets: [
        {
          model: 'free-model',
          provider: 'custom',
          total_tokens: 130,
          cost_usd: 0,
          cost_cny: 0,
          price_configured: true,
          turns: 2
        }
      ],
      days: [
        {
          date: '2026-05-01',
          total_tokens: 130,
          cost_usd: 0,
          cost_cny: 0,
          price_configured: true,
          turns: 2
        }
      ],
      totals: {
        total_tokens: 130,
        cost_usd: 0,
        cost_cny: 0,
        price_configured: true,
        turns: 2,
        days: 1,
        active_days: 1
      }
    })
    expect(formatCost(freeConfigured.buckets[0]?.costUsd, 'en', freeConfigured.buckets[0]?.costCny, freeConfigured.buckets[0]?.priceConfigured)).toBe('$0.0000')
    expect(formatCost(freeConfigured.days[0]?.costUsd, 'zh-CN', freeConfigured.days[0]?.costCny, freeConfigured.days[0]?.priceConfigured)).toBe('￥0.0000')
    expect(formatCost(freeConfigured.totals.costUsd, 'en', freeConfigured.totals.costCny, freeConfigured.totals.priceConfigured)).toBe('$0.0000')
  })

  it('loads model usage from the runtime request bridge', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        group_by: 'model',
        from: '2026-05-01',
        to: '2026-05-01',
        timezone: 'UTC',
        buckets: [{ model: 'Opus 4.8', provider: 'anthropic', total_tokens: 10, turns: 1 }],
        days: [{ date: '2026-05-01', total_tokens: 10, turns: 1 }],
        totals: { total_tokens: 10, turns: 1, days: 1, active_days: 1 }
      })
    }))
    setRuntimeRequest(runtimeRequest)

    const loaded = await loadModelUsage({ from: '2026-05-01', to: '2026-05-01', timezone: 'UTC' })

    expect(loaded?.buckets[0]?.model).toBe('Opus 4.8')
    expect(loaded?.buckets[0]?.provider).toBe('anthropic')
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/usage?group_by=model&from=2026-05-01&to=2026-05-01&timezone=UTC',
      'GET'
    )
  })

  it('times out when the runtime bridge does not settle', async () => {
    vi.useFakeTimers()
    setRuntimeRequest(() => new Promise<never>(() => undefined))

    const pending = loadModelUsage({ from: '2026-05-01', to: '2026-05-01', timezone: 'UTC' }).catch(
      (error: unknown) => error
    )
    await vi.advanceTimersByTimeAsync(USAGE_REQUEST_TIMEOUT_MS)
    const error = await pending

    expect(error).toBeInstanceOf(Error)
    expect((error as Error).message).toBe(`model usage request timed out after ${USAGE_REQUEST_TIMEOUT_MS}ms`)
  })

  it('reports invalid JSON model usage responses with a stable error', async () => {
    setRuntimeRequest(async () => ({ ok: true, status: 200, body: '{bad-json' }))

    await expect(
      loadModelUsage({ from: '2026-05-01', to: '2026-05-01', timezone: 'UTC' })
    ).rejects.toThrow('model usage response was not valid JSON')
  })
})
