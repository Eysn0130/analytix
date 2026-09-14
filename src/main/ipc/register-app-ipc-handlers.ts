import type { PrivateMediaRuntimeRequest } from '../services/private-media-runtime-request'
import { createObjectEditingHandler } from './object-editing-ipc'
import { createPluginPackageHostHandler } from './plugin-package-host-ipc'
import { app, BrowserWindow, dialog, ipcMain, shell, type IpcMainInvokeEvent, type WebContents } from 'electron'
import { watch, type FSWatcher } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { homedir } from 'node:os'
import { basename, dirname, extname, join, resolve } from 'node:path'
import { copyFile, mkdir, readFile, writeFile } from 'node:fs/promises'
import { z } from 'zod'
import type { ClawPlatformInstallPollResult } from '../claw-platform-install'
import { atomicWriteFile } from '../../../packages/runtime/src/adapters/file/atomic-write.js'
import {
  parseProviderRegistryPortableManifestV1,
  PROVIDER_REGISTRY_FAILURE_MESSAGES_V1,
  type AccountCredentialScopeV1
} from '../../../packages/runtime/src/contracts/provider-registry.js'
import {
  type AppSettingsPatch,
  type AppSettingsV1,
  type ClawImChannelV1,
  type ClawRunResult,
  type ClawTaskFromTextResult,
  type ClawRuntimeStatus,
  type ScheduleRunResult,
  type ScheduleRuntimeStatus,
  type ScheduleTaskFromTextResult
} from '../../shared/app-settings'
import type {
  AcceptedSlotDisplayRequest,
  CleaningDiffPreviewRequest,
  ClawImInstallQrResult,
  DesktopCommand,
  DirectSourcePreviewRequest,
  FundsCleaningDiffRevokeResult,
  FundsCSVSnapshotConfirmResult,
  FundsCSVSnapshotStageResult,
  FundsDeterministicCleaningResult,
  FundsImportCancelResult,
  FundsImportStatusResult,
  ImportMappingPreviewRequest,
  LocalDisplayFailure,
  LocalDisplayKind,
  LocalDisplayResponse,
  LocalDisplayResult,
  LocalFilesPickResult,
  RuntimeRequestResult,
  SystemNotificationResult,
  TurnCompleteNotificationPayload,
  UpstreamModelsResult,
  WorkspacePickResult
} from '../../shared/analytix-api'
import { runtimeErrorToError } from '../../shared/runtime-error'
import type { WorkspaceFileSaveAsResult } from '../../shared/workspace-file'
import type { GuiUpdateDownloadResult, GuiUpdateInfo, GuiUpdateInstallResult, GuiUpdateState } from '../../shared/gui-update'
import {
  clawMirrorPayloadSchema,
  clawImInstallPollPayloadSchema,
  clawImTelegramTokenPayloadSchema,
  confirmDialogPayloadSchema,
  clawTaskFromTextPayloadSchema,
  acceptedSlotDisplayRequestSchema,
  cleaningDiffPreviewRequestSchema,
  chromeBrowserUseExtensionPageTargetSchema,
  computerUsePermissionKindSchema,
  deepseekConfigContentSchema,
  desktopCommandSchema,
  defaultPathSchema,
  gitCheckpointCreatePayloadSchema,
  gitCheckpointRestorePayloadSchema,
  gitBranchPayloadSchema,
  guiUpdateChannelSchema,
  logErrorPayloadSchema,
  localPdfTextTargetPayloadSchema,
  notificationPayloadSchema,
  openThreadWindowPayloadSchema,
  openEditorPathPayloadSchema,
  providerCapabilityProbePayloadSchema,
  providerOAuthBeginPayloadSchema,
  providerOAuthConfigurePayloadSchema,
  mcpOAuthBeginPayloadSchema,
  extensionOAuthBeginPayloadSchema,
  oauthAuthorizationIdPayloadSchema,
  providerOAuthSubscriptionPayloadSchema,
  queryCacheInvalidatePayloadSchema,
  rootPathSchema,
  worktreeCommitSchema,
  hubAgentSkillMarkdownPayloadSchema,
  hubAgentPluginMutationPayloadSchema,
  hubAgentMarketplaceSyncPayloadSchema,
  hubAuthChallengeModeSchema,
  hubAuthChallengeVerifyPayloadSchema,
  hubLoginPayloadSchema,
  hubPasswordResetConfirmPayloadSchema,
  hubProfileEventSyncPayloadSchema,
  hubProfileUpdatePayloadSchema,
  hubRegisterPayloadSchema,
  hubVerificationCodePayloadSchema,
  worktreeContinueMergeSchema,
  worktreeMergeSchema,
  worktreePoolIndexSchema,
  worktreePoolSchema,
  worktreeProjectPathSchema,
  worktreeOptionalRootSchema,
  worktreePathSchema,
  threadHandoffCompleteSwitchPayloadSchema,
  threadHandoffFailSwitchPayloadSchema,
  threadHandoffGetPayloadSchema,
  threadHandoffOperationIdPayloadSchema,
  threadHandoffStartPayloadSchema,
  runtimeRequestPayloadSchema,
  directSourcePreviewRequestSchema,
  importMappingPreviewRequestSchema,
  localDisplayResponseSchema,
  scheduleTaskFromTextPayloadSchema,
  shellOpenExternalUrlSchema,
  skillDeletePayloadSchema,
  skillListPayloadSchema,
  skillSaveFilePayloadSchema,
  settingsPatchSchema,
  streamIdSchema,
  uiPluginIdPayloadSchema,
  workspaceDirectoryCreatePayloadSchema,
  workspaceClipboardImageSavePayloadSchema,
  workspaceDirectoryTargetPayloadSchema,
  workspaceEntryDeletePayloadSchema,
  workspaceEntryRenamePayloadSchema,
  workspaceFileCreatePayloadSchema,
  workspaceFileSaveAsPayloadSchema,
  workspaceFileTargetPayloadSchema,
  workspaceFileWatchPayloadSchema,
  workspaceFileWritePayloadSchema,
  speechTranscribePayloadSchema,
  threadTraceEventPayloadSchema,
  writeExportPayloadSchema,
  writeRichClipboardPayloadSchema,
  writeInfographicPayloadSchema,
  writeInlineCompletionPayloadSchema,
  writePrototypeFilePayloadSchema,
  writeRetrievalPayloadSchema,
  workspaceRootSchema,
  legacySessionImportPayloadSchema
} from './app-ipc-schemas'
import { DEFAULT_ANALYTIX_DATA_DIR, resolveAnalytixRuntimeSettings } from '../../shared/app-settings'
import { detectLegacySessions, importLegacySessions } from '../services/legacy-session-import-service'
import type { JsonSettingsStore } from '../settings-store'
import type { ImChannelAccountLifecycle } from '../im-channel-lifecycle'
import type { InstalledExtensionAccountLifecycle } from '../extension-account-lifecycle'
import type { MainOAuthAccountAuthority } from '../provider-oauth-main-authority'
import { probeModelCapabilities } from '../provider-connection'
import type { ClawRuntime } from '../claw-runtime'
import type { ScheduleRuntime } from '../schedule-runtime'
import { sanitizeRuntimeResponse } from '../runtime/analytix-adapter'
import { verifyTelegramBotToken } from '../telegram-runtime'
import { createAndSwitchGitBranch, getGitBranches, switchGitBranch } from '../services/git-service'
import { createGitCheckpoint, restoreGitCheckpoint } from '../services/git-checkpoint-service'
import {
  abortMerge,
  abortRebase,
  acquireWorktree,
  cleanupWorktrees,
  commitWorktree,
  continueMerge,
  findAvailablePoolIndex,
  getWorktreeChanges,
  listWorktrees,
  mergeWorktreeToMain,
  releaseWorktree,
  removeWorktree,
  syncWorktreeFromMain
} from '../services/worktree-service'
import { threadHandoffService } from '../services/thread-handoff-service'
import {
  installUiPluginFromDirectory,
  listUiPlugins,
  loadUiPluginFigures,
  removeUiPlugin
} from '../services/ui-plugin-service'
import { ensureBundledUiPlugins } from '../ui-plugin-bundled'
import {
  createWorkspaceDirectory,
  createWorkspaceFile,
  deleteWorkspaceEntry,
  expandHomePath,
  listEditorsResult,
  listWorkspaceDirectory,
  openEditorPath,
  openPathWithShell,
  readClipboardImage,
  readWorkspaceImage,
  readWorkspaceFile,
  readWorkspacePdf,
  renameWorkspaceEntry,
  resolveOpenTargetPath,
  resolveWorkspaceFile,
  saveWorkspaceClipboardImage,
  writeWorkspaceFile
} from '../services/workspace-service'
import {
  clearWriteInlineCompletionDebugEntries,
  listWriteInlineCompletionDebugEntries,
  requestWriteInlineCompletion
} from '../services/write-inline-completion-service'
import { retrieveWriteContext } from '../services/write-retrieval-service'
import { readLocalPdfText } from '../services/write-pdf-text-service'
import {
  requestWriteInfographic,
  type PrivateMediaImageRequest,
  type PrivateMediaImageResult
} from '../services/write-infographic-service'
import { authorizePrototypePath } from '../services/prototype-embed-registry'
import {
  requestSpeechTranscription,
  type PrivateMediaSpeechRequest,
  type PrivateMediaSpeechResult
} from '../services/speech-to-text-service'
import {
  getComputerUseDoctor,
  getComputerUsePermissions,
  requestComputerUsePermission
} from '../services/computer-use-permissions'
import {
  getChromeBrowserUseStatus,
  openChromeBrowserUseExtensionPage
} from '../services/chrome-browser-use-service'
import { copyWriteDocumentAsRichText, exportWriteDocument } from '../services/write-export-service'
import { deleteGuiSkillPackage, listGuiSkillRoots, listGuiSkills, saveGuiSkillFile } from '../services/skill-service'
import { appendThreadTraceEvent, pruneThreadTraces } from '../services/thread-trace-service'
import { canonicalPath } from '../services/workspace-paths'
import {
  createProviderRegistryIpcHandler
} from './provider-registry-ipc'
import { createProtectedRecoveryRuntimeClient } from './provider-registry-ipc'
import {
  createMainProviderCredentialRecoveryAuthority,
  type ProtectedRecoveryInvokeEvent
} from '../provider-credential-recovery'
import type {
  ProtectedRecoveryImportReceipt,
  ProtectedRecoveryOwnerBinding,
  ProtectedRecoveryRuntimeRequest,
  ProtectedRecoveryRuntimeResult
} from './provider-registry-ipc'

type GuiUpdaterModule = typeof import('../gui-updater')
type HubAccountService = import('../services/hub-account-service').HubAccountService
type LoadHubAccountService = () => Promise<HubAccountService>
type HubAgentMarketplaceServiceModule = typeof import('../services/hub-agent-marketplace-service')

type WorkspaceFileWatchRecord = {
  watcher: FSWatcher
  sender: WebContents
  path: string
  workspaceRoot: string
  timer: ReturnType<typeof setTimeout> | null
}

type RegisterAppIpcHandlersOptions = {
  store: JsonSettingsStore
  loadHubAccountService: LoadHubAccountService
  getMainWindow: () => BrowserWindow | null
  isTrustedProviderRegistrySender?: (event: IpcMainInvokeEvent) => boolean
  applySettingsPatch: (partial: AppSettingsPatch) => Promise<AppSettingsV1>
  saveSettingsPatch: (partial: AppSettingsPatch) => Promise<AppSettingsV1>
  runtimeRequest: (
    path: string,
    method?: string,
    body?: string
  ) => Promise<RuntimeRequestResult>
  /** Main-only alias; never exposed through the renderer runtime request API. */
  protectedRuntimeRequest?: ProtectedRecoveryRuntimeRequest
  getPortableImportReceipt?: () => ProtectedRecoveryImportReceipt | null | Promise<ProtectedRecoveryImportReceipt | null>
  getCurrentPortableManifest?: () => string | Uint8Array | null | Promise<string | Uint8Array | null>
  getCurrentOwnerBindingInventory?: () => ProtectedRecoveryOwnerBinding[] | Promise<ProtectedRecoveryOwnerBinding[]>
  getCurrentProfile?: () => { profileBinding: string; dataDirectory: string } | null | Promise<{ profileBinding: string; dataDirectory: string } | null>
  /** Fixed Main-only media transport; absent injection must fail closed. */
  privateMediaRequest?: PrivateMediaRuntimeRequest
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>
  restartRuntime: () => Promise<void>
  fetchUpstreamModels: () => Promise<UpstreamModelsResult>
  getClawRuntime: () => ClawRuntime | null
  getScheduleRuntime: () => ScheduleRuntime | null
  startFeishuInstallQrcode: (isLark: boolean) => Promise<ClawImInstallQrResult>
  pollFeishuInstall: (deviceCode: string) => Promise<ClawPlatformInstallPollResult>
  startWeixinInstallQrcode: (weixinBridgeUrl?: string) => Promise<ClawImInstallQrResult>
  pollWeixinInstall: (deviceCode: string, weixinBridgeUrl?: string) => Promise<ClawPlatformInstallPollResult>
  imChannelAccountLifecycle: ImChannelAccountLifecycle
  installedExtensionAccountLifecycle: InstalledExtensionAccountLifecycle
  providerOAuthAccountManagement: MainOAuthAccountAuthority
  resolveAnalytixConfigPath: () => string
  onAnalytixMcpConfigWritten?: (path: string, content: string) => Promise<void> | void
  showTurnCompleteNotification: (
    payload: TurnCompleteNotificationPayload
  ) => Promise<SystemNotificationResult>
  openThreadInNewWindow: (threadId: string) => void
  getAppVersion: () => string
  readGuiUpdateState: () => Promise<GuiUpdateState>
  loadGuiUpdaterModule: () => Promise<GuiUpdaterModule>
  resolveLogDirectory: () => string
  logError: (category: string, message: string, detail?: unknown) => void
}

function accountCredentialScopeForClawImChannel(
  channel: ClawImChannelV1
): AccountCredentialScopeV1 | null {
  const account = channel.platformAccount
  if (!account) return null
  switch (account.kind) {
    case 'telegram':
      return {
        owner: 'transport', provider: 'telegram', accountId: account.accountId,
        channelId: channel.id, purpose: 'transport-telegram-bot-token'
      }
    case 'weixin':
      return {
        owner: 'transport', provider: 'weixin', accountId: account.accountId,
        channelId: channel.id, purpose: 'transport-weixin-session-key'
      }
    case 'feishu':
      return {
        owner: 'transport', provider: 'feishu', accountId: account.accountId,
        channelId: channel.id, purpose: 'transport-feishu-app-secret'
      }
  }
}

function mainOwnedConnectedImChannel(
  channelId: string,
  platformAccount: NonNullable<ClawImChannelV1['platformAccount']>
): ClawImChannelV1 {
  const provider = platformAccount.kind
  const now = new Date().toISOString()
  return {
    id: channelId,
    provider,
    label: provider === 'telegram' ? 'Telegram' : provider === 'weixin' ? 'WeChat' : 'Feishu / Lark',
    enabled: true,
    model: 'auto',
    threadId: '',
    workspaceRoot: '',
    agentProfile: {
      name: 'analytix', description: '', identity: '', personality: '', userContext: '', replyRules: ''
    },
    conversations: [],
    platformAccount,
    createdAt: now,
    updatedAt: now
  }
}

