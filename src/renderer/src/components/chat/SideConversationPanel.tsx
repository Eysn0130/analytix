import type { ProcessPaneRoute } from './presentation/chat/ProtectedProcessPane'
import { MessageTimeline } from './MessageTimeline'
import { OwnedSessionStatsPills } from './presentation/chat/TurnUsagePanel'
import { useThreadUsageState } from '../../hooks/use-thread-usage'
import type { ConversationUsageV1 } from '../../../../../packages/runtime/src/contracts/usage'
import { InputBar } from './InputBar'
import type { SideConversation } from '../../store/chat-store-types'
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement
} from 'react'
import { useTranslation } from 'react-i18next'
import { useShallow } from 'zustand/react/shallow'
import {
  ArrowUp as ArrowDownToLine,
  ChevronDown,
  Loader2,
  MessageCircleMore,
  Minus,
  MoreHorizontal,
  Plus,
  Trash2,
  Wrench,
  X
} from '../../design/AnalytixUiIcons'
import { useChatStore } from '../../store/chat-store'

import { GateResponseContext } from './message-timeline-bubbles'

type Props = {
  className?: string
  rightOffset?: number
  onOpenThreadRequest?: (threadId: string, requestId: string) => void
  onRetryConnection?: () => void
  onOpenSettings?: () => void
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

function SideTranscript({ side, conversationUsage, onOpenOwnerThread, processPaneRoute, runtimeConnection, onRetryConnection, onOpenSettings }: {
  processPaneRoute: ProcessPaneRoute; side: SideConversation; conversationUsage: ConversationUsageV1 | null
  runtimeConnection: import('../../agent/types').RuntimeConnectionStatus
  onOpenOwnerThread?: (requestId: string) => void
  onRetryConnection: () => void; onOpenSettings: () => void
}): ReactElement {
  const latestUser = [...side.blocks].reverse().find(block => block.kind === 'user')
  return <GateResponseContext.Provider value={{ onOpenOwnerThread }}>
    <MessageTimeline blocks={side.blocks} live="" activeThreadId={side.threadId}
      runtimeConnection={runtimeConnection} onRetryConnection={onRetryConnection} onOpenSettings={onOpenSettings}
      processPaneRoute={processPaneRoute} conversationUsage={conversationUsage} compactCards contentClassName="max-w-none px-0"
      captureForkOwner={() => {
        const state = useChatStore.getState(), panel = state.sidePanel, captured = state.sideConversations[side.threadId]
        if (!captured || panel.activeSideId !== side.threadId || !panel.open || !processPaneRoute.visible) return null
        const isCurrent = () => {
          const current = useChatStore.getState()
          return current.sidePanel === panel && current.sideConversations[side.threadId] === captured &&
            current.activeThreadId === captured.parentThreadId && !captured.busy
        }
        return { isCurrent, getSnapshot: () => isCurrent() ? { blocks: captured.blocks, busy: captured.busy,
          latestSeq: captured.lastSeq, title: captured.title, rewoundTurnIds: captured.rewoundTurnIds } : null }
      }}
      runtimeStateOverride={{ busy: side.busy, currentTurnId: side.turnId,
        currentTurnUserId: latestUser?.id ?? null, lastTurnUsage: side.lastTurnUsage ?? undefined,
        turnUsageById: side.turnUsageById ?? {} }} />
  </GateResponseContext.Provider>
}

export function SideConversationPanel({
  className,
  rightOffset = 24,
  onOpenThreadRequest, onRetryConnection = () => undefined, onOpenSettings = () => undefined
}: Props): ReactElement | null {
  const { t, i18n } = useTranslation('common')
  const [processPaneOwnerKey] = useState(() => crypto.randomUUID())
  const [draftInput, setDraftInput] = useState('')
  const [minimized, setMinimized] = useState(false)
  const [switchMenuOpen, setSwitchMenuOpen] = useState(false)
  const [moreMenuOpen, setMoreMenuOpen] = useState(false)
  const switchMenuRef = useRef<HTMLDivElement | null>(null)
  const moreMenuRef = useRef<HTMLDivElement | null>(null)
  const prevOpenRef = useRef(false)

  const sideData = useChatStore(
    useShallow((s) => ({
      sides: s.sideConversations,
      panel: s.sidePanel,
      parentThreadId: s.activeThreadId,
      threads: s.threads,
      runtimeConnection: s.runtimeConnection,
      usageRefreshKey: s.usageRefreshKey,
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
  const processPaneRoute = useMemo<ProcessPaneRoute>(() => ({
    surface: 'side', ownerKey: processPaneOwnerKey,
    selectionKey: JSON.stringify([sideData.parentThreadId, activeId]),
    visible: shouldRender && !minimized && Boolean(activeSide)
  }), [processPaneOwnerKey, sideData.parentThreadId, activeId, shouldRender, minimized, activeSide])
  const sideUsage = useThreadUsageState(activeId, processPaneRoute.visible && sideData.runtimeConnection === 'ready',
    `${sideData.usageRefreshKey}:${activeSide?.busy ? 'running' : 'settled'}:${activeSide?.turnId ?? ''}:${Object.values(activeSide?.turnUsageById ?? {}).map(entry => entry.lastSeq).join(',')}`,
    `${sideData.runtimeConnection}:${JSON.stringify(Object.keys(activeSide?.rewoundTurnIds ?? {}).sort())}`)
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
      if (event.defaultPrevented || event.repeat || event.isComposing || event.keyCode === 229) return
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
        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-3 py-3">
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
              <SideTranscript onRetryConnection={onRetryConnection} onOpenSettings={onOpenSettings} processPaneRoute={processPaneRoute} side={activeSide} conversationUsage={sideUsage.conversationUsage} runtimeConnection={sideData.runtimeConnection}
                onOpenOwnerThread={onOpenThreadRequest ? requestId => onOpenThreadRequest(activeSide.threadId, requestId) : undefined} />
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
          {activeSide ? <OwnedSessionStatsPills threadId={activeSide.threadId} projection={sideUsage.conversationUsage ? {
            threadId: activeSide.threadId, billing: sideUsage.conversationUsage.billing, metrics: sideUsage.conversationUsage.metrics
          } : undefined} /> : null}
          <InputBar
            ownerThreadId={activeSide?.threadId ?? null}
            gateOverride={{ threadId: activeSide?.threadId ?? null,
              block: activeSide?.blocks.find((block): block is Extract<import('../../agent/types').ChatBlock, { kind: 'approval' | 'user_input' }> =>
                (block.kind === 'approval' || block.kind === 'user_input') && block.status === 'pending') ?? null,
              onOpenOwnerThread: activeSide && onOpenThreadRequest ? requestId => onOpenThreadRequest(activeSide.threadId, requestId) : undefined }}
            variant="compact" hideThreadContextPanels hideModelPicker hideBtwCommand allowBusyFollowUp={false}
            input={activeSide?.input ?? draftInput}
            setInput={value => activeSide ? sideData.setSideInput(activeSide.threadId, value) : setDraftInput(value)}
            mode="agent" setMode={() => undefined}
            busy={activeSide?.busy ?? false} runtimeReady={sideData.runtimeConnection === 'ready'}
            hasActiveThread={Boolean(activeSide)} workspaceRootOverride={parentThread?.workspace}
            composerModel={activeSide?.model ?? ''} composerPickList={[]} onComposerModelChange={() => undefined}
            queuedMessages={[]} onRemoveQueuedMessage={() => undefined}
            attachmentUploadEnabled={false} fileReferenceEnabled={false}
            onSend={activeSide ? sendActiveSide : sendDraft}
            onInterrupt={() => { if (activeSide) void sideData.interruptSide(activeSide.threadId) }}
          />
        </footer>
      </div>
    </aside>
  )
}
