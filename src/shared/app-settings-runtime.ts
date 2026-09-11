import {
  APPROVAL_POLICIES,
  CURRENT_EXECUTION_POLICY_VERSION,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_DEEPSEEK_BASE_URL,
  DEFAULT_IMAGE_GENERATION_PROTOCOL,
  DEFAULT_ANALYTIX_DATA_DIR,
  DEFAULT_ANALYTIX_MODEL,
  DEFAULT_ANALYTIX_PORT,
  DEFAULT_MUSIC_GENERATION_PROTOCOL,
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  DEFAULT_SANDBOX_MODE,
  SANDBOX_MODES,
  DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
  DEFAULT_TEXT_TO_SPEECH_PROTOCOL,
  DEFAULT_VIDEO_GENERATION_PROTOCOL,
  MODEL_REASONING_EFFORTS,
  MODEL_REASONING_REQUEST_PROTOCOLS,
  normalizeModelEndpointFormat,
  type AppSettingsV1,
  type ApprovalPolicy,
  type AnalytixBrowserApprovalMode,
  type AnalytixBrowserSitePermissionV1,
  type AnalytixBrowserUseSettingsV1,
  type AnalytixComputerUseSettingsV1,
  type AnalytixContextCompactionSettingsV1,
  type AnalytixDesignQualitySettingsV1,
  type AnalytixDesignQualityStrictness,
  type AnalytixHistoryHygieneSettingsV1,
  type AnalytixImageGenerationSettingsV1,
  type AnalytixMcpSearchSettingsV1,
  type AnalytixMusicGenerationSettingsV1,
  type AnalytixRuntimeTuningSettingsV1,
  type AnalytixRuntimeSettingsPatchV1,
  type AnalytixRuntimeSettingsV1,
  type SandboxMode,
  type AnalytixSpeechToTextSettingsV1,
  type AnalytixStorageSettingsV1,
  type AnalytixSubagentProfilePatchV1,
  type AnalytixSubagentProfileV1,
  type AnalytixSubagentsSettingsV1,
  type AnalytixTextToSpeechSettingsV1,
  type AnalytixTokenEconomySettingsV1,
  type AnalytixVisionBridgeSettingsV1,
  type AnalytixVideoGenerationSettingsV1,
  type ImageGenerationProtocol,
  type MusicGenerationProtocol,
  type ModelProviderInputModality,
  type ModelProviderMessagePartSupport,
  type ModelCapabilityProbeResultV1,
  type ModelProviderModelProfilePatchV1,
  type ModelProviderModelProfileV1,
  type ModelProviderProfileV1,
  type ModelProviderReasoningCapabilityV1,
  type ModelProviderSettingsV1,
  type SpeechToTextProtocol,
  type TextToSpeechProtocol,
  type VideoGenerationProtocol
} from './app-settings-types'
import {
  normalizeModelProviderSettings,
  resolveAnalytixRuntimeSettings
} from './app-settings-provider'

const LEGACY_COREAGENT_DATA_DIR = '~/.analytix/coreagent'
const LEGACY_ANALYTIX_DEFAULT_MODEL = 'deepseek-chat'
const LEGACY_LOCAL_HTTP_DEFAULT_PORT = 7878
const APP_SETTINGS_V1_KEYS = new Set([
  'version',
  'locale',
  'theme',
  'uiFontScale',
  'motionPreference',
  'cursorSpotlight',
  'provider',
  'runtime',
  'workspaceRoot',
  'log',
  'notifications',
  'appBehavior',
  'keyboardShortcuts',
  'write',
  'claw',
  'schedule',
  'guiUpdate',
  'codePromptPrefix',
  'disabledSkillIds'
])

type LegacyLocalHttpRuntimeSettingsV1 = {
  binaryPath: string
  port: number
  autoStart: boolean
  baseUrl: string
  runtimeToken: string
  extraCorsOrigins: string[]
  approvalPolicy: ApprovalPolicy
  sandboxMode: SandboxMode
}

type LegacyReasoningEffort = 'low' | 'medium' | 'high' | 'max'
type LegacyReasoningEditMode = 'review' | 'auto' | 'yolo' | 'plan'

type LegacyReasoningRuntimeSettingsV1 = {
  binaryPath: string
  autoStart: boolean
  baseUrl: string
  model: string
  reasoningEffort: LegacyReasoningEffort
  editMode: LegacyReasoningEditMode
}

/**
 * Analytix runtime settings. Mirrors the `analytix serve` CLI
 * options. It is the only active agent settings object the GUI
 * stores after legacy settings have been migrated.
 */
function legacyLocalHttpRuntimeDefaults(port = 7878): LegacyLocalHttpRuntimeSettingsV1 {
  return {
    binaryPath: '',
    port,
    autoStart: true,
    baseUrl: DEFAULT_DEEPSEEK_BASE_URL,
    runtimeToken: '',
    extraCorsOrigins: ['http://localhost:5173', 'http://127.0.0.1:5173'],
    approvalPolicy: DEFAULT_APPROVAL_POLICY,
    sandboxMode: DEFAULT_SANDBOX_MODE
  }
}

function legacyReasoningRuntimeDefaults(): LegacyReasoningRuntimeSettingsV1 {
  return {
    binaryPath: '',
    autoStart: true,
    baseUrl: DEFAULT_DEEPSEEK_BASE_URL,
    model: LEGACY_ANALYTIX_DEFAULT_MODEL,
    reasoningEffort: 'medium',
    editMode: 'auto'
  }
}

export function defaultAnalytixRuntimeSettings(
  port = DEFAULT_ANALYTIX_PORT
): AnalytixRuntimeSettingsV1 {
  return {
    binaryPath: '',
    port,
    autoStart: true,
    baseUrl: '',
    providerId: '',
    endpointFormat: DEFAULT_MODEL_ENDPOINT_FORMAT,
    runtimeToken: '',
    dataDir: DEFAULT_ANALYTIX_DATA_DIR,
    model: DEFAULT_ANALYTIX_MODEL,
    executionPolicyVersion: CURRENT_EXECUTION_POLICY_VERSION,
    approvalPolicy: DEFAULT_APPROVAL_POLICY,
    sandboxMode: DEFAULT_SANDBOX_MODE,
    tokenEconomyMode: false,
    tokenEconomy: defaultAnalytixTokenEconomySettings(),
    insecure: false,
    mcpSearch: defaultAnalytixMcpSearchSettings(),
    storage: defaultAnalytixStorageSettings(),
    contextCompaction: defaultAnalytixContextCompactionSettings(),
    runtimeTuning: defaultAnalytixRuntimeTuningSettings(),
    imageGeneration: defaultAnalytixImageGenerationSettings(),
    speechToText: defaultAnalytixSpeechToTextSettings(),
    textToSpeech: defaultAnalytixTextToSpeechSettings(),
    musicGeneration: defaultAnalytixMusicGenerationSettings(),
    videoGeneration: defaultAnalytixVideoGenerationSettings(),
    modelProfiles: {},
    modelCapabilityProbes: {},
    subagents: defaultAnalytixSubagentsSettings(),
    memoryEnabled: false,
    computerUse: defaultAnalytixComputerUseSettings(),
    browserUse: defaultAnalytixBrowserUseSettings(),
    visionBridge: defaultAnalytixVisionBridgeSettings(),
    quality: defaultAnalytixQualitySettings()
  }
}

export function buildAnalytixRuntimeSettingsKey(settings: AppSettingsV1): string {
  return stableAnalytixSettingsStringify(analytixProductionStartupProjection(settings))
}

/**
 * Settings that can change the production Go process, its config projection,
 * or the derived MCP/skill catalogs assembled before launch. Compatibility
 * fields and Electron-only media features deliberately stay out of this key.
 */
