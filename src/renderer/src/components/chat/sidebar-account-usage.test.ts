import { describe, expect, it } from 'vitest'
import type { HubGatewayQuotaWindow } from '@shared/hub-account'
import {
  quotaWindowProgress,
  selectUsageQuotaWindows,
  usageQuotaEmptyState
} from './sidebar-account-usage'

function quotaWindow(overrides: Partial<HubGatewayQuotaWindow>): HubGatewayQuotaWindow {
  return {
    id: overrides.id ?? 'window',
    scopeType: overrides.scopeType ?? 'plan',
    scopeId: overrides.scopeId ?? 'professional',
    windowKind: overrides.windowKind ?? 'session_5h',
    windowStart: overrides.windowStart ?? null,
    windowEnd: overrides.windowEnd ?? null,
    usedBillableTokens: overrides.usedBillableTokens ?? 0,
    reservedBillableTokens: overrides.reservedBillableTokens ?? 0,
    limit: overrides.limit ?? 100,
    resetAt: overrides.resetAt ?? null,
    requestCount: overrides.requestCount ?? 0,
    label: overrides.label ?? ''
  }
}

describe('sidebar account usage helpers', () => {
  it('counts reserved tokens as occupied quota and exposes remaining quota', () => {
    const progress = quotaWindowProgress(quotaWindow({
      usedBillableTokens: 30,
      reservedBillableTokens: 20,
      limit: 100
    }))

    expect(progress.occupied).toBe(50)
    expect(progress.remaining).toBe(50)
    expect(progress.usedPercent).toBe(50)
    expect(progress.remainingPercent).toBe(50)
    expect(progress.exhausted).toBe(false)
  })

  it('keeps policy and provider quota windows visible', () => {
    const selected = selectUsageQuotaWindows([
      quotaWindow({ id: 'plan', scopeType: 'plan', windowKind: 'week', usedBillableTokens: 10 }),
      quotaWindow({ id: 'policy', scopeType: 'policy', scopeId: 'global', windowKind: 'month', usedBillableTokens: 20 }),
      quotaWindow({ id: 'provider', scopeType: 'provider', scopeId: 'deepseek', windowKind: 'day', usedBillableTokens: 30 })
    ])

    expect(selected.map((window) => window.id)).toEqual(['provider', 'plan', 'policy'])
  })

  it('orders same-kind windows by higher pressure first', () => {
    const selected = selectUsageQuotaWindows([
      quotaWindow({ id: 'plan-low', scopeType: 'plan', windowKind: 'session_5h', usedBillableTokens: 1 }),
      quotaWindow({ id: 'policy-high', scopeType: 'policy', windowKind: 'session_5h', usedBillableTokens: 99 })
    ])

    expect(selected.map((window) => window.id)).toEqual(['policy-high', 'plan-low'])
  })

  it('explains empty admin quota windows instead of rendering an empty usage panel', () => {
    const emptyState = usageQuotaEmptyState({
      user: {
        id: 'user-1',
        email: 'admin@example.invalid',
        fullName: 'Admin',
        displayName: '',
        username: '',
        avatarColor: '',
        avatarStyle: 'initials',
        avatarUrl: '',
        organization: '',
        plan: 'admin',
        storedPlan: '',
        tokenLimit: 100_000_000,
        tokenBalance: 100_000_000,
        professionalUntil: null,
        notificationEmail: 'admin@example.invalid',
        emailVerified: true,
        phoneNumber: '',
        phoneVerified: false,
        realNameVerified: false,
        realNameVerifiedAt: null,
        identityNumberMasked: '',
        realNameVerificationRequired: false,
        realNameVerificationAvailable: false,
        accountReady: true,
        createdAt: ''
      },
      gatewayUsageSummary: {
        unit: 'ai_credits',
        requestCount30d: 0,
        completedCount30d: 0,
        issueCount30d: 0,
        rawTokensToday: 0,
        billableTokensToday: 0,
        rawTokens30d: 0,
        billableTokens30d: 0,
        cachedPromptTokens30d: 0,
        qwenBillableTokens30d: 0,
        deepseekBillableTokens30d: 0,
        mimoBillableTokens30d: 0
      },
      gatewayQuotaWindows: []
    })

    expect(emptyState.title).toBe('管理员账号无窗口限制')
    expect(emptyState.detail).toContain('100,000,000 / 100,000,000 AI Credits')
    expect(emptyState.detail).toContain('5 小时、本周和月度窗口不适用')
  })
})
