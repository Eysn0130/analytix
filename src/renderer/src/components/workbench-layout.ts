import type { SetStateAction, PointerEvent as ReactPointerEvent } from 'react'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { WorkspaceFileTarget } from '@shared/workspace-file'
import type { AppRoute } from '../store/chat-store-types'
import {
  readBrowserStorageItem,
  removeBrowserStorageItem,
  writeBrowserStorageItem
} from '../lib/browser-storage'
import { WORKSPACE_FILE_PREVIEW_EVENT, type WorkspaceFilePreviewDetail } from '../lib/workspace-file-preview'
import type { RightPanelMode } from './chat/WorkbenchTopBar'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../store/workspace-tabs-store'
import i18n from '../i18n'
import { toolResultLocalEffectRequestSchemaV1, toolResultLocalPresentFileRequestSchemaV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'
import { subscribeBeforeSharedActiveCaseContextChange } from '../data-analysis/services/analysis/stats-case-overview-resource'

const LEFT_PANEL_WIDTH_KEY = 'analytix.layout.leftSidebarWidth'
const LEFT_PANEL_COLLAPSED_KEY = 'analytix.layout.leftSidebarCollapsed'
const RIGHT_PANEL_WIDTH_KEY = 'analytix.layout.rightInspectorWidth'
const LEGACY_RIGHT_PANEL_MODE_KEY = 'analytix.layout.rightPanelMode'
const RIGHT_PANEL_OPEN_KEY = 'analytix.layout.rightPanelOpen'
const LAYOUT_STORAGE_VERSION = 'v2'
const DEFAULT_LAYOUT_STORAGE_SCOPE = 'default'
const LEFT_PANEL_DEFAULT = 300
const RIGHT_PANEL_DEFAULT = 360
export const CODE_PANEL_PREFERRED = 560
const LEFT_PANEL_MIN = 240
const LEFT_PANEL_MAX = 520
const RIGHT_PANEL_MIN = 320
const RIGHT_PANEL_MAX = 760
const SIDEBAR_HARD_MIN = 180
const MAIN_MIN_WIDTH = 560
const PANEL_RESIZE_HANDLE_WIDTH = 0
export type WorkbenchLayoutStorageSnapshot = {
  leftSidebarCollapsed: boolean
  leftSidebarWidth: number
  rightPanelOpen: boolean
  rightSidebarWidth: number
  scope: string
}

function clampWidth(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export function workbenchLayoutStorageScope(workspaceRoot: string): string {
  const trimmed = workspaceRoot.trim()
  return trimmed ? encodeURIComponent(trimmed) : DEFAULT_LAYOUT_STORAGE_SCOPE
}

export function workbenchLayoutStorageKey(key: string, scope: string): string {
  return `${key}.${LAYOUT_STORAGE_VERSION}.${scope}`
}

function readScopedStorageItem(key: string, scope: string): string | null {
  const scoped = readBrowserStorageItem(workbenchLayoutStorageKey(key, scope))
  return scoped ?? readBrowserStorageItem(key)
}

function readStoredWidth(key: string, fallback: number, scope: string): number {
  const raw = readScopedStorageItem(key, scope)
  if (!raw) return fallback
  const parsed = Number(raw)
  if (!Number.isFinite(parsed)) return fallback
  return Math.round(parsed)
}

function persistWidth(key: string, width: number, scope: string): void {
  writeBrowserStorageItem(workbenchLayoutStorageKey(key, scope), String(Math.round(width)))
}

function scaledClientX(event: { clientX: number }): number {
  const scale = window.visualViewport?.scale
  return scale && Number.isFinite(scale) && scale > 0 ? event.clientX / scale : event.clientX
}

function createResizeFrameQueue<T>(commit: (value: T) => void): {
  cancel: () => void
  flush: (value?: T) => T | undefined
  schedule: (value: T) => void
} {
  let frameId: number | null = null
  let latestValue: T | undefined
  let committedValue: T | undefined

  const commitLatest = (): T | undefined => {
    if (typeof latestValue === 'undefined') return undefined
    const value = latestValue
    latestValue = undefined
    committedValue = value
    commit(value)
    return value
  }

  return {
    cancel(): void {
      if (frameId !== null) window.cancelAnimationFrame(frameId)
      frameId = null
      latestValue = undefined
    },
    flush(value?: T): T | undefined {
      if (typeof value !== 'undefined') {
        latestValue = value
      }
      if (frameId !== null) {
        window.cancelAnimationFrame(frameId)
        frameId = null
      }
      return commitLatest() ?? committedValue
    },
    schedule(value: T): void {
      latestValue = value
      if (frameId !== null) return
      frameId = window.requestAnimationFrame(() => {
        frameId = null
        commitLatest()
      })
    }
  }
}

function readStoredBoolean(key: string, fallback: boolean, scope: string): boolean {
  const raw = readScopedStorageItem(key, scope)
  if (raw === '1') return true
  if (raw === '0') return false
  return fallback
}

function persistBoolean(key: string, value: boolean, scope: string): void {
  writeBrowserStorageItem(workbenchLayoutStorageKey(key, scope), value ? '1' : '0')
}

function readStoredRightPanelOpen(scope: string): boolean {
  const current = readScopedStorageItem(RIGHT_PANEL_OPEN_KEY, scope)
  if (current === '1' || current === '0') return current === '1'
  const legacy = readScopedStorageItem(LEGACY_RIGHT_PANEL_MODE_KEY, scope)
  return legacy === 'documents' || legacy === 'todo' || legacy === 'changes' || legacy === 'browser'
}

export function readWorkbenchLayoutStorage(workspaceRoot: string): WorkbenchLayoutStorageSnapshot {
  const scope = workbenchLayoutStorageScope(workspaceRoot)
  return {
    leftSidebarCollapsed: readStoredBoolean(LEFT_PANEL_COLLAPSED_KEY, false, scope),
    leftSidebarWidth: readStoredWidth(LEFT_PANEL_WIDTH_KEY, LEFT_PANEL_DEFAULT, scope),
    rightPanelOpen: readStoredRightPanelOpen(scope),
    rightSidebarWidth: readStoredWidth(RIGHT_PANEL_WIDTH_KEY, RIGHT_PANEL_DEFAULT, scope),
    scope
  }
}

/** Dock only when both the chat and the two visible column minima fit. */
export function workbenchRightPanelGeometry(containerWidth: number, leftWidth: number, leftVisible: boolean): {
  overlay: boolean; min: number; max: number
} {
  const width = Number.isFinite(containerWidth) ? Math.max(0, containerWidth) : 0
  const leftMinimum = leftVisible ? LEFT_PANEL_MIN : 0
  const left = leftVisible ? clampWidth(Number.isFinite(leftWidth) ? leftWidth : LEFT_PANEL_DEFAULT, LEFT_PANEL_MIN, LEFT_PANEL_MAX) : 0
  return {
    overlay: width < MAIN_MIN_WIDTH + leftMinimum + RIGHT_PANEL_MIN,
    min: RIGHT_PANEL_MIN,
    max: Math.max(RIGHT_PANEL_MIN, Math.min(RIGHT_PANEL_MAX, width - MAIN_MIN_WIDTH - left))
  }
}

function fitWorkbenchWidths(
  containerWidth: number,
  leftWidth: number,
  rightWidth: number,
  panels: { leftPanelVisible: boolean; rightPanelVisible: boolean }
): { left: number; right: number } {
  const handleWidth =
    (panels.leftPanelVisible ? PANEL_RESIZE_HANDLE_WIDTH : 0) +
    (panels.rightPanelVisible ? PANEL_RESIZE_HANDLE_WIDTH : 0)
  const usableWidth = Math.max(0, containerWidth - handleWidth)

  if (!panels.leftPanelVisible) {
    if (!panels.rightPanelVisible) {
      return {
        left: clampWidth(leftWidth, SIDEBAR_HARD_MIN, LEFT_PANEL_MAX),
        right: clampWidth(rightWidth, RIGHT_PANEL_MIN, RIGHT_PANEL_MAX)
      }
    }
    const safeContainer = Math.max(usableWidth, MAIN_MIN_WIDTH + SIDEBAR_HARD_MIN)
    const rightFloor =
      safeContainer - MAIN_MIN_WIDTH >= RIGHT_PANEL_MIN ? RIGHT_PANEL_MIN : SIDEBAR_HARD_MIN
    const rightCeil = Math.min(
      RIGHT_PANEL_MAX,
      Math.max(rightFloor, safeContainer - MAIN_MIN_WIDTH)
    )
    return {
      left: clampWidth(leftWidth, SIDEBAR_HARD_MIN, LEFT_PANEL_MAX),
      right: clampWidth(rightWidth, rightFloor, rightCeil)
    }
  }

  const safeContainer = Math.max(
    usableWidth,
    MAIN_MIN_WIDTH + SIDEBAR_HARD_MIN + (panels.rightPanelVisible ? SIDEBAR_HARD_MIN : 0)
  )
  if (!panels.rightPanelVisible) {
    const leftFloor =
      safeContainer - MAIN_MIN_WIDTH >= LEFT_PANEL_MIN ? LEFT_PANEL_MIN : SIDEBAR_HARD_MIN
    const leftCeil = Math.min(
      LEFT_PANEL_MAX,
      Math.max(leftFloor, safeContainer - MAIN_MIN_WIDTH)
    )
    return {
      left: clampWidth(leftWidth, leftFloor, leftCeil),
      right: clampWidth(rightWidth, RIGHT_PANEL_MIN, RIGHT_PANEL_MAX)
    }
  }

  const availableSides = Math.max(
    SIDEBAR_HARD_MIN * 2,
    safeContainer - MAIN_MIN_WIDTH
  )
  const leftFloor =
    availableSides - SIDEBAR_HARD_MIN >= LEFT_PANEL_MIN ? LEFT_PANEL_MIN : SIDEBAR_HARD_MIN
  const rightFloor =
    availableSides - SIDEBAR_HARD_MIN >= RIGHT_PANEL_MIN ? RIGHT_PANEL_MIN : SIDEBAR_HARD_MIN

  let nextLeft = clampWidth(leftWidth, leftFloor, LEFT_PANEL_MAX)
  let nextRight = clampWidth(rightWidth, rightFloor, RIGHT_PANEL_MAX)

  if (nextLeft + nextRight > availableSides) {
    const overflow = nextLeft + nextRight - availableSides
    const rightShrink = Math.min(overflow, nextRight - rightFloor)
    nextRight -= rightShrink
    const remaining = overflow - rightShrink
    if (remaining > 0) {
      nextLeft = Math.max(leftFloor, nextLeft - remaining)
    }
  }

  const maxLeft = Math.min(LEFT_PANEL_MAX, availableSides - rightFloor)
  nextLeft = clampWidth(nextLeft, leftFloor, Math.max(leftFloor, maxLeft))
  const maxRight = Math.min(RIGHT_PANEL_MAX, availableSides - nextLeft)
  nextRight = clampWidth(nextRight, rightFloor, Math.max(rightFloor, maxRight))

  return { left: nextLeft, right: nextRight }
}

export function useWorkbenchLayout({
  activeThreadId,
  latestAutoOpenDevPreviewUrl,
  latestDevPreviewUrl,
  route,
  workspaceRoot,
  writeAssistantOpen: _writeAssistantOpen
}: {
  activeThreadId: string | null
  latestAutoOpenDevPreviewUrl: string | null
  latestDevPreviewUrl: string | null
  route: AppRoute
  workspaceRoot: string
  writeAssistantOpen: boolean
}) {
  const initialLayoutRef = useRef<WorkbenchLayoutStorageSnapshot | null>(null)
  if (initialLayoutRef.current === null) {
    initialLayoutRef.current = readWorkbenchLayoutStorage(workspaceRoot)
  }
  const initialLayout = initialLayoutRef.current
  const [layoutStorageScope, setLayoutStorageScope] = useState(initialLayout.scope)
  const restoredScope = useRef<string | null>(null)
  const tabs = useWorkspaceTabsStore((state) => state.tabs)
  const activeTabId = useWorkspaceTabsStore((state) => state.activeTabId)
  const dockOpen = useWorkspaceTabsStore((state) => state.open)
  const selectorOpen = useWorkspaceTabsStore((state) => state.selectorOpen)
  const focused = useWorkspaceTabsStore((state) => state.focused)
  const activeTab = tabs.find((tab) => tab.id === activeTabId)
  const rightPanelMode: RightPanelMode = dockOpen && !selectorOpen ? activeTab?.mode ?? null : null
  const [filePreviewTarget, updateFilePreviewTarget] = useState<WorkspaceFileTarget | null>(null)
  const filePreviewTargetRef = useRef<WorkspaceFileTarget | null>(null)
  const setFilePreviewTarget = useCallback((target: WorkspaceFileTarget | null): void => {
    filePreviewTargetRef.current = target
    updateFilePreviewTarget(target)
  }, [])
  const setRightPanelMode = useCallback((value: SetStateAction<RightPanelMode>): void => {
    const store = useWorkspaceTabsStore.getState()
    const currentMode = store.open && !store.selectorOpen ? store.tabs.find((tab) => tab.id === store.activeTabId)?.mode ?? null : null
    const mode = typeof value === 'function' ? value(currentMode) : value
    if (!mode) { store.setOpen(false); return }
    // A task pane can only open through its explicit task-ID navigation owner.
    if (mode === 'schedule-task') {
      const current = store.tabs.find(tab => tab.id === store.activeTabId && tab.mode === mode && tab.scheduleTaskId)
      if (current) store.activateTab(current.id)
      return
    }
    if (mode === 'documents') {
      const current = store.tabs.find(tab => tab.id === store.activeTabId && tab.mode === 'documents' && tab.path)
      if (current) store.activateTab(current.id)
      else store.showSelector()
      return
    }
    const target = filePreviewTargetRef.current
    if (mode === 'file' && target) {
      if (target.protectedPresent !== undefined) {
        const reference = toolResultLocalPresentFileRequestSchemaV1.safeParse(target.protectedPresent)
        if (!reference.success || target.protectedRead !== undefined) return
        store.openTab({ id: 'protected-present:' + reference.data.viewId + ':' + reference.data.index, kind: 'file', mode, preview: 'source',
          title: target.path.replaceAll('\\', '/').split('/').at(-1) || i18n.t('common:filePreviewTitle'), protectedPresent: reference.data })
        return
      }
      if (target.preview === 'source' || target.protectedRead !== undefined) {
        const reference = toolResultLocalEffectRequestSchemaV1.safeParse(target.protectedRead)
        if (!reference.success) return
        store.openTab({ id: 'protected-read:' + reference.data.viewId, kind: 'file', mode, preview: 'source',
          title: target.path.replaceAll('\\', '/').split('/').at(-1) || i18n.t('common:filePreviewTitle'),
          protectedRead: reference.data, sourceLine: target.line })
        return
      }
      store.openTab({ id: workspaceObjectTabId(target.workspaceRoot ?? '', target.path), kind: 'file', mode,
        title: target.path.replaceAll('\\', '/').split('/').at(-1) || target.path,
        path: target.path, workspaceRoot: target.workspaceRoot })
      return
    }
    const labels: Record<Exclude<RightPanelMode, null | 'schedule-task'>, string> = {
      documents: 'workbenchDocuments', files: 'rightPanelFiles', todo: 'rightPanelTodo', changes: 'rightPanelChanges',
      terminal: 'rightPanelTerminal', browser: 'rightPanelBrowser', file: 'rightPanelFiles', plan: 'rightPanelPlan',
      'sdd-ai': 'sddAssistantTitle', 'child-agent': 'subagentInspectorTitle'
    }
    store.openTab({ id: `tool:${mode}`, kind: 'tool', mode, title: i18n.t(`common:${labels[mode]}`) })
  }, [])
  const [leftSidebarWidth, setLeftSidebarWidth] = useState(initialLayout.leftSidebarWidth)
  const [leftSidebarCollapsed, setLeftSidebarCollapsed] = useState(
    initialLayout.leftSidebarCollapsed
  )
  const [rightSidebarWidth, setRightSidebarWidth] = useState(initialLayout.rightSidebarWidth)
  const [composerElement, setComposerElement] = useState<HTMLDivElement | null>(null)
  const [composerHeight, setComposerHeight] = useState(0)
  const sidebarResizeStopRef = useRef<((commitFinal?: boolean) => void) | null>(null)
  const resizeMounted = useRef(true)
  const [leftResizing, setLeftResizing] = useState(false)
  const [rightResizing, setRightResizing] = useState(false)
  const shellRef = useRef<HTMLDivElement | null>(null)
  const [shellWidth, setShellWidth] = useState(0)
  const windowChromeRef = useRef<HTMLElement | null>(null)
  const rightTabsPaneRef = useRef<HTMLDivElement | null>(null)
  const rightTabsContentRef = useRef<HTMLDivElement | null>(null)
  const leftPaneRef = useRef<HTMLDivElement | null>(null)
  const leftPaneContentRef = useRef<HTMLDivElement | null>(null)
  const rightPaneRef = useRef<HTMLElement | null>(null)
  const rightPaneContentRef = useRef<HTMLDivElement | null>(null)
  const previewThreadId = useRef<string | null>(activeThreadId)
  const autoOpenedPreviewUrlRef = useRef<string | null>(null)
  const rightPanelVisible = dockOpen
  const rightGeometry = workbenchRightPanelGeometry(shellWidth, leftSidebarWidth, !leftSidebarCollapsed)
  const rightPanelAutoOverlay = rightPanelVisible && shellWidth > 0 && rightGeometry.overlay
  const leftBounds = {
    min: fitWorkbenchWidths(shellWidth || window.innerWidth, 0, rightSidebarWidth, { leftPanelVisible: true, rightPanelVisible }).left,
    max: fitWorkbenchWidths(shellWidth || window.innerWidth, LEFT_PANEL_MAX, rightSidebarWidth, { leftPanelVisible: true, rightPanelVisible }).left
  }

  useLayoutEffect(() => {
    const nextLayout = readWorkbenchLayoutStorage(workspaceRoot)
    if (nextLayout.scope === restoredScope.current) return
    const initial = restoredScope.current === null
    restoredScope.current = nextLayout.scope
    const store = useWorkspaceTabsStore.getState()
    // Restore empty structure only. Live object identities and explicit open
    // intents outrank storage; never persist or reacquire a private selector.
    if (store.tabs.length === 0 && (!initial || !store.open)) {
      if (nextLayout.rightPanelOpen) store.showSelector()
      else store.setOpen(false)
    }
    setLayoutStorageScope(nextLayout.scope)
    setLeftSidebarWidth(nextLayout.leftSidebarWidth)
    setLeftSidebarCollapsed(nextLayout.leftSidebarCollapsed)
    setRightSidebarWidth(nextLayout.rightSidebarWidth)
    previewThreadId.current = activeThreadId
    autoOpenedPreviewUrlRef.current = null
  }, [activeThreadId, layoutStorageScope, workspaceRoot])

  useEffect(() => {
    if (leftResizing || restoredScope.current !== layoutStorageScope) return
    persistWidth(LEFT_PANEL_WIDTH_KEY, leftSidebarWidth, layoutStorageScope)
  }, [layoutStorageScope, leftResizing, leftSidebarWidth])

  useEffect(() => {
    if (restoredScope.current !== layoutStorageScope) return
    persistBoolean(LEFT_PANEL_COLLAPSED_KEY, leftSidebarCollapsed, layoutStorageScope)
  }, [layoutStorageScope, leftSidebarCollapsed])

  useEffect(() => {
    if (rightResizing || restoredScope.current !== layoutStorageScope) return
    persistWidth(RIGHT_PANEL_WIDTH_KEY, rightSidebarWidth, layoutStorageScope)
  }, [layoutStorageScope, rightResizing, rightSidebarWidth])

  useEffect(() => {
    if (restoredScope.current !== layoutStorageScope || useWorkspaceTabsStore.getState().open !== rightPanelVisible) return
    persistBoolean(RIGHT_PANEL_OPEN_KEY, rightPanelVisible, layoutStorageScope)
    if (readBrowserStorageItem(workbenchLayoutStorageKey(RIGHT_PANEL_OPEN_KEY, layoutStorageScope)) === (rightPanelVisible ? '1' : '0')) {
      removeBrowserStorageItem(workbenchLayoutStorageKey(LEGACY_RIGHT_PANEL_MODE_KEY, layoutStorageScope))
    }
  }, [layoutStorageScope, rightPanelVisible])

  useLayoutEffect(() => {
    const measure = (): void => setComposerHeight(composerElement?.offsetHeight ?? 0)
    measure()
    if (!composerElement) return
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(composerElement)
    window.addEventListener('resize', measure)
    return () => { observer?.disconnect(); window.removeEventListener('resize', measure) }
  }, [composerElement])

  useLayoutEffect(() => {
    resizeMounted.current = true
    return () => {
      resizeMounted.current = false
      sidebarResizeStopRef.current?.(false)
    }
  }, [])

  useLayoutEffect(() => {
    const retire = (viewId?: string | null) => {
      const store = useWorkspaceTabsStore.getState()
      for (const tab of store.tabs) {
        if ((tab.id.startsWith('protected-read:') || tab.preview === 'source' || tab.protectedRead !== undefined || tab.protectedPresent !== undefined) && (!viewId || (tab.protectedPresent ?? tab.protectedRead)?.viewId === viewId)) store.closeTab(tab.id)
      }
      const target = filePreviewTargetRef.current
      if (target && (target.preview === 'source' || target.protectedRead !== undefined || target.protectedPresent !== undefined) && (!viewId || (target.protectedPresent ?? target.protectedRead)?.viewId === viewId)) setFilePreviewTarget(null)
    }
    // Retire before a scope replacement, hidden document, runtime loss, or the
    // existing Host document lease closes. Ordinary workspace tabs survive.
    const api = window.analytix?.runtime
    const invalidated = api?.onToolResultLocalDisplayInvalidated?.(retire)
    const runtime = api?.onRuntimeStatus?.(status => { if (status.state !== 'running') retire() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(() => retire())
    const hidden = () => { if (document.hidden) retire() }
    document.addEventListener('visibilitychange', hidden)
    return () => { retire(); invalidated?.(); runtime?.(); caseOff(); document.removeEventListener('visibilitychange', hidden) }
  }, [activeThreadId, workspaceRoot, setFilePreviewTarget])

  useEffect(() => {
    const onPreview = (event: Event): void => {
      const detail = (event as CustomEvent<WorkspaceFilePreviewDetail>).detail
      if (!detail?.path) return
      if (detail.protectedPresent !== undefined) {
        const reference = toolResultLocalPresentFileRequestSchemaV1.safeParse(detail.protectedPresent)
        if (!reference.success || reference.data.threadId !== activeThreadId || detail.protectedRead !== undefined) return
      } else if (detail.preview === 'source' || detail.protectedRead !== undefined) {
        const reference = toolResultLocalEffectRequestSchemaV1.safeParse(detail.protectedRead)
        if (!reference.success || reference.data.threadId !== activeThreadId) return
      }
      setFilePreviewTarget({
        ...detail,
        workspaceRoot: detail.workspaceRoot ?? workspaceRoot
      })
      setRightSidebarWidth((width) => Math.max(width, CODE_PANEL_PREFERRED))
      setRightPanelMode('file')
    }

    window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, onPreview)
    return () => window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, onPreview)
  }, [activeThreadId, setFilePreviewTarget, setRightPanelMode, workspaceRoot])

  useEffect(() => {
    if (previewThreadId.current === activeThreadId) return
    previewThreadId.current = activeThreadId
    autoOpenedPreviewUrlRef.current = null
    // Tabs represent workspace objects, so switching a conversation does not close them.
  }, [activeThreadId, rightPanelMode])

  useEffect(() => {
    if (!latestAutoOpenDevPreviewUrl || route !== 'chat') return
    if (autoOpenedPreviewUrlRef.current === latestAutoOpenDevPreviewUrl) return
    autoOpenedPreviewUrlRef.current = latestAutoOpenDevPreviewUrl
    // A background preview is offered by the launch card without stealing focus.
  }, [latestAutoOpenDevPreviewUrl, route])


  useLayoutEffect(() => {
    const sync = (): void => {
      const containerWidth = shellRef.current?.clientWidth || window.innerWidth
      setShellWidth(containerWidth)
      if (rightPanelVisible && workbenchRightPanelGeometry(containerWidth, leftSidebarWidth, !leftSidebarCollapsed).overlay) {
        sidebarResizeStopRef.current?.(false)
        return
      }
      const next = fitWorkbenchWidths(
        containerWidth,
        leftSidebarWidth,
        rightSidebarWidth,
        {
          leftPanelVisible: !leftSidebarCollapsed,
          rightPanelVisible
        }
      )
      if (next.left !== leftSidebarWidth) setLeftSidebarWidth(next.left)
      if (next.right !== rightSidebarWidth) setRightSidebarWidth(next.right)
    }
    sync()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(sync)
    if (shellRef.current) observer?.observe(shellRef.current)
    window.addEventListener('resize', sync)
    return () => { observer?.disconnect(); window.removeEventListener('resize', sync) }
  }, [leftSidebarCollapsed, leftSidebarWidth, rightPanelVisible, rightSidebarWidth])

  const toggleRightPanelMode = (nextMode: Exclude<RightPanelMode, null>): void => {
    setRightPanelMode(nextMode)
  }

  const toggleLeftSidebar = (): void => {
    setLeftSidebarCollapsed((current) => !current)
  }

  const openDevPreview = (): void => {
    if (latestDevPreviewUrl) {
      autoOpenedPreviewUrlRef.current = latestDevPreviewUrl
    }
    setRightPanelMode('browser')
  }

  const resizeSidebarBy = (side: 'left' | 'right', delta: number): void => {
    if (!Number.isFinite(delta) || rightPanelAutoOverlay || focused) return
    const containerWidth = shellRef.current?.clientWidth ?? window.innerWidth
    const next = fitWorkbenchWidths(containerWidth,
      leftSidebarWidth + (side === 'left' ? delta : 0),
      rightSidebarWidth + (side === 'right' ? delta : 0), {
        leftPanelVisible: !leftSidebarCollapsed, rightPanelVisible
      })
    setLeftSidebarWidth(next.left)
    setRightSidebarWidth(next.right)
  }
  const resetLeftSidebarWidth = (): void => {
    const containerWidth = shellRef.current?.clientWidth ?? window.innerWidth
    const next = fitWorkbenchWidths(containerWidth, LEFT_PANEL_DEFAULT, rightSidebarWidth, {
      leftPanelVisible: !leftSidebarCollapsed,
      rightPanelVisible
    })
    setLeftSidebarWidth(next.left)
    if (next.right !== rightSidebarWidth) setRightSidebarWidth(next.right)
  }

  const resetRightSidebarWidth = (): void => {
    const containerWidth = shellRef.current?.clientWidth ?? window.innerWidth
    const next = fitWorkbenchWidths(containerWidth, leftSidebarWidth, RIGHT_PANEL_DEFAULT, {
      leftPanelVisible: !leftSidebarCollapsed,
      rightPanelVisible
    })
    if (next.left !== leftSidebarWidth) setLeftSidebarWidth(next.left)
    setRightSidebarWidth(next.right)
  }

  const applyLeftSidebarDomWidth = (width: number): void => {
    const px = `${Math.round(width)}px`
    windowChromeRef.current?.style.setProperty('--ds-window-chrome-sidebar-width', px)
    if (leftPaneRef.current) {
      leftPaneRef.current.style.width = px
    }
    if (leftPaneContentRef.current) {
      leftPaneContentRef.current.style.minWidth = px
      leftPaneContentRef.current.style.width = px
    }
  }

  const applyRightSidebarDomWidth = (width: number): void => {
    const px = `${Math.round(width)}px`
    if (rightTabsPaneRef.current) rightTabsPaneRef.current.style.width = px
    if (rightTabsContentRef.current) {
      rightTabsContentRef.current.style.minWidth = px
      rightTabsContentRef.current.style.width = px
    }
    if (rightPaneRef.current) {
      rightPaneRef.current.style.width = px
    }
    if (rightPaneContentRef.current) {
      rightPaneContentRef.current.style.minWidth = px
      rightPaneContentRef.current.style.width = px
    }
  }

  const beginSidebarResize = (side: 'left' | 'right', event: ReactPointerEvent<HTMLDivElement>): void => {
    if (event.button !== 0 || rightPanelAutoOverlay || focused || (side === 'left' ? leftSidebarCollapsed : !rightPanelVisible)) return
    sidebarResizeStopRef.current?.()
    event.preventDefault()
    const target = event.currentTarget, pointerId = event.pointerId
    const startX = scaledClientX(event), startLeft = leftSidebarWidth, startRight = rightSidebarWidth
    const prevCursor = document.body.style.cursor, prevUserSelect = document.body.style.userSelect
    const styles: Array<{ node: HTMLElement; property: string; previous: string; side: 'left' | 'right' }> = []
    const remember = (side: 'left' | 'right', node: HTMLElement | null, properties: string[]): void => {
      if (node) for (const property of properties) styles.push({ node, property, side, previous: node.style.getPropertyValue(property) })
    }
    if (!leftSidebarCollapsed) {
      remember('left', windowChromeRef.current, ['--ds-window-chrome-sidebar-width'])
      remember('left', leftPaneRef.current, ['width'])
      remember('left', leftPaneContentRef.current, ['width', 'min-width'])
    }
    if (rightPanelVisible) {
      remember('right', rightTabsPaneRef.current, ['width'])
      remember('right', rightTabsContentRef.current, ['width', 'min-width'])
      remember('right', rightPaneRef.current, ['width'])
      remember('right', rightPaneContentRef.current, ['width', 'min-width'])
    }
    let lastApplied: { left: number; right: number } | undefined
    const resizeQueue = createResizeFrameQueue((next: { left: number; right: number }) => {
      lastApplied = next
      if (!leftSidebarCollapsed) applyLeftSidebarDomWidth(next.left)
      if (rightPanelVisible) applyRightSidebarDomWidth(next.right)
    })
    let stopped = false
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    if (side === 'left') setLeftResizing(true)
    else setRightResizing(true)

    const computeNext = (move: PointerEvent): { left: number; right: number } => {
      const delta = scaledClientX(move) - startX
      return fitWorkbenchWidths(shellRef.current?.clientWidth || window.innerWidth,
        startLeft + (side === 'left' ? delta : 0), startRight - (side === 'right' ? delta : 0), {
          leftPanelVisible: !leftSidebarCollapsed, rightPanelVisible
        })
    }
    const stop = (up?: PointerEvent, commitFinal = true): void => {
      if (stopped) return
      stopped = true
      sidebarResizeStopRef.current = null
      let finalSize: { left: number; right: number } | undefined
      if (commitFinal) finalSize = up ? resizeQueue.flush(computeNext(up)) : resizeQueue.flush()
      else {
        resizeQueue.cancel()
        // Undo only this drag's uncommitted paint. New overlay/scope styles
        // already installed by React belong to the next layout and stay intact.
        if (lastApplied) for (const { node, property, previous, side } of styles) {
          if (node.style.getPropertyValue(property) === `${Math.round(lastApplied[side])}px`) node.style.setProperty(property, previous)
        }
      }
      if (resizeMounted.current) {
        if (finalSize) { setLeftSidebarWidth(finalSize.left); setRightSidebarWidth(finalSize.right) }
        setLeftResizing(false); setRightResizing(false)
      }
      document.body.style.cursor = prevCursor
      document.body.style.userSelect = prevUserSelect
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onCancel)
      window.removeEventListener('blur', onBlur)
      document.removeEventListener('visibilitychange', onHidden)
      target.removeEventListener('lostpointercapture', onCancel)
      try { if (target.hasPointerCapture?.(pointerId)) target.releasePointerCapture(pointerId) } catch { /* Capture already retired. */ }
    }
    const onMove = (move: PointerEvent): void => {
      if (stopped || move.pointerId !== pointerId) return
      move.preventDefault(); resizeQueue.schedule(computeNext(move))
    }
    const onUp = (up: PointerEvent): void => {
      if (up.pointerId !== pointerId) return
      up.preventDefault(); stop(up)
    }
    const onCancel = (cancel: PointerEvent): void => { if (cancel.pointerId === pointerId) stop() }
    const onBlur = (): void => stop()
    const onHidden = (): void => { if (document.hidden) stop() }
    sidebarResizeStopRef.current = (commitFinal = true) => stop(undefined, commitFinal)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
    window.addEventListener('blur', onBlur)
    document.addEventListener('visibilitychange', onHidden)
    target.addEventListener('lostpointercapture', onCancel)
    try { target.setPointerCapture?.(pointerId) } catch { stop() }
  }

  useLayoutEffect(() => () => {
    sidebarResizeStopRef.current?.(false)
  }, [workspaceRoot])
  useLayoutEffect(() => {
    if (rightPanelAutoOverlay || focused || leftSidebarCollapsed || !rightPanelVisible) sidebarResizeStopRef.current?.(false)
  }, [focused, leftSidebarCollapsed, rightPanelAutoOverlay, rightPanelVisible])

  return {
    beginLeftResize: (event: ReactPointerEvent<HTMLDivElement>) => beginSidebarResize('left', event),
    beginRightResize: (event: ReactPointerEvent<HTMLDivElement>) => beginSidebarResize('right', event),
    filePreviewTarget,
    leftPaneContentRef,
    leftPaneRef,
    leftSidebarCollapsed,
    leftResizing,
    leftSidebarWidth,
    leftResizeMin: leftBounds.min,
    leftResizeMax: leftBounds.max,
    openDevPreview,
    rightPanelMode,
    rightPanelVisible,
    rightPanelAutoOverlay,
    rightResizeMin: rightGeometry.min,
    rightResizeMax: rightGeometry.max,
    rightPaneContentRef,
    rightPaneRef,
    rightResizing,
    rightSidebarWidth,
    resizeLeftSidebarBy: (delta: number) => resizeSidebarBy('left', delta),
    resizeRightSidebarBy: (delta: number) => resizeSidebarBy('right', delta),
    resetLeftSidebarWidth,
    resetRightSidebarWidth,
    setFilePreviewTarget,
    setRightPanelMode,
    setRightSidebarWidth,
    shellRef,
    windowChromeRef,
    rightTabsPaneRef,
    rightTabsContentRef,
    composerHeight,
    composerRef: setComposerElement,
    toggleLeftSidebar,
    toggleRightPanelMode
  }
}