function parseIpcPayload<T>(channel: string, schema: z.ZodType<T>, payload: unknown): T {
  const parsed = schema.safeParse(payload)
  if (parsed.success) return parsed.data
  const issue = parsed.error.issues[0]
  throw new Error(`Invalid payload for ${channel}: ${issue?.message ?? 'Bad request.'}`)
}

const LOCAL_DISPLAY_IMPORT_MAPPING_PREVIEW_PATH = '/v1/local-display/import-mapping-preview'
const LOCAL_DISPLAY_CLEANING_DIFF_PREVIEW_PATH = '/v1/local-display/cleaning-diff-preview'
const LOCAL_DISPLAY_DIRECT_SOURCE_PREVIEW_PATH = '/v1/local-display/direct-source-preview'
const LOCAL_DISPLAY_ACCEPTED_SLOT_PATH = '/v1/local-display/accepted-slot-display'
const HOST_FUNDS_IMPORT_STAGE_PATH = '/v1/local-display/funds-import/stage'
const HOST_FUNDS_IMPORT_CONFIRM_PATH = '/v1/local-display/funds-import/confirm'
const HOST_FUNDS_IMPORT_CANCEL_PATH = '/v1/local-display/funds-import/cancel'
const HOST_FUNDS_IMPORT_STATUS_PATH = '/v1/local-display/funds-import/status'
const HOST_FUNDS_CLEANING_RUN_PATH = '/v1/local-display/funds-cleaning/run'
const HOST_FUNDS_CLEANING_REVOKE_PATH = '/v1/local-display/funds-cleaning/revoke'
const LOCAL_DISPLAY_MAX_RESPONSE_BYTES = 1 << 20
const PRIVATE_MEDIA_MAX_RESPONSE_BYTES = 24 << 20

type PrivateMediaFailureCode = Extract<PrivateMediaImageResult, { ok: false }>['code']

function privateMediaFailureCode(response: RuntimeRequestResult): PrivateMediaFailureCode {
  try {
    const parsed = JSON.parse(response.body) as { code?: unknown }
    if (parsed.code === 'invalid_request' || parsed.code === 'unavailable' ||
        parsed.code === 'privacy_unavailable' || parsed.code === 'authority_changed' || parsed.code === 'provider_failed') {
      return parsed.code
    }
  } catch {
    // The Go route owns the raw Provider response. Main projects only a fixed
    // failure enum when its bounded response is malformed.
  }
  return response.status === 400
    ? 'invalid_request'
    : response.status === 409
      ? 'authority_changed'
      : response.status === 503
        ? 'unavailable'
        : 'provider_failed'
}

async function executePrivateMediaImage(
  privateMediaRequest: PrivateMediaRuntimeRequest | undefined,
  request: PrivateMediaImageRequest
): Promise<PrivateMediaImageResult> {
  if (!privateMediaRequest) return { ok: false, code: 'unavailable' }
  const response = await privateMediaRequest(JSON.stringify({ schemaVersion: 1, ...request }))
  if (Buffer.byteLength(response.body, 'utf8') > PRIVATE_MEDIA_MAX_RESPONSE_BYTES) {
    return { ok: false, code: 'provider_failed' }
  }
  if (!response.ok) return { ok: false, code: privateMediaFailureCode(response) }
  try {
    const parsed = JSON.parse(response.body) as Record<string, unknown>
    if (parsed.schemaVersion === 1 && parsed.status === 'ok' &&
        typeof parsed.imageBase64 === 'string' && parsed.imageBase64.length <= 24 * 1024 * 1024 &&
        (parsed.mimeType === 'image/png' || parsed.mimeType === 'image/jpeg' || parsed.mimeType === 'image/webp') &&
        Object.keys(parsed).every((key) => ['schemaVersion', 'status', 'imageBase64', 'mimeType'].includes(key))) {
      return { ok: true, imageBase64: parsed.imageBase64, mimeType: parsed.mimeType }
    }
  } catch {
    // Fixed projection below.
  }
  return { ok: false, code: 'provider_failed' }
}

async function executePrivateMediaSpeech(
  privateMediaRequest: PrivateMediaRuntimeRequest | undefined,
  request: PrivateMediaSpeechRequest
): Promise<PrivateMediaSpeechResult> {
  if (!privateMediaRequest) return { ok: false, code: 'unavailable' }
  const response = await privateMediaRequest(JSON.stringify({ schemaVersion: 1, ...request }))
  if (Buffer.byteLength(response.body, 'utf8') > PRIVATE_MEDIA_MAX_RESPONSE_BYTES) {
    return { ok: false, code: 'provider_failed' }
  }
  if (!response.ok) return { ok: false, code: privateMediaFailureCode(response) }
  try {
    const parsed = JSON.parse(response.body) as Record<string, unknown>
    if (parsed.schemaVersion === 1 && parsed.status === 'ok' &&
        typeof parsed.transcript === 'string' && parsed.transcript.length <= 64 * 1024 &&
        Object.keys(parsed).every((key) => ['schemaVersion', 'status', 'transcript'].includes(key))) {
      return { ok: true, transcript: parsed.transcript }
    }
  } catch {
    // Fixed projection below.
  }
  return { ok: false, code: 'provider_failed' }
}

type PreparedLocalDisplayRequest = {
  body: unknown
  authorityIsCurrent?: () => Promise<boolean>
}

class LocalDisplayAuthorityDeniedError extends Error {}

type DirectSourcePreviewRuntimeRequest = DirectSourcePreviewRequest & {
  workspaceRoot: string
}

type MainOwnedFundsImportGeneration = {
  readonly workspaceRoot: string
  readonly status: 'ready' | 'mapping_invalid'
  readonly totalRowCount: number
  readonly selectors: readonly string[]
  readonly itemsBySelector: Readonly<Record<string, Readonly<{
    selector: string
    sourceIndex: number
    sourceCount: number
    sourceLabel: string
    rowCount: number
    columnCount: number
    status: 'ready' | 'mapping_invalid'
  }>>>
}

type MainOwnedFundsImportOwner = {
  active: MainOwnedFundsImportGeneration | null
  stageInvocation: bigint
}

type MainOwnedCleaningGeneration = {
  readonly workspaceRoot: string
  readonly selector: string
  readonly ruleGeneration: string
  readonly ruleDigest: string
  readonly inputSnapshot: string
  readonly outputSnapshot: string
  readonly transformLineage: string
}

type MainOwnedCleaningOwner = {
  active: MainOwnedCleaningGeneration | null
  runInvocation: bigint
}

const fundsImportSelectorSchema = z.string().regex(/^tlsel1_[a-f0-9]{64}$/)
const fundsImportStatusSchema = z.enum(['ready', 'mapping_invalid'])
const fundsImportSourceItemSchema = z.object({
  selector: fundsImportSelectorSchema,
  sourceIndex: z.number().int().positive().max(16),
  sourceCount: z.number().int().positive().max(16),
  sourceLabel: z.string().regex(/^Source [1-9][0-9]? of [1-9][0-9]?$/).max(32),
  rowCount: z.number().int().positive().max(100_000),
  columnCount: z.number().int().positive().max(64),
  status: fundsImportStatusSchema
}).strict()
const fundsImportStageRuntimeResponseSchema = z.object({
  status: fundsImportStatusSchema,
  totalRowCount: z.number().int().positive().max(100_000),
  items: z.array(fundsImportSourceItemSchema).min(1).max(16)
}).strict().superRefine((value, context) => {
  if (value.items.some((item, index) =>
    item.sourceIndex !== index + 1 || item.sourceCount !== value.items.length ||
    item.sourceLabel !== `Source ${index + 1} of ${value.items.length}`
  )) {
    context.addIssue({ code: 'custom', message: 'funds import source inventory is invalid' })
  }
  if (value.items.reduce((total, item) => total + item.rowCount, 0) !== value.totalRowCount ||
      new Set(value.items.map((item) => item.selector)).size !== value.items.length ||
      value.status === 'ready' !== value.items.every((item) => item.status === 'ready')) {
    context.addIssue({ code: 'custom', message: 'funds import generation status is invalid' })
  }
})
const fundsImportConfirmRuntimeResponseSchema = z.object({
  sourceArtifactSha256: z.string().regex(/^[a-f0-9]{64}$/),
  sourceArtifactByteLength: z.number().int().positive().max(64 * 1024 * 1024),
  sourceRowCount: z.number().int().positive().max(100_000)
}).strict()
const fundsImportCancelRuntimeResponseSchema = z.object({ canceled: z.literal(true) }).strict()
const fundsImportStatusRuntimeResponseSchema = z.object({
  status: fundsImportStatusSchema,
  totalRowCount: z.number().int().positive().max(100_000),
  item: fundsImportSourceItemSchema
}).strict().superRefine((value, context) => {
  if (value.status !== value.item.status ||
      value.totalRowCount < value.item.rowCount + value.item.sourceCount - 1 ||
      (value.item.sourceCount === 1 && value.totalRowCount !== value.item.rowCount) ||
      value.item.sourceIndex > value.item.sourceCount ||
      value.item.sourceLabel !== `Source ${value.item.sourceIndex} of ${value.item.sourceCount}`) {
    context.addIssue({ code: 'custom', message: 'funds import status is inconsistent' })
  }
})
const cleaningOpaqueSelectorSchema = z.string().regex(/^tlsel1_[a-f0-9]{64}$/)
const cleaningGenerationSchema = z.string().regex(/^tlgen1_[a-f0-9]{64}$/)
const cleaningSnapshotSchema = z.string().regex(/^tlsnap1_[a-f0-9]{64}$/)
const cleaningLineageSchema = z.string().regex(/^tllin1_[a-f0-9]{64}$/)
const cleaningDigestSchema = z.string().regex(/^[a-f0-9]{64}$/)
const cleaningCountSchema = z.number().int().positive().max(100_000)
const cleaningChangedCountSchema = z.number().int().nonnegative().max(100_000)
const fundsCleaningRunRuntimeResponseSchema = z.union([
  z.object({
    status: z.literal('committed'),
    selector: cleaningOpaqueSelectorSchema,
    ruleGeneration: cleaningGenerationSchema,
    ruleDigest: cleaningDigestSchema,
    inputSnapshot: cleaningSnapshotSchema,
    outputSnapshot: cleaningSnapshotSchema,
    transformLineage: cleaningLineageSchema,
    rowCount: cleaningCountSchema,
    changedRowCount: cleaningChangedCountSchema
  }).strict(),
  z.object({
    status: z.literal('outcome_unknown'),
    rowCount: cleaningCountSchema,
    changedRowCount: cleaningChangedCountSchema
  }).strict(),
  z.object({
    status: z.literal('pre_cas_failed')
  }).strict()
]).superRefine((value, context) => {
  if ('changedRowCount' in value && value.changedRowCount > value.rowCount) {
    context.addIssue({ code: 'custom', message: 'cleaning change count exceeds row count' })
  }
})
const fundsCleaningRevokeRuntimeResponseSchema = z.object({ revoked: z.literal(true) }).strict()

function freezeMainOwnedFundsImportGeneration(
  workspaceRoot: string,
  stage: z.infer<typeof fundsImportStageRuntimeResponseSchema>
): MainOwnedFundsImportGeneration {
  const itemsBySelector: Record<string, MainOwnedFundsImportGeneration['itemsBySelector'][string]> =
    Object.create(null) as Record<string, MainOwnedFundsImportGeneration['itemsBySelector'][string]>
  const selectors = stage.items.map((item) => {
    itemsBySelector[item.selector] = Object.freeze({ ...item })
    return item.selector
  })
  return Object.freeze({
    workspaceRoot,
    status: stage.status,
    totalRowCount: stage.totalRowCount,
    selectors: Object.freeze(selectors),
    itemsBySelector: Object.freeze(itemsBySelector)
  })
}

function fundsImportStatusMatchesAcceptedGeneration(
  status: z.infer<typeof fundsImportStatusRuntimeResponseSchema>,
  requestedSelector: string,
  generation: MainOwnedFundsImportGeneration
): boolean {
  const accepted = generation.itemsBySelector[requestedSelector]
  return Boolean(accepted) && status.status === generation.status &&
    status.totalRowCount === generation.totalRowCount &&
    status.item.selector === accepted.selector &&
    status.item.sourceIndex === accepted.sourceIndex &&
    status.item.sourceCount === accepted.sourceCount &&
    status.item.sourceLabel === accepted.sourceLabel &&
    status.item.rowCount === accepted.rowCount &&
    status.item.columnCount === accepted.columnCount &&
    status.item.status === accepted.status
}

function fundsCSVStageFailure(
  code: Extract<FundsCSVSnapshotStageResult, { ok: false }>['code'],
  message: string,
  canceled = false
): FundsCSVSnapshotStageResult {
  return { ok: false, canceled, code, message }
}

function fundsImportActionFailure(
  code: 'forbidden' | 'invalid_source' | 'runtime_unavailable',
  message: string
): Extract<FundsCSVSnapshotConfirmResult, { ok: false }> {
  return { ok: false, code, message }
}

async function mainOwnedFundsImportWorkspaceIsCurrent(
  store: JsonSettingsStore,
  generation: MainOwnedFundsImportGeneration
): Promise<boolean> {
  try {
    const current = await store.load()
    const configuredWorkspace = current.workspaceRoot.trim()
    return Boolean(configuredWorkspace) &&
      await canonicalPath(resolve(expandHomePath(configuredWorkspace))) === generation.workspaceRoot
  } catch {
    return false
  }
}

async function revokeMainOwnedFundsImport(
  owner: MainOwnedFundsImportOwner,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  expectedSelector?: string
): Promise<void> {
  const active = owner.active
  if (!active || (expectedSelector && !active.selectors.includes(expectedSelector))) return
  owner.active = null
  const selector = active.selectors[0]
  if (!selector) return
  await localDisplayRequest(HOST_FUNDS_IMPORT_CANCEL_PATH, JSON.stringify({ selector })).catch(() => undefined)
}

async function cancelReturnedFundsImportSelector(
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  selector: string
): Promise<void> {
  await localDisplayRequest(HOST_FUNDS_IMPORT_CANCEL_PATH, JSON.stringify({ selector })).catch(() => undefined)
}

function localDisplayFailure(
  status: number,
  code: LocalDisplayFailure['code'],
  message: string
): LocalDisplayFailure {
  return {
    ok: false,
    status: Number.isInteger(status) && status >= 100 && status <= 599 ? status : 502,
    code,
    message
  }
}

function localDisplayFailureFromRuntime(response: RuntimeRequestResult): LocalDisplayFailure {
  const status = response.status
  const code = status === 404
    ? 'not_found'
    : status === 403 || status === 401
      ? 'forbidden'
      : status === 409
        ? 'conflict'
        : response.ok
          ? 'invalid_response'
          : 'runtime_error'
  return localDisplayFailure(status, code, 'Local display request failed.')
}

