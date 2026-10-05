// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { FloatingComposer } from './FloatingComposer'

let root: Root, host: HTMLDivElement
let previous: ReturnType<typeof useChatStore.getState>
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  previous = useChatStore.getState()
  useChatStore.setState({ activeThreadId: 'menu-fixture', activeThreadGoal: null, route: 'chat',
    workspaceRoot: '/workspace/fixture', blocks: [], busy: false })
  await i18n.changeLanguage('zh')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  await act(async () => root.render(createElement(FloatingComposer, {
    input: '', setInput: vi.fn(), mode: 'agent', setMode: vi.fn(),
    busy: false, runtimeReady: true, hasActiveThread: true,
    composerModel: '', composerPickList: [], onComposerModelChange: vi.fn(),
    queuedMessages: [], onRemoveQueuedMessage: vi.fn(), onSend: vi.fn(), onInterrupt: vi.fn(),
    onPlanCommand: vi.fn(), onNewCommand: vi.fn(), fileReferenceEnabled: true,
    attachmentUploadEnabled: true, hideThreadContextPanels: true
  })))
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); useChatStore.setState(previous)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
const trigger = () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:composerMenuTitle')}"]`)!
const menu = () => host.querySelector<HTMLElement>('[role="menu"]')
async function open() {
  // Commit the menu before waiting for its autofocus frame.
  await act(async () => { trigger().click() })
  await act(async () => { await new Promise<void>(resolve => requestAnimationFrame(() => resolve())) })
  expect(menu()).not.toBeNull()
  expect(document.activeElement).toBe(menu()!.querySelector('button:not(:disabled)'))
}
async function key(target: HTMLElement, value: string, options: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...options })
  await act(async () => target.dispatchEvent(event))
  return event
}

describe('ordinary composer plus-menu keyboard ownership', () => {
  it.each([false, true])('closes on exit Tab without preventing native navigation (shift=%s)', async shiftKey => {
    await open()
    expect(menu()).not.toBeNull()
    const current = document.activeElement as HTMLElement
    expect(menu()!.contains(current)).toBe(true)
    const event = await key(current, 'Tab', { shiftKey })
    expect(event.defaultPrevented).toBe(false)
    expect(menu()).toBeNull()
    expect(trigger().getAttribute('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger())
  })
  it('keeps arrow/Home/End and Escape behavior while closing when focus leaves', async () => {
    await open()
    expect(menu()!.querySelector('button:disabled')).not.toBeNull()
    const items = [...menu()!.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
    await key(items[0], 'End'); expect(document.activeElement).toBe(items.at(-1))
    await key(items.at(-1)!, 'Home'); expect(document.activeElement).toBe(items[0])
    await key(items[0], 'ArrowUp'); expect(document.activeElement).toBe(items.at(-1))
    await key(items.at(-1)!, 'Escape')
    expect(menu()).toBeNull(); expect(document.activeElement).toBe(trigger())
    await open()
    const outside = document.createElement('button'); document.body.append(outside)
    try {
      await act(async () => outside.focus())
      expect(menu()).toBeNull()
      expect(document.activeElement).toBe(outside)
    } finally { outside.remove() }
  })
  it.each([{ key: 'Escape', isComposing: true }, { key: 'Tab', isComposing: true }, { key: 'ArrowDown', isComposing: true }, { key: 'Escape', keyCode: 229 }])('does not treat composition $key keystrokes as menu navigation or dismissal', async options => {
    await open()
    const current = document.activeElement as HTMLElement
    const event = await key(current, options.key, options)
    expect(event.defaultPrevented).toBe(false)
    expect(menu()).not.toBeNull()
    expect(document.activeElement).toBe(current)
  })
})
