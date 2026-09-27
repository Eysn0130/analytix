export type CostEstimateStatus = 'none' | 'complete' | 'partial' | 'unknown'
export type CostKnownCurrency = 'USD' | 'CNY'

export type UsageCostCoverage = {
  costUsd: number | null
  costCny: number | null
  priceConfigured: boolean
  costEstimateStatus: CostEstimateStatus
  costKnownCurrencies: CostKnownCurrency[]
}

function canonicalCurrencies(value: unknown): CostKnownCurrency[] | null {
  if (!Array.isArray(value)) return null
  if (value.length === 0) return []
  if (value.length === 1 && (value[0] === 'USD' || value[0] === 'CNY')) return [value[0]]
  if (value.length === 2 && value[0] === 'USD' && value[1] === 'CNY') return ['USD', 'CNY']
  return null
}

function knownAmount(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null
}

export function normalizeUsageCost(
  raw: Record<string, unknown>,
  style: 'camel' | 'snake'
): UsageCostCoverage {
  const read = (camel: string, snake: string): unknown => {
    const primary = style === 'camel' ? camel : snake
    const fallback = style === 'camel' ? snake : camel
    return Object.prototype.hasOwnProperty.call(raw, primary) ? raw[primary] : raw[fallback]
  }
  const status = read('costEstimateStatus', 'cost_estimate_status')
  const currencies = canonicalCurrencies(read('costKnownCurrencies', 'cost_known_currencies'))
  const configured = read('priceConfigured', 'price_configured')
  const usd = knownAmount(read('costUsd', 'cost_usd'))
  const cny = knownAmount(read('costCny', 'cost_cny'))
  const unavailable: UsageCostCoverage = {
    costUsd: null,
    costCny: null,
    priceConfigured: false,
    costEstimateStatus: 'unknown',
    costKnownCurrencies: []
  }
  if (currencies === null || !['none', 'complete', 'partial', 'unknown'].includes(String(status)) ||
      configured !== (status === 'complete') || usd === null || cny === null ||
      (status === 'none' && [raw.turns, raw.totalTokens, raw.total_tokens].some((amount) => typeof amount === 'number' && amount > 0)) ||
      ((status === 'complete' || status === 'partial') !== (currencies.length > 0)) ||
      (!currencies.includes('USD') && usd !== 0) || (!currencies.includes('CNY') && cny !== 0) ||
      (status !== 'complete' && (usd !== 0 || cny !== 0))) return unavailable
  return {
    costUsd: status === 'complete' && currencies.includes('USD') ? usd : null,
    costCny: status === 'complete' && currencies.includes('CNY') ? cny : null,
    priceConfigured: status === 'complete',
    costEstimateStatus: status as CostEstimateStatus,
    costKnownCurrencies: currencies
  }
}

export function combineUsageCost(left: UsageCostCoverage, right: UsageCostCoverage): UsageCostCoverage {
  if (left.costEstimateStatus === 'none') return right
  if (right.costEstimateStatus === 'none') return left
  const currencies: CostKnownCurrency[] = []
  if (left.costKnownCurrencies.includes('USD') || right.costKnownCurrencies.includes('USD')) currencies.push('USD')
  if (left.costKnownCurrencies.includes('CNY') || right.costKnownCurrencies.includes('CNY')) currencies.push('CNY')
  const status: CostEstimateStatus = left.costEstimateStatus === 'complete' && right.costEstimateStatus === 'complete'
    ? 'complete'
    : left.costEstimateStatus === 'unknown' && right.costEstimateStatus === 'unknown'
      ? 'unknown'
      : 'partial'
  return {
    costUsd: status === 'complete' && currencies.includes('USD') ? (left.costUsd ?? 0) + (right.costUsd ?? 0) : null,
    costCny: status === 'complete' && currencies.includes('CNY') ? (left.costCny ?? 0) + (right.costCny ?? 0) : null,
    priceConfigured: status === 'complete',
    costEstimateStatus: status,
    costKnownCurrencies: currencies
  }
}
