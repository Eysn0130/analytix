import { PresentedFiles } from './presentation/chat/PresentedFiles'
import type { PresentCall } from './presentation/chat/presented-files'
import { useInputComposition } from './use-composer-draft'
import { ToolCallTree } from './presentation/chat/ToolCallTree'
import { Tooltip } from './presentation/Tooltip'
import { DshBrowseOutlineRegular, DshCopyOutlineRegular, DshCheckOutlineRegular, DshEditOutlineRegular } from './presentation/DshToolIcons'
import { messageClock, useCalendarDay } from './presentation/chat/message-chrome'
import actionStyles from './presentation/chat/turn-state/TurnState.module.css'
import userStyles from './UserMessage.module.css'
import { TurnStatusView } from './presentation/chat/TurnStatusView'
import { ProtectedToolResultScope } from './presentation/ProtectedToolResult'
import { ApprovalPanel } from './presentation/chat/ApprovalPanel'
import { QuestionReplyView } from './presentation/chat/QuestionReplyView'
import { QuestionComposer } from './presentation/chat/QuestionComposer'
import type { ReactElement, ReactNode } from 'react'
import { createContext, memo, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy, Download, File, ImageIcon, Loader2, MessageSquareQuote, Plug, Sparkles, Video } from '../../design/AnalytixUiIcons'
import type { HubAgentPluginListItem, SkillListItem } from '@shared/analytix-api'
import { generatedArtifactMetadataSchema } from '../../../../../packages/runtime/src/contracts/generated-artifact'
import { GoalRoundSourceSchemaV1 } from '../../../../../packages/runtime/src/contracts/goal'
import { GeneratedOfficeDeliverables } from './GeneratedOfficeArtifact'
import type { AttachmentReference, ChatBlock, GeneratedFileReference, RuntimeDisclosureMetadata, ToolBlock } from '../../agent/types'
import {
  getUserMessageDisplayParts,
  type ComposerPluginMentionToken,
  type ComposerSkillMentionToken
} from '../../lib/composer-mentions'
import { useChatStore } from '../../store/chat-store'
import { isPendingActiveStreamTurnId } from '../../thread/streaming/active-stream-store'
import { getProvider } from '../../agent/registry'
import { parseWritePromptForDisplay } from '../../write/quoted-selection'
import { parseClawUserPromptForDisplay, type ClawUserPromptDisplay } from '@shared/app-settings'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import { openWorkspacePathInEditor } from '../../lib/open-workspace-path'
import { normalizeWorkspaceRoot } from '../../lib/workspace-path'
import { useValidatedFileReference } from '../../lib/file-reference-validation'
import { previewWorkspaceFile } from '../../lib/workspace-file-preview'
import { AssistantMarkdown } from './AssistantMarkdown'
import { observeTerminalDOMCommit } from '../../thread/tracing/thread-performance-trace'
import { AcceptedSlotDisplay, acceptedFinalHasLocalDisplaySlots } from './AcceptedSlotDisplay'
import { ImagePreviewLightbox } from './ImagePreviewLightbox'
import { WritePromptMetaDisclosure } from './message-timeline-cards'
import { InjectedMemoryMetaChip } from './injected-memory-meta-chip'
import {
  formatChildAgentChip,
  formatJobDiagnosticsChip,
  formatProviderDiagnosticsChip
} from './message-timeline-tools'

const COPY_FEEDBACK_RESET_MS = 1600

const EMPTY_INLINE_MENTION_PLUGINS: HubAgentPluginListItem[] = []

let inlineMentionPluginCache: HubAgentPluginListItem[] | null = null
let inlineMentionPluginPromise: Promise<HubAgentPluginListItem[]> | null = null

function normalizeInlineMentionPluginId(value: string): string {
  return value.trim().toLowerCase()
}

function hubPluginInlineMentionIds(plugin: HubAgentPluginListItem): string[] {
  const marketplace = plugin.upstreamMarketplaceName.trim() || plugin.marketplaceName.trim()
  const rawIds = [
    plugin.pluginName,
    plugin.name,
    plugin.marketplaceName,
    marketplace && plugin.pluginName ? `${plugin.pluginName}@${marketplace}` : '',
    marketplace && plugin.name ? `${plugin.name}@${marketplace}` : ''
  ]
  return rawIds.map((value) => value.trim()).filter(Boolean)
}

function hubPluginInlineMentionDisplayName(plugin: HubAgentPluginListItem): string {
  return plugin.displayName.trim() || plugin.pluginName.trim() || plugin.name.trim()
}

function hubPluginInlineMentionIconUrl(plugin: HubAgentPluginListItem): string | undefined {
  const icon = plugin.iconDataUrl?.trim() || plugin.iconUrl?.trim()
  return icon || undefined
}

function hubPluginInlineMentionBrandColor(plugin: HubAgentPluginListItem): string | undefined {
  const color = plugin.brandColor?.trim()
  return color || undefined
}

function installedInlineMentionPlugins(plugins: HubAgentPluginListItem[] | undefined): HubAgentPluginListItem[] {
  const seen = new Set<string>()
  const installed: HubAgentPluginListItem[] = []
  for (const plugin of plugins ?? []) {
    if (!plugin.installed && !plugin.requiredInstall) continue
    const ids = hubPluginInlineMentionIds(plugin)
    const key = normalizeInlineMentionPluginId(ids[0] ?? '')
    if (!key || seen.has(key)) continue
    seen.add(key)
    installed.push(plugin)
  }
  return installed
}

function findInlineMentionPlugin(
  plugins: HubAgentPluginListItem[],
  pluginId: string
): HubAgentPluginListItem | undefined {
  const requested = normalizeInlineMentionPluginId(pluginId)
  if (!requested) return undefined
  return plugins.find((plugin) =>
    hubPluginInlineMentionIds(plugin).some((id) => normalizeInlineMentionPluginId(id) === requested)
  )
}

async function loadInlineMentionPlugins(): Promise<HubAgentPluginListItem[]> {
  if (inlineMentionPluginCache) return inlineMentionPluginCache
  if (inlineMentionPluginPromise) return inlineMentionPluginPromise
  inlineMentionPluginPromise = (async () => {
    const sync = typeof window === 'undefined' ? undefined : window.analytix?.app?.syncHubAgentMarketplace
    if (typeof sync !== 'function') return EMPTY_INLINE_MENTION_PLUGINS
    const result = await sync({ mode: 'cache' })
    const plugins = installedInlineMentionPlugins(result.plugins)
    if (result.ok || plugins.length > 0) inlineMentionPluginCache = plugins
    return plugins
  })()
    .catch(() => EMPTY_INLINE_MENTION_PLUGINS)
    .finally(() => {
      inlineMentionPluginPromise = null
    })
  return inlineMentionPluginPromise
}

function useInlineMentionPlugins(enabled: boolean): HubAgentPluginListItem[] {
  const [plugins, setPlugins] = useState<HubAgentPluginListItem[]>(() =>
    inlineMentionPluginCache ?? EMPTY_INLINE_MENTION_PLUGINS
  )

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    void loadInlineMentionPlugins().then((items) => {
      if (!cancelled) setPlugins(items)
    })
    return () => {
      cancelled = true
    }
  }, [enabled])

  return plugins
}

function userReferenceOwner(state: ReturnType<typeof useChatStore.getState>, threadId: string | null): { workspace: string; key: string } {
  const thread = state.threads.find(item => item.id === threadId)
  const workspace = normalizeWorkspaceRoot(thread?.workspace) ||
    (threadId && threadId === state.activeThreadId ? normalizeWorkspaceRoot(state.workspaceRoot) : '')
  return { workspace, key: JSON.stringify([threadId, workspace]) }
}

