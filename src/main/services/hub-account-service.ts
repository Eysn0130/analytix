import { app } from 'electron'
import { chmod, mkdir, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { existsSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import type { AppSettingsPatch, AppSettingsV1, ModelProviderModelProfileV1 } from '../../shared/app-settings'
import {
  DEFAULT_HUB_BASE_URL,
  DEFAULT_HUB_GATEWAY_BASE_URL,
  HUB_MODEL_PROVIDER_ID,
  type HubAccountApiResult,
  type HubAccountPlan,
  type HubAccountSnapshot,
  type HubAuthChallenge,
  type HubAuthChallengeMode,
  type HubAuthChallengeState,
  type HubAuthChallengeVerifyRequest,
  type HubAuthChallengeVerifyResult,
  type HubDesktopAuthCredential,
  type HubEntitlement,
  type HubGatewayCredential,
  type HubGatewayModel,
  type HubGatewayModelsResult,
  type HubGatewayProfile,
  type HubGatewayQuotaWindow,
  type HubGatewayUsageSummary,
  type HubLoginRequest,
  type HubPasswordResetConfirmRequest,
  type HubProfileEventSyncItem,
  type HubProfileEventSyncRequest,
  type HubProfileEventSyncResult,
  type HubProfileLocalSyncResult,
  type HubProfileUpdateRequest,
  type HubProfileUpdateResult,
  type HubProductEdition,
  type HubReferralResult,
  type HubRegisterRequest,
  type HubUsageResult,
  type HubUserSnapshot,
  type HubVerificationCodeRequest
} from '../../shared/hub-account'
import type { JsonSettingsStore } from '../settings-store'
import { recordHubActivity } from '../hub-activity-observation'
import {
  clearHubGatewayRuntimeToken,
  setHubGatewayRuntimeToken
} from './hub-gateway-runtime-secret'

type StoredHubAuth = {
  version: 1
  source: 'hub' | 'test-bootstrap'
  desktopAuth: HubDesktopAuthCredential
  user: HubUserSnapshot
  entitlement: HubEntitlement
  gateway?: Omit<HubGatewayCredential, 'token'> & { configured: boolean }
  models?: HubGatewayModel[]
  checkedAt: string
}

type HubLoginResponse = {
  user?: unknown
  desktopAuth?: unknown
  gateway?: unknown
}

type HubEntitlementResponse = {
  user?: unknown
  entitlement?: unknown
  gateway?: unknown
  gatewayQuotaWindows?: unknown
}

export type HubAccountServiceOptions = {
  store: JsonSettingsStore
  restartRuntime?: () => Promise<void>
  logError?: (category: string, message: string, detail?: unknown) => void
  isPackaged?: () => boolean
  nodeEnv?: () => string
}

const HUB_AUTH_FILE_NAME = 'analytix-hub-auth.json'
const HUB_GATEWAY_TOKEN_FILE_NAME = 'analytix-hub-gateway.token'
const PROFILE_LOCAL_SYNC_MAX_THREADS = 500
const PROFILE_LOCAL_SYNC_BATCH_SIZE = 200
const HUB_GATEWAY_MODELS_TIMEOUT_MS = 15_000
const HUB_GATEWAY_INITIALIZATION_ERROR = '登录成功，但模型网关初始化失败，请刷新重试。'

recordHubActivity('moduleLoad')

function text(value: unknown): string {
  return String(value ?? '').trim()
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/u, '')
}

function hubBaseUrl(): string {
  const value = trimTrailingSlash(text(process.env.ANALYTIX_HUB_BASE_URL))
  return value || DEFAULT_HUB_BASE_URL
}

function absoluteHubAssetUrl(value: unknown): string {
  const url = text(value)
  if (!url) return ''
  if (/^https?:\/\//iu.test(url) || url.startsWith('data:')) return url
  if (url.startsWith('/')) return `${hubBaseUrl()}${url}`
  return url
}

function normalizePlan(value: unknown): HubAccountPlan {
  const normalized = text(value).toLowerCase()
  return normalized === 'admin' || normalized === 'professional' || normalized === 'normal'
    ? normalized
    : 'normal'
}

function editionFromPlan(plan: HubAccountPlan): HubProductEdition {
  return plan === 'normal' ? 'standard' : 'professional'
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function normalizeUser(value: unknown): HubUserSnapshot | null {
  const record = objectValue(value)
  const id = text(record.id)
  const email = text(record.email).toLowerCase()
  if (!id || !email) return null
  const plan = normalizePlan(record.plan)
  return {
    id,
    email,
    fullName: text(record.fullName ?? record.full_name) || email,
    displayName: text(record.displayName ?? record.display_name),
    username: text(record.username),
    avatarColor: text(record.avatarColor ?? record.avatar_color),
    avatarStyle: text(record.avatarStyle ?? record.avatar_style) === 'photo'
      ? 'photo'
      : text(record.avatarStyle ?? record.avatar_style) === 'xiezhi'
        ? 'xiezhi'
        : 'initials',
    avatarUrl: absoluteHubAssetUrl(record.avatarUrl ?? record.avatar_url),
    organization: text(record.organization),
    plan,
    storedPlan: text(record.storedPlan ?? record.stored_plan),
    tokenLimit: Number(record.tokenLimit ?? record.token_limit ?? 0),
    tokenBalance: Number(record.tokenBalance ?? record.token_balance ?? 0),
    professionalUntil: text(record.professionalUntil ?? record.professional_until) || null,
    notificationEmail: text(record.notificationEmail ?? record.notification_email) || email,
    emailVerified: Boolean(record.emailVerified ?? record.email_verified),
    phoneNumber: text(record.phoneNumber ?? record.phone_number),
    phoneVerified: Boolean(record.phoneVerified ?? record.phone_verified),
    realNameVerified: Boolean(record.realNameVerified ?? record.real_name_verified),
    realNameVerifiedAt: text(record.realNameVerifiedAt ?? record.real_name_verified_at) || null,
    identityNumberMasked: text(record.identityNumberMasked ?? record.identity_number_masked),
    realNameVerificationRequired: Boolean(record.realNameVerificationRequired ?? record.real_name_verification_required ?? true),
    realNameVerificationAvailable: Boolean(record.realNameVerificationAvailable ?? record.real_name_verification_available),
    accountReady: Boolean(record.accountReady ?? record.account_ready),
    createdAt: text(record.createdAt ?? record.created_at)
  }
}

function normalizeDesktopAuth(value: unknown, fallbackPlan: HubAccountPlan): HubDesktopAuthCredential | null {
  const record = objectValue(value)
  const token = text(record.token)
  if (!token) return null
  const plan = normalizePlan(record.plan || fallbackPlan)
  const edition = text(record.edition) === 'standard' || text(record.edition) === 'professional'
    ? text(record.edition) as HubProductEdition
    : editionFromPlan(plan)
  return {
    token,
    expiresAt: text(record.expiresAt ?? record.expires_at) || null,
    edition,
    canSwitch: Boolean(record.canSwitch ?? record.can_switch),
    plan
  }
}

function normalizeEntitlement(value: unknown, fallbackPlan: HubAccountPlan): HubEntitlement {
  const record = objectValue(value)
  const plan = normalizePlan(record.plan || fallbackPlan)
  const edition = text(record.edition) === 'standard' || text(record.edition) === 'professional'
    ? text(record.edition) as HubProductEdition
    : editionFromPlan(plan)
  return {
    plan,
    edition,
    canSwitch: Boolean(record.canSwitch ?? record.can_switch),
    checkedAt: text(record.checkedAt ?? record.checked_at) || new Date().toISOString(),
    expiresAt: text(record.expiresAt ?? record.expires_at) || null
  }
}

function normalizeGateway(value: unknown): HubGatewayCredential | null {
  const record = objectValue(value)
  const token = text(record.token)
  const baseUrl = trimTrailingSlash(text(record.baseUrl ?? record.base_url)) || DEFAULT_HUB_GATEWAY_BASE_URL
  if (!token) return null
  return {
    token,
    baseUrl,
    expiresAt: text(record.expiresAt ?? record.expires_at) || null
  }
}

function normalizeModels(value: unknown): HubGatewayModel[] {
  const record = objectValue(value)
  const data = Array.isArray(record.data) ? record.data : Array.isArray(value) ? value : []
  const seen = new Set<string>()
  const out: HubGatewayModel[] = []
  for (const item of data) {
    const model = objectValue(item)
    const id = text(model.id)
    if (!id || seen.has(id)) continue
    seen.add(id)
    out.push({
      id,
      ownedBy: text(model.owned_by ?? model.ownedBy),
      created: Number(model.created || 0) || undefined
    })
  }
  return out
}

function requestStatus(error: unknown): number {
  return Number((error as Error & { status?: number }).status || 0)
}

function isMissingHubApi(error: unknown): boolean {
  return requestStatus(error) === 404
}

function normalizeGatewayUsageSummary(value: unknown): HubGatewayUsageSummary {
  const record = objectValue(value)
  const summary: HubGatewayUsageSummary = {
    unit: text(record.unit) || 'ai_credits',
    requestCount30d: Math.max(0, numberValue(record.requestCount30d ?? record.request_count_30d)),
    completedCount30d: Math.max(0, numberValue(record.completedCount30d ?? record.completed_count_30d)),
    issueCount30d: Math.max(0, numberValue(record.issueCount30d ?? record.issue_count_30d)),
    rawTokensToday: Math.max(0, numberValue(record.rawTokensToday ?? record.raw_tokens_today)),
    billableTokensToday: Math.max(0, numberValue(record.billableTokensToday ?? record.billable_tokens_today)),
    rawTokens30d: Math.max(0, numberValue(record.rawTokens30d ?? record.raw_tokens_30d)),
    billableTokens30d: Math.max(0, numberValue(record.billableTokens30d ?? record.billable_tokens_30d)),
    cachedPromptTokens30d: Math.max(0, numberValue(record.cachedPromptTokens30d ?? record.cached_prompt_tokens_30d)),
    qwenBillableTokens30d: Math.max(0, numberValue(record.qwenBillableTokens30d ?? record.qwen_billable_tokens_30d)),
    deepseekBillableTokens30d: Math.max(0, numberValue(record.deepseekBillableTokens30d ?? record.deepseek_billable_tokens_30d)),
    mimoBillableTokens30d: Math.max(0, numberValue(record.mimoBillableTokens30d ?? record.mimo_billable_tokens_30d))
  }
  const ruleVersion = text(record.ruleVersion ?? record.rule_version)
  const accountingStartedAt = text(record.accountingStartedAt ?? record.accounting_started_at)
  if (ruleVersion) summary.ruleVersion = ruleVersion
  if (accountingStartedAt) summary.accountingStartedAt = accountingStartedAt
  return summary
}

function normalizeQuotaWindows(value: unknown): HubGatewayQuotaWindow[] {
  const data = Array.isArray(value) ? value : []
  return data.flatMap((item): HubGatewayQuotaWindow[] => {
    const record = objectValue(item)
    const scopeType = text(record.scopeType ?? record.scope_type)
    const windowKind = text(record.windowKind ?? record.window_kind)
    const limit = Number(record.limit || 0)
    if (
      !['plan', 'policy', 'provider'].includes(scopeType) ||
      !['session_5h', 'day', 'week', 'month', 'billing_cycle'].includes(windowKind) ||
      !Number.isFinite(limit) ||
      limit <= 0
    ) {
      return []
    }
    const used = Number(record.usedBillableTokens ?? record.used_billable_tokens ?? 0)
    const reserved = Number(record.reservedBillableTokens ?? record.reserved_billable_tokens ?? 0)
    const window: HubGatewayQuotaWindow = {
      id: text(record.id) || `${scopeType}:${text(record.scopeId ?? record.scope_id)}:${windowKind}`,
      scopeType: scopeType as HubGatewayQuotaWindow['scopeType'],
      scopeId: text(record.scopeId ?? record.scope_id),
      windowKind: windowKind as HubGatewayQuotaWindow['windowKind'],
      windowStart: text(record.windowStart ?? record.window_start) || null,
      windowEnd: text(record.windowEnd ?? record.window_end) || null,
      usedBillableTokens: Number.isFinite(used) ? Math.max(0, used) : 0,
      reservedBillableTokens: Number.isFinite(reserved) ? Math.max(0, reserved) : 0,
      limit: Math.max(0, limit),
      resetAt: text(record.resetAt ?? record.reset_at) || text(record.windowEnd ?? record.window_end) || null,
      requestCount: Number(record.requestCount ?? record.request_count ?? 0) || 0,
      label: text(record.label)
    }
    const unit = text(record.unit)
    const ruleVersion = text(record.ruleVersion ?? record.rule_version)
    const accountingStartedAt = text(record.accountingStartedAt ?? record.accounting_started_at)
    if (unit) window.unit = unit
    if (ruleVersion) window.ruleVersion = ruleVersion
    if (accountingStartedAt) window.accountingStartedAt = accountingStartedAt
    return [window]
  })
}

function numberValue(value: unknown): number {
  const parsed = Number(value ?? 0)
  return Number.isFinite(parsed) ? parsed : 0
}

function nullableNumberValue(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function booleanValue(value: unknown): boolean {
  return value === true
}

function normalizeGatewayProfile(value: unknown): HubGatewayProfile | undefined {
  if (!value || typeof value !== 'object') return undefined
  const record = objectValue(value)
  const summary = objectValue(record.summary)
  const insights = objectValue(record.activityInsights ?? record.activity_insights)
  const completeness = objectValue(record.dataCompleteness ?? record.data_completeness)
  const dailyUsage = Array.isArray(record.dailyUsage ?? record.daily_usage)
    ? (record.dailyUsage ?? record.daily_usage) as unknown[]
    : []
  const topInvocations = Array.isArray(record.topInvocations ?? record.top_invocations)
    ? (record.topInvocations ?? record.top_invocations) as unknown[]
    : []

  return {
    timezone: text(record.timezone) || 'Asia/Shanghai',
    dailyUsage: dailyUsage.flatMap((item): HubGatewayProfile['dailyUsage'] => {
      const row = objectValue(item)
      const date = text(row.date ?? row.startDate ?? row.start_date)
      if (!/^\d{4}-\d{2}-\d{2}$/u.test(date)) return []
      return [{
        date,
        tokens: Math.max(0, numberValue(row.tokens)),
        rawTokens: Math.max(0, numberValue(row.rawTokens ?? row.raw_tokens)),
        requestCount: Math.max(0, numberValue(row.requestCount ?? row.request_count)),
        maxLatencyMs: Math.max(0, numberValue(row.maxLatencyMs ?? row.max_latency_ms))
      }]
    }),
    summary: {
      totalTextTokens: Math.max(0, numberValue(summary.totalTextTokens ?? summary.total_text_tokens)),
      totalRawTokens: Math.max(0, numberValue(summary.totalRawTokens ?? summary.total_raw_tokens)),
      peakTokens: Math.max(0, numberValue(summary.peakTokens ?? summary.peak_tokens)),
      longestRequestDurationMs: Math.max(0, numberValue(summary.longestRequestDurationMs ?? summary.longest_request_duration_ms)),
      currentStreakDays: Math.max(0, numberValue(summary.currentStreakDays ?? summary.current_streak_days)),
      longestStreakDays: Math.max(0, numberValue(summary.longestStreakDays ?? summary.longest_streak_days)),
      totalRequests: Math.max(0, numberValue(summary.totalRequests ?? summary.total_requests)),
      activeDays: Math.max(0, numberValue(summary.activeDays ?? summary.active_days)),
      providerCount: Math.max(0, numberValue(summary.providerCount ?? summary.provider_count)),
      modelCount: Math.max(0, numberValue(summary.modelCount ?? summary.model_count))
    },
    activityInsights: {
      fastModeUsagePercentage: nullableNumberValue(insights.fastModeUsagePercentage ?? insights.fast_mode_usage_percentage),
      mostUsedReasoningEffort: text(insights.mostUsedReasoningEffort ?? insights.most_used_reasoning_effort) || null,
      mostUsedReasoningEffortPercentage: nullableNumberValue(insights.mostUsedReasoningEffortPercentage ?? insights.most_used_reasoning_effort_percentage),
      uniqueSkillsUsed: nullableNumberValue(insights.uniqueSkillsUsed ?? insights.unique_skills_used),
      totalSkillsUsed: nullableNumberValue(insights.totalSkillsUsed ?? insights.total_skills_used),
      totalRequests: Math.max(0, numberValue(insights.totalRequests ?? insights.total_requests)),
      source: text(insights.source),
      missingFields: Array.isArray(insights.missingFields ?? insights.missing_fields)
        ? ((insights.missingFields ?? insights.missing_fields) as unknown[]).map(text).filter(Boolean)
        : []
    },
    topInvocations: topInvocations.flatMap((item): HubGatewayProfile['topInvocations'] => {
      const row = objectValue(item)
      const name = text(row.name)
      const count = Math.max(0, numberValue(row.count))
      if (!name || count <= 0) return []
      return [{ name, count, source: text(row.source) || undefined }]
    }),
    dataCompleteness: {
      tokenActivity: booleanValue(completeness.tokenActivity ?? completeness.token_activity),
      profileSummary: booleanValue(completeness.profileSummary ?? completeness.profile_summary),
      pluginInvocations: booleanValue(completeness.pluginInvocations ?? completeness.plugin_invocations),
      reasoningEffort: booleanValue(completeness.reasoningEffort ?? completeness.reasoning_effort),
      quickMode: booleanValue(completeness.quickMode ?? completeness.quick_mode)
    }
  }
}

function stableProfileHash(value: string): string {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0).toString(36)
}

function parseProfileDateMs(value: unknown): number {
  const raw = text(value)
  if (!raw) return Number.NaN
  const parsed = Date.parse(raw)
  if (Number.isFinite(parsed)) return parsed
  const normalized = raw.replace(/\.(\d{3})\d+(Z|[+-]\d{2}:?\d{2})$/u, '.$1$2')
  const normalizedParsed = Date.parse(normalized)
  return Number.isFinite(normalizedParsed) ? normalizedParsed : Number.NaN
}

function isoFromMs(value: number): string {
  return new Date(Number.isFinite(value) ? value : Date.now()).toISOString()
}

function expandHomePath(value: string): string {
  const trimmed = text(value)
  if (trimmed === '~') return homedir()
  if (trimmed.startsWith('~/') || trimmed.startsWith('~\\')) return join(homedir(), trimmed.slice(2))
  return trimmed
}

function addProfileInvocation(counts: Map<string, number>, name: unknown, count = 1): void {
  const label = text(name).slice(0, 160)
  const safeCount = Math.trunc(Number(count || 0))
  if (!label || !Number.isFinite(safeCount) || safeCount <= 0) return
  counts.set(label, Math.min(1_000_000, (counts.get(label) ?? 0) + Math.min(safeCount, 10_000)))
}

function profileInvocationList(counts: Map<string, number>): HubProfileEventSyncItem['toolInvocations'] {
  return [...counts.entries()]
    .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
    .slice(0, 80)
    .map(([name, count]) => ({ name, count }))
}

function pluginNameFromProfileTool(toolName: string): string {
  const normalized = text(toolName)
  if (!normalized.startsWith('mcp__')) return ''
  if (normalized.startsWith('mcp__analytix_funds__')) return '@analytix-fund-analysis'
  if (normalized.startsWith('mcp__analytix-computer-use__')) return '@analytix-computer-use'
  const match = normalized.match(/^mcp__(.+?)__/u)
  const serverName = text(match?.[1]).replace(/_/gu, '-')
  if (!serverName) return ''
  if (serverName === 'analytix-funds') return '@analytix-fund-analysis'
  return `@${serverName}`
}

function addProfileSkillNamesFromText(counts: Map<string, number>, value: unknown): void {
  const source = text(value)
  if (!source) return
  for (const match of source.matchAll(/plugin:\/\/([^\s)\]]+)/giu)) {
    const pluginId = text(match[1]).split('@')[0]
    if (pluginId) addProfileInvocation(counts, `@${pluginId}`)
  }
  for (const match of source.matchAll(/\/skill:([A-Za-z0-9._-]+)/gu)) {
    if (match[1]) addProfileInvocation(counts, match[1])
  }
}

async function readJsonlObjects(filePath: string): Promise<Record<string, unknown>[]> {
  try {
    const content = await readFile(filePath, 'utf8')
    return content
      .split(/\r?\n/u)
      .flatMap((line) => {
        const trimmed = line.trim()
        if (!trimmed) return []
        try {
          const parsed = JSON.parse(trimmed)
          return parsed && typeof parsed === 'object' ? [parsed as Record<string, unknown>] : []
        } catch {
          return []
        }
      })
  } catch {
    return []
  }
}

function latestThreadFromMetadata(rows: Record<string, unknown>[]): Record<string, unknown> | null {
  let thread: Record<string, unknown> | null = null
  for (const row of rows) {
    const candidate = objectValue(row.thread)
    if (Object.keys(candidate).length > 0) thread = candidate
  }
  return thread
}

function mostCommonProfileValue(counts: Map<string, number>, fallback: string): string {
  const top = [...counts.entries()]
    .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))[0]
  return top?.[0] || fallback
}