export function analytixProductionStartupProjection(settings: AppSettingsV1): unknown {
  const runtime = resolveAnalytixRuntimeSettings(settings)
  const provider = normalizeModelProviderSettings(settings.provider)
  return {
    runtime: {
      port: runtime.port,
      autoStart: runtime.autoStart,
      baseUrl: runtime.baseUrl,
      providerId: runtime.providerId,
      endpointFormat: runtime.endpointFormat,
      runtimeToken: runtime.runtimeToken,
      dataDir: runtime.dataDir,
      model: runtime.model,
      executionPolicyVersion: runtime.executionPolicyVersion,
      approvalPolicy: runtime.approvalPolicy,
      sandboxMode: runtime.sandboxMode,
      insecure: runtime.insecure,
      mcpSearch: runtime.mcpSearch,
      runtimeTuning: {
        streamIdleTimeoutMs: runtime.runtimeTuning.streamIdleTimeoutMs,
        stepLimits: runtime.runtimeTuning.stepLimits
      },
      subagents: runtime.subagents,
      computerUse: {
        enabled: runtime.computerUse.enabled
      },
      visionBridge: runtime.visionBridge,
      modelCapabilityProbes: runtime.modelCapabilityProbes
    },
    provider: {
      activeProviderId: provider.activeProviderId,
      proxy: provider.proxy,
      providers: provider.providers.map((profile) => ({
        id: profile.id,
        name: profile.name,
        baseUrl: profile.baseUrl,
        endpointFormat: profile.endpointFormat,
        models: profile.models,
        modelProfiles: profile.modelProfiles,
        price: profile.price,
        prices: profile.prices
      }))
    },
    skills: {
      workspaceRoot: settings.workspaceRoot,
      claw: {
        workspaceRoot: settings.claw.im.workspaceRoot,
        channelWorkspaces: settings.claw.channels.map((channel) => channel.workspaceRoot),
        taskWorkspaces: settings.claw.tasks.map((task) => task.workspaceRoot),
        extraDirs: settings.claw.skills.extraDirs,
        disabledDirs: settings.claw.skills.disabledDirs
      },
      schedule: {
        workspaceRoot: settings.schedule.defaultWorkspaceRoot,
        taskWorkspaces: settings.schedule.tasks.map((task) => task.workspaceRoot),
        extraDirs: settings.schedule.skills.extraDirs,
        disabledDirs: settings.schedule.skills.disabledDirs
      }
    }
  }
}

export function analytixRuntimeSettingsEqual(prev: AppSettingsV1, next: AppSettingsV1): boolean {
  return buildAnalytixRuntimeSettingsKey(prev) === buildAnalytixRuntimeSettingsKey(next)
}

function stableAnalytixSettingsStringify(value: unknown): string {
  return JSON.stringify(canonicalAnalytixSettingsValue(value))
}

function canonicalAnalytixSettingsValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalAnalytixSettingsValue)
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    out[key] = canonicalAnalytixSettingsValue((value as Record<string, unknown>)[key])
  }
  return out
}

export function defaultAnalytixQualitySettings(): AnalytixDesignQualitySettingsV1 {
  return {
    enabled: true,
    strictness: 'standard',
    ignoreRules: [],
    ignoreFiles: [],
    maxFindings: 12
  }
}

export function defaultAnalytixComputerUseSettings(): AnalytixComputerUseSettingsV1 {
  return {
    enabled: true,
    mode: 'always',
    maxImageDimension: 1280,
    maxActionsPerTurn: 40,
    allowWhenLocked: true
  }
}

export function defaultAnalytixBrowserUseSettings(): AnalytixBrowserUseSettingsV1 {
  return {
    enabled: true,
    localUrlOpenTarget: 'analytix',
    annotationScreenshotsMode: 'always',
    approvalMode: 'neverAsk',
    fullCdpAccess: true,
    chromeControlEnabled: true,
    sitePermissions: []
  }
}

export function defaultAnalytixVisionBridgeSettings(): AnalytixVisionBridgeSettingsV1 {
  return {
    enabled: false,
    mode: 'auto',
    providerId: 'xiaomi',
    model: 'mimo-v2.5',
    baseUrl: '',
    endpointFormat: DEFAULT_MODEL_ENDPOINT_FORMAT,
    maxImageDimension: 1280,
    maxImageBytes: 1_500_000,
    maxScreenshotsPerTurn: 4,
    observationCacheTtlMs: 120_000,
    injectPolicy: 'observation_text',
    fallbackWhenPrimaryImageUnsupported: true
  }
}

export function defaultAnalytixImageGenerationSettings(): AnalytixImageGenerationSettingsV1 {
  return {
    enabled: false,
    providerId: '',
    protocol: DEFAULT_IMAGE_GENERATION_PROTOCOL,
    baseUrl: '',
    model: '',
    defaultSize: '',
    timeoutMs: 180_000
  }
}

export function defaultAnalytixSpeechToTextSettings(): AnalytixSpeechToTextSettingsV1 {
  return {
    enabled: false,
    providerId: '',
    protocol: DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
    baseUrl: '',
    model: '',
    language: '',
    timeoutMs: 60_000
  }
}

export function defaultAnalytixTextToSpeechSettings(): AnalytixTextToSpeechSettingsV1 {
  return {
    enabled: false,
    providerId: '',
    protocol: DEFAULT_TEXT_TO_SPEECH_PROTOCOL,
    baseUrl: '',
    model: '',
    voice: '',
    format: 'mp3',
    timeoutMs: 120_000
  }
}

export function defaultAnalytixMusicGenerationSettings(): AnalytixMusicGenerationSettingsV1 {
  return {
    enabled: false,
    providerId: '',
    protocol: DEFAULT_MUSIC_GENERATION_PROTOCOL,
    baseUrl: '',
    model: '',
    format: 'mp3',
    timeoutMs: 300_000
  }
}

export function defaultAnalytixVideoGenerationSettings(): AnalytixVideoGenerationSettingsV1 {
  return {
    enabled: false,
    providerId: '',
    protocol: DEFAULT_VIDEO_GENERATION_PROTOCOL,
    baseUrl: '',
    model: '',
    defaultDuration: 6,
    defaultResolution: '1080P',
    timeoutMs: 900_000,
    pollIntervalMs: 10_000
  }
}

export function defaultAnalytixMcpSearchSettings(): AnalytixMcpSearchSettingsV1 {
  return {
    enabled: false,
    mode: 'auto',
    autoThresholdToolCount: 24,
    topKDefault: 5,
    topKMax: 10,
    minScore: 0.15
  }
}

export function defaultAnalytixTokenEconomySettings(): AnalytixTokenEconomySettingsV1 {
  return {
    enabled: false,
    compressToolDescriptions: true,
    compressToolResults: true,
    conciseResponses: true,
    historyHygiene: defaultAnalytixHistoryHygieneSettings()
  }
}

export function defaultAnalytixHistoryHygieneSettings(): AnalytixHistoryHygieneSettingsV1 {
  return {
    maxToolResultLines: 320,
    maxToolResultBytes: 32 * 1024,
    maxToolResultTokens: 8_000,
    maxToolArgumentStringBytes: 8 * 1024,
    maxToolArgumentStringTokens: 2_000,
    maxArrayItems: 80,
    maxCumulativeToolResultTokens: 120_000,
    keepRecentToolResults: 4
  }
}

export function defaultAnalytixStorageSettings(): AnalytixStorageSettingsV1 {
  return {
    backend: 'hybrid',
    sqlitePath: ''
  }
}

export function defaultAnalytixContextCompactionSettings(): AnalytixContextCompactionSettingsV1 {
  return {
    defaultSoftThreshold: 96_000,
    defaultHardThreshold: 108_800,
    summaryMode: 'model',
    summaryTimeoutMs: 15_000,
    summaryMaxTokens: 1_200,
    summaryInputMaxBytes: 96 * 1024
  }
}

export function defaultAnalytixRuntimeTuningSettings(): AnalytixRuntimeTuningSettingsV1 {
  return {
    streamIdleTimeoutMs: 45_000,
    stepLimits: {
      defaultMaxModelSteps: 64,
      userGlobalMaxModelSteps: 0,
      plannerMaxModelSteps: 0,
      headlessMaxModelSteps: 0
    },
    toolStorm: {
      enabled: true,
      windowSize: 8,
      threshold: 3
    },
    toolArgumentRepair: {
      maxStringBytes: 512 * 1024
    }
  }
}

export function defaultAnalytixSubagentsSettings(): AnalytixSubagentsSettingsV1 {
  return {
    enabled: true,
    maxParallel: 8,
    maxChildRuns: 64,
    defaultToolPolicy: 'readOnly',
    defaultProfile: '',
    profiles: {}
  }
}

export function getAnalytixRuntimeSettings(
  settings: AppSettingsV1
): AnalytixRuntimeSettingsV1 {
  const raw = (settings as { runtime?: Partial<AnalytixRuntimeSettingsV1> }).runtime
  return mergeAnalytixRuntimeSettings(defaultAnalytixRuntimeSettings(), {
    ...raw,
    ...normalizeAnalytixExecutionPolicy(raw)
  })
}

type ExecutionPolicyInput = {
  executionPolicyVersion?: unknown
  approvalPolicy?: unknown
  sandboxMode?: unknown
}

const LEGACY_DEFAULT_APPROVAL_POLICY: ApprovalPolicy = 'auto'
const LEGACY_DEFAULT_SANDBOX_MODE: SandboxMode = 'danger-full-access'

function isApprovalPolicy(value: unknown): value is ApprovalPolicy {
  return APPROVAL_POLICIES.includes(value as ApprovalPolicy)
}

function isSandboxMode(value: unknown): value is SandboxMode {
  return SANDBOX_MODES.includes(value as SandboxMode)
}

