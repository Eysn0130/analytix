// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import { SessionHeader } from './SessionHeader'

vi.mock('../hooks/use-thread-usage', async importOriginal => ({ ...await importOriginal<object>(), useThreadUsage: () => null }))
const initial = useChatStore.getState()
let host: HTMLDivElement, root: Root, frames: Map<number, FrameRequestCallback>, frameId: number
let rename: Mock<(title: string) => Promise<void>>, archive: Mock<(threadId: string, archived: boolean) => Promise<void>>
let fork: Mock<() => Promise<void>>, schedule: Mock<() => void>
let pinned: string | null
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  pinned = localStorage.getItem('analytix:pinned-thread-ids:v1')
  localStorage.removeItem('analytix:pinned-thread-ids:v1')
  frames = new Map(); frameId = 0
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++frameId, callback); return frameId })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  rename = vi.fn(async () => undefined); archive = vi.fn(async () => undefined); fork = vi.fn(async () => undefined); schedule = vi.fn()
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  useChatStore.setState({ ...initial, route: 'chat', activeThreadId: 'thread-a',
    threads: [{ id: 'thread-a', title: 'Keyboard audit', updatedAt: '2026-10-05T00:00:00.000Z', model: 'deepseek-chat', mode: 'chat', workspace: '/workspace/analytix' }],
    workspaceRoot: '/workspace/analytix', workspaceLabel: 'Working directory', runtimeConnection: 'ready', busy: false, blocks: [],
    renameActiveThread: rename, archiveThread: archive, forkActiveThread: fork, openSchedule: schedule, setError: vi.fn() })
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove(); useChatStore.setState(initial)
  if (pinned === null) localStorage.removeItem('analytix:pinned-thread-ids:v1')
  else localStorage.setItem('analytix:pinned-thread-ids:v1', pinned)
  vi.unstubAllGlobals(); vi.restoreAllMocks()
})
async function flush(): Promise<void> { await act(async () => { for (const [id, callback] of [...frames]) { frames.delete(id); callback(0) } }) }
async function key(target: HTMLElement, value: string, extra: KeyboardEventInit = {}): Promise<KeyboardEvent> {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...extra })
  await act(async () => target.dispatchEvent(event)); await flush(); return event
}
function item(text: string, scope: ParentNode = document): HTMLButtonElement {
  const found = [...scope.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]')].find(node => node.textContent?.includes(text))
  if (!found) throw new Error('Missing menu item: ' + text)
  return found
}
function trigger(): HTMLButtonElement { return host.querySelector<HTMLButtonElement>('button[aria-label="Chat actions"]')! }

it('does not steal outside focus when an asynchronous Copy callback completes', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  const outside = document.createElement('button'); document.body.append(outside)
  let resolve!: () => void
  const writeText = vi.fn(() => new Promise<void>(done => { resolve = done }))
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  try {
    await act(async () => root.render(createElement(SessionHeader, { compact: true })))
    await key(trigger(), 'ArrowDown'); const copy = item('Copy'); copy.focus(); await key(copy, 'ArrowRight')
    await act(async () => item('Copy chat ID').click())
    expect(writeText).toHaveBeenCalledWith('thread-a')
    await act(async () => outside.focus())
    await act(async () => resolve())
    expect(document.activeElement).toBe(outside)
  } finally {
    outside.remove()
    if (descriptor) Object.defineProperty(navigator, 'clipboard', descriptor)
    else Reflect.deleteProperty(navigator, 'clipboard')
  }
})

