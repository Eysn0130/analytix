// @vitest-environment jsdom

import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import { SessionHeader } from './SessionHeader'

const initialChatState = useChatStore.getState()
let root: Root
let container: HTMLDivElement

describe('SessionHeader provider-independent data import entry', () => {
  beforeEach(async () => {
    const actEnvironment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    await i18n.changeLanguage('en')
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
    useChatStore.setState({
      ...initialChatState,
      activeThreadId: null,
      threads: [],
      workspaceRoot: '/workspace/analytix',
      runtimeConnection: 'ready'
    })
    Object.defineProperty(window, 'analytix', {
      configurable: true,
      value: { runtime: { directSourcePreview: vi.fn(async () => ({})) } }
    })
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    useChatStore.setState(initialChatState)
    Reflect.deleteProperty(window, 'analytix')
    const actEnvironment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('opens the existing preview dialog without an active thread or Provider turn', async () => {
    useChatStore.setState({ runtimeConnection: 'idle' })
    await act(async () => root.render(createElement(SessionHeader, { compact: true })))
    const entry = container.querySelector<HTMLButtonElement>('button[aria-label="Data import"]')
    expect(entry).not.toBeNull()
    expect(entry?.disabled).toBe(false)
    await act(async () => entry?.click())
    expect(document.querySelector('[role="dialog"][aria-label="Direct source preview"]'))
      .not.toBeNull()
  })

  it('keeps local preview available after a model connection failure but requires a workspace', async () => {
    useChatStore.setState({ runtimeConnection: 'offline' })
    await act(async () => root.render(createElement(SessionHeader, { compact: true })))
    expect(container.querySelector<HTMLButtonElement>('button[aria-label="Data import"]')?.disabled)
      .toBe(false)
    expect(container.querySelector('button[aria-label="Chat actions"]')).toBeNull()

    await act(async () => useChatStore.setState({
      runtimeConnection: 'ready', workspaceRoot: ''
    }))
    expect(container.querySelector('button[aria-label="Data import"]')).toBeNull()
  })
})
