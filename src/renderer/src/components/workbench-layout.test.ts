// @vitest-environment jsdom
import { act, createElement, useCallback } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
import { WORKSPACE_FILE_PREVIEW_EVENT } from '../lib/workspace-file-preview'
import { openTaskDetail, scheduleTaskWorkspaceTabId } from './workbench-schedule-controller'
import {
  useWorkbenchLayout,
  WORKBENCH_TIMELINE_MIN_HEIGHT,
  clampWorkbenchTerminalHeight,
  createWorkbenchScrollReserve,
  maxWorkbenchTerminalHeight,
  readWorkbenchLayoutStorage,
  workbenchLayoutStorageKey,
  workbenchLayoutStorageScope,
  workbenchRightPanelGeometry
} from './workbench-layout'

class MemoryStorage {
  private values = new Map<string, string>()

  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }

  removeItem(key: string): void {
    this.values.delete(key)
  }
}

const originalLocalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')

function stubLocalStorage(): MemoryStorage {
  const storage = new MemoryStorage()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage
  })
  return storage
}

function restoreLocalStorage(): void {
  if (originalLocalStorage) {
    Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
  } else {
    Reflect.deleteProperty(globalThis, 'localStorage')
  }
}

afterEach(() => {
  restoreLocalStorage()
})

describe('workbench layout contracts', () => {
  it('describes no bottom reserve when the bottom panel is closed', () => {
    const reserve = createWorkbenchScrollReserve({
      bottomPanelOpen: false,
      bottomPanelHeight: 360
    })

    expect(reserve.bottomPanelOpen).toBe(false)
    expect(reserve.bottomPanelHeight).toBe(0)
    expect(reserve.bottomPanelReservePx).toBe(0)
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-height']).toBe('0px')
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-reserve']).toBe('0px')
  })

  it('includes the bottom resize handle in the timeline reserve', () => {
    const reserve = createWorkbenchScrollReserve({
      bottomPanelOpen: true,
      bottomPanelHeight: 360
    })

    expect(reserve.bottomPanelHeight).toBe(360)
    expect(reserve.bottomPanelReservePx).toBe(364)
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-height']).toBe('360px')
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-reserve']).toBe('364px')
  })

  it('clamps the terminal without consuming the minimum timeline space', () => {
    const containerHeight = 700

    expect(maxWorkbenchTerminalHeight(containerHeight)).toBe(
      containerHeight - WORKBENCH_TIMELINE_MIN_HEIGHT - 4
    )
    expect(clampWorkbenchTerminalHeight(900, containerHeight)).toBe(436)
    expect(clampWorkbenchTerminalHeight(120, containerHeight)).toBe(220)
  })

  it('temporarily hides an unfit terminal instead of overlapping the composer', () => {
    expect(maxWorkbenchTerminalHeight(586, 315)).toBe(0)
    expect(clampWorkbenchTerminalHeight(360, 586, 315)).toBe(0)
    expect(clampWorkbenchTerminalHeight(360, 946, 315)).toBe(360)
    expect(clampWorkbenchTerminalHeight(360, 700, 200)).toBe(236)
    expect(maxWorkbenchTerminalHeight(583, 100)).toBe(0)
    expect(maxWorkbenchTerminalHeight(584, 100)).toBe(220)
    expect(maxWorkbenchTerminalHeight(Number.NaN, 100)).toBe(0)
  })

  it('prefers workspace-scoped layout values while preserving legacy fallback', () => {
    const storage = stubLocalStorage()
    const workspaceRoot = '/Users/sun/Projects/analytix'
    const scope = workbenchLayoutStorageScope(workspaceRoot)
    storage.setItem('analytix.layout.leftSidebarWidth', '312')
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.leftSidebarWidth', scope), '428')
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.terminalOpen', scope), '1')

    const scopedLayout = readWorkbenchLayoutStorage(workspaceRoot)
    const fallbackLayout = readWorkbenchLayoutStorage('/Users/sun/Projects/other')

    expect(scopedLayout.leftSidebarWidth).toBe(428)
    expect(scopedLayout.terminalOpen).toBe(true)
    expect(fallbackLayout.leftSidebarWidth).toBe(312)
    expect(fallbackLayout.terminalOpen).toBe(false)
  })

  it('lets a scoped closed right panel override a legacy open panel', () => {
    const storage = stubLocalStorage()
    const workspaceRoot = '/Users/sun/Projects/analytix'
    const scope = workbenchLayoutStorageScope(workspaceRoot)
    storage.setItem('analytix.layout.rightPanelMode', 'browser')
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.rightPanelMode', scope), 'none')

    expect(readWorkbenchLayoutStorage(workspaceRoot).rightPanelOpen).toBe(false)
  })
})