async function buildLocalProfileEventFromThreadDir(
  threadDirName: string,
  threadDir: string
): Promise<HubProfileEventSyncItem | null> {
  const [metadataRows, messageRows] = await Promise.all([
    readJsonlObjects(join(threadDir, 'metadata.jsonl')),
    readJsonlObjects(join(threadDir, 'messages.jsonl'))
  ])
  const thread = latestThreadFromMetadata(metadataRows)
  const turns = arrayValue(thread?.turns).map(objectValue)
  if (turns.length === 0 && messageRows.length === 0) return null

  const threadId = text(thread?.id) || threadDirName
  const threadHash = stableProfileHash(threadId)
  const toolCounts = new Map<string, number>()
  const skillCounts = new Map<string, number>()
  const reasoningCounts = new Map<string, number>()
  const toolCalls = new Map<string, { name: string; completed: boolean }>()
  const turnWindows = new Map<string, { start: number; end: number }>()
  let latestMs = parseProfileDateMs(thread?.updatedAt ?? thread?.createdAt)
  let longestDurationMs = 0

  for (const turn of turns) {
    const reasoning = text(turn.reasoningEffort ?? turn.reasoning_effort ?? turn.effort).toLowerCase() || 'auto'
    reasoningCounts.set(reasoning, (reasoningCounts.get(reasoning) ?? 0) + 1)
    for (const skillId of arrayValue(turn.activeSkillIds ?? turn.active_skill_ids)) {
      addProfileInvocation(skillCounts, skillId)
    }
    const startedMs = parseProfileDateMs(turn.startedAt ?? turn.started_at ?? turn.createdAt ?? turn.created_at)
    const finishedMs = parseProfileDateMs(turn.finishedAt ?? turn.finished_at ?? turn.updatedAt ?? turn.updated_at)
    if (Number.isFinite(startedMs)) latestMs = Math.max(Number.isFinite(latestMs) ? latestMs : 0, startedMs)
    if (Number.isFinite(finishedMs)) latestMs = Math.max(Number.isFinite(latestMs) ? latestMs : 0, finishedMs)
    if (Number.isFinite(startedMs) && Number.isFinite(finishedMs) && finishedMs >= startedMs) {
      longestDurationMs = Math.max(longestDurationMs, finishedMs - startedMs)
    }
  }

  for (const row of messageRows) {
    const createdMs = parseProfileDateMs(row.createdAt ?? row.created_at)
    const finishedMs = parseProfileDateMs(row.finishedAt ?? row.finished_at ?? row.createdAt ?? row.created_at)
    if (Number.isFinite(createdMs)) latestMs = Math.max(Number.isFinite(latestMs) ? latestMs : 0, createdMs)
    if (Number.isFinite(finishedMs)) latestMs = Math.max(Number.isFinite(latestMs) ? latestMs : 0, finishedMs)

    const turnId = text(row.turnId ?? row.turn_id)
    if (turnId && Number.isFinite(createdMs)) {
      const current = turnWindows.get(turnId) ?? { start: createdMs, end: createdMs }
      current.start = Math.min(current.start, createdMs)
      current.end = Math.max(current.end, Number.isFinite(finishedMs) ? finishedMs : createdMs)
      turnWindows.set(turnId, current)
    }

    if (text(row.role) === 'user' || text(row.kind) === 'user_message') {
      addProfileSkillNamesFromText(skillCounts, row.text)
      for (const skillId of arrayValue(objectValue(row.meta).activeSkillIds)) {
        addProfileInvocation(skillCounts, skillId)
      }
    }

    const toolName = text(row.toolName ?? row.tool_name ?? row.name)
    if (!toolName) continue
    const key = text(row.callId ?? row.call_id ?? row.id) || `${toolName}:${text(row.createdAt ?? row.created_at)}`
    const current = toolCalls.get(key) ?? { name: toolName, completed: false }
    current.name = current.name || toolName
    current.completed = current.completed || text(row.status).toLowerCase() === 'completed'
    toolCalls.set(key, current)
  }

  for (const window of turnWindows.values()) {
    if (Number.isFinite(window.start) && Number.isFinite(window.end) && window.end >= window.start) {
      longestDurationMs = Math.max(longestDurationMs, window.end - window.start)
    }
  }

  for (const tool of toolCalls.values()) {
    addProfileInvocation(toolCounts, tool.name)
    const pluginName = pluginNameFromProfileTool(tool.name)
    if (pluginName) addProfileInvocation(skillCounts, pluginName)
  }

  const mode = text(thread?.mode) || 'agent'
  return {
    eventKey: `thread-detail:${threadHash}`,
    source: 'desktop-local-backfill',
    eventKind: 'thread_detail',
    occurredAt: isoFromMs(latestMs),
    threadIdHash: threadHash,
    mode,
    reasoningEffort: mostCommonProfileValue(reasoningCounts, 'auto'),
    durationMs: Math.min(Math.max(0, Math.round(longestDurationMs)), 24 * 60 * 60 * 1000),
    toolInvocations: profileInvocationList(toolCounts),
    skillInvocations: profileInvocationList(skillCounts)
  }
}