/**
 * Normalizes the independently versioned execution-policy pair. Unversioned
 * legacy defaults migrate to the safer defaults, while other valid explicit
 * choices are preserved and marked current.
 */
export function normalizeAnalytixExecutionPolicy(
  input: ExecutionPolicyInput | null | undefined
): Pick<AnalytixRuntimeSettingsV1, 'executionPolicyVersion' | 'approvalPolicy' | 'sandboxMode'> {
  const approvalPolicy = input?.approvalPolicy
  const sandboxMode = input?.sandboxMode
  const rawVersion = input?.executionPolicyVersion
  const markerAbsent = rawVersion === undefined
  const markerValid = typeof rawVersion === 'number' && Number.isSafeInteger(rawVersion) && rawVersion > 0
  if (!isApprovalPolicy(approvalPolicy) || !isSandboxMode(sandboxMode) || (!markerAbsent && !markerValid)) {
    return {
      executionPolicyVersion: CURRENT_EXECUTION_POLICY_VERSION,
      approvalPolicy: DEFAULT_APPROVAL_POLICY,
      sandboxMode: DEFAULT_SANDBOX_MODE
    }
  }
  const isUnversionedLegacyDefault =
    markerAbsent &&
    approvalPolicy === LEGACY_DEFAULT_APPROVAL_POLICY &&
    sandboxMode === LEGACY_DEFAULT_SANDBOX_MODE
  return {
    executionPolicyVersion: markerValid ? rawVersion : CURRENT_EXECUTION_POLICY_VERSION,
    approvalPolicy: isUnversionedLegacyDefault ? DEFAULT_APPROVAL_POLICY : approvalPolicy,
    sandboxMode: isUnversionedLegacyDefault ? DEFAULT_SANDBOX_MODE : sandboxMode
  }
}

function projectAnalytixRuntimeFields<T extends object | undefined>(input: T): T {
  if (!input) return input
  const canonical = defaultAnalytixRuntimeSettings() as unknown as Record<string, unknown>
  return Object.fromEntries(
    Object.entries(input).filter(([key]) => Object.hasOwn(canonical, key))
  ) as T
}

export function mergeAnalytixRuntimeSettings(
  current: AnalytixRuntimeSettingsV1,
  patch: AnalytixRuntimeSettingsPatchV1 | undefined
): AnalytixRuntimeSettingsV1 {
  const currentWithoutRejectedFields = projectAnalytixRuntimeFields(current)
  const patchWithoutRejectedFields = projectAnalytixRuntimeFields(patch)
  const currentMcpSearch = normalizeAnalytixMcpSearchSettings(current.mcpSearch)
  const nextMcpSearch = normalizeAnalytixMcpSearchSettings({
    ...currentMcpSearch,
    ...(patch?.mcpSearch ?? {})
  })
  const currentTokenEconomy = normalizeAnalytixTokenEconomySettings(
    current.tokenEconomy,
    current.tokenEconomyMode
  )
  const patchedTokenEconomy = normalizeAnalytixTokenEconomySettings({
    ...currentTokenEconomy,
    ...(patch?.tokenEconomy ?? {}),
    historyHygiene: {
      ...currentTokenEconomy.historyHygiene,
      ...(patch?.tokenEconomy?.historyHygiene ?? {})
    }
  }, currentTokenEconomy.enabled)
  const tokenEconomyEnabled = typeof patch?.tokenEconomy?.enabled === 'boolean'
    ? patch.tokenEconomy.enabled
    : typeof patch?.tokenEconomyMode === 'boolean'
      ? patch.tokenEconomyMode
      : patchedTokenEconomy.enabled
  const nextTokenEconomy = {
    ...patchedTokenEconomy,
    enabled: tokenEconomyEnabled
  }
  const currentStorage = normalizeAnalytixStorageSettings(current.storage)
  const nextStorage = normalizeAnalytixStorageSettings({
    ...currentStorage,
    ...(patch?.storage ?? {})
  })
  const currentContextCompaction = normalizeAnalytixContextCompactionSettings(current.contextCompaction)
  const contextCompactionPatch = patch?.contextCompaction ?? {}
  const nextContextCompactionInput = {
    ...currentContextCompaction,
    ...contextCompactionPatch
  }
  if (
    contextCompactionPatch.defaultSoftThreshold !== undefined &&
    contextCompactionPatch.defaultHardThreshold === undefined
  ) {
    nextContextCompactionInput.defaultHardThreshold = contextCompactionPatch.defaultSoftThreshold
  }
  const nextContextCompaction = normalizeAnalytixContextCompactionSettings(nextContextCompactionInput)
  const currentImageGeneration = normalizeAnalytixImageGenerationSettings(current.imageGeneration)
  const nextImageGeneration = normalizeAnalytixImageGenerationSettings({
    ...currentImageGeneration,
    ...(patch?.imageGeneration ?? {})
  })
  const currentSpeechToText = normalizeAnalytixSpeechToTextSettings(current.speechToText)
  const nextSpeechToText = normalizeAnalytixSpeechToTextSettings({
    ...currentSpeechToText,
    ...(patch?.speechToText ?? {})
  })
  const currentTextToSpeech = normalizeAnalytixTextToSpeechSettings(current.textToSpeech)
  const nextTextToSpeech = normalizeAnalytixTextToSpeechSettings({
    ...currentTextToSpeech,
    ...(patch?.textToSpeech ?? {})
  })
  const currentMusicGeneration = normalizeAnalytixMusicGenerationSettings(current.musicGeneration)
  const nextMusicGeneration = normalizeAnalytixMusicGenerationSettings({
    ...currentMusicGeneration,
    ...(patch?.musicGeneration ?? {})
  })
  const currentVideoGeneration = normalizeAnalytixVideoGenerationSettings(current.videoGeneration)
  const nextVideoGeneration = normalizeAnalytixVideoGenerationSettings({
    ...currentVideoGeneration,
    ...(patch?.videoGeneration ?? {})
  })
  const currentComputerUse = normalizeAnalytixComputerUseSettings(current.computerUse)
  const nextComputerUse = normalizeAnalytixComputerUseSettings({
    ...currentComputerUse,
    ...(patch?.computerUse ?? {})
  })
  const currentBrowserUse = normalizeAnalytixBrowserUseSettings(current.browserUse)
  const nextBrowserUse = normalizeAnalytixBrowserUseSettings({
    ...currentBrowserUse,
    ...(patch?.browserUse ?? {})
  })
  const currentVisionBridge = normalizeAnalytixVisionBridgeSettings(current.visionBridge)
  const nextVisionBridge = normalizeAnalytixVisionBridgeSettings({
    ...currentVisionBridge,
    ...(patch?.visionBridge ?? {})
  })
  const currentQuality = normalizeAnalytixQualitySettings(current.quality)
  const nextQuality = normalizeAnalytixQualitySettings({
    ...currentQuality,
    ...(patch?.quality ?? {})
  })
  const currentRuntimeTuning = normalizeAnalytixRuntimeTuningSettings(current.runtimeTuning)
  const nextRuntimeTuning = normalizeAnalytixRuntimeTuningSettings({
    ...currentRuntimeTuning,
    ...(patch?.runtimeTuning
      ? {
          ...(patch.runtimeTuning.streamIdleTimeoutMs !== undefined
            ? { streamIdleTimeoutMs: patch.runtimeTuning.streamIdleTimeoutMs }
            : {}),
          stepLimits: {
            ...currentRuntimeTuning.stepLimits,
            ...(patch.runtimeTuning.stepLimits ?? {})
          },
          toolStorm: {
            ...currentRuntimeTuning.toolStorm,
            ...(patch.runtimeTuning.toolStorm ?? {})
          },
          toolArgumentRepair: {
            ...currentRuntimeTuning.toolArgumentRepair,
            ...(patch.runtimeTuning.toolArgumentRepair ?? {})
          }
        }
      : {})
  })
  const nextModelProfiles = normalizeAnalytixModelProfiles(current.modelProfiles, patch?.modelProfiles)
  const nextModelCapabilityProbes = normalizeModelCapabilityProbes(
    current.modelCapabilityProbes,
    patch?.modelCapabilityProbes
  )
  const currentSubagents = normalizeAnalytixSubagentsSettings(current.subagents)
  const nextSubagents = normalizeAnalytixSubagentsSettings({
    ...currentSubagents,
    ...(patch?.subagents ?? {}),
    profiles: normalizeAnalytixSubagentProfiles(currentSubagents.profiles, patch?.subagents?.profiles)
  })
  const nextExecutionPolicy = normalizeAnalytixExecutionPolicy({
    executionPolicyVersion: patch?.executionPolicyVersion ?? current.executionPolicyVersion,
    approvalPolicy: patch?.approvalPolicy ?? current.approvalPolicy,
    sandboxMode: patch?.sandboxMode ?? current.sandboxMode
  })
  return {
    ...currentWithoutRejectedFields,
    ...(patchWithoutRejectedFields ?? {}),
    ...nextExecutionPolicy,
    tokenEconomyMode: nextTokenEconomy.enabled,
    tokenEconomy: nextTokenEconomy,
    mcpSearch: nextMcpSearch,
    storage: nextStorage,
    contextCompaction: nextContextCompaction,
    runtimeTuning: nextRuntimeTuning,
    imageGeneration: nextImageGeneration,
    speechToText: nextSpeechToText,
    textToSpeech: nextTextToSpeech,
    musicGeneration: nextMusicGeneration,
    videoGeneration: nextVideoGeneration,
    modelProfiles: nextModelProfiles,
    modelCapabilityProbes: nextModelCapabilityProbes,
    subagents: nextSubagents,
    memoryEnabled: patch?.memoryEnabled ?? current.memoryEnabled ?? false,
    computerUse: nextComputerUse,
    browserUse: nextBrowserUse,
    visionBridge: nextVisionBridge,
    quality: nextQuality
  }
}

