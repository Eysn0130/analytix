import { describe, expect, it } from 'vitest'
import { DailyUsageResponseSchema, ModelUsageResponseSchema, RuntimeUsageResponseSchema, ThreadUsageResponseSchema } from '../src/contracts/usage.js'
import {
  MAX_DAILY_USAGE_DAYS,
  UsageService,
  UsageValidationError,
  buildUsageBySourceResponse,
  buildDailyUsageResponse,
  buildModelUsageResponse,
  buildThreadUsageResponse,
  formatDateInTimezone,
  parseDailyUsageQuery,
  parseModelUsageQuery,
  type ThreadUsageRecord
} from '../src/services-test-support/usage-service.js'

function usage(overrides: Partial<ThreadUsageRecord['usage']> = {}): ThreadUsageRecord['usage'] {
  return {
    promptTokens: 100,
    completionTokens: 40,
    totalTokens: 140,
    cachedTokens: 30,
    cacheHitTokens: 30,
    cacheMissTokens: 70,
    cacheHitRate: 0.3,
    turns: 1,
    costUsd: 0.02,
    costCny: 0.14,
    cacheSavingsUsd: 0.01,
    cacheSavingsCny: 0.07,
    tokenEconomySavingsTokens: 50,
    tokenEconomySavingsUsd: 0.005,
    tokenEconomySavingsCny: 0.035,
    ...overrides
  }
}

