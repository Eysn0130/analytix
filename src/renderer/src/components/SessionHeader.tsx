import type { CSSProperties, ReactElement } from 'react'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  Archive,
  Clock3,
  Copy,
  GitBranch,
  GitFork,
  MessageCirclePlus,
  MoreHorizontal,
  PencilLine,
  RotateCcw,
  SquareStack,
  TableProperties,
  X
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FundsCSVSnapshotStageResult } from '@shared/analytix-api'
import { useChatStore } from '../store/chat-store'
import { ActionMenuItem, ActionMenuSeparator } from './common/ActionMenu'
import { formatRelativeTime } from '../lib/format-relative-time'
import { readPinnedThreadIds, subscribePinnedThreadIds, togglePinnedThreadId } from '../lib/pinned-threads'
import { buildThreadMarkdown } from '../lib/thread-markdown'
import {
  buildAnalytixThreadLink,
  buildCodexCompatibleThreadLink
} from '../lib/thread-links'
import { workspaceLabelFromPath } from '../lib/workspace-label'
import {
  formatCompactNumber,
  formatCost,
  formatPercent,
  primaryCacheHitRate,
  useThreadUsage
} from '../hooks/use-thread-usage'
import { AnalytixIconRegistry } from '../design/AnalytixIconRegistry'
import { shouldAutoTitleThread } from '../lib/thread-title'
import {
  DirectSourcePreview,
  ImportMappingPreview,
  useLocalDisplayAuthorityGeneration
} from './chat/AcceptedSlotDisplay'
import { emitStatsCacheInvalidation } from '../data-analysis/services/analysis/stats-cache-events'
import { getSharedActiveCaseContext } from '../data-analysis/services/analysis/stats-case-overview-resource'

type Props = {
  compact?: boolean
  className?: string
  onOpenSideChat?: () => void
}

type ThreadActionsSubmenu = 'copy' | 'branch' | null
type ActionsMenuPlacement = {
  left: number
  top: number
}
type StagedFundsImport = Extract<FundsCSVSnapshotStageResult, { ok: true }>
type TogglePinnedThreadShortcutEvent = {
  key: string
  altKey?: boolean
  metaKey?: boolean
  ctrlKey?: boolean
  shiftKey?: boolean
  repeat?: boolean
  isComposing?: boolean
  defaultPrevented?: boolean
}

export function isTogglePinnedThreadShortcut(event: TogglePinnedThreadShortcutEvent): boolean {
  if (event.defaultPrevented || event.repeat || event.isComposing) return false
  if (event.key.toLowerCase() !== 'p') return false
  return event.altKey === true && event.metaKey === true && event.ctrlKey !== true && event.shiftKey !== true
}

const ACTIONS_MENU_WIDTH = 216
const ACTIONS_MENU_VIEWPORT_MARGIN = 8
const ACTIONS_MENU_TRIGGER_GAP = 6
const useSessionHeaderLayoutEffect = typeof window === 'undefined' ? useEffect : useLayoutEffect

