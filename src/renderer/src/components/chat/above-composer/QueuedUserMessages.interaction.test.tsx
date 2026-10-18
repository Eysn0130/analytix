// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../../i18n'
import { useChatStore } from '../../../store/chat-store'
import { createThreadActions } from '../../../store/chat-store-thread-actions'
import { queuedMessagesForThread } from '../../../store/chat-store-queue'
import type { QueuedUserMessage } from '../../../store/chat-store-types'
import { QueuedUserMessages } from './QueuedUserMessages'
import { InputBar } from '../InputBar'
import { ComposerGoalRow } from './ComposerGoalRow'
import { AboveComposerPanelStack } from './AboveComposerPanel'

const registryMock = vi.hoisted(() => ({ getProvider: vi.fn(() => ({})) }))
vi.mock('../../../agent/registry', () => ({ getProvider: registryMock.getProvider }))

let host: HTMLDivElement, root: Root
let previous: ReturnType<typeof useChatStore.getState>
let actions: ReturnType<typeof createThreadActions>
function QueueFixture() {
  const messages = useChatStore(state => state.queuedMessages)
  return createElement(QueuedUserMessages, { messages })
}
const button = (key: string, owner: ParentNode = host) => owner.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:' + key)}"]`)!
async function enter(input: HTMLInputElement, options: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true, ...options })
  await act(async () => { input.dispatchEvent(event) })
  return event
}
async function inputValue(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  registryMock.getProvider.mockClear()
  previous = useChatStore.getState()
  actions = createThreadActions({ set: partial => useChatStore.setState(partial), get: useChatStore.getState,
    sseAbortRef: { current: null } })
  useChatStore.setState({ ...actions, activeThreadId: 'queue-a', queuedMessages: [], queuedMessagePauses: {},
    busy: true, currentTurnId: null, runtimeConnection: 'ready' })
  await i18n.changeLanguage('zh')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  await act(async () => { root.render(createElement(QueueFixture)) })
})
afterEach(async () => {
  await act(async () => { root.unmount() })
  host.remove(); useChatStore.setState(previous)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('Queue visible thread and keyboard owner', () => {
  // These were SSR fixtures. Queue now reads its actual selected store owner,
  // so run the same visible assertions against the production client snapshot.
  const queued = { threadId: 'queue-a', id: 'visible-queue', text: 'queued follow-up', mode: 'agent' as const }
  it('renders queued messages in the real InputBar above-composer stack', async () => {
    await act(async () => { root.render(createElement(InputBar, {
      input: '', setInput: vi.fn(), mode: 'agent', setMode: vi.fn(), busy: false,
      runtimeReady: true, hasActiveThread: true, workspaceRootOverride: '/workspace/fixture',
      composerModel: '', composerPickList: [], onComposerModelChange: vi.fn(),
      queuedMessages: [queued, { ...queued, threadId: 'queue-b', text: 'other thread draft' }],
      onRemoveQueuedMessage: actions.removeQueuedMessage, onSend: vi.fn(), onInterrupt: vi.fn(),
      attachmentUploadEnabled: false, webAccessAvailable: false
    })) })
    expect(host.querySelector('[data-above-composer-portal]')).not.toBeNull()
    expect(host.textContent).toContain(queued.text)
    expect(host.textContent).not.toContain('other thread draft')
  })
  it('renders Queue before the actual Goal row when both owners exist', async () => {
    await act(async () => { root.render(createElement(AboveComposerPanelStack, null,
      createElement(QueuedUserMessages, { messages: [queued] }),
      createElement(ComposerGoalRow, { goal: { threadId: queued.threadId, objective: 'Ship the top tray', status: 'active',
        tokensUsed: 0, timeUsedSeconds: 61, createdAt: '2026-06-29T00:00:00Z', updatedAt: '2026-06-29T00:01:00Z' },
        elapsedLabel: '1m 1s', onEdit: vi.fn(), onToggleStatus: vi.fn(), onClear: vi.fn() })
    )) })
    expect(host.textContent).toContain(queued.text)
    expect(host.textContent).toContain('Ship the top tray')
    expect(host.textContent!.indexOf(queued.text)).toBeLessThan(host.textContent!.indexOf('Ship the top tray'))
  })

  it('resets an edit across A-B-A and saves only the selected owner even when IDs match', async () => {
    const a: QueuedUserMessage = { threadId: 'queue-a', id: 'same-id', text: 'A original' }
    const b: QueuedUserMessage = { threadId: 'queue-b', id: 'same-id', text: 'B original' }
    await act(async () => { useChatStore.setState({ queuedMessages: [a, b] }) })
    expect(host.querySelectorAll('[data-queued-id]')).toHaveLength(1)
    await act(async () => { button('queuedMessageEdit').click() })
    await inputValue(host.querySelector('input')!, 'unsaved A edit')
    await act(async () => { useChatStore.setState({ activeThreadId: b.threadId }) })
    expect(host.querySelector('input')).toBeNull()
    expect(host.textContent).toContain(b.text)
    expect(host.textContent).not.toContain('unsaved A edit')
    await act(async () => { button('queuedMessageEdit').click() })
    const editor = host.querySelector('input')!
    expect(editor.value).toBe(b.text)
    await inputValue(editor, 'B saved')
    await enter(editor)
    expect(queuedMessagesForThread(useChatStore.getState(), b.threadId)[0].text).toBe('B saved')
    await act(async () => {
      actions.editQueuedMessage(a.threadId, a.id, 'stale A action')
      actions.removeQueuedMessage(a.threadId, a.id)
    })
    await expect(actions.sendQueuedMessageNow(a.threadId, a.id)).resolves.toBe(false)
    expect(queuedMessagesForThread(useChatStore.getState(), a.threadId)).toEqual([a])
    await act(async () => { useChatStore.setState({ activeThreadId: a.threadId }) })
    await act(async () => { button('queuedMessageEdit').click() })
    expect(host.querySelector('input')!.value).toBe(a.text)
  })

  it('ignores IME Enter and disables all receipt-pending controls without allowing duplicate send', async () => {
    const draft: QueuedUserMessage = { threadId: 'queue-a', id: 'draft', text: 'before IME' }
    await act(async () => { useChatStore.setState({ queuedMessages: [draft] }) })
    await act(async () => { button('queuedMessageEdit').click() })
    const editor = host.querySelector('input')!
    await inputValue(editor, 'composed draft')
    await enter(editor, { isComposing: true })
    await enter(editor, { keyCode: 229 })
    expect(queuedMessagesForThread(useChatStore.getState())[0]).toBe(draft)
    expect(host.querySelector('input')).toBe(editor)
    await enter(editor)
    expect(queuedMessagesForThread(useChatStore.getState())[0].text).toBe('composed draft')
    await act(async () => { useChatStore.setState(state => ({ queuedMessages: state.queuedMessages.map(message => ({ ...message, sending: true })), queuedMessagePauses: { 'queue-a': 'interrupted' } })) })
    const row = host.querySelector('[data-queued-id="draft"]')!
    expect(button('queuedMessageResume').disabled).toBe(true)
    for (const control of row.querySelectorAll<HTMLButtonElement>('button')) expect(control.disabled).toBe(true)
    await expect(actions.sendQueuedMessageNow(draft.threadId, draft.id)).resolves.toBe(false)
    expect(registryMock.getProvider).not.toHaveBeenCalled()
    expect(queuedMessagesForThread(useChatStore.getState())).toHaveLength(1)
  })
})
