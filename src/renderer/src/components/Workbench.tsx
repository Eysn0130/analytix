import { nativeWorkspaceCommandFromInput, type NativeWorkspaceCommand } from '@shared/native-office'
import { useNativeReferenceStore, nativeReferencesPrompt, nativeActionReferencesCurrent, type NativeReference } from '../office/native-reference-store'
import { workbenchReferencesCurrent } from '../write/workbench-reference-snapshot'
import { useThreadComposerDraft } from './chat/use-thread-composer-draft'
import { isNativeOfficeFilePath, isWriteWorkspaceFilePath } from '@shared/write-text-file'
import type { CSSProperties, ReactElement } from 'react'
import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useShallow } from 'zustand/react/shallow'
import { ArrowDown } from 'lucide-react'
import {
  modelSupportsImageInput,
  type ApprovalPolicy,
  type ModelProviderModelProfileV1,
  type SandboxMode
} from '@shared/app-settings'
import { parseClawCommand } from '@shared/claw-commands'
import { DEFAULT_COMPOSER_MODEL_IDS } from '@shared/default-composer-models'
import { buildGuiPlanId, buildPlanRelativePath } from '@shared/gui-plan'
import { sddDraftTraceRelativePath } from '@shared/sdd'
import { buildSddTraceSnapshot } from '@shared/sdd-trace'
import {
  findKeyboardShortcutCommand,
  keyboardEventToShortcut,
  resolveKeyboardShortcutBindings,
  type KeyboardShortcutCommandId
} from '@shared/keyboard-shortcuts'
import type { DesktopCommand, ModelProviderModelGroup, SkillListItem } from '@shared/analytix-api'
import type { ClipboardImageReadResult, WorkspaceFileTarget } from '@shared/workspace-file'
import type { AttachmentReference, ChatBlock, NormalizedThread, UserFileReference } from '../agent/types'
import { projectAttachmentReferencesForPublicSurfaces } from '../agent/attachment-public'
import type {
  CoreRuntimeInfoJson,
  CoreRuntimeSkillJson,
  CoreThreadSummarySubagentJson
} from '../agent/analytix-contract'
import { getProvider } from '../agent/registry'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore } from '../store/chat-store'
import { isClawThread, providerIdForComposerModel } from '../store/chat-store-helpers'
import { threadHasPendingRuntimeWork } from '../store/chat-store-runtime-helpers'
import type { AppRoute, PluginHostRoute, SideConversation } from '../store/chat-store-types'
import {
  extractLatestTurnAutoOpenDevPreviewUrls,
  extractLatestTurnDevPreviewUrls
} from '../lib/dev-preview-detection'
import { Sidebar } from './chat/Sidebar'
import { WorkbenchTopBar, type RightPanelMode } from './chat/WorkbenchTopBar'
import { useWorkspaceTabsStore, workspaceObjectTabId, type WorkspaceTab } from '../store/workspace-tabs-store'
import { useNativeOfficeStore } from '../office/native-office-store'
import { WorkspaceTabs, WorkspaceToolSelector, workspacePanelDomId, workspaceTabDomId } from './workbench/WorkspaceTabs'
import { MascotCameoLayer, CameoCelebrationLayer } from './chat/AnimatedWorkLogo'
import { preloadAssistantMarkdownRenderer } from './chat/AssistantMarkdown'
import type {
  ComposerExecutionSettings,
  ComposerFileReference
} from './chat/FloatingComposer'
import { ChatFileTreePanel, type ChatFileTreeReference } from './chat/ChatFileTreePanel'
import {
  composerModelIdLooksMultimodal,
  composerReasoningEffortRequestValue,
  type ComposerReasoningEffort
} from './chat/FloatingComposerModelPicker'
import { SideConversationPanel } from './chat/SideConversationPanel'
import { SessionHeader } from './SessionHeader'
import { ShellNavigationControls } from './shell/ShellNavigationControls'
import { prepareWorkbenchDocumentMessage, workbenchEmptyMessageKeys } from '../write/workbench-document-message'
import { resolveWriteAgentPreset } from '../write/agent-presets'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import { buildSddDraftId, createSddDraft, forgetRememberedSddDraft, useSddDraftStore } from '../sdd/sdd-draft-store'
import type { SddDraft, SddDraftSaveStatus } from '../sdd/sdd-draft-store'
import { listSddDraftHistory, titleFromSddDraftContent } from '../sdd/sdd-draft-history'
import { saveActiveSddDraftToDisk } from '../sdd/sdd-draft-actions'
import { restoreRememberedSddDraft, restoreSddDraft } from '../sdd/sdd-draft-restore'
import { composeSddAssistantPrompt } from '../sdd/sdd-assistant-prompt'
import { frameworkById } from '../sdd/pm-skill-frameworks'
import { collectSddDraftImages, withAttachmentIds, type SddDraftImageReference } from '../sdd/sdd-draft-images'
import { PENDING_INFOGRAPHIC_PROTOCOL } from '../write/infographic-pending'
import { buildSddDraftToPlanPrompt } from '../sdd/sdd-plan-prompt'
import {
  isSddAssistantThread,
  isEmptySddAssistantThreadCandidate,
  markSddAssistantThread,
  releaseSddAssistantThread,
  sddAssistantThreadIdForDraft
} from '../sdd/sdd-thread-registry'
import {
  refreshSddChatTranscriptFromProvider,
  sddDraftRefForThreadId,
  writeSddChatTranscriptForThread
} from '../sdd/sdd-chat-transcript'
import { parseGuiPlanCommand } from '../plan/plan-command'
import { confirmDialog } from '../lib/confirm-dialog'
import { DevPreviewLaunchCard } from './DevPreviewLaunchCard'
import { RuntimeBanner } from './RuntimeBanner'
import type { TimelineReturnToBottomState } from './chat/MessageTimeline'
import { CODE_PANEL_PREFERRED, createWorkbenchScrollReserve, useWorkbenchLayout } from './workbench-layout'
import { useWorkbenchPlanController } from './workbench-plan-controller'
import { prepareImageAttachmentUpload } from '../lib/image-attachment-upload'
import { canUploadImageAttachment, isChatAttachmentUploadEnabled } from '../lib/attachment-upload-availability'
import { normalizeWorkspaceRoot, workspaceRootIdentityKey } from '../lib/workspace-path'
import {
  preloadCaseOverviewForWorkspace
} from '../data-analysis/services/analysis/stats-case-overview-resource'
import { preloadSharedThreadSummary } from './summary/thread-summary-resource'
import { readThreadWorktreeRegistry } from '../lib/thread-worktree-registry'
import { useKeyboardShortcutSettings } from '../lib/keyboard-shortcut-settings'
import { useUiModeCameosEnabled, useUiPluginStore } from '../store/ui-plugin-store'
import { readFocusModePreference, writeFocusModePreference } from '../lib/focus-mode'
import {
  listenRuntimeDiagnosticsFocus,
  type RuntimeDiagnosticsFocus
} from '../lib/runtime-diagnostics-focus'
import { createComposerController, createComposerViewState } from '../composer/composer-controller'
import {
  buildComposerFileContextPrompt,
  composerFileReferenceFromPath,
  isComposerDirectoryReference,
  mergeComposerFileReferences,
  type ComposerFileContextEntry
} from '../lib/composer-file-references'
import { filesUnderDirectory, loadWorkspaceFileIndex } from '../lib/workspace-file-index'
import { AnalytixLoadingPage } from './brand/AnalytixLoadingPage'
import { ChatTimelineIsland, useDevPreviewUrls } from './workbench/ChatTimelineIsland'
import { FloatingComposerIsland } from './workbench/FloatingComposerIsland'
import {
  ChangeInspectorIsland,
  DevBrowserPanelIsland,
  PlanPanel,
  SddAssistantPanelIsland,
  SubagentInspectorPanelIsland,
  ThreadSummaryPanelIsland,
  TodoPanel,
  WorkspaceFilePreviewPanel,
  DocumentWorkspacePanel,
  preloadRightPanelIsland
} from './workbench/RightPanelIslands'
import { WorkbenchResizeHandle } from './workbench/WorkbenchResizeHandle'
import { WorkbenchShell, WorkbenchStage } from './workbench/WorkbenchShell'
import { useShellPanelMotion } from './workbench/useShellPanelMotion'
import type { DataAnalysisItemId } from '../data-analysis/DataAnalysisSurface'

const loadSddDraftEditorView = () =>
  import('./sdd/SddDraftEditorView').then((module) => ({ default: module.SddDraftEditorView }))
const loadDataAnalysisSurface = () =>
  import('../data-analysis/DataAnalysisSurface').then((module) => ({ default: module.DataAnalysisSurface }))
const SddDraftEditorView = lazy(loadSddDraftEditorView)
const DataAnalysisSurface = lazy(loadDataAnalysisSurface)

function runtimeFileReferencesFromComposer(
  references: ComposerFileReference[]
): UserFileReference[] {
  return references
    .map((reference) => ({
      path: reference.path.trim(),
      relativePath: reference.relativePath.trim(),
      name: reference.name.trim(),
      ...(reference.type === 'directory' ? { kind: 'directory' as const } : { kind: 'file' as const })
    }))
    .filter((reference) => reference.path && reference.relativePath && reference.name)
}
const loadPluginMarketplaceView = () =>
  import('./PluginMarketplaceView').then((module) => ({ default: module.PluginMarketplaceView }))
const loadTerminalPanel = () =>
  import('./terminal/TerminalPanel').then((module) => ({ default: module.TerminalPanel }))
const loadScheduleTasksView = () =>
  import('./schedule/ScheduleTasksView').then((module) => ({ default: module.ScheduleTasksView }))
const PluginMarketplaceView = lazy(loadPluginMarketplaceView)
const TerminalPanel = lazy(loadTerminalPanel)
const ScheduleTasksView = lazy(loadScheduleTasksView)

type PendingSddPlanTarget = {
  planId: string
  relativePath: string
  workspaceRoot: string
}

type DockedRightPanelRenderMode = Exclude<RightPanelMode, null>
type WorkbenchLoadingSurface = 'main' | 'sidebar' | 'surface'
type SummaryDisplayMode = 'overlay' | 'shift' | 'gutter'

type ShellHistoryEntry = {
  route: AppRoute
  connectPhoneSidebarOpen: boolean
  pluginHostRoute: PluginHostRoute
}

type ActiveDataAnalysis = {
  workspaceRoot: string
  itemId: DataAnalysisItemId
}

function ComposerReturnToBottomButton({
  state
}: {
  state: TimelineReturnToBottomState | null
}): ReactElement {
  const { t } = useTranslation('common')
  const show = state?.show ?? false
  const labelKey = state?.hasLiveActivity ? 'timelineBackToLive' : 'timelineBackToBottom'
  const label = t(labelKey)
  const visibilityClass = show ? 'opacity-100' : 'pointer-events-none opacity-0'

  return (
    <button
      type="button"
      data-analytix-return-to-bottom-button
      className={`absolute bottom-[calc(100%+1.5rem)] left-1/2 z-30 flex h-8 w-8 -translate-x-1/2 items-center justify-center rounded-full border border-white/60 bg-white/65 bg-clip-padding text-ds-ink shadow-[0_12px_30px_rgba(48,72,104,0.18)] backdrop-blur-2xl backdrop-saturate-150 transition-[opacity,background-color,border-color,box-shadow] duration-150 ease-in-out hover:border-white/75 hover:bg-white/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 dark:border-white/10 dark:bg-ds-elevated/70 dark:text-ds-ink dark:shadow-[0_18px_40px_rgba(0,0,0,0.32)] dark:hover:bg-ds-elevated/85 ${visibilityClass}`}
      title={label}
      aria-label={label}
      aria-hidden={!show}
      tabIndex={show ? 0 : -1}
      onClick={show ? state?.onClick : undefined}
    >
      {state?.hasLiveActivity ? (
        <span aria-hidden className="flex items-center justify-center gap-1">
          <span className="motion-safe:animate-bounce h-1 w-1 rounded-full bg-current opacity-75 [animation-delay:-0.16s]" />
          <span className="motion-safe:animate-bounce h-1 w-1 rounded-full bg-current opacity-75 [animation-delay:-0.08s]" />
          <span className="motion-safe:animate-bounce h-1 w-1 rounded-full bg-current opacity-75" />
        </span>
      ) : (
        <ArrowDown className="h-4 w-4" aria-hidden />
      )}
    </button>
  )
}

type SubagentInspectorState = {
  parentThreadId: string
  selectedKey: string | null
  subagents: CoreThreadSummarySubagentJson[]
}

const SIDE_CONVERSATION_INSPECTOR_KEY_PREFIX = 'side-conversation:'

function sideConversationInspectorKey(threadId: string): string {
  return `${SIDE_CONVERSATION_INSPECTOR_KEY_PREFIX}${threadId}`
}

function sideConversationThreadIdFromInspectorKey(key: string | null | undefined): string | null {
  if (!key?.startsWith(SIDE_CONVERSATION_INSPECTOR_KEY_PREFIX)) return null
  return key.slice(SIDE_CONVERSATION_INSPECTOR_KEY_PREFIX.length).trim() || null
}

function sideConversationInspectorItem(side: SideConversation): CoreThreadSummarySubagentJson {
  const title = side.title.trim() || 'Side chat'
  return {
    schemaVersion: 1,
    id: sideConversationInspectorKey(side.threadId),
    key: sideConversationInspectorKey(side.threadId),
    parentThreadId: side.parentThreadId,
    childThreadId: side.threadId,
    taskKind: 'unknown',
    displayName: title,
    title,
    label: title,
    model: side.model,
    status: side.busy ? 'active' : 'done',
    rawStatus: side.busy ? 'running' : 'done',
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canContinueParent: false,
    canReadOutput: false,
    canOpenThread: true,
    canKill: false,
    canRestart: false,
    createdAt: side.createdAt,
    updatedAt: side.createdAt
  }
}

function mergeSubagentInspectorItems(
  subagents: CoreThreadSummarySubagentJson[],
  sideChats: CoreThreadSummarySubagentJson[]
): CoreThreadSummarySubagentJson[] {
  if (sideChats.length === 0) return subagents
  const seen = new Set(subagents.map((agent) => agent.key))
  const merged = [...subagents]
  for (const sideChat of sideChats) {
    if (seen.has(sideChat.key)) continue
    seen.add(sideChat.key)
    merged.push(sideChat)
  }
  return merged
}

function sameSubagentInspectorItems(
  left: CoreThreadSummarySubagentJson[],
  right: CoreThreadSummarySubagentJson[]
): boolean {
  if (left.length !== right.length) return false
  return left.every((item, index) => {
    const other = right[index]
    return Boolean(other) &&
      item.key === other.key &&
      item.childThreadId === other.childThreadId &&
      item.displayName === other.displayName &&
      item.agentNickname === other.agentNickname &&
      item.title === other.title &&
      item.label === other.label &&
      item.status === other.status &&
      item.rawStatus === other.rawStatus &&
      item.updatedAt === other.updatedAt
  })
}

function WorkbenchLoadingFallback({
  className = '',
  surface = 'main'
}: {
  className?: string
  surface?: WorkbenchLoadingSurface
}): ReactElement {
  return (
    <AnalytixLoadingPage
      className={className}
      fillParent
      label="正在加载 Analytix 工作区"
      surface={surface}
    />
  )
}

function shellHistoryEntryKey(entry: ShellHistoryEntry): string {
  return `${entry.route}:${entry.pluginHostRoute}:${entry.connectPhoneSidebarOpen ? 'phone' : 'main'}`
}

const COMPOSER_FILE_CONTEXT_MAX_CHARS_PER_FILE = 60_000
const COMPOSER_FILE_CONTEXT_MAX_TOTAL_CHARS = 180_000
// Upper bound on how many files a single `@directory` mention expands into, so a
// large folder cannot flood the prompt (the char budget above is the hard cap).
const COMPOSER_DIRECTORY_CONTEXT_MAX_FILES = 60
const SDD_ASSISTANT_TITLE_SYNC_DELAY_MS = 900
const SUBAGENT_PANEL_PREFERRED_WIDTH = 360
const DESKTOP_SHORTCUT_COMMANDS: Partial<Record<KeyboardShortcutCommandId, DesktopCommand>> = {
  quit: 'quit',
  undo: 'undo',
  redo: 'redo',
  cut: 'cut',
  copy: 'copy',
  paste: 'paste',
  'select-all': 'selectAll',
  reload: 'reload',
  'zoom-in': 'zoomIn',
  'zoom-out': 'zoomOut',
  'reset-zoom': 'resetZoom',
  'toggle-devtools': 'toggleDevTools',
  close: 'close',
  minimize: 'minimize',
  'toggle-maximize': 'toggleMaximize'
}

function workspaceFileTargetKey(target: WorkspaceFileTarget | null | undefined): string {
  if (!target?.path) return ''
  return `${target.workspaceRoot ?? ''}\n${target.path}`.replaceAll('\\', '/').toLowerCase()
}

function normalizeModelCapabilityKey(modelId: string): string {
  return modelId.trim().toLowerCase()
}

function modelProfileForGroup(
  group: ModelProviderModelGroup,
  modelId: string
): ModelProviderModelProfileV1 | undefined {
  const key = normalizeModelCapabilityKey(modelId)
  if (!key) return undefined
  const profiles = group.modelProfiles ?? {}
  const direct = profiles[key] ?? profiles[modelId.trim()]
  if (direct) return direct
  return Object.values(profiles).find((profile) =>
    profile.aliases?.some((alias) => normalizeModelCapabilityKey(alias) === key)
  )
}

function modelProfileForSelection(
  groups: readonly ModelProviderModelGroup[],
  modelId: string,
  providerId?: string
): ModelProviderModelProfileV1 | undefined {
  const selectedProviderId = providerId?.trim()
  if (selectedProviderId) {
    const selectedGroup = groups.find((group) => group.providerId === selectedProviderId)
    if (selectedGroup) {
      const profile = modelProfileForGroup(selectedGroup, modelId)
      if (profile) return profile
    }
  }
  for (const group of groups) {
    const profile = modelProfileForGroup(group, modelId)
    if (profile) return profile
  }
  return undefined
}

function readUiScale(): number {
  if (typeof window === 'undefined') return 1
  const rootScale = Number.parseFloat(
    window.getComputedStyle(document.documentElement).getPropertyValue('--ds-ui-scale')
  )
  if (Number.isFinite(rootScale) && rootScale > 0) return rootScale
  const bodyZoom = Number.parseFloat(window.getComputedStyle(document.body).zoom)
  return Number.isFinite(bodyZoom) && bodyZoom > 0 ? bodyZoom : 1
}

function fileNameFromPath(path: string): string {
  return path.replaceAll('\\', '/').split('/').filter(Boolean).pop() || 'image'
}

function isPdfAttachmentFile(file: File): boolean {
  return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')
}

function imageMimeTypeFromFileName(name: string | undefined): string | undefined {
  const lower = name?.toLowerCase() ?? ''
  if (lower.endsWith('.png')) return 'image/png'
  if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg'
  if (lower.endsWith('.webp')) return 'image/webp'
  if (lower.endsWith('.gif')) return 'image/gif'
  if (lower.endsWith('.bmp')) return 'image/bmp'
  if (lower.endsWith('.avif')) return 'image/avif'
  if (lower.endsWith('.heic')) return 'image/heic'
  if (lower.endsWith('.heif')) return 'image/heif'
  return undefined
}

