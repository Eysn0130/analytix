// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  formatCost,
  formatPercent,
  formatUnconfiguredCost,
  loadThreadUsage,
  loadThreadUsageData,
  useThreadUsageState,
  type ThreadUsageState,
  primaryCacheHitRate
} from './use-thread-usage'

import { ThreadUsageResponseSchema, type ConversationUsageV1, type ProviderBillingV1, type ThreadUsageResponse } from '../../../../packages/runtime/src/contracts/usage'

type RuntimeResult = { ok: boolean; status: number; body: string }
type RuntimeRequest = (path: string, method?: string) => Promise<RuntimeResult>
const threadUsagePath = (threadId: string): string => `/v1/usage?${new URLSearchParams({
  group_by: 'thread', thread_id: threadId }).toString()}`
let savedApi: PropertyDescriptor | undefined
let root: Root | undefined
let host: HTMLDivElement | undefined

function setRuntimeRequest(runtimeRequest: RuntimeRequest): void {
  Object.defineProperty(window, 'analytix', { configurable: true,
    value: { runtime: { runtimeRequest } } })
}
beforeEach(() => {
  savedApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
})
afterEach(async () => {
  if (root) await act(async () => root?.unmount())
  host?.remove(); root = undefined; host = undefined
  vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (savedApi) Object.defineProperty(window, 'analytix', savedApi)
  else Reflect.deleteProperty(window, 'analytix')
})

