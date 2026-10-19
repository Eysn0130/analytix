// @vitest-environment jsdom
import { act, createElement, Suspense, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { TerminalWorkspacePanel } from '../workbench/RightPanelIslands'
import { TerminalPanel } from './TerminalPanel'

const mocks = vi.hoisted(() => ({
  create: vi.fn(), dispose: vi.fn(), resize: vi.fn(), write: vi.fn(),
  renderers: [] as { element: HTMLTextAreaElement; writes: string[]; dispose: ReturnType<typeof vi.fn>; focus: ReturnType<typeof vi.fn> }[],
  data: new Set<(payload: { sessionId: string; data: string }) => void>(),
  exits: new Set<(payload: { sessionId: string }) => void>(),
  dimensions: new Set<(payload: { cols: number; rows: number }) => void>(),
  observers: [] as { disconnect: ReturnType<typeof vi.fn>; callback: () => void }[],
  fit: vi.fn()
}))
vi.mock('../../store/chat-store', () => ({ useChatStore: () => null }))
vi.mock('../workbench/ChatTimelineIsland', () => ({ useChatTimelinePanelState: () => null, useDevPreviewUrls: () => [] }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}
  element = document.createElement('textarea')
  writes: string[] = []
  dispose = vi.fn()
  focus = vi.fn(() => this.element.focus())
  constructor() { mocks.renderers.push(this) }
  loadAddon() {}
  open(container: HTMLElement) { container.append(this.element) }
  write(data: string) { this.writes.push(data) }
  onData() { return { dispose: vi.fn() } }
  onResize(callback: (payload: { cols: number; rows: number }) => void) {
    mocks.dimensions.add(callback)
    return { dispose: () => mocks.dimensions.delete(callback) }
  }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class {
  fit = mocks.fit
  proposeDimensions() { return { cols: 80, rows: 24 } }
} }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))

let root: Root, host: HTMLDivElement
let frames: Map<number, FrameRequestCallback>, frameId: number
const flushFrames = async () => act(async () => {
  const pending = [...frames.values()]; frames.clear()
  for (const callback of pending) callback(0)
})
const render = async (opened: boolean, visible: boolean, focusRequest = 0) => {
  await act(async () => root.render(createElement(Suspense, { fallback: null },
    createElement(TerminalWorkspacePanel, { opened, visible, focusRequest, workspaceRoot: '/synthetic/terminal', onCollapse: vi.fn() }))))
  await act(async () => { await vi.dynamicImportSettled() })
  await flushFrames()
}
const output = async (sessionId: string, data: string) => {
  await act(async () => { for (const callback of mocks.data) callback({ sessionId, data }) })
  await flushFrames()
}
const closeAll = async () => {
  await act(async () => host.querySelector('[role="tab"]')!.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true })))
  const button = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find(item => item.textContent === i18n.t('common:terminalCloseAllTabs'))!
  await act(async () => button.click())
}
const action = (key: string) => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t(`common:${key}`)}"]`)!
const menuItem = (key: string) => document.querySelector<HTMLButtonElement>(`button[role="menuitem"][aria-label="${i18n.t(`common:${key}`)}"]`)!
const openActions = async () => {
  await act(async () => {
    const button = action('terminalTabMenuTitle')
    button.focus()
    button.click()
  })
}

beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  vi.clearAllMocks(); mocks.renderers.length = 0; mocks.observers.length = 0; mocks.data.clear(); mocks.exits.clear(); mocks.dimensions.clear()
  mocks.create.mockResolvedValue({ ok: true }); mocks.dispose.mockResolvedValue(undefined)
  frames = new Map(); frameId = 0
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++frameId, callback); return frameId })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  vi.stubGlobal('ResizeObserver', class {
    disconnect = vi.fn()
    constructor(public callback: () => void) { mocks.observers.push(this) }
    observe() {}
  })
  Object.assign(window, { analytix: { terminal: {
    create: mocks.create, dispose: mocks.dispose, resize: mocks.resize, write: mocks.write,
    onData: (callback: (payload: { sessionId: string; data: string }) => void) => { mocks.data.add(callback); return () => mocks.data.delete(callback) },
    onExit: (callback: (payload: { sessionId: string }) => void) => { mocks.exits.add(callback); return () => mocks.exits.delete(callback) }
  } } })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); vi.useRealTimers(); vi.unstubAllGlobals()
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: false })
})