const ANALYTIX_DESIGN_QUALITY_STRICTNESS: readonly AnalytixDesignQualityStrictness[] = [
  'relaxed',
  'standard',
  'strict'
]

function normalizeAnalytixQualitySettings(
  input: Partial<AnalytixDesignQualitySettingsV1> | undefined
): AnalytixDesignQualitySettingsV1 {
  const defaults = defaultAnalytixQualitySettings()
  const strictness =
    input?.strictness && ANALYTIX_DESIGN_QUALITY_STRICTNESS.includes(input.strictness)
      ? input.strictness
      : defaults.strictness
  const sanitizeList = (list: unknown): string[] =>
    Array.isArray(list)
      ? list.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
      : []
  return {
    enabled: input?.enabled !== false,
    strictness,
    ignoreRules: sanitizeList(input?.ignoreRules),
    ignoreFiles: sanitizeList(input?.ignoreFiles),
    maxFindings: boundedPositiveInt(input?.maxFindings, defaults.maxFindings, 100)
  }
}

function normalizeAnalytixComputerUseSettings(
  input: Partial<AnalytixComputerUseSettingsV1> | undefined
): AnalytixComputerUseSettingsV1 {
  const defaults = defaultAnalytixComputerUseSettings()
  const mode = input?.mode === 'always' || input?.mode === 'off' || input?.mode === 'auto'
    ? input.mode
    : defaults.mode
  return {
    enabled: input?.enabled !== false,
    mode,
    maxImageDimension: boundedPositiveInt(input?.maxImageDimension, defaults.maxImageDimension, 4096),
    maxActionsPerTurn: boundedPositiveInt(input?.maxActionsPerTurn, defaults.maxActionsPerTurn, 1000),
    allowWhenLocked: input?.allowWhenLocked !== false
  }
}

type AnalytixBrowserUseSettingsInput = Partial<Omit<AnalytixBrowserUseSettingsV1, 'sitePermissions'>> & {
  sitePermissions?: Array<Partial<AnalytixBrowserSitePermissionV1> | null>
}

function normalizeAnalytixBrowserUseSettings(
  input: AnalytixBrowserUseSettingsInput | undefined
): AnalytixBrowserUseSettingsV1 {
  const defaults = defaultAnalytixBrowserUseSettings()
  const localUrlOpenTarget = input?.localUrlOpenTarget === 'system' || input?.localUrlOpenTarget === 'analytix'
    ? input.localUrlOpenTarget
    : defaults.localUrlOpenTarget
  const annotationScreenshotsMode =
    input?.annotationScreenshotsMode === 'always' ||
    input?.annotationScreenshotsMode === 'necessary' ||
    input?.annotationScreenshotsMode === 'off'
      ? input.annotationScreenshotsMode
      : defaults.annotationScreenshotsMode
  const approvalMode = normalizeBrowserApprovalMode(input?.approvalMode, defaults.approvalMode)
  const rawSitePermissions = Array.isArray(input?.sitePermissions) ? input.sitePermissions : []
  return {
    enabled: input?.enabled !== false,
    localUrlOpenTarget,
    annotationScreenshotsMode,
    approvalMode,
    fullCdpAccess: input?.fullCdpAccess !== false,
    chromeControlEnabled: input?.chromeControlEnabled !== false,
    sitePermissions: rawSitePermissions
      .map(normalizeBrowserSitePermission)
      .filter((item): item is AnalytixBrowserSitePermissionV1 => Boolean(item))
      .slice(0, 100)
  }
}

function normalizeBrowserApprovalMode(
  value: unknown,
  fallback: AnalytixBrowserApprovalMode
): AnalytixBrowserApprovalMode {
  return value === 'alwaysAsk' || value === 'neverAsk' ? value : fallback
}

function normalizeBrowserSitePermission(
  value: Partial<AnalytixBrowserSitePermissionV1> | null | undefined
): AnalytixBrowserSitePermissionV1 | null {
  if (!value || typeof value !== 'object') return null
  const origin = typeof value.origin === 'string' ? value.origin.trim().slice(0, 512) : ''
  if (!origin) return null
  return {
    origin,
    approvalMode: normalizeBrowserApprovalMode(value.approvalMode, 'neverAsk'),
    downloadApprovalMode: normalizeBrowserApprovalMode(value.downloadApprovalMode, 'neverAsk'),
    uploadApprovalMode: normalizeBrowserApprovalMode(value.uploadApprovalMode, 'neverAsk'),
    fullCdpAccess: value.fullCdpAccess !== false
  }
}

const MODEL_CAPABILITY_PROBE_STATUSES = new Set([
  'unknown',
  'supported',
  'unsupported',
  'semantic_failed',
  'auth_failed',
  'http_failed',
  'timeout',
  'failed',
  'stale'
])

function normalizeModelCapabilityProbes(
  current: Record<string, ModelCapabilityProbeResultV1> | undefined,
  patch: Record<string, ModelCapabilityProbeResultV1 | null> | undefined
): Record<string, ModelCapabilityProbeResultV1> {
  const out: Record<string, ModelCapabilityProbeResultV1> = {}
  for (const [key, value] of Object.entries(current ?? {})) {
    const normalized = normalizeModelCapabilityProbe(value)
    if (normalized) out[key] = normalized
  }
  for (const [key, value] of Object.entries(patch ?? {})) {
    if (value === null) {
      delete out[key]
      continue
    }
    const normalized = normalizeModelCapabilityProbe(value)
    if (normalized) out[key] = normalized
  }
  return out
}

function normalizeModelCapabilityProbe(value: ModelCapabilityProbeResultV1 | undefined): ModelCapabilityProbeResultV1 | null {
  if (!value || typeof value !== 'object') return null
  const key = typeof value.key === 'string' ? value.key.trim() : ''
  const providerId = typeof value.providerId === 'string' ? value.providerId.trim() : ''
  const model = typeof value.model === 'string' ? value.model.trim() : ''
  const sanitizedBaseUrl = typeof value.sanitizedBaseUrl === 'string' ? value.sanitizedBaseUrl.trim() : ''
  const probedAt = typeof value.probedAt === 'string' ? value.probedAt : ''
  const staleAfter = typeof value.staleAfter === 'string' ? value.staleAfter : ''
  if (!key || !providerId || !model || !sanitizedBaseUrl || !probedAt || !staleAfter) return null
  const status = normalizeProbeStatus(value.status)
  return {
    key,
    providerId,
    model,
    endpointFormat: normalizeModelEndpointFormat(value.endpointFormat),
    sanitizedBaseUrl,
    ...(typeof value.requestUrl === 'string' && value.requestUrl.trim() ? { requestUrl: value.requestUrl.trim() } : {}),
    probedAt,
    staleAfter,
    imageInput: normalizeProbeStatus(value.imageInput),
    toolCalling: normalizeProbeStatus(value.toolCalling),
    toolResultImage: normalizeProbeStatus(value.toolResultImage),
    status,
    ...(typeof value.httpStatus === 'number' ? { httpStatus: value.httpStatus } : {}),
    ...(typeof value.errorSummary === 'string' && value.errorSummary.trim() ? { errorSummary: value.errorSummary.trim().slice(0, 1000) } : {})
  }
}

function normalizeProbeStatus(value: unknown): ModelCapabilityProbeResultV1['status'] {
  return typeof value === 'string' && MODEL_CAPABILITY_PROBE_STATUSES.has(value)
    ? value as ModelCapabilityProbeResultV1['status']
    : 'unknown'
}