function secretsDir(): string {
  return join(app.getPath('userData'), 'secrets')
}

function authFilePath(): string {
  return join(secretsDir(), HUB_AUTH_FILE_NAME)
}

function gatewayTokenFilePath(): string {
  return join(secretsDir(), HUB_GATEWAY_TOKEN_FILE_NAME)
}

function readGatewayTokenSync(): string {
  recordHubActivity('tokenRead')
  try {
    return readFileSync(gatewayTokenFilePath(), 'utf8').trim()
  } catch {
    return ''
  }
}

function readStoredAuthSync(): StoredHubAuth | null {
  recordHubActivity('tokenRead')
  try {
    const parsed = JSON.parse(readFileSync(authFilePath(), 'utf8')) as Partial<StoredHubAuth>
    if (parsed.version !== 1 || !parsed.desktopAuth?.token || !parsed.user?.id) return null
    return parsed as StoredHubAuth
  } catch {
    return null
  }
}

async function ensureSecretsDir(): Promise<void> {
  await mkdir(secretsDir(), { recursive: true, mode: 0o700 })
  await chmod(secretsDir(), 0o700).catch(() => undefined)
}

async function readStoredAuth(): Promise<StoredHubAuth | null> {
  recordHubActivity('tokenRead')
  try {
    const raw = await readFile(authFilePath(), 'utf8')
    const parsed = JSON.parse(raw) as Partial<StoredHubAuth>
    if (parsed.version !== 1 || !parsed.desktopAuth?.token || !parsed.user?.id) return null
    return parsed as StoredHubAuth
  } catch {
    return null
  }
}

