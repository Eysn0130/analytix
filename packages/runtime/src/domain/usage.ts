import type { UsageSnapshot } from '../contracts/usage.js'
import { emptyUsageSnapshot } from '../contracts/usage.js'

export type UsageEntity = UsageSnapshot

export function zeroUsage(): UsageSnapshot {
  return emptyUsageSnapshot()
}

export function addUsage(into: UsageSnapshot, delta: UsageSnapshot): UsageSnapshot {
  const promptTokens = into.promptTokens + delta.promptTokens
  const completionTokens = into.completionTokens + delta.completionTokens
  const reasoningTokens = (into.reasoningTokens ?? 0) + (delta.reasoningTokens ?? 0)
  const totalTokens = promptTokens + completionTokens
  const cachedTokens = (into.cachedTokens ?? 0) + (delta.cachedTokens ?? 0)
  const cacheHitTokens =
    (into.cacheHitTokens ?? 0) + (delta.cacheHitTokens ?? 0)
  const cacheMissTokens =
    (into.cacheMissTokens ?? 0) + (delta.cacheMissTokens ?? 0)
  const cacheTotal = cacheHitTokens + cacheMissTokens
  const cacheHitRate =
    cacheTotal === 0
      ? null
      : cacheHitTokens / cacheTotal
  const cacheMissReasons = unionStrings(into.cacheMissReasons, delta.cacheMissReasons)
  const cacheSuggestions = unionStrings(into.cacheSuggestions, delta.cacheSuggestions)
  const cacheableTokenHitRate = cacheTotal > 0 ? cacheHitTokens / cacheTotal : undefined
  const totalInputTokenHitRate =
    promptTokens > 0 && cacheTotal > 0 ? cacheHitTokens / promptTokens : undefined
  const turns = into.turns + delta.turns
  const costUsd =
    into.costUsd === undefined && delta.costUsd === undefined
      ? undefined
      : (into.costUsd ?? 0) + (delta.costUsd ?? 0)
  const costCny =
    into.costCny === undefined && delta.costCny === undefined
      ? undefined
      : (into.costCny ?? 0) + (delta.costCny ?? 0)
  const cacheSavingsUsd =
    into.cacheSavingsUsd === undefined && delta.cacheSavingsUsd === undefined
      ? undefined
      : (into.cacheSavingsUsd ?? 0) + (delta.cacheSavingsUsd ?? 0)
  const cacheSavingsCny =
    into.cacheSavingsCny === undefined && delta.cacheSavingsCny === undefined
      ? undefined
      : (into.cacheSavingsCny ?? 0) + (delta.cacheSavingsCny ?? 0)
  const tokenEconomySavingsTokens =
    (into.tokenEconomySavingsTokens ?? 0) + (delta.tokenEconomySavingsTokens ?? 0)
  const tokenEconomySavingsUsd =
    into.tokenEconomySavingsUsd === undefined && delta.tokenEconomySavingsUsd === undefined
      ? undefined
      : (into.tokenEconomySavingsUsd ?? 0) + (delta.tokenEconomySavingsUsd ?? 0)
  const tokenEconomySavingsCny =
    into.tokenEconomySavingsCny === undefined && delta.tokenEconomySavingsCny === undefined
      ? undefined
      : (into.tokenEconomySavingsCny ?? 0) + (delta.tokenEconomySavingsCny ?? 0)
  return {
    promptTokens,
    completionTokens,
    reasoningTokens,
    totalTokens,
    cachedTokens,
    cacheHitTokens,
    cacheMissTokens,
    cacheHitRate,
    cacheableTokenHitRate,
    totalInputTokenHitRate,
    cacheMissReasons,
    cacheSuggestions,
    turns,
    costUsd,
    costCny,
    cacheSavingsUsd,
    cacheSavingsCny,
    tokenEconomySavingsTokens,
    tokenEconomySavingsUsd,
    tokenEconomySavingsCny
  }
}

function unionStrings(
  left: readonly string[] | undefined,
  right: readonly string[] | undefined
): string[] | undefined {
  if (!left?.length && !right?.length) return undefined
  const merged: string[] = []
  const seen = new Set<string>()
  for (const value of [...(left ?? []), ...(right ?? [])]) {
    if (seen.has(value)) continue
    seen.add(value)
    merged.push(value)
  }
  return merged
}
