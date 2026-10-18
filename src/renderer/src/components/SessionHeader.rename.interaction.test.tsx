// @vitest-environment jsdom

import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import { SessionHeader } from './SessionHeader'

vi.mock('../hooks/use-thread-usage', async (importOriginal) => ({
  ...await importOriginal<object>(),
  useThreadUsage: () => null
}))

const initialState = useChatStore.getState()
const threads = ['a', 'b'].map((suffix) => ({
  id: `thread-${suffix}`,
  title: `Chat ${suffix.toUpperCase()}`,
  updatedAt: '2026-10-05T00:00:00.000Z',
  model: 'deepseek-chat',
  mode: 'chat' as const,
  workspace: '/workspace/analytix'
}))
let host: HTMLDivElement
let root: Root
let rename: Mock<(title: string) => Promise<void>>

function deferred(): {
  promise: Promise<void>
  resolve: () => void
  reject: (error: Error) => void
} {
  let resolve!: () => void
  let reject!: (error: Error) => void
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('requestAnimationFrame', vi.fn(() => 0))
  vi.stubGlobal('cancelAnimationFrame', vi.fn())
  await i18n.changeLanguage('en')
  rename = vi.fn(async () => undefined)
  host = document.createElement('div')
  document.body.append(host)
  root = createRoot(host)
  useChatStore.setState({
    ...initialState,
    route: 'chat',
    activeThreadId: 'thread-a',
    threads,
    workspaceRoot: '/workspace/analytix',
    workspaceLabel: 'Working directory',
    runtimeConnection: 'ready',
    busy: false,
    blocks: [],
    renameActiveThread: rename
  })
})

afterEach(async () => {
  await act(async () => root.unmount())
  host.remove()
  useChatStore.setState(initialState)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function input(): HTMLInputElement {
  const editor = host.querySelector<HTMLInputElement>('input[aria-label="Click to rename thread"]')
  if (!editor) throw new Error('Missing rename editor')
  return editor
}

async function beginRename(compact: boolean): Promise<HTMLInputElement> {
  if (compact) {
    const trigger = host.querySelector<HTMLButtonElement>('button[aria-label="Chat actions"]')
    if (!trigger) throw new Error('Missing chat actions trigger')
    await act(async () => trigger.click())
    const entry = [...document.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]')]
      .find((button) => button.textContent?.includes('Rename chat'))
    if (!entry) throw new Error('Missing rename action')
    await act(async () => entry.click())
  } else {
    const title = host.querySelector<HTMLButtonElement>('button[title="Click to rename thread"]')
    if (!title) throw new Error('Missing title rename entry')
    await act(async () => title.click())
  }
  return input()
}

async function typeTitle(editor: HTMLInputElement, title: string): Promise<void> {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  await act(async () => {
    setter.call(editor, title)
    editor.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

async function key(editor: HTMLInputElement, value: string): Promise<void> {
  await act(async () => editor.dispatchEvent(new KeyboardEvent('keydown', {
    key: value,
    bubbles: true,
    cancelable: true
  })))
}

describe.each([true, false])('SessionHeader rename completion compact=%s', (compact) => {
  it('retains the failed draft, suppresses duplicate pending submits, and closes after a successful retry', async () => {
    const failed = deferred()
    const retried = deferred()
    rename.mockImplementationOnce(() => failed.promise)
    rename.mockImplementationOnce(() => retried.promise)
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    const editor = await beginRename(compact)
    await typeTitle(editor, '  Retained draft  ')
    await key(editor, 'Enter')
    await act(async () => editor.dispatchEvent(new FocusEvent('focusout', { bubbles: true })))
    expect(rename).toHaveBeenCalledTimes(1)
    expect(rename).toHaveBeenCalledWith('Retained draft')

    await act(async () => failed.reject(new Error('Synthetic rename failure')))
    expect(input()).toBe(editor)
    expect(input().value).toBe('  Retained draft  ')
    await key(editor, 'Enter')
    expect(rename).toHaveBeenCalledTimes(2)
    expect(input()).toBe(editor)
    await act(async () => retried.resolve())
    expect(host.querySelector('input')).toBeNull()
  })

  it('keeps a newer typed draft when an earlier title is acknowledged', async () => {
    const pending = deferred()
    rename.mockImplementationOnce(() => pending.promise)
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    const editor = await beginRename(compact)
    await typeTitle(editor, 'Submitted title')
    await key(editor, 'Enter')
    await typeTitle(editor, 'Newer draft')
    await act(async () => {
      useChatStore.setState({
        threads: useChatStore.getState().threads.map((thread) => thread.id === 'thread-a'
          ? { ...thread, title: 'Submitted title' }
          : thread)
      })
      pending.resolve()
    })
    expect(input()).toBe(editor)
    expect(input().value).toBe('Newer draft')
    await key(editor, 'Enter')
    expect(rename).toHaveBeenLastCalledWith('Newer draft')
    expect(host.querySelector('input')).toBeNull()
  })

  it('does not close another chat editor when an old rename completes', async () => {
    const pending = deferred()
    rename.mockImplementationOnce(() => pending.promise)
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    const editorA = await beginRename(compact)
    await typeTitle(editorA, 'Old A title')
    await key(editorA, 'Enter')
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-b' }))
    const editorB = await beginRename(compact)
    await typeTitle(editorB, 'B draft')
    await act(async () => pending.resolve())
    expect(input()).toBe(editorB)
    expect(input().value).toBe('B draft')
    expect(rename).toHaveBeenCalledTimes(1)
  })

  it('does not close a reopened A editor after navigating through B', async () => {
    const pending = deferred()
    rename.mockImplementationOnce(() => pending.promise)
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    const oldEditor = await beginRename(compact)
    await typeTitle(oldEditor, 'Old A title')
    await key(oldEditor, 'Enter')
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-b' }))
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-a' }))
    const newEditor = await beginRename(compact)
    await typeTitle(newEditor, 'Reopened A draft')
    await act(async () => {
      useChatStore.setState({
        threads: useChatStore.getState().threads.map((thread) => thread.id === 'thread-a'
          ? { ...thread, title: 'Old A title' }
          : thread)
      })
      pending.resolve()
    })
    expect(input()).toBe(newEditor)
    expect(input().value).toBe('Reopened A draft')
  })

  it('does not close a new editor after the previous editor was cancelled', async () => {
    const pending = deferred()
    rename.mockImplementationOnce(() => pending.promise)
    await act(async () => root.render(createElement(SessionHeader, { compact })))
    const oldEditor = await beginRename(compact)
    await typeTitle(oldEditor, 'Cancelled request title')
    await key(oldEditor, 'Enter')
    await key(oldEditor, 'Escape')
    const newEditor = await beginRename(compact)
    await typeTitle(newEditor, 'New editor draft')
    await act(async () => pending.resolve())
    expect(input()).toBe(newEditor)
    expect(input().value).toBe('New editor draft')
    expect(rename).toHaveBeenCalledTimes(1)
  })
})
