import { z } from 'zod'

export const CostEstimateStatusSchema = z.enum(['none', 'complete', 'partial', 'unknown'])
export type CostEstimateStatus = z.infer<typeof CostEstimateStatusSchema>
export const CostKnownCurrenciesSchema = z.array(z.enum(['USD', 'CNY'])).max(2)
  .refine((currencies) => currencies.length < 2 ||
    (currencies[0] === 'USD' && currencies[1] === 'CNY'))

export function isConsistentCostCoverage(
  value: object,
  style: 'camel' | 'snake'
): boolean {
  const record = value as Record<string, unknown>
  const statusKey = style === 'camel' ? 'costEstimateStatus' : 'cost_estimate_status'
  const currenciesKey = style === 'camel' ? 'costKnownCurrencies' : 'cost_known_currencies'
  const configuredKey = style === 'camel' ? 'priceConfigured' : 'price_configured'
  const usdKey = style === 'camel' ? 'costUsd' : 'cost_usd'
  const cnyKey = style === 'camel' ? 'costCny' : 'cost_cny'
  const statusPresent = Object.prototype.hasOwnProperty.call(record, statusKey)
  const currenciesPresent = Object.prototype.hasOwnProperty.call(record, currenciesKey)
  if (!statusPresent && !currenciesPresent) return true // signed legacy shape
  if (!statusPresent || !currenciesPresent) return false
  const status = record[statusKey]
  const currencies = record[currenciesKey]
  if (!CostEstimateStatusSchema.safeParse(status).success || !CostKnownCurrenciesSchema.safeParse(currencies).success ||
      record[configuredKey] !== (status === 'complete')) return false
  const known = currencies as Array<'USD' | 'CNY'>
  const usd = record[usdKey]
  const cny = record[cnyKey]
  if (typeof usd !== 'number' || !Number.isFinite(usd) || usd < 0 ||
      typeof cny !== 'number' || !Number.isFinite(cny) || cny < 0) return false
  if ((status === 'complete' || status === 'partial') !== (known.length > 0)) return false
  if (status === 'none' && [record.turns, record.totalTokens, record.total_tokens, record.cachedTokens,
    record.cached_tokens, record.cacheMissTokens, record.cache_miss_tokens, record.thread_count]
    .some((amount) => typeof amount === 'number' && amount > 0)) return false
  if ((status !== 'complete' && (usd !== 0 || cny !== 0)) ||
      (!known.includes('USD') && usd !== 0) || (!known.includes('CNY') && cny !== 0)) return false
  return true
}

/**
 * Token, cache, and cost counters emitted with every model response.
 *
 * `cacheHitTokens`/`cacheMissTokens` are optional because some providers
 * (or older model revisions) do not surface prompt-cache hit counts. When
 * the values are absent, `cacheHitRate` is reported as `null` rather than
 * guessing at zero.
 */
export const UsageSnapshotSchema = z.object({
  promptTokens: z.number().int().nonnegative(),
  completionTokens: z.number().int().nonnegative(),
  reasoningTokens: z.number().int().nonnegative().optional(),
  totalTokens: z.number().int().nonnegative(),
  cachedTokens: z.number().int().nonnegative().optional(),
  cacheHitTokens: z.number().int().nonnegative().optional(),
  cacheMissTokens: z.number().int().nonnegative().optional(),
  cacheHitRate: z.number().min(0).max(1).nullable(),
  cacheableTokenHitRate: z.number().min(0).max(1).nullable().optional(),
  totalInputTokenHitRate: z.number().min(0).max(1).nullable().optional(),
  cacheMissReasons: z.array(z.string()).optional(),
  cacheSuggestions: z.array(z.string()).optional(),
  turns: z.number().int().nonnegative(),
  priceConfigured: z.boolean().optional(),
  costEstimateStatus: CostEstimateStatusSchema.optional(),
  costKnownCurrencies: CostKnownCurrenciesSchema.optional(),
  costUsd: z.number().nonnegative().optional(),
  costCny: z.number().nonnegative().optional(),
  /**
   * @deprecated Savings are reported in tokens only (cache hits via
   * `cacheHitTokens`, compression via `tokenEconomySavingsTokens`).
   * The money fields remain parseable for persisted threads recorded
   * by older runtimes but are no longer populated.
   */
  cacheSavingsUsd: z.number().nonnegative().optional(),
  cacheSavingsCny: z.number().nonnegative().optional(),
  tokenEconomySavingsTokens: z.number().int().nonnegative().optional(),
  tokenEconomySavingsUsd: z.number().nonnegative().optional(),
  tokenEconomySavingsCny: z.number().nonnegative().optional(),
  /** Provider reported an unrecoverable error mid-stream. */
  hasError: z.boolean().optional()
})
export type UsageSnapshot = z.infer<typeof UsageSnapshotSchema>

