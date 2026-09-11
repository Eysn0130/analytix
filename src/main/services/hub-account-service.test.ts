import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  defaultAnalytixRuntimeSettings,
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../../shared/app-settings'
import { HUB_MODEL_PROVIDER_ID } from '../../shared/hub-account'

let userDataPath = ''

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getPath: vi.fn(() => userDataPath)
  }
}))

function settings(): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

function store() {
  return {
    load: vi.fn(async () => settings()),
    patch: vi.fn(async () => settings())
  }
}

function authFilePath(): string {
  return join(userDataPath, 'secrets', 'analytix-hub-auth.json')
}

function jsonResponse(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload
  }
}

function syntheticLoginPayload(options: {
  desktopToken?: string
  desktopExpiresAt?: string | null
  gatewayToken?: string
  gatewayExpiresAt?: string | null
  accountReady?: boolean
} = {}): Record<string, unknown> {
  const desktopToken = options.desktopToken ?? 'synthetic-desktop-token'
  const accountReady = options.accountReady ?? true
  return {
    user: {
      id: 'synthetic-user-id',
      email: 'synthetic-user@example.invalid',
      fullName: 'Synthetic User',
      plan: 'professional',
      accountReady
    },
    desktopAuth: {
      token: desktopToken,
      expiresAt: options.desktopExpiresAt ?? null,
      edition: 'professional',
      canSwitch: false,
      plan: 'professional'
    },
    ...(options.gatewayToken
      ? {
          gateway: {
            token: options.gatewayToken,
            baseUrl: 'https://hub.example/v1',
            expiresAt: options.gatewayExpiresAt ?? null
          }
        }
      : {})
  }
}

function syntheticEntitlementPayload(options: {
  gatewayToken?: string
  accountReady?: boolean
} = {}): Record<string, unknown> {
  const accountReady = options.accountReady ?? true
  return {
    user: {
      id: 'synthetic-user-id',
      email: 'synthetic-user@example.invalid',
      fullName: 'Synthetic User',
      plan: 'professional',
      accountReady
    },
    entitlement: {
      plan: 'professional',
      edition: 'professional',
      canSwitch: false
    },
    ...(options.gatewayToken
      ? {
          gateway: {
            token: options.gatewayToken,
            baseUrl: 'https://hub.example/v1'
          }
        }
      : {})
  }
}

function setBootstrapEnv(): void {
  process.env.ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP = '1'
  process.env.ANALYTIX_HUB_TEST_EMAIL = 'desktop-test@example.invalid'
  process.env.ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN = 'desktop-auth-test-token'
  process.env.ANALYTIX_HUB_TEST_GATEWAY_TOKEN = 'gateway-test-token'
  process.env.ANALYTIX_HUB_TEST_GATEWAY_BASE_URL = 'https://hub.example/v1'
}

function clearBootstrapEnv(): void {
  delete process.env.ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP
  delete process.env.ANALYTIX_HUB_TEST_EMAIL
  delete process.env.ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN
  delete process.env.ANALYTIX_HUB_TEST_GATEWAY_TOKEN
  delete process.env.ANALYTIX_HUB_TEST_GATEWAY_BASE_URL
}

