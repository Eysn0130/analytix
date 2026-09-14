import type { WriteShutdownHandler } from './write-shutdown'
import type { ProviderEndpointKind } from './provider-display'
import type {
  AppSettingsPatch,
  AppSettingsV1,
  ClawRunResult,
  ClawTaskFromTextResult,
  ClawRuntimeStatus,
  ModelCapabilityProbeResultV1,
  ModelEndpointFormat,
  ModelProviderModelProfileV1,
  ScheduleReasoningEffort,
  ScheduleRunResult,
  ScheduleRuntimeStatus,
  ScheduleTaskFromTextResult
} from './app-settings'
import type { EditorListResult, EditorOpenResult, OpenEditorPathOptions } from './editor'
import type { GitBranchesResult } from './git-branches'
import type { GitCheckpointCreateResult, GitCheckpointRestoreResult } from './git-checkpoint'
import type {
  MergeResult,
  SyncResult,
  WorktreeChanges,
  WorktreeInfo,
  WorktreePoolStatus
} from './worktree'
import type {
  ThreadHandoffCompleteSwitchRequest,
  ThreadHandoffEvent,
  ThreadHandoffOperation,
  ThreadHandoffRequest,
  ThreadHandoffStartResult
} from './thread-handoff'
import type {
  GuiUpdateChannel,
  GuiUpdateDownloadResult,
  GuiUpdateInfo,
  GuiUpdateInstallResult,
  GuiUpdateState
} from './gui-update'
import type {
  ClipboardImageReadResult,
  LocalPdfTextReadResult,
  LocalPdfTextTarget,
  WorkspaceClipboardImageSavePayload,
  WorkspaceClipboardImageSaveResult,
  WorkspaceFileReadResult,
  WorkspaceFileSaveAsPayload,
  WorkspaceFileSaveAsResult,
  WorkspaceImageReadResult,
  WorkspacePdfReadResult,
  WorkspaceDirectoryCreatePayload,
  WorkspaceDirectoryCreateResult,
  WorkspaceDirectoryListResult,
  WorkspaceDirectoryTarget,
  WorkspaceEntryRenamePayload,
  WorkspaceEntryRenameResult,
  WorkspaceEntryDeletePayload,
  WorkspaceEntryDeleteResult,
  WorkspaceFileChangePayload,
  WorkspaceFileCreatePayload,
  WorkspaceFileCreateResult,
  WorkspaceFileResolveResult,
  WorkspaceFileTarget,
  WorkspaceFileWatchPayload,
  WorkspaceFileWatchResult,
  WorkspaceFileWritePayload,
  WorkspaceFileWriteResult
} from './workspace-file'
import type {
  WriteInlineCompletionDebugEntry,
  WriteInlineCompletionRequest,
  WriteInlineCompletionResult
} from './write-inline-completion'
import type {
  WriteInfographicRequest,
  WriteInfographicResult
} from './write-infographic'
import type {
  SpeechTranscriptionRequest,
  SpeechTranscriptionResult
} from './speech-to-text'
import type {
  UiPluginListItem,
  UiPluginManifestV1,
  UiPluginRuntimeFigures
} from './ui-plugin'
import type {
  WriteRetrievalRequest,
  WriteRetrievalResult
} from './write-retrieval'
import type {
  WriteExportPayload,
  WriteExportResult,
  WriteRichClipboardPayload,
  WriteRichClipboardResult
} from './write-export'
import type {
  TerminalCreatePayload,
  TerminalCreateResult,
  TerminalDataPayload,
  TerminalExitPayload,
  TerminalResizePayload,
  TerminalWritePayload
} from './terminal'
import type {
  BackgroundTaskActionPayload,
  BackgroundTaskListResult,
  BackgroundTaskMutationResult,
  BackgroundTaskOutputPayload,
  BackgroundTaskOutputResult,
  BackgroundTaskRegisterPayload,
  BackgroundTaskThreadPayload
} from './background-task'
import type { ThreadTraceEventPayload } from './thread-trace'
import type { DataAnalysisBridgeApi } from './data-analysis'
import type {
  HubAccountApiResult,
  HubAccountSnapshot,
  HubAuthChallenge,
  HubAuthChallengeMode,
  HubAuthChallengeState,
  HubAuthChallengeVerifyRequest,
  HubAuthChallengeVerifyResult,
  HubGatewayModelsResult,
  HubLoginRequest,
  HubPasswordResetConfirmRequest,
  HubProfileEventSyncRequest,
  HubProfileEventSyncResult,
  HubProfileLocalSyncResult,
  HubProfileUpdateRequest,
  HubProfileUpdateResult,
  HubReferralResult,
  HubRegisterRequest,
  HubUsageResult,
  HubVerificationCodeRequest
} from './hub-account'
import type { RuntimeStatusPublicV1 } from './analytix-runtime-status'
import type { PublicRuntimeSseRejectionReasonCode } from './public-runtime-sse'
import {
  CLEANING_DIFF_PREVIEW_FIELDS_V1,
  DIRECT_SOURCE_PREVIEW_FIELDS_V1,
  IMPORT_MAPPING_PREVIEW_FIELDS_V1
} from '../../packages/runtime/src/contracts/typed-local-data-surface.js'
import type {
  ProviderRegistryProbeResponseV1,
  ProviderRegistryOAuthBindingV1,
  ProviderRegistryPublicProviderV1,
  ProviderRegistryRequestV1,
  ProviderRegistryResultV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import type {
  AcceptedSlotDisplayRequestV1,
  AcceptedSlotDisplayResponseV1,
  CleaningDiffPreviewFieldV1,
  CleaningDiffPreviewRequestV1,
  CleaningDiffPreviewResponseV1,
  DirectSourcePreviewFieldV1,
  DirectSourcePreviewRequestV1,
  DirectSourcePreviewResponseV1,
  ImportMappingPreviewFieldV1,
  ImportMappingPreviewRequestV1,
  ImportMappingPreviewResponseV1,
  TypedLocalDataSurfaceDisplayModeV1,
  TypedLocalDataSurfaceKindV1,
  TypedLocalDataSurfaceResponseV1
} from '../../packages/runtime/src/contracts/typed-local-data-surface.js'

export type AnalytixRuntimeStatusPayload = RuntimeStatusPublicV1

export type RuntimeRequestResult = { ok: boolean; status: number; body: string }
export type ProviderRegistryRequest = ProviderRegistryRequestV1
export type ProviderRegistryResult = ProviderRegistryResultV1
export type ProviderRegistryPublicProvider = ProviderRegistryPublicProviderV1
export type ProviderRegistryProbeResponse = ProviderRegistryProbeResponseV1
export type LocalDisplayMode = TypedLocalDataSurfaceDisplayModeV1
export const IMPORT_MAPPING_PREVIEW_FIELDS = IMPORT_MAPPING_PREVIEW_FIELDS_V1
export const CLEANING_DIFF_PREVIEW_FIELDS = CLEANING_DIFF_PREVIEW_FIELDS_V1
export const DIRECT_SOURCE_PREVIEW_FIELDS = DIRECT_SOURCE_PREVIEW_FIELDS_V1
export type ImportMappingPreviewField = ImportMappingPreviewFieldV1
export type CleaningDiffPreviewField = CleaningDiffPreviewFieldV1
export type DirectSourcePreviewField = DirectSourcePreviewFieldV1
export type ImportMappingPreviewRequest = ImportMappingPreviewRequestV1
export type CleaningDiffPreviewRequest = CleaningDiffPreviewRequestV1
export type DirectSourcePreviewRequest = DirectSourcePreviewRequestV1
export type AcceptedSlotDisplayRequest = AcceptedSlotDisplayRequestV1
export type LocalDisplaySlot = AcceptedSlotDisplayResponseV1['slots'][number]
export type LocalDisplayKind = TypedLocalDataSurfaceKindV1
export type DirectSourcePreviewCell = DirectSourcePreviewResponseV1['rows'][number]['cells'][number]
export type DirectSourcePreviewRow = DirectSourcePreviewResponseV1['rows'][number]
export type ImportMappingPreviewResponse = ImportMappingPreviewResponseV1
export type CleaningDiffPreviewResponse = CleaningDiffPreviewResponseV1
export type DirectSourcePreviewResponse = DirectSourcePreviewResponseV1
export type AcceptedSlotDisplayResponse = AcceptedSlotDisplayResponseV1
export type LocalDisplayResponse = TypedLocalDataSurfaceResponseV1
export type LocalDisplayErrorCode =
  | 'invalid_request'
  | 'invalid_response'
  | 'runtime_unavailable'
  | 'runtime_error'
  | 'not_found'
  | 'forbidden'
  | 'conflict'
export type LocalDisplayFailure = {
  ok: false
  status: number
  code: LocalDisplayErrorCode
  message: string
}
export type LocalDisplayResult = LocalDisplayResponse | LocalDisplayFailure
export type FundsImportStatus = 'ready' | 'mapping_invalid'
export type FundsImportSourceItem = {
  selector: string
  sourceIndex: number
  sourceCount: number
  sourceLabel: string
  rowCount: number
  columnCount: number
  status: FundsImportStatus
}
export type FundsCSVSnapshotStageResult =
  | { ok: true; status: FundsImportStatus; totalRowCount: number; items: FundsImportSourceItem[] }
  | { ok: false; canceled: boolean; code: 'forbidden' | 'invalid_source' | 'runtime_unavailable' | 'capability_unavailable'; message: string }
export type FundsCSVSnapshotConfirmResult =
  | { ok: true; rowCount: number }
  | { ok: false; code: 'forbidden' | 'invalid_source' | 'runtime_unavailable'; message: string }
export type FundsImportCancelResult =
  | { ok: true; canceled: true }
  | { ok: false; code: 'forbidden' | 'invalid_source' | 'runtime_unavailable'; message: string }
export type FundsImportStatusResult =
  | { ok: true; status: FundsImportStatus; totalRowCount: number; item: FundsImportSourceItem }
  | { ok: false; code: 'forbidden' | 'invalid_source' | 'runtime_unavailable'; message: string }
export type FundsDeterministicCleaningResult =
  | {
      ok: true
      status: 'committed'
      selector: string
      ruleGeneration: string
      ruleDigest: string
      inputSnapshot: string
      outputSnapshot: string
      transformLineage: string
      rowCount: number
      changedRowCount: number
    }
  | {
      ok: false
      status: 'outcome_unknown' | 'unavailable'
      code: 'outcome_unknown' | 'pre_cas_failed' | 'forbidden' | 'runtime_unavailable'
      message: string
      rowCount?: number
      changedRowCount?: number
    }
export type FundsCleaningDiffRevokeResult =
  | { ok: true; revoked: true }
  | { ok: false; code: 'forbidden' | 'runtime_unavailable'; message: string }
export type WorkspacePickResult = { canceled: boolean; path: string | null }
export type LocalFilesPickResult = { canceled: boolean; paths: string[] }
export type PathOpenResult = { ok: boolean; message?: string }
export const DESKTOP_COMMANDS = [
  'undo',
  'redo',
  'cut',
  'copy',
  'paste',
  'selectAll',
  'reload',
  'zoomIn',
  'zoomOut',
  'resetZoom',
  'toggleDevTools',
  'minimize',
  'toggleMaximize',
  'toggleFullscreen',
  'close',
  'quit'
] as const
export type DesktopCommand = typeof DESKTOP_COMMANDS[number]
export type SkillSaveResult = { ok: true; path: string } | { ok: false; message: string }
export type SkillDeleteResult = { ok: true; path: string } | { ok: false; message: string }
export type SkillListItem = {
  id: string
  name: string
  description?: string
  root: string
  entryPath: string
  scope: 'project' | 'global'
  legacy: boolean
}
export type SkillListResult =
  | { ok: true; skills: SkillListItem[]; validationErrors: Array<{ root: string; message: string }> }
  | { ok: false; message: string }
export type SkillRootListItem = {
  id: string
  disableKey: string
  path: string
  scope: 'project' | 'global'
  source: 'common' | 'extra'
  labelKey?: string
  exists: boolean
  enabled: boolean
  skillCount: number
}
export type SkillRootListResult =
  | { ok: true; roots: SkillRootListItem[] }
  | { ok: false; message: string }
export type UiPluginListIpcResult = { plugins: UiPluginListItem[] }
export type UiPluginInstallIpcResult =
  | { canceled: true }
  | { canceled: false; ok: true; plugin: UiPluginListItem }
  | { canceled: false; ok: false; errors: string[] }
export type UiPluginLoadIpcResult =
  | { ok: true; manifest: UiPluginManifestV1; figures: UiPluginRuntimeFigures }
  | { ok: false; error: string }
export type HubAgentPluginPlatform = 'mac-arm64' | 'mac-x64' | 'win'
export type HubAgentMarketplaceSyncMode = 'cache' | 'catalog' | 'full'
export type HubAgentPluginSource = {
  source?: string
  archiveType?: string
  url?: string
  sha256?: string
  sizeBytes?: number
  fileName?: string
  path?: string
}
export type HubAgentPluginListItem = {
  name: string
  marketplaceName: string
  upstreamMarketplaceName: string
  pluginName: string
  platform: HubAgentPluginPlatform
  version: string
  displayName: string
  shortDescription: string
  category: string
  developerName: string
  source: HubAgentPluginSource
  policy: Record<string, unknown>
  manifest: Record<string, unknown>
  marketplaceEntry: Record<string, unknown>
  mcpServerIds: string[]
  iconUrl?: string
  iconDataUrl?: string
  brandColor?: string
  requiresCodexConnector?: boolean
  requiredInstall: boolean
  installed: boolean
}
export type HubAgentSkillListItem = {
  id: string
  skillName: string
  platform: HubAgentPluginPlatform | 'all'
  displayName: string
  shortDescription: string
  scope: 'system' | 'user' | 'plugin'
  sourceKind: 'runtime-system' | 'runtime-user' | 'plugin'
  upstreamMarketplaceName: string
  pluginName: string
  version: string
  category: string
  iconUrl?: string
  iconDataUrl?: string
  skillPath: string
  requiredInstall: boolean
  installed: boolean
}
export type HubAgentSkillMarkdownRequest = {
  id?: string
  skillName: string
  displayName?: string
  pluginName?: string
  version?: string
  upstreamMarketplaceName?: string
  skillPath?: string
  sourceKind?: HubAgentSkillListItem['sourceKind']
}
export type HubAgentSkillMarkdownResult =
  | {
      ok: true
      path: string
      content: string
      source: 'managed-skill' | 'plugin-cache'
    }
  | {
      ok: false
      message: string
    }
export type HubAgentPluginMutationRequest = {
  pluginName: string
  version?: string
  upstreamMarketplaceName?: string
}
export type HubAgentPluginMutationResult =
  | {
      ok: true
      pluginName: string
      version: string
      path?: string
    }
  | {
      ok: false
      message: string
      managed?: boolean
    }
export type HubAgentMarketplaceSyncResult =
  | {
      ok: true
      platform: HubAgentPluginPlatform | ''
      marketplaceName: string
      marketplaceRoot: string
      generatedAt: string
      changed: boolean
      plugins: HubAgentPluginListItem[]
      skills: HubAgentSkillListItem[]
      requiredPluginNames: string[]
      requiredSkillNames: string[]
      copiedSkills: string[]
      removedSkills: string[]
      errors: string[]
    }
  | {
      ok: false
      message: string
      platform?: HubAgentPluginPlatform | ''
      marketplaceRoot?: string
      plugins?: HubAgentPluginListItem[]
      skills?: HubAgentSkillListItem[]
      errors?: string[]
    }
export type AnalytixRuntimeConfigFileResult = { path: string; content: string; exists: boolean }
export type AnalytixRuntimeConfigSaveResult = { ok: true; path: string }
export type TurnCompleteNotificationPayload = {
  threadId?: string
  title: string
  body: string
}
export type SystemNotificationResult =
  | { ok: true; shown: boolean; reason?: string }
  | { ok: false; message: string }
export type ThreadTraceRecordResult =
  | { ok: true; path: string }
  | { ok: false; message: string }
export type ClawChannelActivityPayload = {
  channelId: string
  threadId: string
}
export type ClawChannelMirrorResult =
  | { ok: true }
  | { ok: false; message: string }
export type UpstreamModelsResult =
  | { ok: true; modelIds: string[]; defaultModelId?: string; modelGroups?: ModelProviderModelGroup[] }
  | { ok: false; message: string }
export type ModelProviderModelGroup = {
  providerId: string
  label: string
  endpointKind?: ProviderEndpointKind
  modelIds: string[]
  /** Display-only labels projected from the committed provider endpoint. */
  modelLabels?: Record<string, string>
  modelProfiles?: Record<string, ModelProviderModelProfileV1>
}
export type ModelCapabilityProbeRequest = {
  providerId: string
  model: string
  baseUrl: string
  endpointFormat: ModelEndpointFormat
}
export type ModelCapabilityProbeResult = ModelCapabilityProbeResultV1 & {
  ok: boolean
  latencyMs?: number
  message?: string
}
export type ClawImInstallQrResult =
  | { ok: true; url: string; deviceCode: string; userCode: string; interval: number; expireIn: number }
  | { ok: false; message: string }
export type ClawImInstallPollResult =
  | { done: true; kind: 'feishu'; appId: string; domain: string; channelId: string; credentialConfigured: true; settingsCommitted: true }
  | { done: true; kind: 'weixin'; accountId: string; channelId: string; credentialConfigured: true; settingsCommitted: true }
  | { done: false; error?: string }
export type ClawImTelegramConnectErrorCode = 'invalid_format' | 'rejected' | 'network' | 'unknown'
export type ClawImTelegramConnectResult =
  | { ok: true; botId: number; botUsername: string; botFirstName: string; channelId: string; credentialConfigured: true; settingsCommitted: true }
  | { ok: false; code: ClawImTelegramConnectErrorCode; message: string }
export type ConfirmDialogOptions = {
  message: string
  detail?: string
  confirmLabel?: string
  cancelLabel?: string
}
/** Which legacy install a set of importable conversations came from. */
export type LegacySessionSourceKind = 'analytix' | 'coreagent' | 'kun' | 'custom'
export type LegacySessionDetectedSource = {
  id: string
  kind: LegacySessionSourceKind
  /** Absolute path to the legacy threads directory. */
  path: string
  /** Conversation folders found in this source. */
  threadCount: number
  /** Folders not already present in the destination (would be newly imported). */
  newCount: number
}
export type LegacySessionDetectResult = {
  /** Destination threads directory (current Analytix data dir + /threads). */
  destDir: string
  sources: LegacySessionDetectedSource[]
}
export type LegacySessionImportSourceSummary = {
  path: string
  total: number
  imported: number
  skipped: number
}
export type LegacySessionImportSummary = {
  destDir: string
  /** Conversation folders seen across all sources. */
  total: number
  /** Folders copied into the destination this run. */
  imported: number
  /** Folders skipped because they already existed (or failed to copy). */
  skipped: number
  sources: LegacySessionImportSourceSummary[]
}
export type LegacySessionImportResult =
  | ({ ok: true } & LegacySessionImportSummary)
  | { ok: false; message: string }
export type PublicRuntimeSseEvent = Record<string, unknown> & {
  kind?: string
  seq?: number
}
/** One IPC message carries every host-filtered SSE event parsed from a network chunk. */
export type SseEventPayload = { streamId: string; events: PublicRuntimeSseEvent[] }
export type SseEndPayload = { streamId: string }
export type SseErrorPayload = {
  streamId: string
  status?: number
  code?: string
  message?: string
  reasonCode?: PublicRuntimeSseRejectionReasonCode
}
/** Exact renderer receipt binding required for an accepted-final terminal ACK. */
export type AcceptedFinalSseAckBindingV1 = Readonly<{
  batchId: string
  threadId: string
  turnId: string
  publicationCommitId: string
}>
export type ComputerUsePermissionKind = 'accessibility' | 'screenRecording'
export type ComputerUsePermissionState = 'granted' | 'denied' | 'unknown'
export type ComputerUsePermissions = {
  platform: string
  supported: boolean
  needsPermission: boolean
  accessibility: ComputerUsePermissionState
  screenRecording: ComputerUsePermissionState
  /** Accessibility is enabled in System Settings but needs an app relaunch to take effect. */
  accessibilityNeedsRestart: boolean
}
export type ComputerUseDoctorCheck = {
  id: string
  label: string
  status: 'passed' | 'warning' | 'failed' | 'unknown'
  message: string
}
export type ComputerUseDoctorResult = {
  platform: string
  backend: {
    preferred: 'analytix-computer-use'
    selected: 'analytix-computer-use' | 'nut-js'
    available: boolean
    reason?: string
  }
  checks: ComputerUseDoctorCheck[]
}
export type ChromeBrowserUseConnectionState =
  | 'connected'
  | 'disconnected'
  | 'configuredNotVerified'
  | 'extensionMissing'
  | 'extensionDisabled'
  | 'nativeHostMissing'
  | 'nativeHostInvalid'
  | 'diagnosticsUnavailable'
  | 'unsupported'

export type ChromeBrowserUseExtensionProfile = {
  profileDirectory: string
  profilePath: string
  preferencesPath: string | null
  extensionPath: string
  selected: boolean
  installed: boolean
  registered: boolean
  enabled: boolean
  disabled: boolean
  state: number | null
  disableReasons: unknown[]
  versions: string[]
}

export type ChromeBrowserUseExtensionStatus = {
  status: 'enabled' | 'disabled' | 'missing' | 'unknown' | 'error'
  extensionId: string
  userDataDirectory?: string
  selectedProfileDirectory?: string
  selectedProfilePath?: string
  profiles: ChromeBrowserUseExtensionProfile[]
  problem?: string
}

export type ChromeBrowserUseNativeHostStatus = {
  status: 'configured' | 'missing' | 'invalid' | 'unsupported' | 'error'
  expectedHostName: string
  expectedExtensionId: string
  expectedOrigin: string
  manifestPath?: string
  registryKey?: string | null
  registryManifestPath?: string | null
  actualHostName?: string
  actualType?: string
  actualHostPath?: string
  hostPathExists?: boolean
  allowedOrigins?: string[]
  problem?: string
}

export type ChromeBrowserUseStatus = {
  platform: string
  checkedAt: string
  extensionId: string
  nativeHostName: string
  connected: boolean
  state: ChromeBrowserUseConnectionState
  reason?: string
  extension: ChromeBrowserUseExtensionStatus
  nativeHost: ChromeBrowserUseNativeHostStatus
  connectionProbe: {
    status: 'unavailable' | 'passed' | 'failed'
    browserClientPath?: string
    problem?: string
  }
}
export type ChromeBrowserUseExtensionPageTarget = 'webstore' | 'settings'
export type QueryCacheInvalidatePayload = {
  queryKey: string[]
  sourceClientId?: string
}

export type OAuthAccountPublicResult =
  | {
      ok: true
      authorizationId: string
      expiresAt: string
      phase: 'pending' | 'processing'
    }
  | {
      ok: false
      code: 'invalid_request' | 'unavailable' | 'forbidden' | 'conflict'
      message: string
    }

export type OAuthAccountActionResult =
  | { ok: true; cancelled?: true }
  | Extract<OAuthAccountPublicResult, { ok: false }>

/**
 * Main-owned protected recovery results are intentionally a bounded status
 * projection. Artifact bytes, paths, local bindings, and credential refs stay
 * in the Main process and the operation-specific Go client.
 */
export type ProviderCredentialRecoveryResult =
  | {
      ok: true
      status: 'request-created' | 'bundle-created' | 'receipt-created' | 'finalized'
      entryCount?: number
    }
  | {
      ok: false
      code:
        | 'invalid_context'
        | 'invalid_path'
        | 'cancelled'
        | 'invalid_request'
        | 'invalid_response'
        | 'runtime_unavailable'
        | 'conflict'
        | 'verification_failure'
    }

/** Internal preload IPC map. Renderer code must use AnalytixDomainFacade domains instead. */
export type AnalytixFlatApi = {
  platform: string
  startupSurfaceReady: () => void
  getSettings: () => Promise<AppSettingsV1>
  setSettings: (partial: AppSettingsPatch) => Promise<AppSettingsV1>
  saveSettingsSilent: (partial: AppSettingsPatch) => Promise<AppSettingsV1>
  getHubAccountSnapshot: () => Promise<HubAccountSnapshot>
  refreshHubAccount: () => Promise<HubAccountSnapshot>
  loginHubAccount: (request: HubLoginRequest) => Promise<HubAccountSnapshot>
  registerHubAccount: (request: HubRegisterRequest) => Promise<HubAccountSnapshot>
  logoutHubAccount: () => Promise<HubAccountSnapshot>
  sendHubRegisterVerificationCode: (
    request: HubVerificationCodeRequest
  ) => Promise<HubAccountApiResult<{ expiresAt: string; retryAfterSeconds: number }>>
  sendHubPasswordResetVerificationCode: (
    request: HubVerificationCodeRequest
  ) => Promise<HubAccountApiResult<{ expiresAt: string; retryAfterSeconds: number }>>
  confirmHubPasswordReset: (
    request: HubPasswordResetConfirmRequest
  ) => Promise<HubAccountApiResult<Record<string, unknown>>>
  fetchHubAuthChallenge: (mode: HubAuthChallengeMode) => Promise<HubAccountApiResult<{ challenge: HubAuthChallenge }>>
  fetchHubAuthChallengeState: (
    mode: HubAuthChallengeMode
  ) => Promise<HubAccountApiResult<{ state: HubAuthChallengeState }>>
  verifyHubAuthChallenge: (
    request: HubAuthChallengeVerifyRequest
  ) => Promise<HubAccountApiResult<{ verification: HubAuthChallengeVerifyResult }>>
  updateHubProfile: (request: HubProfileUpdateRequest) => Promise<HubAccountApiResult<HubProfileUpdateResult>>
  syncHubProfileEvents: (request: HubProfileEventSyncRequest) => Promise<HubAccountApiResult<HubProfileEventSyncResult>>
  syncHubLocalProfileEvents: () => Promise<HubAccountApiResult<HubProfileLocalSyncResult>>
  getHubUsage: () => Promise<HubAccountApiResult<HubUsageResult>>
  getHubReferral: () => Promise<HubAccountApiResult<HubReferralResult>>
  getHubModels: () => Promise<HubAccountApiResult<HubGatewayModelsResult>>
  providerRegistryRequest: (request: ProviderRegistryRequest) => Promise<ProviderRegistryResult>
  createDestinationRequest: () => Promise<ProviderCredentialRecoveryResult>
  createSourceBundle: () => Promise<ProviderCredentialRecoveryResult>
  applyDestinationBundle: () => Promise<ProviderCredentialRecoveryResult>
  finalizeSourceReceipt: () => Promise<ProviderCredentialRecoveryResult>
  configureProviderOAuth: (request: {
    providerId: string
    oauthBinding: ProviderRegistryOAuthBindingV1
  }) => Promise<OAuthAccountActionResult>
  beginProviderOAuth: (request: { providerId: string }) => Promise<OAuthAccountPublicResult>
  statusProviderOAuth: (request: { authorizationId: string }) => Promise<OAuthAccountPublicResult>
  cancelProviderOAuth: (request: { authorizationId: string }) => Promise<OAuthAccountActionResult>
  revokeProviderOAuth: (request: { providerId: string }) => Promise<OAuthAccountActionResult>
  deleteProviderOAuth: (request: { providerId: string }) => Promise<OAuthAccountActionResult>
  replaceProviderOAuthSubscription: (request: {
    providerId: string
    subscriptionToken: string
  }) => Promise<OAuthAccountActionResult>
  beginMcpOAuth: (request: { serverId: string; accountId: string }) => Promise<OAuthAccountPublicResult>
  statusMcpOAuth: (request: { authorizationId: string }) => Promise<OAuthAccountPublicResult>
  cancelMcpOAuth: (request: { authorizationId: string }) => Promise<OAuthAccountActionResult>
  revokeMcpOAuth: (request: { serverId: string; accountId: string }) => Promise<OAuthAccountActionResult>
  deleteMcpOAuth: (request: { serverId: string; accountId: string }) => Promise<OAuthAccountActionResult>
  beginExtensionOAuth: (request: {
    pluginId: string
    serverId: string
    accountId: string
  }) => Promise<OAuthAccountPublicResult>
  statusExtensionOAuth: (request: { authorizationId: string }) => Promise<OAuthAccountPublicResult>
  cancelExtensionOAuth: (request: { authorizationId: string }) => Promise<OAuthAccountActionResult>
  revokeExtensionOAuth: (request: {
    pluginId: string
    serverId: string
    accountId: string
  }) => Promise<OAuthAccountActionResult>
  deleteExtensionOAuth: (request: {
    pluginId: string
    serverId: string
    accountId: string
  }) => Promise<OAuthAccountActionResult>
  runtimeRequest: (path: string, method?: string, body?: string) => Promise<RuntimeRequestResult>
  importMappingPreview: (request: ImportMappingPreviewRequest) => Promise<LocalDisplayResult>
  cleaningDiffPreview: (request: CleaningDiffPreviewRequest) => Promise<LocalDisplayResult>
  directSourcePreview: (request: DirectSourcePreviewRequest) => Promise<LocalDisplayResult>
  acceptedSlotDisplay: (request: AcceptedSlotDisplayRequest) => Promise<LocalDisplayResult>
  stageFundsCSVSnapshot: () => Promise<FundsCSVSnapshotStageResult>
  confirmFundsCSVSnapshot: (selector: string) => Promise<FundsCSVSnapshotConfirmResult>
  cancelFundsCSVImport: (selector: string) => Promise<FundsImportCancelResult>
  statusFundsCSVImport: (selector: string) => Promise<FundsImportStatusResult>
  runDeterministicFundsCleaning: () => Promise<FundsDeterministicCleaningResult>
  revokeCleaningDiffPreview: (selector: string) => Promise<FundsCleaningDiffRevokeResult>
  restartRuntime: () => Promise<void>
  fetchUpstreamModels: () => Promise<UpstreamModelsResult>
  probeModelCapabilities: (payload: ModelCapabilityProbeRequest) => Promise<ModelCapabilityProbeResult>
  getClawStatus: () => Promise<ClawRuntimeStatus>
  runClawTask: (taskId: string) => Promise<ClawRunResult>
  getScheduleStatus: () => Promise<ScheduleRuntimeStatus>
  runScheduleTask: (taskId: string) => Promise<ScheduleRunResult>
  startClawImInstallQr: (
    provider: 'feishu' | 'weixin',
    options?: { isLark?: boolean }
  ) => Promise<ClawImInstallQrResult>
  pollClawImInstall: (
    provider: 'feishu' | 'weixin',
    deviceCode: string
  ) => Promise<ClawImInstallPollResult>
  connectTelegramBot: (
    botToken: string,
    allowedChatIds?: string
  ) => Promise<ClawImTelegramConnectResult>
  disconnectClawImChannel: (channelId: string) => Promise<AppSettingsV1>
  pickWorkspaceDirectory: (defaultPath?: string) => Promise<WorkspacePickResult>
  pickLocalFiles: (defaultPath?: string) => Promise<LocalFilesPickResult>
  confirmDialog: (options: ConfirmDialogOptions) => Promise<boolean>
  /** Detect importable conversations from a legacy product install. */
  detectLegacySessions: () => Promise<LegacySessionDetectResult>
  /** Import legacy conversations; omit sourceDir to import all auto-detected sources. */
  importLegacySessions: (sourceDir?: string) => Promise<LegacySessionImportResult>
  /** Open a directory picker for choosing a legacy conversations folder. */
  pickLegacySessionDir: () => Promise<WorkspacePickResult>
  listSkills: (workspaceRoot?: string) => Promise<SkillListResult>
  listSkillRoots: (workspaceRoot?: string) => Promise<SkillRootListResult>
  saveSkillFile: (rootPath: string, skillName: string, content: string) => Promise<SkillSaveResult>
  deleteSkill: (rootPath: string, skillName: string) => Promise<SkillDeleteResult>
  openSkillRoot: (rootPath: string) => Promise<PathOpenResult>
  listUiPlugins: () => Promise<UiPluginListIpcResult>
  installUiPlugin: () => Promise<UiPluginInstallIpcResult>
  removeUiPlugin: (id: string) => Promise<{ ok: boolean }>
  loadUiPlugin: (id: string) => Promise<UiPluginLoadIpcResult>
  syncHubAgentMarketplace: (options?: {
    forceRefresh?: boolean
    mode?: HubAgentMarketplaceSyncMode
  }) => Promise<HubAgentMarketplaceSyncResult>
  installHubAgentPlugin: (request: HubAgentPluginMutationRequest) => Promise<HubAgentPluginMutationResult>
  uninstallHubAgentPlugin: (request: HubAgentPluginMutationRequest) => Promise<HubAgentPluginMutationResult>
  readHubAgentSkillMarkdown: (request: HubAgentSkillMarkdownRequest) => Promise<HubAgentSkillMarkdownResult>
  getAnalytixConfigFile: () => Promise<AnalytixRuntimeConfigFileResult>
  setAnalytixConfigFile: (content: string) => Promise<AnalytixRuntimeConfigSaveResult>
  openAnalytixConfigDir: () => Promise<PathOpenResult>
  getGitBranches: (workspaceRoot: string) => Promise<GitBranchesResult>
  switchGitBranch: (workspaceRoot: string, branch: string) => Promise<GitBranchesResult>
  createAndSwitchGitBranch: (workspaceRoot: string, branch: string) => Promise<GitBranchesResult>
  createGitCheckpoint: (payload: { workspaceRoot: string; threadId: string; timeoutMs?: number }) => Promise<GitCheckpointCreateResult>
  restoreGitCheckpoint: (payload: { checkpointId: string; allowPartialRestore?: boolean }) => Promise<GitCheckpointRestoreResult>
  acquireWorktree: (params: {
    projectPath: string
    poolIndex: number
    taskId: string
    force?: boolean
    worktreeRoot?: string
  }) => Promise<WorktreeInfo>
  releaseWorktree: (params: { projectPath: string; poolIndex: number }) => Promise<void>
  listWorktrees: (params: { projectPath: string; worktreeRoot?: string }) => Promise<WorktreePoolStatus>
  removeWorktree: (params: {
    projectPath: string
    poolIndex: number
    worktreeRoot?: string
  }) => Promise<void>
  getWorktreeChanges: (params: { worktreePath: string }) => Promise<WorktreeChanges>
  commitWorktree: (params: { worktreePath: string; message: string }) => Promise<string>
  mergeWorktree: (params: {
    projectPath: string
    poolIndex: number
    commitMessage?: string
    worktreeRoot?: string
  }) => Promise<MergeResult>
  abortWorktreeMerge: (params: { projectPath: string }) => Promise<void>
  continueWorktreeMerge: (params: { projectPath: string; message?: string }) => Promise<MergeResult>
  syncWorktreeFromMain: (params: {
    projectPath: string
    poolIndex: number
    worktreeRoot?: string
  }) => Promise<SyncResult>
  abortWorktreeRebase: (params: { worktreePath: string }) => Promise<void>
  cleanupWorktrees: (params: { projectPath: string; worktreeRoot?: string }) => Promise<void>
  findAvailableWorktreePoolIndex: (params: {
    projectPath: string
    worktreeRoot?: string
  }) => Promise<number | null>
  startThreadHandoff: (request: ThreadHandoffRequest) => Promise<ThreadHandoffStartResult>
  retryThreadHandoff: (params: { operationId: string }) => Promise<ThreadHandoffStartResult>
  getThreadHandoffOperations: (params?: { operationId?: string }) => Promise<{ operations: ThreadHandoffOperation[] }>
  cancelThreadHandoff: (params: { operationId: string }) => Promise<{ operation: ThreadHandoffOperation }>
  removeThreadHandoff: (params: { operationId: string }) => Promise<{ removed: boolean }>
  completeThreadHandoffSwitch: (
    params: ThreadHandoffCompleteSwitchRequest
  ) => Promise<{ operation: ThreadHandoffOperation }>
  failThreadHandoffSwitch: (
    params: { operationId: string; message: string }
  ) => Promise<{ operation: ThreadHandoffOperation }>
  onThreadHandoffEvent: (handler: (event: ThreadHandoffEvent) => void) => () => void
  listEditors: () => Promise<EditorListResult>
  openEditorPath: (options: OpenEditorPathOptions) => Promise<EditorOpenResult>
  listWorkspaceDirectory: (options: WorkspaceDirectoryTarget) => Promise<WorkspaceDirectoryListResult>
  resolveWorkspaceFile: (options: WorkspaceFileTarget) => Promise<WorkspaceFileResolveResult>
  readWorkspaceFile: (options: WorkspaceFileTarget) => Promise<WorkspaceFileReadResult>
  readWorkspaceImage: (options: WorkspaceFileTarget) => Promise<WorkspaceImageReadResult>
  readWorkspacePdf: (options: WorkspaceFileTarget) => Promise<WorkspacePdfReadResult>
  readLocalPdfText: (options: LocalPdfTextTarget) => Promise<LocalPdfTextReadResult>
  saveWorkspaceFileAs: (payload: WorkspaceFileSaveAsPayload) => Promise<WorkspaceFileSaveAsResult>
  writeWorkspaceFile: (payload: WorkspaceFileWritePayload) => Promise<WorkspaceFileWriteResult>
  createWorkspaceFile: (payload: WorkspaceFileCreatePayload) => Promise<WorkspaceFileCreateResult>
  createWorkspaceDirectory: (
    payload: WorkspaceDirectoryCreatePayload
  ) => Promise<WorkspaceDirectoryCreateResult>
  saveWorkspaceClipboardImage: (
    payload: WorkspaceClipboardImageSavePayload
  ) => Promise<WorkspaceClipboardImageSaveResult>
  readClipboardImage: () => Promise<ClipboardImageReadResult>
  getPathForFile: (file: File) => string
  renameWorkspaceEntry: (
    payload: WorkspaceEntryRenamePayload
  ) => Promise<WorkspaceEntryRenameResult>
  deleteWorkspaceEntry: (
    payload: WorkspaceEntryDeletePayload
  ) => Promise<WorkspaceEntryDeleteResult>
  watchWorkspaceFile: (payload: WorkspaceFileWatchPayload) => Promise<WorkspaceFileWatchResult>
  unwatchWorkspaceFile: (watchId: string) => Promise<boolean>
  onWorkspaceFileChanged: (handler: (payload: WorkspaceFileChangePayload) => void) => () => void
  requestWriteInlineCompletion: (
    payload: WriteInlineCompletionRequest
  ) => Promise<WriteInlineCompletionResult>
  retrieveWriteContext: (
    payload: WriteRetrievalRequest
  ) => Promise<WriteRetrievalResult>
  generateWriteInfographic: (
    payload: WriteInfographicRequest
  ) => Promise<WriteInfographicResult>
  authorizeWritePrototype: (payload: {
    path: string
    workspaceRoot: string
  }) => Promise<
    { ok: true; absolutePath: string; fileUrl: string } | { ok: false; message: string }
  >
  openWritePrototype: (payload: {
    path: string
    workspaceRoot: string
  }) => Promise<{ ok: boolean; message?: string }>
  transcribeSpeech: (
    payload: SpeechTranscriptionRequest
  ) => Promise<SpeechTranscriptionResult>
  listWriteInlineCompletionDebugEntries: () => Promise<WriteInlineCompletionDebugEntry[]>
  clearWriteInlineCompletionDebugEntries: () => Promise<boolean>
  exportWriteDocument: (payload: WriteExportPayload) => Promise<WriteExportResult>
  copyWriteDocumentAsRichText: (
    payload: WriteRichClipboardPayload
  ) => Promise<WriteRichClipboardResult>
  startSse: (threadId: string, sinceSeq: number, streamId?: string) => Promise<{ streamId: string }>
  ackSseEvent: (
    streamId: string,
    seq: number,
    acceptedFinal?: AcceptedFinalSseAckBindingV1
  ) => Promise<boolean>
  stopSse: (streamId: string) => Promise<boolean>
  onSseEvent: (handler: (payload: SseEventPayload) => void) => () => void
  onSseEnd: (handler: (payload: SseEndPayload) => void) => () => void
  onSseError: (handler: (payload: SseErrorPayload) => void) => () => void
  onClawChannelActivity: (handler: (payload: ClawChannelActivityPayload) => void) => () => void
  onRuntimeStatus: (handler: (payload: AnalytixRuntimeStatusPayload) => void) => () => void
  mirrorClawChannelMessage: (
    threadId: string,
    text: string,
    direction: 'user' | 'assistant'
  ) => Promise<ClawChannelMirrorResult>
  mirrorClawChannelMessageToFeishu: (
    threadId: string,
    text: string,
    direction: 'user' | 'assistant'
  ) => Promise<ClawChannelMirrorResult>
  createClawTaskFromText: (
    text: string,
    options?: { channelId?: string; providerId?: string; modelHint?: string; reasoningEffort?: ScheduleReasoningEffort; mode?: 'agent' | 'plan' }
  ) => Promise<ClawTaskFromTextResult>
  createScheduleTaskFromText: (
    text: string,
    options?: { workspaceRoot?: string; clawChannelId?: string; providerId?: string; modelHint?: string; reasoningEffort?: ScheduleReasoningEffort; mode?: 'agent' | 'plan' }
  ) => Promise<ScheduleTaskFromTextResult>
  runDesktopCommand: (command: DesktopCommand) => Promise<void>
  openThreadInNewWindow: (threadId: string) => Promise<void>
  openExternal: (url: string) => Promise<void>
  getComputerUsePermissions: () => Promise<ComputerUsePermissions>
  getComputerUseDoctor: () => Promise<ComputerUseDoctorResult>
  getChromeBrowserUseStatus: () => Promise<ChromeBrowserUseStatus>
  openChromeBrowserUseExtensionPage: (
    target: ChromeBrowserUseExtensionPageTarget
  ) => Promise<PathOpenResult>
  requestComputerUsePermission: (
    kind: ComputerUsePermissionKind
  ) => Promise<ComputerUsePermissions>
  invalidateQueryCache: (payload: QueryCacheInvalidatePayload) => Promise<boolean>
  onQueryCacheInvalidated: (handler: (payload: QueryCacheInvalidatePayload) => void) => () => void
  showTurnCompleteNotification: (
    payload: TurnCompleteNotificationPayload
  ) => Promise<SystemNotificationResult>
  getAppVersion: () => Promise<string>
  getGuiUpdateState: () => Promise<GuiUpdateState>
  checkGuiUpdate: (channel?: GuiUpdateChannel) => Promise<GuiUpdateInfo>
  downloadGuiUpdate: (channel?: GuiUpdateChannel) => Promise<GuiUpdateDownloadResult>
  installGuiUpdate: () => Promise<GuiUpdateInstallResult>
  onGuiUpdateState: (handler: (payload: GuiUpdateState) => void) => () => void
  logError: (category: string, message: string, detail?: unknown) => Promise<void>
  getLogPath: () => Promise<string>
  openLogDir: () => Promise<{ ok: boolean; message?: string }>
  recordThreadTrace: (event: ThreadTraceEventPayload) => Promise<ThreadTraceRecordResult>
  createTerminal: (payload: TerminalCreatePayload) => Promise<TerminalCreateResult>
  writeToTerminal: (payload: TerminalWritePayload) => Promise<boolean>
  resizeTerminal: (payload: TerminalResizePayload) => Promise<boolean>
  disposeTerminal: (sessionId: string) => Promise<boolean>
  onTerminalData: (handler: (payload: TerminalDataPayload) => void) => () => void
  onTerminalExit: (handler: (payload: TerminalExitPayload) => void) => () => void
  registerBackgroundTask: (payload: BackgroundTaskRegisterPayload) => Promise<BackgroundTaskMutationResult>
  listBackgroundTasks: (payload: BackgroundTaskThreadPayload) => Promise<BackgroundTaskListResult>
  snapshotBackgroundTasks: (payload: BackgroundTaskThreadPayload) => Promise<BackgroundTaskListResult>
  killBackgroundTask: (payload: BackgroundTaskActionPayload) => Promise<BackgroundTaskMutationResult>
  restartBackgroundTask: (payload: BackgroundTaskActionPayload) => Promise<BackgroundTaskMutationResult>
  readBackgroundTaskOutput: (payload: BackgroundTaskOutputPayload) => Promise<BackgroundTaskOutputResult>
}

export type AnalytixSettingsApi = Pick<
  AnalytixFlatApi,
  'getSettings' | 'setSettings' | 'saveSettingsSilent'
>

export type AnalytixAccountApi = {
  getSnapshot: AnalytixFlatApi['getHubAccountSnapshot']
  refresh: AnalytixFlatApi['refreshHubAccount']
  login: AnalytixFlatApi['loginHubAccount']
  register: AnalytixFlatApi['registerHubAccount']
  logout: AnalytixFlatApi['logoutHubAccount']
  sendRegisterVerificationCode: AnalytixFlatApi['sendHubRegisterVerificationCode']
  sendPasswordResetVerificationCode: AnalytixFlatApi['sendHubPasswordResetVerificationCode']
  confirmPasswordReset: AnalytixFlatApi['confirmHubPasswordReset']
  fetchChallenge: AnalytixFlatApi['fetchHubAuthChallenge']
  fetchChallengeState: AnalytixFlatApi['fetchHubAuthChallengeState']
  verifyChallenge: AnalytixFlatApi['verifyHubAuthChallenge']
  updateProfile: AnalytixFlatApi['updateHubProfile']
  syncProfileEvents: AnalytixFlatApi['syncHubProfileEvents']
  syncLocalProfileEvents: AnalytixFlatApi['syncHubLocalProfileEvents']
  getUsage: AnalytixFlatApi['getHubUsage']
  getReferral: AnalytixFlatApi['getHubReferral']
  getModels: AnalytixFlatApi['getHubModels']
}

export type AnalytixRuntimeApi = Pick<
  AnalytixFlatApi,
  | 'runtimeRequest'
  | 'restartRuntime'
  | 'fetchUpstreamModels'
  | 'probeModelCapabilities'
  | 'startSse'
  | 'ackSseEvent'
  | 'stopSse'
  | 'onSseEvent'
  | 'onSseEnd'
  | 'onSseError'
  | 'onRuntimeStatus'
  | 'getAnalytixConfigFile'
  | 'setAnalytixConfigFile'
  | 'openAnalytixConfigDir'
> & Pick<
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

export type AnalytixProviderRegistryApi = {
  request: AnalytixFlatApi['providerRegistryRequest']
}

export type AnalytixProviderCredentialRecoveryApi = {
  createDestinationRequest: AnalytixFlatApi['createDestinationRequest']
  createSourceBundle: AnalytixFlatApi['createSourceBundle']
  applyDestinationBundle: AnalytixFlatApi['applyDestinationBundle']
  finalizeSourceReceipt: AnalytixFlatApi['finalizeSourceReceipt']
}

export type AnalytixProviderOAuthApi = {
  configure: AnalytixFlatApi['configureProviderOAuth']
  begin: AnalytixFlatApi['beginProviderOAuth']
  status: AnalytixFlatApi['statusProviderOAuth']
  cancel: AnalytixFlatApi['cancelProviderOAuth']
  revoke: AnalytixFlatApi['revokeProviderOAuth']
  delete: AnalytixFlatApi['deleteProviderOAuth']
  replaceSubscription: AnalytixFlatApi['replaceProviderOAuthSubscription']
}

export type AnalytixMcpOAuthApi = {
  begin: AnalytixFlatApi['beginMcpOAuth']
  status: AnalytixFlatApi['statusMcpOAuth']
  cancel: AnalytixFlatApi['cancelMcpOAuth']
  revoke: AnalytixFlatApi['revokeMcpOAuth']
  delete: AnalytixFlatApi['deleteMcpOAuth']
}

export type AnalytixExtensionOAuthApi = {
  begin: AnalytixFlatApi['beginExtensionOAuth']
  status: AnalytixFlatApi['statusExtensionOAuth']
  cancel: AnalytixFlatApi['cancelExtensionOAuth']
  revoke: AnalytixFlatApi['revokeExtensionOAuth']
  delete: AnalytixFlatApi['deleteExtensionOAuth']
}

export type AnalytixConnectPhoneApi = {
  getStatus: AnalytixFlatApi['getClawStatus']
  runTask: AnalytixFlatApi['runClawTask']
  startImInstallQr: AnalytixFlatApi['startClawImInstallQr']
  pollImInstall: AnalytixFlatApi['pollClawImInstall']
  connectTelegramBot: AnalytixFlatApi['connectTelegramBot']
  disconnectImChannel: AnalytixFlatApi['disconnectClawImChannel']
  onChannelActivity: AnalytixFlatApi['onClawChannelActivity']
  mirrorChannelMessage: AnalytixFlatApi['mirrorClawChannelMessage']
  mirrorChannelMessageToFeishu: AnalytixFlatApi['mirrorClawChannelMessageToFeishu']
  createTaskFromText: AnalytixFlatApi['createClawTaskFromText']
}

export type AnalytixScheduleApi = {
  getStatus: AnalytixFlatApi['getScheduleStatus']
  runTask: AnalytixFlatApi['runScheduleTask']
  createTaskFromText: AnalytixFlatApi['createScheduleTaskFromText']
}

export type AnalytixWorkspaceApi = {
  pickDirectory: AnalytixFlatApi['pickWorkspaceDirectory']
  getGitBranches: AnalytixFlatApi['getGitBranches']
  switchGitBranch: AnalytixFlatApi['switchGitBranch']
  createAndSwitchGitBranch: AnalytixFlatApi['createAndSwitchGitBranch']
  createGitCheckpoint: AnalytixFlatApi['createGitCheckpoint']
  restoreGitCheckpoint: AnalytixFlatApi['restoreGitCheckpoint']
  acquireWorktree: AnalytixFlatApi['acquireWorktree']
  releaseWorktree: AnalytixFlatApi['releaseWorktree']
  listWorktrees: AnalytixFlatApi['listWorktrees']
  removeWorktree: AnalytixFlatApi['removeWorktree']
  getWorktreeChanges: AnalytixFlatApi['getWorktreeChanges']
  commitWorktree: AnalytixFlatApi['commitWorktree']
  mergeWorktree: AnalytixFlatApi['mergeWorktree']
  abortWorktreeMerge: AnalytixFlatApi['abortWorktreeMerge']
  continueWorktreeMerge: AnalytixFlatApi['continueWorktreeMerge']
  syncWorktreeFromMain: AnalytixFlatApi['syncWorktreeFromMain']
  abortWorktreeRebase: AnalytixFlatApi['abortWorktreeRebase']
  cleanupWorktrees: AnalytixFlatApi['cleanupWorktrees']
  findAvailableWorktreePoolIndex: AnalytixFlatApi['findAvailableWorktreePoolIndex']
  startThreadHandoff: AnalytixFlatApi['startThreadHandoff']
  retryThreadHandoff: AnalytixFlatApi['retryThreadHandoff']
  getThreadHandoffOperations: AnalytixFlatApi['getThreadHandoffOperations']
  cancelThreadHandoff: AnalytixFlatApi['cancelThreadHandoff']
  removeThreadHandoff: AnalytixFlatApi['removeThreadHandoff']
  completeThreadHandoffSwitch: AnalytixFlatApi['completeThreadHandoffSwitch']
  failThreadHandoffSwitch: AnalytixFlatApi['failThreadHandoffSwitch']
  onThreadHandoffEvent: AnalytixFlatApi['onThreadHandoffEvent']
  listEditors: AnalytixFlatApi['listEditors']
  openEditorPath: AnalytixFlatApi['openEditorPath']
}

export type AnalytixFilesApi = {
  listDirectory: AnalytixFlatApi['listWorkspaceDirectory']
  resolve: AnalytixFlatApi['resolveWorkspaceFile']
  read: AnalytixFlatApi['readWorkspaceFile']
  readImage: AnalytixFlatApi['readWorkspaceImage']
  readPdf: AnalytixFlatApi['readWorkspacePdf']
  readLocalPdfText: AnalytixFlatApi['readLocalPdfText']
  saveAs: AnalytixFlatApi['saveWorkspaceFileAs']
  write: AnalytixFlatApi['writeWorkspaceFile']
  createFile: AnalytixFlatApi['createWorkspaceFile']
  createDirectory: AnalytixFlatApi['createWorkspaceDirectory']
  saveClipboardImage: AnalytixFlatApi['saveWorkspaceClipboardImage']
  readClipboardImage: AnalytixFlatApi['readClipboardImage']
  pickLocalFiles: AnalytixFlatApi['pickLocalFiles']
  renameEntry: AnalytixFlatApi['renameWorkspaceEntry']
  deleteEntry: AnalytixFlatApi['deleteWorkspaceEntry']
  watch: AnalytixFlatApi['watchWorkspaceFile']
  unwatch: AnalytixFlatApi['unwatchWorkspaceFile']
  onChanged: AnalytixFlatApi['onWorkspaceFileChanged']
  getPathForFile: AnalytixFlatApi['getPathForFile']
}

export type AnalytixWriteApi = Pick<
  AnalytixFlatApi,
  | 'requestWriteInlineCompletion'
  | 'retrieveWriteContext'
  | 'generateWriteInfographic'
  | 'authorizeWritePrototype'
  | 'openWritePrototype'
  | 'listWriteInlineCompletionDebugEntries'
  | 'clearWriteInlineCompletionDebugEntries'
  | 'exportWriteDocument'
  | 'copyWriteDocumentAsRichText'
> & { onShutdown: (handler: WriteShutdownHandler) => () => void }

export type AnalytixSpeechApi = {
  transcribe: AnalytixFlatApi['transcribeSpeech']
}

export type AnalytixTerminalApi = {
  create: AnalytixFlatApi['createTerminal']
  write: AnalytixFlatApi['writeToTerminal']
  resize: AnalytixFlatApi['resizeTerminal']
  dispose: AnalytixFlatApi['disposeTerminal']
  onData: AnalytixFlatApi['onTerminalData']
  onExit: AnalytixFlatApi['onTerminalExit']
}

export type AnalytixBackgroundTasksApi = {
  register: AnalytixFlatApi['registerBackgroundTask']
  list: AnalytixFlatApi['listBackgroundTasks']
  snapshot: AnalytixFlatApi['snapshotBackgroundTasks']
  kill: AnalytixFlatApi['killBackgroundTask']
  restart: AnalytixFlatApi['restartBackgroundTask']
  output: AnalytixFlatApi['readBackgroundTaskOutput']
}

export type AnalytixUpdatesApi = {
  getState: AnalytixFlatApi['getGuiUpdateState']
  check: AnalytixFlatApi['checkGuiUpdate']
  download: AnalytixFlatApi['downloadGuiUpdate']
  install: AnalytixFlatApi['installGuiUpdate']
  onState: AnalytixFlatApi['onGuiUpdateState']
}

export type AnalytixLogsApi = {
  error: AnalytixFlatApi['logError']
  getPath: AnalytixFlatApi['getLogPath']
  openDir: AnalytixFlatApi['openLogDir']
}

export type AnalytixAppApi = {
  platform: AnalytixFlatApi['platform']
  startupSurfaceReady: AnalytixFlatApi['startupSurfaceReady']
  confirmDialog: AnalytixFlatApi['confirmDialog']
  runDesktopCommand: AnalytixFlatApi['runDesktopCommand']
  openThreadInNewWindow: AnalytixFlatApi['openThreadInNewWindow']
  openExternal: AnalytixFlatApi['openExternal']
  getComputerUsePermissions: AnalytixFlatApi['getComputerUsePermissions']
  getComputerUseDoctor: AnalytixFlatApi['getComputerUseDoctor']
  getChromeBrowserUseStatus: AnalytixFlatApi['getChromeBrowserUseStatus']
  openChromeBrowserUseExtensionPage: AnalytixFlatApi['openChromeBrowserUseExtensionPage']
  requestComputerUsePermission: AnalytixFlatApi['requestComputerUsePermission']
  invalidateQueryCache: AnalytixFlatApi['invalidateQueryCache']
  onQueryCacheInvalidated: AnalytixFlatApi['onQueryCacheInvalidated']
  showTurnCompleteNotification: AnalytixFlatApi['showTurnCompleteNotification']
  getVersion: AnalytixFlatApi['getAppVersion']
  listSkills: AnalytixFlatApi['listSkills']
  listSkillRoots: AnalytixFlatApi['listSkillRoots']
  saveSkillFile: AnalytixFlatApi['saveSkillFile']
  deleteSkill: AnalytixFlatApi['deleteSkill']
  openSkillRoot: AnalytixFlatApi['openSkillRoot']
  listUiPlugins: AnalytixFlatApi['listUiPlugins']
  installUiPlugin: AnalytixFlatApi['installUiPlugin']
  removeUiPlugin: AnalytixFlatApi['removeUiPlugin']
  loadUiPlugin: AnalytixFlatApi['loadUiPlugin']
  syncHubAgentMarketplace: AnalytixFlatApi['syncHubAgentMarketplace']
  installHubAgentPlugin: AnalytixFlatApi['installHubAgentPlugin']
  uninstallHubAgentPlugin: AnalytixFlatApi['uninstallHubAgentPlugin']
  readHubAgentSkillMarkdown: AnalytixFlatApi['readHubAgentSkillMarkdown']
}

export type AnalytixDiagnosticsApi = {
  detectLegacySessions: AnalytixFlatApi['detectLegacySessions']
  importLegacySessions: AnalytixFlatApi['importLegacySessions']
  pickLegacySessionDir: AnalytixFlatApi['pickLegacySessionDir']
  listWriteInlineCompletionDebugEntries: AnalytixFlatApi['listWriteInlineCompletionDebugEntries']
  clearWriteInlineCompletionDebugEntries: AnalytixFlatApi['clearWriteInlineCompletionDebugEntries']
  logError: AnalytixFlatApi['logError']
  getLogPath: AnalytixFlatApi['getLogPath']
  runtimeRequest: AnalytixFlatApi['runtimeRequest']
  recordThreadTrace: AnalytixFlatApi['recordThreadTrace']
}

export type AnalytixDomainFacade = {
  office: {
    onAnnotationInputFreeze: (handler: (frozen: boolean) => void) => () => void
    onMenuRequested: (handler: (target: import('./native-office').NativeOfficeMenuTarget) => void) => () => void
    showActionMenu: (request: import('./native-office').NativeOfficeActionMenu) => Promise<{actionId:string | null}>
    onWorkspaceCommand: (handler: (command: import('./native-office').NativeWorkspaceCommand) => void) => () => void
    pickFile: (request: import('zod').infer<typeof import('./native-office').nativeOfficePickerRequestSchema>) => Promise<import('zod').infer<typeof import('./native-office').nativeOfficePickerResponseSchema>>
    request: (request: import('./native-office').NativeOfficeRequest) => Promise<import('./native-office').NativeOfficeResponse>
    onChange: (handler: (view: import('./native-office').NativeOfficeView | null) => void) => () => void
  }
  packageHost: {
    request: (request: import('../../packages/runtime/src/contracts/plugin-package-host').PluginPackageHostRequest) => Promise<import('../../packages/runtime/src/contracts/plugin-package-host').PluginPackageHostResponse>
  }
  objects: {
    resolveArtifact: (request: import('../../packages/runtime/src/contracts/generated-artifact').GeneratedArtifactRequest) => Promise<import('../../packages/runtime/src/contracts/generated-artifact').GeneratedArtifactResponse>
    request: (request: import('../../packages/runtime/src/contracts/object-editing').ObjectEditingRequest) => Promise<import('../../packages/runtime/src/contracts/object-editing').ObjectEditingResponse>
  }
  settings: AnalytixSettingsApi
  account: AnalytixAccountApi
  providerRegistry: AnalytixProviderRegistryApi
  providerCredentialRecovery: AnalytixProviderCredentialRecoveryApi
  providerOAuth: AnalytixProviderOAuthApi
  mcpOAuth: AnalytixMcpOAuthApi
  extensionOAuth: AnalytixExtensionOAuthApi
  runtime: AnalytixRuntimeApi
  connectPhone: AnalytixConnectPhoneApi
  schedule: AnalytixScheduleApi
  workspace: AnalytixWorkspaceApi
  files: AnalytixFilesApi
  write: AnalytixWriteApi
  speech: AnalytixSpeechApi
  terminal: AnalytixTerminalApi
  backgroundTasks: AnalytixBackgroundTasksApi
  updates: AnalytixUpdatesApi
  logs: AnalytixLogsApi
  app: AnalytixAppApi
  diagnostics: AnalytixDiagnosticsApi
  dataAnalysis: DataAnalysisBridgeApi
}

export type AnalytixApi = AnalytixDomainFacade