describe('workspace tab layout compatibility', () => {
  it('reopens only an explicit taskId tab on the Schedule route without persisting its task metadata', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const storage = stubLocalStorage(), workspace = '/synthetic/schedule', scope = workbenchLayoutStorageScope(workspace)
    useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    let layout!: ReturnType<typeof useWorkbenchLayout>
    function Harness() {
      layout = useWorkbenchLayout({ activeThreadId: 'unrelated-thread', workspaceRoot: workspace,
        latestAutoOpenDevPreviewUrl: null, latestDevPreviewUrl: null, route: 'schedule', writeAssistantOpen: false })
      return null
    }
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(Harness)))
      await act(async () => layout.setRightPanelMode('schedule-task'))
      expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
      await act(async () => openTaskDetail('task-id', 'Actual task'))
      expect(layout.rightPanelMode).toBe('schedule-task'); expect(layout.rightPanelVisible).toBe(true)
      await act(async () => useWorkspaceTabsStore.getState().setOpen(false))
      await act(async () => layout.setRightPanelMode('schedule-task'))
      expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, activeTabId: scheduleTaskWorkspaceTabId('task-id') })
      expect(storage.getItem(workbenchLayoutStorageKey('analytix.layout.rightPanelOpen', scope))).toBe('1')
      expect(storage.getItem(workbenchLayoutStorageKey('analytix.layout.rightPanelMode', scope))).toBeNull()
      await act(async () => useWorkspaceTabsStore.getState().closeTab(scheduleTaskWorkspaceTabId('task-id')))
      expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: true, tabs: [] })
    } finally {
      await act(async () => root.unmount())
      useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    }
  })
  it('refits on composer measurement, restores focus and retains the requested terminal preference', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const storage = stubLocalStorage()
    const workspace = '/synthetic/terminal-budget'
    const scope = workbenchLayoutStorageScope(workspace)
    const heightKey = workbenchLayoutStorageKey('analytix.layout.terminalHeight', scope)
    const openKey = workbenchLayoutStorageKey('analytix.layout.terminalOpen', scope)
    storage.setItem(heightKey, '360')
    storage.setItem(openKey, '1')
    let notify: () => void = () => {}
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { notify = callback }
      observe() {}
      disconnect() {}
    })
    let stageHeight = 946
    let composerHeight = 315
    let layout: ReturnType<typeof useWorkbenchLayout>
    function Harness({ present = true }: { present?: boolean }) {
      layout = useWorkbenchLayout({ activeThreadId: 'a', workspaceRoot: workspace,
        latestAutoOpenDevPreviewUrl: null, latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
      const stageRef = useCallback((node: HTMLElement | null): void => {
        if (node) Object.defineProperty(node, 'clientHeight', { configurable: true, get: () => stageHeight })
        layout.terminalStageRef(node)
      }, [])
      const composerRef = useCallback((node: HTMLDivElement | null): void => {
        if (node) Object.defineProperty(node, 'offsetHeight', { configurable: true, get: () => composerHeight })
        layout.terminalComposerRef(node)
      }, [])
      return createElement('div', null,
        createElement('button', { id: 'workbench-terminal-toggle' }, 'Terminal'),
        present ? createElement('section', { ref: stageRef },
          createElement('div', { ref: composerRef }),
          createElement('div', { ref: layout.terminalPaneRef }, createElement('button', { id: 'terminal-child' }, 'Tab'))) : null)
    }
    const container = document.createElement('div')
    const root = createRoot(container)
    document.body.append(container)
    try {
      await act(async () => root.render(createElement(Harness)))
      expect(layout!.terminalHeight).toBe(360)
      expect(layout!.composerHeight).toBe(315)
      container.querySelector<HTMLButtonElement>('#terminal-child')?.focus()
      stageHeight = 586
      await act(async () => notify())
      expect(layout!.terminalHeight).toBe(0)
      expect(layout!.terminalVisible).toBe(false)
      expect(document.activeElement?.id).toBe('workbench-terminal-toggle')
      expect(storage.getItem(heightKey)).toBe('360')
      expect(storage.getItem(openKey)).toBe('1')
      stageHeight = 946
      await act(async () => notify())
      expect(layout!.terminalHeight).toBe(360)
      composerHeight = 500
      await act(async () => notify())
      expect(layout!.composerHeight).toBe(500)
      expect(layout!.terminalHeight).toBe(0)
      await act(async () => root.render(createElement(Harness, { present: false })))
      expect(layout!.composerHeight).toBe(0)
      expect(layout!.terminalHeight).toBe(0)
      composerHeight = 315
      await act(async () => root.render(createElement(Harness)))
      expect(layout!.composerHeight).toBe(315)
      expect(layout!.terminalHeight).toBe(360)
      expect(storage.getItem(heightKey)).toBe('360')
    } finally {
      await act(async () => root.unmount())
      container.remove()
      vi.unstubAllGlobals()
    }
  })

  it('keeps file identity and bottom terminal state when the right dock folds or the thread changes', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    let layout: ReturnType<typeof useWorkbenchLayout>
    function Harness({ thread, workspace }: { thread: string; workspace: string }) {
      layout = useWorkbenchLayout({ activeThreadId: thread, workspaceRoot: workspace,
        latestAutoOpenDevPreviewUrl: null, latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
      return null
    }
    const container = document.createElement('div')
    const root = createRoot(container)
    document.body.append(container)
    try {
      await act(async () => root.render(createElement(Harness, { thread: 'a', workspace: '/synthetic/a' })))
      await act(async () => {
        layout.toggleTerminal()
        window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: { workspaceRoot: '/synthetic/a', path: '/synthetic/a/report.csv' } }))
      })
      expect(layout!.rightPanelVisible).toBe(true)
      expect(layout!.rightPanelMode).toBe('file')
      expect(layout!.terminalOpen).toBe(true)
      const identity = useWorkspaceTabsStore.getState().activeTabId
      await act(async () => useWorkspaceTabsStore.getState().toggleOpen())
      expect(layout!.rightPanelVisible).toBe(false)
      expect(layout!.terminalOpen).toBe(true)
      await act(async () => useWorkspaceTabsStore.getState().toggleOpen())
      await act(async () => root.render(createElement(Harness, { thread: 'b', workspace: '/synthetic/a' })))
      expect(layout!.rightPanelMode).toBe('file')
      expect(useWorkspaceTabsStore.getState().activeTabId).toBe(identity)
      expect(layout!.terminalOpen).toBe(true)
      // A retained tab owns its source workspace even when navigation moves elsewhere.
      await act(async () => root.render(createElement(Harness, { thread: 'c', workspace: '/synthetic/b' })))
      expect(layout!.filePreviewTarget).toMatchObject({ workspaceRoot: '/synthetic/a', path: '/synthetic/a/report.csv' })
      await act(async () => useWorkspaceTabsStore.getState().closeTab(identity!))
      expect(layout!.rightPanelVisible).toBe(true)
      expect(layout!.rightPanelMode).toBeNull()
      expect(useWorkspaceTabsStore.getState().selectorOpen).toBe(true)
    } finally {
      await act(async () => root.unmount())
      container.remove()
      useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    }
  })
})

