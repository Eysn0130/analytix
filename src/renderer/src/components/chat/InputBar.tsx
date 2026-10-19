/* Selected presentation and resident-input composition adapted from DeepSeek Harness
 * 5badb15009ae1756c3afe0ae0cef1faafc290ccc; Copyright (c) 2026 DeepSeek.
 * MIT; see THIRD_PARTY_NOTICES.md and the presentation/provenance ledgers.
 * Analytix retains its ProseMirror editor, draft/submit owners and protected Gate contracts. */
import type { ConversationPresentationPhase } from './ConversationWidthControls'
import { GateResponseContext, MessageBubble } from './message-timeline-bubbles'
import {
  useEffect,
  useCallback,
  useLayoutEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type ClipboardEvent as ReactClipboardEvent,
  type DragEvent as ReactDragEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type ReactElement
} from 'react'
import {
  Archive,
  ArrowUp,
  Download,
  FileEdit,
  FileText,
  Folder,
  GitBranch,
  GitFork,
  ImagePlus,
  ListTodo,
  Loader2,
  MessageCircleMore,
  Mic,
  Minimize2,
  Paperclip,
  PauseCircle,
  Pencil,
  Plug,
  Plus,
  PlayCircle,
  RotateCcw,
  Search,
  SearchCode,
  ShieldCheck,
  Sparkles,
  StopIcon as Square,
  Target,
  Trash2,
  X
} from '../../design/AnalytixUiIcons'
import { OwnedSessionStatsPills } from './presentation/chat/TurnUsagePanel'
import dockCss from './InputBarDock.module.css'
import { createPortal } from 'react-dom'
import { useAnchoredPanel } from './presentation/useAnchoredPanel'
import panelCss from './presentation/AnchoredPanel.module.css'
import { ComposerReferenceChip } from './ComposerReferenceChip'
import { ComposerDropOverlay } from './ComposerDropOverlay'
import { useTranslation } from 'react-i18next'
import type { HubAgentPluginListItem, ModelProviderModelGroup } from '@shared/analytix-api'
import type { AttachmentReference, ChatBlock, ReviewTarget } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import { normalizeWorkspaceRoot } from '../../lib/workspace-path'
import {
  composerFileReferenceFromPath,
  filterWorkspaceFileMentionSuggestions,
  formatComposerFileMentionToken,
  isComposerDirectoryReference,
  removeComposerFileMentionToken,
  type ComposerFileReference
} from '../../lib/composer-file-references'
import {
  extractComposerPluginMentions,
  extractComposerSkillMentions,
  formatComposerPluginMentionToken,
  formatComposerSkillMentionToken,
  getComposerAtMentionAtCursor,
  replaceComposerAtMentionInInput,
  type ComposerAtMention,
  type ComposerPluginMentionToken,
  type ComposerSkillMentionToken
} from '../../lib/composer-mentions'
import {
  loadWorkspaceFileIndex,
  loadWorkspaceMentionPathSuggestions,
  mergeMentionCandidates
} from '../../lib/workspace-file-index'
import {
  COMPACT_COMMAND_ALIASES,
  buildResearchPrompt,
  getGoalPanelDraftObjective,
  getComposerCommandRows,
  getSlashQuery,
  NEW_COMMAND_ALIASES,
  parseBtwCommand,
  parseCompactCommand,
  parseGoalCommand,
  parseNewCommand,
  parseResearchCommand,
  parseReviewCommand,
  RESEARCH_COMMAND_ALIASES,
  REVIEW_COMMAND_ALIASES,
  type SlashCommand,
  type SlashCommandId
} from './floating-composer-commands'
import { FloatingComposerCommandMenu } from './FloatingComposerCommandMenu'
export { buildResearchPrompt, parseBtwCommand, parseCompactCommand, parseGoalCommand, parseNewCommand, parseResearchCommand, parseReviewCommand } from './floating-composer-commands'
import { useThreadUsageState } from '../../hooks/use-thread-usage'
import { buildContextCapacity } from '../../lib/context-capacity'
import { ContextCapacityPopover } from './ContextCapacityPopover'
import { GitBranchPicker } from './GitBranchPicker'
import { WorkspaceProjectPicker } from './WorkspaceProjectPicker'
import {
  FloatingComposerModelPicker,
  type ComposerReasoningEffort
} from './FloatingComposerModelPicker'
import {
  FloatingComposerExecutionPicker,
  type ComposerExecutionSettings
} from './FloatingComposerExecutionPicker'
import type { QueuedUserMessage } from '../../store/chat-store-types'
import { AboveComposerPanelStack } from './above-composer/AboveComposerPanel'
import { ComposerInProgressStatusPill } from './above-composer/ComposerInProgressStatusPill'
import { QueuedUserMessages } from './above-composer/QueuedUserMessages'
import { ComposerGoalRow, goalRoundLimitExhausted, shouldPauseGoal } from './above-composer/ComposerGoalRow'
import {
  ThreadHandoffInlineProgress,
  ThreadHandoffProgressModal
} from './above-composer/ThreadHandoffProgressModal'
import { ImagePreviewLightbox } from './ImagePreviewLightbox'
import { useComposerDraft } from './use-composer-draft'
import { useSpeechToTextEnabled, useVoiceDictation } from './use-voice-dictation'
import { VoiceRecordingStrip } from './VoiceRecordingStrip'
import {
  collectComposerChangeSummary,
  collectCurrentTurnDiffSummary,
  type ComposerChangedFile
} from '../../lib/composer-change-summary'
import { ComposerPromptEditor, type ComposerPromptEditorHandle } from './ComposerPromptEditor'
import type { ComposerPromptMentionMetadata } from '../../lib/composer-prompt-document'

import type { ComposerAttachmentUpload } from '../../store/composer-drafts'

export type { ComposerFileReference } from '../../lib/composer-file-references'
export type { ComposerExecutionSettings } from './FloatingComposerExecutionPicker'

const CONTEXT_CAPACITY_RING_SIZE = 14
const CONTEXT_CAPACITY_RING_STROKE = 2
const CONTEXT_CAPACITY_RING_RADIUS = 5.5

type Props = {
  gateOverride?: { threadId?: string | null; block: Extract<ChatBlock, { kind: 'approval' | 'user_input' }> | null; onOpenOwnerThread?: (requestId: string) => void }
  variant?: 'default' | 'compact'
  /** Secondary composers keep their own thread and submission callbacks. */
  ownerThreadId?: string | null
  allowBusyFollowUp?: boolean
  voiceDictationEnabled?: boolean
  dictationOwnerKey?: string
  presentationPhase?: ConversationPresentationPhase
  className?: string
  workspaceRootOverride?: string
  activeSkillWorkspace?: string
  input: string
  setInput: (v: string) => void
  mode: 'plan' | 'agent'
  setMode: (m: 'plan' | 'agent') => void
  busy: boolean
  runtimeReady: boolean
  hasActiveThread: boolean
  composerModel: string
  composerProviderId?: string
  composerPickList: string[]
  composerModelGroups?: ModelProviderModelGroup[]
  composerReasoningEffort?: string
  onComposerModelChange: (modelId: string, providerId?: string) => void
  onComposerReasoningEffortChange?: (effort: ComposerReasoningEffort) => void
  onConfigureProviders?: () => void
  onReloadComposerModels?: () => Promise<void>
  onOpenPermissionSettings?: () => void
  hideModelPicker?: boolean
  modelPickerMode?: 'select' | 'combobox'
  hideThreadContextPanels?: boolean
  queuedMessages: QueuedUserMessage[]
  onRemoveQueuedMessage: (threadId: string, id: string) => void
  documentReferenceCount?: number
  attachments?: AttachmentReference[]
  attachmentUploadEnabled?: boolean
  attachmentUploadBusy?: boolean
  attachmentUploadError?: string | null
  attachmentUploads?: readonly ComposerAttachmentUpload[]
  onRetryAttachmentUpload?: (id: string) => void
  onRemoveAttachmentUpload?: (id: string) => void
  fileReferenceEnabled?: boolean
  fileReferences?: ComposerFileReference[]
  webAccessAvailable?: boolean
  executionSettings?: ComposerExecutionSettings | null
  executionSettingsApplying?: boolean
  executionSettingsLegacy?: boolean
  executionSettingsUnavailable?: boolean
  changedFiles?: ComposerChangedFile[]
  changedFileStats?: { added: number; removed: number } | null
  skillCommands?: Array<{
    id: string
    name: string
    description?: string
    root?: string
    scope?: 'project' | 'global'
    legacy?: boolean
    triggers?: {
      commands?: string[]
      fileTypes?: string[]
      promptPatterns?: string[]
    }
  }>
  disabledSkillIds?: string[]
  onPickAttachments?: (files: File[]) => void
  onPasteClipboardImage?: (options?: { silentNoImage?: boolean }) => void | Promise<void>
  onRemoveAttachment?: (id: string) => void
  onAddFileReference?: (reference: ComposerFileReference) => void
  onPickFileReferences?: () => void
  onOpenFileReferencePicker?: () => void
  onRemoveFileReference?: (relativePath: string) => void
  onSend: (intent?: 'queue' | 'steer') => void
  onInterrupt: (options?: { discard?: boolean }) => void
  onPlanCommand?: () => void
  onNewCommand?: () => void
  /** Worktree parallel mode toggle (single-use per new conversation). */
  useWorktreePool?: boolean
  onToggleWorktreeMode?: () => void
  onReviewCommand?: (target: ReviewTarget) => void
  onExecutionSettingsChange?: (patch: Partial<ComposerExecutionSettings>) => void
  onOpenChanges?: () => void
  onReviewChanges?: () => void
  reviewChangesDisabled?: boolean
  /**
   * When set, the `/btw` slash command is offered. It is omitted from
   * side-conversation composers (non-goal: no nested `/btw`).
   */
  onBtwCommand?: (seedText?: string) => void
  /**
   * Hide the `/btw` slash entry (e.g. inside a side conversation).
   */
  hideBtwCommand?: boolean
  /** Selected model's window metadata, independent of historical input samples. */
  contextWindowTokens?: number
  /** Catalog metadata; these counts are not context-token measurements. */
  runtimeToolCount?: number
  runtimeSkillCount?: number
}

type SkillCommand = NonNullable<Props['skillCommands']>[number]

type ComposerMentionSuggestion =
  | {
      kind: 'plugin'
      key: string
      title: string
      description: string
      badge: string
      token: string
      plugin: HubAgentPluginListItem
      iconUrl?: string
      brandColor?: string
    }
  | {
      kind: 'skill'
      key: string
      title: string
      description: string
      badge: string
      token: string
      skill: SkillCommand
    }
  | {
      kind: 'file'
      key: string
      title: string
      description: string
      badge: string
      reference: ComposerFileReference
    }

type ComposerMentionSection = {
  key: 'plugins' | 'skills' | 'files'
  title: string
  items: ComposerMentionSuggestion[]
}

type ComposerPluginMentionChip = {
  mention: ComposerPluginMentionToken
  plugin?: HubAgentPluginListItem
  title: string
  description: string
  iconUrl?: string
  brandColor?: string
}

type ComposerSkillMentionChip = {
  mention: ComposerSkillMentionToken
  skill?: SkillCommand
  title: string
  description: string
}

const EMPTY_MODEL_GROUPS: ModelProviderModelGroup[] = []
const EMPTY_ATTACHMENTS: AttachmentReference[] = []
const EMPTY_FILE_REFERENCES: ComposerFileReference[] = []
const EMPTY_CHANGED_FILES: ComposerChangedFile[] = []
const EMPTY_SKILL_COMMANDS: SkillCommand[] = []
const EMPTY_HUB_AGENT_PLUGINS: HubAgentPluginListItem[] = []
const MAX_COMPOSER_AT_MENTION_ITEMS = 8


type ComposerTransferItem = {
  kind?: string
  type?: string
  getAsFile?: () => File | null
}

export type ComposerImageTransferSource = {
  files?: ArrayLike<File> | null
  items?: ArrayLike<ComposerTransferItem> | null
}

export type ComposerClipboardImageSource = ComposerImageTransferSource & {
  getData?: (format: string) => string
}

function ComposerImageAttachmentPreview({
  attachment,
  onRemoveAttachment
}: {
  attachment: AttachmentReference
  onRemoveAttachment?: (id: string) => void
}): ReactElement {
  const { t } = useTranslation('common')
  const [imagePreviewOpen, setImagePreviewOpen] = useState(false)
  const title = attachment.name || attachment.id
  const previewUrl = attachment.previewUrl ?? ''

  return (
    <span
      className="dsh-composer-image ds-no-drag relative block h-16 w-16 overflow-hidden rounded-lg border border-ds-border-muted bg-ds-card"
      title={title}
    >
      <button
        type="button"
        onClick={() => setImagePreviewOpen(true)}
        className="block h-full w-full cursor-zoom-in"
        aria-label={t('imagePreviewOpen', { name: title })}
        title={t('imagePreviewOpen', { name: title })}
      >
        <img
          src={previewUrl}
          alt={title}
          className="h-full w-full object-cover"
        />
      </button>
      {onRemoveAttachment ? (
        <button
          type="button"
          onClick={() => onRemoveAttachment(attachment.id)}
          className="dsh-composer-image-remove absolute right-1 top-1 flex h-[18px] w-[18px] items-center justify-center rounded-full bg-zinc-950 text-white transition hover:bg-zinc-800"
          aria-label={t('composerRemoveAttachment')}
          title={t('composerRemoveAttachment')}
        >
          <X className="h-3 w-3" strokeWidth={2.2} />
        </button>
      ) : null}
      <ImagePreviewLightbox
        open={imagePreviewOpen}
        src={previewUrl}
        alt={title}
        title={title}
        downloadHref={previewUrl}
        downloadName={title}
        onClose={() => setImagePreviewOpen(false)}
      />
    </span>
  )
}

