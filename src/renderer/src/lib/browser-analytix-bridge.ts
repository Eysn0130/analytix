import {
  DEFAULT_ANALYTIX_PORT,
  DEFAULT_GUI_UPDATE_CHANNEL,
  DEFAULT_WRITE_WORKSPACE_ROOT,
  defaultAnalytixRuntimeSettings,
  defaultClawSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  mergeAnalytixRuntimeSettings,
  mergeClawSettings,
  mergeModelProviderSettings,
  mergeScheduleSettings,
  mergeWriteSettings,
  modelCapabilityProbeKey,
  normalizeAppBehaviorSettings,
  normalizeAppSettings,
  normalizeKeyboardShortcuts,
  type AppSettingsPatch,
  type AppSettingsV1,
  type GuiUpdateChannel
} from '@shared/app-settings'
import type {
  AnalytixApi,
  AnalytixFlatApi,
  AnalytixRuntimeStatusPayload,
  ClawChannelActivityPayload,
  ComputerUsePermissionKind,
  FundsCSVSnapshotConfirmResult,
  FundsCSVSnapshotStageResult,
  FundsImportCancelResult,
  FundsImportStatusResult,
  LocalDisplayResult,
  ProviderCredentialRecoveryResult,
  ProviderRegistryResult,
  SseEndPayload,
  SseErrorPayload,
  SseEventPayload
} from '@shared/analytix-api'
import type { WorkspaceFileChangePayload } from '@shared/workspace-file'
import { createRuntimeStatusPublicV1 } from '@shared/analytix-runtime-status'
import {
  containsPrivateAcceptedFinalAuthority,
  PublicRuntimeEventFilter,
  sanitizePublicRuntimeValue
} from '@shared/public-runtime-content'
import { projectPublicRuntimeHTTPError } from '@shared/runtime-error'
import {
  isPublicRuntimeSseRejectionReasonCode,
  isStrictPublicRuntimeSseIpcPayload,
  projectPublicRuntimeSseBlock,
  takePublicRuntimeSseBlock
} from '@shared/public-runtime-sse'
import {
  DEFAULT_HUB_GATEWAY_BASE_URL,
  HUB_MODEL_PROVIDER_ID,
  type HubAccountSnapshot,
  type HubReferralResult,
  type HubUsageResult
} from '@shared/hub-account'
import {
  AttachmentContentResponse,
  AttachmentMetadataResponse,
  AttachmentUploadResponse
} from '../../../../packages/runtime/src/contracts/attachments.js'
import { RuntimeInfoResponse as RuntimeInfoResponseSchema } from '../../../../packages/runtime/src/contracts/runtime-info.js'
import {
  AttachmentDiagnosticsResponseV2 as AttachmentDiagnosticsResponseSchema,
  MemoryDiagnosticsResponseV2 as MemoryDiagnosticsResponseSchema,
  RuntimeToolsResponse as RuntimeToolsResponseSchema
} from '../../../../packages/runtime/src/contracts/runtime-tools.js'
import {
  ThreadSummaryResponse as ThreadSummaryResponseSchema,
  ThreadSummaryTaskMutationResponse as ThreadSummaryTaskMutationResponseSchema
} from '../../../../packages/runtime/src/contracts/threads.js'
import {
  CaseProjectDetailResponseV1Schema,
  CaseProjectListResponseV1Schema,
  CaseProjectThreadsResponseV1Schema
} from '../../../../packages/runtime/src/contracts/case-projects.js'
import {
  TaskJobKillResponseV1Schema,
  TaskJobListResponseV1Schema,
  TaskJobOutputResponseV1Schema,
  TaskJobWaitResponseV1Schema,
  threadSummaryTaskIdentityMatchesV1,
  ThreadSummaryTaskOutputResponseV1Schema
} from '../../../../packages/runtime/src/contracts/task-job-output.js'
import {
  PROVIDER_REGISTRY_FAILURE_MESSAGES_V1,
  PROVIDER_REGISTRY_SCHEMA_VERSION_V1
} from '../../../../packages/runtime/src/contracts/provider-registry.js'

const SETTINGS_STORAGE_KEY = 'analytix.browserPreview.settings'
const ACCOUNT_PREVIEW_STORAGE_KEY = 'analytix.browserPreview.account'
const RUNTIME_PROXY_PREFIX = '/__analytix-runtime'
const DESKTOP_SETTINGS_PATH = '/__analytix-desktop/settings'
const SSE_RECONNECT_BASE_MS = 750
const SSE_RECONNECT_MAX_MS = 5_000
const SSE_PENDING_EVENT_BATCH_MS = 16
const BROWSER_PREVIEW_RENDERER_DIAGNOSTIC = '[analytix] [renderer] event=renderer_diagnostic_failed'

function emitBrowserPreviewRendererDiagnostic(): void {
  try {
    console.error(BROWSER_PREVIEW_RENDERER_DIAGNOSTIC)
  } catch {
    // Browser-preview diagnostics must not affect the renderer.
  }
}

function browserProviderRegistryUnavailable(): Promise<ProviderRegistryResult> {
  return Promise.resolve({
    schemaVersion: PROVIDER_REGISTRY_SCHEMA_VERSION_V1,
    error: {
      code: 'runtime_unavailable',
      message: PROVIDER_REGISTRY_FAILURE_MESSAGES_V1.runtime_unavailable
    }
  })
}

function browserProtectedRecoveryUnavailable(): Promise<ProviderCredentialRecoveryResult> {
  return Promise.resolve({ ok: false, code: 'runtime_unavailable' })
}

function browserOAuthUnavailable() {
  return Promise.resolve({
    ok: false as const,
    code: 'unavailable' as const,
    message: browserPreviewUnavailable('OAuth account management')
  })
}

type BrowserAnalytixBridgeOptions = {
  force?: boolean
  runtimeProxyPrefix?: string
}

type SseControllerState = {
  controller: AbortController
  stopped: boolean
  threadId: string
  ackedSinceSeq: number
  deliveredSinceSeq: number
  ackWaiters: Set<() => void>
  publicEventFilter: PublicRuntimeEventFilter
}

const sseControllers = new Map<string, SseControllerState>()

function claimsAcceptedFinalAuthority(value: Record<string, unknown>): boolean {
  return containsPrivateAcceptedFinalAuthority(value) || [
    'acceptedFinal',
    'acceptedFinalView',
    'acceptedFinalDigest',
    'publicationCommitId',
    'publicationEventId',
    'publicationSlot',
    'publicationPayloadDigest'
  ].some((key) => Object.prototype.hasOwnProperty.call(value, key))
}

function withholdUnverifiedBrowserAcceptedFinal(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value
      .map((entry) => withholdUnverifiedBrowserAcceptedFinal(entry))
      .filter((entry) => entry !== undefined)
  }
  if (!value || typeof value !== 'object') return value
  const record = value as Record<string, unknown>
  if (claimsAcceptedFinalAuthority(record)) return undefined
  const projected: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(record)) {
    const child = withholdUnverifiedBrowserAcceptedFinal(entry)
    if (child !== undefined) projected[key] = child
  }
  return projected
}

function isLocalBrowserPreview(): boolean {
  if (typeof window === 'undefined') return false
  if (!import.meta.env.DEV) return false
  const protocol = window.location.protocol
  if (protocol !== 'http:' && protocol !== 'https:') return false
  return ['localhost', '127.0.0.1', '::1'].includes(window.location.hostname)
}

function readEnvString(name: 'VITE_ANALYTIX_BROWSER_WORKSPACE_ROOT' | 'VITE_ANALYTIX_RUNTIME_PORT'): string {
  const env = import.meta.env as Record<string, string | undefined>
  return env[name]?.trim() ?? ''
}