it('keeps protected source tabs opaque and retires them on workspace/thread/Host invalidation without retiring ordinary tabs', async () => {
  stubLocalStorage()
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  useWorkspaceTabsStore.setState(useWorkspaceTabsStore.getInitialState(), true)
  let invalidate!: (viewId: string | null) => void
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    onToolResultLocalDisplayInvalidated: (fn: typeof invalidate) => { invalidate = fn; return () => {} }
  } } })
  let layout!: ReturnType<typeof useWorkbenchLayout>
  function Harness({ thread, workspace }: { thread: string; workspace: string }) {
    layout = useWorkbenchLayout({ activeThreadId: thread, workspaceRoot: workspace, latestAutoOpenDevPreviewUrl: null,
      latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
    return null
  }
  const element = document.createElement('div'), root = createRoot(element)
  document.body.append(element)
  const protectedRead = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
    viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64) }
  const source = { path: '/synthetic/a/private.md', workspaceRoot: '/synthetic/a', preview: 'source', protectedRead, line: 9 }
  try {
    await act(async () => root.render(createElement(Harness, { thread: 'thread-a', workspace: '/synthetic/a' })))
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: source })))
    expect(useWorkspaceTabsStore.getState().tabs[0]).toMatchObject({ preview: 'source', title: 'private.md', protectedRead, sourceLine: 9 })
    expect(useWorkspaceTabsStore.getState().tabs[0].path).toBeUndefined()
    expect(useWorkspaceTabsStore.getState().tabs[0].workspaceRoot).toBeUndefined()
    await act(async () => invalidate(protectedRead.viewId))
    expect(layout.filePreviewTarget).toBeNull(); expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: { path: '/synthetic/a/ordinary.go', workspaceRoot: '/synthetic/a' } })))
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: source })))
    await act(async () => root.render(createElement(Harness, { thread: 'thread-b', workspace: '/synthetic/b' })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)
    expect(useWorkspaceTabsStore.getState().tabs[0].path).toBe('/synthetic/a/ordinary.go')
    expect(layout.filePreviewTarget).toBeNull()
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: source })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)
    // Missing source references never reconstruct ordinary path authority.
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: { path: '/forged.md', preview: 'source' } })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)
  } finally { await act(async () => root.unmount()); element.remove() }
})

