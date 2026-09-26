// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ThreadTraceEventPayload } from '@shared/thread-trace'
import '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { registerTerminalDOMTrace } from '../../thread/tracing/thread-performance-trace'
import { MessageBubble } from './message-timeline-bubbles'

describe('actual terminal answer component trace', () => {
  let root: Root
  let container: HTMLDivElement
  let events: ThreadTraceEventPayload[]
  beforeEach(() => {
    events = []
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    window.localStorage.setItem('ANALYTIX_THREAD_TRACE', '1')
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
    window.analytix = { diagnostics: { recordThreadTrace: async (event: ThreadTraceEventPayload) => {
      events.push(event)
      return { ok: true }
    } } } as unknown as Window['analytix']
    useChatStore.setState({ activeThreadId: 'trace-a' })
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(() => root.unmount())
    container.remove()
    window.localStorage.clear()
    vi.unstubAllGlobals()
  })
  const render = async (id: string, text = 'Synthetic committed answer') => {
    await act(() => root.render(createElement(MessageBubble, { block: { id, kind: 'assistant', text } })))
  }
  it('observes real DOM after store correlation and deduplicates remount/replay', async () => {
    registerTerminalDOMTrace('trace-a', 'answer-1', 41, false)
    await render('answer-1')
    expect(container.querySelector('[data-terminal-trace-seq="41"]')?.textContent).toContain('Synthetic committed answer')
    expect(events.filter(e => e.name === 'thread.terminal.dom_committed')).toHaveLength(1)
    await act(() => root.render(null))
    registerTerminalDOMTrace('trace-a', 'answer-1', 41, false)
    await render('answer-1')
    expect(events.filter(e => e.name === 'thread.terminal.dom_committed')).toHaveLength(1)
    expect(JSON.stringify(events)).not.toContain('Synthetic committed answer')
  })
  it('does not observe a private draft or an unregistered rejected candidate', async () => {
    await render('live-assistant')
    await render('unregistered-rejected')
    expect(events.filter(e => e.name === 'thread.terminal.dom_committed')).toHaveLength(0)
  })
  it('keeps A and B separate and trace-off does not change rendered content', async () => {
    registerTerminalDOMTrace('trace-a', 'shared-item', 51, true)
    await act(() => useChatStore.setState({ activeThreadId: 'trace-b' }))
    await render('shared-item')
    expect(events.filter(e => e.name === 'thread.terminal.dom_committed')).toHaveLength(0)
    await act(() => useChatStore.setState({ activeThreadId: 'trace-a' }))
    expect(events.filter(e => e.name === 'thread.terminal.dom_committed')).toHaveLength(1)
    window.localStorage.clear()
    await render('trace-disabled')
    expect(container.textContent).toContain('Synthetic committed answer')
    expect(container.querySelector('[data-terminal-trace-seq]')).toBeNull()
  })
  it('a throwing optional diagnostics bridge cannot break the answer commit', async () => {
    window.analytix.diagnostics.recordThreadTrace = () => { throw new Error('diagnostics unavailable') }
    registerTerminalDOMTrace('trace-a', 'bridge-failed', 61, false)
    await render('bridge-failed')
    expect(container.textContent).toContain('Synthetic committed answer')
  })
})