function useInlineMentionSkills(enabled: boolean, ownerKey: string, workspaceRoot: string): SkillListItem[] {
  const [entry, setEntry] = useState<{ owner: string; skills: SkillListItem[] } | null>(null)
  useEffect(() => {
    if (!enabled || typeof window.analytix?.app?.listSkills !== 'function') return
    let cancelled = false
    void window.analytix.app.listSkills(workspaceRoot || undefined).then(result => {
      if (!cancelled) setEntry({ owner: ownerKey, skills: result.ok ? result.skills : [] })
    }).catch(() => {
      if (!cancelled) setEntry({ owner: ownerKey, skills: [] })
    })
    return () => { cancelled = true }
  }, [enabled, ownerKey, workspaceRoot])
  return entry?.owner === ownerKey ? entry.skills : []
}

function UserMessageInlineText({ text, meta, threadId }: { text: string; meta?: RuntimeDisclosureMetadata; threadId: string | null }): ReactElement {
  const loadedSkillIds = useMemo(() => metaStringArray(meta, 'activeSkillIds'), [meta])
  const parts = useMemo(() => getUserMessageDisplayParts(text, loadedSkillIds), [text, loadedSkillIds])
  const plugins = useInlineMentionPlugins(parts.some(part => part.kind === 'plugin'))
  const ownerKey = useChatStore(state => userReferenceOwner(state, threadId).key)
  const workspaceRoot = useChatStore(state => userReferenceOwner(state, threadId).workspace)
  const activeOwner = useChatStore(state => threadId !== null && state.activeThreadId === threadId)
  const skills = useInlineMentionSkills(activeOwner && parts.some(part => part.kind === 'loaded-skill' ||
    part.kind === 'skill' && loadedSkillIds.includes(part.mention.skillId)), ownerKey, workspaceRoot)
  const skillPath = (id: string): string | undefined => {
    if (!activeOwner || !loadedSkillIds.includes(id)) return undefined
    const matching = skills.filter(skill => skill.id === id)
    return matching.length === 1 ? matching[0]!.entryPath : undefined
  }

  return <>{parts.map(part => {
    if (part.kind === 'text') return part.text
    if (part.kind === 'plugin') return <UserMessageInlinePluginMention key={part.key} mention={part.mention}
      plugin={findInlineMentionPlugin(plugins, part.mention.pluginId)} />
    if (part.kind === 'skill') return <UserMessageInlineSkillMention key={part.key} mention={part.mention}
      path={skillPath(part.mention.skillId)} ownerKey={ownerKey} workspaceRoot={workspaceRoot} threadId={threadId} />
    if (part.kind === 'loaded-skill') return <UserMessageInlineReference key={part.key} kind="skill"
      title={`/${part.skillId}`} display={`/${part.skillId}`} path={skillPath(part.skillId)}
      ownerKey={ownerKey} workspaceRoot={workspaceRoot} threadId={threadId} slash />
    return <UserMessageInlineReference key={part.key} kind={part.directory ? 'folder' : 'file'}
      title={part.label} display={part.display} path={part.directory || !activeOwner ? undefined : part.path}
      ownerKey={ownerKey} workspaceRoot={workspaceRoot} threadId={threadId} />
  })}</>
}

/** Reference geometry/selection guard adapted from DSH user-text.tsx at 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
function UserMessageInlineReference({ kind, title, display, path, ownerKey, workspaceRoot, threadId, slash, icon }: {
  kind: 'file' | 'folder' | 'skill'; title: string; display: string; path?: string
  ownerKey: string; workspaceRoot: string; threadId: string | null; slash?: boolean; icon?: ReactNode
}): ReactElement {
  const target = useMemo(() => path && !Array.from(path).some(character => {
    const code = character.charCodeAt(0)
    return code <= 0x1f || code === 0x7f
  }) &&
    !/^[\\/]{2}/u.test(path) && (!/^[a-z][a-z\d+.-]*:/iu.test(path) || /^[a-z]:[\\/]/iu.test(path)) ? { path } : null, [path])
  const validation = useValidatedFileReference(target, workspaceRoot)
  const contents = <>{icon ?? (kind === 'file' ? <DshBrowseOutlineRegular className={userStyles.referenceIcon} />
    : kind === 'folder' ? <UserMessageFolderIcon /> : null)}{display}</>
  const className = `${userStyles.referenceChip}${slash ? ` ${userStyles.slashChip}` : ''}`
  if (validation.status !== 'valid') return <span className={className} title={title}
    data-ref-chip={kind} data-user-message-inline-mention={kind}>{contents}</span>
  return <button type="button" className={`${className} ${userStyles.referenceAction}`} title={title}
    data-ref-chip={kind} data-user-message-inline-mention={kind} onClick={event => {
      if (event.detail > 1 || (event.detail !== 0 && event.currentTarget.ownerDocument.getSelection()?.isCollapsed === false)) return
      const state = useChatStore.getState()
      if (!threadId || state.activeThreadId !== threadId || userReferenceOwner(state, threadId).key !== ownerKey) return
      previewWorkspaceFile({ path: validation.path, workspaceRoot })
    }}>{contents}</button>
}

/* DSH FolderCloseArtwork at 5badb15009ae1756c3afe0ae0cef1faafc290ccc; scoped to this user leaf.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
function UserMessageFolderIcon(): ReactElement {
  return <svg width={16} height={16} className={userStyles.referenceIcon} viewBox="0 0 16 16" fill="none"
    xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
    <path d="M1.50439 3.11059C1.50439 2.55831 1.95211 2.1106 2.50439 2.1106H5.43389C5.67773 2.1106 5.91318 2.19969 6.09593 2.36113L7.71649 3.79265C7.89924 3.95409 8.1347 4.04319 8.3785 4.04319H13.4958C14.0481 4.04319 14.4958 4.4909 14.4958 5.04319V12.8894C14.4958 13.4417 14.0481 13.8894 13.4958 13.8894H2.50439C1.95211 13.8894 1.50439 13.4417 1.50439 12.8894V4.04319V3.11059Z" stroke="currentColor" />
    <path d="M3.63501 7.66614H12.3647" stroke="currentColor" />
  </svg>
}

function UserMessageInlinePluginMention({
  mention,
  plugin
}: {
  mention: ComposerPluginMentionToken
  plugin?: HubAgentPluginListItem
}): ReactElement {
  const title = plugin ? hubPluginInlineMentionDisplayName(plugin) : mention.label.trim() || mention.pluginId
  const iconUrl = plugin ? hubPluginInlineMentionIconUrl(plugin) : undefined
  const brandColor = plugin ? hubPluginInlineMentionBrandColor(plugin) : undefined
  return (
    <span
      data-user-message-inline-mention="plugin"
      className={userStyles.referenceChip}
      title={title}
      style={brandColor ? { color: brandColor } : undefined}
    >
      {iconUrl ? (
        <span className={userStyles.referenceImage}>
          <img
            src={iconUrl}
            alt=""
            className="h-full w-full object-contain"
            onError={(event) => {
              event.currentTarget.style.display = 'none'
            }}
          />
        </span>
      ) : (
        <Plug className={userStyles.referenceIcon} strokeWidth={1.9} />
      )}
      <span className="leading-[inherit]">{title}</span>
    </span>
  )
}

function UserMessageInlineSkillMention({ mention, path, ownerKey, workspaceRoot, threadId }: {
  mention: ComposerSkillMentionToken; path?: string; ownerKey: string; workspaceRoot: string; threadId: string | null
}): ReactElement {
  const title = mention.label.trim() || mention.skillId
  return <UserMessageInlineReference kind="skill" title={title} display={title} path={path}
    ownerKey={ownerKey} workspaceRoot={workspaceRoot} threadId={threadId}
    icon={<Sparkles className={userStyles.referenceIcon} strokeWidth={1.8} />} />
}

/**
 * User message bubble with hover affordance to rewind/edit. Click the rewind
 * pill, the bubble flips into a textarea, and Resend submits an edited
 * version of the message — locally truncating subsequent turns and starting
 * a fresh turn on the same thread (see chat-store `rewindAndResend`).
 */