it('keeps Present index tabs opaque and retires them with the original lease', async () => {
  stubLocalStorage()
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  useWorkspaceTabsStore.setState(useWorkspaceTabsStore.getInitialState(), true)
  let invalidate!: (viewId: string | null) => void
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    onToolResultLocalDisplayInvalidated: (fn: typeof invalidate) => { invalidate = fn; return () => {} }
  } } })
  let layout!: ReturnType<typeof useWorkbenchLayout>
  function Harness({ thread, workspace }: { thread: string; workspace: string }) {
    layout = useWorkbenchLayout({ activeThreadId: thread, workspaceRoot: workspace, latestAutoOpenDevPreviewUrl: null,
      latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
    return null
  }
  const element = document.createElement('div'), root = createRoot(element)
  document.body.append(element)
  const protectedPresent = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
    viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 1 }
  const source = { path: '/synthetic/a/private.md', workspaceRoot: '/synthetic/a', preview: 'source', protectedPresent }
  try {
    await act(async () => root.render(createElement(Harness, { thread: 'thread-a', workspace: '/synthetic/a' })))
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: source })))
    expect(useWorkspaceTabsStore.getState().tabs[0]).toMatchObject({ preview: 'source', title: 'private.md', protectedPresent })
    expect(useWorkspaceTabsStore.getState().tabs[0].path).toBeUndefined()
    expect(useWorkspaceTabsStore.getState().tabs[0].workspaceRoot).toBeUndefined()
    await act(async () => invalidate(protectedPresent.viewId))
    expect(layout.filePreviewTarget).toBeNull(); expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: { path: '/synthetic/a/ordinary.go', workspaceRoot: '/synthetic/a' } })))
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: source })))
    await act(async () => root.render(createElement(Harness, { thread: 'thread-b', workspace: '/synthetic/b' })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)
    expect(useWorkspaceTabsStore.getState().tabs[0].path).toBe('/synthetic/a/ordinary.go')
    expect(layout.filePreviewTarget).toBeNull()
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: source })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)
    // Missing source references never reconstruct ordinary path authority.
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: { path: '/forged.md', preview: 'source' } })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)
  } finally { await act(async () => root.unmount()); element.remove() }
})