function browserPreviewWorkspaceRoot(): string {
  return readEnvString('VITE_ANALYTIX_BROWSER_WORKSPACE_ROOT') || DEFAULT_WRITE_WORKSPACE_ROOT
}

function browserPreviewRuntimePort(): number {
  const raw = Number(readEnvString('VITE_ANALYTIX_RUNTIME_PORT'))
  return Number.isFinite(raw) && raw > 0 ? Math.round(raw) : DEFAULT_ANALYTIX_PORT
}

function createBrowserPreviewDefaultSettings(): AppSettingsV1 {
  const workspaceRoot = browserPreviewWorkspaceRoot()
  const runtime = defaultAnalytixRuntimeSettings(browserPreviewRuntimePort())
  return normalizeAppSettings({
    version: 1,
    locale: 'zh',
    theme: 'system',
    uiFontScale: 'small',
    cursorSpotlight: true,
    provider: defaultModelProviderSettings(),
    runtime: {
      ...runtime,
      autoStart: false
    },
    workspaceRoot,
    log: {
      enabled: true,
      retentionDays: 2
    },
    notifications: {
      turnComplete: true
    },
    appBehavior: normalizeAppBehaviorSettings(),
    keyboardShortcuts: normalizeKeyboardShortcuts(),
    guiUpdate: {
      channel: DEFAULT_GUI_UPDATE_CHANNEL
    },
    codePromptPrefix: '',
    disabledSkillIds: [],
    write: {
      ...defaultWriteSettings(),
      defaultWorkspaceRoot: workspaceRoot,
      activeWorkspaceRoot: workspaceRoot,
      workspaces: [workspaceRoot]
    },
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings()
  })
}

function readBrowserPreviewAccountSeed(): Record<string, unknown> | null {
  try {
    const params = new URLSearchParams(window.location.search)
    if (params.get('accountPreview') === '1') {
      return {
        email: params.get('accountPreviewEmail') || undefined,
        fullName: params.get('accountPreviewName') || undefined,
        displayName: params.get('accountPreviewDisplayName') || undefined,
        username: params.get('accountPreviewUsername') || undefined,
        avatarColor: params.get('accountPreviewAvatarColor') || undefined,
        avatarStyle: params.get('accountPreviewAvatarStyle') || undefined,
        avatarUrl: params.get('accountPreviewAvatarUrl') || undefined,
        plan: params.get('accountPreviewPlan') || undefined
      }
    }
    const raw = window.localStorage?.getItem(ACCOUNT_PREVIEW_STORAGE_KEY)?.trim()
    if (!raw || raw === '0' || raw.toLowerCase() === 'false') return null
    if (raw === '1' || raw.toLowerCase() === 'true') return {}
    const parsed = JSON.parse(raw) as Record<string, unknown>
    return parsed?.authenticated === false ? null : parsed
  } catch {
    return {}
  }
}

function createBrowserPreviewAccountSnapshot(seed: Record<string, unknown> | null): HubAccountSnapshot {
  if (!seed) {
    return {
      authenticated: false,
      gatewayConfigured: false,
      accountReady: false,
      source: 'none',
      error: browserPreviewUnavailable('Hub account')
    }
  }
  const email = String(seed.email ?? 'preview@analytix.top').trim() || 'preview@analytix.top'
  const fullName = String(seed.fullName ?? 'eysn').trim() || email
  const displayName = String(seed.displayName ?? fullName).trim() || fullName
  const username = String(seed.username ?? email.split('@')[0] ?? '').trim().replace(/^@/u, '')
  const plan = seed.plan === 'normal' ? 'normal' : 'professional'
  const tokenLimit = Number(seed.tokenLimit ?? 2_000_000)
  const tokenBalance = Number(seed.tokenBalance ?? 1_286_400)
  const checkedAt = new Date().toISOString()
  return {
    authenticated: true,
    gatewayConfigured: true,
    accountReady: true,
    source: 'hub',
    checkedAt,
    user: {
      id: 'browser-preview-user',
      email,
      fullName,
      displayName,
      username,
      avatarColor: String(seed.avatarColor ?? '#22C55E').trim() || '#22C55E',
      avatarStyle: seed.avatarStyle === 'photo' ? 'photo' : seed.avatarStyle === 'xiezhi' ? 'xiezhi' : 'initials',
      avatarUrl: String(seed.avatarUrl ?? '').trim(),
      organization: String(seed.organization ?? 'Analytix Preview'),
      plan,
      storedPlan: plan,
      tokenLimit,
      tokenBalance,
      notificationEmail: email,
      emailVerified: true,
      phoneVerified: true,
      realNameVerified: true,
      realNameVerificationRequired: false,
      realNameVerificationAvailable: true,
      accountReady: true,
      createdAt: checkedAt
    },
    entitlement: {
      plan,
      edition: plan === 'normal' ? 'standard' : 'professional',
      canSwitch: true,
      checkedAt
    },
    gateway: {
      configured: true,
      baseUrl: DEFAULT_HUB_GATEWAY_BASE_URL
    },
    models: [
      { id: 'qwen-plus', ownedBy: 'analytix-hub' },
      { id: 'deepseek-chat', ownedBy: 'analytix-hub' },
      { id: 'mimo-latest', ownedBy: 'analytix-hub' }
    ]
  }
}

function browserPreviewHubSettingsPatch(): AppSettingsPatch {
  const hubModels = ['qwen-plus', 'deepseek-chat', 'mimo-latest']
  return {
    provider: {
      activeProviderId: HUB_MODEL_PROVIDER_ID,
      providers: [{
        id: HUB_MODEL_PROVIDER_ID,
        name: 'Analytix Hub',
        baseUrl: DEFAULT_HUB_GATEWAY_BASE_URL,
        endpointFormat: 'chat_completions',
        models: hubModels,
        modelProfiles: {}
      }]
    },
    runtime: {
      providerId: HUB_MODEL_PROVIDER_ID,
      model: hubModels[0]
    }
  }
}

function createBrowserPreviewUsage(snapshot: HubAccountSnapshot): HubUsageResult {
  const user = snapshot.user ?? createBrowserPreviewAccountSnapshot({}).user!
  return {
    user,
    gatewayUsageSummary: {
      requestCount30d: 128,
      completedCount30d: 121,
      issueCount30d: 2,
      rawTokensToday: 18_240,
      billableTokensToday: 12_960,
      rawTokens30d: 713_420,
      billableTokens30d: 512_800,
      cachedPromptTokens30d: 86_400,
      qwenBillableTokens30d: 268_000,
      deepseekBillableTokens30d: 171_600,
      mimoBillableTokens30d: 73_200
    },
    gatewayQuotaWindows: [
      {
        id: 'preview:normal:session_5h',
        scopeType: 'plan',
        scopeId: 'normal',
        windowKind: 'session_5h',
        windowStart: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
        windowEnd: new Date(Date.now() + 4 * 60 * 60 * 1000).toISOString(),
        usedBillableTokens: 54_200,
        reservedBillableTokens: 0,
        limit: 200_000,
        resetAt: new Date(Date.now() + 4 * 60 * 60 * 1000).toISOString(),
        requestCount: 8,
        label: '普通版 5 小时'
      },
      {
        id: 'preview:normal:day',
        scopeType: 'plan',
        scopeId: 'normal',
        windowKind: 'day',
        windowStart: new Date().toISOString(),
        windowEnd: new Date(Date.now() + 12 * 60 * 60 * 1000).toISOString(),
        usedBillableTokens: 129_600,
        reservedBillableTokens: 0,
        limit: 500_000,
        resetAt: new Date(Date.now() + 12 * 60 * 60 * 1000).toISOString(),
        requestCount: 18,
        label: '普通版 今日免费'
      },
      {
        id: 'preview:normal:week',
        scopeType: 'plan',
        scopeId: 'normal',
        windowKind: 'week',
        windowStart: new Date().toISOString(),
        windowEnd: new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString(),
        usedBillableTokens: 512_800,
        reservedBillableTokens: 0,
        limit: 3_500_000,
        resetAt: new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString(),
        requestCount: 121,
        label: '普通版 本周免费'
      }
    ]
  }
}