export function SessionHeader({ compact = false, className = '', onOpenSideChat }: Props): ReactElement {
  const { t, i18n } = useTranslation('common')
  const threads = useChatStore((s) => s.threads)
  const activeThreadId = useChatStore((s) => s.activeThreadId)
  const busy = useChatStore((s) => s.busy)
  const runtimeConnection = useChatStore((s) => s.runtimeConnection)
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const workspaceLabel = useChatStore((s) => s.workspaceLabel)
  const blocks = useChatStore((s) => s.blocks)
  const renameActiveThread = useChatStore((s) => s.renameActiveThread)
  const archiveThread = useChatStore((s) => s.archiveThread)
  const forkActiveThread = useChatStore((s) => s.forkActiveThread)
  const openSchedule = useChatStore((s) => s.openSchedule)
  const setError = useChatStore((s) => s.setError)

  const storeSnapshot = useChatStore.getState()
  const active = threads.find((th) => th.id === activeThreadId) ??
    storeSnapshot.threads.find((th) => th.id === (activeThreadId || storeSnapshot.activeThreadId))
  const activeTitle = active && !shouldAutoTitleThread(active) ? active.title : ''
  const activeWorkspaceLabel =
    workspaceLabel || storeSnapshot.workspaceLabel || (active?.workspace ? workspaceLabelFromPath(active.workspace) : '')
  const [editing, setEditing] = useState(false)
  const [draftTitle, setDraftTitle] = useState('')
  const [actionsMenuOpen, setActionsMenuOpen] = useState(false)
  const [actionsMenuPlacement, setActionsMenuPlacement] = useState<ActionsMenuPlacement | null>(null)
  const [actionsSubmenu, setActionsSubmenu] = useState<ThreadActionsSubmenu>(null)
  const [sourcePreviewOpen, setSourcePreviewOpen] = useState(false)
  const [fundsCSVStageState, setFundsCSVStageState] = useState<'idle' | 'running' | 'staged' | 'confirming' | 'success' | 'failed'>('idle')
  const [stagedFundsImport, setStagedFundsImport] = useState<StagedFundsImport | null>(null)
  const [selectedImportItem, setSelectedImportItem] = useState(0)
  const [pinnedThreadIds, setPinnedThreadIds] = useState(() => readPinnedThreadIds())
  const actionsMenuRef = useRef<HTMLDivElement>(null)
  const actionsMenuPortalRef = useRef<HTMLDivElement>(null)
  const actionsMenuPortalTarget = typeof document === 'undefined' ? null : document.body
  const localDisplayAuthority = useLocalDisplayAuthorityGeneration()
  const localDisplayAuthorityRef = useRef(localDisplayAuthority)
  const stagedImportSelectorRef = useRef<string | null>(null)
  const fundsImportRequestGenerationRef = useRef(0)
  // Usage stats are no longer shown in compact mode (the composer footer
  // already shows them in the chat route), so skip fetching there.
  const threadUsage = useThreadUsage(
    activeThreadId,
    runtimeConnection === 'ready' && !compact,
    `${active?.updatedAt ?? ''}:${busy ? 'busy' : 'idle'}`
  )
  const forkedFromTitle = active?.forkedFromTitle?.trim() ?? ''
  const forkLabel =
    active?.forkedFromThreadId
      ? forkedFromTitle
        ? t('sessionForkedFrom', { title: forkedFromTitle })
        : t('sessionForked')
      : ''

  useEffect(() => {
    if (active) {
      setDraftTitle(active.title)
    } else {
      setDraftTitle('')
    }
    setEditing(false)
    setActionsMenuOpen(false)
    setActionsSubmenu(null)
  }, [active])

  useEffect(() => {
    const selector = stagedImportSelectorRef.current
    fundsImportRequestGenerationRef.current += 1
    if (selector) void window.analytix.runtime.cancelFundsCSVImport(selector)
    stagedImportSelectorRef.current = null
    setStagedFundsImport(null)
    setSelectedImportItem(0)
    setSourcePreviewOpen(false)
    setFundsCSVStageState('idle')
  }, [active?.id])

  useEffect(() => {
    if (localDisplayAuthorityRef.current === localDisplayAuthority) return
    localDisplayAuthorityRef.current = localDisplayAuthority
    const selector = stagedImportSelectorRef.current
    fundsImportRequestGenerationRef.current += 1
    if (selector) void window.analytix.runtime.cancelFundsCSVImport(selector)
    stagedImportSelectorRef.current = null
    setStagedFundsImport(null)
    setSelectedImportItem(0)
    setFundsCSVStageState('idle')
  }, [localDisplayAuthority])

  useEffect(() => () => {
    fundsImportRequestGenerationRef.current += 1
    const selector = stagedImportSelectorRef.current
    if (selector) void window.analytix.runtime.cancelFundsCSVImport(selector)
    stagedImportSelectorRef.current = null
  }, [])

  useEffect(() => {
    return subscribePinnedThreadIds(() => setPinnedThreadIds(readPinnedThreadIds()))
  }, [])

  useEffect(() => {
    if (!actionsMenuOpen) return
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (target instanceof Node && actionsMenuRef.current?.contains(target)) return
      if (target instanceof Node && actionsMenuPortalRef.current?.contains(target)) return
      setActionsMenuOpen(false)
      setActionsMenuPlacement(null)
      setActionsSubmenu(null)
    }
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== 'Escape') return
      setActionsMenuOpen(false)
      setActionsMenuPlacement(null)
      setActionsSubmenu(null)
    }
    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [actionsMenuOpen])

  useEffect(() => {
    if (!sourcePreviewOpen) return
    const closeOnEscape = (event: KeyboardEvent): void => {
      if (event.key !== 'Escape') return
      setSourcePreviewOpen(false)
      fundsImportRequestGenerationRef.current += 1
      const selector = stagedImportSelectorRef.current
      stagedImportSelectorRef.current = null
      setStagedFundsImport(null)
      setSelectedImportItem(0)
      setFundsCSVStageState('idle')
      if (selector) void window.analytix.runtime.cancelFundsCSVImport(selector)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [sourcePreviewOpen])

  const positionActionsMenu = useCallback((): void => {
    const anchor = actionsMenuRef.current
    if (!anchor || typeof window === 'undefined' || typeof document === 'undefined') return
    const rect = anchor.getBoundingClientRect()
    const viewportWidth = document.documentElement.clientWidth || window.innerWidth
    const maxLeft = Math.max(ACTIONS_MENU_VIEWPORT_MARGIN, viewportWidth - ACTIONS_MENU_WIDTH - ACTIONS_MENU_VIEWPORT_MARGIN)
    const left = Math.min(Math.max(rect.left, ACTIONS_MENU_VIEWPORT_MARGIN), maxLeft)
    const top = Math.max(rect.bottom + ACTIONS_MENU_TRIGGER_GAP, ACTIONS_MENU_VIEWPORT_MARGIN)
    setActionsMenuPlacement({ left, top })
  }, [])

  useSessionHeaderLayoutEffect(() => {
    if (!actionsMenuOpen) return undefined
    positionActionsMenu()
    window.addEventListener('resize', positionActionsMenu)
    window.addEventListener('scroll', positionActionsMenu, true)
    return () => {
      window.removeEventListener('resize', positionActionsMenu)
      window.removeEventListener('scroll', positionActionsMenu, true)
    }
  }, [actionsMenuOpen, positionActionsMenu])

  const commitTitle = (): void => {
    if (!active) {
      setEditing(false)
      return
    }
    const next = draftTitle.trim()
    if (!next || next === active.title) {
      setDraftTitle(active.title)
      setEditing(false)
      return
    }
    void renameActiveThread(next).finally(() => setEditing(false))
  }

  const closeActionsMenu = (): void => {
    setActionsMenuOpen(false)
    setActionsMenuPlacement(null)
    setActionsSubmenu(null)
  }

  const toggleActionsMenu = (): void => {
    setActionsSubmenu(null)
    if (actionsMenuOpen) {
      setActionsMenuPlacement(null)
      setActionsMenuOpen(false)
      return
    }
    positionActionsMenu()
    setActionsMenuOpen(true)
  }

  const beginRename = (): void => {
    if (!active) return
    setDraftTitle(active.title)
    setEditing(true)
    closeActionsMenu()
  }

  const togglePin = (): void => {
    if (!active) return
    togglePinnedThreadId(active.id)
    setPinnedThreadIds(readPinnedThreadIds())
    closeActionsMenu()
  }

  useEffect(() => {
    if (!active?.id || editing) return undefined
    const onKeyDown = (event: KeyboardEvent): void => {
      if (!isTogglePinnedThreadShortcut(event)) return
      event.preventDefault()
      togglePinnedThreadId(active.id)
      setPinnedThreadIds(readPinnedThreadIds())
      closeActionsMenu()
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [active?.id, editing])

  const archiveCurrentThread = (): void => {
    if (!active) return
    closeActionsMenu()
    void archiveThread(active.id, active.archived !== true)
  }

  const openSideChat = (): void => {
    if (!active) return
    closeActionsMenu()
    onOpenSideChat?.()
  }

  const openSourcePreview = (): void => {
    if (!workspaceRoot.trim() || runtimeConnection !== 'ready') return
    closeActionsMenu()
    setSourcePreviewOpen(true)
  }

  const stageFundsCSVSnapshot = async (): Promise<void> => {
    if (fundsCSVStageState === 'running' || fundsCSVStageState === 'confirming') return
    setFundsCSVStageState('running')
    setStagedFundsImport(null)
    setSelectedImportItem(0)
    const previousSelector = stagedImportSelectorRef.current
    stagedImportSelectorRef.current = null
    const requestGeneration = ++fundsImportRequestGenerationRef.current
    try {
      if (previousSelector) {
        await window.analytix.runtime.cancelFundsCSVImport(previousSelector)
      }
      const result = await window.analytix.runtime.stageFundsCSVSnapshot()
      if (requestGeneration !== fundsImportRequestGenerationRef.current) {
        if (result.ok && result.items[0]?.selector) {
          await window.analytix.runtime.cancelFundsCSVImport(result.items[0].selector).catch(() => undefined)
        }
        return
      }
      if (!result.ok) {
        setFundsCSVStageState(result.canceled ? 'idle' : 'failed')
        return
      }
      stagedImportSelectorRef.current = result.items[0]?.selector ?? null
      setStagedFundsImport(result)
      setFundsCSVStageState('staged')
    } catch {
      if (requestGeneration !== fundsImportRequestGenerationRef.current) return
      setFundsCSVStageState('failed')
    }
  }

  const cancelStagedFundsImport = async (): Promise<void> => {
    fundsImportRequestGenerationRef.current += 1
    const selector = stagedImportSelectorRef.current
    stagedImportSelectorRef.current = null
    setStagedFundsImport(null)
    setSelectedImportItem(0)
    setFundsCSVStageState('idle')
    if (selector) {
      await window.analytix.runtime.cancelFundsCSVImport(selector).catch(() => undefined)
    }
  }

  const closeSourcePreview = (): void => {
    setSourcePreviewOpen(false)
    void cancelStagedFundsImport()
  }

  const confirmFundsCSVSnapshot = async (): Promise<void> => {
    const selector = stagedFundsImport?.items[selectedImportItem]?.selector
    if (!selector || stagedFundsImport.status !== 'ready' || fundsCSVStageState === 'confirming') return
    setFundsCSVStageState('confirming')
    const requestGeneration = ++fundsImportRequestGenerationRef.current
    try {
      const status = await window.analytix.runtime.statusFundsCSVImport(selector)
      if (requestGeneration !== fundsImportRequestGenerationRef.current) return
      if (!status.ok || status.status !== 'ready' || status.item.selector !== selector) {
        await window.analytix.runtime.cancelFundsCSVImport(selector).catch(() => undefined)
        stagedImportSelectorRef.current = null
        setStagedFundsImport(null)
        setFundsCSVStageState('failed')
        return
      }
      const result = await window.analytix.runtime.confirmFundsCSVSnapshot(selector)
      if (requestGeneration !== fundsImportRequestGenerationRef.current) return
      stagedImportSelectorRef.current = null
      setStagedFundsImport(null)
      setSelectedImportItem(0)
      if (!result.ok) {
        setFundsCSVStageState('failed')
        return
      }
      const activeCase = getSharedActiveCaseContext()
      if (activeCase?.caseId) {
        emitStatsCacheInvalidation({
          caseId: activeCase.caseId,
          eventId: `funds-import-confirm:${Date.now()}`,
          source: 'funds-csv-admission',
          occurredAt: Date.now()
        })
      }
      setFundsCSVStageState('success')
    } catch {
      if (requestGeneration !== fundsImportRequestGenerationRef.current) return
      stagedImportSelectorRef.current = null
      setStagedFundsImport(null)
      setFundsCSVStageState('failed')
    }
  }

  const copyText = async (text: string): Promise<void> => {
    try {
      if (!navigator?.clipboard?.writeText) throw new Error('Clipboard unavailable')
      await navigator.clipboard.writeText(text)
      setError(null)
    } catch {
      setError(t('copyFailed'))
    } finally {
      closeActionsMenu()
    }
  }

  const copyThreadMarkdown = (): void => {
    if (!active) return
    void copyText(buildThreadMarkdown(active, activeWorkspaceLabel, blocks))
  }

  const copyThreadId = (): void => {
    if (!active) return
    void copyText(active.id)
  }

  const copyWorkspacePath = (): void => {
    if (!active) return
    void copyText(active.workspace ?? workspaceLabel)
  }

  const copyThreadDeeplink = (): void => {
    if (!active) return
    void copyText(buildAnalytixThreadLink(active.id))
  }

  const copyCodexThreadLink = (): void => {
    if (!active) return
    void copyText(buildCodexCompatibleThreadLink(active.id))
  }

  const forkCurrentThread = (): void => {
    closeActionsMenu()
    void forkActiveThread()
  }

  const openAutomations = (): void => {
    closeActionsMenu()
    openSchedule()
  }

  const openCurrentThreadInNewWindow = (): void => {
    if (!active) return
    closeActionsMenu()
    if (typeof window.analytix?.app?.openThreadInNewWindow !== 'function') {
      setError(t('sessionActionUnavailable'))
      return
    }
    void window.analytix.app.openThreadInNewWindow(active.id).catch(() => {
      setError(t('sessionActionUnavailable'))
    })
  }

  const activePinned = active ? pinnedThreadIds.has(active.id) : false
  const archived = active?.archived === true
  const threadActionsDisabled = !active || runtimeConnection !== 'ready'
  const sourcePreviewDisabled = threadActionsDisabled || !workspaceRoot.trim()

  const renderThreadActionsMenu = (floating = false): ReactElement | null => {
    if (!active) return null
    const PinActionIcon = activePinned ? AnalytixIconRegistry.icons.pinFilled : AnalytixIconRegistry.icons.pin
    const menuStyle: CSSProperties | undefined = floating && actionsMenuPlacement
      ? {
          left: actionsMenuPlacement.left,
          top: actionsMenuPlacement.top
        }
      : undefined
    return (
      <div
        ref={floating ? actionsMenuPortalRef : undefined}
        role="menu"
        aria-label={t('sessionActionsMenuTitle')}
        className={`ds-session-actions-menu${floating ? ' ds-session-actions-menu-floating ds-no-drag' : ''}`}
        style={menuStyle}
        onPointerLeave={() => setActionsSubmenu(null)}
      >
        <ActionMenuItem
          icon={<PinActionIcon className="h-5 w-5" />}
          label={activePinned ? t('sessionActionUnpin') : t('sessionActionPin')}
          shortcut="⌥⌘P"
          active={activePinned}
          onClick={togglePin}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
        <ActionMenuItem
          icon={<PencilLine className="h-5 w-5" strokeWidth={1.9} />}
          label={t('sessionActionRename')}
          shortcut="⌥⌘R"
          disabled={threadActionsDisabled}
          title={threadActionsDisabled ? t('runtimeActionNeedsConnection') : undefined}
          onClick={beginRename}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
        <ActionMenuItem
          icon={archived ? <RotateCcw className="h-5 w-5" strokeWidth={1.9} /> : <Archive className="h-5 w-5" strokeWidth={1.9} />}
          label={archived ? t('sessionActionRestore') : t('sessionActionArchive')}
          shortcut="⇧⌘A"
          disabled={threadActionsDisabled}
          title={threadActionsDisabled ? t('runtimeActionNeedsConnection') : undefined}
          onClick={archiveCurrentThread}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
        <ActionMenuSeparator />
        <ActionMenuItem
          icon={<MessageCirclePlus className="h-5 w-5" strokeWidth={1.9} />}
          label={t('sessionActionOpenSideChat')}
          shortcut="⌥⌘S"
          disabled={threadActionsDisabled}
          title={threadActionsDisabled ? t('runtimeActionNeedsConnection') : undefined}
          onClick={openSideChat}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
        <ActionMenuItem
          icon={<TableProperties className="h-5 w-5" strokeWidth={1.9} />}
          label={t('sessionActionDataImport')}
          disabled={sourcePreviewDisabled}
          title={sourcePreviewDisabled ? t('runtimeActionNeedsConnection') : undefined}
          onClick={openSourcePreview}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
        <div className="ds-session-actions-menu-submenu-anchor">
          <ActionMenuItem
            icon={<Copy className="h-5 w-5" strokeWidth={1.9} />}
            label={t('sessionActionCopy')}
            hasSubmenu
            onClick={() => setActionsSubmenu(actionsSubmenu === 'copy' ? null : 'copy')}
            onPointerEnter={() => setActionsSubmenu('copy')}
          />
          {actionsSubmenu === 'copy' ? (
            <div className="ds-session-actions-submenu" role="menu" aria-label={t('sessionActionCopy')}>
              <ActionMenuItem
                icon={<Copy className="h-5 w-5" strokeWidth={1.9} />}
                label={t('sessionActionCopyMarkdown')}
                onClick={copyThreadMarkdown}
              />
              <ActionMenuItem
                icon={<SquareStack className="h-5 w-5" strokeWidth={1.9} />}
                label={t('sessionActionCopyThreadId')}
                onClick={copyThreadId}
              />
              <ActionMenuItem
                icon={<Copy className="h-5 w-5" strokeWidth={1.9} />}
                label={t('sessionActionCopyWorkspacePath')}
                disabled={!active.workspace && !workspaceLabel}
                onClick={copyWorkspacePath}
              />
              <ActionMenuItem
                icon={<Copy className="h-5 w-5" strokeWidth={1.9} />}
                label={t('sessionActionCopyDeeplink')}
                onClick={copyThreadDeeplink}
              />
              <ActionMenuItem
                icon={<Copy className="h-5 w-5" strokeWidth={1.9} />}
                label={t('sessionActionCopyCodexLink')}
                onClick={copyCodexThreadLink}
              />
            </div>
          ) : null}
        </div>
        <div className="ds-session-actions-menu-submenu-anchor">
          <ActionMenuItem
            icon={<GitBranch className="h-5 w-5" strokeWidth={1.9} />}
            label={t('sessionActionBranch')}
            hasSubmenu
            disabled={threadActionsDisabled || busy}
            title={busy ? t('threadActionBusy') : threadActionsDisabled ? t('runtimeActionNeedsConnection') : undefined}
            onClick={() => setActionsSubmenu(actionsSubmenu === 'branch' ? null : 'branch')}
            onPointerEnter={() => setActionsSubmenu('branch')}
          />
          {actionsSubmenu === 'branch' ? (
            <div className="ds-session-actions-submenu" role="menu" aria-label={t('sessionActionBranch')}>
              <ActionMenuItem
                icon={<GitFork className="h-5 w-5" strokeWidth={1.9} />}
                label={t('sessionActionForkCurrent')}
                disabled={threadActionsDisabled || busy}
                onClick={forkCurrentThread}
              />
            </div>
          ) : null}
        </div>
        <ActionMenuItem
          icon={<Clock3 className="h-5 w-5" strokeWidth={1.9} />}
          label={t('sessionActionAddAutomation')}
          onClick={openAutomations}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
        <ActionMenuSeparator />
        <ActionMenuItem
          icon={<SquareStack className="h-5 w-5" strokeWidth={1.9} />}
          label={t('sessionActionOpenInNewWindow')}
          disabled={threadActionsDisabled}
          title={threadActionsDisabled ? t('runtimeActionNeedsConnection') : undefined}
          onClick={openCurrentThreadInNewWindow}
          onPointerEnter={() => setActionsSubmenu(null)}
        />
      </div>
    )
  }

  const actionsMenuPortal = actionsMenuOpen && actionsMenuPortalTarget
    ? createPortal(renderThreadActionsMenu(true), actionsMenuPortalTarget)
    : null
  const sourcePreviewPortal = sourcePreviewOpen && actionsMenuPortalTarget
    ? createPortal(
        <div
          className="fixed inset-0 z-[100] flex items-center justify-center bg-black/45 p-4 ds-no-drag"
          role="presentation"
          onPointerDown={(event) => {
            if (event.target === event.currentTarget) closeSourcePreview()
          }}
        >
          <section
            role="dialog"
            aria-modal="true"
            aria-label={t('directSourcePreviewTitle')}
            className="max-h-[82vh] w-[min(96vw,1120px)] overflow-hidden rounded-2xl border border-ds-border bg-ds-elevated p-4 shadow-2xl"
          >
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-sm font-semibold text-ds-ink">{t('directSourcePreviewTitle')}</h2>
              <button
                type="button"
                className="rounded-md p-1 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={t('close')}
                onClick={closeSourcePreview}
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <button
                type="button"
                className="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition hover:opacity-90 disabled:cursor-wait disabled:opacity-60"
                disabled={fundsCSVStageState === 'running' || fundsCSVStageState === 'confirming'}
                onClick={() => void stageFundsCSVSnapshot()}
              >
                {fundsCSVStageState === 'running' ? t('fundsCSVStageRunning') :
                  stagedFundsImport ? t('fundsCSVReselectButton') : t('fundsCSVStageButton')}
              </button>
              {stagedFundsImport ? (
                <>
                  <button
                    type="button"
                    className="rounded-lg border border-ds-border px-3 py-1.5 text-xs font-medium text-ds-ink transition hover:bg-ds-hover disabled:cursor-not-allowed disabled:opacity-50"
                    disabled={stagedFundsImport.status !== 'ready' || fundsCSVStageState === 'confirming'}
                    onClick={() => void confirmFundsCSVSnapshot()}
                  >
                    {fundsCSVStageState === 'confirming' ? t('fundsCSVConfirmRunning') : t('fundsCSVConfirmButton')}
                  </button>
                  <button
                    type="button"
                    className="rounded-lg px-3 py-1.5 text-xs text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                    onClick={() => void cancelStagedFundsImport()}
                  >
                    {t('fundsCSVCancelButton')}
                  </button>
                </>
              ) : null}
              {fundsCSVStageState === 'success' ? (
                <p className="text-xs text-emerald-600">{t('fundsCSVStageSuccess')}</p>
              ) : fundsCSVStageState === 'failed' ? (
                <p className="text-xs text-ds-danger">{t('fundsCSVStageFailed')}</p>
              ) : stagedFundsImport?.status === 'mapping_invalid' ? (
                <p className="text-xs text-ds-danger">{t('fundsCSVMappingInvalid')}</p>
              ) : null}
            </div>
            {stagedFundsImport ? (
              <div className="mt-3 border-t border-ds-border pt-3">
                <div className="mb-2 flex flex-wrap gap-2">
                  {stagedFundsImport.items.map((item, index) => (
                    <button
                      key={item.selector}
                      type="button"
                      className={`rounded-md px-2.5 py-1 text-xs transition ${index === selectedImportItem ? 'bg-accent text-white' : 'bg-ds-hover text-ds-muted hover:text-ds-ink'}`}
                      onClick={() => setSelectedImportItem(index)}
                    >
                      {item.sourceLabel}
                    </button>
                  ))}
                </div>
                {stagedFundsImport.items[selectedImportItem] ? (
                  <ImportMappingPreview selector={stagedFundsImport.items[selectedImportItem].selector} />
                ) : null}
              </div>
            ) : null}
            <DirectSourcePreview />
          </section>
        </div>,
        actionsMenuPortalTarget
      )
    : null

  if (compact) {
    return (
      <>
        <div
          className={`session-header-compact flex min-h-0 min-w-0 flex-1 items-center gap-2 text-left ${className}`}
        >
          {active ? (
            <div className="min-w-0 flex-1">
              {activeTitle || editing ? (
                <div className="flex min-w-0 items-center gap-1.5">
                  {editing ? (
                    <input
                      className="ds-session-title-compact min-w-0 flex-1 rounded-lg border border-ds-border bg-ds-elevated px-2 py-1 text-[13px] font-semibold leading-[18px] tracking-[-0.01em] text-ds-ink focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/20"
                      value={draftTitle}
                      onChange={(e) => setDraftTitle(e.target.value)}
                      onBlur={() => commitTitle()}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          commitTitle()
                        }
                        if (e.key === 'Escape') {
                          setDraftTitle(active.title)
                          setEditing(false)
                        }
                      }}
                      aria-label={t('renameThreadHint')}
                      autoFocus
                    />
                  ) : (
                    <div
                      className="ds-session-title-compact min-w-0 truncate text-[13px] font-semibold leading-[17px] tracking-[-0.01em] text-ds-ink opacity-95"
                      title={activeTitle}
                    >
                      {activeTitle}
                    </div>
                  )}
                  {activeTitle ? (
                    <div ref={actionsMenuRef} className="ds-session-actions-anchor">
                      <button
                        type="button"
                        className="ds-session-actions-trigger"
                        data-state={actionsMenuOpen ? 'open' : 'closed'}
                        aria-label={t('sessionActionsMenuTitle')}
                        aria-expanded={actionsMenuOpen}
                        title={t('sessionActionsMenuTitle')}
                        onClick={toggleActionsMenu}
                      >
                        <MoreHorizontal className="ds-session-actions-trigger-icon" strokeWidth={2} />
                      </button>
                      {actionsMenuOpen && !actionsMenuPortalTarget ? renderThreadActionsMenu() : null}
                    </div>
                  ) : null}
                </div>
              ) : null}
              <div className="session-header-compact-meta flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10.5px] leading-[14px] text-ds-faint">
                <span className="session-meta-workspace max-w-[min(42vw,240px)] truncate">{activeWorkspaceLabel}</span>
                <span className="session-meta-workspace-separator opacity-70">·</span>
                <span className="session-meta-mode shrink-0 capitalize">{active.mode}</span>
                <span className="session-meta-mode-separator opacity-70">·</span>
                <span className="session-meta-time shrink-0 tabular-nums">
                  {formatRelativeTime(active.updatedAt, i18n.language)}
                </span>
                {active.forkedFromThreadId ? (
                  <>
                    <span className="session-meta-fork-separator opacity-70">·</span>
                    <span
                      className="session-meta-fork inline-flex min-w-0 max-w-[min(34vw,220px)] items-center gap-1 truncate"
                      title={forkLabel}
                    >
                      <GitFork className="h-3 w-3 shrink-0" strokeWidth={1.8} />
                      <span className="truncate">
                        {forkedFromTitle
                          ? t('sessionForkedFromCompact', { title: forkedFromTitle })
                          : t('sessionForked')}
                      </span>
                    </span>
                  </>
                ) : null}
              </div>
            </div>
          ) : (
            <div className="min-w-0 flex-1" aria-hidden />
          )}
        </div>
        {actionsMenuPortal}
        {sourcePreviewPortal}
      </>
    )
  }

  return (
    <>
      <div className={`flex min-h-[74px] min-w-0 flex-1 items-center gap-4 px-5 py-4 sm:px-6 ${className}`}>
        {active && activeTitle ? (
          <>
            <div className="min-w-0 flex-1">
              <div className="mb-1 flex min-w-0 items-center gap-2 text-[12.5px] font-medium text-ds-faint">
                <span>{activeWorkspaceLabel}</span>
                <span>·</span>
                <span className="capitalize">{active.mode}</span>
                <span>·</span>
                <span>{formatRelativeTime(active.updatedAt, i18n.language)}</span>
              </div>
              <div className="flex min-w-0 items-center gap-2.5">
                {editing ? (
                  <input
                    className="min-w-0 flex-1 rounded-2xl border border-ds-border bg-ds-elevated px-3.5 py-2 text-[21px] font-semibold tracking-[-0.02em] text-ds-ink focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/20"
                    value={draftTitle}
                    onChange={(e) => setDraftTitle(e.target.value)}
                    onBlur={() => commitTitle()}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        commitTitle()
                      }
                      if (e.key === 'Escape') {
                        setDraftTitle(active.title)
                        setEditing(false)
                      }
                    }}
                    aria-label={t('renameThreadHint')}
                    autoFocus
                  />
                ) : (
                  <button
                    type="button"
                    className="min-w-0 truncate text-left text-[22px] font-semibold tracking-[-0.03em] text-ds-ink transition hover:text-accent"
                    title={t('renameThreadHint')}
                    onClick={() => setEditing(true)}
                  >
                    {activeTitle}
                  </button>
                )}
                <div ref={actionsMenuRef} className="ds-session-actions-anchor">
                  <button
                    type="button"
                    className="ds-session-actions-trigger ds-session-actions-trigger-lg"
                    data-state={actionsMenuOpen ? 'open' : 'closed'}
                    aria-label={t('sessionActionsMenuTitle')}
                    aria-expanded={actionsMenuOpen}
                    title={t('sessionActionsMenuTitle')}
                    onClick={toggleActionsMenu}
                  >
                    <MoreHorizontal className="ds-session-actions-trigger-icon" strokeWidth={2} />
                  </button>
                  {actionsMenuOpen && !actionsMenuPortalTarget ? renderThreadActionsMenu() : null}
                </div>
              </div>
              <div className="mt-2 flex min-w-0 flex-wrap items-center gap-2 text-[12.5px] text-ds-faint">
                <span className="inline-flex items-center rounded-full border border-ds-border bg-ds-subtle px-2.5 py-1 font-medium capitalize text-ds-muted">
                  {active.mode}
                </span>
                {active.workspace ? (
                  <span className="truncate rounded-full border border-ds-border bg-ds-card/70 px-2.5 py-1">
                    {active.workspace.split(/[/\\]/).pop()}
                  </span>
                ) : null}
                {active.forkedFromThreadId ? (
                  <span
                    className="inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-full border border-accent/18 bg-accent/8 px-2.5 py-1 font-medium text-accent"
                    title={forkLabel}
                  >
                    <GitFork className="h-3.5 w-3.5 shrink-0" strokeWidth={1.8} />
                    <span className="truncate">{forkLabel}</span>
                  </span>
                ) : null}
                {threadUsage ? (
                  <>
                    <span
                      className="inline-flex items-center rounded-full border border-ds-border bg-ds-subtle px-2.5 py-1 font-medium text-ds-muted"
                      title={t('sessionUsageTitle', { turns: threadUsage.turns })}
                    >
                      {t('sessionUsageTokens', {
                        tokens: formatCompactNumber(threadUsage.totalTokens)
                      })}
                    </span>
                    <span className="inline-flex items-center rounded-full border border-ds-border bg-ds-card/70 px-2.5 py-1 font-medium text-ds-muted">
                      {t('sessionUsageCost', {
                        cost: formatCost(threadUsage.costUsd, i18n.language, threadUsage.costCny, threadUsage.priceConfigured)
                      })}
                    </span>
                    <span
                      className="inline-flex items-center rounded-full border border-ds-border bg-ds-card/70 px-2.5 py-1 font-medium text-ds-muted"
                      title={t(
                        threadUsage.lastTurnCacheHitRate != null
                          ? 'sessionUsageCacheTitleWithLatest'
                          : 'sessionUsageCacheTitle',
                        {
                          cache: formatPercent(threadUsage.cacheHitRate),
                          latestCache: formatPercent(threadUsage.lastTurnCacheHitRate),
                          cached: formatCompactNumber(threadUsage.cachedTokens),
                          miss: formatCompactNumber(threadUsage.cacheMissTokens)
                        }
                      )}
                    >
                      {t('sessionUsageCache', { cache: formatPercent(primaryCacheHitRate(threadUsage)) })}
                    </span>
                  </>
                ) : null}
              </div>
            </div>
          </>
        ) : (
          <div className="min-w-0">
            <div className="text-[12.5px] font-medium uppercase tracking-[0.16em] text-ds-faint">
              {workspaceLabel}
            </div>
            <div className="mt-1 text-[20px] font-semibold tracking-[-0.02em] text-ds-ink">
              {t('noSessionSelected')}
            </div>
            <div className="mt-1 text-[13.5px] text-ds-faint">{t('sessionHeaderHint')}</div>
          </div>
        )}
        {busy ? (
          <span className="ml-auto shrink-0 rounded-full bg-amber-500/18 px-3 py-1.5 text-[12.5px] font-semibold text-amber-950 dark:text-amber-100">
            {t('running')}
          </span>
        ) : null}
      </div>
      {actionsMenuPortal}
      {sourcePreviewPortal}
    </>
  )
}