async function handleLocalDisplayRequest<TRequest>(
  kind: LocalDisplayKind,
  schema: z.ZodType<TRequest>,
  path: string,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  rendererIsCurrent: () => boolean,
  responseMatchesRequest: (response: LocalDisplayResponse, request: TRequest) => boolean,
  payload: unknown,
  prepareRuntimeRequest?: (request: TRequest) => Promise<PreparedLocalDisplayRequest>
): Promise<LocalDisplayResult> {
  if (!rendererIsCurrent()) {
    return localDisplayFailure(403, 'forbidden', 'Local display request failed.')
  }
  const request = parseIpcPayload(path, schema, payload)
  let prepared: PreparedLocalDisplayRequest
  try {
    prepared = prepareRuntimeRequest
      ? await prepareRuntimeRequest(request)
      : { body: request }
  } catch (error) {
    if (error instanceof LocalDisplayAuthorityDeniedError) {
      return localDisplayFailure(403, 'forbidden', 'Local display request failed.')
    }
    return localDisplayFailure(400, 'invalid_request', 'Local display request failed.')
  }
  if (!rendererIsCurrent()) {
    return localDisplayFailure(403, 'forbidden', 'Local display request failed.')
  }
  let response: RuntimeRequestResult
  try {
    response = await localDisplayRequest(path, JSON.stringify(prepared.body))
  } catch {
    return localDisplayFailure(503, 'runtime_unavailable', 'Local display request failed.')
  }
  let authorityIsCurrent = true
  try {
    authorityIsCurrent = prepared.authorityIsCurrent
      ? await prepared.authorityIsCurrent()
      : true
  } catch {
    authorityIsCurrent = false
  }
  if (!rendererIsCurrent() || !authorityIsCurrent) {
    response = { ok: false, status: 403, body: '' }
    return localDisplayFailure(403, 'forbidden', 'Local display request failed.')
  }
  if (!response.ok) return localDisplayFailureFromRuntime(response)
  if (Buffer.byteLength(response.body, 'utf8') > LOCAL_DISPLAY_MAX_RESPONSE_BYTES) {
    return localDisplayFailure(502, 'invalid_response', 'Local display response was invalid.')
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(response.body)
  } catch {
    return localDisplayFailure(502, 'invalid_response', 'Local display response was invalid.')
  }
  const validated = localDisplayResponseSchema.safeParse(parsed)
  if (
    !validated.success ||
    validated.data.kind !== kind ||
    !responseMatchesRequest(validated.data, request)
  ) {
    return localDisplayFailure(502, 'invalid_response', 'Local display response was invalid.')
  }
  return validated.data
}

async function prepareMainOwnedDirectSourcePreviewRequest(
  store: JsonSettingsStore,
  request: DirectSourcePreviewRequest
): Promise<PreparedLocalDisplayRequest> {
  const settings = await store.load()
  const configuredWorkspace = settings.workspaceRoot.trim()
  if (!configuredWorkspace) throw new Error('Current workspace is unavailable.')
  const workspaceRoot = await canonicalPath(resolve(expandHomePath(configuredWorkspace)))
  const body: DirectSourcePreviewRuntimeRequest = { workspaceRoot, ...request }
  return {
    body,
    authorityIsCurrent: async () => {
      const currentSettings = await store.load()
      const currentWorkspace = currentSettings.workspaceRoot.trim()
      if (!currentWorkspace) return false
      return await canonicalPath(resolve(expandHomePath(currentWorkspace))) === workspaceRoot
    }
  }
}

async function mainOwnedCleaningWorkspaceIsCurrent(
  store: JsonSettingsStore,
  generation: MainOwnedCleaningGeneration
): Promise<boolean> {
  try {
    const settings = await store.load()
    const configuredWorkspace = settings.workspaceRoot.trim()
    return Boolean(configuredWorkspace) &&
      await canonicalPath(resolve(expandHomePath(configuredWorkspace))) === generation.workspaceRoot
  } catch {
    return false
  }
}

async function revokeMainOwnedCleaningGeneration(
  owner: MainOwnedCleaningOwner,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  expectedSelector?: string
): Promise<boolean> {
  const active = owner.active
  if (!active || (expectedSelector && active.selector !== expectedSelector)) return false
  owner.active = null
  try {
    const response = await localDisplayRequest(
      HOST_FUNDS_CLEANING_REVOKE_PATH,
      JSON.stringify({ selector: active.selector })
    )
    if (!response.ok || Buffer.byteLength(response.body, 'utf8') > 1024) return false
    return fundsCleaningRevokeRuntimeResponseSchema.safeParse(JSON.parse(response.body)).success
  } catch {
    return false
  }
}

async function prepareMainOwnedCleaningDiffRequest(
  store: JsonSettingsStore,
  owner: MainOwnedCleaningOwner,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  request: CleaningDiffPreviewRequest
): Promise<PreparedLocalDisplayRequest> {
  const active = owner.active
  if (!active || active.selector !== request.selector ||
      !await mainOwnedCleaningWorkspaceIsCurrent(store, active)) {
    if (active?.selector === request.selector) {
      await revokeMainOwnedCleaningGeneration(owner, localDisplayRequest, request.selector)
    }
    throw new Error('Current cleaning diff generation is unavailable.')
  }
  return {
    body: request,
    authorityIsCurrent: async () => {
      const current = owner.active === active && active.selector === request.selector &&
        await mainOwnedCleaningWorkspaceIsCurrent(store, active)
      if (!current && owner.active === active) {
        await revokeMainOwnedCleaningGeneration(owner, localDisplayRequest, request.selector)
      }
      return current
    }
  }
}

function cleaningRunFailure(
  status: 'outcome_unknown' | 'unavailable',
  code: 'outcome_unknown' | 'pre_cas_failed' | 'forbidden' | 'runtime_unavailable',
  message: string,
  counts?: { rowCount: number; changedRowCount: number }
): Extract<FundsDeterministicCleaningResult, { ok: false }> {
  return { ok: false, status, code, message, ...counts }
}

async function runMainOwnedDeterministicCleaning(
  store: JsonSettingsStore,
  owner: MainOwnedCleaningOwner,
  rendererIsCurrent: () => boolean,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>
): Promise<FundsDeterministicCleaningResult> {
  if (!rendererIsCurrent()) {
    return cleaningRunFailure('unavailable', 'forbidden', 'Deterministic cleaning is unavailable.')
  }
  await revokeMainOwnedCleaningGeneration(owner, localDisplayRequest)
  const invocation = owner.runInvocation + 1n
  owner.runInvocation = invocation
  const invocationIsCurrent = () => owner.runInvocation === invocation
  let workspaceRoot: string
  try {
    const settings = await store.load()
    const configuredWorkspace = settings.workspaceRoot.trim()
    if (!configuredWorkspace) throw new Error('workspace unavailable')
    workspaceRoot = await canonicalPath(resolve(expandHomePath(configuredWorkspace)))
  } catch {
    return cleaningRunFailure('unavailable', 'forbidden', 'Current workspace is unavailable.')
  }
  if (!rendererIsCurrent() || !invocationIsCurrent()) {
    return cleaningRunFailure('unavailable', 'forbidden', 'Deterministic cleaning authority changed.')
  }
  let response: RuntimeRequestResult
  try {
    response = await localDisplayRequest(HOST_FUNDS_CLEANING_RUN_PATH, JSON.stringify({ workspaceRoot }))
  } catch {
    return cleaningRunFailure(
      'outcome_unknown',
      'outcome_unknown',
      'The DSV2 outcome is unknown. Reconcile the current snapshot before retrying.'
    )
  }
  if (Buffer.byteLength(response.body, 'utf8') > 64 * 1024) {
    return cleaningRunFailure(
      'outcome_unknown',
      'outcome_unknown',
      'The DSV2 outcome is unknown. Reconcile the current snapshot before retrying.'
    )
  }
  if (!response.ok) {
    return cleaningRunFailure(
      'outcome_unknown',
      'outcome_unknown',
      'The DSV2 outcome is unknown. Reconcile the current snapshot before retrying.'
    )
  }
  let parsed: z.infer<typeof fundsCleaningRunRuntimeResponseSchema>
  try {
    const validated = fundsCleaningRunRuntimeResponseSchema.safeParse(JSON.parse(response.body))
    if (!validated.success) throw new Error('invalid response')
    parsed = validated.data
  } catch {
    return cleaningRunFailure(
      'outcome_unknown',
      'outcome_unknown',
      'The DSV2 outcome is unknown. Reconcile the current snapshot before retrying.'
    )
  }
  if (parsed.status === 'pre_cas_failed') {
    return cleaningRunFailure(
      'unavailable',
      'pre_cas_failed',
      'Deterministic cleaning did not advance the current DSV2 snapshot.'
    )
  }
  if (parsed.status === 'outcome_unknown') {
    return cleaningRunFailure(
      'outcome_unknown',
      'outcome_unknown',
      'The DSV2 outcome is unknown. Reconcile the current snapshot before retrying.',
      { rowCount: parsed.rowCount, changedRowCount: parsed.changedRowCount }
    )
  }
  const generation = Object.freeze({
    workspaceRoot,
    selector: parsed.selector,
    ruleGeneration: parsed.ruleGeneration,
    ruleDigest: parsed.ruleDigest,
    inputSnapshot: parsed.inputSnapshot,
    outputSnapshot: parsed.outputSnapshot,
    transformLineage: parsed.transformLineage
  })
  const current = rendererIsCurrent() && invocationIsCurrent() &&
    await mainOwnedCleaningWorkspaceIsCurrent(store, generation)
  if (!current) {
    await localDisplayRequest(
      HOST_FUNDS_CLEANING_REVOKE_PATH,
      JSON.stringify({ selector: parsed.selector })
    ).catch(() => undefined)
    return cleaningRunFailure(
      'outcome_unknown',
      'outcome_unknown',
      'The cleaning committed but display authority changed. Reconcile the current snapshot.'
    )
  }
  owner.active = generation
  return { ok: true, ...parsed }
}

async function revokeMainOwnedCleaningDiffPreview(
  selectorInput: unknown,
  owner: MainOwnedCleaningOwner,
  rendererIsCurrent: () => boolean,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>
): Promise<FundsCleaningDiffRevokeResult> {
  const selector = parseIpcPayload(HOST_FUNDS_CLEANING_REVOKE_PATH, cleaningOpaqueSelectorSchema, selectorInput)
  if (!rendererIsCurrent() || owner.active?.selector !== selector) {
    return { ok: false, code: 'forbidden', message: 'Cleaning diff preview is unavailable.' }
  }
  const revoked = await revokeMainOwnedCleaningGeneration(owner, localDisplayRequest, selector)
  return revoked
    ? { ok: true, revoked: true }
    : { ok: false, code: 'runtime_unavailable', message: 'Cleaning diff preview could not be revoked.' }
}

async function prepareMainOwnedImportMappingRequest(
  store: JsonSettingsStore,
  owner: MainOwnedFundsImportOwner,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  request: ImportMappingPreviewRequest
): Promise<PreparedLocalDisplayRequest> {
  const active = owner.active
  if (!active || !active.selectors.includes(request.selector) ||
      !await mainOwnedFundsImportWorkspaceIsCurrent(store, active)) {
    if (active?.selectors.includes(request.selector)) {
      await revokeMainOwnedFundsImport(owner, localDisplayRequest, request.selector)
    }
    throw new Error('Current import generation is unavailable.')
  }
  return {
    body: request,
    authorityIsCurrent: async () => {
      const stillCurrent = owner.active === active && active.selectors.includes(request.selector) &&
        await mainOwnedFundsImportWorkspaceIsCurrent(store, active)
      if (!stillCurrent && owner.active === active) {
        await revokeMainOwnedFundsImport(owner, localDisplayRequest, request.selector)
      }
      return stillCurrent
    }
  }
}