describe('right workspace structure and available-space lifecycle', () => {
  it('derives dock bounds from the visible column minima instead of a viewport breakpoint', () => {
    expect(workbenchRightPanelGeometry(1119, 300, true).overlay).toBe(true)
    expect(workbenchRightPanelGeometry(1120, 240, true)).toEqual({ overlay: false, min: 320, max: 320 })
    expect(workbenchRightPanelGeometry(879, 300, false).overlay).toBe(true)
    expect(workbenchRightPanelGeometry(880, 300, false)).toEqual({ overlay: false, min: 320, max: 320 })
    expect(workbenchRightPanelGeometry(1180, 300, true).max).toBe(320)
    expect(workbenchRightPanelGeometry(2000, 300, true).max).toBe(760)
    expect(workbenchRightPanelGeometry(Number.NaN, Number.NaN, true)).toEqual({ overlay: true, min: 320, max: 320 })
  })

  it('migrates an open legacy mode to empty structure and restores the accepted closed state', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const storage = stubLocalStorage(), workspace = '/synthetic/restore-right', scope = workbenchLayoutStorageScope(workspace)
    const openKey = workbenchLayoutStorageKey('analytix.layout.rightPanelOpen', scope)
    const legacyKey = workbenchLayoutStorageKey('analytix.layout.rightPanelMode', scope)
    storage.setItem(legacyKey, 'documents')
    useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    let layout!: ReturnType<typeof useWorkbenchLayout>
    function Harness() {
      layout = useWorkbenchLayout({ activeThreadId: 'a', workspaceRoot: workspace, latestAutoOpenDevPreviewUrl: null,
        latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
      return null
    }
    const host = document.createElement('div'); let root = createRoot(host)
    try {
      await act(async () => root.render(createElement(Harness)))
      expect(layout.rightPanelVisible).toBe(true)
      expect(layout.rightPanelMode).toBeNull()
      expect(layout.filePreviewTarget).toBeNull()
      expect(useWorkspaceTabsStore.getState()).toMatchObject({ tabs: [], activeTabId: null, selectorOpen: true })
      expect(storage.getItem(openKey)).toBe('1')
      expect(storage.getItem(legacyKey)).toBeNull()
      await act(async () => useWorkspaceTabsStore.getState().setOpen(false))
      expect(storage.getItem(openKey)).toBe('0')
      await act(async () => root.unmount())
      root = createRoot(host)
      await act(async () => root.render(createElement(Harness)))
      expect(layout.rightPanelVisible).toBe(false)
      expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
    } finally {
      await act(async () => root.unmount())
      useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
      vi.unstubAllGlobals()
    }
  })

  it('preserves the active object and dock preference while its container enters and leaves overlay', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const storage = stubLocalStorage(), workspace = '/synthetic/overlay-owner', scope = workbenchLayoutStorageScope(workspace)
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.rightPanelOpen', scope), '0')
    const tab = { id: 'retained-office', kind: 'document' as const, mode: 'documents' as const,
      title: 'Retained', workspaceRoot: workspace, path: `${workspace}/report.docx`, dirty: true }
    useWorkspaceTabsStore.setState({ tabs: [tab], activeTabId: tab.id, selectorOpen: false, open: true, focused: false })
    let width = 2000, notify: () => void = () => {}, layout!: ReturnType<typeof useWorkbenchLayout>
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { notify = callback }
      observe() {} disconnect() {}
    })
    function Harness() {
      layout = useWorkbenchLayout({ activeThreadId: 'a', workspaceRoot: workspace, latestAutoOpenDevPreviewUrl: null,
        latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
      const shell = useCallback((node: HTMLDivElement | null) => {
        if (node) Object.defineProperty(node, 'clientWidth', { configurable: true, get: () => width })
        layout.shellRef.current = node
      }, [])
      return createElement('div', { ref: shell })
    }
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(Harness)))
      const dockWidth = layout.rightSidebarWidth
      expect(layout.rightPanelAutoOverlay).toBe(false)
      width = 681
      await act(async () => notify())
      expect(layout.rightPanelAutoOverlay).toBe(true)
      expect(layout.rightSidebarWidth).toBe(dockWidth)
      expect(useWorkspaceTabsStore.getState()).toMatchObject({ activeTabId: tab.id, open: true, selectorOpen: false })
      expect(useWorkspaceTabsStore.getState().tabs[0]).toBe(tab)
      width = 2000
      await act(async () => notify())
      expect(layout.rightPanelAutoOverlay).toBe(false)
      expect(useWorkspaceTabsStore.getState().tabs[0]).toBe(tab)
      expect(storage.getItem(workbenchLayoutStorageKey('analytix.layout.rightPanelOpen', scope))).toBe('1')
      expect(storage.getItem(workbenchLayoutStorageKey('analytix.layout.rightPanelMode', scope))).toBeNull()
    } finally {
      await act(async () => root.unmount())
      useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
      vi.unstubAllGlobals()
    }
  })

  it.each(['blur', 'blur-after-frame', 'capture-loss', 'overlay-after-frame', 'unmount'] as const)('retires a sidebar drag on %s without leaving frames or global input styles', async end => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    stubLocalStorage()
    useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: true, focused: false })
    const frames = new Map<number, FrameRequestCallback>(); let frameId = 0
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.set(++frameId, callback); return frameId })
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id) })
    let width = 2000, notify: () => void = () => {}
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { notify = callback }
      observe() {} disconnect() {}
    })
    let layout!: ReturnType<typeof useWorkbenchLayout>
    function Harness() {
      layout = useWorkbenchLayout({ activeThreadId: 'a', workspaceRoot: '/synthetic/drag', latestAutoOpenDevPreviewUrl: null,
        latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
      const shell = useCallback((node: HTMLDivElement | null) => {
        if (node) Object.defineProperty(node, 'clientWidth', { configurable: true, get: () => width })
        layout.shellRef.current = node
      }, [])
      return createElement('div', { ref: shell }, createElement('aside', { ref: layout.rightPaneRef,
        style: { width: layout.rightPanelAutoOverlay ? 'auto' : layout.rightSidebarWidth } }))
    }
    const host = document.createElement('div'), root = createRoot(host), handle = document.createElement('div')
    const previousCursor = document.body.style.cursor, previousSelect = document.body.style.userSelect
    document.body.style.cursor = 'wait'; document.body.style.userSelect = 'text'
    let unmounted = false
    const pointer = (type: string, clientX: number): PointerEvent => {
      const event = new MouseEvent(type, { clientX, cancelable: true })
      Object.defineProperty(event, 'pointerId', { value: 5 })
      return event as PointerEvent
    }
    try {
      await act(async () => root.render(createElement(Harness)))
      await act(async () => layout.beginRightResize({ button: 0, clientX: 1200, pointerId: 5, currentTarget: handle,
        preventDefault: () => undefined } as unknown as Parameters<typeof layout.beginRightResize>[0]))
      window.dispatchEvent(pointer('pointermove', 1180))
      expect(frames.size).toBe(1)
      if (end.endsWith('after-frame')) {
        const callback = frames.values().next().value!
        frames.clear()
        await act(async () => callback(0))
        expect(host.querySelector('aside')?.style.width).toBe('380px')
      }
      if (end === 'unmount') { await act(async () => root.unmount()); unmounted = true }
      else await act(async () => {
        if (end.startsWith('blur')) window.dispatchEvent(new Event('blur'))
        else if (end === 'overlay-after-frame') { width = 681; notify() }
        else handle.dispatchEvent(pointer('lostpointercapture', 0))
      })
      expect(frames.size).toBe(0)
      expect(document.body.style.cursor).toBe('wait')
      expect(document.body.style.userSelect).toBe('text')
      window.dispatchEvent(pointer('pointermove', 1000))
      expect(frames.size).toBe(0)
      if (!unmounted) expect(layout.rightResizing).toBe(false)
      if (end === 'blur-after-frame') expect(layout.rightSidebarWidth).toBe(380)
      if (end === 'overlay-after-frame') {
        expect(layout.rightPanelAutoOverlay).toBe(true)
        expect(layout.rightSidebarWidth).toBe(360)
        expect(host.querySelector('aside')?.style.width).toBe('auto')
      }
    } finally {
      if (!unmounted) await act(async () => root.unmount())
      raf.mockRestore(); cancel.mockRestore()
      document.body.style.cursor = previousCursor; document.body.style.userSelect = previousSelect
      useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
      vi.unstubAllGlobals()
    }
  })
})