function isImageAttachmentFile(file: File): boolean {
  return file.type.toLowerCase().startsWith('image/') || Boolean(imageMimeTypeFromFileName(file.name))
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let index = 0; index < bytes.length; index += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(index, index + 0x8000))
  }
  return btoa(binary)
}

function clipComposerFileContext(
  content: string,
  remainingChars: number,
  sourceTruncated: boolean
): { content: string; truncated: boolean; consumed: number } {
  const limit = Math.max(0, Math.min(COMPOSER_FILE_CONTEXT_MAX_CHARS_PER_FILE, remainingChars))
  const clipped = content.slice(0, limit)
  return {
    content: clipped,
    truncated: sourceTruncated || clipped.length < content.length,
    consumed: clipped.length
  }
}

function sddDraftPlanRelativePath(draft: SddDraft): string {
  const parts = draft.relativePath.replaceAll('\\', '/').split('/').filter(Boolean)
  const draftFolder = parts.at(-2)?.trim() || draft.id.split(':').pop()?.trim() || `draft-${Date.now()}`
  return buildPlanRelativePath(`sdd-${draftFolder}`)
}

function sddDraftSourceRequest(markdown: string, fallbackPath: string): string {
  const firstMeaningfulLine = markdown
    .split('\n')
    .map((line) => line.replace(/^#+\s*/, '').trim())
    .find(Boolean)
  return (firstMeaningfulLine || fallbackPath).slice(0, 160)
}

function sddAssistantThreadTitle(markdown: string, fallback: string): string {
  return titleFromSddDraftContent(markdown, fallback).trim() || fallback
}

function sddPlanMatchesPendingTarget(
  plan: { id: string; workspaceRoot: string; relativePath: string } | null,
  target: PendingSddPlanTarget | null
): boolean {
  if (!plan || !target) return false
  if (plan.id === target.planId) return true
  return buildGuiPlanId(plan.workspaceRoot, plan.relativePath) === target.planId
}

function mergeSkillCommands(
  runtimeSkills: CoreRuntimeSkillJson[],
  localSkills: SkillListItem[]
): CoreRuntimeSkillJson[] {
  const merged = new Map<string, CoreRuntimeSkillJson>()
  for (const skill of localSkills) {
    merged.set(skill.id, {
      id: skill.id,
      name: skill.name,
      description: skill.description,
      root: skill.root,
      legacy: skill.legacy,
      scope: skill.scope
    })
  }
  for (const skill of runtimeSkills) {
    const existing = merged.get(skill.id)
    merged.set(skill.id, existing ? {
      ...skill,
      ...existing,
      triggers: skill.triggers ?? existing.triggers,
      allowedTools: skill.allowedTools ?? existing.allowedTools
    } : skill)
  }
  return [...merged.values()]
}

function sddAssistantContextFromBlocks(blocks: ChatBlock[], maxMessages = 10): string {
  const messages: string[] = []
  for (const block of blocks) {
    if (block.kind !== 'user' && block.kind !== 'assistant') continue
    if (block.kind === 'user' && block.meta?.displayText) continue
    const text = block.text.trim()
    if (!text) continue
    messages.push(`${block.kind === 'user' ? 'User' : 'Requirement AI'}:\n${text}`)
  }
  return messages.slice(-maxMessages).join('\n\n').slice(0, 12_000)
}

function base64ImageToFile(image: SddDraftImageReference): File {
  return base64ToFile(image.dataBase64, fileNameFromPath(image.relativePath), image.mimeType)
}

function clipboardImageToFile(image: Extract<ClipboardImageReadResult, { ok: true }>): File {
  return base64ToFile(image.dataBase64, image.name, image.mimeType)
}

function base64ToFile(dataBase64: string, name: string, mimeType: string): File {
  const binary = atob(dataBase64)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return new File([bytes], name || 'image', { type: mimeType })
}

export function Workbench(): ReactElement {
  const { t } = useTranslation('common')
  const {
		    threads,
		    caseProjects,
		    caseProjectIndexStatus,
		    caseProjectThreadsById,
	    caseProjectLoadingById,
	    caseProjectErrorsById,
	    caseProjectExpandedById,
	    threadSearch,
    showArchivedThreads,
    activeThreadId,
    selectThread,
    createThread,
    error,
    runtimeErrorDetail,
    busy,
    route,
    pluginHostRoute,
    workspaceRoot,
    runtimeConnection,
    setRoute,
    openCode,
    openWrite,
    openSettings,
    openPlugins,
    openClaw,
    openSchedule,
    chooseWorkspace,
    clawChannels,
    activeClawChannelId,
    selectClawChannel,
    resetClawChannelSession,
    setClawChannelModel,
    appendLocalClawTurn,
    setError,
    sendMessage,
    reviewActiveThread,
    queuedMessages,
    removeQueuedMessage,
    startThreadHandoff,
    hydrateThreadHandoffOperations,
    handleThreadHandoffEvent,
    interrupt,
    probeRuntime,
    composerModel,
    composerProviderId,
    composerPickList,
    composerModelGroups,
    disabledSkillIds,
    setComposerModel,
	    setThreadSearch,
	    setCaseProjectExpanded,
    renameThread,
    archiveThread,
    deleteThread,
    forkActiveThread,
    spawnSideConversation,
    selectSideConversation,
    closeSideConversation,
    setSidePanelOpen,
    sideConversations,
    sidePanel
  } = useChatStore(
    useShallow((s) => ({
		      threads: s.threads,
		      caseProjects: s.caseProjects,
		      caseProjectIndexStatus: s.caseProjectIndexStatus,
		      caseProjectThreadsById: s.caseProjectThreadsById,
	      caseProjectLoadingById: s.caseProjectLoadingById,
	      caseProjectErrorsById: s.caseProjectErrorsById,
	      caseProjectExpandedById: s.caseProjectExpandedById,
	      threadSearch: s.threadSearch,
      showArchivedThreads: s.showArchivedThreads,
      activeThreadId: s.activeThreadId,
      selectThread: s.selectThread,
      createThread: s.createThread,
      error: s.error,
      runtimeErrorDetail: s.runtimeErrorDetail,
      busy: s.busy,
      route: s.route,
      pluginHostRoute: s.pluginHostRoute,
      workspaceRoot: s.workspaceRoot,
      runtimeConnection: s.runtimeConnection,
      setRoute: s.setRoute,
      openCode: s.openCode,
      openWrite: s.openWrite,
      openSettings: s.openSettings,
      openPlugins: s.openPlugins,
      openClaw: s.openClaw,
      openSchedule: s.openSchedule,
      chooseWorkspace: s.chooseWorkspace,
      clawChannels: s.clawChannels,
      activeClawChannelId: s.activeClawChannelId,
      selectClawChannel: s.selectClawChannel,
      resetClawChannelSession: s.resetClawChannelSession,
      setClawChannelModel: s.setClawChannelModel,
      appendLocalClawTurn: s.appendLocalClawTurn,
      setError: s.setError,
      sendMessage: s.sendMessage,
      reviewActiveThread: s.reviewActiveThread,
      queuedMessages: s.queuedMessages,
      removeQueuedMessage: s.removeQueuedMessage,
      startThreadHandoff: s.startThreadHandoff,
      hydrateThreadHandoffOperations: s.hydrateThreadHandoffOperations,
      handleThreadHandoffEvent: s.handleThreadHandoffEvent,
      interrupt: s.interrupt,
      probeRuntime: s.probeRuntime,
      composerModel: s.composerModel,
      composerProviderId: s.composerProviderId,
      composerPickList: s.composerPickList,
      composerModelGroups: s.composerModelGroups,
      disabledSkillIds: s.disabledSkillIds,
      setComposerModel: s.setComposerModel,
	      setThreadSearch: s.setThreadSearch,
	      setCaseProjectExpanded: s.setCaseProjectExpanded,
      renameThread: s.renameThread,
      archiveThread: s.archiveThread,
      deleteThread: s.deleteThread,
      forkActiveThread: s.forkActiveThread,
      spawnSideConversation: s.spawnSideConversation,
      selectSideConversation: s.selectSideConversation,
      closeSideConversation: s.closeSideConversation,
      setSidePanelOpen: s.setSidePanelOpen,
      sideConversations: s.sideConversations,
      sidePanel: s.sidePanel
    }))
  )
  const {
    draft: composerDraft, setInput, setAttachments: setComposerAttachments,
    setFileReferences: setComposerFileReferences, clearSubmitted: clearSubmittedComposer
  } = useThreadComposerDraft(
    threads.find((thread) => thread.id === activeThreadId)?.workspace || workspaceRoot || '',
    activeThreadId
  )
  const { input, attachments: composerAttachments, fileReferences: composerFileReferences } = composerDraft
  const [mode, setMode] = useState<'plan' | 'agent'>('agent')
  const [useWorktreePool, setUseWorktreePool] = useState(false)
  const [composerReasoningEffort, setComposerReasoningEffort] =
    useState<ComposerReasoningEffort>('max')
  const [runtimeInfo, setRuntimeInfo] = useState<CoreRuntimeInfoJson | null>(null)
  const [runtimeSkills, setRuntimeSkills] = useState<CoreRuntimeSkillJson[]>([])
  const [composerExecutionSettings, setComposerExecutionSettings] =
    useState<ComposerExecutionSettings | null>(null)
  const [composerExecutionApplying, setComposerExecutionApplying] = useState(false)
  const [attachmentUploadBusy, setAttachmentUploadBusy] = useState(false)
  const [attachmentUploadError, setAttachmentUploadError] = useState<string | null>(null)
  const [connectPhoneSidebarOpen, setConnectPhoneSidebarOpen] = useState(false)
  const [activeDataAnalysis, setActiveDataAnalysis] = useState<ActiveDataAnalysis | null>(null)
  const initUiPlugins = useUiPluginStore((s) => s.initUiPlugins)
  const uiModeCameosEnabled = useUiModeCameosEnabled()
  const [focusModeEnabled, setFocusModeEnabled] = useState(readFocusModePreference)
  const [runtimeLogPath, setRuntimeLogPath] = useState('')
  const [planPanelOverlayPreferred, setPlanPanelOverlayPreferred] = useState(false)
  const [timelineReturnToBottomState, setTimelineReturnToBottomState] =
    useState<TimelineReturnToBottomState | null>(null)
  const writeAssistantOpen = useWriteWorkspaceStore((s) => s.assistantOpen)
  const writeAssistantModel = useWriteWorkspaceStore((s) => s.assistantModel)
  const writeAssistantProviderId = useWriteWorkspaceStore((s) => s.assistantProviderId)
  const setWriteAssistantModel = useWriteWorkspaceStore((s) => s.setAssistantModel)
  const activeSddDraft = useSddDraftStore((s) => s.activeDraft)
  const sddDraftContent = useSddDraftStore((s) => s.content)
  const sddDraftOperationStatus = useSddDraftStore((s) => s.operationStatus)
  const writeAssistantPickList = useMemo(() => {
    const ordered = new Set<string>()
    for (const id of DEFAULT_COMPOSER_MODEL_IDS) {
      const normalized = id.trim()
      if (normalized && normalized.toLowerCase() !== 'auto') ordered.add(normalized)
    }
    for (const id of composerPickList) {
      const normalized = id.trim()
      if (normalized && normalized.toLowerCase() !== 'auto') ordered.add(normalized)
    }
    const current = writeAssistantModel.trim()
    if (current && current.toLowerCase() !== 'auto') ordered.add(current)
    return [...ordered]
  }, [composerPickList, writeAssistantModel])
  const resolvedWriteAssistantProviderId = useMemo(() => {
    const stored = writeAssistantProviderId.trim()
    if (stored) {
      const group = composerModelGroups.find((item) => item.providerId === stored)
      const modelKey = normalizeModelCapabilityKey(writeAssistantModel)
      const storedMatchesModel =
        !modelKey ||
        group?.modelIds.some((modelId) => normalizeModelCapabilityKey(modelId) === modelKey) === true
      if (group && storedMatchesModel) return stored
    }
    return providerIdForComposerModel(composerModelGroups, writeAssistantModel)
  }, [composerModelGroups, writeAssistantModel, writeAssistantProviderId])
  const stageInsetClass = 'ds-stage-inset'
  const [viewportWidth, setViewportWidth] = useState(() =>
    typeof window === 'undefined' ? 0 : Math.round(window.innerWidth)
  )
  const keyboardShortcuts = useKeyboardShortcutSettings()
  const keyboardShortcutBindings = useMemo(
    () => resolveKeyboardShortcutBindings(keyboardShortcuts),
    [keyboardShortcuts]
  )

  const prevThreadId = useRef<string | null>(null)
  // PM-skill framework selected via an assistant-panel button. The id is only
  // applied on send when its injected prompt text is still present in the
  // composer (see sendSddAssistantPrompt) — so editing the prompt away, clearing
  // the composer, or switching drafts all drop it without a stale-guidance leak.
  const pendingSddFrameworkRef = useRef<string | null>(null)
  const pendingSddFrameworkPromptRef = useRef<string | null>(null)
  const inputRef = useRef('')
  const sddUpgradeInFlightRef = useRef(false)
  const sddUpgradeTargetRef = useRef<PendingSddPlanTarget | null>(null)
  const sddTitleSyncTimerRef = useRef<number | null>(null)
  const lastSyncedSddTitleRef = useRef<Record<string, string>>({})
  const detectedDevPreviewUrls = useDevPreviewUrls(extractLatestTurnDevPreviewUrls)
  const autoOpenDevPreviewUrls = useDevPreviewUrls(extractLatestTurnAutoOpenDevPreviewUrls)
  const activeClawChannel = useMemo(
    () => clawChannels.find((channel) => channel.id === activeClawChannelId) ?? null,
    [activeClawChannelId, clawChannels]
  )
  const activeSkillWorkspace = useMemo(
    () => threads.find((thread) => thread.id === activeThreadId)?.workspace || workspaceRoot || '',
    [activeThreadId, threads, workspaceRoot]
  )
  const activeThread = useMemo(
    () => threads.find((thread) => thread.id === activeThreadId) ?? null,
    [activeThreadId, threads]
  )
  const currentSideConversations = useMemo(
    () =>
      Object.values(sideConversations)
        .filter((side) => side.parentThreadId === activeThreadId)
        .sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt)),
    [activeThreadId, sideConversations]
  )
  const currentSideConversationSubagents = useMemo(
    () => currentSideConversations.map(sideConversationInspectorItem),
    [currentSideConversations]
  )
  const [subagentInspector, setSubagentInspector] = useState<SubagentInspectorState | null>(null)
  const [hiddenSubagentInspectorKeysByParent, setHiddenSubagentInspectorKeysByParent] =
    useState<Record<string, string[]>>({})
  const activeSubagentInspector = useMemo<SubagentInspectorState | null>(() => {
    if (subagentInspector?.parentThreadId !== activeThreadId) return null
    const hiddenKeys = new Set(activeThreadId ? hiddenSubagentInspectorKeysByParent[activeThreadId] ?? [] : [])
    const visibleSubagents = subagentInspector.subagents.filter((agent) => !hiddenKeys.has(agent.key))
    const merged = mergeSubagentInspectorItems(visibleSubagents, currentSideConversationSubagents)
    const selectedKey = subagentInspector.selectedKey && merged.some((agent) => agent.key === subagentInspector.selectedKey)
      ? subagentInspector.selectedKey
      : currentSideConversationSubagents.at(-1)?.key ?? merged[0]?.key ?? null
    return {
      ...subagentInspector,
      selectedKey,
      subagents: merged
    }
  }, [activeThreadId, currentSideConversationSubagents, hiddenSubagentInspectorKeysByParent, subagentInspector])
  const fileTreeWorkspaceRoot = useMemo(
    () => normalizeWorkspaceRoot(threads.find((thread) => thread.id === activeThreadId)?.workspace || workspaceRoot),
    [activeThreadId, threads, workspaceRoot]
  )
  const activeCaseProjectWorkspaceRoot = useMemo(() => {
    const targetWorkspaceRoot = normalizeWorkspaceRoot(activeDataAnalysis?.workspaceRoot || fileTreeWorkspaceRoot || workspaceRoot)
    const targetKey = workspaceRootIdentityKey(targetWorkspaceRoot)
    if (!targetKey) return ''
    const matchedProject = caseProjects.find((project) => workspaceRootIdentityKey(project.rootPath) === targetKey)
    return matchedProject ? normalizeWorkspaceRoot(matchedProject.rootPath) : ''
  }, [activeDataAnalysis?.workspaceRoot, caseProjects, fileTreeWorkspaceRoot, workspaceRoot])
  const activeThreadWorktreeRecord = activeThreadId
    ? readThreadWorktreeRegistry().worktrees[activeThreadId] ?? null
    : null
  const latestDevPreviewUrl = detectedDevPreviewUrls[0] ?? null
  const latestAutoOpenDevPreviewUrl = autoOpenDevPreviewUrls[0] ?? null
  const handleToggleWorktreeMode = useCallback(() => {
    if (!activeThreadId || !activeThread) {
      setUseWorktreePool((enabled) => !enabled)
      return
    }
    if (attachmentUploadBusy) {
      setError(t('threadHandoffBlockedAttachmentUpload'))
      return
    }
    if (activeThreadWorktreeRecord) {
      void startThreadHandoff({
        direction: 'to-local',
        sourceThreadId: activeThreadId,
        sourceWorkspace: activeThreadWorktreeRecord.worktreePath,
        localWorkspace: activeThreadWorktreeRecord.projectPath,
        projectPath: activeThreadWorktreeRecord.projectPath,
        poolIndex: activeThreadWorktreeRecord.poolIndex,
        sourceBranch: activeThreadWorktreeRecord.branch
      })
      return
    }
    const sourceWorkspace = normalizeWorkspaceRoot(activeThread.workspace || workspaceRoot)
    if (!sourceWorkspace) {
      setError(t('workspaceRequiredToCreateThread'))
      return
    }
    void startThreadHandoff({
      direction: 'to-worktree',
      sourceThreadId: activeThreadId,
      sourceWorkspace
    })
  }, [
    activeThread,
    activeThreadId,
    activeThreadWorktreeRecord,
    attachmentUploadBusy,
    setError,
    startThreadHandoff,
    t,
    workspaceRoot
  ])
  const {
    beginLeftResize,
    beginRightResize,
    beginTerminalResize,
    filePreviewTarget,
    leftPaneContentRef,
    leftPaneRef,
    leftSidebarCollapsed,
    leftResizing,
    leftSidebarWidth,
    openDevPreview,
    rightPanelMode,
    rightPanelVisible,
    rightPaneContentRef,
    rightPaneRef,
    rightResizing,
    rightSidebarWidth,
    resetLeftSidebarWidth,
    resetRightSidebarWidth,
    resetTerminalHeight,
    setFilePreviewTarget,
    setRightPanelMode,
    setRightSidebarWidth,
    shellRef,
    terminalHeight,
    terminalOpen,
    terminalResizing,
    toggleLeftSidebar,
    toggleTerminal,
  } = useWorkbenchLayout({
    activeThreadId,
    latestAutoOpenDevPreviewUrl,
    latestDevPreviewUrl,
    route,
    workspaceRoot,
    writeAssistantOpen
  })
  const [documentsMounted, setDocumentsMounted] = useState(false)
  const workspaceTabs = useWorkspaceTabsStore((state) => state.tabs)
  const workspaceActiveTabId = useWorkspaceTabsStore((state) => state.activeTabId)
  const workspaceSelectorOpen = useWorkspaceTabsStore((state) => state.selectorOpen)
  const documentFocused = useWorkspaceTabsStore((state) => state.focused)
  const setDocumentFocused = useCallback((value: boolean | ((previous: boolean) => boolean)): void => {
    const state = useWorkspaceTabsStore.getState()
    state.setFocused(typeof value === 'function' ? value(state.focused) : value)
  }, [])
  const workspaceActiveTab = workspaceTabs.find((tab) => tab.id === workspaceActiveTabId) ?? null
  const nativeDocumentTarget = useNativeOfficeStore((state) => state.target)
  const nativeDocumentViews = useNativeOfficeStore((state) => state.views)
  const textDocumentPath = useWriteWorkspaceStore((state) => state.activeFilePath)
  const textDocumentRoot = useWriteWorkspaceStore((state) => state.workspaceRoot)
  const textDocumentSaveStatus = useWriteWorkspaceStore((state) => state.saveStatus)
  const textDocumentLoading = useWriteWorkspaceStore((state) => state.fileLoading)
  const textDocumentError = useWriteWorkspaceStore((state) => state.fileError)
  const registerDocumentTab = useCallback((root: string, path: string): void => {
    if (!root || !path) return
    const tab: WorkspaceTab = { id: workspaceObjectTabId(root, path), kind: 'document', mode: 'documents',
      title: path.replaceAll('\\', '/').split('/').at(-1) || path, workspaceRoot: root, path }
    const state = useWorkspaceTabsStore.getState()
    state.closeTab('tool:documents')
    state.openTab(tab)
    setDocumentsMounted(true)
  }, [])
  useEffect(() => {
    if (nativeDocumentTarget) registerDocumentTab(nativeDocumentTarget.workspace, nativeDocumentTarget.path)
  }, [nativeDocumentTarget, registerDocumentTab])
  useEffect(() => {
    if (textDocumentPath && !useNativeOfficeStore.getState().target) registerDocumentTab(textDocumentRoot, textDocumentPath)
  }, [textDocumentPath, textDocumentRoot, registerDocumentTab])
  useEffect(() => {
    if (!textDocumentPath || !textDocumentRoot) return
    useWorkspaceTabsStore.getState().patchTab(workspaceObjectTabId(textDocumentRoot, textDocumentPath), {
      dirty: textDocumentSaveStatus !== 'saved', loading: textDocumentLoading, error: Boolean(textDocumentError)
    })
  }, [textDocumentPath, textDocumentRoot, textDocumentSaveStatus, textDocumentLoading, textDocumentError])
  useEffect(() => {
    const state = useWorkspaceTabsStore.getState()
    for (const tab of state.tabs) {
      if (tab.kind !== 'document' || !tab.workspaceRoot || !tab.path) continue
      const view = nativeDocumentViews[JSON.stringify([tab.workspaceRoot, tab.path])]
      if (view) state.patchTab(tab.id, { dirty: view.dirty, loading: view.status === 'loading', error: view.status === 'error' })
    }
  }, [nativeDocumentViews])
  const nativeReferences = useNativeReferenceStore(state => state.references)
  const documentQuotes = useWriteWorkspaceStore((state) => state.quotedSelections)
  useEffect(() => {
    if (route !== 'write' && rightPanelMode !== 'documents') return
    let cancelled = false
    const root = useNativeOfficeStore.getState().target?.workspace || useWriteWorkspaceStore.getState().workspaceRoot || activeThread?.workspace || workspaceRoot
    void (async () => {
      if (root) await useWriteWorkspaceStore.getState().initializeWorkspace(root)
      if (cancelled) return
      setDocumentsMounted(true)
      setRightPanelMode('documents')
      setRightSidebarWidth((width) => Math.max(width, 640))
      if (route === 'write') useChatStore.getState().setRoute('chat')
    })()
    return () => { cancelled = true }
  }, [activeThread?.workspace, route, rightPanelMode, workspaceRoot, setRightPanelMode, setRightSidebarWidth])

  useEffect(() => {
    if (rightPanelMode !== 'file' || !filePreviewTarget || !isWriteWorkspaceFilePath(filePreviewTarget.path)) return
    let cancelled = false
    const target = filePreviewTarget
    const root = target.workspaceRoot || workspaceRoot
    void (async () => {
      await useWriteWorkspaceStore.getState().initializeWorkspace(root)
      if (cancelled || useWriteWorkspaceStore.getState().workspaceRoot !== root) return
      await useWriteWorkspaceStore.getState().openFile(root, target.path)
      if (cancelled) return
      const nativeTarget = useNativeOfficeStore.getState().target
      const opened = isNativeOfficeFilePath(target.path)
        ? nativeTarget?.workspace === root && nativeTarget.path === target.path
        : useWriteWorkspaceStore.getState().activeFilePath === target.path
      if (!opened) return
      registerDocumentTab(root, target.path)
      setRightPanelMode('documents')
    })()
    return () => { cancelled = true }
  }, [filePreviewTarget, rightPanelMode, workspaceRoot, setRightPanelMode, registerDocumentTab])

  useEffect(() => {
    useWriteWorkspaceStore.getState().clearQuotedSelections()
    setDocumentFocused(false)
  }, [activeThreadId, workspaceRoot, setDocumentFocused])

  const workbenchShellStyle = useMemo(
    () => ({ '--ds-shell-navigation-sidebar-width': `${leftSidebarWidth}px` }) as CSSProperties,
    [leftSidebarWidth]
  )
  const [leftSidebarMounted, setLeftSidebarMounted] = useState(!leftSidebarCollapsed)
  const [runtimeDiagnosticsFocus, setRuntimeDiagnosticsFocus] = useState<RuntimeDiagnosticsFocus | null>(null)

  useEffect(() => {
    if (!leftSidebarCollapsed) {
      setLeftSidebarMounted(true)
      return undefined
    }

    const timeoutId = window.setTimeout(() => setLeftSidebarMounted(false), 520)
    return () => window.clearTimeout(timeoutId)
  }, [leftSidebarCollapsed])

  useEffect(() => {
    void hydrateThreadHandoffOperations()
    return window.analytix.workspace.onThreadHandoffEvent((event) => {
      handleThreadHandoffEvent(event.operation)
    })
  }, [handleThreadHandoffEvent, hydrateThreadHandoffOperations])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadPluginMarketplaceView()
      void loadScheduleTasksView()
      preloadRightPanelIsland('todo')
      preloadRightPanelIsland('changes')
      preloadRightPanelIsland('browser')
      preloadRightPanelIsland('summary')
      preloadAssistantMarkdownRenderer()
    }, 450)
    const heavyTimer = window.setTimeout(() => {
      void loadDataAnalysisSurface()
      void loadSddDraftEditorView()
      void loadTerminalPanel()
      preloadRightPanelIsland('file')
      preloadRightPanelIsland('plan')
      preloadRightPanelIsland('sdd-ai')
      preloadRightPanelIsland('child-agent')
      preloadRightPanelIsland('write-assistant')
    }, 1200)
    return () => {
      window.clearTimeout(timer)
      window.clearTimeout(heavyTimer)
    }
  }, [])

  useEffect(() => listenRuntimeDiagnosticsFocus((focus) => {
    setRuntimeDiagnosticsFocus(focus)
    preloadRightPanelIsland('summary')
    setRightPanelMode('summary')
  }), [setRightPanelMode])

  const inspectChildAgent = useCallback((subagents: CoreThreadSummarySubagentJson[], selectedKey: string): void => {
    if (!activeThreadId) return
    setHiddenSubagentInspectorKeysByParent((current) => {
      const hidden = current[activeThreadId]
      if (!hidden?.includes(selectedKey)) return current
      const nextHidden = hidden.filter((key) => key !== selectedKey)
      if (nextHidden.length === 0) {
        const next = { ...current }
        delete next[activeThreadId]
        return next
      }
      return { ...current, [activeThreadId]: nextHidden }
    })
    setSubagentInspector({
      parentThreadId: activeThreadId,
      selectedKey,
      subagents
    })
    setRightSidebarWidth((width) => Math.max(width, SUBAGENT_PANEL_PREFERRED_WIDTH))
    useWorkspaceTabsStore.getState().openTab({ id: `agent:${activeThreadId}:${selectedKey}`, kind: 'tool', mode: 'child-agent', instanceId: selectedKey, title: t('workspaceSubagent', { defaultValue: '子代理' }) })
  }, [activeThreadId, setRightSidebarWidth, t])

  const selectSubagentInspector = useCallback((selectedKey: string): void => {
    const sideThreadId = sideConversationThreadIdFromInspectorKey(selectedKey)
    if (sideThreadId) {
      selectSideConversation(sideThreadId)
      setSidePanelOpen(false)
    }
    setSubagentInspector((current) => current ? { ...current, selectedKey } : current)
  }, [selectSideConversation, setSidePanelOpen])

  const closeSubagentInspectorTab = useCallback((key: string): void => {
    const sideThreadId = sideConversationThreadIdFromInspectorKey(key)
    if (sideThreadId) {
      void closeSideConversation(sideThreadId).catch((error) => {
        setError(error instanceof Error ? error.message : String(error))
      })
    } else if (activeThreadId) {
      setHiddenSubagentInspectorKeysByParent((current) => {
        const hidden = current[activeThreadId] ?? []
        if (hidden.includes(key)) return current
        return { ...current, [activeThreadId]: [...hidden, key] }
      })
    }
    setSubagentInspector((current) => {
      if (!current || current.selectedKey !== key) return current
      return { ...current, selectedKey: null }
    })
  }, [activeThreadId, closeSideConversation, setError])

  const syncSubagentInspector = useCallback((
    parentThreadId: string,
    subagents: CoreThreadSummarySubagentJson[]
  ): void => {
    setSubagentInspector((current) => {
      if (!current || current.parentThreadId !== parentThreadId) return current
      if (sameSubagentInspectorItems(current.subagents, subagents)) return current
      const selectedKey = current.selectedKey && (
        subagents.some((agent) => agent.key === current.selectedKey) ||
        Boolean(sideConversationThreadIdFromInspectorKey(current.selectedKey))
      )
        ? current.selectedKey
        : subagents[0]?.key ?? current.selectedKey
      return { ...current, selectedKey, subagents }
    })
  }, [])

  useEffect(() => {
    if (!subagentInspector) return
    if (subagentInspector.parentThreadId === activeThreadId) return
    setSubagentInspector(null)
  }, [activeThreadId, subagentInspector])

  useEffect(() => {
    if (rightPanelMode !== 'child-agent') return
    if (activeSubagentInspector) return
    useWorkspaceTabsStore.getState().showSelector()
  }, [activeSubagentInspector, rightPanelMode])

  useEffect(() => {
    if (route === 'write') {
      preloadRightPanelIsland('documents')
      return
    }
    if (route === 'plugins') {
      void loadPluginMarketplaceView()
      return
    }
    if (route === 'schedule') {
      void loadScheduleTasksView()
      return
    }
    if (activeSddDraft) {
      void loadSddDraftEditorView()
    }
  }, [activeSddDraft, route])

  useEffect(() => {
    if (!terminalOpen) return
    void loadTerminalPanel()
  }, [terminalOpen])

  const terminalPanelMotion = useShellPanelMotion({
    isVisible: terminalOpen,
    size: terminalHeight
  })
  const workbenchScrollReserve = useMemo(
    () =>
      createWorkbenchScrollReserve({
        bottomPanelOpen: terminalPanelMotion.isMounted,
        bottomPanelHeight: terminalPanelMotion.animatedSize
      }),
    [terminalPanelMotion.animatedSize, terminalPanelMotion.isMounted]
  )
  const chatStageStyle = workbenchScrollReserve.timelineReserveStyle
  const titleForSddDraft = useCallback((draft: SddDraft): string => {
    const snapshot = useSddDraftStore.getState()
    const markdown = snapshot.activeDraft?.id === draft.id ? snapshot.content : ''
    return sddAssistantThreadTitle(markdown, t('sddUntitledRequirement'))
  }, [t])
  const renameSddAssistantThreadToDraft = useCallback(async (
    threadId: string,
    draft: SddDraft
  ): Promise<void> => {
    const targetId = threadId.trim()
    const nextTitle = titleForSddDraft(draft)
    if (!targetId || !nextTitle || runtimeConnection !== 'ready') return
    const currentTitle = useChatStore.getState().threads.find((thread) => thread.id === targetId)?.title.trim()
    if (currentTitle === nextTitle || lastSyncedSddTitleRef.current[targetId] === nextTitle) return
    try {
      await getProvider().renameThread(targetId, nextTitle)
      lastSyncedSddTitleRef.current[targetId] = nextTitle
      useChatStore.setState((state) => ({
        threads: state.threads.map((thread) =>
          thread.id === targetId ? { ...thread, title: nextTitle } : thread
        )
      }))
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error))
    }
  }, [runtimeConnection, setError, titleForSddDraft])
  const {
    activeGuiPlan,
    buildGuiPlan,
    handleGuiPlanCommand,
    openGuiPlanPanel,
    replanChangedRequirements,
    sendPlanTurn,
    verifyGuiPlan
  } = useWorkbenchPlanController({
    busy,
    mode,
    route,
    sendMessage,
    setError,
    setMode,
    setRightPanelMode,
    setRightSidebarWidth,
    t,
    workspaceRoot,
    onPlanBuildStarted: async (plan) => {
      const threadId = plan.threadId?.trim() || useChatStore.getState().activeThreadId
      const draft = useSddDraftStore.getState().activeDraft
      if (!threadId) return
      if (draft) await renameSddAssistantThreadToDraft(threadId, draft)
      if (!releaseSddAssistantThread(threadId)) return
      await useChatStore.getState().refreshThreads()
    }
  })
  const planPanelInOverlay =
    route === 'chat' &&
    !activeSddDraft &&
    rightPanelMode === 'plan' &&
    planPanelOverlayPreferred

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    const media = window.matchMedia('(max-width: 900px), (orientation: portrait)')
    const sync = (): void => setPlanPanelOverlayPreferred(media.matches)
    sync()
    if (typeof media.addEventListener === 'function') {
      media.addEventListener('change', sync)
      return () => media.removeEventListener('change', sync)
    }
    media.addListener(sync)
    return () => media.removeListener(sync)
  }, [])

  useEffect(() => {
    if (!planPanelInOverlay) return
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') setRightPanelMode(null)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [planPanelInOverlay, setRightPanelMode])

  useEffect(() => {
    const runDesktopShortcut = (command: DesktopCommand): void => {
      if (typeof window.analytix?.app?.runDesktopCommand !== 'function') return
      void window.analytix.app.runDesktopCommand(command)
    }

    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.defaultPrevented || event.repeat || event.isComposing) return
      const workspaceCommand = nativeWorkspaceCommandFromInput({key:event.key,control:event.ctrlKey,meta:event.metaKey,shift:event.shiftKey,alt:event.altKey,isComposing:event.isComposing})
      if (workspaceCommand && (workspaceCommand !== 'close-tab' || (event.target instanceof Element && event.target.closest('#workbench-right-workspace')))) {
        event.preventDefault()
        window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', {detail:workspaceCommand}))
        return
      }
      const commandId = findKeyboardShortcutCommand(
        keyboardShortcutBindings,
        keyboardEventToShortcut(event)
      )
      if (!commandId) return
      event.preventDefault()

      if (commandId === 'toggle-plan-mode') {
        if (mode === 'plan') {
          setMode('agent')
        } else {
          setMode('plan')
          void handleGuiPlanCommand()
        }
        return
      }
      if (commandId === 'new-chat') {
        void createThread()
        return
      }
      if (commandId === 'choose-workspace') {
        void chooseWorkspace()
        return
      }
      if (commandId === 'settings') {
        openSettings()
        return
      }

      const desktopCommand = DESKTOP_SHORTCUT_COMMANDS[commandId]
      if (desktopCommand) runDesktopShortcut(desktopCommand)
    }

    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [
    chooseWorkspace,
    createThread,
    handleGuiPlanCommand,
    keyboardShortcutBindings,
    mode,
    openSettings,
    setMode
  ])
  const showDevPreviewCard =
    route === 'chat' &&
    latestDevPreviewUrl !== null

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.analytix?.logs?.getPath !== 'function') return
    let cancelled = false
    void window.analytix
      .logs.getPath()
      .then((path) => {
        if (!cancelled) setRuntimeLogPath(path)
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    // 形象工坊:读取偏好、应用 DOM 属性/token,并在插件模式下加载图集
    void initUiPlugins()
  }, [initUiPlugins])

  useEffect(() => {
    if (typeof document === 'undefined') return
    document.documentElement.setAttribute('data-focus-mode', focusModeEnabled ? 'on' : 'off')
  }, [focusModeEnabled])

  const updateFocusMode = (enabled: boolean): void => {
    writeFocusModePreference(enabled)
    setFocusModeEnabled(enabled)
  }

  useEffect(() => {
    const previousThreadId = prevThreadId.current
    prevThreadId.current = activeThreadId
    if (previousThreadId !== null && previousThreadId !== activeThreadId && sidePanel.open) {
      setSidePanelOpen(false)
    }
  }, [activeThreadId, setSidePanelOpen, sidePanel.open])

  const openSideChatInspector = useCallback((selectedSideThreadId?: string | null): void => {
    if (!activeThreadId) {
      setError(t('sideConversationNeedsActiveThread'))
      return
    }
    const sideThreadId = selectedSideThreadId?.trim() || currentSideConversations.at(-1)?.threadId || null
    if (sideThreadId) selectSideConversation(sideThreadId)
    setSidePanelOpen(false)
    setSubagentInspector((current) => ({
      parentThreadId: activeThreadId,
      selectedKey: sideThreadId ? sideConversationInspectorKey(sideThreadId) : current?.selectedKey ?? null,
      subagents: current?.parentThreadId === activeThreadId ? current.subagents : []
    }))
    setRightSidebarWidth((width) => Math.max(width, SUBAGENT_PANEL_PREFERRED_WIDTH))
    useWorkspaceTabsStore.getState().openTab({ id: `sidechat:${activeThreadId}:${sideThreadId ?? 'latest'}`, kind: 'tool', mode: 'child-agent', instanceId: sideThreadId ? sideConversationInspectorKey(sideThreadId) : undefined, title: t('sidePanelTabLabel') })
  }, [
    activeThreadId,
    currentSideConversations,
    selectSideConversation,
    setError,
    setRightSidebarWidth,
    setSidePanelOpen,
    t
  ])

  const createSideChatInInspector = useCallback(async (seedText?: string): Promise<void> => {
    if (!activeThreadId) {
      setError(t('sideConversationNeedsActiveThread'))
      return
    }
    const sideThreadId = await spawnSideConversation(seedText)
    if (!sideThreadId) return
    openSideChatInspector(sideThreadId)
  }, [activeThreadId, openSideChatInspector, setError, spawnSideConversation, t])

  const openSideChat = useCallback((): void => {
    const latestSide = currentSideConversations.at(-1)
    if (latestSide) {
      openSideChatInspector(latestSide.threadId)
      return
    }
    void createSideChatInInspector()
  }, [createSideChatInInspector, currentSideConversations, openSideChatInspector])

  useEffect(() => {
    let cancelled = false
    void rendererRuntimeClient.getSettings()
      .then((settings) => {
        if (cancelled) return
        setComposerExecutionSettings({
          approvalPolicy: settings.runtime.approvalPolicy,
          sandboxMode: settings.runtime.sandboxMode
        })
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [])

  const updateComposerExecutionSettings = (patch: Partial<ComposerExecutionSettings>): void => {
    if (!composerExecutionSettings || composerExecutionApplying) return
    const previous = composerExecutionSettings
    const next = { ...previous, ...patch }
    setComposerExecutionSettings(next)
    setComposerExecutionApplying(true)
    void rendererRuntimeClient.setSettings({
      runtime: {
        ...(patch.approvalPolicy ? { approvalPolicy: patch.approvalPolicy as ApprovalPolicy } : {}),
        ...(patch.sandboxMode ? { sandboxMode: patch.sandboxMode as SandboxMode } : {})
      }
    }).then((settings) => {
      setComposerExecutionSettings({
        approvalPolicy: settings.runtime.approvalPolicy,
        sandboxMode: settings.runtime.sandboxMode
      })
      void probeRuntime('background')
    }).catch((error: unknown) => {
      setComposerExecutionSettings(previous)
      setError(error instanceof Error ? error.message : String(error))
    }).finally(() => setComposerExecutionApplying(false))
  }

  const codeThreads = useMemo(
    () => threads.filter((thread) =>
      !isClawThread(thread, clawChannels) &&
      !isSddAssistantThread(thread)
    ),
    [clawChannels, threads]
  )

  const mirrorClawCommand = async (userText: string, replyText: string): Promise<void> => {
    if (!activeThreadId || typeof window.analytix?.connectPhone?.mirrorChannelMessage !== 'function') return
    const userResult = await window.analytix.connectPhone.mirrorChannelMessage(
      activeThreadId,
      userText,
      'user'
    )
    if (!userResult.ok) return
    await window.analytix.connectPhone.mirrorChannelMessage(
      activeThreadId,
      replyText,
      'assistant'
    )
  }

  const clawHelpText = (): string =>
    [
      t('clawHelpTitle'),
      '',
      `- \`/help\`: ${t('clawHelpCommandHelp')}`,
      `- \`/new\`: ${t('clawHelpCommandNew')}`,
      `- \`/model auto\`: ${t('clawHelpCommandModelAuto')}`,
      `- \`/model pro\`: ${t('clawHelpCommandModelPro')}`,
      `- \`/model flash\`: ${t('clawHelpCommandModelFlash')}`,
      `- \`/model\`: ${t('clawHelpCommandModelShow')}`
    ].join('\n')

  useEffect(() => {
    inputRef.current = input
  }, [input])

  useEffect(() => {
    if (rightPanelMode === 'plan' && !activeGuiPlan) {
      setRightPanelMode(null)
    }
  }, [activeGuiPlan, rightPanelMode, setRightPanelMode])

  useEffect(() => {
    if (
      !activeGuiPlan ||
      !sddUpgradeInFlightRef.current ||
      !sddPlanMatchesPendingTarget(activeGuiPlan, sddUpgradeTargetRef.current)
    ) {
      return
    }
    sddUpgradeInFlightRef.current = false
    sddUpgradeTargetRef.current = null
    useSddDraftStore.getState().setOperationStatus('idle')
    const completedDraft = useSddDraftStore.getState().activeDraft
    if (completedDraft) forgetRememberedSddDraft(completedDraft)
    useSddDraftStore.getState().clearActiveDraft()
  }, [activeGuiPlan])

  useEffect(() => {
    if (
      busy ||
      !sddUpgradeInFlightRef.current ||
      sddDraftOperationStatus !== 'upgrading' ||
      sddPlanMatchesPendingTarget(activeGuiPlan, sddUpgradeTargetRef.current)
    ) {
      return
    }
    const timeout = window.setTimeout(() => {
      if (!sddUpgradeInFlightRef.current) return
      if (useSddDraftStore.getState().operationStatus !== 'upgrading') return
      sddUpgradeInFlightRef.current = false
      sddUpgradeTargetRef.current = null
      useSddDraftStore.getState().setOperationStatus('error', t('planToolResultMissing'))
    }, 800)
    return () => window.clearTimeout(timeout)
  }, [activeGuiPlan, busy, sddDraftOperationStatus, t])

  useEffect(() => {
    let cancelled = false
    const runtimeReady = runtimeConnection === 'ready'
    const provider = getProvider()
    if (!runtimeReady) {
      setRuntimeInfo(null)
      return () => {
        cancelled = true
      }
    }

    if (provider.getRuntimeInfo) {
      void provider.getRuntimeInfo()
        .then((info) => {
          if (!cancelled) setRuntimeInfo(info)
        })
        .catch(() => {
          if (!cancelled) setRuntimeInfo(null)
        })
    } else {
      setRuntimeInfo(null)
    }

    const runtimeSkillsTask = provider.listSkills
      ? provider.listSkills().catch(() => [])
      : Promise.resolve([])
    const localSkillsTask = typeof window !== 'undefined' && typeof window.analytix?.app?.listSkills === 'function'
      ? window.analytix.app.listSkills(activeSkillWorkspace || undefined).catch(() => ({
          ok: true as const,
          skills: [],
          validationErrors: []
        }))
      : Promise.resolve({ ok: true as const, skills: [], validationErrors: [] })

    void Promise.all([runtimeSkillsTask, localSkillsTask]).then(([runtimeSkillList, localSkillsResult]) => {
      if (cancelled) return
      const localSkillList = localSkillsResult.ok ? localSkillsResult.skills : []
      setRuntimeSkills(mergeSkillCommands(runtimeSkillList, localSkillList))
    })
    return () => {
      cancelled = true
    }
  }, [activeSkillWorkspace, runtimeConnection])

  const selectedComposerModel = route === 'claw'
    ? activeClawChannel?.model ?? 'auto'
    : rightPanelMode === 'sdd-ai'
      ? writeAssistantModel
    : composerModel
  const selectedComposerProviderId = rightPanelMode === 'sdd-ai'
    ? resolvedWriteAssistantProviderId
    : route === 'chat'
      ? composerProviderId
      : ''
  const selectedModelSupportsImageInput = useMemo(() => {
    const selected = selectedComposerModel.trim()
    const runtimeModel = runtimeInfo?.capabilities.model
    if (!selected || selected.toLowerCase() === 'auto') {
      return runtimeModel?.inputModalities.includes('image') === true
    }
    const profile = modelProfileForSelection(composerModelGroups, selected, selectedComposerProviderId)
    if (modelSupportsImageInput(profile)) return true
    if (composerModelIdLooksMultimodal(selected)) return true
    if (runtimeModel && normalizeModelCapabilityKey(runtimeModel.id) === normalizeModelCapabilityKey(selected)) {
      return runtimeModel.inputModalities.includes('image')
    }
    return false
  }, [composerModelGroups, runtimeInfo, selectedComposerModel, selectedComposerProviderId])
  const visionBridgeAvailable = runtimeInfo?.capabilities.visionBridge?.available === true

  useEffect(() => {
    setAttachmentUploadError(null)
  }, [route, selectedComposerModel, selectedComposerProviderId])

  const attachmentUploadEnabled = isChatAttachmentUploadEnabled({
    runtimeConnection,
    route,
    mode,
    attachmentStoreAvailable: runtimeInfo?.capabilities.attachments.available,
    modelSupportsImageInput: selectedModelSupportsImageInput
  })
  const imageAttachmentUploadEnabled = canUploadImageAttachment({
    attachmentUploadEnabled,
    modelSupportsImageInput: selectedModelSupportsImageInput,
    visionBridgeAvailable
  })
  const webAccessAvailable =
    runtimeInfo?.capabilities.web.fetch.available === true ||
    runtimeInfo?.capabilities.web.search.available === true

  const clearComposerAttachments = (): void => {
    setComposerAttachments([])
  }

  const activeComposerWorkspace = (): string | undefined => {
    const sddDraft = useSddDraftStore.getState().activeDraft
    if (rightPanelMode === 'sdd-ai' && sddDraft?.workspaceRoot) return sddDraft.workspaceRoot
    const writeWorkspace = useWriteWorkspaceStore.getState().workspaceRoot
    if (route === 'write' && writeWorkspace.trim()) return writeWorkspace
    return threads.find((thread) => thread.id === activeThreadId)?.workspace || workspaceRoot || undefined
  }

  const addComposerFileReference = (reference: ComposerFileReference): void => {
    setComposerFileReferences((current) => mergeComposerFileReferences(current, reference))
  }

  const pickComposerFileReferences = async (): Promise<void> => {
    const pickLocalFiles = window.analytix?.files?.pickLocalFiles
    if (typeof pickLocalFiles !== 'function') return
    const result = await pickLocalFiles(activeSkillWorkspace || undefined)
    if (result.canceled) return
    for (const path of result.paths) {
      addComposerFileReference(composerFileReferenceFromPath(path, activeSkillWorkspace))
    }
  }

  const removeComposerFileReference = (relativePath: string): void => {
    const key = relativePath.trim().replaceAll('\\', '/').replace(/\/+/g, '/').toLowerCase()
    setComposerFileReferences((current) =>
      current.filter((reference) =>
        reference.relativePath.trim().replaceAll('\\', '/').replace(/\/+/g, '/').toLowerCase() !== key
      )
    )
  }

  const openWorkspaceFilePreviewTarget = (target: WorkspaceFileTarget): void => {
    const workspace = target.workspaceRoot ?? fileTreeWorkspaceRoot
    if (!workspace) return
    const nextTarget = {
      ...target,
      workspaceRoot: workspace
    }
    setFilePreviewTarget(nextTarget)
    setRightSidebarWidth((width) => Math.max(width, CODE_PANEL_PREFERRED))
    setRightPanelMode('file')
  }

  const previewWorkspaceFileFromSidebar = (path: string, selectedWorkspaceRoot?: string): void => {
    const workspace = selectedWorkspaceRoot || fileTreeWorkspaceRoot
    if (!workspace) return
    openWorkspaceFilePreviewTarget({ path, workspaceRoot: workspace })
  }

  const closeWorkspaceFilePreviewTarget = (target: WorkspaceFileTarget): void => {
    void closeWorkspaceTab(workspaceObjectTabId(target.workspaceRoot ?? '', target.path))
  }

  const addWorkspaceReferenceFromSidebar = (reference: ChatFileTreeReference): void => {
    addComposerFileReference(reference)
  }

  const openFileTreeSidePanel = (): void => {
    setRightPanelMode('files')
  }

  const handlePickAttachments = async (
    files: File[],
    options: { localFilePaths?: string[] } = {}
  ): Promise<void> => {
    if (!files.length || !attachmentUploadEnabled) return
    const provider = getProvider()
    if (typeof provider.uploadAttachment !== 'function') {
      setAttachmentUploadError(t('composerAttachmentUnavailable'))
      return
    }
    setAttachmentUploadBusy(true)
    setAttachmentUploadError(null)
    try {
      const workspace = activeComposerWorkspace()
      if (!activeThreadId || !workspace) {
        throw new Error(t('composerAttachmentUnavailable'))
      }
      const attachmentCapabilities = runtimeInfo?.capabilities.attachments
      if (!attachmentCapabilities) {
        setAttachmentUploadError(t('composerAttachmentUnavailable'))
        return
      }
      const uploaded: AttachmentReference[] = []
      for (const [index, file] of files.entries()) {
        const localFilePath =
          options.localFilePaths?.[index] ||
          (typeof window.analytix?.files?.getPathForFile === 'function' ? window.analytix.files.getPathForFile(file) : '')
        if (isPdfAttachmentFile(file)) {
          if (!localFilePath || typeof window.analytix?.files?.readLocalPdfText !== 'function') {
            throw new Error(t('composerPdfAttachmentUnavailable'))
          }
          const result = await window.analytix.files.readLocalPdfText({ path: localFilePath })
          if (!result.ok) throw new Error(result.message)
          const documentText = result.text.trim()
          if (!documentText) throw new Error(t('composerPdfAttachmentNoText'))
          const attachment = await provider.uploadAttachment({
            name: file.name || fileNameFromPath(result.path),
            mimeType: 'application/pdf',
            dataBase64: arrayBufferToBase64(await file.arrayBuffer()),
            documentText,
            pageCount: result.pageCount,
            threadId: activeThreadId,
            workspace
          })
          uploaded.push({
            id: attachment.id,
            kind: 'document',
            name: attachment.name,
            mimeType: attachment.mimeType,
            byteSize: attachment.byteSize,
            pageCount: attachment.pageCount ?? result.pageCount,
            truncated: attachment.truncated ?? result.truncated,
            textPreview: documentText.slice(0, 240),
            documentText,
            localFilePath,
            FilePath: localFilePath
          })
          continue
        }
        if (!isImageAttachmentFile(file)) {
          throw new Error(t('composerAttachmentUnsupportedType'))
        }
        if (!imageAttachmentUploadEnabled) {
          throw new Error(t('composerAttachmentModelUnsupported'))
        }
        const prepared = await prepareImageAttachmentUpload(file, attachmentCapabilities)
        const attachment = await provider.uploadAttachment({
          name: file.name || 'image',
          mimeType: prepared.mimeType,
          dataBase64: prepared.dataBase64,
          textFallback: prepared.textFallback,
          threadId: activeThreadId,
          workspace
        })
        uploaded.push({
          id: attachment.id,
          kind: 'image',
          name: attachment.name,
          mimeType: attachment.mimeType,
          width: attachment.width,
          height: attachment.height,
          ...(localFilePath ? { localFilePath } : {}),
          ...(localFilePath ? { FilePath: localFilePath } : {}),
          previewUrl: `data:${prepared.mimeType};base64,${prepared.dataBase64}`
        })
      }
      if (uploaded.length > 0) {
        setComposerAttachments((current) => {
          const byId = new Map(current.map((attachment) => [attachment.id, attachment]))
          for (const attachment of uploaded) {
            byId.set(attachment.id, attachment)
          }
          return [...byId.values()]
        })
      }
    } catch (error) {
      setAttachmentUploadError(error instanceof Error ? error.message : String(error))
    } finally {
      setAttachmentUploadBusy(false)
    }
  }

  const removeComposerAttachment = (id: string): void => {
    setComposerAttachments((current) => current.filter((attachment) => attachment.id !== id))
  }

  const handlePasteClipboardImage = async (options: { silentNoImage?: boolean } = {}): Promise<void> => {
    if (!attachmentUploadEnabled) return
    if (typeof window.analytix?.files?.readClipboardImage !== 'function') {
      setAttachmentUploadError(t('composerAttachmentUnavailable'))
      return
    }
    const image = await window.analytix.files.readClipboardImage()
    if (!image.ok) {
      if (options.silentNoImage) return
      setAttachmentUploadError(image.message)
      return
    }
    await handlePickAttachments([clipboardImageToFile(image)], { localFilePaths: [image.localFilePath] })
  }

  const sendWritePrompt = (value: string, references?: NativeReference[]): void => {
    void handleSendAsync(value, references)
  }

  const createSddAssistantThreadForDraft = async (draft: SddDraft): Promise<string | null> => {
    const normalizedWorkspace = normalizeWorkspaceRoot(draft.workspaceRoot)
    if (!normalizedWorkspace) {
      setError(t('workspaceRequiredToCreateThread'))
      return null
    }
    if (runtimeConnection !== 'ready') {
      setError(t('runtimeActionNeedsConnection'))
      return null
    }
    try {
      const provider = getProvider()
      const model = writeAssistantModel.trim()
      const providerId = resolvedWriteAssistantProviderId.trim()
      const thread = await provider.createThread({
        workspace: normalizedWorkspace,
        title: titleForSddDraft(draft),
        mode: 'agent',
        ...(model ? { model } : {}),
        ...(providerId ? { providerId } : {})
      })
      const normalizedThread = {
        ...thread,
        workspace: normalizeWorkspaceRoot(thread.workspace) || normalizedWorkspace
      }
      markSddAssistantThread(draft, normalizedThread.id)
      // Record the thread association inside the requirement unit right away.
      void writeSddChatTranscriptForThread({
        workspaceRoot: draft.workspaceRoot,
        draftRelativePath: draft.relativePath,
        threadId: normalizedThread.id,
        blocks: []
      })
      useChatStore.setState((state) => ({
        activeThreadId: normalizedThread.id,
        threads: state.threads.some((item) => item.id === normalizedThread.id)
          ? state.threads
          : [normalizedThread, ...state.threads]
      }))
      setRoute('chat')
      await selectThread(normalizedThread.id)
      void useChatStore.getState().refreshThreads()
      return normalizedThread.id
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error))
      return null
    }
  }

  const ensureSddAssistantThreadForDraft = async (draft: SddDraft): Promise<string | null> => {
    const registeredThreadId = sddAssistantThreadIdForDraft(draft)
    if (registeredThreadId) {
      setRoute('chat')
      if (useChatStore.getState().activeThreadId !== registeredThreadId) {
        await selectThread(registeredThreadId)
      }
      if (useChatStore.getState().activeThreadId === registeredThreadId) {
        void renameSddAssistantThreadToDraft(registeredThreadId, draft)
        return registeredThreadId
      }
    }
    return createSddAssistantThreadForDraft(draft)
  }

  useEffect(() => {
    const draft = activeSddDraft
    if (!draft || runtimeConnection !== 'ready') return
    const threadId = sddAssistantThreadIdForDraft(draft)
    if (!threadId) return
    const nextTitle = sddAssistantThreadTitle(sddDraftContent, t('sddUntitledRequirement'))
    if (!nextTitle || lastSyncedSddTitleRef.current[threadId] === nextTitle) return
    if (sddTitleSyncTimerRef.current) {
      window.clearTimeout(sddTitleSyncTimerRef.current)
    }
    sddTitleSyncTimerRef.current = window.setTimeout(() => {
      sddTitleSyncTimerRef.current = null
      const latestDraft = useSddDraftStore.getState().activeDraft
      if (!latestDraft || latestDraft.id !== draft.id) return
      const latestThreadId = sddAssistantThreadIdForDraft(latestDraft)
      if (latestThreadId !== threadId) return
      void renameSddAssistantThreadToDraft(threadId, latestDraft)
    }, SDD_ASSISTANT_TITLE_SYNC_DELAY_MS)
    return () => {
      if (sddTitleSyncTimerRef.current) {
        window.clearTimeout(sddTitleSyncTimerRef.current)
        sddTitleSyncTimerRef.current = null
      }
    }
  }, [activeSddDraft, renameSddAssistantThreadToDraft, runtimeConnection, sddDraftContent, t])

  const openSddRequirementDraft = async (
    draft: SddDraft,
    content: string,
    options: {
      lastSavedContent?: string
      saveStatus?: SddDraftSaveStatus
      openAssistant?: boolean
    } = {}
  ): Promise<boolean> => {
    setActiveDataAnalysis(null)
    useSddDraftStore.getState().setActiveDraft(draft, content, {
      lastSavedContent: options.lastSavedContent,
      saveStatus: options.saveStatus
    })
    // Self-heal the unit's conversation record (covers turns that completed
    // while the draft was closed or in another thread).
    void refreshSddChatTranscriptFromProvider(draft)
    setInput('')
    setMode('agent')
    setRoute('chat')
    if (options.openAssistant ?? runtimeConnection === 'ready') {
      setRightSidebarWidth((width) => Math.max(width, 420))
      const sddThreadId = await ensureSddAssistantThreadForDraft(draft)
      if (sddThreadId) {
        setRightPanelMode('sdd-ai')
      } else {
        setRightPanelMode(null)
      }
    } else {
      setRightPanelMode(null)
    }
    return true
  }

  const dismissActiveSddDraft = useCallback((options: { closeAssistant?: boolean } = {}): void => {
    const draft = useSddDraftStore.getState().activeDraft
    if (draft) {
      void saveActiveSddDraftToDisk()
      useSddDraftStore.getState().clearActiveDraft()
    }
    if (options.closeAssistant && rightPanelMode === 'sdd-ai') setRightPanelMode(null)
  }, [rightPanelMode, setRightPanelMode])

  const prepareThreadContextAction = useCallback(async (threadId: string): Promise<boolean> => {
    const targetThreadId = threadId.trim()
    if (!targetThreadId || runtimeConnection !== 'ready') return false
    setConnectPhoneSidebarOpen(false)
    if (useSddDraftStore.getState().activeDraft) dismissActiveSddDraft({ closeAssistant: true })
    setRoute('chat')
    if (useChatStore.getState().activeThreadId !== targetThreadId) {
      await selectThread(targetThreadId)
    }
    return true
  }, [dismissActiveSddDraft, runtimeConnection, selectThread, setRoute])

  const openSideChatForThread = useCallback(async (threadId: string): Promise<void> => {
    if (!(await prepareThreadContextAction(threadId))) return
    const state = useChatStore.getState()
    const targetThreadId = threadId.trim()
    const latestSide = Object.values(state.sideConversations)
      .filter((side) => side.parentThreadId === targetThreadId)
      .sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt))
      .at(-1)
    if (latestSide) {
      state.selectSideConversation(latestSide.threadId)
    }
    state.setSidePanelOpen(false)
    setSubagentInspector((current) => ({
      parentThreadId: targetThreadId,
      selectedKey: latestSide ? sideConversationInspectorKey(latestSide.threadId) : null,
      subagents: current?.parentThreadId === targetThreadId ? current.subagents : []
    }))
    setRightSidebarWidth((width) => Math.max(width, SUBAGENT_PANEL_PREFERRED_WIDTH))
    setRightPanelMode('child-agent')
  }, [prepareThreadContextAction, setRightPanelMode, setRightSidebarWidth])

  const forkThreadFromSidebar = useCallback(async (threadId: string): Promise<void> => {
    if (!(await prepareThreadContextAction(threadId))) return
    await forkActiveThread()
  }, [forkActiveThread, prepareThreadContextAction])

  const openSddAssistantPanel = async (): Promise<void> => {
    const draft = useSddDraftStore.getState().activeDraft
    if (!draft) return
    setRightSidebarWidth((width) => Math.max(width, 420))
    const threadId = await ensureSddAssistantThreadForDraft(draft)
    if (!threadId) return
    setRightPanelMode('sdd-ai')
  }

  const toggleSddAssistantPanel = async (): Promise<void> => {
    if (rightPanelMode === 'sdd-ai') {
      setRightPanelMode(null)
      return
    }
    await openSddAssistantPanel()
  }

  const quoteToSddAssistant = (prompt: string): void => {
    const trimmed = prompt.trim()
    if (!trimmed) return
    setInput(input.trim() ? `${input.trim()}\n\n${trimmed}` : trimmed)
    void openSddAssistantPanel()
  }

  const startNewSddRequirement = async (): Promise<void> => {
    const activeCodeWorkspace = activeThreadId
      ? normalizeWorkspaceRoot(codeThreads.find((thread) => thread.id === activeThreadId)?.workspace ?? '')
      : ''
    let targetWorkspace = activeCodeWorkspace || normalizeWorkspaceRoot(workspaceRoot)
    if (!targetWorkspace) {
      const picked = await chooseWorkspace({ selectThreadAfter: false })
      targetWorkspace = normalizeWorkspaceRoot(picked ?? useChatStore.getState().workspaceRoot)
    }
    if (!targetWorkspace) {
      setError(t('workspaceRequiredToCreateThread'))
      return
    }
    const restored = await restoreRememberedSddDraft({
      workspaceRoot: targetWorkspace,
      readWorkspaceFile: window.analytix.files.read
    })
    if (restored.kind === 'restored') {
      await openSddRequirementDraft(restored.draft, restored.content, {
        lastSavedContent: restored.lastSavedContent,
        saveStatus: restored.saveStatus
      })
      return
    }

    const draftUuid = globalThis.crypto?.randomUUID?.() ?? `draft-${Date.now()}`
    const draft = createSddDraft({ id: draftUuid, workspaceRoot: targetWorkspace })
    const initialContent = [
      `# ${t('sddUntitledRequirement')}`,
      '',
      `## ${t('sddTemplateBackground')}`,
      '',
      `## ${t('sddTemplateGoal')}`,
      '',
      `## ${t('sddTemplateAcceptance')}`,
      ''
    ].join('\n')
    const result = await window.analytix.files.createFile({
      workspaceRoot: targetWorkspace,
      path: draft.relativePath,
      content: initialContent
    })
    if (!result.ok) {
      setError(result.message)
      return
    }
    const activeDraft = { ...draft, absolutePath: result.path }
    await openSddRequirementDraft(activeDraft, initialContent)
  }

  const openSddRequirementDraftFromHistory = async (draft: SddDraft): Promise<void> => {
    const current = useSddDraftStore.getState().activeDraft
    if (current && current.id !== draft.id) {
      await saveActiveSddDraftToDisk()
    }
    const restored = await restoreSddDraft({
      draft,
      readWorkspaceFile: window.analytix.files.read
    })
    if (restored.kind !== 'restored') {
      setError(restored.kind === 'unreadable' ? restored.message : t('sddDraftHistoryOpenFailed'))
      return
    }
    await openSddRequirementDraft(restored.draft, restored.content, {
      lastSavedContent: restored.lastSavedContent,
      saveStatus: restored.saveStatus
    })
  }

  const sddDraftFromRegisteredThread = (threadId: string): SddDraft | null => {
    const ref = sddDraftRefForThreadId(threadId)
    if (!ref) return null
    const timestamp = new Date(0).toISOString()
    return {
      id: buildSddDraftId(ref.workspaceRoot, ref.draftRelativePath),
      workspaceRoot: ref.workspaceRoot,
      relativePath: ref.draftRelativePath,
      createdAt: timestamp,
      updatedAt: timestamp
    }
  }

  const findSddDraftForSidebarThread = async (
    threadId: string,
    thread: NormalizedThread | null
  ): Promise<SddDraft | null> => {
    const normalizedThreadId = threadId.trim()
    if (!normalizedThreadId) return null

    if (isSddAssistantThread(thread ?? { id: normalizedThreadId })) {
      return sddDraftFromRegisteredThread(normalizedThreadId)
    }

    if (thread && !isEmptySddAssistantThreadCandidate(thread)) return null
    const listWorkspaceDirectory = window.analytix?.files?.listDirectory
    const readWorkspaceFile = window.analytix?.files?.read
    if (typeof listWorkspaceDirectory !== 'function' || typeof readWorkspaceFile !== 'function') {
      return null
    }

    const targetWorkspace = normalizeWorkspaceRoot(thread?.workspace || workspaceRoot)
    if (!targetWorkspace) return null
    const history = await listSddDraftHistory({
      workspaceRoot: targetWorkspace,
      listWorkspaceDirectory,
      readWorkspaceFile,
      limit: 80
    }).catch(() => [])
    return history.find((draft) => draft.chatThreadIds?.includes(normalizedThreadId)) ?? null
  }

  // NOTE: We intentionally do NOT auto-restore a remembered requirement draft
  // on mount / workspace switch. Opening the app (or switching the working
  // directory) should land on a clean new conversation in the selected
  // directory — not silently reopen the last requirement. Remembered drafts
  // stay reachable from the sidebar (需求草稿) and the "新建需求" restore-or-create
  // flow; they just no longer hijack startup. See the workspace picker below
  // the composer for switching directories.

  // Inject a PM-skill framework prompt (see pm-skill-frameworks.ts) into the
  // Requirement AI composer and remember it so the next send applies the
  // framework's guidance. Frameworks without guidance (the generic
  // clarify/research actions) only set the composer text.
  const applySddFramework = (frameworkId: string): void => {
    const framework = frameworkById(frameworkId)
    if (!framework?.promptKey) return
    const promptText = t(framework.promptKey)
    setInput(input.trim() ? `${input.trim()}\n\n${promptText}` : promptText)
    // Arm the framework only when it carries guidance; the latest click wins.
    pendingSddFrameworkRef.current = framework.guidance ? framework.id : null
    pendingSddFrameworkPromptRef.current = framework.guidance ? promptText : null
  }

  const sendSddAssistantPrompt = async (value: string): Promise<void> => {
    const v = value.trim()
    const draft = useSddDraftStore.getState().activeDraft
    const attachments = composerAttachments
    const imageAttachments = attachments.filter((attachment) => attachment.kind === 'image')
    const documentAttachments = attachments.filter((attachment) => attachment.kind === 'document')
    const attachmentIds = attachments.map((attachment) => attachment.id)
    const publicAttachments = projectAttachmentReferencesForPublicSurfaces(attachments)
    if ((!v && attachmentIds.length === 0 && documentAttachments.length === 0) || !draft) return
    if (attachmentIds.length > 0 && !attachmentUploadEnabled) {
      setAttachmentUploadError(t('composerAttachmentModelUnsupported'))
      return
    }
    if (imageAttachments.length > 0 && !imageAttachmentUploadEnabled) {
      setAttachmentUploadError(t('composerAttachmentModelUnsupported'))
      return
    }
    const threadId = await ensureSddAssistantThreadForDraft(draft)
    if (!threadId) return
    const snapshot = useSddDraftStore.getState()
    void saveActiveSddDraftToDisk()
    const userPrompt = v || (documentAttachments.length > 0
      ? t('composerFileOnlyPrompt')
      : t('composerImageOnlyPrompt'))
    // Apply the armed framework only if its injected prompt is still in the
    // message being sent — editing it away, clearing the composer, or switching
    // drafts all leave a value that no longer contains it, so it is dropped.
    const pendingPrompt = pendingSddFrameworkPromptRef.current
    const frameworkId =
      pendingSddFrameworkRef.current && pendingPrompt && value.includes(pendingPrompt)
        ? pendingSddFrameworkRef.current
        : null
    const prompt = composeSddAssistantPrompt({
      userPrompt,
      draftMarkdown: snapshot.content,
      draftRelativePath: draft.relativePath,
      workspaceRoot: draft.workspaceRoot,
      ...(frameworkId ? { frameworkIds: [frameworkId] } : {})
    })
    setInput('')
    const model = writeAssistantModel.trim()
    const providerId = resolvedWriteAssistantProviderId.trim()
    const reasoningEffort = composerReasoningEffortRequestValue(composerReasoningEffort)
    const sent = await sendMessage(prompt, mode === 'plan' ? 'plan' : 'agent', {
      displayText: v || (documentAttachments.length > 0
        ? t('composerFileOnlyDisplay', { count: documentAttachments.length })
        : t('composerImageOnlyDisplay')),
      ...(model ? { model } : {}),
      ...(providerId ? { providerId } : {}),
      ...(reasoningEffort ? { reasoningEffort } : {}),
      ...(attachmentIds.length ? { attachmentIds } : {}),
      ...(publicAttachments.length ? { attachments: publicAttachments } : {})
    })
    if (sent) {
      pendingSddFrameworkRef.current = null
      pendingSddFrameworkPromptRef.current = null
      if (attachmentIds.length > 0) clearComposerAttachments()
    } else {
      // Restore the composer (incl. any framework prompt) so a retry re-applies
      // the same guidance the user still sees; the refs are intentionally kept.
      setInput(v)
    }
  }

  const uploadSddImagesAsAttachments = async (
    images: SddDraftImageReference[],
    threadId: string,
    workspace: string
  ): Promise<{ images: SddDraftImageReference[]; attachmentIds: string[] }> => {
    const provider = getProvider()
    const attachmentCapabilities = runtimeInfo?.capabilities.attachments
    if (!attachmentCapabilities || typeof provider.uploadAttachment !== 'function') {
      throw new Error(t('composerAttachmentUnavailable'))
    }
    const attachmentIds: string[] = []
    for (const image of images) {
      const file = base64ImageToFile(image)
      const prepared = await prepareImageAttachmentUpload(file, attachmentCapabilities)
      const attachment = await provider.uploadAttachment({
        name: fileNameFromPath(image.relativePath),
        mimeType: prepared.mimeType,
        dataBase64: prepared.dataBase64,
        textFallback: prepared.textFallback,
        threadId,
        workspace
      })
      attachmentIds.push(attachment.id)
    }
    return { images: withAttachmentIds(images, attachmentIds), attachmentIds }
  }

  const firstVisionCapableModel = (): { modelId: string; providerId?: string } | null => {
    for (const group of composerModelGroups) {
      for (const modelId of group.modelIds) {
        const profile = modelProfileForSelection(composerModelGroups, modelId, group.providerId)
        if (modelSupportsImageInput(profile) || composerModelIdLooksMultimodal(modelId)) {
          const providerId = group.providerId.trim()
          return {
            modelId,
            ...(providerId ? { providerId } : {})
          }
        }
      }
    }
    return null
  }

  /** Send a prototype-generation turn to the SDD assistant. Image-driven
   * prototypes need a vision model: prompt to switch when the current one
   * cannot read images. Returns false when nothing was sent. */
  const sendSddPrototypeTurn = async (payload: {
    prompt: string
    displayText: string
    image?: { absolutePath: string; alt: string }
  }): Promise<boolean> => {
    const draft = useSddDraftStore.getState().activeDraft
    if (!draft) return false
    if (runtimeConnection !== 'ready') {
      useSddDraftStore.getState().setOperationStatus('error', t('runtimeActionNeedsConnection'))
      return false
    }

    if (payload.image && !selectedModelSupportsImageInput) {
      const visionSelection = firstVisionCapableModel()
      if (!visionSelection) {
        useSddDraftStore.getState().setOperationStatus('error', t('sddPrototypeNoVisionModel'))
        return false
      }
      const switchModel = await confirmDialog(
        t('sddPrototypeSwitchVisionModel', { model: visionSelection.modelId })
      )
      if (!switchModel) return false
      setWriteAssistantModel(visionSelection.modelId, visionSelection.providerId)
    }

    const threadId = await ensureSddAssistantThreadForDraft(draft)
    if (!threadId) return false
    await openSddAssistantPanel()

    let attachmentIds: string[] = []
    if (payload.image) {
      try {
        const read = await window.analytix.files.readImage({
          path: payload.image.absolutePath,
          workspaceRoot: draft.workspaceRoot
        })
        if (!read.ok) throw new Error(read.message)
        const dataBase64 = read.dataUrl.split(';base64,', 2)[1] ?? ''
        if (!dataBase64) throw new Error(t('composerAttachmentUnavailable'))
        const uploaded = await uploadSddImagesAsAttachments(
          [
            {
              index: 1,
              alt: payload.image.alt,
              markdownPath: payload.image.absolutePath,
              relativePath: payload.image.absolutePath,
              mimeType: read.mimeType,
              dataBase64,
              byteSize: read.size
            }
          ],
          threadId,
          draft.workspaceRoot
        )
        attachmentIds = uploaded.attachmentIds
      } catch (error) {
        useSddDraftStore.getState().setOperationStatus(
          'error',
          error instanceof Error ? error.message : String(error)
        )
        return false
      }
    }

    const assistantSelection = useWriteWorkspaceStore.getState()
    const model = assistantSelection.assistantModel.trim()
    const providerId =
      assistantSelection.assistantProviderId.trim() || providerIdForComposerModel(composerModelGroups, model)
    return sendMessage(payload.prompt, 'agent', {
      displayText: payload.displayText,
      ...(model ? { model } : {}),
      ...(providerId ? { providerId } : {}),
      ...(attachmentIds.length ? { attachmentIds } : {})
    })
  }

  const handleSddNextStep = async (): Promise<void> => {
    const snapshot = useSddDraftStore.getState()
    const draft = snapshot.activeDraft
    if (!draft) return
    if (sddUpgradeInFlightRef.current || snapshot.operationStatus === 'upgrading') return
    if (!snapshot.content.trim()) {
      useSddDraftStore.getState().setOperationStatus('error', t('sddEmptyDraftError'))
      return
    }
    // An in-flight image placeholder would be snapshotted into the plan prompt
    // (and trip the image collector); finish or delete it first.
    if (snapshot.content.includes(PENDING_INFOGRAPHIC_PROTOCOL)) {
      useSddDraftStore.getState().setOperationStatus('error', t('sddPendingImageBlocked'))
      return
    }
    const chatSnapshot = useChatStore.getState()
    if (chatSnapshot.busy || threadHasPendingRuntimeWork(chatSnapshot.blocks)) {
      setError(t('composerQueuePlaceholder'))
      return
    }
    if (chatSnapshot.runtimeConnection !== 'ready') {
      setError(t('runtimeActionNeedsConnection'))
      return
    }
    sddUpgradeInFlightRef.current = true
    useSddDraftStore.getState().setOperationStatus('upgrading')
    const saved = await saveActiveSddDraftToDisk()
    if (!saved) {
      sddUpgradeInFlightRef.current = false
      useSddDraftStore.getState().setOperationStatus('error', useSddDraftStore.getState().error)
      return
    }

    const threadId = await ensureSddAssistantThreadForDraft(draft)
    if (!threadId) {
      sddUpgradeInFlightRef.current = false
      useSddDraftStore.getState().setOperationStatus('idle')
      return
    }

    const collected = await collectSddDraftImages({
      markdown: useSddDraftStore.getState().content,
      draftRelativePath: draft.relativePath,
      workspaceRoot: draft.workspaceRoot
    })
    if (collected.errors.length > 0) {
      sddUpgradeInFlightRef.current = false
      useSddDraftStore.getState().setOperationStatus('error', collected.errors.join('\n'))
      return
    }

    const supportsImageAttachments =
      collected.images.length > 0 &&
      runtimeInfo?.capabilities.model.inputModalities.includes('image') === true &&
      runtimeInfo.capabilities.attachments.available === true &&
      typeof getProvider().uploadAttachment === 'function'

    let imagesForPrompt = collected.images
    let attachmentIds: string[] = []
    let imageMode: 'attachments' | 'base64' | 'none' =
      collected.images.length === 0 ? 'none' : 'base64'

    if (supportsImageAttachments) {
      try {
        const uploaded = await uploadSddImagesAsAttachments(collected.images, threadId, draft.workspaceRoot)
        imagesForPrompt = uploaded.images
        attachmentIds = uploaded.attachmentIds
        imageMode = 'attachments'
      } catch (error) {
        sddUpgradeInFlightRef.current = false
        useSddDraftStore.getState().setOperationStatus(
          'error',
          error instanceof Error ? error.message : String(error)
        )
        return
      }
    }

    const latestDraftContent = useSddDraftStore.getState().content
    const planRelativePath = sddDraftPlanRelativePath(draft)
    const planId = buildGuiPlanId(draft.workspaceRoot, planRelativePath)
    const sourceRequest = sddDraftSourceRequest(latestDraftContent, draft.relativePath)
    const assistantContext = sddAssistantContextFromBlocks(useChatStore.getState().blocks)
    const prompt = buildSddDraftToPlanPrompt({
      draftMarkdown: latestDraftContent,
      draftRelativePath: draft.relativePath,
      planRelativePath,
      assistantContext,
      workspaceRoot: draft.workspaceRoot,
      images: imagesForPrompt,
      imageMode,
      ...(draft.designContext ? { designContext: draft.designContext } : {})
    })
    sddUpgradeTargetRef.current = {
      planId,
      relativePath: planRelativePath,
      workspaceRoot: draft.workspaceRoot
    }
    setMode('plan')
    const model = writeAssistantModel.trim()
    const providerId = resolvedWriteAssistantProviderId.trim()
    const sent = await sendPlanTurn(prompt, {
      displayText: t('sddGeneratePlanAction'),
      workspaceRoot: draft.workspaceRoot,
      ...(model ? { model } : {}),
      ...(providerId ? { providerId } : {}),
      guiPlan: {
        operation: 'draft',
        workspaceRoot: draft.workspaceRoot,
        relativePath: planRelativePath,
        planId,
        sourceRequest
      },
      ...(attachmentIds.length ? { attachmentIds } : {})
    })
    if (!sent) {
      sddUpgradeInFlightRef.current = false
      sddUpgradeTargetRef.current = null
      useSddDraftStore.getState().setOperationStatus('idle')
      return
    }
    // Baseline the trace snapshot so later draft edits can be detected as
    // requirement drift against the plan that is about to be generated.
    const tracePath = sddDraftTraceRelativePath(draft.relativePath)
    if (tracePath) {
      await window.analytix
        .files.write({
          workspaceRoot: draft.workspaceRoot,
          path: tracePath,
          content: JSON.stringify(
            buildSddTraceSnapshot(latestDraftContent, planRelativePath),
            null,
            2
          )
        })
        .catch(() => undefined)
    }
  }

  const readComposerFileContextEntries = async (
    references: ComposerFileReference[],
    workspace: string
  ): Promise<ComposerFileContextEntry[]> => {
    const entries: ComposerFileContextEntry[] = []
    const seen = new Set<string>()
    let remainingChars = COMPOSER_FILE_CONTEXT_MAX_TOTAL_CHARS

    const contextKey = (path: string): string =>
      path.trim().replaceAll('\\', '/').replace(/\/+/g, '/').toLowerCase()

    // strict=true (explicit file mention) surfaces read errors to the user;
    // strict=false (directory expansion) silently skips files that vanished.
    const appendFileEntry = async (
      reference: ComposerFileReference,
      strict: boolean
    ): Promise<void> => {
      if (remainingChars <= 0) return
      const key = contextKey(reference.relativePath || reference.path)
      if (seen.has(key)) return
      const result = await window.analytix.files.read({
        ...(reference.workspaceRoot === null
          ? {}
          : { workspaceRoot: reference.workspaceRoot || workspace }),
        path: reference.workspaceRoot === null
          ? reference.path
          : (reference.relativePath || reference.path)
      })
      if (!result.ok) {
        if (!strict) return
        throw new Error(t('composerFileReadFailed', {
          path: reference.relativePath,
          message: result.message
        }))
      }
      seen.add(key)
      const clipped = clipComposerFileContext(result.content, remainingChars, result.truncated)
      remainingChars -= clipped.consumed
      entries.push({
        relativePath: reference.relativePath,
        content: clipped.content,
        ...(clipped.truncated ? { truncated: true } : {})
      })
    }

    for (const reference of references) {
      if (remainingChars <= 0) break
      if (isComposerDirectoryReference(reference)) {
        const index = await loadWorkspaceFileIndex(workspace).catch(() => null)
        const dirFiles = index
          ? filesUnderDirectory(index.files, reference.relativePath)
              .slice(0, COMPOSER_DIRECTORY_CONTEXT_MAX_FILES)
          : []
        for (const file of dirFiles) {
          if (remainingChars <= 0) break
          await appendFileEntry(file, false)
        }
        continue
      }
      await appendFileEntry(reference, true)
    }
    return entries
  }

  const handleSend = (): void => {
    void handleSendAsync()
  }

  const handleSendAsync = async (overrideInput?: string, actionReferences?: NativeReference[]): Promise<void> => {
    const v = (overrideInput ?? input).trim()
    const nativeAction = actionReferences !== undefined
    const documentState = useWriteWorkspaceStore.getState()
    const frozenQuotes = nativeAction ? [] : [...documentState.quotedSelections]
    const frozenNativeReferences = structuredClone(actionReferences ?? useNativeReferenceStore.getState().references.filter(r => r.threadId === activeThreadId))
    const editorRequest = overrideInput !== undefined && !nativeAction
    const documentContext = frozenQuotes.length > 0 || editorRequest
    const currentReferenceSnapshot = () => {
      const chat = useChatStore.getState()
      const doc = useWriteWorkspaceStore.getState()
      return {
        threadId: chat.activeThreadId,
        threadWorkspace: chat.threads.find((thread) => thread.id === chat.activeThreadId)?.workspace || chat.workspaceRoot,
        documentWorkspace: doc.workspaceRoot, filePath: doc.activeFilePath,
        content: doc.activeFileKind === 'pdf' ? String(doc.pdfMtimeMs) : doc.fileContent
      }
    }
    const frozenReference = { ...currentReferenceSnapshot(), quotes: frozenQuotes, documentContext }
    const referenceCurrent = () => {
      const current = currentReferenceSnapshot()
      const native = useNativeOfficeStore.getState()
      return workbenchReferencesCurrent(frozenReference, current) &&
        (!nativeAction || route === 'chat' && !!current.threadId) &&
        (!nativeAction || nativeActionReferencesCurrent(frozenNativeReferences, [...(native.view ? [native.view] : []), ...Object.values(native.views)])) &&
        frozenNativeReferences.every(reference => reference.threadId === current.threadId &&
          normalizeWorkspaceRoot(reference.workspace) === normalizeWorkspaceRoot(current.threadWorkspace))
    }
    if (!referenceCurrent()) { setError(t('workbenchReferenceChanged')); return }
    const attachments = !nativeAction && (route === 'chat' || route === 'write') ? composerAttachments : []
    const imageAttachments = attachments.filter((attachment) => attachment.kind === 'image')
    const documentAttachments = attachments.filter((attachment) => attachment.kind === 'document')
    const attachmentIds = attachments.map((attachment) => attachment.id)
    const publicAttachments = projectAttachmentReferencesForPublicSurfaces(attachments)
    const fileReferences = !nativeAction && route === 'chat' ? composerFileReferences : []
    const clearSubmittedDraft = (): void => {
      if (!nativeAction) clearSubmittedComposer({ includeInput: overrideInput === undefined, attachments, fileReferences })
      for (const reference of frozenNativeReferences) useNativeReferenceStore.getState().remove(reference.id)
      for (const quote of frozenQuotes) useWriteWorkspaceStore.getState().removeQuotedSelection(quote.id)
    }
    const runtimeFileReferences = runtimeFileReferencesFromComposer(fileReferences)
    const reasoningEffort = composerReasoningEffortRequestValue(composerReasoningEffort)
    if (!v && attachmentIds.length === 0 && documentAttachments.length === 0 && fileReferences.length === 0 && frozenQuotes.length === 0 && frozenNativeReferences.length === 0) return
    if (attachmentIds.length > 0 && !attachmentUploadEnabled) {
      setAttachmentUploadError(t('composerAttachmentModelUnsupported'))
      return
    }
    if (imageAttachments.length > 0 && !imageAttachmentUploadEnabled) {
      setAttachmentUploadError(t('composerAttachmentModelUnsupported'))
      return
    }
    const emptyMessage = workbenchEmptyMessageKeys(
      fileReferences.length + documentAttachments.length, frozenQuotes.length + frozenNativeReferences.length, imageAttachments.length
    )
    const emptyDisplayText = v ? undefined : t(emptyMessage.display, { count: emptyMessage.count })
    const rawMessageText = v || t(emptyMessage.prompt)
    const editorPreset = !editorRequest ? undefined : documentState.agentPresets.find(
      (preset) => preset.id === documentState.assistantAgentPresetId
    )
    const prepareChatMessage = async (): Promise<{
      text: string
      displayText?: string
      fileReferences?: UserFileReference[]
    } | null> => {
      const documentMessage = await prepareWorkbenchDocumentMessage({
        input: rawMessageText, quotes: frozenQuotes, editorRequest,
        workspaceRoot: documentState.workspaceRoot, activeFilePath: documentState.activeFilePath,
        requestUserInputAvailable: typeof getProvider().submitUserInputResponse === 'function',
        editorPersona: editorPreset ? resolveWriteAgentPreset(editorPreset).persona : undefined,
        retrieveContext: window.analytix?.write?.retrieveWriteContext
      })
      const messageText = frozenNativeReferences.length ? `${documentMessage}\n\n${nativeReferencesPrompt(frozenNativeReferences)}` : documentMessage
      if (fileReferences.length === 0) {
        return {
          text: messageText,
          ...(emptyDisplayText ? { displayText: emptyDisplayText } : {})
        }
      }
      const workspace = normalizeWorkspaceRoot(
        threads.find((thread) => thread.id === activeThreadId)?.workspace || workspaceRoot
      )
      if (!workspace) {
        setError(t('workspaceRequiredToCreateThread'))
        return null
      }
      try {
        const fileContext = await readComposerFileContextEntries(fileReferences, workspace)
        const displayText = v || emptyDisplayText
        return {
          text: buildComposerFileContextPrompt(messageText, fileContext),
          ...(displayText ? { displayText } : {}),
          ...(runtimeFileReferences.length ? { fileReferences: runtimeFileReferences } : {})
        }
      } catch (error) {
        setError(error instanceof Error ? error.message : String(error))
        return null
      }
    }

    if (!nativeAction && activeSddDraft && rightPanelMode === 'sdd-ai') {
      void sendSddAssistantPrompt(v)
      return
    }
    const planCommand = nativeAction ? null : parseGuiPlanCommand(v)
    if (planCommand) {
      setInput('')
      void handleGuiPlanCommand(planCommand.kind === 'create' ? planCommand.request : undefined)
      return
    }
    if (route === 'chat' && mode === 'plan') {
      const prepared = await prepareChatMessage()
      if (!prepared) return
      if (!referenceCurrent()) { setError(t('workbenchReferenceChanged')); return }
      const model = composerModel.trim()
      const providerId = composerProviderId.trim()
      const sent = await sendPlanTurn(prepared.text, {
        ...(prepared.displayText ? { displayText: prepared.displayText } : {}),
        ...(model ? { model } : {}),
        ...(providerId ? { providerId } : {}),
        ...(reasoningEffort ? { reasoningEffort } : {}),
        ...(attachmentIds.length ? { attachmentIds } : {}),
        ...(publicAttachments.length ? { attachments: publicAttachments } : {}),
        ...(prepared.fileReferences?.length ? { fileReferences: prepared.fileReferences } : {})
      })
      if (sent) {
        clearSubmittedDraft()
      }
      return
    }
    if (route === 'claw') {
      const command = parseClawCommand(v)
      if (command?.kind === 'clear') {
        if (!activeClawChannelId) {
          setError(t('clawNoActiveIm'))
          return
        }
        setInput('')
        void (async () => {
          await resetClawChannelSession(activeClawChannelId)
          const replyText = t('clawNewSessionStarted')
          appendLocalClawTurn(v, replyText)
          await mirrorClawCommand(v, replyText)
        })()
        return
      }
      if (command?.kind === 'help') {
        setInput('')
        const replyText = clawHelpText()
        appendLocalClawTurn(v, replyText)
        void mirrorClawCommand(v, replyText)
        return
      }
      if (command?.kind === 'model') {
        if (!activeClawChannelId) {
          setError(t('clawNoActiveIm'))
          return
        }
        setInput('')
        void (async () => {
          await setClawChannelModel(activeClawChannelId, command.model)
          const replyText = t('clawModelChanged', { model: command.model })
          appendLocalClawTurn(v, replyText)
          await mirrorClawCommand(v, replyText)
        })()
        return
      }
      if (command?.kind === 'showModel') {
        if (!activeClawChannelId) {
          setError(t('clawNoActiveIm'))
          return
        }
        setInput('')
        const replyText = t('clawModelCurrent', {
          model: activeClawChannel?.model ?? 'auto'
        })
        appendLocalClawTurn(v, replyText)
        void mirrorClawCommand(v, replyText)
        return
      }
      if (command?.kind === 'showProvider' || command?.kind === 'provider') {
        setError('Provider commands are available in IM chats.')
        return
      }
      if (!activeClawChannelId) {
        setError(t('clawNoActiveIm'))
        return
      }
      setInput('')
      void (async () => {
        const taskResult = typeof window.analytix?.connectPhone?.createTaskFromText === 'function'
          ? await window.analytix.connectPhone.createTaskFromText(v, {
              channelId: activeClawChannelId,
              modelHint: activeClawChannel?.model,
              ...(reasoningEffort ? { reasoningEffort } : {}),
              mode
            })
          : { kind: 'noop' as const }
        if (taskResult.kind === 'created') {
          appendLocalClawTurn(v, taskResult.confirmationText)
          await mirrorClawCommand(v, taskResult.confirmationText)
          return
        }
        if (taskResult.kind === 'error') {
          appendLocalClawTurn(v, `Failed to create scheduled task: ${taskResult.message}`)
          return
        }
        if (!activeThreadId) {
          await selectClawChannel(activeClawChannelId)
          await useChatStore.getState().sendMessage(v, mode === 'plan' ? 'plan' : 'agent', {
            ...(reasoningEffort ? { reasoningEffort } : {})
          })
          return
        }
        await sendMessage(v, mode === 'plan' ? 'plan' : 'agent', {
          ...(reasoningEffort ? { reasoningEffort } : {})
        })
      })()
      return
    }
    const prepared = await prepareChatMessage()
    if (!prepared) return
    if (!referenceCurrent()) { setError(t('workbenchReferenceChanged')); return }
    const sent = await sendMessage(prepared.text, mode === 'plan' ? 'plan' : 'agent', {
      ...(prepared.displayText ? { displayText: prepared.displayText } : {}),
      ...(reasoningEffort ? { reasoningEffort } : {}),
      ...(attachmentIds.length ? { attachmentIds } : {}),
      ...(publicAttachments.length ? { attachments: publicAttachments } : {}),
      ...(prepared.fileReferences?.length ? { fileReferences: prepared.fileReferences } : {})
    })
    if (sent) {
      clearSubmittedDraft()
    }
  }

  const mainComposerModel =
    route === 'claw'
      ? clawChannels.find((channel) => channel.id === activeClawChannelId)?.model ?? 'auto'
      : composerModel
  const composerViewState = useMemo(
    () =>
      createComposerViewState({
        input,
        mode,
        busy,
        runtimeReady: runtimeConnection === 'ready',
        hasActiveThread: Boolean(activeThreadId),
        contextWindowTokens: runtimeInfo?.capabilities.model.contextWindowTokens,
        runtimeToolCount:
          runtimeInfo
            ? runtimeInfo.capabilities.mcp.search?.active
              ? runtimeInfo.capabilities.mcp.search.advertisedToolCount
              : runtimeInfo.capabilities.mcp.toolCount
            : undefined,
        runtimeSkillCount: runtimeInfo?.capabilities.skills.discoveredSkills,
        composerModel: mainComposerModel,
        composerProviderId: route === 'chat' ? composerProviderId : undefined,
        composerReasoningEffort:
          route === 'chat' || route === 'claw' ? composerReasoningEffort : undefined
      }),
    [
      activeThreadId,
      busy,
      composerProviderId,
      composerReasoningEffort,
      input,
      mainComposerModel,
      mode,
      route,
      runtimeConnection,
      runtimeInfo
    ]
  )
  const composerController = createComposerController({
    setInput,
    setMode,
    send: handleSend,
    interrupt: (options) => void interrupt(options),
    setModel: (modelId, providerId) => {
      if (route === 'claw' && activeClawChannelId) {
        void setClawChannelModel(activeClawChannelId, modelId)
        return
      }
      setComposerModel(modelId, providerId)
    },
    setReasoningEffort:
      route === 'chat' || route === 'claw' ? setComposerReasoningEffort : undefined
  })

  const openThread = (id: string): void => {
    setConnectPhoneSidebarOpen(false)
    setActiveDataAnalysis(null)
    void (async () => {
      const thread = threads.find((item) => item.id === id) ?? null
      const sddDraft = await findSddDraftForSidebarThread(id, thread)
      if (sddDraft) {
        markSddAssistantThread(sddDraft, id)
        await openSddRequirementDraftFromHistory(sddDraft)
        void useChatStore.getState().refreshThreads()
        return
      }
      if (useSddDraftStore.getState().activeDraft) dismissActiveSddDraft({ closeAssistant: true })
      setRoute('chat')
      await selectThread(id)
    })()
  }

  const startNewChat = (): void => {
    if (activeSddDraft) dismissActiveSddDraft({ closeAssistant: true })
    setConnectPhoneSidebarOpen(false)
    setActiveDataAnalysis(null)
    setRoute('chat')
    void createThread()
  }

  const startNewChatInWorkspace = (workspaceRoot: string): void => {
    if (activeSddDraft) dismissActiveSddDraft({ closeAssistant: true })
    setConnectPhoneSidebarOpen(false)
    setActiveDataAnalysis(null)
    setRoute('chat')
    void createThread({ workspaceRoot })
  }

  const openDataAnalysis = (targetWorkspaceRoot: string, itemId: DataAnalysisItemId): void => {
    const normalizedWorkspaceRoot = normalizeWorkspaceRoot(targetWorkspaceRoot)
    if (!normalizedWorkspaceRoot) return
    if (activeSddDraft) dismissActiveSddDraft({ closeAssistant: true })
    setConnectPhoneSidebarOpen(false)
    setRightPanelMode(null)
    setActiveDataAnalysis({ workspaceRoot: normalizedWorkspaceRoot, itemId })
    setRoute('chat')
    void loadDataAnalysisSurface()
    void preloadCaseOverviewForWorkspace(normalizedWorkspaceRoot).catch(() => {})
  }

  const openCodeMode = (): void => {
    setConnectPhoneSidebarOpen(false)
    setActiveDataAnalysis(null)
    void openCode()
  }

  const openWriteMode = (): void => {
    setConnectPhoneSidebarOpen(false)
    void (async () => {
      await openWrite()
    })()
  }

  const openPluginsView = (): void => {
    setConnectPhoneSidebarOpen(false)
    openPlugins(sidebarView === 'claw' ? 'claw' : 'chat')
  }

  const openScheduleView = (): void => {
    setConnectPhoneSidebarOpen(false)
    openSchedule()
  }

  const toggleConnectPhone = (): void => {
    if (activeSddDraft) dismissActiveSddDraft({ closeAssistant: true })
    openClaw()
    setConnectPhoneSidebarOpen((open) => !open)
  }

  const currentShellHistoryEntry = useMemo<ShellHistoryEntry>(() => ({
    route,
    connectPhoneSidebarOpen,
    pluginHostRoute
  }), [connectPhoneSidebarOpen, pluginHostRoute, route])
  const currentShellHistoryEntryRef = useRef(currentShellHistoryEntry)
  const lastShellHistoryEntryRef = useRef<ShellHistoryEntry | null>(null)
  const applyingShellHistoryKeyRef = useRef<string | null>(null)
  const [shellHistory, setShellHistory] = useState<{
    back: ShellHistoryEntry[]
    forward: ShellHistoryEntry[]
  }>({
    back: [],
    forward: []
  })

  useEffect(() => {
    currentShellHistoryEntryRef.current = currentShellHistoryEntry
    const currentKey = shellHistoryEntryKey(currentShellHistoryEntry)
    const applyingKey = applyingShellHistoryKeyRef.current
    if (applyingKey) {
      lastShellHistoryEntryRef.current = currentShellHistoryEntry
      if (currentKey === applyingKey) applyingShellHistoryKeyRef.current = null
      return
    }

    const previous = lastShellHistoryEntryRef.current
    if (!previous) {
      lastShellHistoryEntryRef.current = currentShellHistoryEntry
      return
    }
    if (shellHistoryEntryKey(previous) === currentKey) {
      lastShellHistoryEntryRef.current = currentShellHistoryEntry
      return
    }

    setShellHistory((history) => ({
      back: [...history.back, previous].slice(-80),
      forward: []
    }))
    lastShellHistoryEntryRef.current = currentShellHistoryEntry
  }, [currentShellHistoryEntry])

  const applyShellHistoryEntry = useCallback((entry: ShellHistoryEntry): void => {
    if (activeSddDraft) dismissActiveSddDraft({ closeAssistant: true })
    setConnectPhoneSidebarOpen(entry.connectPhoneSidebarOpen)

    if (entry.route === 'write') {
      void (async () => {
        await useWriteWorkspaceStore.getState().openWorkspaceHome()
        await openWrite()
      })()
      return
    }

    if (entry.route === 'plugins') {
      openPlugins(entry.pluginHostRoute)
      return
    }

    if (entry.route === 'claw') {
      openClaw()
      return
    }

    if (entry.route === 'schedule') {
      openSchedule()
      return
    }

    if (entry.route === 'settings') {
      openSettings()
      return
    }

    void openCode()
  }, [activeSddDraft, dismissActiveSddDraft, openClaw, openCode, openPlugins, openSchedule, openSettings, openWrite])

  const navigateShellHistoryBack = useCallback((): void => {
    const target = shellHistory.back[shellHistory.back.length - 1]
    if (!target) return
    const current = currentShellHistoryEntryRef.current
    applyingShellHistoryKeyRef.current = shellHistoryEntryKey(target)
    setShellHistory((history) => ({
      back: history.back.slice(0, -1),
      forward: [current, ...history.forward].slice(0, 80)
    }))
    applyShellHistoryEntry(target)
  }, [applyShellHistoryEntry, shellHistory.back])

  const navigateShellHistoryForward = useCallback((): void => {
    const target = shellHistory.forward[0]
    if (!target) return
    const current = currentShellHistoryEntryRef.current
    applyingShellHistoryKeyRef.current = shellHistoryEntryKey(target)
    setShellHistory((history) => ({
      back: [...history.back, current].slice(-80),
      forward: history.forward.slice(1)
    }))
    applyShellHistoryEntry(target)
  }, [applyShellHistoryEntry, shellHistory.forward])

  const sidebarView: 'chat' | 'write' | 'claw' | 'schedule' =
    route === 'claw' || (route === 'plugins' && pluginHostRoute === 'claw')
      ? 'claw'
      : route === 'schedule'
        ? 'schedule'
      : route === 'write'
        ? 'write'
        : 'chat'

  const closeRightPanel = (): void => {
    setDocumentFocused(false)
    useWorkspaceTabsStore.getState().setOpen(false)
    requestAnimationFrame(() => document.querySelector<HTMLButtonElement>('[aria-controls="workbench-right-workspace"]')?.focus())
  }

  const renderRuntimeBanner = (message: string, detail?: string | null): ReactElement => (
    <RuntimeBanner
      message={message}
      detail={detail}
      logPath={runtimeLogPath || null}
      runtimeReady={runtimeConnection === 'ready'}
      stageInsetClass={stageInsetClass}
      t={t}
      onOpenLogDir={
        typeof window !== 'undefined' && typeof window.analytix?.logs?.openDir === 'function'
          ? () => window.analytix.logs.openDir()
          : undefined
      }
      onOpenSettings={() => openSettings('agents')}
      onRetryConnection={() => void probeRuntime('user', { restart: true })}
    />
  )

  const rightPanelDockedVisible = rightPanelVisible && !planPanelInOverlay
  const fileTreeSidePanelOffset = 0
  const currentDockedRightPanelMode: DockedRightPanelRenderMode | null = rightPanelMode
  const rightPanelMotion = useShellPanelMotion({
    isVisible: rightPanelDockedVisible,
    size: rightSidebarWidth
  })
  const [rightPanelRenderMode, setRightPanelRenderMode] =
    useState<DockedRightPanelRenderMode | null>(currentDockedRightPanelMode)
  useEffect(() => {
    if (rightPanelDockedVisible) {
      setRightPanelRenderMode(currentDockedRightPanelMode)
    }
  }, [currentDockedRightPanelMode, rightPanelDockedVisible])

  useEffect(() => {
    preloadRightPanelIsland(currentDockedRightPanelMode)
  }, [currentDockedRightPanelMode])

  useEffect(() => {
    if (route !== 'chat' || !activeCaseProjectWorkspaceRoot) return undefined
    let cancelled = false
    const timer = window.setTimeout(() => {
      if (cancelled) return
      void preloadCaseOverviewForWorkspace(activeCaseProjectWorkspaceRoot).catch(() => {})
    }, activeDataAnalysis ? 0 : 64)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [activeCaseProjectWorkspaceRoot, activeDataAnalysis, route])

  useEffect(() => {
    if (!activeThreadId || route !== 'chat' || activeSddDraft || activeDataAnalysis) return undefined
    let frameId: number | null = null
    const timer = window.setTimeout(() => {
      frameId = window.requestAnimationFrame(() => {
        preloadRightPanelIsland('summary')
        void preloadSharedThreadSummary(activeThreadId).catch(() => {})
      })
    }, 48)
    return () => {
      window.clearTimeout(timer)
      if (frameId !== null) window.cancelAnimationFrame(frameId)
    }
  }, [activeDataAnalysis, activeSddDraft, activeThreadId, route])

  useEffect(() => {
    if (rightPanelMode === 'summary') preloadRightPanelIsland('summary')
  }, [rightPanelMode])

  const renderPlanPanel = (className: string): ReactElement => (
    <PlanPanel
      workspaceRoot={workspaceRoot}
      activeThreadId={activeThreadId}
      runtimeReady={runtimeConnection === 'ready'}
      busy={busy}
      className={className}
      onCollapse={closeRightPanel}
      onBuildPlan={() => void buildGuiPlan()}
      onVerifyPlan={() => void verifyGuiPlan()}
      onReplanChanged={(ids) => void replanChangedRequirements(ids)}
    />
  )

  const renderSummaryPanel = (): ReactElement => (
    <ThreadSummaryPanelIsland activeThreadId={activeThreadId} className="h-full w-full"
      diagnosticsFocus={runtimeDiagnosticsFocus} onDiagnosticsFocusHandled={() => setRuntimeDiagnosticsFocus(null)}
      onCollapse={closeRightPanel} onInspectChildAgent={inspectChildAgent}
      onSubagentsChange={syncSubagentInspector} onOpenChanges={() => setRightPanelMode('changes')} />
  )

  const activateWorkspaceTab = (id: string): void => {
    const tab = useWorkspaceTabsStore.getState().tabs.find((item) => item.id === id)
    if (!tab) return
    if (tab.mode === 'file' && tab.path) setFilePreviewTarget({ path: tab.path, workspaceRoot: tab.workspaceRoot })
    if (tab.mode === 'child-agent' && tab.instanceId) selectSubagentInspector(tab.instanceId)
    useWorkspaceTabsStore.getState().activateTab(id)
  }

  const closeWorkspaceTab = async (id: string): Promise<void> => {
    const tab = useWorkspaceTabsStore.getState().tabs.find((item) => item.id === id)
    if (!tab) return
    if (tab.kind === 'document' && tab.path && tab.workspaceRoot) {
      if (isNativeOfficeFilePath(tab.path)) {
        if (!(await useNativeOfficeStore.getState().close(tab.workspaceRoot, tab.path))) {
          activateWorkspaceTab(id)
          return
        }
      } else if (useWriteWorkspaceStore.getState().activeFilePath === tab.path) {
        if (!(await useWriteWorkspaceStore.getState().openWorkspaceHome(tab.workspaceRoot))) return
      }
    }
    useWorkspaceTabsStore.getState().closeTab(id)
    const next = useWorkspaceTabsStore.getState().activeTabId
    if (next && useWorkspaceTabsStore.getState().open && !useWorkspaceTabsStore.getState().selectorOpen) activateWorkspaceTab(next)
  }

  const workspaceCommandHandler = useRef<(command: NativeWorkspaceCommand['command']) => void>(() => {})
  workspaceCommandHandler.current = command => {
    const tabs = useWorkspaceTabsStore.getState()
    if (command === 'toggle-workspace') tabs.toggleOpen()
    else if (command === 'toggle-terminal') toggleTerminal()
    else if (tabs.activeTabId) void closeWorkspaceTab(tabs.activeTabId)
  }
  useEffect(() => {
    const fromKeyboard = (event: Event) => {
      const command = (event as CustomEvent).detail
      if (['toggle-workspace','toggle-terminal','close-tab'].includes(command)) workspaceCommandHandler.current(command)
    }
    window.addEventListener('analytix:workspace-shortcut', fromKeyboard)
    const unsubscribe = window.analytix?.office?.onWorkspaceCommand?.(event => {
      if (event.objectId === useNativeOfficeStore.getState().view?.objectId) workspaceCommandHandler.current(event.command)
    })
    return () => { unsubscribe?.(); window.removeEventListener('analytix:workspace-shortcut', fromKeyboard) }
  }, [])

  const renderRightPanel = (): ReactElement | null => {
    if (!rightPanelDockedVisible && !rightPanelMotion.isMounted && !documentsMounted) return null
    const panelMode = rightPanelDockedVisible ? currentDockedRightPanelMode : rightPanelRenderMode

    return (
      <aside
        ref={rightPaneRef}
        id="workbench-right-workspace"
        inert={!rightPanelDockedVisible}
        className="ds-right-sidebar-pane ds-no-drag h-full min-h-0 shrink-0"
        data-open={rightPanelDockedVisible ? 'true' : 'false'}
        data-resizing={rightResizing ? 'true' : 'false'}
        aria-hidden={!rightPanelDockedVisible}
        style={{
          opacity: rightPanelMotion.opacity,
          width: documentFocused ? 'auto' : rightPanelMotion.animatedSize,
          ...(documentFocused ? { position: 'absolute', inset: `48px 8px ${terminalOpen ? terminalHeight + 24 : 8}px 8px`, height: 'auto', zIndex: 60 } as const : {})
        }}
      >
        {rightPanelDockedVisible ? (
          <WorkbenchResizeHandle
            edge="left"
            isResizing={rightResizing}
            onPointerDown={beginRightResize}
            onReset={resetRightSidebarWidth}
          />
        ) : null}
        <div className="ds-right-sidebar-pane-shadow" aria-hidden />
        <div className="ds-right-sidebar-pane-clip">
          <div
            ref={rightPaneContentRef}
            className="ds-right-sidebar-pane-content flex min-h-0 flex-col"
            style={{
              minWidth: documentFocused ? 0 : rightSidebarWidth,
              width: documentFocused ? '100%' : rightSidebarWidth,
              // Document selection menus use viewport coordinates. Layout/paint
              // containment would rebase their fixed position onto this pane.
              contain: panelMode === 'documents' ? 'none' : undefined
            }}
          >
            <WorkspaceTabs tabs={workspaceTabs} activeTabId={workspaceActiveTabId} selectorOpen={workspaceSelectorOpen}
              focused={documentFocused} onSelect={activateWorkspaceTab} onClose={closeWorkspaceTab}
              onReorder={useWorkspaceTabsStore.getState().reorderTab} onAdd={useWorkspaceTabsStore.getState().showSelector}
              onToggleFocus={() => setDocumentFocused((value) => !value)} onCollapse={closeRightPanel} />
            {workspaceSelectorOpen ? <WorkspaceToolSelector filesEnabled={Boolean(fileTreeWorkspaceRoot)}
              sideChatEnabled={runtimeConnection === 'ready' && Boolean(activeThreadId)} planEnabled={Boolean(activeGuiPlan)}
              onOpen={(action) => {
                if (action === 'terminal') { if (!terminalOpen) toggleTerminal(); return }
                if (action === 'sidechat') { openSideChat(); return }
                setRightPanelMode(action)
              }} /> : null}
            {workspaceTabs.filter(tab => tab.id !== workspaceActiveTabId).map(tab => <div key={tab.id} hidden role="tabpanel" id={workspacePanelDomId(tab.id)} aria-labelledby={workspaceTabDomId(tab.id)} />)}
            <div className="min-h-0 flex-1" hidden={workspaceSelectorOpen} role="tabpanel"
              id={workspaceActiveTab ? workspacePanelDomId(workspaceActiveTab.id) : undefined}
              aria-labelledby={workspaceActiveTab ? workspaceTabDomId(workspaceActiveTab.id) : undefined} tabIndex={0}>
            <Suspense fallback={<WorkbenchLoadingFallback surface="sidebar" />}>
              {documentsMounted ? (
                <div className="h-full min-h-0" hidden={panelMode !== 'documents'}>
                  <DocumentWorkspacePanel threadId={activeThreadId} activeTab={workspaceActiveTab} visible={panelMode === 'documents' && rightPanelVisible && !workspaceSelectorOpen} input={input} setInput={setInput} onSubmitPrompt={sendWritePrompt}
                    fileBrowser={renderFileTreeSidePanel(workspaceActiveTab?.workspaceRoot || textDocumentRoot || fileTreeWorkspaceRoot, false)}
                    focused={documentFocused} onToggleFocus={() => setDocumentFocused((value) => !value)}
                    onCollapse={closeRightPanel} onOpenSettings={() => openSettings('write')}
                    onFocusConversation={() => {
                      setDocumentFocused(false)
                      requestAnimationFrame(() => document.querySelector<HTMLElement>('.ds-chat-stage .composer-prompt-editor [contenteditable="true"]')?.focus())
                    }} />
                </div>
              ) : null}
              {workspaceTabs.some((tab) => tab.mode === 'browser') ? (
                <div className="h-full min-h-0" hidden={panelMode !== 'browser'} inert={panelMode !== 'browser' || !rightPanelVisible}>
                  <DevBrowserPanelIsland preferredUrl={latestDevPreviewUrl} className="h-full max-h-full w-full flex-col" onCollapse={closeRightPanel} />
                </div>
              ) : null}
              {panelMode === 'files' ? renderFileTreeSidePanel() : panelMode === 'summary' ? renderSummaryPanel() : panelMode === 'child-agent' && activeSubagentInspector ? (
                <SubagentInspectorPanelIsland tabbedWorkspace
                  subagents={activeSubagentInspector.subagents}
                  selectedKey={activeSubagentInspector.selectedKey}
                  runtimeConnection={runtimeConnection}
                  composerModel={composerModel}
                  composerProviderId={composerProviderId}
                  composerPickList={composerPickList}
                  composerModelGroups={composerModelGroups}
                  composerReasoningEffort={composerReasoningEffort}
                  setComposerModel={setComposerModel}
                  setComposerReasoningEffort={setComposerReasoningEffort}
                  onSelectSubagent={selectSubagentInspector}
                  onCloseSubagentTab={closeSubagentInspectorTab}
                  onCreateSideChat={() => void createSideChatInInspector()}
                  createSideChatDisabled={runtimeConnection !== 'ready' || !activeThreadId}
                  onCollapse={closeRightPanel}
                  onRetryConnection={() => void probeRuntime('user', { restart: true })}
                  onOpenSettings={() => openSettings('agents')}
                  onConfigureProviders={() => openSettings('providers')}
                  className="h-full max-h-full w-full"
                />
              ) : panelMode === 'sdd-ai' && activeSddDraft ? (
                <SddAssistantPanelIsland
                  draft={activeSddDraft}
                  input={input}
                  setInput={setInput}
                  mode={mode}
                  setMode={setMode}
                  busy={busy}
                  runtimeConnection={runtimeConnection}
                  activeThreadId={activeThreadId}
                  composerModel={writeAssistantModel}
                  composerProviderId={resolvedWriteAssistantProviderId}
                  composerPickList={writeAssistantPickList}
                  composerModelGroups={composerModelGroups}
                  composerReasoningEffort={composerReasoningEffort}
                  setComposerModel={setWriteAssistantModel}
                  setComposerReasoningEffort={setComposerReasoningEffort}
                  queuedMessages={queuedMessages}
                  removeQueuedMessage={removeQueuedMessage}
                  attachments={composerAttachments}
                  attachmentUploadEnabled={attachmentUploadEnabled}
                  attachmentUploadBusy={attachmentUploadBusy}
                  attachmentUploadError={attachmentUploadError}
                  onPickAttachments={(files) => void handlePickAttachments(files)}
                  onPasteClipboardImage={(options) => void handlePasteClipboardImage(options)}
                  onRemoveAttachment={removeComposerAttachment}
                  onSend={handleSend}
                  onInterrupt={(options) => void interrupt(options)}
                  onRetryConnection={() => void probeRuntime('user', { restart: true })}
                  onOpenSettings={() => openSettings('agents')}
                  onConfigureProviders={() => openSettings('providers')}
                  onApplyFramework={applySddFramework}
                  onNewConversation={() => {
                    setInput('')
                    pendingSddFrameworkRef.current = null
                    pendingSddFrameworkPromptRef.current = null
                    void createSddAssistantThreadForDraft(activeSddDraft)
                  }}
                  onCollapse={closeRightPanel}
                  className="h-full max-h-full w-full"
                />
              ) : panelMode === 'changes' ? (
                <ChangeInspectorIsland
                  className="h-full max-h-full w-full flex-col"
                  onCollapse={closeRightPanel}
                />
              ) : panelMode === 'todo' ? (
                <TodoPanel
                  className="h-full max-h-full w-full"
                  onCollapse={closeRightPanel}
                  onOpenPlan={openGuiPlanPanel}
                />
              ) : panelMode === 'plan' ? (
                renderPlanPanel('h-full max-h-full w-full')
              ) : panelMode === 'file' ? (
                <WorkspaceFilePreviewPanel
                  target={filePreviewTarget}
                  tabbedWorkspace
                  workspaceRoot={workspaceRoot}
                  className="h-full max-h-full w-full"
                  onSelectTarget={openWorkspaceFilePreviewTarget}
                  onCloseTarget={closeWorkspaceFilePreviewTarget}
                  onClose={() => { if (workspaceActiveTabId) void closeWorkspaceTab(workspaceActiveTabId) }}
                />
              ) : null}
            </Suspense>
            </div>
          </div>
        </div>
      </aside>
    )
  }

  const renderFileTreeSidePanel = (browserRoot = fileTreeWorkspaceRoot, collapsible = true): ReactElement => browserRoot ? (
    <ChatFileTreePanel workspaceRoot={browserRoot} selectedPath={workspaceActiveTab?.path || filePreviewTarget?.path}
      onPreviewFile={previewWorkspaceFileFromSidebar} onAddReference={addWorkspaceReferenceFromSidebar}
      onCollapse={collapsible ? closeRightPanel : undefined} t={t} fill />
  ) : <div className="p-5 text-sm text-ds-muted">{t('workspaceRequiredToCreateThread')}</div>

  const renderPlanPanelOverlay = (): ReactElement | null => {
    if (!planPanelInOverlay) return null
    return (
      <div
        className="ds-plan-panel-overlay ds-no-drag"
        role="dialog"
        aria-modal="true"
        aria-label={t('planPanelTitle')}
      >
        <button
          type="button"
          className="ds-plan-panel-overlay-backdrop"
          aria-label={t('cancel')}
          onClick={closeRightPanel}
        />
        <div className="ds-plan-panel-overlay-card">
          <Suspense fallback={<WorkbenchLoadingFallback surface="sidebar" />}>
            {renderPlanPanel('h-full max-h-full w-full')}
          </Suspense>
        </div>
      </div>
    )
  }

  return (
    <WorkbenchShell ref={shellRef} style={workbenchShellStyle}>
      <ShellNavigationControls
        leftSidebarCollapsed={leftSidebarCollapsed}
        sidebarLabel={leftSidebarCollapsed ? t('sidebarExpand') : t('sidebarCollapse')}
        backLabel={t('navigateBack')}
        forwardLabel={t('navigateForward')}
        newChatLabel={t('newAgent')}
        canGoBack={shellHistory.back.length > 0}
        canGoForward={shellHistory.forward.length > 0}
        newChatDisabled={runtimeConnection !== 'ready'}
        onToggleSidebar={toggleLeftSidebar}
        onBack={navigateShellHistoryBack}
        onForward={navigateShellHistoryForward}
        onNewChat={startNewChat}
      />
      <div
        ref={leftPaneRef}
        className="ds-left-sidebar-pane min-h-0 shrink-0"
        data-collapsed={leftSidebarCollapsed ? 'true' : 'false'}
        data-resizing={leftResizing ? 'true' : 'false'}
        aria-hidden={leftSidebarCollapsed}
        style={{ width: leftSidebarCollapsed ? 0 : leftSidebarWidth }}
      >
        <div className="ds-left-sidebar-pane-clip">
          <div
            ref={leftPaneContentRef}
            className="ds-left-sidebar-pane-content"
            style={{ minWidth: leftSidebarWidth, width: leftSidebarWidth }}
          >
            {leftSidebarMounted ? (
                <Sidebar
		                  threads={codeThreads}
		                  caseProjects={caseProjects}
		                  caseProjectIndexStatus={caseProjectIndexStatus}
		                  caseProjectThreadsById={caseProjectThreadsById}
	                  caseProjectLoadingById={caseProjectLoadingById}
	                  caseProjectErrorsById={caseProjectErrorsById}
	                  caseProjectExpandedById={caseProjectExpandedById}
	                  activeThreadId={activeThreadId}
                  activeView={sidebarView}
                  connectPhoneSidebarOpen={connectPhoneSidebarOpen}
                  pluginsActive={route === 'plugins'}
                  runtimeReady={runtimeConnection === 'ready'}
                  threadSearch={threadSearch}
                  showArchivedThreads={showArchivedThreads}
	                  onThreadSearchChange={setThreadSearch}
	                  onSetCaseProjectExpanded={setCaseProjectExpanded}
                  onSelectThread={openThread}
                  onRenameThread={renameThread}
                  onArchiveThread={(id) => archiveThread(id, true)}
                  onDeleteThread={deleteThread}
                  onRestoreThread={(id) => archiveThread(id, false)}
                  onOpenSideChat={openSideChatForThread}
                  onForkThread={forkThreadFromSidebar}
                  onNewChat={startNewChat}
                  onNewChatInWorkspace={startNewChatInWorkspace}
                  onNewRequirement={() => void startNewSddRequirement()}
                  onOpenRequirementDraft={(draft) => void openSddRequirementDraftFromHistory(draft)}
                  activeDataAnalysis={activeDataAnalysis}
                  onOpenDataAnalysis={openDataAnalysis}
                  onOpenSettings={(section) => openSettings(section)}
                  onOpenPlugins={openPluginsView}
                  focusModeEnabled={focusModeEnabled}
                  onFocusModeChange={updateFocusMode}
                  onToggleConnectPhone={toggleConnectPhone}
                  onCodeOpen={openCodeMode}
                  onWriteOpen={openWriteMode}
                  onScheduleOpen={openScheduleView}
                />
            ) : null}
          </div>
        </div>
        {!leftSidebarCollapsed ? (
          <WorkbenchResizeHandle
            edge="right"
            isResizing={leftResizing}
            onPointerDown={beginLeftResize}
            onReset={resetLeftSidebarWidth}
          />
        ) : null}
      </div>

      <WorkbenchStage className={route === 'plugins' ? 'px-0' : ''}>
        {route === 'plugins' ? (
          <Suspense fallback={<WorkbenchLoadingFallback />}>
            <PluginMarketplaceView leftSidebarCollapsed={leftSidebarCollapsed} />
          </Suspense>
        ) : route === 'schedule' ? (
          <Suspense fallback={<WorkbenchLoadingFallback />}>
            <ScheduleTasksView
              leftSidebarCollapsed={leftSidebarCollapsed}
              onOpenThread={openThread}
            />
          </Suspense>
        ) : (
          <>
            <div className="flex min-h-0 flex-1">
              <div className="flex min-h-0 min-w-0 flex-1">
                {activeSddDraft ? (
                  <Suspense fallback={<WorkbenchLoadingFallback className="min-w-0 flex-1" />}>
                    <SddDraftEditorView
                      leftSidebarCollapsed={leftSidebarCollapsed}
                      assistantOpen={rightPanelMode === 'sdd-ai'}
                      onToggleAssistant={() => void toggleSddAssistantPanel()}
                      onAssistantQuote={quoteToSddAssistant}
                      onPrototypeTurn={sendSddPrototypeTurn}
                      onNext={() => void handleSddNextStep()}
                      onClose={() => dismissActiveSddDraft({ closeAssistant: true })}
                      nextDisabled={busy || runtimeConnection !== 'ready' || sddDraftOperationStatus === 'upgrading'}
                    />
                  </Suspense>
                ) : activeDataAnalysis ? (
                  <Suspense fallback={<WorkbenchLoadingFallback className="min-w-0 flex-1" />}>
                    <DataAnalysisSurface
                      workspaceRoot={activeDataAnalysis.workspaceRoot}
                      activeItemId={activeDataAnalysis.itemId}
                      onNavigate={(itemId) => {
                        setActiveDataAnalysis((current) => current
                          ? { ...current, itemId }
                          : { workspaceRoot: workspaceRoot || activeDataAnalysis.workspaceRoot, itemId })
                      }}
                    />
                  </Suspense>
                ) : (
                  <section
                    className="ds-chat-stage ds-no-drag relative flex min-h-0 min-w-0 flex-1 flex-col"
                    data-bottom-panel-open={workbenchScrollReserve.bottomPanelOpen ? 'true' : 'false'}
                    style={chatStageStyle}
                  >
                    <header className="chat-topbar ds-chat-shell-header ds-topbar-surface relative z-50 flex min-h-[46px] w-full shrink-0 items-stretch overflow-visible">
                      <div aria-hidden="true" className="chat-topbar-drag-region" />
                      <div className="chat-topbar-grid grid w-full min-w-0 items-center gap-2.5 px-3 py-2 sm:px-4 md:pl-5 md:pr-2">
                        <div
                          className={`chat-topbar-session ds-shell-controls-safe-motion flex min-w-0 items-center gap-2.5 ${
                            leftSidebarCollapsed ? 'ds-shell-controls-safe-inset' : ''
                          }`}
                        >
                          <SessionHeader compact className="min-w-0 flex-1" onOpenSideChat={openSideChat} />
                        </div>
                        <div className="chat-topbar-actions flex min-w-0 flex-nowrap items-center justify-end gap-1.5 self-center">
                          {busy ? (
                            <span className="inline-flex shrink-0 rounded-full bg-amber-500/16 px-2.5 py-1 text-[11.5px] font-semibold text-amber-950 dark:text-amber-100">
                              {t('running')}
                            </span>
                          ) : null}
                          <WorkbenchTopBar workspaceOpen={rightPanelVisible}
                            onToggleWorkspace={useWorkspaceTabsStore.getState().toggleOpen}
                            terminalOpen={terminalOpen} onToggleTerminal={toggleTerminal} />
                        </div>
                      </div>
                    </header>
                    <div className={`${stageInsetClass} flex min-h-0 min-w-0 flex-1 flex-col`}>
                      <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
                        <ChatTimelineIsland
                          activeThreadId={activeThreadId}
                          onReturnToBottomStateChange={setTimelineReturnToBottomState}
                          runtimeConnection={runtimeConnection}
                          runtimeError={error}
                          onRetryConnection={() => void probeRuntime('user', { restart: true })}
                          onOpenSettings={() => openSettings('agents')}
                          onSelectSuggestion={(text) => setInput(text)}
                          focusModeEnabled={focusModeEnabled}
                          planActionsBusy={busy}
                          onBuildPlan={() => void buildGuiPlan()}
                          onOpenPlan={openGuiPlanPanel}
                          devPreviewCard={
                            showDevPreviewCard ? (
                              <DevPreviewLaunchCard
                                url={latestDevPreviewUrl}
                                opened={rightPanelMode === 'browser'}
                                onOpen={openDevPreview}
                              />
                            ) : null
                          }
                        />
                        {uiModeCameosEnabled && !focusModeEnabled ? <MascotCameoLayer /> : null}
                        {!focusModeEnabled ? <CameoCelebrationLayer active={busy} suppressed={Boolean(error)} /> : null}
                      </div>
                      <div className="ds-no-drag flex shrink-0 justify-center px-2 pb-3 pt-0 sm:px-4 md:px-6 lg:px-8">
                        <div
                          className={`ds-composer-return-to-bottom-shell relative flex w-full max-w-4xl flex-col `}
                        >
                          <div data-analytix-return-to-bottom-anchor className="relative h-0">
                            <ComposerReturnToBottomButton state={timelineReturnToBottomState} />
                          </div>
                          {nativeReferences.some(r => r.threadId === activeThreadId) ? <div className="mb-2 flex flex-wrap gap-2" aria-label="原生文档引用">
                            {nativeReferences.filter(r => r.threadId === activeThreadId).map(reference => <div key={reference.id} className="flex items-center gap-2 rounded-lg border border-ds-border px-2 py-1 text-xs">
                              <button type="button" title={[reference.note, reference.text].filter(Boolean).join('\n\n')} onClick={() => useWorkspaceTabsStore.getState().openTab({id:workspaceObjectTabId(reference.workspace, reference.path), kind:'document', mode:'documents', title:reference.path.split(/[\\/]/).at(-1) ?? reference.path, path:reference.path, workspaceRoot:reference.workspace})}>{reference.label}{reference.selection.capture?.truncated ? ' · 部分引用' : ''}{reference.editable === false ? ' · 仅讨论' : ''}{reference.note ? <span className="ml-1 text-ds-muted">· {reference.note.slice(0, 48)}{reference.note.length > 48 ? '…' : ''}</span> : null}</button>
                              <button type="button" aria-label={`移除 ${reference.label}`} onClick={() => useNativeReferenceStore.getState().remove(reference.id)}>×</button>
                            </div>)}
                          </div> : null}
                          {documentQuotes.length ? (
                            <div className="mb-2 flex flex-wrap gap-2" aria-label={t('workbenchReferences')}>
                              {documentQuotes.map((quote) => (
                                <div key={quote.id} className="flex items-center gap-2 rounded-lg border border-ds-border px-2 py-1 text-xs">
                                  <button type="button" onClick={() => setRightPanelMode('documents')} title={quote.text}>
                                    {quote.sourceTitle} · {quote.lineStart ?? quote.pageStart ?? ''}–{quote.lineEnd ?? quote.pageEnd ?? ''}
                                  </button>
                                  <button type="button" aria-label={t('writeRemoveQuote')} onClick={() => useWriteWorkspaceStore.getState().removeQuotedSelection(quote.id)}>×</button>
                                </div>
                              ))}
                            </div>
                          ) : null}
                          <FloatingComposerIsland
                            activeSkillWorkspace={activeSkillWorkspace}
                            className="max-w-none"
                            input={composerViewState.input}
                            setInput={composerController.setInput}
                            mode={composerViewState.mode}
                            setMode={composerController.setMode}
                            busy={composerViewState.busy}
                            runtimeReady={composerViewState.runtimeReady}
                            hasActiveThread={composerViewState.hasActiveThread}
                            contextWindowTokens={composerViewState.contextWindowTokens}
                            runtimeToolCount={composerViewState.runtimeToolCount}
                            runtimeSkillCount={composerViewState.runtimeSkillCount}
                            composerModel={composerViewState.composerModel}
                            composerProviderId={composerViewState.composerProviderId}
                            composerPickList={composerPickList}
                            composerModelGroups={composerModelGroups}
                            composerReasoningEffort={composerViewState.composerReasoningEffort}
                            onComposerModelChange={composerController.setModel}
                            onComposerReasoningEffortChange={composerController.setReasoningEffort}
                            onConfigureProviders={() => openSettings('providers')}
                            onOpenPermissionSettings={() => openSettings('permissions')}
                            onSend={composerController.send}
                            documentReferenceCount={documentQuotes.length + nativeReferences.filter(reference => reference.threadId === activeThreadId).length}
                            attachments={composerAttachments}
                            attachmentUploadEnabled={attachmentUploadEnabled}
                            attachmentUploadBusy={attachmentUploadBusy}
                            attachmentUploadError={attachmentUploadError}
                            fileReferenceEnabled={route === 'chat' && !activeSddDraft && !activeDataAnalysis}
                            fileReferences={composerFileReferences}
                            webAccessAvailable={webAccessAvailable}
                            executionSettings={composerExecutionSettings}
                            executionSettingsApplying={composerExecutionApplying}
                            skillCommands={runtimeSkills}
                            disabledSkillIds={disabledSkillIds}
                            onPickAttachments={(files) => void handlePickAttachments(files)}
                            onPasteClipboardImage={(options) => void handlePasteClipboardImage(options)}
                            onRemoveAttachment={removeComposerAttachment}
                            onAddFileReference={addComposerFileReference}
                            onPickFileReferences={() => void pickComposerFileReferences()}
                            onOpenFileReferencePicker={openFileTreeSidePanel}
                            onRemoveFileReference={removeComposerFileReference}
                            queuedMessages={queuedMessages}
                            onRemoveQueuedMessage={removeQueuedMessage}
                            onInterrupt={composerController.interrupt}
                            onPlanCommand={() => void handleGuiPlanCommand()}
                            useWorktreePool={useWorktreePool || Boolean(activeThreadWorktreeRecord)}
                            onToggleWorktreeMode={handleToggleWorktreeMode}
                            onNewCommand={() => {
                              const usePool = useWorktreePool
                              setUseWorktreePool(false)
                              void createThread({
                                workspaceRoot: activeSkillWorkspace,
                                forceNew: true,
                                ...(usePool ? { useWorktreePool: true } : {})
                              })
                            }}
                            onReviewCommand={(target) => void reviewActiveThread(target)}
                            onExecutionSettingsChange={updateComposerExecutionSettings}
                            onOpenChanges={() => setRightPanelMode('changes')}
                            onReviewChanges={() => void reviewActiveThread({ kind: 'uncommittedChanges' })}
                            reviewChangesDisabled={busy || runtimeConnection !== 'ready'}
                            onBtwCommand={(seedText) => {
                              if (seedText?.trim()) {
                                void createSideChatInInspector(seedText)
                                return
                              }
                              openSideChat()
                            }}
                          />
                        </div>
                      </div>
                    </div>
                    {terminalPanelMotion.isMounted ? (
                      <div
                        className="ds-terminal-panel-shell ds-no-drag"
                        data-open={terminalOpen ? 'true' : 'false'}
                        data-resizing={terminalResizing ? 'true' : 'false'}
                        style={{
                          height: terminalPanelMotion.animatedSize,
                          opacity: terminalPanelMotion.opacity
                        }}
                      >
                        {terminalOpen ? (
                          <WorkbenchResizeHandle
                            edge="top"
                            isResizing={terminalResizing}
                            onPointerDown={beginTerminalResize}
                            onReset={resetTerminalHeight}
                          />
                        ) : null}
                        <div className="ds-terminal-panel-clip">
                          <div
                            className="ds-terminal-panel-content"
                            style={{ height: terminalHeight }}
                          >
                            <Suspense fallback={<WorkbenchLoadingFallback surface="surface" />}>
                              <TerminalPanel
                                workspaceRoot={workspaceRoot}
                                height={terminalHeight}
                                className="w-full"
                                onCollapse={toggleTerminal}
                              />
                            </Suspense>
                          </div>
                        </div>
                      </div>
                    ) : null}
                  </section>
                )}
              </div>

              {route === 'chat' && !activeSddDraft && !activeDataAnalysis ? (
                <SideConversationPanel
                  rightOffset={(rightPanelDockedVisible ? rightSidebarWidth + 24 : 24) + fileTreeSidePanelOffset}
                />
              ) : null}

              {renderRightPanel()}
            </div>
          </>
        )}
        {renderPlanPanelOverlay()}
      </WorkbenchStage>
    </WorkbenchShell>
  )
}