function normalizeAnalytixVisionBridgeSettings(
  input: Partial<AnalytixVisionBridgeSettingsV1> | undefined
): AnalytixVisionBridgeSettingsV1 {
  const defaults = defaultAnalytixVisionBridgeSettings()
  const mode = input?.mode === 'always' || input?.mode === 'off' || input?.mode === 'auto'
    ? input.mode
    : defaults.mode
  const injectPolicy = input?.injectPolicy === 'observation_text'
    ? input.injectPolicy
    : defaults.injectPolicy
  return {
    enabled: input?.enabled === true,
    mode,
    providerId: typeof input?.providerId === 'string' ? input.providerId.trim() : defaults.providerId,
    model: typeof input?.model === 'string' && input.model.trim() ? input.model.trim() : defaults.model,
    baseUrl: typeof input?.baseUrl === 'string' ? input.baseUrl.trim() : defaults.baseUrl,
    endpointFormat: normalizeModelEndpointFormat(input?.endpointFormat),
    maxImageDimension: boundedPositiveInt(input?.maxImageDimension, defaults.maxImageDimension, 4096),
    maxImageBytes: boundedPositiveInt(input?.maxImageBytes, defaults.maxImageBytes, 8 * 1024 * 1024),
    maxScreenshotsPerTurn: boundedPositiveInt(input?.maxScreenshotsPerTurn, defaults.maxScreenshotsPerTurn, 16),
    observationCacheTtlMs: boundedPositiveInt(input?.observationCacheTtlMs, defaults.observationCacheTtlMs, 3_600_000),
    injectPolicy,
    fallbackWhenPrimaryImageUnsupported: input?.fallbackWhenPrimaryImageUnsupported !== false
  }
}

function normalizeAnalytixImageGenerationSettings(
  input: Partial<AnalytixImageGenerationSettingsV1> | undefined
): AnalytixImageGenerationSettingsV1 {
  const defaults = defaultAnalytixImageGenerationSettings()
  const defaultSize = typeof input?.defaultSize === 'string' ? input.defaultSize.trim() : ''
  return {
    enabled: input?.enabled === true,
    providerId: typeof input?.providerId === 'string' ? input.providerId.trim() : defaults.providerId,
    protocol: normalizeAnalytixImageGenerationProtocol(input?.protocol),
    baseUrl: typeof input?.baseUrl === 'string' ? input.baseUrl.trim() : defaults.baseUrl,
    model: typeof input?.model === 'string' ? input.model.trim() : defaults.model,
    defaultSize: /^(auto|\d+x\d+)$/.test(defaultSize) ? defaultSize : '',
    timeoutMs: boundedPositiveInt(input?.timeoutMs, defaults.timeoutMs, 600_000)
  }
}

function normalizeAnalytixImageGenerationProtocol(value: unknown): ImageGenerationProtocol {
  return value === 'minimax-image' ? 'minimax-image' : DEFAULT_IMAGE_GENERATION_PROTOCOL
}

function normalizeAnalytixSpeechToTextSettings(
  input: Partial<AnalytixSpeechToTextSettingsV1> | undefined
): AnalytixSpeechToTextSettingsV1 {
  const defaults = defaultAnalytixSpeechToTextSettings()
  return {
    enabled: input?.enabled === true,
    providerId: typeof input?.providerId === 'string' ? input.providerId.trim() : defaults.providerId,
    protocol: normalizeAnalytixSpeechToTextProtocol(input?.protocol),
    baseUrl: typeof input?.baseUrl === 'string' ? input.baseUrl.trim() : defaults.baseUrl,
    model: typeof input?.model === 'string' ? input.model.trim() : defaults.model,
    language: typeof input?.language === 'string' ? input.language.trim().toLowerCase().slice(0, 16) : defaults.language,
    timeoutMs: boundedPositiveInt(input?.timeoutMs, defaults.timeoutMs, 600_000)
  }
}

function normalizeAnalytixSpeechToTextProtocol(value: unknown): SpeechToTextProtocol {
  return value === 'mimo-asr' ? 'mimo-asr' : DEFAULT_SPEECH_TO_TEXT_PROTOCOL
}

function normalizeAnalytixTextToSpeechSettings(
  input: Partial<AnalytixTextToSpeechSettingsV1> | undefined
): AnalytixTextToSpeechSettingsV1 {
  const defaults = defaultAnalytixTextToSpeechSettings()
  return {
    enabled: input?.enabled === true,
    providerId: typeof input?.providerId === 'string' ? input.providerId.trim() : defaults.providerId,
    protocol: normalizeAnalytixTextToSpeechProtocol(input?.protocol),
    baseUrl: typeof input?.baseUrl === 'string' ? input.baseUrl.trim() : defaults.baseUrl,
    model: typeof input?.model === 'string' ? input.model.trim() : defaults.model,
    voice: typeof input?.voice === 'string' ? input.voice.trim().slice(0, 128) : defaults.voice,
    format: normalizeAudioFormat(input?.format, defaults.format),
    timeoutMs: boundedPositiveInt(input?.timeoutMs, defaults.timeoutMs, 600_000)
  }
}

function normalizeAnalytixTextToSpeechProtocol(value: unknown): TextToSpeechProtocol {
  return value === 'minimax-t2a' || value === 'mimo-tts'
    ? value
    : DEFAULT_TEXT_TO_SPEECH_PROTOCOL
}

function normalizeAnalytixMusicGenerationSettings(
  input: Partial<AnalytixMusicGenerationSettingsV1> | undefined
): AnalytixMusicGenerationSettingsV1 {
  const defaults = defaultAnalytixMusicGenerationSettings()
  return {
    enabled: input?.enabled === true,
    providerId: typeof input?.providerId === 'string' ? input.providerId.trim() : defaults.providerId,
    protocol: normalizeAnalytixMusicGenerationProtocol(input?.protocol),
    baseUrl: typeof input?.baseUrl === 'string' ? input.baseUrl.trim() : defaults.baseUrl,
    model: typeof input?.model === 'string' ? input.model.trim() : defaults.model,
    format: normalizeAudioFormat(input?.format, defaults.format),
    timeoutMs: boundedPositiveInt(input?.timeoutMs, defaults.timeoutMs, 900_000)
  }
}

function normalizeAnalytixMusicGenerationProtocol(value: unknown): MusicGenerationProtocol {
  return value === 'minimax-music' ? 'minimax-music' : DEFAULT_MUSIC_GENERATION_PROTOCOL
}

function normalizeAnalytixVideoGenerationSettings(
  input: Partial<AnalytixVideoGenerationSettingsV1> | undefined
): AnalytixVideoGenerationSettingsV1 {
  const defaults = defaultAnalytixVideoGenerationSettings()
  return {
    enabled: input?.enabled === true,
    providerId: typeof input?.providerId === 'string' ? input.providerId.trim() : defaults.providerId,
    protocol: normalizeAnalytixVideoGenerationProtocol(input?.protocol),
    baseUrl: typeof input?.baseUrl === 'string' ? input.baseUrl.trim() : defaults.baseUrl,
    model: typeof input?.model === 'string' ? input.model.trim() : defaults.model,
    defaultDuration: boundedPositiveInt(input?.defaultDuration, defaults.defaultDuration, 60),
    defaultResolution: typeof input?.defaultResolution === 'string' && input.defaultResolution.trim()
      ? input.defaultResolution.trim().slice(0, 32)
      : defaults.defaultResolution,
    timeoutMs: boundedPositiveInt(input?.timeoutMs, defaults.timeoutMs, 1_800_000),
    pollIntervalMs: boundedPositiveInt(input?.pollIntervalMs, defaults.pollIntervalMs, 60_000)
  }
}

function normalizeAnalytixVideoGenerationProtocol(value: unknown): VideoGenerationProtocol {
  return value === 'minimax-video' ? 'minimax-video' : DEFAULT_VIDEO_GENERATION_PROTOCOL
}

function normalizeAudioFormat(value: unknown, fallback: string): string {
  if (typeof value !== 'string') return fallback
  const normalized = value.trim().toLowerCase()
  return /^(mp3|wav|flac|pcm16)$/.test(normalized) ? normalized : fallback
}

function normalizeAnalytixTokenEconomySettings(
  input: Partial<AnalytixTokenEconomySettingsV1> | undefined,
  enabledFallback = false
): AnalytixTokenEconomySettingsV1 {
  return {
    enabled: typeof input?.enabled === 'boolean' ? input.enabled : enabledFallback,
    compressToolDescriptions: input?.compressToolDescriptions !== false,
    compressToolResults: input?.compressToolResults !== false,
    conciseResponses: input?.conciseResponses !== false,
    historyHygiene: normalizeAnalytixHistoryHygieneSettings(input?.historyHygiene)
  }
}

