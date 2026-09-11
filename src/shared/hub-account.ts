export const HUB_MODEL_PROVIDER_ID = 'analytix-hub'
export const DEFAULT_HUB_BASE_URL = 'https://analytix.top'
export const DEFAULT_HUB_GATEWAY_BASE_URL = 'https://analytix.top/v1'

export type HubAccountPlan = 'normal' | 'professional' | 'admin'
export type HubProductEdition = 'standard' | 'professional'
export type HubAvatarStyle = 'initials' | 'xiezhi' | 'photo'

export type HubUserSnapshot = {
  id: string
  email: string
  fullName: string
  displayName?: string
  username?: string
  avatarColor?: string
  avatarStyle?: HubAvatarStyle
  avatarUrl?: string
  organization?: string
  plan: HubAccountPlan
  storedPlan?: string
  tokenLimit?: number
  tokenBalance?: number
  professionalUntil?: string | null
  notificationEmail?: string
  emailVerified?: boolean
  phoneNumber?: string
  phoneVerified?: boolean
  realNameVerified?: boolean
  realNameVerifiedAt?: string | null
  identityNumberMasked?: string
  realNameVerificationRequired?: boolean
  realNameVerificationAvailable?: boolean
  accountReady: boolean
  createdAt?: string
}

export type HubDesktopAuthCredential = {
  token: string
  expiresAt?: string | null
  edition: HubProductEdition
  canSwitch: boolean
  plan?: HubAccountPlan
}

export type HubGatewayCredential = {
  token: string
  baseUrl: string
  expiresAt?: string | null
}

export type HubEntitlement = {
  plan: HubAccountPlan
  edition: HubProductEdition
  canSwitch: boolean
  checkedAt: string
  expiresAt?: string | null
}

export type HubAccountSnapshot = {
  authenticated: boolean
  gatewayConfigured: boolean
  accountReady: boolean
  source: 'none' | 'hub' | 'test-bootstrap'
  checkedAt?: string
  user?: HubUserSnapshot
  entitlement?: HubEntitlement
  gateway?: {
    configured: boolean
    baseUrl: string
    expiresAt?: string | null
  }
  models?: HubGatewayModel[]
  error?: string
}

export type HubLoginRequest = {
  email: string
  password: string
  authChallengeProof?: string
  rememberForDays?: number
}

export type HubRegisterRequest = {
  fullName: string
  organization?: string
  phoneNumber: string
  email: string
  password: string
  verificationCode: string
  authChallengeProof?: string
  referralCode?: string
  rememberForDays?: number
}

export type HubVerificationCodeRequest = {
  email: string
  authChallengeProof?: string
}

export type HubPasswordResetConfirmRequest = {
  email: string
  verificationCode: string
  nextPassword: string
}

export type HubAuthChallengeMode = 'login' | 'register'

export type HubAuthChallengeTile = {
  id: string
  imageUri: string
}

export type HubAuthChallenge = {
  challengeId: string
  prompt: {
    primary: string
    secondary: string
  }
  tiles: HubAuthChallengeTile[]
  expiresAt: string
}

export type HubAuthChallengeState = {
  challengeRequired: boolean
  failedAttempts: number
  threshold: number
}

export type HubAuthChallengeVerifyRequest = {
  mode: HubAuthChallengeMode
  challengeId: string
  selectedTileIds: string[]
}

export type HubAuthChallengeVerifyResult = {
  ok: boolean
  proofToken: string
  expiresAt: string
  remainingUses: number
}

export type HubGatewayModel = {
  id: string
  ownedBy?: string
  created?: number
}

export type HubGatewayModelsResult = {
  models: HubGatewayModel[]
  defaultModelId?: string
}

export type HubGatewayUsageSummary = {
  unit?: string
  ruleVersion?: string
  accountingStartedAt?: string | null
  requestCount30d: number
  completedCount30d: number
  issueCount30d: number
  rawTokensToday: number
  billableTokensToday: number
  rawTokens30d: number
  billableTokens30d: number
  cachedPromptTokens30d: number
  qwenBillableTokens30d: number
  deepseekBillableTokens30d: number
  mimoBillableTokens30d: number
}

export type HubGatewayQuotaWindow = {
  id: string
  unit?: string
  ruleVersion?: string
  accountingStartedAt?: string | null
  scopeType: 'plan' | 'policy' | 'provider'
  scopeId: string
  windowKind: 'session_5h' | 'day' | 'week' | 'month' | 'billing_cycle'
  windowStart: string | null
  windowEnd: string | null
  usedBillableTokens: number
  reservedBillableTokens: number
  limit: number
  resetAt: string | null
  requestCount: number
  label: string
}

export type HubProfileDailyUsage = {
  date: string
  tokens: number
  rawTokens: number
  requestCount: number
  maxLatencyMs: number
}

export type HubGatewayProfile = {
  timezone: string
  dailyUsage: HubProfileDailyUsage[]
  summary: {
    totalTextTokens: number
    totalRawTokens: number
    peakTokens: number
    longestRequestDurationMs: number
    currentStreakDays: number
    longestStreakDays: number
    totalRequests: number
    activeDays: number
    providerCount: number
    modelCount: number
  }
  activityInsights: {
    fastModeUsagePercentage: number | null
    mostUsedReasoningEffort: string | null
    mostUsedReasoningEffortPercentage: number | null
    uniqueSkillsUsed: number | null
    totalSkillsUsed: number | null
    totalRequests: number
    source: string
    missingFields: string[]
  }
  topInvocations: Array<{
    name: string
    count: number
    source?: string
  }>
  dataCompleteness: {
    tokenActivity: boolean
    profileSummary: boolean
    pluginInvocations: boolean
    reasoningEffort: boolean
    quickMode: boolean
  }
}

export type HubProfileInvocation = {
  name: string
  count: number
}

export type HubProfileEventSyncItem = {
  eventKey: string
  occurredAt: string
  source?: string
  eventKind?: string
  threadIdHash?: string
  mode?: string
  reasoningEffort?: string
  durationMs?: number
  toolInvocations?: HubProfileInvocation[]
  skillInvocations?: HubProfileInvocation[]
}

export type HubProfileEventSyncRequest = {
  events: HubProfileEventSyncItem[]
}

export type HubProfileEventSyncResult = {
  acceptedCount: number
  syncedCount: number
}

export type HubProfileLocalSyncResult = HubProfileEventSyncResult & {
  scannedThreadCount: number
  generatedEventCount: number
}

export type HubProfileUpdateRequest = {
  displayName?: string
  username?: string
  avatarColor?: string
  avatarStyle?: HubAvatarStyle
  avatarImageDataUrl?: string
}

export type HubProfileUpdateResult = {
  user: HubUserSnapshot
}

export type HubUsageResult = {
  user: HubUserSnapshot
  gatewayUsageSummary: HubGatewayUsageSummary
  gatewayQuotaWindows: HubGatewayQuotaWindow[]
  gatewayProfile?: HubGatewayProfile
}

export type HubReferralRecord = {
  id: string
  referredUserId: string
  referredUserName: string
  referredUserEmailMasked: string
  status: 'pending' | 'completed'
  createdAt: string
  qualifiedAt?: string | null
  rewardGrantedAt?: string | null
}

export type HubReferral = {
  code: string
  shareUrl: string
  rewardTokenGrant: number
  pendingCount: number
  completedCount: number
  rewardedTokenTotal: number
  referrals: HubReferralRecord[]
}

export type HubReferralResult = {
  referral: HubReferral
}

export type HubAccountApiResult<T> =
  | ({ ok: true } & T)
  | { ok: false; message: string; snapshot?: HubAccountSnapshot }
