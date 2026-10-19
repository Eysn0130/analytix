// @vitest-environment jsdom
import { act, createElement, Suspense, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { TerminalWorkspacePanel } from '../workbench/RightPanelIslands'

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

describe('retained workspace terminal lifecycle', () => {
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
