import type { HubGatewayQuotaWindow, HubUsageResult } from '@shared/hub-account'

export type SidebarQuotaWindowProgress = {
  limit: number
  used: number
  reserved: number
  occupied: number
  remaining: number
  usedPercent: number
  remainingPercent: number
  exhausted: boolean
}

export type SidebarUsageEmptyState = {
  title: string
  detail: string
}

const WINDOW_KIND_ORDER: Record<HubGatewayQuotaWindow['windowKind'], number> = {
  session_5h: 0,
  day: 1,
  week: 2,
  month: 3,
  billing_cycle: 4
}

const SCOPE_TYPE_ORDER: Record<HubGatewayQuotaWindow['scopeType'], number> = {
  provider: 0,
  policy: 1,
  plan: 2
}

function positiveNumber(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? Math.max(0, parsed) : 0
}

function percent(numerator: number, denominator: number): number {
  if (denominator <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((numerator / denominator) * 100)))
}

export function quotaWindowProgress(window: HubGatewayQuotaWindow): SidebarQuotaWindowProgress {
  const limit = positiveNumber(window.limit)
  const used = positiveNumber(window.usedBillableTokens)
  const reserved = positiveNumber(window.reservedBillableTokens)
  const occupied = used + reserved
  const remaining = Math.max(0, limit - occupied)
  return {
    limit,
    used,
    reserved,
    occupied,
    remaining,
    usedPercent: percent(occupied, limit),
    remainingPercent: percent(remaining, limit),
    exhausted: limit > 0 && remaining <= 0
  }
}

export function formatQuotaAmount(value: number): string {
  return Math.max(0, Math.round(Number(value || 0))).toLocaleString('zh-CN')
}

export function quotaUnitLabel(unit: string | undefined): string {
  return unit === 'tokens' ? 'tokens' : 'AI Credits'
}

export function usageQuotaEmptyState(usage: HubUsageResult): SidebarUsageEmptyState {
  const unitLabel = quotaUnitLabel(usage.gatewayUsageSummary.unit)
  const balance = positiveNumber(usage.user.tokenBalance)
  const limit = positiveNumber(usage.user.tokenLimit)
  const balanceLabel = limit > 0
    ? `当前可用 ${formatQuotaAmount(balance)} / ${formatQuotaAmount(limit)} ${unitLabel}`
    : `近 30 天已用 ${formatQuotaAmount(usage.gatewayUsageSummary.billableTokens30d)} ${unitLabel}`

  if (usage.user.plan === 'admin') {
    return {
      title: '管理员账号无窗口限制',
      detail: `${balanceLabel}，5 小时、本周和月度窗口不适用于管理员账号。`
    }
  }

  return {
    title: '暂无额度窗口',
    detail: `${balanceLabel}，Hub 当前没有返回可展示的 5 小时、本周或月度额度窗口。`
  }
}

export function selectUsageQuotaWindows(windows: HubGatewayQuotaWindow[]): HubGatewayQuotaWindow[] {
  return windows
    .filter((window) => positiveNumber(window.limit) > 0)
    .slice()
    .sort((left, right) => {
      const leftKindOrder = WINDOW_KIND_ORDER[left.windowKind] ?? 99
      const rightKindOrder = WINDOW_KIND_ORDER[right.windowKind] ?? 99
      if (leftKindOrder !== rightKindOrder) return leftKindOrder - rightKindOrder

      const leftProgress = quotaWindowProgress(left)
      const rightProgress = quotaWindowProgress(right)
      if (leftProgress.usedPercent !== rightProgress.usedPercent) {
        return rightProgress.usedPercent - leftProgress.usedPercent
      }

      const leftScopeOrder = SCOPE_TYPE_ORDER[left.scopeType] ?? 99
      const rightScopeOrder = SCOPE_TYPE_ORDER[right.scopeType] ?? 99
      if (leftScopeOrder !== rightScopeOrder) return leftScopeOrder - rightScopeOrder

      return (left.label || left.id).localeCompare(right.label || right.id)
    })
}
