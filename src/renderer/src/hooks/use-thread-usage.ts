import { useEffect, useState } from 'react'
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

export type ThreadUsageState = {
  usage: ThreadUsageSummary | null
  loading: boolean
  loaded: boolean
}

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

export async function loadThreadUsage(threadId: string): Promise<ThreadUsageSummary | null> {
  const params = new URLSearchParams({
    group_by: 'thread',
    thread_id: threadId
  })
  const r = await rendererRuntimeClient.runtimeRequest(`/v1/usage?${params.toString()}`, 'GET')
  if (!r.ok || !r.body.trim()) return null
  const parsed = parseUsageResponse<{
    buckets?: Array<Record<string, unknown>>
  }>(r.body, 'thread usage')
  const bucket = parsed.buckets?.find((item) => {
    const candidates = [item.thread_id, item.key, item.id, item.label]
    return candidates.some((candidate) => candidate === threadId)
  })
  if (!bucket) return null
  const inputTokens = usageNumber(bucket.input_tokens)
  const outputTokens = usageNumber(bucket.output_tokens)
  const reasoningTokens = usageNumber(bucket.reasoning_tokens)
  const bucketCacheHitRate = usageRate(bucket.cache_hit_rate)
  const cachedTokens = usageNumber(bucket.cached_tokens)
  const cacheMissTokens = usageNumber(bucket.cache_miss_tokens)
  const canDeriveCacheHitRate =
    hasFiniteNumber(bucket, 'cached_tokens') &&
    hasFiniteNumber(bucket, 'cache_miss_tokens') &&
    cachedTokens + cacheMissTokens > 0
  const cacheHitRate = bucketCacheHitRate ?? (
    canDeriveCacheHitRate ? cachedTokens / (cachedTokens + cacheMissTokens) : null
  )
  const lastTurnCacheHitRate = usageRate(bucket.last_turn_cache_hit_rate)
  const explicitTotalTokens = usageNumber(bucket.total_tokens)
  const totalTokens = explicitTotalTokens > 0 ? explicitTotalTokens : inputTokens + outputTokens
  const cost = normalizeUsageCost(bucket, 'snake')
  const tokenEconomySavingsTokens = usageNumber(bucket.token_economy_savings_tokens)
  const turns = usageNumber(bucket.turns)
  if (
    totalTokens <= 0 &&
    cachedTokens <= 0 &&
    (cost.costUsd ?? 0) <= 0 &&
    (cost.costCny ?? 0) <= 0 &&
    tokenEconomySavingsTokens <= 0 &&
    turns <= 0
  ) return null
  return {
    inputTokens,
    outputTokens,
    reasoningTokens,
    cachedTokens,
    cacheMissTokens,
    cacheHitRate,
    lastTurnCacheHitRate,
    totalTokens,
    ...cost,
    tokenEconomySavingsTokens,
    turns
  }
}

export function useThreadUsageState(
  threadId: string | null | undefined,
  enabled: boolean,
  refreshKey: unknown
): ThreadUsageState {
  const [state, setState] = useState<ThreadUsageState>({
    usage: null,
    loading: false,
    loaded: false
  })

  useEffect(() => {
    let cancelled = false
    if (!threadId || !enabled) {
      setState({ usage: null, loading: false, loaded: false })
      return
    }
    setState((current) => ({ ...current, loading: true }))
    void loadThreadUsage(threadId)
      .then((usage) => {
        if (!cancelled) setState({ usage, loading: false, loaded: true })
      })
      .catch(() => {
        if (!cancelled) setState({ usage: null, loading: false, loaded: true })
      })
    return () => {
      cancelled = true
    }
  }, [enabled, refreshKey, threadId])

  return state
}

export function useThreadUsage(
  threadId: string | null | undefined,
  enabled: boolean,
  refreshKey: unknown
): ThreadUsageSummary | null {
  return useThreadUsageState(threadId, enabled, refreshKey).usage
}