describe('shared terminal viewer header', () => {
  it('keeps the session strip and SVG actions in one header with workspace controls and shared tooltips', async () => {
    const onToggleFocus = vi.fn(), onCollapse = vi.fn()
    await act(async () => root.render(createElement(TerminalPanel, {
      workspaceRoot: '/synthetic/terminal', onCollapse,
      viewerControls: { focused: false, onToggleFocus, onCollapse }
    })))
    await flushFrames()
    const headers = host.querySelectorAll('[data-workspace-viewer-header]')
    expect(headers).toHaveLength(1)
    expect(headers[0].querySelector('[role="tablist"]')).toBe(host.querySelector('[role="tablist"]'))
    expect(headers[0].querySelectorAll('[role="tab"]')).toHaveLength(1)
    for (const key of ['terminalNewTab', 'terminalRestart', 'terminalTabMenuTitle', 'workbenchFocus', 'workbenchCollapse']) {
      const button = action(key)
      expect(headers[0].contains(button)).toBe(true)
      expect(button.querySelector('svg')).not.toBeNull()
      expect(button.hasAttribute('title')).toBe(false)
    }
    const restart = action('terminalRestart')
    await act(async () => restart.dispatchEvent(new MouseEvent('mouseover', { bubbles: true })))
    const tooltip = document.querySelector('[role="tooltip"][data-portal]')!
    expect(tooltip.textContent).toBe(i18n.t('common:terminalRestart'))
    expect(host.contains(tooltip)).toBe(false)
    await act(async () => restart.dispatchEvent(new MouseEvent('mouseout', { bubbles: true })))
    await act(async () => action('workbenchFocus').click())
    await act(async () => action('workbenchCollapse').click())
    expect(onToggleFocus).toHaveBeenCalledTimes(1)
    expect(onCollapse).toHaveBeenCalledTimes(1)
    expect(mocks.create).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ sessionId: 'main' }))
    expect(mocks.dispose).not.toHaveBeenCalled()
  })

  it('returns keyboard menu focus and routes rename through the existing session without creating a PTY', async () => {
    await render(true, true)
    const trigger = action('terminalTabMenuTitle')
    await openActions()
    const menu = document.querySelector('[role="menu"]')!
    expect(host.contains(menu)).toBe(false)
    expect(document.activeElement).toBe(menuItem('terminalRenameTab'))
    expect(document.activeElement?.closest('[data-workbench-terminal-menu]')).not.toBeNull()
    expect(menuItem('terminalCloseOtherTabs').disabled).toBe(true)
    await act(async () => menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    await openActions()
    await act(async () => menuItem('terminalRenameTab').click())
    await flushFrames()
    const input = host.querySelector<HTMLInputElement>(`input[aria-label="${i18n.t('common:terminalRenameTab')}"]`)!
    expect(document.activeElement).toBe(input)
    expect(input.value).toBe(i18n.t('common:terminalTabTitle', { index: 1 }))
    expect(document.querySelector('[role="menu"]')).toBeNull()
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'Build logs')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
    expect(host.querySelector('[role="tab"]')!.textContent).toBe('Build logs')
    expect(mocks.create).toHaveBeenCalledTimes(1)
    expect(mocks.dispose).not.toHaveBeenCalled()
  })

  it('retires the header menu when hidden or switching sessions while retaining the PTYs', async () => {
    await render(true, true)
    await openActions()
    await render(true, false)
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(action('terminalTabMenuTitle').disabled).toBe(true)
    await render(true, true)
    await act(async () => action('terminalNewTab').click())
    await openActions()
    const firstTab = host.querySelector<HTMLButtonElement>('[role="tab"]')!
    await act(async () => firstTab.click())
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(firstTab.getAttribute('aria-selected')).toBe('true')
    expect(mocks.create.mock.calls.map(([request]) => request.sessionId)).toEqual([
      'main', expect.stringMatching(/^tab-/), 'main'
    ])
    expect(mocks.dispose).not.toHaveBeenCalled()
  })

  it('uses the active session for shared-menu close-others and captures close-all before collapsing', async () => {
    const collapse = vi.fn()
    function Harness() {
      const [visible, setVisible] = useState(true)
      return createElement(Suspense, { fallback: null }, createElement(TerminalWorkspacePanel, {
        opened: true, visible, workspaceRoot: '/synthetic/terminal',
        onCollapse: () => { collapse(); setVisible(false) }
      }))
    }
    await act(async () => root.render(createElement(Harness)))
    await act(async () => { await vi.dynamicImportSettled() })
    await flushFrames()
    await act(async () => action('terminalNewTab').click())
    const activeId = mocks.create.mock.calls[1][0].sessionId
    await openActions()
    expect(menuItem('terminalCloseOtherTabs').disabled).toBe(false)
    await act(async () => menuItem('terminalCloseOtherTabs').click())
    expect(mocks.dispose).toHaveBeenCalledExactlyOnceWith('main')
    expect(host.querySelectorAll('[role="tab"]')).toHaveLength(1)
    expect(mocks.create).toHaveBeenCalledTimes(2)
    await openActions()
    const close = menuItem('terminalCloseAllTabs')
    await act(async () => close.focus())
    expect(document.activeElement?.closest('[data-workbench-terminal-menu]')).not.toBeNull()
    await act(async () => close.click())
    expect(mocks.dispose.mock.calls.map(([sessionId]) => sessionId)).toEqual(['main', activeId])
    expect(collapse).toHaveBeenCalledTimes(1)
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(host.querySelector<HTMLElement>('#workbench-workspace-terminal')!.hidden).toBe(true)
    expect(mocks.create).toHaveBeenCalledTimes(2)
  })
})