function normalizeAnalytixHistoryHygieneSettings(
  input: Partial<AnalytixHistoryHygieneSettingsV1> | undefined
): AnalytixHistoryHygieneSettingsV1 {
  const defaults = defaultAnalytixHistoryHygieneSettings()
  return {
    maxToolResultLines: boundedPositiveInt(input?.maxToolResultLines, defaults.maxToolResultLines, 100_000),
    maxToolResultBytes: boundedPositiveInt(input?.maxToolResultBytes, defaults.maxToolResultBytes, 8 * 1024 * 1024),
    maxToolResultTokens: boundedPositiveInt(input?.maxToolResultTokens, defaults.maxToolResultTokens, 256_000),
    maxToolArgumentStringBytes: boundedPositiveInt(
      input?.maxToolArgumentStringBytes,
      defaults.maxToolArgumentStringBytes,
      8 * 1024 * 1024
    ),
    maxToolArgumentStringTokens: boundedPositiveInt(
      input?.maxToolArgumentStringTokens,
      defaults.maxToolArgumentStringTokens,
      64_000
    ),
    maxArrayItems: boundedPositiveInt(input?.maxArrayItems, defaults.maxArrayItems, 10_000),
    maxCumulativeToolResultTokens: boundedNonNegativeInt(
      input?.maxCumulativeToolResultTokens,
      defaults.maxCumulativeToolResultTokens,
      2_000_000
    ),
    keepRecentToolResults: boundedNonNegativeInt(
      input?.keepRecentToolResults,
      defaults.keepRecentToolResults,
      10_000
    )
  }
}

function normalizeAnalytixMcpSearchSettings(
  input: Partial<AnalytixMcpSearchSettingsV1> | undefined
): AnalytixMcpSearchSettingsV1 {
  const defaults = defaultAnalytixMcpSearchSettings()
  const topKMax = positiveInt(input?.topKMax, defaults.topKMax)
  const topKDefault = Math.min(positiveInt(input?.topKDefault, defaults.topKDefault), topKMax)
  return {
    enabled: input?.enabled === true,
    mode: input?.mode === 'direct' || input?.mode === 'search' || input?.mode === 'auto'
      ? input.mode
      : defaults.mode,
    autoThresholdToolCount: positiveInt(input?.autoThresholdToolCount, defaults.autoThresholdToolCount),
    topKDefault,
    topKMax,
    minScore: nonNegativeNumber(input?.minScore, defaults.minScore)
  }
}

function positiveInt(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? Math.floor(value)
    : fallback
}

function nonNegativeNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : fallback
}

function boundedPositiveInt(value: unknown, fallback: number, max = Number.MAX_SAFE_INTEGER): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return fallback
  return Math.min(Math.floor(value), max)
}

/** Like {@link boundedPositiveInt} but accepts `0` (e.g. "disabled"). */
function boundedNonNegativeInt(value: unknown, fallback: number, max = Number.MAX_SAFE_INTEGER): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return fallback
  return Math.min(Math.floor(value), max)
}

function normalizeAnalytixStorageSettings(
  input: Partial<AnalytixStorageSettingsV1> | undefined
): AnalytixStorageSettingsV1 {
  const defaults = defaultAnalytixStorageSettings()
  return {
    backend: input?.backend === 'file' || input?.backend === 'hybrid'
      ? input.backend
      : defaults.backend,
    sqlitePath: typeof input?.sqlitePath === 'string' ? input.sqlitePath.trim() : defaults.sqlitePath
  }
}

function normalizeAnalytixContextCompactionSettings(
  input: Partial<AnalytixContextCompactionSettingsV1> | undefined
): AnalytixContextCompactionSettingsV1 {
  const defaults = defaultAnalytixContextCompactionSettings()
  const defaultSoftThreshold = boundedPositiveInt(input?.defaultSoftThreshold, defaults.defaultSoftThreshold)
  const requestedHardThreshold = boundedPositiveInt(input?.defaultHardThreshold, defaults.defaultHardThreshold)
  return {
    defaultSoftThreshold,
    defaultHardThreshold: Math.max(defaultSoftThreshold, requestedHardThreshold),
    summaryMode: input?.summaryMode === 'model' || input?.summaryMode === 'heuristic'
      ? input.summaryMode
      : defaults.summaryMode,
    summaryTimeoutMs: boundedPositiveInt(input?.summaryTimeoutMs, defaults.summaryTimeoutMs, 120_000),
    summaryMaxTokens: boundedPositiveInt(input?.summaryMaxTokens, defaults.summaryMaxTokens, 16_000),
    summaryInputMaxBytes: boundedPositiveInt(input?.summaryInputMaxBytes, defaults.summaryInputMaxBytes, 8 * 1024 * 1024)
  }
}

function normalizeAnalytixRuntimeTuningSettings(
  input: Partial<AnalytixRuntimeTuningSettingsV1> | undefined
): AnalytixRuntimeTuningSettingsV1 {
  const defaults = defaultAnalytixRuntimeTuningSettings()
  return {
    streamIdleTimeoutMs: boundedNonNegativeInt(
      input?.streamIdleTimeoutMs,
      defaults.streamIdleTimeoutMs,
      3_600_000
    ),
    stepLimits: {
      defaultMaxModelSteps: boundedNonNegativeInt(
        input?.stepLimits?.defaultMaxModelSteps,
        defaults.stepLimits.defaultMaxModelSteps,
        10_000
      ),
      userGlobalMaxModelSteps: boundedNonNegativeInt(
        input?.stepLimits?.userGlobalMaxModelSteps,
        defaults.stepLimits.userGlobalMaxModelSteps,
        10_000
      ),
      plannerMaxModelSteps: boundedNonNegativeInt(
        input?.stepLimits?.plannerMaxModelSteps,
        defaults.stepLimits.plannerMaxModelSteps,
        10_000
      ),
      headlessMaxModelSteps: boundedNonNegativeInt(
        input?.stepLimits?.headlessMaxModelSteps,
        defaults.stepLimits.headlessMaxModelSteps,
        10_000
      )
    },
    toolStorm: {
      enabled: input?.toolStorm?.enabled !== false,
      windowSize: boundedPositiveInt(input?.toolStorm?.windowSize, defaults.toolStorm.windowSize, 128),
      threshold: Math.max(2, boundedPositiveInt(input?.toolStorm?.threshold, defaults.toolStorm.threshold, 128))
    },
    toolArgumentRepair: {
      maxStringBytes: boundedPositiveInt(
        input?.toolArgumentRepair?.maxStringBytes,
        defaults.toolArgumentRepair.maxStringBytes,
        16 * 1024 * 1024
      )
    }
  }
}

function normalizeAnalytixModelProfiles(
  current: Record<string, ModelProviderModelProfileV1> | undefined,
  patch: Record<string, ModelProviderModelProfilePatchV1 | null> | undefined
): Record<string, ModelProviderModelProfileV1> {
  const profiles: Record<string, ModelProviderModelProfileV1> = {}
  for (const [rawModelId, rawProfile] of Object.entries(current ?? {})) {
    const modelId = normalizeModelProfileId(rawModelId)
    if (!modelId) continue
    profiles[modelId] = normalizeAnalytixModelProfile(rawProfile)
  }
  if (!patch || typeof patch !== 'object' || Array.isArray(patch)) return profiles
  for (const [rawModelId, rawProfile] of Object.entries(patch)) {
    const modelId = normalizeModelProfileId(rawModelId)
    if (!modelId) continue
    if (rawProfile === null) {
      delete profiles[modelId]
      continue
    }
    profiles[modelId] = normalizeAnalytixModelProfile({
      ...(profiles[modelId] ?? {}),
      ...rawProfile
    })
  }
  return profiles
}

function normalizeAnalytixSubagentsSettings(
  input: Partial<AnalytixSubagentsSettingsV1> | undefined
): AnalytixSubagentsSettingsV1 {
  const defaults = defaultAnalytixSubagentsSettings()
  const profiles = normalizeAnalytixSubagentProfiles(input?.profiles, undefined)
  const requestedDefaultProfile = typeof input?.defaultProfile === 'string'
    ? input.defaultProfile.trim()
    : defaults.defaultProfile
  return {
    enabled: input?.enabled !== false,
    maxParallel: boundedNonNegativeInt(input?.maxParallel, defaults.maxParallel, 128),
    maxChildRuns: boundedNonNegativeInt(input?.maxChildRuns, defaults.maxChildRuns, 10_000),
    defaultToolPolicy: normalizeAnalytixSubagentToolPolicy(input?.defaultToolPolicy) ?? defaults.defaultToolPolicy,
    defaultProfile: requestedDefaultProfile && Object.prototype.hasOwnProperty.call(profiles, requestedDefaultProfile)
      ? requestedDefaultProfile
      : '',
    profiles
  }
}