function createBrowserPreviewReferral(): HubReferralResult {
  return {
    referral: {
      code: 'PREVIEW88',
      shareUrl: 'https://analytix.top/invite/PREVIEW88',
      rewardTokenGrant: 100_000,
      pendingCount: 1,
      completedCount: 3,
      rewardedTokenTotal: 300_000,
      referrals: []
    }
  }
}

function readStoredSettings(): AppSettingsV1 {
  const defaults = createBrowserPreviewDefaultSettings()
  try {
    const raw = window.localStorage?.getItem(SETTINGS_STORAGE_KEY)
    if (!raw) return defaults
    const parsed = JSON.parse(raw) as Partial<AppSettingsV1>
    return mergeSettings(defaults, parsed)
  } catch {
    return defaults
  }
}

async function readDesktopPreviewSettings(): Promise<AppSettingsPatch | null> {
  try {
    const response = await fetch(DESKTOP_SETTINGS_PATH, {
      headers: {
        Accept: 'application/json'
      }
    })
    if (!response.ok) return null
    return JSON.parse(await response.text()) as AppSettingsPatch
  } catch {
    return null
  }
}

function writeStoredSettings(settings: AppSettingsV1): void {
  try {
    window.localStorage?.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(settings))
  } catch {
    /* Browser preview settings are best-effort only. */
  }
}

function mergeSettings(current: AppSettingsV1, patch: AppSettingsPatch): AppSettingsV1 {
  return normalizeAppSettings({
    ...current,
    ...patch,
    provider: mergeModelProviderSettings(current.provider, patch.provider),
    runtime: mergeAnalytixRuntimeSettings(current.runtime, patch.runtime),
    log: {
      ...current.log,
      ...(patch.log ?? {})
    },
    notifications: {
      ...current.notifications,
      ...(patch.notifications ?? {})
    },
    appBehavior: normalizeAppBehaviorSettings({
      ...current.appBehavior,
      ...(patch.appBehavior ?? {})
    }),
    keyboardShortcuts: normalizeKeyboardShortcuts({
      ...current.keyboardShortcuts,
      ...(patch.keyboardShortcuts ?? {})
    }),
    guiUpdate: {
      ...current.guiUpdate,
      ...(patch.guiUpdate ?? {})
    },
    write: mergeWriteSettings(current.write, patch.write),
    claw: mergeClawSettings(current.claw, patch.claw),
    schedule: mergeScheduleSettings(current.schedule, patch.schedule)
  })
}

function createUnsubscribe<T>(set: Set<(payload: T) => void>, handler: (payload: T) => void): () => void {
  set.add(handler)
  return () => set.delete(handler)
}

function unavailable(message: string): Promise<never> {
  return Promise.reject(new Error(message))
}

function unsupportedResult(message: string): Promise<{ ok: false; message: string }> {
  return Promise.resolve({ ok: false, message })
}

function browserPreviewUnavailable(feature: string): string {
  return `${feature} is only available in the Electron desktop app.`
}

function buildProxyPath(prefix: string, pathAndQuery: string): string {
  const path = pathAndQuery.startsWith('/') ? pathAndQuery : `/${pathAndQuery}`
  return `${prefix}${path}`
}

function decodeBrowserRuntimeRouteSegmentV1(value: string): string | null {
  try {
    const decoded = decodeURIComponent(value)
    return decoded && !decoded.includes('/') && !decoded.includes('\\') ? decoded : null
  } catch {
    return null
  }
}