it.each([
  ['busy', { busy: true }],
  ['offline', { runtimeConnection: 'offline' as const }]
])('returns to an enabled root action when %s gates disable the focused Branch submenu', async (_label, gate) => {
  await act(async () => root.render(createElement(SessionHeader, { compact: true })))
  await key(trigger(), 'ArrowDown')
  const branch = item('Branch'); branch.focus(); await key(branch, 'ArrowRight')
  const submenu = document.querySelector<HTMLElement>('[role="menu"][aria-label="Branch"]')!
  const child = submenu.querySelector<HTMLButtonElement>('button[role="menuitem"]')!
  expect(document.activeElement).toBe(child)
  await act(async () => useChatStore.setState(gate))
  expect(branch.disabled).toBe(true); expect(child.disabled).toBe(true)
  expect(document.querySelector('[role="menu"][aria-label="Branch"]')).toBeNull()
  expect(document.querySelector('[data-session-actions-menu]')).not.toBeNull()
  expect(document.activeElement).toBe(item('Pin'))
  expect(fork).not.toHaveBeenCalled()
  await key(document.activeElement as HTMLElement, 'Escape')
  expect(document.querySelector('[data-session-actions-menu]')).toBeNull()
  expect(document.activeElement).toBe(trigger())
})

describe.each([true, false])('Session menu compact=%s', compact => {
  it('enters first/last enabled actions without firing callbacks and handles layered submenu Escape', async () => {
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    trigger().focus(); await key(trigger(), 'ArrowDown')
    expect(document.activeElement).toBe(item('Pin'))
    const copy = item('Copy')
    copy.focus(); await key(copy, 'ArrowRight')
    expect(document.activeElement).toBe(item('Copy chat as Markdown'))
    await key(document.activeElement as HTMLElement, 'End')
    expect(document.activeElement).toBe(item('Copy Codex-readable link'))
    await key(document.activeElement as HTMLElement, 'Escape', { isComposing: true })
    expect(document.querySelector('[role="menu"][aria-label="Copy"]')).not.toBeNull()
    await key(document.activeElement as HTMLElement, 'Escape')
    expect(document.activeElement).toBe(copy)
    await key(copy, 'Escape'); expect(document.activeElement).toBe(trigger())
    expect(document.querySelector('[data-session-actions-menu]')).toBeNull()
    await key(trigger(), 'ArrowUp'); expect(document.activeElement).toBe(item('Open in new window'))
    expect(rename).not.toHaveBeenCalled(); expect(archive).not.toHaveBeenCalled(); expect(fork).not.toHaveBeenCalled(); expect(schedule).not.toHaveBeenCalled()
  })

  it('keeps native Tab unprevented, preserves ready/busy gates and restores focus on same-thread refresh', async () => {
    useChatStore.setState({ runtimeConnection: 'offline', busy: true })
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    await key(trigger(), 'ArrowDown')
    expect(item('Rename chat').disabled).toBe(true); expect(item('Branch').disabled).toBe(true)
    await key(document.activeElement as HTMLElement, 'ArrowDown')
    expect(document.activeElement).toBe(item('Data import'))
    const event = await key(document.activeElement as HTMLElement, 'Tab', { shiftKey: true })
    expect(event.defaultPrevented).toBe(false); expect(document.querySelector('[data-session-actions-menu]')).toBeNull()
    await key(trigger(), 'ArrowDown')
    await act(async () => useChatStore.setState({ threads: useChatStore.getState().threads.map(thread => ({ ...thread, updatedAt: '2026-10-05T00:01:00.000Z' })) }))
    expect(document.querySelector('[data-session-actions-menu]')).toBeNull(); expect(document.activeElement).toBe(trigger())
  })

  it('keeps a keyboard-focused child when hover crosses another item and sends rename through its existing callback', async () => {
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    await key(trigger(), 'ArrowDown'); const copy = item('Copy'); copy.focus(); await key(copy, 'ArrowRight')
    const child = document.activeElement
    await act(async () => item('Rename chat').dispatchEvent(new MouseEvent('pointerover', { bubbles: true })))
    expect(document.activeElement).toBe(child); expect(child?.isConnected).toBe(true)
    await key(child as HTMLElement, 'ArrowLeft'); await act(async () => item('Rename chat').click())
    const input = host.querySelector<HTMLInputElement>('input')!
    expect(document.activeElement).toBe(input)
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => { setter.call(input, '  Updated title  '); input.dispatchEvent(new Event('input', { bubbles: true })) })
    await key(input, 'Enter'); expect(rename).toHaveBeenCalledWith('Updated title')
  })
})