async function writeStoredAuth(auth: StoredHubAuth): Promise<void> {
  await ensureSecretsDir()
  await writeFile(authFilePath(), `${JSON.stringify(auth, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 })
  await chmod(authFilePath(), 0o600).catch(() => undefined)
}

async function writeGatewayToken(gateway: HubGatewayCredential | null): Promise<void> {
  if (!gateway?.token) {
    await rm(gatewayTokenFilePath(), { force: true }).catch(() => undefined)
    clearHubGatewayRuntimeToken()
    return
  }
  await ensureSecretsDir()
  await writeFile(gatewayTokenFilePath(), `${gateway.token}\n`, { encoding: 'utf8', mode: 0o600 })
  await chmod(gatewayTokenFilePath(), 0o600).catch(() => undefined)
  setHubGatewayRuntimeToken(gateway.token)
}

async function requestHub<T>(path: string, options: RequestInit = {}, token = ''): Promise<T> {
  recordHubActivity('request')
  const response = await fetch(`${hubBaseUrl()}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers ?? {})
    }
  })
  if (!response.ok) {
    // Hub error bodies are untrusted and may contain credentials or personal
    // data. Keep both UI errors and diagnostics status-only.
    const error = new Error(`Hub request failed with status ${response.status}.`)
    error.name = 'HubRequestError'
    ;(error as Error & { status?: number }).status = response.status
    throw error
  }
  return await response.json().catch(() => ({})) as T
}

