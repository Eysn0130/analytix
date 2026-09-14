import {
  useEffect,
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
  BarChart3,
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
  Send,
  Sparkles,
  Square,
  Target,
  Trash2,
  X
} from 'lucide-react'
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
export { buildResearchPrompt, parseBtwCommand, parseCompactCommand, parseGoalCommand, parseNewCommand, parseResearchCommand, parseReviewCommand } from './floating-composer-commands'
import {
  formatCompactNumber,
  formatCost,
  formatPercent,
  primaryCacheHitRate,
  useThreadUsageState
} from '../../hooks/use-thread-usage'
import type { ThreadUsageSummary } from '../../hooks/use-thread-usage'
import { buildContextCapacity, estimateBlockTokens } from '../../lib/context-capacity'
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
import { ComposerQueuedMessageList } from './above-composer/ComposerQueuedMessageList'
import { ComposerGoalRow } from './above-composer/ComposerGoalRow'
import {
  ThreadHandoffInlineProgress,
  ThreadHandoffProgressModal
} from './above-composer/ThreadHandoffProgressModal'
import { ImagePreviewLightbox } from './ImagePreviewLightbox'
import { useComposerDraft } from './use-composer-draft'
import { useSpeechToTextEnabled, useVoiceDictation } from './use-voice-dictation'
import { VoiceRecordingStrip } from './VoiceRecordingStrip'
import {
  collectCurrentTurnDiffSummary,
  type ComposerChangedFile
} from '../../lib/composer-change-summary'
import { ComposerPromptEditor, type ComposerPromptEditorHandle } from './ComposerPromptEditor'
import type { ComposerPromptMentionMetadata } from '../../lib/composer-prompt-document'

export type { ComposerFileReference } from '../../lib/composer-file-references'
export type { ComposerExecutionSettings } from './FloatingComposerExecutionPicker'

const CONTEXT_CAPACITY_RING_SIZE = 24
const CONTEXT_CAPACITY_RING_STROKE = 2.5
const CONTEXT_CAPACITY_RING_RADIUS = (CONTEXT_CAPACITY_RING_SIZE - CONTEXT_CAPACITY_RING_STROKE) / 2
const CONTEXT_CAPACITY_RING_CIRCUMFERENCE = 2 * Math.PI * CONTEXT_CAPACITY_RING_RADIUS

function contextCapacityColor(usedRatio: number): string {
  if (usedRatio >= 0.9) return '#d9544e'
  if (usedRatio >= 0.75) return '#d9920f'
  return 'var(--ds-accent)'
}

type Props = {
  variant?: 'default' | 'compact'
  className?: string
  workspaceRootOverride?: string
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
  onOpenPermissionSettings?: () => void
  hideModelPicker?: boolean
  modelPickerMode?: 'select' | 'combobox'
  hideThreadContextPanels?: boolean
  queuedMessages: QueuedUserMessage[]
  onRemoveQueuedMessage: (id: string) => void
  documentReferenceCount?: number
  attachments?: AttachmentReference[]
  attachmentUploadEnabled?: boolean
  attachmentUploadBusy?: boolean
  attachmentUploadError?: string | null
  fileReferenceEnabled?: boolean
  fileReferences?: ComposerFileReference[]
  webAccessAvailable?: boolean
  executionSettings?: ComposerExecutionSettings | null
  executionSettingsApplying?: boolean
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
  onSend: () => void
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
  /** Active model's context window, for the 上下文容量 gauge. */
  contextWindowTokens?: number
  /** Tool definitions advertised to the model (built-ins are added on top). */
  runtimeToolCount?: number
  /** Skills in the always-injected catalog. */
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

const EMPTY_CONTEXT_BLOCKS: ChatBlock[] = []
const EMPTY_MODEL_GROUPS: ModelProviderModelGroup[] = []
const EMPTY_ATTACHMENTS: AttachmentReference[] = []
const EMPTY_FILE_REFERENCES: ComposerFileReference[] = []
const EMPTY_CHANGED_FILES: ComposerChangedFile[] = []
const EMPTY_SKILL_COMMANDS: SkillCommand[] = []
const EMPTY_HUB_AGENT_PLUGINS: HubAgentPluginListItem[] = []
const MAX_COMPOSER_AT_MENTION_ITEMS = 8

function isPendingBlockingRequestBlock(block: ChatBlock): boolean {
  return (
    (block.kind === 'approval' && block.status === 'pending') ||
    (block.kind === 'user_input' && block.status === 'pending')
  )
}

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
      className="ds-no-drag relative block h-20 w-20 overflow-hidden rounded-lg border border-ds-border-muted bg-ds-card shadow-sm"
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
          className="absolute right-1 top-1 flex h-5 w-5 items-center justify-center rounded-full bg-zinc-950 text-white shadow-sm transition hover:bg-zinc-800"
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

function ComposerPrimarySendIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg
      width={20}
      height={20}
      viewBox="0 0 20 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden
    >
      <path
        d="M9.33467 16.6663V4.93978L4.6374 9.63704L4.1667 9.16634L3.69599 8.69661L9.52998 2.86263L9.63447 2.77767C9.8925 2.60753 10.2433 2.63564 10.4704 2.86263L16.3034 8.69661L16.3884 8.80111C16.5588 9.05922 16.5306 9.40982 16.3034 9.63704C16.0762 9.86414 15.7255 9.89242 15.4675 9.722L15.363 9.63704L10.6647 4.9388V16.6663C10.6647 17.0336 10.367 17.3314 9.99971 17.3314C9.63259 17.3312 9.33467 17.0335 9.33467 16.6663ZM4.6374 9.63704C4.3777 9.89674 3.95569 9.89674 3.69599 9.63704C3.43657 9.37744 3.43668 8.95628 3.69599 8.69661L4.6374 9.63704Z"
        fill="currentColor"
      />
    </svg>
  )
}

function ComposerPrimaryStopIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg
      width={20}
      height={20}
      viewBox="0 0 20 20"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden
    >
      <path d="M4.5 5.75C4.5 5.05964 5.05964 4.5 5.75 4.5H14.25C14.9404 4.5 15.5 5.05964 15.5 5.75V14.25C15.5 14.9404 14.9404 15.5 14.25 15.5H5.75C5.05964 15.5 4.5 14.9404 4.5 14.25V5.75Z" />
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
    preventDefault()
    if (onPasteClipboardImage) {
      void onPasteClipboardImage({ silentNoImage: false })
      return true
    }
    onPickAttachments?.(files)
    return true
  }
  if (!onPasteClipboardImage) return false

  const shouldPreventDefault = !hasPlainText || hasImageTransfer
  if (shouldPreventDefault) preventDefault()
  void onPasteClipboardImage({ silentNoImage: !shouldPreventDefault })
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