// Test data constructors reference the canonical types/schema; these are not DTO definitions.
function billing(kind: 'complete' | 'partial' | 'unknown' | 'zero' = 'complete'): ProviderBillingV1 {
  const attemptCount = kind === 'zero' ? 0 : 2
  const knownAttemptCount = kind === 'complete' ? 2 : kind === 'partial' ? 1 : 0
  const complete = kind === 'complete' || kind === 'zero'
  const count = (value: number): ProviderBillingV1['inputTokens'] => ({
    reportedValue: knownAttemptCount === 0 ? 0 : value, knownAttemptCount, attemptCount, complete })
  return { inventoryComplete: kind !== 'unknown', closed: complete, attemptCount,
    settledAttemptCount: knownAttemptCount,
    inputTokens: count(7), outputTokens: count(5), uncachedInputTokens: count(2),
    cacheReadTokens: count(3), cacheWriteTokens: count(2), reasoningTokens: count(1), totalTokens: count(12) }
}
function conversation(kind: 'complete' | 'partial' | 'unknown' | 'zero' = 'complete'): ConversationUsageV1 {
  const bill = billing(kind)
  return { schemaVersion: 'conversation-usage.v1', billing: bill,
    turns: [{ turnId: 'turn-one', billing: bill }],
    metrics: { turnCount: 1, stepCount: null,
      llmMs: { reportedValue: 0, knownStepCount: 0, complete: false },
      toolMs: { reportedValue: 0, knownCallCount: 0, callCount: 0, complete: true },
      ttftMs: 0, ttftSteps: 0, decodeMs: 0, decodeTokens: 0, decodeSteps: 0 } }
}
function response(threadId: string, projection?: ConversationUsageV1,
  legacyTotal = 0): RuntimeResult {
  const counters = { input_tokens: legacyTotal, output_tokens: 0, reasoning_tokens: 0,
    cached_tokens: 0, cache_miss_tokens: legacyTotal, total_tokens: legacyTotal,
    cost_usd: 0, cost_cny: 0, price_configured: false,
    cache_savings_usd: 0, cache_savings_cny: 0, token_economy_savings_tokens: 0,
    token_economy_savings_usd: 0, token_economy_savings_cny: 0,
    turns: 0, cache_hit_rate: null }
  // Emit a normal canonical /v1/usage response, including defaults and totals.
  const value = ThreadUsageResponseSchema.parse({ group_by: 'thread',
    buckets: [{ ...counters, thread_id: threadId,
      ...(projection ? { conversationUsage: projection } : {}) }],
    totals: { ...counters, thread_count: 1 } })
  return { ok: true, status: 200, body: JSON.stringify(value) }
}
function mutateResponse(value: RuntimeResult,
  change: (body: ThreadUsageResponse) => void): RuntimeResult {
  const body = JSON.parse(value.body) as ThreadUsageResponse
  change(body)
  return { ...value, body: JSON.stringify(body) }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
type ProbeProps = { id: string; refreshKey: string; lifecycleKey: string; enabled?: boolean }
const seen: Array<{ id: string; lifecycleKey: string; state: ThreadUsageState }> = []
function Probe({ id, refreshKey, lifecycleKey, enabled = true }: ProbeProps) {
  const state = useThreadUsageState(id, enabled, refreshKey, lifecycleKey)
  seen.push({ id, lifecycleKey, state })
  return createElement('output', { 'data-usage-owner': id }, JSON.stringify({
    legacy: state.usage?.totalTokens ?? null,
    reported: state.conversationUsage?.billing.totalTokens.reportedValue ?? null,
    loading: state.loading, loaded: state.loaded }))
}
async function renderProbe(props: ProbeProps): Promise<void> {
  if (!root) { host = document.createElement('div'); document.body.append(host); root = createRoot(host) }
  await act(async () => root?.render(createElement(Probe, props)))
}
const current = (): ThreadUsageState => seen[seen.length - 1].state


describe('thread usage formatting', () => {
  it('uses RMB for Chinese locales and USD for English locales', () => {
    expect(formatCost(0.125, 'zh', 0.88, 'complete', ['USD', 'CNY'])).toBe('￥0.8800 + $0.1250')
    expect(formatCost(0.125, 'zh-CN', 0.88, 'complete', ['USD', 'CNY'])).toBe('￥0.8800 + $0.1250')
    expect(formatCost(0.125, 'en', null, 'complete', ['USD'])).toBe('$0.1250')
    expect(formatCost(0.125, 'zh', null, 'complete', ['USD'])).toBe('$0.1250')
    expect(formatCost(null, 'en', 0.88, 'complete', ['CNY'])).toBe('￥0.8800')
    expect(formatCost(0.00000001, 'en', null, 'complete', ['USD'])).toBe('$<0.0001')
  })

  it('labels missing pricing as not configured instead of showing zero or a dash', () => {
    expect(formatUnconfiguredCost('en')).toBe('Cost unavailable')
    expect(formatUnconfiguredCost('zh-CN')).toBe('费用未知')
    expect(formatCost(null, 'en', null)).toBe('Cost unavailable')
    expect(formatCost(0, 'en', 0)).toBe('Cost unavailable')
    expect(formatCost(undefined, 'zh-CN')).toBe('费用未知')
    expect(formatCost(0, 'zh-CN', null, 'complete', ['USD'])).toBe('$0.0000')
    expect(formatCost(null, 'en', 0, 'complete', ['CNY'])).toBe('￥0.0000')
    expect(formatCost(0, 'en', 0, 'partial', ['USD'])).toBe('Cost partially unknown')
    expect(formatCost(0, 'zh-CN', 0, 'partial', ['USD'])).toBe('费用未完整估计')
    expect(formatCost(0.25, 'en', 0)).toBe('Cost unavailable')
  })

  it('prefers latest-turn cache hit rate for compact cache chips', () => {
    expect(primaryCacheHitRate({ cacheHitRate: 0.4, lastTurnCacheHitRate: 0.95 })).toBe(0.95)
    expect(primaryCacheHitRate({ cacheHitRate: 0.4, lastTurnCacheHitRate: null })).toBe(0.4)
    expect(primaryCacheHitRate({ cacheHitRate: null, lastTurnCacheHitRate: null })).toBeNull()
  })

  it('shows tiny non-zero percentages without rounding them to zero', () => {
    expect(formatPercent(0)).toBe('0%')
    expect(formatPercent(0.0005)).toBe('<0.1%')
    expect(formatPercent(0.001)).toBe('0.1%')
    expect(formatPercent(0.754)).toBe('75%')
  })

  it('keeps cache tokens visible but leaves rate unknown for cachedTokens-only thread usage buckets', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_cached_only')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_cached_only',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cached_tokens: 42,
                cache_hit_rate: null,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_cached_only')

    expect(usage).toMatchObject({
      inputTokens: 100,
      outputTokens: 20,
      cachedTokens: 42,
      cacheMissTokens: 0,
      cacheHitRate: null,
      costUsd: null,
      costCny: null
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
  })

  it('derives cache hit rate when Go usage buckets include hit/miss tokens without a rate', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_hit_miss_no_rate')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_hit_miss_no_rate',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cached_tokens: 30,
                cache_miss_tokens: 70,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_hit_miss_no_rate')

    expect(usage).toMatchObject({
      cachedTokens: 30,
      cacheMissTokens: 70,
      cacheHitRate: 0.3
    })
  })

  it('keeps CNY-only thread cost instead of coercing it to USD zero', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_cny_only')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_cny_only',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cost_usd: 0,
                cost_cny: 0.06909,
                price_configured: true,
                cost_estimate_status: 'complete',
                cost_known_currencies: ['CNY'],
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_cny_only')

    expect(usage).toMatchObject({
      costUsd: null,
      costCny: 0.06909
    })
  })

  it('keeps token usage visible while treating missing provider pricing as unconfigured cost', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_unpriced')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_unpriced',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cost_usd: 0,
                cost_cny: 0,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_unpriced')

    expect(usage).toMatchObject({
      totalTokens: 120,
      costUsd: null,
      costCny: null,
      priceConfigured: false
    })
    expect(formatCost(usage?.costUsd, 'en', usage?.costCny)).toBe('Cost unavailable')
  })

  it('uses total_tokens when a Go aggregate bucket omits input and output tokens', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_total_only')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_total_only',
                total_tokens: 120,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_total_only')

    expect(usage).toMatchObject({
      inputTokens: 0,
      outputTokens: 0,
      totalTokens: 120,
      turns: 1
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
  })

  it('preserves explicit zero-cost configured pricing separately from missing pricing', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_free_configured')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_free_configured',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cost_usd: 0,
                cost_cny: 0,
                price_configured: true,
                cost_estimate_status: 'complete',
                cost_known_currencies: ['USD'],
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_free_configured')

    expect(usage).toMatchObject({
      totalTokens: 120,
      costUsd: 0,
      costCny: null,
      priceConfigured: true,
      costEstimateStatus: 'complete',
      costKnownCurrencies: ['USD']
    })
    expect(formatCost(usage?.costUsd, 'zh-CN', usage?.costCny, usage?.costEstimateStatus, usage?.costKnownCurrencies)).toBe('$0.0000')
  })

  it('uses explicit aggregate thread cache telemetry when available', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_aggregate_cache')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_aggregate_cache',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cache_savings_usd: 0.003,
                cache_savings_cny: 0.0216,
                token_economy_savings_tokens: 4096,
                token_economy_savings_usd: 0.0018,
                token_economy_savings_cny: 0.0126,
                cached_tokens: 40,
                cache_miss_tokens: 60,
                cache_hit_rate: 0.4,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_aggregate_cache')

    expect(usage).toMatchObject({
      cachedTokens: 40,
      cacheMissTokens: 60,
      cacheHitRate: 0.4,
      tokenEconomySavingsTokens: 4096
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
  })

  it('requests only the selected thread usage bucket', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_native_cache')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_native_cache',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cached_tokens: 80,
                cache_miss_tokens: 20,
                cache_hit_rate: 0.8,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_native_cache')

    expect(usage).toMatchObject({
      cachedTokens: 80,
      cacheMissTokens: 20,
      cacheHitRate: 0.8
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
    expect(runtimeRequest).toHaveBeenCalledWith(threadUsagePath('thr_native_cache'), 'GET')
  })

  it('surfaces the latest-turn cache rate distinctly from the cumulative rate', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_last_turn')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_last_turn',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cached_tokens: 80,
                cache_miss_tokens: 20,
                cache_hit_rate: 0.55,
                last_turn_cache_hit_rate: 0.986,
                turns: 2
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_last_turn')

    expect(usage).toMatchObject({
      cacheHitRate: 0.55,
      lastTurnCacheHitRate: 0.986
    })
  })

  it('falls back to null last-turn cache rate when the field is absent', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_no_last_turn')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_no_last_turn',
                input_tokens: 100,
                output_tokens: 20,
                total_tokens: 120,
                cached_tokens: 80,
                cache_miss_tokens: 20,
                cache_hit_rate: 0.8,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_no_last_turn')

    expect(usage?.lastTurnCacheHitRate).toBeNull()
    expect(usage?.cacheHitRate).toBe(0.8)
  })

  it('reports invalid JSON thread usage responses with a stable error', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_bad_json')) {
        return { ok: true, status: 200, body: '{bad-json' }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    await expect(loadThreadUsage('thr_bad_json')).rejects.toThrow(
      'thread usage response was not valid JSON'
    )
  })

  it('uses aggregate telemetry without requesting thread detail', async () => {
    const runtimeRequest = vi.fn<RuntimeRequest>(async (path) => {
      if (path === threadUsagePath('thr_no_detail_request')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            buckets: [
              {
                thread_id: 'thr_no_detail_request',
                input_tokens: 100,
                output_tokens: 20,
                cached_tokens: 40,
                cache_miss_tokens: 60,
                cache_hit_rate: 0.4,
                turns: 1
              }
            ]
          })
        }
      }
      throw new Error(`unexpected request: ${path}`)
    })
    setRuntimeRequest(runtimeRequest)

    const usage = await loadThreadUsage('thr_no_detail_request')

    expect(usage).toMatchObject({
      cachedTokens: 40,
      cacheMissTokens: 60,
      cacheHitRate: 0.4
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
  })
})

