import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type MutableRefObject,
  type ReactElement
} from 'react'
import { useTranslation } from 'react-i18next'
import { useShallow } from 'zustand/react/shallow'
import {
  ArrowDownToLine,
  ChevronDown,
  CornerDownLeft,
  Loader2,
  MessageCircleMore,
  Minus,
  MoreHorizontal,
  Plus,
  Square,
  Trash2,
  Wrench,
  X
} from 'lucide-react'
import { useChatStore } from '../../store/chat-store'
import type { ChatBlock, ToolBlock } from '../../agent/types'
import { splitThink } from '../../thread/projection/thread-turns'
import { useActiveStreamSnapshot } from '../../thread/streaming/active-stream-store'
import { AssistantMarkdown } from './AssistantMarkdown'
import { summarizeToolBlock } from './message-timeline-process'
import { formatChildAgentChip } from './message-timeline-tools'
import { redactSecretText } from '@shared/secret-redaction'

type Props = {
  className?: string
  rightOffset?: number
}

const SIDE_STICK_TO_BOTTOM_PX = 96

type SideScrollPosition = {
  scrollTop: number
  stickToBottom: boolean
}

type SideChatComposerProps = {
  value: string
  onChange: (value: string) => void
  onSend: () => void
  onStop?: () => void
  busy?: boolean
  disabled?: boolean
  placeholder: string
  stopLabel?: string
}

function formatInheritedTime(value: string, locale: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
    month: 'short',
    day: 'numeric'
  }).format(date)
}

function compactSideTitle(value: string): string {
  const trimmed = value.replace(/\s*·\s*side$/i, '').trim()
  const prefix = Array.from(trimmed || value.trim()).slice(0, 5).join('')
  return prefix || value
}

function overlayStyle(rightOffset = 24): CSSProperties {
  const offset = Math.max(12, Math.round(rightOffset))
  return {
    right: `min(${offset}px, calc(12px + max(0px, 100vw - 760px)))`
  }
}

function SideChatComposer({
  value,
  onChange,
  onSend,
  onStop,
  busy = false,
  disabled = false,
  placeholder,
  stopLabel
}: SideChatComposerProps): ReactElement {
  const actionDisabled = busy ? !onStop : disabled || value.trim().length === 0
  const actionLabel = busy ? (stopLabel ?? placeholder) : placeholder

  const runAction = (): void => {
    if (busy) {
      onStop?.()
      return
    }
    onSend()
  }

  const handleKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>): void => {
    if (event.key !== 'Enter' || event.shiftKey || event.metaKey || event.ctrlKey) return
    event.preventDefault()
    if (!busy && !actionDisabled) onSend()
  }

  return (
    <div className="rounded-[10px] border border-ds-border-muted bg-ds-card/80 px-2 py-1.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.42)] dark:bg-white/[0.045]">
      <div className="flex items-end gap-1.5">
        <textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={handleKeyDown}
          disabled={disabled || busy}
          rows={1}
          placeholder={placeholder}
          className="max-h-24 min-h-[30px] flex-1 resize-none bg-transparent px-1 py-1 text-[13px] leading-5 text-ds-ink outline-none placeholder:text-ds-faint disabled:cursor-not-allowed disabled:opacity-65"
        />
        <button
          type="button"
          onClick={runAction}
          disabled={actionDisabled}
          className="mb-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={actionLabel}
          title={actionLabel}
        >
          {busy ? (
            <Square className="h-3.5 w-3.5" strokeWidth={1.9} />
          ) : (
            <CornerDownLeft className="h-3.5 w-3.5" strokeWidth={1.9} />
          )}
        </button>
      </div>
    </div>
  )
}

function SideAssistantBubble({
  text,
  streaming,
  rowId
}: {
  text: string
  streaming: boolean
  rowId: string
}): ReactElement | null {
  if (!text.trim()) return null
  return (
    <div className="ds-markdown ds-chat-answer min-w-0 max-w-full text-[13px] leading-5 text-ds-ink">
      <AssistantMarkdown text={text} streaming={streaming} rowId={rowId} />
    </div>
  )
}

function sideToolChild(block: ToolBlock): Record<string, unknown> | null {
  const child = block.meta?.child
  return child && typeof child === 'object' && !Array.isArray(child)
    ? child as Record<string, unknown>
    : null
}