describe('retained workspace terminal lifecycle', () => {
  it('retires workspace controls while retaining a hidden renderer and restores a single control on presentation', async () => {
    const viewerControls = { focused: false, onToggleFocus: vi.fn(), onCollapse: vi.fn() }
    const draw = async (visible: boolean) => {
      await act(async () => root.render(createElement(Suspense, { fallback: null },
        createElement(TerminalWorkspacePanel, { opened: true, visible, viewerControls, workspaceRoot: '/synthetic/terminal', onCollapse: vi.fn() }))))
      await act(async () => { await vi.dynamicImportSettled() })
      await flushFrames()
    }
    await draw(true)
    const renderer = mocks.renderers[0]
    expect(host.querySelectorAll('#workbench-workspace-collapse')).toHaveLength(1)
    await draw(false)
    expect(host.querySelectorAll('#workbench-workspace-collapse')).toHaveLength(0)
    expect(renderer.dispose).not.toHaveBeenCalled()
    await draw(true)
    expect(host.querySelectorAll('#workbench-workspace-collapse')).toHaveLength(1)
    expect(mocks.create).toHaveBeenCalledTimes(1)
  })
  it('creates once, keeps live output through switching, folding, closing and reopening, and refocuses the existing renderer', async () => {
    await render(false, false)
    expect(mocks.create).not.toHaveBeenCalled()
    await render(true, false)
    expect(mocks.create).not.toHaveBeenCalled()
    await render(true, true, 1)
    const renderer = mocks.renderers[0]
    expect(mocks.create).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ sessionId: 'main' }))
    await output('main', 'before switch\n')
    for (const opened of [true, false]) {
      await render(opened, false)
      await output('main', 'while hidden\n')
      vi.useFakeTimers()
      const fits = mocks.fit.mock.calls.length
      const resizes = mocks.resize.mock.calls.length
      await act(async () => { mocks.observers[0].callback(); vi.advanceTimersByTime(100) })
      await act(async () => { for (const callback of mocks.dimensions) callback({ cols: 1, rows: 1 }) })
      expect(mocks.fit).toHaveBeenCalledTimes(fits)
      expect(mocks.resize).toHaveBeenCalledTimes(resizes)
      vi.useRealTimers()
      await render(true, true, 2)
      expect(mocks.renderers[0]).toBe(renderer)
      expect(document.activeElement).toBe(renderer.element)
    }
    await render(true, true, 3)
    expect(renderer.writes.join('')).toBe('before switch\nwhile hidden\nwhile hidden\n')
    expect(mocks.create).toHaveBeenCalledTimes(1)
    expect(mocks.dispose).not.toHaveBeenCalled()
    expect(renderer.dispose).not.toHaveBeenCalled()
  })

  it('keeps internal session tabs and their active session across closing the outer workspace tab', async () => {
    await render(true, true)
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:terminalNewTab')}"]`)!.click())
    const session = mocks.create.mock.calls[1][0].sessionId
    expect(session).not.toBe('main')
    await render(false, false)
    await render(true, true)
    expect(host.querySelectorAll('[role="tab"]')).toHaveLength(2)
    expect(mocks.create).toHaveBeenCalledTimes(2)
    expect(mocks.dispose).not.toHaveBeenCalled()
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:terminalCloseTab')}"]`)!.click())
    expect(mocks.dispose).toHaveBeenCalledExactlyOnceWith('main')
  })

  it('only the explicit close-all action disposes sessions and creates a fresh shell on the next open', async () => {
    function Harness() {
      const [visible, setVisible] = useState(true)
      return createElement(Suspense, { fallback: null },
        createElement('button', { onClick: () => setVisible(true), 'data-reopen': '' }, 'Reopen'),
        createElement(TerminalWorkspacePanel, { opened: true, visible, workspaceRoot: '/synthetic/terminal', onCollapse: () => setVisible(false) }))
    }
    await act(async () => root.render(createElement(Harness)))
    await act(async () => { await vi.dynamicImportSettled() })
    await flushFrames()
    await closeAll()
    expect(mocks.dispose).toHaveBeenCalledExactlyOnceWith('main')
    expect(mocks.create).toHaveBeenCalledTimes(1)
    expect(host.querySelector<HTMLElement>('#workbench-workspace-terminal')!.hidden).toBe(true)
    await act(async () => host.querySelector<HTMLButtonElement>('[data-reopen]')!.click())
    await flushFrames()
    expect(mocks.create).toHaveBeenCalledTimes(2)
    expect(mocks.create.mock.calls[1][0].sessionId).not.toBe('main')
  })

  it.each([
    ['new tab', 'resolve'], ['new tab', 'reject'], ['files', 'resolve'], ['files', 'reject']
  ] as const)('preserves a newer %s selection after captured close-all disposal (%s)', async (selection, outcome) => {
    let resolve!: () => void, reject!: (error: Error) => void
    mocks.dispose.mockImplementationOnce(() => new Promise<void>((done, fail) => { resolve = done; reject = fail }))
    const collapse = vi.fn()
    function Harness() {
      const [surface, setSurface] = useState('terminal')
      return createElement(Suspense, { fallback: null },
        createElement('button', { onClick: () => setSurface('terminal'), 'data-reopen': '' }, 'Reopen'),
        createElement('button', { onClick: () => setSurface('files'), 'data-files': '' }, 'Files'),
        createElement('div', { 'data-surface': surface }),
        createElement(TerminalWorkspacePanel, { opened: true, visible: surface === 'terminal', workspaceRoot: '/synthetic/terminal',
          onCollapse: () => { collapse(); setSurface('closed') } }))
    }
    await act(async () => root.render(createElement(Harness)))
    await act(async () => { await vi.dynamicImportSettled() })
    await flushFrames()
    await closeAll()
    expect(collapse).toHaveBeenCalledTimes(1)
    expect(host.querySelector('[data-surface]')!.getAttribute('data-surface')).toBe('closed')
    expect(host.querySelectorAll('[role="tab"]')).toHaveLength(0)
    if (selection === 'new tab') {
      await act(async () => host.querySelector<HTMLButtonElement>('[data-reopen]')!.click())
      await flushFrames()
      await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:terminalNewTab')}"]`)!.click())
      await flushFrames()
      const sessionIds = mocks.create.mock.calls.map(([request]) => request.sessionId)
      expect(sessionIds).toHaveLength(3)
      expect(new Set(sessionIds).size).toBe(3)
    } else {
      await act(async () => host.querySelector<HTMLButtonElement>('[data-files]')!.click())
    }
    const activeRenderer = mocks.renderers.at(-1)
    await act(async () => { if (outcome === 'resolve') resolve(); else reject(new Error('synthetic disposal failure')) })
    await flushFrames()
    expect(collapse).toHaveBeenCalledTimes(1)
    expect(mocks.dispose).toHaveBeenCalledExactlyOnceWith('main')
    expect(host.querySelector('[data-surface]')!.getAttribute('data-surface')).toBe(selection === 'new tab' ? 'terminal' : 'files')
    expect(host.querySelectorAll('[role="tab"]')).toHaveLength(selection === 'new tab' ? 2 : 0)
    expect(mocks.create).toHaveBeenCalledTimes(selection === 'new tab' ? 3 : 1)
    if (selection === 'new tab') expect(activeRenderer?.dispose).not.toHaveBeenCalled()
  })

  it('does not reattach a retired session when an earlier restart finishes after close-all', async () => {
    let finishRestart!: () => void
    mocks.dispose.mockImplementationOnce(() => new Promise<void>(resolve => { finishRestart = resolve }))
    function Harness() {
      const [visible, setVisible] = useState(true)
      return createElement(Suspense, { fallback: null },
        createElement('button', { onClick: () => setVisible(true), 'data-reopen': '' }, 'Reopen'),
        createElement(TerminalWorkspacePanel, { opened: true, visible, workspaceRoot: '/synthetic/terminal', onCollapse: () => setVisible(false) }))
    }
    await act(async () => root.render(createElement(Harness)))
    await act(async () => { await vi.dynamicImportSettled() })
    await flushFrames()
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:terminalRestart')}"]`)!.click())
    await closeAll()
    await act(async () => host.querySelector<HTMLButtonElement>('[data-reopen]')!.click())
    await flushFrames()
    const freshSessionId = mocks.create.mock.calls[1][0].sessionId
    expect(freshSessionId).not.toBe('main')
    await act(async () => finishRestart())
    await flushFrames()
    expect(mocks.create.mock.calls.map(([request]) => request.sessionId)).toEqual(['main', freshSessionId])
    expect(mocks.renderers.at(-1)?.dispose).not.toHaveBeenCalled()
  })

  it('cleans listeners and observers when unmounted while PTY attachment is pending, without killing that session', async () => {
    let resolve!: (result: { ok: true }) => void
    mocks.create.mockImplementation(() => new Promise(result => { resolve = result }))
    await render(true, true, 1)
    await render(false, false)
    await render(true, true, 2)
    expect(mocks.create).toHaveBeenCalledTimes(1)
    expect(mocks.data.size).toBe(1); expect(mocks.exits.size).toBe(1)
    await act(async () => root.render(null))
    expect(mocks.data.size).toBe(0); expect(mocks.exits.size).toBe(0)
    expect(mocks.observers[0].disconnect).toHaveBeenCalledTimes(1)
    expect(mocks.renderers[0].dispose).toHaveBeenCalledTimes(1)
    await act(async () => resolve({ ok: true }))
    expect(mocks.dispose).not.toHaveBeenCalled()
  })
})