it('keeps legacy structure recoverable when the replacement layout write is rejected', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const storage = stubLocalStorage(), workspace = '/synthetic/rejected-layout', scope = workbenchLayoutStorageScope(workspace)
  const legacyKey = workbenchLayoutStorageKey('analytix.layout.rightPanelMode', scope)
  const openKey = workbenchLayoutStorageKey('analytix.layout.rightPanelOpen', scope)
  storage.setItem(legacyKey, 'documents')
  const write = storage.setItem.bind(storage)
  const rejection = vi.spyOn(storage, 'setItem').mockImplementation((key, value) => {
    if (key === openKey) throw new Error('Synthetic layout storage rejection')
    write(key, value)
  })
  useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
  function Harness() {
    useWorkbenchLayout({ activeThreadId: 'a', workspaceRoot: workspace, latestAutoOpenDevPreviewUrl: null,
      latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
    return null
  }
  const host = document.createElement('div'), root = createRoot(host)
  try {
    await act(async () => root.render(createElement(Harness)))
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: true, tabs: [] })
    expect(storage.getItem(openKey)).toBeNull()
    expect(storage.getItem(legacyKey)).toBe('documents')
  } finally {
    await act(async () => root.unmount())
    rejection.mockRestore()
    useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    vi.unstubAllGlobals()
  }
})

