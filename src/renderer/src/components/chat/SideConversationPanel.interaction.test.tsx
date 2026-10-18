// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { dispatchAnalytixRuntimeEvent } from '../../agent/analytix-mapper'
import type { ThreadEventSink } from '../../agent/types'
import type { SideConversation } from '../../store/chat-store-types'
import { useChatStore } from '../../store/chat-store'
import { appendActiveStreamDeltas, clearActiveStream, resetActiveStream } from '../../thread/streaming/active-stream-store'
import i18n from '../../i18n'
import { SideConversationPanel } from './SideConversationPanel'
const { detail } = vi.hoisted(() => ({ detail: vi.fn(async () => ({ authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} })) }))
vi.mock('../../agent/registry', () => ({ getProvider: () => ({ getThreadDetail: detail }) }))
vi.mock('./use-voice-dictation', async (importOriginal) => ({
  ...await importOriginal<typeof import('./use-voice-dictation')>(),
  useSpeechToTextEnabled: () => true
}))
let descriptor: PropertyDescriptor | undefined
let processApi: ReturnType<typeof makeProcessApi>
const sideScope = { threadId: 'side', turnId: 'turn-side', contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'side-incarnation' }
function sideSnapshot(preview = 'PRIVATE_SIDE_PREVIEW') {
  return { status: 'snapshot', schemaVersion: 2, revision: '1', scope: sideScope,
    units: [{ unitId: 'side-unit', producerId: 'side-producer', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }],
    blocks: [{ blockId: 'block', kind: 'native_reasoning', source: { kind: 'native', unitId: 'side-unit', producerId: 'side-producer', providerSequence: '1', attempt: 1, nativeIndex: 0, sourceOrdinal: '1' },
      nativeIndex: 0, state: 'completed', attempt: 1, preview, durationMs: 1, truncated: false, classification: 'credential_redacted' }] }
}
function makeProcessApi() {
  return { authorizeProcessPane: vi.fn(async () => ({ status: 'authorized', schemaVersion: 2,
    viewerId: '11111111-1111-4111-8111-111111111111', revision: '1', scope: sideScope, presence: { nativeBlockCount: 1, toolCaptureCount: 0 } })),
    readProcessPane: vi.fn(async (request: { draftsOnly?: boolean }): Promise<unknown> => request.draftsOnly
      ? { ...sideSnapshot(), units: [], blocks: [] } : sideSnapshot()),
    setProcessPaneExpanded: vi.fn(async () => ({ status: 'ok' })), closeProcessPane: vi.fn(async () => ({ status: 'ok' })),
    openProcessCapture: vi.fn(async () => ({ status: 'unavailable' })), onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined }
}
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>
const side = (): SideConversation => ({ threadId: 'side', parentThreadId: 'parent', title: 'Synthetic side', createdAt: '2026-10-06T00:00:00Z', inheritedAt: '2026-10-06T00:00:00Z', blocks: [{ kind: 'user', id: 'u', text: 'Synthetic request', meta: { turnId: 'turn-side' } }], liveAssistant: '', lastSeq: 1, input: 'Synthetic input', model: '', reasoningEffort: 'medium', busy: true, turnId: 'turn-side', userItemId: 'u', error: null })
async function render() { await act(async () => root.render(createElement(SideConversationPanel))) }
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} }); vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0)); vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id)); await i18n.changeLanguage('zh'); descriptor = Object.getOwnPropertyDescriptor(window, 'analytix'); processApi = makeProcessApi(); Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: processApi } }); previous = useChatStore.getState(); useChatStore.setState({ activeThreadId: 'parent', route: 'chat', workspaceRoot: '/workspace/synthetic', sidePanel: { open: true, activeSideId: 'side' }, sideConversations: { side: side() }, runtimeConnection: 'ready' }); container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); if (descriptor) Object.defineProperty(window, 'analytix', descriptor); else Reflect.deleteProperty(window, 'analytix'); clearActiveStream(); useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals() })
describe('side uses the same central composition', () => {
  it('shows the identity-scoped running estimate and keeps candidate prose hidden', async () => {
    appendActiveStreamDeltas({ threadId: 'side', turnId: 'turn-side', lastSeq: 1, startedAt: Date.now() - 12_000, runtimeRunning: true })
    await dispatchAnalytixRuntimeEvent({ kind: 'assistant_text_delta', seq: 2, timestamp: '2026-10-06T00:00:00Z', threadId: 'side', turnId: 'turn-side', text: 'PRIVATE_CANDIDATE' }, {} as ThreadEventSink, async () => {})
    await render()
    expect(container.querySelector('[data-analytix-chat-view]')).not.toBeNull()
    expect(container.querySelector(`[title="${i18n.t('common:dshRunningEstimate')}"]`)).not.toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_CANDIDATE')
    await act(async () => { clearActiveStream('side', 'turn-side'); resetActiveStream('side', { turnId: 'other-turn', startedAt: Date.now() - 12_000 }) })
    expect(container.querySelector(`[title="${i18n.t('common:dshRunningEstimate')}"]`)).toBeNull()
  })
  it('replaces the secondary composer with owner navigation without calling the primary resolver', async () => {
    const owner = vi.fn(), resolver = vi.fn()
    const value = side(); value.busy = false; value.blocks.push({ kind: 'user_input', id: 'gate', requestId: 'request', status: 'pending', questions: [{ id: 'q', header: 'Synthetic question', question: 'Synthetic question', options: [] }] })
    useChatStore.setState({ sideConversations: { side: value }, resolveUserInput: resolver })
    await act(async () => root.render(createElement(SideConversationPanel, { onOpenThreadRequest: owner })))
    expect(container.querySelector('[data-analytix-gate-composer]')!.getAttribute('data-gate-thread-id')).toBe('side')
    expect(container.querySelector('textarea')).toBeNull()
    const respond = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:respondInOwnedThread'))!
    await act(async () => respond.click()); expect(owner).toHaveBeenCalledWith('side', 'request'); expect(resolver).not.toHaveBeenCalled()
  })
})