function normalizeAnalytixSubagentProfiles(
  current: Record<string, AnalytixSubagentProfileV1> | undefined,
  patch: Record<string, AnalytixSubagentProfilePatchV1 | null> | undefined
): Record<string, AnalytixSubagentProfileV1> {
  const profiles: Record<string, AnalytixSubagentProfileV1> = {}
  const currentNames = Object.keys(current ?? {}).sort()
  for (const rawName of currentNames) {
    const name = normalizeAnalytixSubagentProfileName(rawName)
    if (!name) continue
    profiles[name] = normalizeAnalytixSubagentProfile(current?.[rawName])
  }
  if (!patch || typeof patch !== 'object' || Array.isArray(patch)) return profiles
  for (const rawName of Object.keys(patch).sort()) {
    const name = normalizeAnalytixSubagentProfileName(rawName)
    if (!name) continue
    const rawProfile = patch[rawName]
    if (rawProfile === null) {
      delete profiles[name]
      continue
    }
    profiles[name] = normalizeAnalytixSubagentProfile({
      ...(profiles[name] ?? {}),
      ...rawProfile
    })
  }
  return profiles
}

function normalizeAnalytixSubagentProfile(
  input: AnalytixSubagentProfilePatchV1 | undefined
): AnalytixSubagentProfileV1 {
  return {
    prompt: typeof input?.prompt === 'string' ? input.prompt.trim().slice(0, 16 * 1024) : '',
    model: typeof input?.model === 'string' ? input.model.trim().slice(0, 256) : '',
    effort: normalizeAnalytixSubagentEffort(input?.effort) ?? '',
    toolPolicy: normalizeAnalytixSubagentToolPolicy(input?.toolPolicy) ?? 'readOnly',
    tools: normalizeAnalytixSubagentToolScope(input?.tools)
  }
}

function normalizeAnalytixSubagentProfileName(value: string): string {
  return value.trim().slice(0, 128)
}

function normalizeAnalytixSubagentToolPolicy(value: unknown): AnalytixSubagentsSettingsV1['defaultToolPolicy'] | undefined {
  return value === 'readOnly' || value === 'inherit' ? value : undefined
}

function normalizeAnalytixSubagentEffort(value: unknown): AnalytixSubagentProfileV1['effort'] | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim().toLowerCase()
  return normalized === 'off' ||
    normalized === 'low' ||
    normalized === 'medium' ||
    normalized === 'high' ||
    normalized === 'max'
    ? normalized
    : undefined
}

function normalizeAnalytixSubagentToolScope(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  const tools: string[] = []
  for (const item of value) {
    if (typeof item !== 'string') continue
    const tool = item.trim().slice(0, 128)
    if (!tool || seen.has(tool)) continue
    seen.add(tool)
    tools.push(tool)
  }
  return tools.sort()
}

function normalizeAnalytixModelProfile(
  input: ModelProviderModelProfilePatchV1 | undefined
): ModelProviderModelProfileV1 {
  const inputModalities = normalizeAnalytixModelInputModalities(input?.inputModalities)
  const fallbackMessageParts: ModelProviderMessagePartSupport[] = inputModalities.includes('image')
    ? ['text', 'image_url']
    : ['text']
  const contextWindowTokens = typeof input?.contextWindowTokens === 'number' &&
    Number.isInteger(input.contextWindowTokens) &&
    input.contextWindowTokens > 0
    ? input.contextWindowTokens
    : undefined
  const reasoning = normalizeAnalytixReasoningCapability(input?.reasoning)
  const endpointFormat = typeof input?.endpointFormat === 'string' && input.endpointFormat.trim()
    ? normalizeModelEndpointFormat(input.endpointFormat)
    : undefined
  return {
    ...(normalizeAnalytixProfileAliases(input?.aliases).length
      ? { aliases: normalizeAnalytixProfileAliases(input?.aliases) }
      : {}),
    ...(contextWindowTokens ? { contextWindowTokens } : {}),
    inputModalities,
    outputModalities: normalizeAnalytixModelInputModalities(input?.outputModalities),
    supportsToolCalling: input?.supportsToolCalling !== false,
    messageParts: normalizeAnalytixModelMessageParts(input?.messageParts, fallbackMessageParts),
    ...(reasoning ? { reasoning } : {}),
    ...(endpointFormat ? { endpointFormat } : {})
  }
}

function normalizeAnalytixReasoningCapability(
  input: ModelProviderModelProfilePatchV1['reasoning'] | undefined
): ModelProviderReasoningCapabilityV1 | undefined {
  if (!input || typeof input !== 'object') return undefined
  const supportedEfforts = normalizeAnalytixReasoningEfforts(input.supportedEfforts)
  if (supportedEfforts.length === 0) return undefined
  const defaultEffort = normalizeAnalytixReasoningEffort(input.defaultEffort)
  const requestProtocol = normalizeAnalytixReasoningRequestProtocol(input.requestProtocol)
  if (!requestProtocol) return undefined
  return {
    supportedEfforts,
    defaultEffort: defaultEffort && supportedEfforts.includes(defaultEffort)
      ? defaultEffort
      : supportedEfforts[0],
    requestProtocol
  }
}

function normalizeAnalytixReasoningEfforts(value: unknown): ModelProviderReasoningCapabilityV1['supportedEfforts'] {
  if (!Array.isArray(value)) return []
  const efforts: ModelProviderReasoningCapabilityV1['supportedEfforts'] = []
  for (const item of value) {
    const effort = normalizeAnalytixReasoningEffort(item)
    if (effort && !efforts.includes(effort)) efforts.push(effort)
  }
  return efforts
}

function normalizeAnalytixReasoningEffort(value: unknown): ModelProviderReasoningCapabilityV1['defaultEffort'] | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim().toLowerCase()
  return MODEL_REASONING_EFFORTS.includes(normalized as ModelProviderReasoningCapabilityV1['defaultEffort'])
    ? normalized as ModelProviderReasoningCapabilityV1['defaultEffort']
    : undefined
}

function normalizeAnalytixReasoningRequestProtocol(
  value: unknown
): ModelProviderReasoningCapabilityV1['requestProtocol'] | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim().toLowerCase()
  return MODEL_REASONING_REQUEST_PROTOCOLS.includes(normalized as ModelProviderReasoningCapabilityV1['requestProtocol'])
    ? normalized as ModelProviderReasoningCapabilityV1['requestProtocol']
    : undefined
}

function normalizeModelProfileId(value: string): string {
  return value.trim().slice(0, 128)
}

function normalizeAnalytixProfileAliases(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const aliases: string[] = []
  for (const item of value) {
    if (typeof item !== 'string') continue
    const alias = item.trim().slice(0, 128)
    if (alias && !aliases.includes(alias)) aliases.push(alias)
    if (aliases.length >= 50) break
  }
  return aliases
}

function normalizeAnalytixModelInputModalities(value: unknown): ModelProviderInputModality[] {
  if (!Array.isArray(value)) return ['text']
  const modalities: ModelProviderInputModality[] = []
  for (const item of value) {
    if ((item === 'text' || item === 'image') && !modalities.includes(item)) {
      modalities.push(item)
    }
    if (modalities.length >= 8) break
  }
  return modalities.length > 0 ? modalities : ['text']
}

function normalizeAnalytixModelMessageParts(
  value: unknown,
  fallback: ModelProviderMessagePartSupport[]
): ModelProviderMessagePartSupport[] {
  if (!Array.isArray(value)) return [...fallback]
  const parts: ModelProviderMessagePartSupport[] = []
  for (const item of value) {
    if (
      (item === 'text' || item === 'image_url' || item === 'input_image') &&
      !parts.includes(item)
    ) {
      parts.push(item)
    }
    if (parts.length >= 8) break
  }
  return parts.length > 0 ? parts : [...fallback]
}

export function withAnalytixRuntimeSettings(
  settings: AppSettingsV1,
  analytix: AnalytixRuntimeSettingsV1
): AppSettingsV1 {
  return {
    ...settings,
    runtime: analytix
  }
}

export function applyAnalytixRuntimePatch(
  settings: AppSettingsV1,
  patch: AnalytixRuntimeSettingsPatchV1 | undefined
): AppSettingsV1 {
  return withAnalytixRuntimeSettings(
    settings,
    mergeAnalytixRuntimeSettings(getAnalytixRuntimeSettings(settings), patch)
  )
}

export function isAnalytixRuntimeInsecure(runtime: Pick<AnalytixRuntimeSettingsV1, 'insecure' | 'runtimeToken'>): boolean {
  return runtime.insecure === true
}