async function stageMainOwnedFundsCSVSnapshot(
  store: JsonSettingsStore,
  getMainWindow: () => BrowserWindow | null,
  rendererIsCurrent: () => boolean,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  owner: MainOwnedFundsImportOwner
): Promise<FundsCSVSnapshotStageResult> {
  const stageInvocation = owner.stageInvocation + 1n
  owner.stageInvocation = stageInvocation
  const invocationIsCurrent = () => owner.stageInvocation === stageInvocation
  if (!rendererIsCurrent()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  await revokeMainOwnedFundsImport(owner, localDisplayRequest)
  if (!invocationIsCurrent() || !rendererIsCurrent()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  const settings = await store.load()
  if (!invocationIsCurrent()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  const configuredWorkspace = settings.workspaceRoot.trim()
  if (!configuredWorkspace) {
    return fundsCSVStageFailure('invalid_source', 'Current workspace is unavailable.')
  }
  let workspaceRoot: string
  try {
    workspaceRoot = await canonicalPath(resolve(expandHomePath(configuredWorkspace)))
  } catch {
    return fundsCSVStageFailure('invalid_source', 'Current workspace is unavailable.')
  }
  if (!invocationIsCurrent()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  const window = getMainWindow()
  if (!window || window.isDestroyed()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  const selection = await dialog.showOpenDialog(window, {
    title: '选择 CSV 或 ZIP 并预览映射',
    defaultPath: workspaceRoot,
    properties: ['openFile'],
    filters: [{ name: 'CSV or ZIP', extensions: ['csv', 'zip'] }]
  })
  if (!invocationIsCurrent()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  if (selection.canceled || selection.filePaths.length !== 1) {
    return fundsCSVStageFailure('invalid_source', 'Data import was cancelled.', true)
  }
  if (!rendererIsCurrent()) {
    return fundsCSVStageFailure('forbidden', 'Data import is unavailable.')
  }
  let sourcePath: string
  try {
    // Preserve the user-selected path identity so the Go acquisition owner can
    // reject symlinks instead of receiving an already-resolved target path.
    sourcePath = resolve(selection.filePaths[0])
  } catch {
    return fundsCSVStageFailure('invalid_source', 'The selected import source is unavailable.')
  }

  let response: RuntimeRequestResult
  try {
    response = await localDisplayRequest(HOST_FUNDS_IMPORT_STAGE_PATH, JSON.stringify({
      workspaceRoot,
      sourcePath
    }))
    if (response.ok && response.status === 202 && Buffer.byteLength(response.body, 'utf8') <= 1024) {
      const creation = z.object({
        status: z.literal('case_creation_required'),
        intent: z.string().regex(/^[a-f0-9]{64}$/)
      }).strict().safeParse(JSON.parse(response.body))
      const workspaceIsCurrent = async () => {
        const current = await store.load()
        if (!current.workspaceRoot.trim()) return false
        const selected = await canonicalPath(resolve(expandHomePath(current.workspaceRoot.trim())))
        return selected === workspaceRoot && invocationIsCurrent() && rendererIsCurrent() && !window.isDestroyed()
      }
      if (!creation.success || !await workspaceIsCurrent()) {
        return fundsCSVStageFailure('forbidden', 'Data import authority changed.')
      }
      const confirmed = await dialog.showMessageBox(window, {
        type: 'question', title: '建立案件项目',
        message: '为当前工作区建立案件项目并预览所选数据？',
        detail: '此操作会保存案件身份。数据预览后仍需点击“确认建立快照”；取消预览不会删除已建立的案件。',
        buttons: ['取消', '建立案件并预览'], defaultId: 0, cancelId: 0, noLink: true
      })
      if (!await workspaceIsCurrent()) {
        return fundsCSVStageFailure('forbidden', 'Data import authority changed.')
      }
      if (confirmed.response !== 1) {
        return fundsCSVStageFailure('invalid_source', 'Data import was cancelled.', true)
      }
      response = await localDisplayRequest(HOST_FUNDS_IMPORT_STAGE_PATH, JSON.stringify({
        workspaceRoot, sourcePath, createCaseIntent: creation.data.intent
      }))
    }
  } catch {
    return fundsCSVStageFailure('runtime_unavailable', 'Data import failed.')
  }
  if (!response.ok && response.status === 503 && Buffer.byteLength(response.body, 'utf8') <= 1024) {
    try {
      const capability = z.object({
        code: z.literal('funds_import_capability_unavailable'),
        message: z.string().max(256)
      }).strict().safeParse(JSON.parse(response.body))
      if (capability.success) {
        return fundsCSVStageFailure('capability_unavailable', 'Trusted data import capability is unavailable in this runtime environment.')
      }
    } catch {
      // Unknown/malformed failures retain the generic closed projection below.
    }
  }
  if (!response.ok || Buffer.byteLength(response.body, 'utf8') > 64 * 1024) {
    return fundsCSVStageFailure(
      response.status === 400 ? 'invalid_source' : 'runtime_unavailable',
      'Data import failed.'
    )
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(response.body)
  } catch {
    return fundsCSVStageFailure('runtime_unavailable', 'Data import returned an invalid result.')
  }
  const validated = fundsImportStageRuntimeResponseSchema.safeParse(parsed)
  if (!validated.success) {
    return fundsCSVStageFailure('runtime_unavailable', 'Data import returned an invalid generation.')
  }
  if (!invocationIsCurrent()) {
    const selector = validated.data.items[0]?.selector
    if (selector) await cancelReturnedFundsImportSelector(localDisplayRequest, selector)
    return fundsCSVStageFailure('forbidden', 'Data import authority changed.')
  }
  let authorityCurrent = rendererIsCurrent() && invocationIsCurrent()
  try {
    const current = await store.load()
    const configuredCurrent = current.workspaceRoot.trim()
    authorityCurrent = authorityCurrent && Boolean(configuredCurrent) &&
      await canonicalPath(resolve(expandHomePath(configuredCurrent))) === workspaceRoot
  } catch {
    authorityCurrent = false
  }
  authorityCurrent = authorityCurrent && invocationIsCurrent()
  if (!authorityCurrent) {
    const selector = validated.data.items[0]?.selector
    if (selector) await cancelReturnedFundsImportSelector(localDisplayRequest, selector)
    return fundsCSVStageFailure('forbidden', 'Data import authority changed.')
  }
  owner.active = freezeMainOwnedFundsImportGeneration(workspaceRoot, validated.data)
  return { ok: true, ...validated.data }
}

async function runFundsImportSelectorAction<T extends FundsCSVSnapshotConfirmResult | FundsImportCancelResult | FundsImportStatusResult>(
  selectorInput: unknown,
  store: JsonSettingsStore,
  owner: MainOwnedFundsImportOwner,
  rendererIsCurrent: () => boolean,
  path: string,
  localDisplayRequest: (path: string, body: string) => Promise<RuntimeRequestResult>,
  parse: (value: unknown, selector: string, generation: MainOwnedFundsImportGeneration) => T | null
): Promise<T> {
  const selector = parseIpcPayload(path, fundsImportSelectorSchema, selectorInput)
  const active = owner.active
  if (!active || !active.selectors.includes(selector)) {
    return fundsImportActionFailure('invalid_source', 'Data import generation is unavailable.') as T
  }
  if (!rendererIsCurrent() || !await mainOwnedFundsImportWorkspaceIsCurrent(store, active)) {
    await revokeMainOwnedFundsImport(owner, localDisplayRequest, selector)
    return fundsImportActionFailure('forbidden', 'Data import is unavailable.') as T
  }
  let response: RuntimeRequestResult
  try {
    response = await localDisplayRequest(path, JSON.stringify({ selector }))
  } catch {
    if (path !== HOST_FUNDS_IMPORT_STATUS_PATH) {
      // Confirm may already have linearized its DSV2 CAS even when the reply is
      // lost. This only revokes remaining staging; it does not imply rollback.
      await revokeMainOwnedFundsImport(owner, localDisplayRequest, selector)
    }
    return fundsImportActionFailure('runtime_unavailable', 'Data import failed.') as T
  }
  if (!rendererIsCurrent() || owner.active !== active ||
      !await mainOwnedFundsImportWorkspaceIsCurrent(store, active)) {
    await revokeMainOwnedFundsImport(owner, localDisplayRequest, selector)
    return fundsImportActionFailure('forbidden', 'Data import authority changed.') as T
  }
  if (!response.ok || Buffer.byteLength(response.body, 'utf8') > 64 * 1024) {
    if (path !== HOST_FUNDS_IMPORT_STATUS_PATH) {
      await revokeMainOwnedFundsImport(owner, localDisplayRequest, selector)
    }
    return fundsImportActionFailure(
      response.status === 400 ? 'invalid_source' : 'runtime_unavailable',
      'Data import failed.'
    ) as T
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(response.body)
  } catch {
    if (path !== HOST_FUNDS_IMPORT_STATUS_PATH) {
      await revokeMainOwnedFundsImport(owner, localDisplayRequest, selector)
    }
    return fundsImportActionFailure('runtime_unavailable', 'Data import returned an invalid result.') as T
  }
  const result = parse(parsed, selector, active)
  if (!result) {
    if (path !== HOST_FUNDS_IMPORT_STATUS_PATH) {
      await revokeMainOwnedFundsImport(owner, localDisplayRequest, selector)
    }
    return fundsImportActionFailure('runtime_unavailable', 'Data import returned an invalid result.') as T
  }
  if (path === HOST_FUNDS_IMPORT_CONFIRM_PATH || path === HOST_FUNDS_IMPORT_CANCEL_PATH) {
    owner.active = null
  }
  return result
}

function localDisplayRendererIsCurrent(
  event: IpcMainInvokeEvent,
  getMainWindow: () => BrowserWindow | null
): boolean {
  const window = getMainWindow()
  return Boolean(
    window &&
    !window.isDestroyed() &&
    !event.sender.isDestroyed() &&
    window.webContents === event.sender &&
    event.senderFrame != null &&
    event.senderFrame === event.sender.mainFrame
  )
}

function safeSaveAsFileName(input: string | undefined, fallback = 'generated-file'): string {
  const candidate = (input ?? '').trim().replace(/\0/g, '')
  const name = basename(candidate) || fallback
  if (name === '.' || name === '..') return fallback
  return name
}

function saveDialogFilters(fileName: string, mimeType: string | undefined): Electron.FileFilter[] {
  const ext = extname(fileName).replace(/^\./, '').trim()
  const mime = mimeType?.toLowerCase().trim() ?? ''
  const filters: Electron.FileFilter[] = []
  if (mime.startsWith('image/')) {
    filters.push({ name: 'Images', extensions: ext ? [ext] : ['png', 'jpg', 'jpeg', 'webp', 'gif'] })
  } else if (mime.startsWith('video/')) {
    filters.push({ name: 'Videos', extensions: ext ? [ext] : ['mp4', 'webm', 'mov', 'm4v'] })
  } else if (ext) {
    filters.push({ name: `${ext.toUpperCase()} file`, extensions: [ext] })
  }
  filters.push({ name: 'All Files', extensions: ['*'] })
  return filters
}

async function saveWorkspaceFileAs(
  payload: unknown,
  getMainWindow: () => BrowserWindow | null
): Promise<WorkspaceFileSaveAsResult> {
  const request = parseIpcPayload('file:save-as', workspaceFileSaveAsPayloadSchema, payload)
  try {
    const sourcePath = request.sourcePath
      ? await resolveOpenTargetPath(request.sourcePath, request.workspaceRoot, { allowBasenameFallback: false })
      : ''
    const fileName = safeSaveAsFileName(request.suggestedName || (sourcePath ? basename(sourcePath) : undefined))
    const defaultPath = request.workspaceRoot?.trim()
      ? join(expandHomePath(request.workspaceRoot), fileName)
      : fileName
    const options: Electron.SaveDialogOptions = {
      title: 'Save generated file',
      defaultPath,
      filters: saveDialogFilters(fileName, request.mimeType)
    }
    const mainWindow = getMainWindow()
    const result = mainWindow
      ? await dialog.showSaveDialog(mainWindow, options)
      : await dialog.showSaveDialog(options)
    if (result.canceled || !result.filePath) {
      return { ok: false, canceled: true, message: 'Save cancelled.' }
    }

    const targetPath = resolve(result.filePath)
    await mkdir(dirname(targetPath), { recursive: true })
    if (sourcePath) {
      if (resolve(sourcePath) !== targetPath) {
        await copyFile(sourcePath, targetPath)
      }
    } else if (request.dataBase64) {
      await writeFile(targetPath, Buffer.from(request.dataBase64, 'base64'))
    } else {
      return { ok: false, message: 'No file data was available to save.' }
    }
    return { ok: true, path: targetPath }
  } catch (error) {
    return { ok: false, message: error instanceof Error ? error.message : String(error) }
  }
}

function validateMcpConfigContent(content: string): void {
  const trimmed = content.trim()
  if (!trimmed) return
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed) as unknown
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`MCP config must be JSON: ${message}`)
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('MCP config must be a JSON object.')
  }
}

function runDesktopCommand(
  command: DesktopCommand,
  sender: WebContents,
  getMainWindow: () => BrowserWindow | null
): void {
  const mainWindow = getMainWindow()
  const contents = mainWindow && !mainWindow.isDestroyed() ? mainWindow.webContents : sender

  switch (command) {
    case 'undo':
      contents.undo()
      return
    case 'redo':
      contents.redo()
      return
    case 'cut':
      contents.cut()
      return
    case 'copy':
      contents.copy()
      return
    case 'paste':
      contents.paste()
      return
    case 'selectAll':
      contents.selectAll()
      return
    case 'reload':
      contents.reload()
      return
    case 'zoomIn':
      contents.setZoomLevel(contents.getZoomLevel() + 1)
      return
    case 'zoomOut':
      contents.setZoomLevel(contents.getZoomLevel() - 1)
      return
    case 'resetZoom':
      contents.setZoomLevel(0)
      return
    case 'toggleDevTools':
      contents.toggleDevTools()
      return
    case 'minimize':
      if (mainWindow && !mainWindow.isDestroyed()) mainWindow.minimize()
      return
    case 'toggleMaximize':
      if (!mainWindow || mainWindow.isDestroyed()) return
      if (mainWindow.isMaximized()) {
        mainWindow.unmaximize()
      } else {
        mainWindow.maximize()
      }
      return
    case 'toggleFullscreen':
      if (!mainWindow || mainWindow.isDestroyed()) return
      mainWindow.setFullScreen(!mainWindow.isFullScreen())
      return
    case 'close':
      if (mainWindow && !mainWindow.isDestroyed()) mainWindow.close()
      return
    case 'quit':
      app.quit()
      return
  }
}

export function registerAppIpcHandlers(options: RegisterAppIpcHandlersOptions): void {
  const {
    store,
    loadHubAccountService,
    getMainWindow,
    isTrustedProviderRegistrySender,
    applySettingsPatch,
    saveSettingsPatch,
    runtimeRequest,
    protectedRuntimeRequest,
    getPortableImportReceipt: getPortableImportReceiptOption,
    getCurrentPortableManifest,
    getCurrentOwnerBindingInventory,
    getCurrentProfile,
    localDisplayRequest,
    privateMediaRequest,
    restartRuntime,
    fetchUpstreamModels,
    getClawRuntime,
    getScheduleRuntime,
    startFeishuInstallQrcode,
    pollFeishuInstall,
    startWeixinInstallQrcode,
    pollWeixinInstall,
    imChannelAccountLifecycle,
    installedExtensionAccountLifecycle,
    providerOAuthAccountManagement,
    resolveAnalytixConfigPath,
    onAnalytixMcpConfigWritten,
    showTurnCompleteNotification,
    openThreadInNewWindow,
    getAppVersion,
    readGuiUpdateState,
    loadGuiUpdaterModule,
    resolveLogDirectory,
    logError
  } = options
  void pruneThreadTraces(app.getPath('userData'))
  const workspaceFileWatchers = new Map<string, WorkspaceFileWatchRecord>()
  let hubAgentMarketplaceServicePromise: Promise<HubAgentMarketplaceServiceModule> | null = null
  const loadHubAgentMarketplaceService = (): Promise<HubAgentMarketplaceServiceModule> => {
    if (!hubAgentMarketplaceServicePromise) {
      hubAgentMarketplaceServicePromise = import('../services/hub-agent-marketplace-service')
        .catch((error) => {
          hubAgentMarketplaceServicePromise = null
          throw error
        })
    }
    return hubAgentMarketplaceServicePromise
  }
  const fundsImportOwner: MainOwnedFundsImportOwner = { active: null, stageInvocation: 0n }
  const fundsCleaningOwner: MainOwnedCleaningOwner = { active: null, runInvocation: 0n }
  const providerRegistryIpcHandler = createProviderRegistryIpcHandler(runtimeRequest)
  let latestPortableImportReceipt: ProtectedRecoveryImportReceipt | null = null
  const protectedRecoveryRuntimeClient = protectedRuntimeRequest
    ? createProtectedRecoveryRuntimeClient(protectedRuntimeRequest)
    : async (): Promise<ProtectedRecoveryRuntimeResult> => ({
        ok: false,
        code: 'runtime_unavailable'
      })
  const protectedRecoveryAuthority = createMainProviderCredentialRecoveryAuthority({
    protectedRuntimeRequest: protectedRecoveryRuntimeClient,
    getCurrentWindow: (event) => getMainWindow() as never,
    getCurrentProfile,
    getPortableImportReceipt: getPortableImportReceiptOption ?? (() => latestPortableImportReceipt),
    getCurrentPortableManifest,
    getCurrentOwnerBindingInventory,
    runtimeRequest
  })
  threadHandoffService.subscribe((event) => {
    getMainWindow()?.webContents.send('thread-handoff:event', event)
  })

  const disposeWorkspaceFileWatch = (watchId: string): boolean => {
    const record = workspaceFileWatchers.get(watchId)
    if (!record) return false
    if (record.timer) clearTimeout(record.timer)
    try {
      record.watcher.close()
    } catch (error) {
      logError('workspace-watch', 'Failed to close workspace file watcher', {
        watchId,
        message: error instanceof Error ? error.message : String(error)
      })
    }
    workspaceFileWatchers.delete(watchId)
    return true
  }

  const disposeWorkspaceFileWatchesForSender = (sender: WebContents): void => {
    for (const [watchId, record] of workspaceFileWatchers) {
      if (record.sender.id === sender.id) {
        disposeWorkspaceFileWatch(watchId)
      }
    }
  }

  const emitWorkspaceFileChange = async (watchId: string): Promise<void> => {
    const record = workspaceFileWatchers.get(watchId)
    if (!record) return
    const changedAt = new Date().toISOString()
    try {
      const result = await readWorkspaceFile({
        path: record.path,
        workspaceRoot: record.workspaceRoot
      })
      const latest = workspaceFileWatchers.get(watchId)
      if (!latest || latest.sender.isDestroyed()) return
      if (result.ok) {
        latest.sender.send('file:workspace-changed', {
          ok: true,
          watchId,
          workspaceRoot: latest.workspaceRoot,
          path: result.path,
          content: result.content,
          size: result.size,
          truncated: result.truncated,
          changedAt
        })
        return
      }
      latest.sender.send('file:workspace-changed', {
        ok: false,
        watchId,
        workspaceRoot: latest.workspaceRoot,
        path: latest.path,
        message: result.message,
        changedAt
      })
    } catch (error) {
      const latest = workspaceFileWatchers.get(watchId)
      if (!latest || latest.sender.isDestroyed()) return
      latest.sender.send('file:workspace-changed', {
        ok: false,
        watchId,
        workspaceRoot: latest.workspaceRoot,
        path: latest.path,
        message: error instanceof Error ? error.message : String(error),
        changedAt
      })
    }
  }

  const scheduleWorkspaceFileChange = (watchId: string): void => {
    const record = workspaceFileWatchers.get(watchId)
    if (!record) return
    if (record.timer) clearTimeout(record.timer)
    record.timer = setTimeout(() => {
      const latest = workspaceFileWatchers.get(watchId)
      if (!latest) return
      latest.timer = null
      void emitWorkspaceFileChange(watchId)
    }, 90)
  }

  ipcMain.handle('settings:get', async () => store.load())
  ipcMain.handle('settings:set', async (_, partial: unknown) =>
    applySettingsPatch(
      parseIpcPayload('settings:set', settingsPatchSchema, partial) as AppSettingsPatch
    )
  )
  ipcMain.handle('settings:save-silent', async (_, partial: unknown) =>
    saveSettingsPatch(
      parseIpcPayload('settings:save-silent', settingsPatchSchema, partial) as AppSettingsPatch
    )
  )

  ipcMain.handle('hub-account:snapshot', async () =>
    (await loadHubAccountService()).getSnapshot()
  )
  ipcMain.handle('hub-account:refresh', async () =>
    (await loadHubAccountService()).refresh()
  )
  ipcMain.handle('hub-account:login', async (_, payload: unknown) =>
    (await loadHubAccountService()).login(
      parseIpcPayload('hub-account:login', hubLoginPayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:register', async (_, payload: unknown) =>
    (await loadHubAccountService()).register(
      parseIpcPayload('hub-account:register', hubRegisterPayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:logout', async () =>
    (await loadHubAccountService()).logout()
  )
  ipcMain.handle('hub-account:register-code', async (_, payload: unknown) =>
    (await loadHubAccountService()).sendRegisterVerificationCode(
      parseIpcPayload('hub-account:register-code', hubVerificationCodePayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:password-reset-code', async (_, payload: unknown) =>
    (await loadHubAccountService()).sendPasswordResetVerificationCode(
      parseIpcPayload('hub-account:password-reset-code', hubVerificationCodePayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:password-reset-confirm', async (_, payload: unknown) =>
    (await loadHubAccountService()).confirmPasswordReset(
      parseIpcPayload('hub-account:password-reset-confirm', hubPasswordResetConfirmPayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:challenge', async (_, mode: unknown) =>
    (await loadHubAccountService()).fetchChallenge(
      parseIpcPayload('hub-account:challenge', hubAuthChallengeModeSchema, mode)
    )
  )
  ipcMain.handle('hub-account:challenge-state', async (_, mode: unknown) =>
    (await loadHubAccountService()).fetchChallengeState(
      parseIpcPayload('hub-account:challenge-state', hubAuthChallengeModeSchema, mode)
    )
  )
  ipcMain.handle('hub-account:challenge-verify', async (_, payload: unknown) =>
    (await loadHubAccountService()).verifyChallenge(
      parseIpcPayload('hub-account:challenge-verify', hubAuthChallengeVerifyPayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:profile-update', async (_, payload: unknown) =>
    (await loadHubAccountService()).updateProfile(
      parseIpcPayload('hub-account:profile-update', hubProfileUpdatePayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:profile-events', async (_, payload: unknown) =>
    (await loadHubAccountService()).syncProfileEvents(
      parseIpcPayload('hub-account:profile-events', hubProfileEventSyncPayloadSchema, payload)
    )
  )
  ipcMain.handle('hub-account:profile-events:local-sync', async () =>
    (await loadHubAccountService()).syncLocalProfileEvents()
  )
  ipcMain.handle('hub-account:usage', async () =>
    (await loadHubAccountService()).usage()
  )
  ipcMain.handle('hub-account:referral', async () =>
    (await loadHubAccountService()).referral()
  )
  ipcMain.handle('hub-account:models', async () =>
    (await loadHubAccountService()).models()
  )

  ipcMain.handle('query-cache:invalidate', async (event, payload: unknown) => {
    const request = parseIpcPayload(
      'query-cache:invalidate',
      queryCacheInvalidatePayloadSchema,
      payload
    )
    for (const win of BrowserWindow.getAllWindows()) {
      if (win.isDestroyed() || win.webContents.isDestroyed()) continue
      if (win.webContents.id === event.sender.id) continue
      win.webContents.send('query-cache:invalidate', request)
    }
    return true
  })

  ipcMain.handle('provider-registry:request', async (event, payload: unknown) => {
    const operation = payload && typeof payload === 'object' && !Array.isArray(payload)
      ? (payload as { operation?: unknown }).operation
      : undefined
    if (operation === 'import-portable-manifest' ||
      operation === 'connect' || operation === 'update' || operation === 'select' ||
      operation === 'disconnect' || operation === 'explicit-delete' || operation === 'credential-replace') {
      let trusted = false
      try {
        trusted = Boolean(event?.sender && !event.sender.isDestroyed() &&
          event.senderFrame && event.senderFrame === event.sender.mainFrame &&
          isTrustedProviderRegistrySender?.(event) === true)
      } catch {
        // Missing or stale Electron identities must not reach any write effect.
      }
      if (!trusted) {
        return {
          schemaVersion: 1,
          error: { code: 'unauthorized', message: PROVIDER_REGISTRY_FAILURE_MESSAGES_V1.unauthorized }
        }
      }
      // A new ordinary import is the only source for the Main-owned protected
      // import receipt. Any subsequent ordinary Registry mutation invalidates
      // the cached receipt instead of allowing a stale renderer assertion.
      latestPortableImportReceipt = null
    }
    const result = await providerRegistryIpcHandler(payload)
    if (operation === 'import-portable-manifest' &&
      result && typeof result === 'object' && !('error' in result) &&
      'entries' in result && Array.isArray(result.entries) &&
      typeof (payload as { manifestJson?: unknown }).manifestJson === 'string') {
      const current = await providerRegistryIpcHandler({ schemaVersion: 1, operation: 'list' })
      const manifestJson = (payload as { manifestJson: string }).manifestJson
      const manifest = parseProviderRegistryPortableManifestV1(manifestJson)
      let ownerInventory: ProtectedRecoveryOwnerBinding[] | null = null
      try {
        const resolved = getCurrentOwnerBindingInventory
          ? await getCurrentOwnerBindingInventory()
          : []
        ownerInventory = Array.isArray(resolved) ? resolved.map((entry) => ({ ...entry })) : null
      } catch {
        ownerInventory = null
      }
      if (!('error' in current) && 'providers' in current && manifest && ownerInventory) {
        const providerCount = manifest.providers.length
        const usedOwnerBindings = new Set<string>()
        const entries = result.entries.map((entry, index) => {
          const provider = current.providers.find((candidate) => candidate.id === entry.destinationProviderId)
          if (!provider || provider.tombstone || provider.credentialConfigured) return null
          let destinationOwnerBinding: ProtectedRecoveryOwnerBinding | undefined
          if (index >= providerCount) {
            const descriptor = manifest.accounts[index - providerCount]
            if (!descriptor || descriptor.correlation !== entry.correlation) return null
            const candidates = ownerInventory!.filter((candidate) =>
              candidate.owner === descriptor.owner && candidate.provider === descriptor.provider &&
              candidate.purpose === descriptor.purpose &&
              (candidate.correlation === undefined || candidate.correlation === descriptor.correlation)
            )
            if (candidates.length !== 1) return null
            const candidate = candidates[0]!
            const stableIdentity = [
              candidate.owner, candidate.provider, candidate.accountId, candidate.channelId,
              candidate.purpose, candidate.fingerprint
            ].join('\u0000')
            if (!candidate.accountId || !candidate.channelId || !candidate.fingerprint ||
              usedOwnerBindings.has(stableIdentity)) return null
            usedOwnerBindings.add(stableIdentity)
            destinationOwnerBinding = { ...candidate, correlation: descriptor.correlation }
          }
          return {
            correlation: entry.correlation,
            destinationProviderId: entry.destinationProviderId,
            status: entry.status,
            fence: {
              revision: provider.revision,
              generation: provider.generation,
              incarnation: provider.incarnation
            },
            ...(destinationOwnerBinding ? { destinationOwnerBinding } : {})
          }
        })
        if (entries.every((entry): entry is NonNullable<typeof entry> => entry !== null)) {
          latestPortableImportReceipt = {
            manifestJson,
            entries
          }
        }
      }
    }
    return result
  })

  const protectedRecoveryHandler = (
    action: (event: ProtectedRecoveryInvokeEvent) => Promise<unknown>
  ) => async (event: IpcMainInvokeEvent, payload?: unknown): Promise<unknown> => {
    if (payload !== undefined) return { ok: false, code: 'invalid_request' }
    return action(event as ProtectedRecoveryInvokeEvent)
  }
  ipcMain.handle(
    'provider-credential-recovery:create-destination-request',
    protectedRecoveryHandler((event) => protectedRecoveryAuthority.createDestinationRequest(event))
  )
  ipcMain.handle(
    'provider-credential-recovery:create-source-bundle',
    protectedRecoveryHandler((event) => protectedRecoveryAuthority.createSourceBundle(event))
  )
  ipcMain.handle(
    'provider-credential-recovery:apply-destination-bundle',
    protectedRecoveryHandler((event) => protectedRecoveryAuthority.applyDestinationBundle(event))
  )
  ipcMain.handle(
    'provider-credential-recovery:finalize-source-receipt',
    protectedRecoveryHandler((event) => protectedRecoveryAuthority.finalizeSourceReceipt(event))
  )

  const oauthAuthority = (event: IpcMainInvokeEvent): { webContentsId: number } | null =>
    localDisplayRendererIsCurrent(event, getMainWindow)
      ? { webContentsId: event.sender.id }
      : null
  const oauthForbidden = () => ({
    ok: false as const,
    code: 'forbidden' as const,
    message: 'OAuth account management is unavailable.'
  })

  ipcMain.handle('provider-oauth:begin', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    return providerOAuthAccountManagement.beginProvider(
      parseIpcPayload('provider-oauth:begin', providerOAuthBeginPayloadSchema, payload),
      authority
    )
  })
  ipcMain.handle('provider-oauth:configure', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.configureProvider(
          parseIpcPayload('provider-oauth:configure', providerOAuthConfigurePayloadSchema, payload)
        )
      : oauthForbidden()
  )
  ipcMain.handle('mcp-oauth:begin', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    return providerOAuthAccountManagement.beginMcp(
      parseIpcPayload('mcp-oauth:begin', mcpOAuthBeginPayloadSchema, payload),
      authority
    )
  })
  ipcMain.handle('extension-oauth:begin', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    return providerOAuthAccountManagement.beginExtension(
      parseIpcPayload('extension-oauth:begin', extensionOAuthBeginPayloadSchema, payload),
      authority
    )
  })
  ipcMain.handle('provider-oauth:status', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    const request = parseIpcPayload('provider-oauth:status', oauthAuthorizationIdPayloadSchema, payload)
    return providerOAuthAccountManagement.statusProvider(request.authorizationId, authority)
  })
  ipcMain.handle('provider-oauth:cancel', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    const request = parseIpcPayload('provider-oauth:cancel', oauthAuthorizationIdPayloadSchema, payload)
    return providerOAuthAccountManagement.cancelProvider(request.authorizationId, authority)
  })
  ipcMain.handle('mcp-oauth:status', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    const request = parseIpcPayload('mcp-oauth:status', oauthAuthorizationIdPayloadSchema, payload)
    return providerOAuthAccountManagement.statusMcp(request.authorizationId, authority)
  })
  ipcMain.handle('mcp-oauth:cancel', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    const request = parseIpcPayload('mcp-oauth:cancel', oauthAuthorizationIdPayloadSchema, payload)
    return providerOAuthAccountManagement.cancelMcp(request.authorizationId, authority)
  })
  ipcMain.handle('extension-oauth:status', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    const request = parseIpcPayload('extension-oauth:status', oauthAuthorizationIdPayloadSchema, payload)
    return providerOAuthAccountManagement.statusExtension(request.authorizationId, authority)
  })
  ipcMain.handle('extension-oauth:cancel', async (event, payload: unknown) => {
    const authority = oauthAuthority(event)
    if (!authority) return oauthForbidden()
    const request = parseIpcPayload('extension-oauth:cancel', oauthAuthorizationIdPayloadSchema, payload)
    return providerOAuthAccountManagement.cancelExtension(request.authorizationId, authority)
  })
  ipcMain.handle('provider-oauth:revoke', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.revokeProvider(
          parseIpcPayload('provider-oauth:revoke', providerOAuthBeginPayloadSchema, payload)
        )
      : oauthForbidden()
  )
  ipcMain.handle('provider-oauth:delete', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.deleteProvider(
          parseIpcPayload('provider-oauth:delete', providerOAuthBeginPayloadSchema, payload)
        )
      : oauthForbidden()
  )
  ipcMain.handle('provider-oauth:replace-subscription', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.replaceProviderSubscription(
          parseIpcPayload(
            'provider-oauth:replace-subscription', providerOAuthSubscriptionPayloadSchema, payload
          )
        )
      : oauthForbidden()
  )
  ipcMain.handle('mcp-oauth:revoke', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.revokeMcp(
          parseIpcPayload('mcp-oauth:revoke', mcpOAuthBeginPayloadSchema, payload)
        )
      : oauthForbidden()
  )
  ipcMain.handle('mcp-oauth:delete', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.deleteMcp(
          parseIpcPayload('mcp-oauth:delete', mcpOAuthBeginPayloadSchema, payload)
        )
      : oauthForbidden()
  )
  ipcMain.handle('extension-oauth:revoke', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.revokeExtension(
          parseIpcPayload('extension-oauth:revoke', extensionOAuthBeginPayloadSchema, payload)
        )
      : oauthForbidden()
  )
  ipcMain.handle('extension-oauth:delete', async (event, payload: unknown) =>
    oauthAuthority(event)
      ? providerOAuthAccountManagement.deleteExtension(
          parseIpcPayload('extension-oauth:delete', extensionOAuthBeginPayloadSchema, payload)
        )
      : oauthForbidden()
  )

  ipcMain.handle('runtime:request', async (_, payload: unknown) => {
    const request = parseIpcPayload('runtime:request', runtimeRequestPayloadSchema, payload)
    const response = await runtimeRequest(request.path, request.method, request.body)
    return sanitizeRuntimeResponse(response, request.path, undefined, request.method)
  })

  ipcMain.handle('runtime:import-mapping-preview', async (event, payload: unknown) =>
    handleLocalDisplayRequest(
      'import_mapping_preview',
      importMappingPreviewRequestSchema,
      LOCAL_DISPLAY_IMPORT_MAPPING_PREVIEW_PATH,
      localDisplayRequest,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      (response, request: ImportMappingPreviewRequest) =>
        response.kind === 'import_mapping_preview' &&
        response.selector === request.selector &&
        response.displayMode === request.displayMode &&
        response.rowOffset === request.rowOffset &&
        response.rowLimit === request.rowLimit &&
        response.fields.length === request.fields.length &&
        response.fields.every((field, index) => field === request.fields[index]),
      payload,
      (request) => prepareMainOwnedImportMappingRequest(store, fundsImportOwner, localDisplayRequest, request)
    )
  )

  ipcMain.handle('runtime:cleaning-diff-preview', async (event, payload: unknown) =>
    handleLocalDisplayRequest(
      'cleaning_diff_preview',
      cleaningDiffPreviewRequestSchema,
      LOCAL_DISPLAY_CLEANING_DIFF_PREVIEW_PATH,
      localDisplayRequest,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      (response, request: CleaningDiffPreviewRequest) =>
        response.kind === 'cleaning_diff_preview' &&
        response.selector === request.selector &&
        fundsCleaningOwner.active?.selector === request.selector &&
        response.lineage.inputSnapshot === fundsCleaningOwner.active.inputSnapshot &&
        response.lineage.ruleGeneration === fundsCleaningOwner.active.ruleGeneration &&
        response.lineage.ruleDigest === fundsCleaningOwner.active.ruleDigest &&
        response.lineage.outputSnapshot === fundsCleaningOwner.active.outputSnapshot &&
        response.lineage.transformLineage === fundsCleaningOwner.active.transformLineage &&
        response.displayMode === request.displayMode &&
        response.rowOffset === request.rowOffset &&
        response.rowLimit === request.rowLimit &&
        response.fields.length === request.fields.length &&
        response.fields.every((field, index) => field === request.fields[index]),
      payload,
      (request) => prepareMainOwnedCleaningDiffRequest(
        store,
        fundsCleaningOwner,
        localDisplayRequest,
        request
      )
    )
  )

  ipcMain.handle('runtime:direct-source-preview', async (event, payload: unknown) =>
    handleLocalDisplayRequest(
      'direct_source_preview',
      directSourcePreviewRequestSchema,
      LOCAL_DISPLAY_DIRECT_SOURCE_PREVIEW_PATH,
      localDisplayRequest,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      (response, request: DirectSourcePreviewRequest) =>
        response.kind === 'direct_source_preview' &&
        response.displayMode === request.displayMode &&
        response.view === request.view &&
        response.rowOffset === request.rowOffset &&
        response.rowLimit === request.rowLimit &&
        response.fields.length === request.fields.length &&
        response.fields.every((field, index) => field === request.fields[index]),
      payload,
      (request) => prepareMainOwnedDirectSourcePreviewRequest(store, request)
    )
  )

  ipcMain.handle('runtime:accepted-slot-display', async (event, payload: unknown) =>
    handleLocalDisplayRequest(
      'accepted_slot_display',
      acceptedSlotDisplayRequestSchema,
      LOCAL_DISPLAY_ACCEPTED_SLOT_PATH,
      localDisplayRequest,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      (response, request: AcceptedSlotDisplayRequest) =>
        response.kind === 'accepted_slot_display' &&
        response.threadId === request.threadId &&
        response.turnId === request.turnId &&
        response.acceptedFinalDigest === request.acceptedFinalDigest &&
        response.displayMode === request.displayMode,
      payload
    )
  )

  ipcMain.handle('runtime:stage-funds-csv-snapshot', async (event) => {
    const result = await stageMainOwnedFundsCSVSnapshot(
      store,
      getMainWindow,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      localDisplayRequest,
      fundsImportOwner
    )
    if (result.ok) {
      const selector = result.items[0]?.selector
      if (selector) {
        event.sender.once('destroyed', () => {
          void revokeMainOwnedFundsImport(fundsImportOwner, localDisplayRequest, selector)
        })
      }
    }
    return result
  })

  ipcMain.handle('runtime:confirm-funds-csv-snapshot', async (event, selector: unknown) =>
    runFundsImportSelectorAction<FundsCSVSnapshotConfirmResult>(
      selector,
      store,
      fundsImportOwner,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      HOST_FUNDS_IMPORT_CONFIRM_PATH,
      localDisplayRequest,
      (value) => {
        const parsed = fundsImportConfirmRuntimeResponseSchema.safeParse(value)
        return parsed.success ? { ok: true, rowCount: parsed.data.sourceRowCount } : null
      }
    )
  )

  ipcMain.handle('runtime:cancel-funds-csv-import', async (event, selector: unknown) =>
    runFundsImportSelectorAction<FundsImportCancelResult>(
      selector,
      store,
      fundsImportOwner,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      HOST_FUNDS_IMPORT_CANCEL_PATH,
      localDisplayRequest,
      (value) => {
        const parsed = fundsImportCancelRuntimeResponseSchema.safeParse(value)
        return parsed.success ? { ok: true, canceled: true } : null
      }
    )
  )

  ipcMain.handle('runtime:status-funds-csv-import', async (event, selector: unknown) =>
    runFundsImportSelectorAction<FundsImportStatusResult>(
      selector,
      store,
      fundsImportOwner,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      HOST_FUNDS_IMPORT_STATUS_PATH,
      localDisplayRequest,
      (value, requestedSelector, generation) => {
        const parsed = fundsImportStatusRuntimeResponseSchema.safeParse(value)
        return parsed.success && fundsImportStatusMatchesAcceptedGeneration(
          parsed.data,
          requestedSelector,
          generation
        )
          ? { ok: true, ...parsed.data }
          : null
      }
    )
  )

  ipcMain.handle('runtime:run-deterministic-funds-cleaning', async (event) => {
    const result = await runMainOwnedDeterministicCleaning(
      store,
      fundsCleaningOwner,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      localDisplayRequest
    )
    if (result.ok) {
      const selector = result.selector
      event.sender.once('destroyed', () => {
        void revokeMainOwnedCleaningGeneration(fundsCleaningOwner, localDisplayRequest, selector)
      })
    }
    return result
  })

  ipcMain.handle('runtime:revoke-cleaning-diff-preview', async (event, selector: unknown) =>
    revokeMainOwnedCleaningDiffPreview(
      selector,
      fundsCleaningOwner,
      () => localDisplayRendererIsCurrent(event, getMainWindow),
      localDisplayRequest
    )
  )

  ipcMain.handle('runtime:restart', async () => {
    fundsCleaningOwner.runInvocation += 1n
    await revokeMainOwnedCleaningGeneration(fundsCleaningOwner, localDisplayRequest)
    try {
      await restartRuntime()
    } catch {
      throw runtimeErrorToError({
        code: 'runtime_unavailable',
        message: 'The Analytix runtime is unavailable.'
      })
    }
  })

  ipcMain.handle('upstream:models', async () => fetchUpstreamModels())

  ipcMain.handle('provider:capability-probe', async (_, payload: unknown) => {
    const request = parseIpcPayload('provider:capability-probe', providerCapabilityProbePayloadSchema, payload)
    const result = await probeModelCapabilities(request, await store.load())
    const { ok: _ok, latencyMs: _latencyMs, message: _message, ...storedResult } = result
    await saveSettingsPatch({
      runtime: {
        modelCapabilityProbes: {
          [result.key]: storedResult
        }
      }
    })
    return result
  })

  ipcMain.handle('claw:status', async (): Promise<ClawRuntimeStatus> =>
    getClawRuntime()?.status() ?? {
      imServerRunning: false,
      imUrl: '',
      runningTaskIds: []
    }
  )

  ipcMain.handle('claw:task:run', async (_, taskId: unknown): Promise<ClawRunResult> => {
    const normalizedTaskId = parseIpcPayload('claw:task:run', streamIdSchema, taskId)
    const scheduleRuntime = getScheduleRuntime()
    if (!scheduleRuntime) return { ok: false, message: 'Schedule runtime is not initialized.' }
    return scheduleRuntime.runTask(normalizedTaskId)
  })

  ipcMain.handle('schedule:status', async (): Promise<ScheduleRuntimeStatus> =>
    getScheduleRuntime()?.status() ?? {
      internalServerRunning: false,
      internalUrl: '',
      runningTaskIds: [],
      powerSaveBlockerActive: false
    }
  )

  ipcMain.handle('schedule:task:run', async (_, taskId: unknown): Promise<ScheduleRunResult> => {
    const normalizedTaskId = parseIpcPayload('schedule:task:run', streamIdSchema, taskId)
    const scheduleRuntime = getScheduleRuntime()
    if (!scheduleRuntime) return { ok: false, message: 'Schedule runtime is not initialized.' }
    return scheduleRuntime.runTask(normalizedTaskId)
  })

  ipcMain.handle(
    'claw:channel:mirror',
    async (_, payload: unknown) => {
      const request = parseIpcPayload('claw:channel:mirror', clawMirrorPayloadSchema, payload)
      const clawRuntime = getClawRuntime()
      if (!clawRuntime) return { ok: false as const, message: 'Connect Phone runtime is not initialized.' }
      return clawRuntime.mirrorThreadMessageToIm(
        request.threadId,
        request.text,
        request.direction
      )
    }
  )

  ipcMain.handle(
    'claw:channel:mirror-to-feishu',
    async (_, payload: unknown) => {
      const request = parseIpcPayload('claw:channel:mirror-to-feishu', clawMirrorPayloadSchema, payload)
      const clawRuntime = getClawRuntime()
      if (!clawRuntime) return { ok: false as const, message: 'Connect Phone runtime is not initialized.' }
      return clawRuntime.mirrorThreadMessageToIm(
        request.threadId,
        request.text,
        request.direction
      )
    }
  )

  ipcMain.handle(
    'claw:task:create-from-text',
    async (_, payload: unknown): Promise<ClawTaskFromTextResult> => {
      const request = parseIpcPayload(
        'claw:task:create-from-text',
        clawTaskFromTextPayloadSchema,
        payload
      )
      const scheduleRuntime = getScheduleRuntime()
      if (!scheduleRuntime) return { kind: 'error', message: 'Schedule runtime is not initialized.' }
      const settings = await store.load()
      const channel = request.channelId
        ? settings.claw.channels.find((item) => item.id === request.channelId)
        : undefined
      return scheduleRuntime.createScheduledTaskFromText(request.text, {
        workspaceRoot: channel?.workspaceRoot || settings.schedule.defaultWorkspaceRoot || settings.workspaceRoot,
        clawChannelId: channel?.id ?? request.channelId,
        providerId: request.providerId,
        modelHint: request.modelHint,
        reasoningEffort: request.reasoningEffort,
        mode: request.mode
      })
    }
  )

  ipcMain.handle(
    'schedule:task:create-from-text',
    async (_, payload: unknown): Promise<ScheduleTaskFromTextResult> => {
      const request = parseIpcPayload(
        'schedule:task:create-from-text',
        scheduleTaskFromTextPayloadSchema,
        payload
      )
      const scheduleRuntime = getScheduleRuntime()
      if (!scheduleRuntime) return { kind: 'error', message: 'Schedule runtime is not initialized.' }
      return scheduleRuntime.createScheduledTaskFromText(request.text, {
        workspaceRoot: request.workspaceRoot,
        clawChannelId: request.clawChannelId,
        providerId: request.providerId,
        modelHint: request.modelHint,
        reasoningEffort: request.reasoningEffort,
        mode: request.mode
      })
    }
  )

  ipcMain.handle(
    'claw:im-install:qrcode',
    async (_, payload: unknown) => {
      const request = parseIpcPayload(
        'claw:im-install:qrcode',
        z.object({ provider: z.enum(['feishu', 'weixin']), isLark: z.boolean().optional() }).strict(),
        payload
      )
      if (request.provider === 'weixin') {
        return startWeixinInstallQrcode()
      }
      return startFeishuInstallQrcode(request.isLark === true)
    }
  )

  ipcMain.handle(
    'claw:im-install:poll',
    async (_, payload: unknown) => {
      const request = parseIpcPayload('claw:im-install:poll', clawImInstallPollPayloadSchema, payload)
      const result = request.provider === 'weixin'
        ? await pollWeixinInstall(request.deviceCode)
        : await pollFeishuInstall(request.deviceCode)
      if (!result.done) return result
      const channelId = `im-${result.kind}-${randomUUID()}`
      if (result.kind === 'weixin') {
        const scope = {
          owner: 'transport', provider: 'weixin', accountId: result.accountId,
          channelId, purpose: 'transport-weixin-session-key'
        } as const
        const platformAccount = {
          kind: 'weixin' as const, accountId: result.accountId, createdAt: new Date().toISOString()
        }
        await imChannelAccountLifecycle.connect({
          channel: mainOwnedConnectedImChannel(channelId, platformAccount),
          scope,
          credential: { kind: 'weixin', sessionKey: result.token }
        })
        return {
          done: true as const, kind: result.kind, accountId: result.accountId, channelId,
          credentialConfigured: true as const, settingsCommitted: true as const
        }
      }
      const scope = {
        owner: 'transport', provider: 'feishu', accountId: result.appId,
        channelId, purpose: 'transport-feishu-app-secret'
      } as const
      const platformAccount = {
        kind: 'feishu' as const, accountId: result.appId, appId: result.appId,
        domain: result.domain, createdAt: new Date().toISOString()
      }
      await imChannelAccountLifecycle.connect({
        channel: mainOwnedConnectedImChannel(channelId, platformAccount),
        scope,
        credential: { kind: 'feishu', appSecret: result.appSecret }
      })
      return {
        done: true as const, kind: result.kind, appId: result.appId, domain: result.domain, channelId,
        credentialConfigured: true as const, settingsCommitted: true as const
      }
    }
  )

  ipcMain.handle(
    'claw:im-install:telegram-token',
    async (_, payload: unknown) => {
      const request = parseIpcPayload(
        'claw:im-install:telegram-token',
        clawImTelegramTokenPayloadSchema,
        payload
      )
      const verification = await verifyTelegramBotToken(request.botToken)
      if (!verification.ok) return verification
      const channelId = `im-telegram-${randomUUID()}`
      const scope = {
        owner: 'transport', provider: 'telegram', accountId: channelId,
        channelId, purpose: 'transport-telegram-bot-token'
      } as const
      const platformAccount = {
        kind: 'telegram' as const, accountId: channelId, allowedChatIds: request.allowedChatIds ?? '',
        ...(verification.botUsername ? { botUsername: verification.botUsername } : {}),
        createdAt: new Date().toISOString()
      }
      await imChannelAccountLifecycle.connect({
        channel: mainOwnedConnectedImChannel(channelId, platformAccount),
        scope,
        credential: { kind: 'telegram', botToken: request.botToken, allowedChatIds: request.allowedChatIds ?? '' }
      })
      return { ...verification, channelId, credentialConfigured: true as const, settingsCommitted: true as const }
    }
  )

  ipcMain.handle('claw:im-channel:disconnect', async (_, payload: unknown) => {
    const request = parseIpcPayload(
      'claw:im-channel:disconnect',
      z.object({ channelId: z.string().trim().min(1).max(512) }).strict(),
      payload
    )
    const currentSettings = await store.load()
    const channel = currentSettings.claw.channels.find((item) => item.id === request.channelId)
    if (!channel) return currentSettings
    const scope = accountCredentialScopeForClawImChannel(channel)
    if (!scope) throw new Error('The channel account binding is unavailable.')
    return imChannelAccountLifecycle.disconnect(request.channelId, scope)
  })

  ipcMain.handle('workspace:pick-directory', async (_, defaultPath: unknown): Promise<WorkspacePickResult> => {
    const normalizedDefaultPath = parseIpcPayload(
      'workspace:pick-directory',
      z.object({ defaultPath: defaultPathSchema }).strict(),
      { defaultPath }
    ).defaultPath
    const options: Electron.OpenDialogOptions = {
      title: 'Select working directory',
      defaultPath: normalizedDefaultPath,
      properties: ['openDirectory', 'createDirectory', 'dontAddToRecent']
    }
    const mainWindow = getMainWindow()
    const result = mainWindow
      ? await dialog.showOpenDialog(mainWindow, options)
      : await dialog.showOpenDialog(options)
    return {
      canceled: result.canceled,
      path: result.canceled ? null : (result.filePaths[0] ?? null)
    }
  })

  ipcMain.handle('file:pick-local-files', async (_, defaultPath: unknown): Promise<LocalFilesPickResult> => {
    const normalizedDefaultPath = parseIpcPayload(
      'file:pick-local-files',
      z.object({ defaultPath: defaultPathSchema }).strict(),
      { defaultPath }
    ).defaultPath
    const options: Electron.OpenDialogOptions = {
      title: 'Add files to conversation',
      defaultPath: normalizedDefaultPath,
      properties: ['openFile', 'multiSelections', 'dontAddToRecent']
    }
    const mainWindow = getMainWindow()
    const result = mainWindow
      ? await dialog.showOpenDialog(mainWindow, options)
      : await dialog.showOpenDialog(options)
    return {
      canceled: result.canceled,
      paths: result.canceled ? [] : result.filePaths
    }
  })

  // Replaces window.confirm in the renderer: the synchronous native confirm
  // leaves the WebContents unable to focus inputs after it closes
  // (electron/electron#19977), which froze the composer after deleting threads.
  ipcMain.handle('dialog:confirm', async (_, payload: unknown): Promise<boolean> => {
    const request = parseIpcPayload('dialog:confirm', confirmDialogPayloadSchema, payload)
    const options: Electron.MessageBoxOptions = {
      type: 'warning',
      buttons: [request.confirmLabel ?? 'OK', request.cancelLabel ?? 'Cancel'],
      defaultId: 0,
      cancelId: 1,
      message: request.message,
      detail: request.detail,
      noLink: true
    }
    const mainWindow = getMainWindow()
    const result = mainWindow
      ? await dialog.showMessageBox(mainWindow, options)
      : await dialog.showMessageBox(options)
    return result.response === 0
  })

  ipcMain.handle(
    'skill:save-file',
    async (_, payload: unknown) => {
      const request = parseIpcPayload('skill:save-file', skillSaveFilePayloadSchema, payload)
      return saveGuiSkillFile(request.rootPath, request.skillName, request.content)
    }
  )

  ipcMain.handle(
    'skill:delete',
    async (_, payload: unknown) => {
      const request = parseIpcPayload('skill:delete', skillDeletePayloadSchema, payload)
      return deleteGuiSkillPackage(request.rootPath, request.skillName)
    }
  )

  ipcMain.handle('skill:list', async (_, payload: unknown) => {
    const request = parseIpcPayload('skill:list', skillListPayloadSchema, payload)
    const settings = await store.load()
    return listGuiSkills(settings, request.workspaceRoot)
  })

  ipcMain.handle('skill:list-roots', async (_, payload: unknown) => {
    const request = parseIpcPayload('skill:list-roots', skillListPayloadSchema, payload)
    const settings = await store.load()
    return listGuiSkillRoots(settings, request.workspaceRoot)
  })

  ipcMain.handle('skill:open-root', async (_, rootPath: unknown) => {
    const normalizedRootPath = parseIpcPayload('skill:open-root', rootPathSchema, rootPath)
    try {
      const target = expandHomePath(normalizedRootPath)
      if (!target) {
        return { ok: false as const, message: 'Skill directory is required.' }
      }
      await mkdir(target, { recursive: true })
      return openPathWithShell(target)
    } catch (error) {
      return {
        ok: false as const,
        message: error instanceof Error ? error.message : String(error)
      }
    }
  })

  ipcMain.handle('ui-plugin:list', async () => {
    const analytixHomeDir = join(homedir(), '.analytix')
    await ensureBundledUiPlugins(analytixHomeDir)
    return { plugins: await listUiPlugins(analytixHomeDir) }
  })

  ipcMain.handle('ui-plugin:install', async () => {
    const mainWindow = getMainWindow()
    const options: Electron.OpenDialogOptions = {
      title: 'Select a UI plugin folder',
      properties: ['openDirectory', 'dontAddToRecent']
    }
    const picked = mainWindow
      ? await dialog.showOpenDialog(mainWindow, options)
      : await dialog.showOpenDialog(options)
    const sourceDir = picked.filePaths[0]
    if (picked.canceled || !sourceDir) {
      return { canceled: true as const }
    }
    const result = await installUiPluginFromDirectory(join(homedir(), '.analytix'), sourceDir)
    if (!result.ok) {
      return { canceled: false as const, ok: false as const, errors: result.errors }
    }
    return { canceled: false as const, ok: true as const, plugin: result.plugin }
  })

  ipcMain.handle('ui-plugin:remove', async (_, payload: unknown) => {
    const request = parseIpcPayload('ui-plugin:remove', uiPluginIdPayloadSchema, payload)
    return { ok: await removeUiPlugin(join(homedir(), '.analytix'), request.id) }
  })

  ipcMain.handle('ui-plugin:load', async (_, payload: unknown) => {
    const request = parseIpcPayload('ui-plugin:load', uiPluginIdPayloadSchema, payload)
    const analytixHomeDir = join(homedir(), '.analytix')
    await ensureBundledUiPlugins(analytixHomeDir)
    return loadUiPluginFigures(analytixHomeDir, request.id)
  })

  ipcMain.handle('hub-agent-marketplace:sync', async (_, payload: unknown) => {
    const request = parseIpcPayload(
      'hub-agent-marketplace:sync',
      hubAgentMarketplaceSyncPayloadSchema,
      payload ?? {}
    )
    const marketplace = await loadHubAgentMarketplaceService()
    return marketplace.syncHubAgentMarketplace(marketplace.hubAgentMarketplaceRuntimeHome(), {
      forceRefresh: request.forceRefresh,
      mode: request.mode
    })
  })

  ipcMain.handle('hub-agent-marketplace:install-plugin', async (_, payload: unknown) => {
    const request = parseIpcPayload(
      'hub-agent-marketplace:install-plugin',
      hubAgentPluginMutationPayloadSchema,
      payload
    )
    const marketplace = await loadHubAgentMarketplaceService()
    return marketplace.installHubAgentPlugin(marketplace.hubAgentMarketplaceRuntimeHome(), request)
  })

  ipcMain.handle('hub-agent-marketplace:uninstall-plugin', async (_, payload: unknown) => {
    const request = parseIpcPayload(
      'hub-agent-marketplace:uninstall-plugin',
      hubAgentPluginMutationPayloadSchema,
      payload
    )
    const marketplace = await loadHubAgentMarketplaceService()
    const result = await marketplace.uninstallHubAgentPlugin(
      marketplace.hubAgentMarketplaceRuntimeHome(),
      request
    )
    if (result.ok) await installedExtensionAccountLifecycle.deleteVerifiedPluginAccounts(result.pluginName)
    return result
  })

  ipcMain.handle('hub-agent-marketplace:read-skill-markdown', async (_, payload: unknown) => {
    const request = parseIpcPayload(
      'hub-agent-marketplace:read-skill-markdown',
      hubAgentSkillMarkdownPayloadSchema,
      payload
    )
    const marketplace = await loadHubAgentMarketplaceService()
    return marketplace.readHubAgentSkillMarkdown(marketplace.hubAgentMarketplaceRuntimeHome(), request)
  })

  ipcMain.handle('analytix:runtime-config:read', async () => {
    const path = resolveAnalytixConfigPath()
    try {
      const content = await readFile(path, 'utf8')
      return { path, content, exists: true as const }
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        return { path, content: '', exists: false as const }
      }
      throw error
    }
  })

  ipcMain.handle('analytix:runtime-config:write', async (_, content: unknown) => {
    const validatedContent = parseIpcPayload(
      'analytix:runtime-config:write',
      deepseekConfigContentSchema,
      content
    )
    const path = resolveAnalytixConfigPath()
    validateMcpConfigContent(validatedContent)
    await mkdir(dirname(path), { recursive: true })
    await atomicWriteFile(path, validatedContent, { mode: 0o600 })
    try {
      await onAnalytixMcpConfigWritten?.(path, validatedContent)
    } catch (error: unknown) {
      logError('mcp-config', 'Failed to apply MCP config change after write', {
        path,
        message: error instanceof Error ? error.message : String(error)
      })
    }
    return { ok: true as const, path }
  })

  ipcMain.handle('analytix:runtime-config:open-dir', async () => {
    try {
      const path = resolveAnalytixConfigPath()
      const dirPath = dirname(path)
      await mkdir(dirPath, { recursive: true })
      return openPathWithShell(dirPath)
    } catch (error) {
      return {
        ok: false as const,
        message: error instanceof Error ? error.message : String(error)
      }
    }
  })

  const resolveAnalytixThreadsDataDir = async (): Promise<string> => {
    const settings = await store.load()
    const runtime = resolveAnalytixRuntimeSettings(settings)
    return expandHomePath(runtime.dataDir?.trim() || DEFAULT_ANALYTIX_DATA_DIR)
  }

  ipcMain.handle('analytix:sessions:detect-legacy-kun', async () =>
    detectLegacySessions({ homeDir: homedir(), destDataDir: await resolveAnalytixThreadsDataDir() })
  )

  ipcMain.handle('analytix:sessions:import-legacy-kun', async (_, payload: unknown) => {
    const request = parseIpcPayload('analytix:sessions:import-legacy-kun', legacySessionImportPayloadSchema, payload)
    try {
      const summary = await importLegacySessions({
        homeDir: homedir(),
        destDataDir: await resolveAnalytixThreadsDataDir(),
        ...(request.sourceDir ? { sourceDir: request.sourceDir } : {}),
        log: (message, detail) => logError('legacy-session-import', message, detail)
      })
      return { ok: true as const, ...summary }
    } catch (error) {
      return {
        ok: false as const,
        message: error instanceof Error ? error.message : String(error)
      }
    }
  })

  ipcMain.handle('analytix:sessions:pick-legacy-kun-source-dir', async (): Promise<WorkspacePickResult> => {
    const options: Electron.OpenDialogOptions = {
      title: 'Select a folder containing previous conversations',
      properties: ['openDirectory', 'dontAddToRecent']
    }
    const mainWindow = getMainWindow()
    const result = mainWindow
      ? await dialog.showOpenDialog(mainWindow, options)
      : await dialog.showOpenDialog(options)
    return {
      canceled: result.canceled,
      path: result.canceled ? null : (result.filePaths[0] ?? null)
    }
  })

  ipcMain.handle('git:branches', async (_, workspaceRoot: unknown) =>
    getGitBranches(parseIpcPayload('git:branches', workspaceRootSchema, workspaceRoot))
  )
  ipcMain.handle(
    'git:switch-branch',
    async (_, payload: unknown) => {
      const request = parseIpcPayload('git:switch-branch', gitBranchPayloadSchema, payload)
      return switchGitBranch(request.workspaceRoot, request.branch)
    }
  )
  ipcMain.handle(
    'git:create-and-switch-branch',
    async (_, payload: unknown) => {
      const request = parseIpcPayload(
        'git:create-and-switch-branch',
        gitBranchPayloadSchema,
        payload
      )
      return createAndSwitchGitBranch(request.workspaceRoot, request.branch)
    }
  )
  ipcMain.handle('git:checkpoint:create', async (_, payload: unknown) => {
    const request = parseIpcPayload('git:checkpoint:create', gitCheckpointCreatePayloadSchema, payload)
	    return createGitCheckpoint({
	      dataDir: await resolveAnalytixThreadsDataDir(),
	      workspaceRoot: request.workspaceRoot,
	      threadId: request.threadId,
	      timeoutMs: request.timeoutMs
	    })
	  })
  ipcMain.handle('git:checkpoint:restore', async (_, payload: unknown) => {
    const request = parseIpcPayload('git:checkpoint:restore', gitCheckpointRestorePayloadSchema, payload)
	    return restoreGitCheckpoint({
	      dataDir: await resolveAnalytixThreadsDataDir(),
	      checkpointId: request.checkpointId,
	      allowPartialRestore: request.allowPartialRestore === true
	    })
	  })

  // Worktree pool management
  ipcMain.handle('worktree:acquire', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:acquire', worktreeOptionalRootSchema, payload)
    return acquireWorktree({
      projectPath: r.projectPath,
      poolIndex: r.poolIndex,
      taskId: r.taskId,
      force: r.force,
      worktreeRoot: r.worktreeRoot
    })
  })
  ipcMain.handle('worktree:release', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:release', worktreePoolIndexSchema, payload)
    return releaseWorktree({ projectPath: r.projectPath, poolIndex: r.poolIndex })
  })
  ipcMain.handle('worktree:list', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:list', worktreePoolSchema, payload)
    return listWorktrees({ projectPath: r.projectPath, worktreeRoot: r.worktreeRoot })
  })
  ipcMain.handle('worktree:remove', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:remove', worktreePoolIndexSchema, payload)
    return removeWorktree({
      projectPath: r.projectPath,
      poolIndex: r.poolIndex,
      worktreeRoot: r.worktreeRoot
    })
  })
  ipcMain.handle('worktree:changes', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:changes', worktreePathSchema, payload)
    return getWorktreeChanges({ worktreePath: r.worktreePath })
  })
  ipcMain.handle('worktree:commit', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:commit', worktreeCommitSchema, payload)
    return commitWorktree({ worktreePath: r.worktreePath, message: r.message })
  })
  ipcMain.handle('worktree:merge', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:merge', worktreeMergeSchema, payload)
    return mergeWorktreeToMain({
      projectPath: r.projectPath,
      poolIndex: r.poolIndex,
      commitMessage: r.commitMessage,
      worktreeRoot: r.worktreeRoot
    })
  })
  ipcMain.handle('worktree:abort-merge', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:abort-merge', worktreeProjectPathSchema, payload)
    return abortMerge({ projectPath: r.projectPath })
  })
  ipcMain.handle('worktree:continue-merge', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:continue-merge', worktreeContinueMergeSchema, payload)
    return continueMerge({ projectPath: r.projectPath, message: r.message })
  })
  ipcMain.handle('worktree:sync', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:sync', worktreePoolIndexSchema, payload)
    return syncWorktreeFromMain({
      projectPath: r.projectPath,
      poolIndex: r.poolIndex,
      worktreeRoot: r.worktreeRoot
    })
  })
  ipcMain.handle('worktree:abort-rebase', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:abort-rebase', worktreePathSchema, payload)
    return abortRebase({ worktreePath: r.worktreePath })
  })
  ipcMain.handle('worktree:cleanup', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:cleanup', worktreePoolSchema, payload)
    return cleanupWorktrees({ projectPath: r.projectPath, worktreeRoot: r.worktreeRoot })
  })
  ipcMain.handle('worktree:find-available', async (_, payload: unknown) => {
    const r = parseIpcPayload('worktree:find-available', worktreePoolSchema, payload)
    return findAvailablePoolIndex({ projectPath: r.projectPath, worktreeRoot: r.worktreeRoot })
  })

  ipcMain.handle('thread-handoff:start', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:start', threadHandoffStartPayloadSchema, payload)
    return { operation: await threadHandoffService.start(r) }
  })
  ipcMain.handle('thread-handoff:retry', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:retry', threadHandoffOperationIdPayloadSchema, payload)
    return { operation: await threadHandoffService.retry(r.operationId) }
  })
  ipcMain.handle('thread-handoff:get', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:get', threadHandoffGetPayloadSchema, payload ?? {})
    return { operations: threadHandoffService.get(r.operationId) }
  })
  ipcMain.handle('thread-handoff:cancel', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:cancel', threadHandoffOperationIdPayloadSchema, payload)
    return { operation: threadHandoffService.cancel(r.operationId) }
  })
  ipcMain.handle('thread-handoff:remove', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:remove', threadHandoffOperationIdPayloadSchema, payload)
    return { removed: threadHandoffService.remove(r.operationId) }
  })
  ipcMain.handle('thread-handoff:complete-switch', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:complete-switch', threadHandoffCompleteSwitchPayloadSchema, payload)
    return { operation: await threadHandoffService.completeSwitch(r) }
  })
  ipcMain.handle('thread-handoff:fail-switch', async (_, payload: unknown) => {
    const r = parseIpcPayload('thread-handoff:fail-switch', threadHandoffFailSwitchPayloadSchema, payload)
    return { operation: await threadHandoffService.failSwitch(r.operationId, r.message) }
  })

  ipcMain.handle('editor:list', async () => listEditorsResult())
  ipcMain.handle('editor:open-path', async (_, payload: unknown) =>
    openEditorPath(parseIpcPayload('editor:open-path', openEditorPathPayloadSchema, payload))
  )

  const packageHost = createPluginPackageHostHandler(localDisplayRequest)
  ipcMain.handle('plugin:package-host', async (event, payload: unknown) => {
    const main = getMainWindow()
    if (!main || main.isDestroyed() || event.sender !== main.webContents ||
        event.senderFrame !== main.webContents.mainFrame) {
      return { ok: false, code: 'identity_invalid', message: 'Plugin control requires the main workspace.' }
    }
    return packageHost(payload)
  })

  const objectEditing = createObjectEditingHandler(localDisplayRequest)
  ipcMain.handle('object:editing', async (event, payload: unknown) => {
    const main = getMainWindow()
    if (!main || main.isDestroyed() || event.sender !== main.webContents ||
        event.senderFrame !== main.webContents.mainFrame) {
      return { ok: false, code: 'forbidden', message: 'The protected editor is available only in the main workspace.' }
    }
    return objectEditing(payload)
  })
  ipcMain.handle('file:resolve-workspace', async (_, payload: unknown) =>
    resolveWorkspaceFile(
      parseIpcPayload('file:resolve-workspace', workspaceFileTargetPayloadSchema, payload)
    )
  )
  ipcMain.handle('file:list-workspace-directory', async (_, payload: unknown) =>
    listWorkspaceDirectory(
      parseIpcPayload('file:list-workspace-directory', workspaceDirectoryTargetPayloadSchema, payload)
    )
  )
  ipcMain.handle('file:read-workspace', async (_, payload: unknown) =>
    readWorkspaceFile(
      parseIpcPayload('file:read-workspace', workspaceFileTargetPayloadSchema, payload)
    )
  )
  ipcMain.handle('file:read-workspace-image', async (_, payload: unknown) =>
    readWorkspaceImage(
      parseIpcPayload('file:read-workspace-image', workspaceFileTargetPayloadSchema, payload)
    )
  )
  ipcMain.handle('file:read-workspace-pdf', async (_, payload: unknown) =>
    readWorkspacePdf(
      parseIpcPayload('file:read-workspace-pdf', workspaceFileTargetPayloadSchema, payload)
    )
  )
  ipcMain.handle('file:read-local-pdf-text', async (_, payload: unknown) => {
    const result = await readLocalPdfText(
      parseIpcPayload('file:read-local-pdf-text', localPdfTextTargetPayloadSchema, payload)
    )
    if (!result.ok) return result
    return {
      ok: true,
      path: result.path,
      size: result.size,
      mtimeMs: result.mtimeMs,
      pageCount: result.pageCount,
      text: result.pages.map((page) => page.text).join('\n\n'),
      hasText: result.hasText,
      truncated: result.truncated
    }
  })
  ipcMain.handle('file:save-as', async (_, payload: unknown) =>
    saveWorkspaceFileAs(payload, getMainWindow)
  )
  ipcMain.handle('file:write-workspace', async (_, payload: unknown) =>
    writeWorkspaceFile(
      parseIpcPayload('file:write-workspace', workspaceFileWritePayloadSchema, payload)
    )
  )
  ipcMain.handle('file:create-workspace', async (_, payload: unknown) =>
    createWorkspaceFile(
      parseIpcPayload('file:create-workspace', workspaceFileCreatePayloadSchema, payload)
    )
  )
  ipcMain.handle('file:create-workspace-directory', async (_, payload: unknown) =>
    createWorkspaceDirectory(
      parseIpcPayload('file:create-workspace-directory', workspaceDirectoryCreatePayloadSchema, payload)
    )
  )
  ipcMain.handle('file:save-workspace-clipboard-image', async (_, payload: unknown) =>
    saveWorkspaceClipboardImage(
      parseIpcPayload(
        'file:save-workspace-clipboard-image',
        workspaceClipboardImageSavePayloadSchema,
        payload
      )
    )
  )
  ipcMain.handle('clipboard:read-image', async () => readClipboardImage())
  ipcMain.handle('file:rename-workspace-entry', async (_, payload: unknown) =>
    renameWorkspaceEntry(
      parseIpcPayload('file:rename-workspace-entry', workspaceEntryRenamePayloadSchema, payload)
    )
  )
  ipcMain.handle('file:delete-workspace-entry', async (_, payload: unknown) =>
    deleteWorkspaceEntry(
      parseIpcPayload('file:delete-workspace-entry', workspaceEntryDeletePayloadSchema, payload)
    )
  )
  ipcMain.handle('file:watch-workspace', async (event, payload: unknown) => {
    const request = parseIpcPayload('file:watch-workspace', workspaceFileWatchPayloadSchema, payload)
    const initial = await readWorkspaceFile(request)
    let watchedPath: string
    let initialContent: string
    let initialSize: number
    let initialTruncated: boolean
    if (initial.ok) {
      watchedPath = initial.path
      initialContent = initial.content
      initialSize = initial.size
      initialTruncated = initial.truncated
    } else {
      const initialImage = await readWorkspaceImage(request)
      if (!initialImage.ok) return initial
      watchedPath = initialImage.path
      initialContent = ''
      initialSize = initialImage.size
      initialTruncated = false
    }

    const watchId = randomUUID()
    try {
      const watcher = watch(watchedPath, { persistent: false }, () => {
        scheduleWorkspaceFileChange(watchId)
      })
      workspaceFileWatchers.set(watchId, {
        watcher,
        sender: event.sender,
        path: watchedPath,
        workspaceRoot: request.workspaceRoot,
        timer: null
      })
      event.sender.once('destroyed', () => disposeWorkspaceFileWatchesForSender(event.sender))
      return {
        ok: true as const,
        watchId,
        path: watchedPath,
        content: initialContent,
        size: initialSize,
        truncated: initialTruncated,
        startedAt: new Date().toISOString()
      }
    } catch (error) {
      return {
        ok: false as const,
        message: error instanceof Error ? error.message : String(error)
      }
    }
  })
  ipcMain.handle('file:unwatch-workspace', async (_, watchId: unknown) =>
    disposeWorkspaceFileWatch(parseIpcPayload('file:unwatch-workspace', streamIdSchema, watchId))
  )
  ipcMain.handle('write:export', async (event, payload: unknown) => {
    const request = parseIpcPayload('write:export', writeExportPayloadSchema, payload)
    if (!localDisplayRendererIsCurrent(event, getMainWindow)) {
      return {
        ok: false as const,
        canceled: false as const,
        message: 'Write export requires the current main window.'
      }
    }

    const initialSettings = await store.load()
    const configuredWorkspace = initialSettings.write.activeWorkspaceRoot.trim()
    if (!configuredWorkspace) {
      return {
        ok: false as const,
        canceled: false as const,
        message: 'The active Write workspace is unavailable.'
      }
    }
    const workspaceRoot = await canonicalPath(resolve(expandHomePath(configuredWorkspace)))
    const authorityCurrent = async (): Promise<boolean> => {
      if (!localDisplayRendererIsCurrent(event, getMainWindow)) return false
      try {
        const currentSettings = await store.load()
        const currentWorkspace = currentSettings.write.activeWorkspaceRoot.trim()
        if (!currentWorkspace) return false
        return await canonicalPath(resolve(expandHomePath(currentWorkspace))) === workspaceRoot
      } catch {
        return false
      }
    }

    return exportWriteDocument(request, {
      parentWindow: getMainWindow(),
      workspaceRoot,
      authorityCurrent
    })
  })
  ipcMain.handle('write:copy-rich-text', async (event, payload: unknown) => {
    const request = parseIpcPayload('write:copy-rich-text', writeRichClipboardPayloadSchema, payload)
    if (!localDisplayRendererIsCurrent(event, getMainWindow)) {
      return {
        ok: false as const,
        message: 'Write export requires the current main window.'
      }
    }

    const initialSettings = await store.load()
    const configuredWorkspace = initialSettings.write.activeWorkspaceRoot.trim()
    if (!configuredWorkspace) {
      return {
        ok: false as const,
        message: 'The active Write workspace is unavailable.'
      }
    }
    const workspaceRoot = await canonicalPath(resolve(expandHomePath(configuredWorkspace)))
    const authorityCurrent = async (): Promise<boolean> => {
      if (!localDisplayRendererIsCurrent(event, getMainWindow)) return false
      try {
        const currentSettings = await store.load()
        const currentWorkspace = currentSettings.write.activeWorkspaceRoot.trim()
        if (!currentWorkspace) return false
        return await canonicalPath(resolve(expandHomePath(currentWorkspace))) === workspaceRoot
      } catch {
        return false
      }
    }

    return copyWriteDocumentAsRichText(request, {
      workspaceRoot,
      authorityCurrent
    })
  })
  ipcMain.handle('write:inline-completion', async (_, payload: unknown) =>
    requestWriteInlineCompletion(
      await store.load(),
      parseIpcPayload('write:inline-completion', writeInlineCompletionPayloadSchema, payload),
      runtimeRequest
    )
  )
  ipcMain.handle('write:retrieve-context', async (_, payload: unknown) => {
    try {
      const context = await retrieveWriteContext(
        parseIpcPayload('write:retrieve-context', writeRetrievalPayloadSchema, payload)
      )
      return { ok: true as const, context }
    } catch (error) {
      return {
        ok: false as const,
        message: error instanceof Error ? error.message : String(error)
      }
    }
  })
  ipcMain.handle('write:generate-infographic', async (_, payload: unknown) =>
    requestWriteInfographic(
      await store.load(),
      parseIpcPayload('write:generate-infographic', writeInfographicPayloadSchema, payload),
      { executeMediaRequest: (request) => executePrivateMediaImage(privateMediaRequest, request) }
    )
  )
  ipcMain.handle('write:authorize-prototype', async (_, payload: unknown) => {
    const request = parseIpcPayload('write:authorize-prototype', writePrototypeFilePayloadSchema, payload)
    return authorizePrototypePath(request.path, request.workspaceRoot)
  })
  ipcMain.handle('write:open-prototype', async (_, payload: unknown) => {
    const request = parseIpcPayload('write:open-prototype', writePrototypeFilePayloadSchema, payload)
    const authorized = await authorizePrototypePath(request.path, request.workspaceRoot)
    if (!authorized.ok) return authorized
    return openPathWithShell(authorized.absolutePath)
  })
  ipcMain.handle('speech:transcribe', async (_, payload: unknown) =>
    requestSpeechTranscription(
      await store.load(),
      parseIpcPayload('speech:transcribe', speechTranscribePayloadSchema, payload),
      { executeMediaRequest: (request) => executePrivateMediaSpeech(privateMediaRequest, request) }
    )
  )
  ipcMain.handle('write:inline-completion-debug:list', async () => listWriteInlineCompletionDebugEntries())
  ipcMain.handle('write:inline-completion-debug:clear', async () => {
    clearWriteInlineCompletionDebugEntries()
    return true
  })
  ipcMain.handle('desktop:command', async (event, command: unknown) => {
    runDesktopCommand(
      parseIpcPayload('desktop:command', desktopCommandSchema, command),
      event.sender,
      getMainWindow
    )
  })
  ipcMain.handle('shell:open-external', async (_, url: unknown) => {
    const validatedUrl = parseIpcPayload('shell:open-external', shellOpenExternalUrlSchema, url)
    await shell.openExternal(validatedUrl)
  })
  ipcMain.handle('computer-use:permissions', async () => getComputerUsePermissions())
  ipcMain.handle('computer-use:doctor', async () => getComputerUseDoctor())
  ipcMain.handle('chrome-browser-use:status', async () => getChromeBrowserUseStatus())
  ipcMain.handle('chrome-browser-use:open-extension-page', async (_, target: unknown) =>
    openChromeBrowserUseExtensionPage(
      parseIpcPayload(
        'chrome-browser-use:open-extension-page',
        chromeBrowserUseExtensionPageTargetSchema,
        target
      )
    )
  )
  ipcMain.handle('computer-use:request-permission', async (_, kind: unknown) => {
    const parsed = parseIpcPayload(
      'computer-use:request-permission',
      computerUsePermissionKindSchema,
      kind
    )
    return requestComputerUsePermission(parsed)
  })
  ipcMain.handle('notification:turn-complete', async (_, payload: unknown) =>
    showTurnCompleteNotification(
      parseIpcPayload('notification:turn-complete', notificationPayloadSchema, payload)
    )
  )
  ipcMain.handle('thread:open-new-window', async (_, payload: unknown) => {
    const request = parseIpcPayload('thread:open-new-window', openThreadWindowPayloadSchema, payload)
    openThreadInNewWindow(request.threadId)
  })
  ipcMain.handle('app:version', async () => getAppVersion())
  ipcMain.handle('gui:update-state', async () => readGuiUpdateState())
  ipcMain.handle('gui:update-check', async (_, channel: unknown): Promise<GuiUpdateInfo> => {
    const module = await loadGuiUpdaterModule()
    return module.checkGuiUpdate(
      parseIpcPayload(
        'gui:update-check',
        z.object({ channel: guiUpdateChannelSchema }).strict(),
        { channel }
      ).channel
    )
  })
  ipcMain.handle('gui:update-download', async (_, channel: unknown): Promise<GuiUpdateDownloadResult> => {
    const module = await loadGuiUpdaterModule()
    return module.downloadGuiUpdate(
      parseIpcPayload(
        'gui:update-download',
        z.object({ channel: guiUpdateChannelSchema }).strict(),
        { channel }
      ).channel
    )
  })
  ipcMain.handle('gui:update-install', async (): Promise<GuiUpdateInstallResult> => {
    const module = await loadGuiUpdaterModule()
    return module.installGuiUpdate()
  })

  ipcMain.handle('log:error', async (_, payload: unknown) => {
    const request = parseIpcPayload('log:error', logErrorPayloadSchema, payload)
    logError('renderer-ipc', 'Renderer diagnostic reported', request)
  })
  ipcMain.handle('log:get-path', async () => resolveLogDirectory())
  ipcMain.handle('log:open-dir', async () => {
    const dir = resolveLogDirectory()
    try {
      await mkdir(dir, { recursive: true })
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      return { ok: false, message }
    }
    const error = await shell.openPath(dir)
    if (error) return { ok: false, message: error }
    return { ok: true }
  })
  ipcMain.handle('diagnostics:thread-trace', async (_, payload: unknown) =>
    appendThreadTraceEvent(
      app.getPath('userData'),
      parseIpcPayload('diagnostics:thread-trace', threadTraceEventPayloadSchema, payload)
    )
  )
}