describe('canonical whole-log thread usage through the normal runtime bridge', () => {
  it.each(['complete', 'partial', 'unknown', 'zero'] as const)(
    'retains %s projection independently of the zero legacy positive filter', async kind => {
      const request = vi.fn<RuntimeRequest>(async () => response('A', conversation(kind)))
      setRuntimeRequest(request)
      const data = await loadThreadUsageData('A')
      expect(data.usage).toBeNull()
      expect(data.conversationUsage).toEqual(conversation(kind))
      expect(request).toHaveBeenCalledExactlyOnceWith(threadUsagePath('A'), 'GET')
      // Coverage is carried intact, including known zero and unknown zero.
      expect(data.conversationUsage?.billing.totalTokens.complete).toBe(kind === 'complete' || kind === 'zero')
    })
  it('does not synthesize missing whole-log billing from positive legacy totals', async () => {
    setRuntimeRequest(async () => response('A', undefined, 99))
    expect(await loadThreadUsageData('A')).toMatchObject({ usage: { totalTokens: 99 }, conversationUsage: null })
  })
  it.each(['foreign', 'duplicate-thread', 'duplicate-turn', 'invalid-projection', 'foreign-alias'] as const)(
    'rejects %s without granting projection ownership', async kind => {
      const malformed = mutateResponse(response('A', conversation()), body => {
        if (kind === 'foreign') body.buckets[0].thread_id = 'B'
        if (kind === 'duplicate-thread') body.buckets.push(body.buckets[0])
        if (kind === 'duplicate-turn') body.buckets[0].conversationUsage!.turns.push(
          body.buckets[0].conversationUsage!.turns[0])
        if (kind === 'invalid-projection') body.buckets[0].conversationUsage!.billing.inputTokens.reportedValue = -1
        if (kind === 'foreign-alias') {
          body.buckets[0].thread_id = 'B'
          Object.assign(body.buckets[0], { key: 'A', label: 'A' })
        }
      })
      setRuntimeRequest(async () => malformed)
      await expect(loadThreadUsageData('A')).rejects.toThrow(/thread usage/)
      seen.length = 0
      await renderProbe({ id: 'A', refreshKey: 'initial', lifecycleKey: 'runtime-1' })
      expect(current()).toMatchObject({ usage: null, conversationUsage: null, loading: false, loaded: true })
    })
  it('keeps recorded zero samples and nullable historical steps as producer values', async () => {
    const projection = conversation()
    projection.metrics = { ...projection.metrics!, ttftMs: 0, ttftSteps: 1,
      decodeMs: 1000, decodeTokens: 0, decodeSteps: 1 }
    setRuntimeRequest(async () => response('A', projection))
    const data = await loadThreadUsageData('A')
    expect(data.conversationUsage?.metrics).toMatchObject({ stepCount: null, ttftMs: 0,
      ttftSteps: 1, decodeMs: 1000, decodeTokens: 0, decodeSteps: 1 })
  })
  it('retains an ordinary same-owner refresh but ignores late responses after A -> B -> A', async () => {
    const initial = deferred<RuntimeResult>(), staleA = deferred<RuntimeResult>()
    const staleB = deferred<RuntimeResult>(), newestA = deferred<RuntimeResult>()
    const request = vi.fn<RuntimeRequest>().mockImplementationOnce(() => initial.promise)
      .mockImplementationOnce(() => staleA.promise).mockImplementationOnce(() => staleB.promise)
      .mockImplementationOnce(() => newestA.promise)
    setRuntimeRequest(request); seen.length = 0
    await renderProbe({ id: 'A', refreshKey: 'initial', lifecycleKey: 'runtime-1' })
    await act(async () => { initial.resolve(response('A', conversation(), 99)); await initial.promise })
    await renderProbe({ id: 'A', refreshKey: 'refresh', lifecycleKey: 'runtime-1' })
    expect(current().conversationUsage?.billing.totalTokens.reportedValue).toBe(12)
    expect(current().loading).toBe(true)
    const beforeB = seen.length
    await renderProbe({ id: 'B', refreshKey: 'initial', lifecycleKey: 'runtime-1' })
    expect(seen.slice(beforeB).every(value => value.state.usage === null && value.state.conversationUsage === null)).toBe(true)
    const beforeA = seen.length
    await renderProbe({ id: 'A', refreshKey: 'newest', lifecycleKey: 'runtime-1' })
    expect(seen.slice(beforeA).every(value => value.state.conversationUsage === null)).toBe(true)
    await act(async () => {
      staleA.resolve(response('A', conversation('partial'), 777)); staleB.resolve(response('B', conversation(), 888))
      await Promise.all([staleA.promise, staleB.promise])
    })
    expect(current()).toMatchObject({ usage: null, conversationUsage: null, loading: true })
    await act(async () => { newestA.resolve(response('A', conversation('zero'))); await newestA.promise })
    expect(current().conversationUsage?.billing.totalTokens).toMatchObject({ reportedValue: 0, complete: true })
    expect(request.mock.calls.map(call => call[0])).toEqual([
      threadUsagePath('A'), threadUsagePath('A'), threadUsagePath('B'), threadUsagePath('A')])
  })
  it.each(['reconnect-2', 'rewind-seq-10'])(
    'clears on the first same-thread %s lifecycle render and ignores older work', async nextEpoch => {
      const initial = deferred<RuntimeResult>(), stale = deferred<RuntimeResult>(), next = deferred<RuntimeResult>()
      setRuntimeRequest(vi.fn<RuntimeRequest>().mockImplementationOnce(() => initial.promise)
        .mockImplementationOnce(() => stale.promise).mockImplementationOnce(() => next.promise))
      seen.length = 0
      await renderProbe({ id: 'A', refreshKey: 'initial', lifecycleKey: 'runtime-1' })
      await act(async () => { initial.resolve(response('A', conversation(), 99)); await initial.promise })
      await renderProbe({ id: 'A', refreshKey: 'refresh', lifecycleKey: 'runtime-1' })
      const before = seen.length
      await renderProbe({ id: 'A', refreshKey: 'refresh', lifecycleKey: nextEpoch })
      expect(seen.slice(before).every(value => value.state.usage === null && value.state.conversationUsage === null)).toBe(true)
      await act(async () => { stale.resolve(response('A', conversation(), 777)); await stale.promise })
      expect(current().conversationUsage).toBeNull()
      await act(async () => { next.resolve(response('A', conversation('zero'))); await next.promise })
      expect(current().conversationUsage?.billing.closed).toBe(true)
      expect(current().usage).toBeNull()
    })
  it('drops both groups while runtime is disabled and refreshes when re-enabled', async () => {
    setRuntimeRequest(vi.fn<RuntimeRequest>().mockResolvedValueOnce(response('A', conversation(), 99))
      .mockResolvedValueOnce(response('A', conversation('zero'))))
    seen.length = 0
    await renderProbe({ id: 'A', refreshKey: 'initial', lifecycleKey: 'runtime-1' })
    expect(current().conversationUsage).not.toBeNull()
    await renderProbe({ id: 'A', refreshKey: 'initial', lifecycleKey: 'runtime-1', enabled: false })
    expect(current()).toMatchObject({ usage: null, conversationUsage: null, loading: false, loaded: false })
    await renderProbe({ id: 'A', refreshKey: 'initial', lifecycleKey: 'runtime-2' })
    expect(current().conversationUsage?.billing.totalTokens.reportedValue).toBe(0)
  })
})