describe('HubAccountService bootstrap and secret handling', () => {
  beforeEach(() => {
    userDataPath = mkdtempSync(join(tmpdir(), 'analytix-hub-account-'))
    clearBootstrapEnv()
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).endsWith('/models')) {
        return {
          ok: false,
          status: 503,
          json: async () => ({ error: 'models unavailable in unit test' })
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ ok: true })
      }
    }))
  })

  afterEach(async () => {
    const { clearHubGatewayRuntimeToken } = await import('./hub-gateway-runtime-secret')
    clearHubGatewayRuntimeToken()
    clearBootstrapEnv()
    vi.unstubAllGlobals()
    rmSync(userDataPath, { recursive: true, force: true })
  })

  it('ignores test bootstrap in packaged builds', async () => {
    setBootstrapEnv()
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => true,
      nodeEnv: () => 'production'
    })

    await expect(service.getSnapshot()).resolves.toMatchObject({
      authenticated: false,
      gatewayConfigured: false,
      accountReady: false,
      source: 'none'
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('allows guarded dev bootstrap and keeps gateway token out of settings', async () => {
    setBootstrapEnv()
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ data: [{ id: 'deepseek-v4-pro', owned_by: 'hub' }] })
    })))
    const { createHubAccountService, gatewayTokenFilePath } = await import('./hub-account-service')
    const { hubGatewayRuntimeTokenForProvider } = await import('./hub-gateway-runtime-secret')
    const testStore = store()
    const service = createHubAccountService({
      store: testStore as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    const snapshot = await service.getSnapshot()

    expect(snapshot).toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: true,
      source: 'test-bootstrap'
    })
    expect(snapshot.gateway).not.toHaveProperty('token')
    expect(existsSync(gatewayTokenFilePath())).toBe(true)
    expect(hubGatewayRuntimeTokenForProvider(HUB_MODEL_PROVIDER_ID)).toBe('gateway-test-token')
    expect(testStore.patch).not.toHaveBeenCalledWith(expect.objectContaining({
      provider: expect.objectContaining({ apiKey: 'gateway-test-token' })
    }))
  })

  it('uses packaged identity rather than bundler NODE_ENV for the explicit test bootstrap', async () => {
    setBootstrapEnv()
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'production'
    })

    await expect(service.getSnapshot()).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      source: 'test-bootstrap'
    })
    vi.mocked(fetch).mockClear()
    await expect(service.refresh()).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      source: 'test-bootstrap'
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('forwards remember settings and persists only the desktop auth token', async () => {
    const loginBodies: Array<Record<string, unknown>> = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).endsWith('/api/auth/login')) {
        loginBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
        return jsonResponse(syntheticLoginPayload({
          desktopToken: `synthetic-desktop-token-${loginBodies.length}`,
          accountReady: false
        }))
      }
      return jsonResponse({})
    }))
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({
      email: 'synthetic-user@example.invalid',
      password: '',
      rememberForDays: 30
    })
    expect(loginBodies[0]).toMatchObject({ rememberForDays: 30, rememberMe: true })
    const rememberedAuth = JSON.parse(readFileSync(authFilePath(), 'utf8')) as Record<string, unknown>
    expect(rememberedAuth).toMatchObject({ desktopAuth: { token: 'synthetic-desktop-token-1' } })
    expect(rememberedAuth).not.toHaveProperty('password')
    expect(rememberedAuth).not.toHaveProperty('rememberForDays')
    expect(JSON.stringify(rememberedAuth.desktopAuth)).not.toContain('password')

    await service.login({
      email: 'synthetic-user@example.invalid',
      password: '',
      rememberForDays: 0
    })
    expect(loginBodies[1]).toMatchObject({ rememberForDays: 0, rememberMe: false })
    const nonRememberedAuth = JSON.parse(readFileSync(authFilePath(), 'utf8')) as Record<string, unknown>
    expect(nonRememberedAuth).toMatchObject({ desktopAuth: { token: 'synthetic-desktop-token-2' } })
    expect(nonRememberedAuth).not.toHaveProperty('password')
    expect(nonRememberedAuth).not.toHaveProperty('rememberForDays')
    expect(JSON.stringify(nonRememberedAuth.desktopAuth)).not.toContain('password')
  })

  it('reuses persisted desktop auth across service restart and refreshes entitlement', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), init })
      if (String(url).endsWith('/api/auth/login')) {
        return jsonResponse(syntheticLoginPayload({
          desktopToken: 'synthetic-desktop-token-restart',
          accountReady: true
        }))
      }
      if (String(url).endsWith('/api/desktop/entitlement')) {
        return jsonResponse(syntheticEntitlementPayload({
          gatewayToken: 'synthetic-gateway-token-refresh'
        }))
      }
      if (String(url).endsWith('/models')) {
        return jsonResponse({ data: [{ id: 'synthetic-model', owned_by: 'hub' }] })
      }
      return jsonResponse({})
    }))
    const { createHubAccountService } = await import('./hub-account-service')
    const initialService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await initialService.login({
      email: 'synthetic-user@example.invalid',
      password: '',
      rememberForDays: 30
    })
    calls.length = 0

    const restartedService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    const snapshot = await restartedService.refresh()

    expect(calls.filter((call) => call.url.endsWith('/api/auth/login'))).toHaveLength(0)
    const entitlementCall = calls.find((call) => call.url.endsWith('/api/desktop/entitlement'))
    expect(entitlementCall?.init?.headers).toMatchObject({
      Authorization: 'Bearer synthetic-desktop-token-restart'
    })
    expect(snapshot).toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: true,
      user: { id: 'synthetic-user-id' },
      models: [{ id: 'synthetic-model' }]
    })
  })

  it('clears persisted auth and gateway tokens when entitlement returns 401', async () => {
    let mode: 'seed' | 'expired' = 'seed'
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (mode === 'seed' && String(url).endsWith('/api/auth/login')) {
        return jsonResponse(syntheticLoginPayload({
          desktopToken: 'synthetic-desktop-token-401',
          gatewayToken: 'synthetic-gateway-token-401'
        }))
      }
      if (mode === 'seed' && String(url).endsWith('/models')) {
        return jsonResponse({ data: [{ id: 'synthetic-model-401', owned_by: 'hub' }] })
      }
      if (mode === 'expired' && String(url).endsWith('/api/desktop/entitlement')) {
        return {
          ok: false,
          status: 401,
          json: async () => ({ error: 'synthetic unauthorized response' })
        }
      }
      return jsonResponse({})
    }))
    const { createHubAccountService, gatewayTokenFilePath } = await import('./hub-account-service')
    const initialService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await initialService.login({
      email: 'synthetic-user@example.invalid',
      password: '',
      rememberForDays: 30
    })
    expect(existsSync(authFilePath())).toBe(true)
    expect(existsSync(gatewayTokenFilePath())).toBe(true)

    mode = 'expired'
    const restartedService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await expect(restartedService.refresh()).resolves.toMatchObject({
      authenticated: false,
      gatewayConfigured: false,
      accountReady: false,
      source: 'none',
      error: '桌面授权已过期，请重新登录。'
    })
    expect(existsSync(authFilePath())).toBe(false)
    expect(existsSync(gatewayTokenFilePath())).toBe(false)
  })

  it('retains persisted auth and returns a fallback snapshot when entitlement returns 503', async () => {
    let mode: 'seed' | 'unavailable' = 'seed'
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (mode === 'seed' && String(url).endsWith('/api/auth/login')) {
        return jsonResponse(syntheticLoginPayload({
          desktopToken: 'synthetic-desktop-token-503',
          gatewayToken: 'synthetic-gateway-token-503'
        }))
      }
      if (mode === 'seed' && String(url).endsWith('/models')) {
        return jsonResponse({ data: [{ id: 'synthetic-model-503', owned_by: 'hub' }] })
      }
      if (mode === 'unavailable' && String(url).endsWith('/api/desktop/entitlement')) {
        return {
          ok: false,
          status: 503,
          json: async () => ({ error: 'synthetic unavailable response' })
        }
      }
      return jsonResponse({})
    }))
    const { createHubAccountService, gatewayTokenFilePath } = await import('./hub-account-service')
    const initialService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await initialService.login({
      email: 'synthetic-user@example.invalid',
      password: '',
      rememberForDays: 30
    })
    expect(existsSync(authFilePath())).toBe(true)
    expect(existsSync(gatewayTokenFilePath())).toBe(true)

    mode = 'unavailable'
    const restartedService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await expect(restartedService.refresh()).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: true,
      error: 'Hub request failed with status 503.'
    })
    expect(existsSync(authFilePath())).toBe(true)
    expect(existsSync(gatewayTokenFilePath())).toBe(true)
  })

  it('does not locally reject an expired desktop token when entitlement remains valid', async () => {
    let mode: 'seed' | 'refresh' = 'seed'
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (mode === 'seed' && String(url).endsWith('/api/auth/login')) {
        return jsonResponse(syntheticLoginPayload({
          desktopToken: 'synthetic-desktop-token-expired',
          desktopExpiresAt: '2000-01-01T00:00:00.000Z',
          accountReady: false
        }))
      }
      if (mode === 'refresh' && String(url).endsWith('/api/desktop/entitlement')) {
        return jsonResponse(syntheticEntitlementPayload({ accountReady: false }))
      }
      return jsonResponse({})
    }))
    const { createHubAccountService } = await import('./hub-account-service')
    const initialService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await initialService.login({
      email: 'synthetic-user@example.invalid',
      password: '',
      rememberForDays: 30
    })

    mode = 'refresh'
    const restartedService = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await expect(restartedService.refresh()).resolves.toMatchObject({
      authenticated: true,
      accountReady: false
    })
    expect(existsSync(authFilePath())).toBe(true)
  })

  it('clears persisted and in-memory gateway tokens on logout', async () => {
    setBootstrapEnv()
    const { createHubAccountService, gatewayTokenFilePath } = await import('./hub-account-service')
    const { hubGatewayRuntimeTokenForProvider } = await import('./hub-gateway-runtime-secret')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })
    await service.getSnapshot()
    expect(existsSync(gatewayTokenFilePath())).toBe(true)

    await service.logout()

    expect(existsSync(gatewayTokenFilePath())).toBe(false)
    expect(hubGatewayRuntimeTokenForProvider(HUB_MODEL_PROVIDER_ID)).toBe('')
  })

  it('keeps Hub error bodies out of logout diagnostics', async () => {
    const logoutBody = vi.fn(async () => ({ error: 'private upstream detail' }))
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            },
            gateway: {
              token: 'gateway-token',
              baseUrl: 'https://hub.example/v1'
            }
          })
        }
      }
      if (String(url).endsWith('/models')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ data: [{ id: 'deepseek-v4-pro', owned_by: 'hub' }] })
        }
      }
      if (String(url).endsWith('/api/desktop/logout')) {
        return {
          ok: false,
          status: 502,
          json: logoutBody
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))
    const logError = vi.fn()
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      logError,
      restartRuntime: vi.fn(async () => undefined),
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({ email: 'desktop-test@example.invalid', password: 'password' })
    await service.logout()

    expect(logoutBody).not.toHaveBeenCalled()
    expect(JSON.stringify(logError.mock.calls)).not.toContain('private upstream detail')
    expect(logError).toHaveBeenCalledWith(
      'hub-account',
      'Hub logout failed',
      {
        code: 'hub_logout_failed',
        status: 502,
        failureType: 'HubRequestError'
      }
    )
  })

  it('falls back to desktop entitlement when the desktop console endpoint is missing', async () => {
    const calls: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      calls.push(String(url))
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            }
          })
        }
      }
      if (String(url).endsWith('/api/desktop/console')) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: '未找到对应 API。' })
        }
      }
      if (String(url).endsWith('/api/desktop/entitlement')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            entitlement: {
              plan: 'professional',
              edition: 'professional',
              canSwitch: false
            },
            gatewayQuotaWindows: [{
              id: 'professional:week',
              scopeType: 'plan',
              scopeId: 'professional',
              windowKind: 'week',
              windowStart: '2026-07-01T00:00:00.000Z',
              windowEnd: '2026-07-08T00:00:00.000Z',
              usedBillableTokens: 300,
              reservedBillableTokens: 20,
              limit: 2000,
              resetAt: '2026-07-08T00:00:00.000Z',
              requestCount: 3,
              label: '专业版 本周'
            }]
          })
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))

    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({ email: 'desktop-test@example.invalid', password: 'password' })
    const result = await service.usage()

    expect(result).toMatchObject({
      ok: true,
      user: {
        email: 'desktop-test@example.invalid',
        tokenLimit: 2000,
        tokenBalance: 1200
      },
      gatewayUsageSummary: {
        completedCount30d: 0,
        billableTokensToday: 0,
        billableTokens30d: 0
      },
      gatewayQuotaWindows: [{
        id: 'professional:week',
        usedBillableTokens: 300,
        reservedBillableTokens: 20,
        limit: 2000,
        label: '专业版 本周'
      }]
    })
    expect(calls.some((url) => url.endsWith('/api/desktop/console'))).toBe(true)
    expect(calls.some((url) => url.endsWith('/api/desktop/entitlement'))).toBe(true)
  })

  it('normalizes authoritative gateway profile data from the desktop console endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            }
          })
        }
      }
      if (String(url).endsWith('/api/desktop/console')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            gatewayUsageSummary: {
              requestCount30d: 4,
              completedCount30d: 4,
              issueCount30d: 0,
              rawTokensToday: 1200,
              billableTokensToday: 1000,
              rawTokens30d: 6200,
              billableTokens30d: 5000,
              cachedPromptTokens30d: 0,
              qwenBillableTokens30d: 1000,
              deepseekBillableTokens30d: 2000,
              mimoBillableTokens30d: 2000
            },
            gatewayQuotaWindows: [],
            gatewayProfile: {
              timezone: 'Asia/Shanghai',
              dailyUsage: [{
                date: '2026-07-03',
                tokens: 1000,
                rawTokens: 1200,
                requestCount: 2,
                maxLatencyMs: 5000
              }],
              summary: {
                totalTextTokens: 5000,
                totalRawTokens: 6200,
                peakTokens: 3000,
                longestRequestDurationMs: 45000,
                currentStreakDays: 2,
                longestStreakDays: 7,
                totalRequests: 4,
                activeDays: 3,
                providerCount: 2,
                modelCount: 3
              },
              activityInsights: {
                fastModeUsagePercentage: null,
                mostUsedReasoningEffort: null,
                mostUsedReasoningEffortPercentage: null,
                uniqueSkillsUsed: null,
                totalSkillsUsed: null,
                totalRequests: 4,
                source: 'hub_llm_requests',
                missingFields: ['skill_invocations']
              },
              topInvocations: [],
              dataCompleteness: {
                tokenActivity: true,
                profileSummary: true,
                pluginInvocations: false,
                reasoningEffort: false,
                quickMode: false
              }
            }
          })
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))

    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({ email: 'desktop-test@example.invalid', password: 'password' })
    const result = await service.usage()

    expect(result).toMatchObject({
      ok: true,
      gatewayProfile: {
        timezone: 'Asia/Shanghai',
        dailyUsage: [{
          date: '2026-07-03',
          tokens: 1000,
          rawTokens: 1200,
          requestCount: 2,
          maxLatencyMs: 5000
        }],
        summary: {
          totalTextTokens: 5000,
          totalRawTokens: 6200,
          peakTokens: 3000,
          longestRequestDurationMs: 45000,
          currentStreakDays: 2,
          longestStreakDays: 7,
          totalRequests: 4,
          activeDays: 3,
          providerCount: 2,
          modelCount: 3
        },
        dataCompleteness: {
          pluginInvocations: false,
          reasoningEffort: false,
          quickMode: false
        }
      }
    })
    expect(result.ok && result.gatewayProfile?.activityInsights.missingFields).toContain('skill_invocations')
  })

  it('syncs desktop profile events with the stored desktop auth token', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), init })
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            }
          })
        }
      }
      if (String(url).endsWith('/api/desktop/profile-events')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ acceptedCount: 1, syncedCount: 1 })
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))

    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({ email: 'desktop-test@example.invalid', password: 'password' })
    const result = await service.syncProfileEvents({
      events: [{
        eventKey: 'thread-detail:test',
        occurredAt: '2026-07-04T12:00:00.000Z',
        source: 'desktop',
        eventKind: 'thread_detail',
        threadIdHash: 'test',
        mode: 'agent',
        reasoningEffort: 'auto',
        durationMs: 1200,
        toolInvocations: [{ name: 'read', count: 2 }],
        skillInvocations: [{ name: '@analytix-fund-analysis', count: 1 }]
      }]
    })

    expect(result).toMatchObject({ ok: true, acceptedCount: 1, syncedCount: 1 })
    const syncCall = calls.find((call) => call.url.endsWith('/api/desktop/profile-events'))
    expect(syncCall?.init?.method).toBe('POST')
    expect(syncCall?.init?.headers).toMatchObject({ Authorization: 'Bearer desktop-token' })
    expect(JSON.parse(String(syncCall?.init?.body))).toMatchObject({
      events: [{
        eventKey: 'thread-detail:test',
        skillInvocations: [{ name: '@analytix-fund-analysis', count: 1 }]
      }]
    })
  })

  it('backfills local thread profile events from the runtime data dir', async () => {
    const dataDir = mkdtempSync(join(tmpdir(), 'analytix-profile-data-'))
    const threadDir = join(dataDir, 'threads', 'thr_profile')
    const emptyThreadDir = join(dataDir, 'threads', 'thr_empty')
    mkdirSync(threadDir, { recursive: true })
    mkdirSync(emptyThreadDir, { recursive: true })
    writeFileSync(join(emptyThreadDir, 'metadata.jsonl'), [
      JSON.stringify({
        kind: 'thread_metadata',
        thread: {
          id: 'thr_empty',
          mode: 'agent',
          updatedAt: '2026-07-04T12:00:04.000Z',
          turns: []
        }
      })
    ].join('\n'), 'utf8')
    writeFileSync(join(threadDir, 'metadata.jsonl'), [
      JSON.stringify({
        kind: 'thread_metadata',
        thread: {
          id: 'thr_profile',
          mode: 'agent',
          updatedAt: '2026-07-04T12:00:03.000Z',
          turns: [{
            id: 'turn_1',
            reasoningEffort: 'high',
            activeSkillIds: ['$frontend-design'],
            startedAt: '2026-07-04T12:00:00.000Z',
            finishedAt: '2026-07-04T12:00:03.500Z'
          }]
        }
      })
    ].join('\n'), 'utf8')
    writeFileSync(join(threadDir, 'messages.jsonl'), [
      JSON.stringify({
        id: 'user_1',
        turnId: 'turn_1',
        role: 'user',
        kind: 'user_message',
        createdAt: '2026-07-04T12:00:00.000Z',
        text: '请使用 plugin://analytix-fund-analysis@1.0.0 分析。'
      }),
      JSON.stringify({
        id: 'tool_1_pending',
        turnId: 'turn_1',
        kind: 'tool_call',
        role: 'tool',
        status: 'pending',
        callId: 'call_1',
        toolName: 'mcp__analytix_funds__rank_holders',
        createdAt: '2026-07-04T12:00:01.000Z'
      }),
      JSON.stringify({
        id: 'tool_1_done',
        turnId: 'turn_1',
        kind: 'tool_call',
        role: 'tool',
        status: 'completed',
        callId: 'call_1',
        toolName: 'mcp__analytix_funds__rank_holders',
        createdAt: '2026-07-04T12:00:01.000Z',
        finishedAt: '2026-07-04T12:00:02.000Z'
      })
    ].join('\n'), 'utf8')

    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), init })
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            }
          })
        }
      }
      if (String(url).endsWith('/api/desktop/profile-events')) {
        const body = JSON.parse(String(init?.body || '{}')) as { events?: unknown[] }
        return {
          ok: true,
          status: 200,
          json: async () => ({
            acceptedCount: body.events?.length ?? 0,
            syncedCount: body.events?.length ?? 0
          })
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))

    const appSettings = settings()
    appSettings.runtime.dataDir = dataDir
    const testStore = {
      load: vi.fn(async () => appSettings),
      patch: vi.fn(async () => appSettings)
    }
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: testStore as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({ email: 'desktop-test@example.invalid', password: 'password' })
    const result = await service.syncLocalProfileEvents()

    expect(result).toMatchObject({
      ok: true,
      acceptedCount: 1,
      syncedCount: 1,
      scannedThreadCount: 2,
      generatedEventCount: 1
    })
    const syncCall = calls.find((call) => call.url.endsWith('/api/desktop/profile-events'))
    const payload = JSON.parse(String(syncCall?.init?.body))
    expect(payload.events[0]).toMatchObject({
      eventKind: 'thread_detail',
      reasoningEffort: 'high',
      toolInvocations: [{ name: 'mcp__analytix_funds__rank_holders', count: 1 }]
    })
    expect(payload.events[0].skillInvocations).toEqual(
      expect.arrayContaining([
        { name: '@analytix-fund-analysis', count: 2 },
        { name: '$frontend-design', count: 1 }
      ])
    )

    rmSync(dataDir, { recursive: true, force: true })
  })

  it('waits for model profile sync and runtime restart before login becomes ready', async () => {
    let releaseModels!: () => void
    let releaseRestart!: () => void
    const modelSync = new Promise((resolve) => {
      releaseModels = () => resolve({
        ok: true,
        status: 200,
        json: async () => ({
          data: [
            { id: 'deepseek-v4-pro', owned_by: 'hub' },
            { id: 'deepseek-v4-flash', owned_by: 'hub' },
            { id: 'qwen3.7-plus', owned_by: 'hub' }
          ]
        })
      })
    })
    const runtimeRestart = new Promise<void>((resolve) => {
      releaseRestart = resolve
    })
    const restartRuntime = vi.fn(() => runtimeRestart)

    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            },
            gateway: {
              token: 'gateway-token',
              baseUrl: 'https://hub.example/v1'
            }
          })
        }
      }
      if (String(url).endsWith('/models')) {
        return modelSync
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))

    const testStore = store()
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: testStore as never,
      restartRuntime,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    let loginSettled = false
    const loginPromise = service.login({ email: 'desktop-test@example.invalid', password: 'password' }).then((snapshot) => {
      loginSettled = true
      return snapshot
    })
    await expect(Promise.race([
      loginPromise.then(() => 'logged-in'),
      new Promise((resolve) => setTimeout(() => resolve('blocked'), 25))
    ])).resolves.toBe('blocked')

    let concurrentSnapshotSettled = false
    const concurrentSnapshot = service.getSnapshot().then((snapshot) => {
      concurrentSnapshotSettled = true
      return snapshot
    })
    await expect(Promise.race([
      concurrentSnapshot.then(() => 'snapshot'),
      new Promise((resolve) => setTimeout(() => resolve('blocked'), 25))
    ])).resolves.toBe('blocked')

    expect(restartRuntime).not.toHaveBeenCalled()
    releaseModels()
    await vi.waitFor(() => expect(restartRuntime).toHaveBeenCalledTimes(1))
    expect(loginSettled).toBe(false)
    expect(testStore.patch).toHaveBeenCalledWith(
      expect.objectContaining({
        runtime: expect.objectContaining({
          providerId: HUB_MODEL_PROVIDER_ID,
          model: 'deepseek-v4-flash'
        }),
        provider: expect.objectContaining({
          activeProviderId: HUB_MODEL_PROVIDER_ID,
          providers: expect.arrayContaining([
            expect.objectContaining({
              id: HUB_MODEL_PROVIDER_ID,
              modelProfiles: expect.objectContaining({
                'deepseek-v4-pro': expect.objectContaining({
                  contextWindowTokens: 131_072,
                  reasoning: {
                    supportedEfforts: ['off', 'high', 'max'],
                    defaultEffort: 'high',
                    requestProtocol: 'deepseek-chat-completions'
                  }
                }),
                'deepseek-v4-flash': expect.objectContaining({
                  contextWindowTokens: 131_072
                }),
                'qwen3.7-plus': expect.objectContaining({
                  contextWindowTokens: 262_144
                })
              })
            })
          ])
        })
      })
    )
    expect(testStore.patch).toHaveBeenCalledWith(expect.objectContaining({
      provider: expect.objectContaining({
        providers: expect.arrayContaining([expect.objectContaining({
          id: HUB_MODEL_PROVIDER_ID,
          modelProfiles: expect.objectContaining({
            'qwen3.7-plus': expect.not.objectContaining({
              reasoning: expect.anything()
            })
          })
        })])
      })
    }))
    releaseRestart()
    await expect(loginPromise).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: true,
      models: [
        { id: 'deepseek-v4-pro', ownedBy: 'hub' },
        { id: 'deepseek-v4-flash', ownedBy: 'hub' },
        { id: 'qwen3.7-plus', ownedBy: 'hub' }
      ]
    })
    await expect(concurrentSnapshot).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: true
    })
    expect(concurrentSnapshotSettled).toBe(true)
  })

  it('keeps valid auth but fails closed when gateway initialization fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).endsWith('/api/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            user: {
              id: 'user-1',
              email: 'desktop-test@example.invalid',
              fullName: 'Desktop Test',
              plan: 'professional',
              tokenLimit: 2000,
              tokenBalance: 1200,
              accountReady: true
            },
            desktopAuth: {
              token: 'desktop-token',
              edition: 'professional',
              canSwitch: false,
              plan: 'professional'
            },
            gateway: {
              token: 'gateway-token',
              baseUrl: 'https://hub.example/v1'
            }
          })
        }
      }
      if (String(url).endsWith('/models')) {
        return {
          ok: false,
          status: 503,
          json: async () => ({ error: 'private upstream detail' })
        }
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({})
      }
    }))
    const restartRuntime = vi.fn(async () => undefined)
    const logError = vi.fn()
    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      restartRuntime,
      logError,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await expect(service.login({ email: 'desktop-test@example.invalid', password: 'password' })).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: false,
      models: [],
      error: '登录成功，但模型网关初始化失败，请刷新重试。'
    })
    await expect(service.getSnapshot()).resolves.toMatchObject({
      authenticated: true,
      gatewayConfigured: true,
      accountReady: false,
      models: []
    })
    expect(restartRuntime).not.toHaveBeenCalled()
    expect(JSON.stringify(logError.mock.calls)).not.toContain('private upstream detail')
    expect(logError).toHaveBeenCalledWith(
      'hub-account',
      'Failed to initialize Hub gateway after Hub login',
      expect.objectContaining({ code: 'hub_gateway_initialization_failed' })
    )
  })

  it('observes only value-safe dimensions for an explicit deprecated Hub action', async () => {
    vi.resetModules()
    const observation = await import('../hub-activity-observation')
    observation.resetHubActivityForTests()
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).endsWith('/api/auth/login')) {
        return jsonResponse(syntheticLoginPayload({
          desktopToken: 'synthetic-observation-desktop-token',
          gatewayToken: 'synthetic-observation-gateway-token'
        }))
      }
      if (String(url).endsWith('/models')) {
        return jsonResponse({ data: [{ id: 'synthetic-observation-model', owned_by: 'hub' }] })
      }
      return jsonResponse({})
    }))

    const { createHubAccountService } = await import('./hub-account-service')
    const service = createHubAccountService({
      store: store() as never,
      isPackaged: () => false,
      nodeEnv: () => 'test'
    })

    await service.login({
      email: 'synthetic-observation@example.invalid',
      password: 'synthetic-observation-canary',
      rememberForDays: 0
    })

    const result = observation.observeHubActivity()
    expect(result).toMatchObject({
      schemaVersion: 1,
      moduleLoad: 2,
      serviceInstance: 1,
      refreshTimer: 0,
      request: 2,
      fallback: 1,
      anyActivity: true
    })
    expect(result.tokenRead).toBeGreaterThan(0)
    expect(Object.keys(result)).toEqual([
      'schemaVersion',
      'moduleLoad',
      'serviceInstance',
      'refreshTimer',
      'tokenRead',
      'request',
      'fallback',
      'anyActivity'
    ])
  })
})