const DateStringSchema = z.string().regex(/^\d{4}-\d{2}-\d{2}$/)

export const DailyUsageCountersSchema = z.object({
  input_tokens: z.number().int().nonnegative(),
  output_tokens: z.number().int().nonnegative(),
  reasoning_tokens: z.number().int().nonnegative(),
  cached_tokens: z.number().int().nonnegative(),
  cache_hit_tokens: z.number().int().nonnegative().optional(),
  cache_miss_tokens: z.number().int().nonnegative(),
  total_tokens: z.number().int().nonnegative(),
  cost_usd: z.number().nonnegative(),
  cost_cny: z.number().nonnegative(),
  price_configured: z.boolean().default(false),
  cost_estimate_status: CostEstimateStatusSchema.optional(),
  cost_known_currencies: CostKnownCurrenciesSchema.optional(),
  cache_savings_usd: z.number().nonnegative(),
  cache_savings_cny: z.number().nonnegative(),
  token_economy_savings_tokens: z.number().int().nonnegative(),
  token_economy_savings_usd: z.number().nonnegative(),
  token_economy_savings_cny: z.number().nonnegative(),
  turns: z.number().int().nonnegative(),
  thread_count: z.number().int().nonnegative(),
  cache_hit_rate: z.number().min(0).max(1).nullable()
})
export type DailyUsageCounters = z.infer<typeof DailyUsageCountersSchema>

export const DailyUsageBucketSchema = DailyUsageCountersSchema.extend({
  date: DateStringSchema
})
export type DailyUsageBucket = z.infer<typeof DailyUsageBucketSchema>

export const DailyUsageTotalsSchema = DailyUsageCountersSchema.extend({
  days: z.number().int().nonnegative(),
  active_days: z.number().int().nonnegative()
})
export type DailyUsageTotals = z.infer<typeof DailyUsageTotalsSchema>

export const DailyUsageResponseSchema = z.object({
  group_by: z.literal('day'),
  from: DateStringSchema,
  to: DateStringSchema,
  timezone: z.string().min(1),
  buckets: z.array(DailyUsageBucketSchema),
  totals: DailyUsageTotalsSchema
}).superRefine((response, ctx) => {
  for (const [index, bucket] of response.buckets.entries()) {
    if (!isConsistentCostCoverage(bucket, 'snake')) {
      ctx.addIssue({ code: 'custom', path: ['buckets', index], message: 'cost coverage is inconsistent' })
    }
  }
  if (!isConsistentCostCoverage(response.totals, 'snake')) {
    ctx.addIssue({ code: 'custom', path: ['totals'], message: 'cost coverage is inconsistent' })
  }
})
export type DailyUsageResponse = z.infer<typeof DailyUsageResponseSchema>

export const ThreadUsageBucketSchema = DailyUsageCountersSchema.omit({
  thread_count: true
}).extend({
  thread_id: z.string().min(1),
  provider: z.string().min(1).optional(),
  /**
   * Cache hit rate of the most recent turn (by completedAt), distinct from the
   * thread-cumulative `cache_hit_rate`. The cumulative rate is dragged down by
   * the unavoidable cold first turn; this reflects steady-state caching for the
   * usage chip. Null when the latest turn had no cache telemetry.
   */
  last_turn_cache_hit_rate: z.number().min(0).max(1).nullable().default(null),
  last_turn_cacheable_hit_rate: z.number().min(0).max(1).nullable().default(null),
  last_turn_total_input_hit_rate: z.number().min(0).max(1).nullable().default(null),
  last_cache_miss_reasons: z.array(z.string()).default([]),
  last_cache_suggestions: z.array(z.string()).default([])
})
export type ThreadUsageBucket = z.infer<typeof ThreadUsageBucketSchema>

export const ThreadUsageTotalsSchema = DailyUsageCountersSchema.omit({
  thread_count: true
}).extend({
  thread_count: z.number().int().nonnegative()
})
export type ThreadUsageTotals = z.infer<typeof ThreadUsageTotalsSchema>