function ComposerPrimarySendIcon({ className, mainChat }: { className?: string; mainChat: boolean }): ReactElement {
  if (!mainChat) return <ArrowUp size={20} className={className} aria-hidden />
  return (
    <svg viewBox="0 0 16 16" width="16" height="16" className={className} aria-hidden>
      <path d="M8.3125 0.980183C8.66767 1.0531 8.97902 1.20418 9.2627 1.43233C9.48724 1.61297 9.73029 1.85793 9.97949 2.10714L14.707 6.83468L13.293 8.24874L9 3.95577V15.0417H7V3.95577L2.70703 8.24874L1.29297 6.83468L6.02051 2.10714C6.26971 1.85793 6.51277 1.61297 6.7373 1.43233C6.97662 1.23986 7.28445 1.04402 7.6875 0.980183C7.8973 0.947006 8.1031 0.95516 8.3125 0.980183Z" fill="currentColor" />
    </svg>
  )
}

function ComposerPrimaryStopIcon({ className, mainChat }: { className?: string; mainChat: boolean }): ReactElement {
  if (!mainChat) return <Square size={20} fill="currentColor" className={className} aria-hidden />
  return (
    <svg viewBox="0 0 16 16" width="16" height="16" className={className} aria-hidden>
      <rect x="3" y="3" width="10" height="10" rx="3" fill="currentColor" />
    </svg>
  )
}

function arrayLikeValues<T>(value: ArrayLike<T> | null | undefined): T[] {
  if (!value) return []
  const out: T[] = []
  for (let index = 0; index < value.length; index += 1) {
    const item = value[index]
    if (item) out.push(item)
  }
  return out
}

function isImageMimeType(value: string | undefined): boolean {
  return value?.toLowerCase().startsWith('image/') === true
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

function isPdfFile(file: File): boolean {
  return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')
}

function comparablePath(path: string | undefined): string {
  return (path ?? '').replace(/\\/g, '/').replace(/\/+$/g, '').toLowerCase()
}

function isProjectSkillRoot(skillRoot: string | undefined, workspaceRoot: string): boolean {
  const root = comparablePath(skillRoot)
  const workspace = comparablePath(workspaceRoot)
  return Boolean(root && workspace && (root === workspace || root.startsWith(`${workspace}/`)))
}

function isProjectSkill(skill: { root?: string; scope?: 'project' | 'global' }, workspaceRoot: string): boolean {
  return skill.scope === 'project' || (skill.scope !== 'global' && isProjectSkillRoot(skill.root, workspaceRoot))
}

function normalizeSkillCommandId(id: string): string {
  return id.trim().replace(/^\/?skill:/i, '').trim()
}

function disabledSkillIdSet(ids: string[] | undefined): Set<string> {
  return new Set((ids ?? []).map(normalizeSkillCommandId).filter(Boolean))
}

function normalizeMentionSearchText(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase()
}

function scoreMentionTextCandidate(parts: Array<string | undefined>, query: string): number {
  const normalizedQuery = normalizeMentionSearchText(query)
  if (!normalizedQuery) return 1
  let best = 0
  for (const rawPart of parts) {
    const part = normalizeMentionSearchText(rawPart)
    if (!part) continue
    if (part === normalizedQuery) best = Math.max(best, 100)
    else if (part.startsWith(normalizedQuery)) best = Math.max(best, 78)
    else if (part.includes(` ${normalizedQuery}`) || part.includes(`/${normalizedQuery}`)) {
      best = Math.max(best, 58)
    } else if (part.includes(normalizedQuery)) {
      best = Math.max(best, 34)
    }
  }
  return best
}

function hubPluginMentionId(plugin: HubAgentPluginListItem): string {
  return plugin.pluginName.trim() || plugin.name.trim() || plugin.marketplaceName.trim()
}

function hubPluginMentionDisplayName(plugin: HubAgentPluginListItem): string {
  return plugin.displayName.trim() || plugin.pluginName.trim() || plugin.name.trim()
}

function hubPluginMentionDescription(plugin: HubAgentPluginListItem): string {
  return plugin.shortDescription.trim() || plugin.developerName.trim() || plugin.marketplaceName.trim()
}

function hubPluginMentionIconUrl(plugin: HubAgentPluginListItem): string | undefined {
  const icon = plugin.iconDataUrl?.trim() || plugin.iconUrl?.trim()
  return icon || undefined
}

function hubPluginMentionBrandColor(plugin: HubAgentPluginListItem): string | undefined {
  const color = plugin.brandColor?.trim()
  return color || undefined
}

function installedHubAgentPlugins(plugins: HubAgentPluginListItem[] | undefined): HubAgentPluginListItem[] {
  const seen = new Set<string>()
  const installed: HubAgentPluginListItem[] = []
  for (const plugin of plugins ?? []) {
    if (!plugin.installed && !plugin.requiredInstall) continue
    const id = hubPluginMentionId(plugin)
    const key = id.toLowerCase()
    if (!id || seen.has(key)) continue
    seen.add(key)
    installed.push(plugin)
  }
  return installed.sort((left, right) =>
    hubPluginMentionDisplayName(left).localeCompare(hubPluginMentionDisplayName(right))
  )
}

function normalizedImageFile(file: File, mimeTypeHint?: string): File | null {
  const mimeType = isImageMimeType(file.type)
    ? file.type
    : isImageMimeType(mimeTypeHint)
      ? mimeTypeHint
      : imageMimeTypeFromFileName(file.name)
  if (!mimeType) return null
  if (file.type === mimeType) return file
  return new File([file], file.name || 'image', {
    type: mimeType,
    lastModified: file.lastModified
  })
}

export function imageFilesFromTransfer(source: ComposerImageTransferSource | null | undefined): File[] {
  if (!source) return []
  const files: File[] = []
  const seen = new Set<File>()
  const addFile = (file: File | null | undefined, mimeTypeHint?: string): void => {
    if (!file || seen.has(file)) return
    seen.add(file)
    const normalized = normalizedImageFile(file, mimeTypeHint)
    if (normalized) files.push(normalized)
  }

  for (const item of arrayLikeValues(source.items)) {
    if (item.kind && item.kind !== 'file') continue
    if (!isImageMimeType(item.type)) continue
    addFile(item.getAsFile?.(), item.type)
  }
  for (const file of arrayLikeValues(source.files)) {
    addFile(file)
  }
  return files
}

export function imageTransferHasImages(source: ComposerImageTransferSource | null | undefined): boolean {
  if (!source) return false
  if (arrayLikeValues(source.files).some((file) => normalizedImageFile(file) !== null)) return true
  return arrayLikeValues(source.items).some((item) =>
    (!item.kind || item.kind === 'file') && isImageMimeType(item.type)
  )
}

export function handleComposerImagePaste({
  canPickAttachment,
  clipboardData,
  preventDefault,
  onPickAttachments,
  onPasteClipboardImage
}: {
  canPickAttachment: boolean
  clipboardData: ComposerClipboardImageSource
  preventDefault: () => void
  onPickAttachments?: (files: File[]) => void
  onPasteClipboardImage?: (options?: { silentNoImage?: boolean }) => void | Promise<void>
}): boolean {
  if (!canPickAttachment || (!onPickAttachments && !onPasteClipboardImage)) return false
  const files = imageFilesFromTransfer(clipboardData)
  const hasPlainText = Boolean(clipboardData.getData?.('text/plain'))
  const hasImageTransfer = imageTransferHasImages(clipboardData)
  if (files.length > 0) {
    // Leave mixed clipboard text to the editor's native paste transaction and history.
    if (!hasPlainText) preventDefault()
    if (onPasteClipboardImage) {
      void onPasteClipboardImage({ silentNoImage: false })
      return !hasPlainText
    }
    onPickAttachments?.(files)
    return !hasPlainText
  }
  if (!onPasteClipboardImage) return false

  const shouldPreventDefault = !hasPlainText
  if (shouldPreventDefault) preventDefault()
  void onPasteClipboardImage({ silentNoImage: hasPlainText && !hasImageTransfer })
  return shouldPreventDefault
}

export function formatGoalElapsedSeconds(seconds: number): string {
  const value = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0))
  if (value < 60) return `${value}s`
  const minutes = Math.floor(value / 60)
  const remainingSeconds = value % 60
  if (value < 3600) {
    return remainingSeconds === 0
      ? `${minutes}m`
      : `${minutes}m ${remainingSeconds}s`
  }
  const hours = Math.floor(value / 3600)
  const remainingMinutes = Math.floor((value % 3600) / 60)
  return remainingMinutes === 0
    ? `${hours}h`
    : `${hours}h ${remainingMinutes}m`
}

export function shouldShowGoalFloater({
  compact,
  hasActiveGoal,
  slashQuery,
  goalPanelOpen,
  composerMenuOpen
}: {
  compact: boolean
  hasActiveGoal: boolean
  slashQuery: string | null
  goalPanelOpen: boolean
  composerMenuOpen: boolean
}): boolean {
  return !compact && hasActiveGoal && slashQuery == null && !goalPanelOpen && !composerMenuOpen
}

