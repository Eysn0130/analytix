// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { processDisplaySnapshotSchemaV1 } from '../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import { toolResultLocalDisplaySchemaV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ProcessDisplayBlockV1, ProcessDisplayUnitV2, ProcessPaneAuthorizeRequestV1, ProcessPaneReadRequestV1 } from '../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { AnalytixApi } from '../../../../../../shared/analytix-api'
import { createRuntimeStatusPublicV1 } from '../../../../../../shared/analytix-runtime-status'
import type { CoreTurnItemJson } from '../../../../agent/analytix-contract'
import { chatBlockFromItem } from '../../../../agent/analytix-mapper'
import type { ChatBlock } from '../../../../agent/types'
import i18n from '../../../../i18n'
import { ProtectedToolResult, ProtectedToolResultScope } from '../ProtectedToolResult'
import { ProtectedProcessPane, type ProcessPaneRoute } from './ProtectedProcessPane'
import { QuestionComposer } from './QuestionComposer'
import { presentationPolicyFor } from './chat-view-model'
import { useChatStore } from '../../../../store/chat-store'
const facadeTransport = vi.hoisted(() => ({
  expose: vi.fn<(name: string, value: AnalytixApi) => void>(),
  invoke: vi.fn<(channel: string, request?: unknown) => Promise<unknown>>(),
  listeners: new Map<string, Set<(event: unknown, value: unknown) => void>>(),
  mainHandlers: new Map<string, (event: unknown, payload?: unknown) => unknown>()
}))
vi.mock('electron', () => ({
  ipcMain: { handle: (channel: string, handler: (event: unknown, payload?: unknown) => unknown) => facadeTransport.mainHandlers.set(channel, handler) },
  contextBridge: { exposeInMainWorld: facadeTransport.expose },
  ipcRenderer: {
    invoke: facadeTransport.invoke,
    on: (channel: string, handler: (event: unknown, value: unknown) => void) => {
      const handlers = facadeTransport.listeners.get(channel) ?? new Set()
      handlers.add(handler); facadeTransport.listeners.set(channel, handlers)
    },
    removeListener: (channel: string, handler: (event: unknown, value: unknown) => void) => {
      facadeTransport.listeners.get(channel)?.delete(handler)
    }
  },
  webUtils: { getPathForFile: () => '' }
}))
let facadeCleanups: Array<() => void> = []
const finalization = vi.hoisted(() => vi.fn(() => null))
vi.mock('../../AssistantMarkdown', () => ({ AssistantMarkdown: finalization }))
let root: Root, container: HTMLDivElement, previous: PropertyDescriptor | undefined
let api: ReturnType<typeof makeApi>, invalidators: Map<string, () => void>, runtimeInvalidators: Set<(status: { state: string }) => void>
const viewer = '11111111-1111-4111-8111-111111111111'
const scope = (threadId = 'thread', turnId = 'turn') => ({ threadId, turnId, contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'incarnation-a' })
const anchor = (suffix = 'a') => ({ callId: `call-${suffix}`, toolCallItemId: `call-item-${suffix}`, toolResultItemId: `result-item-${suffix}`, startedEventSeq: '10' })
const unit = (calls: ProcessDisplayUnitV2['calls'] = []): ProcessDisplayUnitV2 => ({ unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', placement: calls.length ? 'anchored' : 'unit_only', calls, precedingCalls: [] })
const block = (body = false, kind: ProcessDisplayBlockV1['kind'] = 'native_reasoning', suffix = 'a'): ProcessDisplayBlockV1 => ({
  blockId: `block-${suffix}`, kind, state: 'completed', attempt: 1,
  source: kind === 'native_reasoning' ? { kind: 'native', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt: 1, nativeIndex: 0, sourceOrdinal: '1' }
    : { kind: 'tool', unitId: 'unit-a', ...anchor(suffix) },
  ...(kind === 'native_reasoning' ? { nativeIndex: 0 } : {}), title: 'SAFE_PRIVATE_TITLE', preview: 'SAFE_PRIVATE_PREVIEW',
  durationMs: 123, captureDigest: 'b'.repeat(64), truncated: false, classification: 'credential_redacted',
  ...(body ? { body: { text: '**PRIVATE_BODY**\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n![ALT](https://private.invalid/image)', before: 'PRIVATE_BEFORE', after: 'PRIVATE_AFTER', error: '', addedLines: 2, removedLines: 1, approximate: false } } : {})
})
const snapshot = (body = false, threadId = 'thread', turnId = 'turn', revision = '18446744073709551614') => ({
  status: 'snapshot' as const, schemaVersion: 2, revision, scope: scope(threadId, turnId), units: [unit()], blocks: [block(body)]
})
const publicTool = (suffix = 'a'): Extract<ChatBlock, { kind: 'tool' }> => ({ kind: 'tool', id: `public-${suffix}`, status: 'success', summary: 'same title', detail: 'public detail',
  meta: { toolName: 'edit', turnId: 'turn', callId: anchor(suffix).callId, sourceItemId: anchor(suffix).toolResultItemId } })
function makeApi() {
  let count = 0
  const viewers = [viewer, '22222222-2222-4222-8222-222222222222', '33333333-3333-4333-8333-333333333333']
  return {
    authorizeProcessPane: vi.fn(async (request: ProcessPaneAuthorizeRequestV1): Promise<unknown> => ({ status: 'authorized', schemaVersion: 2,
      viewerId: viewers[count++] ?? crypto.randomUUID(), revision: '18446744073709551614', scope: scope(request.threadId, request.turnId), presence: { nativeBlockCount: 1, toolCaptureCount: 0 } })),
    readProcessPane: vi.fn(async (_request: ProcessPaneReadRequestV1): Promise<unknown> => snapshot()),
    setProcessPaneExpanded: vi.fn(async (_request: unknown): Promise<unknown> => ({ status: 'ok' })),
    closeProcessPane: vi.fn(async (_request: unknown): Promise<unknown> => ({ status: 'ok' })),
    openProcessCapture: vi.fn(async (_request: unknown): Promise<unknown> => ({ status: 'unavailable' })),
    openProcessWebLink: vi.fn(async (_request: unknown): Promise<unknown> => ({ status: 'ok' })),
    onProcessPaneInvalidated: vi.fn((request: { paneId: string }, handler: () => void) => { invalidators.set(request.paneId, handler); return () => { invalidators.delete(request.paneId) } }),
    onRuntimeStatus: vi.fn((handler: (status: { state: string }) => void) => { runtimeInvalidators.add(handler); return () => { runtimeInvalidators.delete(handler) } })
  }
}
const route = (surface: ProcessPaneRoute['surface'] = 'main', visible = true): ProcessPaneRoute => ({ surface, ownerKey: surface, selectionKey: 'selection', visible })
async function render(props: Partial<Parameters<typeof ProtectedProcessPane>[0]> = {}, toolScope?: string) {
  const pane = createElement(ProtectedProcessPane, { route: route(), threadId: 'thread', turnId: 'turn', processOpen: true, processing: true, runtimeAvailable: true, ...props })
  await act(async () => root.render(toolScope ? createElement(ProtectedToolResultScope.Provider, { value: toolScope }, pane) : pane))
}
async function toggle(scope: ParentNode = container) {
  const row = scope.querySelector<HTMLElement>('[data-protected-process-block] [data-disclosure-row]')
  expect(row).not.toBeNull(); await act(async () => row!.click())
}
beforeEach(async () => {
  facadeCleanups = []
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.useFakeTimers(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  await i18n.changeLanguage('zh'); invalidators = new Map(); runtimeInvalidators = new Set(); api = makeApi(); finalization.mockClear()
  previous = Object.getOwnPropertyDescriptor(window, 'analytix')
  // Model the Main-owned source-only projection for tests using the short fake
  // facade. Actual preload tests below independently exercise its strict fence.
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: Object.assign(Object.create(api), {
    readProcessPane: async (request: ProcessPaneReadRequestV1) => {
      const result = await api.readProcessPane(request)
      if (!request.draftsOnly || !result || typeof result !== 'object' || !('status' in result) || result.status !== 'snapshot') return result
      const value = result as ReturnType<typeof snapshot>
      const blocks = value.blocks.filter(block => block.source.kind === 'assistant_draft')
      return { ...value, blocks, units: value.units.filter(unit => blocks.some(block => block.source.unitId === unit.unitId)) }
    } }) } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})

describe('component-local protected assistant revisions', () => {
  const draft = (parts?: string[], attempt = 1, blockId = 'draft-' + attempt): ProcessDisplayBlockV1 => {
    let bytes = 0
    const segments = parts?.map(text => { const offsetBytes = String(bytes); bytes += new TextEncoder().encode(text).length; return { offsetBytes, text } })
    return { blockId, kind: 'assistant_draft', state: 'running', attempt,
      source: { kind: 'assistant_draft', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt },
      contentClassification: 'unclassified_inline', classification: 'source_exact_local', durationMs: 0, truncated: false,
      ...(segments ? { draftSegments: segments, draftBytes: String(bytes) } : {}) }
  }
  let revision: number, current: ProcessDisplayBlockV1[], expanded: Set<string>
  beforeEach(() => {
    revision = 2; current = [draft(['<think>PRIVATE_FIRST_DRAFT</think> '])]; expanded = new Set()
    api.authorizeProcessPane.mockImplementation(async request => ({ status: 'authorized', schemaVersion: 2, viewerId: viewer, revision: '1',
      scope: scope(request.threadId, request.turnId), presence: { nativeBlockCount: 0, toolCaptureCount: 0, assistantDraftCount: 1 } }))
    api.setProcessPaneExpanded.mockImplementation(async request => {
      expanded = new Set((request as { expandedBlockIds: string[] }).expandedBlockIds); return { status: 'ok' }
    })
    api.readProcessPane.mockImplementation(async () => ({ ...snapshot(), revision: String(revision), blocks: current.map(block => {
      if (expanded.has(block.blockId)) return block
      const { draftSegments: _segments, draftBytes: _bytes, ...metadata } = block; return metadata
    }) }))
  })
  it('replaces multiple before-end revisions in the same real unit seat without public storage or finalization', async () => {
    const storeBefore = useChatStore.getState().blocks
    await render({ policy: presentationPolicyFor('compact') })
    const seat = container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')!
    expect(seat.textContent).toContain('<think>PRIVATE_FIRST_DRAFT</think>')
    expect(container.querySelector('[data-disclosure-row]')).toBeNull()
    expect(expanded).toEqual(new Set(['draft-1']))
    revision++; current = [draft(['PRIVATE_REPLACEMENT_DRAFT '])]
    await act(async () => vi.advanceTimersByTime(500))
    expect(container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')).toBe(seat)
    expect(seat.textContent).toContain('PRIVATE_REPLACEMENT_DRAFT')
    expect(seat.textContent).not.toContain('PRIVATE_FIRST_DRAFT')
    expect(finalization).not.toHaveBeenCalled()
    expect(useChatStore.getState().blocks).toBe(storeBefore)
    expect(useChatStore.getState().liveAssistant).not.toContain('PRIVATE_REPLACEMENT_DRAFT')
  })
  it('renders real local draft revisions as inert streaming Markdown and retires them only at the authorized terminal', async () => {
    const storeBefore = useChatStore.getState().blocks
    const text = '**PRIVATE_BOLD**\n\n| A | B |\n| - | - |\n| local | cell |\n\n$x$\n\n<think>literal source only</think>\n\n[External](https://private.invalid) ![ALT](https://private.invalid/image.png)'
    current = [draft([text])]
    await render()
    const seat = container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')!
    const bold = seat.querySelector('strong')
    expect(bold?.textContent).toBe('PRIVATE_BOLD')
    expect(seat.querySelector('table td')?.textContent).toBe('local')
    expect(seat.textContent).toContain('$x$'); expect(seat.querySelector('.katex')).toBeNull()
    expect(seat.textContent).toContain('<think>literal source only</think>')
    expect(seat.querySelector('a,img,button,think,[data-reasoning-id]')).toBeNull()
    revision++; current = [draft([text, '\n\nLAST_DRAFT_BLOCK'])]
    await act(async () => vi.advanceTimersByTimeAsync(500))
    expect(container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')).toBe(seat)
    expect(seat.querySelector('strong')).toBe(bold)
    expect(seat.textContent).toContain('LAST_DRAFT_BLOCK')
    expect(useChatStore.getState().blocks).toBe(storeBefore); expect(finalization).not.toHaveBeenCalled()
    await render({ processing: false, terminalAuthorized: true, answer: createElement('p', null, 'APPROVED_FINAL') })
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_BOLD'); expect(container.textContent).toContain('APPROVED_FINAL')
    expect(useChatStore.getState().blocks).toBe(storeBefore)
  })
  it.each(['standard', 'compact'] as const)('keeps late draft discovery alive beside public progress and folded reasoning in %s', async mode => {
    const storeBefore = useChatStore.getState().blocks
    const reasoning: ProcessDisplayBlockV1 = {
      blockId: 'provider-reasoning', kind: 'native_reasoning', state: 'running', attempt: 1,
      source: { kind: 'provider_reasoning', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt: 1, sourceOrdinal: '1' },
      title: 'PRIVATE_REASONING_SENTINEL', preview: 'PRIVATE_REASONING_SENTINEL', durationMs: 1,
      truncated: false, classification: 'credential_redacted'
    }
    const progressMeta = { turnId: 'turn', stage: 'post_send' as const }
    const processBlocks: ChatBlock[] = [{ kind: 'system', id: 'runtime_status_turn_provider_progress', text: 'Provider request started', meta: progressMeta }]
    const props = { policy: presentationPolicyFor(mode), processBlocks }
    revision = 1; current = [reasoning]
    api.authorizeProcessPane.mockImplementation(async request => ({ status: 'authorized', schemaVersion: 2, viewerId: viewer, revision: '1',
      scope: scope(request.threadId, request.turnId), presence: { nativeBlockCount: 1, toolCaptureCount: 0, assistantDraftCount: 0 } }))
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot(), revision: String(revision), blocks: current }).success).toBe(true)
    await render(props)
    const initialReads = api.readProcessPane.mock.calls.length
    await act(async () => vi.advanceTimersByTimeAsync(1800))
    const completed = { ...reasoning, state: 'completed' as const, captureDigest: 'd'.repeat(64) }
    revision++; current = [completed, draft(['**LATE_PRIVATE_DRAFT**\n\n| A | B |\n| - | - |\n| local | cell |'])]
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot(), revision: String(revision), blocks: current }).success).toBe(true)
    await act(async () => vi.advanceTimersByTimeAsync(500))
    const seat = container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')
    expect(seat).not.toBeNull()
    expect(seat?.querySelector('strong')?.textContent).toBe('LATE_PRIVATE_DRAFT')
    expect(seat?.querySelector('table td')?.textContent).toBe('local')
    expect(expanded).toEqual(new Set(['draft-1']))
    expect(container.textContent).not.toContain('PRIVATE_REASONING_SENTINEL')
    expect(container.textContent).not.toContain(i18n.t('common:dshProcessUnavailable'))
    revision++; current = [completed, draft(['**LATE_PRIVATE_DRAFT**\n\nSECOND_LIVE_REVISION'])]
    await act(async () => vi.advanceTimersByTimeAsync(4500))
    expect(container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')).toBe(seat)
    expect(seat?.textContent).toContain('SECOND_LIVE_REVISION')
    const subsequentReads = api.readProcessPane.mock.calls.slice(initialReads)
    expect(subsequentReads.length).toBeGreaterThan(1)
    expect(subsequentReads.every(([request]) => request.draftsOnly === true)).toBe(true)
    const foldedHeader = container.querySelector('[data-process-group] > div > [data-disclosure-row]')
    expect(foldedHeader?.textContent).toContain(i18n.t('common:dshTurnProcess'))
    expect(foldedHeader?.querySelector('[data-shimmer]')).toBeNull()
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(useChatStore.getState().blocks).toBe(storeBefore)
    expect(finalization).not.toHaveBeenCalled()
    await render({ ...props, processing: false, terminalAuthorized: true, answer: createElement('p', null, 'AUTHORIZED_FINAL') })
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
    expect(container.textContent).toContain('AUTHORIZED_FINAL')
    expect(container.textContent).not.toContain('SECOND_LIVE_REVISION')
  })
  it('shows every projected UTF-8 segment beyond 32KiB with literal unclassified markup', async () => {
    const first = '中😀'.repeat(4681), tail = '<think>完整末尾</think> END'
    current = [draft([first, tail])]
    await render()
    expect(container.querySelector('[data-protected-assistant-draft] [data-protected-process-body]')?.textContent).toBe(first + tail)
    expect(container.querySelector('iframe,img,a')).toBeNull()
  })
  it('clears an old attempt and its selection while preserving the logical seat, and denies later attempt rollback', async () => {
    await render()
    const seat = container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')!
    const range = document.createRange(); range.selectNodeContents(container.querySelector('[data-protected-process-body]')!); document.getSelection()!.addRange(range)
    const old = { ...draft(), state: 'superseded' as const, classification: 'withheld' as const }
    revision++; current = [old, draft(['NEW_ATTEMPT_ONLY '], 2)]
    await act(async () => vi.advanceTimersByTime(500))
    expect(container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')).toBe(seat)
    expect(seat.textContent).toContain('NEW_ATTEMPT_ONLY'); expect(seat.textContent).not.toContain('PRIVATE_FIRST_DRAFT')
    expect(document.getSelection()?.rangeCount).toBe(0)
    revision++; current = [draft(['LATE_OLD_ATTEMPT '])]
    await act(async () => vi.advanceTimersByTime(500))
    expect(container.textContent).not.toContain('LATE_OLD_ATTEMPT')
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
  })
  it('retires selection and late reply on revoke while sibling composer draft survives', async () => {
    await render()
    const composer = document.createElement('textarea'); composer.value = 'USER_INPUT_DRAFT'; document.body.append(composer)
    const range = document.createRange(); range.selectNodeContents(container.querySelector('[data-protected-process-body]')!); document.getSelection()!.addRange(range)
    let finish!: (value: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => vi.advanceTimersByTime(500))
    await act(async () => [...invalidators.values()][0]())
    await act(async () => finish({ ...snapshot(), revision: String(++revision), blocks: [draft(['LATE_REVOKED_DRAFT '])] }))
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
    expect(container.textContent).not.toContain('LATE_REVOKED_DRAFT')
    expect(document.getSelection()?.rangeCount).toBe(0); expect(composer.value).toBe('USER_INPUT_DRAFT'); composer.remove()
  })
  it('takes over the same response slot only on a terminal authorization and never restores draft on a late poll', async () => {
    await render({ answer: createElement('p', {}, 'UNCOMMITTED_PUBLIC_PROSE') })
    const seat = container.querySelector('[data-analytix-assistant-response-seat]')!
    expect(seat.textContent).not.toContain('UNCOMMITTED_PUBLIC_PROSE')
    await render({ terminalAuthorized: true, processing: false, answer: createElement('p', {}, 'AUTHORIZED_TERMINAL_FINAL') })
    expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
    expect(seat.textContent).toBe('AUTHORIZED_TERMINAL_FINAL')
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
    revision++; current = [draft(['LATE_POST_TERMINAL '])]
    await render({ terminalAuthorized: false, processing: true, answer: createElement('p', {}, 'AUTHORIZED_TERMINAL_FINAL') })
    await act(async () => vi.advanceTimersByTime(500))
    expect(seat.textContent).not.toContain('LATE_POST_TERMINAL')
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
  })
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges()
  for (const cleanup of facadeCleanups) cleanup()
  facadeTransport.listeners.clear()
  facadeTransport.mainHandlers.clear()
  if (previous) Object.defineProperty(window, 'analytix', previous); else Reflect.deleteProperty(window, 'analytix')
  delete document.documentElement.dataset.bridge; vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
describe('V2 mounted protected inline process', () => {
  it.each(['toolResultItemId', 'startedEventSeq'] as const)('does not let partial preceding anchors erase a known %s before a conflict', async field => {
    const full = anchor(), partial = { callId: full.callId, toolCallItemId: full.toolCallItemId }
    const preceding = { ...unit(), unitId: 'unit-b', providerSequence: '2', placement: 'anchored' as const, precedingCalls: [partial] }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([full]), preceding] })
    await render({ processBlocks: [publicTool()], policy: presentationPolicyFor('verbose') })
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    const conflicting = { ...preceding, unitId: 'unit-c', providerSequence: '3', precedingCalls: [{ ...full, [field]: field === 'startedEventSeq' ? '11' : 'different-result' }] }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [preceding, conflicting], blocks: [] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(api.closeProcessPane).toHaveBeenCalled()
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    expect(container.textContent).toContain(i18n.t('common:dshProcessUnavailable'))
  })

  it('retires a late hidden identity even when the visible IDs are unchanged and the producer settles', async () => {
    const otherUnit = { ...unit([anchor('b')]), unitId: 'unit-b', providerSequence: '2' }
    const a = { ...block(), preview: 'VISIBLE_A_PREVIEW' }
    const b = { ...block(false, 'edit', 'b'), title: 'LATE_HIDDEN_B_TITLE', preview: 'LATE_HIDDEN_B_PREVIEW', path: '/synthetic/late-hidden-b',
      source: { kind: 'tool' as const, unitId: otherUnit.unitId, ...anchor('b') } }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit()], blocks: [a] })
    const props = { policy: presentationPolicyFor('standard'), processBlocks: [publicTool('b')] }
    await render(props)
    await act(async () => container.querySelector<HTMLElement>('[data-process-group="unit:unit-a"] > div:first-child button')!.click())
    expect(container.textContent).toContain('VISIBLE_A_PREVIEW')
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit(), otherUnit], blocks: [a, b] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    await render({ ...props, processing: false })
    expect(container.textContent).not.toContain('LATE_HIDDEN_B')
    let finish!: (reply: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render({ ...props, processing: false, policy: presentationPolicyFor('verbose') })
    expect(container.textContent).not.toContain('LATE_HIDDEN_B')
    expect(container.textContent).not.toContain('/synthetic/late-hidden-b')
    await act(async () => finish({ ...snapshot(), units: [unit(), otherUnit], blocks: [a, b] }))
    expect(container.textContent).toContain('LATE_HIDDEN_B_PREVIEW')
  })
  it('clears native parent pins on a fresh generation with a reused block ID and another proven owner', async () => {
    const processBlocks: ChatBlock[] = [publicTool(), { kind: 'assistant', id: 'public-separator', text: 'Public note' }, publicTool('b')]
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])] })
    await render({ processBlocks, policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-chat-seat="block-a"]')?.closest('[data-process-group]')?.getAttribute('data-process-group')).toBe('public:public-a')
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor('b')])] })
    await render({ processBlocks, policy: presentationPolicyFor('verbose'), route: { ...route(), selectionKey: 'fresh-selection' } })
    expect(container.querySelector('[data-chat-seat="block-a"]')?.closest('[data-process-group]')?.getAttribute('data-process-group')).toBe('public:public-b')
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
  })
  it('keeps the native and public parents stable when a real call prefix grows across public sections', async () => {
    const processBlocks: ChatBlock[] = [publicTool(), { kind: 'assistant', id: 'public-separator', text: 'Public note' }, publicTool('b')]
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor('b')])] })
    await render({ processBlocks, policy: presentationPolicyFor('verbose') })
    const native = container.querySelector('[data-chat-seat="block-a"]')!, parent = native.parentElement
    const publicOwner = container.querySelector('[data-analytix-process-block-id="public-b"]')!, publicParent = publicOwner.parentElement
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor('b'), anchor()])] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(native)
    expect(native.parentElement).toBe(parent)
    expect(publicOwner.parentElement).toBe(publicParent)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
  })

  it.each(['compact', 'standard', 'detailed', 'verbose'] as const)('keeps errors independent of real native units and whole folding in %s mode', async mode => {
    const error: ChatBlock = { kind: 'system', id: 'error-seat', severity: 'error', text: 'Public failure', detail: 'Safe diagnostic' }
    const processBlocks = [publicTool(), error, publicTool('b')]
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor(), anchor('b')])] })
    await render({ processBlocks, policy: presentationPolicyFor('verbose') })
    const errorSeat = container.querySelector<HTMLElement>('[data-chat-seat="error-seat"]')!
    await render({ processBlocks, policy: presentationPolicyFor(mode), processing: false })
    expect(container.querySelector('[data-chat-seat="error-seat"]')).toBe(errorSeat)
    expect(errorSeat.closest('[hidden]')).toBeNull()
    expect(errorSeat.closest('[data-process-group]')?.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-chat-seat="public-a"]')).toHaveLength(1)
    if (mode !== 'verbose') {
      const normalReads = api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
      await act(async () => vi.advanceTimersByTimeAsync(1500))
      expect(api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly)).toHaveLength(normalReads)
    }
    await render({ processBlocks, policy: presentationPolicyFor(mode), processing: false, processOpen: false })
    expect(errorSeat.closest('[hidden]')).toBeNull()
    expect(container.querySelector('[data-chat-seat="public-a"]')?.closest('[hidden]')).not.toBeNull()
    await act(async () => errorSeat.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
    expect(errorSeat.textContent).toContain('Safe diagnostic')
  })

  it('keeps real retry and error seats outside folded native groups without resuming ordinary polling', async () => {
    const retry: ChatBlock = { kind: 'system', id: 'retry-seat', text: 'Retrying', meta: { providerRetry: { attempt: 2, maxAttempt: 3 } } }
    const error: ChatBlock = { kind: 'system', id: 'error-seat', severity: 'error', text: 'Public failure' }
    const processBlocks = [publicTool(), retry, publicTool('b'), error]
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor(), anchor('b')])] })
    await render({ processBlocks, policy: presentationPolicyFor('verbose') })
    await render({ processBlocks, policy: presentationPolicyFor('compact') })
    const retrySeat = container.querySelector('[data-chat-seat="retry-seat"]')!
    expect(retrySeat.closest('[hidden]')).toBeNull()
    expect(retrySeat.closest('[data-process-group]')?.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
    const normalReads = api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly)).toHaveLength(normalReads)
    expect(api.readProcessPane.mock.calls.at(-1)?.[0].draftsOnly).toBe(true)
    await render({ processBlocks, policy: presentationPolicyFor('compact'), processOpen: false })
    expect(retrySeat.closest('[hidden]')).not.toBeNull()
    expect(container.querySelector('[data-chat-seat="error-seat"]')?.closest('[hidden]')).toBeNull()
  })

  it('discovers only draft sources through a folded public group and renews native/tool discovery only on actual group open', async () => {
    const ordinaryReads = () => api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
    api.authorizeProcessPane.mockResolvedValue({ status: 'authorized', schemaVersion: 2, viewerId: viewer,
      revision: '0', scope: scope(), presence: { nativeBlockCount: 0, toolCaptureCount: 0 } })
    api.readProcessPane.mockResolvedValue({ ...snapshot(false, 'thread', 'turn', '1'), units: [], blocks: [] })
    const processBlocks = [publicTool(), publicTool('b')]
    await render({ processBlocks, policy: presentationPolicyFor('compact') })
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(ordinaryReads()).toBe(0)
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(0)
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await render({ processBlocks, policy: presentationPolicyFor('verbose') })
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    expect(ordinaryReads()).toBe(0)
    await render({ processBlocks, policy: presentationPolicyFor('compact') })
    const group = container.querySelector<HTMLElement>('[data-process-group] [data-disclosure-row]')!
    await act(async () => group.click())
    expect(ordinaryReads()).toBe(1)
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    const reads = ordinaryReads()
    expect(reads).toBeLessThanOrEqual(9)
    await act(async () => group.click())
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    expect(ordinaryReads()).toBe(reads)
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it('fences an in-flight empty discovery when the unchanged empty IDs become publicly folded', async () => {
    let finish!: (reply: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const processBlocks = [publicTool(), publicTool('b')]
    await render({ processBlocks, policy: presentationPolicyFor('verbose') })
    expect(api.readProcessPane).toHaveBeenCalledTimes(1)
    await render({ processBlocks, policy: presentationPolicyFor('compact') })
    await act(async () => finish(snapshot()))
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    expect(api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly)).toHaveLength(1)
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
  })

  it.each(['native_reasoning', 'edit'] as const)('discovers late %s after an authorized empty first snapshot without replacing the public owner', async kind => {
    api.authorizeProcessPane.mockResolvedValue({ status: 'authorized', schemaVersion: 2, viewerId: viewer,
      revision: '0', scope: scope(), presence: { nativeBlockCount: 0, toolCaptureCount: 0 } })
    const empty = { ...snapshot(false, 'thread', 'turn', '1'), units: [], blocks: [] }
    const late = { ...snapshot(false, 'thread', 'turn', '2'), units: [unit(kind === 'edit' ? [anchor()] : [])], blocks: [block(false, kind)] }
    api.readProcessPane.mockResolvedValueOnce(empty).mockResolvedValue(late)
    await render({ processBlocks: kind === 'edit' ? [publicTool()] : [] })
    const publicOwner = container.querySelector('[data-analytix-process-block-id="public-a"]')
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(1)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    expect(container.textContent).not.toContain('PRIVATE_BODY')
    if (kind === 'edit') {
      expect(container.querySelectorAll('[data-analytix-process-block-id="public-a"]')).toHaveLength(1)
      expect(container.querySelector('[data-analytix-process-block-id="public-a"]')).toBe(publicOwner)
    }
    api.readProcessPane.mockResolvedValueOnce({ ...late, revision: '3', blocks: [block(true, kind)] })
    const row = kind === 'edit' ? publicOwner!.querySelector<HTMLElement>('[data-disclosure-row]')
      : container.querySelector<HTMLElement>('[data-protected-process-block] [data-disclosure-row]')
    expect(row).not.toBeNull()
    await act(async () => row!.click())
    expect(container.textContent).toContain('PRIVATE_BODY')
    expect(container.querySelector(kind === 'native_reasoning' ? '[data-markdown-view-only="true"]' : '[data-tool-io]')).not.toBeNull()
  })
  it('bounds empty native/tool discovery while draft-only polling continues, without renewing native/tool scope on mode changes', async () => {
    const ordinaryReads = () => api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
    api.authorizeProcessPane.mockResolvedValue({ status: 'authorized', schemaVersion: 2, viewerId: viewer,
      revision: '0', scope: scope(), presence: { nativeBlockCount: 0, toolCaptureCount: 0 } })
    api.readProcessPane.mockResolvedValue({ ...snapshot(false, 'thread', 'turn', '1'), units: [], blocks: [] })
    await render()
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    const reads = ordinaryReads()
    expect(reads).toBeGreaterThan(1); expect(reads).toBeLessThanOrEqual(9)
    expect(api.readProcessPane.mock.calls.some(([request]) => request.draftsOnly)).toBe(true)
    await render({ policy: presentationPolicyFor('verbose') })
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    expect(ordinaryReads()).toBe(reads)
    await render({ processOpen: false, policy: presentationPolicyFor('detailed') })
    await render({ processOpen: true, policy: presentationPolicyFor('verbose') })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(ordinaryReads()).toBe(reads)
    await render({ processing: false, processOpen: false, status: 'completed', offerProcessEntry: true })
    const beforeClick = api.readProcessPane.mock.calls.length
    const header = container.querySelector<HTMLElement>('[data-analytix-turn-process]')!
    expect(header).not.toBeNull(); await act(async () => header.click())
    await render({ processing: false, processOpen: true, status: 'completed', offerProcessEntry: true })
    expect(api.readProcessPane).toHaveBeenCalledTimes(beforeClick + 1)
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it('bounds settlement discovery and visible-seat hydration to two reads and then stops', async () => {
    api.authorizeProcessPane.mockResolvedValue({ status: 'authorized', schemaVersion: 2, viewerId: viewer,
      revision: '0', scope: scope(), presence: { nativeBlockCount: 0, toolCaptureCount: 0 } })
    api.readProcessPane.mockResolvedValue({ ...snapshot(false, 'thread', 'turn', '1'), units: [], blocks: [] })
    await render()
    await act(async () => vi.advanceTimersByTimeAsync(1000))
    const first = block(), second = { ...block(false, 'native_reasoning', 'b'), nativeIndex: 1, source: { ...first.source, kind: 'native' as const, nativeIndex: 1, sourceOrdinal: '2' } }
    api.readProcessPane.mockResolvedValueOnce(snapshot(false, 'thread', 'turn', '2'))
      .mockResolvedValue({ ...snapshot(false, 'thread', 'turn', '3'), blocks: [first, second] })
    const before = api.readProcessPane.mock.calls.length
    await render({ processing: false, policy: presentationPolicyFor('verbose') })
    expect(api.readProcessPane).toHaveBeenCalledTimes(before + 2)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    const settled = api.readProcessPane.mock.calls.length
    await act(async () => vi.advanceTimersByTimeAsync(30_000))
    expect(api.readProcessPane).toHaveBeenCalledTimes(settled)
  })
  it('resumes visible polling on the same lease when the producer becomes active again', async () => {
    await render({ policy: presentationPolicyFor('verbose') })
    await render({ processing: false, policy: presentationPolicyFor('verbose') })
    const settled = api.readProcessPane.mock.calls.length
    await act(async () => vi.advanceTimersByTimeAsync(2000))
    expect(api.readProcessPane).toHaveBeenCalledTimes(settled)
    await render({ processing: true, policy: presentationPolicyFor('verbose') })
    await act(async () => vi.advanceTimersByTimeAsync(1000))
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(settled)
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it('preserves a matched public owner, path, metadata and already-open disclosure when capture arrives', async () => {
    let finish!: (value: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const publicBlock: ChatBlock = { ...publicTool(), filePath: '/synthetic/public-owner.txt',
      meta: { ...publicTool().meta, diagnostics: { jobId: 'synthetic-job' }, activeSkillIds: ['public-skill'], sources: [{ title: 'PUBLIC_SOURCE', url: 'https://example.invalid/public' }] } }
    await render({ processBlocks: [publicBlock] })
    const owner = container.querySelector<HTMLElement>('[data-analytix-process-block-id="public-a"]')!
    const row = owner.querySelector<HTMLElement>('[data-disclosure-row]')!, parent = owner.parentElement
    const diagnostics = owner.querySelector('[aria-label="' + i18n.t('common:openRuntimeDiagnostics') + '"]')!
    expect(diagnostics).not.toBeNull()
    await act(async () => row.click())
    expect(owner.textContent).toContain('public detail')
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])], blocks: [block(true, 'edit')] })
    await act(async () => finish({ ...snapshot(), units: [unit([anchor()])], blocks: [block(false, 'edit')] }))
    expect(container.querySelector('[data-analytix-process-block-id="public-a"]')).toBe(owner)
    expect(owner.parentElement).toBe(parent)
    expect(owner.querySelector('[data-disclosure-row]')).toBe(row)
    expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(owner.textContent).toContain('/synthetic/public-owner.txt')
    expect(owner.textContent).toContain('public detail')
    expect(owner.textContent).toContain('PUBLIC_SOURCE')
    expect(owner.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    expect(owner.querySelector('[aria-label="' + i18n.t('common:openRuntimeDiagnostics') + '"]')).toBe(diagnostics)
    expect(owner.querySelectorAll('[data-disclosure-row]')).toHaveLength(1)
  })
  it('does not restore matched private body from a preserved public disclosure after group reopen', async () => {
    const preview = { ...snapshot(), units: [unit([anchor()])], blocks: [block(false, 'edit')] }
    api.readProcessPane.mockResolvedValue(preview)
    await render({ processBlocks: [publicTool()] })
    const publicRow = container.querySelector<HTMLElement>('[data-analytix-process-block-id="public-a"] [data-disclosure-row]')!
    api.readProcessPane.mockResolvedValueOnce({ ...preview, blocks: [block(true, 'edit')] })
    await act(async () => publicRow.click())
    expect(container.textContent).toContain('PRIVATE_BODY')
    await render({ processBlocks: [publicTool()], policy: presentationPolicyFor('compact') })
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [] })
    const expansions = api.setProcessPaneExpanded.mock.calls.length
    await act(async () => container.querySelector<HTMLElement>('[data-process-group] [data-disclosure-row]')!.click())
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(api.setProcessPaneExpanded).toHaveBeenCalledTimes(expansions)
    expect(publicRow.getAttribute('aria-expanded')).toBe('true')
    await act(async () => publicRow.click())
    api.readProcessPane.mockResolvedValueOnce({ ...preview, blocks: [block(true, 'edit')] })
    await act(async () => publicRow.click())
    expect(container.textContent).toContain('PRIVATE_BODY')
  })
  it.each(['ancestor', 'runtime'] as const)('does not restore matched private body from old public intent after %s retirement', async cause => {
    const preview = { ...snapshot(), units: [unit([anchor()])], blocks: [block(false, 'edit')] }
    api.readProcessPane.mockResolvedValue(preview)
    await render({ processBlocks: [publicTool()] })
    const publicRow = container.querySelector<HTMLElement>('[data-analytix-process-block-id="public-a"] [data-disclosure-row]')!
    api.readProcessPane.mockResolvedValueOnce({ ...preview, blocks: [block(true, 'edit')] })
    await act(async () => publicRow.click())
    expect(container.textContent).toContain('PRIVATE_BODY')
    const expansions = api.setProcessPaneExpanded.mock.calls.length
    await act(async () => {
      if (cause === 'ancestor') { container.hidden = true; await Promise.resolve() }
      else runtimeInvalidators.forEach(handler => handler({ state: 'stopped' }))
    })
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    await act(async () => {
      if (cause === 'ancestor') { container.hidden = false; await Promise.resolve() }
      else runtimeInvalidators.forEach(handler => handler({ state: 'running' }))
    })
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(container.querySelector('[data-analytix-process-block-id="public-a"] [data-disclosure-row]')).toBe(publicRow)
    expect(publicRow.getAttribute('aria-expanded')).toBe('true')
    expect(api.setProcessPaneExpanded).toHaveBeenCalledTimes(expansions)
  })
  it('keeps hidden-group metadata out while another actual group continues receiving fresh preview', async () => {
    const otherUnit = { ...unit([anchor('b')]), unitId: 'unit-b', providerSequence: '2' }
    const a = { ...block(), title: 'VISIBLE_A_TITLE', preview: 'VISIBLE_A_PREVIEW' }
    const b = { ...block(false, 'edit', 'b'), title: 'HIDDEN_B_TITLE', preview: 'HIDDEN_B_PREVIEW', path: '/synthetic/hidden-b',
      source: { kind: 'tool' as const, unitId: 'unit-b', ...anchor('b') } }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit(), otherUnit], blocks: [a, b] })
    await render({ policy: presentationPolicyFor('standard'), processBlocks: [publicTool('b')] })
    expect(container.textContent).not.toContain('VISIBLE_A'); expect(container.textContent).not.toContain('HIDDEN_B')
    const group = container.querySelector<HTMLElement>('[data-process-group="unit:unit-a"] [data-disclosure-row]')!
    expect(group).not.toBeNull()
    await act(async () => group.click())
    expect(container.textContent).toContain('VISIBLE_A_PREVIEW'); expect(container.textContent).not.toContain('HIDDEN_B')
    expect(container.textContent).not.toContain('/synthetic/hidden-b')
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit(), otherUnit], blocks: [{ ...a, preview: 'VISIBLE_A_UPDATED' }, b] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.textContent).toContain('VISIBLE_A_UPDATED'); expect(container.textContent).not.toContain('HIDDEN_B')
    await act(async () => group.click())
    expect(container.textContent).not.toContain('VISIBLE_A'); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    const ordinaryReads = () => api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
    const reads = ordinaryReads()
    await act(async () => vi.advanceTimersByTimeAsync(3000))
    expect(ordinaryReads()).toBe(reads)
    expect(api.readProcessPane.mock.calls.at(-1)?.[0].draftsOnly).toBe(true)
    await act(async () => group.click())
    expect(api.readProcessPane).toHaveBeenLastCalledWith({ viewerId: viewer, waitMs: 0 })
    expect(container.textContent).toContain('VISIBLE_A_UPDATED'); expect(container.textContent).not.toContain('HIDDEN_B')
  })
  it('fences a late private poll after the actual group is folded', async () => {
    await render()
    let finish!: (value: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => vi.advanceTimersByTimeAsync(500))
    await render({ policy: presentationPolicyFor('compact') })
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await act(async () => finish(snapshot()))
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
  })
  it.each(['revoked', 'malformed'] as const)('never retries %s startup or treats it as empty presence', async status => {
    api.authorizeProcessPane.mockResolvedValue(status === 'malformed' ? { status: 'not_a_contract' } : { status })
    await render(); await act(async () => vi.advanceTimersByTimeAsync(60_000))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(api.readProcessPane).not.toHaveBeenCalled(); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    expect(container.textContent).toContain(i18n.t('common:dshProcessUnavailable'))
  })
  it('cancels pending startup recovery on actual pane hiding', async () => {
    api.authorizeProcessPane.mockResolvedValue({ status: 'unavailable' })
    await render(); await render({ route: route('main', false) })
    await act(async () => vi.advanceTimersByTimeAsync(60_000))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(api.readProcessPane).not.toHaveBeenCalled()
  })
  it('recovers bounded startup unavailability while the same runtime remains running', async () => {
    api.authorizeProcessPane.mockResolvedValueOnce({ status: 'unavailable' })
    await render()
    expect(container.textContent).toContain(i18n.t('common:dshProcessUnavailable'))
    expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    await act(async () => vi.advanceTimersByTimeAsync(1000))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
  })
  it('bounds startup retries and cancels them when the pane becomes hidden', async () => {
    api.authorizeProcessPane.mockResolvedValue({ status: 'unavailable' })
    await render()
    await act(async () => vi.advanceTimersByTimeAsync(1000))
    await act(async () => vi.advanceTimersByTimeAsync(2000))
    await act(async () => vi.advanceTimersByTimeAsync(4000))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(4)
    await act(async () => vi.advanceTimersByTimeAsync(60_000))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(4)
    await render({ route: route('main', false) })
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
  })
  it('labels provider summaries explicitly and rejects changing that provenance on a retained source', async () => {
    const summary = { ...block(), nativeIndex: undefined, source: { kind: 'provider_reasoning' as const, unitId: 'unit-a', producerId: 'producer-a',
      providerSequence: '1', attempt: 1, sourceOrdinal: '1', reasoningForm: 'summary' as const } }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), blocks: [summary] })
    await render()
    expect(container.textContent).toContain('推理摘要')
    expect(container.textContent).not.toContain('SAFE_PRIVATE_TITLE')
    api.readProcessPane.mockResolvedValue({ ...snapshot(false, 'thread', 'turn', '18446744073709551615'),
      blocks: [{ ...summary, source: { ...summary.source, reasoningForm: undefined } }] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(api.closeProcessPane).toHaveBeenCalled()
    expect(container.textContent).not.toContain('SAFE_PRIVATE_PREVIEW')
  })
  it('automatically leases a visible native pane and expands a real row once, with no separate permission/capture buttons', async () => {
    await render()
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(container.querySelectorAll('[data-process-pane-id]')).toHaveLength(1)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    for (const key of ['dshProcessOpen', 'dshProcessClose', 'dshProcessCapture']) expect(container.textContent).not.toContain(i18n.t('common:' + key))
    expect(api.readProcessPane.mock.calls[0][0]).toEqual({ viewerId: viewer, waitMs: 0 })
    api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: ['block-a'] })
    expect(api.readProcessPane.mock.calls[1][0]).toEqual({ viewerId: viewer, waitMs: 0 })
    expect(container.querySelector('strong')?.textContent).toBe('PRIVATE_BODY')
    expect(container.querySelector('table')).not.toBeNull(); expect(container.querySelector('img,a,[role="dialog"]')).toBeNull()
    expect(finalization).not.toHaveBeenCalled(); expect(api.openProcessCapture).not.toHaveBeenCalled()
    await toggle(); expect(container.textContent).not.toContain('PRIVATE_BODY'); expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [] })
  })
  it('obtains only genuine presence while the whole process is folded and never paints a Think before snapshot', async () => {
    await render({ processOpen: false })
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1); expect(api.readProcessPane).not.toHaveBeenCalled()
    expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    let finish!: (value: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render(); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    await act(async () => finish(snapshot())); expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it('clears all preview/body on whole-turn fold, retains only its lease and reads preview on explicit reopen', async () => {
    await render(); api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    await render({ processOpen: false })
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [] })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    await render(); expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it('shares policy without new permission reads and clears bodies on step-group fold', async () => {
    await render(); api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    await render({ policy: presentationPolicyFor('compact') })
    expect(container.textContent).not.toContain('PRIVATE_BODY'); expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    const ordinaryReads = () => api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
    const foldedReads = ordinaryReads()
    await act(async () => vi.advanceTimersByTimeAsync(3000))
    expect(ordinaryReads()).toBe(foldedReads)
    expect(api.readProcessPane.mock.calls.at(-1)?.[0].draftsOnly).toBe(true)
    expect(container.querySelector('[data-process-group] [data-disclosure-row]')?.getAttribute('aria-expanded')).toBe('false')
    const group = container.querySelector<HTMLElement>('[data-process-group] > div [data-disclosure-row]')!
    await act(async () => group.click())
    expect(container.querySelector('[data-protected-process-block]')?.textContent).not.toContain('PRIVATE_BODY')
    await render({ policy: presentationPolicyFor('verbose') })
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
    expect(container.querySelector('[data-protected-process-block]')?.textContent).toContain('SAFE_PRIVATE_PREVIEW')
  })
  it('keeps three surfaces independent and targets one pane invalidation, while runtime loss clears all', async () => {
    const panes = (['main', 'side', 'inspector'] as const).map(surface => createElement(ProtectedProcessPane, {
      key: surface, route: route(surface), threadId: 'thread', turnId: 'turn', processOpen: true, processing: true, runtimeAvailable: true }))
    await act(async () => root.render(createElement('div', {}, panes)))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(3); expect(api.closeProcessPane).not.toHaveBeenCalled()
    const side = container.querySelector<HTMLElement>('[data-process-surface="side"]')!, sideId = side.dataset.processPaneId!
    await act(async () => invalidators.get(sideId)!())
    expect(side.textContent).not.toContain('SAFE_PRIVATE')
    for (const surface of ['main', 'inspector']) expect(container.querySelector(`[data-process-surface="${surface}"]`)?.textContent).toContain('SAFE_PRIVATE')
    expect(api.closeProcessPane).toHaveBeenCalledTimes(1)
    await act(async () => runtimeInvalidators.forEach(handler => handler({ state: 'stopped' })))
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(api.closeProcessPane).toHaveBeenCalledTimes(3)
    await render(); expect(container.textContent).not.toContain('PRIVATE_BODY')
  })
  it('ignores a late A response across A→B→A even though thread and turn strings recur', async () => {
    let finish!: (value: unknown) => void
    api.authorizeProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render(); await render({ route: { ...route(), selectionKey: 'B' } }); await render()
    const count = api.closeProcessPane.mock.calls.length
    await act(async () => finish({ status: 'authorized', schemaVersion: 2, viewerId: '44444444-4444-4444-8444-444444444444', revision: '1', scope: scope(), presence: { nativeBlockCount: 1, toolCaptureCount: 0 } }))
    expect(api.closeProcessPane).toHaveBeenCalledTimes(count + 1)
    expect(api.closeProcessPane).toHaveBeenLastCalledWith({ viewerId: '44444444-4444-4444-8444-444444444444' })
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
  })
  it.each(['contextEpoch', 'processIncarnation', 'contextDigest'] as const)('fails closed on a changed %s reply fence', async field => {
    const next = snapshot(); next.scope = { ...next.scope, [field]: field === 'contextEpoch' ? '2' : field === 'contextDigest' ? 'c'.repeat(64) : 'other' }
    api.readProcessPane.mockResolvedValue(next); await render()
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(container.textContent).toContain(i18n.t('common:dshProcessUnavailable'))
    expect(api.closeProcessPane).toHaveBeenCalledWith({ viewerId: viewer })
  })
  it('rejects unexpanded bodies and successful V1 replies without V2 authority identity', async () => {
    api.readProcessPane.mockResolvedValue(snapshot(true)); await render()
    expect(container.textContent).not.toContain('PRIVATE_BODY'); expect(api.closeProcessPane).toHaveBeenCalled()
    await act(async () => root.render(null))
    api.authorizeProcessPane.mockResolvedValue({ status: 'authorized', viewerId: viewer, revision: '1', scope: { threadId: 'thread', turnId: 'turn', contextDigest: 'a'.repeat(64) } })
    const count = api.readProcessPane.mock.calls.length
    await render(); expect(api.readProcessPane).toHaveBeenCalledTimes(count); expect(container.textContent).not.toContain('SAFE_PRIVATE')
  })
  it('joins same-named captures by exact call/result item, with reversed completion and no extra rows', async () => {
    const a = { ...block(false, 'edit', 'a'), preview: 'CAPTURE_A' }, b = { ...block(false, 'edit', 'b'), preview: 'CAPTURE_B' }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor('a'), anchor('b')])], blocks: [b, a] })
    await render({ processBlocks: [publicTool('a'), publicTool('b')] })
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(2)
    const seats = [...container.querySelectorAll<HTMLElement>('[data-chat-seat]')]
    expect(seats.map(node => node.dataset.chatSeat)).toEqual(['public-a', 'public-b'])
    expect(seats[0].textContent).toContain('CAPTURE_A'); expect(seats[1].textContent).toContain('CAPTURE_B')
    const before = seats[0]
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor('a'), anchor('b')])], blocks: [a, b] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.querySelector('[data-chat-seat="public-a"]')).toBe(before)
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(2)
  })
  it('does not guess associations from title/path or a wrong item and preserves actual public content', async () => {
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])], blocks: [block(false, 'edit')] })
    const wrong = publicTool(); if (wrong.kind === 'tool') wrong.meta = { ...wrong.meta, sourceItemId: 'unrelated-item' }
    await render({ processBlocks: [wrong] })
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
    expect(container.textContent).toContain('same title'); expect(container.textContent).not.toContain('SAFE_PRIVATE')
  })
  it('keeps source ordinal stable under replay and rejects an identity-conflicting replay', async () => {
    await render(); const seat = container.querySelector('[data-chat-seat="block-a"]')
    await act(async () => vi.advanceTimersByTimeAsync(1500)); expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(seat)
    const changed = block(); if (changed.source.kind === 'native') changed.source = { ...changed.source, sourceOrdinal: '2' }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), blocks: [changed] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(api.closeProcessPane).toHaveBeenCalled()
  })
  it('keeps public seats and their disclosures across delayed association, native insertion and whole fold', async () => {
    let finish!: (value: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const publicBlocks = [publicTool('a'), publicTool('b')]
    await render({ processBlocks: publicBlocks })
    const seatA = container.querySelector<HTMLElement>('[data-chat-seat="public-a"]')!
    const seatB = container.querySelector<HTMLElement>('[data-chat-seat="public-b"]')!
    await act(async () => seatB.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
    expect(seatB.textContent).toContain('public detail')
    const otherUnit = { ...unit([anchor('b')]), unitId: 'unit-b', providerSequence: '2' }
    const reply = { ...snapshot(), units: [unit([anchor('a')]), otherUnit] }
    api.readProcessPane.mockResolvedValue(reply)
    await act(async () => finish(reply))
    expect(container.querySelector('[data-chat-seat="public-a"]')).toBe(seatA)
    expect(container.querySelector('[data-chat-seat="public-b"]')).toBe(seatB)
    expect(seatB.textContent).toContain('public detail')
    await render({ processBlocks: publicBlocks, processOpen: false })
    expect(container.querySelector('[data-chat-seat="public-a"]')).toBe(seatA)
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await render({ processBlocks: publicBlocks })
    expect(container.querySelector('[data-chat-seat="public-b"]')).toBe(seatB)
    expect(seatB.textContent).not.toContain('public detail')
  })
  it('keeps both units sharing a real preceding anchor and fixes transport replay order', async () => {
    const afterA = { ...unit(), unitId: 'unit-after-a', providerSequence: '2', placement: 'anchored' as const, precedingCalls: [anchor()] }
    const afterB = { ...unit(), unitId: 'unit-after-b', providerSequence: '3', placement: 'anchored' as const, precedingCalls: [anchor()] }
    const native = (unit: ProcessDisplayUnitV2, suffix: string) => {
      const value = block(false, 'native_reasoning', suffix)
      if (value.source.kind === 'native') value.source = { ...value.source, unitId: unit.unitId, providerSequence: unit.providerSequence }
      return value
    }
    const a = native(afterA, 'after-a'), b = native(afterB, 'after-b')
    const reply = { ...snapshot(), units: [afterA, afterB], blocks: [a, b] }
    api.readProcessPane.mockResolvedValue(reply); await render({ processBlocks: [publicTool()] })
    expect([...container.querySelectorAll<HTMLElement>('[data-chat-seat]')].map(node => node.dataset.chatSeat)).toEqual(['public-a', 'block-after-a', 'block-after-b'])
    const seats = [...container.querySelectorAll('[data-chat-seat]')]
    api.readProcessPane.mockResolvedValue({ ...reply, units: [afterB, afterA], blocks: [b, a] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual(seats)
  })
  it('rejects an anchor disappearing or moving to a different unit on replay', async () => {
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])] })
    await render({ processBlocks: [publicTool(), publicTool('b')] })
    expect(container.querySelector('[data-chat-seat="block-a"]')).not.toBeNull()
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor('b')])] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(api.closeProcessPane).toHaveBeenCalled()
  })
  it.each(['before', 'after'] as const)('places an unseen anchored native %s its exact late public counterpart without pinning a tail owner', async relation => {
    const anchoredUnit = relation === 'before' ? unit([anchor()]) : { ...unit(), placement: 'anchored' as const, precedingCalls: [anchor()] }
    const reply = { ...snapshot(), units: [anchoredUnit] }
    api.readProcessPane.mockResolvedValue(reply)
    await render({ processBlocks: [publicTool('b')], policy: presentationPolicyFor('verbose') })
    const existingPublic = container.querySelector('[data-chat-seat="public-b"]')!
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBeNull()
    expect(container.querySelector('[data-process-group="unit:unit-a"]')).toBeNull()
    await render({ processBlocks: [publicTool('b'), publicTool()], policy: presentationPolicyFor('verbose') })
    const seats = [...container.querySelectorAll<HTMLElement>('[data-chat-seat]')]
    expect(seats.map(seat => seat.dataset.chatSeat)).toEqual(relation === 'before'
      ? ['public-b', 'block-a', 'public-a'] : ['public-b', 'public-a', 'block-a'])
    expect(container.querySelector('[data-chat-seat="public-b"]')).toBe(existingPublic)
    expect(container.querySelector('[data-chat-seat="block-a"]')!.closest('[data-process-group]')?.getAttribute('data-process-group')).toBe('public:public-b')
    expect(container.querySelector('[data-process-group="unit:unit-a"]')).toBeNull()
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it.each(['pending', 'unit_only'] as const)('keeps the actual shown %s owner while its newly anchored public counterpart remains delayed', async placement => {
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [{ ...unit(), placement }] })
    await render({ processBlocks: [publicTool('b')], policy: presentationPolicyFor('verbose') })
    const nativeSeat = container.querySelector('[data-chat-seat="block-a"]')!, owner = nativeSeat.closest('[data-process-group]')!
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(nativeSeat)
    expect(nativeSeat.closest('[data-process-group]')).toBe(owner)
    await render({ processBlocks: [publicTool('b'), publicTool()], policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(nativeSeat)
    expect(nativeSeat.closest('[data-process-group]')).toBe(owner)
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it('retains an actually shown unit-only native owner when exact public anchors subsequently arrive', async () => {
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit()] })
    await render({ processBlocks: [publicTool()], policy: presentationPolicyFor('verbose') })
    const nativeSeat = container.querySelector('[data-chat-seat="block-a"]')!, owner = nativeSeat.closest('[data-process-group]')!
    expect(owner.getAttribute('data-process-group')).toBe('unit:unit-a')
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(nativeSeat)
    expect(nativeSeat.closest('[data-process-group]')).toBe(owner)
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it('accepts actual optional call/result completion and pending-to-anchored metadata without changing the native seat', async () => {
    const pending = { ...unit(), placement: 'pending' as const }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [pending] })
    const publicCall = publicTool(); if (publicCall.kind === 'tool') publicCall.meta = { ...publicCall.meta, sourceItemId: anchor().toolCallItemId }
    await render({ processBlocks: [publicCall] }); const nativeSeat = container.querySelector('[data-chat-seat="block-a"]')
    const call = { callId: anchor().callId, toolCallItemId: anchor().toolCallItemId }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([call])] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(nativeSeat)
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])] })
    await act(async () => vi.advanceTimersByTimeAsync(1500))
    expect(container.querySelector('[data-chat-seat="block-a"]')).toBe(nativeSeat)
    expect(container.textContent).toContain('SAFE_PRIVATE'); expect(api.closeProcessPane).not.toHaveBeenCalled()
  })
  it.each(['hidden', 'inert', 'aria-hidden', 'style'] as const)('synchronously retires copies when a real ancestor becomes %s', async attribute => {
    await render(); api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    await act(async () => { container.setAttribute(attribute, attribute === 'style' ? 'display:none' : attribute === 'aria-hidden' ? 'true' : '') })
    expect(container.textContent).not.toContain('PRIVATE_BODY'); expect(container.textContent).not.toContain('SAFE_PRIVATE')
    expect(api.closeProcessPane).toHaveBeenCalled()
    await act(async () => container.removeAttribute(attribute))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
  })
  it('starts offline without assuming runtime authority and acquires on the first genuine running event', async () => {
    await render({ runtimeAvailable: false })
    expect(api.authorizeProcessPane).not.toHaveBeenCalled(); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    await act(async () => runtimeInvalidators.forEach(handler => handler({ state: 'running' })))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1); expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    await render({ runtimeAvailable: true }); expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1); expect(container.textContent).not.toContain('PRIVATE_BODY')
  })
  it('gets fresh authority when the document and runtime genuinely return, without restoring old bodies', async () => {
    await render(); api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    await act(async () => { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) })
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    await act(async () => { vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); document.dispatchEvent(new Event('visibilitychange')) })
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
    await act(async () => runtimeInvalidators.forEach(handler => handler({ state: 'stopped' })))
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await act(async () => runtimeInvalidators.forEach(handler => handler({ state: 'running' })))
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(3)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW'); expect(container.textContent).not.toContain('PRIVATE_BODY')
  })
  it('fails delivery for duplicate/root replacement and never acquires in browser/hidden panes', async () => {
    let finish!: (value: unknown) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render()
    const duplicate = document.createElement('div'); duplicate.dataset.processPaneId = container.querySelector<HTMLElement>('[data-process-pane-id]')!.dataset.processPaneId
    await act(async () => { container.append(duplicate); finish(snapshot()) })
    expect(container.textContent).not.toContain('SAFE_PRIVATE'); duplicate.remove()
    await act(async () => root.render(null)); const calls = api.authorizeProcessPane.mock.calls.length
    document.documentElement.dataset.bridge = 'browser-preview'; await render()
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(calls)
    expect(container.querySelector('[role="status"]')?.textContent).toBe(i18n.t('common:dshProcessUnavailable'))
    delete document.documentElement.dataset.bridge; await render({ route: route('main', false) })
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(calls)
  })
  it('blocks private selection transfers while public text and ordinary editors retain their copy behavior', async () => {
    await render({ processBlocks: [{ kind: 'system', id: 'public', text: 'PUBLIC_STATUS', severity: 'info' }] })
    api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    const range = document.createRange(); range.selectNodeContents(container.querySelector('[data-protected-process-body]')!); document.getSelection()!.addRange(range)
    for (const name of ['copy', 'cut', 'dragstart']) { const event = new Event(name, { bubbles: true, cancelable: true }); document.dispatchEvent(event); expect(event.defaultPrevented).toBe(true) }
    const input = document.createElement('textarea'); document.body.append(input); input.focus()
    const event = new Event('copy', { bubbles: true, cancelable: true }); input.dispatchEvent(event); expect(event.defaultPrevented).toBe(false); input.remove()
    // Editor focus may replace the DOM selection; reselect the real private
    // body so the fold assertion continues to exercise sensitive selection.
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    await toggle(); expect(document.getSelection()?.rangeCount).toBe(0)
    const publicRange = document.createRange(); publicRange.selectNodeContents(container.querySelector('[data-chat-seat="public"]')!); document.getSelection()!.addRange(publicRange)
    const copy = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(copy); expect(copy.defaultPrevented).toBe(false)
  })
  it('blocks a mixed selection before the existing protected-tool host copy effect', async () => {
    const copy = vi.fn(async () => ({ ok: true }))
    const selector = { turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
    Object.assign(api, { openToolResultLocalDisplay: async () => ({ ok: true, display: { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash', capture: { kind: 'shell', status: 'completed', body: 'EXISTING_TOOL_BODY', label: 'synthetic', exitCode: 0, durationMs: 1, truncated: false, startLine: 0, endLine: 0, totalLines: 0 } } }),
      closeToolResultLocalDisplay: async () => undefined, copyToolResultLocalDisplay: copy, saveToolResultLocalDisplay: async () => ({ ok: true }), onToolResultLocalDisplayInvalidated: () => () => undefined })
    await act(async () => root.render(createElement('div', {},
      createElement(ProtectedToolResult, { threadId: 'thread', block: { kind: 'tool', id: 'old-tool', summary: 'bash', status: 'success', meta: { localResult: selector } } }),
      createElement(ProtectedProcessPane, { route: route(), threadId: 'thread', turnId: 'turn', processOpen: true, processing: true, runtimeAvailable: true }))))
    await act(async () => [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:protectedToolOpen'))!.click())
    api.readProcessPane.mockResolvedValueOnce(snapshot(true)); await toggle()
    const range = document.createRange(); range.setStartBefore(container.querySelector('[data-protected-tool-body]')!); range.setEndAfter(container.querySelector('[data-protected-process-body]')!)
    document.getSelection()!.addRange(range)
    const event = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true); expect(copy).not.toHaveBeenCalled()
  })
})

describe('V2 inline recovery through the actual preload facade', () => {
  const control = { schemaVersion: 2 as const, documentEpoch: '1', authorityEpoch: '0' }
  const authorization = (viewerId = viewer) => ({ status: 'authorized' as const, schemaVersion: 2 as const,
    viewerId, revision: '18446744073709551614', scope: scope(), presence: { nativeBlockCount: 1, toolCaptureCount: 0 } })
  const calls = (channel: string) => facadeTransport.invoke.mock.calls.filter(([called]) => called === channel)
  const emit = (channel: string, value: unknown) => {
    for (const handler of [...(facadeTransport.listeners.get(channel) ?? [])]) handler({}, value)
  }
  async function loadFacade() {
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    facadeTransport.expose.mockClear(); facadeTransport.invoke.mockReset(); facadeTransport.listeners.clear()
    facadeTransport.invoke.mockImplementation(async channel => {
      if (channel === 'process-local:authorize') return { control, result: authorization() }
      if (channel === 'process-local:read') return snapshot()
      if (channel === 'process-local:expand' || channel === 'process-local:close') return { status: 'ok' }
      throw new Error(`Unexpected synthetic IPC channel: ${channel}`)
    })
    const registration = vi.spyOn(document, 'addEventListener')
    vi.resetModules()
    // Load the native module at runtime without adding Electron/Node globals
    // to the renderer TypeScript program. Its actual facade is still exposed.
    await vi.importActual('../../../../../../preload/index')
    // Keep the actual renderer document and remove only the preload listener
    // captured during this import after the mounted leaf has been retired.
    for (const [name, handler] of registration.mock.calls) if (name === 'visibilitychange') {
      facadeCleanups.push(() => document.removeEventListener(name, handler))
    }
    const exposed = facadeTransport.expose.mock.calls.find(([name]) => name === 'analytix')
    if (!exposed) throw new Error('The real preload did not expose its facade')
    Object.defineProperty(window, 'analytix', { configurable: true, value: exposed[1] })
    const notifications: string[] = []
    facadeCleanups.push(exposed[1].runtime.onProcessPaneInvalidated(() => notifications.push('invalidated')))
    return notifications
  }
  it.each(['main', 'side', 'inspector'] as const)('renders multiple draft revisions before end through the real native facade on %s without expanding folded reasoning/tools', async surface => {
    await loadFacade()
    let revision = 2, text = '<think>FACADE_PRIVATE_FIRST</think> ', ids = new Set<string>()
    const actualUnit = unit([anchor(), anchor('b')])
    const draft: ProcessDisplayBlockV1 = { blockId: 'draft-a', kind: 'assistant_draft', state: 'running', attempt: 1,
      source: { kind: 'assistant_draft', unitId: actualUnit.unitId, producerId: actualUnit.producerId, providerSequence: actualUnit.providerSequence, attempt: 1 },
      contentClassification: 'unclassified_inline', classification: 'source_exact_local', truncated: false, durationMs: 0 }
    facadeTransport.invoke.mockImplementation(async (channel, payload) => {
      if (channel === 'process-local:authorize') return { control, result: { ...authorization(), revision: '1', presence: { nativeBlockCount: 1, toolCaptureCount: 0, assistantDraftCount: 1 } } }
      if (channel === 'process-local:expand') {
        ids = new Set((payload as { expandedBlockIds: string[] }).expandedBlockIds)
        expect(ids.has('block-a')).toBe(false)
        return { status: 'ok' }
      }
      if (channel === 'process-local:close') return { status: 'ok' }
      if (channel === 'process-local:read') {
        const request = payload as ProcessPaneReadRequestV1
        // The mocked IPC owner performs the same source projection as Main;
        // preload and Pane still validate the genuine native envelope/lease.
        const blocks = [{ ...draft, ...(ids.has(draft.blockId) ? { draftSegments: [{ offsetBytes: '0', text }], draftBytes: String(new TextEncoder().encode(text).length) } : {}) },
          ...request.draftsOnly ? [] : [{ ...block(), source: { ...block().source, unitId: actualUnit.unitId }, preview: 'FOLDED_REASONING_PRIVATE_PREVIEW' }]]
        return { ...snapshot(), revision: String(revision), units: [actualUnit], blocks }
      }
      throw new Error(`Unexpected synthetic IPC channel: ${channel}`)
    })
    await render({ route: route(surface), processBlocks: [publicTool(), publicTool('b')], policy: presentationPolicyFor('compact') })
    const responseSeat = container.querySelector('[data-analytix-assistant-response-seat]')!
    const logicalSeat = container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')!
    expect(logicalSeat.textContent).toContain('<think>FACADE_PRIVATE_FIRST</think>')
    expect(container.textContent).not.toContain('FOLDED_REASONING_PRIVATE_PREVIEW')
    expect(ids).toEqual(new Set([draft.blockId]))
    expect(calls('process-local:read').every(([, request]) => (request as ProcessPaneReadRequestV1).draftsOnly)).toBe(true)
    const composer = document.createElement('textarea'); composer.value = 'PRESERVED_COMPOSER_DRAFT'; document.body.append(composer)
    container.scrollTop = 137
    revision++; text = 'FACADE_PRIVATE_SECOND '
    await act(async () => vi.advanceTimersByTime(500))
    expect(container.querySelector('[data-chat-seat="unit-a:assistant_draft"]')).toBe(logicalSeat)
    expect(logicalSeat.textContent).toContain('FACADE_PRIVATE_SECOND'); expect(logicalSeat.textContent).not.toContain('FACADE_PRIVATE_FIRST')
    expect(container.scrollTop).toBe(137); expect(composer.value).toBe('PRESERVED_COMPOSER_DRAFT')
    await render({ route: route(surface), processBlocks: [publicTool(), publicTool('b')], policy: presentationPolicyFor('compact'),
      processing: false, terminalAuthorized: true, answer: createElement('p', {}, 'AUTHORIZED_BATCH_ANSWER') })
    expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(responseSeat)
    expect(responseSeat.textContent).toBe('AUTHORIZED_BATCH_ANSWER')
    expect(container.scrollTop).toBe(137); expect(composer.value).toBe('PRESERVED_COMPOSER_DRAFT')
    expect(finalization).not.toHaveBeenCalled(); composer.remove()
    await act(async () => emit('process-local:invalidated', { ...control, authorityEpoch: '1' }))
    expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
  })
  it('preserves decoded size through the real preload facade and retires it on IPC invalidation', async () => {
    await loadFacade()
    facadeTransport.invoke.mockImplementation(async channel => {
      if (channel === 'process-local:authorize') return { control, result: authorization() }
      if (channel === 'process-local:read') return { ...snapshot(), units: [unit([anchor()])], blocks: [{ ...block(false, 'write'), path: '/synthetic/file.ts', inputUtf16CodeUnits: 1025 }] }
      if (channel === 'process-local:expand' || channel === 'process-local:close') return { status: 'ok' }
      throw new Error(`Unexpected synthetic IPC channel: ${channel}`)
    })
    await render({ processBlocks: [publicTool()], policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-protected-input-size]')?.textContent).toBe('2KB')
    await act(async () => emit('process-local:invalidated', { ...control, authorityEpoch: '1' }))
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
  })
  it.each(['unavailable', 'busy'] as const)('receives real facade %s before retry and recovers preview without body', async status => {
    const notifications = await loadFacade()
    facadeTransport.invoke.mockResolvedValueOnce({ control, result: { status } })
    await render()
    expect(calls('process-local:authorize')).toHaveLength(1)
    expect(notifications).toEqual([])
    expect(container.textContent).toContain(i18n.t('common:dshProcessUnavailable'))
    expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    expect(calls('process-local:read')).toHaveLength(0)
    expect(calls('process-local:close')).toHaveLength(0)
    await act(async () => vi.advanceTimersByTimeAsync(1000))
    expect(calls('process-local:authorize')).toHaveLength(2)
    expect(calls('process-local:authorize')[1][1]).toEqual(calls('process-local:authorize')[0][1])
    expect(calls('process-local:read')).toHaveLength(1)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(notifications).toEqual([])
  })
  it('cancels pending recovery on actual IPC control advance with no owner', async () => {
    const status = 'unavailable'
    const notifications = await loadFacade()
    facadeTransport.invoke.mockResolvedValueOnce({ control, result: { status } })
    await render()
    await act(async () => emit('process-local:invalidated', control))
    expect(notifications).toEqual([])
    await act(async () => emit('process-local:invalidated', { ...control, authorityEpoch: '1' }))
    expect(notifications).toEqual(['invalidated'])
    expect(container.textContent).not.toContain('SAFE_PRIVATE')
    await act(async () => vi.advanceTimersByTimeAsync(60_000))
    expect(calls('process-local:authorize')).toHaveLength(1)
    expect(calls('process-local:read')).toHaveLength(0)
    await act(async () => {
      emit('process-local:invalidated', { ...control, authorityEpoch: '1' })
      emit('process-local:invalidated', control)
    })
    expect(notifications).toEqual(['invalidated'])
  })
  it.each(['unavailable', 'authorized'] as const)('does not let a late old %s reply clear the fresh renderer owner', async status => {
    const notifications = await loadFacade()
    let finish!: (reply: unknown) => void
    facadeTransport.invoke.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render()
    await act(async () => emit('runtime:status', createRuntimeStatusPublicV1({ code: 'runtime_starting', source: 'restart' })))
    await act(async () => emit('runtime:status', createRuntimeStatusPublicV1({ code: 'runtime_ready', source: 'restart' })))
    expect(calls('process-local:authorize')).toHaveLength(2)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    const before = [...notifications]
    const oldViewer = '44444444-4444-4444-8444-444444444444'
    await act(async () => finish({ control, result: status === 'authorized' ? authorization(oldViewer) : { status } }))
    expect(notifications).toEqual(before)
    expect(container.textContent).toContain('SAFE_PRIVATE_PREVIEW')
    expect(container.textContent).not.toContain('PRIVATE_BODY')
    await act(async () => vi.advanceTimersByTimeAsync(500))
    expect(calls('process-local:authorize')).toHaveLength(2)
    expect(calls('process-local:read').length).toBeGreaterThan(1)
    expect(calls('process-local:read').every(([, request]) => request !== null && typeof request === 'object'
      && 'viewerId' in request && request.viewerId === viewer)).toBe(true)
    expect(calls('process-local:close')).toEqual(status === 'authorized' ? [['process-local:close', { viewerId: oldViewer }]] : [])
    expect(notifications).toEqual(before)
  })
})


describe('typed mutation body under the existing protected pane authority', () => {
  const realCall = { callId: 'call_host_' + 'a'.repeat(64), toolCallItemId: 'item_call_' + 'a'.repeat(64),
    toolResultItemId: 'item_result_' + 'b'.repeat(64), startedEventSeq: '10' }
  const mappedResult = (toolName: 'write' | 'edit' | 'bash', state: 'completed' | 'failed' | 'canceled' = 'completed') => {
    const result = chatBlockFromItem({ id: realCall.toolResultItemId, threadId: 'thread', turnId: 'turn', role: 'tool',
      status: state === 'canceled' ? 'aborted' : state, createdAt: '2026-10-09T00:00:00Z', kind: 'tool_result', toolName,
      callId: realCall.callId, toolKind: toolName === 'bash' ? 'command_execution' : 'file_change', isError: state !== 'completed',
      output: { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: state === 'canceled' ? 'cancelled' : state,
        code: state === 'completed' ? 'tool_completed' : state === 'canceled' ? 'tool_cancelled' : 'tool_failed',
        messageKey: state === 'completed' ? 'tool_completed' : state === 'canceled' ? 'tool_cancelled' : 'tool_failed',
        privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } })
    if (result?.kind !== 'tool') throw new Error('fixture must reach the actual mapper tool result')
    expect(result.meta?.localResult).toEqual({ turnId: 'turn', callId: realCall.callId, resultItemId: realCall.toolResultItemId })
    return result
  }
  const patchBody = { text: '--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n-old host line\n+new host line\n',
    before: 'SEPARATELY_CAPPED_BEFORE', after: 'SEPARATELY_CAPPED_AFTER', error: '', addedLines: 1, removedLines: 1, approximate: false }
  function installCapture(metadata: ProcessDisplayBlockV1, body = patchBody) {
    let expanded = new Set<string>(), revision = 1
    api.authorizeProcessPane.mockResolvedValue({ status: 'authorized', schemaVersion: 2, viewerId: viewer, revision: '1',
      scope: scope(), presence: { nativeBlockCount: 0, toolCaptureCount: 1 } })
    api.setProcessPaneExpanded.mockImplementation(async request => {
      expanded = new Set((request as { expandedBlockIds: string[] }).expandedBlockIds); return { status: 'ok' }
    })
    api.readProcessPane.mockImplementation(async () => processDisplaySnapshotSchemaV1.parse({ ...snapshot(), revision: String(++revision),
      units: [unit([realCall])], blocks: [{ ...metadata, ...(expanded.has(metadata.blockId) ? { body } : {}) }] }))
  }
  function installInspect(toolName: string, state: 'completed' | 'failed' | 'canceled' = 'completed', typedBody?: string) {
    const listeners = new Set<(viewId: string | null) => void>()
    const display = toolResultLocalDisplaySchemaV1.parse({ schemaVersion: 1, snapshotDigest: 'c'.repeat(64),
      resultItemId: realCall.toolResultItemId, toolName, capture: { kind: typedBody === undefined ? 'inspect' : 'shell', status: state,
        body: typedBody ?? '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false },
      inspect: { inputJSON: toolName === 'bash' ? '{"command":"printf PRIVATE_REQUEST_BODY"}' : '{"path":"/synthetic/file.ts","content":"PRIVATE_REQUEST_BODY"}', inputTruncated: false,
        schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } })
    const toolApi = { openToolResultLocalDisplay: vi.fn(async (_request: unknown): Promise<unknown> => ({ ok: true, display })),
      closeToolResultLocalDisplay: vi.fn(async () => undefined),
      onToolResultLocalDisplayInvalidated: (listener: (viewId: string | null) => void) => { listeners.add(listener); return () => { listeners.delete(listener) } } }
    Object.assign(api, toolApi)
    return { toolApi, listeners }
  }
  it.each(['write', 'edit'] as const)('keeps the real mapper %s Diff and request Inspect in their existing owners', async toolName => {
    const publicBlock = mappedResult(toolName), { toolApi, listeners } = installInspect(toolName)
    const metadata: ProcessDisplayBlockV1 = { ...block(false, toolName), source: { kind: 'tool', unitId: 'unit-a', ...realCall },
      path: '/synthetic/file.ts', inputUtf16CodeUnits: 1025, effectApplied: true, settlementStatus: 'completed' }
    installCapture(metadata)
    const props = { processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') }
    const storeBefore = useChatStore.getState().blocks
    await render(props, 'thread')
    expect(toolApi.openToolResultLocalDisplay).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread',
      turnId: 'turn', callId: realCall.callId, resultItemId: realCall.toolResultItemId }))
    expect(container.querySelector('[data-diff]')).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_REQUEST_BODY')
    await act(async () => container.querySelector<HTMLElement>(`[data-analytix-process-block-id="${publicBlock.id}"] [data-disclosure-row]`)!.click())
    expect(container.querySelectorAll('[data-diff]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-protected-process-body]')).toHaveLength(1)
    expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
    expect(container.textContent).toContain('new host line'); expect(container.textContent).not.toContain('SEPARATELY_CAPPED_AFTER')
    expect(container.textContent).not.toContain(i18n.t('common:dshToolInspect.unavailable'))
    const inspect = [...container.querySelectorAll<HTMLButtonElement>('button')].filter(button => button.textContent === i18n.t('common:dshToolInspect.inspect'))
    expect(inspect).toHaveLength(1); await act(async () => inspect[0].click())
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    await act(async () => container.querySelector<HTMLButtonElement>('[role="tab"][id$="-payload"]')!.click())
    expect(container.textContent).toContain('PRIVATE_REQUEST_BODY'); expect(container.querySelectorAll('[data-diff]')).toHaveLength(1)
    expect(useChatStore.getState().blocks).toBe(storeBefore); expect(JSON.stringify(publicBlock)).not.toMatch(/new host line|PRIVATE_REQUEST_BODY/)
    for (const invalidate of [...listeners]) await act(async () => invalidate(null))
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_REQUEST_BODY')
    expect(container.querySelectorAll('[data-diff]')).toHaveLength(1)
    for (const invalidate of [...invalidators.values()]) await act(async () => invalidate())
    expect(container.querySelector('[data-diff],[data-tool-result-inspector]')).toBeNull()
    expect(container.textContent).not.toMatch(/new host line|PRIVATE_REQUEST_BODY/)
    expect(toolApi.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it('keeps the applied Diff available when ordinary Inspect needs a fresh authorized retry', async () => {
    const publicBlock = mappedResult('write'), { toolApi } = installInspect('write')
    toolApi.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: false, reason: 'unavailable' })
    installCapture({ ...block(false, 'write'), source: { kind: 'tool', unitId: 'unit-a', ...realCall },
      path: '/synthetic/file.ts', effectApplied: true, settlementStatus: 'completed' })
    await render({ processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') }, 'thread')
    await act(async () => container.querySelector<HTMLElement>(`[data-analytix-process-block-id="${publicBlock.id}"] [data-disclosure-row]`)!.click())
    expect(container.querySelectorAll('[data-diff]')).toHaveLength(1)
    await act(async () => [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:protectedToolRetry'))!.click())
    expect(toolApi.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
    expect(container.querySelectorAll('[data-diff]')).toHaveLength(1)
    expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
    expect([...container.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:dshToolInspect.inspect'))).toBe(true)
    expect(container.textContent).not.toContain('PRIVATE_REQUEST_BODY')
  })
  it.each(['failed', 'canceled'] as const)('does not invent a Diff or expose request input for real mapper %s mutations', async state => {
    const publicBlock = mappedResult('edit', state); installInspect('edit', state)
    installCapture({ ...block(false, 'edit'), state, source: { kind: 'tool', unitId: 'unit-a', ...realCall },
      path: '/synthetic/file.ts', effectApplied: false, settlementStatus: state === 'canceled' ? 'aborted' : 'failed' },
      { ...patchBody, text: 'HOST_SETTLEMENT_FAILURE', before: '', after: '', error: 'HOST_SETTLEMENT_FAILURE', addedLines: 0, removedLines: 0 })
    const props = { processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') }
    await render(props, 'thread')
    await act(async () => container.querySelector<HTMLElement>(`[data-analytix-process-block-id="${publicBlock.id}"] [data-disclosure-row]`)!.click())
    expect(container.querySelector('[data-diff],[data-protected-tool-body]')).toBeNull()
    expect(container.textContent).not.toMatch(/new host line|SEPARATELY_CAPPED_AFTER|PRIVATE_REQUEST_BODY/)
    expect([...container.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:dshToolInspect.inspect'))).toBe(true)
    await render({ ...props, route: route('main', false) }, 'thread')
    expect(container.querySelector('[data-protected-process-body],[data-tool-result-inspector]')).toBeNull()
  })
  it('keeps the actual typed Bash result exclusive over the same-call process capture', async () => {
    const publicBlock = mappedResult('bash'); installInspect('bash', 'completed', 'TYPED_TERMINAL_OUTPUT')
    installCapture({ ...block(false, 'shell'), source: { kind: 'tool', unitId: 'unit-a', ...realCall } }, { ...patchBody, text: 'DUPLICATE_PROCESS_OUTPUT' })
    await render({ processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') }, 'thread')
    await act(async () => container.querySelector<HTMLElement>(`[data-analytix-process-block-id="${publicBlock.id}"] [data-disclosure-row]`)!.click())
    expect(container.textContent).toContain('TYPED_TERMINAL_OUTPUT')
    expect(container.textContent).not.toContain('DUPLICATE_PROCESS_OUTPUT')
    expect(container.querySelectorAll('[data-terminal]')).toHaveLength(1)
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
  })
  it.each([undefined, 0, 1, 1024, 1025, 1026])('shows actual decoded size %s in anchored mutation rows and clears it on retirement', async inputUtf16CodeUnits => {
    const metadata = { ...block(false, 'edit'), path: '/synthetic/file.ts', ...(inputUtf16CodeUnits === undefined ? {} : { inputUtf16CodeUnits }) }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])], blocks: [metadata] })
    await render({ processBlocks: [publicTool()], policy: presentationPolicyFor('verbose') })
    const size = container.querySelector('[data-protected-input-size]')
    if (inputUtf16CodeUnits === undefined) expect(size).toBeNull()
    else expect(size?.textContent).toBe(`${Math.ceil(inputUtf16CodeUnits / 1024)}KB`)
    expect(container.textContent).not.toContain('PRIVATE_BODY')
    for (const off of invalidators.values()) await act(async () => off())
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
  })
  it('waits for the exact public mutation anchor before showing decoded size', async () => {
    const metadata = { ...block(false, 'write'), path: '/synthetic/file.ts', inputUtf16CodeUnits: 1025 }
    api.readProcessPane.mockResolvedValue({ ...snapshot(), units: [unit([anchor()])], blocks: [metadata] })
    await render({ policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
    const publicRow = publicTool()
    await render({ processBlocks: [{ ...publicRow, meta: { ...publicRow.meta, toolName: 'write' } }], policy: presentationPolicyFor('verbose') })
    expect(container.querySelectorAll('[data-protected-input-size]')).toHaveLength(1)
    expect(container.querySelector('[data-protected-input-size]')?.textContent).toBe('2KB')
    await render({ route: route('main', false) })
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
  })

  it.each([{ state: 'completed', effectApplied: true, settlementStatus: 'completed' },
    { state: 'failed', effectApplied: true, settlementStatus: 'failed' },
    { state: 'completed', effectApplied: false, settlementStatus: 'no_effect' }] as const)(
    'shows only Host-applied diffs for $state/$effectApplied and retires them on invalidation', async facts => {
      const metadata = { ...block(false, 'edit'), ...facts, path: '/synthetic/file.ts', inputUtf16CodeUnits: 1025 }
      const body = { text: '--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n-old host line\n+new host line\n',
        before: 'SEPARATELY_CAPPED_BEFORE', after: 'SEPARATELY_CAPPED_AFTER', error: facts.state === 'failed' ? 'HOST_SETTLEMENT_FAILURE' : '', addedLines: 19, removedLines: 12, approximate: true }
      const result = { ...snapshot(), units: [unit([anchor()])], blocks: [metadata] }
      api.readProcessPane.mockResolvedValueOnce(result)
      await render({ processBlocks: [publicTool()], policy: presentationPolicyFor('verbose') })
      api.readProcessPane.mockResolvedValueOnce({ ...result, revision: '18446744073709551615', blocks: [{ ...metadata, body }] })
      await act(async () => container.querySelector<HTMLElement>('[data-analytix-process-block-id="public-a"] [data-disclosure-row]')!.click())
      expect(Boolean(container.querySelector('[data-diff]'))).toBe(facts.effectApplied)
      expect(container.querySelector('[data-protected-input-size]')?.textContent ?? null).toBe(facts.state === 'completed' ? '2KB' : null)
      if (facts.effectApplied) {
        expect(container.textContent).toContain('new host line'); expect(container.textContent).not.toContain('SEPARATELY_CAPPED_AFTER')
        expect(container.querySelector('[data-protected-process-body]')?.querySelector('button[aria-label="复制脱敏文本"]')).toBeNull()
      }
      if (facts.state === 'failed') expect(container.textContent).toContain('HOST_SETTLEMENT_FAILURE')
      for (const off of invalidators.values()) await act(async () => off())
      expect(container.querySelector('[data-diff]')).toBeNull(); expect(container.textContent).not.toContain('new host line')
    })
})


describe('active Web input in the existing native pane', () => {
  it('opens the sealed request role and retires URL state when the pane hides', async () => {
    const call = { callId: 'call-web', toolCallItemId: 'call-item-web', startedEventSeq: '12' }
    const publicItem: CoreTurnItemJson = { id: call.toolCallItemId, threadId: 'thread', turnId: 'turn', role: 'assistant',
      status: 'running', createdAt: '2026-10-08T00:00:00Z', kind: 'tool_call', toolName: 'web_fetch', callId: call.callId, toolKind: 'tool_call',
      arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
        privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } }
    const publicBlock = chatBlockFromItem(publicItem)
    if (publicBlock?.kind !== 'tool') throw new Error('fixture must reach the actual mapper tool block')
    const input: ProcessDisplayBlockV1 = { blockId: 'web-input', kind: 'web_fetch', state: 'running', attempt: 0,
      source: { kind: 'tool', unitId: 'unit-a', ...call }, durationMs: 0, truncated: false, classification: 'source_exact_local',
      requestUrl: 'https://example.invalid/actual/request?case=alice', inputDigest: 'c'.repeat(64) }
    api.authorizeProcessPane.mockImplementation(async request => ({ status: 'authorized', schemaVersion: 2, viewerId: viewer,
      revision: '1', scope: scope(request.threadId, request.turnId), presence: { nativeBlockCount: 0, toolCaptureCount: 1 } }))
    api.readProcessPane.mockImplementation(async () => ({ ...snapshot(), revision: '2', units: [unit([call])], blocks: [input] }))
    await render({ processBlocks: [publicBlock] })
    const rows = container.querySelectorAll('[data-web-fetch-row]')
    expect(rows).toHaveLength(1)
    const links = rows[0].querySelectorAll<HTMLButtonElement>('[data-tool-web-link]')
    expect(links).toHaveLength(1)
    const link = links[0]
    expect(link?.textContent).toBe(input.requestUrl)
    expect(container.querySelector('a[href]')).toBeNull()
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    await act(async () => link!.click())
    expect(api.openProcessWebLink).toHaveBeenCalledWith({ viewerId: viewer, blockId: input.blockId, inputDigest: input.inputDigest, role: 'request' })
    expect(JSON.stringify(api.openProcessWebLink.mock.calls)).not.toContain(input.requestUrl)
    await render({ processBlocks: [publicBlock], route: route('main', false) })
    expect(container.textContent).not.toContain(input.requestUrl)
  })
})


describe('private Questions through the actual Main and preload facade', () => {
  const privateQuestionSelectorV1 = (suffix = 'a') => {
    const requestId = 'input_' + suffix.repeat(64)
    return { threadId: 'question-thread', turnId: 'question-turn', itemId: 'item_' + requestId, requestId }
  }
  function privateQuestionDisplayV1(selector = privateQuestionSelectorV1(), label = 'CURRENT') {
    return { schemaVersion: 1, ...selector, status: 'submitted', answerState: 'available',
      questions: [{ id: selector.requestId + '_1', header: 'Header', question: label + '_PRIVATE_QUESTION', multiSelect: false, options: [] }],
      submission: { schemaVersion: 1, format: 'structured', answers: [{ id: selector.requestId + '_1', kind: 'answer',
        optionIds: [], customText: label + '_PRIVATE_ANSWER' }] } }
  }
  it('renders only fresh actual Main/preload Questions and clears a revoked late answer at the real consumer', async () => {
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    facadeTransport.expose.mockClear(); facadeTransport.invoke.mockReset(); facadeTransport.listeners.clear(); facadeTransport.mainHandlers.clear()
    const registration = vi.spyOn(document, 'addEventListener')
    vi.resetModules()
    // Runtime-only native imports keep Electron/Node implementation types out
    // of the renderer program. The window fixture is the only native cast;
    // this callable signature is narrower than the actual Main registration.
    const { EventEmitter } = await vi.importActual<{ EventEmitter: new () => {
      emit: (event: string, ...args: unknown[]) => boolean
    } }>('node:events')
    const frame = {}, sender = Object.assign(new EventEmitter(), { id: 592, mainFrame: frame,
      isDestroyed: () => false, send: vi.fn((channel: string, payload: unknown) => {
        for (const handler of facadeTransport.listeners.get(channel) ?? []) handler({}, payload)
      }) })
    const main = Object.assign(new EventEmitter(), { webContents: sender, isDestroyed: () => false,
      isVisible: () => true, isMinimized: () => false })
    // The existing afterEach unmounts the leaf before retiring native owners,
    // then removes only preload listeners captured from this document.
    facadeCleanups.push(() => { sender.emit('destroyed'); main.emit('closed') })
    const event = { sender, senderFrame: frame }
    type QuestionTransport = (path: string, body: string, signal?: AbortSignal) => Promise<{ ok: boolean; status: number; body: string }>
    const transport = vi.fn(async (_path: string, _body: string, _signal?: AbortSignal) =>
      ({ ok: true, status: 200, body: JSON.stringify(privateQuestionDisplayV1()) }))
    const native = await vi.importActual<{ registerUserInputLocalDisplayIpcV1: (
      getMainWindow: () => never, transport: QuestionTransport
    ) => void }>('../../../../../../main/ipc/user-input-local-display-ipc')
    native.registerUserInputLocalDisplayIpcV1(() => main as never, transport)
    facadeTransport.invoke.mockImplementation(async (channel: string, request?: unknown) => {
      const handler = facadeTransport.mainHandlers.get(channel)
      if (!handler) throw new Error('Unregistered private test action.')
      return handler(event, request)
    })
    await vi.importActual('../../../../../../preload/index')
    for (const [name, handler] of registration.mock.calls) if (name === 'visibilitychange') {
      facadeCleanups.push(() => document.removeEventListener(name, handler))
    }
    const exposed = facadeTransport.expose.mock.calls.find(([name]) => name === 'analytix')
    if (!exposed) throw new Error('The real preload did not expose its facade')
    const nativeApi = exposed[1]
    Object.defineProperty(window, 'analytix', { configurable: true, value: nativeApi })
    await i18n.changeLanguage('en')
    const renderQuestion = (selector: ReturnType<typeof privateQuestionSelectorV1>) => createElement(QuestionComposer, {
      selector, readOnly: true, block: { kind: 'user_input', id: selector.itemId, requestId: selector.requestId,
        status: 'submitted', questions: [], meta: { turnId: selector.turnId } } })
    const visibleText = () => container.textContent + [...container.querySelectorAll('textarea')].map(field => field.value).join('\n')
    await act(async () => root.render(renderQuestion(privateQuestionSelectorV1())))
    expect(visibleText()).toContain('CURRENT_PRIVATE_QUESTION')
    expect(visibleText()).toContain('CURRENT_PRIVATE_ANSWER')
    let finish!: (value: { ok: boolean; status: number; body: string }) => void
    transport.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const oldSelector = privateQuestionSelectorV1('b')
    await act(async () => root.render(renderQuestion(oldSelector)))
    expect(visibleText()).not.toContain('CURRENT_PRIVATE_QUESTION')
    const oldSignal = transport.mock.calls.at(-1)![2]!
    await act(async () => { await nativeApi.runtime.invalidateUserInputLocalDisplay() })
    expect(oldSignal.aborted).toBe(true)
    await act(async () => finish({ ok: true, status: 200, body: JSON.stringify(privateQuestionDisplayV1(oldSelector, 'LATE')) }))
    expect(visibleText()).not.toMatch(/CURRENT_PRIVATE_QUESTION|CURRENT_PRIVATE_ANSWER|LATE_PRIVATE_QUESTION|LATE_PRIVATE_ANSWER/)
    expect(transport).toHaveBeenCalledTimes(2)
    for (const [path, raw] of transport.mock.calls) {
      expect(path).toBe('/v1/local-display/user-input')
      expect(JSON.parse(raw)).not.toHaveProperty('viewId')
    }
    expect(facadeTransport.invoke.mock.calls.some(([channel]) => channel === 'runtime:request')).toBe(false)
  })
})

describe('actual protected Pane live reasoning, dispatch and preparation consumers', () => {
  const payload = (text: string): NonNullable<ProcessDisplayBlockV1['body']> => ({
    text, before: '', after: '', error: '', addedLines: 0, removedLines: 0, approximate: false
  })
  const reasoning = (sourceKind: 'native' | 'provider_reasoning', attempt = 1, state: 'running' | 'completed' = 'running',
    blockId = `live-${sourceKind}-${attempt}`, text = `LIVE_REASONING_BODY_${attempt}`): ProcessDisplayBlockV1 => ({
    blockId, kind: 'native_reasoning', state, attempt,
    source: sourceKind === 'native'
      ? { kind: 'native', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt, nativeIndex: 0, sourceOrdinal: String(attempt) }
      : { kind: 'provider_reasoning', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt, sourceOrdinal: String(attempt) },
    ...(sourceKind === 'native' ? { nativeIndex: 0 } : {}),
    title: 'Synthetic reasoning', preview: `LIVE_REASONING_PREVIEW_${attempt}`, durationMs: 1,
    truncated: false, classification: 'credential_redacted', body: payload(text),
    ...(state === 'completed' ? { captureDigest: 'd'.repeat(64) } : {})
  })
  const preparation = (state: 'preparing' | 'unknown' | 'withheld' = 'preparing', sourceOrdinal = '2'): ProcessDisplayBlockV1 => ({
    blockId: 'live-preparation', kind: 'tool_preparation', state, attempt: 1,
    source: { kind: 'tool_preparation', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt: 1, callId: 'call-preparation', sourceOrdinal },
    durationMs: 0, truncated: false, classification: state === 'preparing' ? 'credential_redacted' : 'withheld',
    ...(state === 'preparing' ? { toolName: 'write', path: '/synthetic/preparing.ts', inputUtf16CodeUnits: 1025 } : {})
  })
  // These sources use one actual Host observation counter within their unit.
  // The explicit ordinal is independent of arrival order, retry index or NativeIndex.
  const observedReasoning = (sourceOrdinal: string, blockId = 'live-provider_reasoning-1', text = 'LIVE_REASONING_BODY_1'): ProcessDisplayBlockV1 => {
    const value = reasoning('provider_reasoning', 1, 'running', blockId, text)
    if (value.source.kind !== 'provider_reasoning') throw new Error('fixture requires actual provider reasoning')
    return { ...value, source: { ...value.source, sourceOrdinal } }
  }
  const publicCall = (toolName: string, call: Pick<ProcessDisplayUnitV2['calls'][number], 'callId' | 'toolCallItemId'>,
    sourceItemId = call.toolCallItemId): Extract<ChatBlock, { kind: 'tool' }> => {
    const item: CoreTurnItemJson = { id: sourceItemId, threadId: 'thread', turnId: 'turn', role: 'assistant',
      status: 'running', createdAt: '2026-10-08T00:00:00Z', kind: 'tool_call', toolName, callId: call.callId, toolKind: 'tool_call',
      arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
        privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } }
    const result = chatBlockFromItem(item)
    if (result?.kind !== 'tool') throw new Error('fixture must reach the actual public mapper tool row')
    return result
  }
  let revision: number, current: ProcessDisplayBlockV1[], currentUnits: ProcessDisplayUnitV2[], expanded: Set<string>
  const closed = (blocks: ProcessDisplayBlockV1[]) => processDisplaySnapshotSchemaV1.parse({
    ...snapshot(false, 'thread', 'turn', String(revision)), units: currentUnits, blocks
  })
  const update = (blocks: ProcessDisplayBlockV1[], units = currentUnits) => {
    revision++; current = blocks; currentUnits = units
    // Validate the actual full fixture independently of the async Pane, so an
    // invalid fake DTO cannot make a negative consumer assertion pass by accident.
    closed(current)
  }
  const tick = async () => { await act(async () => vi.advanceTimersByTimeAsync(500)) }
  const privateSeat = (blockId: string) => [...container.querySelectorAll<HTMLElement>('[data-chat-seat]')]
    .find(seat => seat.querySelector(`[data-protected-process-block="${blockId}"]`))

  beforeEach(() => {
    revision = 1; current = []; currentUnits = [unit()]; expanded = new Set()
    api.authorizeProcessPane.mockImplementation(async request => ({ status: 'authorized', schemaVersion: 2,
      viewerId: viewer, revision: '1', scope: scope(request.threadId, request.turnId), presence: { nativeBlockCount: 0, toolCaptureCount: 0 } }))
    api.setProcessPaneExpanded.mockImplementation(async request => {
      expanded = new Set((request as { expandedBlockIds: string[] }).expandedBlockIds)
      return { status: 'ok' }
    })
    api.readProcessPane.mockImplementation(async () => closed(current.map(block => {
      if (expanded.has(block.blockId)) return block
      const { body: _body, input: _input, ...metadata } = block
      return metadata
    })))
  })

  it('uses actual preparation and reasoning titles while immediately retiring private header previews', async () => {
    const prep = preparation(), reason = observedReasoning('3', 'title-reasoning')
    const call = { callId: 'call-preparation', toolCallItemId: 'call-item-preparation', startedEventSeq: '12' }
    const publicRunning = publicCall('write', call)
    update([prep]); await render({ policy: presentationPolicyFor('standard') }); await tick()
    const privateGroupKey = 'unit:unit-a', publicGroupKey = 'public:' + publicRunning.id
    const groupKeys = () => [...container.querySelectorAll('[data-process-group]')].map(group => group.getAttribute('data-process-group')).sort()
    const header = (groupKey = privateGroupKey) => {
      expect(container.textContent).not.toContain(i18n.t('common:dshProcessUnavailable'))
      const headers = container.querySelectorAll<HTMLButtonElement>(`[data-process-group="${groupKey}"] > div:first-child > button`)
      expect(headers).toHaveLength(1)
      return headers[0]
    }
    expect(groupKeys()).toEqual([privateGroupKey])
    // Folded draft-only reads retain reopening identity, not fresh preparation.
    expect(header().textContent).toContain(i18n.t('common:dshTurnProcess'))
    expect(header().getAttribute('aria-expanded')).toBe('false')
    expect(header().querySelector('[data-shimmer]')).toBeNull()
    expect(header().textContent).not.toContain(i18n.t('common:dshStepProcess.preparing.tools'))
    expect(header().textContent).not.toContain('/synthetic/preparing.ts')
    await act(async () => header().click()); await tick()
    expect(header().textContent).toContain(i18n.t('common:dshStepProcess.preparing.write'))
    expect(header().textContent).toContain('/synthetic/preparing.ts')
    await act(async () => header().click())
    expect(header().textContent).not.toContain('/synthetic/preparing.ts')
    update([preparation('unknown'), reason], [unit([call])])
    await render({ policy: presentationPolicyFor('standard'), processBlocks: [publicRunning] })
    await tick()
    // Until a fresh Host unit admits its call anchor, the public tool owns a
    // separate standard-mode folded group and cannot drive the private unit.
    expect(groupKeys()).toEqual([privateGroupKey, publicGroupKey].sort())
    const publicSeat = container.querySelector(`[data-chat-seat="${publicRunning.id}"]`)
    expect(publicSeat).not.toBeNull()
    expect(publicSeat!.closest('[hidden]')?.getAttribute('hidden')).toBe('until-found')
    expect(publicSeat!.closest('[data-process-group]')?.getAttribute('data-process-group')).toBe(publicGroupKey)
    expect(header().closest('[data-process-group]')?.contains(publicSeat)).toBe(false)
    expect(header(publicGroupKey).getAttribute('aria-expanded')).toBe('false')
    expect(header(publicGroupKey).closest('[hidden]')).toBeNull(); expect(header(publicGroupKey).disabled).toBe(false)
    await act(async () => header(publicGroupKey).click()); await tick()
    expect(header(publicGroupKey).getAttribute('aria-expanded')).toBe('true')
    expect(publicSeat!.closest('[hidden]')).toBeNull()
    expect(header().closest('[data-process-group]')?.contains(publicSeat)).toBe(false)
    expect(container.querySelector('[data-protected-process-block="live-preparation"]')).toBeNull()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(container.textContent).not.toContain('LIVE_REASONING_PREVIEW_1')
    expect(header().textContent).toContain(i18n.t('common:dshTurnProcess'))
    expect(header().querySelector('[data-shimmer]')).toBeNull()
    expect(header().getAttribute('aria-expanded')).toBe('false')
    expect(header().closest('[hidden]')).toBeNull(); expect(header().disabled).toBe(false)
    await act(async () => header().click()); await tick()
    expect(groupKeys()).toEqual([publicGroupKey])
    expect(header(publicGroupKey).textContent).toContain(i18n.t('common:dshStepProcess.live.write'))
    expect(header(publicGroupKey).getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelector(`[data-chat-seat="${reason.blockId}"]`)?.closest('[data-process-group]')?.getAttribute('data-process-group')).toBe(publicGroupKey)
    expect(container.querySelector('[data-protected-process-block="live-preparation"]')).toBeNull()
    await act(async () => header(publicGroupKey).click())
    expect(header(publicGroupKey).getAttribute('aria-expanded')).toBe('false')
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(container.textContent).not.toContain('LIVE_REASONING_PREVIEW_1')
    const publicCompleted = { ...publicRunning, status: 'success' as const }
    await render({ policy: presentationPolicyFor('standard'), processBlocks: [publicCompleted] }); await tick()
    // Public completion does not refresh folded private reasoning state.
    expect(header(publicGroupKey).textContent).toContain(i18n.t('common:dshStepProcess.done.write'))
    expect(header(publicGroupKey).textContent).not.toContain(i18n.t('common:dshStepProcess.live.thinking'))
    expect(header(publicGroupKey).getAttribute('aria-expanded')).toBe('false')
    expect(header(publicGroupKey).querySelector('[data-shimmer]')).toBeNull()
    expect(header(publicGroupKey).textContent).not.toContain('LIVE_REASONING_PREVIEW_1')
    expect(header(publicGroupKey).closest('[hidden]')).toBeNull(); expect(header(publicGroupKey).disabled).toBe(false)
    await act(async () => header(publicGroupKey).click()); await tick()
    expect(header(publicGroupKey).textContent).toContain(i18n.t('common:dshStepProcess.live.thinking'))
    expect(header(publicGroupKey).textContent).toContain('LIVE_REASONING_PREVIEW_1')
    await act(async () => [...invalidators.values()][0]())
    expect(container.textContent).not.toContain('LIVE_REASONING_PREVIEW_1')
    await tick()
    expect(container.textContent).not.toContain('LIVE_REASONING_PREVIEW_1')
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(1)
  })
  it.each(['native', 'provider_reasoning'] as const)('keeps actual %s Markdown incremental until its terminal snapshot', async sourceKind => {
    const text = '**REASONING_START**\n\n$x$\n\nTail'
    const running = reasoning(sourceKind, 1, 'running', 'reasoning-markdown', text)
    update([running]); await render({ policy: presentationPolicyFor('verbose') })
    const seat = privateSeat(running.blockId)!
    await toggle(seat)
    expect(seat.querySelector('strong')?.textContent).toBe('REASONING_START')
    expect(seat.querySelector('.katex')).toBeNull(); expect(seat.textContent).toContain('$x$')
    update([reasoning(sourceKind, 1, 'completed', running.blockId, text)])
    await tick()
    expect(privateSeat(running.blockId)).toBe(seat)
    expect(seat.querySelector('.katex')).not.toBeNull()
    expect(seat.querySelector('a,img,button')).toBeNull()
  })

  it.each(['native', 'provider_reasoning'] as const)('expands actual running %s and settles the same seat while retiring the old selection', async sourceKind => {
    const running = reasoning(sourceKind), storedBlocks = useChatStore.getState().blocks
    update([running])
    await render({ policy: presentationPolicyFor('verbose') })
    const seat = privateSeat(running.blockId)
    expect(seat).toBeDefined()
    expect(seat!.querySelector('[data-disclosure-row]')?.getAttribute('aria-expanded')).toBe('false')
    expect(container.textContent).toContain('LIVE_REASONING_PREVIEW_1')
    expect(container.textContent).not.toContain('LIVE_REASONING_BODY_1')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    await toggle(seat!)
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [running.blockId] })
    const body = seat!.querySelector('[data-protected-process-body]')!
    expect(body.textContent).toContain('LIVE_REASONING_BODY_1')
    expect(body.querySelector('[data-markdown-view-only="true"]')).not.toBeNull()
    const selection = document.getSelection()!, range = document.createRange()
    range.selectNodeContents(body); selection.addRange(range)
    expect(selection.rangeCount).toBe(1)
    const selectedText = selection.toString()
    update([{ ...running, preview: 'UPDATED_RUNNING_PREVIEW' }]); await tick()
    expect(selection.rangeCount).toBe(1)
    expect(selection.toString()).toBe(selectedText)

    update([reasoning(sourceKind, 1, 'completed', running.blockId, 'SETTLED_REASONING_BODY')])
    await tick()
    expect(privateSeat(running.blockId)).toBe(seat)
    expect(seat!.textContent).toContain('SETTLED_REASONING_BODY')
    expect(seat!.textContent).not.toContain('LIVE_REASONING_BODY_1')
    expect(seat!.querySelector('[data-state]')?.getAttribute('data-state')).toBe('ok')
    expect(selection.rangeCount).toBe(0)
    expect(useChatStore.getState().blocks).toBe(storedBlocks)
    expect(finalization).not.toHaveBeenCalled()
    expect(api.openProcessCapture).not.toHaveBeenCalled()
  })

  it.each([true, false])('uses the actual shared sourceOrdinal across preparation and reasoning despite reversed arrival (preparation first: %s)', async preparationFirst => {
    const prep = preparation('preparing', preparationFirst ? '1' : '2'), reason = observedReasoning(preparationFirst ? '2' : '1')
    update(preparationFirst ? [reason, prep] : [prep, reason])
    await render({ policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    // Private descendants repeat ownership markers for transfer protection and
    // shimmer decoration. Count actual seats, never those repeated markers.
    const order = () => [...container.querySelectorAll<HTMLElement>('[data-chat-seat]')]
      .map(seat => seat.querySelector<HTMLElement>('[data-protected-process-block]')?.dataset.protectedProcessBlock)
    const expected = preparationFirst ? [prep.blockId, reason.blockId] : [reason.blockId, prep.blockId]
    expect(order()).toEqual(expected)
    update(preparationFirst ? [prep, reason] : [reason, prep]); await tick()
    expect(order()).toEqual(expected)
  })

  it.each([true, false])('uses the actual retired preparation ordinal around the exact public call (preparation first: %s)', async preparationFirst => {
    const prep = preparation('unknown', preparationFirst ? '1' : '2'), reason = observedReasoning(preparationFirst ? '2' : '1')
    const call = { callId: 'call-preparation', toolCallItemId: 'call-item-preparation', startedEventSeq: '12' }
    const publicBlock = publicCall('write', call)
    update(preparationFirst ? [reason, prep] : [prep, reason], [unit([call])])
    await render({ processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const seats = [...container.querySelectorAll<HTMLElement>('[data-chat-seat]')]
    expect(seats).toHaveLength(2)
    const publicSeat = seats.find(seat => seat.querySelector('[data-analytix-process-block-id]')?.getAttribute('data-analytix-process-block-id') === publicBlock.id)!
    const reasonSeat = privateSeat(reason.blockId)!
    expect(publicSeat).toBeDefined(); expect(reasonSeat).toBeDefined()
    expect(seats).toEqual(preparationFirst ? [publicSeat, reasonSeat] : [reasonSeat, publicSeat])
    expect(privateSeat(prep.blockId)).toBeUndefined()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
  })

  it('rehydrates a preparation-only unit after the whole process is folded and reopened', async () => {
    const prep = preparation(), policy = presentationPolicyFor('verbose')
    update([prep]); await render({ policy })
    expect(privateSeat(prep.blockId)?.textContent).toContain('/synthetic/preparing.ts')
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const readsBeforeFold = api.readProcessPane.mock.calls.length
    await render({ policy, processOpen: false })
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    await tick()
    expect(api.readProcessPane).toHaveBeenCalledTimes(readsBeforeFold)
    await render({ policy, processOpen: true })
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeFold)
    expect(privateSeat(prep.blockId)?.textContent).toContain('/synthetic/preparing.ts')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(api.authorizeProcessPane).toHaveBeenCalledOnce()
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it('rehydrates a preparation-only group after group folding clears all display metadata', async () => {
    const prep = preparation(), policy = presentationPolicyFor('compact')
    update([prep]); await render({ policy })
    const group = container.querySelector('[data-process-group]')!
    const groupToggle = group.querySelector<HTMLButtonElement>('button[data-process-activity]')!
    expect(groupToggle).not.toBeNull()
    expect(groupToggle.getAttribute('aria-expanded')).toBe('false')
    await act(async () => groupToggle.click())
    expect(privateSeat(prep.blockId)?.textContent).toContain('/synthetic/preparing.ts')
    await act(async () => groupToggle.click())
    const ordinaryReads = () => api.readProcessPane.mock.calls.filter(([request]) => !request.draftsOnly).length
    const beforeReopen = ordinaryReads()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    await tick()
    expect(ordinaryReads()).toBe(beforeReopen)
    await act(async () => groupToggle.click())
    expect(container.querySelector('[data-process-group]')).toBe(group)
    expect(ordinaryReads()).toBeGreaterThan(beforeReopen)
    expect(privateSeat(prep.blockId)?.textContent).toContain('/synthetic/preparing.ts')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(api.authorizeProcessPane).toHaveBeenCalledOnce()
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it('rejects a completed reasoning block replayed as running despite a higher revision', async () => {
    const settled = reasoning('provider_reasoning', 1, 'completed')
    update([settled]); await render({ policy: presentationPolicyFor('verbose') })
    expect(privateSeat(settled.blockId)).toBeDefined()
    update([{ ...reasoning('provider_reasoning', 1, 'running', settled.blockId, 'REPLAYED_RUNNING_BODY'), preview: 'REPLAYED_RUNNING_PREVIEW' }])
    await tick()
    expect(container.textContent).not.toContain('REPLAYED_RUNNING_BODY')
    expect(container.textContent).not.toContain('REPLAYED_RUNNING_PREVIEW')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(api.closeProcessPane).toHaveBeenCalled()
  })

  it('retires the previous reasoning attempt and rejects a later old running attempt', async () => {
    const old = reasoning('provider_reasoning')
    update([old]); await render({ policy: presentationPolicyFor('verbose') })
    await toggle(privateSeat(old.blockId)!)
    const range = document.createRange(); range.selectNodeContents(container.querySelector('[data-protected-process-body]')!)
    document.getSelection()!.addRange(range)
    update([reasoning('provider_reasoning', 2)])
    await tick()
    expect(container.textContent).toContain('LIVE_REASONING_PREVIEW_2')
    expect(container.textContent).not.toContain('LIVE_REASONING_BODY_1')
    expect(document.getSelection()?.rangeCount).toBe(0)
    expect(expanded.has(old.blockId)).toBe(false)
    update([{ ...old, preview: 'LATE_OLD_ATTEMPT_PREVIEW', body: payload('LATE_OLD_ATTEMPT_BODY') }])
    await tick()
    expect(container.textContent).not.toContain('LATE_OLD_ATTEMPT')
    expect(api.closeProcessPane).toHaveBeenCalled()
  })

  it.each(['provider_reasoning', 'tool_preparation'] as const)('pins immutable %s source coordinates across replay', async kind => {
    const original = kind === 'provider_reasoning' ? reasoning('provider_reasoning') : preparation()
    update([original]); await render({ policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    expect(privateSeat(original.blockId)).toBeDefined()
    const changed: ProcessDisplayBlockV1 = original.source.kind === 'provider_reasoning'
      ? { ...original, source: { ...original.source, sourceOrdinal: '9' }, preview: 'IDENTITY_CONFLICT_PREVIEW' }
      : original.source.kind === 'tool_preparation'
        ? { ...original, source: { ...original.source, callId: 'different-call' }, path: '/synthetic/identity-conflict.ts' }
        : original
    update([changed]); await tick()
    expect(container.textContent).not.toContain('IDENTITY_CONFLICT_PREVIEW')
    expect(container.textContent).not.toContain('/synthetic/identity-conflict.ts')
    expect(api.closeProcessPane).toHaveBeenCalled()
  })

  it('expands a real public Bash row to the dispatched command banner without an output body or exit', async () => {
    const call = { callId: 'call-live-shell', toolCallItemId: 'call-item-live-shell', startedEventSeq: '12' }
    const command: ProcessDisplayBlockV1 = { blockId: 'live-shell', kind: 'shell', state: 'running', attempt: 0,
      source: { kind: 'tool', unitId: 'unit-a', ...call }, title: 'synthetic command --local-only',
      durationMs: 1, truncated: false, classification: 'credential_redacted' }
    const publicBlock = publicCall('bash', call), before = JSON.stringify(publicBlock)
    update([command], [unit([call])])
    await render({ processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') })
    const row = container.querySelector<HTMLElement>('[data-tool-view="shell"] [data-disclosure-row]')
    expect(row).not.toBeNull()
    expect(row!.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelector('[data-terminal]')).toBeNull()
    await act(async () => row!.click())
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [command.blockId] })
    expect(row!.getAttribute('aria-expanded')).toBe('true')
    const terminal = container.querySelector<HTMLElement>('[data-tool-view="shell"] [data-terminal]')
    expect(terminal).not.toBeNull()
    expect(terminal!.textContent).toContain(command.title)
    expect(terminal!.hasAttribute('data-running')).toBe(true)
    expect(terminal!.hasAttribute('data-body')).toBe(false)
    expect(terminal!.textContent).not.toContain(i18n.t('common:protectedToolExit', { code: 0 }))
    expect(container.querySelector('[data-tool-io]')).toBeNull()
    expect(command.body).toBeUndefined(); expect(command.exitCode).toBeUndefined()
    expect(JSON.stringify(publicBlock)).toBe(before)
    expect(api.openProcessCapture).not.toHaveBeenCalled()
  })

  it.each(['todo_write', 'task', 'create_goal', 'get_goal', 'update_goal', 'list_jobs', 'bash_output', 'kill_shell', 'wait'])('delivers complete running %s input through the actual shared capture slot and retires on settlement', async name => {
    const call = { callId: 'call-raw', toolCallItemId: 'call-item-raw', startedEventSeq: '12' }
    const raw: ProcessDisplayBlockV1 = { blockId: 'live-raw', kind: 'tool_input', state: 'running', attempt: 0,
      source: { kind: 'tool', unitId: 'unit-a', ...call }, durationMs: 0, truncated: false, classification: 'source_exact_local',
      input: { availability: 'available', inputJSON: ' {\n"sequence":9007199254740993,"exponent":1e+03,"case":"Alice"\n} ' } }
    const publicBlock = publicCall(name, call)
    if (name === 'task') publicBlock.toolKind = 'subagent'
    if (['list_jobs', 'bash_output', 'kill_shell', 'wait'].includes(name)) publicBlock.toolKind = 'command_execution'
    const publicBefore = JSON.stringify(publicBlock)
    update([raw], [unit([call])]); await render({ processBlocks: [publicBlock], policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    expect(row.getAttribute('aria-expanded')).toBe('false')
    await act(async () => row.click())
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [raw.blockId] })
    expect(container.querySelector('[data-tool-raw-input]')?.textContent).toBe(raw.input?.availability === 'available' ? raw.input.inputJSON : undefined)
    expect(JSON.stringify(publicBlock)).toBe(publicBefore)
    if (raw.source.kind !== 'tool') throw new Error('Fixture must retain its actual tool source')
    update([{ ...raw, state: 'unknown', classification: 'withheld', input: undefined, source: { ...raw.source, kind: 'tool', toolResultItemId: 'result-item-raw' } }],
      [unit([{ ...call, toolResultItemId: 'result-item-raw' }])]); await tick()
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    expect(container.textContent).not.toContain('9007199254740993')
    expect(api.openProcessCapture).not.toHaveBeenCalled()
  })

  it('keeps Read on its path and typed presenter despite a shared private running input producer', async () => {
    const call = { callId: 'call-read-raw', toolCallItemId: 'call-item-read-raw', startedEventSeq: '12' }
    const raw: ProcessDisplayBlockV1 = { blockId: 'read-raw', kind: 'tool_input', state: 'running', attempt: 0,
      source: { kind: 'tool', unitId: 'unit-a', ...call }, durationMs: 0, truncated: false, classification: 'source_exact_local',
      input: { availability: 'available', inputJSON: '{"path":"PRIVATE_READ_ARGUMENT"}' } }
    update([raw], [unit([call])]); await render({ processBlocks: [publicCall('read', call)], policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-tool-view="read"]')).not.toBeNull()
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_READ_ARGUMENT')
    expect(expanded.size).toBe(0)
  })

  it('keeps preparation nonexpandable, consolidates only the exact anchor and continues polling to the real capture', async () => {
    const pending = preparation(), call = { callId: 'call-preparation', toolCallItemId: 'call-item-preparation', startedEventSeq: '12' }
    update([pending]); await render({ policy: presentationPolicyFor('verbose') })
    const prepSeat = privateSeat(pending.blockId)
    expect(prepSeat).toBeDefined()
    const prepRow = prepSeat!.querySelector<HTMLElement>('[data-disclosure-row]')!
    expect(prepRow.getAttribute('aria-expanded')).toBeNull()
    expect(prepRow.getAttribute('role')).toBeNull()
    expect(prepSeat!.textContent).toContain('/synthetic/preparing.ts')
    expect(prepSeat!.querySelector('[data-protected-input-size]')?.textContent).toBe('2KB')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    await act(async () => prepRow.click())
    expect(api.setProcessPaneExpanded.mock.calls.every(([request]) => (request as { expandedBlockIds: string[] }).expandedBlockIds.length === 0)).toBe(true)

    const wrong = publicCall('write', call, 'unrelated-public-call-item')
    await render({ processBlocks: [wrong], policy: presentationPolicyFor('verbose') })
    update([pending], [unit([call])]); await tick()
    expect(privateSeat(pending.blockId)).toBe(prepSeat)
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(2)
    const correct = publicCall('write', call), before = JSON.stringify(correct)
    await render({ processBlocks: [correct], policy: presentationPolicyFor('verbose') })
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
    expect(container.querySelector('[data-analytix-process-block-id]')?.getAttribute('data-analytix-process-block-id')).toBe(correct.id)
    expect(privateSeat(pending.blockId)).toBeUndefined()
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
    expect(JSON.stringify(correct)).toBe(before)

    // The hidden preparation identity is still a visibility/control owner for
    // this exact public seat. No preparation text is attached to the public
    // block, and its legitimate next poll must discover the real capture.
    const publicSeat = container.querySelector('[data-chat-seat]')!
    const settledCall = { ...call, toolResultItemId: 'result-item-preparation' }
    const actual: ProcessDisplayBlockV1 = { blockId: 'live-write-capture', kind: 'write', state: 'completed', attempt: 0,
      source: { kind: 'tool', unitId: 'unit-a', ...settledCall }, captureDigest: 'e'.repeat(64),
      preview: 'ACTUAL_CAPTURE_PREVIEW', path: '/synthetic/actual-capture.ts', durationMs: 1,
      truncated: false, classification: 'credential_redacted', body: payload('ACTUAL_CAPTURE_BODY') }
    update([preparation('unknown'), actual], [unit([settledCall])])
    const readsBeforeCapture = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeCapture)
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
    expect(container.querySelector('[data-chat-seat]')).toBe(publicSeat)
    expect(privateSeat(pending.blockId)).toBeUndefined()
    expect(privateSeat(actual.blockId)).toBe(publicSeat)
    expect(publicSeat.textContent).toContain('ACTUAL_CAPTURE_PREVIEW')
    expect(publicSeat.textContent).not.toContain('/synthetic/preparing.ts')
    expect(container.textContent).not.toContain('ACTUAL_CAPTURE_BODY')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()

    const actualRow = publicSeat.querySelector<HTMLElement>('[data-analytix-process-block-id] [data-disclosure-row]')!
    await act(async () => actualRow.click())
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [actual.blockId] })
    expect(publicSeat.querySelector('[data-protected-process-body] [data-tool-io]')?.textContent).toContain('ACTUAL_CAPTURE_BODY')
    expect(publicSeat.textContent).not.toContain('/synthetic/preparing.ts')
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
    expect(JSON.stringify(correct)).toBe(before)
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it.each(['unknown', 'withheld'] as const)('does not display %s preparation or restore it from a later active replay', async state => {
    const retired = preparation(state)
    // A legitimate visible sibling keeps the ordinary read lane active. This
    // proves the retired-source fence receives and rejects the replay, rather
    // than passing because an entirely invisible pane stopped polling.
    const sibling = reasoning('provider_reasoning')
    update([retired, sibling]); await render({ policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    expect(privateSeat(sibling.blockId)).toBeDefined()
    const readsBeforeReplay = api.readProcessPane.mock.calls.length
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
    expect(container.querySelector('[data-protected-process-block="live-preparation"]')).toBeNull()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    update([preparation(), sibling]); await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeReplay)
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(container.querySelector('[data-protected-input-size]')).toBeNull()
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(api.closeProcessPane).toHaveBeenCalled()
  })
  it('inserts a late earlier preparation before an admitted reasoning seat without replacing its DOM or selection', async () => {
    const reason = observedReasoning('2', 'late-origin-reasoning', 'LATE_ORIGIN_REASONING_BODY')
    update([reason]); await render({ policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const reasonSeat = privateSeat(reason.blockId)!, parent = reasonSeat.parentElement
    await toggle(reasonSeat)
    const body = reasonSeat.querySelector('[data-protected-process-body]')!
    const selection = document.getSelection()!, range = document.createRange()
    range.selectNodeContents(body); selection.addRange(range)
    const selectedText = selection.toString()
    expect(selectedText).toContain('LATE_ORIGIN_REASONING_BODY')

    const prep = preparation('preparing', '1')
    // Transport still delivers the old reasoning first. Only the actual shared
    // observation ordinal proves the earlier position of the late named tool.
    update([reason, prep]); const readsBeforeLateOrigin = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeLateOrigin)
    const prepSeat = privateSeat(prep.blockId)!
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([prepSeat, reasonSeat])
    expect(privateSeat(reason.blockId)).toBe(reasonSeat)
    expect(reasonSeat.parentElement).toBe(parent)
    expect(prepSeat.parentElement).toBe(parent)
    expect(reasonSeat.querySelector('[data-protected-process-body]')).toBe(body)
    expect(selection.rangeCount).toBe(1)
    expect(selection.toString()).toBe(selectedText)
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it('refines D then R through late P and actual C in the same public parent, and retains it through capture handoff and whole fold', async () => {
    const d = publicTool('d'), dCall = anchor('d'), cCall = { callId: 'call-preparation', toolCallItemId: 'call-item-preparation', startedEventSeq: '12' }
    const c = publicCall('write', cCall), beforeD = JSON.stringify(d), beforeC = JSON.stringify(c)
    const actualUnit: ProcessDisplayUnitV2 = { ...unit(), placement: 'anchored', precedingCalls: [dCall] }
    const reason = observedReasoning('2', 'after-D-reasoning'), prep = preparation('preparing', '1')
    const policy = presentationPolicyFor('verbose')
    update([reason], [actualUnit]); await render({ processBlocks: [d], policy })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const dSeat = container.querySelector('[data-chat-seat]')!, reasonSeat = privateSeat(reason.blockId)!, parent = reasonSeat.parentElement
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([dSeat, reasonSeat])
    expect(dSeat.parentElement).toBe(parent)

    update([reason, prep]); await tick()
    const prepSeat = privateSeat(prep.blockId)!
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([dSeat, prepSeat, reasonSeat])
    expect(prepSeat.parentElement).toBe(parent)
    expect(privateSeat(reason.blockId)).toBe(reasonSeat)
    expect(reasonSeat.parentElement).toBe(parent)

    const retired = preparation('unknown', '1'), dispatchedUnit = { ...actualUnit, calls: [cCall] }
    update([reason, retired], [dispatchedUnit])
    await render({ processBlocks: [d, c], policy })
    const readsBeforeDispatch = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeDispatch)
    const cSeat = [...container.querySelectorAll('[data-chat-seat]')]
      .find(seat => seat.querySelector('[data-analytix-process-block-id]')?.getAttribute('data-analytix-process-block-id') === c.id)!
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([dSeat, cSeat, reasonSeat])
    expect(cSeat.parentElement).toBe(parent)
    expect(reasonSeat.parentElement).toBe(parent)
    expect(privateSeat(reason.blockId)).toBe(reasonSeat)
    expect(privateSeat(prep.blockId)).toBeUndefined()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(JSON.stringify(d)).toBe(beforeD); expect(JSON.stringify(c)).toBe(beforeC)

    const settledCall = { ...cCall, toolResultItemId: 'result-item-after-D' }
    const capture: ProcessDisplayBlockV1 = { blockId: 'after-D-capture', kind: 'write', state: 'completed', attempt: 0,
      source: { kind: 'tool', unitId: 'unit-a', ...settledCall }, captureDigest: 'f'.repeat(64),
      preview: 'AFTER_D_CAPTURE_PREVIEW', path: '/synthetic/after-D-capture.ts', durationMs: 1,
      truncated: false, classification: 'credential_redacted', body: payload('AFTER_D_CAPTURE_BODY') }
    update([capture, reason, retired], [{ ...actualUnit, calls: [settledCall] }]); await tick()
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([dSeat, cSeat, reasonSeat])
    expect(privateSeat(capture.blockId)).toBe(cSeat)
    expect(cSeat.textContent).toContain('AFTER_D_CAPTURE_PREVIEW')
    expect(container.textContent).not.toContain('AFTER_D_CAPTURE_BODY')
    const cRow = cSeat.querySelector<HTMLElement>('[data-analytix-process-block-id] [data-disclosure-row]')!
    await act(async () => cRow.click())
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [capture.blockId] })
    expect(cSeat.querySelector('[data-protected-process-body] [data-tool-io]')?.textContent).toContain('AFTER_D_CAPTURE_BODY')

    await render({ processBlocks: [d, c], policy, processOpen: false })
    expect(container.textContent).not.toContain('AFTER_D_CAPTURE_BODY')
    expect(container.textContent).not.toContain('LIVE_REASONING_PREVIEW_1')
    const readsWhileFolded = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane).toHaveBeenCalledTimes(readsWhileFolded)
    await render({ processBlocks: [d, c], policy, processOpen: true })
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsWhileFolded)
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([dSeat, cSeat, reasonSeat])
    expect(reasonSeat.parentElement).toBe(parent)
    expect(cSeat.parentElement).toBe(parent)
    expect(container.textContent).toContain('LIVE_REASONING_PREVIEW_1')
    expect(container.textContent).not.toContain('AFTER_D_CAPTURE_BODY')
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(JSON.stringify(d)).toBe(beforeD); expect(JSON.stringify(c)).toBe(beforeC)
    expect(api.authorizeProcessPane).toHaveBeenCalledOnce()
    expect(api.closeProcessPane).not.toHaveBeenCalled()
  })

  it('refines a reasoning fallback before an already mounted C to after C without replacing its node or permanent parent', async () => {
    const call = { callId: 'call-preparation', toolCallItemId: 'call-item-preparation', startedEventSeq: '12' }
    const c = publicCall('write', call), before = JSON.stringify(c), reason = observedReasoning('2', 'mounted-C-reasoning')
    update([reason], [unit([call])]); await render({ processBlocks: [c], policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const reasonSeat = privateSeat(reason.blockId)!, parent = reasonSeat.parentElement
    const cSeat = [...container.querySelectorAll('[data-chat-seat]')].find(seat => seat !== reasonSeat)!
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([reasonSeat, cSeat])
    expect(cSeat.parentElement).toBe(parent)
    update([reason, preparation('unknown', '1')]); const readsBeforeBoundary = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeBoundary)
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([cSeat, reasonSeat])
    expect(privateSeat(reason.blockId)).toBe(reasonSeat)
    expect(reasonSeat.parentElement).toBe(parent)
    expect(cSeat.parentElement).toBe(parent)
    expect(privateSeat('live-preparation')).toBeUndefined()
    expect(container.textContent).not.toContain('/synthetic/preparing.ts')
    expect(JSON.stringify(c)).toBe(before)
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    // Selection continuity when an existing DOM node moves across C remains a
    // browser/native observation seam; this case proves identity and ownership.
  })

  it('rejects preparation origin ordinal drift in an otherwise valid later DTO', async () => {
    const original = preparation('preparing', '2')
    update([original]); await render({ policy: presentationPolicyFor('verbose') })
    expect(privateSeat(original.blockId)).toBeDefined()
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const changed = { ...preparation('preparing', '3'), path: '/synthetic/changed-origin.ts' }
    update([changed]); const readsBeforeDrift = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeDrift)
    expect(container.textContent).not.toContain('/synthetic/changed-origin.ts')
    expect(api.closeProcessPane).toHaveBeenCalled()
  })

  it('places a fresh earlier P before R after the real preceding D in one public parent', async () => {
    const d = publicTool('d'), beforeD = JSON.stringify(d), reason = observedReasoning('2', 'fresh-after-D-reasoning')
    const prep = preparation('preparing', '1')
    const actualUnit: ProcessDisplayUnitV2 = { ...unit(), placement: 'anchored', precedingCalls: [anchor('d')] }
    // The first accepted reply is already mixed, and arrival deliberately puts
    // R before P. Neither a prior R placement nor array order proves the axis.
    update([reason, prep], [actualUnit])
    await render({ processBlocks: [d], policy: presentationPolicyFor('verbose') })
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    const dSeat = container.querySelector('[data-analytix-process-block-id="public-d"]')!.closest('[data-chat-seat]')!
    const prepSeat = privateSeat(prep.blockId)!, reasonSeat = privateSeat(reason.blockId)!
    expect(prepSeat).toBeDefined(); expect(reasonSeat).toBeDefined()
    expect([...container.querySelectorAll('[data-chat-seat]')]).toEqual([dSeat, prepSeat, reasonSeat])
    expect(prepSeat.parentElement).toBe(dSeat.parentElement)
    expect(reasonSeat.parentElement).toBe(dSeat.parentElement)
    expect(container.querySelectorAll('[data-process-group]')).toHaveLength(1)
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(JSON.stringify(d)).toBe(beforeD)
  })

  it('rejects a cross-poll origin ordinal collision even when each reply is individually schema-valid', async () => {
    const prep = preparation('preparing', '2')
    update([prep]); await render({ policy: presentationPolicyFor('verbose') })
    expect(privateSeat(prep.blockId)).toBeDefined()
    expect(api.closeProcessPane).not.toHaveBeenCalled()
    // The new reply contains only R2, so the reply-local schema has no duplicate.
    // The mounted leaf must also check the previously admitted P2 source pin.
    const reason = observedReasoning('2', 'cross-poll-reasoning', 'COLLIDING_REASONING_BODY')
    update([{ ...reason, preview: 'COLLIDING_REASONING_PREVIEW' }])
    const readsBeforeCollision = api.readProcessPane.mock.calls.length
    await tick()
    expect(api.readProcessPane.mock.calls.length).toBeGreaterThan(readsBeforeCollision)
    expect(container.textContent).not.toContain('COLLIDING_REASONING_PREVIEW')
    expect(container.textContent).not.toContain('COLLIDING_REASONING_BODY')
    expect(container.querySelector('[data-protected-process-body]')).toBeNull()
    expect(api.closeProcessPane).toHaveBeenCalled()
  })

})