function browserThreadSummaryResponseIdentityMatchesV1(path: string, value: unknown): boolean {
  const match = /^\/v1\/threads\/([^/]+)\/summary(?:\/tasks\/([^/]+)\/(output|kill|restart))?$/.exec(path)
  if (!match) return !/^\/v1\/threads\/[^/]+\/summary(?:\/|$)/.test(path)
  const threadId = decodeBrowserRuntimeRouteSegmentV1(match[1])
  if (!threadId || !value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  if (!match[2]) return record.threadId === threadId
  const taskId = decodeBrowserRuntimeRouteSegmentV1(match[2])
  if (!taskId) return false
  if (match[3] === 'output') return threadSummaryTaskIdentityMatchesV1(record.taskId, taskId)
  const task = record.task
  return Boolean(task && typeof task === 'object' && !Array.isArray(task) &&
    threadSummaryTaskIdentityMatchesV1((task as Record<string, unknown>).id, taskId))
}

async function runtimeRequestViaProxy(
  prefix: string,
  pathAndQuery: string,
  method = 'GET',
  body?: string
): Promise<{ ok: boolean; status: number; body: string }> {
  try {
    const headers = new Headers()
    headers.set('Accept', 'application/json')
    if (body !== undefined) headers.set('Content-Type', 'application/json')
    const response = await fetch(buildProxyPath(prefix, pathAndQuery), {
      method,
      headers,
      body
    })
    const rawBody = await response.text()
    if (!response.ok) {
      let parsed: unknown = null
      try {
        parsed = JSON.parse(rawBody)
      } catch {
        // Raw upstream text is private and intentionally discarded.
      }
      return {
        ok: false,
        status: response.status,
        body: JSON.stringify(projectPublicRuntimeHTTPError(response.status, parsed))
      }
    }

    const path = pathAndQuery.split('?', 1)[0]
    if (!rawBody.trim() && !browserRuntimeResponseRequiresBody(path)) {
      return { ok: true, status: response.status, body: '' }
    }
    if (!rawBody.trim()) {
      return browserRuntimeBoundaryFailure('runtime_response_schema_invalid')
    }

    let parsed: unknown
    try {
      parsed = JSON.parse(rawBody)
    } catch {
      return browserRuntimeBoundaryFailure('runtime_response_not_public')
    }
    if (containsPrivateAcceptedFinalAuthority(parsed)) {
      return browserRuntimeBoundaryFailure('runtime_response_not_public')
    }

    const outputSchema = path === '/v1/runtime/info'
      ? RuntimeInfoResponseSchema
      : path === '/v1/runtime/tools'
        ? RuntimeToolsResponseSchema
        : path === '/v1/attachments/diagnostics'
          ? AttachmentDiagnosticsResponseSchema
            : path === '/v1/memory/diagnostics'
              ? MemoryDiagnosticsResponseSchema
              : path === '/v1/case-projects'
                ? CaseProjectListResponseV1Schema
                : /^\/v1\/case-projects\/[^/]+\/threads$/.test(path)
                  ? CaseProjectThreadsResponseV1Schema
                  : /^\/v1\/case-projects\/[^/]+\/detail$/.test(path)
                    ? CaseProjectDetailResponseV1Schema
              : path === '/v1/runtime/task-jobs/output'
                ? TaskJobOutputResponseV1Schema
              : path === '/v1/runtime/task-jobs/list'
                ? TaskJobListResponseV1Schema
                : path === '/v1/runtime/task-jobs/wait'
                  ? TaskJobWaitResponseV1Schema
                  : path === '/v1/runtime/task-jobs/kill'
                    ? TaskJobKillResponseV1Schema
                    : /^\/v1\/threads\/[^/]+\/summary$/.test(path)
                      ? ThreadSummaryResponseSchema
                      : /^\/v1\/threads\/[^/]+\/summary\/tasks\/[^/]+\/(kill|restart)$/.test(path)
                        ? ThreadSummaryTaskMutationResponseSchema
                        : /^\/v1\/threads\/[^/]+\/summary\/tasks\/[^/]+\/output$/.test(path)
                          ? ThreadSummaryTaskOutputResponseV1Schema
                          : path === '/v1/attachments'
                            ? AttachmentUploadResponse
                            : /^\/v1\/attachments\/[^/]+\/content$/.test(path)
                              ? AttachmentContentResponse
                              : /^\/v1\/attachments\/[^/]+$/.test(path)
                                ? AttachmentMetadataResponse
                                : null
    const validated = outputSchema?.safeParse(parsed)
    if (validated && !validated.success) {
      return browserRuntimeBoundaryFailure('runtime_response_schema_invalid')
    }
    let publicValue: unknown
    if (outputSchema && validated?.success) {
      const requiresExactPublicProjection = path.startsWith('/v1/runtime/task-jobs/') ||
        path.startsWith('/v1/case-projects') ||
        /^\/v1\/threads\/[^/]+\/summary(?:\/|$)/.test(path)
      if (requiresExactPublicProjection) {
        const sanitized = sanitizePublicRuntimeValue(validated.data)
        if (sanitized === undefined || JSON.stringify(sanitized) !== JSON.stringify(validated.data)) {
          return browserRuntimeBoundaryFailure('runtime_response_not_public')
        }
        const publicValidated = outputSchema.safeParse(sanitized)
        if (!publicValidated.success || !browserThreadSummaryResponseIdentityMatchesV1(path, publicValidated.data)) {
          return browserRuntimeBoundaryFailure('runtime_response_not_public')
        }
        publicValue = publicValidated.data
      } else {
        publicValue = validated.data
      }
    } else {
      publicValue = withholdUnverifiedBrowserAcceptedFinal(sanitizePublicRuntimeValue(parsed))
    }
    const publicBody = JSON.stringify(publicValue)
    if (!publicBody) return browserRuntimeBoundaryFailure('runtime_response_not_public')
    return {
      ok: true,
      status: response.status,
      body: publicBody
    }
  } catch {
    return {
      ok: false,
      status: 0,
      body: JSON.stringify({
        code: 'fetch_failed',
        message: 'Runtime request failed (fetch_failed).'
      })
    }
  }
}

async function browserLocalDisplayUnavailable(): Promise<LocalDisplayResult> {
  return {
    ok: false,
    status: 403,
    code: 'forbidden',
    message: 'Typed local display is available only through the desktop host.'
  }
}

async function browserFundsCSVSnapshotStageUnavailable(): Promise<FundsCSVSnapshotStageResult> {
  return {
    ok: false,
    canceled: false,
    code: 'forbidden',
    message: 'Funds CSV snapshot staging is available only through the desktop host.'
  }
}

async function browserFundsCSVSnapshotConfirmUnavailable(): Promise<FundsCSVSnapshotConfirmResult> {
  return {
    ok: false,
    code: 'forbidden',
    message: 'Funds import confirmation is available only through the desktop host.'
  }
}

async function browserFundsCSVImportCancelUnavailable(): Promise<FundsImportCancelResult> {
  return {
    ok: false,
    code: 'forbidden',
    message: 'Funds import cancellation is available only through the desktop host.'
  }
}

async function browserFundsCSVImportStatusUnavailable(): Promise<FundsImportStatusResult> {
  return {
    ok: false,
    code: 'forbidden',
    message: 'Funds import status is available only through the desktop host.'
  }
}

function browserRuntimeResponseRequiresBody(path: string): boolean {
  return path === '/v1/runtime/info' ||
    path === '/v1/runtime/tools' ||
    path === '/v1/attachments/diagnostics' ||
    path === '/v1/memory/diagnostics'
}

function browserRuntimeBoundaryFailure(
  code: 'runtime_response_schema_invalid' | 'runtime_response_not_public'
): { ok: false; status: number; body: string } {
  return {
    ok: false,
    status: 502,
    body: JSON.stringify({
      code,
      message: code === 'runtime_response_schema_invalid'
        ? 'Runtime response failed schema validation.'
        : 'Runtime response was blocked at the public boundary.'
    })
  }
}

function sseEventsPath(threadId: string, sinceSeq: number): string {
	const path = `/v1/threads/${encodeURIComponent(threadId)}/events`
	const params = new URLSearchParams({ since_seq: String(sinceSeq), live: '1' })
	return `${path}?${params.toString()}`
}

async function waitForBrowserSseAck(
  state: SseControllerState,
  seq: number,
  timeoutMs = 500
): Promise<void> {
  if (seq <= 0 || state.ackedSinceSeq >= seq || state.stopped || state.controller.signal.aborted) return
  await new Promise<void>((resolve) => {
    const done = (): void => {
      window.clearTimeout(timer)
      state.ackWaiters.delete(onAck)
      state.controller.signal.removeEventListener('abort', done)
      resolve()
    }
    const onAck = (): void => {
      if (state.ackedSinceSeq >= seq) done()
    }
    const timer = window.setTimeout(done, timeoutMs)
    state.ackWaiters.add(onAck)
    state.controller.signal.addEventListener('abort', done, { once: true })
  })
}

function sleepWithAbort(ms: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted || ms <= 0) return Promise.resolve()
  return new Promise((resolve) => {
    const timer = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    const onAbort = (): void => {
      window.clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      resolve()
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

function startBrowserSseLoop(input: {
  prefix: string
  streamId: string
  threadId: string
  sinceSeq: number
  onEvents: Set<(payload: SseEventPayload) => void>
  onEnd: Set<(payload: SseEndPayload) => void>
  onError: Set<(payload: SseErrorPayload) => void>
}): void {
  const state = sseControllers.get(input.streamId)
  if (!state) return
  void (async () => {
    let reconnectDelayMs = SSE_RECONNECT_BASE_MS
    try {
      while (!state.stopped && !state.controller.signal.aborted) {
        try {
          const response = await fetch(buildProxyPath(input.prefix, sseEventsPath(input.threadId, state.ackedSinceSeq)), {
            headers: {
              Accept: 'text/event-stream',
              ...(state.ackedSinceSeq > 0 ? { 'Last-Event-ID': String(state.ackedSinceSeq) } : {})
            },
            signal: state.controller.signal
          })
          if (!response.ok || !response.body) {
            if (response.status >= 400 && response.status < 500 && response.status !== 408 && response.status !== 429) {
              for (const handler of input.onError) handler({ streamId: input.streamId, status: response.status })
              return
            }
            await sleepWithAbort(reconnectDelayMs, state.controller.signal)
            reconnectDelayMs = Math.min(reconnectDelayMs * 2, SSE_RECONNECT_MAX_MS)
            continue
          }
          reconnectDelayMs = SSE_RECONNECT_BASE_MS
          const reader = response.body.getReader()
          const decoder = new TextDecoder()
          let buffer = ''
          let pendingEvents: Record<string, unknown>[] = []
          let throttleTimer: ReturnType<typeof window.setTimeout> | null = null
          let firstEventFlushed = false
          let protocolViolationReason = ''
          let authorityRevoked = false

          const flushEvents = (): boolean => {
            if (throttleTimer !== null) {
              window.clearTimeout(throttleTimer)
              throttleTimer = null
            }
            if (state.stopped || state.controller.signal.aborted) {
              pendingEvents = []
              return false
            }
            if (pendingEvents.length === 0) return true
            let batchMaxSeq = state.deliveredSinceSeq
            for (const event of pendingEvents) {
              if (typeof event.seq === 'number') batchMaxSeq = Math.max(batchMaxSeq, event.seq)
            }
            const batch = pendingEvents
            pendingEvents = []
            const payload = { streamId: input.streamId, events: batch }
            if (!isStrictPublicRuntimeSseIpcPayload(payload)) return false
            state.deliveredSinceSeq = batchMaxSeq
            for (const handler of input.onEvents) handler(payload)
            return true
          }

          try {
            while (!state.stopped && !state.controller.signal.aborted) {
              const { done, value } = await reader.read()
              if (done) break
              buffer += decoder.decode(value, { stream: true })
              let hasNewEvents = false
              let next: { block: string; rest: string } | null
              while ((next = takePublicRuntimeSseBlock(buffer)) !== null) {
                buffer = next.rest
                const decision = projectPublicRuntimeSseBlock(next.block, state.threadId, state.publicEventFilter)
                if (decision === null) continue
                if (decision.status === 'revoke') {
                  // Keep revocation control-only so no queued event is
                  // rendered or cursor-advanced ahead of the purge barrier.
                  pendingEvents = [decision.event]
                  authorityRevoked = true
                  flushEvents()
                  break
                }
                if (decision.status !== 'emit') {
                  protocolViolationReason = decision.reason
                  break
                }
                if (claimsAcceptedFinalAuthority(decision.event)) {
                  protocolViolationReason = 'accepted_final_authority_unavailable'
                  break
                }
                pendingEvents.push(decision.event)
                if (!firstEventFlushed) {
                  firstEventFlushed = true
                  if (!flushEvents()) break
                } else {
                  hasNewEvents = true
                }
              }
              if (protocolViolationReason || authorityRevoked) break
              if (hasNewEvents && throttleTimer === null) {
                throttleTimer = window.setTimeout(() => {
                  throttleTimer = null
                  flushEvents()
                }, SSE_PENDING_EVENT_BATCH_MS)
              }
            }
          } finally {
            reader.releaseLock()
            flushEvents()
          }
          if (protocolViolationReason) {
            const reasonCode = isPublicRuntimeSseRejectionReasonCode(protocolViolationReason)
              ? protocolViolationReason
              : 'invalid_public_projection'
            for (const handler of input.onError) {
              handler({
                streamId: input.streamId,
                code: 'sse_event_rejected',
                message: 'Runtime request failed (sse_event_rejected).',
                reasonCode
              })
            }
            return
          }
          if (authorityRevoked) return
          await waitForBrowserSseAck(state, state.deliveredSinceSeq)
        } catch (error) {
          if (state.stopped || state.controller.signal.aborted) return
          for (const handler of input.onError) {
            handler({
              streamId: input.streamId,
              code: 'sse_stream_error',
              message: 'Runtime request failed (sse_stream_error).'
            })
          }
          await sleepWithAbort(reconnectDelayMs, state.controller.signal)
          reconnectDelayMs = Math.min(reconnectDelayMs * 2, SSE_RECONNECT_MAX_MS)
        }
      }
    } finally {
      if (sseControllers.get(input.streamId) === state) sseControllers.delete(input.streamId)
      if (!state.stopped && sseControllers.get(input.streamId) === undefined) {
        for (const handler of input.onEnd) handler({ streamId: input.streamId })
      }
    }
  })()
}

function createBrowserAnalytixApi(prefix: string): AnalytixApi {
  let settings = readStoredSettings()
  let desktopSettingsPromise: Promise<void> | null = null
  const sseEventHandlers = new Set<(payload: SseEventPayload) => void>()
  const sseEndHandlers = new Set<(payload: SseEndPayload) => void>()
  const sseErrorHandlers = new Set<(payload: SseErrorPayload) => void>()
  const runtimeStatusHandlers = new Set<(payload: AnalytixRuntimeStatusPayload) => void>()
  const channelActivityHandlers = new Set<(payload: ClawChannelActivityPayload) => void>()
  const fileChangeHandlers = new Set<(payload: WorkspaceFileChangePayload) => void>()
  const unsupported = browserPreviewUnavailable('This action')
  let accountSnapshot = createBrowserPreviewAccountSnapshot(readBrowserPreviewAccountSeed())
  if (accountSnapshot.authenticated) {
    settings = mergeSettings(settings, browserPreviewHubSettingsPatch())
  }

  const updateSettings = (patch: AppSettingsPatch): AppSettingsV1 => {
    settings = mergeSettings(settings, patch)
    writeStoredSettings(settings)
    return settings
  }

  const ensureDesktopSettingsLoaded = async (): Promise<void> => {
    if (desktopSettingsPromise) return desktopSettingsPromise
    desktopSettingsPromise = readDesktopPreviewSettings()
      .then((desktopSettings) => {
        if (!desktopSettings) return
        settings = mergeSettings(settings, desktopSettings)
      })
    return desktopSettingsPromise
  }

  const flatApi = {
    importMappingPreview: browserLocalDisplayUnavailable,
    cleaningDiffPreview: browserLocalDisplayUnavailable,
    directSourcePreview: browserLocalDisplayUnavailable,
    acceptedSlotDisplay: browserLocalDisplayUnavailable,
    stageFundsCSVSnapshot: browserFundsCSVSnapshotStageUnavailable,
    confirmFundsCSVSnapshot: browserFundsCSVSnapshotConfirmUnavailable,
    cancelFundsCSVImport: browserFundsCSVImportCancelUnavailable,
    statusFundsCSVImport: browserFundsCSVImportStatusUnavailable,
    runDeterministicFundsCleaning: async () => ({
      ok: false,
      status: 'unavailable',
      code: 'runtime_unavailable',
      message: browserPreviewUnavailable('Deterministic funds cleaning')
    }),
    revokeCleaningDiffPreview: async () => ({
      ok: false,
      code: 'runtime_unavailable',
      message: browserPreviewUnavailable('Cleaning diff preview')
    })
  } satisfies Pick<
    AnalytixFlatApi,
    | 'importMappingPreview'
    | 'cleaningDiffPreview'
    | 'directSourcePreview'
    | 'acceptedSlotDisplay'
    | 'stageFundsCSVSnapshot'
    | 'confirmFundsCSVSnapshot'
    | 'cancelFundsCSVImport'
    | 'statusFundsCSVImport'
    | 'runDeterministicFundsCleaning'
    | 'revokeCleaningDiffPreview'
  >

  return {
    settings: {
      getSettings: async () => {
        await ensureDesktopSettingsLoaded()
        return settings
      },
      setSettings: async (partial) => {
        await ensureDesktopSettingsLoaded()
        return updateSettings(partial)
      },
      saveSettingsSilent: async (partial) => {
        await ensureDesktopSettingsLoaded()
        return updateSettings(partial)
      }
    },
    account: {
      getSnapshot: async () => accountSnapshot,
      refresh: async () => accountSnapshot,
      login: () => unavailable(browserPreviewUnavailable('Hub login')),
      register: () => unavailable(browserPreviewUnavailable('Hub registration')),
      logout: async () => {
        window.localStorage?.removeItem(ACCOUNT_PREVIEW_STORAGE_KEY)
        accountSnapshot = createBrowserPreviewAccountSnapshot(null)
        return accountSnapshot
      },
      sendRegisterVerificationCode: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Hub verification code')
      }),
      sendPasswordResetVerificationCode: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Hub password reset')
      }),
      confirmPasswordReset: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Hub password reset')
      }),
      fetchChallenge: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Hub auth challenge')
      }),
      fetchChallengeState: async () => ({
        ok: true,
        state: {
          challengeRequired: false,
          failedAttempts: 0,
          threshold: 3
        }
      }),
      verifyChallenge: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Hub auth challenge')
      }),
      updateProfile: async (request) => {
        if (!accountSnapshot.authenticated || !accountSnapshot.user) {
          return {
            ok: false,
            message: browserPreviewUnavailable('Hub profile')
          }
        }
        const updatedUser = {
          ...accountSnapshot.user,
          displayName: request.displayName ?? accountSnapshot.user.displayName,
          username: request.username ?? accountSnapshot.user.username,
          avatarColor: request.avatarColor ?? accountSnapshot.user.avatarColor,
          avatarStyle: request.avatarStyle ?? accountSnapshot.user.avatarStyle,
          avatarUrl: request.avatarImageDataUrl || accountSnapshot.user.avatarUrl
        }
        accountSnapshot = {
          ...accountSnapshot,
          checkedAt: new Date().toISOString(),
          user: updatedUser
        }
        return {
          ok: true,
          user: updatedUser
        }
      },
      syncProfileEvents: async () => ({
        ok: true,
        acceptedCount: 0,
        syncedCount: 0
      }),
      syncLocalProfileEvents: async () => ({
        ok: true,
        acceptedCount: 0,
        syncedCount: 0,
        scannedThreadCount: 0,
        generatedEventCount: 0
      }),
      getUsage: async () => accountSnapshot.authenticated
        ? ({ ok: true, ...createBrowserPreviewUsage(accountSnapshot) })
        : ({
            ok: false,
            message: browserPreviewUnavailable('Hub usage')
          }),
      getReferral: async () => accountSnapshot.authenticated
        ? ({ ok: true, ...createBrowserPreviewReferral() })
        : ({
            ok: false,
            message: browserPreviewUnavailable('Hub referral')
          }),
      getModels: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Hub models')
      })
    },
    providerRegistry: {
      request: browserProviderRegistryUnavailable
    },
    providerCredentialRecovery: {
      createDestinationRequest: browserProtectedRecoveryUnavailable,
      createSourceBundle: browserProtectedRecoveryUnavailable,
      applyDestinationBundle: browserProtectedRecoveryUnavailable,
      finalizeSourceReceipt: browserProtectedRecoveryUnavailable
    },
    providerOAuth: {
      configure: browserOAuthUnavailable,
      begin: browserOAuthUnavailable,
      status: browserOAuthUnavailable,
      cancel: browserOAuthUnavailable,
      revoke: browserOAuthUnavailable,
      delete: browserOAuthUnavailable,
      replaceSubscription: browserOAuthUnavailable
    },
    mcpOAuth: {
      begin: browserOAuthUnavailable,
      status: browserOAuthUnavailable,
      cancel: browserOAuthUnavailable,
      revoke: browserOAuthUnavailable,
      delete: browserOAuthUnavailable
    },
    extensionOAuth: {
      begin: browserOAuthUnavailable,
      status: browserOAuthUnavailable,
      cancel: browserOAuthUnavailable,
      revoke: browserOAuthUnavailable,
      delete: browserOAuthUnavailable
    },
    runtime: {
      runtimeRequest: (path, method, body) => runtimeRequestViaProxy(prefix, path, method, body),
      importMappingPreview: flatApi.importMappingPreview,
      cleaningDiffPreview: flatApi.cleaningDiffPreview,
      directSourcePreview: flatApi.directSourcePreview,
      acceptedSlotDisplay: flatApi.acceptedSlotDisplay,
      stageFundsCSVSnapshot: flatApi.stageFundsCSVSnapshot,
      confirmFundsCSVSnapshot: flatApi.confirmFundsCSVSnapshot,
      cancelFundsCSVImport: flatApi.cancelFundsCSVImport,
      statusFundsCSVImport: flatApi.statusFundsCSVImport,
      runDeterministicFundsCleaning: flatApi.runDeterministicFundsCleaning,
      revokeCleaningDiffPreview: flatApi.revokeCleaningDiffPreview,
      restartRuntime: () => unavailable(browserPreviewUnavailable('Restart runtime')),
      fetchUpstreamModels: async () => ({
        ok: true,
        modelIds: [settings.runtime.model].filter(Boolean),
        defaultModelId: settings.runtime.model
      }),
      probeModelCapabilities: async (payload) => {
        const probedAt = new Date()
        const staleAfter = new Date(probedAt.getTime() + 60_000)
        return {
          ok: false,
          key: modelCapabilityProbeKey(payload),
          providerId: payload.providerId,
          model: payload.model,
          endpointFormat: payload.endpointFormat,
          sanitizedBaseUrl: payload.baseUrl,
          probedAt: probedAt.toISOString(),
          staleAfter: staleAfter.toISOString(),
          imageInput: 'failed',
          toolCalling: 'failed',
          toolResultImage: 'failed',
          status: 'failed',
          errorSummary: browserPreviewUnavailable('Model capability probe'),
          message: browserPreviewUnavailable('Model capability probe')
        }
      },
      startSse: async (threadId, sinceSeq, streamId) => {
        if (!Number.isSafeInteger(sinceSeq) || sinceSeq < 0) throw new Error('invalid_sse_cursor')
        const id = streamId?.trim() || crypto.randomUUID()
        const existing = sseControllers.get(id)
        if (existing) {
          existing.stopped = true
          existing.controller.abort()
        }
        sseControllers.set(id, {
          controller: new AbortController(),
          stopped: false,
          threadId,
          ackedSinceSeq: sinceSeq,
          deliveredSinceSeq: sinceSeq,
          ackWaiters: new Set(),
          publicEventFilter: new PublicRuntimeEventFilter()
        })
        startBrowserSseLoop({
          prefix,
          streamId: id,
          threadId,
          sinceSeq,
          onEvents: sseEventHandlers,
          onEnd: sseEndHandlers,
          onError: sseErrorHandlers
        })
        return { streamId: id }
      },
      ackSseEvent: async (streamId, seq, acceptedFinal) => {
        if (acceptedFinal) return false
        const state = sseControllers.get(streamId)
        if (!state || !Number.isSafeInteger(seq) || seq < 0 || seq > state.deliveredSinceSeq) return false
        state.ackedSinceSeq = Math.max(state.ackedSinceSeq, seq)
        for (const notify of state.ackWaiters) notify()
        return true
      },
      stopSse: async (streamId) => {
        const state = sseControllers.get(streamId)
        if (!state) return false
        state.stopped = true
        state.controller.abort()
        sseControllers.delete(streamId)
        return true
      },
      onSseEvent: (handler) => createUnsubscribe(sseEventHandlers, handler),
      onSseEnd: (handler) => createUnsubscribe(sseEndHandlers, handler),
      onSseError: (handler) => createUnsubscribe(sseErrorHandlers, handler),
      onRuntimeStatus: (handler) => {
        const unsubscribe = createUnsubscribe(runtimeStatusHandlers, handler)
        handler(createRuntimeStatusPublicV1({
          code: 'browser_preview_ready',
          source: 'browser-preview',
          at: new Date().toISOString()
        }))
        return unsubscribe
      },
      getAnalytixConfigFile: () => Promise.resolve({ path: '', content: '', exists: false }),
      setAnalytixConfigFile: () => unavailable(browserPreviewUnavailable('Runtime config editing')),
      openAnalytixConfigDir: () => Promise.resolve({ ok: false, message: browserPreviewUnavailable('Open config directory') })
    },
    connectPhone: {
      getStatus: async () => ({ imServerRunning: false, imUrl: '', runningTaskIds: [] }),
      runTask: () => unsupportedResult(browserPreviewUnavailable('Connect Phone tasks')),
      startImInstallQr: () => unsupportedResult(browserPreviewUnavailable('Connect Phone QR install')),
      pollImInstall: async () => ({ done: false, error: browserPreviewUnavailable('Connect Phone install polling') }),
      connectTelegramBot: async () => ({
        ok: false,
        code: 'unknown',
        message: browserPreviewUnavailable('Telegram bot connection')
      }),
      disconnectImChannel: () => unavailable(browserPreviewUnavailable('Phone connection setup')),
      onChannelActivity: (handler) => createUnsubscribe(channelActivityHandlers, handler),
      mirrorChannelMessage: () => unsupportedResult(browserPreviewUnavailable('Message mirroring')),
      mirrorChannelMessageToFeishu: () => unsupportedResult(browserPreviewUnavailable('Feishu message mirroring')),
      createTaskFromText: async () => ({ kind: 'noop' })
    },
    schedule: {
      getStatus: async () => ({
        internalServerRunning: false,
        internalUrl: '',
        runningTaskIds: [],
        powerSaveBlockerActive: false
      }),
      runTask: () => unsupportedResult(browserPreviewUnavailable('Scheduled tasks')),
      createTaskFromText: async () => ({ kind: 'noop' })
    },
    workspace: {
      pickDirectory: async () => ({ canceled: true, path: null }),
      getGitBranches: async () => ({ ok: false, reason: 'git_unavailable', message: browserPreviewUnavailable('Git branches') }),
      switchGitBranch: async () => ({ ok: false, reason: 'git_unavailable', message: browserPreviewUnavailable('Git branch switching') }),
      createAndSwitchGitBranch: async () => ({ ok: false, reason: 'git_unavailable', message: browserPreviewUnavailable('Git branch creation') }),
      createGitCheckpoint: async () => ({ ok: false, reason: 'git_unavailable', message: browserPreviewUnavailable('Git checkpoint creation') }),
      restoreGitCheckpoint: async () => ({ ok: false, reason: 'git_unavailable', message: browserPreviewUnavailable('Git checkpoint restore') }),
      acquireWorktree: () => unavailable(browserPreviewUnavailable('Worktrees')),
      releaseWorktree: async () => undefined,
      listWorktrees: async (params) => ({
        projectPath: params.projectPath,
        poolDir: '',
        mainBranch: '',
        headCommit: '',
        worktrees: [],
        inUseCount: 0,
        isGitRepo: false
      }),
      removeWorktree: async () => undefined,
      getWorktreeChanges: async (params) => ({
        worktreePath: params.worktreePath,
        baseCommit: '',
        currentCommit: '',
        modifiedFiles: [],
        addedFiles: [],
        deletedFiles: [],
        hasUncommittedChanges: false
      }),
      commitWorktree: () => unavailable(browserPreviewUnavailable('Worktree commit')),
      mergeWorktree: () => unavailable(browserPreviewUnavailable('Worktree merge')),
      abortWorktreeMerge: async () => undefined,
      continueWorktreeMerge: () => unavailable(browserPreviewUnavailable('Worktree merge')),
      syncWorktreeFromMain: () => unavailable(browserPreviewUnavailable('Worktree sync')),
      abortWorktreeRebase: async () => undefined,
      cleanupWorktrees: async () => undefined,
      findAvailableWorktreePoolIndex: async () => null,
      startThreadHandoff: () => unavailable(browserPreviewUnavailable('Thread handoff')),
      retryThreadHandoff: () => unavailable(browserPreviewUnavailable('Thread handoff retry')),
      getThreadHandoffOperations: async () => ({ operations: [] }),
      cancelThreadHandoff: () => unavailable(browserPreviewUnavailable('Thread handoff cancel')),
      removeThreadHandoff: async () => ({ removed: false }),
      completeThreadHandoffSwitch: () => unavailable(browserPreviewUnavailable('Thread handoff completion')),
      failThreadHandoffSwitch: () => unavailable(browserPreviewUnavailable('Thread handoff failure handling')),
      onThreadHandoffEvent: () => () => undefined,
      listEditors: async () => ({ editors: [], defaultEditorId: '' }),
      openEditorPath: async () => ({ ok: false, message: browserPreviewUnavailable('Open editor') })
    },
    office: {
      onAnnotationInputFreeze: () => () => undefined,
      onMenuRequested: () => () => undefined,
      showActionMenu: async () => ({actionId:null}),
      onWorkspaceCommand: () => () => undefined,
      pickFile: async () => ({ ok: false, error: 'unavailable' }),
      request: async () => ({ ok: false, view: null, error: 'unavailable' }),
      onChange: () => () => undefined
    },
    packageHost: {
      request: async () => ({ ok: false, code: 'identity_invalid', message: 'Plugin control requires the desktop workspace.' })
    },
    objects: {
      resolveArtifact: async () => ({ok:false,code:'unavailable'}),
      request: async () => ({ ok: false, code: 'forbidden', message: 'Protected object editing requires the desktop workspace.' })
    },
    files: {
      listDirectory: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace file listing') }),
      resolve: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace file resolving') }),
      read: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace file reading') }),
      readImage: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace image reading') }),
      readPdf: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace PDF reading') }),
      readLocalPdfText: async () => ({ ok: false, message: browserPreviewUnavailable('Local PDF text reading') }),
      saveAs: async () => ({ ok: false, canceled: true, message: browserPreviewUnavailable('Save file') }),
      write: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace file writing') }),
      createFile: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace file creation') }),
      createDirectory: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace directory creation') }),
      saveClipboardImage: async () => ({ ok: false, message: browserPreviewUnavailable('Clipboard image saving') }),
      readClipboardImage: async () => ({ ok: false, message: browserPreviewUnavailable('Clipboard image reading') }),
      pickLocalFiles: async () => ({ canceled: true, paths: [] }),
      renameEntry: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace rename') }),
      deleteEntry: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace delete') }),
      watch: async () => ({ ok: false, message: browserPreviewUnavailable('Workspace file watching') }),
      unwatch: async () => false,
      onChanged: (handler) => createUnsubscribe(fileChangeHandlers, handler),
      getPathForFile: () => ''
    },
    write: {
      onShutdown: () => () => undefined,
      requestWriteInlineCompletion: async () => ({ ok: false, message: browserPreviewUnavailable('Write inline completion') }),
      retrieveWriteContext: async () => ({ ok: false, message: browserPreviewUnavailable('Write retrieval') }),
      generateWriteInfographic: async () => ({ ok: false, message: browserPreviewUnavailable('Write infographic generation') }),
      authorizeWritePrototype: async () => ({ ok: false, message: browserPreviewUnavailable('Write prototype authorization') }),
      openWritePrototype: async () => ({ ok: false, message: browserPreviewUnavailable('Write prototype opening') }),
      listWriteInlineCompletionDebugEntries: async () => [],
      clearWriteInlineCompletionDebugEntries: async () => true,
      exportWriteDocument: async () => ({ ok: false, canceled: false, message: browserPreviewUnavailable('Write export') }),
      copyWriteDocumentAsRichText: async () => ({ ok: false, message: browserPreviewUnavailable('Rich text copy') })
    },
    speech: {
      transcribe: async () => ({ ok: false, text: '', message: browserPreviewUnavailable('Speech transcription') })
    },
    terminal: {
      create: async () => ({ ok: false, sessionId: '', message: browserPreviewUnavailable('Terminal') }),
      write: async () => false,
      resize: async () => false,
      dispose: async () => false,
      onData: () => () => undefined,
      onExit: () => () => undefined
    },
    backgroundTasks: {
      register: async () => ({ ok: false, message: browserPreviewUnavailable('Background tasks') }),
      list: async () => ({ tasks: [] }),
      snapshot: async () => ({ tasks: [] }),
      kill: async () => ({ ok: false, message: browserPreviewUnavailable('Background task kill') }),
      restart: async () => ({ ok: false, message: browserPreviewUnavailable('Background task restart') }),
      output: async () => ({ ok: false, message: browserPreviewUnavailable('Background task output') })
    },
    updates: {
      getState: async () => ({ status: 'idle' }),
      check: async (channel?: GuiUpdateChannel) => ({
        ok: false,
        currentVersion: 'browser-preview',
        channel: channel ?? settings.guiUpdate.channel,
        message: browserPreviewUnavailable('GUI update check'),
        code: 'unsupported'
      }),
      download: async () => ({
        ok: false,
        currentVersion: 'browser-preview',
        message: browserPreviewUnavailable('GUI update download'),
        code: 'unsupported'
      }),
      install: async () => ({
        ok: false,
        currentVersion: 'browser-preview',
        message: browserPreviewUnavailable('GUI update install'),
        code: 'unsupported'
      }),
      onState: () => () => undefined
    },
    logs: {
      error: async (_category, _message, _detail) => {
        emitBrowserPreviewRendererDiagnostic()
      },
      getPath: async () => '',
      openDir: async () => ({ ok: false, message: browserPreviewUnavailable('Open log directory') })
    },
    app: {
      platform: navigator.platform.toLowerCase().includes('mac') ? 'darwin' : 'browser',
      startupSurfaceReady: () => undefined,
      confirmDialog: async (options) => window.confirm([options.message, options.detail].filter(Boolean).join('\n\n')),
      runDesktopCommand: async () => undefined,
      openThreadInNewWindow: async () => undefined,
      openExternal: async (url) => {
        window.open(url, '_blank', 'noopener,noreferrer')
      },
      getComputerUsePermissions: async () => ({
        platform: 'browser',
        supported: false,
        needsPermission: false,
        accessibility: 'unknown',
        screenRecording: 'unknown',
        accessibilityNeedsRestart: false
      }),
      getComputerUseDoctor: async () => ({
        platform: 'browser',
        backend: {
          preferred: 'analytix-computer-use',
          selected: 'analytix-computer-use',
          available: false,
          reason: browserPreviewUnavailable('Computer Use doctor')
        },
        checks: [{
          id: 'browser-preview',
          label: 'Browser preview',
          status: 'unknown',
          message: browserPreviewUnavailable('Computer Use doctor')
        }]
      }),
      getChromeBrowserUseStatus: async () => ({
        platform: 'browser',
        checkedAt: new Date().toISOString(),
        extensionId: 'bccibejdpcjcdlcnbpempcpjgladapgk',
        nativeHostName: 'top.analytix.codexextension',
        connected: false,
        state: 'diagnosticsUnavailable',
        reason: browserPreviewUnavailable('Chrome Browser Use diagnostics'),
        extension: {
          status: 'unknown',
          extensionId: 'bccibejdpcjcdlcnbpempcpjgladapgk',
          profiles: [],
          problem: browserPreviewUnavailable('Chrome extension diagnostics')
        },
        nativeHost: {
          status: 'unsupported',
          expectedHostName: 'top.analytix.codexextension',
          expectedExtensionId: 'bccibejdpcjcdlcnbpempcpjgladapgk',
          expectedOrigin: 'chrome-extension://bccibejdpcjcdlcnbpempcpjgladapgk/',
          problem: browserPreviewUnavailable('Chrome native host diagnostics')
        },
        connectionProbe: {
          status: 'unavailable',
          problem: browserPreviewUnavailable('Chrome Browser Use connection probe')
        }
      }),
      openChromeBrowserUseExtensionPage: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Chrome extension page')
      }),
      requestComputerUsePermission: async (_kind: ComputerUsePermissionKind) => ({
        platform: 'browser',
        supported: false,
        needsPermission: false,
        accessibility: 'unknown',
        screenRecording: 'unknown',
        accessibilityNeedsRestart: false
      }),
      invalidateQueryCache: async () => true,
      onQueryCacheInvalidated: () => () => undefined,
      showTurnCompleteNotification: async () => ({ ok: true, shown: false, reason: browserPreviewUnavailable('System notification') }),
      getVersion: async () => 'browser-preview',
      listSkills: async () => ({ ok: true, skills: [], validationErrors: [] }),
      listSkillRoots: async () => ({ ok: true, roots: [] }),
      saveSkillFile: async () => ({ ok: false, message: browserPreviewUnavailable('Skill editing') }),
      deleteSkill: async () => ({ ok: false, message: browserPreviewUnavailable('Skill delete') }),
      openSkillRoot: async () => ({ ok: false, message: browserPreviewUnavailable('Open skill root') }),
      listUiPlugins: async () => ({ plugins: [] }),
      installUiPlugin: async () => ({ canceled: true }),
      removeUiPlugin: async () => ({ ok: false }),
      loadUiPlugin: async () => ({ ok: false, error: browserPreviewUnavailable('UI plugins') }),
      syncHubAgentMarketplace: async () => ({ ok: false, message: browserPreviewUnavailable('Marketplace sync') }),
      installHubAgentPlugin: async () => ({ ok: false, message: browserPreviewUnavailable('Plugin install') }),
      uninstallHubAgentPlugin: async () => ({ ok: false, message: browserPreviewUnavailable('Plugin uninstall') }),
      readHubAgentSkillMarkdown: async () => ({ ok: false, message: browserPreviewUnavailable('Skill markdown reading') })
    },
    dataAnalysis: {
      ensureBackend: async () => ({
        phase: 'failed',
        detail: browserPreviewUnavailable('Data analysis backend'),
        updatedAt: new Date().toISOString()
      }),
      ensureWorkspaceCase: async () => ({
        ok: false,
        message: browserPreviewUnavailable('Data analysis workspace case')
      }),
      getRuntimeInfo: async () => ({
        isPackaged: false,
        backend: {
          managed: false,
          phase: 'failed',
          detail: browserPreviewUnavailable('Data analysis backend'),
          updatedAt: new Date().toISOString()
        }
      }),
      onBackendRuntimeState: () => () => undefined,
      pickFiles: async () => ({ canceled: true, paths: [], filePaths: [] }),
      pickDirectory: async () => ({ canceled: true, path: '', filePath: '' }),
      openExternal: async (url) => {
        window.open(url, '_blank', 'noopener,noreferrer')
        return true
      },
      setWindowChrome: async () => false
    },
    diagnostics: {
      detectLegacySessions: async () => ({ destDir: '', sources: [] }),
      importLegacySessions: async () => ({ ok: false, message: browserPreviewUnavailable('Legacy import') }),
      pickLegacySessionDir: async () => ({ canceled: true, path: null }),
      listWriteInlineCompletionDebugEntries: async () => [],
      clearWriteInlineCompletionDebugEntries: async () => true,
      logError: async (_category, _message, _detail) => {
        emitBrowserPreviewRendererDiagnostic()
      },
      getLogPath: async () => '',
      runtimeRequest: (path, method, body) => runtimeRequestViaProxy(prefix, path, method, body),
      recordThreadTrace: async () => ({ ok: false, message: browserPreviewUnavailable('Thread tracing') })
    }
  }
}

export function installBrowserAnalytixBridge(options: BrowserAnalytixBridgeOptions = {}): boolean {
  if (typeof window === 'undefined') return false
  if (window.analytix && !options.force) return false
  if (!options.force && !isLocalBrowserPreview()) return false
  window.analytix = createBrowserAnalytixApi(options.runtimeProxyPrefix ?? RUNTIME_PROXY_PREFIX)
  document.documentElement.dataset.bridge = 'browser-preview'
  return true
}