it('automatically leases the actual side identity, clears on minimize/close and gets fresh authority on explicit restore', async () => {
  const openActualGroup = async () => {
    const group = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button[data-disclosure-row]')
    expect(group).not.toBeNull()
    if (group!.getAttribute('aria-expanded') === 'false') await act(async () => group!.click())
  }
  await render(); expect(processApi.authorizeProcessPane).toHaveBeenCalledTimes(1)
  await openActualGroup(); expect(container.textContent).toContain('PRIVATE_SIDE_PREVIEW')
  expect(processApi.authorizeProcessPane).toHaveBeenLastCalledWith(expect.objectContaining({ threadId: 'side', turnId: 'turn-side' }))
  await act(async () => container.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:sidePanelMinimize')}"]`)!.click())
  expect(container.textContent).not.toContain('PRIVATE_SIDE_PREVIEW'); expect(processApi.closeProcessPane).toHaveBeenCalled()
  await act(async () => container.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:sidePanelExpand')}"]`)!.click())
  expect(processApi.authorizeProcessPane).toHaveBeenCalledTimes(2)
  await openActualGroup(); expect(container.textContent).toContain('PRIVATE_SIDE_PREVIEW')
  await act(async () => useChatStore.setState({ activeThreadId: 'other-parent' }))
  expect(container.textContent).not.toContain('PRIVATE_SIDE_PREVIEW')
  await act(async () => useChatStore.setState({ activeThreadId: 'parent' }))
  expect(processApi.authorizeProcessPane).toHaveBeenCalledTimes(3)
  await act(async () => container.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:sidePanelHide')}"]`)!.click())
  expect(container.querySelector('[data-process-surface="side"]')).toBeNull(); expect(processApi.closeProcessPane).toHaveBeenCalledTimes(3)
})
it('ignores a late side response after its selection is retired', async () => {
  let release: (value: unknown) => void = () => undefined
  processApi.readProcessPane.mockImplementation(() => new Promise(resolve => { release = resolve }))
  await render()
  await act(async () => useChatStore.setState({ activeThreadId: 'other-parent' }))
  await act(async () => release(sideSnapshot('PRIVATE_LATE_SIDE')))
  expect(container.querySelector('[data-process-surface="side"]')).toBeNull()
  expect(container.textContent).not.toContain('PRIVATE_SIDE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_LATE_SIDE')
})
it('opens the existing floating draft without creating a thread or sending anything on open/minimize/restore', async () => {
  const spawn = vi.fn(async () => null), send = vi.fn(async () => true)
  useChatStore.setState({ sidePanel: { open: false, activeSideId: null }, sideConversations: {}, spawnSideConversation: spawn, sendSideMessage: send })
  await render(); expect(container.querySelector('aside')).toBeNull()
  await act(async () => useChatStore.getState().openSideConversationDraft())
  expect(container.querySelector('[data-composer-prompt-editor] .ProseMirror')).not.toBeNull(); expect(container.textContent).toContain(i18n.t('common:sidePanelDraftEmpty'))
  await act(async () => container.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:sidePanelMinimize')}"]`)!.click())
  await act(async () => container.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:sidePanelExpand')}"]`)!.click())
  expect(spawn).not.toHaveBeenCalled(); expect(send).not.toHaveBeenCalled(); expect(processApi.authorizeProcessPane).not.toHaveBeenCalled()
})

it('submits the visible normal floating draft once through the existing action and ignores IME/repeat Enter', async () => {
  const spawn = vi.fn(async () => null), send = vi.fn(async () => true)
  useChatStore.setState({ sidePanel: { open: true, activeSideId: null }, sideConversations: {}, spawnSideConversation: spawn, sendSideMessage: send })
  await render(); const input = container.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
  await act(async () => {
    const paragraph = document.createElement('p'); paragraph.textContent = '  Synthetic draft submit  '
    input.replaceChildren(paragraph)
    await new Promise(resolve => window.setTimeout(resolve, 0))
  })
  for (const options of [{ isComposing: true }, { keyCode: 229 }, { repeat: true }]) {
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true, ...options })))
    expect(spawn).not.toHaveBeenCalled()
  }
  await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
  expect(spawn).toHaveBeenCalledExactlyOnceWith('Synthetic draft submit')
  expect(send).not.toHaveBeenCalled(); expect(processApi.authorizeProcessPane).not.toHaveBeenCalled()
  expect(input.textContent).toBe('')
})


it('does not substitute primary latest usage or invented zeros for missing side whole-log groups', async () => {
  const value = side(); value.busy = false
  await act(async () => useChatStore.setState({ sideConversations: { side: value },
    lastTurnUsage: { threadId: 'parent', snapshot: { inputTokens: 100, outputTokens: 900, totalTokens: 1000,
      reasoningTokens: 0, cachedTokens: 50, cacheMissTokens: 50, cacheHitRate: 0.5,
      costUsd: null, costCny: null, priceConfigured: false, tokenEconomySavingsTokens: 0, turns: 1 } } }))
  await render()
  expect(container.querySelector('footer [data-composer-prompt-editor] .ProseMirror')?.textContent).toBe('Synthetic input')
  expect(container.querySelector('footer [data-composer-stat]')).toBeNull()
  expect(container.querySelector('footer .ds-composer-usage')).toBeNull()
})


let rangeGeometry: Array<PropertyDescriptor | undefined>
beforeEach(() => {
  rangeGeometry = ['getClientRects', 'getBoundingClientRect'].map(name => Object.getOwnPropertyDescriptor(Range.prototype, name))
  Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
  Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
})
afterEach(() => ['getClientRects', 'getBoundingClientRect'].forEach((name, index) => {
  const descriptor = rangeGeometry[index]
  if (descriptor) Object.defineProperty(Range.prototype, name, descriptor)
  else Reflect.deleteProperty(Range.prototype, name)
}))
it('keeps active side input, literal slash submission and Stop bound to that side only', async () => {
  const send = vi.fn(async () => true), stop = vi.fn(async () => undefined), primaryCompact = vi.fn(async () => undefined)
  const value = side(); value.busy = false; value.input = '/compact literal side message'
  useChatStore.setState({ sideConversations: { side: value }, sendSideMessage: send, interruptSide: stop, compactActiveThread: primaryCompact })
  await render()
  const editor = () => container.querySelector<HTMLElement>('footer [data-composer-prompt-editor] .ProseMirror')!
  expect(container.querySelector('footer [data-analytix-input-bar]')).not.toBeNull()
  expect(container.querySelector(`footer button[aria-label="${i18n.t('common:composerVoiceStart')}"]`)).toBeNull()
  expect(container.textContent).not.toContain(i18n.t('common:slashCommandMenuTitle'))
  await act(async () => editor().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
  expect(useChatStore.getState().sidePanel.open).toBe(false)
  expect(useChatStore.getState().sideConversations.side.input).toBe('/compact literal side message')
  expect(send).not.toHaveBeenCalled()
  await act(async () => useChatStore.getState().setSidePanelOpen(true))
  expect(editor().textContent).toBe('/compact literal side message')
  await act(async () => editor().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
  expect(send).toHaveBeenCalledExactlyOnceWith('side', '/compact literal side message')
  expect(primaryCompact).not.toHaveBeenCalled(); expect(stop).not.toHaveBeenCalled()
  await act(async () => useChatStore.setState(state => ({ sideConversations: { side: { ...state.sideConversations.side, busy: true } } })))
  await act(async () => editor().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
  expect(send).toHaveBeenCalledOnce(); expect(stop).not.toHaveBeenCalled()
  await act(async () => container.querySelector<HTMLButtonElement>(`footer button[aria-label="${i18n.t('common:interrupt')}"]`)!.click())
  expect(stop).toHaveBeenCalledExactlyOnceWith('side')
  expect(useChatStore.getState().activeThreadId).toBe('parent')
})


it('keeps the resident editor and side drafts separate when selecting a different side', async () => {
  const send = vi.fn(async () => true)
  const a = side(); a.busy = false; a.input = 'Side A input'
  const b = { ...a, threadId: 'side-b', title: 'Side B', input: 'Side B input' }
  useChatStore.setState({ sideConversations: { side: a, 'side-b': b }, sendSideMessage: send })
  await render()
  const editor = container.querySelector<HTMLElement>('footer [data-composer-prompt-editor] .ProseMirror')!
  expect(editor.textContent).toBe('Side A input')
  await act(async () => useChatStore.setState({ sidePanel: { open: true, activeSideId: 'side-b' } }))
  expect(container.querySelector('footer [data-composer-prompt-editor] .ProseMirror')).toBe(editor)
  expect(editor.textContent).toBe('Side B input')
  await act(async () => editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
  expect(send).toHaveBeenCalledExactlyOnceWith('side-b', 'Side B input')
  expect(useChatStore.getState().sideConversations.side.input).toBe('Side A input')
  expect(useChatStore.getState().activeThreadId).toBe('parent')
})


it('keeps the leased side draft and unsent input through tool interleaving and Stop until a bound terminal replaces the answer once', async () => {
  const expanded = new Set<string>(), stop = vi.fn(async () => undefined)
  processApi.authorizeProcessPane.mockImplementation(async () => ({ status: 'authorized', schemaVersion: 2,
    viewerId: '11111111-1111-4111-8111-111111111111', revision: '1', scope: sideScope,
    presence: { nativeBlockCount: 0, toolCaptureCount: 0, assistantDraftCount: 1 } }))
  processApi.setProcessPaneExpanded.mockImplementation(async (...args: unknown[]) => {
    const request = args[0]
    expanded.clear(); for (const id of (request as { expandedBlockIds: string[] }).expandedBlockIds) expanded.add(id)
    return { status: 'ok' }
  })
  processApi.readProcessPane.mockImplementation(async () => ({ status: 'snapshot', schemaVersion: 2, revision: '2', scope: sideScope,
    units: [{ unitId: 'side-unit', producerId: 'side-producer', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }],
    blocks: [{ blockId: 'side-draft', kind: 'assistant_draft', state: 'running', attempt: 1, durationMs: 0,
      truncated: false, classification: 'source_exact_local', contentClassification: 'provider_text',
      source: { kind: 'assistant_draft', unitId: 'side-unit', producerId: 'side-producer', providerSequence: '1', attempt: 1 },
      ...(expanded.has('side-draft') ? { draftSegments: [{ offsetBytes: '0', text: 'SIDE_LOCAL_DRAFT' }], draftBytes: '16' } : {}) }] }))
  const value = side(); value.input = 'UNSENT_SIDE_INPUT'
  useChatStore.setState({ sideConversations: { side: value }, interruptSide: stop })
  await render()
  const seat = container.querySelector('[data-analytix-assistant-response-seat]')!
  expect(seat.textContent!.match(/SIDE_LOCAL_DRAFT/g)).toHaveLength(1)
  await act(async () => useChatStore.setState(state => ({ sideConversations: { side: { ...state.sideConversations.side,
    blocks: [...value.blocks, { kind: 'tool', id: 'side-tool', status: 'success', toolKind: 'tool_call', summary: 'Actual tool',
      meta: { toolName: 'read', turnId: 'turn-side', callId: 'side-call' } }] } } })))
  expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
  expect(seat.textContent!.match(/SIDE_LOCAL_DRAFT/g)).toHaveLength(1)
  await act(async () => container.querySelector<HTMLButtonElement>(`footer button[aria-label="${i18n.t('common:interrupt')}"]`)!.click())
  expect(stop).toHaveBeenCalledExactlyOnceWith('side')
  expect(useChatStore.getState().sideConversations.side.input).toBe('UNSENT_SIDE_INPUT')
  expect(seat.textContent!.match(/SIDE_LOCAL_DRAFT/g)).toHaveLength(1)
  await act(async () => useChatStore.setState(state => ({ sideConversations: { side: { ...state.sideConversations.side, busy: false, turnId: null,
    blocks: [...state.sideConversations.side.blocks, { kind: 'assistant', id: 'actual-side-final', text: 'SIDE_BOUND_FINAL', meta: { turnId: 'turn-side' },
      assistantItemIdentity: { threadId: 'side', turnId: 'turn-side', itemId: 'actual-side-final' },
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'side', turnId: 'turn-side',
        itemId: 'actual-side-final', firstSeq: 10, lastSeq: 12, status: 'completed' } }] } } })))
  expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
  expect(seat.textContent).not.toContain('SIDE_LOCAL_DRAFT')
  expect(seat.textContent!.match(/SIDE_BOUND_FINAL/g)).toHaveLength(1)
  expect(container.querySelector('footer [data-composer-prompt-editor] .ProseMirror')?.textContent).toBe('UNSENT_SIDE_INPUT')
})