it.each(['blur', 'workspace'] as const)('ends terminal resizing on %s and ignores retired pointers without leaving a frame or cursor', async end => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const storage = stubLocalStorage(), workspace = '/synthetic/terminal-blur'
  storage.setItem(workbenchLayoutStorageKey('analytix.layout.terminalOpen', workbenchLayoutStorageScope(workspace)), '1')
  const nextWorkspace = '/synthetic/terminal-next', nextScope = workbenchLayoutStorageScope(nextWorkspace)
  storage.setItem(workbenchLayoutStorageKey('analytix.layout.terminalOpen', nextScope), '1')
  storage.setItem(workbenchLayoutStorageKey('analytix.layout.terminalHeight', nextScope), '300')
  useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
  const frames = new Map<number, FrameRequestCallback>(); let frameId = 0, layout!: ReturnType<typeof useWorkbenchLayout>
  const raf = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.set(++frameId, callback); return frameId })
  const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id) })
  function Harness({ currentWorkspace = workspace }: { currentWorkspace?: string }) {
    layout = useWorkbenchLayout({ activeThreadId: 'a', workspaceRoot: currentWorkspace, latestAutoOpenDevPreviewUrl: null,
      latestDevPreviewUrl: null, route: 'chat', writeAssistantOpen: false })
    const stage = useCallback((node: HTMLElement | null) => {
      if (node) Object.defineProperty(node, 'clientHeight', { configurable: true, value: 946 })
      layout.terminalStageRef(node)
    }, [])
    const composer = useCallback((node: HTMLDivElement | null) => {
      if (node) Object.defineProperty(node, 'offsetHeight', { configurable: true, value: 315 })
      layout.terminalComposerRef(node)
    }, [])
    return createElement('section', { ref: stage }, createElement('div', { ref: composer }))
  }
  const host = document.createElement('div'), root = createRoot(host), handle = document.createElement('div')
  const cursor = document.body.style.cursor, select = document.body.style.userSelect
  const move = (id: number): PointerEvent => {
    const event = new MouseEvent('pointermove', { clientY: 190, cancelable: true })
    Object.defineProperty(event, 'pointerId', { value: id })
    return event as PointerEvent
  }
  try {
    await act(async () => root.render(createElement(Harness)))
    expect(layout.terminalVisible).toBe(true)
    await act(async () => layout.beginTerminalResize({ button: 0, clientY: 200, pointerId: 7, currentTarget: handle,
      preventDefault: () => undefined } as unknown as Parameters<typeof layout.beginTerminalResize>[0]))
    window.dispatchEvent(move(8))
    expect(frames.size).toBe(0)
    window.dispatchEvent(move(7))
    expect(frames.size).toBe(1)
    if (end === 'workspace') await act(async () => root.render(createElement(Harness, { currentWorkspace: nextWorkspace })))
    else await act(async () => window.dispatchEvent(new Event('blur')))
    expect(frames.size).toBe(0)
    expect(layout.terminalHeight).toBe(end === 'workspace' ? 300 : 367)
    expect(layout.terminalResizing).toBe(false)
    expect(document.body.style.cursor).toBe(cursor)
    expect(document.body.style.userSelect).toBe(select)
    window.dispatchEvent(move(7))
    expect(frames.size).toBe(0)
    if (end === 'workspace') expect(storage.getItem(workbenchLayoutStorageKey('analytix.layout.terminalHeight', nextScope))).toBe('300')
  } finally {
    await act(async () => root.unmount())
    raf.mockRestore(); cancel.mockRestore()
    document.body.style.cursor = cursor; document.body.style.userSelect = select
    useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: false, focused: false })
    vi.unstubAllGlobals()
  }
})


