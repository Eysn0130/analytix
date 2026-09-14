import type { CSSProperties, SetStateAction, PointerEvent as ReactPointerEvent } from 'react'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { WorkspaceFileTarget } from '@shared/workspace-file'
import type { AppRoute } from '../store/chat-store-types'
import {
  readBrowserStorageItem,
  writeBrowserStorageItem
} from '../lib/browser-storage'
import { WORKSPACE_FILE_PREVIEW_EVENT, type WorkspaceFilePreviewDetail } from '../lib/workspace-file-preview'
import type { RightPanelMode } from './chat/WorkbenchTopBar'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../store/workspace-tabs-store'
import i18n from '../i18n'

const LEFT_PANEL_WIDTH_KEY = 'analytix.layout.leftSidebarWidth'
const LEFT_PANEL_COLLAPSED_KEY = 'analytix.layout.leftSidebarCollapsed'
const RIGHT_PANEL_WIDTH_KEY = 'analytix.layout.rightInspectorWidth'
const RIGHT_PANEL_MODE_KEY = 'analytix.layout.rightPanelMode'
const TERMINAL_OPEN_KEY = 'analytix.layout.terminalOpen'
const TERMINAL_HEIGHT_KEY = 'analytix.layout.terminalHeight'
const LAYOUT_STORAGE_VERSION = 'v2'
const DEFAULT_LAYOUT_STORAGE_SCOPE = 'default'
const LEFT_PANEL_DEFAULT = 300
const RIGHT_PANEL_DEFAULT = 360
export const CODE_PANEL_PREFERRED = 560
const LEFT_PANEL_MIN = 240
const LEFT_PANEL_MAX = 520
const RIGHT_PANEL_MIN = 280
const RIGHT_PANEL_MAX = 760
const SIDEBAR_HARD_MIN = 180
const MAIN_MIN_WIDTH = 560
const PANEL_RESIZE_HANDLE_WIDTH = 0
// Bottom terminal drawer sizing. The drawer lives below the chat stage and
// resizes vertically, so it has its own clamps instead of the column widths.
const TERMINAL_HEIGHT_DEFAULT = 360
const TERMINAL_HEIGHT_MIN = 220
const TERMINAL_HEIGHT_MAX = 760
const TERMINAL_RESIZE_HANDLE_HEIGHT = 4
export const WORKBENCH_TIMELINE_MIN_HEIGHT = 260

type WorkbenchScrollReserveStyle = CSSProperties & {
  '--analytix-bottom-panel-reserve': string
  '--analytix-bottom-panel-height': string
}

export type WorkbenchScrollReserve = {
  bottomPanelOpen: boolean
  bottomPanelHeight: number
  bottomPanelReservePx: number
  timelineReserveStyle: WorkbenchScrollReserveStyle
}

