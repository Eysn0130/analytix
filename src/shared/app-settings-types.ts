import type { GuiUpdateChannel } from './gui-update'
import type { KeyboardShortcutsConfigV1 } from './keyboard-shortcuts'
import type { ApprovalPolicy, SandboxMode } from '../../packages/runtime/src/contracts/policy.js'
import type { ComputerUseMode } from '../../packages/runtime/src/contracts/capabilities.js'
import type { ModelEndpointFormat } from '../../packages/runtime/src/contracts/model-endpoint-format.js'
export {
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  inferModelEndpointFormatFromUrl,
  isCustomModelEndpointFormat,
  MODEL_ENDPOINT_FORMATS,
  modelEndpointPath,
  normalizeModelEndpointFormat,
  resolveModelEndpointFormat,
  usesChatCompletionsShape
} from '../../packages/runtime/src/contracts/model-endpoint-format.js'
export { DEFAULT_GUI_UPDATE_CHANNEL, normalizeGuiUpdateChannel, type GuiUpdateChannel } from './gui-update'
export {
  APPROVAL_POLICIES,
  CURRENT_EXECUTION_POLICY_VERSION,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  SANDBOX_MODES,
  type ApprovalPolicy,
  type SandboxMode
} from '../../packages/runtime/src/contracts/policy.js'
export const ANALYTIX_TOOL_PERMISSION_MODES = ['request-approval', 'auto-approval', 'full-access', 'custom'] as const
export type AnalytixToolPermissionMode = (typeof ANALYTIX_TOOL_PERMISSION_MODES)[number]
export type AnalytixToolPermissionPreset = Exclude<AnalytixToolPermissionMode, 'custom'>