export type ComposerThreadUsageDisplay = {
  tokens: string
  cost: string
  saved: string
  cache: string
  primaryCache: string
  latestCache: string
  cached: string
  miss: string
  turns: number
  showContextSavings: boolean
  showCache: boolean
  showLatestCache: boolean
}

export function buildComposerThreadUsageDisplay(
  threadUsage: ThreadUsageSummary,
  locale: string
): ComposerThreadUsageDisplay {
  return {
    tokens: formatCompactNumber(threadUsage.totalTokens),
    cost: formatCost(threadUsage.costUsd, locale, threadUsage.costCny, threadUsage.priceConfigured),
    saved: formatCompactNumber(threadUsage.tokenEconomySavingsTokens),
    cache: formatPercent(threadUsage.cacheHitRate),
    primaryCache: formatPercent(primaryCacheHitRate(threadUsage)),
    latestCache: formatPercent(threadUsage.lastTurnCacheHitRate),
    cached: formatCompactNumber(threadUsage.cachedTokens),
    miss: formatCompactNumber(threadUsage.cacheMissTokens),
    turns: threadUsage.turns,
    showContextSavings: threadUsage.tokenEconomySavingsTokens > 0,
    showCache: threadUsage.turns > 1,
    showLatestCache: threadUsage.lastTurnCacheHitRate != null
  }
}

export function buildComposerTurnChipText(
  threadUsageDisplay: Pick<ComposerThreadUsageDisplay, 'turns'> | null
): string {
  if (!threadUsageDisplay || !Number.isFinite(threadUsageDisplay.turns)) return '-'
  return String(Math.max(0, Math.round(threadUsageDisplay.turns)))
}