function sideToolDetailPreview(block: ToolBlock): string {
  const detail = redactSecretText(block.detail ?? '').trim()
  if (!detail) return ''
  const compact = detail.replace(/\s+/g, ' ')
  return compact.length > 240 ? `${compact.slice(0, 239).trimEnd()}...` : compact
}

function SideToolBubble({ block }: { block: ToolBlock }): ReactElement {
  const { t } = useTranslation('common')
  const childChip = formatChildAgentChip(sideToolChild(block), t)
  const detailPreview = sideToolDetailPreview(block)
  const summary = summarizeToolBlock(block, t)
  const toneClass = block.status === 'error'
    ? 'border-red-200/80 bg-red-50/80 text-red-700 dark:border-red-900/45 dark:bg-red-500/10 dark:text-red-200'
    : 'border-ds-border-muted bg-ds-card/70 text-ds-muted'

  return (
    <div className={`rounded-[12px] border px-3 py-2 text-[12px] ${toneClass}`}>
      <div className="flex min-w-0 items-center gap-2">
        <Wrench className="h-3 w-3 shrink-0" strokeWidth={1.9} />
        <span className="min-w-0 flex-1 truncate font-medium" title={summary}>
          {summary}
        </span>
        {block.status === 'running' ? (
          <Loader2 className="h-3 w-3 shrink-0 animate-spin" strokeWidth={1.9} />
        ) : (
          <span className="shrink-0 text-[11px] uppercase tracking-wide opacity-70">{block.status}</span>
        )}
      </div>
      {block.filePath || childChip || detailPreview ? (
        <div className="mt-1.5 flex min-w-0 flex-col gap-1 text-[11.5px] leading-4 opacity-85">
          {block.filePath ? (
            <div className="truncate font-mono" title={block.filePath}>{block.filePath}</div>
          ) : null}
          {childChip ? (
            <div className="truncate" title={childChip.title}>
              {t('toolChildAgent')} {childChip.label}{childChip.detail ? ` · ${childChip.detail}` : ''}
            </div>
          ) : null}
          {detailPreview ? (
            <div className="line-clamp-2 break-words" title={redactSecretText(block.detail ?? '')}>
              {detailPreview}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function SideMessageBubble({ block }: { block: ChatBlock }): ReactElement | null {
  if (block.kind === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[86%] rounded-[14px] bg-ds-card px-3 py-2 text-[13px] leading-5 text-ds-ink shadow-[0_6px_18px_rgba(20,47,95,0.06)]">
          <div className="ds-markdown whitespace-pre-wrap break-words">{block.text}</div>
        </div>
      </div>
    )
  }
  if (block.kind === 'assistant') {
    const streaming = block.id === 'live-assistant'
    const split = splitThink(block.text)
    return (
      <SideAssistantBubble
        text={split.content}
        streaming={streaming}
        rowId={block.id}
      />
    )
  }
  if (block.kind === 'tool') {
    return <SideToolBubble block={block} />
  }
  if (block.kind === 'approval' || block.kind === 'compaction') {
    return (
      <div className="rounded-full border border-ds-border-muted bg-ds-card/60 px-3 py-1.5 text-[12px] text-ds-muted">
        {block.summary}
      </div>
    )
  }
  if (block.kind === 'user_input') {
    return (
      <div className="rounded-full border border-ds-border-muted bg-ds-card/60 px-3 py-1.5 text-[12px] text-ds-muted">
        {block.questions.map((q) => q.question).join(' · ') || 'user input'}
      </div>
    )
  }
  if (block.kind === 'system') {
    return (
      <div className="rounded-[12px] border border-ds-border-muted bg-ds-card/55 px-3 py-2 text-[12px] text-ds-muted">
        {block.text}
      </div>
    )
  }
  return null
}

function SideActiveStreamRows({
  threadId,
  turnId,
  scrollRef,
  stickToBottomRef
}: {
  threadId: string
  turnId: string | null
  scrollRef: MutableRefObject<HTMLDivElement | null>
  stickToBottomRef: MutableRefObject<boolean>
}): ReactElement | null {
  const activeStream = useActiveStreamSnapshot(threadId, turnId)
  const scrollFrameRef = useRef<number | null>(null)
  const assistantText = activeStream.liveAssistantContent

  useLayoutEffect(() => {
    if (!activeStream.version || !stickToBottomRef.current) return
    const el = scrollRef.current
    if (!el) return
    if (scrollFrameRef.current !== null) {
      window.cancelAnimationFrame(scrollFrameRef.current)
      scrollFrameRef.current = null
    }
    const applyScroll = (): void => {
      scrollFrameRef.current = null
      if (!stickToBottomRef.current) return
      const targetTop = Math.max(0, el.scrollHeight - el.clientHeight)
      if (Math.abs(el.scrollTop - targetTop) >= 0.5) {
        el.scrollTop = targetTop
      }
    }
    scrollFrameRef.current = window.requestAnimationFrame(applyScroll)
    return () => {
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current)
        scrollFrameRef.current = null
      }
    }
  }, [activeStream.version, scrollRef, stickToBottomRef])

  if (!activeStream.threadId || !assistantText.trim()) return null
  const rowIdBase = `side:${threadId}:${turnId ?? activeStream.turnId ?? 'live'}`
  return (
    <SideAssistantBubble
      text={assistantText}
      streaming
      rowId={`${rowIdBase}:assistant`}
    />
  )
}

export function SideConversationPanel({
  className,
  rightOffset = 24
}: Props): ReactElement | null {
  const { t, i18n } = useTranslation('common')
  const [draftInput, setDraftInput] = useState('')
  const [minimized, setMinimized] = useState(false)
  const [switchMenuOpen, setSwitchMenuOpen] = useState(false)
  const [moreMenuOpen, setMoreMenuOpen] = useState(false)
  const switchMenuRef = useRef<HTMLDivElement | null>(null)
  const moreMenuRef = useRef<HTMLDivElement | null>(null)
  const sideScrollRef = useRef<HTMLDivElement | null>(null)
  const sideStickToBottomRef = useRef(true)
  const activeSideIdRef = useRef<string | null>(null)
  const sideScrollPositionsRef = useRef(new Map<string, SideScrollPosition>())
  const prevOpenRef = useRef(false)

  const sideData = useChatStore(
    useShallow((s) => ({
      sides: s.sideConversations,
      panel: s.sidePanel,
      parentThreadId: s.activeThreadId,
      threads: s.threads,
      runtimeConnection: s.runtimeConnection,
      spawnSideConversation: s.spawnSideConversation,
      sendSideMessage: s.sendSideMessage,
      interruptSide: s.interruptSide,
      setSideInput: s.setSideInput,
      selectSideConversation: s.selectSideConversation,
      setSidePanelOpen: s.setSidePanelOpen,
      openSideConversationDraft: s.openSideConversationDraft,
      discardSideConversation: s.discardSideConversation,
      promoteSideConversation: s.promoteSideConversation
    }))
  )

  const currentSides = useMemo(
    () =>
      Object.values(sideData.sides)
        .filter((side) => side.parentThreadId === sideData.parentThreadId)
        .sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt)),
    [sideData.parentThreadId, sideData.sides]
  )
  const sideIds = currentSides.map((side) => side.threadId)
  const activeId =
    sideData.panel.activeSideId && sideIds.includes(sideData.panel.activeSideId)
      ? sideData.panel.activeSideId
      : null
  const activeSide = activeId ? sideData.sides[activeId] : null
  const parentThread = sideData.parentThreadId
    ? sideData.threads.find((thread) => thread.id === sideData.parentThreadId) ?? null
    : null
  const runningCount = currentSides.reduce((count, side) => count + (side.busy ? 1 : 0), 0)
  const shouldRender = Boolean(sideData.parentThreadId && sideData.panel.open)
  const showDraft = shouldRender && !activeSide
  const rightStyle = overlayStyle(rightOffset)

  useEffect(() => {
    if (sideData.panel.open && !prevOpenRef.current) {
      setMinimized(false)
    }
    prevOpenRef.current = sideData.panel.open
  }, [sideData.panel.open])

  useEffect(() => {
    if (!shouldRender) return
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        setSwitchMenuOpen(false)
        setMoreMenuOpen(false)
        setMinimized(false)
        sideData.setSidePanelOpen(false)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [shouldRender, sideData])

  useEffect(() => {
    if (!switchMenuOpen && !moreMenuOpen) return
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (
        target instanceof Node &&
        (switchMenuRef.current?.contains(target) || moreMenuRef.current?.contains(target))
      ) {
        return
      }
      setSwitchMenuOpen(false)
      setMoreMenuOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    return () => window.removeEventListener('pointerdown', onPointerDown)
  }, [switchMenuOpen, moreMenuOpen])

  useEffect(() => {
    const el = sideScrollRef.current
    if (!el) return
    const savePosition = (): void => {
      const distanceToBottom = Math.max(0, el.scrollHeight - el.scrollTop - el.clientHeight)
      sideStickToBottomRef.current = distanceToBottom <= SIDE_STICK_TO_BOTTOM_PX
      if (activeId) {
        sideScrollPositionsRef.current.set(activeId, {
          scrollTop: el.scrollTop,
          stickToBottom: sideStickToBottomRef.current
        })
      }
    }
    el.addEventListener('scroll', savePosition, { passive: true })
    savePosition()
    return () => el.removeEventListener('scroll', savePosition)
  }, [activeId])

  useLayoutEffect(() => {
    const el = sideScrollRef.current
    if (!el) return
    if (activeSideIdRef.current !== activeId) {
      activeSideIdRef.current = activeId
      const saved = activeId ? sideScrollPositionsRef.current.get(activeId) : undefined
      sideStickToBottomRef.current = saved?.stickToBottom ?? true
      el.scrollTop = saved && !saved.stickToBottom
        ? Math.min(saved.scrollTop, Math.max(0, el.scrollHeight - el.clientHeight))
        : Math.max(0, el.scrollHeight - el.clientHeight)
      return
    }
    if (!activeId || !sideStickToBottomRef.current) return
    el.scrollTop = Math.max(0, el.scrollHeight - el.clientHeight)
  }, [
    activeId,
    activeSide?.blocks.length,
    activeSide?.busy,
    activeSide?.error,
    activeSide?.liveAssistant.length
  ])

  if (!shouldRender) return null

  const titleCount = sideIds.length > 0 ? ` · ${sideIds.length}` : ''
  const activeIsBackgroundAgent = activeSide?.source === 'background-agent'
  const title = activeIsBackgroundAgent
    ? activeSide.title || t('sidePanelBackgroundAgentTitle')
    : `${t('sidePanelTitle')}${titleCount}`
  const subtitle = activeIsBackgroundAgent
    ? parentThread
      ? t('sidePanelBackgroundAgentParentLabel', { title: parentThread.title })
      : t('sidePanelBackgroundAgentTitle')
    : parentThread
      ? t('sidePanelParentLabel', { title: parentThread.title })
      : t('sidePanelParentMissing')

  const closeWindow = (): void => {
    setMinimized(false)
    setSwitchMenuOpen(false)
    setMoreMenuOpen(false)
    sideData.setSidePanelOpen(false)
  }

  const openDraft = (): void => {
    setSwitchMenuOpen(false)
    setMoreMenuOpen(false)
    sideData.openSideConversationDraft()
  }

  const sendDraft = (): void => {
    const text = draftInput.trim()
    if (!text) return
    setDraftInput('')
    void sideData.spawnSideConversation(text)
  }

  const sendActiveSide = (): void => {
    if (!activeSide) return
    void sideData.sendSideMessage(activeSide.threadId, activeSide.input)
  }

  const discardActiveSide = (): void => {
    if (!activeSide) return
    setMoreMenuOpen(false)
    void sideData.discardSideConversation(activeSide.threadId)
  }

  const promoteActiveSide = (): void => {
    if (!activeSide) return
    setMoreMenuOpen(false)
    void sideData.promoteSideConversation(activeSide.threadId)
  }

  if (minimized) {
    return (
      <button
        type="button"
        onClick={() => setMinimized(false)}
        className={`ds-side-chat-mini ds-no-drag fixed bottom-[112px] z-40 flex h-11 items-center gap-2 rounded-full border border-ds-border-muted bg-ds-card/94 px-3 text-ds-muted shadow-[0_16px_42px_rgba(20,47,95,0.18)] backdrop-blur-xl transition hover:bg-ds-card hover:text-ds-ink ${className ?? ''}`}
        style={rightStyle}
        aria-label={t('sidePanelExpand')}
        title={t('sidePanelExpand')}
      >
        <MessageCircleMore className="h-4 w-4" strokeWidth={1.85} />
        <span className="text-[12px] font-semibold">{Math.max(sideIds.length, 1)}</span>
        {runningCount > 0 ? (
          <span className="h-2 w-2 animate-pulse rounded-full bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.18)]" />
        ) : null}
      </button>
    )
  }

  return (
    <aside
      className={`ds-side-chat ds-no-drag fixed bottom-[112px] z-40 flex max-h-[min(520px,calc(100vh-180px))] w-[min(360px,calc(100vw-24px))] flex-col overflow-hidden rounded-[14px] border border-ds-border bg-ds-card/96 text-ds-ink shadow-[0_22px_64px_rgba(20,47,95,0.2)] backdrop-blur-xl dark:bg-ds-card/96 dark:shadow-[0_24px_72px_rgba(0,0,0,0.46)] ${className ?? ''}`}
      style={rightStyle}
      aria-label={t('sidePanelTitle')}
    >
      <header className="ds-right-panel-topbar border-b border-ds-border-muted">
        <div ref={switchMenuRef} className="ds-right-panel-title-group is-menu-anchor relative">
          <MessageCircleMore className="ds-right-panel-title-icon" strokeWidth={1.85} />
          <button
            type="button"
            onClick={() => sideIds.length > 0 && setSwitchMenuOpen((open) => !open)}
            className="flex min-w-0 flex-shrink-0 items-center gap-1 rounded-md text-left transition hover:text-accent disabled:hover:text-ds-ink"
            disabled={sideIds.length === 0}
            aria-expanded={switchMenuOpen}
            title={title}
          >
            <span className="ds-right-panel-title">{title}</span>
            {sideIds.length > 0 ? (
              <ChevronDown className="h-3 w-3 shrink-0 text-ds-faint" strokeWidth={1.9} />
            ) : null}
          </button>
          <div className="ds-right-panel-subtitle" title={subtitle}>
            {subtitle}
          </div>

          {switchMenuOpen ? (
            <div className="absolute left-0 top-full z-50 mt-2 w-64 overflow-hidden rounded-[12px] border border-ds-border bg-ds-card/98 p-1 shadow-[0_18px_46px_rgba(20,47,95,0.18)] backdrop-blur-xl">
              {currentSides.map((side) => {
                const selected = side.threadId === activeSide?.threadId
                return (
                  <button
                    key={side.threadId}
                    type="button"
                    onClick={() => {
                      sideData.selectSideConversation(side.threadId)
                      setSwitchMenuOpen(false)
                    }}
                    className={`flex min-h-[34px] w-full items-center gap-2 rounded-lg px-2 text-left text-[12.5px] transition ${
                      selected ? 'bg-ds-hover text-ds-ink' : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
                    }`}
                  >
                    <span className="min-w-0 flex-1 truncate" title={side.title}>
                      {compactSideTitle(side.title)}
                    </span>
                    {side.busy ? (
                      <span className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-emerald-500" />
                    ) : null}
                  </button>
                )
              })}
            </div>
          ) : null}
        </div>

        <div className="ds-right-panel-actions">
          <button
            type="button"
            onClick={openDraft}
            className="ds-sidebar-toggle-button ds-right-panel-icon-button"
            aria-label={t('sidePanelNew')}
            title={t('sidePanelNew')}
          >
            <Plus className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
          {activeSide && !activeIsBackgroundAgent ? (
            <button
              type="button"
              onClick={discardActiveSide}
              className="ds-sidebar-toggle-button ds-right-panel-icon-button hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-300"
              aria-label={t('sidePanelDiscardTitle')}
              title={t('sidePanelDiscardTitle')}
            >
              <Trash2 className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
          ) : null}
          {!activeIsBackgroundAgent ? (
          <div ref={moreMenuRef} className="relative">
            <button
              type="button"
              onClick={() => setMoreMenuOpen((open) => !open)}
              disabled={!activeSide}
              className="ds-sidebar-toggle-button ds-right-panel-icon-button disabled:cursor-not-allowed disabled:opacity-35"
              aria-label={t('sidePanelMore')}
              title={t('sidePanelMore')}
              aria-expanded={moreMenuOpen}
            >
              <MoreHorizontal className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
            {moreMenuOpen && activeSide ? (
              <div className="absolute right-0 top-full z-50 mt-2 w-44 overflow-hidden rounded-[12px] border border-ds-border bg-ds-card/98 p-1 text-[12.5px] shadow-[0_18px_46px_rgba(20,47,95,0.18)] backdrop-blur-xl">
                <button
                  type="button"
                  onClick={promoteActiveSide}
                  className="flex min-h-[32px] w-full items-center gap-2 rounded-lg px-2 text-left text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                >
                  <ArrowDownToLine className="h-3.5 w-3.5" strokeWidth={1.8} />
                  <span className="min-w-0 flex-1 truncate">{t('sidePanelPromote')}</span>
                </button>
              </div>
            ) : null}
          </div>
          ) : null}
          <button
            type="button"
            onClick={() => setMinimized(true)}
            className="ds-sidebar-toggle-button ds-right-panel-icon-button"
            aria-label={t('sidePanelMinimize')}
            title={t('sidePanelMinimize')}
          >
            <Minus className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
          <button
            type="button"
            onClick={closeWindow}
            className="ds-sidebar-toggle-button ds-right-panel-icon-button"
            aria-label={t('sidePanelHide')}
            title={t('sidePanelHide')}
          >
            <X className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
        </div>
      </header>

      <div className="flex min-h-[190px] flex-1 flex-col overflow-hidden">
        <div ref={sideScrollRef} className="min-h-0 flex-1 space-y-3 overflow-y-auto px-3 py-3">
          {activeSide ? (
            <>
              <div className="text-[11.5px] leading-4 text-ds-faint">
                {t('sidePanelInheritedAt', {
                  time: formatInheritedTime(activeSide.inheritedAt, i18n.language)
                })}
              </div>
              {activeSide.blocks.length === 0 && !activeSide.busy && !activeSide.liveAssistant ? (
                <div className="flex min-h-[132px] flex-col items-center justify-center gap-2 text-center text-[12.5px] text-ds-faint">
                  <MessageCircleMore className="h-5 w-5 opacity-60" strokeWidth={1.7} />
                  <p>{t('sidePanelEmpty')}</p>
                </div>
              ) : null}
              {activeSide.blocks.map((block) => (
                <SideMessageBubble key={block.id} block={block} />
              ))}
              {activeSide.liveAssistant ? (
                <SideMessageBubble
                  block={{
                    kind: 'assistant',
                    id: 'live-assistant',
                    text: activeSide.liveAssistant
                  }}
                />
              ) : null}
              <SideActiveStreamRows
                threadId={activeSide.threadId}
                turnId={activeSide.turnId}
                scrollRef={sideScrollRef}
                stickToBottomRef={sideStickToBottomRef}
              />
              {activeSide.busy ? (
                <div className="flex items-center gap-2 text-[12px] text-ds-faint">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
                  <span>{t('sidePanelThinking')}</span>
                </div>
              ) : null}
              {activeSide.error ? (
                <div className="rounded-[12px] border border-red-300/70 bg-red-500/10 px-3 py-2 text-[12px] text-red-700 dark:border-red-800/60 dark:bg-red-950/35 dark:text-red-200">
                  {activeSide.error}
                </div>
              ) : null}
            </>
          ) : (
            <div className="flex min-h-[168px] flex-col items-center justify-center gap-2 text-center text-[12.5px] leading-5 text-ds-faint">
              <MessageCircleMore className="h-5 w-5 opacity-65" strokeWidth={1.7} />
              <p>{t('sidePanelDraftEmpty')}</p>
            </div>
          )}
        </div>

        <footer className="shrink-0 border-t border-ds-border-muted px-2.5 pb-2.5 pt-2">
          {activeSide ? (
            <SideChatComposer
              value={activeSide.input}
              onChange={(value) => sideData.setSideInput(activeSide.threadId, value)}
              onSend={sendActiveSide}
              onStop={() => {
                void sideData.interruptSide(activeSide.threadId)
              }}
              busy={activeSide.busy}
              disabled={sideData.runtimeConnection !== 'ready'}
              placeholder={t('sidePanelComposerPlaceholder')}
              stopLabel={t('sidePanelStop')}
            />
          ) : (
            <SideChatComposer
              value={draftInput}
              onChange={setDraftInput}
              onSend={sendDraft}
              disabled={sideData.runtimeConnection !== 'ready'}
              placeholder={t('sidePanelComposerPlaceholder')}
            />
          )}
        </footer>
      </div>
    </aside>
  )
}
