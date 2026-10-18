import { useEffect, useState } from 'react'
import { ConversationUsageV1Schema, type ConversationUsageV1 } from '../../../../packages/runtime/src/contracts/usage'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { parseUsageResponse } from './usage-response'
import {
  normalizeUsageCost,
  type CostEstimateStatus,
  type CostKnownCurrency,
  type UsageCostCoverage
} from '../agent/usage-cost'

export type ThreadUsageSummary = UsageCostCoverage & {
  inputTokens: number
  outputTokens: number
  reasoningTokens: number
  cachedTokens: number
  cacheMissTokens: number
  /** Thread-cumulative cache hit rate (dragged down by the cold first turn). */
  cacheHitRate: number | null
  /** Cache hit rate of the most recent turn; preferred for the usage chip. */
  lastTurnCacheHitRate: number | null
  totalTokens: number
  tokenEconomySavingsTokens: number
  turns: number
}

export type ThreadUsageData = {
  usage: ThreadUsageSummary | null
  conversationUsage: ConversationUsageV1 | null
}
export type ThreadUsageState = ThreadUsageData & { loading: boolean; loaded: boolean }

const emptyData = (): ThreadUsageData => ({ usage: null, conversationUsage: null })
const emptyState = (loading = false): ThreadUsageState => ({ ...emptyData(), loading, loaded: false })
const record = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value)


function usageNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function hasFiniteNumber(record: Record<string, unknown>, key: string): boolean {
  return typeof record[key] === 'number' && Number.isFinite(record[key])
}

function usageRate(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : null
}