export function FloatingComposer({
  variant = 'default',
  className,
  workspaceRootOverride,
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
  fileReferenceEnabled = false,
  fileReferences = EMPTY_FILE_REFERENCES,
  executionSettings = null,
  executionSettingsApplying = false,
  changedFiles = EMPTY_CHANGED_FILES,
  changedFileStats = null,
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
  contextWindowTokens,
  runtimeToolCount,
  runtimeSkillCount
}: Props): ReactElement {
  const { t, i18n } = useTranslation('common')
  const route = useChatStore((s) => s.route)
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const activeThreadId = useChatStore((s) => s.activeThreadId)
  const usageRefreshKey = useChatStore((s) => s.usageRefreshKey)
  const lastTurnUsage = useChatStore((s) => s.lastTurnUsage)
  const threads = useChatStore((s) => s.threads)
  const compactActiveThread = useChatStore((s) => s.compactActiveThread)
  const forkActiveThread = useChatStore((s) => s.forkActiveThread)
  const archiveThread = useChatStore((s) => s.archiveThread)
  const activeThreadGoal = useChatStore((s) => s.activeThreadGoal)
  const setActiveThreadGoal = useChatStore((s) => s.setActiveThreadGoal)
  const setActiveThreadGoalStatus = useChatStore((s) => s.setActiveThreadGoalStatus)
  const clearActiveThreadGoal = useChatStore((s) => s.clearActiveThreadGoal)
  const activeThreadTodos = useChatStore((s) => s.activeThreadTodos)
  const currentTurnId = useChatStore((s) => s.currentTurnId)
  const hasPendingBlockingRequest = useChatStore((s) => s.blocks.some(isPendingBlockingRequestBlock))
  const currentTurnDiffBlocks = useChatStore((s) => s.blocks)
  const clawChannels = useChatStore((s) => s.clawChannels)
  const activeClawChannelId = useChatStore((s) => s.activeClawChannelId)
  const compact = variant === 'compact'
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const speechToTextEnabled = useSpeechToTextEnabled()
  const dictationInputRef = useRef(input)
  useEffect(() => {
    dictationInputRef.current = input
  }, [input])
  const dictationPrimaryActionRef = useRef<(() => void) | null>(null)
  const dictation = useVoiceDictation({
    onText: (text, intent) => {
      const existing = dictationInputRef.current.replace(/\s+$/, '')
      setInput(existing ? `${existing} ${text}` : text)
      if (intent === 'send') {
        // 等 setInput 的重渲染落地后再走正常的发送路径,
        // 这样语音直发和手动点发送行为完全一致。
        window.setTimeout(() => dictationPrimaryActionRef.current?.(), 0)
      }
    }
  })
  const showVoiceDictation = speechToTextEnabled
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
  const showThreadUsageFooter = !compact && route === 'chat' && Boolean(activeThreadId) && runtimeReady
  const threadUsageState = useThreadUsageState(
    activeThreadId,
    showThreadUsageFooter,
    `${activeThread?.updatedAt ?? ''}:${busy ? 'busy' : 'idle'}:${usageRefreshKey}`
  )
  const threadUsage = threadUsageState.usage
  const threadUsageDisplay = useMemo(
    () => threadUsage ? buildComposerThreadUsageDisplay(threadUsage, i18n.language) : null,
    [threadUsage, i18n.language]
  )
  const threadUsageTurnChipText = useMemo(
    () => buildComposerTurnChipText(threadUsageDisplay),
    [threadUsageDisplay]
  )
  const threadUsageFooterTitle = useMemo(() => {
    if (!threadUsageDisplay) return null
    return [
      threadUsageDisplay.showCache
        ? t('sessionUsageCache', { cache: threadUsageDisplay.primaryCache })
        : null,
      t('sessionUsageTurns', { turns: threadUsageDisplay.turns })
    ].filter((part): part is string => Boolean(part)).join(' · ')
  }, [threadUsageDisplay, t])
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

  const canEditComposer = route === 'claw' ? clawHasInboundConversation : true
  const canCompose = runtimeReady && (
    route === 'claw'
      ? clawHasInboundConversation
      : (hasActiveThread || !!effectiveWorkspaceRoot)
  )
  const canChangeModel = canCompose && !busy
  const canSend = canCompose && (
    input.trim().length > 0 ||
    documentReferenceCount > 0 ||
    (attachmentUploadEnabled && attachments.length > 0) ||
    (fileReferenceEnabled && fileReferences.length > 0)
  )
  const canPickAttachment = canCompose && attachmentUploadEnabled && !attachmentUploadBusy
  const canPickFileReference = canCompose && fileReferenceEnabled && Boolean(effectiveWorkspaceRoot) && Boolean(onOpenFileReferencePicker)
  const canPickLocalFileReference = canCompose && fileReferenceEnabled && Boolean(onPickFileReferences)
  const showIntentToolbar = !compact && route === 'chat'
  const showComposerMenuButton = showIntentToolbar
  const canTogglePlanMode = canCompose && Boolean(onPlanCommand)
  const canCreateNewThread = runtimeReady && route !== 'claw' && Boolean(effectiveWorkspaceRoot) && Boolean(onNewCommand)
  const canOpenGoalPanel = canCompose && route !== 'claw'
  const canRunReview = canCompose && route !== 'claw' && Boolean(onReviewCommand)
  const canToggleWorktreeMode = canCompose && route !== 'claw' && Boolean(onToggleWorktreeMode)
  const canOpenComposerMenu = showComposerMenuButton
    && (canPickFileReference || canPickLocalFileReference || canTogglePlanMode || canCreateNewThread || canOpenGoalPanel || canRunReview || canToggleWorktreeMode)
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
  const slashQuery = getSlashQuery(input)
  const [composerCursor, setComposerCursor] = useState(() => input.length)
  const [selectedCommandIndex, setSelectedCommandIndex] = useState(0)
  const [agentPlugins, setAgentPlugins] = useState<HubAgentPluginListItem[]>(EMPTY_HUB_AGENT_PLUGINS)
  const [agentPluginsLoading, setAgentPluginsLoading] = useState(false)
  const [fileMentionSuggestions, setFileMentionSuggestions] = useState<ComposerFileReference[]>([])
  const [fileMentionLoading, setFileMentionLoading] = useState(false)
  const [selectedMentionIndex, setSelectedMentionIndex] = useState(0)
  const [dismissedMentionKey, setDismissedMentionKey] = useState<string | null>(null)
  const [composerMenuOpen, setComposerMenuOpen] = useState(false)
  const [goalPanelOpen, setGoalPanelOpen] = useState(false)
  const [contextCapacityOpen, setContextCapacityOpen] = useState(false)
  const [goalRuntimeNowMs, setGoalRuntimeNowMs] = useState(() => Date.now())
  const composerRootRef = useRef<HTMLDivElement | null>(null)
  const composerMenuButtonRef = useRef<HTMLButtonElement | null>(null)
  const composerMenuPanelRef = useRef<HTMLDivElement | null>(null)
  const goalPanelRef = useRef<HTMLDivElement | null>(null)
  const contextCapacityRef = useRef<HTMLDivElement | null>(null)
  const messageTokenCacheRef = useRef<WeakMap<object, number>>(new WeakMap())
  // Cache the last-known runtime capacity inputs. `runtimeInfo` (and thus these
  // props) goes null whenever the runtime drops/reconnects; without caching, the
  // chip would vanish ("context 没有了") and flap in/out as the connection flaps,
  // which itself reads as flicker. Writing refs during render is idempotent here.
  const lastKnownWindowRef = useRef(0)
  if (typeof contextWindowTokens === 'number' && contextWindowTokens > 0) {
    lastKnownWindowRef.current = contextWindowTokens
  }
  const lastKnownToolCountRef = useRef(0)
  if (typeof runtimeToolCount === 'number') lastKnownToolCountRef.current = runtimeToolCount
  const lastKnownSkillCountRef = useRef(0)
  if (typeof runtimeSkillCount === 'number') lastKnownSkillCountRef.current = runtimeSkillCount
  const effectiveContextWindow =
    typeof contextWindowTokens === 'number' && contextWindowTokens > 0
      ? contextWindowTokens
      : lastKnownWindowRef.current
  const effectiveToolCount =
    typeof runtimeToolCount === 'number' ? runtimeToolCount : lastKnownToolCountRef.current
  const effectiveSkillCount =
    typeof runtimeSkillCount === 'number' ? runtimeSkillCount : lastKnownSkillCountRef.current
  const canShowContextCapacity =
    !compact && route === 'chat' && Boolean(activeThreadId) && effectiveContextWindow > 0
  // Freeze the measured total for the duration of a turn: the runtime can emit
  // several `usage` events while streaming, and tracking them live makes the
  // chip jitter (visible flicker). Adopt the latest value only while idle.
  const liveMeasuredTotal =
    lastTurnUsage && lastTurnUsage.threadId === activeThreadId
      ? lastTurnUsage.snapshot.inputTokens
      : null
  const measuredTotalRef = useRef<number | null>(null)
  if (!busy) measuredTotalRef.current = liveMeasuredTotal
  const measuredContextTotal = busy ? measuredTotalRef.current : liveMeasuredTotal
  // The message estimate only feeds the per-category split (popover) or the
  // no-measured-total fallback. Never subscribe to `blocks` while streaming with
  // the popover closed — blocks churn on every delta and re-render the whole
  // composer. Freeze the last estimate in a ref instead.
  const needMessageEstimate =
    canShowContextCapacity && (contextCapacityOpen || measuredContextTotal == null)
  const subscribeContextBlocks = needMessageEstimate && (contextCapacityOpen || !busy)
  const contextBlocks = useChatStore((s) => (subscribeContextBlocks ? s.blocks : EMPTY_CONTEXT_BLOCKS))
  const conversationTokensRef = useRef(0)
  const conversationTokens = useMemo(() => {
    if (!subscribeContextBlocks) return conversationTokensRef.current
    // Cache per block: block identity is preserved for unchanged history across
    // streaming updates, so only the block that changed is re-estimated.
    const cache = messageTokenCacheRef.current
    let sum = 0
    for (const block of contextBlocks) {
      let cached = cache.get(block)
      if (cached === undefined) {
        cached = estimateBlockTokens(block)
        cache.set(block, cached)
      }
      sum += cached
    }
    conversationTokensRef.current = sum
    return sum
  }, [subscribeContextBlocks, contextBlocks])
  const contextCapacity = useMemo(() => {
    if (!canShowContextCapacity) return null
    return buildContextCapacity({
      windowTokens: effectiveContextWindow,
      lastTurnInputTokens: measuredContextTotal,
      messageTokens: conversationTokens,
      toolCount: effectiveToolCount,
      skillCount: effectiveSkillCount
    })
  }, [
    canShowContextCapacity,
    effectiveContextWindow,
    measuredContextTotal,
    conversationTokens,
    effectiveToolCount,
    effectiveSkillCount
  ])
  const showContextCapacity = canShowContextCapacity && Boolean(contextCapacity)
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
              : hasActiveThread
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
            : t('composerSlashHint')
  const slashCommands = useMemo<SlashCommand[]>(() => {
    const threadActionDisabled = !runtimeReady || busy || !activeThreadId
    const goalActionDisabled = !canOpenGoalPanel
    const disabledSkills = disabledSkillIdSet(disabledSkillIds)
    const commands: SlashCommand[] = []
    if (route !== 'claw') {
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
        icon: <ListTodo className="h-4 w-4" strokeWidth={1.9} />
      })
    }

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

      commands.push({
        id: 'goal',
        title: t('slashCommandGoalTitle'),
        description: t('slashCommandGoalDescription'),
        keywords: ['goal', 'objective', 'target', '目标', '任务'],
        icon: <Target className="h-4 w-4" strokeWidth={1.9} />,
        disabled: goalActionDisabled
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
    disabledSkillIds,
    t
  ])

  const filteredSlashCommands = useMemo(() => {
    if (slashQuery == null) return []
    if (!slashQuery) return slashCommands
    return slashCommands.filter((command) => {
      const haystack = [command.id, command.title, command.description, ...command.keywords]
      return haystack.some((part) => part.toLowerCase().includes(slashQuery))
    })
  }, [slashCommands, slashQuery])

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
  const showAtMentionMenu =
    canCompose &&
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
    && goalPanelDraftObjective.length > 0
  const busyCanSendFollowup = busy && input.trim().length > 0
  const primaryActionLabel = busy
    ? busyCanSendFollowup
      ? t('composerSteerSend')
      : t('interrupt')
    : highlightedSlashCommand
    ? t('slashCommandApply')
    : canSetGoalPanelDraft
      ? t('goalSetCurrentInput')
      : t('send')
  const primaryActionDisabled = busy
    ? false
    : highlightedSlashCommand
    ? highlightedSlashCommand.disabled === true
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
  const goalMenuChecked = activeThreadGoal?.status === 'active'

  useEffect(() => {
    setSelectedCommandIndex(0)
  }, [slashQuery])

  useEffect(() => {
    setSelectedMentionIndex(0)
  }, [activeAtMentionKey])

  useEffect(() => {
    if (route === 'claw') {
      setAgentPlugins(EMPTY_HUB_AGENT_PLUGINS)
      setAgentPluginsLoading(false)
      return
    }
    if (!showAtMentionMenu) {
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
  }, [route, showAtMentionMenu])

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
      if (event.key !== 'Escape') return
      setComposerMenuOpen(false)
      setGoalPanelOpen(false)
    }

    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [composerMenuOpen, goalPanelOpen])

  useEffect(() => {
    if (!contextCapacityOpen) return
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (!(target instanceof Node)) return
      if (contextCapacityRef.current?.contains(target)) return
      setContextCapacityOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') setContextCapacityOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [contextCapacityOpen])

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

  const applySlashCommand = (commandId: SlashCommandId): void => {
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
    draft.focusComposer()
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
    if (activeThreadGoal?.status === 'active') {
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

  const handlePrimaryAction = (): void => {
    if (busy && !busyCanSendFollowup) {
      onInterrupt()
      return
    }
    if (busyCanSendFollowup) {
      onSend()
      return
    }
    if (highlightedSlashCommand) {
      if (highlightedSlashCommand.disabled) return
      applySlashCommand(highlightedSlashCommand.id)
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
    onSend()
  }
  dictationPrimaryActionRef.current = primaryActionDisabled ? null : handlePrimaryAction

  const handleComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement | HTMLDivElement>): void => {
    const sendByEnter =
      event.key === 'Enter' && !event.shiftKey && !event.metaKey && !event.ctrlKey
    const composing = draft.isComposingEvent(event)

    if (!composing && showAtMentionMenu) {
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
      if ((event.key === 'Enter' || event.key === 'Tab') && highlightedMention) {
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
        setInput('')
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
    handlePrimaryAction()
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
    if (compact || route !== 'chat' || !canEditComposer) return
    const active = document.activeElement
    const activeIsExternalEditor =
      active instanceof HTMLElement &&
      Boolean(active.closest("input,textarea,select,[contenteditable='true']")) &&
      !composerRootRef.current?.contains(active)
    if (activeIsExternalEditor) return

    const frame = window.requestAnimationFrame(() => {
      const current = document.activeElement
      const currentIsExternalEditor =
        current instanceof HTMLElement &&
        Boolean(current.closest("input,textarea,select,[contenteditable='true']")) &&
        !composerRootRef.current?.contains(current)
      if (!currentIsExternalEditor) {
        draft.textareaRef.current?.focus()
      }
    })

    return () => window.cancelAnimationFrame(frame)
  }, [activeThreadId, canEditComposer, compact, route, runtimeReady, draft.textareaRef])

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

  const handleComposerDrop = (event: ReactDragEvent<HTMLDivElement>): void => {
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

  const renderMentionIcon = (suggestion: ComposerMentionSuggestion, active: boolean): ReactElement => {
    const iconFrameClass = `flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-[10px] ${
      active ? 'bg-white text-accent shadow-sm dark:bg-ds-card' : 'bg-ds-hover text-ds-muted'
    }`
    if (suggestion.kind === 'plugin') {
      if (suggestion.iconUrl) {
        return (
          <span className={iconFrameClass}>
            <img
              src={suggestion.iconUrl}
              alt=""
              className="h-full w-full object-contain"
              onError={(event) => {
                event.currentTarget.style.display = 'none'
              }}
            />
          </span>
        )
      }
      return (
        <span
          className={iconFrameClass}
          style={suggestion.brandColor ? { backgroundColor: suggestion.brandColor, color: '#fff' } : undefined}
        >
          <Plug className="h-4 w-4" strokeWidth={1.8} />
        </span>
      )
    }
    if (suggestion.kind === 'skill') {
      return (
        <span className={iconFrameClass}>
          <Sparkles className="h-4 w-4" strokeWidth={1.8} />
        </span>
      )
    }
    return (
      <span className={iconFrameClass}>
        {isComposerDirectoryReference(suggestion.reference) ? (
          <Folder className="h-4 w-4" strokeWidth={1.8} />
        ) : (
          <FileText className="h-4 w-4" strokeWidth={1.8} />
        )}
      </span>
    )
  }

  return (
    <div
      ref={composerRootRef}
      className={compact
        ? `ds-floating-composer ds-no-drag pointer-events-auto w-full pb-0 pt-0 ${className ?? ''}`
        : `ds-floating-composer ds-no-drag ds-chat-column-inset pointer-events-auto w-full max-w-4xl pb-3 pt-0 ${className ?? ''}`}
    >
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
            <ComposerQueuedMessageList messages={queuedMessages} onRemove={onRemoveQueuedMessage} />
            <ComposerGoalRow
              goal={activeThreadGoal}
              elapsedLabel={goalElapsedLabel}
              onEdit={() => {
                setGoalPanelOpen(true)
                draft.focusComposer()
              }}
              onToggleStatus={() => {
                if (activeThreadGoal) {
                  void setActiveThreadGoalStatus(activeThreadGoal.status === 'active' ? 'paused' : 'active')
                }
              }}
              onClear={() => {
                void clearActiveThreadGoal()
              }}
            />
            <ThreadHandoffInlineProgress />
          </AboveComposerPanelStack>
        </>
      )}

      <div className="relative">
        {hideThreadContextPanels ? null : <ThreadHandoffProgressModal />}

        {composerMenuOpen && slashQuery == null ? (
          <div
            ref={composerMenuPanelRef}
            className="absolute bottom-12 left-1 z-40 w-48 overflow-hidden rounded-[18px] border border-ds-border bg-white py-1.5 text-[13px] text-ds-muted shadow-[0_18px_48px_rgba(20,47,95,0.16)] dark:bg-ds-card"
          >
            {fileReferenceEnabled ? (
              <button
                type="button"
                disabled={!canPickLocalFileReference}
                onClick={handleLocalFileReferenceMenuClick}
                className="ds-no-drag flex h-8 w-full items-center gap-2 px-3 text-left transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent disabled:hover:text-ds-muted"
              >
                <FileText className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
                <span className="min-w-0 flex-1 truncate">{t('composerAddLocalFiles')}</span>
              </button>
            ) : null}
            {fileReferenceEnabled ? (
              <button
                type="button"
                disabled={!canPickFileReference}
                onClick={handleFileReferenceMenuClick}
                className="ds-no-drag flex h-8 w-full items-center gap-2 px-3 text-left transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent disabled:hover:text-ds-muted"
              >
                <Paperclip className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
                <span className="min-w-0 flex-1 truncate">{t('composerBrowseWorkspaceFiles')}</span>
              </button>
            ) : null}
            {attachmentUploadEnabled ? (
              <>
                {fileReferenceEnabled ? <div className="my-1 h-px bg-ds-border-muted/70" /> : null}
                <button
                  type="button"
                  disabled={!canPickAttachment || !onPickAttachments}
                  onClick={handleAttachmentMenuClick}
                  className="ds-no-drag flex h-8 w-full items-center gap-2 px-3 text-left transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent disabled:hover:text-ds-muted"
                >
                  {attachmentUploadBusy ? (
                    <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" strokeWidth={1.9} />
                  ) : (
                    <ImagePlus className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
                  )}
                  <span className="min-w-0 flex-1 truncate">{t('composerAddImage')}</span>
                </button>
                <div className="my-1 h-px bg-ds-border-muted/70" />
              </>
            ) : null}
            <button
              type="button"
              disabled={!canTogglePlanMode}
              onClick={handlePlanToolbarClick}
              className="ds-no-drag flex h-8 w-full items-center gap-2 px-3 text-left transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent disabled:hover:text-ds-muted"
            >
              <ListTodo className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
              <span className="min-w-0 flex-1 truncate">{t('composerMenuPlanMode')}</span>
              <span
                role="switch"
                aria-checked={mode === 'plan'}
                className={`relative h-5 w-9 shrink-0 rounded-full ring-1 transition ${
                  mode === 'plan'
                    ? 'bg-accent ring-accent/35 shadow-[inset_0_1px_0_rgba(255,255,255,0.24)]'
                    : 'bg-ds-border-muted ring-ds-border-muted'
                }`}
              >
                <span
                  className={`absolute top-0.5 h-4 w-4 rounded-full bg-white ring-1 ring-black/5 transition ${
                    mode === 'plan' ? 'translate-x-[17px]' : 'translate-x-0.5'
                  } shadow-[0_1px_4px_rgba(20,47,95,0.28)]`}
                />
              </span>
            </button>
            <button
              type="button"
              disabled={!canOpenGoalPanel}
              onClick={handleGoalMenuClick}
              className="ds-no-drag flex h-8 w-full items-center gap-2 px-3 text-left transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent disabled:hover:text-ds-muted"
            >
              <Target className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
              <span className="min-w-0 flex-1 truncate">{t('composerMenuPursueGoal')}</span>
              <span
                role="switch"
                aria-checked={goalMenuChecked}
                className={`relative h-5 w-9 shrink-0 rounded-full ring-1 transition ${
                  goalMenuChecked
                    ? 'bg-accent ring-accent/35 shadow-[inset_0_1px_0_rgba(255,255,255,0.24)]'
                    : 'bg-ds-border-muted ring-ds-border-muted'
                }`}
              >
                <span
                  className={`absolute top-0.5 h-4 w-4 rounded-full bg-white ring-1 ring-black/5 transition ${
                    goalMenuChecked ? 'translate-x-[17px]' : 'translate-x-0.5'
                  } shadow-[0_1px_4px_rgba(20,47,95,0.28)]`}
                />
              </span>
            </button>
            {canToggleWorktreeMode ? (
              <button
                type="button"
                disabled={!canToggleWorktreeMode}
                onClick={handleWorktreeToolbarClick}
                className="ds-no-drag flex h-8 w-full items-center gap-2 px-3 text-left transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent disabled:hover:text-ds-muted"
              >
                <GitBranch className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
                <span className="min-w-0 flex-1 truncate">{t('composerMenuWorktreeMode')}</span>
                <span
                  role="switch"
                  aria-checked={useWorktreePool}
                  className={`relative h-5 w-9 shrink-0 rounded-full ring-1 transition ${
                    useWorktreePool
                      ? 'bg-accent ring-accent/35 shadow-[inset_0_1px_0_rgba(255,255,255,0.24)]'
                      : 'bg-ds-border-muted ring-ds-border-muted'
                  }`}
                >
                  <span
                    className={`absolute top-0.5 h-4 w-4 rounded-full bg-white ring-1 ring-black/5 transition ${
                      useWorktreePool ? 'translate-x-[17px]' : 'translate-x-0.5'
                    } shadow-[0_1px_4px_rgba(20,47,95,0.28)]`}
                  />
                </span>
              </button>
            ) : null}
          </div>
        ) : null}

        {slashQuery != null ? (
          <div className="ds-card-strong absolute bottom-full left-1/2 z-30 mb-2 w-[calc(100%_-_1rem)] max-w-[760px] -translate-x-1/2 overflow-hidden rounded-[16px] p-1.5 shadow-[0_18px_46px_rgba(20,47,95,0.14)]">
            <div className="flex h-7 items-center px-2.5 text-[11.5px] font-semibold text-ds-muted">
              {t('slashCommandMenuTitle')}
            </div>
            {filteredSlashCommands.length > 0 ? (
              <div className="flex max-h-[min(300px,calc(100vh-260px))] flex-col gap-0.5 overflow-y-auto pr-1">
                {filteredSlashCommands.map((command) => {
                  const active = highlightedSlashCommand?.id === command.id
                  return (
                    <button
                      key={command.id}
                      type="button"
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => applySlashCommand(command.id)}
                      disabled={command.disabled}
                      className={`flex min-h-[52px] w-full items-center gap-2.5 rounded-[12px] px-2.5 py-2 text-left transition disabled:cursor-not-allowed disabled:opacity-45 ${
                        active && !command.disabled
                          ? 'bg-ds-hover text-ds-ink shadow-[inset_0_0_0_1px_rgba(20,47,95,0.06)]'
                          : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink disabled:hover:bg-transparent disabled:hover:text-ds-muted'
                      }`}
                    >
                      <span
                        className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-[10px] ${
                          active && !command.disabled ? 'bg-white text-accent shadow-sm dark:bg-ds-card' : 'bg-ds-hover text-ds-muted'
                        }`}
                      >
                        {command.icon}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-[13.5px] font-semibold leading-5 text-inherit">
                          {command.title}
                        </span>
                        <span className="mt-0.5 block truncate text-[12px] leading-4 text-ds-faint">
                          {command.description}
                        </span>
                      </span>
                      <span className="hidden min-w-[106px] shrink-0 flex-col items-end gap-1 sm:flex">
                        {command.scopeLabel ? (
                          <span className="text-[10.5px] font-semibold leading-none text-ds-muted">
                            {command.scopeLabel}
                          </span>
                        ) : null}
                        <span className="max-w-[150px] truncate rounded-full border border-ds-border-muted px-2 py-0.5 text-[10.5px] font-semibold leading-4 text-ds-faint">
                          {command.badge ?? `/${command.id}`}
                        </span>
                      </span>
                    </button>
                  )
                })}
              </div>
            ) : (
              <div className="rounded-[12px] border border-dashed border-ds-border-muted px-3 py-3 text-[12px] text-ds-faint">
                {t('slashCommandEmpty')}
              </div>
            )}
          </div>
        ) : null}

        {showAtMentionMenu ? (
          <div className="ds-card-strong absolute bottom-full left-1/2 z-30 mb-2 w-[calc(100%_-_1rem)] max-w-[680px] -translate-x-1/2 overflow-hidden rounded-[16px] p-1.5 shadow-[0_18px_46px_rgba(20,47,95,0.14)]">
            <div className="flex h-7 items-center gap-2 px-2.5 text-[11.5px] font-semibold text-ds-muted">
              <Plug className="h-3.5 w-3.5 text-ds-faint" strokeWidth={1.9} />
              <span>{t('composerAtMentionMenuTitle')}</span>
              {atMentionLoading ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin text-ds-faint" strokeWidth={1.9} />
              ) : null}
            </div>
            {flatMentionSuggestions.length > 0 ? (
              <div className="flex max-h-[min(300px,calc(100vh-260px))] flex-col gap-1 overflow-y-auto pr-1">
                {mentionSections.map((section) => (
                  <div key={section.key} className="flex flex-col gap-0.5">
                    <div className="px-2.5 pt-1 text-[10.5px] font-semibold text-ds-faint">
                      {section.title}
                    </div>
                    {section.items.map((suggestion) => {
                      const active = highlightedMention?.key === suggestion.key
                      return (
                        <button
                          key={suggestion.key}
                          type="button"
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => applyMentionSuggestion(suggestion)}
                          className={`flex min-h-[46px] w-full items-center gap-2.5 rounded-[12px] px-2.5 py-2 text-left transition ${
                            active
                              ? 'bg-ds-hover text-ds-ink shadow-[inset_0_0_0_1px_rgba(20,47,95,0.06)]'
                              : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
                          }`}
                        >
                          {renderMentionIcon(suggestion, active)}
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-[13.5px] font-semibold leading-5 text-inherit">
                              {suggestion.title}
                            </span>
                            <span className="mt-0.5 block truncate text-[12px] leading-4 text-ds-faint">
                              {suggestion.description}
                            </span>
                          </span>
                          <span className="hidden max-w-[190px] shrink-0 truncate rounded-full border border-ds-border-muted px-2 py-0.5 text-[10.5px] font-semibold leading-4 text-ds-faint sm:block">
                            {suggestion.badge}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                ))}
              </div>
            ) : (
              <div className="rounded-[12px] border border-dashed border-ds-border-muted px-3 py-3 text-[12px] text-ds-faint">
                {atMentionLoading ? t('composerAtMentionLoading') : t('composerAtMentionEmpty')}
              </div>
            )}
          </div>
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
                  {activeThreadGoal?.status === 'active' ? (
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
                  ) : activeThreadGoal ? (
                    <button
                      type="button"
                      onClick={() => {
                        setGoalPanelOpen(false)
                        void setActiveThreadGoalStatus('active')
                      }}
                      className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-ds-border bg-ds-card text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                      aria-label={t('goalActionResume')}
                      title={t('goalActionResume')}
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
          className={`ds-composer-shell ds-chat-composer ds-frosted ds-no-drag flex flex-col gap-1 px-3 pb-2 pt-2 transition ${
            draft.focused ? 'ds-chat-composer-focus' : ''
          } ${compact ? 'rounded-[24px] px-3 py-2 shadow-none' : ''}`}
          onMouseDown={handleComposerShellMouseDown}
          onPaste={handleComposerPaste}
          onDragOver={handleComposerDragOver}
          onDrop={handleComposerDrop}
        >
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
          {fileReferences.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2 px-1">
              {fileReferences.map((reference) => {
                const isDirectory = isComposerDirectoryReference(reference)
                const displayPath = isDirectory ? `${reference.relativePath}/` : reference.relativePath
                return (
                  <span
                    key={`${reference.type ?? 'file'}:${reference.relativePath}`}
                    className="ds-no-drag inline-flex h-7 max-w-full items-center gap-1.5 rounded-lg border border-ds-border-muted bg-ds-card/80 px-2 text-[12px] font-medium text-ds-muted"
                    title={displayPath}
                  >
                    {isDirectory ? (
                      <Folder className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    ) : (
                      <FileText className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    )}
                    <span className="max-w-52 truncate">{displayPath}</span>
                    {onRemoveFileReference ? (
                      <button
                        type="button"
                        onClick={() => removeFileReference(reference)}
                        className="rounded-full p-0.5 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                        aria-label={t('composerRemoveFileReference')}
                        title={t('composerRemoveFileReference')}
                      >
                        <X className="h-3 w-3" strokeWidth={2} />
                      </button>
                    ) : null}
                  </span>
                )
              })}
            </div>
          ) : null}
          {attachments.length > 0 || attachmentUploadError ? (
            <div className="flex flex-wrap items-center gap-2 px-1">
              {attachments.map((attachment) => (
                attachment.previewUrl ? (
                  <ComposerImageAttachmentPreview
                    key={attachment.id}
                    attachment={attachment}
                    onRemoveAttachment={onRemoveAttachment}
                  />
                ) : (
                  <span
                    key={attachment.id}
                    className="ds-no-drag inline-flex h-7 max-w-full items-center gap-1.5 rounded-lg border border-ds-border-muted bg-ds-card/80 px-2 text-[12px] font-medium text-ds-muted"
                    title={attachment.name || attachment.id}
                  >
                    {attachment.kind === 'document' ? (
                      <FileText className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    ) : (
                      <ImagePlus className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
                    )}
                    <span className="max-w-40 truncate">{attachment.name || attachment.id}</span>
                    {attachment.kind === 'document' && attachment.pageCount ? (
                      <span className="shrink-0 text-[11px] text-ds-faint">
                        {attachment.pageCount}p{attachment.truncated ? '+' : ''}
                      </span>
                    ) : null}
                    {onRemoveAttachment ? (
                      <button
                        type="button"
                        onClick={() => onRemoveAttachment(attachment.id)}
                        className="rounded-full p-0.5 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                        aria-label={t('composerRemoveAttachment')}
                        title={t('composerRemoveAttachment')}
                      >
                        <X className="h-3 w-3" strokeWidth={2} />
                      </button>
                    ) : null}
                  </span>
                )
              ))}
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
          <div
            className={`ds-composer-toolbar flex min-h-9 items-center gap-2 ${
              showToolbarStartControls ? 'justify-between' : 'justify-end'
            }`}
          >
            {showToolbarStartControls ? (
              <div className="flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden">
                {showComposerMenuButton ? (
                  <>
                    <button
                      ref={composerMenuButtonRef}
                      type="button"
                      disabled={!canOpenComposerMenu}
                      onClick={handleComposerMenuButtonClick}
                      className={`ds-no-drag flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45 ${
                        composerMenuOpen ? 'bg-ds-hover text-ds-ink' : ''
                      }`}
                      aria-label={t('composerMenuTitle')}
                      title={t('composerMenuTitle')}
                    >
                      <Plus className="h-5 w-5" strokeWidth={1.8} />
                    </button>
                    {executionSettings && onExecutionSettingsChange ? (
                      <FloatingComposerExecutionPicker
                        value={executionSettings}
                        applying={executionSettingsApplying}
                        onChange={onExecutionSettingsChange}
                        onOpenPermissionSettings={onOpenPermissionSettings}
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
                    {activeThreadGoal?.status === 'active' ? (
                      <span
                        className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full bg-ds-hover px-2.5 text-[13px] font-medium text-ds-muted"
                        title={t('slashCommandGoalTitle')}
                      >
                        <Target className="h-3.5 w-3.5" strokeWidth={1.9} />
                        <span>{t('slashCommandGoalTitle')}</span>
                      </span>
                    ) : null}
                  </>
                ) : null}
              </div>
            ) : null}
            <div
              className={`flex min-w-0 items-center justify-end gap-1.5 ${
                stretchModelPicker || dictation.status === 'recording' ? 'flex-1' : 'shrink-0'
              }`}
            >
              {dictation.status === 'recording' ? (
                <>
                  <VoiceRecordingStrip
                    getLevel={dictation.getLevel}
                    startedAtMs={dictation.startedAtMs}
                  />
                  <button
                    type="button"
                    onClick={() => dictation.stop('insert')}
                    className="ds-no-drag flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-ds-border bg-ds-card text-ds-ink shadow-sm transition hover:bg-ds-hover"
                    aria-label={t('composerVoiceStop')}
                    title={t('composerVoiceStop')}
                  >
                    <Square className="h-3 w-3 fill-current" strokeWidth={2.4} />
                  </button>
                  <button
                    type="button"
                    onClick={() => dictation.stop('send')}
                    className="ds-no-drag flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-zinc-950 text-white shadow-[0_10px_22px_rgba(20,47,95,0.22)] transition hover:bg-zinc-800 dark:bg-white dark:text-zinc-950 dark:hover:bg-zinc-200"
                    aria-label={t('composerVoiceSend')}
                    title={t('composerVoiceSend')}
                  >
                    <Send className="h-4 w-4" strokeWidth={2.2} />
                  </button>
                </>
              ) : (
              <>
              {showContextCapacity && contextCapacity ? (
                <div className="relative shrink-0" ref={contextCapacityRef}>
                  <button
                    type="button"
                    onClick={() => setContextCapacityOpen((open) => !open)}
                    className="ds-composer-context ds-no-drag relative inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-ds-border-muted bg-ds-card p-0 text-[9px] font-semibold leading-none text-ds-muted transition hover:bg-ds-hover"
                    aria-label={[
                      t('contextCapacityChipAria', {
                        percent: formatPercent(contextCapacity.usedRatio)
                      }),
                      threadUsageDisplay
                        ? t('sessionUsageTurns', { turns: threadUsageDisplay.turns })
                        : null
                    ].filter((part): part is string => Boolean(part)).join(' · ')}
                    aria-expanded={contextCapacityOpen}
                    title={[
                      t('contextCapacityTitle'),
                      threadUsageDisplay
                        ? t('sessionUsageTurns', { turns: threadUsageDisplay.turns })
                        : null
                    ].filter((part): part is string => Boolean(part)).join(' · ')}
                  >
                    <svg
                      className="h-6 w-6 -rotate-90 shrink-0"
                      viewBox={`0 0 ${CONTEXT_CAPACITY_RING_SIZE} ${CONTEXT_CAPACITY_RING_SIZE}`}
                      aria-hidden="true"
                    >
                      <circle
                        cx={CONTEXT_CAPACITY_RING_SIZE / 2}
                        cy={CONTEXT_CAPACITY_RING_SIZE / 2}
                        r={CONTEXT_CAPACITY_RING_RADIUS}
                        fill="none"
                        stroke="var(--ds-surface-subtle)"
                        strokeWidth={CONTEXT_CAPACITY_RING_STROKE}
                      />
                      <circle
                        cx={CONTEXT_CAPACITY_RING_SIZE / 2}
                        cy={CONTEXT_CAPACITY_RING_SIZE / 2}
                        r={CONTEXT_CAPACITY_RING_RADIUS}
                        fill="none"
                        stroke={contextCapacityColor(contextCapacity.usedRatio)}
                        strokeWidth={CONTEXT_CAPACITY_RING_STROKE}
                        strokeLinecap="round"
                        strokeDasharray={CONTEXT_CAPACITY_RING_CIRCUMFERENCE}
                        strokeDashoffset={
                          CONTEXT_CAPACITY_RING_CIRCUMFERENCE *
                          (1 - Math.min(1, Math.max(0, contextCapacity.usedRatio)))
                        }
                      />
                    </svg>
                    <span className="pointer-events-none absolute inset-0 flex items-center justify-center tabular-nums">
                      {threadUsageTurnChipText}
                    </span>
                  </button>
                  {contextCapacityOpen ? (
                    <div className="absolute bottom-full right-0 z-30 mb-2">
                      <ContextCapacityPopover capacity={contextCapacity} />
                    </div>
                  ) : null}
                </div>
              ) : null}
              {hideModelPicker ? null : (
                <FloatingComposerModelPicker
                  compact={compact}
                  mode={modelPickerMode}
                  composerModel={composerModel}
                  composerProviderId={composerProviderId}
                  composerPickList={composerPickList}
                  composerModelGroups={composerModelGroups}
                  composerReasoningEffort={composerReasoningEffort}
                  canChangeModel={canChangeModel}
                  stretch={stretchModelPicker}
                  onComposerModelChange={onComposerModelChange}
                  onComposerReasoningEffortChange={onComposerReasoningEffortChange}
                  onConfigureProviders={onConfigureProviders}
                />
              )}
              {showVoiceDictation ? (
                <button
                  type="button"
                  disabled={dictation.status === 'transcribing' || !canEditComposer}
                  onClick={dictation.toggle}
                  className="ds-no-drag flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
                  aria-label={
                    dictation.status === 'transcribing'
                      ? t('composerVoiceTranscribing')
                      : t('composerVoiceStart')
                  }
                  title={
                    dictation.status === 'transcribing'
                      ? t('composerVoiceTranscribing')
                      : t('composerVoiceStart')
                  }
                >
                  {dictation.status === 'transcribing' ? (
                    <Loader2 className="h-4 w-4 animate-spin" strokeWidth={2.2} />
                  ) : (
                    <Mic className="h-4 w-4" strokeWidth={2} />
                  )}
                </button>
              ) : null}
              <button
                type="button"
                disabled={primaryActionDisabled}
                onClick={handlePrimaryAction}
                className="ds-composer-primary-action-button ds-no-drag"
                aria-label={primaryActionLabel}
                title={primaryActionLabel}
              >
                {primaryActionLoading ? (
                  <Loader2 className="ds-composer-primary-action-icon animate-spin" strokeWidth={2.2} />
                ) : busy && !busyCanSendFollowup ? (
                  <ComposerPrimaryStopIcon className="ds-composer-primary-action-icon" />
                ) : (
                  <ComposerPrimarySendIcon className="ds-composer-primary-action-icon" />
                )}
              </button>
              </>
              )}
            </div>
          </div>
        </div>
      </div>
      {compact ? null : (
        <div className="ds-composer-footer mt-1 flex min-h-7 flex-wrap items-center justify-between gap-x-2.5 gap-y-1.5 px-3">
          <div className="ds-composer-footer-left flex min-w-0 flex-1 flex-wrap items-center gap-2">
            {route === 'chat' ? (
              <WorkspaceProjectPicker currentWorkspaceRoot={effectiveWorkspaceRoot} />
            ) : null}
            <GitBranchPicker workspaceRoot={effectiveWorkspaceRoot} />
            {showThreadUsageFooter ? (
              <div
                className="ds-composer-usage ds-no-drag inline-flex min-h-7 max-w-full min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 overflow-visible rounded-lg border border-ds-border-muted bg-ds-card/72 px-2.5 py-0.5 text-[12.5px] font-medium leading-5 text-ds-muted shadow-sm"
                title={threadUsageFooterTitle ?? t('sessionUsageUnavailable')}
              >
                <BarChart3 className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.9} />
                {threadUsageDisplay ? (
                  <>
                    {threadUsageDisplay.showCache ? (
                      <span className="ds-composer-usage-cache shrink-0 truncate tabular-nums">
                        {t('sessionUsageCache', {
                          cache: threadUsageDisplay.primaryCache
                        })}
                      </span>
                    ) : null}
                    {threadUsageDisplay.showCache ? (
                      <span className="ds-composer-usage-turns-separator text-ds-faint">·</span>
                    ) : null}
                    <span className="ds-composer-usage-turns shrink-0 truncate tabular-nums">
                      {t('sessionUsageTurns', { turns: threadUsageDisplay.turns })}
                    </span>
                  </>
                ) : (
                  <span className="shrink-0 text-ds-faint">
                    {threadUsageState.loading
                      ? t('sessionUsageLoading')
                      : t('sessionUsageUnavailable')}
                  </span>
                )}
              </div>
            ) : null}
          </div>
          {footerHint ? (
            <div className="ds-composer-footer-hint min-w-0 flex-1 text-right text-[12.5px] font-medium text-ds-faint">
              <span className="block truncate">{footerHint}</span>
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}