function modelProfileForHubModel(modelId: string): ModelProviderModelProfileV1 {
  const contextWindowTokens = /^qwen/i.test(modelId) ? 262_144 : 131_072
  const reasoning: ModelProviderModelProfileV1['reasoning'] = /^deepseek-v4(?:-|$)/i.test(modelId)
    ? {
        supportedEfforts: ['off', 'high', 'max'],
        defaultEffort: 'high',
        requestProtocol: 'deepseek-chat-completions'
      }
    : undefined
  return {
    inputModalities: ['text'],
    outputModalities: ['text'],
    supportsToolCalling: true,
    messageParts: ['text'],
    contextWindowTokens,
    ...(reasoning ? { reasoning } : {})
  }
}

function normalizeBootstrapEnabled(options: HubAccountServiceOptions): boolean {
  return (
    !options.isPackaged?.() &&
    text(process.env.ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP) === '1'
  )
}

export class HubAccountService {
  private readonly store: JsonSettingsStore
  private readonly restartRuntime?: () => Promise<void>
  private readonly logError?: (category: string, message: string, detail?: unknown) => void
  private readonly bootstrapAllowed: boolean
  private authMutationTail: Promise<void> = Promise.resolve()

  constructor(options: HubAccountServiceOptions) {
    recordHubActivity('serviceInstance')
    this.store = options.store
    this.restartRuntime = options.restartRuntime
    this.logError = options.logError
    this.bootstrapAllowed = normalizeBootstrapEnabled({
      ...options,
      isPackaged: options.isPackaged ?? (() => app.isPackaged),
      nodeEnv: options.nodeEnv ?? (() => process.env.NODE_ENV ?? '')
    })
    const stored = readStoredAuthSync()
    setHubGatewayRuntimeToken(stored ? readGatewayTokenSync() : '')
  }

  async getSnapshot(): Promise<HubAccountSnapshot> {
    return this.runAuthMutation(() => this.getSnapshotNow())
  }

  private async getSnapshotNow(): Promise<HubAccountSnapshot> {
    const stored = await readStoredAuth()
    const gatewayToken = readGatewayTokenSync()
    if (!stored) {
      await this.clearLocalAuth()
      if (this.bootstrapAllowed) return this.bootstrapForTests()
      return { authenticated: false, gatewayConfigured: false, accountReady: false, source: 'none' }
    }
    if (gatewayToken) setHubGatewayRuntimeToken(gatewayToken)
    return this.snapshotFromStored(stored, Boolean(gatewayToken))
  }

  async refresh(): Promise<HubAccountSnapshot> {
    return this.runAuthMutation(() => this.refreshNow())
  }

  private async refreshNow(): Promise<HubAccountSnapshot> {
    const stored = await readStoredAuth()
    if (!stored) {
      if (this.bootstrapAllowed) return this.bootstrapForTests()
      await this.clearLocalAuth()
      return { authenticated: false, gatewayConfigured: false, accountReady: false, source: 'none' }
    }
    if (this.bootstrapAllowed && stored.source === 'test-bootstrap') {
      const gatewayConfigured = Boolean(readGatewayTokenSync())
      if (gatewayConfigured) setHubGatewayRuntimeToken(readGatewayTokenSync())
      return this.snapshotFromStored(stored, gatewayConfigured)
    }
    try {
      const entitlement = await requestHub<HubEntitlementResponse>('/api/desktop/entitlement', { method: 'GET' }, stored.desktopAuth.token)
      const user = normalizeUser(entitlement.user)
      if (!user) throw new Error('Hub did not return a valid user.')
      let nextGateway = normalizeGateway(entitlement.gateway)
      if (!nextGateway && user.accountReady) {
        const refreshedGateway = await requestHub<HubEntitlementResponse>(
          '/api/desktop/gateway/refresh',
          { method: 'POST', body: '{}' },
          stored.desktopAuth.token
        )
        nextGateway = normalizeGateway(refreshedGateway.gateway)
      }
      const nextStored = await this.persistAuth({
        source: stored.source,
        desktopAuth: stored.desktopAuth,
        user,
        entitlement: normalizeEntitlement(entitlement.entitlement, user.plan),
        gateway: nextGateway,
        models: nextGateway ? [] : stored.models
      })
      if (nextGateway) {
        try {
          const ready = await this.finishPostAuthGatewaySync(nextStored, nextGateway, 'refresh')
          return this.snapshotFromStored(ready, readGatewayTokenSync() === nextGateway.token)
        } catch (error) {
          return this.gatewayInitializationFailure(nextStored, 'refresh', error)
        }
      }
      return this.snapshotFromStored(nextStored, false)
    } catch (error) {
      const status = (error as Error & { status?: number }).status
      if (status === 401) {
        await this.clearLocalAuth()
        await this.restartRuntime?.().catch((restartError) => {
          this.logError?.('hub-account', 'Failed to restart runtime after expired Hub authorization', {
            code: 'hub_expired_auth_runtime_restart_failed',
            failureType: restartError instanceof Error ? restartError.name : 'unknown'
          })
        })
        return { authenticated: false, gatewayConfigured: false, accountReady: false, source: 'none', error: '桌面授权已过期，请重新登录。' }
      }
      this.logError?.('hub-account', 'Failed to refresh Hub account', {
        code: 'hub_account_refresh_failed',
        status: requestStatus(error),
        failureType: error instanceof Error ? error.name : 'unknown'
      })
      const fallback = this.snapshotFromStored(stored, Boolean(readGatewayTokenSync()))
      return { ...fallback, error: error instanceof Error ? error.message : String(error) }
    }
  }