export function formatCompactNumber(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`
  return new Intl.NumberFormat().format(value)
}

function isChineseLocale(locale?: string): boolean {
  const normalized = (locale ?? '').trim().toLowerCase()
  return normalized === 'zh' || normalized.startsWith('zh-')
}

function fallbackLocale(): string {
  return typeof navigator !== 'undefined' ? navigator.language : 'en'
}

function formatMoneyValue(value: number): string {
  const safeValue = Number.isFinite(value) ? value : 0
  if (safeValue > 0 && safeValue < 0.0001) return '<0.0001'
  return safeValue.toFixed(safeValue >= 1 ? 2 : 4)
}

export function formatUnconfiguredCost(locale = fallbackLocale()): string {
  return isChineseLocale(locale) ? '费用未知' : 'Cost unavailable'
}

export function formatCost(
  costUsd: number | null | undefined,
  locale = fallbackLocale(),
  costCny?: number | null,
  status?: CostEstimateStatus,
  currencies: readonly CostKnownCurrency[] = []
): string {
  if (status === 'partial') return isChineseLocale(locale) ? '费用未完整估计' : 'Cost partially unknown'
  if (status !== 'complete') return formatUnconfiguredCost(locale)
  const usd = currencies.includes('USD') && costUsd != null && Number.isFinite(costUsd)
    ? `$${formatMoneyValue(costUsd)}` : null
  const cny = currencies.includes('CNY') && costCny != null && Number.isFinite(costCny)
    ? `￥${formatMoneyValue(costCny)}` : null
  // Different currencies are separate estimates, never a fixed FX conversion.
  return (isChineseLocale(locale) ? [cny, usd] : [usd, cny]).filter(Boolean).join(' + ') || formatUnconfiguredCost(locale)
}

export function formatPercent(value: number | null): string {
  if (value == null || !Number.isFinite(value)) return '-'
  const percent = Math.max(0, Math.min(100, value * 100))
  if (percent > 0 && percent < 0.1) return '<0.1%'
  if (percent === 0 || percent >= 10) return `${Math.round(percent)}%`
  return `${percent.toFixed(1)}%`
}

export function primaryCacheHitRate(
  usage: Pick<ThreadUsageSummary, 'cacheHitRate' | 'lastTurnCacheHitRate'>
): number | null {
  return usage.lastTurnCacheHitRate ?? usage.cacheHitRate
}

function legacyUsageFromBucket(bucket: Record<string, unknown>): ThreadUsageSummary | null {
  const inputTokens = usageNumber(bucket.input_tokens)
  const outputTokens = usageNumber(bucket.output_tokens)
  const reasoningTokens = usageNumber(bucket.reasoning_tokens)
  const bucketCacheHitRate = usageRate(bucket.cache_hit_rate)
  const cachedTokens = usageNumber(bucket.cached_tokens)
  const cacheMissTokens = usageNumber(bucket.cache_miss_tokens)
  const canDeriveCacheHitRate = hasFiniteNumber(bucket, 'cached_tokens') &&
    hasFiniteNumber(bucket, 'cache_miss_tokens') && cachedTokens + cacheMissTokens > 0
  const cacheHitRate = bucketCacheHitRate ?? (canDeriveCacheHitRate
    ? cachedTokens / (cachedTokens + cacheMissTokens) : null)
  const lastTurnCacheHitRate = usageRate(bucket.last_turn_cache_hit_rate)
  const explicitTotalTokens = usageNumber(bucket.total_tokens)
  const totalTokens = explicitTotalTokens > 0 ? explicitTotalTokens : inputTokens + outputTokens
  const cost = normalizeUsageCost(bucket, 'snake')
  const tokenEconomySavingsTokens = usageNumber(bucket.token_economy_savings_tokens)
  const turns = usageNumber(bucket.turns)
  if (totalTokens <= 0 && cachedTokens <= 0 && (cost.costUsd ?? 0) <= 0 &&
      (cost.costCny ?? 0) <= 0 && tokenEconomySavingsTokens <= 0 && turns <= 0) return null
  return { inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheMissTokens,
    cacheHitRate, lastTurnCacheHitRate, totalTokens, ...cost, tokenEconomySavingsTokens, turns }
}

export async function loadThreadUsageData(threadId: string): Promise<ThreadUsageData> {
  const params = new URLSearchParams({ group_by: 'thread', thread_id: threadId })
  const response = await rendererRuntimeClient.runtimeRequest(`/v1/usage?${params.toString()}`, 'GET')
  if (!response.ok || !response.body.trim()) return emptyData()
  const parsed = parseUsageResponse<unknown>(response.body, 'thread usage')
  if (!record(parsed) || (parsed.group_by !== undefined && parsed.group_by !== 'thread') ||
      !Array.isArray(parsed.buckets) || !parsed.buckets.every(record)) {
    throw new Error('thread usage response was not a thread bucket response')
  }
  // The request is already scoped by thread_id. Never accept label/key/id aliases,
  // a second requested bucket, or a response containing a foreign bucket.
  if (parsed.buckets.length === 0) return emptyData()
  if (parsed.buckets.some(bucket => bucket.thread_id !== threadId)) {
    throw new Error('thread usage response did not match requested thread')
  }
  if (parsed.buckets.length !== 1) throw new Error('thread usage response had duplicate thread buckets')
  const bucket = parsed.buckets[0]
  let conversationUsage: ConversationUsageV1 | null = null
  if (Object.prototype.hasOwnProperty.call(bucket, 'conversationUsage')) {
    const validated = ConversationUsageV1Schema.safeParse(bucket.conversationUsage)
    if (!validated.success) throw new Error('thread usage conversation projection was invalid')
    const ids = new Set<string>()
    for (const turn of validated.data.turns) {
      if (ids.has(turn.turnId)) throw new Error('thread usage conversation projection had duplicate turns')
      ids.add(turn.turnId)
    }
    conversationUsage = validated.data
  }
  // Whole-log projection survives a zero/absent retained legacy total.
  return { usage: legacyUsageFromBucket(bucket), conversationUsage }
}

export async function loadThreadUsage(threadId: string): Promise<ThreadUsageSummary | null> {
  return (await loadThreadUsageData(threadId)).usage
}

export function useThreadUsageState(
  threadId: string | null | undefined,
  enabled: boolean,
  refreshKey: unknown,
  lifecycleKey: unknown = null
): ThreadUsageState {
  // lifecycleKey is caller-owned runtime/reconnect/rewind identity, not a clock.
  // Callers should use a stable primitive. Existing three-argument calls remain valid.
  const [owned, setOwned] = useState<{
    threadId: string | null; lifecycleKey: unknown; state: ThreadUsageState
  }>({ threadId: null, lifecycleKey: null, state: emptyState() })
  useEffect(() => {
    let cancelled = false
    if (!threadId || !enabled) {
      setOwned({ threadId: null, lifecycleKey, state: emptyState() })
      return
    }
    setOwned(current => ({ threadId, lifecycleKey, state:
      current.threadId === threadId && Object.is(current.lifecycleKey, lifecycleKey)
        ? { ...current.state, loading: true } : emptyState(true) }))
    void loadThreadUsageData(threadId).then(data => {
      if (!cancelled) setOwned({ threadId, lifecycleKey,
        state: { ...data, loading: false, loaded: true } })
    }).catch(() => {
      if (!cancelled) setOwned({ threadId, lifecycleKey,
        state: { ...emptyData(), loading: false, loaded: true } })
    })
    return () => { cancelled = true }
  }, [enabled, threadId, refreshKey, lifecycleKey])
  if (!enabled || !threadId) return emptyState()
  // Owner and lifecycle checks run before effects; reconnect/rewind cannot show
  // the old whole-log projection for one frame. Ordinary refresh keeps its value.
  return owned.threadId === threadId && Object.is(owned.lifecycleKey, lifecycleKey)
    ? owned.state : emptyState(true)
}

export function useThreadUsage(threadId: string | null | undefined, enabled: boolean,
  refreshKey: unknown): ThreadUsageSummary | null {
  return useThreadUsageState(threadId, enabled, refreshKey).usage
}