it('keeps a legacy path tab identity but sends no-path documents navigation to the selector',async()=>{
  Object.assign(globalThis,{IS_REACT_ACT_ENVIRONMENT:true});stubLocalStorage()
  useWorkspaceTabsStore.setState({tabs:[],activeTabId:null,selectorOpen:true,open:true,focused:false})
  let layout!:ReturnType<typeof useWorkbenchLayout>
  function Harness(){layout=useWorkbenchLayout({activeThreadId:'legacy',workspaceRoot:'/synthetic',latestAutoOpenDevPreviewUrl:null,latestDevPreviewUrl:null,route:'chat',writeAssistantOpen:false});return null}
  const host=document.createElement('div'),root=createRoot(host)
  const tab={id:'retained-original-id',kind:'document' as const,mode:'documents' as const,title:'old.md',path:'/synthetic/old.md',workspaceRoot:'/synthetic'}
  try {
    await act(async()=>root.render(createElement(Harness)))
    await act(async()=>layout.setRightPanelMode('documents'))
    expect(useWorkspaceTabsStore.getState()).toMatchObject({selectorOpen:true,tabs:[]})
    await act(async()=>useWorkspaceTabsStore.getState().openTab(tab))
    await act(async()=>layout.setRightPanelMode('documents'))
    expect(useWorkspaceTabsStore.getState()).toMatchObject({activeTabId:tab.id,selectorOpen:false,tabs:[tab]})
    await act(async()=>useWorkspaceTabsStore.getState().openTab({id:'tool:documents',kind:'tool',mode:'documents',title:'Legacy'}))
    await act(async()=>layout.setRightPanelMode('documents'))
    expect(useWorkspaceTabsStore.getState().selectorOpen).toBe(true)
    expect(useWorkspaceTabsStore.getState().tabs.find(item=>item.id===tab.id)).toEqual(tab)
  } finally {await act(async()=>root.unmount());useWorkspaceTabsStore.setState({tabs:[],activeTabId:null,selectorOpen:true,open:false})}
})