  async login(request: HubLoginRequest): Promise<HubAccountSnapshot> {
    return this.runAuthMutation(async () => {
      const payload = await requestHub<HubLoginResponse>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({
          email: text(request.email).toLowerCase(),
          password: request.password,
          authChallengeProof: text(request.authChallengeProof),
          rememberForDays: Number(request.rememberForDays || 0),
          rememberMe: Number(request.rememberForDays || 0) > 0
        })
      })
      return this.completeAuthResponse(payload, 'hub')
    })
  }

  async register(request: HubRegisterRequest): Promise<HubAccountSnapshot> {
    return this.runAuthMutation(async () => {
      const payload = await requestHub<HubLoginResponse>('/api/auth/register', {
        method: 'POST',
        body: JSON.stringify({
          ...request,
          email: text(request.email).toLowerCase(),
          organization: text(request.organization),
          authChallengeProof: text(request.authChallengeProof),
          referralCode: text(request.referralCode),
          rememberForDays: Number(request.rememberForDays || 0),
          rememberMe: Number(request.rememberForDays || 0) > 0
        })
      })
      return this.completeAuthResponse(payload, 'hub')
    })
  }

  async sendRegisterVerificationCode(request: HubVerificationCodeRequest): Promise<HubAccountApiResult<{ expiresAt: string; retryAfterSeconds: number }>> {
    return this.safeResult(async () => {
      const payload = await requestHub<{ expiresAt?: string; retryAfterSeconds?: number }>('/api/auth/register/send-verification-code', {
        method: 'POST',
        body: JSON.stringify({
          email: text(request.email).toLowerCase(),
          authChallengeProof: text(request.authChallengeProof)
        })
      })
      return {
        expiresAt: text(payload.expiresAt),
        retryAfterSeconds: Number(payload.retryAfterSeconds || 0)
      }
    })
  }

  async sendPasswordResetVerificationCode(request: HubVerificationCodeRequest): Promise<HubAccountApiResult<{ expiresAt: string; retryAfterSeconds: number }>> {
    return this.safeResult(async () => {
      const payload = await requestHub<{ expiresAt?: string; retryAfterSeconds?: number }>('/api/auth/password-reset/send-verification-code', {
        method: 'POST',
        body: JSON.stringify({ email: text(request.email).toLowerCase() })
      })
      return {
        expiresAt: text(payload.expiresAt),
        retryAfterSeconds: Number(payload.retryAfterSeconds || 0)
      }
    })
  }

  async confirmPasswordReset(request: HubPasswordResetConfirmRequest): Promise<HubAccountApiResult<Record<string, unknown>>> {
    return this.safeResult(async () => {
      await requestHub('/api/auth/password-reset/confirm', {
        method: 'POST',
        body: JSON.stringify({
          email: text(request.email).toLowerCase(),
          verificationCode: text(request.verificationCode),
          nextPassword: request.nextPassword
        })
      })
      return {}
    })
  }

  async fetchChallenge(mode: HubAuthChallengeMode): Promise<HubAccountApiResult<{ challenge: HubAuthChallenge }>> {
    return this.safeResult(async () => ({
      challenge: await requestHub<HubAuthChallenge>(`/api/auth/challenge?mode=${encodeURIComponent(mode)}`, { method: 'GET' })
    }))
  }

  async fetchChallengeState(mode: HubAuthChallengeMode): Promise<HubAccountApiResult<{ state: HubAuthChallengeState }>> {
    return this.safeResult(async () => ({
      state: await requestHub<HubAuthChallengeState>(`/api/auth/challenge/state?mode=${encodeURIComponent(mode)}`, { method: 'GET' })
    }))
  }

  async verifyChallenge(request: HubAuthChallengeVerifyRequest): Promise<HubAccountApiResult<{ verification: HubAuthChallengeVerifyResult }>> {
    return this.safeResult(async () => ({
      verification: await requestHub<HubAuthChallengeVerifyResult>('/api/auth/challenge/verify', {
        method: 'POST',
        body: JSON.stringify(request)
      })
    }))
  }

  async syncProfileEvents(request: HubProfileEventSyncRequest): Promise<HubAccountApiResult<HubProfileEventSyncResult>> {
    return this.withDesktopAuth(async (token) =>
      requestHub<HubProfileEventSyncResult>('/api/desktop/profile-events', {
        method: 'POST',
        body: JSON.stringify(request)
      }, token)
    )
  }

  async updateProfile(request: HubProfileUpdateRequest): Promise<HubAccountApiResult<HubProfileUpdateResult>> {
    return this.withDesktopAuth(async (token) => {
      const payload = await requestHub<HubProfileUpdateResult>('/api/desktop/profile', {
        method: 'PATCH',
        body: JSON.stringify(request)
      }, token)
      const user = normalizeUser(payload.user)
      if (!user) throw new Error('Hub 未返回有效用户信息。')

      const stored = await readStoredAuth()
      if (stored) {
        await writeStoredAuth({
          ...stored,
          user,
          entitlement: normalizeEntitlement(stored.entitlement, user.plan),
          checkedAt: new Date().toISOString()
        })
      }

      return { user }
    })
  }

  async syncLocalProfileEvents(): Promise<HubAccountApiResult<HubProfileLocalSyncResult>> {
    return this.withDesktopAuth(async (token) => {
      const { scannedThreadCount, events } = await this.collectLocalProfileEvents()
      let acceptedCount = 0
      let syncedCount = 0

      for (let index = 0; index < events.length; index += PROFILE_LOCAL_SYNC_BATCH_SIZE) {
        const batch = events.slice(index, index + PROFILE_LOCAL_SYNC_BATCH_SIZE)
        const result = await requestHub<HubProfileEventSyncResult>('/api/desktop/profile-events', {
          method: 'POST',
          body: JSON.stringify({ events: batch })
        }, token)
        acceptedCount += result.acceptedCount
        syncedCount += result.syncedCount
      }

      return {
        acceptedCount,
        syncedCount,
        scannedThreadCount,
        generatedEventCount: events.length
      }
    })
  }

  async usage(): Promise<HubAccountApiResult<HubUsageResult>> {
    return this.withDesktopAuth(async (token) => {
      let payload: HubUsageResult | HubEntitlementResponse
      try {
        payload = await requestHub<HubUsageResult>('/api/desktop/console', { method: 'GET' }, token)
      } catch (error) {
        if (!isMissingHubApi(error)) throw error
        payload = await requestHub<HubEntitlementResponse>('/api/desktop/entitlement', { method: 'GET' }, token)
      }
      const user = normalizeUser(payload.user)
      if (!user) throw new Error('Hub 未返回有效用户信息。')
      return {
        user,
        gatewayUsageSummary: normalizeGatewayUsageSummary('gatewayUsageSummary' in payload ? payload.gatewayUsageSummary : undefined),
        gatewayQuotaWindows: normalizeQuotaWindows('gatewayQuotaWindows' in payload ? payload.gatewayQuotaWindows : []),
        gatewayProfile: normalizeGatewayProfile('gatewayProfile' in payload ? payload.gatewayProfile : undefined)
      }
    })
  }

  async referral(): Promise<HubAccountApiResult<HubReferralResult>> {
    return this.withDesktopAuth(async (token) => {
      try {
        return await requestHub<HubReferralResult>('/api/desktop/referral', { method: 'GET' }, token)
      } catch (error) {
        if (!isMissingHubApi(error)) throw error
        throw new Error('邀请信息暂未同步，请在个人账户中查看邀请。')
      }
    })
  }

  async models(): Promise<HubAccountApiResult<HubGatewayModelsResult>> {
    return this.safeResult(async () => {
      const stored = await readStoredAuth()
      const baseUrl = stored?.gateway?.baseUrl || DEFAULT_HUB_GATEWAY_BASE_URL
      const gatewayToken = readGatewayTokenSync()
      if (!stored?.desktopAuth.token || !gatewayToken) throw new Error('缺少网关凭证。')
      const models = await this.fetchModelsFromGateway({ token: gatewayToken, baseUrl })
      return { models, defaultModelId: models[0]?.id }
    })
  }

  async logout(): Promise<HubAccountSnapshot> {
    return this.runAuthMutation(async () => {
      const stored = await readStoredAuth()
      const gatewayToken = readGatewayTokenSync()
      if (stored?.desktopAuth.token) {
        await requestHub('/api/desktop/logout', {
          method: 'POST',
          body: JSON.stringify({ gatewayToken })
        }, stored.desktopAuth.token).catch((error) => {
          this.logError?.('hub-account', 'Hub logout failed', {
            code: 'hub_logout_failed',
            status: requestStatus(error),
            failureType: error instanceof Error ? error.name : 'unknown'
          })
        })
      }
      await this.clearLocalAuth()
      await this.restartRuntime?.().catch(() => undefined)
      return { authenticated: false, gatewayConfigured: false, accountReady: false, source: 'none' }
    })
  }

  private async completeAuthResponse(payload: HubLoginResponse, source: 'hub' | 'test-bootstrap'): Promise<HubAccountSnapshot> {
    const user = normalizeUser(payload.user)
    if (!user) throw new Error('Hub 未返回有效用户信息。')
    const desktopAuth = normalizeDesktopAuth(payload.desktopAuth, user.plan)
    if (!desktopAuth) throw new Error('Hub 未返回桌面授权凭证。')
    const gateway = normalizeGateway(payload.gateway)
    const stored = await this.persistAuth({
      source,
      desktopAuth,
      user,
      entitlement: normalizeEntitlement(desktopAuth, user.plan),
      gateway,
      models: []
    })
    if (!gateway) {
      return this.snapshotFromStored(stored, false)
    }
    try {
      const ready = await this.finishPostAuthGatewaySync(stored, gateway, source === 'hub' ? 'login' : 'test bootstrap')
      return this.snapshotFromStored(ready, readGatewayTokenSync() === gateway.token)
    } catch (error) {
      return this.gatewayInitializationFailure(stored, source === 'hub' ? 'login' : 'test bootstrap', error)
    }
  }

  private async persistAuth(input: {
    source: 'hub' | 'test-bootstrap'
    desktopAuth: HubDesktopAuthCredential
    user: HubUserSnapshot
    entitlement: HubEntitlement
    gateway?: HubGatewayCredential | null
    models?: HubGatewayModel[]
  }): Promise<StoredHubAuth> {
    await writeGatewayToken(input.gateway ?? null)
    const stored: StoredHubAuth = {
      version: 1,
      source: input.source,
      desktopAuth: input.desktopAuth,
      user: input.user,
      entitlement: input.entitlement,
      gateway: input.gateway
        ? {
            configured: true,
            baseUrl: input.gateway.baseUrl,
            expiresAt: input.gateway.expiresAt
          }
        : undefined,
      models: input.models ?? [],
      checkedAt: new Date().toISOString()
    }
    await writeStoredAuth(stored)
    return stored
  }

  private snapshotFromStored(stored: StoredHubAuth, gatewayConfigured: boolean): HubAccountSnapshot {
    return {
      authenticated: true,
      gatewayConfigured,
      accountReady: stored.user.accountReady && gatewayConfigured && (stored.models?.length ?? 0) > 0,
      source: stored.source,
      checkedAt: stored.checkedAt,
      user: stored.user,
      entitlement: stored.entitlement,
      gateway: stored.gateway
        ? {
            configured: gatewayConfigured,
            baseUrl: stored.gateway.baseUrl,
            expiresAt: stored.gateway.expiresAt
          }
        : undefined,
      models: stored.models ?? []
    }
  }

  private async clearLocalAuth(): Promise<void> {
    await rm(authFilePath(), { force: true }).catch(() => undefined)
    await writeGatewayToken(null)
  }

  private async fetchAndSyncModels(gateway: HubGatewayCredential): Promise<HubGatewayModel[]> {
    const models = await this.fetchModelsFromGateway(gateway)
    if (models.length === 0) {
      throw new Error('Hub model list is empty.')
    }
    await this.syncManagedProvider(gateway, models)
    return models
  }

  private async finishPostAuthGatewaySync(
    expected: StoredHubAuth,
    gateway: HubGatewayCredential,
    reason: string
  ): Promise<StoredHubAuth> {
    const models = await this.fetchAndSyncModels(gateway)
    await this.restartRuntime?.()
    const stored = await readStoredAuth()
    const currentGatewayToken = readGatewayTokenSync()
    if (
      !stored?.gateway?.configured ||
      stored.desktopAuth.token !== expected.desktopAuth.token ||
      stored.user.id !== expected.user.id ||
      stored.gateway.baseUrl !== trimTrailingSlash(gateway.baseUrl) ||
      currentGatewayToken !== gateway.token
    ) {
      throw new Error(`Hub ${reason} state changed before gateway initialization completed.`)
    }
    const ready: StoredHubAuth = {
      ...stored,
      models,
      checkedAt: new Date().toISOString()
    }
    await writeStoredAuth(ready)
    return ready
  }

  private gatewayInitializationFailure(stored: StoredHubAuth, reason: string, error: unknown): HubAccountSnapshot {
    this.logError?.('hub-account', `Failed to initialize Hub gateway after Hub ${reason}`, {
      code: 'hub_gateway_initialization_failed',
      failureType: error instanceof Error ? error.name : 'unknown'
    })
    return {
      ...this.snapshotFromStored({ ...stored, models: [] }, Boolean(stored.gateway?.configured)),
      error: HUB_GATEWAY_INITIALIZATION_ERROR
    }
  }

  private async fetchModelsFromGateway(gateway: Pick<HubGatewayCredential, 'token' | 'baseUrl'>): Promise<HubGatewayModel[]> {
    recordHubActivity('request')
    const response = await fetch(`${trimTrailingSlash(gateway.baseUrl)}/models`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${gateway.token}` },
      signal: AbortSignal.timeout(HUB_GATEWAY_MODELS_TIMEOUT_MS)
    })
    if (!response.ok) {
      // A gateway response body is untrusted and may contain upstream
      // credentials or personal data. Keep diagnostics status-only.
      throw new Error(`读取 Hub 模型列表失败（${response.status}）。`)
    }
    const payload = await response.json().catch(() => ({}))
    return normalizeModels(payload)
  }

  private async syncManagedProvider(gateway: Pick<HubGatewayCredential, 'baseUrl'>, models: HubGatewayModel[]): Promise<AppSettingsV1> {
    recordHubActivity('fallback')
    const settings = await this.store.load()
    const modelIds = models.map((model) => model.id)
    const currentModel = settings.runtime.model.trim()
    const selectedModel = modelIds.includes(currentModel) ? currentModel : modelIds[0] || currentModel
    const modelProfiles = Object.fromEntries(
      modelIds.map((modelId) => [modelId, modelProfileForHubModel(modelId)])
    )
    const providers = [
      ...settings.provider.providers.filter((provider) => provider.id !== HUB_MODEL_PROVIDER_ID),
      {
        id: HUB_MODEL_PROVIDER_ID,
        name: 'Analytix Hub',
        baseUrl: trimTrailingSlash(gateway.baseUrl),
        endpointFormat: 'chat_completions' as const,
        models: modelIds,
        modelProfiles
      }
    ]
    const patch: AppSettingsPatch = {
      provider: {
        ...settings.provider,
        activeProviderId: HUB_MODEL_PROVIDER_ID,
        providers
      },
      runtime: {
        providerId: HUB_MODEL_PROVIDER_ID,
        model: selectedModel
      }
    }
    return this.store.patch(patch)
  }

  private async collectLocalProfileEvents(): Promise<{
    scannedThreadCount: number
    events: HubProfileEventSyncItem[]
  }> {
    const settings = await this.store.load()
    const runtimeDataDir = expandHomePath(settings.runtime.dataDir)
    if (!runtimeDataDir) return { scannedThreadCount: 0, events: [] }
    const threadsDir = join(runtimeDataDir, 'threads')
    const entries = await readdir(threadsDir, { withFileTypes: true }).catch(() => [])
    const threadDirStats = await Promise.all(entries
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .map(async (name) => ({
        name,
        mtimeMs: await stat(join(threadsDir, name)).then((value) => value.mtimeMs).catch(() => 0)
      })))
    const threadDirs = threadDirStats
      .sort((left, right) => right.mtimeMs - left.mtimeMs || left.name.localeCompare(right.name))
      .slice(0, PROFILE_LOCAL_SYNC_MAX_THREADS)
      .map((item) => item.name)
    const events: HubProfileEventSyncItem[] = []

    for (const threadDirName of threadDirs) {
      const event = await buildLocalProfileEventFromThreadDir(threadDirName, join(threadsDir, threadDirName))
      if (event) events.push(event)
    }

    return {
      scannedThreadCount: threadDirs.length,
      events
    }
  }

  private async withDesktopAuth<T extends object>(callback: (token: string) => Promise<T>): Promise<HubAccountApiResult<T>> {
    return this.safeResult(async () => {
      const stored = await readStoredAuth()
      if (!stored?.desktopAuth.token) throw new Error('缺少桌面授权凭证。')
      return callback(stored.desktopAuth.token)
    })
  }

  private async safeResult<T extends object>(callback: () => Promise<T>): Promise<HubAccountApiResult<T>> {
    try {
      return { ok: true, ...(await callback()) }
    } catch (error) {
      return {
        ok: false,
        message: error instanceof Error ? error.message : String(error),
        snapshot: await this.getSnapshot().catch(() => undefined)
      }
    }
  }

  private runAuthMutation<T>(operation: () => Promise<T>): Promise<T> {
    const run = this.authMutationTail.then(operation, operation)
    this.authMutationTail = run.then(() => undefined, () => undefined)
    return run
  }

  private async bootstrapForTests(): Promise<HubAccountSnapshot> {
    const gatewayToken = text(process.env.ANALYTIX_HUB_TEST_GATEWAY_TOKEN)
    const desktopToken = text(process.env.ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN)
    if (gatewayToken && desktopToken) {
      const gateway: HubGatewayCredential = {
        token: gatewayToken,
        baseUrl: trimTrailingSlash(text(process.env.ANALYTIX_HUB_TEST_GATEWAY_BASE_URL)) || DEFAULT_HUB_GATEWAY_BASE_URL
      }
      const user: HubUserSnapshot = {
        id: 'test-bootstrap',
        email: text(process.env.ANALYTIX_HUB_TEST_EMAIL).toLowerCase() || 'test@example.invalid',
        fullName: text(process.env.ANALYTIX_HUB_TEST_NAME) || 'Analytix Test',
        plan: 'professional',
        accountReady: true
      }
      const stored = await this.persistAuth({
        source: 'test-bootstrap',
        desktopAuth: {
          token: desktopToken,
          edition: 'professional',
          canSwitch: false,
          plan: 'professional'
        },
        user,
        entitlement: {
          plan: 'professional',
          edition: 'professional',
          canSwitch: false,
          checkedAt: new Date().toISOString()
        },
        gateway,
        models: []
      })
      try {
        const ready = await this.finishPostAuthGatewaySync(stored, gateway, 'test bootstrap')
        return this.snapshotFromStored(ready, readGatewayTokenSync() === gateway.token)
      } catch (error) {
        return this.gatewayInitializationFailure(stored, 'test bootstrap', error)
      }
    }

    const email = text(process.env.ANALYTIX_HUB_TEST_EMAIL).toLowerCase()
    const password = text(process.env.ANALYTIX_HUB_TEST_PASSWORD)
    if (!email || !password) {
      return {
        authenticated: false,
        gatewayConfigured: false,
        accountReady: false,
        source: 'none',
        error: 'Test bootstrap is enabled but Hub test credentials are missing.'
      }
    }
    try {
      const payload = await requestHub<HubLoginResponse>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({
          email,
          password,
          rememberForDays: 7,
          rememberMe: true
        })
      })
      return this.completeAuthResponse(payload, 'test-bootstrap')
    } catch (error) {
      return {
        authenticated: false,
        gatewayConfigured: false,
        accountReady: false,
        source: 'none',
        error: error instanceof Error ? error.message : String(error)
      }
    }
  }
}

export function createHubAccountService(options: HubAccountServiceOptions): HubAccountService {
  return new HubAccountService(options)
}

export function hubGatewayTokenFileExists(): boolean {
  return existsSync(gatewayTokenFilePath())
}

export { gatewayTokenFilePath }