type LegacyAnalytixRuntimeInput = Partial<AnalytixRuntimeSettingsV1>
type LegacyProviderSettingsInput = Partial<Omit<ModelProviderSettingsV1, 'providers'>> & {
  providers?: Array<Partial<ModelProviderProfileV1>>
}
type LegacyAppSettingsShape = Partial<Omit<AppSettingsV1, 'runtime' | 'provider'>> & {
  runtime?: LegacyAnalytixRuntimeInput
  provider?: LegacyProviderSettingsInput
  agentProvider?: unknown
  deepseek?: Partial<LegacyLocalHttpRuntimeSettingsV1>
  agents?: {
    codewhale?: Partial<LegacyLocalHttpRuntimeSettingsV1>
    reasonix?: Partial<LegacyReasoningRuntimeSettingsV1>
  }
}

function nonEmptyStringOrFallback(value: unknown, fallback: string): string {
  return typeof value === 'string' && value.trim() ? value : fallback
}

function upgradeLegacyAnalytixDefaultDataDir(value: unknown): string {
  if (typeof value !== 'string') return DEFAULT_ANALYTIX_DATA_DIR
  const trimmed = value.trim()
  const normalized = trimmed.replace(/\\/g, '/').toLowerCase()
  if (
    !trimmed ||
    normalized === LEGACY_COREAGENT_DATA_DIR ||
    normalized.endsWith('/.analytix/coreagent')
  ) {
    return DEFAULT_ANALYTIX_DATA_DIR
  }
  return trimmed
}

function upgradeLegacyAnalytixDefaultModel(value: unknown, fallback: string): string {
  const model = nonEmptyStringOrFallback(value, fallback).trim()
  return model === LEGACY_ANALYTIX_DEFAULT_MODEL ? DEFAULT_ANALYTIX_MODEL : model
}

function upgradeLegacyAnalytixDefaultPort(value: unknown, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return fallback
  return Math.floor(value) === LEGACY_LOCAL_HTTP_DEFAULT_PORT ? DEFAULT_ANALYTIX_PORT : Math.floor(value)
}

function selectedLegacyRuntimeSettings(parsed: LegacyAppSettingsShape): LegacyAnalytixRuntimeInput {
  const provider = typeof parsed.agentProvider === 'string' ? parsed.agentProvider : ''
  if (!provider && parsed.deepseek) {
    const source = parsed.deepseek
    const defaults = legacyLocalHttpRuntimeDefaults(LEGACY_LOCAL_HTTP_DEFAULT_PORT)
    return {
      binaryPath: '',
      port: source.port ?? defaults.port,
      autoStart: source.autoStart ?? defaults.autoStart,
      baseUrl: source.baseUrl ?? defaults.baseUrl,
      runtimeToken: source.runtimeToken ?? defaults.runtimeToken,
      approvalPolicy: source.approvalPolicy ?? defaults.approvalPolicy,
      sandboxMode: source.sandboxMode ?? defaults.sandboxMode
    }
  }

  if (provider === 'reasonix') {
    const source = parsed.agents?.reasonix ?? {}
    const defaults = legacyReasoningRuntimeDefaults()
    return {
      binaryPath: '',
      autoStart: source.autoStart ?? defaults.autoStart,
      baseUrl: source.baseUrl ?? defaults.baseUrl,
      model: source.model ?? defaults.model
    }
  }

  if (provider === 'codewhale' || provider === 'deepseek-runtime') {
    const source = {
      ...(provider === 'codewhale' ? parsed.agents?.codewhale ?? {} : {}),
      ...(parsed.deepseek ?? {})
    }
    const defaults = legacyLocalHttpRuntimeDefaults(
      provider === 'codewhale' ? DEFAULT_ANALYTIX_PORT : LEGACY_LOCAL_HTTP_DEFAULT_PORT
    )
    return {
      binaryPath: '',
      port: source.port ?? defaults.port,
      autoStart: source.autoStart ?? defaults.autoStart,
      baseUrl: source.baseUrl ?? defaults.baseUrl,
      runtimeToken: source.runtimeToken ?? defaults.runtimeToken,
      approvalPolicy: source.approvalPolicy ?? defaults.approvalPolicy,
      sandboxMode: source.sandboxMode ?? defaults.sandboxMode
    }
  }

  return {}
}

export function migrateLegacyAppSettings(parsed: LegacyAppSettingsShape): Partial<AppSettingsV1> {
  const hasProviderSettings = typeof parsed.provider === 'object' && parsed.provider !== null
  const analytixDefaults = defaultAnalytixRuntimeSettings()
  const explicitAnalytix: LegacyAnalytixRuntimeInput = projectAnalytixRuntimeFields(
    parsed.runtime ?? selectedLegacyRuntimeSettings(parsed)
  )
  const executionPolicy = normalizeAnalytixExecutionPolicy(explicitAnalytix)
  const legacySeed = {
    binaryPath: analytixDefaults.binaryPath,
    port: upgradeLegacyAnalytixDefaultPort(explicitAnalytix.port, analytixDefaults.port),
    autoStart: explicitAnalytix.autoStart ?? analytixDefaults.autoStart,
    baseUrl: explicitAnalytix.baseUrl ?? analytixDefaults.baseUrl,
    providerId: '',
    endpointFormat: DEFAULT_MODEL_ENDPOINT_FORMAT,
    runtimeToken: explicitAnalytix.runtimeToken ?? analytixDefaults.runtimeToken,
    model: explicitAnalytix.model ?? analytixDefaults.model,
    ...executionPolicy
  }
  const provider = normalizeModelProviderSettings({
    activeProviderId: parsed.provider?.activeProviderId,
    baseUrl: hasProviderSettings
      ? parsed.provider?.baseUrl
      : nonEmptyStringOrFallback(explicitAnalytix.baseUrl, legacySeed.baseUrl),
    providers: parsed.provider?.providers
  })
  const analytix = {
    ...analytixDefaults,
    ...legacySeed,
    ...explicitAnalytix,
    ...executionPolicy,
    port: legacySeed.port,
    baseUrl: hasProviderSettings ? explicitAnalytix.baseUrl ?? '' : '',
    runtimeToken: nonEmptyStringOrFallback(explicitAnalytix.runtimeToken, legacySeed.runtimeToken),
    dataDir: upgradeLegacyAnalytixDefaultDataDir(explicitAnalytix.dataDir),
    model: upgradeLegacyAnalytixDefaultModel(explicitAnalytix.model, legacySeed.model),
    tokenEconomyMode: typeof explicitAnalytix.tokenEconomy?.enabled === 'boolean'
      ? explicitAnalytix.tokenEconomy.enabled
      : explicitAnalytix.tokenEconomyMode ?? analytixDefaults.tokenEconomyMode,
    tokenEconomy: normalizeAnalytixTokenEconomySettings(
      explicitAnalytix.tokenEconomy,
      explicitAnalytix.tokenEconomyMode ?? analytixDefaults.tokenEconomyMode
    ),
    mcpSearch: normalizeAnalytixMcpSearchSettings(explicitAnalytix.mcpSearch),
    storage: normalizeAnalytixStorageSettings(explicitAnalytix.storage),
    contextCompaction: normalizeAnalytixContextCompactionSettings(explicitAnalytix.contextCompaction),
    runtimeTuning: normalizeAnalytixRuntimeTuningSettings(explicitAnalytix.runtimeTuning),
    imageGeneration: normalizeAnalytixImageGenerationSettings(explicitAnalytix.imageGeneration),
    speechToText: normalizeAnalytixSpeechToTextSettings(explicitAnalytix.speechToText),
    textToSpeech: normalizeAnalytixTextToSpeechSettings(explicitAnalytix.textToSpeech),
    musicGeneration: normalizeAnalytixMusicGenerationSettings(explicitAnalytix.musicGeneration),
    videoGeneration: normalizeAnalytixVideoGenerationSettings(explicitAnalytix.videoGeneration),
    modelCapabilityProbes: normalizeModelCapabilityProbes({}, explicitAnalytix.modelCapabilityProbes as Record<string, ModelCapabilityProbeResultV1 | null> | undefined),
    computerUse: normalizeAnalytixComputerUseSettings(explicitAnalytix.computerUse),
    browserUse: normalizeAnalytixBrowserUseSettings(explicitAnalytix.browserUse),
    visionBridge: normalizeAnalytixVisionBridgeSettings(explicitAnalytix.visionBridge),
    quality: normalizeAnalytixQualitySettings(explicitAnalytix.quality)
  }
  const {
    runtime: _runtime,
    agentProvider: _agentProvider,
    deepseek: _deepseek,
    agents: _agents,
    ...rest
  } = parsed
  void _runtime
  void _agentProvider
  void _deepseek
  void _agents
  const canonicalRest = Object.fromEntries(
    Object.entries(rest).filter(([key]) => APP_SETTINGS_V1_KEYS.has(key))
  ) as Partial<AppSettingsV1>
  return {
    ...canonicalRest,
    provider,
    runtime: analytix
  }
}