export type WorkbenchLayoutStorageSnapshot = {
  leftSidebarCollapsed: boolean
  leftSidebarWidth: number
  rightPanelMode: RightPanelMode
  rightSidebarWidth: number
  scope: string
  terminalHeight: number
  terminalOpen: boolean
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

function scaledClientY(event: { clientY: number }): number {
  const scale = window.visualViewport?.scale
  return scale && Number.isFinite(scale) && scale > 0 ? event.clientY / scale : event.clientY
}

function createResizeFrameQueue<T>(commit: (value: T) => void): {
  flush: (value?: T) => T | undefined
  schedule: (value: T) => void
} {
  let frameId: number | null = null
  let latestValue: T | undefined

  const commitLatest = (): T | undefined => {
    if (typeof latestValue === 'undefined') return undefined
    const value = latestValue
    latestValue = undefined
    commit(value)
    return value
  }

  return {
    flush(value?: T): T | undefined {
      if (typeof value !== 'undefined') {
        latestValue = value
      }
      if (frameId !== null) {
        window.cancelAnimationFrame(frameId)
        frameId = null
      }
      return commitLatest()
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

export function maxWorkbenchTerminalHeight(containerHeight: number): number {
  const safeContainerHeight = Number.isFinite(containerHeight)
    ? containerHeight
    : WORKBENCH_TIMELINE_MIN_HEIGHT + TERMINAL_HEIGHT_DEFAULT
  return Math.max(
    TERMINAL_HEIGHT_MIN,
    Math.min(TERMINAL_HEIGHT_MAX, safeContainerHeight - WORKBENCH_TIMELINE_MIN_HEIGHT)
  )
}

export function clampWorkbenchTerminalHeight(value: number, containerHeight: number): number {
  return clampWidth(value, TERMINAL_HEIGHT_MIN, maxWorkbenchTerminalHeight(containerHeight))
}

export function createWorkbenchScrollReserve({
  bottomPanelOpen,
  bottomPanelHeight
}: {
  bottomPanelOpen: boolean
  bottomPanelHeight: number
}): WorkbenchScrollReserve {
  const visibleHeight = bottomPanelOpen ? Math.max(0, Math.round(bottomPanelHeight)) : 0
  const reservePx = bottomPanelOpen ? visibleHeight + TERMINAL_RESIZE_HANDLE_HEIGHT : 0
  return {
    bottomPanelOpen,
    bottomPanelHeight: visibleHeight,
    bottomPanelReservePx: reservePx,
    timelineReserveStyle: {
      '--analytix-bottom-panel-height': `${visibleHeight}px`,
      '--analytix-bottom-panel-reserve': `${reservePx}px`
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

function readStoredRightPanelMode(scope: string): RightPanelMode {
  const raw = readScopedStorageItem(RIGHT_PANEL_MODE_KEY, scope)
  return raw === 'documents' || raw === 'todo' || raw === 'changes' || raw === 'browser' ? raw : null
}

function persistRightPanelMode(mode: RightPanelMode, scope: string): void {
  const key = workbenchLayoutStorageKey(RIGHT_PANEL_MODE_KEY, scope)
  if (mode === 'documents' || mode === 'todo' || mode === 'changes' || mode === 'browser') {
    writeBrowserStorageItem(key, mode)
  } else {
    writeBrowserStorageItem(key, 'none')
  }
}

export function readWorkbenchLayoutStorage(workspaceRoot: string): WorkbenchLayoutStorageSnapshot {
  const scope = workbenchLayoutStorageScope(workspaceRoot)
  return {
    leftSidebarCollapsed: readStoredBoolean(LEFT_PANEL_COLLAPSED_KEY, false, scope),
    leftSidebarWidth: readStoredWidth(LEFT_PANEL_WIDTH_KEY, LEFT_PANEL_DEFAULT, scope),
    rightPanelMode: readStoredRightPanelMode(scope),
    rightSidebarWidth: readStoredWidth(RIGHT_PANEL_WIDTH_KEY, RIGHT_PANEL_DEFAULT, scope),
    scope,
    terminalHeight: readStoredWidth(TERMINAL_HEIGHT_KEY, TERMINAL_HEIGHT_DEFAULT, scope),
    terminalOpen: readStoredBoolean(TERMINAL_OPEN_KEY, false, scope)
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
  const tabs = useWorkspaceTabsStore((state) => state.tabs)
  const activeTabId = useWorkspaceTabsStore((state) => state.activeTabId)
  const dockOpen = useWorkspaceTabsStore((state) => state.open)
  const selectorOpen = useWorkspaceTabsStore((state) => state.selectorOpen)
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
    if (mode === 'documents') {
      const current = store.tabs.find((tab) => tab.id === store.activeTabId && tab.mode === 'documents')
      if (current) { store.activateTab(current.id); return }
    }
    const target = filePreviewTargetRef.current
    if (mode === 'file' && target) {
      store.openTab({ id: workspaceObjectTabId(target.workspaceRoot ?? '', target.path), kind: 'file', mode,
        title: target.path.replaceAll('\\', '/').split('/').at(-1) || target.path,
        path: target.path, workspaceRoot: target.workspaceRoot })
      return
    }
    const labels: Record<Exclude<RightPanelMode, null>, string> = {
      documents: 'workbenchDocuments', files: 'rightPanelFiles', todo: 'rightPanelTodo', changes: 'rightPanelChanges',
      browser: 'rightPanelBrowser', file: 'rightPanelFiles', plan: 'rightPanelPlan', summary: 'rightPanelSummary',
      'sdd-ai': 'sddAssistantTitle', 'child-agent': 'subagentInspectorTitle'
    }
    store.openTab({ id: `tool:${mode}`, kind: 'tool', mode, title: i18n.t(`common:${labels[mode]}`) })
  }, [])
  const [leftSidebarWidth, setLeftSidebarWidth] = useState(initialLayout.leftSidebarWidth)
  const [leftSidebarCollapsed, setLeftSidebarCollapsed] = useState(
    initialLayout.leftSidebarCollapsed
  )
  const [rightSidebarWidth, setRightSidebarWidth] = useState(initialLayout.rightSidebarWidth)
  const [terminalOpen, setTerminalOpen] = useState(initialLayout.terminalOpen)
  const [terminalHeight, setTerminalHeight] = useState(initialLayout.terminalHeight)
  const [leftResizing, setLeftResizing] = useState(false)
  const [rightResizing, setRightResizing] = useState(false)
  const [terminalResizing, setTerminalResizing] = useState(false)
  const shellRef = useRef<HTMLDivElement | null>(null)
  const leftPaneRef = useRef<HTMLDivElement | null>(null)
  const leftPaneContentRef = useRef<HTMLDivElement | null>(null)
  const rightPaneRef = useRef<HTMLElement | null>(null)
  const rightPaneContentRef = useRef<HTMLDivElement | null>(null)
  const previewThreadId = useRef<string | null>(activeThreadId)
  const autoOpenedPreviewUrlRef = useRef<string | null>(null)
  const rightPanelVisible = dockOpen

  useEffect(() => {
    const nextLayout = readWorkbenchLayoutStorage(workspaceRoot)
    if (nextLayout.scope === layoutStorageScope) return
    setLayoutStorageScope(nextLayout.scope)
    setLeftSidebarWidth(nextLayout.leftSidebarWidth)
    setLeftSidebarCollapsed(nextLayout.leftSidebarCollapsed)
    setRightSidebarWidth(nextLayout.rightSidebarWidth)
    setTerminalOpen(nextLayout.terminalOpen)
    setTerminalHeight(nextLayout.terminalHeight)
    previewThreadId.current = activeThreadId
    autoOpenedPreviewUrlRef.current = null
  }, [activeThreadId, layoutStorageScope, workspaceRoot])

  useEffect(() => {
    if (leftResizing) return
    persistWidth(LEFT_PANEL_WIDTH_KEY, leftSidebarWidth, layoutStorageScope)
  }, [layoutStorageScope, leftResizing, leftSidebarWidth])

  useEffect(() => {
    persistBoolean(LEFT_PANEL_COLLAPSED_KEY, leftSidebarCollapsed, layoutStorageScope)
  }, [layoutStorageScope, leftSidebarCollapsed])

  useEffect(() => {
    if (rightResizing) return
    persistWidth(RIGHT_PANEL_WIDTH_KEY, rightSidebarWidth, layoutStorageScope)
  }, [layoutStorageScope, rightResizing, rightSidebarWidth])

  useEffect(() => {
    persistRightPanelMode(rightPanelMode, layoutStorageScope)
  }, [layoutStorageScope, rightPanelMode])

  useEffect(() => {
    persistBoolean(TERMINAL_OPEN_KEY, terminalOpen, layoutStorageScope)
  }, [layoutStorageScope, terminalOpen])

  useEffect(() => {
    if (terminalResizing) return
    persistWidth(TERMINAL_HEIGHT_KEY, terminalHeight, layoutStorageScope)
  }, [layoutStorageScope, terminalHeight, terminalResizing])

  useEffect(() => {
    const onPreview = (event: Event): void => {
      const detail = (event as CustomEvent<WorkspaceFilePreviewDetail>).detail
      if (!detail?.path) return
      setFilePreviewTarget({
        ...detail,
        workspaceRoot: detail.workspaceRoot ?? workspaceRoot
      })
      setRightSidebarWidth((width) => Math.max(width, CODE_PANEL_PREFERRED))
      setRightPanelMode('file')
    }

    window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, onPreview)
    return () => window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, onPreview)
  }, [setFilePreviewTarget, setRightPanelMode, workspaceRoot])

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
      const containerWidth = shellRef.current?.clientWidth ?? window.innerWidth
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
    window.addEventListener('resize', sync)
    return () => window.removeEventListener('resize', sync)
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

  const resetTerminalHeight = (): void => {
    const containerHeight = shellRef.current?.clientHeight ?? window.innerHeight
    setTerminalHeight(clampWorkbenchTerminalHeight(TERMINAL_HEIGHT_DEFAULT, containerHeight))
  }

  const applyLeftSidebarDomWidth = (width: number): void => {
    const px = `${Math.round(width)}px`
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
    if (rightPaneRef.current) {
      rightPaneRef.current.style.width = px
    }
    if (rightPaneContentRef.current) {
      rightPaneContentRef.current.style.minWidth = px
      rightPaneContentRef.current.style.width = px
    }
  }

  const beginLeftResize = (event: ReactPointerEvent<HTMLDivElement>): void => {
    if (leftSidebarCollapsed || event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture?.(event.pointerId)
    const startX = scaledClientX(event)
    const startLeft = leftSidebarWidth
    const startRight = rightSidebarWidth
    const prevCursor = document.body.style.cursor
    const prevUserSelect = document.body.style.userSelect
    const commit = (next: { left: number; right: number }): void => {
      applyLeftSidebarDomWidth(next.left)
      if (rightPanelVisible) applyRightSidebarDomWidth(next.right)
    }
    const resizeQueue = createResizeFrameQueue(commit)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    setLeftResizing(true)

    const computeNext = (moveEvent: PointerEvent): { left: number; right: number } => {
      const containerWidth = shellRef.current?.clientWidth ?? window.innerWidth
      const delta = scaledClientX(moveEvent) - startX
      return fitWorkbenchWidths(
        containerWidth,
        startLeft + delta,
        startRight,
        {
          leftPanelVisible: true,
          rightPanelVisible
        }
      )
    }

    const onMove = (moveEvent: PointerEvent): void => {
      moveEvent.preventDefault()
      resizeQueue.schedule(computeNext(moveEvent))
    }

    const stop = (upEvent?: PointerEvent): void => {
      const finalSize = upEvent ? resizeQueue.flush(computeNext(upEvent)) : resizeQueue.flush()
      if (finalSize) {
        setLeftSidebarWidth(finalSize.left)
        setRightSidebarWidth(finalSize.right)
      }
      document.body.style.cursor = prevCursor
      document.body.style.userSelect = prevUserSelect
      setLeftResizing(false)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onCancel)
    }

    const onUp = (upEvent: PointerEvent): void => {
      upEvent.preventDefault()
      stop(upEvent)
    }

    const onCancel = (cancelEvent: PointerEvent): void => {
      stop(cancelEvent)
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
  }

  const beginRightResize = (event: ReactPointerEvent<HTMLDivElement>): void => {
    if (event.button !== 0 || !rightPanelVisible) return
    event.preventDefault()
    event.currentTarget.setPointerCapture?.(event.pointerId)
    const startX = scaledClientX(event)
    const startLeft = leftSidebarWidth
    const startRight = rightSidebarWidth
    const prevCursor = document.body.style.cursor
    const prevUserSelect = document.body.style.userSelect
    const commit = (next: { left: number; right: number }): void => {
      if (!leftSidebarCollapsed) applyLeftSidebarDomWidth(next.left)
      applyRightSidebarDomWidth(next.right)
    }
    const resizeQueue = createResizeFrameQueue(commit)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    setRightResizing(true)

    const computeNext = (moveEvent: PointerEvent): { left: number; right: number } => {
      const containerWidth = shellRef.current?.clientWidth ?? window.innerWidth
      const delta = scaledClientX(moveEvent) - startX
      return fitWorkbenchWidths(
        containerWidth,
        startLeft,
        startRight - delta,
        {
          leftPanelVisible: !leftSidebarCollapsed,
          rightPanelVisible: true
        }
      )
    }

    const onMove = (moveEvent: PointerEvent): void => {
      moveEvent.preventDefault()
      resizeQueue.schedule(computeNext(moveEvent))
    }

    const stop = (upEvent?: PointerEvent): void => {
      const finalSize = upEvent ? resizeQueue.flush(computeNext(upEvent)) : resizeQueue.flush()
      if (finalSize) {
        setLeftSidebarWidth(finalSize.left)
        setRightSidebarWidth(finalSize.right)
      }
      document.body.style.cursor = prevCursor
      document.body.style.userSelect = prevUserSelect
      setRightResizing(false)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onCancel)
    }

    const onUp = (upEvent: PointerEvent): void => {
      upEvent.preventDefault()
      stop(upEvent)
    }

    const onCancel = (cancelEvent: PointerEvent): void => {
      stop(cancelEvent)
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
  }

  // Bottom terminal drawer: dragging the top edge up grows the panel. The
  // clamps keep enough chat stage visible above it.
  const beginTerminalResize = (event: ReactPointerEvent<HTMLDivElement>): void => {
    if (event.button !== 0 || !terminalOpen) return
    event.preventDefault()
    event.currentTarget.setPointerCapture?.(event.pointerId)
    const startY = scaledClientY(event)
    const startHeight = terminalHeight
    const prevCursor = document.body.style.cursor
    const prevUserSelect = document.body.style.userSelect
    const resizeQueue = createResizeFrameQueue(setTerminalHeight)
    document.body.style.cursor = 'row-resize'
    document.body.style.userSelect = 'none'
    setTerminalResizing(true)

    const computeNext = (moveEvent: PointerEvent): number => {
      const containerHeight = shellRef.current?.clientHeight ?? window.innerHeight
      const delta = startY - scaledClientY(moveEvent)
      return clampWorkbenchTerminalHeight(startHeight + delta, containerHeight)
    }

    const onMove = (moveEvent: PointerEvent): void => {
      moveEvent.preventDefault()
      resizeQueue.schedule(computeNext(moveEvent))
    }

    const stop = (upEvent?: PointerEvent): void => {
      if (upEvent) resizeQueue.flush(computeNext(upEvent))
      else resizeQueue.flush()
      document.body.style.cursor = prevCursor
      document.body.style.userSelect = prevUserSelect
      setTerminalResizing(false)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onCancel)
    }

    const onUp = (upEvent: PointerEvent): void => {
      upEvent.preventDefault()
      stop(upEvent)
    }

    const onCancel = (cancelEvent: PointerEvent): void => {
      stop(cancelEvent)
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
  }

  const toggleTerminal = (): void => {
    setTerminalOpen((current) => !current)
  }

  return {
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
    toggleRightPanelMode,
    toggleTerminal
  }
}