export const ThreadUsageResponseSchema = z.object({
  group_by: z.literal('thread'),
  buckets: z.array(ThreadUsageBucketSchema),
  totals: ThreadUsageTotalsSchema
}).superRefine((response, ctx) => {
  for (const [index, bucket] of response.buckets.entries()) {
    if (!isConsistentCostCoverage(bucket, 'snake')) {
      ctx.addIssue({ code: 'custom', path: ['buckets', index], message: 'cost coverage is inconsistent' })
    }
  }
  if (!isConsistentCostCoverage(response.totals, 'snake')) {
    ctx.addIssue({ code: 'custom', path: ['totals'], message: 'cost coverage is inconsistent' })
  }
})
export type ThreadUsageResponse = z.infer<typeof ThreadUsageResponseSchema>

export const UsageSourceBucketSchema = z.object({
  source: z.string().min(1),
  usage: UsageSnapshotSchema,
  childRunIds: z.array(z.string().min(1)).default([])
})
export type UsageSourceBucket = z.infer<typeof UsageSourceBucketSchema>

export const RuntimeUsageResponseSchema = z.object({
  total: UsageSnapshotSchema,
  perThread: z.array(z.object({
    threadId: z.string().min(1),
    usage: UsageSnapshotSchema
  })),
  bySource: z.array(UsageSourceBucketSchema).default([])
}).superRefine((response, ctx) => {
  if (!isConsistentCostCoverage(response.total, 'camel')) {
    ctx.addIssue({ code: 'custom', path: ['total'], message: 'cost coverage is inconsistent' })
  }
  for (const [index, entry] of response.perThread.entries()) {
    if (!isConsistentCostCoverage(entry.usage, 'camel')) {
      ctx.addIssue({ code: 'custom', path: ['perThread', index, 'usage'], message: 'cost coverage is inconsistent' })
    }
  }
  for (const [index, entry] of response.bySource.entries()) {
    if (!isConsistentCostCoverage(entry.usage, 'camel')) {
      ctx.addIssue({ code: 'custom', path: ['bySource', index, 'usage'], message: 'cost coverage is inconsistent' })
    }
  }
})
export type RuntimeUsageResponse = z.infer<typeof RuntimeUsageResponseSchema>

export const ModelUsageBucketSchema = DailyUsageCountersSchema.extend({
  model: z.string().min(1),
  provider: z.string().min(1).optional()
})
export type ModelUsageBucket = z.infer<typeof ModelUsageBucketSchema>

export const ModelUsageDayBucketSchema = DailyUsageBucketSchema
export type ModelUsageDayBucket = z.infer<typeof ModelUsageDayBucketSchema>

export const ModelUsageResponseSchema = z.object({
  group_by: z.literal('model'),
  from: DateStringSchema,
  to: DateStringSchema,
  timezone: z.string().min(1),
  buckets: z.array(ModelUsageBucketSchema),
  days: z.array(ModelUsageDayBucketSchema),
  totals: DailyUsageTotalsSchema
}).superRefine((response, ctx) => {
  for (const [index, bucket] of response.buckets.entries()) {
    if (!isConsistentCostCoverage(bucket, 'snake')) {
      ctx.addIssue({ code: 'custom', path: ['buckets', index], message: 'cost coverage is inconsistent' })
    }
  }
  for (const [index, day] of response.days.entries()) {
    if (!isConsistentCostCoverage(day, 'snake')) {
      ctx.addIssue({ code: 'custom', path: ['days', index], message: 'cost coverage is inconsistent' })
    }
  }
  if (!isConsistentCostCoverage(response.totals, 'snake')) {
    ctx.addIssue({ code: 'custom', path: ['totals'], message: 'cost coverage is inconsistent' })
  }
})
export type ModelUsageResponse = z.infer<typeof ModelUsageResponseSchema>

export const emptyUsageSnapshot = (): UsageSnapshot => ({
  promptTokens: 0,
  completionTokens: 0,
  reasoningTokens: 0,
  totalTokens: 0,
  cachedTokens: 0,
  cacheHitTokens: 0,
  cacheMissTokens: 0,
  cacheHitRate: null,
  turns: 0,
  priceConfigured: false,
  costEstimateStatus: 'none',
  costKnownCurrencies: [],
  tokenEconomySavingsTokens: 0
})