export function analytixToolPermissionModeSettings(
  mode: AnalytixToolPermissionPreset
): { approvalPolicy: ApprovalPolicy; sandboxMode: SandboxMode } {
  switch (mode) {
    case 'request-approval':
      return { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' }
    case 'auto-approval':
      return { approvalPolicy: 'untrusted', sandboxMode: 'danger-full-access' }
    case 'full-access':
      return { approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }
  }
}

export function analytixToolPermissionModeFromSettings(
  settings: Pick<{ approvalPolicy: ApprovalPolicy; sandboxMode: SandboxMode }, 'approvalPolicy' | 'sandboxMode'>
): AnalytixToolPermissionMode {
  if (settings.approvalPolicy === 'on-request' && settings.sandboxMode === 'workspace-write') {
    return 'request-approval'
  }
  if (settings.approvalPolicy === 'untrusted' && settings.sandboxMode === 'danger-full-access') {
    return 'auto-approval'
  }
  if (settings.approvalPolicy === 'auto' && settings.sandboxMode === 'danger-full-access') {
    return 'full-access'
  }
  return 'custom'
}
export type UiFontScale = 'small' | 'medium' | 'large'
export const APP_MOTION_PREFERENCES = ['on', 'system', 'off'] as const
export type AppMotionPreference = (typeof APP_MOTION_PREFERENCES)[number]
export const DEFAULT_APP_MOTION_PREFERENCE: AppMotionPreference = 'on'
export function normalizeAppMotionPreference(value: unknown): AppMotionPreference {
  return APP_MOTION_PREFERENCES.includes(value as AppMotionPreference)
    ? (value as AppMotionPreference)
    : DEFAULT_APP_MOTION_PREFERENCE
}
export type ScheduleRunMode = 'agent' | 'plan'
export type ScheduleKind = 'manual' | 'interval' | 'daily' | 'at'
export type ScheduleTaskStatus = 'idle' | 'running' | 'success' | 'error'
export const SCHEDULE_TASK_MESSAGE_KEYS = [
  'schedule_task_idle',
  'schedule_task_running',
  'schedule_task_started',
  'schedule_task_completed',
  'schedule_task_failed',
  'schedule_task_interrupted'
] as const
export type ScheduleTaskMessageKey = (typeof SCHEDULE_TASK_MESSAGE_KEYS)[number]
export type ScheduleModel = 'deepseek-v4-pro' | 'deepseek-v4-flash'
export type ScheduleReasoningEffort = 'auto' | 'off' | 'low' | 'medium' | 'high' | 'max'
export type ClawRunMode = ScheduleRunMode
export type ClawImProvider = 'feishu' | 'weixin' | 'telegram'
export type ClawScheduleKind = ScheduleKind
export type ClawTaskStatus = ScheduleTaskStatus
export type ClawModel = 'auto' | ScheduleModel

export const DEFAULT_DEEPSEEK_BASE_URL = 'https://api.deepseek.com'
export const CUSTOM_IMAGE_GENERATION_PROVIDER_ID = 'custom'
export const IMAGE_GENERATION_PROTOCOLS = ['openai-images', 'minimax-image'] as const
export type ImageGenerationProtocol = (typeof IMAGE_GENERATION_PROTOCOLS)[number]
export const DEFAULT_IMAGE_GENERATION_PROTOCOL: ImageGenerationProtocol = 'openai-images'
export const CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID = 'custom'
export const SPEECH_TO_TEXT_PROTOCOLS = ['openai-transcriptions', 'mimo-asr'] as const
export type SpeechToTextProtocol = (typeof SPEECH_TO_TEXT_PROTOCOLS)[number]
export const DEFAULT_SPEECH_TO_TEXT_PROTOCOL: SpeechToTextProtocol = 'openai-transcriptions'
export const CUSTOM_TEXT_TO_SPEECH_PROVIDER_ID = 'custom'
export const TEXT_TO_SPEECH_PROTOCOLS = ['openai-speech', 'minimax-t2a', 'mimo-tts'] as const
export type TextToSpeechProtocol = (typeof TEXT_TO_SPEECH_PROTOCOLS)[number]
export const DEFAULT_TEXT_TO_SPEECH_PROTOCOL: TextToSpeechProtocol = 'openai-speech'
export const CUSTOM_MUSIC_GENERATION_PROVIDER_ID = 'custom'
export const MUSIC_GENERATION_PROTOCOLS = ['minimax-music'] as const
export type MusicGenerationProtocol = (typeof MUSIC_GENERATION_PROTOCOLS)[number]
export const DEFAULT_MUSIC_GENERATION_PROTOCOL: MusicGenerationProtocol = 'minimax-music'
export const CUSTOM_VIDEO_GENERATION_PROVIDER_ID = 'custom'
export const VIDEO_GENERATION_PROTOCOLS = ['minimax-video'] as const
export type VideoGenerationProtocol = (typeof VIDEO_GENERATION_PROTOCOLS)[number]
export const DEFAULT_VIDEO_GENERATION_PROTOCOL: VideoGenerationProtocol = 'minimax-video'
export const DEFAULT_CLAW_MODEL = 'auto'
export const CLAW_MODEL_IDS = ['auto', 'deepseek-v4-pro', 'deepseek-v4-flash'] as const
export const DEFAULT_SCHEDULE_MODEL = 'deepseek-v4-flash'
export const SCHEDULE_MODEL_IDS = ['deepseek-v4-pro', 'deepseek-v4-flash'] as const
export const DEFAULT_SCHEDULE_REASONING_EFFORT = 'medium'
export const SCHEDULE_REASONING_EFFORT_IDS = ['auto', 'off', 'low', 'medium', 'high', 'max'] as const
export const DEFAULT_SCHEDULE_INTERNAL_PORT = 8788
// 这些默认目录与 legacy-data-migration.ts 的 HOME_DATA_MIGRATION_MAPPINGS
// 一一对应:老安装的 ~/.analytix/* 在启动期被搬到这里。
export const DEFAULT_WRITE_WORKSPACE_ROOT = '~/.analytix/write_workspace'
export const DEFAULT_ANALYTIX_DATA_DIR = '~/.analytix/data'
export const DEFAULT_ANALYTIX_MODEL = 'deepseek-v4-flash'
export const DEFAULT_WRITE_INLINE_COMPLETION_BASE_URL = 'https://api.deepseek.com/beta'
export const DEFAULT_WRITE_INLINE_COMPLETION_MODEL = 'deepseek-v4-flash'
export const WRITE_INLINE_COMPLETION_MODEL_IDS = ['deepseek-v4-pro', 'deepseek-v4-flash'] as const
export const DEFAULT_WRITE_INLINE_COMPLETION_DEBOUNCE_MS = 650
export const DEFAULT_WRITE_INLINE_COMPLETION_MIN_ACCEPT_SCORE = 0.52
export const DEFAULT_WRITE_INLINE_COMPLETION_MAX_TOKENS = 96
export const DEFAULT_WRITE_INLINE_LONG_COMPLETION_DEBOUNCE_MS = 2_800
export const DEFAULT_WRITE_INLINE_LONG_COMPLETION_MIN_ACCEPT_SCORE = 0.36
export const DEFAULT_WRITE_INLINE_LONG_COMPLETION_MAX_TOKENS = 256
export const DEFAULT_ANALYTIX_PORT = 8899
export const DEFAULT_WEIXIN_BRIDGE_RPC_URL = 'http://127.0.0.1:18790/api/v1/admin/rpc'
export const DEFAULT_MODEL_PROVIDER_ID = 'deepseek'
export const NETWORK_PROXY_PROTOCOLS = ['http', 'https', 'socks', 'socks4', 'socks4a', 'socks5', 'socks5h'] as const
export type NetworkProxyProtocol = (typeof NETWORK_PROXY_PROTOCOLS)[number]
export type NetworkProxySettingsV1 = {
  enabled: boolean
  url: string
}
export type { ModelEndpointFormat }
export const MODEL_PROVIDER_INPUT_MODALITIES = ['text', 'image'] as const
export type ModelProviderInputModality = (typeof MODEL_PROVIDER_INPUT_MODALITIES)[number]
export const MODEL_PROVIDER_MESSAGE_PARTS = ['text', 'image_url', 'input_image'] as const
export type ModelProviderMessagePartSupport = (typeof MODEL_PROVIDER_MESSAGE_PARTS)[number]
export const MODEL_REASONING_EFFORTS = ['auto', 'off', 'low', 'medium', 'high', 'max'] as const
export type ModelReasoningEffort = (typeof MODEL_REASONING_EFFORTS)[number]
export const MODEL_REASONING_REQUEST_PROTOCOLS = [
  'none',
  'deepseek-chat-completions',
  'glm-chat-completions',
  'mimo-chat-completions',
  'openai-responses',
  'anthropic-thinking'
] as const
export type ModelReasoningRequestProtocol = (typeof MODEL_REASONING_REQUEST_PROTOCOLS)[number]
export type ModelProviderReasoningCapabilityV1 = {
  supportedEfforts: ModelReasoningEffort[]
  defaultEffort: ModelReasoningEffort
  requestProtocol: ModelReasoningRequestProtocol
}
export type ModelProviderPricingV1 = {
  /** Price per 1M cached input tokens, in `currency`. */
  cacheHit?: number
  /** Price per 1M uncached input tokens, in `currency`. */
  input?: number
  /** Price per 1M output tokens, in `currency`. */
  output?: number
  currency?: 'USD' | 'CNY'
}
export type ModelProviderModelProfileV1 = {
  aliases?: string[]
  contextWindowTokens?: number
  inputModalities: ModelProviderInputModality[]
  outputModalities: ModelProviderInputModality[]
  supportsToolCalling: boolean
  messageParts: ModelProviderMessagePartSupport[]
  reasoning?: ModelProviderReasoningCapabilityV1
  /** Per-model wire-format override. Omitted means "inherit the provider's endpointFormat". */
  endpointFormat?: ModelEndpointFormat
  price?: ModelProviderPricingV1
}
export type ModelProviderImageCapabilityV1 = {
  protocol: ImageGenerationProtocol
  baseUrl: string
  models: string[]
}
export type ModelProviderSpeechCapabilityV1 = {
  protocol: SpeechToTextProtocol
  baseUrl: string
  models: string[]
}
export type ModelProviderTextToSpeechCapabilityV1 = {
  protocol: TextToSpeechProtocol
  baseUrl: string
  models: string[]
}
export type ModelProviderMusicCapabilityV1 = {
  protocol: MusicGenerationProtocol
  baseUrl: string
  models: string[]
}
export type ModelProviderVideoCapabilityV1 = {
  protocol: VideoGenerationProtocol
  baseUrl: string
  models: string[]
}
export type ModelProviderProfileV1 = {
  id: string
  name: string
  baseUrl: string
  endpointFormat: ModelEndpointFormat
  models: string[]
  modelProfiles: Record<string, ModelProviderModelProfileV1>
  price?: ModelProviderPricingV1
  prices?: Record<string, ModelProviderPricingV1>
  image?: ModelProviderImageCapabilityV1
  speech?: ModelProviderSpeechCapabilityV1
  textToSpeech?: ModelProviderTextToSpeechCapabilityV1
  music?: ModelProviderMusicCapabilityV1
  video?: ModelProviderVideoCapabilityV1
}
export type ModelProviderSettingsV1 = {
  /** Active provider profile for the Analytix runtime when runtime.providerId is unset. */
  activeProviderId?: string
  baseUrl: string
  proxy: NetworkProxySettingsV1
  providers: ModelProviderProfileV1[]
}

export type ModelProviderImageCapabilityPatchV1 = Partial<ModelProviderImageCapabilityV1>
export type ModelProviderSpeechCapabilityPatchV1 = Partial<ModelProviderSpeechCapabilityV1>
export type ModelProviderTextToSpeechCapabilityPatchV1 = Partial<ModelProviderTextToSpeechCapabilityV1>
export type ModelProviderMusicCapabilityPatchV1 = Partial<ModelProviderMusicCapabilityV1>
export type ModelProviderVideoCapabilityPatchV1 = Partial<ModelProviderVideoCapabilityV1>
export type ModelProviderModelProfilePatchV1 = Partial<ModelProviderModelProfileV1>
export type ModelProviderProfilePatchV1 = Partial<Omit<ModelProviderProfileV1, 'image' | 'speech' | 'textToSpeech' | 'music' | 'video' | 'modelProfiles'>> & {
  modelProfiles?: Record<string, ModelProviderModelProfilePatchV1 | null>
  image?: ModelProviderImageCapabilityPatchV1 | null
  speech?: ModelProviderSpeechCapabilityPatchV1 | null
  textToSpeech?: ModelProviderTextToSpeechCapabilityPatchV1 | null
  music?: ModelProviderMusicCapabilityPatchV1 | null
  video?: ModelProviderVideoCapabilityPatchV1 | null
}
export type ModelProviderSettingsPatchV1 = Partial<
  Omit<ModelProviderSettingsV1, 'providers' | 'proxy'>
> & {
  proxy?: Partial<NetworkProxySettingsV1>
  providers?: ModelProviderProfilePatchV1[]
}

export type AnalytixRuntimeSettingsV1 = {
  binaryPath: string
  port: number
  autoStart: boolean
  /** Optional override. Leave empty to inherit the General model provider Base URL. */
  baseUrl: string
  /** Selected General model provider profile. Empty or missing means the default provider. */
  providerId: string
  /** Effective model request format. Resolved from the selected model provider. */
  endpointFormat: ModelEndpointFormat
  runtimeToken: string
  dataDir: string
  model: string
  /** Persisted marker for approval/sandbox default semantics. */
  executionPolicyVersion: number
  approvalPolicy: ApprovalPolicy
  sandboxMode: SandboxMode
  /** Compress safe tool context before each model call. */
  tokenEconomyMode: boolean
  /** Detailed token-saving behavior used when building Analytix model requests. */
  tokenEconomy: AnalytixTokenEconomySettingsV1
  /** When true, the runtime skips bearer-token auth. Local dev only. */
  insecure: boolean
  /** GUI-managed MCP progressive discovery/search settings written into Analytix config.json. */
  mcpSearch: AnalytixMcpSearchSettingsV1
  /** Persistent store backend used by Analytix. */
  storage: AnalytixStorageSettingsV1
  /** Fallback compaction thresholds and summary behavior. Per-model thresholds live in Analytix config models.profiles. */
  contextCompaction: AnalytixContextCompactionSettingsV1
  /** Low-level loop guards and model argument repair tuning. */
  runtimeTuning: AnalytixRuntimeTuningSettingsV1
  /** Electron-owned image generation provider used by Write image tools. */
  imageGeneration: AnalytixImageGenerationSettingsV1
  /** Speech-to-text provider used for voice input in the composer. */
  speechToText: AnalytixSpeechToTextSettingsV1
  /** Compatibility-only text-to-speech settings; no production Go tool consumes them. */
  textToSpeech: AnalytixTextToSpeechSettingsV1
  /** Compatibility-only music settings; no production Go tool consumes them. */
  musicGeneration: AnalytixMusicGenerationSettingsV1
  /** Compatibility-only video settings; no production Go tool consumes them. */
  videoGeneration: AnalytixVideoGenerationSettingsV1
  /** GUI-owned model capability profiles written into Analytix `models.profiles`. */
  modelProfiles: Record<string, ModelProviderModelProfileV1>
  /** Sanitized per-provider/model capability probe cache used to override native image support at runtime. */
  modelCapabilityProbes: Record<string, ModelCapabilityProbeResultV1>
  /** GUI-owned subagent profile/runtime settings written into Analytix `capabilities.subagents`. */
  subagents: AnalytixSubagentsSettingsV1
  /** Whether long-term memory is enabled in the Analytix runtime. */
  memoryEnabled: boolean
  /** Host computer-use (screenshot + mouse/keyboard control) settings. */
  computerUse: AnalytixComputerUseSettingsV1
  /** GUI-managed in-app browser and Browser Use preferences. */
  browserUse: AnalytixBrowserUseSettingsV1
  /** Vision model bridge that converts screenshots into text observations for text-only primary models. */
  visionBridge: AnalytixVisionBridgeSettingsV1
  /** First-party design-quality linter applied to frontend output. */
  quality: AnalytixDesignQualitySettingsV1
}

/** Detection aggressiveness for the design-quality linter. */
export type AnalytixDesignQualityStrictness = 'relaxed' | 'standard' | 'strict'

export type AnalytixDesignQualitySettingsV1 = {
  /** Master switch. Off means the builtin design-quality hook never fires. */
  enabled: boolean
  strictness: AnalytixDesignQualityStrictness
  /** Rule ids to suppress. */
  ignoreRules: string[]
  /** Relative-path glob patterns to skip. */
  ignoreFiles: string[]
  /** Cap on findings folded into a single tool result. */
  maxFindings: number
}

export type AnalytixComputerUseSettingsV1 = {
  /** Master switch. Off means the computer_use tool is never registered. */
  enabled: boolean
  /**
   * `auto`: advertise only to image-capable models. `always`: advertise
   * whenever enabled and the native backend is available. `off`: never advertise.
   */
  mode: ComputerUseMode
  /** Longest screenshot edge (px); larger captures are downscaled for grounding. */
  maxImageDimension: number
  /** Hard cap on computer_use actions per turn. */
  maxActionsPerTurn: number
  /** GUI policy for whether lock-screen operation is allowed when the backend supports it. */
  allowWhenLocked: boolean
}

export const ANALYTIX_BROWSER_LOCAL_URL_TARGETS = ['analytix', 'system'] as const
export type AnalytixBrowserLocalUrlTarget = (typeof ANALYTIX_BROWSER_LOCAL_URL_TARGETS)[number]
export const ANALYTIX_BROWSER_ANNOTATION_SCREENSHOT_MODES = ['always', 'necessary', 'off'] as const
export type AnalytixBrowserAnnotationScreenshotMode = (typeof ANALYTIX_BROWSER_ANNOTATION_SCREENSHOT_MODES)[number]
export const ANALYTIX_BROWSER_APPROVAL_MODES = ['alwaysAsk', 'neverAsk'] as const
export type AnalytixBrowserApprovalMode = (typeof ANALYTIX_BROWSER_APPROVAL_MODES)[number]

export type AnalytixBrowserSitePermissionV1 = {
  origin: string
  approvalMode: AnalytixBrowserApprovalMode
  downloadApprovalMode: AnalytixBrowserApprovalMode
  uploadApprovalMode: AnalytixBrowserApprovalMode
  fullCdpAccess: boolean
}

export type AnalytixBrowserUseSettingsV1 = {
  /** Master switch for GUI-managed in-app Browser controls. */
  enabled: boolean
  localUrlOpenTarget: AnalytixBrowserLocalUrlTarget
  annotationScreenshotsMode: AnalytixBrowserAnnotationScreenshotMode
  /** `neverAsk` maps to the Codex UI's "Always allow" browser approval mode. */
  approvalMode: AnalytixBrowserApprovalMode
  fullCdpAccess: boolean
  chromeControlEnabled: boolean
  sitePermissions: AnalytixBrowserSitePermissionV1[]
}

export type AnalytixVisionBridgeMode = 'auto' | 'always' | 'off'
export type AnalytixVisionBridgeInjectPolicy = 'observation_text'

export type AnalytixVisionBridgeSettingsV1 = {
  enabled: boolean
  mode: AnalytixVisionBridgeMode
  /** Empty inherits the active provider; a provider id reuses that provider's key/base URL unless overridden below. */
  providerId: string
  model: string
  /** Optional override. Empty inherits the selected bridge provider Base URL. */
  baseUrl: string
  /** Effective request protocol for the bridge model. Resolved from the bridge provider unless overridden. */
  endpointFormat: ModelEndpointFormat
  maxImageDimension: number
  maxImageBytes: number
  maxScreenshotsPerTurn: number
  observationCacheTtlMs: number
  injectPolicy: AnalytixVisionBridgeInjectPolicy
  fallbackWhenPrimaryImageUnsupported: boolean
}

export type ModelCapabilityProbeStatus =
  | 'unknown'
  | 'supported'
  | 'unsupported'
  | 'semantic_failed'
  | 'auth_failed'
  | 'http_failed'
  | 'timeout'
  | 'failed'
  | 'stale'

export type ModelCapabilityProbeResultV1 = {
  key: string
  providerId: string
  model: string
  endpointFormat: ModelEndpointFormat
  sanitizedBaseUrl: string
  requestUrl?: string
  probedAt: string
  staleAfter: string
  imageInput: ModelCapabilityProbeStatus
  toolCalling: ModelCapabilityProbeStatus
  toolResultImage: ModelCapabilityProbeStatus
  status: ModelCapabilityProbeStatus
  httpStatus?: number
  errorSummary?: string
}

export type AnalytixImageGenerationSettingsV1 = {
  enabled: boolean
  /** Existing provider profile to use for image generation. Empty or "custom" uses the fields below. */
  providerId: string
  /** Request protocol used when providerId is custom. Provider presets override this with their image capability. */
  protocol: ImageGenerationProtocol
  /** Custom image API root, or an override for the selected provider image API root. */
  baseUrl: string
  model: string
  /** Default "WxH" or "auto" used when the model omits aspect ratio and size. Empty means provider default. */
  defaultSize: string
  timeoutMs: number
}

export type AnalytixSpeechToTextSettingsV1 = {
  enabled: boolean
  /** Existing provider profile to use for speech recognition. Empty or "custom" uses the fields below. */
  providerId: string
  /** Request protocol used when providerId is custom. Provider presets override this with their speech capability. */
  protocol: SpeechToTextProtocol
  /** Custom speech API root, or an override for the selected provider speech API root. */
  baseUrl: string
  model: string
  /** Language hint sent to the provider ("zh", "en", ...). Empty means auto-detect. */
  language: string
  timeoutMs: number
}

export type AnalytixTextToSpeechSettingsV1 = {
  enabled: boolean
  /** Existing provider profile to use for speech generation. Empty or "custom" uses the fields below. */
  providerId: string
  /** Request protocol used when providerId is custom. Provider presets override this with their TTS capability. */
  protocol: TextToSpeechProtocol
  /** Custom TTS API root, or an override for the selected provider TTS API root. */
  baseUrl: string
  model: string
  /** Provider voice id/name. Empty means provider default. */
  voice: string
  /** Default output audio format such as mp3 or wav. */
  format: string
  timeoutMs: number
}

export type AnalytixMusicGenerationSettingsV1 = {
  enabled: boolean
  /** Existing provider profile to use for music generation. Empty or "custom" uses the fields below. */
  providerId: string
  protocol: MusicGenerationProtocol
  baseUrl: string
  model: string
  /** Default output audio format such as mp3 or wav. */
  format: string
  timeoutMs: number
}

export type AnalytixVideoGenerationSettingsV1 = {
  enabled: boolean
  /** Existing provider profile to use for video generation. Empty or "custom" uses the fields below. */
  providerId: string
  protocol: VideoGenerationProtocol
  baseUrl: string
  model: string
  /** Default video duration in seconds. */
  defaultDuration: number
  /** Default provider resolution value, e.g. 1080P. */
  defaultResolution: string
  timeoutMs: number
  pollIntervalMs: number
}

export type AnalytixMcpSearchMode = 'direct' | 'search' | 'auto'

export type AnalytixMcpSearchSettingsV1 = {
  enabled: boolean
  mode: AnalytixMcpSearchMode
  autoThresholdToolCount: number
  topKDefault: number
  topKMax: number
  minScore: number
}

export type AnalytixStorageBackend = 'hybrid' | 'file'

export type AnalytixStorageSettingsV1 = {
  backend: AnalytixStorageBackend
  sqlitePath: string
}

export type AnalytixCompactionSummaryMode = 'heuristic' | 'model'

export type AnalytixHistoryHygieneSettingsV1 = {
  maxToolResultLines: number
  maxToolResultBytes: number
  maxToolResultTokens: number
  maxToolArgumentStringBytes: number
  maxToolArgumentStringTokens: number
  maxArrayItems: number
  maxCumulativeToolResultTokens: number
  keepRecentToolResults: number
}

export type AnalytixTokenEconomySettingsV1 = {
  enabled: boolean
  compressToolDescriptions: boolean
  compressToolResults: boolean
  conciseResponses: boolean
  historyHygiene: AnalytixHistoryHygieneSettingsV1
}

export type AnalytixContextCompactionSettingsV1 = {
  defaultSoftThreshold: number
  defaultHardThreshold: number
  summaryMode: AnalytixCompactionSummaryMode
  summaryTimeoutMs: number
  summaryMaxTokens: number
  summaryInputMaxBytes: number
}

export type AnalytixToolStormSettingsV1 = {
  enabled: boolean
  windowSize: number
  threshold: number
}

export type AnalytixToolArgumentRepairSettingsV1 = {
  maxStringBytes: number
}

export type AnalytixRuntimeStepLimitSettingsV1 = {
  /** Default per-turn model/tool loop budget. `0` uses the bounded host default. */
  defaultMaxModelSteps: number
  /** User-global override. `0` inherits `defaultMaxModelSteps`. */
  userGlobalMaxModelSteps: number
  /** Plan-mode override. `0` inherits the agent/default limit. */
  plannerMaxModelSteps: number
  /** Headless/subagent override. `0` inherits the agent/default limit. */
  headlessMaxModelSteps: number
}

export type AnalytixRuntimeTuningSettingsV1 = {
  /**
   * Max idle gap (ms) between streaming chunks before a turn fails with
   * `stream_idle_timeout`. `0` disables the guard — useful for local LLM
   * servers that stay silent while prefilling a very large prompt.
   */
  streamIdleTimeoutMs: number
  stepLimits: AnalytixRuntimeStepLimitSettingsV1
  toolStorm: AnalytixToolStormSettingsV1
  toolArgumentRepair: AnalytixToolArgumentRepairSettingsV1
}

export type AnalytixSubagentToolPolicy = 'readOnly' | 'inherit'
export type AnalytixSubagentReasoningEffort = 'off' | 'low' | 'medium' | 'high' | 'max'

export type AnalytixSubagentProfileV1 = {
  /** Profile prompt prepended to the delegated task prompt. */
  prompt: string
  /** Optional child model override for this profile. */
  model: string
  /** Optional child reasoning-effort override for this profile. */
  effort: AnalytixSubagentReasoningEffort | ''
  /** Tool policy applied after the optional explicit tool scope. */
  toolPolicy: AnalytixSubagentToolPolicy
  /** Optional explicit tool scope. Empty inherits the policy default. */
  tools: string[]
}

export type AnalytixSubagentsSettingsV1 = {
  enabled: boolean
  maxParallel: number
  maxChildRuns: number
  defaultToolPolicy: AnalytixSubagentToolPolicy
  defaultProfile: string
  profiles: Record<string, AnalytixSubagentProfileV1>
}

export type AnalytixSubagentProfilePatchV1 = Partial<AnalytixSubagentProfileV1>
export type AnalytixSubagentsSettingsPatchV1 = Partial<
  Omit<AnalytixSubagentsSettingsV1, 'profiles'>
> & {
  profiles?: Record<string, AnalytixSubagentProfilePatchV1 | null>
}

export type AnalytixRuntimeTuningSettingsPatchV1 = {
  streamIdleTimeoutMs?: number
  stepLimits?: Partial<AnalytixRuntimeStepLimitSettingsV1>
  toolStorm?: Partial<AnalytixToolStormSettingsV1>
  toolArgumentRepair?: Partial<AnalytixToolArgumentRepairSettingsV1>
}

export type AnalytixTokenEconomySettingsPatchV1 = Partial<
  Omit<AnalytixTokenEconomySettingsV1, 'historyHygiene'>
> & {
  historyHygiene?: Partial<AnalytixHistoryHygieneSettingsV1>
}

export type AnalytixRuntimeSettingsPatchV1 = Partial<
  Omit<
    AnalytixRuntimeSettingsV1,
    'mcpSearch' | 'storage' | 'contextCompaction' | 'runtimeTuning' | 'tokenEconomy' | 'imageGeneration' | 'speechToText' | 'textToSpeech' | 'musicGeneration' | 'videoGeneration' | 'computerUse' | 'browserUse' | 'visionBridge' | 'quality' | 'modelProfiles' | 'modelCapabilityProbes' | 'subagents'
  >
> & {
  mcpSearch?: Partial<AnalytixMcpSearchSettingsV1>
  tokenEconomy?: AnalytixTokenEconomySettingsPatchV1
  storage?: Partial<AnalytixStorageSettingsV1>
  contextCompaction?: Partial<AnalytixContextCompactionSettingsV1>
  runtimeTuning?: AnalytixRuntimeTuningSettingsPatchV1
  imageGeneration?: Partial<AnalytixImageGenerationSettingsV1>
  speechToText?: Partial<AnalytixSpeechToTextSettingsV1>
  textToSpeech?: Partial<AnalytixTextToSpeechSettingsV1>
  musicGeneration?: Partial<AnalytixMusicGenerationSettingsV1>
  videoGeneration?: Partial<AnalytixVideoGenerationSettingsV1>
  computerUse?: Partial<AnalytixComputerUseSettingsV1>
  browserUse?: Partial<Omit<AnalytixBrowserUseSettingsV1, 'sitePermissions'>> & {
    sitePermissions?: Array<Partial<AnalytixBrowserSitePermissionV1> | null>
  }
  visionBridge?: Partial<AnalytixVisionBridgeSettingsV1>
  quality?: Partial<AnalytixDesignQualitySettingsV1>
  modelProfiles?: Record<string, ModelProviderModelProfilePatchV1 | null>
  modelCapabilityProbes?: Record<string, ModelCapabilityProbeResultV1 | null>
  subagents?: AnalytixSubagentsSettingsPatchV1
}

export type LogConfigV1 = {
  enabled: boolean
  retentionDays: number
}

export type NotificationConfigV1 = {
  turnComplete: boolean
}

export const WINDOW_CLOSE_ACTIONS = ['ask', 'tray', 'quit'] as const
export type WindowCloseAction = typeof WINDOW_CLOSE_ACTIONS[number]

export type AppBehaviorConfigV1 = {
  openAtLogin: boolean
  startMinimized: boolean
  closeAction?: WindowCloseAction
  /** Legacy compatibility field. New code should use closeAction. */
  closeToTray: boolean
}

export type ScheduleSkillSettingsV1 = {
  defaultNames: string[]
  extraDirs: string[]
  /**
   * Discovered skill roots the user turned off. Holds common-directory ids
   * (e.g. `global-codex`) and/or normalized absolute paths for custom dirs.
   */
  disabledDirs: string[]
}

export type ScheduledTaskScheduleV1 = {
  kind: ScheduleKind
  everyMinutes: number
  timeOfDay: string
  atTime: string
}

export type ScheduledTaskV1 = {
  id: string
  title: string
  enabled: boolean
  prompt: string
  workspaceRoot: string
  /** Optional Connect Phone channel whose persona/defaults should drive this scheduled task. */
  clawChannelId: string
  /** Selected model provider for this scheduled task. Empty means the current/default runtime provider. */
  providerId?: string
  model: string
  reasoningEffort: ScheduleReasoningEffort
  mode: ScheduleRunMode
  schedule: ScheduledTaskScheduleV1
  createdAt: string
  updatedAt: string
  lastRunAt: string
  nextRunAt: string
  lastStatus: ScheduleTaskStatus
  /**
   * Host-owned closed message key. The empty value is accepted only while
   * normalizing legacy/in-memory task drafts and is never written to disk.
   */
  lastMessage: ScheduleTaskMessageKey | ''
  lastThreadId: string
}

export type ScheduleInternalSettingsV1 = {
  port: number
  secret: string
}

export type ScheduleSettingsV1 = {
  enabled: boolean
  defaultWorkspaceRoot: string
  /** Default model provider used when creating scheduled tasks. Empty means the current/default runtime provider. */
  providerId?: string
  model: string
  mode: ScheduleRunMode
  promptPrefix: string
  skills: ScheduleSkillSettingsV1
  keepAwake: boolean
  internal: ScheduleInternalSettingsV1
  tasks: ScheduledTaskV1[]
}

export type ClawSkillSettingsV1 = {
  defaultNames: string[]
  extraDirs: string[]
  /**
   * Discovered skill roots the user turned off. Holds common-directory ids
   * (e.g. `global-codex`) and/or normalized absolute paths for custom dirs.
   */
  disabledDirs: string[]
  promptPrefix: string
}

export type ClawImSettingsV1 = {
  enabled: boolean
  provider: ClawImProvider
  port: number
  path: string
  secret: string
  weixinBridgeUrl: string
  workspaceRoot: string
  /** Default model provider for IM channels without their own provider. Empty inherits Analytix runtime provider. */
  providerId?: string
  model: string
  mode: ClawRunMode
  responseTimeoutMs: number
}

export type ClawTaskScheduleV1 = {
  kind: ClawScheduleKind
  everyMinutes: number
  timeOfDay: string
  atTime: string
}

export type ClawTaskV1 = ScheduledTaskV1

export type ClawImAgentProfileV1 = {
  name: string
  description: string
  identity: string
  personality: string
  userContext: string
  replyRules: string
}

export type ClawImPlatformAccountV1 =
  | { kind: 'feishu'; accountId: string; appId: string; domain: string; createdAt: string }
  | { kind: 'weixin'; accountId: string; createdAt: string }
  | {
      kind: 'telegram'
      accountId: string
      allowedChatIds: string
      botUsername?: string
      createdAt: string
    }

export type ClawImRemoteSessionV1 = {
  chatId: string
  messageId: string
  threadId: string
  senderId: string
  senderName: string
  updatedAt: string
}

export type ClawImConversationV1 = {
  id: string
  chatId: string
  remoteThreadId: string
  latestMessageId: string
  senderId: string
  senderName: string
  /** Analytix thread id this conversation maps to. */
  localThreadId: string
  workspaceRoot: string
  createdAt: string
  updatedAt: string
}

export type ClawImChannelV1 = {
  id: string
  provider: ClawImProvider
  label: string
  enabled: boolean
  /** Model provider used by this IM channel. Empty inherits the IM/global provider. */
  providerId?: string
  model: string
  /** Analytix thread id this channel maps to. */
  threadId: string
  workspaceRoot: string
  agentProfile: ClawImAgentProfileV1
  platformAccount?: ClawImPlatformAccountV1
  remoteSession?: ClawImRemoteSessionV1
  conversations: ClawImConversationV1[]
  /** When the one-time IM welcome/intro message was delivered. */
  welcomeSentAt?: string
  createdAt: string
  updatedAt: string
  /** When provider === 'feishu', stream agent replies into a Feishu / Lark markdown card. */
  feishuStream?: boolean
}

export type ClawSettingsV1 = {
  enabled: boolean
  skills: ClawSkillSettingsV1
  im: ClawImSettingsV1
  channels: ClawImChannelV1[]
  tasks: ClawTaskV1[]
}

export type WriteInlineCompletionSettingsV1 = {
  enabled: boolean
  retrievalEnabled: boolean
  longCompletionEnabled: boolean
  /** When true, Write inherits Analytix's selected provider instead of using `providerId`. */
  inheritProvider: boolean
  /** Selected provider for Write inline completion when `inheritProvider` is false. */
  providerId: string
  baseUrl: string
  /** When true, Write inherits Analytix's runtime model instead of using `model` as an override. */
  inheritModel: boolean
  model: string
  debounceMs: number
  longDebounceMs: number
  minAcceptScore: number
  longMinAcceptScore: number
  maxTokens: number
  longMaxTokens: number
}

/** 'edit' rewrites the selection in place; 'chat' hands it to the sidebar assistant. */
export type WriteQuickActionMode = 'edit' | 'chat'

export type WriteQuickActionV1 = {
  /** Stable identifier; built-in ids ('polish' | 'explain' | 'reformat') get localized fallbacks. */
  id: string
  /** Display label shown in the selection toolbar; empty = localized default for built-in ids. */
  label: string
  /** Prompt used for the edit/chat; empty = localized default for built-in ids. */
  prompt: string
  /** Whether the result rewrites the selection in place ('edit') or goes to the sidebar ('chat'). */
  mode: WriteQuickActionMode
}

export type WriteSelectionAssistSettingsV1 = {
  /** Custom infographic generation prompt prefix; empty = built-in default. */
  infographicPrompt: string
  /** Custom UI design mockup prompt prefix; empty = built-in default. */
  designDraftPrompt: string
  /** Custom interactive HTML prototype prompt; empty = built-in default. */
  prototypePrompt: string
  quickActions: WriteQuickActionV1[]
}

export type WriteFontPreset =
  | 'system'
  | 'sourceHanSans'
  | 'yahei'
  | 'pingfang'
  | 'simhei'
  | 'simsun'
  | 'kaiti'
  | 'custom'

export const WRITE_FONT_PRESETS: readonly WriteFontPreset[] = [
  'system',
  'sourceHanSans',
  'yahei',
  'pingfang',
  'simhei',
  'simsun',
  'kaiti',
  'custom'
] as const

export const WRITE_EDITOR_FONT_SIZE_MIN = 12
export const WRITE_EDITOR_FONT_SIZE_MAX = 28
export const DEFAULT_WRITE_EDITOR_FONT_SIZE_PX = 14
export const WRITE_EDITOR_LINE_HEIGHT_MIN = 1.4
export const WRITE_EDITOR_LINE_HEIGHT_MAX = 2.2
export const DEFAULT_WRITE_EDITOR_LINE_HEIGHT = 1.75
export type WriteTextAlign = 'left' | 'center' | 'right' | 'justify'
export const WRITE_TEXT_ALIGNS: readonly WriteTextAlign[] = ['left', 'center', 'right', 'justify'] as const

/**
 * Typography for the Write editor prose surfaces (rich editor, CodeMirror live
 * appearance, and the markdown preview). The raw source appearance keeps its
 * monospace family but still honors the configured size.
 */
export type WriteTypographySettingsV1 = {
  /** Named font preset; 'custom' uses `customFontFamily`. */
  fontPreset: WriteFontPreset
  /** CSS font-family stack used when `fontPreset === 'custom'`. */
  customFontFamily: string
  /** Base font size in px, clamped to [WRITE_EDITOR_FONT_SIZE_MIN, WRITE_EDITOR_FONT_SIZE_MAX]. */
  fontSizePx: number
  /** Unitless line-height, clamped to [WRITE_EDITOR_LINE_HEIGHT_MIN, WRITE_EDITOR_LINE_HEIGHT_MAX]. */
  lineHeight: number
  /** Legacy/global fallback; toolbar alignment now applies to selected blocks. */
  textAlign: WriteTextAlign
}

export const WRITE_AGENT_PRESET_MAX_COUNT = 12
export const WRITE_AGENT_PRESET_NAME_MAX_CHARS = 40
export const WRITE_AGENT_PERSONA_MAX_CHARS = 4000

/**
 * A named, reusable writing-assistant persona (plot coordinator, line editor,
 * foreshadowing tracker, continuity checker…). The persona text frames the
 * assistant for a specific creative role and can be switched per conversation.
 */
export type WriteAgentPresetV1 = {
  /** Stable id; built-in ids ('coordinator' | 'editor' | 'foreshadowing' | 'continuity') get localized name/persona fallbacks. */
  id: string
  /** Display name; empty = localized default for built-in ids. */
  name: string
  /** Short emoji/glyph badge shown in the switcher. */
  emoji: string
  /** Persona + behavior rules used to frame the assistant. Empty = localized default for built-in ids. */
  persona: string
}

export type WriteSettingsV1 = {
  defaultWorkspaceRoot: string
  activeWorkspaceRoot: string
  workspaces: string[]
  inlineCompletion: WriteInlineCompletionSettingsV1
  selectionAssist: WriteSelectionAssistSettingsV1
  typography: WriteTypographySettingsV1
  agentPresets: WriteAgentPresetV1[]
}

export type ClawSettingsPatchV1 = Partial<Omit<ClawSettingsV1, 'skills' | 'im' | 'channels' | 'tasks'>> & {
  skills?: Partial<ClawSkillSettingsV1>
  im?: Partial<ClawImSettingsV1>
  channels?: Array<Partial<ClawImChannelV1>>
  tasks?: Array<Partial<ClawTaskV1>>
}

export type ScheduleSettingsPatchV1 = Partial<
  Omit<ScheduleSettingsV1, 'skills' | 'internal' | 'tasks'>
> & {
  skills?: Partial<ScheduleSkillSettingsV1>
  internal?: Partial<ScheduleInternalSettingsV1>
  tasks?: Array<Partial<ScheduledTaskV1>>
}

export type WriteSettingsPatchV1 = Partial<Omit<WriteSettingsV1, 'inlineCompletion' | 'selectionAssist' | 'typography' | 'agentPresets'>> & {
  inlineCompletion?: Partial<WriteInlineCompletionSettingsV1>
  selectionAssist?: Partial<Omit<WriteSelectionAssistSettingsV1, 'quickActions'>> & {
    /** Replaced wholesale when present. */
    quickActions?: Array<Partial<WriteQuickActionV1>>
  }
  typography?: Partial<WriteTypographySettingsV1>
  /** Replaced wholesale when present. */
  agentPresets?: Array<Partial<WriteAgentPresetV1>>
}

export type ClawGeneratedFileV1 = {
  path: string
  relativePath?: string
  fileName: string
}

export type ClawRunResult =
  | {
      ok: true
      threadId: string
      turnId?: string
      text?: string
      message?: string
      files?: ClawGeneratedFileV1[]
      /**
       * Whether the watched turn finished within the response window.
       * `false` means it outran the IM timeout and is still running —
       * the caller should ack now and push the result when it finishes.
       * Absent on the fire-and-forget (no `waitForResult`) path.
       */
      completed?: boolean
    }
  | { ok: false; message: string }

export type ScheduleRunResult = ClawRunResult

export type ScheduleTaskFromTextResult =
  | { kind: 'noop' }
  | { kind: 'created'; taskId: string; title: string; scheduleAt: string; confirmationText: string }
  | { kind: 'error'; message: string }

export type ClawTaskFromTextResult = ScheduleTaskFromTextResult

export type ClawRuntimeStatus = {
  imServerRunning: boolean
  imUrl: string
  runningTaskIds: string[]
}

export type ScheduleRuntimeStatus = {
  internalServerRunning: boolean
  internalUrl: string
  runningTaskIds: string[]
  powerSaveBlockerActive: boolean
}

export type GuiUpdateConfigV1 = {
  channel: GuiUpdateChannel
}

export type AppSettingsV1 = {
  version: 1
  locale: 'en' | 'zh'
  theme: 'system' | 'light' | 'dark'
  uiFontScale: UiFontScale
  motionPreference?: AppMotionPreference
  cursorSpotlight?: boolean
  provider: ModelProviderSettingsV1
  runtime: AnalytixRuntimeSettingsV1
  workspaceRoot: string
  log: LogConfigV1
  notifications: NotificationConfigV1
  appBehavior: AppBehaviorConfigV1
  keyboardShortcuts: KeyboardShortcutsConfigV1
  write: WriteSettingsV1
  claw: ClawSettingsV1
  schedule: ScheduleSettingsV1
  guiUpdate: GuiUpdateConfigV1
  codePromptPrefix: string
  /** User-disabled skill IDs. Disabled skills are hidden from command surfaces. */
  disabledSkillIds: string[]
}

export type AppSettingsPatch = Partial<
  Omit<AppSettingsV1, 'provider' | 'runtime' | 'log' | 'notifications' | 'appBehavior' | 'keyboardShortcuts' | 'write' | 'claw' | 'schedule' | 'guiUpdate'>
> & {
  provider?: ModelProviderSettingsPatchV1
  runtime?: AnalytixRuntimeSettingsPatchV1
  log?: Partial<LogConfigV1>
  notifications?: Partial<NotificationConfigV1>
  appBehavior?: Partial<AppBehaviorConfigV1>
  keyboardShortcuts?: Partial<KeyboardShortcutsConfigV1>
  write?: WriteSettingsPatchV1
  claw?: ClawSettingsPatchV1
  schedule?: ScheduleSettingsPatchV1
  guiUpdate?: Partial<GuiUpdateConfigV1>
}