describe('daily usage service', () => {
  it('seeds cached usage carryover and continues accumulating new turns', () => {
    const service = new UsageService()

    service.seedThread('thr_seed', usage({
      promptTokens: 100,
      completionTokens: 20,
      totalTokens: 120,
      cachedTokens: 80,
      cacheHitTokens: 80,
      cacheMissTokens: 20,
      turns: 2,
      costUsd: 0.01,
      costCny: 0.07,
      cacheSavingsUsd: 0.005,
      cacheSavingsCny: 0.035,
      tokenEconomySavingsTokens: 40,
      tokenEconomySavingsUsd: 0.004,
      tokenEconomySavingsCny: 0.028
    }))
    const after = service.record('thr_seed', usage({
      promptTokens: 10,
      completionTokens: 5,
      totalTokens: 15,
      cachedTokens: 9,
      cacheHitTokens: 9,
      cacheMissTokens: 1,
      turns: 1,
      costUsd: 0.002,
      costCny: 0.014,
      cacheSavingsUsd: 0.001,
      cacheSavingsCny: 0.007,
      tokenEconomySavingsTokens: 10,
      tokenEconomySavingsUsd: 0.001,
      tokenEconomySavingsCny: 0.007
    }))

    expect(after.promptTokens).toBe(110)
    expect(after.cacheHitTokens).toBe(89)
    expect(after.cacheMissTokens).toBe(21)
    expect(after.cacheHitRate).toBeCloseTo(89 / 110)
    expect(after.turns).toBe(3)
    expect(after.tokenEconomySavingsTokens).toBe(50)
    expect(service.total().costUsd).toBeCloseTo(0.012)
    expect(service.total().cacheSavingsUsd).toBeCloseTo(0.006)
    expect(service.total().tokenEconomySavingsUsd).toBeCloseTo(0.005)
    expect(service.cacheSnapshot('thr_seed')).toMatchObject({
      hits: 89,
      misses: 21,
      hitRate: 89 / 110
    })
  })

  it('records token economy savings without counting an extra model turn', () => {
    const service = new UsageService()

    service.record('thr_savings', usage({
      promptTokens: 12,
      completionTokens: 3,
      totalTokens: 15,
      turns: 1,
      tokenEconomySavingsTokens: 0,
      tokenEconomySavingsUsd: 0,
      tokenEconomySavingsCny: 0
    }))
    const after = service.recordTokenEconomySavings('thr_savings', {
      tokenEconomySavingsTokens: 8000,
      tokenEconomySavingsUsd: 0.00348,
      tokenEconomySavingsCny: 0.024
    })

    expect(after.turns).toBe(1)
    expect(after.totalTokens).toBe(15)
    expect(after.tokenEconomySavingsTokens).toBe(8000)
    expect(after.tokenEconomySavingsUsd).toBeCloseTo(0.00348)
    expect(after.tokenEconomySavingsCny).toBeCloseTo(0.024)
  })

  it('does not guess cache hit telemetry from cachedTokens-only carryover', () => {
    const service = new UsageService()

    service.seedThread('thr_unknown_cache', usage({
      cachedTokens: 42,
      cacheHitTokens: undefined,
      cacheMissTokens: undefined,
      cacheHitRate: null
    }))

    expect(service.forThread('thr_unknown_cache')).toMatchObject({
      cachedTokens: 42,
      cacheHitRate: null
    })
    expect(service.forThread('thr_unknown_cache').cacheHitTokens).toBeUndefined()
    expect(service.forThread('thr_unknown_cache').cacheMissTokens).toBeUndefined()
    expect(service.cacheSnapshot('thr_unknown_cache')).toMatchObject({
      hits: 0,
      misses: 0,
      hitRate: null
    })
  })

  it('does not turn unsupported provider usage into cache misses while accumulating', () => {
    const service = new UsageService()

    const after = service.record('thr_unsupported_cache', usage({
      promptTokens: 100,
      completionTokens: 10,
      totalTokens: 110,
      cachedTokens: undefined,
      cacheHitTokens: undefined,
      cacheMissTokens: undefined,
      cacheHitRate: null
    }))

    expect(after.promptTokens).toBe(100)
    expect(after.cacheHitRate).toBeNull()
    expect(after.cacheHitTokens).toBeUndefined()
    expect(after.cacheMissTokens).toBeUndefined()
    expect(service.total().cacheHitRate).toBeNull()
    expect(service.total().cacheHitTokens).toBeUndefined()
    expect(service.total().cacheMissTokens).toBeUndefined()
    expect(service.cacheSnapshot('thr_unsupported_cache')).toMatchObject({
      hits: 0,
      misses: 0,
      hitRate: null
    })
  })

  it('enriches recorded usage with cache diagnostics for legacy TypeScript usage chips', () => {
    const service = new UsageService()

    service.record('thr_cache_diag', usage({
      promptTokens: 1000,
      completionTokens: 10,
      totalTokens: 1010,
      cacheHitTokens: 0,
      cacheMissTokens: 700,
      cacheHitRate: 0
    }), {
      prefixHash: 'prefix_b',
      systemHash: 'system_a',
      prefixItemsHash: 'items_a',
      toolsHash: 'tools_b',
      toolSchemaTokens: 12,
      prefixChanged: true,
      prefixChangeReasons: ['tools'],
      toolSourceChanged: true,
      toolSourceChangeReasons: ['tool_sources'],
      cacheTelemetrySupported: true,
      cacheHitTokens: 0,
      cacheMissTokens: 700
    })

    const response = buildThreadUsageResponse([
      {
        threadId: 'thr_cache_diag',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: service.forThread('thr_cache_diag')
      }
    ])
    expect(response.buckets[0]?.last_turn_cacheable_hit_rate).toBe(0)
    expect(response.buckets[0]?.last_turn_total_input_hit_rate).toBe(0)
    expect(response.buckets[0]?.last_cache_miss_reasons).toEqual([
      'tool_catalog_changed',
      'provider_cache_miss',
      'cache_ttl_unknown'
    ])
    expect(response.buckets[0]?.last_cache_suggestions).toContain(
      'Keep MCP, Skill, and built-in tool schemas stable within a thread.'
    )
  })

  it('parses a valid daily usage query with explicit timezone', () => {
    expect(
      parseDailyUsageQuery({
        group_by: 'day',
        from: '2026-05-01',
        to: '2026-05-31',
        timezone: 'Asia/Shanghai'
      })
    ).toEqual({
      groupBy: 'day',
      from: '2026-05-01',
      to: '2026-05-31',
      timezone: 'Asia/Shanghai'
    })
  })

  it('rejects unsupported grouping and invalid ranges', () => {
    expect(() => parseDailyUsageQuery({ group_by: 'week' })).toThrow(UsageValidationError)
    expect(() =>
      parseDailyUsageQuery({ group_by: 'day', from: '2026-06-02', to: '2026-06-01' })
    ).toThrow('from must be on or before to')
    expect(() =>
      parseDailyUsageQuery({ group_by: 'day', from: '2026-01-01', to: '2027-01-10' })
    ).toThrow(`${MAX_DAILY_USAGE_DAYS} days or less`)
  })

  it('uses the runtime default timezone when timezone is omitted', () => {
    const parsed = parseDailyUsageQuery(
      { group_by: 'day', from: '2026-06-01', to: '2026-06-01' },
      'UTC'
    )

    expect(parsed.timezone).toBe('UTC')
  })

  it('expands rolling usage windows in the requested timezone', () => {
    const parsed = parseDailyUsageQuery(
      { group_by: 'day', window: 'week', timezone: 'Asia/Shanghai' },
      'UTC',
      new Date('2026-06-03T16:30:00.000Z')
    )

    expect(parsed).toEqual({
      groupBy: 'day',
      from: '2026-05-29',
      to: '2026-06-04',
      timezone: 'Asia/Shanghai'
    })
  })

  it('parses a valid model usage query with explicit timezone', () => {
    expect(
      parseModelUsageQuery({
        group_by: 'model',
        from: '2026-05-01',
        to: '2026-05-31',
        timezone: 'Asia/Shanghai'
      })
    ).toEqual({
      groupBy: 'model',
      from: '2026-05-01',
      to: '2026-05-31',
      timezone: 'Asia/Shanghai'
    })
  })

  it('groups turns by the requested timezone', () => {
    expect(formatDateInTimezone('2026-05-01T16:30:00.000Z', 'Asia/Shanghai')).toBe('2026-05-02')
  })

  it('returns contiguous buckets, zero days, totals, and thread counts', () => {
    const response = buildDailyUsageResponse(
      [
        {
          threadId: 'thr_a',
          completedAt: '2026-05-01T10:00:00.000Z',
          usage: usage()
        },
        {
          threadId: 'thr_b',
          completedAt: '2026-05-03T10:00:00.000Z',
          usage: usage({ promptTokens: 50, completionTokens: 10, totalTokens: 60, turns: 2 })
        }
      ],
      { groupBy: 'day', from: '2026-05-01', to: '2026-05-03', timezone: 'UTC' }
    )

    expect(DailyUsageResponseSchema.parse(response)).toEqual(response)
    expect(response.buckets.map((bucket) => bucket.date)).toEqual([
      '2026-05-01',
      '2026-05-02',
      '2026-05-03'
    ])
    expect(response.buckets[1]).toMatchObject({
      total_tokens: 0,
      turns: 0,
      thread_count: 0
    })
    expect(response.totals).toMatchObject({
      total_tokens: 200,
      turns: 3,
      thread_count: 2,
      days: 3,
      active_days: 2,
      cache_savings_usd: 0.02
    })
  })

  it('keeps cache hit rate unknown when cache telemetry is absent', () => {
    const response = buildDailyUsageResponse(
      [
        {
          threadId: 'thr_a',
          completedAt: '2026-05-01T10:00:00.000Z',
          usage: usage({
            cachedTokens: undefined,
            cacheHitTokens: undefined,
            cacheMissTokens: undefined,
            cacheHitRate: null
          })
        }
      ],
      { groupBy: 'day', from: '2026-05-01', to: '2026-05-01', timezone: 'UTC' }
    )

    expect(response.buckets[0]?.cache_hit_rate).toBeNull()
    expect(response.totals.cache_hit_rate).toBeNull()
  })

  it('does not treat cachedTokens-only usage as cache hits in aggregate buckets', () => {
    const cachedTokensOnly = usage({
      cachedTokens: 42,
      cacheHitTokens: undefined,
      cacheMissTokens: undefined,
      cacheHitRate: null
    })
    const records: ThreadUsageRecord[] = [
      {
        threadId: 'thr_unknown_cache',
        model: 'Opus 4.8',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: cachedTokensOnly
      }
    ]

    const threadResponse = buildThreadUsageResponse(records)
    const dailyResponse = buildDailyUsageResponse(records, {
      groupBy: 'day',
      from: '2026-05-01',
      to: '2026-05-01',
      timezone: 'UTC'
    })
    const modelResponse = buildModelUsageResponse(records, {
      groupBy: 'model',
      from: '2026-05-01',
      to: '2026-05-01',
      timezone: 'UTC'
    })

    for (const counters of [
      threadResponse.buckets[0],
      threadResponse.totals,
      dailyResponse.buckets[0],
      dailyResponse.totals,
      modelResponse.buckets[0],
      modelResponse.totals
    ]) {
      expect(counters?.cached_tokens).toBe(0)
      expect(counters?.cache_miss_tokens).toBe(0)
      expect(counters?.cache_hit_rate).toBeNull()
    }
  })

  it('aggregates provider-reported reasoning tokens across usage views', () => {
    const records: ThreadUsageRecord[] = [
      {
        threadId: 'thr_reasoning_a',
        model: 'reasoning-model',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: usage({ reasoningTokens: 12 })
      },
      {
        threadId: 'thr_reasoning_b',
        model: 'reasoning-model',
        completedAt: '2026-05-01T10:05:00.000Z',
        usage: usage({ reasoningTokens: 8 })
      }
    ]

    const threadResponse = buildThreadUsageResponse(records)
    const dailyResponse = buildDailyUsageResponse(records, {
      groupBy: 'day',
      from: '2026-05-01',
      to: '2026-05-01',
      timezone: 'UTC'
    })
    const modelResponse = buildModelUsageResponse(records, {
      groupBy: 'model',
      from: '2026-05-01',
      to: '2026-05-01',
      timezone: 'UTC'
    })

    expect(threadResponse.totals.reasoning_tokens).toBe(20)
    expect(dailyResponse.totals.reasoning_tokens).toBe(20)
    expect(dailyResponse.buckets[0]?.reasoning_tokens).toBe(20)
    expect(modelResponse.totals.reasoning_tokens).toBe(20)
    expect(modelResponse.buckets[0]?.reasoning_tokens).toBe(20)
  })

  it('preserves configured zero-cost pricing across usage views', () => {
    const records: ThreadUsageRecord[] = [
      {
        threadId: 'thr_free',
        model: 'free-model',
        providerId: 'custom-free',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: usage({
          costUsd: 0,
          costCny: 0,
          priceConfigured: true
        })
      }
    ]

    const threadResponse = buildThreadUsageResponse(records)
    const dailyResponse = buildDailyUsageResponse(records, {
      groupBy: 'day',
      from: '2026-05-01',
      to: '2026-05-02',
      timezone: 'UTC'
    })
    const modelResponse = buildModelUsageResponse(records, {
      groupBy: 'model',
      from: '2026-05-01',
      to: '2026-05-02',
      timezone: 'UTC'
    })

    expect(ThreadUsageResponseSchema.parse(threadResponse)).toEqual(threadResponse)
    expect(DailyUsageResponseSchema.parse(dailyResponse)).toEqual(dailyResponse)
    expect(ModelUsageResponseSchema.parse(modelResponse)).toEqual(modelResponse)
    expect(threadResponse.buckets[0]?.price_configured).toBe(true)
    expect(threadResponse.totals.price_configured).toBe(true)
    expect(dailyResponse.buckets[0]?.price_configured).toBe(true)
    expect(dailyResponse.buckets[1]?.price_configured).toBe(false)
    expect(dailyResponse.totals.price_configured).toBe(true)
    expect(modelResponse.buckets[0]?.price_configured).toBe(true)
    expect(modelResponse.days[0]?.price_configured).toBe(true)
    expect(modelResponse.days[1]?.price_configured).toBe(false)
    expect(modelResponse.totals.price_configured).toBe(true)
  })

  it('preserves thread-grouped usage buckets alongside daily grouping', () => {
    const response = buildThreadUsageResponse([
      {
        threadId: 'thr_b',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: usage({ promptTokens: 10, completionTokens: 5, totalTokens: 15 })
      },
      {
        threadId: 'thr_a',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: usage({ promptTokens: 100, completionTokens: 20, totalTokens: 120 })
      },
      {
        threadId: 'thr_b',
        completedAt: '2026-05-02T10:00:00.000Z',
        usage: usage({ promptTokens: 30, completionTokens: 10, totalTokens: 40 })
      }
    ])

    expect(ThreadUsageResponseSchema.parse(response)).toEqual(response)
    expect(response.group_by).toBe('thread')
    expect(response.buckets.map((bucket) => bucket.thread_id)).toEqual(['thr_a', 'thr_b'])
    expect(response.buckets[1]).toMatchObject({
      input_tokens: 40,
      output_tokens: 15,
      total_tokens: 55,
      turns: 2
    })
    expect(response.totals).toMatchObject({
      total_tokens: 175,
      thread_count: 2,
      turns: 3,
      cache_savings_usd: 0.03
    })
  })

  it('reports last_turn_cache_hit_rate from the most recent turn, not the cumulative rate', () => {
    const response = buildThreadUsageResponse([
      {
        threadId: 'thr_x',
        completedAt: '2026-05-01T10:00:00.000Z',
        // Cold first turn: mostly miss.
        usage: usage({ cachedTokens: 1024, cacheHitTokens: 1024, cacheMissTokens: 8049, cacheHitRate: 1024 / (1024 + 8049) })
      },
      {
        threadId: 'thr_x',
        completedAt: '2026-05-01T10:05:00.000Z',
        // Warm second turn: mostly hit.
        usage: usage({
          cachedTokens: 8960,
          cacheHitTokens: 8960,
          cacheMissTokens: 128,
          promptTokens: 10000,
          totalTokens: 10040,
          cacheHitRate: 8960 / (8960 + 128),
          cacheableTokenHitRate: 8960 / (8960 + 128),
          totalInputTokenHitRate: 8960 / 10000,
          cacheMissReasons: ['tool_catalog_changed'],
          cacheSuggestions: ['Keep MCP and Skill tools stable within a thread.']
        })
      }
    ])

    const bucket = response.buckets.find((item) => item.thread_id === 'thr_x')
    // Cumulative is dragged down by the cold turn (~55%)...
    expect(bucket?.cache_hit_rate).toBeCloseTo((1024 + 8960) / (9073 + 9088), 4)
    // ...but the chip-facing field reflects the latest (warm) turn (~98.6%).
    expect(bucket?.last_turn_cache_hit_rate).toBeCloseTo(8960 / (8960 + 128), 6)
    expect(bucket?.last_turn_cacheable_hit_rate).toBeCloseTo(8960 / (8960 + 128), 6)
    expect(bucket?.last_turn_total_input_hit_rate).toBeCloseTo(8960 / 10000, 6)
    expect(bucket?.last_cache_miss_reasons).toEqual(['tool_catalog_changed'])
    expect(bucket?.last_cache_suggestions).toEqual(['Keep MCP and Skill tools stable within a thread.'])
    expect(ThreadUsageResponseSchema.parse(response)).toEqual(response)
  })

  it('leaves last_turn_cache_hit_rate null when the latest turn lacks cache telemetry', () => {
    const response = buildThreadUsageResponse([
      {
        threadId: 'thr_y',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: usage({ cachedTokens: 0, cacheHitTokens: undefined, cacheMissTokens: undefined, cacheHitRate: null })
      }
    ])
    expect(response.buckets[0]?.last_turn_cache_hit_rate).toBeNull()
  })

  it('groups runtime usage by turn and subagent source with child run attribution', () => {
    const records: ThreadUsageRecord[] = [
      {
        threadId: 'thr_parent',
        completedAt: '2026-05-01T10:00:00.000Z',
        usage: usage({ promptTokens: 12, completionTokens: 3, totalTokens: 15, cacheHitTokens: 0, cacheMissTokens: 12 })
      },
      {
        threadId: 'thr_child',
        completedAt: '2026-05-01T10:01:00.000Z',
        usageSource: 'subagent',
        childRunId: 'job-1',
        usage: usage({ promptTokens: 20, completionTokens: 4, totalTokens: 24, cacheHitTokens: 10, cacheMissTokens: 10 })
      },
      {
        threadId: 'thr_child_2',
        completedAt: '2026-05-01T10:02:00.000Z',
        usageSource: 'subagent',
        childRunId: 'job-2',
        usage: usage({ promptTokens: 30, completionTokens: 6, totalTokens: 36, cacheHitTokens: 15, cacheMissTokens: 15 })
      }
    ]

    const bySource = buildUsageBySourceResponse(records)
    const runtimeResponse = RuntimeUsageResponseSchema.parse({
      total: usage({ promptTokens: 62, completionTokens: 13, totalTokens: 75, cacheHitTokens: 25, cacheMissTokens: 37 }),
      perThread: [],
      bySource
    })

    expect(runtimeResponse.bySource.map((bucket) => bucket.source)).toEqual(['subagent', 'turn'])
    expect(runtimeResponse.bySource.find((bucket) => bucket.source === 'turn')?.usage.totalTokens).toBe(15)
    expect(runtimeResponse.bySource.find((bucket) => bucket.source === 'subagent')).toMatchObject({
      childRunIds: ['job-1', 'job-2'],
      usage: {
        totalTokens: 60,
        cacheHitTokens: 25,
        cacheMissTokens: 25
      }
    })
  })

  it('groups usage by model with daily bars and model totals', () => {
    const response = buildModelUsageResponse(
      [
        {
          threadId: 'thr_a',
          model: 'Opus 4.8',
          completedAt: '2026-05-01T10:00:00.000Z',
          usage: usage({ promptTokens: 100, completionTokens: 20, totalTokens: 120 })
        },
        {
          threadId: 'thr_b',
          model: 'Opus 4.7',
          completedAt: '2026-05-02T10:00:00.000Z',
          usage: usage({ promptTokens: 40, completionTokens: 10, totalTokens: 50 })
        },
        {
          threadId: 'thr_c',
          model: 'Opus 4.8',
          completedAt: '2026-05-02T12:00:00.000Z',
          usage: usage({ promptTokens: 60, completionTokens: 30, totalTokens: 90 })
        }
      ],
      { groupBy: 'model', from: '2026-05-01', to: '2026-05-03', timezone: 'UTC' }
    )

    expect(ModelUsageResponseSchema.parse(response)).toEqual(response)
    expect(response.group_by).toBe('model')
    expect(response.days.map((bucket) => bucket.date)).toEqual([
      '2026-05-01',
      '2026-05-02',
      '2026-05-03'
    ])
    expect(response.days.map((bucket) => bucket.total_tokens)).toEqual([120, 140, 0])
    expect(response.buckets.map((bucket) => bucket.model)).toEqual(['Opus 4.8', 'Opus 4.7'])
    expect(response.buckets[0]).toMatchObject({
      input_tokens: 160,
      output_tokens: 50,
      total_tokens: 210,
      thread_count: 2
    })
    expect(response.totals).toMatchObject({
      total_tokens: 260,
      active_days: 2,
      thread_count: 3
    })
  })

  it('marks model usage provider as mixed when the same model spans providers', () => {
    const response = buildModelUsageResponse(
      [
        {
          threadId: 'thr_deepseek',
          model: 'shared-model',
          providerId: 'deepseek',
          completedAt: '2026-05-01T10:00:00.000Z',
          usage: usage({ promptTokens: 20, completionTokens: 5, totalTokens: 25 })
        },
        {
          threadId: 'thr_openai',
          model: 'shared-model',
          providerId: 'openai-compatible',
          completedAt: '2026-05-01T11:00:00.000Z',
          usage: usage({ promptTokens: 30, completionTokens: 10, totalTokens: 40 })
        },
        {
          threadId: 'thr_anthropic',
          model: 'claude',
          providerId: 'anthropic',
          completedAt: '2026-05-01T12:00:00.000Z',
          usage: usage({ promptTokens: 10, completionTokens: 2, totalTokens: 12 })
        }
      ],
      { groupBy: 'model', from: '2026-05-01', to: '2026-05-01', timezone: 'UTC' }
    )

    expect(ModelUsageResponseSchema.parse(response)).toEqual(response)
    expect(response.buckets.find((bucket) => bucket.model === 'shared-model')).toMatchObject({
      provider: 'mixed',
      total_tokens: 65,
      thread_count: 2
    })
    expect(response.buckets.find((bucket) => bucket.model === 'claude')).toMatchObject({
      provider: 'anthropic',
      total_tokens: 12
    })
  })
})
