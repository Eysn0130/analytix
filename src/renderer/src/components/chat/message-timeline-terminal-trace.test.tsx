// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ThreadTraceEventPayload } from '@shared/thread-trace'
import type { ChatBlock } from '../../agent/types'
import type { ChatState, TurnForkOwner, TurnForkSnapshot } from '../../store/chat-store-types'
import i18n from '../../i18n'
import { MessageTimeline } from './MessageTimeline'
import { useChatStore } from '../../store/chat-store'
import { registerTerminalDOMTrace } from '../../thread/tracing/thread-performance-trace'
import { ChatView } from './presentation/chat/ChatView'
import { deriveTurnSections } from '../../thread/projection/derive-turn-sections'
import { GateResponseContext, MessageBubble } from './message-timeline-bubbles'

describe('actual terminal answer component trace', () => {
  let root: Root
  let container: HTMLDivElement
  let events: ThreadTraceEventPayload[]
  let originalState: ChatState
  let originalBridge: PropertyDescriptor | undefined
  beforeEach(() => {
    originalState = useChatStore.getState()
    originalBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
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
    useChatStore.setState(originalState, true)
    if (originalBridge) Object.defineProperty(window, 'analytix', originalBridge)
    else Reflect.deleteProperty(window, 'analytix')
    window.localStorage.clear()
    vi.unstubAllGlobals()
  })
  type Surface = 'main' | 'side' | 'inspector'
  type Negative = 'no-owner' | 'hidden' | 'offline' | 'running' | 'failed' | 'unknown' | 'stale'
  function forkTrace(surface: Surface, negative?: Negative) {
    const threadId = 'trace-' + surface, turnId = surface + '-turn'
    const userId = surface + '-user', finalItemId = surface + '-answer'
    const receipt = { schemaVersion: 1 as const, batchDigest: 'a'.repeat(64), threadId, turnId,
      itemId: finalItemId, firstSeq: 11, lastSeq: 13,
      status: negative === 'failed' ? 'failed' as const : 'completed' as const }
    const blocks: ChatBlock[] = [
      { kind: 'user', id: userId, text: 'Synthetic request',
        meta: { turnId, workspaceCheckpointId: surface + '-checkpoint' } },
      { kind: 'assistant', id: finalItemId, text: 'Synthetic committed ' + surface + ' answer',
        meta: { turnId }, ...(negative === 'unknown' ? {} : { generalTerminalProjectionReceipt: receipt }) }
    ]
    const snapshot: TurnForkSnapshot = { blocks, busy: negative === 'running', latestSeq: 13, rewoundTurnIds: {} }
    let current = negative !== 'stale'
    const external: TurnForkOwner = { isCurrent: () => current, getSnapshot: () => snapshot }
    const fork = vi.fn<ChatState['forkThreadFromTurn']>(async () => {})
    const rollback = vi.fn<ChatState['rollbackWorkspaceToCheckpoint']>(async () => {})
    // The secondary checkpoint is deliberately present too: rollback absence
    // must follow Main ownership rather than a fixture missing its checkpoint.
    useChatStore.setState({ activeThreadId: surface === 'main' ? threadId : 'trace-parent', route: 'chat',
      blocks: surface === 'main' ? blocks : [{ kind: 'user', id: 'parent-user', text: 'Unrelated Main body' }],
      lastSeq: surface === 'main' ? 13 : 101, rewoundTurnIds: {}, busy: false,
      currentTurnId: null, currentTurnUserId: null, runtimeConnection: 'ready', error: null,
      forkThreadFromTurn: fork, rollbackWorkspaceToCheckpoint: rollback })
    const element = (selectionKey: string = surface) => {
      const timeline = createElement(MessageTimeline, {
        blocks, live: '', activeThreadId: threadId, runtimeConnection: negative === 'offline' ? 'offline' : 'ready',
        onRetryConnection() {}, onOpenSettings() {}, conversationUsage: null,
        processPaneRoute: { surface, ownerKey: surface, selectionKey, visible: negative !== 'hidden' },
        runtimeStateOverride: { busy: negative === 'running',
          currentTurnId: negative === 'running' ? turnId : null,
          currentTurnUserId: negative === 'running' ? userId : null,
          turnStartedAtByUserId: {}, turnDurationByUserId: {}, authoritativeTurnTimings: {},
          authoritativeTurnStatuses: negative === 'unknown' ? {} : { [turnId]: receipt.status } },
        ...(surface !== 'main' && negative !== 'no-owner' ? { captureForkOwner: () => external } : {})
      })
      return surface === 'main' ? timeline : createElement(GateResponseContext.Provider, { value: {} }, timeline)
    }
    const button = (key: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(node => node.getAttribute('aria-label') === i18n.t('common:' + key))
    return { threadId, turnId, finalItemId, receipt, blocks, snapshot, fork, rollback, element, button,
      setCurrent(value: boolean) { current = value } }
  }
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
  it.each(['main', 'side', 'inspector'] as const)('passes the actual %s Footer source and keeps rollback Main-owned', async surface => {
    const h = forkTrace(surface)
    await act(async () => root.render(h.element()))
    expect(container.querySelector('[data-clock="end"]')).not.toBeNull()
    const forkButton = h.button('forkFromAssistantResponse')!
    expect(forkButton).toBeDefined(); expect(forkButton.getAttribute('aria-disabled')).not.toBe('true')
    await act(async () => forkButton.click())
    expect(h.fork).toHaveBeenCalledOnce()
    const [turnId, source] = h.fork.mock.calls[0]!
    expect(turnId).toBe(h.turnId)
    expect(source).toMatchObject({ threadId: h.threadId, turnId: h.turnId, finalItemId: h.finalItemId,
      status: 'completed', endSeq: 13, proof: { kind: 'general', receipt: h.receipt } })
    expect(source!.isCurrent()).toBe(true)
    expect(source!.getSnapshot()).toEqual(h.snapshot)
    expect(source!.getSnapshot()!.blocks).toEqual(h.blocks)
    const rollbackButton = h.button('rollbackWorkspaceFromAssistantResponse')
    if (surface === 'main') {
      expect(rollbackButton).toBeDefined()
      await act(async () => rollbackButton!.click())
      expect(h.rollback).toHaveBeenCalledWith('main-checkpoint')
    } else { expect(rollbackButton).toBeUndefined(); expect(h.rollback).not.toHaveBeenCalled() }
  })

  it.each((['side', 'inspector'] as const).flatMap(surface =>
    (['no-owner', 'hidden', 'offline', 'running', 'failed', 'unknown', 'stale'] as const)
      .map(negative => ({ surface, negative }))))('does not dispatch the $surface $negative Fork', async ({ surface, negative }) => {
    const h = forkTrace(surface, negative)
    await act(async () => root.render(h.element()))
    const button = h.button('forkFromAssistantResponse')
    if (negative === 'no-owner' || negative === 'unknown') expect(button).toBeUndefined()
    else {
      expect(button).toBeDefined()
      if (negative === 'offline' || negative === 'running' || negative === 'failed') expect(button!.getAttribute('aria-disabled')).toBe('true')
      await act(async () => button!.click())
    }
    expect(h.fork).not.toHaveBeenCalled()
    expect(h.button('rollbackWorkspaceFromAssistantResponse')).toBeUndefined()
  })

  it('keeps the original wrong-Main-owner and null-owner action negatives', async () => {
    const h = forkTrace('main')
    await act(async () => { useChatStore.setState({ activeThreadId: 'trace-other' }); root.render(h.element()) })
    expect(h.button('forkFromAssistantResponse')).toBeUndefined()
    expect(h.button('rollbackWorkspaceFromAssistantResponse')).toBeUndefined()
    await act(async () => {
      useChatStore.setState({ activeThreadId: null })
      root.render(createElement(MessageTimeline, { blocks: h.blocks, live: '', activeThreadId: null,
        runtimeConnection: 'ready', conversationUsage: null, onRetryConnection() {}, onOpenSettings() {},
        runtimeStateOverride: { authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} } }))
    })
    expect(h.button('forkFromAssistantResponse')).toBeUndefined()
    expect(h.button('rollbackWorkspaceFromAssistantResponse')).toBeUndefined()
    expect(h.fork).not.toHaveBeenCalled(); expect(h.rollback).not.toHaveBeenCalled()
  })

  it.each(['main', 'side', 'inspector'] as const)('retires the captured %s source after A→B→A and unmount', async surface => {
    const h = forkTrace(surface)
    await act(async () => root.render(h.element('A')))
    await act(async () => h.button('forkFromAssistantResponse')!.click())
    const source = h.fork.mock.calls[0]![1]!
    await act(async () => root.render(h.element('B')))
    await act(async () => root.render(h.element('A')))
    expect(source.isCurrent()).toBe(false); expect(source.getSnapshot()).toBeNull()
    await act(async () => root.render(null))
    expect(source.isCurrent()).toBe(false); expect(source.getSnapshot()).toBeNull()
  })
  it('observes the shared central final with its explicit secondary thread and deduplicates replay', async () => {
    useChatStore.setState({ activeThreadId: 'trace-other' })
    registerTerminalDOMTrace('trace-side', 'central-answer', 71, false)
    const turn = { blocks: [{ kind: 'assistant' as const, id: 'central-answer', text: 'Synthetic central committed answer' }] }
    const element = createElement(ChatView, { threadId: 'trace-side', turnId: 'turn-side', turn, processing: false,
      turnStatus: 'completed', sections: deriveTurnSections({ turn, isProcessing: false, liveProcessText: '', liveContent: '', workspaceRoot: '' }) })
    await act(() => root.render(element))
    expect(container.querySelector('[data-terminal-trace-seq="71"]')?.textContent).toContain('Synthetic central committed answer')
    expect(events.filter(event => event.name === 'thread.terminal.dom_committed')).toHaveLength(1)
    await act(() => root.render(null)); registerTerminalDOMTrace('trace-side', 'central-answer', 71, false)
    await act(() => root.render(element)); expect(events.filter(event => event.name === 'thread.terminal.dom_committed')).toHaveLength(1)
    expect(JSON.stringify(events)).not.toContain('Synthetic central committed answer')
  })

})