export function InputBar({
  gateOverride,
  variant = 'default',
  ownerThreadId,
  allowBusyFollowUp = true,
  voiceDictationEnabled,
  dictationOwnerKey,
  presentationPhase,
  className,
  workspaceRootOverride,
  activeSkillWorkspace,
  input,
  setInput,
  mode,
  setMode,
  busy,
  runtimeReady,
  hasActiveThread,
  composerModel,
  composerProviderId,
  composerPickList,
  composerModelGroups = EMPTY_MODEL_GROUPS,
  composerReasoningEffort,
  onComposerModelChange,
  onComposerReasoningEffortChange,
  onConfigureProviders,
  onReloadComposerModels,
  onOpenPermissionSettings,
  hideModelPicker = false,
  modelPickerMode = 'select',
  hideThreadContextPanels = false,
  queuedMessages,
  onRemoveQueuedMessage,
  documentReferenceCount = 0,
  attachments = EMPTY_ATTACHMENTS,
  attachmentUploadEnabled = false,
  attachmentUploadBusy = false,
  attachmentUploadError = null,
  attachmentUploads = [],
  onRetryAttachmentUpload,
  onRemoveAttachmentUpload,
  fileReferenceEnabled = false,
  fileReferences = EMPTY_FILE_REFERENCES,
  executionSettings = null,
  executionSettingsApplying = false,
  executionSettingsLegacy = false,
  executionSettingsUnavailable = false,
  changedFiles: suppliedChangedFiles,
  changedFileStats: suppliedChangedFileStats,
  skillCommands = EMPTY_SKILL_COMMANDS,
  disabledSkillIds,
  onPickAttachments,
  onPasteClipboardImage,
  onRemoveAttachment,
  onAddFileReference,
  onPickFileReferences,
  onOpenFileReferencePicker,
  onRemoveFileReference,
  onSend,
  onInterrupt,
  onPlanCommand,
  onNewCommand,
  useWorktreePool = false,
  onToggleWorktreeMode,
  onReviewCommand,
  onExecutionSettingsChange,
  onOpenChanges,
  onReviewChanges,
  reviewChangesDisabled = false,
  onBtwCommand,
  hideBtwCommand = false,
  contextWindowTokens
}: Props): ReactElement {
  const { t, i18n } = useTranslation('common')
  const route = useChatStore((s) => s.route)
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const primaryThreadId = useChatStore((s) => s.activeThreadId)
  const secondaryOwner = ownerThreadId !== undefined
  const activeThreadId = secondaryOwner ? ownerThreadId ?? null : primaryThreadId
  const usageRefreshKey = useChatStore((s) => s.usageRefreshKey)
  const rewoundTurnIds = useChatStore((s) => s.rewoundTurnIds)
  const lastTurnUsage = useChatStore((s) => s.lastTurnUsage)
  const threads = useChatStore((s) => s.threads)
  const compactActiveThread = useChatStore((s) => s.compactActiveThread)
  const forkActiveThread = useChatStore((s) => s.forkActiveThread)
  const archiveThread = useChatStore((s) => s.archiveThread)
  const activeThreadGoal = useChatStore((s) => s.activeThreadGoal)
  const activeGoalActivation = useChatStore((s) => s.activeGoalActivation)
  const setActiveThreadGoal = useChatStore((s) => s.setActiveThreadGoal)
  const setActiveThreadGoalStatus = useChatStore((s) => s.setActiveThreadGoalStatus)
  const clearActiveThreadGoal = useChatStore((s) => s.clearActiveThreadGoal)
  const activeThreadTodos = useChatStore((s) => s.activeThreadTodos)
  const currentTurnId = useChatStore((s) => s.currentTurnId)
  const primaryGate = useChatStore(s => s.blocks.find((block): block is Extract<ChatBlock, { kind: 'approval' | 'user_input' }> =>
    (block.kind === 'approval' || block.kind === 'user_input') && block.status === 'pending') ?? null)
  const [questionLeaves, setQuestionLeaves] = useState<{ threadId: string | null; ids: string[] }>({ threadId: null, ids: [] })
  const priorQuestionId = questionLeaves.threadId === activeThreadId ? questionLeaves.ids.at(-1) : undefined
  const selectedInput = useChatStore(s => s.userInputPanel?.threadId === s.activeThreadId
    ? s.blocks.find((block): block is Extract<ChatBlock, { kind: 'user_input' }> => block.kind === 'user_input' &&
      (block.status === 'pending' || block.status === 'continued' || s.userInputPanel!.view === 'review') && s.userInputPanel!.requestId === block.requestId) ?? null
    : s.blocks.find((block): block is Extract<ChatBlock, { kind: 'user_input' }> => block.kind === 'user_input' && block.status === 'pending') ??
      s.blocks.find((block): block is Extract<ChatBlock, { kind: 'user_input' }> => block.kind === 'user_input' && block.id === priorQuestionId && block.status === 'continued') ?? null)
  const composerGate = gateOverride === undefined ? primaryGate?.kind === 'approval' ? primaryGate : selectedInput : gateOverride.block
  const hasPendingBlockingRequest = Boolean(gateOverride === undefined ? primaryGate : gateOverride.block?.status === 'pending')
  const currentTurnDiffBlocks = useChatStore((s) => s.blocks)
  // Derive history changes at the input owner, preserving Workbench's stream isolation.
  const composerChangeSummary = useMemo(
    () => activeSkillWorkspace ? collectComposerChangeSummary(currentTurnDiffBlocks, activeSkillWorkspace) : null,
    [activeSkillWorkspace, currentTurnDiffBlocks]
  )
  const changedFiles = suppliedChangedFiles ?? composerChangeSummary?.files ?? EMPTY_CHANGED_FILES
  const changedFileStats = suppliedChangedFileStats ?? composerChangeSummary

  const retainedQuestionIds = questionLeaves.threadId === activeThreadId ? questionLeaves.ids : []
  const presentedQuestionId = gateOverride === undefined && composerGate?.kind === 'user_input' ? composerGate.id : null
  const questionIds = presentedQuestionId && !retainedQuestionIds.includes(presentedQuestionId)
    ? [...retainedQuestionIds, presentedQuestionId].slice(-32) : retainedQuestionIds
  const mountedQuestions = gateOverride === undefined ? currentTurnDiffBlocks.filter((block): block is Extract<ChatBlock, { kind: 'user_input' }> =>
    block.kind === 'user_input' && (['pending', 'continued'].includes(block.status) && questionIds.includes(block.id) || block.id === presentedQuestionId)) : []
  useLayoutEffect(() => {
    if (gateOverride !== undefined) return
    setQuestionLeaves(previous => {
      const ids = previous.threadId === activeThreadId ? previous.ids : []
      const next = presentedQuestionId && !ids.includes(presentedQuestionId) ? [...ids, presentedQuestionId].slice(-32) : ids
      return previous.threadId === activeThreadId && next === previous.ids ? previous : { threadId: activeThreadId, ids: next }
    })
  }, [activeThreadId, presentedQuestionId, gateOverride])
  const clawChannels = useChatStore((s) => s.clawChannels)
  const activeClawChannelId = useChatStore((s) => s.activeClawChannelId)
  const compact = variant === 'compact'
  const mainChatComposer = route === 'chat' && !compact && !secondaryOwner
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const speechToTextEnabled = useSpeechToTextEnabled()
  const dictationInputRef = useRef(input)
  useLayoutEffect(() => {
    dictationInputRef.current = input
  }, [input])
  const dictationPrimaryActionRef = useRef<(() => void) | null>(null)
  const dictation = useVoiceDictation({
    ownerKey: JSON.stringify([secondaryOwner, activeThreadId, dictationOwnerKey, voiceDictationEnabled]),
    onText: (text, intent, isCurrent) => {
      if (!isCurrent()) return
      const existing = dictationInputRef.current.replace(/\s+$/, '')
      setInput(existing ? `${existing} ${text}` : text)
      if (intent === 'send') {
        // 等 setInput 的重渲染落地后再走正常的发送路径,
        // 这样语音直发和手动点发送行为完全一致。
        window.setTimeout(() => { if (isCurrent()) dictationPrimaryActionRef.current?.() }, 0)
      }
    }
  })
  const showVoiceDictation = (voiceDictationEnabled ?? !secondaryOwner) && speechToTextEnabled
  const activeClawChannel = useMemo(
    () => clawChannels.find((channel) => channel.id === activeClawChannelId) ?? null,
    [activeClawChannelId, clawChannels]
  )
  const activeThreadWorkspace = activeThreadId
    ? threads.find((thread) => thread.id === activeThreadId)?.workspace
    : ''
  const activeThread = activeThreadId
    ? threads.find((thread) => thread.id === activeThreadId) ?? null
    : null
  const activeThreadArchived = activeThread?.archived === true
  const showThreadUsageFooter = !compact && presentationPhase !== 'hero' && route === 'chat' && Boolean(activeThreadId) && runtimeReady
  const threadUsageState = useThreadUsageState(
    activeThreadId,
    showThreadUsageFooter,
    `${activeThread?.updatedAt ?? ''}:${busy ? 'busy' : 'idle'}:${usageRefreshKey}`,
    `${runtimeReady}:${JSON.stringify(Object.keys(rewoundTurnIds ?? {}).sort())}`
  )
  const effectiveWorkspaceRoot = normalizeWorkspaceRoot(activeThreadWorkspace || workspaceRootOverride || workspaceRoot)
  const clawAgentName =
    activeClawChannel?.agentProfile.name.trim()
    || activeClawChannel?.label.trim()
    || t('clawEmptyHeroFallbackName')
  const clawHasInboundConversation = Boolean(
    activeThreadId ||
    activeClawChannel?.threadId.trim() ||
    activeClawChannel?.conversations.some((conversation) => conversation.localThreadId.trim()) ||
    activeClawChannel?.conversations.length ||
    activeClawChannel?.remoteSession?.chatId?.trim()
  )

  const canEditComposer = (!busy || allowBusyFollowUp) && (secondaryOwner || route !== 'claw' || clawHasInboundConversation)
  const canCompose = runtimeReady && (!busy || allowBusyFollowUp) && (
    !secondaryOwner && route === 'claw'
      ? clawHasInboundConversation
      : (hasActiveThread || !!effectiveWorkspaceRoot)
  )
  const canChangeModel = canCompose && !busy
  const executionPolicyBlocked = executionSettingsApplying || executionSettingsUnavailable
  const hasComposerPayload = (
    input.trim().length > 0 ||
    documentReferenceCount > 0 ||
    attachmentUploads.length > 0 ||
    (attachmentUploadEnabled && attachments.length > 0) ||
    (fileReferenceEnabled && fileReferences.length > 0)
  )
  const canSend = canCompose && !executionPolicyBlocked && !attachmentUploadBusy && attachmentUploads.length === 0 && hasComposerPayload
  const canPickAttachment = canCompose && attachmentUploadEnabled && !attachmentUploadBusy
  const canPickFileReference = canCompose && fileReferenceEnabled && Boolean(effectiveWorkspaceRoot) && Boolean(onOpenFileReferencePicker)
  const canPickLocalFileReference = canCompose && fileReferenceEnabled && Boolean(onPickFileReferences)
  const showIntentToolbar = !compact && route === 'chat'
  const showComposerMenuButton = showIntentToolbar
  const canTogglePlanMode = canCompose && Boolean(onPlanCommand)
  const canCreateNewThread = runtimeReady && route !== 'claw' && Boolean(effectiveWorkspaceRoot) && Boolean(onNewCommand)
  const canOpenGoalPanel = !secondaryOwner && canCompose && route !== 'claw'
  const canToggleWorktreeMode = canCompose && route !== 'claw' && Boolean(onToggleWorktreeMode)
  const canOpenComposerMenu = showComposerMenuButton
  const showToolbarStartControls = showComposerMenuButton
  const currentTurnDiffSummary = useMemo(
    () => collectCurrentTurnDiffSummary(currentTurnDiffBlocks, currentTurnId, effectiveWorkspaceRoot),
    [currentTurnDiffBlocks, currentTurnId, effectiveWorkspaceRoot]
  )
  const showInlineTurnDiffSummary =
    busy && currentTurnId != null && !hasPendingBlockingRequest && currentTurnDiffSummary != null
  const showChangeSummary =
    !compact && route === 'chat' && changedFiles.length > 0 && !showInlineTurnDiffSummary
  const effectiveChangedFileStats = changedFileStats ?? changedFiles.reduce(
    (stats, file) => ({
      added: stats.added + file.added,
      removed: stats.removed + file.removed
    }),
    { added: 0, removed: 0 }
  )
  const visibleChangedFiles = changedFiles.slice(0, 3)
  const hiddenChangedFileCount = Math.max(0, changedFiles.length - visibleChangedFiles.length)
  const stretchModelPicker =
    compact && modelPickerMode === 'combobox' && !showToolbarStartControls && !hideModelPicker
  const draft = useComposerDraft({ input, canCompose: canEditComposer })
  const composerEditorRef = useRef<ComposerPromptEditorHandle | null>(null)
  const [dismissedSlashKey, setDismissedSlashKey] = useState<string | null>(null)
  const slashOwnerKey = JSON.stringify([activeThreadId, effectiveWorkspaceRoot, input])
  const slashQuery = secondaryOwner || dismissedSlashKey === slashOwnerKey ? null : getSlashQuery(input)
  const [composerCursor, setComposerCursor] = useState(() => input.length)
  const [selectedCommandIndex, setSelectedCommandIndex] = useState(0)
  const [agentPlugins, setAgentPlugins] = useState<HubAgentPluginListItem[]>(EMPTY_HUB_AGENT_PLUGINS)
  const [agentPluginsLoading, setAgentPluginsLoading] = useState(false)
  const [fileMentionSuggestions, setFileMentionSuggestions] = useState<ComposerFileReference[]>([])
  const [fileMentionLoading, setFileMentionLoading] = useState(false)
  const [selectedMentionIndex, setSelectedMentionIndex] = useState(0)
  const [dismissedMentionKey, setDismissedMentionKey] = useState<string | null>(null)
  const composerMenuId = useId()
  const [composerMenuOpen, setComposerMenuOpen] = useState(false)
  const composerOwnerRevisionRef = useRef(0)
  const [composerOwnerRevision, setComposerOwnerRevision] = useState(0)
  const [composerPicker, setComposerPicker] = useState<'permission' | 'model' | null>(null)
  const [goalPanelOpen, setGoalPanelOpen] = useState(false)
  const [goalRuntimeNowMs, setGoalRuntimeNowMs] = useState(() => Date.now())
  const composerRootRef = useRef<HTMLDivElement | null>(null)
  const composerMenuButtonRef = useRef<HTMLButtonElement | null>(null)
  const composerMenuPanelRef = useRef<HTMLDivElement | null>(null)
  const goalPanelRef = useRef<HTMLDivElement | null>(null)
  const composerOwnerKey = JSON.stringify([activeThreadId, effectiveWorkspaceRoot, composerOwnerRevision])
  const contextOwnerKey = JSON.stringify([composerOwnerKey, composerProviderId ?? '', composerModel])
  const canShowContextCapacity =
    !compact && runtimeReady && route === 'chat' && Boolean(activeThreadId)
  // Recording replaces the trigger, so it must also retire its portaled panel.
  const voiceActivity = dictation.status !== 'idle'
  const capacityTriggerVisible = canShowContextCapacity && !voiceActivity
  const capacityPanel = useAnchoredPanel<HTMLDivElement>({
    enabled: capacityTriggerVisible, ownerKey: contextOwnerKey
  })
  const contextCapacityOpen = capacityPanel.open
  const contextCapacity = useMemo(() => buildContextCapacity({
    windowTokens: contextWindowTokens,
    model: composerModel,
    providerId: composerProviderId,
    threadId: activeThreadId,
    lastTurnUsage
  }), [contextWindowTokens, composerModel, composerProviderId, activeThreadId, lastTurnUsage])
  const goalRuntimeStartedAtRef = useRef<number | null>(null)
  const placeholder = !runtimeReady
    ? t('runtimeActionNeedsConnection')
    : !hasActiveThread && !effectiveWorkspaceRoot
      ? t('workspaceRequiredToCreateThread')
      : goalPanelOpen && route !== 'claw'
        ? t('goalComposerPlaceholder')
      : busy
        ? t('composerQueuePlaceholder')
        : route === 'claw'
            ? clawHasInboundConversation
              ? t('clawPlaceholder', { name: clawAgentName })
              : t('clawPlaceholderNeedsInbound')
            : mode === 'plan'
              ? t('composerPlanPlaceholder')
              : hasActiveThread && presentationPhase !== 'hero'
                ? t('placeholder')
                : t('composerStartsThread')
  const footerHint = !runtimeReady
    ? t('composerOfflineHint')
    : !hasActiveThread && !effectiveWorkspaceRoot
      ? t('composerWorkspaceHint')
      : route === 'claw'
          ? clawHasInboundConversation
            ? t('clawComposerHint')
            : t('clawComposerHintNeedsInbound')
          : useWorktreePool
            ? t('composerWorktreeModeHint')
            : null
  const slashCommands = useMemo<SlashCommand[]>(() => {
    const threadActionDisabled = !runtimeReady || busy || !activeThreadId
    const goalActionDisabled = !canOpenGoalPanel
    const disabledSkills = disabledSkillIdSet(disabledSkillIds)
    const commands: SlashCommand[] = []
    if (route !== 'claw') {
      commands.push(
        { id: 'file', title: t('slashCommandFileTitle'), description: t('slashCommandFileDescription'),
          keywords: ['file', 'files', '文件'], icon: <FileText />, disabled: !canPickLocalFileReference && !canPickFileReference },
        { id: 'feedback', title: t('slashCommandFeedbackTitle'), description: t('slashCommandFeedbackDescription'),
          keywords: ['feedback', '反馈'], icon: <MessageCircleMore />, disabled: true, disabledReason: t('slashCommandFeedbackUnavailable') },
        { id: 'permission', title: t('slashCommandPermissionTitle'), description: t('slashCommandPermissionDescription'),
          keywords: ['permission', 'permissions', 'approval', '权限'], icon: <ShieldCheck />,
          disabled: !runtimeReady || (!executionSettings && !executionSettingsUnavailable) || !onExecutionSettingsChange || executionSettingsApplying },
        { id: 'model', title: t('slashCommandModelTitle'), description: t('slashCommandModelDescription'),
          keywords: ['model', '模型'], icon: <Sparkles />, disabled: hideModelPicker || !canChangeModel },
        { id: 'export', title: t('slashCommandExportTitle'), description: t('slashCommandExportDescription'),
          keywords: ['export', 'download', 'logs', '下载', '日志'], icon: <Download />, disabled: true, disabledReason: t('slashCommandExportUnavailable') }
      )
      if (fileReferenceEnabled && canPickLocalFileReference) commands.push({ id: 'workspace-files',
        title: t('composerBrowseWorkspaceFiles'), description: t('slashCommandFileDescription'),
        keywords: ['workspace', 'files', '工作区'], icon: <Folder />, disabled: !canPickFileReference })
      if (attachmentUploadEnabled) commands.push({ id: 'attachments', title: t('composerAddImage'),
        description: t('composerAddImage'), keywords: ['image', 'attachment', 'pdf', '图片', '附件'],
        icon: attachmentUploadBusy ? <Loader2 className="animate-spin" /> : <ImagePlus />, disabled: !canPickAttachment || !onPickAttachments })
      if (onToggleWorktreeMode) commands.push({ id: 'worktree', title: t('composerMenuWorktreeMode'),
        description: t('composerWorktreeModeHint'), keywords: ['worktree', 'parallel', '工作树'],
        icon: <GitBranch />, disabled: !canToggleWorktreeMode, checked: useWorktreePool })
      commands.push({
        id: 'new',
        title: t('slashCommandNewTitle'),
        description: t('slashCommandNewDescription'),
        keywords: ['create', 'new', 'thread', 'chat', '会话', '新建', ...NEW_COMMAND_ALIASES],
        icon: <Plus className="h-4 w-4" strokeWidth={1.9} />,
        disabled: !canCreateNewThread
      })
      commands.push({
        id: 'research',
        title: t('slashCommandResearchTitle'),
        description: t('slashCommandResearchDescription'),
        keywords: ['research', 'deep', 'web', 'sources', 'papers', 'evidence', ...RESEARCH_COMMAND_ALIASES],
        icon: <Search className="h-4 w-4" strokeWidth={1.9} />,
        disabled: !runtimeReady
      })
    }
    if (onPlanCommand) {
      commands.push({
        id: 'plan',
        title: t('slashCommandPlanTitle'),
        description: t('slashCommandPlanDescription'),
        keywords: ['plan', 'planner', 'planning', '规划', '计划'],
        icon: <ListTodo className="h-4 w-4" strokeWidth={1.9} />,
        disabled: !canTogglePlanMode,
        checked: mode === 'plan'
      })
    }

    if (secondaryOwner) return commands.filter(command => ['file', 'workspace-files', 'attachments', 'plan', 'model', 'permission', 'worktree'].includes(command.id))

    if (route !== 'claw') {
      const dynamicSkillCommands = skillCommands
        .filter((skill) => skill.id.trim() && skill.name.trim())
        .filter((skill) => !disabledSkills.has(normalizeSkillCommandId(skill.id)))
        .sort((left, right) => {
          const leftProject = isProjectSkill(left, effectiveWorkspaceRoot)
          const rightProject = isProjectSkill(right, effectiveWorkspaceRoot)
          if (leftProject !== rightProject) return leftProject ? -1 : 1
          return left.name.localeCompare(right.name)
        })
        .slice(0, 40)
        .map<SlashCommand>((skill) => {
          const prompt = `/skill:${skill.id} `
          const scopeLabel = isProjectSkill(skill, effectiveWorkspaceRoot)
            ? t('slashSkillScopeProject')
            : t('slashSkillScopeGlobal')
          const triggers = [
            ...(skill.triggers?.commands ?? []),
            ...(skill.triggers?.fileTypes ?? []),
            ...(skill.triggers?.promptPatterns ?? [])
          ]
          return {
            id: `skill:${skill.id}`,
            kind: 'skill',
            title: skill.name,
            description: skill.description?.trim() || t('slashSkillDescriptionFallback'),
            keywords: [skill.id, skill.name, skill.root ?? '', scopeLabel, 'skill', '技能', ...triggers],
            icon: <Sparkles className="h-4 w-4" strokeWidth={1.9} />,
            badge: prompt.trim(),
            scopeLabel,
            skillPrompt: prompt,
            disabled: !runtimeReady
          }
        })
      commands.push(...dynamicSkillCommands)
      commands.push(...agentPlugins.slice(0, 40).map<SlashCommand>(plugin => ({
        id: `plugin:${hubPluginMentionId(plugin)}`, kind: 'plugin',
        title: hubPluginMentionDisplayName(plugin), description: plugin.shortDescription?.trim() || plugin.name,
        keywords: [hubPluginMentionId(plugin), plugin.name, 'plugin', '插件'],
        icon: <Plug />, disabled: !runtimeReady
      })))

      commands.push({
        id: 'goal',
        title: t('slashCommandGoalTitle'),
        description: t('slashCommandGoalDescription'),
        keywords: ['goal', 'objective', 'target', '目标', '任务'],
        icon: <Target className="h-4 w-4" strokeWidth={1.9} />,
        disabled: goalActionDisabled || activeThreadGoal?.status !== 'complete' && !shouldPauseGoal(activeThreadGoal, activeGoalActivation) && goalRoundLimitExhausted(activeThreadGoal),
        checked: shouldPauseGoal(activeThreadGoal, activeGoalActivation)
      })

      if (onBtwCommand && !hideBtwCommand) {
        // `/btw` is available even while the main thread is busy — the
        // point of the command is to run a parallel aside next to a
        // running task.
        commands.push({
          id: 'btw',
          title: t('slashCommandBtwTitle'),
          description: t('slashCommandBtwDescription'),
          keywords: ['btw', 'by-the-way', 'aside', 'side', '顺便', '旁支'],
          icon: <MessageCircleMore className="h-4 w-4" strokeWidth={1.9} />,
          disabled: !runtimeReady || !activeThreadId
        })
      }

      if (onReviewCommand) {
        commands.push({
          id: 'review',
          title: t('slashCommandReviewTitle'),
          description: t('slashCommandReviewDescription'),
          keywords: REVIEW_COMMAND_ALIASES,
          icon: <SearchCode className="h-4 w-4" strokeWidth={1.9} />,
          disabled: threadActionDisabled
        })
      }

      commands.push(
        {
          id: 'compact',
          title: t('slashCommandCompactTitle'),
          description: t('slashCommandCompactDescription'),
          keywords: COMPACT_COMMAND_ALIASES,
          icon: <Minimize2 className="h-4 w-4" strokeWidth={1.9} />,
          disabled: threadActionDisabled
        },
        {
          id: 'fork',
          title: t('slashCommandForkTitle'),
          description: t('slashCommandForkDescription'),
          keywords: ['fork', 'branch', 'copy', '分叉', '复制'],
          icon: <GitFork className="h-4 w-4" strokeWidth={1.9} />,
          disabled: threadActionDisabled
        }
      )

      if (activeThreadArchived) {
        commands.push({
          id: 'restore',
          title: t('slashCommandRestoreTitle'),
          description: t('slashCommandRestoreDescription'),
          keywords: ['restore', 'unarchive', '恢复'],
          icon: <RotateCcw className="h-4 w-4" strokeWidth={1.9} />,
          disabled: threadActionDisabled
        })
      } else {
        commands.push({
          id: 'archive',
          title: t('slashCommandArchiveTitle'),
          description: t('slashCommandArchiveDescription'),
          keywords: ['archive', 'hide', '归档'],
          icon: <Archive className="h-4 w-4" strokeWidth={1.9} />,
          disabled: threadActionDisabled
        })
      }
    }

    return commands
  }, [
    secondaryOwner,
    activeThreadArchived,
    activeThreadId,
    busy,
    canOpenGoalPanel,
    effectiveWorkspaceRoot,
    hideBtwCommand,
    onBtwCommand,
    canCreateNewThread,
    onPlanCommand,
    onReviewCommand,
    route,
    runtimeReady,
    skillCommands,
    agentPlugins,
    disabledSkillIds,
    canPickLocalFileReference, canPickFileReference, canPickAttachment, canTogglePlanMode, canToggleWorktreeMode,
    canChangeModel, hideModelPicker, executionSettings, executionSettingsApplying, executionSettingsUnavailable, onExecutionSettingsChange,
    fileReferenceEnabled, attachmentUploadEnabled, attachmentUploadBusy, onPickAttachments, onToggleWorktreeMode, useWorktreePool,
    mode, activeThreadGoal, activeGoalActivation,
    t
  ])

  const filteredSlashCommands = useMemo(() => {
    if (slashQuery == null) return []
    return getComposerCommandRows(slashCommands, slashQuery)
  }, [slashCommands, slashQuery])
  const composerMenuItems = useMemo(() => getComposerCommandRows(slashCommands, ''), [slashCommands])
  const commandGroupLabels = {
    add: t('composerCommandSectionAdd'), commands: t('composerCommandSectionCommands'),
    extensions: t('composerCommandSectionExtensions'), skills: t('composerCommandSectionSkills'),
    plugins: t('composerCommandSectionPlugins'), worktree: t('composerCommandSectionWorktree')
  }

  const highlightedSlashCommand =
    filteredSlashCommands.length > 0
      ? filteredSlashCommands[Math.min(selectedCommandIndex, filteredSlashCommands.length - 1)]
      : null
  const activeAtMention = useMemo<ComposerAtMention | null>(() => {
    if (slashQuery != null) return null
    return getComposerAtMentionAtCursor(input, composerCursor)
  }, [composerCursor, input, slashQuery])
  const activeAtMentionKey = activeAtMention
    ? `${activeAtMention.start}:${activeAtMention.query}:${activeAtMention.quoted ? 'q' : 'p'}`
    : null
  const composerPluginMentions = useMemo(
    () => extractComposerPluginMentions(input),
    [input]
  )
  const composerSkillMentions = useMemo(
    () => extractComposerSkillMentions(input),
    [input]
  )
  const mentionedPluginIds = useMemo(
    () => new Set(composerPluginMentions.map((mention) => mention.pluginId.trim().toLowerCase()).filter(Boolean)),
    [composerPluginMentions]
  )
  const mentionedSkillIds = useMemo(
    () => new Set(composerSkillMentions.map((mention) => normalizeSkillCommandId(mention.skillId).toLowerCase()).filter(Boolean)),
    [composerSkillMentions]
  )
  const installedAgentPluginsForMentions = useMemo(
    () => installedHubAgentPlugins(agentPlugins),
    [agentPlugins]
  )
  const disclosureKeysRef = useRef({ slash: slashOwnerKey, mention: activeAtMentionKey })
  useLayoutEffect(() => { disclosureKeysRef.current = { slash: slashOwnerKey, mention: activeAtMentionKey } })
  useEffect(() => {
    if (secondaryOwner) return
    return useChatStore.subscribe((next, previous) => {
      if (next.activeThreadId === previous.activeThreadId && next.workspaceRoot === previous.workspaceRoot && next.threads === previous.threads) return
      const workspaceFor = (state: typeof next) => normalizeWorkspaceRoot(
        state.threads.find(thread => thread.id === state.activeThreadId)?.workspace || workspaceRootOverride || state.workspaceRoot)
      if (next.activeThreadId === previous.activeThreadId && workspaceFor(next) === workspaceFor(previous)) return
      setComposerOwnerRevision(++composerOwnerRevisionRef.current)
      setComposerMenuOpen(false)
      setComposerPicker(null)
      setGoalPanelOpen(false)
      setDismissedSlashKey(disclosureKeysRef.current.slash)
      setDismissedMentionKey(disclosureKeysRef.current.mention)
      setFileMentionSuggestions([])
    })
  }, [secondaryOwner, workspaceRootOverride])

  const showAtMentionMenu =
    !secondaryOwner && canCompose &&
    Boolean(activeAtMention) &&
    activeAtMentionKey !== dismissedMentionKey &&
    !composerMenuOpen &&
    !goalPanelOpen
  const mentionSections = useMemo<ComposerMentionSection[]>(() => {
    if (!activeAtMention) return []
    const query = activeAtMention.query
    const queryIsEmpty = query.trim().length === 0
    const sections: ComposerMentionSection[] = []
    let remaining = MAX_COMPOSER_AT_MENTION_ITEMS
    const addSection = (section: ComposerMentionSection): void => {
      if (remaining <= 0 || section.items.length === 0) return
      const items = section.items.slice(0, remaining)
      if (items.length === 0) return
      remaining -= items.length
      sections.push({ ...section, items })
    }

    const pluginItems = installedAgentPluginsForMentions
      .map((plugin) => {
        const id = hubPluginMentionId(plugin)
        const title = hubPluginMentionDisplayName(plugin)
        const description = hubPluginMentionDescription(plugin)
        const score = scoreMentionTextCandidate(
          [
            id,
            title,
            description,
            plugin.marketplaceName,
            plugin.upstreamMarketplaceName,
            plugin.developerName,
            plugin.category
          ],
          query
        )
        return {
          plugin,
          id,
          title,
          description,
          score
        }
      })
      .filter((item) => item.id && item.score > 0 && !mentionedPluginIds.has(item.id.toLowerCase()))
      .sort((left, right) =>
        right.score - left.score ||
        left.title.localeCompare(right.title)
      )
      .slice(0, queryIsEmpty ? 4 : 3)
      .map<ComposerMentionSuggestion>((item) => ({
        kind: 'plugin',
        key: `plugin:${item.id}`,
        title: item.title,
        description: item.description || t('composerPluginMentionDescriptionFallback'),
        badge: `@${item.id}`,
        token: formatComposerPluginMentionToken(item.title, item.id),
        plugin: item.plugin,
        iconUrl: hubPluginMentionIconUrl(item.plugin),
        brandColor: hubPluginMentionBrandColor(item.plugin)
      }))
    addSection({
      key: 'plugins',
      title: t('composerMentionSectionPlugins'),
      items: pluginItems
    })

    const disabledSkills = disabledSkillIdSet(disabledSkillIds)
    const skillItems = queryIsEmpty
      ? []
      : skillCommands
          .map((skill) => {
            const id = normalizeSkillCommandId(skill.id)
            const scopeLabel = isProjectSkill(skill, effectiveWorkspaceRoot)
              ? t('slashSkillScopeProject')
              : t('slashSkillScopeGlobal')
            const triggers = [
              ...(skill.triggers?.commands ?? []),
              ...(skill.triggers?.fileTypes ?? []),
              ...(skill.triggers?.promptPatterns ?? [])
            ]
            const score = scoreMentionTextCandidate(
              [id, skill.name, skill.description, skill.root, scopeLabel, ...triggers],
              query
            )
            return { skill, id, scopeLabel, score }
          })
          .filter((item) =>
            item.id &&
            item.score > 0 &&
            !disabledSkills.has(item.id) &&
            !mentionedSkillIds.has(item.id.toLowerCase())
          )
          .sort((left, right) =>
            right.score - left.score ||
            left.skill.name.localeCompare(right.skill.name)
          )
          .slice(0, 2)
          .map<ComposerMentionSuggestion>((item) => ({
            kind: 'skill',
            key: `skill:${item.id}`,
            title: item.skill.name,
            description: item.skill.description?.trim() || t('slashSkillDescriptionFallback'),
            badge: item.scopeLabel,
            token: formatComposerSkillMentionToken(item.skill.name, item.id),
            skill: item.skill
          }))
    addSection({
      key: 'skills',
      title: t('composerMentionSectionSkills'),
      items: skillItems
    })

    const fileItems = fileMentionSuggestions
      .map<ComposerMentionSuggestion>((reference) => {
        const isDirectory = isComposerDirectoryReference(reference)
        return {
          kind: 'file',
          key: `file:${reference.type ?? 'file'}:${reference.relativePath}`,
          title: isDirectory ? `${reference.name}/` : reference.name,
          description: isDirectory ? `${reference.relativePath}/` : reference.relativePath,
          badge: formatComposerFileMentionToken(reference.relativePath, isDirectory),
          reference
        }
      })
    addSection({
      key: 'files',
      title: t('composerMentionSectionFiles'),
      items: fileItems
    })

    return sections
  }, [
    activeAtMention,
    disabledSkillIds,
    effectiveWorkspaceRoot,
    fileMentionSuggestions,
    installedAgentPluginsForMentions,
    mentionedPluginIds,
    mentionedSkillIds,
    skillCommands,
    t
  ])
  const flatMentionSuggestions = useMemo(
    () => mentionSections.flatMap((section) => section.items),
    [mentionSections]
  )
  const highlightedMention =
    flatMentionSuggestions.length > 0
      ? flatMentionSuggestions[Math.min(selectedMentionIndex, flatMentionSuggestions.length - 1)]
      : null
  const atMentionLoading = agentPluginsLoading || fileMentionLoading
  const pluginMentionChips = useMemo<ComposerPluginMentionChip[]>(() => {
    const pluginById = new Map(
      installedAgentPluginsForMentions.map((plugin) => [hubPluginMentionId(plugin).toLowerCase(), plugin])
    )
    return composerPluginMentions.map((mention) => {
      const plugin = pluginById.get(mention.pluginId.trim().toLowerCase())
      return {
        mention,
        plugin,
        title: plugin ? hubPluginMentionDisplayName(plugin) : mention.label || mention.pluginId,
        description: plugin ? hubPluginMentionDescription(plugin) : mention.pluginId,
        iconUrl: plugin ? hubPluginMentionIconUrl(plugin) : undefined,
        brandColor: plugin ? hubPluginMentionBrandColor(plugin) : undefined
      }
    })
  }, [composerPluginMentions, installedAgentPluginsForMentions])
  const skillMentionChips = useMemo<ComposerSkillMentionChip[]>(() => {
    const skillById = new Map(
      skillCommands.map((skill) => [normalizeSkillCommandId(skill.id).toLowerCase(), skill])
    )
    return composerSkillMentions.map((mention) => {
      const skill = skillById.get(normalizeSkillCommandId(mention.skillId).toLowerCase())
      return {
        mention,
        skill,
        title: skill?.name.trim() || mention.label || mention.skillId,
        description: skill?.description?.trim() || mention.skillId
      }
    })
  }, [composerSkillMentions, skillCommands])
  const composerPromptMentionMetadata = useMemo<ComposerPromptMentionMetadata>(() => ({
    plugins: pluginMentionChips.map((chip) => ({
      pluginId: chip.mention.pluginId,
      title: chip.title,
      description: chip.description,
      iconUrl: chip.iconUrl,
      brandColor: chip.brandColor
    })),
    skills: skillMentionChips.map((chip) => ({
      skillId: chip.mention.skillId,
      title: chip.title,
      description: chip.description
    }))
  }), [pluginMentionChips, skillMentionChips])
  const parsedGoalCommand = parseGoalCommand(input)
  const goalPanelDraftObjective = getGoalPanelDraftObjective(input, goalPanelOpen)
  const canSetGoalPanelDraft =
    route !== 'claw'
    && runtimeReady
    && canOpenGoalPanel
    && !executionPolicyBlocked
    && goalPanelDraftObjective.length > 0
  const primaryActionIsStop = busy && !highlightedSlashCommand && (!allowBusyFollowUp || !hasComposerPayload)
  const busyCanSendFollowup = busy && canSend
  const primaryActionLabel = highlightedSlashCommand
    ? t('slashCommandApply')
    : primaryActionIsStop
    ? t('interrupt')
    : busy
    ? t('composerSteerSend')
    : canSetGoalPanelDraft
      ? t('goalSetCurrentInput')
      : t('send')
  const primaryActionDisabled = highlightedSlashCommand
    ? highlightedSlashCommand.disabled === true
    : primaryActionIsStop
    ? false
    : busy
    ? !canSend
    : canSetGoalPanelDraft
      ? false
    : !canSend
  const primaryActionLoading = !runtimeReady
  const goalRuntimeStartedAtMs = goalRuntimeStartedAtRef.current
  const liveGoalElapsedSeconds =
    busy && activeThreadGoal?.status === 'active' && goalRuntimeStartedAtMs != null
      ? Math.max(0, Math.floor((goalRuntimeNowMs - goalRuntimeStartedAtMs) / 1000))
      : 0
  const goalElapsedLabel = activeThreadGoal
    ? formatGoalElapsedSeconds((activeThreadGoal.timeUsedSeconds ?? 0) + liveGoalElapsedSeconds)
    : ''
  const goalMenuChecked = shouldPauseGoal(activeThreadGoal, activeGoalActivation)
  const goalResumeExhausted = !goalMenuChecked && goalRoundLimitExhausted(activeThreadGoal)

  const commandDiscoveryOpen = composerMenuOpen || slashQuery != null
  useEffect(() => {
    setSelectedCommandIndex(0)
  }, [slashQuery])

  useEffect(() => {
    if (slashQuery == null) return
    const dismissOutside = (event: PointerEvent | FocusEvent) => {
      if (event.target instanceof Node && !composerRootRef.current?.contains(event.target)) setDismissedSlashKey(slashOwnerKey)
    }
    document.addEventListener('pointerdown', dismissOutside, true)
    document.addEventListener('focusin', dismissOutside)
    return () => {
      document.removeEventListener('pointerdown', dismissOutside, true)
      document.removeEventListener('focusin', dismissOutside)
    }
  }, [slashOwnerKey, slashQuery])

  const previousComposerContext = useRef(JSON.stringify([activeThreadId, effectiveWorkspaceRoot]))
  useLayoutEffect(() => {
    const context = JSON.stringify([activeThreadId, effectiveWorkspaceRoot])
    if (previousComposerContext.current === context) return
    previousComposerContext.current = context
    setComposerMenuOpen(false)
    setComposerPicker(null)
    setGoalPanelOpen(false)
    setDismissedSlashKey(slashOwnerKey)
    setDismissedMentionKey(activeAtMentionKey)
  }, [activeThreadId, effectiveWorkspaceRoot, slashOwnerKey, activeAtMentionKey])

  useEffect(() => {
    if (!voiceActivity) return
    setComposerMenuOpen(false)
    setComposerPicker(null)
  }, [voiceActivity])

  useEffect(() => {
    setSelectedMentionIndex(0)
  }, [activeAtMentionKey])

  useEffect(() => {
    if (route === 'claw') {
      setAgentPlugins(EMPTY_HUB_AGENT_PLUGINS)
      setAgentPluginsLoading(false)
      return
    }
    if (!showAtMentionMenu && !commandDiscoveryOpen) {
      setAgentPluginsLoading(false)
      return
    }
    const sync = window.analytix?.app?.syncHubAgentMarketplace
    if (typeof sync !== 'function') {
      setAgentPlugins(EMPTY_HUB_AGENT_PLUGINS)
      setAgentPluginsLoading(false)
      return
    }

    let cancelled = false
    setAgentPluginsLoading(true)
    void sync({ mode: 'cache' })
      .then((result) => {
        if (cancelled) return
        setAgentPlugins(installedHubAgentPlugins(result.plugins))
      })
      .catch(() => {
        if (!cancelled) setAgentPlugins(EMPTY_HUB_AGENT_PLUGINS)
      })
      .finally(() => {
        if (!cancelled) setAgentPluginsLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [route, showAtMentionMenu, commandDiscoveryOpen, composerOwnerKey])

  useEffect(() => {
    if (slashQuery != null || goalPanelOpen) setComposerMenuOpen(false)
  }, [goalPanelOpen, slashQuery])

  useEffect(() => {
    if (!showAtMentionMenu || !activeAtMention || !fileReferenceEnabled || !effectiveWorkspaceRoot) {
      setFileMentionSuggestions((current) => (current.length === 0 ? current : []))
      setFileMentionLoading(false)
      return
    }

    let cancelled = false
    const query = activeAtMention.query
    const timer = window.setTimeout(() => {
      setFileMentionLoading(true)
      // Resolve the index and any deep path-mention target in parallel so a
      // deeply nested file the bounded index never reached still resolves
      // (issue #340).
      void Promise.all([
        loadWorkspaceFileIndex(effectiveWorkspaceRoot),
        loadWorkspaceMentionPathSuggestions(effectiveWorkspaceRoot, query).catch(() => [])
      ])
        .then(([index, pathSuggestions]) => {
          if (cancelled) return
          const candidates = mergeMentionCandidates(
            [...index.directories, ...index.files],
            pathSuggestions
          )
          setFileMentionSuggestions(
            filterWorkspaceFileMentionSuggestions(candidates, query, fileReferences)
          )
        })
        .catch(() => {
          if (!cancelled) setFileMentionSuggestions([])
        })
        .finally(() => {
          if (!cancelled) setFileMentionLoading(false)
        })
    }, 80)

    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [activeAtMention, effectiveWorkspaceRoot, fileReferenceEnabled, fileReferences, showAtMentionMenu])

  useEffect(() => {
    if (!composerMenuOpen) return
    const frame = requestAnimationFrame(() => composerMenuPanelRef.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus({ preventScroll: true }))
    return () => cancelAnimationFrame(frame)
  }, [composerMenuOpen])

  useEffect(() => {
    if (!composerMenuOpen && !goalPanelOpen) return

    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (!(target instanceof Node)) return
      if (composerMenuButtonRef.current?.contains(target)) return
      if (composerMenuPanelRef.current?.contains(target)) return
      if (goalPanelRef.current?.contains(target)) return
      setComposerMenuOpen(false)
      setGoalPanelOpen(false)
    }

    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.isComposing || event.keyCode === 229 || event.defaultPrevented || event.key !== 'Escape') return
      if (composerMenuOpen) { event.preventDefault(); composerMenuButtonRef.current?.focus({ preventScroll: true }) }
      setComposerMenuOpen(false)
      setGoalPanelOpen(false)
    }

    const onFocusIn = (event: FocusEvent): void => {
      if (!composerMenuOpen || !(event.target instanceof Node)) return
      if (composerMenuPanelRef.current?.contains(event.target) || composerMenuButtonRef.current?.contains(event.target)) return
      setComposerMenuOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('focusin', onFocusIn)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('focusin', onFocusIn)
    }
  }, [composerMenuOpen, goalPanelOpen])

  useEffect(() => {
    const shouldTimeGoal = busy && activeThreadGoal?.status === 'active'
    if (!shouldTimeGoal) {
      goalRuntimeStartedAtRef.current = null
      setGoalRuntimeNowMs(Date.now())
      return
    }

    if (goalRuntimeStartedAtRef.current == null) {
      const startedAt = Date.now()
      goalRuntimeStartedAtRef.current = startedAt
      setGoalRuntimeNowMs(startedAt)
    }

    const interval = window.setInterval(() => {
      setGoalRuntimeNowMs(Date.now())
    }, 1000)
    return () => window.clearInterval(interval)
  }, [busy, activeThreadGoal?.createdAt, activeThreadGoal?.objective, activeThreadGoal?.status])

  const applySlashCommand = (commandId: SlashCommandId, source: 'slash' | 'menu' = 'slash'): void => {
    if (composerOwnerRevision !== composerOwnerRevisionRef.current) return
    const selected = slashCommands.find(command => command.id === commandId)
    if (!selected || selected.disabled) return
    setComposerMenuOpen(false)
    if (source === 'slash') setDismissedSlashKey(slashOwnerKey)
    if (commandId === 'file' || commandId === 'workspace-files' || commandId === 'attachments') {
      if (source === 'slash') setInput('')
      if (commandId === 'attachments') handleAttachmentMenuClick()
      else if (commandId === 'workspace-files' || !canPickLocalFileReference) handleFileReferenceMenuClick()
      else handleLocalFileReferenceMenuClick()
      return
    }
    if (commandId === 'permission' || commandId === 'model') {
      if (source === 'slash') setInput('')
      setComposerPicker(commandId)
      return
    }
    if (commandId === 'worktree') { handleWorktreeToolbarClick(); return }
    if (source === 'menu' && commandId === 'plan') { handlePlanToolbarClick(); return }
    if (source === 'menu' && commandId === 'goal') { handleGoalMenuClick(); return }
    if (commandId.startsWith('plugin:')) {
      const plugin = agentPlugins.find(item => hubPluginMentionId(item) === commandId.slice('plugin:'.length))
      if (!plugin) return
      const token = formatComposerPluginMentionToken(hubPluginMentionDisplayName(plugin), hubPluginMentionId(plugin))
      if (source === 'menu') insertTextAtComposerCursor(token)
      else {
        setInput(`${token} `)
        focusComposerAtRawCursorAfterRender(token.length + 1, token.length + 1)
      }
      return
    }
    if (commandId.startsWith('skill:')) {
      const command = slashCommands.find((item) => item.id === commandId)
      if (command?.skillPrompt) {
        setInput(command.skillPrompt)
        draft.focusComposer()
      }
      return
    }
    if (commandId === 'plan') {
      setInput('')
      setMode('plan')
      onPlanCommand?.()
      draft.focusComposer()
      return
    }
    if (commandId === 'new' && onNewCommand) {
      setInput('')
      onNewCommand()
      draft.focusComposer()
      return
    }
    if (commandId === 'compact') {
      setInput('')
      void compactActiveThread()
      draft.focusComposer()
      return
    }
    if (commandId === 'goal') {
      setInput('')
      setGoalPanelOpen(true)
      draft.focusComposer()
      return
    }
    if (commandId === 'research') {
      setMode('agent')
      setInput(buildResearchPrompt(t('slashCommandResearchPrompt'), null))
      draft.focusComposer()
      return
    }
    if (commandId === 'review' && onReviewCommand) {
      setInput('')
      void onReviewCommand({ kind: 'uncommittedChanges' })
      draft.focusComposer()
      return
    }
    if (commandId === 'fork') {
      setInput('')
      void forkActiveThread()
      draft.focusComposer()
      return
    }
    if (commandId === 'archive' && activeThreadId) {
      setInput('')
      void archiveThread(activeThreadId, true)
      draft.focusComposer()
      return
    }
    if (commandId === 'restore' && activeThreadId) {
      setInput('')
      void archiveThread(activeThreadId, false)
      draft.focusComposer()
      return
    }
    if (commandId === 'btw' && onBtwCommand) {
      // Empty aside — open a side conversation without a seed question.
      setInput('')
      void onBtwCommand()
      return
    }
  }

  const runGoalCommand = (command: ReturnType<typeof parseGoalCommand>): boolean => {
    if (command === false) return false
    if (!canOpenGoalPanel) return true
    setInput('')
    setGoalPanelOpen(false)
    if (command.action === 'menu') {
      setGoalPanelOpen(true)
      draft.focusComposer()
      return true
    }
    if (command.action === 'set') {
      void setActiveThreadGoal(command.objective)
      return true
    }
    if (command.action === 'research') {
      void setActiveThreadGoal(command.objective, { research: true })
      return true
    }
    if (command.action === 'pause') {
      void setActiveThreadGoalStatus('paused')
      return true
    }
    if (command.action === 'resume') {
      void setActiveThreadGoalStatus('active')
      return true
    }
    if (command.action === 'clear') {
      void clearActiveThreadGoal()
      return true
    }
    return true
  }

  const setGoalFromComposerInput = (): boolean => {
    if (!canSetGoalPanelDraft) return false
    setInput('')
    setGoalPanelOpen(false)
    void setActiveThreadGoal(goalPanelDraftObjective)
    draft.focusComposer()
    return true
  }

  const handleComposerMenuButtonClick = (): void => {
    if (!canOpenComposerMenu) return
    setGoalPanelOpen(false)
    setComposerMenuOpen((open) => !open)
    if (composerMenuOpen) composerMenuButtonRef.current?.focus({ preventScroll: true })
  }

  const handleAttachmentMenuClick = (): void => {
    if (!canPickAttachment || !onPickAttachments) return
    setComposerMenuOpen(false)
    fileInputRef.current?.click()
    draft.focusComposer()
  }

  const handleFileReferenceMenuClick = (): void => {
    if (!canPickFileReference) return
    setComposerMenuOpen(false)
    onOpenFileReferencePicker?.()
    draft.focusComposer()
  }

  const handleLocalFileReferenceMenuClick = (): void => {
    if (!canPickLocalFileReference) return
    setComposerMenuOpen(false)
    onPickFileReferences?.()
    draft.focusComposer()
  }

  const handlePlanToolbarClick = (): void => {
    if (!canTogglePlanMode) return
    setComposerMenuOpen(false)
    if (mode === 'plan') {
      setMode('agent')
    } else {
      setMode('plan')
      onPlanCommand?.()
    }
    draft.focusComposer()
  }

  const handleGoalMenuClick = (): void => {
    if (!canOpenGoalPanel) return
    setComposerMenuOpen(false)
    if (activeThreadGoal?.status === 'complete') {
      setGoalPanelOpen(true)
    } else if (goalMenuChecked) {
      void setActiveThreadGoalStatus('paused')
    } else if (activeThreadGoal) {
      void setActiveThreadGoalStatus('active')
    } else {
      setGoalPanelOpen(true)
    }
    draft.focusComposer()
  }

  const handleWorktreeToolbarClick = (): void => {
    if (!onToggleWorktreeMode) return
    setComposerMenuOpen(false)
    onToggleWorktreeMode()
    draft.focusComposer()
  }

  const focusComposerAtRawCursor = (cursor: number, inputLength = input.length): void => {
    const boundedCursor = Math.max(0, Math.min(inputLength, cursor))
    composerEditorRef.current?.setRawSelection(boundedCursor)
    setComposerCursor(boundedCursor)
  }

  const focusComposerAtRawCursorAfterRender = (cursor: number, inputLength = input.length): void => {
    window.requestAnimationFrame(() => {
      focusComposerAtRawCursor(cursor, inputLength)
      window.requestAnimationFrame(() => {
        focusComposerAtRawCursor(cursor, inputLength)
      })
    })
  }

  const replaceActiveAtMention = (token: string): void => {
    if (!activeAtMention) return
    const next = replaceComposerAtMentionInInput(input, activeAtMention, token)
    const nextVisibleCursor = Math.max(0, Math.min(next.input.length, next.cursor))
    setInput(next.input)
    setDismissedMentionKey(null)
    focusComposerAtRawCursorAfterRender(nextVisibleCursor, next.input.length)
  }

  const applyPluginMention = (suggestion: Extract<ComposerMentionSuggestion, { kind: 'plugin' }>): void => {
    replaceActiveAtMention(suggestion.token)
  }

  const applySkillMention = (suggestion: Extract<ComposerMentionSuggestion, { kind: 'skill' }>): void => {
    replaceActiveAtMention(suggestion.token)
  }

  const applyFileMention = (reference: ComposerFileReference | null): void => {
    if (!reference || !activeAtMention) return
    const token = formatComposerFileMentionToken(
      reference.relativePath,
      isComposerDirectoryReference(reference)
    )
    onAddFileReference?.(reference)
    replaceActiveAtMention(token)
  }

  const applyMentionSuggestion = (suggestion: ComposerMentionSuggestion | null): void => {
    if (composerOwnerRevision !== composerOwnerRevisionRef.current) return
    if (!suggestion) return
    if (suggestion.kind === 'plugin') {
      applyPluginMention(suggestion)
      return
    }
    if (suggestion.kind === 'skill') {
      applySkillMention(suggestion)
      return
    }
    applyFileMention(suggestion.reference)
  }

  const removeFileReference = (reference: ComposerFileReference): void => {
    onRemoveFileReference?.(reference.relativePath)
    const nextInput = removeComposerFileMentionToken(
      input,
      reference.relativePath,
      isComposerDirectoryReference(reference)
    )
    if (nextInput !== input) {
      setInput(nextInput)
      focusComposerAtRawCursorAfterRender(nextInput.length, nextInput.length)
    }
    draft.focusComposer()
  }

  const handlePrimaryAction = (intent?: 'queue' | 'steer'): void => {
    if (!busy && attachmentUploadBusy) return
    if (highlightedSlashCommand) {
      if (!highlightedSlashCommand.disabled) applySlashCommand(highlightedSlashCommand.id)
      return
    }
    if (primaryActionIsStop) {
      onInterrupt()
      return
    }
    if (executionPolicyBlocked || (busy && !canSend)) return
    if (busyCanSendFollowup) {
      if (intent) onSend(intent)
      else onSend()
      return
    }
    if (secondaryOwner) {
      onSend()
      return
    }
    if (setGoalFromComposerInput()) {
      return
    }
    if (runGoalCommand(parsedGoalCommand)) {
      return
    }
    if (onNewCommand && parseNewCommand(input)) {
      const command = slashCommands.find((item) => item.id === 'new')
      if (command?.disabled) return
      setInput('')
      onNewCommand()
      draft.focusComposer()
      return
    }
    const compactCommand = parseCompactCommand(input)
    if (compactCommand) {
      const command = slashCommands.find((item) => item.id === 'compact')
      if (command?.disabled) return
      setInput('')
      void compactActiveThread(compactCommand.reason)
      draft.focusComposer()
      return
    }
    const researchTopic = parseResearchCommand(input)
    if (researchTopic !== false) {
      const command = slashCommands.find((item) => item.id === 'research')
      if (command?.disabled) return
      setMode('agent')
      setInput(buildResearchPrompt(t('slashCommandResearchPrompt'), researchTopic))
      draft.focusComposer()
      return
    }
    if (onReviewCommand) {
      const reviewCommand = parseReviewCommand(input)
      if (reviewCommand !== false) {
        const command = slashCommands.find((item) => item.id === 'review')
        if (command?.disabled) return
        setInput('')
        void onReviewCommand(reviewCommand)
        draft.focusComposer()
        return
      }
    }
    // Send-time interception: `/btw <question>` is treated as a side
    // conversation spawn, mirroring the plan-mode interception.
    if (onBtwCommand && !hideBtwCommand) {
      const parsed = parseBtwCommand(input)
      if (parsed !== false) {
        setInput('')
        void onBtwCommand(parsed ?? undefined)
        return
      }
    }
    if (intent) onSend(intent)
    else onSend()
  }
  useLayoutEffect(() => {
    dictationPrimaryActionRef.current = primaryActionDisabled || busy && !busyCanSendFollowup ? null : handlePrimaryAction
  })

  const handleComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement | HTMLDivElement>): void => {
    const accelerated = event.metaKey !== event.ctrlKey
    const sendByEnter = event.key === 'Enter' && !event.shiftKey && !event.altKey
      && !(event.metaKey && event.ctrlKey) && !event.getModifierState('AltGraph')
    const composing = draft.isComposingEvent(event)
    // Holding Enter must not select a menu item and then submit or stop a turn.
    if (!composing && event.key === 'Enter' && event.repeat) {
      event.preventDefault()
      return
    }

    if (!composing && showAtMentionMenu) {
      if (event.key === 'Tab' && event.shiftKey) {
        setDismissedMentionKey(activeAtMentionKey)
        setFileMentionSuggestions([])
        return
      }
      if (event.key === 'ArrowDown' && flatMentionSuggestions.length > 0) {
        event.preventDefault()
        setSelectedMentionIndex((current) => (current + 1) % flatMentionSuggestions.length)
        return
      }
      if (event.key === 'ArrowUp' && flatMentionSuggestions.length > 0) {
        event.preventDefault()
        setSelectedMentionIndex((current) =>
          current === 0 ? flatMentionSuggestions.length - 1 : current - 1
        )
        return
      }
      if ((sendByEnter || event.key === 'Tab' && !event.shiftKey) && highlightedMention) {
        event.preventDefault()
        applyMentionSuggestion(highlightedMention)
        return
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        setDismissedMentionKey(activeAtMentionKey)
        setFileMentionSuggestions([])
        return
      }
    }

    if (!composing && slashQuery != null) {
      if (event.key === 'Tab' && event.shiftKey) {
        setDismissedSlashKey(slashOwnerKey)
        return
      }
      if ((sendByEnter || event.key === 'Tab') && highlightedSlashCommand) {
        event.preventDefault()
        if (!highlightedSlashCommand.disabled) applySlashCommand(highlightedSlashCommand.id)
        return
      }
      if (event.key === 'ArrowDown' && filteredSlashCommands.length > 0) {
        event.preventDefault()
        setSelectedCommandIndex((current) => (current + 1) % filteredSlashCommands.length)
        return
      }
      if (event.key === 'ArrowUp' && filteredSlashCommands.length > 0) {
        event.preventDefault()
        setSelectedCommandIndex((current) =>
          current === 0 ? filteredSlashCommands.length - 1 : current - 1
        )
        return
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        setDismissedSlashKey(slashOwnerKey)
        return
      }
    }

    if (!composing && event.key === 'Enter' && event.shiftKey && !event.metaKey && !event.ctrlKey) {
      event.preventDefault()
      insertTextAtComposerCursor('\n')
      return
    }

    if (!sendByEnter || composing) return

    event.preventDefault()
    // Keyboard submission never invokes the empty-draft Stop button action.
    if (attachmentUploadBusy || primaryActionDisabled || (busy && !busyCanSendFollowup)) return
    handlePrimaryAction(busy && accelerated ? 'queue' : undefined)
  }

  const handleComposerShellMouseDown = (event: ReactMouseEvent<HTMLDivElement>): void => {
    if (!canEditComposer) return
    const target = event.target
    if (
      target instanceof Element &&
      target.closest("button,input,textarea,select,a,summary,[role='button'],[contenteditable='true']")
    ) {
      return
    }
    event.preventDefault()
    draft.textareaRef.current?.focus()
  }

  useEffect(() => {
    if (compact || route !== 'chat' || !canEditComposer || presentationPhase === 'settling') return
    const mayFocusComposer = (): boolean => {
      const active = document.activeElement
      return active === null || active === document.body || active === document.documentElement ||
        Boolean(composerRootRef.current?.contains(active))
    }
    if (!mayFocusComposer()) return

    const frame = window.requestAnimationFrame(() => {
      // Modal restoration or user navigation may establish a new focus owner before this frame.
      if (composerMenuPanelRef.current || goalPanelRef.current) return
      const current = document.activeElement
      if (current instanceof Node && composerMenuButtonRef.current?.contains(current)) return
      if (mayFocusComposer()) draft.textareaRef.current?.focus()
    })

    return () => window.cancelAnimationFrame(frame)
  }, [activeThreadId, canEditComposer, compact, presentationPhase, route, runtimeReady, draft.textareaRef])

  const handleAttachmentInput = (event: ChangeEvent<HTMLInputElement>): void => {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ''
    if (files.length === 0 || !onPickAttachments) return
    onPickAttachments(files)
  }

  const handleComposerPaste = (event: ReactClipboardEvent<HTMLElement>): void => {
    handleComposerImagePaste({
      canPickAttachment,
      clipboardData: event.clipboardData,
      preventDefault: () => event.preventDefault(),
      onPickAttachments,
      onPasteClipboardImage
    })
  }

  const handleComposerDragOver = (event: ReactDragEvent<HTMLDivElement>): void => {
    const dataTransferTypes = Array.from(event.dataTransfer.types ?? [])
    const canAcceptImages = canPickAttachment && imageTransferHasImages(event.dataTransfer)
    const canAcceptPdf = canPickAttachment && Array.from(event.dataTransfer.files ?? []).some(isPdfFile)
    const canAcceptLocalFiles = canPickLocalFileReference && dataTransferTypes.includes('Files')
    if (!canAcceptLocalFiles && !canAcceptImages && !canAcceptPdf) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'copy'
  }

  const insertTextAtComposerCursor = (text: string): void => {
    if (!text) return
    const currentValue = input
    const selection = composerEditorRef.current?.getRawSelection()
    const selectionStart = selection?.from ?? composerCursor ?? currentValue.length
    const selectionEnd = selection?.to ?? selectionStart
    const before = currentValue.slice(0, selectionStart)
    const after = currentValue.slice(selectionEnd)
    const shouldPad = !text.includes('\n')
    const leadingPad = shouldPad && before.length > 0 && !/\s$/.test(before) ? ' ' : ''
    const trailingPad = shouldPad && after.length > 0 && !/^\s/.test(after) ? ' ' : ''
    const insertion = `${leadingPad}${text}${trailingPad}`
    const nextInput = `${before}${insertion}${after}`
    const nextCursor = before.length + insertion.length - trailingPad.length
    setInput(nextInput)
    window.requestAnimationFrame(() => {
      focusComposerAtRawCursor(nextCursor, nextInput.length)
    })
  }

  const handleComposerDrop = (event: ReactDragEvent<HTMLDivElement> | DragEvent): void => {
    if (composerOwnerRevision !== composerOwnerRevisionRef.current) return
    if (!event.dataTransfer) return
    const imageFiles = canPickAttachment ? imageFilesFromTransfer(event.dataTransfer) : []
    const rawFiles = Array.from(event.dataTransfer.files ?? [])
    const isImageLike = (file: File): boolean =>
      isImageMimeType(file.type) || Boolean(imageMimeTypeFromFileName(file.name))
    const pdfFiles = canPickAttachment ? rawFiles.filter(isPdfFile) : []
    const pathFiles = canPickLocalFileReference && onAddFileReference
      ? rawFiles.filter((file) => !isImageLike(file) && !isPdfFile(file))
      : []
    if (imageFiles.length === 0 && pdfFiles.length === 0 && pathFiles.length === 0) return
    event.preventDefault()
    if ((imageFiles.length > 0 || pdfFiles.length > 0) && onPickAttachments) {
      onPickAttachments([...imageFiles, ...pdfFiles])
    }
    if (pathFiles.length > 0) {
      const paths: string[] = []
      for (const file of pathFiles) {
        try {
          const path = window.analytix.files.getPathForFile(file)
          if (path) paths.push(path)
        } catch {
          // ignore files we cannot resolve a filesystem path for
        }
      }
      for (const path of paths) {
        onAddFileReference?.(composerFileReferenceFromPath(path, effectiveWorkspaceRoot))
      }
    }
    draft.focusComposer()
  }

  const canAcceptComposerDrop = useCallback((transfer: DataTransfer): boolean => {
    if (composerOwnerRevision !== composerOwnerRevisionRef.current) return false
    if (canPickLocalFileReference && Array.from(transfer.types ?? []).includes('Files')) return true
    return canPickAttachment && (imageTransferHasImages(transfer) ||
      Array.from(transfer.files ?? []).some(isPdfFile) ||
      Array.from(transfer.items ?? []).some(item => item.kind === 'file' && item.type === 'application/pdf'))
  }, [canPickAttachment, canPickLocalFileReference, composerOwnerRevision])

  const renderMentionIcon = (suggestion: ComposerMentionSuggestion): ReactElement => {
    if (suggestion.kind === 'plugin') {
      if (suggestion.iconUrl) {
        return (
          <img
            src={suggestion.iconUrl}
            alt=""
            className="h-3.5 w-3.5 object-contain"
            onError={event => { event.currentTarget.style.display = 'none' }}
          />
        )
      }
      return <Plug strokeWidth={1.8} style={suggestion.brandColor ? { color: suggestion.brandColor } : undefined} />
    }
    if (suggestion.kind === 'skill') return <Sparkles strokeWidth={1.8} />
    return isComposerDirectoryReference(suggestion.reference) ? <Folder strokeWidth={1.8} /> : <FileText strokeWidth={1.8} />
  }

  const mentionMenuItems = mentionSections.flatMap(section => section.items.map(suggestion => ({
    id: suggestion.key,
    title: suggestion.title,
    description: suggestion.description,
    icon: renderMentionIcon(suggestion),
    canonicalName: suggestion.kind === 'file' ? suggestion.title : suggestion.badge,
    section: section.key,
    sectionLabel: section.title,
    suggestion
  })))

  // Preserve the feedback toast DOM anchor; InputBar owns the actual presentation.
  return (
    <div
      ref={composerRootRef}
      data-analytix-input-bar
      data-main-chat-composer={mainChatComposer ? '' : undefined}
      data-variant={variant}
      data-input-layout={presentationPhase === 'hero' ? 'hero' : 'composer'}
      className={compact
        ? `dsh-input-bar ds-no-drag pointer-events-auto w-full pb-0 pt-0 ds-floating-composer ${className ?? ''}`
        : `dsh-input-bar ds-no-drag ds-chat-column-inset pointer-events-auto w-full max-w-4xl pb-3 pt-0 ds-floating-composer ${className ?? ''}`}
    >
      <ComposerDropOverlay enabled={mainChatComposer && canCompose && !voiceActivity && composerGate?.status !== 'pending'}
        ownerKey={composerOwnerKey} rootRef={composerRootRef} canAccept={canAcceptComposerDrop}
        onDrop={handleComposerDrop} label={t('composerDropFilesHint')} />
      {mountedQuestions.map(block => <div key={activeThreadId + ':' + block.id + ':' + block.requestId}
        hidden={composerGate?.id !== block.id} inert={composerGate?.id !== block.id}
        data-analytix-gate-composer data-gate-thread-id={activeThreadId}>
        <GateResponseContext.Provider value={null}><MessageBubble block={block} threadId={activeThreadId} presented={composerGate?.id === block.id} /></GateResponseContext.Provider>
      </div>)}
      {composerGate && (gateOverride !== undefined || composerGate.kind === 'approval') ? <div key={(gateOverride === undefined ? activeThreadId : gateOverride.threadId) + ':' + composerGate.id} data-analytix-gate-composer data-gate-thread-id={gateOverride === undefined ? activeThreadId : gateOverride.threadId}>
        <GateResponseContext.Provider value={gateOverride === undefined ? null : { onOpenOwnerThread: gateOverride.onOpenOwnerThread }}>
          <MessageBubble block={composerGate} threadId={gateOverride === undefined ? activeThreadId : gateOverride.threadId} />
        </GateResponseContext.Provider>
      </div> : null}
      {composerGate?.status === 'pending' ? null : <>
      {hideThreadContextPanels ? null : (
        <>
          <ComposerInProgressStatusPill
            todos={activeThreadTodos}
            diffSummary={currentTurnDiffSummary}
            busy={busy}
            currentTurnId={currentTurnId}
            hasBlockingRequest={hasPendingBlockingRequest}
            onOpenChanges={onOpenChanges}
          />

          <AboveComposerPanelStack>
            <QueuedUserMessages messages={queuedMessages} onRemove={onRemoveQueuedMessage} />

            <ThreadHandoffInlineProgress />
          </AboveComposerPanelStack>
        </>
      )}



          {compact || presentationPhase !== 'hero' ? null : (
            <div className="dsh-input-accessory ds-composer-context ds-no-drag flex min-h-7 min-w-0 flex-wrap items-center gap-x-2 gap-y-1 px-1">
              {route === 'chat' ? (
                <WorkspaceProjectPicker currentWorkspaceRoot={effectiveWorkspaceRoot} />
              ) : null}
              <GitBranchPicker workspaceRoot={effectiveWorkspaceRoot} />
            </div>
          )}
      <div className="relative">
        {hideThreadContextPanels ? null : <ThreadHandoffProgressModal />}

        {composerMenuOpen && slashQuery == null ? (
          <FloatingComposerCommandMenu
            ref={composerMenuPanelRef}
            id={composerMenuId}
            items={composerMenuItems}
            groupLabels={commandGroupLabels}
            ariaLabel={t('composerMenuTitle')}
            emptyLabel={t('slashCommandEmpty')}
            onSelect={command => applySlashCommand(command.id, 'menu')}
            onKeyDown={event => {
              if (draft.isComposingEvent(event) || event.defaultPrevented) return
              if (event.key === 'Tab') {
                composerMenuButtonRef.current?.focus({ preventScroll: true })
                setComposerMenuOpen(false)
                return
              }
              if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
              event.preventDefault()
              const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'))
              if (!items.length) return
              const current = items.indexOf(document.activeElement as HTMLButtonElement)
              const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 :
                (current + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length
              items[next]?.focus({ preventScroll: true })
            }}
          />
        ) : null}

        {slashQuery != null ? (
          <FloatingComposerCommandMenu
            id={`${composerMenuId}-slash`}
            role="listbox"
            items={filteredSlashCommands}
            selectedIndex={Math.min(selectedCommandIndex, filteredSlashCommands.length - 1)}
            groupLabels={commandGroupLabels}
            ariaLabel={t('slashCommandMenuTitle')}
            emptyLabel={t('slashCommandEmpty')}
            onHover={setSelectedCommandIndex}
            onSelect={command => applySlashCommand(command.id)}
          />
        ) : null}

        {showAtMentionMenu ? (
          <FloatingComposerCommandMenu
            id={`${composerMenuId}-mention`}
            role="listbox"
            items={mentionMenuItems}
            selectedIndex={Math.min(selectedMentionIndex, mentionMenuItems.length - 1)}
            ariaLabel={t('composerAtMentionMenuTitle')}
            emptyLabel={atMentionLoading ? t('composerAtMentionLoading') : t('composerAtMentionEmpty')}
            loading={atMentionLoading}
            onHover={setSelectedMentionIndex}
            onSelect={item => applyMentionSuggestion(item.suggestion)}
          />
        ) : null}

        {goalPanelOpen && slashQuery == null ? (
          <div
            ref={goalPanelRef}
            className="absolute inset-x-2 bottom-full z-30 mb-3 overflow-hidden rounded-[26px] border border-ds-border bg-ds-card/95 p-3 shadow-[0_18px_52px_rgba(20,47,95,0.14)] backdrop-blur-xl dark:bg-ds-card/90"
          >
            <div className="flex items-start gap-3">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-ds-border-muted text-ds-muted">
                <Target className="h-4 w-4" strokeWidth={1.9} />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-2">
                  <div className="truncate text-[14px] font-semibold text-ds-ink">
                    {activeThreadGoal ? activeThreadGoal.objective : t('goalNoActiveTitle')}
                  </div>
                  {activeThreadGoal ? (
                    <span className="shrink-0 rounded-lg border border-ds-border-muted bg-ds-card px-2 py-0.5 text-[11px] font-semibold text-ds-muted">
                      {t(`goalStatusShort.${activeThreadGoal.status}`)}
                    </span>
                  ) : null}
                </div>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {canSetGoalPanelDraft ? (
                    <button
                      type="button"
                      onClick={setGoalFromComposerInput}
                      className="rounded-full border border-ds-border bg-ds-card px-3 py-1.5 text-[12px] font-semibold text-ds-ink transition hover:bg-ds-hover"
                    >
                      {t('goalSetCurrentInput')}
                    </button>
                  ) : null}
                  {goalMenuChecked ? (
                    <button
                      type="button"
                      onClick={() => {
                        setGoalPanelOpen(false)
                        void setActiveThreadGoalStatus('paused')
                      }}
                      className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-ds-border bg-ds-card text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                      aria-label={t('goalActionPause')}
                      title={t('goalActionPause')}
                    >
                      <PauseCircle className="h-4 w-4" strokeWidth={1.9} />
                    </button>
                  ) : activeThreadGoal && activeThreadGoal.status !== 'complete' ? (
                    <button
                      type="button"
                      onClick={() => {
                        setGoalPanelOpen(false)
                        void setActiveThreadGoalStatus('active')
                      }}
                      className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-ds-border bg-ds-card text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                      aria-label={t('goalActionResume')}
                      title={t('goalActionResume')}
                      disabled={goalResumeExhausted}
                    >
                      <PlayCircle className="h-4 w-4" strokeWidth={1.9} />
                    </button>
                  ) : null}
                  {activeThreadGoal ? (
                    <button
                      type="button"
                      onClick={() => {
                        setGoalPanelOpen(false)
                        void clearActiveThreadGoal()
                      }}
                      className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-ds-border bg-ds-card text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                      aria-label={t('goalActionClear')}
                      title={t('goalActionClear')}
                    >
                      <Trash2 className="h-4 w-4" strokeWidth={1.9} />
                    </button>
                  ) : null}
                </div>
              </div>
              <button
                type="button"
                onClick={() => setGoalPanelOpen(false)}
                className="rounded-lg p-1.5 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={t('close')}
                title={t('close')}
              >
                <X className="h-4 w-4" strokeWidth={2} />
              </button>
            </div>
          </div>
        ) : null}

        <div
          className={`dsh-input-card ds-no-drag ${
            draft.focused ? 'is-focused' : ''
          }`}
          onMouseDown={handleComposerShellMouseDown}
          onPaste={handleComposerPaste}
          onDragOver={handleComposerDragOver}
          onDrop={handleComposerDrop}
        >
          {compact || hideThreadContextPanels ? null : (
            <ComposerGoalRow
              goal={activeThreadGoal}
              elapsedLabel={goalElapsedLabel}
              onEdit={() => {
                setGoalPanelOpen(true)
                draft.focusComposer()
              }}
              onToggleStatus={() => {
                if (activeThreadGoal && activeThreadGoal.status !== 'complete') {
                  void setActiveThreadGoalStatus(goalMenuChecked ? 'paused' : 'active')
                }
              }}
              onClear={() => {
                void clearActiveThreadGoal()
              }}
            />
          )}
          {showChangeSummary ? (
            <div className="ds-no-drag mb-1 rounded-2xl border border-ds-border-muted bg-ds-card/78 px-3 py-2 shadow-sm">
              <div className="flex min-w-0 items-center gap-2">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-ds-hover text-ds-muted">
                  <FileEdit className="h-4 w-4" strokeWidth={1.8} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[13px] font-semibold text-ds-ink">
                    <span className="truncate">{t('composerChangedFilesTitle', { count: changedFiles.length })}</span>
                    <span className="font-mono text-[12px] text-ds-diff-added">
                      +{effectiveChangedFileStats.added}
                    </span>
                    <span className="font-mono text-[12px] text-ds-diff-removed">
                      -{effectiveChangedFileStats.removed}
                    </span>
                  </div>
                  <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-ds-muted">
                    {visibleChangedFiles.map((file) => (
                      <span key={file.path} className="max-w-[220px] truncate" title={file.path}>
                        {file.path}
                      </span>
                    ))}
                    {hiddenChangedFileCount > 0 ? (
                      <span className="text-ds-faint">
                        {t('composerChangedFilesMore', { count: hiddenChangedFileCount })}
                      </span>
                    ) : null}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1.5">
                  {onOpenChanges ? (
                    <button
                      type="button"
                      onClick={onOpenChanges}
                      className="rounded-full border border-ds-border bg-ds-card px-3 py-1.5 text-[12px] font-semibold text-ds-ink transition hover:bg-ds-hover"
                    >
                      {t('composerOpenChanges')}
                    </button>
                  ) : null}
                  {onReviewChanges ? (
                    <button
                      type="button"
                      disabled={reviewChangesDisabled}
                      onClick={onReviewChanges}
                      className="inline-flex items-center gap-1.5 rounded-full border border-ds-border bg-ds-card px-3 py-1.5 text-[12px] font-semibold text-ds-ink transition hover:bg-ds-hover disabled:cursor-not-allowed disabled:opacity-55"
                    >
                      <SearchCode className="h-3.5 w-3.5" strokeWidth={1.8} />
                      {t('composerReviewChanges')}
                    </button>
                  ) : null}
                </div>
              </div>
            </div>
          ) : null}
          {fileReferences.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2 px-1">
              {fileReferences.map((reference) => {
                const isDirectory = isComposerDirectoryReference(reference)
                const displayPath = isDirectory ? `${reference.relativePath}/` : reference.relativePath
                return (
                  <ComposerReferenceChip
                    key={`${reference.type ?? 'file'}:${reference.relativePath}`}
                    title={displayPath}
                    remove={onRemoveFileReference ? {
                      label: t('composerRemoveFileReference'), onClick: () => removeFileReference(reference)
                    } : undefined}
                  >
                    {isDirectory ? (
                      <Folder className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    ) : (
                      <FileText className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    )}
                    <span className="min-w-0 max-w-52 truncate">{displayPath}</span>
                  </ComposerReferenceChip>
                )
              })}
            </div>
          ) : null}
          {attachments.length > 0 || attachmentUploads.length > 0 || attachmentUploadError ? (
            <div className="flex flex-wrap items-center gap-2 px-1">
              {attachments.map((attachment) => (
                attachment.previewUrl ? (
                  <ComposerImageAttachmentPreview
                    key={attachment.id}
                    attachment={attachment}
                    onRemoveAttachment={onRemoveAttachment}
                  />
                ) : (
                  <ComposerReferenceChip
                    key={attachment.id}
                    title={attachment.name || attachment.id}
                    remove={onRemoveAttachment ? {
                      label: t('composerRemoveAttachment'), onClick: () => onRemoveAttachment(attachment.id)
                    } : undefined}
                  >
                    {attachment.kind === 'document' ? (
                      <FileText className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    ) : (
                      <ImagePlus className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    )}
                    <span className="min-w-0 max-w-40 truncate">{attachment.name || attachment.id}</span>
                    {attachment.kind === 'document' && attachment.pageCount ? (
                      <span className="shrink-0 text-[11px] text-ds-faint">
                        {attachment.pageCount}p{attachment.truncated ? '+' : ''}
                      </span>
                    ) : null}
                  </ComposerReferenceChip>
                )
              ))}
              {attachmentUploads.map(upload => <ComposerReferenceChip key={upload.id} title={upload.error || upload.file.name}
                remove={onRemoveAttachmentUpload ? {
                  label: t('composerRemoveAttachment'), onClick: () => onRemoveAttachmentUpload(upload.id)
                } : undefined}>
                {upload.status === 'uploading' ? <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" /> : <FileText className="h-3.5 w-3.5 shrink-0" />}
                <span className="max-w-40 truncate">{upload.file.name}</span>
                <span role="status">{t(upload.status === 'failed' ? 'composerAttachmentFailed' : 'composerAttachmentUploading')}</span>
                {upload.status === 'failed' && onRetryAttachmentUpload ? <button type="button"
                  className="rounded px-1 hover:bg-ds-hover disabled:opacity-45" disabled={attachmentUploadBusy || !canCompose} aria-label={t('composerRetryAttachment')}
                  onClick={() => onRetryAttachmentUpload(upload.id)}>{t('composerRetryAttachment')}</button> : null}
              </ComposerReferenceChip>)}
              {attachmentUploadError ? (
                <span className="min-w-0 break-words text-[12px] font-medium text-red-600 dark:text-red-300">
                  {attachmentUploadError}
                </span>
              ) : null}
            </div>
          ) : null}
          {attachmentUploadEnabled ? (
            <input
              ref={fileInputRef}
              type="file"
              accept="image/png,image/jpeg,image/webp,image/heic,image/heif,.heic,.heif,application/pdf,.pdf"
              multiple
              className="hidden"
              onChange={handleAttachmentInput}
            />
          ) : null}
          {dictation.error ? (
            <div className="px-1">
              <span className="min-w-0 break-words text-[12px] font-medium text-red-600 dark:text-red-300">
                {dictation.error}
              </span>
            </div>
          ) : null}
          <ComposerPromptEditor
            ref={composerEditorRef}
            value={input}
            metadata={composerPromptMentionMetadata}
            placeholder={placeholder}
            disabled={!canEditComposer}
            compact={compact}
            spellCheck={false}
            onChange={(nextInput) => {
              setInput(nextInput)
              setDismissedMentionKey(null)
            }}
            onCursorChange={setComposerCursor}
            onFocus={draft.onFocus}
            onBlur={draft.onBlur}
            onCompositionStart={draft.onCompositionStart}
            onCompositionEnd={draft.onCompositionEnd}
            onKeyDownCapture={handleComposerKeyDown}
            onElement={(element) => {
              draft.textareaRef.current = element
            }}
          />
          <div
            className={`ds-composer-toolbar flex min-h-9 items-center ${
              showToolbarStartControls ? 'justify-between' : 'justify-end'
            }`}
          >
            {showToolbarStartControls && !voiceActivity ? (
              <div className="ds-composer-tools flex min-w-0 items-center">
                {showComposerMenuButton ? (
                  <>
                    <button
                      ref={composerMenuButtonRef}
                      type="button"
                      disabled={!canOpenComposerMenu}
                      onClick={handleComposerMenuButtonClick}
                      className={`ds-composer-utility-button ds-no-drag flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-ds-muted hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 ${
                        composerMenuOpen ? 'bg-ds-hover text-ds-ink' : ''
                      }`}
                      aria-haspopup="menu"
                      aria-expanded={composerMenuOpen}
                      aria-controls={composerMenuId}
                      aria-label={t('composerMenuTitle')}
                      title={t('composerMenuTitle')}
                    >
                      <Plus className="h-5 w-5" strokeWidth={1.8} />
                    </button>
                    {onExecutionSettingsChange && (executionSettings || executionSettingsUnavailable) ? (
                      <FloatingComposerExecutionPicker
                        key={composerOwnerKey}
                        open={composerPicker === 'permission'}
                        onOpenChange={open => setComposerPicker(open ? 'permission' : null)}
                        value={executionSettings}
                        applying={executionSettingsApplying}
                        legacy={executionSettingsLegacy}
                        unavailable={executionSettingsUnavailable}
                        onChange={patch => { if (composerOwnerRevision === composerOwnerRevisionRef.current) onExecutionSettingsChange(patch) }}
                        onOpenPermissionSettings={onOpenPermissionSettings ? () => { if (composerOwnerRevision === composerOwnerRevisionRef.current) onOpenPermissionSettings() } : undefined}
                      />
                    ) : null}
                    {mode === 'plan' ? (
                      <span
                        className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full bg-ds-hover px-2.5 text-[13px] font-medium text-ds-muted"
                        title={t('slashCommandPlanTitle')}
                      >
                        <ListTodo className="h-3.5 w-3.5" strokeWidth={1.9} />
                        <span>{t('slashCommandPlanTitle')}</span>
                      </span>
                    ) : null}

                  </>
                ) : null}
              </div>
            ) : null}
            <div
              className="ds-composer-trailing flex min-w-0 items-center justify-end"
              data-stretch={stretchModelPicker || voiceActivity || undefined}
            >
              {voiceActivity ? (
                <div className="dsh-composer-voice-activity" role="group" aria-label={t('composerVoiceStart')}>
                  <button type="button" className="dsh-composer-voice-control" onClick={dictation.cancel}
                    aria-label={t('composerVoiceCancel')} title={t('composerVoiceCancel')}><X aria-hidden="true" /></button>
                  {dictation.status === 'recording' ? <>
                    <VoiceRecordingStrip getLevel={dictation.getLevel} startedAtMs={dictation.startedAtMs} />
                    <button type="button" className="dsh-composer-voice-control" onClick={() => dictation.stop('insert')}
                      aria-label={t('composerVoiceStop')} title={t('composerVoiceStop')}><Square className="h-3 w-3 fill-current" /></button>
                  </> : <span className="dsh-composer-voice-pending" role="status"><Loader2 className="h-4 w-4 animate-spin" />{t('composerVoiceTranscribing')}</span>}
                </div>
              ) : <>
                {hideModelPicker ? null : (
                  <FloatingComposerModelPicker
                    key={composerOwnerKey}
                    open={composerPicker === 'model'}
                    onOpenChange={open => setComposerPicker(open ? 'model' : null)}
                    compact={compact}
                    mode={modelPickerMode}
                    composerModel={composerModel}
                    composerProviderId={composerProviderId}
                    composerPickList={composerPickList}
                    composerModelGroups={composerModelGroups}
                    composerReasoningEffort={composerReasoningEffort}
                    canChangeModel={canChangeModel}
                    stretch={stretchModelPicker}
                    onComposerModelChange={(model, providerId) => { if (composerOwnerRevision === composerOwnerRevisionRef.current) onComposerModelChange(model, providerId) }}
                    onComposerReasoningEffortChange={onComposerReasoningEffortChange ? effort => { if (composerOwnerRevision === composerOwnerRevisionRef.current) onComposerReasoningEffortChange(effort) } : undefined}
                    onConfigureProviders={onConfigureProviders ? () => { if (composerOwnerRevision === composerOwnerRevisionRef.current) onConfigureProviders() } : undefined}
                    onReloadComposerModels={onReloadComposerModels ? () => composerOwnerRevision === composerOwnerRevisionRef.current ? onReloadComposerModels() : Promise.resolve() : undefined}
                  />
                )}
                {showVoiceDictation ? <button type="button" disabled={!canEditComposer} onClick={dictation.toggle}
                  className="ds-composer-utility-button ds-no-drag flex shrink-0 items-center justify-center rounded-full text-ds-muted hover:bg-ds-hover hover:text-ds-ink disabled:opacity-60"
                  aria-label={t('composerVoiceStart')} title={t('composerVoiceStart')}><Mic className="h-4 w-4" strokeWidth={2} /></button> : null}
              </>}
              <button
                type="button"
                data-analytix-chat-token-scope={mainChatComposer ? '' : undefined}
                disabled={primaryActionDisabled}
                onClick={() => handlePrimaryAction()}
                className="ds-composer-primary-action-button ds-no-drag"
                aria-label={primaryActionLabel}
                title={primaryActionLabel}
              >
                {primaryActionLoading ? (
                  <Loader2 className="ds-composer-primary-action-icon animate-spin" strokeWidth={2.2} />
                ) : primaryActionIsStop ? (
                  <ComposerPrimaryStopIcon mainChat={mainChatComposer} className="ds-composer-primary-action-icon" />
                ) : (
                  <ComposerPrimarySendIcon mainChat={mainChatComposer} className="ds-composer-primary-action-icon" />
                )}
              </button>
            </div>
          </div>
        </div>
      </div>
      {showThreadUsageFooter || capacityTriggerVisible ? (
        <div className={dockCss.dock} data-composer-stats-dock data-analytix-chat-token-scope>
          {showThreadUsageFooter ? <OwnedSessionStatsPills threadId={activeThreadId} projection={threadUsageState.conversationUsage && activeThreadId ? {
            threadId: activeThreadId, billing: threadUsageState.conversationUsage.billing, metrics: threadUsageState.conversationUsage.metrics
          } : undefined} /> : null}
              {capacityTriggerVisible && contextCapacity ? (
                <div className="shrink-0" ref={capacityPanel.rootRef}>
                  <button
                    type="button"
                    ref={capacityPanel.triggerRef}
                    onClick={() => capacityPanel.setOpen(!contextCapacityOpen)}
                    className={`${dockCss.capacityTrigger} ds-composer-context ds-no-drag`}
                    aria-label={t('contextCapacityChipAria')}
                    aria-haspopup="dialog"
                    aria-expanded={contextCapacityOpen}
                    aria-controls={contextCapacityOpen ? capacityPanel.id : undefined}
                    title={t('contextCapacityTitle')}
                  >
                    <svg
                      className={dockCss.capacityRing}
                      width={CONTEXT_CAPACITY_RING_SIZE}
                      height={CONTEXT_CAPACITY_RING_SIZE}
                      viewBox={`0 0 ${CONTEXT_CAPACITY_RING_SIZE} ${CONTEXT_CAPACITY_RING_SIZE}`}
                      aria-hidden="true"
                    >
                      <circle
                        cx={CONTEXT_CAPACITY_RING_SIZE / 2}
                        cy={CONTEXT_CAPACITY_RING_SIZE / 2}
                        r={CONTEXT_CAPACITY_RING_RADIUS}
                        fill="none"
                        stroke="var(--dsw-alias-border-l3)"
                        strokeWidth={CONTEXT_CAPACITY_RING_STROKE}
                      />
                    </svg>
                    <span>{t('contextCapacityChipLabel')}</span>
                  </button>
                  {contextCapacityOpen ? createPortal(
                    <div id={capacityPanel.id} ref={capacityPanel.panelRef}
                      className={`${panelCss.panel} ${dockCss.capacityPanel}`} style={capacityPanel.style}
                      role="dialog" aria-label={t('contextCapacityTitle')}>
                      <ContextCapacityPopover capacity={contextCapacity} />
                    </div>, document.body
                  ) : null}
                </div>
              ) : null}
        </div>
      ) : null}
      {compact || presentationPhase === 'hero' || !footerHint ? null : (
        <div className="ds-composer-footer mt-1 min-h-7 px-3">
          <div className="ds-composer-footer-hint min-w-0 text-right text-[12.5px] font-medium text-ds-faint">
            <span className="block truncate">{footerHint}</span>
          </div>
        </div>
      )}
      </>}
    </div>
  )
}