function GoalRoundSourceView({ block }: { block: Extract<ChatBlock, { kind: 'user' }> }): ReactElement {
  const { t, i18n } = useTranslation('common')
  const source = GoalRoundSourceSchemaV1.safeParse(block.source)
  const timestamp = block.createdAt && Number.isFinite(Date.parse(block.createdAt)) ? block.createdAt : undefined
  return <div data-goal-round-source className="flex min-w-0 flex-col gap-1 py-1 text-[13px] text-ds-muted">
    <div className="flex items-center gap-1.5"><Sparkles size={14} aria-hidden="true" />
      <span>{source.success ? t('goalRoundSourceTriggered', { round: source.data.round }) : t('goalRoundSourceUnavailable')}</span>
      {timestamp ? <time dateTime={timestamp} className="text-ds-faint">{formatMessageDateTime(timestamp, i18n.language)}</time> : null}
    </div>
    {source.success ? <details><summary>{t('goalRoundSourceDetails')}</summary><dl>
      <dt>{t('goalRoundSourceGoal')}</dt><dd>{source.data.goalId}</dd>
      <dt>{t('goalRoundSourceRevision')}</dt><dd>{source.data.revision}</dd>
    </dl></details> : null}
  </div>
}

function UserMessageBubble({
  block,
  threadId,
  actionsReveal = 'always'
}: {
  block: Extract<ChatBlock, { kind: 'user' }>
  threadId: string | null
  actionsReveal?: 'always' | 'hover'
}): ReactElement {
  const { t } = useTranslation('common')
  const composition = useInputComposition()
  const busy = useChatStore((s) => s.busy)
  const route = useChatStore((s) => s.route)
  const rewindAndResend = useChatStore((s) => s.rewindAndResend)
  const publicBlockText = projectOrdinaryPublicText(block.text)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(publicBlockText)
  const [writeMetaOpen, setWriteMetaOpen] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const editOriginRef = useRef<{ threadId: string | null; workspace: string; blockId: string } | null>(null)
  const parsedWritePrompt = useMemo(() => {
    if (route !== 'write' && route !== 'chat') return null
    const parsed = parseWritePromptForDisplay(publicBlockText)
    return parsed?.userInput.trim() ? parsed : null
  }, [publicBlockText, route])
  const parsedClawPrompt = useMemo(() => {
    const parsed = parseClawUserPromptForDisplay(publicBlockText)
    if (!parsed.managed && !parsed.inbound && block.managedBy !== 'claw' && route !== 'claw') return null
    return parsed
  }, [block.managedBy, publicBlockText, route])
  const metaDisplayText =
    typeof block.meta?.displayText === 'string' && block.meta.displayText.trim()
      ? projectOrdinaryPublicText(block.meta.displayText.trim())
      : null
  const displayText = projectOrdinaryPublicText(
    metaDisplayText ?? parsedWritePrompt?.userInput ?? parsedClawPrompt?.text ?? publicBlockText
  )
  const day = useCalendarDay(Boolean(block.createdAt && Number.isFinite(Date.parse(block.createdAt))))
  const clock = messageClock(block.createdAt, day, t)
  const canEdit = !metaDisplayText
  const showClawInboundCard = route === 'claw' && parsedClawPrompt?.inbound === true

  useEffect(() => {
    if (!editing) return
    const el = textareaRef.current
    if (!el) return
    el.focus()
    const len = el.value.length
    el.setSelectionRange(len, len)
    // Auto-size to content
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 360)}px`
  }, [editing])

  useEffect(() => {
    setWriteMetaOpen(false)
  }, [block.id])

  const startEdit = (): void => {
    if (busy || !canEdit) return
    const state = useChatStore.getState()
    const owner = state.threads.find(thread => thread.id === state.activeThreadId)
    editOriginRef.current = { threadId: state.activeThreadId, blockId: block.id,
      workspace: normalizeWorkspaceRoot(owner?.workspace) || normalizeWorkspaceRoot(state.workspaceRoot) }
    setDraft(displayText)
    setEditing(true)
  }

  const cancelEdit = (): void => {
    editOriginRef.current = null
    setDraft(publicBlockText)
    setEditing(false)
  }

  const submit = async (): Promise<void> => {
    const trimmed = draft.trim()
    if (!trimmed || busy) return
    const state = useChatStore.getState()
    const owner = state.threads.find(thread => thread.id === state.activeThreadId)
    const origin = editOriginRef.current
    // A retained local editor cannot transfer its payload after navigation.
    if (!origin || origin.threadId !== state.activeThreadId || origin.blockId !== block.id ||
      origin.workspace !== (normalizeWorkspaceRoot(owner?.workspace) || normalizeWorkspaceRoot(state.workspaceRoot))) return
    // Close only after the store owns the validated local payload. Rejected
    // operations retain this editor; later failures use its guarded draft.
    await rewindAndResend(block.id, trimmed, () => {
      editOriginRef.current = null
      setEditing(false)
    })
  }

  if (editing) {
    return (
      <div className={userStyles.row}>
        <div className={userStyles.stack}>
          <UserAttachmentPreviews meta={block.meta} />
          <div data-user-message-bubble className={`${userStyles.bubble} border border-accent/35 ring-1 ring-accent/15`}>
            <textarea
              data-user-message-editor
              ref={textareaRef}
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value)
                const el = e.currentTarget
                el.style.height = 'auto'
                el.style.height = `${Math.min(el.scrollHeight, 360)}px`
              }}
              onCompositionStart={composition.onCompositionStart}
              onCompositionEnd={composition.onCompositionEnd}
              onKeyDown={(e) => {
                if (composition.isComposingEvent(e) || e.repeat) return
                if (e.key === 'Escape') {
                  e.preventDefault()
                  cancelEdit()
                } else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  void submit()
                }
              }}
              rows={2}
              className={userStyles.editor}
            />
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-[12px] text-ds-faint">{t('rewindHint')}</span>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={cancelEdit}
                  className="rounded-md px-3 py-1 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                >
                  {t('rewindCancel')}
                </button>
                <button
                  type="button"
                  onClick={() => void submit()}
                  disabled={!draft.trim() || busy}
                  className="rounded-md bg-accent px-3 py-1 text-[13px] font-medium text-[var(--ds-composer-primary-action-fg)] shadow-sm transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {t('rewindResend')}
                </button>
              </div>
            </div>
          </div>
          {block.meta?.workspaceCheckpointId ? (
            <div className="mt-2 min-w-0 text-left text-[12px] font-medium leading-5 text-ds-faint">
              {t('rewindFileRollbackNotice')}
            </div>
          ) : null}
        </div>
      </div>
    )
  }

  return (
    <div className={userStyles.row} data-actions-reveal={actionsReveal}>
      <div className={userStyles.stack}>
        <UserAttachmentPreviews meta={block.meta} />
        {showClawInboundCard && parsedClawPrompt ? (
          <ClawInboundMessageCard display={parsedClawPrompt} text={displayText} />
        ) : (
          <div data-user-message-bubble className={userStyles.bubble}>
            <div className="whitespace-pre-wrap break-words [overflow-wrap:anywhere] text-left">
              <UserMessageInlineText text={displayText} meta={block.meta} threadId={threadId} />
            </div>
            {parsedWritePrompt ? (
              <WritePromptMetaDisclosure
                display={parsedWritePrompt}
                expanded={writeMetaOpen}
                onToggle={() => setWriteMetaOpen((value) => !value)}
              />
            ) : null}
            <RuntimeMetaChips meta={block.meta} align="right" hideAttachments />
          </div>
        )}
      </div>
      <div className={`${actionStyles.actions} ${userStyles.actions}`} data-clock="start">
        {clock ? <time dateTime={block.createdAt} className={actionStyles.timeStart}>{clock}</time> : null}
        <CopyFeedbackButton text={displayText} iconOnly chrome="turn" className={actionStyles.action} />
        {canEdit ? (
          <Tooltip label={t('rewindEditMessage')} side="bottom"><button
            type="button"
            onClick={startEdit}
            disabled={busy}
            aria-label={t('rewindEditMessage')}
            className={`${actionStyles.action} ${userStyles.editAction}`}
          >
            <DshEditOutlineRegular />
          </button></Tooltip>
        ) : null}
      </div>
    </div>
  )
}

function ClawInboundMessageCard({
  display,
  text
}: {
  display: ClawUserPromptDisplay
  text: string
}): ReactElement {
  const { t } = useTranslation('common')
  const meta = [
    display.sender ? t('clawTimelineSender', { sender: display.sender }) : '',
    display.chatType ? t('clawTimelineChatType', { chatType: display.chatType }) : '',
    display.messageType ? t('clawTimelineMessageType', { messageType: display.messageType }) : '',
    display.mentions ? t('clawTimelineMentions', { mentions: display.mentions }) : ''
  ].filter(Boolean)

  return (
    <div data-user-message-bubble className="w-full max-w-[min(560px,calc(100vw-3rem))] rounded-[18px] border border-ds-border bg-ds-card px-4 py-3 text-left shadow-[0_14px_34px_rgba(86,103,136,0.08)]">
      <div className="flex items-center gap-2 text-[12px] font-semibold text-ds-muted">
        <MessageSquareQuote className="h-3.5 w-3.5" strokeWidth={1.8} />
        <span>{t('clawTimelineInbound', { source: display.sourceLabel ?? t('claw') })}</span>
      </div>
      <div className="mt-2 whitespace-pre-wrap break-words text-[15px] leading-6 text-ds-ink [overflow-wrap:anywhere]">
        {text}
      </div>
      {meta.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {meta.map((item) => (
            <span
              key={item}
              className="rounded-md border border-ds-border-muted bg-ds-subtle px-2 py-0.5 text-[11px] text-ds-muted"
            >
              {item}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  )
}

const USER_INPUT_FREEFORM_LABEL = 'Answer'

function metaStringArray(meta: Record<string, unknown> | undefined, key: string): string[] {
  const value = meta?.[key]
  if (!Array.isArray(value)) return []
  return value.filter((entry): entry is string => typeof entry === 'string' && entry.trim().length > 0)
}

function metaAttachmentReferences(meta: RuntimeDisclosureMetadata | undefined): AttachmentReference[] {
  const value = meta?.attachments
  if (!Array.isArray(value)) return []
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') return null
      const raw = entry as Record<string, unknown>
      const id = typeof raw.id === 'string' && raw.id.trim() ? raw.id.trim() : ''
      if (!id) return null
      const name = typeof raw.name === 'string' && raw.name.trim() ? raw.name.trim() : undefined
      const mimeType = typeof raw.mimeType === 'string' && raw.mimeType.trim() ? raw.mimeType.trim() : undefined
      const byteSize = typeof raw.byteSize === 'number' && Number.isFinite(raw.byteSize) ? raw.byteSize : undefined
      const previewUrl = typeof raw.previewUrl === 'string' && raw.previewUrl.trim() ? raw.previewUrl.trim() : undefined
      const localFilePath =
        typeof raw.localFilePath === 'string' && raw.localFilePath.trim()
          ? raw.localFilePath.trim()
          : undefined
      const FilePath =
        typeof raw.FilePath === 'string' && raw.FilePath.trim()
          ? raw.FilePath.trim()
          : localFilePath
      const width = typeof raw.width === 'number' && Number.isFinite(raw.width) ? raw.width : undefined
      const height = typeof raw.height === 'number' && Number.isFinite(raw.height) ? raw.height : undefined
      return {
        id,
        ...(name ? { name } : {}),
        ...(mimeType ? { mimeType } : {}),
        ...(byteSize ? { byteSize } : {}),
        ...(width ? { width } : {}),
        ...(height ? { height } : {}),
        ...(previewUrl ? { previewUrl } : {}),
        ...(localFilePath ? { localFilePath } : {}),
        ...(FilePath ? { FilePath } : {})
      }
    })
    .filter((entry): entry is AttachmentReference => entry !== null)
}

type TimelineMediaReference = GeneratedFileReference & {
  id?: string
  localFilePath?: string
  FilePath?: string
}

function readMediaString(raw: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return undefined
}

function normalizeGeneratedFileReference(entry: unknown): GeneratedFileReference | null {
  if (!entry || typeof entry !== 'object') return null
  const raw = entry as Record<string, unknown>
  const id = readMediaString(raw, 'id', 'attachmentId')
  const name = readMediaString(raw, 'name', 'fileName', 'filename')
  const mimeType = readMediaString(raw, 'mimeType', 'type', 'mediaType')
  const previewUrl = readMediaString(raw, 'previewUrl', 'dataUrl', 'url')
  const path = readMediaString(raw, 'path', 'file')
  const relativePath = readMediaString(raw, 'relativePath', 'relative_path')
  const absolutePath = readMediaString(raw, 'absolutePath', 'absolute_path')
  const localFilePath = readMediaString(raw, 'localFilePath', 'local_file_path')
  const FilePath = readMediaString(raw, 'FilePath') ?? localFilePath
  const byteSize = typeof raw.byteSize === 'number' && Number.isFinite(raw.byteSize) ? raw.byteSize : undefined
  const width = typeof raw.width === 'number' && Number.isFinite(raw.width) ? raw.width : undefined
  const height = typeof raw.height === 'number' && Number.isFinite(raw.height) ? raw.height : undefined
  const normalized: GeneratedFileReference = {
    ...(id ? { id } : {}),
    ...(name ? { name } : {}),
    ...(mimeType ? { mimeType } : {}),
    ...(byteSize ? { byteSize } : {}),
    ...(width ? { width } : {}),
    ...(height ? { height } : {}),
    ...(previewUrl ? { previewUrl } : {}),
    ...(path ? { path } : {}),
    ...(relativePath ? { relativePath } : {}),
    ...(absolutePath ? { absolutePath } : {}),
    ...(localFilePath ? { localFilePath } : {}),
    ...(FilePath ? { FilePath } : {})
  }
  return Object.keys(normalized).length > 0 ? normalized : null
}

function metaGeneratedFileReferences(meta: Record<string, unknown> | undefined): GeneratedFileReference[] {
  const value = meta?.generatedFiles
  if (!Array.isArray(value)) return []
  return value
    .map((entry) => normalizeGeneratedFileReference(entry))
    .filter((entry): entry is GeneratedFileReference => entry !== null)
}

function mediaKey(media: TimelineMediaReference): string {
  return (
    media.id ||
    media.localFilePath ||
    media.FilePath ||
    media.absolutePath ||
    media.relativePath ||
    media.path ||
    media.previewUrl ||
    media.name ||
    'media'
  )
}

function mediaName(media: TimelineMediaReference): string {
  const path = media.relativePath || media.path || media.absolutePath || media.localFilePath || media.FilePath || ''
  const fromPath = path.split(/[\\/]/).filter(Boolean).at(-1)
  return media.name || fromPath || media.id || 'file'
}

function mediaPath(media: TimelineMediaReference): string | undefined {
  return media.relativePath || media.path || media.absolutePath || media.localFilePath || media.FilePath
}

function mediaMime(media: TimelineMediaReference): string {
  return media.mimeType?.toLowerCase() ?? ''
}

function mediaIsImage(media: TimelineMediaReference): boolean {
  const mimeType = mediaMime(media)
  if (mimeType.startsWith('image/')) return true
  return /\.(?:png|jpe?g|webp|gif|bmp|svg)$/i.test(mediaName(media))
}

function mediaIsVideo(media: TimelineMediaReference): boolean {
  const mimeType = mediaMime(media)
  if (mimeType.startsWith('video/')) return true
  return /\.(?:mp4|webm|mov|m4v|ogg)$/i.test(mediaName(media))
}

function formatByteSize(byteSize: number | undefined): string {
  if (typeof byteSize !== 'number' || !Number.isFinite(byteSize) || byteSize <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB']
  let value = byteSize
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  const digits = value >= 10 || unit === 0 ? 0 : 1
  return `${value.toFixed(digits)} ${units[unit]}`
}

function dataUrlPayload(dataUrl: string | undefined): { dataBase64: string; mimeType?: string } | null {
  if (!dataUrl?.startsWith('data:')) return null
  const match = dataUrl.match(/^data:([^;,]+)?(?:;[^,]*)?;base64,(.*)$/)
  if (!match?.[2]) return null
  return {
    dataBase64: match[2],
    ...(match[1] ? { mimeType: match[1] } : {})
  }
}

function mergeMediaReferences(
  attachments: AttachmentReference[],
  generatedFiles: GeneratedFileReference[]
): TimelineMediaReference[] {
  const media: TimelineMediaReference[] = []
  const indexByKey = new Map<string, number>()
  const indexByName = new Map<string, number>()

  const add = (entry: TimelineMediaReference, allowNameMerge: boolean): void => {
    const key = mediaKey(entry)
    const normalizedName = mediaName(entry).toLowerCase()
    const existingIndex = indexByKey.get(key) ?? (allowNameMerge ? indexByName.get(normalizedName) : undefined)
    if (existingIndex !== undefined) {
      media[existingIndex] = { ...media[existingIndex], ...entry }
      indexByKey.set(mediaKey(media[existingIndex]), existingIndex)
      indexByName.set(mediaName(media[existingIndex]).toLowerCase(), existingIndex)
      return
    }
    indexByKey.set(key, media.length)
    indexByName.set(normalizedName, media.length)
    media.push(entry)
  }

  for (const file of generatedFiles) add(file, false)
  for (const attachment of attachments) add(attachment, true)
  return media
}

type MediaPreviewRequest =
  | { key: string; id: string; mode: 'attachment' }
  | { key: string; path: string; mode: 'workspace-image' }

function isMediaPreviewRequest(entry: MediaPreviewRequest | null): entry is MediaPreviewRequest {
  return entry !== null
}

function useMediaPreviewUrls(media: TimelineMediaReference[]): Record<string, string> {
  const activeThreadId = useChatStore((s) => s.activeThreadId)
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const [resolvedPreviewUrls, setResolvedPreviewUrls] = useState<Record<string, string>>({})
  const [failedPreviewIds, setFailedPreviewIds] = useState<Record<string, true>>({})
  const previewRequests = useMemo(
    () =>
      media
        .map((item) => {
          const key = mediaKey(item)
          if (item.previewUrl || resolvedPreviewUrls[key] || failedPreviewIds[key]) return null
          if (item.id && (mediaIsImage(item) || mediaIsVideo(item) || !item.mimeType)) {
            return { key, id: item.id, mode: 'attachment' } satisfies MediaPreviewRequest
          }
          const path = mediaIsImage(item) ? mediaPath(item) : undefined
          if (path) return { key, path, mode: 'workspace-image' } satisfies MediaPreviewRequest
          return null
        })
        .filter(isMediaPreviewRequest),
    [failedPreviewIds, media, resolvedPreviewUrls]
  )
  const missingPreviewKey = previewRequests
    .map((request) =>
      request.mode === 'attachment'
        ? `attachment:${request.id}`
        : `workspace-image:${request.path}`
    )
    .join('\n')

  useEffect(() => {
    if (!missingPreviewKey) return
    const provider = getProvider()
    let cancelled = false
    void Promise.all(
      previewRequests.map(async (request) => {
        try {
          if (request.mode === 'attachment' && request.id && typeof provider.getAttachmentContent === 'function') {
            if (!activeThreadId || !workspaceRoot) {
              return { key: request.key, failed: true as const }
            }
            const content = await provider.getAttachmentContent(request.id, {
              threadId: activeThreadId,
              workspace: workspaceRoot
            })
            return {
              key: request.key,
              previewUrl: `data:${content.attachment.mimeType};base64,${content.dataBase64}`
            }
          }
          if (request.mode === 'workspace-image' && request.path && typeof window.analytix?.files?.readImage === 'function') {
            const result = await window.analytix.files.readImage({
              path: request.path,
              ...(workspaceRoot ? { workspaceRoot } : {})
            })
            if (result.ok) return { key: request.key, previewUrl: result.dataUrl }
          }
          return { key: request.key, failed: true as const }
        } catch {
          return { key: request.key, failed: true as const }
        }
      })
    ).then((results) => {
      if (cancelled) return
      setResolvedPreviewUrls((current) => {
        const next = { ...current }
        for (const result of results) {
          if ('previewUrl' in result && typeof result.previewUrl === 'string') {
            next[result.key] = result.previewUrl
          }
        }
        return next
      })
      setFailedPreviewIds((current) => {
        const next = { ...current }
        for (const result of results) {
          if ('failed' in result) next[result.key] = true
        }
        return next
      })
    })
    return () => {
      cancelled = true
    }
  }, [activeThreadId, missingPreviewKey, previewRequests, workspaceRoot])

  return resolvedPreviewUrls
}

function MediaPreviewTile({
  media,
  previewUrl,
  variant
}: {
  media: TimelineMediaReference
  previewUrl?: string
  variant: 'user' | 'tool' | 'conversation'
}): ReactElement {
  const { t } = useTranslation('common')
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [imagePreviewOpen, setImagePreviewOpen] = useState(false)
  const title = mediaName(media)
  const filePath = mediaPath(media)
  const mimeType = media.mimeType || (mediaIsImage(media) ? 'image' : mediaIsVideo(media) ? 'video' : '')
  const byteSize = formatByteSize(media.byteSize)
  const hasRichPreview = !!previewUrl && (mediaIsImage(media) || mediaIsVideo(media))
  const tileClass =
    variant === 'conversation'
      ? hasRichPreview
        ? 'h-72 w-full overflow-hidden rounded-lg border border-ds-border-muted bg-ds-card shadow-sm sm:h-80'
        : 'min-h-44 w-full overflow-hidden rounded-lg border border-ds-border-muted bg-ds-card shadow-sm'
      : variant === 'tool'
        ? 'block h-32 w-40 overflow-hidden rounded-lg border border-ds-border-muted bg-ds-card shadow-sm'
        : 'block h-28 w-36 overflow-hidden rounded-lg border border-ds-border-muted bg-ds-card shadow-sm'
  const revealClass = variant === 'user' ? '' : ' ds-media-printer-reveal'
  const mediaClass = 'h-full w-full object-contain'
  const canSave = Boolean(filePath || dataUrlPayload(previewUrl))
  const saveLabel =
    saveState === 'saving'
      ? t('generatedFileSaving')
      : saveState === 'saved'
        ? t('generatedFileSaved')
        : saveState === 'error'
          ? t('generatedFileSaveFailed')
          : t('generatedFileDownload')
  const handleSaveAs = async (): Promise<void> => {
    if (saveState === 'saving' || typeof window.analytix?.files?.saveAs !== 'function') return
    const data = dataUrlPayload(previewUrl)
    if (!filePath && !data) {
      setSaveState('error')
      return
    }
    setSaveState('saving')
    try {
      const result = await window.analytix.files.saveAs({
        suggestedName: title,
        ...(filePath ? { sourcePath: filePath } : {}),
        ...(workspaceRoot ? { workspaceRoot } : {}),
        ...(media.mimeType || data?.mimeType ? { mimeType: media.mimeType ?? data?.mimeType } : {}),
        ...(data && !filePath ? { dataBase64: data.dataBase64 } : {})
      })
      if (result.ok) {
        setSaveState('saved')
        window.setTimeout(() => setSaveState('idle'), 1600)
      } else if (result.canceled) {
        setSaveState('idle')
      } else {
        setSaveState('error')
      }
    } catch (error) {
      setSaveState('error')
      void window.analytix?.logs?.error?.('file-save-as', 'Failed to save generated file', {
        message: error instanceof Error ? error.message : String(error),
        filePath,
        title
      }).catch(() => undefined)
    }
  }
  const saveButtonClass =
    'inline-flex h-7 items-center justify-center rounded-md border border-ds-border-muted bg-ds-card/90 px-2 text-[11.5px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-50'
  const iconButtonClass =
    'absolute right-2 top-2 inline-flex h-8 w-8 items-center justify-center rounded-md border border-ds-border-muted bg-ds-card/92 text-ds-muted shadow-sm backdrop-blur transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-50'
  const saveIcon = saveState === 'saving'
    ? <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
    : saveState === 'saved'
      ? <Check className="h-3.5 w-3.5" strokeWidth={2} />
      : <Download className="h-3.5 w-3.5" strokeWidth={1.9} />

  if (previewUrl && mediaIsImage(media)) {
    return (
      <figure className={`${tileClass}${revealClass} relative`} title={title}>
        <button
          type="button"
          onClick={() => setImagePreviewOpen(true)}
          className="block h-full w-full cursor-zoom-in"
          title={t('imagePreviewOpen', { name: title })}
          aria-label={t('imagePreviewOpen', { name: title })}
        >
          <img src={previewUrl} alt={title} className={mediaClass} loading="lazy" />
        </button>
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            void handleSaveAs()
          }}
          disabled={!canSave || saveState === 'saving'}
          title={saveLabel}
          aria-label={saveLabel}
          className={iconButtonClass}
        >
          {saveIcon}
        </button>
        <ImagePreviewLightbox
          open={imagePreviewOpen}
          src={previewUrl}
          alt={title}
          title={title}
          downloadDisabled={!canSave || saveState === 'saving'}
          downloadLabel={saveLabel}
          onDownload={() => void handleSaveAs()}
          onClose={() => setImagePreviewOpen(false)}
        />
      </figure>
    )
  }

  if (previewUrl && mediaIsVideo(media)) {
    return (
      <figure className={`${tileClass} relative`} title={title}>
        <video src={previewUrl} className={mediaClass} controls preload="metadata" />
        <button
          type="button"
          onClick={() => void handleSaveAs()}
          disabled={!canSave || saveState === 'saving'}
          title={saveLabel}
          aria-label={saveLabel}
          className={iconButtonClass}
        >
          {saveIcon}
        </button>
      </figure>
    )
  }

  const Icon = mediaIsVideo(media) ? Video : mediaIsImage(media) ? ImageIcon : File
  return (
    <div className={`${tileClass} flex flex-col justify-between p-3`} title={title}>
      <div className="flex min-w-0 items-start gap-2">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-ds-border-muted bg-ds-subtle text-ds-muted">
          <Icon className="h-4 w-4" strokeWidth={1.8} />
        </span>
        <div className="min-w-0">
          <div className="line-clamp-2 break-words text-[12.5px] font-semibold leading-5 text-ds-ink">
            {title}
          </div>
          <div className="mt-0.5 truncate text-[11px] text-ds-faint">
            {[mimeType, byteSize].filter(Boolean).join(' · ') || t('generatedFilePreviewUnavailable')}
          </div>
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          onClick={() => void handleSaveAs()}
          disabled={!canSave || saveState === 'saving'}
          className={saveButtonClass}
          title={saveLabel}
        >
          <span className="mr-1.5">{saveIcon}</span>
          {t('generatedFileDownload')}
        </button>
      {filePath ? (
        <button
          type="button"
          onClick={() => void openWorkspacePathInEditor({ path: filePath }, workspaceRoot)}
          className={saveButtonClass}
        >
          {t('filePreviewOpenEditor')}
        </button>
      ) : null}
      </div>
    </div>
  )
}

function MediaAttachmentGallery({
  media,
  variant
}: {
  media: TimelineMediaReference[]
  variant: 'user' | 'tool' | 'conversation'
}): ReactElement | null {
  const resolvedPreviewUrls = useMediaPreviewUrls(media)
  if (media.length === 0) return null
  const wrapperClass =
    variant === 'conversation'
      ? `grid w-full max-w-2xl grid-cols-1 gap-2 ${media.length > 1 ? 'sm:grid-cols-2' : ''}`
      : variant === 'tool'
        ? 'flex min-w-0 flex-wrap gap-2 border-t border-ds-border-muted/60 px-4 py-3'
        : 'flex max-w-full flex-wrap justify-end gap-2'

  return (
    <div className={wrapperClass}>
      {media.map((item) => {
        const key = mediaKey(item)
        return (
          <MediaPreviewTile
            key={key}
            media={item}
            previewUrl={item.previewUrl ?? resolvedPreviewUrls[key]}
            variant={variant}
          />
        )
      })}
    </div>
  )
}

export function GeneratedFilesPanel({ blocks, present }: { blocks: ToolBlock[]; present?: { threadId: string | null; calls: PresentCall[] } }): ReactElement | null {
  const { t } = useTranslation('common')
  const artifacts = useMemo(() => {
    const seen = new Set<string>()
    return blocks.flatMap(block => {
      const parsed = generatedArtifactMetadataSchema.safeParse(block.meta?.generatedArtifact)
      if (block.status !== 'success' || !parsed.success || seen.has(parsed.data.artifactId)) return []
      seen.add(parsed.data.artifactId)
      return [parsed.data]
    })
  }, [blocks])
  const media = useMemo(
    () =>
      blocks.flatMap((block) =>
        mergeMediaReferences(
          metaAttachmentReferences(block.meta as RuntimeDisclosureMetadata | undefined),
          metaGeneratedFileReferences(block.meta)
        )
      ),
    [blocks]
  )

  if (media.length === 0 && artifacts.length === 0 && !present?.calls.length) return null

  return (
    <div className="flex min-w-0 flex-col gap-2">
      {present ? <PresentedFiles threadId={present.threadId} calls={present.calls} artifacts={artifacts} /> : <GeneratedOfficeDeliverables artifacts={artifacts} />}
      {media.length > 0 ? <div className="text-[12px] font-semibold text-ds-faint">{t('generatedFilesTitle')}</div> : null}
      <MediaAttachmentGallery media={media} variant="conversation" />
    </div>
  )
}

function UserAttachmentPreviews({
  meta
}: {
  meta?: RuntimeDisclosureMetadata
}): ReactElement | null {
  const attachments = useMemo(() => {
    const attachmentIds = metaStringArray(meta, 'attachmentIds')
    const byId = new Map<string, AttachmentReference>()
    for (const attachment of metaAttachmentReferences(meta)) {
      byId.set(attachment.id, attachment)
    }
    for (const id of attachmentIds) {
      if (!byId.has(id)) byId.set(id, { id })
    }
    return [...byId.values()]
  }, [meta])

  if (attachments.length === 0) return null

  return (
    <div className={userStyles.attachments} data-message-attachments>
      <MediaAttachmentGallery media={attachments} variant="user" />
    </div>
  )
}

export function ToolAttachmentPreviews({
  meta
}: {
  meta?: Record<string, unknown>
}): ReactElement | null {
  const attachments = useMemo(
    () => metaAttachmentReferences(meta as RuntimeDisclosureMetadata | undefined),
    [meta]
  )

  if (attachments.length === 0) return null

  return <MediaAttachmentGallery media={attachments} variant="tool" />
}

function metaSources(meta: Record<string, unknown> | undefined): Array<{ title?: string; url?: string }> {
  const value = meta?.sources
  if (!Array.isArray(value)) return []
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') return null
      const raw = entry as Record<string, unknown>
      const title = typeof raw.title === 'string' && raw.title.trim() ? raw.title.trim() : undefined
      const url = typeof raw.url === 'string' && raw.url.trim() ? raw.url.trim() : undefined
      return title || url ? { ...(title ? { title } : {}), ...(url ? { url } : {}) } : null
    })
    .filter((entry): entry is { title?: string; url?: string } => entry !== null)
}

function RuntimeMetaChips({
  meta,
  align = 'left',
  hideAttachments = false
}: {
  meta?: Record<string, unknown>
  align?: 'left' | 'right'
  hideAttachments?: boolean
}): ReactElement | null {
  const { t } = useTranslation('common')
  const attachmentIds = metaStringArray(meta, 'attachmentIds')
  const activeSkillIds = metaStringArray(meta, 'activeSkillIds')
  const injectedMemoryIds = metaStringArray(meta, 'injectedMemoryIds')
  const sources = metaSources(meta)
  const child = meta?.child && typeof meta.child === 'object' ? meta.child as Record<string, unknown> : null
  const childChip = formatChildAgentChip(child, t)
  const jobDiagnosticsChip = formatJobDiagnosticsChip(meta, t)
  const providerDiagnosticsChip = formatProviderDiagnosticsChip(meta, t)
  if (
    (hideAttachments || attachmentIds.length === 0) &&
    activeSkillIds.length === 0 &&
    injectedMemoryIds.length === 0 &&
    sources.length === 0 &&
    !childChip &&
    !jobDiagnosticsChip &&
    !providerDiagnosticsChip
  ) {
    return null
  }
  const chipClass = 'inline-flex max-w-full items-center gap-1 rounded-md border border-ds-border-muted bg-ds-card/75 px-1.5 py-0.5 text-[11px] font-medium text-ds-faint'
  return (
    <div className={`mt-2 flex min-w-0 flex-wrap gap-1.5 ${align === 'right' ? 'justify-end' : ''}`}>
      {!hideAttachments && attachmentIds.length > 0 ? (
        <span className={chipClass} title={attachmentIds.join(', ')}>
          {t('toolAttachments')} {attachmentIds.length}
        </span>
      ) : null}
      {activeSkillIds.length > 0 ? (
        <span className={chipClass} title={activeSkillIds.join(', ')}>
          {t('toolActiveSkills')} {activeSkillIds.length}
        </span>
      ) : null}
      {injectedMemoryIds.length > 0 ? (
        <InjectedMemoryMetaChip meta={meta} memoryIds={injectedMemoryIds} chipClass={chipClass} />
      ) : null}
      {childChip ? (
        <span className={chipClass} title={childChip.title}>
          {t('toolChildAgent')} <span className="max-w-28 truncate font-mono text-ds-muted">{childChip.label}</span>
          {childChip.detail ? (
            <span className="max-w-48 truncate text-ds-muted">{childChip.detail}</span>
          ) : null}
        </span>
      ) : null}
      {jobDiagnosticsChip ? (
        <span className={chipClass} title={jobDiagnosticsChip.title}>
          {jobDiagnosticsChip.label}
          {jobDiagnosticsChip.detail ? (
            <span className="max-w-48 truncate text-ds-muted">{jobDiagnosticsChip.detail}</span>
          ) : null}
        </span>
      ) : null}
      {providerDiagnosticsChip ? (
        <span className={chipClass} title={providerDiagnosticsChip.title}>
          {providerDiagnosticsChip.label}
          <span className="max-w-72 truncate text-ds-muted">{providerDiagnosticsChip.detail}</span>
        </span>
      ) : null}
      {sources.slice(0, 4).map((source, index) =>
        source.url ? (
          <a
            key={`${source.url}-${index}`}
            href={source.url}
            target="_blank"
            rel="noreferrer"
            className={chipClass}
            title={source.url}
          >
            {t('toolSources')} {index + 1}
          </a>
        ) : (
          <span key={`${source.title}-${index}`} className={chipClass} title={source.title}>
            {t('toolSources')} {index + 1}
          </span>
        )
      )}
    </div>
  )
}

/* Turn chrome and clipboard fallback adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
export function CopyFeedbackButton({
  text,
  iconOnly = false,
  chrome = 'standard',
  className
}: {
  text: string
  iconOnly?: boolean
  chrome?: 'standard' | 'turn'
  className?: string
}): ReactElement {
  const { t } = useTranslation('common')
  const [status, setStatus] = useState<'idle' | 'success' | 'error'>('idle')
  const resetRef = useRef<number | null>(null)
  const copyPending = useRef(false), copyEpoch = useRef(0)
  useEffect(() => {
    copyEpoch.current += 1
    copyPending.current = false
    setStatus('idle')
    if (resetRef.current !== null) { window.clearTimeout(resetRef.current); resetRef.current = null }
    return () => {
      copyEpoch.current += 1
      copyPending.current = false
      if (resetRef.current !== null) window.clearTimeout(resetRef.current)
    }
  }, [text])

  const scheduleReset = (): void => {
    if (resetRef.current !== null) window.clearTimeout(resetRef.current)
    resetRef.current = window.setTimeout(() => {
      setStatus('idle')
      resetRef.current = null
    }, chrome === 'turn' ? 1000 : COPY_FEEDBACK_RESET_MS)
  }

  const handleCopy = async (): Promise<void> => {
    if (copyPending.current || status === 'success') return
    const epoch = copyEpoch.current
    copyPending.current = true
    let copied = false
    try {
      if (navigator?.clipboard?.writeText) await navigator.clipboard.writeText(text)
      else if (chrome === 'turn' && typeof document.execCommand === 'function') {
        // Pinned DSH clipboard fallback; a rejected async write does not retry.
        const exec = document.execCommand.bind(document), area = document.createElement('textarea')
        area.value = text; area.setAttribute('readonly', '')
        area.style.position = 'fixed'; area.style.left = '-9999px'
        document.body.appendChild(area)
        try { area.select(); if (!exec('copy')) throw new Error('Clipboard unavailable') }
        finally { area.remove() }
      } else throw new Error('Clipboard unavailable')
      if (epoch !== copyEpoch.current) return
      copied = true
      setStatus('success')
    } catch {
      if (epoch !== copyEpoch.current) return
      setStatus(chrome === 'turn' ? 'idle' : 'error')
    }
    copyPending.current = false
    if (copied || chrome === 'standard') scheduleReset()
  }

  const success = status === 'success'
  const error = status === 'error'
  const label = success ? t('copySuccess') : error ? t('copyFailed') : t('copyMessage')
  const iconClassName = iconOnly ? 'h-4 w-4' : 'h-3.5 w-3.5'

  const button = (
    <button
      type="button"
      onClick={() => void handleCopy()}
      title={chrome === 'turn' ? undefined : label}
      aria-label={label}
      className={className ?? `flex shrink-0 items-center rounded-md transition ${
        iconOnly
          ? 'gap-0 p-1 hover:bg-ds-hover'
          : 'gap-1 px-1.5 py-0.5 hover:bg-ds-hover'
      } ${
        success
          ? 'text-emerald-500'
          : error
            ? 'text-rose-400'
            : 'text-ds-faint hover:text-ds-muted'
      }`}
    >
      {chrome === 'turn' ? (success ? <DshCheckOutlineRegular /> : <DshCopyOutlineRegular />) : success ? (
        <Check className={iconClassName} strokeWidth={2} />
      ) : (
        <Copy className={iconClassName} strokeWidth={1.8} />
      )}
      {!iconOnly ? <span>{label}</span> : null}
    </button>
  )
  return chrome === 'turn' ? <Tooltip label={label} side="bottom">{button}</Tooltip> : button
}

/** A secondary timeline can display a gate, but only its owning main-thread
 * responder may submit it. No action falls back to another thread's blocks. */
export const GateResponseContext = createContext<{
  onOpenOwnerThread?: (requestId: string) => void
} | null>(null)

// A turn can contain later user steering as well as its opening input. Keep
// recency available to those nested leaves without changing tool ownership.
export const UserMessageActionsRevealContext = createContext<{
  latestUserId?: string
  reveal: 'always' | 'hover'
} | null>(null)

/**
 * Memoized so settled bubbles skip re-render while streaming deltas
 * re-render only the live bubble; block references stay stable in the
 * store for unchanged blocks.
 */
export const MessageBubble = memo(MessageBubbleImpl)

function formatMessageDateTime(input: string, locale: string): string {
  const date = new Date(input)
  if (Number.isNaN(date.getTime())) return input
  const now = new Date()
  const sameYear = date.getFullYear() === now.getFullYear()
  return new Intl.DateTimeFormat(locale, {
    ...(sameYear ? {} : { year: 'numeric' }),
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

/**
 * Memoized so settled bubbles skip re-render while streaming deltas
 * re-render only the live bubble; block references stay stable in the
 * store for unchanged blocks.
 */

function MessageBubbleImpl({
  block,
  nested = false,
  onAssistantFinalized,
  threadId,
  showActions = true,
  presented = true
}: {
  block: ChatBlock
  nested?: boolean
  threadId?: string | null
  showActions?: boolean
  presented?: boolean
  onAssistantFinalized?: () => void
}): ReactElement {
  const { t, i18n } = useTranslation('common')
  const resolveApproval = useChatStore((s) => s.resolveApproval)
  const resolveUserInput = useChatStore((s) => s.resolveUserInput)
  const selectUserInputPanel = useChatStore(s => s.selectUserInputPanel)
  const lateReplyTargetTurnId = useChatStore(s => s.busy && !isPendingActiveStreamTurnId(s.currentTurnId) && s.activeThreadId === (threadId ?? s.activeThreadId) ? s.currentTurnId : null)
  const responseContext = useContext(GateResponseContext)
  const referenceScopeThreadId = useContext(ProtectedToolResultScope)
  const userActions = useContext(UserMessageActionsRevealContext)
  const resolving = useChatStore(state => block.kind === 'approval' ? state.pendingGateResolutionIds?.[`approval:${block.approvalId}`] === true : block.kind === 'user_input' && block.status === 'pending' && state.pendingGateResolutionIds?.[`user_input:${block.requestId}`] === true)
  const activeTraceThreadId = useChatStore((s) => s.activeThreadId)
  const replyBlocks = useChatStore(s => s.blocks)
  const replyRequests = replyBlocks.filter((candidate): candidate is Extract<ChatBlock, { kind: 'user_input' }> => candidate.kind === 'user_input' && candidate.status === 'continued')
  const traceThreadId = threadId === undefined ? activeTraceThreadId : threadId
  const answerRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    answerRef.current?.removeAttribute('data-terminal-trace-seq')
    if (block.kind !== 'assistant' || block.id === 'live-assistant' || !answerRef.current) return
    if (traceThreadId) observeTerminalDOMCommit(traceThreadId, block.id, answerRef.current)
  }, [block, traceThreadId])
  if (block.kind === 'user') {
    if (block.source !== undefined) return <GoalRoundSourceView block={block} />
    const actionsReveal = userActions && block.id !== userActions.latestUserId ? 'hover' : userActions?.reveal
    const referenceThreadId = threadId === undefined ? referenceScopeThreadId : threadId
    const fallback = <UserMessageBubble block={block} threadId={referenceThreadId} actionsReveal={actionsReveal} />
    const message = block.meta?.steeringStatus === 'accepted' && block.meta.clientUserMessageId && traceThreadId && traceThreadId === activeTraceThreadId
      ? <QuestionReplyView key={traceThreadId + ':' + block.id} block={block} requests={replyRequests} threadId={traceThreadId} fallback={fallback} /> : fallback
    return nested ? <div data-content-search-unit-key={`${block.id}:user`}>{message}</div> : message
  }
  if (block.kind === 'assistant') {
    const streaming = block.id === 'live-assistant'
    const createdAtLabel = block.createdAt
      ? formatMessageDateTime(block.createdAt, i18n.language)
      : null
    return (
      <div className="group/message flex min-w-0 max-w-full flex-col">
        {block.meta?.factHistoryState === 'retained_snapshot' && block.acceptedFinalProjectionReceipt ? (
          <div className="mb-1 text-xs text-ds-muted">{t('retainedSnapshotHistory')}</div>
        ) : null}
        <div ref={answerRef} className="ds-chat-answer min-w-0 max-w-full text-ds-ink">
          <AssistantMarkdown
            text={block.text}
            streaming={streaming}
            rowId={block.id}
            onFinalized={!streaming ? onAssistantFinalized : undefined}
          />
        </div>
        {!streaming && block.acceptedFinalProjectionReceipt &&
          acceptedFinalHasLocalDisplaySlots(block.acceptedFinalView) ? (
          <AcceptedSlotDisplay receipt={block.acceptedFinalProjectionReceipt} />
        ) : null}
        {!streaming && showActions ? (
          <div className="mt-1 flex min-h-5 min-w-0 items-center justify-between gap-3 text-[11.5px] text-ds-faint opacity-0 transition duration-150 group-hover/message:opacity-100">
            <span className="min-w-0 truncate">{createdAtLabel ?? ''}</span>
            <div className="flex shrink-0 items-center gap-1.5">
              <CopyFeedbackButton text={block.text} />
            </div>
          </div>
        ) : null}
      </div>
    )
  }
  if (block.kind === 'tool') {
    return <ProtectedToolResultScope.Provider value={threadId ?? traceThreadId}>
      <ToolCallTree blocks={[block]} processing={block.status === 'running'} />
    </ProtectedToolResultScope.Provider>
  }
  if (block.kind === 'user_input') {
    return <QuestionComposer key={`${traceThreadId}:${block.requestId}`} block={block} resolving={resolving} presented={presented}
      selector={traceThreadId && typeof block.meta?.turnId === 'string' ? {
        threadId: traceThreadId, turnId: block.meta.turnId, itemId: block.id, requestId: block.requestId
      } : null}
      readOnly={Boolean(responseContext) || !['pending', 'continued'].includes(block.status)} onOpenOwnerThread={responseContext?.onOpenOwnerThread}
      lateReplyTargetTurnId={lateReplyTargetTurnId}
      getLateReplyExpectedTurnId={traceThreadId && !responseContext ? async () => {
        const owns = () => useChatStore.getState().activeThreadId === traceThreadId && useChatStore.getState().route === 'chat'
        if (!owns()) return null
        const detail = await getProvider().getThreadDetail(traceThreadId)
        const turnId = detail.latestTurnId
        return owns() && turnId && !isPendingActiveStreamTurnId(turnId) ? turnId : null
      } : undefined}
      onHide={traceThreadId === activeTraceThreadId && !responseContext ? () => selectUserInputPanel(null) : undefined}
      onLateSubmit={(answers, clientUserMessageId, expectedTurnId) => resolveUserInput({ itemId: block.id, requestId: block.requestId, threadId: traceThreadId, turnId: typeof block.meta?.turnId === 'string' ? block.meta.turnId : null }, { kind: 'late', answers, clientUserMessageId, expectedTurnId })}
      onSubmit={answers => resolveUserInput({ itemId: block.id, requestId: block.requestId, threadId: traceThreadId, turnId: typeof block.meta?.turnId === 'string' ? block.meta.turnId : null }, { kind: 'submit', answers })}
      onCancel={() => resolveUserInput({ itemId: block.id, requestId: block.requestId, threadId: traceThreadId, turnId: typeof block.meta?.turnId === 'string' ? block.meta.turnId : null }, { kind: 'cancel' })} />
  }
  if (block.kind === 'approval') {
    return <ApprovalPanel key={`${traceThreadId}:${block.approvalId}`} block={block} resolving={resolving}
      readOnly={Boolean(responseContext)} onOpenOwnerThread={responseContext?.onOpenOwnerThread}
      onResolve={decision => resolveApproval(block.id, decision)} />
  }
  if (block.kind === 'compaction') return <TurnStatusView block={block} />
  if (block.kind === 'review') {
    return (
      <div className="ds-card-soft rounded-[18px] px-3 py-2 text-[13.5px] text-ds-muted">
        {block.reviewText || block.title}
      </div>
    )
  }
  if (block.kind === 'system') return <TurnStatusView block={block} />
  return <></>
}
