// @vitest-environment jsdom
import { parse } from 'postcss'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider, ChatBlock, RuntimeChildMetadata, ThreadEventSink, ToolBlock } from '../../../../agent/types'
import { AnalytixRuntimeProvider } from '../../../../agent/analytix-runtime'
import { buildThreadEventSink } from '../../../../store/chat-store-runtime'
import { createSideActions, teardownAllSideSubscriptions } from '../../../../store/chat-store-side-actions'
import { appendActiveStreamDeltas, clearActiveStream, pendingActiveStreamTurnId } from '../../../../thread/streaming/active-stream-store'
import type { ThreadTurn } from '../../../../thread/projection/thread-turns'
import { deriveTurnSections } from '../../../../thread/projection/derive-turn-sections'
import { useChatStore } from '../../../../store/chat-store'
import i18n from '../../../../i18n'
import { ChatView } from './ChatView'
import { ToolRow } from './ToolRow'
import { ToolCallTree } from './ToolCallTree'
import { groupProcessSections, presentationPolicyFor } from './chat-view-model'
import { reasoningPreview } from './ReasoningRow'
import { readWorkDetailsPreference, writeWorkDetailsPreference } from '../../../../lib/chat-presentation-preferences'
import { MessageTimeline } from '../../MessageTimeline'
import { chatBlockFromItem, createOrdinaryHistoryReplay, dispatchAnalytixRuntimeEvents, generalTerminalProjectionBatchFromRuntime, normalizeChildMetadataForRenderer, threadFromCore } from '../../../../agent/analytix-mapper'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../../../../agent/runtime-client'
import type { CoreRuntimeEventJson, CoreThreadJson, CoreThreadSummarySubagentJson, CoreTurnItemJson } from '../../../../agent/analytix-contract'
import type { ToolResultLocalViewResultV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ChatStoreGet, ChatStoreSet, SideConversation } from '../../../../store/chat-store-types'
import { createMaintenanceActions } from '../../../../store/chat-store-maintenance-actions'
import { createThreadActions } from '../../../../store/chat-store-thread-actions'
import { invalidateThreadDetailCache } from '../../../../lib/thread-detail-cache'
import { readThreadForkRegistry } from '../../../../lib/thread-fork-registry'
import { SideConversationPanel } from '../../SideConversationPanel'
import { InputBar } from '../../InputBar'
import { loadThreadUsageData } from '../../../../hooks/use-thread-usage'
import { ThreadUsageResponseSchema, type ConversationUsageV1, type ProviderBillingV1 } from '../../../../../../../packages/runtime/src/contracts/usage'
import { SubagentInspectorPanel } from '../../../summary/SubagentInspectorPanel'
import bashHeaderCss from './toolviews/bash-row.module.css'
import toolRowCss from './ToolRow.module.css'
import disclosureCss from '../DisclosureRow.module.css'
const registryFixture = vi.hoisted(() => ({ provider: null as AgentProvider | null }))
vi.mock('../../../../agent/registry', () => ({ getProvider: () => registryFixture.provider ?? ({ getThreadDetail: async () => ({ authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} }) }) }))
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>
const user: Extract<ChatBlock, { kind: 'user' }> = { kind: 'user', id: 'user', text: 'Synthetic request', meta: { turnId: 'turn' } }
const tool: ToolBlock = { kind: 'tool', id: 'tool', summary: 'read public fixture', detail: 'Public detail text', status: 'success', meta: { toolName: 'read', turnId: 'turn' } }
function footerConversation(turnId: string, inputTokens = 220, outputTokens = 35): ConversationUsageV1 {
  const count = (reportedValue: number): ProviderBillingV1['inputTokens'] => ({
    reportedValue, knownAttemptCount: 2, attemptCount: 2, complete: true
  })
  const billing: ProviderBillingV1 = {
    inventoryComplete: true, closed: true, attemptCount: 2, settledAttemptCount: 2,
    inputTokens: count(inputTokens), outputTokens: count(outputTokens),
    uncachedInputTokens: count(inputTokens), cacheReadTokens: count(0), cacheWriteTokens: count(0),
    reasoningTokens: count(0), totalTokens: count(inputTokens + outputTokens)
  }
  return { schemaVersion: 'conversation-usage.v1', billing, turns: [{ turnId, billing }],
    metrics: { turnCount: 1, stepCount: 2,
      llmMs: { reportedValue: 1000, knownStepCount: 2, complete: true },
      toolMs: { reportedValue: 0, knownCallCount: 0, callCount: 0, complete: true },
      ttftMs: 400, ttftSteps: 1, decodeMs: 1000, decodeTokens: 10, decodeSteps: 1 } }
}
function normalFooterUsageResponse(threadId: string, conversationUsage: ConversationUsageV1 | null): ForkRequestResult {
  // Deliberately contradictory retained legacy total3; normal Pills must show
  // the canonical255 (and not physical-attempt legacy150 or LastResult3).
  const counters = { input_tokens: 3, output_tokens: 0, reasoning_tokens: 0,
    cached_tokens: 0, cache_miss_tokens: 3, total_tokens: 3,
    cost_usd: 0, cost_cny: 0, price_configured: false,
    cache_savings_usd: 0, cache_savings_cny: 0, token_economy_savings_tokens: 0,
    token_economy_savings_usd: 0, token_economy_savings_cny: 0,
    turns: 0, cache_hit_rate: null }
  const value = ThreadUsageResponseSchema.parse({ group_by: 'thread',
    buckets: [{ ...counters, thread_id: threadId,
      ...(conversationUsage ? { conversationUsage } : {}) }],
    totals: { ...counters, thread_count: 1 } })
  // Execute against the official Core contract combination. Fail loudly if an
  // older envelope schema strips the canonical field instead of accepting it.
  if (conversationUsage) expect(value.buckets[0]).toMatchObject({ conversationUsage })
  return { ok: true, status: 200, body: JSON.stringify(value) }
}
function historicalFooterFixture(threadId: string) {
  const timestamp = '2026-10-08T00:00:00Z', text = 'Restored ordinary answer'
  const item = { id: 'closing', threadId, turnId: 'turn', role: 'assistant',
    kind: 'assistant_text', status: 'completed', createdAt: timestamp, finishedAt: timestamp, text,
    ordinaryResult: { schemaVersion: 1, purpose: 'analytix.ordinary-result/v1',
      projectionVersion: 'analytix.ordinary-output-projection/v1', logicalEffect: 'ordinary', ordinaryWork: true,
      candidateOrigin: 'provider_ordinary_only', evidenceAuthority: false, citationAuthority: false,
      factAnswerAllowed: false, text, textSha256: '6'.repeat(64), resultDigest: '7'.repeat(64) } }
  const batch = { schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch', batchDigest: 'a'.repeat(64), threadId, turnId: 'turn',
    seq: 13, firstSeq: 11, lastSeq: 13, timestamp, generalTerminalCommitId: 'b'.repeat(64),
    generalTerminalAuthorityKind: 'general_terminal_cas', generalTerminalAuthorityDigest: 'c'.repeat(64),
    eventManifestDigest: 'd'.repeat(64), projectedEventsDigest: 'e'.repeat(64), transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
    events: [
      { kind: 'item_completed', seq: 11, timestamp, threadId, turnId: 'turn', itemId: 'closing', item },
      { kind: 'usage', seq: 12, timestamp, threadId, turnId: 'turn', model: 'public-model',
        usage: { promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
          cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
          cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0, priceConfigured: false,
          cacheSavingsUsd: 0, cacheSavingsCny: 0, tokenEconomySavingsTokens: 0, turns: 1 },
        cacheDiagnostics: { providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1', providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1, providerAttemptCount: 2,
          providerAttemptStatuses: { succeeded: 2, failed: 0, cancelled: 0, timedOut: 0, streamAborted: 0 },
          providerAttemptInputTokens: { complete: true, knownObservationCount: 2, observationCount: 2, value: 120 },
          providerAttemptOutputTokens: { complete: true, knownObservationCount: 2, observationCount: 2, value: 30 },
          providerAttemptCacheHitTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
          providerAttemptCacheMissTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
          providerAttemptCacheRate: { known: false, numerator: 0, denominator: 0 } },
        usageFinalStatus: 'completed' },
      { kind: 'turn_completed', seq: 13, timestamp, threadId, turnId: 'turn',
        status: 'completed', terminalReason: 'success' }
    ], eventManifest: [
      { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
    ] }
  return { timestamp, text, batch }
}
async function render(turn: ThreadTurn, props: Partial<Parameters<typeof ChatView>[0]> = {}) {
  const processing = props.processing ?? false
  await act(async () => root.render(createElement(ChatView, { threadId: 'thread', turnId: 'turn', turn,
    sections: deriveTurnSections({ turn, isProcessing: processing, liveProcessText: '', liveContent: '', workspaceRoot: '' }), processing, ...props })))
}
function accessibleText(node: Element): string {
  const content = node.cloneNode(true) as Element
  content.querySelectorAll('[aria-hidden="true"], [inert]').forEach(decoration => decoration.remove())
  return content.textContent ?? ''
}
async function providerStageFixture(surface: 'main' | 'side', threadId: string, turnId: string) {
  const provider = new AnalytixRuntimeProvider()
  vi.spyOn(provider, 'getThreadDetail').mockResolvedValue({ blocks: [], latestSeq: 0, threadStatus: 'idle' })
  invalidateThreadDetailCache(threadId)
  registryFixture.provider = provider
  const opening: Extract<ChatBlock, { kind: 'user' }> = { ...user, meta: { turnId } }
  useChatStore.setState({ activeThreadId: surface === 'main' ? threadId : 'parent-thread', blocks: [opening],
    runtimeConnection: 'ready', busy: true, currentTurnId: turnId, currentTurnUserId: opening.id, lastSeq: 0,
    liveAssistant: '', queuedMessages: [], sideConversations: {},
    sidePanel: { ...useChatStore.getState().sidePanel, open: false, activeSideId: null } })
  let sink: ThreadEventSink
  if (surface === 'main') sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId })
  else {
    let captured: ThreadEventSink | undefined
    vi.spyOn(provider, 'subscribeThreadEvents').mockImplementation((_owner, _seq, incoming, signal) => {
      captured = incoming
      return new Promise<void>(resolve => signal.addEventListener('abort', () => resolve(), { once: true }))
    })
    const actions = createSideActions({ set: useChatStore.setState, get: useChatStore.getState, getProvider: () => provider,
      t: key => i18n.t(key), formatRuntimeError: error => String(error), shouldOpenSettingsForError: () => false })
    expect(await actions.openThreadInSidePanel(threadId)).toBe(true)
    expect(captured).toBeDefined(); sink = captured!
    useChatStore.setState(state => ({ sideConversations: { ...state.sideConversations,
      [threadId]: { ...state.sideConversations[threadId], blocks: [opening], lastSeq: 0, busy: true, turnId, userItemId: opening.id } } }))
  }
  const blocks = () => surface === 'main' ? useChatStore.getState().blocks : useChatStore.getState().sideConversations[threadId].blocks
  const setBlocks = (next: ChatBlock[]) => surface === 'main' ? useChatStore.setState({ blocks: next })
    : useChatStore.setState(state => ({ sideConversations: { ...state.sideConversations,
      [threadId]: { ...state.sideConversations[threadId], blocks: next } } }))
  return { blocks, setBlocks,
    emit: async (event: CoreRuntimeEventJson) => {
      await act(async () => dispatchAnalytixRuntimeEvents([event], sink, async () => undefined))
    },
    draw: async () => render({ user: opening, blocks: blocks().filter(block => block.id !== opening.id) },
      { threadId, turnId, processing: surface === 'main' ? useChatStore.getState().busy : useChatStore.getState().sideConversations[threadId].busy }),
    close: async () => {
      await act(async () => root.render(null)); teardownAllSideSubscriptions(); clearActiveStream(threadId)
      invalidateThreadDetailCache(threadId); registryFixture.provider = null
    }
  }
}
async function processScrollFixture(running = true, initialHeight = 600) {
  const observers = new Set<ProcessObserver>()
  class ProcessObserver implements ResizeObserver {
    readonly targets = new Set<Element>()
    constructor(readonly callback: ResizeObserverCallback) { observers.add(this) }
    observe(target: Element): void { this.targets.add(target) }
    unobserve(target: Element): void { this.targets.delete(target) }
    disconnect(): void { observers.delete(this) }
  }
  vi.stubGlobal('ResizeObserver', ProcessObserver)
  let blocks: ToolBlock[] = [{ ...tool, id: 'live-a', status: running ? 'running' : 'success' }, { ...tool, id: 'live-b', status: running ? 'running' : 'success' }]
  let props = { blocks, processing: running, policy: presentationPolicyFor('compact'), visible: true }
  await act(async () => root.render(createElement(ToolCallTree, props)))
  const body = container.querySelector<HTMLElement>('[data-step-process-body]')!
  const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child button')!
  let position = 0, height = initialHeight, target: number | null = null, reads = 0
  const clamp = (value: number) => Math.max(0, Math.min(Math.max(0, height - 100), value))
  const scrollTo = vi.fn((options: ScrollToOptions) => {
    if (options.behavior === 'smooth') target = options.top ?? position
    else { target = null; position = clamp(options.top ?? position) }
  })
  Object.defineProperties(body, {
    scrollHeight: { configurable: true, get: () => { reads++; return body.hidden ? 0 : height } },
    clientHeight: { configurable: true, get: () => { reads++; return body.hidden ? 0 : 100 } },
    scrollTop: { configurable: true, get: () => { reads++; return position }, set: (value: number) => { position = clamp(value) } },
    scrollTo: { configurable: true, value: scrollTo }
  })
  return {
    body, header, scrollTo, observers,
    get position() { return position }, get target() { return target }, get reads() { return reads },
    async open() { await act(async () => header.click()) },
    async resize(nextHeight = height) {
      height = nextHeight; position = clamp(position)
      await act(async () => { for (const observer of observers) if (observer.targets.has(body)) observer.callback([], observer) })
    },
    async move(top: number) { position = clamp(top); await act(async () => body.dispatchEvent(new Event('scroll', { bubbles: true }))) },
    async finish(top = target ?? position) {
      position = clamp(top); target = null
      await act(async () => { body.dispatchEvent(new Event('scroll', { bubbles: true })); body.dispatchEvent(new Event('scrollend')) })
    },
    async update(next: Partial<typeof props>) { props = { ...props, ...next }; await act(async () => root.render(createElement(ToolCallTree, props))) },
    async settle() { blocks = blocks.map<ToolBlock>(block => ({ ...block, status: 'success' })); props = { ...props, blocks, processing: false }; await act(async () => root.render(createElement(ToolCallTree, props))) }
  }
}
beforeEach(async () => { clearActiveStream(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.spyOn(rendererRuntimeClient, 'onRuntimeStatus').mockReturnValue(() => undefined); await i18n.changeLanguage('zh'); previous = useChatStore.getState(); useChatStore.setState({ activeThreadId: 'thread', route: 'chat' }); container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); clearActiveStream(); container.remove(); useChatStore.setState(previous); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
describe('runtime turn ownership through the real mapper, steering action and mounted Timeline', () => {
  let previousBridge: PropertyDescriptor | undefined, previousMode: ReturnType<typeof readWorkDetailsPreference>
  let scrollDescriptor: PropertyDescriptor | undefined
  const timestamp = '2026-10-08T00:00:00Z'
  const map = (item: Partial<CoreTurnItemJson> & Pick<CoreTurnItemJson, 'id' | 'role' | 'kind'>) => {
    const block = chatBlockFromItem({ threadId: 'thread', turnId: 'turn', status: 'completed', createdAt: timestamp, ...item })
    expect(block).not.toBeNull()
    return block!
  }
  const mappedUser = () => map({ id: 'opening', role: 'user', kind: 'user_message', text: 'Opening request' })
  const mappedTools = (prefix: string) => [1, 2].map(index => map({ id: `${prefix}-item-${index}`, role: 'tool', kind: 'tool_call',
    toolName: 'read', callId: `${prefix}-call-${index}` }))
  async function draw(live = 'Synthetic live answer', status?: 'completed' | 'failed' | 'aborted') {
    const state = useChatStore.getState()
    await act(async () => root.render(createElement(MessageTimeline, { blocks: state.blocks, live,
      activeThreadId: 'thread', runtimeConnection: 'ready', onRetryConnection() {}, onOpenSettings() {}, conversationUsage: null,
      edgeAlignedScroll: true, processPaneRoute: { surface: 'main', ownerKey: 'steering-fixture', selectionKey: 'thread', visible: true },
      runtimeStateOverride: status ? { busy: state.busy, currentTurnId: state.currentTurnId, currentTurnUserId: state.currentTurnUserId,
        authoritativeTurnStatuses: { turn: status } } : undefined })))
  }
  const processHeaders = () => [...container.querySelectorAll<HTMLButtonElement>('[data-process-group] > div:first-child button')]
  const assertOneOwner = () => {
    expect(container.querySelectorAll('[data-analytix-chat-view]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-process-pane-id]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-analytix-assistant-response-seat]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-analytix-chat-view] > [data-actions-reveal]')).toHaveLength(1)
  }
  beforeEach(() => {
    previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix'); previousMode = readWorkDetailsPreference()
    scrollDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1)); vi.stubGlobal('cancelAnimationFrame', vi.fn())
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(normalizeAppSettings({} as AppSettingsV1))
    writeWorkDetailsPreference('detailed')
    useChatStore.setState({ activeThreadId: 'thread', workspaceRoot: '', runtimeConnection: 'ready', busy: true,
      currentTurnId: 'turn', currentTurnUserId: 'opening', blocks: [mappedUser(), ...mappedTools('before')],
      liveAssistant: '', lastSeq: 10, queuedMessages: [], watchTurnCompletion: {},
      turnStartedAtByUserId: {}, turnDurationByUserId: {}, turnUsageById: {},
      refreshThreads: vi.fn(async () => undefined), drainQueuedMessages: vi.fn(async () => undefined) })
  })
  afterEach(async () => {
    await act(async () => root.render(null)); clearActiveStream('thread'); registryFixture.provider = null
    writeWorkDetailsPreference(previousMode)
    if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge); else Reflect.deleteProperty(window, 'analytix')
    if (scrollDescriptor) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', scrollDescriptor)
    else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
  })
  it('keeps the same active seat through pending/admitted steering and one committed completed tail', async () => {
    const authorize = vi.fn(async () => ({ status: 'authorized', schemaVersion: 2, viewerId: crypto.randomUUID(), revision: '1',
      scope: privateScope, presence: { nativeBlockCount: 0, toolCaptureCount: 0 } }))
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { authorizeProcessPane: authorize,
      readProcessPane: vi.fn(async () => ({ status: 'snapshot', schemaVersion: 2, revision: '1', scope: privateScope, units: [], blocks: [] })),
      closeProcessPane: vi.fn(async () => ({ status: 'ok' })), setProcessPaneExpanded: async () => ({ status: 'ok' }),
      onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined } } })
    let releaseSteer!: (result: Awaited<ReturnType<NonNullable<AgentProvider['steerUserMessage']>>>) => void
    const steerUserMessage = vi.fn<NonNullable<AgentProvider['steerUserMessage']>>(() => new Promise(resolve => { releaseSteer = resolve }))
    registryFixture.provider = { steerUserMessage, getThreadDetail: async () => ({
      blocks: useChatStore.getState().blocks, latestSeq: useChatStore.getState().lastSeq,
      authoritativeTurnTimings: {}, authoritativeTurnStatuses: { turn: 'running' }
    }) } as unknown as AgentProvider
    const actions = createThreadActions({ set: partial => useChatStore.setState(partial), get: useChatStore.getState,
      sseAbortRef: { current: null } })
    await draw(); assertOneOwner()
    const pane = container.querySelector('[data-process-pane-id]')
    let submission!: Promise<boolean>
    await act(async () => { submission = actions.sendMessage('First direction', 'agent') })
    const pending = useChatStore.getState().blocks.find(block => block.kind === 'user' && block.id !== 'opening')!
    expect(pending).toMatchObject({ kind: 'user', meta: { turnId: 'turn', steeringStatus: 'pending' } })
    await draw(); assertOneOwner()
    expect(container.querySelector('[data-process-pane-id]')).toBe(pane)
    expect(container.querySelectorAll('[data-analytix-live-assistant]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-analytix-live-progress]')).toHaveLength(1)
    expect(authorize).toHaveBeenCalledTimes(1)
    await act(async () => {
      releaseSteer({ threadId: 'thread', turnId: 'turn', clientUserMessageId: pending.id, admittedSeq: 42 })
      expect(await submission).toBe(true)
    })
    const after = mappedTools('after')
    useChatStore.setState(state => ({ blocks: [...state.blocks, ...after] }))
    await draw(); assertOneOwner()
    expect(useChatStore.getState().blocks.find(block => block.id === pending.id)).toMatchObject({ meta: { steeringStatus: 'admitted' } })
    const inputSeat = container.querySelector<HTMLElement>(`[data-chat-seat="${pending.id}"]`)!
    expect(inputSeat.textContent).toContain('First direction')
    const beforeSeat = container.querySelector(`[data-chat-seat="${mappedTools('before')[0].id}"]`)!
    const afterSeat = container.querySelector(`[data-chat-seat="${after[0].id}"]`)!
    expect(beforeSeat.compareDocumentPosition(inputSeat) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(inputSeat.compareDocumentPosition(afterSeat) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // The existing rail appears at four inputs; exercise its real entry rule.
    for (const [index, text] of ['Second direction', 'Third direction'].entries()) {
      let laterSubmission!: Promise<boolean>
      await act(async () => { laterSubmission = actions.sendMessage(text, 'agent') })
      const later = useChatStore.getState().blocks.at(-1)!
      await act(async () => {
        releaseSteer({ threadId: 'thread', turnId: 'turn', clientUserMessageId: later.id, admittedSeq: 43 + index })
        expect(await laterSubmission).toBe(true)
      })
    }
    await draw(); assertOneOwner()
    const navigation = [...container.querySelectorAll<HTMLButtonElement>('[data-thread-user-message-navigation-item-id]')]
    const inputs = useChatStore.getState().blocks.filter(block => block.kind === 'user')
    expect(navigation.map(button => button.dataset.threadUserMessageNavigationItemId)).toEqual(inputs.map(input => `${input.id}:user`))
    expect(navigation).toHaveLength(4)
    await act(async () => navigation[1].click())
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalled()
    const scroll = vi.mocked(HTMLElement.prototype.scrollIntoView)
    expect(scroll.mock.instances.at(-1)).toBe(inputSeat.querySelector('[data-content-search-unit-key]'))
    const batch = generalTerminalProjectionBatchFromRuntime(historicalFooterFixture('thread').batch)!
    expect(batch).not.toBeNull()
    const sink = buildThreadEventSink(partial => useChatStore.setState(partial), useChatStore.getState)
    await act(async () => sink.onGeneralTerminalBatch!(batch))
    expect(useChatStore.getState()).toMatchObject({ busy: false, currentTurnId: null })
    await draw(''); assertOneOwner()
    expect(container.querySelector('[data-process-pane-id]')).toBe(pane)
    expect(container.querySelectorAll('[data-clock="end"]')).toHaveLength(1)
    expect(container.querySelector('[data-analytix-live-assistant]')).toBeNull()
    expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
    expect(container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')?.disabled).toBe(true)
    const headers = processHeaders()
    expect(headers).toHaveLength(2)
    expect(headers.map(header => header.getAttribute('aria-expanded'))).toEqual(['false', 'false'])
    await act(async () => headers[0].click())
    expect(headers.map(header => header.getAttribute('aria-expanded'))).toEqual(['true', 'false'])
    expect(inputSeat.closest('[hidden]')).toBeNull()
    await act(async () => headers[1].click())
    expect(headers.map(header => header.getAttribute('aria-expanded'))).toEqual(['true', 'true'])
    expect(authorize).toHaveBeenCalledTimes(1)
  })
  it.each(['failed', 'aborted'] as const)('retires the single live owner through a real %s terminal batch', async status => {
    const steering = map({ id: 'steering', role: 'user', kind: 'user_message', text: 'Direction', delivery: 'steer' })
    useChatStore.setState(state => ({ blocks: [...state.blocks, steering, ...mappedTools('after')] }))
    await draw(); assertOneOwner()
    const pane = container.querySelector('[data-process-pane-id]')
    const { batch } = historicalFooterFixture('thread')
    const terminal = generalTerminalProjectionBatchFromRuntime({ ...batch, seq: 12, lastSeq: 12,
      events: [{ ...batch.events[1], seq: 11, usageFinalStatus: status },
        { ...batch.events[2], seq: 12, kind: `turn_${status}`, status, terminalReason: status === 'failed' ? 'provider_failure' : 'cancel',
          ...(status === 'aborted' ? { discard: false, cancelled: true, cancelledPendingGates: 0 } : {}) }],
      eventManifest: batch.eventManifest.slice(1) })!
    expect(terminal).not.toBeNull()
    await act(async () => buildThreadEventSink(partial => useChatStore.setState(partial), useChatStore.getState).onGeneralTerminalBatch!(terminal))
    expect(useChatStore.getState()).toMatchObject({ busy: false, currentTurnId: null })
    await draw('', status); assertOneOwner()
    expect(container.querySelector('[data-process-pane-id]')).toBe(pane)
    expect(container.querySelector('[data-analytix-live-assistant]')).toBeNull()
    expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
    expect(container.querySelector('[data-clock="end"]')).toBeNull()
    expect(container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')?.disabled).toBe(true)
    expect(container.querySelector(`[data-chat-seat="${steering.id}"]`)!.closest('[hidden]')).toBeNull()
  })
})
describe('Schedule creation tail in the central turn', () => {
  function creation(): ToolBlock {
    const callId = 'call_host_' + 'a'.repeat(64), callItem = 'item_call_' + 'b'.repeat(64), resultItem = 'item_result_' + 'c'.repeat(64)
    return { kind: 'tool', id: callId, summary: 'Schedule', status: 'success', toolKind: 'tool_call', meta: {
      toolName: 'mcp__gui_schedule__gui_schedule_create', recordThreadId: 'thread', turnId: 'turn', callId,
      toolCallItemId: callItem, toolResultItemId: resultItem,
      toolCallIdentity: { threadId: 'thread', turnId: 'turn', callId, itemId: callItem },
      toolResultIdentity: { threadId: 'thread', turnId: 'turn', callId, itemId: resultItem },
      localResult: { turnId: 'turn', callId, resultItemId: resultItem }
    } }
  }
  function installReceipt(block: ToolBlock) {
    const prior = Object.getOwnPropertyDescriptor(window, 'analytix')
    const task = normalizeAppSettings({ schedule: { tasks: [{ id: 'synthetic-task', title: 'Receipt task', prompt: 'Synthetic task instruction' }] } } as AppSettingsV1).schedule.tasks[0]
    const api = {
      openToolResultLocalDisplay: vi.fn(async (): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display: {
        schemaVersion: 1, snapshotDigest: 'f'.repeat(64), resultItemId: block.meta!.localResult!.resultItemId,
        toolName: 'mcp__gui_schedule__gui_schedule_create', capture: { kind: 'schedule', status: 'completed',
          schedule: { operation: 'create', status: 'success', message: '', task },
          label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false }
      } })),
      closeToolResultLocalDisplay: vi.fn(async () => {}),
      onToolResultLocalDisplayInvalidated: vi.fn(() => () => {}),
      onRuntimeStatus: vi.fn(() => () => {})
    }
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api,
      schedule: { listTasks: vi.fn(() => new Promise(() => {})) } } })
    return { api, async restore() {
      await act(async () => root.render(null))
      if (prior) Object.defineProperty(window, 'analytix', prior)
      else Reflect.deleteProperty(window, 'analytix')
    } }
  }

  it.each(['completed', 'failed', 'aborted'] as const)('shows a receipt without final prose after a %s turn, outside the folded process', async turnStatus => {
    const block = creation(), fixture = installReceipt(block), preference = readWorkDetailsPreference()
    writeWorkDetailsPreference('compact')
    try {
      await render({ user, blocks: [block] }, { turnStatus, runtimeAvailable: false })
      const card = container.querySelector('[data-tool="schedule_create"]')!
      expect(card.textContent).toContain('Receipt task')
      expect(card.closest('[data-process-pane-id]')).toBeNull()
      expect(container.querySelector('[data-message-feedback]')).toBeNull()
      expect(fixture.api.openToolResultLocalDisplay).toHaveBeenCalledTimes(1)
      const processControl = container.querySelector<HTMLElement>('[data-analytix-turn-process]')!
      if (turnStatus === 'completed') {
        expect(processControl.getAttribute('aria-expanded')).toBe('false')
        await act(async () => processControl.click())
        expect(processControl.getAttribute('aria-expanded')).toBe('true')
        await act(async () => processControl.click())
        expect(processControl.getAttribute('aria-expanded')).toBe('false')
        expect(container.querySelector('[data-tool="schedule_create"]')).toBe(card)
        expect(fixture.api.openToolResultLocalDisplay).toHaveBeenCalledTimes(1)
      } else expect((processControl as HTMLButtonElement).disabled).toBe(true)
    } finally { await fixture.restore(); writeWorkDetailsPreference(preference) }
  })

  it.each(['processing', 'unknown', 'orphan', 'foreign', 'mismatched'] as const)('does not read a tail receipt for %s ownership or terminal state', async kind => {
    const block = creation(), fixture = installReceipt(block)
    const candidate: ToolBlock = kind === 'orphan' ? { ...block, meta: { ...block.meta, toolResultIdentity: undefined } }
      : kind === 'foreign' ? { ...block, meta: { ...block.meta, turnId: 'another-turn' } }
        : kind === 'mismatched' ? { ...block, meta: { ...block.meta, localResult: { ...block.meta!.localResult!, resultItemId: 'item_other_' + 'd'.repeat(64) } } }
          : block
    try {
      await render({ user, blocks: [candidate] }, { processing: kind === 'processing',
        turnStatus: kind === 'unknown' ? undefined : 'completed', runtimeAvailable: false })
      expect(container.querySelector('[data-tool="schedule_create"]')).toBeNull()
      expect(fixture.api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    } finally { await fixture.restore() }
  })
})
describe('shared central turn composition', () => {
  it('mounts feedback only for a final completed runtime assistant identity', async () => {
    const block: Extract<ChatBlock, { kind: 'assistant' }> = { kind: 'assistant', id: 'real-message', text: 'Final reply', meta: { turnId: 'turn' },
      assistantItemIdentity: { threadId: 'thread', turnId: 'turn', itemId: 'real-message' } }
    await render({ user, blocks: [block] }, { turnStatus: 'failed' })
    expect(container.querySelector('[data-message-feedback]')?.getAttribute('data-message-feedback')).toBe('real-message')
    for (const invalid of [
      { ...block, assistantItemIdentity: undefined, id: 'live_assistant_partial' },
      { ...block, assistantItemIdentity: undefined, id: 'a-123' },
      { ...block, assistantItemIdentity: { ...block.assistantItemIdentity!, threadId: 'other' } },
      { ...block, assistantItemIdentity: { ...block.assistantItemIdentity!, itemId: 'other' } }
    ]) {
      await render({ user, blocks: [invalid] }, { turnStatus: 'completed' })
      expect(container.querySelector('[data-message-feedback]')).toBeNull()
    }
    await render({ user, blocks: [block] }, { processing: true, turnStatus: 'running' })
    expect(container.querySelector('[data-message-feedback]')).toBeNull()
  })
  it.each((['main', 'side', 'inspector'] as const).flatMap(surface =>
    (['accepted', 'general'] as const).map(terminalKind => ({ surface, terminalKind }))))(
    'keeps the $surface protected seat until its $terminalKind batch receipt replaces it', async ({ surface, terminalKind }) => {
    vi.useFakeTimers(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
    const viewerId = '11111111-1111-4111-8111-111111111111', digest = 'f'.repeat(64)
    const scope = { threadId: 'thread', turnId: 'turn', contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'process-a' }
    let expanded = new Set<string>()
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
      authorizeProcessPane: async () => ({ status: 'authorized', schemaVersion: 2, viewerId, scope, revision: '1', presence: { nativeBlockCount: 0, toolCaptureCount: 0, assistantDraftCount: 1 } }),
      readProcessPane: async () => ({ status: 'snapshot', schemaVersion: 2, revision: '2', scope,
        units: [{ unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }],
        blocks: [{ blockId: 'draft-a', kind: 'assistant_draft', state: 'running', attempt: 1, durationMs: 0, truncated: false, classification: 'source_exact_local', contentClassification: 'provider_text',
          source: { kind: 'assistant_draft', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', attempt: 1 },
          ...(expanded.has('draft-a') ? { draftSegments: [{ offsetBytes: '0', text: 'PRIVATE_PROCESS_DRAFT ' }], draftBytes: '22' } : {}) }] }),
      setProcessPaneExpanded: async (request: { expandedBlockIds: string[] }) => { expanded = new Set(request.expandedBlockIds); return { status: 'ok' } },
      closeProcessPane: async () => ({ status: 'ok' }),
      onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
    } } })
    const pane = { processPaneRoute: { surface, ownerKey: surface, selectionKey: 'selection', visible: true }, runtimeAvailable: true }
    const composer = document.createElement('textarea'); composer.value = 'UNSENT_USER_DRAFT'; document.body.append(composer)
    try {
      await render({ user, blocks: [] }, { ...pane, processing: true })
      const seat = container.querySelector('[data-analytix-assistant-response-seat]')!
      expect(seat.textContent).toContain('PRIVATE_PROCESS_DRAFT')
      container.scrollTop = 221
      const ordinary: Extract<ChatBlock, { kind: 'assistant' }> = { kind: 'assistant', id: 'ordinary', text: 'NON_BATCH_MATCHING_PROSE', meta: { turnId: 'turn' } }
      await render({ user, blocks: [ordinary] }, { ...pane, processing: false, turnStatus: 'completed' })
      expect(seat.textContent).not.toContain('NON_BATCH_MATCHING_PROSE')
      const accepted: Extract<ChatBlock, { kind: 'assistant' }> = { ...ordinary, id: 'accepted-final', text: 'AUTHORIZED_FINAL_BATCH',
        acceptedFinalView: { schemaVersion: 3, acceptedFinalDigest: digest, publicationState: 'accepted', variant: 'GeneralGuidanceAnswer', terminalReason: 'success', blockerCode: '', coverageStatus: 'guidance_only', checkedScopeDigest: '', missingScopeCount: 0, claimCount: 0, claimTypes: [],
          receiptMetadata: { projection: 'masked_metadata_only', count: 0, setDigest: '8'.repeat(64), citations: [] }, noHitWording: '', acceptedAt: '2026-10-07T00:00:00Z' },
        acceptedFinalProjectionReceipt: { schemaVersion: 1, batchId: '7'.repeat(64), threadId: 'thread', turnId: 'turn', publicationCommitId: digest, lastSeq: 22 },
        acceptedFinalProjectionTerminal: { status: 'completed', createdAt: '2026-10-07T00:00:00Z', acceptedFinalDigest: digest, terminalReason: 'success' } }
      const general: Extract<ChatBlock, { kind: 'assistant' }> = { ...ordinary, id: 'general-final', text: 'AUTHORIZED_FINAL_BATCH',
        generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'thread', turnId: 'turn',
          itemId: 'general-final', firstSeq: 20, lastSeq: 22, status: 'completed' } }
      const final = terminalKind === 'accepted' ? accepted : general
      const mismatched: typeof final = terminalKind === 'accepted'
        ? { ...accepted, acceptedFinalProjectionReceipt: { ...accepted.acceptedFinalProjectionReceipt!, turnId: 'other' } }
        : { ...general, generalTerminalProjectionReceipt: { ...general.generalTerminalProjectionReceipt!, itemId: 'other' } }
      await render({ user, blocks: [mismatched] }, { ...pane, turnStatus: 'completed' })
      expect(seat.textContent).not.toContain('AUTHORIZED_FINAL_BATCH')
      if (terminalKind === 'general') {
        await render({ user, blocks: [{ ...general, generalTerminalProjectionReceipt: { ...general.generalTerminalProjectionReceipt!, lastSeq: 21 } }] }, { ...pane, turnStatus: 'completed' })
        expect(seat.textContent).not.toContain('AUTHORIZED_FINAL_BATCH')
      }
      await render({ user, blocks: [final] }, { ...pane, turnStatus: 'completed' })
      expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
      expect(seat.textContent).toContain('AUTHORIZED_FINAL_BATCH'); expect(seat.textContent).not.toContain('PRIVATE_PROCESS_DRAFT')
      expect(container.scrollTop).toBe(221); expect(composer.value).toBe('UNSENT_USER_DRAFT')
      expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
    } finally {
      await act(async () => root.render(null)); composer.remove()
      if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix')
    }
  })
  it.each(['completed', 'failed', 'aborted', undefined] as const)('uses the actual %s turn status for footer and Fork eligibility', async status => {
    const final: Extract<ChatBlock, { kind: 'assistant' }> = { kind: 'assistant', id: 'closing', text: 'Actual closing reply', meta: { turnId: 'turn' } }
    const fork = { busy: false, run: vi.fn() }
    await render({ user, blocks: [final] }, { turnStatus: status, fork })
    const footer = container.querySelector('[data-clock="end"]')
    if (status === undefined) { expect(footer).toBeNull(); return }
    expect(footer).not.toBeNull()
    const button = footer!.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:forkFromAssistantResponse')}"]`)!
    expect(button.getAttribute('aria-disabled')).toBe(status === 'failed' ? 'true' : null)
    await act(async () => button.click())
    expect(fork.run).toHaveBeenCalledTimes(status === 'failed' ? 0 : 1)
  })
  it('renders recorded Goal history without borrowing current facts or revoking the actual turn Fork boundary', async () => {
    const final: Extract<ChatBlock, { kind: 'assistant' }> = { kind: 'assistant', id: 'closing', text: 'Actual closing reply', meta: { turnId: 'turn' } }
    const history: Extract<ChatBlock, { kind: 'goal' }> = { kind: 'goal', id: 'goal-history', operation: 'pause',
      goal: { id: 'recorded-goal', revision: 2, threadId: 'thread', objective: 'Recorded paused objective', status: 'paused', maxGoalRounds: 8, roundsStarted: 3,
        tokensUsed: 0, timeUsedSeconds: 0, createdAt: '2026-10-08T00:00:00Z', updatedAt: '2026-10-08T00:01:00Z' } }
    const cleared: Extract<ChatBlock, { kind: 'goal' }> = { kind: 'goal', id: 'goal-cleared', operation: 'clear', goal: null }
    useChatStore.setState({ activeThreadGoal: { id: 'current-goal', revision: 9, threadId: 'thread', objective: 'Current unrelated objective', status: 'active', maxGoalRounds: 256, roundsStarted: 12,
      tokensUsed: 0, timeUsedSeconds: 0, createdAt: '2026-10-08T00:00:00Z', updatedAt: '2026-10-08T00:09:00Z' },
      activeGoalActivation: { id: 'current-goal', revision: 9, activation: 'armed' } })
    const fork = { busy: false, run: vi.fn() }
    await render({ user, blocks: [final, history, cleared] }, { turnStatus: 'completed', fork })
    const records = container.querySelectorAll<HTMLElement>('[data-analytix-goal-history]')
    expect(records).toHaveLength(2)
    expect(records[0]!.textContent).toContain('Recorded paused objective')
    expect(records[0]!.textContent).toContain(`${i18n.t('common:goalRoundsStarted', { defaultValue: 'Rounds started' })}: 3`)
    expect(records[0]!.textContent).toContain(`${i18n.t('common:goalMaxRounds', { defaultValue: 'Round limit' })}: 8`)
    expect(records[0]!.textContent).not.toContain('Current unrelated objective')
    expect(records[0]!.textContent).not.toContain(i18n.t('common:goalActivation', { defaultValue: 'Automatic continuation' }))
    expect(records[1]!.textContent).toContain(i18n.t('common:goalClearedTimeline'))
    expect(records[1]!.textContent).not.toContain('Recorded paused objective')
    expect(records[0]!.querySelector('button,input,textarea')).toBeNull()
    const button = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:forkFromAssistantResponse')}"]`)!
    expect(button.getAttribute('aria-disabled')).toBeNull()
    await act(async () => button.click())
    expect(fork.run).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread', turnId: 'turn', finalItemId: 'closing', status: 'completed' }))
  })
  it('keeps missing legacy Goal history counts unknown through the shared turn composition', async () => {
    const history: Extract<ChatBlock, { kind: 'goal' }> = { kind: 'goal', id: 'legacy-goal-history', operation: 'legacy_update',
      goal: { threadId: 'thread', objective: 'Legacy recorded objective', status: 'active', tokensUsed: 0, timeUsedSeconds: 0,
        createdAt: '2026-10-08T00:00:00Z', updatedAt: '2026-10-08T00:01:00Z' } }
    await render({ user, blocks: [history] })
    const record = container.querySelector<HTMLElement>('[data-analytix-goal-history]')!
    expect(record.textContent).toContain('Legacy recorded objective')
    const unknown = i18n.t('common:goalFactUnknown', { defaultValue: 'Unknown' })
    expect(record.textContent).toContain(`${i18n.t('common:goalRoundsStarted', { defaultValue: 'Rounds started' })}: ${unknown}`)
    expect(record.textContent).toContain(`${i18n.t('common:goalMaxRounds', { defaultValue: 'Round limit' })}: ${unknown}`)
    expect(record.textContent).not.toContain('256')
    expect(record.textContent).not.toContain(i18n.t('common:goalActivation', { defaultValue: 'Automatic continuation' }))
  })
  it.each((['main', 'side'] as const).flatMap(surface => [true, false].map(verified => ({ surface, verified }))))(
    'uses the real $surface snapshot-required replay for the first historical Footer (verified: $verified)', async ({ surface, verified }) => {
      const threadId = surface === 'main' ? 'thread' : 'side-history-thread'
      const { timestamp, text, batch } = historicalFooterFixture(threadId)
      const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
      let onData: Parameters<Window['analytix']['runtime']['onSseEvent']>[0] | undefined
      let onEnd: Parameters<Window['analytix']['runtime']['onSseEnd']>[0] | undefined
      const startSse = vi.fn<Window['analytix']['runtime']['startSse']>(async (owner, _seq, streamId, options) => {
        expect(owner).toBe(threadId); expect(options).toEqual({ replayThroughSeq: 16 })
        queueMicrotask(() => {
          onData?.({ streamId: streamId!, events: [{ kind: 'item_created', seq: 1, threadId, turnId: 'turn', timestamp,
            itemId: 'user', item: { id: 'user', threadId, turnId: 'turn', role: 'user', kind: 'user_message',
              status: 'completed', createdAt: timestamp, text: 'Historical request' } }] })
          if (verified) onData?.({ streamId: streamId!, events: [batch as Parameters<NonNullable<typeof onData>>[0]['events'][number]] })
          onData?.({ streamId: streamId!, events: [{ kind: 'cursor_advanced', seq: 16, threadId, timestamp, reason: 'restricted_content_removed' }] })
          onEnd?.({ streamId: streamId! })
        })
        return { streamId: streamId! }
      })
      Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
        runtimeRequest: vi.fn<Window['analytix']['runtime']['runtimeRequest']>(async (path, method) => {
          expect(path).toBe('/v1/usage?group_by=thread&thread_id=' + encodeURIComponent(threadId)); expect(method).toBe('GET')
          return normalFooterUsageResponse(threadId, footerConversation('turn', 120, 30))
        }), startSse, stopSse: vi.fn(async () => undefined), ackSseEvent: vi.fn(async () => true),
        onSseEvent: (handler: NonNullable<typeof onData>) => { onData = handler; return () => { onData = undefined } },
        onSseEnd: (handler: NonNullable<typeof onEnd>) => { onEnd = handler; return () => { onEnd = undefined } },
        onSseError: () => () => undefined
      } } })
      const provider = new AnalytixRuntimeProvider()
      const forged: ChatBlock = { kind: 'assistant', id: 'closing', text: 'Forged GET result', createdAt: timestamp,
        meta: { turnId: 'turn' }, generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64),
          threadId, turnId: 'turn', itemId: 'closing', firstSeq: 11, lastSeq: 13, status: 'completed' } }
      const detail: Awaited<ReturnType<AgentProvider['getThreadDetail']>> = { latestSeq: 16, threadStatus: 'idle', latestTurnId: 'compact-turn',
        blocks: [{ kind: 'compaction', id: 'compact-primary', summary: 'Context compacted', status: 'success', meta: { turnId: 'compact-turn' } },
          ...(!verified ? [forged] : [])], authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} }
      const read = vi.spyOn(provider, 'getThreadDetail').mockResolvedValue(detail)
      registryFixture.provider = provider
      useChatStore.setState({ activeThreadId: surface === 'main' ? threadId : 'parent-thread', blocks: [], liveAssistant: '',
        lastSeq: 0, busy: false, currentTurnId: null, currentTurnUserId: null, runtimeConnection: 'ready',
        turnUsageById: {}, rewoundTurnIds: {}, sideConversations: {},
        sidePanel: { ...useChatStore.getState().sidePanel, open: false, activeSideId: null } })
      try {
        let sink: ThreadEventSink
        if (surface === 'main') sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId })
        else {
          let captured: ThreadEventSink | undefined
          read.mockResolvedValueOnce({ blocks: [], latestSeq: 0, threadStatus: 'idle' })
          vi.spyOn(provider, 'subscribeThreadEvents').mockImplementation((_owner, _seq, incoming, signal) => {
            captured = incoming
            return new Promise<void>(resolve => signal.addEventListener('abort', () => resolve(), { once: true }))
          })
          const side = createSideActions({ set: useChatStore.setState, get: useChatStore.getState, getProvider: () => provider,
            t: key => i18n.t(key), formatRuntimeError: error => String(error), shouldOpenSettingsForError: () => false })
          expect(await side.openThreadInSidePanel(threadId)).toBe(true)
          expect(captured).toBeDefined(); sink = captured!
        }
        await sink.onSnapshotRequired?.({ threadId, seq: 16, highestSeq: 16, replayEventCount: 1 })
        expect(startSse).toHaveBeenCalledOnce()
        const state = useChatStore.getState()
        const blocks = surface === 'main' ? state.blocks : state.sideConversations[threadId].blocks
        const turnUsageById = surface === 'main' ? state.turnUsageById : state.sideConversations[threadId].turnUsageById ?? {}
        const canonical = await loadThreadUsageData(threadId)
        await act(async () => root.render(createElement(MessageTimeline, { blocks, live: '', activeThreadId: threadId, conversationUsage: canonical.conversationUsage,
          runtimeConnection: 'ready', onRetryConnection: () => undefined, onOpenSettings: () => undefined,
          runtimeStateOverride: { busy: false, currentTurnId: null, currentTurnUserId: null, turnStartedAtByUserId: {},
            turnDurationByUserId: {}, authoritativeTurnTimings: {}, authoritativeTurnStatuses: {}, turnUsageById } })))
        const footer = container.querySelector<HTMLElement>('[data-clock="end"]')
        if (!verified) {
          expect(footer).toBeNull()
          expect(blocks.every(block => block.kind !== 'assistant' || block.generalTerminalProjectionReceipt === undefined)).toBe(true)
          return
        }
        expect(footer).not.toBeNull(); expect(container.textContent).toContain(text)
        expect(footer!.querySelector(`button[aria-label="${i18n.t('common:copyMessage')}"]`)).not.toBeNull()
        const fork = footer!.querySelector(`button[aria-label="${i18n.t('common:forkFromAssistantResponse')}"]`)
        if (surface === 'main') expect(fork).not.toBeNull()
        else expect(fork).toBeNull() // Secondary display cannot call the primary-owner Fork action.
        const usage = footer!.querySelector<HTMLButtonElement>('button[aria-haspopup="dialog"]')!
        expect(usage.textContent).toContain('150')
        await act(async () => usage.click())
        expect(document.querySelector('[data-turn-usage-details]')).not.toBeNull()
      } finally {
        await act(async () => root.render(null)); teardownAllSideSubscriptions(); clearActiveStream(); registryFixture.provider = null
        if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge); else Reflect.deleteProperty(window, 'analytix')
      }
    })
  it.each(['main', 'side'] as const)('keeps the real $surface live Footer when same-cursor usage replay settles later', async surface => {
    const threadId = surface === 'main' ? 'thread' : 'side-live-replay-thread'
    const { timestamp, text, batch } = historicalFooterFixture(threadId)
    const projection = generalTerminalProjectionBatchFromRuntime(batch)!
    expect(projection).not.toBeNull()
    const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
    let onData: Parameters<Window['analytix']['runtime']['onSseEvent']>[0] | undefined
    let onEnd: Parameters<Window['analytix']['runtime']['onSseEnd']>[0] | undefined
    let replayId: string | undefined
    const startSse = vi.fn<Window['analytix']['runtime']['startSse']>(async (owner, _seq, streamId, options) => {
      expect(owner).toBe(threadId); expect(options).toEqual({ replayThroughSeq: 13 })
      replayId = streamId; return { streamId: streamId! }
    })
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
      runtimeRequest: vi.fn<Window['analytix']['runtime']['runtimeRequest']>(async (path, method) => {
          expect(path).toBe('/v1/usage?group_by=thread&thread_id=' + encodeURIComponent(threadId)); expect(method).toBe('GET')
          return normalFooterUsageResponse(threadId, footerConversation('turn', 120, 30))
        }), startSse, stopSse: vi.fn(async () => undefined), ackSseEvent: vi.fn(async () => true),
      onSseEvent: (handler: NonNullable<typeof onData>) => { onData = handler; return () => { onData = undefined } },
      onSseEnd: (handler: NonNullable<typeof onEnd>) => { onEnd = handler; return () => { onEnd = undefined } },
      onSseError: () => () => undefined
    } } })
    const provider = new AnalytixRuntimeProvider()
    const initialUser: ChatBlock = { kind: 'user', id: 'user', text: 'Historical request', createdAt: timestamp, meta: { turnId: 'turn' } }
    const detail: Awaited<ReturnType<AgentProvider['getThreadDetail']>> = { blocks: [initialUser, projection.terminalItem!], latestSeq: 13,
      threadStatus: 'idle', latestTurnId: 'turn', authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} }
    const read = vi.spyOn(provider, 'getThreadDetail').mockResolvedValue(detail)
    registryFixture.provider = provider
    useChatStore.setState({ activeThreadId: surface === 'main' ? threadId : 'parent-thread', blocks: [initialUser], liveAssistant: '',
      lastSeq: 10, busy: true, currentTurnId: 'turn', currentTurnUserId: 'user', runtimeConnection: 'ready',
      turnUsageById: {}, rewoundTurnIds: {}, sideConversations: {}, sidePanel: { ...useChatStore.getState().sidePanel, open: false, activeSideId: null } })
    try {
      let sink: ThreadEventSink
      if (surface === 'main') sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId })
      else {
        let captured: ThreadEventSink | undefined
        read.mockResolvedValueOnce({ blocks: [], latestSeq: 0, threadStatus: 'idle' })
        vi.spyOn(provider, 'subscribeThreadEvents').mockImplementation((_owner, _seq, incoming, signal) => {
          captured = incoming
          return new Promise<void>(resolve => signal.addEventListener('abort', () => resolve(), { once: true }))
        })
        const side = createSideActions({ set: useChatStore.setState, get: useChatStore.getState, getProvider: () => provider,
          t: key => i18n.t(key), formatRuntimeError: error => String(error), shouldOpenSettingsForError: () => false })
        expect(await side.openThreadInSidePanel(threadId)).toBe(true); sink = captured!
        useChatStore.setState(state => ({ sideConversations: { ...state.sideConversations,
          [threadId]: { ...state.sideConversations[threadId], blocks: [initialUser], lastSeq: 10, busy: true, turnId: 'turn', userItemId: 'user' } } }))
      }
      const reconcile = sink.onSnapshotRequired?.({ threadId, seq: 10, highestSeq: 10, replayEventCount: 1 })
      await vi.waitFor(() => expect(startSse).toHaveBeenCalledOnce())
      await sink.onGeneralTerminalBatch?.(projection)
      const blocksNow = () => surface === 'main' ? useChatStore.getState().blocks : useChatStore.getState().sideConversations[threadId].blocks
      const committed = blocksNow().find(block => block.kind === 'assistant') as Extract<ChatBlock, { kind: 'assistant' }>
      expect(committed.generalTerminalProjectionReceipt).toBeDefined()
      onData?.({ streamId: replayId!, events: [batch as Parameters<NonNullable<typeof onData>>[0]['events'][number]] })
      onEnd?.({ streamId: replayId! }); await reconcile
      const blocks = blocksNow()
      const restored = blocks.find(block => block.kind === 'assistant') as typeof committed
      expect(restored.generalTerminalProjectionReceipt).toEqual(committed.generalTerminalProjectionReceipt)
      const state = useChatStore.getState(), turnUsageById = surface === 'main' ? state.turnUsageById : state.sideConversations[threadId].turnUsageById ?? {}
      const canonical = await loadThreadUsageData(threadId)
      await act(async () => root.render(createElement(MessageTimeline, { blocks, live: '', activeThreadId: threadId, conversationUsage: canonical.conversationUsage,
        runtimeConnection: 'ready', onRetryConnection: () => undefined, onOpenSettings: () => undefined,
        runtimeStateOverride: { busy: false, currentTurnId: null, currentTurnUserId: null, turnStartedAtByUserId: {},
          turnDurationByUserId: {}, authoritativeTurnTimings: {}, authoritativeTurnStatuses: {}, turnUsageById } })))
      const footer = container.querySelector('[data-clock="end"]')!
      expect(footer).not.toBeNull(); expect(container.textContent).toContain(text)
      expect(footer.querySelector(`button[aria-label="${i18n.t('common:copyMessage')}"]`)).not.toBeNull()
      expect(footer.querySelector('button[aria-haspopup="dialog"]')!.textContent).toContain('150')
    } finally {
      await act(async () => root.render(null)); teardownAllSideSubscriptions(); clearActiveStream(); registryFixture.provider = null
      if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge); else Reflect.deleteProperty(window, 'analytix')
    }
  })
  it('accepts only a terminal receipt bound to this owner and closing item as end evidence', async () => {
    const final: Extract<ChatBlock, { kind: 'assistant' }> = { kind: 'assistant', id: 'closing', text: 'Actual closing reply', meta: { turnId: 'turn' },
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'thread', turnId: 'turn',
        itemId: 'closing', firstSeq: 20, lastSeq: 22, status: 'completed' } }
    await render({ user, blocks: [final] })
    expect(container.querySelector('[data-clock="end"]')).not.toBeNull()
    for (const mismatch of [{ threadId: 'foreign' }, { turnId: 'foreign' }, { itemId: 'foreign' }]) {
      await render({ user, blocks: [{ ...final, generalTerminalProjectionReceipt: { ...final.generalTerminalProjectionReceipt!, ...mismatch } }] })
      expect(container.querySelector('[data-clock="end"]')).toBeNull()
    }
  })
  it('restores the actual historical Copy, Fork and Usage footer from a verified terminal batch after compaction', async () => {
    const { timestamp, text, batch } = historicalFooterFixture('thread')
    const usageRequest = vi.spyOn(rendererRuntimeClient, 'runtimeRequest').mockImplementation(async (path, method) => {
      expect(path).toBe('/v1/usage?group_by=thread&thread_id=thread'); expect(method).toBe('GET')
      return normalFooterUsageResponse('thread', footerConversation('turn', 120, 30))
    })
    const canonical = (await loadThreadUsageData('thread')).conversationUsage
    expect(usageRequest).toHaveBeenCalledOnce()
    const replay = createOrdinaryHistoryReplay('thread', 16)
    replay.accept([{ kind: 'item_created', seq: 1, threadId: 'thread', turnId: 'turn',
      item: { id: 'user', threadId: 'thread', turnId: 'turn', role: 'user', kind: 'user_message',
        status: 'completed', createdAt: timestamp, text: 'Historical request' } }])
    replay.accept([batch])
    replay.accept([{ kind: 'compaction_completed', seq: 16, threadId: 'thread', turnId: 'compact-turn', summary: 'Context compacted' }])
    const restored = replay.mergeSnapshot([
      { kind: 'compaction', id: 'compact', status: 'success', summary: 'Context compacted', meta: { turnId: 'compact-turn' } },
      { kind: 'user', id: 'later-user', text: 'Current primary request', meta: { turnId: 'later-turn' } }
    ])
    const fork = vi.fn().mockResolvedValue(undefined)
    useChatStore.setState({ busy: false, currentTurnId: null, currentTurnUserId: null, forkThreadFromTurn: fork })
    const show = async (blocks: ChatBlock[], conversationUsage = canonical) => act(async () => root.render(createElement(MessageTimeline, {
      conversationUsage,
      blocks, live: '', activeThreadId: 'thread', runtimeConnection: 'ready',
      onRetryConnection: () => undefined, onOpenSettings: () => undefined,
      runtimeStateOverride: { busy: false, currentTurnId: null, currentTurnUserId: null,
        turnStartedAtByUserId: {}, turnDurationByUserId: {}, authoritativeTurnTimings: {},
        authoritativeTurnStatuses: {}, turnUsageById: replay.turnUsage() }
    })))
    await show(restored)
    const footer = container.querySelector<HTMLElement>('[data-clock="end"]')!
    expect(footer).not.toBeNull()
    const copyButton = footer.querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:copyMessage') + '"]')!
    expect(copyButton).not.toBeNull()
    const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    try {
      await act(async () => copyButton.click())
      expect(writeText).toHaveBeenCalledWith(text)
    } finally {
      if (clipboardDescriptor) Object.defineProperty(navigator, 'clipboard', clipboardDescriptor)
      else Reflect.deleteProperty(navigator, 'clipboard')
    }
    const forkButton = footer.querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:forkFromAssistantResponse') + '"]')!
    expect(forkButton.getAttribute('aria-disabled')).toBeNull()
    await act(async () => forkButton.click())
    expect(fork).toHaveBeenCalledWith('turn', expect.objectContaining({ threadId: 'thread', turnId: 'turn', finalItemId: 'closing', status: 'completed', endSeq: 13, isCurrent: expect.any(Function), getSnapshot: expect.any(Function) }))
    const usageButton = footer.querySelector<HTMLButtonElement>('button[aria-haspopup="dialog"]')!
    expect(usageButton.textContent).toContain('150')
    await act(async () => usageButton.click())
    const outputLabel = [...document.querySelectorAll('[data-turn-usage-details] dt')]
      .find(label => label.textContent === i18n.t('common:dshUsageOutput'))!
    expect(outputLabel.nextElementSibling?.firstChild?.textContent).toBe(i18n.t('common:dshUsageTokens', { tokens: '30' }))
    expect(outputLabel.nextElementSibling?.querySelector('span')?.textContent).toBe(i18n.t('common:dshUsageReasoningSubset', {
      tokens: i18n.t('common:dshUsageTokens', { tokens: '0' })
    }))
    // Retained physical-attempt legacy150 cannot fill a missing canonical group.
    await show(restored, null)
    expect(container.querySelector('[data-clock="end"]')).not.toBeNull()
    expect(container.querySelector('[data-analytix-turn-usage]')).toBeNull()
    // Replay attribution and a usage entry alone are not terminal authority.
    await show(restored.map(block => block.kind === 'assistant'
      ? { ...block, generalTerminalProjectionReceipt: undefined } : block))
    expect(container.querySelector('[data-clock="end"]')).toBeNull()
  })
  it.each(['error', 'stopped'] as const)('suppresses success links, suffix and totals in a %s row while retaining its real body', async state => {
    const onOpenFile = vi.fn(), inspect = vi.fn(), toggle = vi.fn()
    await act(async () => root.render(createElement(ToolRow, { title: 'Read', state, summary: '/synthetic/file.ts', errorSummary: 'First public failure\nLater line',
      filePath: '/synthetic/file.ts', href: 'https://example.invalid/source', onOpenFile, summarySuffix: 'SUCCESS_SUFFIX', diffStats: { added: 2, removed: 1 },
      expanded: true, expandable: true, onToggle: toggle, inspect: { label: 'Inspect record', run: inspect }, body: { kind: 'details', content: 'Actual bounded detail' } })))
    expect(container.textContent).toContain(i18n.t(state === 'error' ? 'common:protectedToolFailed' : 'common:dshTurnStopped'))
    expect(container.textContent).not.toContain('toolActionFailed')
    if (state === 'error') expect(container.textContent).toContain('First public failure')
    else { expect(container.textContent).toContain('/synthetic/file.ts'); expect(container.textContent).not.toContain('First public failure') }
    expect(container.textContent).not.toContain('Later line')
    expect(container.textContent).not.toContain('SUCCESS_SUFFIX')
    expect(container.querySelector('a,[data-tool-file-link],[data-analytix-diff-stats]')).toBeNull()
    expect(container.textContent).toContain('Actual bounded detail')
    const inspectButton = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect record')!
    await act(async () => inspectButton.click())
    expect(inspect).toHaveBeenCalledOnce(); expect(toggle).not.toHaveBeenCalled(); expect(onOpenFile).not.toHaveBeenCalled()
  })
  it('prepares only from explicit state props without inventing a preparing producer', async () => {
    const toggle = vi.fn()
    await act(async () => root.render(createElement(ToolRow, { title: 'Read', state: 'preparing', expanded: true, expandable: true, onToggle: toggle,
      filePath: '/synthetic/preparing.ts', onOpenFile: vi.fn(), body: { kind: 'details', content: 'Unavailable preparing output' } })))
    const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    await act(async () => row.click())
    expect(toggle).not.toHaveBeenCalled(); expect(row.getAttribute('aria-expanded')).toBeNull()
    expect(container.querySelector('[data-tool-file-link]')).toBeNull()
    expect(container.textContent).not.toContain('Unavailable preparing output')
  })
  it('keeps the real live group title for its minimum 150ms before switching activity', async () => {
    vi.useFakeTimers(); vi.setSystemTime(20_000)
    const blocks: ToolBlock[] = [{ ...tool, id: 'stable-a', status: 'running' }, { ...tool, id: 'stable-b', status: 'running' }]
    const props = { blocks, processing: true, policy: presentationPolicyFor('compact') }
    await act(async () => root.render(createElement(ToolCallTree, props)))
    const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child button')!
    expect(accessibleText(header)).toBe(i18n.t('common:dshStepProcess.live.read'))
    await act(async () => vi.advanceTimersByTime(100))
    await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: [...blocks,
      { ...tool, id: 'stable-shell', status: 'running', meta: { toolName: 'bash' } }] })))
    expect(accessibleText(header)).toBe(i18n.t('common:dshStepProcess.live.read'))
    await act(async () => vi.advanceTimersByTime(49))
    expect(accessibleText(header)).toBe(i18n.t('common:dshStepProcess.live.read'))
    await act(async () => vi.advanceTimersByTime(1))
    expect(accessibleText(header)).toBe(i18n.t('common:dshStepProcess.live.commands'))
    expect(container.querySelector('[data-process-group] > div:first-child button')).toBe(header)
  })

  it.each([{ running: true, height: 600 }, { running: false, height: 600 }, { running: false, height: 100 }])(
    'positions a $height px real group with running=$running only after its visible layout', async ({ running, height }) => {
    const fixture = await processScrollFixture(running, height)
    await fixture.open()
    expect(fixture.body.hidden).toBe(false); expect(fixture.position).toBe(0)
    expect(fixture.scrollTo).not.toHaveBeenCalled(); expect(fixture.reads).toBe(0)
    const observer = [...fixture.observers].find(observer => observer.targets.has(fixture.body))!
    expect(observer.targets.has(fixture.body.firstElementChild!)).toBe(true)
    await fixture.resize()
    expect(fixture.position).toBe(running ? height - 100 : 0)
    await fixture.resize(800)
    expect(fixture.target).toBe(running ? 700 : null)
    await fixture.finish()
    expect(fixture.position).toBe(running ? 700 : 0)
    await fixture.move(200)
    const requests = fixture.scrollTo.mock.calls.length
    await fixture.resize(1_000)
    expect(fixture.position).toBe(200); expect(fixture.scrollTo).toHaveBeenCalledTimes(requests)
    await fixture.move(900)
    await fixture.resize(1_200)
    expect(fixture.target).toBe(1_100)
    await fixture.finish()
    expect(fixture.body.hasAttribute('data-scroll-down')).toBe(false)
    expect(container.querySelector('[data-step-process-body]')).toBe(fixture.body)
  })

  it('keeps one native group target until its own scrollend and catches final growth after settling', async () => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.resize(800)
    expect(fixture.scrollTo).toHaveBeenCalledExactlyOnceWith({ top: 700, behavior: 'smooth' })
    await fixture.move(550)
    await fixture.resize(1_000)
    const beforeNestedEnd = fixture.reads
    await act(async () => fixture.body.firstElementChild!.dispatchEvent(new Event('scrollend', { bubbles: true })))
    expect(fixture.reads).toBe(beforeNestedEnd)
    expect(fixture.target).toBe(700); expect(fixture.scrollTo).toHaveBeenCalledTimes(1)
    await fixture.settle(); await fixture.resize(1_200)
    expect(fixture.target).toBe(700); expect(fixture.scrollTo).toHaveBeenCalledTimes(1)
    await fixture.finish()
    expect(fixture.position).toBe(700); expect(fixture.target).toBe(1_100)
    expect(fixture.scrollTo).toHaveBeenLastCalledWith({ top: 1_100, behavior: 'smooth' })
    await fixture.finish()
    expect(fixture.position).toBe(1_100); expect(fixture.body.hasAttribute('data-scroll-down')).toBe(false)
  })

  it('preserves a group reader after native scrolling stops away from its target without an input gesture', async () => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.resize(800); await fixture.resize(1_000)
    await fixture.finish(600)
    expect(fixture.target).toBeNull()
    const requests = fixture.scrollTo.mock.calls.length
    await fixture.resize(1_200)
    expect(fixture.position).toBe(600); expect(fixture.scrollTo).toHaveBeenCalledTimes(requests)
    expect(fixture.body.hasAttribute('data-scroll-up')).toBe(true); expect(fixture.body.hasAttribute('data-scroll-down')).toBe(true)
  })

  it('positions within-tolerance group floors immediately and cancels a shrinking native target', async () => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.move(499.6); await fixture.resize()
    expect(fixture.position).toBe(500); expect(fixture.scrollTo).not.toHaveBeenCalled()
    await fixture.resize(800)
    expect(fixture.target).toBe(700)
    await fixture.resize(400)
    expect(fixture.position).toBe(300); expect(fixture.target).toBeNull()
    expect(fixture.scrollTo).toHaveBeenLastCalledWith({ top: 300, behavior: 'instant' })
    await fixture.resize(600)
    expect(fixture.target).toBe(500)
    await fixture.finish()
    expect(fixture.position).toBe(500)
  })

  it.each(['wheel', 'touchstart', 'pointerdown'])('cancels group motion on %s while retaining no-op input intent until the reader moves', async type => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.resize(800); await fixture.move(550)
    await act(async () => fixture.body.dispatchEvent(new Event(type, { bubbles: true })))
    expect(fixture.scrollTo).toHaveBeenLastCalledWith({ top: 550, behavior: 'instant' })
    await fixture.finish()
    await fixture.resize(1_000)
    expect(fixture.target).toBe(900)
    await act(async () => fixture.body.dispatchEvent(new Event(type, { bubbles: true })))
    await fixture.move(200); await fixture.finish()
    const requests = fixture.scrollTo.mock.calls.length
    await fixture.resize(1_200)
    expect(fixture.position).toBe(200); expect(fixture.scrollTo).toHaveBeenCalledTimes(requests)
  })

  it.each(['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '])('interrupts group motion for unprevented %s from an editable child', async key => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.resize(800)
    const input = document.createElement('input'); fixture.body.firstElementChild!.append(input)
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })))
    expect(fixture.scrollTo).toHaveBeenLastCalledWith({ top: 500, behavior: 'instant' })
    await fixture.finish(); await fixture.resize(1_000)
    expect(fixture.target).toBe(900)
  })

  it('retains group motion for prevented scroll keys and unrelated keys', async () => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.resize(800)
    const input = document.createElement('input'); fixture.body.firstElementChild!.append(input)
    input.addEventListener('keydown', event => { if (event.key === 'ArrowUp') event.preventDefault() })
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true }))
      input.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', bubbles: true, cancelable: true }))
    })
    expect(fixture.target).toBe(700); expect(fixture.scrollTo).toHaveBeenCalledTimes(1)
  })

  it.each(['system', 'product'])('follows group growth immediately under %s reduced motion', async source => {
    const previousMotion = document.documentElement.dataset.motionReduced
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: source === 'system' })))
    if (source === 'product') document.documentElement.dataset.motionReduced = 'true'
    try {
      const fixture = await processScrollFixture()
      await fixture.open(); await fixture.resize(); await fixture.resize(800)
      expect(fixture.position).toBe(700); expect(fixture.target).toBeNull(); expect(fixture.scrollTo).not.toHaveBeenCalled()
      await fixture.resize(1_000)
      expect(fixture.position).toBe(900); expect(fixture.target).toBeNull()
    } finally {
      if (previousMotion === undefined) delete document.documentElement.dataset.motionReduced
      else document.documentElement.dataset.motionReduced = previousMotion
    }
  })

  it.each(['closed', 'hidden', 'ungrouped', 'replaced', 'unmounted'])('retires group motion and observers when %s', async mode => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.resize(800); await fixture.move(550)
    if (mode === 'closed') await act(async () => fixture.header.click())
    else if (mode === 'hidden') await fixture.update({ visible: false })
    else if (mode === 'ungrouped') await fixture.update({ policy: presentationPolicyFor('detailed') })
    else if (mode === 'replaced') await fixture.update({ blocks: [{ ...tool, id: 'next-a', status: 'running' }, { ...tool, id: 'next-b', status: 'running' }] })
    else await act(async () => root.render(null))
    expect(fixture.scrollTo).toHaveBeenLastCalledWith({ top: 550, behavior: 'instant' })
    expect(fixture.target).toBeNull(); expect(fixture.observers.size).toBe(0)
    const requests = fixture.scrollTo.mock.calls.length
    await fixture.resize(1_000); await fixture.finish()
    expect(fixture.scrollTo).toHaveBeenCalledTimes(requests)
  })

  it('reopens the retained settled group body at the top after a reader position', async () => {
    const fixture = await processScrollFixture()
    await fixture.open(); await fixture.resize(); await fixture.move(200); await fixture.settle()
    expect(fixture.position).toBe(200)
    await act(async () => fixture.header.click())
    await fixture.open()
    expect(fixture.position).toBe(200)
    await fixture.resize()
    expect(fixture.position).toBe(0); expect(fixture.target).toBeNull()
    expect(container.querySelector('[data-step-process-body]')).toBe(fixture.body)
  })

  it.each([
    { language: 'zh', title: '执行了命令并已读取文件' },
    { language: 'en', title: 'Ran commands and read files' }
  ])('ranks the settled $language group by distinct real calls while preserving tie order', async ({ language, title }) => {
    await i18n.changeLanguage(language)
    const blocks: ToolBlock[] = [
      { ...tool, id: 'read-call', meta: { toolName: 'read', callId: 'read-a' } },
      { ...tool, id: 'bash-call-b', meta: { toolName: 'bash', callId: 'bash-b' } },
      { ...tool, id: 'bash-call-c', meta: { toolName: 'bash', callId: 'bash-c' } },
      { ...tool, id: 'read-result', meta: { toolName: 'read', callId: 'read-a' } }
    ]
    await act(async () => root.render(createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('compact') })))
    const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child button')!
    expect(header.textContent).toBe(title)
    expect(header.dataset.processActivity).toBe('commands')
    expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(4)
    expect(header.textContent).not.toMatch(/\d/)
  })
  it.each([
    { language: 'zh', title: '已读取文件，执行了命令，更新了计划等' },
    { language: 'en', title: 'Read files, ran commands, updated the plan, etc.' }
  ])('shows only the first three settled $language activity categories and the actual remainder', async ({ language, title }) => {
    await i18n.changeLanguage(language)
    const blocks: ToolBlock[] = ['read', 'bash', 'update_plan', 'grep', 'custom_tool'].map((toolName, index) => ({ ...tool,
      id: 'ranked-' + index, meta: { toolName, callId: 'actual-' + toolName } }))
    await act(async () => root.render(createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('compact') })))
    const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child button')!
    expect(header.textContent).toBe(title)
    expect(header.dataset.processActivity).toBe('read')
    expect(header.textContent).not.toMatch(/\d/)
  })
  it.each([
    { language: 'zh', title: '已读取文件并写入文件' },
    { language: 'en', title: 'Read files and wrote files' }
  ])('uses the pinned shared-prefix $language grammar without displaying counts', async ({ language, title }) => {
    await i18n.changeLanguage(language)
    const blocks = ['read', 'write'].map(toolName => ({ ...tool, id: toolName, meta: { toolName, callId: 'actual-' + toolName } }))
    await act(async () => root.render(createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('compact') })))
    expect(container.querySelector('[data-process-group] > div:first-child button')!.textContent).toBe(title)
  })

  it.each([
    { surface: 'main', threadId: 'thread-current', turnId: 'turn_1' },
    { surface: 'side', threadId: 'side-thread', turnId: 'side_turn' }
  ])('keeps neutral request stages outside the actual $surface mounted transcript, including restored rows', async ({ surface, threadId, turnId }) => {
    const fixture = await providerStageFixture(surface as 'main' | 'side', threadId, turnId)
    try {
      for (const [index, stage] of (['pre_send', 'post_send', 'response_received'] as const).entries()) {
        await fixture.emit({ kind: 'pipeline_stage', seq: index + 1, timestamp: '2026-10-08T00:00:00Z',
          threadId, turnId, stage, label: 'UNTRUSTED_STAGE_ROW_PROSE' })
        await fixture.draw()
        expect(fixture.blocks().some(block => block.kind === 'system')).toBe(false)
        expect(container.querySelector('[data-runtime-diagnostics-stage]')).toBeNull()
        expect(container.textContent).not.toContain('UNTRUSTED_STAGE_ROW_PROSE')
      }
      fixture.setBlocks([...fixture.blocks(), { kind: 'system', id: 'old-stage', text: 'OLD_MODEL_REQUEST_PROSE',
        meta: { turnId, stage: 'post_send' } }])
      await fixture.draw()
      expect(container.querySelector('[data-runtime-diagnostics-stage]')).toBeNull()
      expect(container.textContent).not.toContain('OLD_MODEL_REQUEST_PROSE')
      expect(fixture.blocks().at(-1)).toMatchObject({ id: 'old-stage', meta: { stage: 'post_send' } })
    } finally { await fixture.close() }
  })

  it('keeps ordinary notices and their existing diagnostics action while retiring neutral request rows', async () => {
    const fixture = await providerStageFixture('main', 'thread', 'turn')
    try {
      fixture.setBlocks([user, { kind: 'system', id: 'ordinary', text: 'Response received',
        detail: 'Actual projected public diagnostic', meta: { diagnostics: { jobId: 'synthetic-job' } } }])
      await fixture.draw()
      expect(container.textContent).toContain('Response received')
      expect(container.querySelector('[data-runtime-diagnostics-stage]')).toBeNull()
      expect(container.querySelector('[aria-label="' + i18n.t('common:openRuntimeDiagnostics') + '"]')).not.toBeNull()
    } finally { await fixture.close() }
  })

  it('preserves mapped provider error and retry facts when progress prose is retired', async () => {
    const fixture = await providerStageFixture('main', 'thread', 'turn')
    try {
      await i18n.changeLanguage('en')
      // A protocol-valid neutral phase can also carry a real error diagnostic.
      // It must not take the routine progress-only retirement path.
      await fixture.emit({ kind: 'pipeline_stage', seq: 1, timestamp: '2026-10-08T00:00:00Z', threadId: 'thread', turnId: 'turn',
        stage: 'post_send', details: { providerError: { kind: 'network', status: 503, retryable: true } } })
      const providerError = fixture.blocks().find(block => block.kind === 'system') as Extract<ChatBlock, { kind: 'system' }>
      expect(providerError.meta).toMatchObject({ stage: 'post_send', providerError: { status: 503, kind: 'network' } })
      await i18n.changeLanguage('zh'); await fixture.draw()
      const notice = container.querySelector<HTMLDetailsElement>('[data-runtime-diagnostics-stage="post_send"]')!
      expect(notice.textContent).toContain(providerError.text)
      await act(async () => notice.querySelector<HTMLElement>('summary')!.click())
      expect(notice.open).toBe(true); expect(notice.textContent).toContain('503')

      await fixture.emit({ kind: 'pipeline_stage', seq: 2, timestamp: '2026-10-08T00:00:01Z', threadId: 'thread', turnId: 'turn',
        stage: 'provider_retrying', attempt: 2, maxAttempt: 3, details: { retry: { schemaVersion: 1, source: 'provider_retry',
          phase: 'started', logicalSequence: 7, outerAttempt: 2 } } })
      const retry = fixture.blocks().find(block => block.kind === 'system' && block.meta?.providerRetry) as Extract<ChatBlock, { kind: 'system' }>
      expect(retry.meta?.providerRetry).toMatchObject({ attempt: 2, maxAttempt: 3, lifecycle: { phase: 'started', logicalSequence: 7 } })
      expect(retry.meta).not.toHaveProperty('stage')
      // A retained compatible block may carry both fields even though today's
      // retry mapper does not add a neutral stage. Its typed retry wins.
      fixture.setBlocks(fixture.blocks().map(block => block.id === retry.id ? { ...retry, meta: { ...retry.meta, stage: 'post_send' } } : block))
      await fixture.draw()
      const retryRow = container.querySelector<HTMLDetailsElement>('[data-analytix-retry]')!
      expect(retryRow.dataset.retryState).toBe('started')
      expect(retryRow.textContent).toContain(retry.text)
      expect(accessibleText(retryRow.querySelector('summary')!)).toContain(i18n.t('common:dshRetryStarted'))
      await act(async () => retryRow.querySelector<HTMLElement>('summary')!.click())
      expect(retryRow.open).toBe(true); expect(retryRow.textContent).toContain(retry.text)

      await fixture.emit({ kind: 'error', seq: 3, timestamp: '2026-10-08T00:00:02Z', threadId: 'thread', turnId: 'turn',
        itemId: 'public-stage-error', code: 'http_503', severity: 'error', message: 'Synthetic provider failure' })
      const error = fixture.blocks().find(block => block.kind === 'system' && block.severity === 'error') as Extract<ChatBlock, { kind: 'system' }>
      expect(error).toBeDefined(); expect(error.code).toBe('http_503')
      // Error mapper/store does not propagate stage. This is explicit history
      // compatibility, not a claim that a current producer emitted both.
      fixture.setBlocks(fixture.blocks().map(block => block.id === error.id ? { ...error, meta: { ...error.meta, stage: 'post_send' } } : block))
      await fixture.draw()
      const errorRow = container.querySelector('[data-analytix-turn-error]')!
      expect(errorRow.querySelector('[role="status"]')!.textContent).toContain(error.text)
      expect(errorRow.textContent).toContain(error.code!)
      const disclosure = errorRow.querySelector<HTMLButtonElement>('[data-disclosure-row]')!
      await act(async () => disclosure.click())
      expect(disclosure.getAttribute('aria-expanded')).toBe('true')
      expect(errorRow.textContent).toContain('post_send')
    } finally { await fixture.close() }
  })

  it('ticks only the running tail and settles to a real fixed process header', async () => {
    vi.useFakeTimers(); vi.setSystemTime(20_000)
    let renders = 0
    const turn: ThreadTurn = { user, blocks: [tool] }
    function Probe() { renders++; return createElement(ChatView, { threadId: 'thread', turnId: 'turn', turn,
      sections: deriveTurnSections({ turn, isProcessing: true, liveProcessText: '', liveContent: '', workspaceRoot: '' }), processing: true, running: true, runningStartedAt: 10_000 }) }
    await act(async () => root.render(createElement(Probe)))
    expect(container.querySelector('[data-analytix-turn-process]')).toBeNull()
    const tail = container.querySelector('[data-analytix-live-progress]')!
    expect(tail.textContent).toContain('10秒')
    await act(async () => vi.advanceTimersByTime(2000))
    expect(tail.textContent).toContain('12秒'); expect(renders).toBe(1)
    expect(tail.querySelector('[role="status"]')!.textContent).toBe(i18n.t('common:dshRunning'))
    expect(tail.querySelector('[role="status"]')!.textContent).not.toContain('12')
    await render(turn, { turnStatus: 'completed', completedDurationMs: 3200 })
    const header = container.querySelector<HTMLElement>('[data-analytix-turn-process]')!
    expect(header.textContent).toContain('3秒'); expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
    expect(header.getAttribute('aria-expanded')).toBe('false')
    await act(async () => header.click())
    const group = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button')!
    expect(group).not.toBeNull(); expect(group.closest('[hidden]')).toBeNull()
    expect(group.disabled).toBe(false); expect(group.getAttribute('aria-expanded')).toBe('false')
    await act(async () => group.click())
    expect(group.getAttribute('aria-expanded')).toBe('true')
    const row = container.querySelector<HTMLElement>('[data-chat-seat="tool"] [data-disclosure-row]')!
    expect(row.closest('[hidden]')).toBeNull()
    expect(row.getAttribute('aria-expanded')).toBe('false'); expect(container.textContent).not.toContain('Public detail text')
    await act(async () => row.click()); expect(container.textContent).toContain('Public detail text')
    await act(async () => header.click()); expect(container.textContent).not.toContain('Public detail text')
    await act(async () => vi.advanceTimersByTime(60_000)); expect(header.textContent).toContain('3秒')
    expect(container.querySelector('.ds-work-stack,.ds-work-timeline-detail')).toBeNull()
  })
  it('keeps unknown, failed and aborted terminal presentation honest without final prose', async () => {
    const turn: ThreadTurn = { user, blocks: [tool] }
    await render(turn); expect(container.querySelector('[data-analytix-turn-process]')!.textContent).toBe(i18n.t('common:dshTurnProcess'))
    expect(container.textContent).not.toContain(i18n.t('common:processed'))
    await render(turn, { turnStatus: 'failed' }); expect(container.textContent).toContain(i18n.t('common:dshProcessFailed'))
    expect(container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!.disabled).toBe(true)
    expect(container.querySelector('[data-chat-seat="tool"]')).not.toBeNull()
    await render(turn, { turnStatus: 'aborted' }); expect(container.textContent).toContain(i18n.t('common:dshTurnStopped'))
    expect(container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!.disabled).toBe(true)
    expect(container.querySelector('[data-chat-seat="tool"]')).not.toBeNull()
  })
  it('exposes only public error detail through a single real disclosure', async () => {
    const turn: ThreadTurn = { user, blocks: [{ kind: 'system', id: 'public-error', severity: 'error',
      text: 'Synthetic provider error', code: 'http_400', detail: 'Synthetic safe diagnostic detail' }] }
    await render(turn, { turnStatus: 'failed' })
    expect(container.textContent).toContain('http_400')
    expect(container.textContent).not.toContain('Synthetic safe diagnostic detail')
    const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    expect(row.getAttribute('aria-expanded')).toBe('false')
    await act(async () => row.click()); expect(container.textContent).toContain('Synthetic safe diagnostic detail')
    await act(async () => row.click()); expect(container.textContent).not.toContain('Synthetic safe diagnostic detail')
  })
  it('keeps a private think tag out of public reasoning and final content', async () => {
    const turn: ThreadTurn = { user, blocks: [{ kind: 'assistant', id: 'preface', text: '<think>PRIVATE_REASONING_SENTINEL</think>UNTRUSTED_MIXED_PROSE' }, { kind: 'assistant', id: 'public-note', text: 'Public process note' }, tool] }
    await render(turn, { processing: true })
    expect(container.textContent).toContain('Public process note'); expect(container.textContent).not.toContain('PRIVATE_REASONING_SENTINEL'); expect(container.textContent).not.toContain('UNTRUSTED_MIXED_PROSE')
  })
  it('keeps public successful diff counts outside the truncated row summary without opening detail', async () => {
    const patch = '--- a/fixture.ts\n+++ b/fixture.ts\n@@ -1 +1,2 @@\n-old\n+new\n+second'
    await render({ user, blocks: [{ ...tool, toolKind: 'file_change', filePath: '/fixture/long/path/fixture.ts',
      summary: 'edit: fixture', detail: patch, meta: { toolName: 'edit' } }] }, { processing: true })
    const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    expect(row.getAttribute('aria-expanded')).toBe('false')
    expect(row.textContent).toContain('+2'); expect(row.textContent).toContain('-1')
    const counts = row.querySelector('[data-analytix-diff-stats]')!
    expect(counts).not.toBeNull()
    const trailing = counts.parentElement!
    expect(trailing.previousElementSibling?.textContent).toContain('fixture.ts')
    expect(trailing.previousElementSibling?.contains(counts)).toBe(false)
    expect(container.textContent).not.toContain('second')
    for (const changed of [
      { status: 'running' as const, detail: patch }, { status: 'error' as const, detail: patch },
      { status: 'success' as const, detail: 'PRIVATE_PATCH_WITHOUT_PUBLIC_DIFF_MARKERS' },
      { status: 'success' as const, detail: '--- a/fixture.ts\n+++ b/fixture.ts\n+not-a-hunk' }
    ]) {
      await render({ user, blocks: [{ ...tool, toolKind: 'file_change', ...changed }] }, { processing: true })
      expect(container.querySelector('[data-analytix-diff-stats]')).toBeNull()
    }
    const apiDescriptor = Object.getOwnPropertyDescriptor(window, 'analytix')
    const open = vi.fn()
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { openToolResultLocalDisplay: open } } })
    await render({ user, blocks: [{ ...tool, toolKind: 'file_change', detail: patch,
      meta: { turnId: 'turn', localResult: { turnId: 'foreign-turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) } } }] }, { processing: true })
    expect(open).not.toHaveBeenCalled()
    await render({ user, blocks: [{ ...tool, toolKind: 'file_change', detail: patch,
      meta: { localResult: { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) } } }] }, { processing: true })
    expect(container.querySelector('[data-analytix-diff-stats]')).toBeNull()
    expect(open).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ threadId: 'thread', turnId: 'turn',
      callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }))
    if (apiDescriptor) Object.defineProperty(window, 'analytix', apiDescriptor)
    else Reflect.deleteProperty(window, 'analytix')
  })
  it('shows only the first redacted public failure line ahead of an available file path', async () => {
    // Deterministic synthetic redaction fixture; never use a real credential.
    const syntheticRedactionToken = 'sk-' + 'x'.repeat(24)
    await render({ user, blocks: [{ ...tool, status: 'error', filePath: '/fixture/missing.ts',
      detail: 'Public failure token=' + syntheticRedactionToken + ' ' + 'x'.repeat(120) + '\nSECOND_LINE_HIDDEN' }] }, { processing: true })
    const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    expect(row.textContent).toContain('Public failure')
    expect(row.textContent).not.toContain('/fixture/missing.ts')
    expect(row.textContent).not.toContain(syntheticRedactionToken)
    expect(row.textContent).not.toContain('SECOND_LINE_HIDDEN')
    expect(row.textContent).toContain('x'.repeat(120)); expect(row.getAttribute('aria-expanded')).toBe('false')
    await render({ user, blocks: [{ ...tool, status: 'error', filePath: '/fixture/missing.ts', detail: '' }] }, { processing: true })
    expect(container.textContent).toContain('/fixture/missing.ts')
  })
  it.each([
    ['\nSECOND_LINE_HIDDEN', '', false, true],
    ['  Public failure\nSECOND_LINE_HIDDEN', '  Public failure', true, true],
    ['Public failure\r\nSECOND_LINE_HIDDEN', 'Public failure\r', true, true],
    ['Public failure\rnext', 'Public failure\rnext', true, false],
    ['  \nSECOND_LINE_HIDDEN', '  ', true, true]
  ])('preserves the literal public error first line for %j', async (detail, firstLine, hasSummary, expandable) => {
    await render({ user, blocks: [{ ...tool, status: 'error', filePath: '/fixture/missing.ts', detail }] }, { processing: true })
    const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    const summary = row.querySelector('.' + toolRowCss.summary)
    expect(summary !== null).toBe(hasSummary)
    expect(summary?.textContent ?? '').toBe(firstLine)
    expect(row.querySelector('.' + toolRowCss.sep) !== null).toBe(hasSummary)
    expect(row.textContent).not.toContain('/fixture/missing.ts')
    expect(row.textContent).not.toContain('SECOND_LINE_HIDDEN')
    // A full one-line error already equals its detail, so the existing
    // summary/detail dedup has no disclosure; LF-tail cases remain collapsed.
    expect(row.getAttribute('aria-expanded')).toBe(expandable ? 'false' : null)
  })
  it.each(['prefix', 'exact'] as const)('preserves a literal error first line that matches the %s operation title', async shape => {
    for (const language of ['zh', 'en']) {
      await i18n.changeLanguage(language)
      const title = String(i18n.t('common:toolBuiltinRead'))
      const firstLine = shape === 'prefix' ? title + ' public failure' : title
      await render({ user, blocks: [{ ...tool, status: 'error', filePath: '/fixture/missing.ts',
        detail: firstLine + '\nSECOND_LINE_HIDDEN' }] }, { processing: true })
      const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
      expect(row.querySelector('.' + toolRowCss.summary)?.textContent).toBe(firstLine)
      expect(row.querySelector('.' + toolRowCss.sep)).not.toBeNull()
      expect(row.textContent).not.toContain('/fixture/missing.ts')
      expect(row.textContent).not.toContain('SECOND_LINE_HIDDEN')
      expect(row.getAttribute('aria-expanded')).toBe('false')
    }
  })
  it.each([
    ['write', 'file_change', 'toolBuiltinWrite'], ['edit', 'file_change', 'toolBuiltinEdit'],
    ['multi_edit', 'file_change', 'toolBuiltinEdit'], ['bash', 'command_execution', 'dshToolFamily.shell'],
    ['', 'command_execution', 'dshToolFamily.shell'], ['', 'file_change', 'dshToolFamily.diff']
  ] as const)('titles %s from real operation and keeps an unidentified shell generic', async (toolName, toolKind, labelKey) => {
    await render({ user, blocks: [{ ...tool, summary: 'Tool activity', toolKind, meta: { toolName } }] }, { processing: true })
    const row = container.querySelector('[data-analytix-process-block-id="tool"]')!
    const title = row.querySelector(toolKind === 'command_execution'
      ? '[data-variant="bash"] > .' + bashHeaderCss.title
      : '[data-disclosure-row] .' + toolRowCss.title)
    expect(title).not.toBeNull()
    expect(title!.textContent).toBe(i18n.t('common:' + labelKey))
    expect(row.textContent).not.toContain('Bash')
  })
  it('resolves every central error color to the existing distinct light and dark danger tokens', async () => {
    // Vitest intentionally stubs CSS modules, so inspect source declarations.
    // Loading the test-only built-in through Vitest avoids Node globals/types
    // leaking into the renderer web project.
    const fs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
    const base = import.meta.url
    const readCss = (relative: string) => {
      const url = new URL(relative, base); expect(url.protocol).toBe('file:')
      return fs.readFileSync(decodeURIComponent(url.pathname), 'utf8')
    }
    const hiddenSelector = '.processBody[hidden]:not([hidden="until-found"])'
    const hiddenRules: string[] = []
    parse(readCss('./ChatView.module.css')).walkRules(hiddenSelector, rule => { rule.walkDecls('display', declaration => { hiddenRules.push(declaration.value) }) })
    expect(hiddenRules).toEqual(['none'])
    const processBody = document.createElement('div'); processBody.className = 'processBody'
    expect(processBody.matches(hiddenSelector)).toBe(false)
    processBody.hidden = true
    expect(processBody.matches(hiddenSelector)).toBe(true)
    processBody.setAttribute('hidden', 'until-found')
    expect(processBody.matches(hiddenSelector)).toBe(false)
    const theme = parse(readCss('../../../../styles/base-shell.css'))
    const danger = new Set<string>(); theme.walkDecls('--ds-danger', declaration => { danger.add(declaration.value) })
    expect(danger.size).toBeGreaterThan(1)
    const tokens = parse(readCss('../tokens.css'))
    const scopedDanger = new Set<string>(), bridges: string[] = []
    tokens.walkDecls('--dsw-alias-state-error-primary', declaration => { scopedDanger.add(declaration.value) })
    tokens.walkDecls('--ds-danger', declaration => { bridges.push(declaration.value) })
    expect(scopedDanger).toEqual(new Set(['rgb(236, 19, 19)', 'rgb(242, 90, 90)']))
    expect(bridges).toEqual(['var(--dsw-alias-state-error-primary)'])
    for (const [name, selector, expected] of [
      ['ToolRow', '.errorSummary', 'var(--dsw-alias-state-error-primary)'],
      ['ToolRow', '.ioText[data-error]', 'var(--dsw-alias-state-error-primary)'],
      ['QuestionComposer', '.feedback', 'var(--dsw-alias-state-error-primary)'],
      ['ChatView', '.error', 'var(--ds-danger)'], ['ApprovalPanel', '.error', 'var(--ds-danger)']
    ]) {
      const sheet = parse(readCss('./' + name + '.module.css'))
      const values: string[] = []; sheet.walkRules(selector, rule => { rule.walkDecls('color', d => { values.push(d.value) }) })
      expect(values).toEqual([expected])
    }
  })
  it('omits the separator when deduplicating a bare operation leaves no summary', async () => {
    for (const operation of ['read', 'write', 'edit']) {
      await render({ user, blocks: [{ ...tool, summary: '', detail: '', meta: { toolName: operation } }] }, { processing: true })
      const block = container.querySelector('[data-analytix-process-block-id="tool"]')!
      const content = block.querySelector('[data-disclosure-row] > .' + disclosureCss.content)!.firstElementChild!
      expect(content.children).toHaveLength(1) // actual text content: title only, no dot or empty summary
      expect(content.querySelector('[data-shimmer-decoration]')).toBeNull()
    }
  })
  it('counts header-like content inside complete hunks and ignores text outside them', async () => {
    for (const [patch, expected] of [
      ['--- a/x\n+++ b/x\n@@ -0,0 +1 @@\n+++literal\n+OUTSIDE_HUNK', '+1 -0'],
      ['--- a/x\n+++ b/x\n@@ -1 +1 @@\n---old\n+++new\n-OUTSIDE_HUNK', '+1 -1'],
      ['--- a/x\n+++ b/x\n@@ -1 +1,2 @@\n-old\n+new', null],
      ['--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n@@ -2 +2 @@\n---literal\n+++literal', '+2 -2']
    ] as const) {
      await render({ user, blocks: [{ ...tool, toolKind: 'file_change', summary: 'edit: x', detail: patch,
        meta: { toolName: 'edit' } }] }, { processing: true })
      const counts = container.querySelector('[data-analytix-diff-stats]')
      if (expected === null) expect(counts).toBeNull()
      else expect(counts?.textContent).toBe(expected)
    }
  })
})


const privateScope = { threadId: 'thread', turnId: 'turn', contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'incarnation-a' }
it('leases a visible private-only turn without a fake Think and does not acquire again when process/policy changes', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(window, 'analytix')
  const authorize = vi.fn(async (request: { threadId: string; turnId: string }) => ({ status: 'authorized', schemaVersion: 2, viewerId: crypto.randomUUID(), revision: '1', scope: { ...privateScope, threadId: request.threadId, turnId: request.turnId }, presence: { nativeBlockCount: 0, toolCaptureCount: 0 } }))
  const read = vi.fn(async () => ({ status: 'snapshot', schemaVersion: 2, revision: '1', scope: privateScope, units: [], blocks: [] }))
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { authorizeProcessPane: authorize,
    readProcessPane: read, closeProcessPane: vi.fn(async () => ({ status: 'ok' })), setProcessPaneExpanded: async () => ({ status: 'ok' }),
    onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined } } })
  try {
    const turn: ThreadTurn = { user, blocks: [] }
    const props = { processing: true, runtimeAvailable: true, processPaneRoute: { surface: 'main' as const, ownerKey: 'main', selectionKey: 'thread', visible: true } }
    await render(turn, props)
    expect(container.querySelector('[data-process-pane-id]')).not.toBeNull(); expect(authorize).toHaveBeenCalledTimes(1)
    expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    await render(turn, { ...props, processing: false, turnStatus: 'completed' })
    expect(authorize).toHaveBeenCalledTimes(1); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    const reads = read.mock.calls.length
    await act(async () => writeWorkDetailsPreference('compact'))
    expect(read).toHaveBeenCalledTimes(reads); expect(authorize).toHaveBeenCalledTimes(1)
  } finally { await act(async () => root.render(null)); writeWorkDetailsPreference('detailed'); if (descriptor) Object.defineProperty(window, 'analytix', descriptor); else Reflect.deleteProperty(window, 'analytix') }
})
it('lets Go decide authority for a user-opened old turn without authorizing passive history or hidden defaults', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(window, 'analytix')
  const authorize = vi.fn(async (request: { threadId: string; turnId: string }) => ({ status: 'authorized', schemaVersion: 2, viewerId: crypto.randomUUID(), revision: '1', scope: { ...privateScope, threadId: request.threadId, turnId: request.turnId }, presence: { nativeBlockCount: 0, toolCaptureCount: 0 } }))
  const close = vi.fn(async () => ({ status: 'ok' }))
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { authorizeProcessPane: authorize, closeProcessPane: close,
    readProcessPane: async () => ({ status: 'snapshot', schemaVersion: 2, revision: '1', scope: privateScope, units: [], blocks: [] }),
    setProcessPaneExpanded: async () => ({ status: 'ok' }), openProcessCapture: async () => ({ status: 'unavailable' }),
    onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined } } })
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  const oldUser: ChatBlock = { ...user, id: 'historical-user', meta: { turnId: 'historical-turn' } }
  const props = { blocks: [oldUser, { ...tool, id: 'historical-tool', meta: { ...tool.meta, turnId: 'historical-turn' } }, user],
    live: '', activeThreadId: 'thread', runtimeConnection: 'ready' as const, onRetryConnection() {}, onOpenSettings() {},
    runtimeStateOverride: { busy: true, currentTurnId: 'turn', currentTurnUserId: 'user' } }
  try {
    await act(async () => root.render(createElement(MessageTimeline, { ...props, edgeAlignedScroll: true })))
    expect(authorize).toHaveBeenCalledTimes(1)
    await act(async () => writeWorkDetailsPreference('verbose'))
    expect(authorize).toHaveBeenCalledTimes(1)
    await act(async () => writeWorkDetailsPreference('detailed'))
    const historicalHeader = container.querySelector<HTMLElement>('[data-analytix-chat-view] [data-analytix-turn-process]')!
    await act(async () => historicalHeader.click())
    expect(authorize).toHaveBeenCalledTimes(2)
    expect(authorize).toHaveBeenLastCalledWith(expect.objectContaining({ threadId: 'thread', turnId: 'historical-turn' }))
    await act(async () => root.render(createElement(MessageTimeline, { ...props, edgeAlignedScroll: false })))
    expect(close).toHaveBeenCalled(); expect(container.querySelector('[data-protected-process-block]')).toBeNull()
    await act(async () => root.render(createElement(MessageTimeline, { ...props, edgeAlignedScroll: true,
      processPaneRoute: { surface: 'inspector', ownerKey: 'explicit', selectionKey: 'new', visible: false } })))
    expect(authorize).toHaveBeenCalledTimes(2)
  } finally { await act(async () => root.render(null)); if (descriptor) Object.defineProperty(window, 'analytix', descriptor); else Reflect.deleteProperty(window, 'analytix') }
})
it.each([
  ['verbose', 'completed'], ['detailed', 'failed'], ['detailed', 'aborted'], ['detailed', 'completed']
] as const)('keeps an actual user process entry for native-only %s/%s history without passive authority', async (mode, status) => {
  const descriptor = Object.getOwnPropertyDescriptor(window, 'analytix')
  const authorize = vi.fn(async () => ({ status: 'authorized', schemaVersion: 2, viewerId: '11111111-1111-4111-8111-111111111111',
    revision: '1', scope: privateScope, presence: { nativeBlockCount: 1, toolCaptureCount: 0 } }))
  const privateUnit = { unitId: 'history-unit', producerId: 'history-producer', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }
  const privateBlock = { blockId: 'history-private', kind: 'native_reasoning', state: 'completed', attempt: 1, nativeIndex: 0,
    source: { kind: 'native', unitId: privateUnit.unitId, producerId: privateUnit.producerId, providerSequence: '1', attempt: 1, nativeIndex: 0, sourceOrdinal: '1' },
    title: 'OLD_PRIVATE_THINK', preview: 'OLD_PRIVATE_PREVIEW', durationMs: 100, truncated: false, classification: 'credential_redacted', captureDigest: 'b'.repeat(64) }
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    authorizeProcessPane: authorize, closeProcessPane: async () => ({ status: 'ok' }), setProcessPaneExpanded: async () => ({ status: 'ok' }),
    readProcessPane: async () => ({ status: 'snapshot', schemaVersion: 2, revision: '1', scope: privateScope, units: [privateUnit], blocks: [privateBlock] }),
    onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined } } })
  try {
    await act(async () => writeWorkDetailsPreference(mode))
    await render({ user, blocks: [{ kind: 'assistant', id: 'final-history', text: 'Published final' }] }, { turnStatus: status, processPaneAutoAcquire: false,
      runtimeAvailable: true, processPaneRoute: { surface: 'main', ownerKey: 'main', selectionKey: 'old-user-selected', visible: true } })
    expect(authorize).not.toHaveBeenCalled(); expect(container.textContent).not.toContain('OLD_PRIVATE')
    const header = container.querySelector<HTMLElement>('[data-analytix-turn-process]')!
    expect(header.tagName).toBe('BUTTON'); header.focus(); expect(document.activeElement).toBe(header)
    await act(async () => header.click())
    expect(authorize).toHaveBeenCalledTimes(1)
    const group = container.querySelector<HTMLElement>('[data-process-group] > div:first-child [data-disclosure-row]')
    if (group) await act(async () => group.click())
    expect(container.textContent).toContain('OLD_PRIVATE_PREVIEW')
    expect(container.textContent).toContain('Published final')
  } finally {
    await act(async () => root.render(null)); writeWorkDetailsPreference('detailed')
    if (descriptor) Object.defineProperty(window, 'analytix', descriptor); else Reflect.deleteProperty(window, 'analytix')
  }
})
it('uses completed first lines and only finished live paragraph first lines for Think summaries', () => {
  expect(reasoningPreview('**First**', true)).toBe('')
  expect(reasoningPreview('**First**\nbody\n\nSecond incomplete', true)).toBe('First')
  expect(reasoningPreview('**First**\nbody\n\n**Second**\nbody', true)).toBe('Second')
  expect(reasoningPreview('**First**\nbody\n\nSecond\nbody', false)).toBe('First')
  expect(reasoningPreview('\nsecond line', false)).toBe('')
})
it('changes completed folding policy without remounting the public row seat or resetting a manual disclosure', async () => {
  await render({ user, blocks: [tool] }, { processing: true })
  const seat = container.querySelector('[data-chat-seat="tool"]')
  const row = container.querySelector<HTMLElement>('[data-analytix-process-block-id="tool"] [data-disclosure-row]')!
  await act(async () => row.click()); expect(container.textContent).toContain('Public detail text')
  await act(async () => writeWorkDetailsPreference('verbose'))
  expect(container.querySelector('[data-chat-seat="tool"]')).toBe(seat); expect(container.textContent).toContain('Public detail text')
  await render({ user, blocks: [tool] }, { processing: false, turnStatus: 'completed' })
  expect(container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')?.disabled).toBe(true)
  await act(async () => writeWorkDetailsPreference('detailed'))
  expect(container.querySelector('[data-chat-seat="tool"]')).toBe(seat)
})

function nativeChildProgress(id: string, childId: string, childRunId: string, childStatus: RuntimeChildMetadata['childStatus'], summary: string, detail = ''): ToolBlock {
  const child = normalizeChildMetadataForRenderer({ parentThreadId: 'thread', parentTurnId: 'turn', kind: 'subagent',
    childId, childRunId, childThreadId: `${childId}-thread`, jobId: `${childId}-job`, childStatus })
  if (!child) throw new Error('Child fixture must pass the real metadata mapper')
  return { ...tool, id, toolKind: 'subagent', summary, detail, status: childStatus === 'completed' ? 'success' : 'running',
    meta: { ...tool.meta, toolName: 'task', child } }
}

async function expandNativeChildSeat(id: string): Promise<HTMLElement> {
  const seat = container.querySelector<HTMLElement>(`[data-chat-seat="${id}"]`)!
  expect(seat).not.toBeNull()
  const group = seat.closest('[data-process-group]')?.querySelector<HTMLElement>('button[data-process-activity]')
  if (group?.getAttribute('aria-expanded') === 'false') await act(async () => group.click())
  const row = seat.querySelector<HTMLElement>('[data-subagent-row] [data-disclosure-row]')!
  expect(row).not.toBeNull()
  if (row.getAttribute('aria-expanded') === 'true') await act(async () => row.click())
  expect(row.getAttribute('aria-expanded')).toBe('false')
  await act(async () => row.click())
  expect(row.getAttribute('aria-expanded')).toBe('true')
  return seat
}

it('preserves child identity merging for distinct progress item IDs', async () => {
  const first = nativeChildProgress('child-progress-a', 'child-a', 'child-run-a', 'running', 'FIRST_SUMMARY', 'FIRST_DETAIL')
  const next = nativeChildProgress('child-progress-b', 'child-a', 'child-run-a', 'completed', 'SECOND_SUMMARY')
  const sections = groupProcessSections([first, next])
  expect(sections).toHaveLength(1); expect(sections[0].blocks).toHaveLength(1)
  expect(sections[0].blocks[0]).toMatchObject({ id: next.id, summary: 'SECOND_SUMMARY', detail: 'FIRST_DETAIL',
    meta: { child: { childId: 'child-a', childRunId: 'child-run-a', childStatus: 'completed' } } })
  expect(sections[0].blocks[0]).not.toMatchObject({ summary: 'FIRST_SUMMARY' })
  await render({ user, blocks: [first, next] }, { processing: true })
  expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
  expect(container.querySelector('[data-chat-seat="child-progress-a"]')).toBeNull()
  const seat = await expandNativeChildSeat(next.id)
  expect(seat.querySelector('[data-subagent-row] [data-state]')?.getAttribute('data-state')).toBe('ok')
  const details = seat.querySelector('[data-dsh-subagent-details]')!
  expect([...details.querySelectorAll('dd')].map(field => field.textContent)).toEqual(expect.arrayContaining(['child-a', 'child-a-job']))
  expect(details.querySelector('[data-tone="success"]')?.textContent).toBe(i18n.t('common:dshRecordedTool.jobStatus.completed'))
  expect(seat.textContent).not.toContain('undefined')
})

it('keeps interleaved child updates separate after a later accepted completed observation', async () => {
  const firstA = nativeChildProgress('child-a-first', 'child-a', 'child-run-a', 'running', 'A_FIRST_SUMMARY', 'A_FIRST_DETAIL')
  const firstB = nativeChildProgress('child-b-first', 'child-b', 'child-run-b', 'running', 'B_FIRST_SUMMARY', 'B_FIRST_DETAIL')
  const nextA = nativeChildProgress('child-a-next', 'child-a', 'child-run-a', 'completed', 'A_SECOND_SUMMARY')
  const nextB = nativeChildProgress('child-b-next', 'child-b', 'child-run-b', 'completed', 'B_SECOND_SUMMARY')
  const updates = [firstA, firstB, nextA, nextB]
  await render({ user, blocks: updates }, { processing: true })
  expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(2)
  const seatA = await expandNativeChildSeat(nextA.id), seatB = await expandNativeChildSeat(nextB.id)
  expect(seatA.querySelector('[data-tone="success"]')?.textContent).toBe(i18n.t('common:dshRecordedTool.jobStatus.completed'))
  expect(seatB.querySelector('[data-tone="success"]')?.textContent).toBe(i18n.t('common:dshRecordedTool.jobStatus.completed'))
  // A later accepted completed observation updates A only; this does not simulate stale SSE rejection.
  const laterA = { ...nextA, id: 'child-a-completed-followup', summary: 'A_THIRD_SUMMARY' }, followedUp = [...updates, laterA]
  const sections = groupProcessSections(followedUp)
  expect(sections).toHaveLength(1); expect(sections[0].blocks).toHaveLength(2)
  expect(sections[0].blocks[0]).toMatchObject({ id: laterA.id, summary: 'A_THIRD_SUMMARY', detail: 'A_FIRST_DETAIL',
    meta: { child: { childId: 'child-a', childRunId: 'child-run-a', childStatus: 'completed' } } })
  expect(sections[0].blocks[0]).not.toMatchObject({ summary: 'A_SECOND_SUMMARY' })
  expect(sections[0].blocks[1]).toMatchObject({ id: nextB.id, summary: 'B_SECOND_SUMMARY', detail: 'B_FIRST_DETAIL',
    meta: { child: { childId: 'child-b', childRunId: 'child-run-b', childStatus: 'completed' } } })
  expect(sections[0].blocks[1]).not.toMatchObject({ summary: 'B_FIRST_SUMMARY' })
  await render({ user, blocks: followedUp }, { processing: true })
  expect(container.querySelectorAll('[data-chat-seat]')).toHaveLength(2)
  expect(container.querySelector('[data-chat-seat="child-a-next"]')).toBeNull()
  const followupSeatA = await expandNativeChildSeat(laterA.id), currentSeatB = await expandNativeChildSeat(nextB.id)
  expect(followupSeatA.querySelector('[data-subagent-row] [data-state]')?.getAttribute('data-state')).toBe('ok')
  expect([...followupSeatA.querySelectorAll('[data-dsh-subagent-details] dd')].map(field => field.textContent)).toEqual(expect.arrayContaining(['child-a', 'child-a-job']))
  expect(followupSeatA.querySelector('[data-tone="success"]')?.textContent).toBe(i18n.t('common:dshRecordedTool.jobStatus.completed'))
  expect(followupSeatA.textContent).not.toContain('child-b')
  expect(currentSeatB.querySelector('[data-subagent-row] [data-state]')?.getAttribute('data-state')).toBe('ok')
  expect([...currentSeatB.querySelectorAll('[data-dsh-subagent-details] dd')].map(field => field.textContent)).toEqual(expect.arrayContaining(['child-b', 'child-b-job']))
  expect(currentSeatB.querySelector('[data-tone="success"]')?.textContent).toBe(i18n.t('common:dshRecordedTool.jobStatus.completed'))
  expect(currentSeatB.textContent).not.toContain('child-a')
})

// Same existing owner across all process surfaces; history timing never becomes
// the current Turn clock, and settlement removes the live timer.
it.each(['main', 'side', 'inspector'] as const)('belongs only to the actual current Turn on the %s surface', async surface => {
  vi.useFakeTimers(); vi.setSystemTime(20_000)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  const historyUser: ChatBlock = { ...user, id: 'historical-user', meta: { turnId: 'historical-turn' } }
  const props = { blocks: [historyUser, { kind: 'assistant' as const, id: 'historical-final', text: 'History final', meta: { turnId: 'historical-turn' } }, user],
    live: '', activeThreadId: 'thread', runtimeConnection: 'ready' as const, onRetryConnection() {}, onOpenSettings() {}, edgeAlignedScroll: true,
    processPaneRoute: { surface, ownerKey: surface, selectionKey: 'matching-turn', visible: false } }
  const state = { busy: true, currentTurnId: 'turn', currentTurnUserId: 'user',
    authoritativeTurnTimings: {}, authoritativeTurnStatuses: { turn: 'running' as const },
    turnStartedAtByUserId: { 'historical-user': 1000 } }
  await act(async () => root.render(createElement(MessageTimeline, { ...props, runtimeStateOverride: state })))
  const views = container.querySelectorAll('[data-analytix-chat-view]')
  expect(views).toHaveLength(2)
  expect(views[0].querySelector('[data-analytix-live-progress]')).toBeNull()
  const current = views[1].querySelector('[data-analytix-live-progress]')!
  expect(current).not.toBeNull()
  expect(current.querySelector('[aria-live="off"]')).toBeNull()
  await act(async () => appendActiveStreamDeltas({ threadId: 'thread', turnId: 'turn', lastSeq: 1, startedAt: 10_000, runtimeRunning: true }))
  await act(async () => root.render(createElement(MessageTimeline, { ...props, runtimeStateOverride: { ...state,
    turnStartedAtByUserId: { 'historical-user': 1000, user: 10_000 } } })))
  expect(container.querySelectorAll('[data-analytix-live-progress]')).toHaveLength(1)
  expect(accessibleText(container.querySelector('[data-analytix-live-progress] [aria-live="off"]')!)).toBe(i18n.t('common:dshRunningFor', { duration: '10秒' }))
  await act(async () => vi.advanceTimersByTime(1000))
  expect(accessibleText(container.querySelector('[data-analytix-live-progress] [aria-live="off"]')!)).toBe(i18n.t('common:dshRunningFor', { duration: '11秒' }))
  await act(async () => root.render(createElement(MessageTimeline, { ...props, runtimeStateOverride: { ...state, busy: false,
    authoritativeTurnStatuses: { turn: 'completed' }, authoritativeTurnTimings: { turn: { turnId: 'turn', startedAt: 10_000, finishedAt: 13_200 } } } })))
  expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
  await act(async () => vi.advanceTimersByTime(60_000))
  expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
  expect(container.querySelectorAll('[data-analytix-turn-process]')[1].textContent).toContain('3秒')
})

// Renderer/action/adapter/bridge regressions. GET and resident SSE are typed boundary fixtures; no native acceptance is claimed.
type ForkDetail = Awaited<ReturnType<AgentProvider['getThreadDetail']>>
type ForkRequestResult = Awaited<ReturnType<Window['analytix']['runtime']['runtimeRequest']>>

async function settleForkActions() {
  await act(async () => { for (let n = 0; n < 15; n++) await Promise.resolve() })
}
function forkButtonIn(container: HTMLElement) {
  const footer = container.querySelector<HTMLElement>('[data-clock="end"]')
  expect(footer).not.toBeNull()
  const button = Array.from(footer!.querySelectorAll<HTMLButtonElement>('button'))
    .find(item => item.getAttribute('aria-label') === i18n.t('common:forkFromAssistantResponse'))
  expect(button).toBeDefined()
  expect(button!.getAttribute('aria-disabled')).not.toBe('true')
  return button!
}
function installFooterForkHarness(input: {
  sourceThreadId: string; sourceTurnId: string; parentThreadId?: string;
  sourceDetail: ForkDetail
  publicReplayEvents?: Parameters<Parameters<Window['analytix']['runtime']['onSseEvent']>[0]>[0]['events']
}) {
  const sourceId = input.sourceThreadId
  const forkId = 'fork-' + sourceId
  const timestamp = '2026-10-09T00:00:00Z'
  const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
  rendererRuntimeClient.invalidateSettings()
  const settings = normalizeAppSettings({ performanceUsageMode: 'detailed' } as AppSettingsV1)
  const registryKey = 'analytix.threadForks.v1'
  const registryBefore = localStorage.getItem(registryKey)
  // Remove only this fixture's owned IDs, not all storage or all cache entries.
  for (const id of [sourceId, forkId]) invalidateThreadDetailCache(id)
  const source: CoreThreadJson = {
    id: sourceId, title: 'SOURCE ' + sourceId, model: 'synthetic-model', mode: 'agent', status: 'idle',
    ...(input.parentThreadId ? { relation: 'side' as const, parentThreadId: input.parentThreadId } : {}),
    createdAt: timestamp, updatedAt: timestamp
  }
  const parent: CoreThreadJson = {
    ...source, id: input.parentThreadId ?? sourceId, title: 'PARENT ' + (input.parentThreadId ?? sourceId),
    relation: 'primary', parentThreadId: undefined
  }
  const forked: CoreThreadJson = {
    ...source, id: forkId, title: 'Forked ' + sourceId, relation: 'fork', parentThreadId: sourceId,
    forkedFromThreadId: sourceId, forkedFromTitle: source.title,
    forkedFromMessageCount: 2, forkedFromTurnCount: 1,
    latestSeq: 0, turns: []
  }
  const okFork: ForkRequestResult = { ok: true, status: 201, body: JSON.stringify(forked) }
  const usageByThread = new Map<string, ConversationUsageV1 | null>([
    [sourceId, footerConversation(input.sourceTurnId)], [forkId, null]
  ])
  let heldFork: Promise<ForkRequestResult> | null = null
  const request = vi.fn<Window['analytix']['runtime']['runtimeRequest']>(async (path, method, body) => {
    const requestUrl = new URL(path, 'https://fixture.invalid')
    if (requestUrl.pathname === '/v1/usage' && method === 'GET') {
      expect(requestUrl.searchParams.get('group_by')).toBe('thread')
      const requestedId = requestUrl.searchParams.get('thread_id')
      if (!requestedId || !usageByThread.has(requestedId)) throw new Error('Unexpected normal Usage owner: ' + requestedId)
      return normalFooterUsageResponse(requestedId, usageByThread.get(requestedId)!)
    }
    if (path === '/v1/threads/' + sourceId + '/fork' && method === 'POST') {
      expect(JSON.parse(body ?? '{}')).toEqual({ turnId: input.sourceTurnId, endSeq: 13 })
      return heldFork ?? okFork
    }
    if (path.startsWith('/v1/threads?') && method === 'GET') {
      const threads = parent.id === source.id ? [source, forked] : [parent, source, forked]
      return { ok: true, status: 200, body: JSON.stringify({ threads }) }
    }
    if (requestUrl.pathname.startsWith('/v1/threads/') && requestUrl.pathname.endsWith('/goal') && method === 'GET') {
      const owner = decodeURIComponent(requestUrl.pathname.slice('/v1/threads/'.length, -'/goal'.length))
      if (!detailById.has(owner)) throw new Error('Unexpected Goal owner: ' + owner)
      return { ok: true, status: 200, body: JSON.stringify({ goal: null, activation: null, history: [] }) }
    }
    throw new Error('Unexpected synthetic runtime request: ' + method + ' ' + path)
  })
  type ForkSseData = Parameters<Window['analytix']['runtime']['onSseEvent']>[0]
  type ForkSseEnd = Parameters<Window['analytix']['runtime']['onSseEnd']>[0]
  const data = new Set<ForkSseData>(), ends = new Set<ForkSseEnd>()
  const boundedReplayBridge = {
    startSse: vi.fn<Window['analytix']['runtime']['startSse']>(async (threadId, _since, streamId, replay) => {
      if (!streamId || !replay) throw new Error('Missing bounded replay binding')
      expect(threadId).toBe(sourceId)
      queueMicrotask(() => {
        const sourceEvents = input.publicReplayEvents?.filter(event => typeof event.seq === 'number' && event.seq <= replay.replayThroughSeq)
        if (sourceEvents?.length) for (const handler of data) handler({ streamId, events: sourceEvents })
        for (const handler of data) handler({ streamId, events: [{ kind: 'cursor_advanced',
          seq: replay.replayThroughSeq, threadId, timestamp, reason: 'restricted_content_removed' }] })
        for (const handler of ends) handler({ streamId })
      })
      return { streamId }
    }),
    stopSse: vi.fn(async () => undefined), ackSseEvent: vi.fn(async () => true),
    onSseEvent: (handler: ForkSseData) => { data.add(handler); return () => { data.delete(handler) } },
    onSseEnd: (handler: ForkSseEnd) => { ends.add(handler); return () => { ends.delete(handler) } },
    onSseError: () => () => undefined
  }

  Object.defineProperty(window, 'analytix', { configurable: true, value: { settings: { getSettings: async () => settings }, runtime: {
    ...(window.analytix?.runtime ?? {}), ...boundedReplayBridge, runtimeRequest: request,
    authorizeProcessPane: vi.fn(async () => ({ status: 'unavailable' })),
    closeProcessPane: vi.fn(async () => ({ status: 'ok' })),
    onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
  } } })
  const provider = new AnalytixRuntimeProvider()
  const detailById = new Map<string, ForkDetail>([
    [sourceId, { ...input.sourceDetail, thread: threadFromCore(source) }],
    [forkId, { thread: threadFromCore(forked), blocks: [], latestSeq: 0, threadStatus: 'idle' }]
  ])
  const detail = vi.spyOn(provider, 'getThreadDetail').mockImplementation(async id => {
    const value = detailById.get(id)
    if (!value) throw new Error('Unexpected detail owner: ' + id)
    return value
  })
  const streams: Array<{ threadId: string; sink: ThreadEventSink; signal: AbortSignal; close: () => void }> = []
  vi.spyOn(provider, 'subscribeThreadEvents').mockImplementation((threadId, _seq, sink, signal) =>
    new Promise<void>(resolve => {
      const close = () => { signal.removeEventListener('abort', close); resolve() }
      streams.push({ threadId, sink, signal, close })
      if (signal.aborted) close()
      else signal.addEventListener('abort', close, { once: true })
    }))
  const sseAbortRef = { current: null as AbortController | null }
  const set: ChatStoreSet = partial => useChatStore.setState(partial)
  const get: ChatStoreGet = () => useChatStore.getState()
  const context = { set, get, sseAbortRef }
  const threadActions = createThreadActions(context)
  const maintenance = createMaintenanceActions(context)
  set({ ...threadActions, ...maintenance,
    refreshThreads: async () => { set({ threads: await provider.listThreads() }) },
    threads: parent.id === source.id ? [threadFromCore(source)] : [threadFromCore(parent), threadFromCore(source)],
    runtimeConnection: 'ready', watchTurnCompletion: {}, error: null
  })
  return {
    provider, request, detail, detailById, streams, source, forked, threadActions, usageByThread,
    holdFork: () => {
      let release!: (value: ForkRequestResult) => void
      heldFork = new Promise(resolve => { release = resolve })
      return () => { release(okFork) }
    },
    expectSelected: () => {
      const posts = request.mock.calls.filter(([, method]) => method === 'POST')
      expect(posts).toHaveLength(1)
      expect(posts[0]![0]).toBe('/v1/threads/' + sourceId + '/fork')
      expect(JSON.parse(posts[0]![2]!)).toEqual({ turnId: input.sourceTurnId, endSeq: 13 })
      expect(detail).toHaveBeenCalledWith(forkId)
      expect(useChatStore.getState()).toMatchObject({ activeThreadId: forkId, lastSeq: 0, busy: false, error: null })
      expect(streams.some(stream => stream.threadId === forkId && !stream.signal.aborted)).toBe(true)
      expect(readThreadForkRegistry().forks[forkId]).toMatchObject({ parentThreadId: sourceId,
        forkedFromMessageCount: 2, forkedFromTurnCount: 1 })
    },
    dispose: () => {
      // Call only after root.render(null) retires actual mounted Inspector/Side owners.
      sseAbortRef.current?.abort()
      for (const stream of streams) stream.close()
      for (const id of [sourceId, forkId]) invalidateThreadDetailCache(id)
      if (registryBefore === null) localStorage.removeItem(registryKey)
      else localStorage.setItem(registryKey, registryBefore)
      if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge)
      else Reflect.deleteProperty(window, 'analytix')
      rendererRuntimeClient.invalidateSettings()
    }
  }
}

const commonStat = (key: string, values?: Record<string, unknown>) =>
  String(i18n.t('common:' + key, values))
function normalStatValue(selector: string, key: string) {
  const list = document.querySelector(selector)
  expect(list).not.toBeNull()
  const term = Array.from(list!.querySelectorAll('dt')).find(node => node.textContent === commonStat(key))
  expect(term).toBeDefined(); expect(term!.nextElementSibling?.tagName).toBe('DD')
  return term!.nextElementSibling!.textContent
}
async function assertNormalCallerUsage(h: ReturnType<typeof installFooterForkHarness>, sourceId: string) {
  const projection = h.usageByThread.get(sourceId)!
  expect(projection).not.toBeNull()
  const calls = h.request.mock.calls.filter(([path, method]) =>
    method === 'GET' && new URL(path, 'https://fixture.invalid').pathname === '/v1/usage')
  expect(calls.length).toBeGreaterThan(0)
  for (const [path] of calls) {
    const query = new URL(path, 'https://fixture.invalid').searchParams
    expect(query.get('group_by')).toBe('thread'); expect(query.get('thread_id')).toBe(sourceId)
  }
  const turnUsage = container.querySelector<HTMLButtonElement>('[data-analytix-turn-usage] button')
  expect(turnUsage).not.toBeNull()
  expect(turnUsage!.textContent).toContain(String(projection!.billing.totalTokens.reportedValue))
  const pill = container.querySelector<HTMLButtonElement>('[data-composer-stat="usage"] button')
  expect(pill).not.toBeNull()
  expect(pill!.textContent).toContain(String(projection!.billing.totalTokens.reportedValue))
  await act(async () => pill!.click())
  expect(normalStatValue('[data-session-stats-usage]', 'dshUsageInput')).toBe(
    commonStat('dshUsageTokens', { tokens: String(projection!.billing.uncachedInputTokens.reportedValue) }))
  expect(normalStatValue('[data-session-stats-usage]', 'dshUsageOutput')).toBe(
    commonStat('dshUsageTokens', { tokens: String(projection!.billing.outputTokens.reportedValue) }))
  await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
  expect(document.querySelector('[data-session-stats-usage]')).toBeNull()
  if (useChatStore.getState().sidePanel.activeSideId === sourceId) expect(useChatStore.getState().sidePanel.open).toBe(true)
  const activity = container.querySelector<HTMLButtonElement>('[data-composer-stat="activity"] button')
  expect(activity).not.toBeNull()
  await act(async () => activity!.click())
  expect(normalStatValue('[data-session-stats-details]', 'dshStatsTtft')).toBe(
    commonStat('dshDurationCompactSeconds', { seconds: 0.4 }))
  expect(normalStatValue('[data-session-stats-details]', 'dshStatsSpeed')).toBe(
    commonStat('dshUsageTokensPerSecond', { tps: 10 }))
  await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
}

it('Footer receipt primary binding carries real no-compaction hydration into the mounted Footer Fork proof', async () => {
  const { timestamp, batch } = historicalFooterFixture('thread')
  const primary = [chatBlockFromItem({ id: 'primary-user', threadId: 'thread', turnId: 'turn', kind: 'user_message', role: 'user',
    status: 'completed', createdAt: timestamp, text: 'Primary request' })!, chatBlockFromItem(batch.events[0].item as Parameters<typeof chatBlockFromItem>[0])!]
  expect(primary[1]).not.toHaveProperty('generalTerminalProjectionReceipt')
  const h = installFooterForkHarness({ sourceThreadId: 'thread', sourceTurnId: 'turn',
    publicReplayEvents: [batch as Parameters<Parameters<Window['analytix']['runtime']['onSseEvent']>[0]>[0]['events'][number]],
    sourceDetail: { blocks: primary, latestSeq: 13, threadStatus: 'idle', latestTurnId: 'turn', authoritativeTurnStatuses: { turn: 'completed' } } })
  registryFixture.provider = h.provider
  try {
    const detail = await h.provider.getThreadDetail('thread')
    const displayed = await h.provider.restoreThreadDisplayHistory('thread', detail)
    expect(displayed.blocks.map(block => block.id)).toEqual(['primary-user', 'closing'])
    useChatStore.setState({ activeThreadId: 'thread', route: 'chat', blocks: displayed.blocks, lastSeq: 13,
      rewoundTurnIds: {}, busy: false, currentTurnId: null, currentTurnUserId: null })
    const fork = vi.fn(), final = displayed.blocks[1] as Extract<ChatBlock, { kind: 'assistant' }>
    await render({ user: displayed.blocks[0] as Extract<ChatBlock, { kind: 'user' }>, blocks: [final] },
      { turnStatus: 'completed', fork: { busy: false, run: fork } })
    expect(container.querySelector('[data-clock="end"]')).not.toBeNull()
    await act(async () => forkButtonIn(container).click())
    expect(fork).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread', turnId: 'turn', finalItemId: 'closing', endSeq: 13,
      proof: { kind: 'general', receipt: { schemaVersion: 1, batchDigest: batch.batchDigest, threadId: 'thread', turnId: 'turn',
        itemId: 'closing', firstSeq: 11, lastSeq: 13, status: 'completed' } } }))
    expect(primary[1]).not.toHaveProperty('generalTerminalProjectionReceipt')
  } finally {
    await act(async () => root.render(null)); h.dispose(); registryFixture.provider = null; await settleForkActions()
  }
})

it('forks the main Footer owner through the real adapter and actual selectThread', async () => {
  const { timestamp, batch } = historicalFooterFixture('thread')
  const replay = createOrdinaryHistoryReplay('thread', 13)
  replay.accept([{ kind: 'item_created', seq: 1, threadId: 'thread', turnId: 'turn',
    item: { id: 'user', threadId: 'thread', turnId: 'turn', role: 'user', kind: 'user_message',
      status: 'completed', createdAt: timestamp, text: 'Historical request' } }])
  replay.accept([batch])
  const blocks = replay.mergeSnapshot([])
  const h = installFooterForkHarness({ sourceThreadId: 'thread', sourceTurnId: 'turn',
    publicReplayEvents: [batch as Parameters<Parameters<Window['analytix']['runtime']['onSseEvent']>[0]>[0]['events'][number]],
    sourceDetail: { blocks, latestSeq: 13, threadStatus: 'idle', latestTurnId: 'turn',
      authoritativeTurnStatuses: { turn: 'completed' } } })
  registryFixture.provider = h.provider
  try {
    useChatStore.setState({ activeThreadId: 'thread', route: 'chat', blocks, lastSeq: 13,
      rewoundTurnIds: {}, busy: false, currentTurnId: null, currentTurnUserId: null, turnUsageById: replay.turnUsage() })
    await act(async () => root.render(createElement('div', null,
      createElement(MessageTimeline, { blocks, live: '', activeThreadId: 'thread', runtimeConnection: 'ready',
        onRetryConnection() {}, onOpenSettings() {} }),
      createElement(InputBar, {
        input: '', setInput: vi.fn(), mode: 'agent', setMode: vi.fn(), busy: false,
        runtimeReady: true, hasActiveThread: true,
        composerModel: h.source.model, composerPickList: [h.source.model], onComposerModelChange: vi.fn(),
        queuedMessages: [], onRemoveQueuedMessage: vi.fn(), onSend: vi.fn(), onInterrupt: vi.fn(),
        hideThreadContextPanels: true
      })
    )))
    await settleForkActions()
    await assertNormalCallerUsage(h, 'thread')
    // The actual same-cursor snapshot recovery retries the normal Usage GET.
    // A legacy-only response retires billing while leaving committed end proof.
    h.usageByThread.set('thread', null)
    const beforeRefresh = h.request.mock.calls.filter(([path]) => path.startsWith('/v1/usage?')).length
    const sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId: 'thread' })
    await act(async () => sink.onSnapshotRequired?.({ threadId: 'thread', seq: 13, highestSeq: 13 }))
    await settleForkActions()
    expect(h.request.mock.calls.filter(([path]) => path.startsWith('/v1/usage?')).length).toBeGreaterThan(beforeRefresh)
    expect(useChatStore.getState().turnUsageById.turn.snapshot.cacheDiagnostics?.providerAttemptInputTokens?.value).toBe(120)
    expect(container.querySelector('[data-analytix-turn-usage]')).toBeNull()
    expect(container.querySelector('[data-composer-stat="usage"]')).toBeNull()
    expect(container.querySelector('[data-clock="end"]')).not.toBeNull()
    expect(container.querySelector(`button[aria-label="${i18n.t('common:copyMessage')}"]`)).not.toBeNull()

    await act(async () => forkButtonIn(container).click())
    await settleForkActions()
    h.expectSelected()
  } finally {
    await act(async () => root.render(null))
    h.dispose(); registryFixture.provider = null
    await settleForkActions()
  }
})

it('does not navigate an old Footer Fork after a real owner selection wins', async () => {
  const { timestamp, batch } = historicalFooterFixture('thread')
  const replay = createOrdinaryHistoryReplay('thread', 13)
  replay.accept([{ kind: 'item_created', seq: 1, threadId: 'thread', turnId: 'turn',
    item: { id: 'user', threadId: 'thread', turnId: 'turn', role: 'user', kind: 'user_message',
      status: 'completed', createdAt: timestamp, text: 'Historical request' } }])
  replay.accept([batch]); const blocks = replay.mergeSnapshot([])
  const h = installFooterForkHarness({ sourceThreadId: 'thread', sourceTurnId: 'turn',
    sourceDetail: { blocks, latestSeq: 13, threadStatus: 'idle', authoritativeTurnStatuses: { turn: 'completed' } } })
  registryFixture.provider = h.provider
  const other = { ...threadFromCore(h.source), id: 'other-owner', title: 'Other selected owner' }
  const otherBlocks: ChatBlock[] = [{ kind: 'user', id: 'other-user', text: 'NEW OWNER BODY', meta: { turnId: 'other-turn' } }]
  h.detailById.set(other.id, { thread: other, blocks: otherBlocks, latestSeq: 0, threadStatus: 'idle' })
  const releaseFork = h.holdFork()
  try {
    useChatStore.setState({ activeThreadId: 'thread', route: 'chat', blocks, lastSeq: 13,
      busy: false, currentTurnId: null, currentTurnUserId: null, rewoundTurnIds: {} })
    await act(async () => root.render(createElement(MessageTimeline, {
      blocks, live: '', activeThreadId: 'thread', runtimeConnection: 'ready',
      onRetryConnection() {}, onOpenSettings() {}
    })))
    await settleForkActions()
    await act(async () => forkButtonIn(container).click())
    await settleForkActions()
    expect(h.request.mock.calls.filter(([, method]) => method === 'POST')).toHaveLength(1)
    await act(async () => useChatStore.getState().selectThread(other.id))
    expect(useChatStore.getState().activeThreadId).toBe(other.id)
    await act(async () => { releaseFork() })
    await settleForkActions()
    expect(useChatStore.getState()).toMatchObject({ activeThreadId: other.id, blocks: otherBlocks, busy: false, error: null })
    expect(h.detail).not.toHaveBeenCalledWith(h.forked.id)
    expect(h.streams.some(stream => stream.threadId === h.forked.id)).toBe(false)
    expect(h.request.mock.calls.filter(([, method]) => method === 'POST')).toHaveLength(1)
  } finally {
    releaseFork() // also settles the deferred in a failing assertion path
    await act(async () => root.render(null))
    h.dispose(); registryFixture.provider = null
    await settleForkActions()
  }
})


it('forks the real Side Footer source and selects the result without using Main history', async () => {
  const sourceId = 'side-fork-source', { timestamp, batch } = historicalFooterFixture(sourceId)
  const replay = createOrdinaryHistoryReplay(sourceId, 13)
  replay.accept([{ kind: 'item_created', seq: 1, threadId: sourceId, turnId: 'turn',
    item: { id: 'user', threadId: sourceId, turnId: 'turn', role: 'user', kind: 'user_message', status: 'completed', createdAt: timestamp, text: 'SIDE SOURCE HISTORY' } }])
  replay.accept([batch]); const blocks = replay.mergeSnapshot([])
  const side: SideConversation = { threadId: sourceId, parentThreadId: 'parent', title: 'Side source', createdAt: timestamp,
    inheritedAt: timestamp, blocks, liveAssistant: '', lastSeq: 13, input: '', model: '', reasoningEffort: 'medium',
    busy: false, turnId: null, userItemId: 'user', error: null, rewoundTurnIds: {} }
  const h = installFooterForkHarness({ sourceThreadId: sourceId, sourceTurnId: 'turn', parentThreadId: 'parent',
    sourceDetail: { blocks, latestSeq: 13, threadStatus: 'idle', authoritativeTurnStatuses: { turn: 'completed' } } })
  registryFixture.provider = h.provider
  h.usageByThread.set(sourceId, footerConversation('turn', 70, 13))
  try {
    useChatStore.setState({ activeThreadId: 'parent', route: 'chat', lastSeq: 41,
      blocks: [{ kind: 'user', id: 'main-other', text: 'WRONG MAIN OWNER', meta: { turnId: 'main-other' } }],
      busy: false, currentTurnId: null, currentTurnUserId: null, rewoundTurnIds: {},
      sidePanel: { open: true, activeSideId: sourceId }, sideConversations: { [sourceId]: side } })
    await act(async () => root.render(createElement(SideConversationPanel)))
    await settleForkActions()
    await assertNormalCallerUsage(h, sourceId)
    await act(async () => forkButtonIn(container).click())
    await settleForkActions(); h.expectSelected()
    expect(h.request.mock.calls.some(([path]) => path === '/v1/threads/parent/fork')).toBe(false)
    expect(useChatStore.getState().blocks).toEqual([])
    expect(useChatStore.getState().sideConversations[sourceId]?.blocks).toEqual(blocks)
  } finally {
    await act(async () => root.render(null)); h.dispose(); registryFixture.provider = null
    await settleForkActions()
  }
})

it('forks the real Inspector child Footer after its actual mapped terminal batch', async () => {
  const sourceId = 'child-fork-source', { timestamp, batch } = historicalFooterFixture(sourceId)
  const sourceUser: ChatBlock = { kind: 'user', id: 'user', text: 'CHILD SOURCE HISTORY', meta: { turnId: 'turn' } }
  const replay = createOrdinaryHistoryReplay(sourceId, 13)
  replay.accept([{ kind: 'item_created', seq: 1, threadId: sourceId, turnId: 'turn',
    item: { id: 'user', threadId: sourceId, turnId: 'turn', role: 'user', kind: 'user_message', status: 'completed', createdAt: timestamp, text: sourceUser.text } }])
  replay.accept([batch]); const blocks = replay.mergeSnapshot([])
  const h = installFooterForkHarness({ sourceThreadId: sourceId, sourceTurnId: 'turn', parentThreadId: 'parent',
    sourceDetail: { blocks: [sourceUser], latestSeq: 10, latestTurnId: 'turn', latestUserMessageId: 'user', threadStatus: 'running' } })
  registryFixture.provider = h.provider
  h.usageByThread.set(sourceId, footerConversation('turn', 90, 17))
  const child: CoreThreadSummarySubagentJson = { schemaVersion: 1, id: sourceId, key: sourceId, childThreadId: sourceId,
    parentThreadId: 'parent', status: 'active', rawStatus: 'running', outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output', factAnswerAllowed: false, evidenceAuthority: false,
    canContinueParent: false, canReadOutput: false, canOpenThread: true, canKill: false, canRestart: false, updatedAt: timestamp }
  try {
    useChatStore.setState({ activeThreadId: 'parent', route: 'chat', lastSeq: 41,
      blocks: [{ kind: 'user', id: 'main-other', text: 'WRONG MAIN OWNER', meta: { turnId: 'main-other' } }],
      busy: false, currentTurnId: null, currentTurnUserId: null, rewoundTurnIds: {} })
    await act(async () => root.render(createElement(SubagentInspectorPanel, {
      visible: true, subagents: [child], selectedKey: sourceId, runtimeConnection: 'ready', composerModel: 'synthetic-model',
      composerPickList: ['synthetic-model'], composerReasoningEffort: 'medium', setComposerModel() {}, setComposerReasoningEffort() {},
      onSelectSubagent() {}, onCollapse() {}, onRetryConnection() {}, onOpenSettings() {}, tabbedWorkspace: true
    })))
    await settleForkActions()
    expect(container.querySelector('[data-clock="end"]')).toBeNull()
    const stream = h.streams.slice().reverse().find(item => item.threadId === sourceId && !item.signal.aborted)
    expect(stream).toBeDefined()
    h.detailById.set(sourceId, { thread: threadFromCore(h.source), blocks, latestSeq: 13, latestTurnId: 'turn',
      latestUserMessageId: 'user', threadStatus: 'idle', authoritativeTurnStatuses: { turn: 'completed' } })
    await act(async () => dispatchAnalytixRuntimeEvents([batch], stream!.sink, async () => {}))
    await settleForkActions()
    await assertNormalCallerUsage(h, sourceId)
    await act(async () => forkButtonIn(container).click())
    await settleForkActions(); h.expectSelected()
    expect(h.request.mock.calls.some(([path]) => path === '/v1/threads/parent/fork')).toBe(false)
    expect(useChatStore.getState().blocks).toEqual([])
  } finally {
    await act(async () => root.render(null)); h.dispose(); registryFixture.provider = null
    await settleForkActions()
  }
})


describe('committed output cap hint', () => {
  it('shows a cap hint from a scoped final receipt and never from running telemetry', async () => {
    const final: Extract<ChatBlock, { kind: 'assistant' }> = { kind: 'assistant', id: 'cap-final', text: 'Safe partial answer', meta: { turnId: 'turn' },
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'thread', turnId: 'turn', itemId: 'cap-final',
        firstSeq: 20, lastSeq: 22, status: 'completed', endReason: { kind: 'max-tokens' } } }
    await render({ user, blocks: [final] })
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).not.toBeNull()
    await render({ user, blocks: [final] }, { processing: true })
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).toBeNull()
    await render({ user, blocks: [{ ...final, generalTerminalProjectionReceipt: undefined }] }, { turnStatus: 'completed' })
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).toBeNull()
    await render({ user, blocks: [{ ...final, generalTerminalProjectionReceipt: { ...final.generalTerminalProjectionReceipt!, status: 'failed' } }] })
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).toBeNull()
  })
  it('shows an empty completed cap without creating an assistant bubble and rejects another turn receipt', async () => {
    const capUser: Extract<ChatBlock, { kind: 'user' }> = { ...user as Extract<ChatBlock, { kind: 'user' }>,
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'thread', turnId: 'turn', itemId: 'user',
        firstSeq: 20, lastSeq: 21, status: 'completed', endReason: { kind: 'max-tokens' } } }
    await render({ user: capUser, blocks: [] })
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).not.toBeNull()
    expect(container.textContent).not.toContain('Safe partial answer')
    await render({ user: { ...capUser, generalTerminalProjectionReceipt: { ...capUser.generalTerminalProjectionReceipt!, turnId: 'other-turn' } }, blocks: [] })
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).toBeNull()
  })
})


it('renders controlled Goal source from its actual item without human input chrome', async () => {
  const block = chatBlockFromItem({ id: 'goal-user', threadId: 'thread', turnId: 'turn', role: 'user',
    kind: 'user_message', status: 'completed', createdAt: '2026-10-08T00:00:00Z', text: '<goal_round>actual internal continuation</goal_round>',
    source: { kind: 'goal', goalId: 'actual-goal', revision: 4, round: 3 } } as CoreTurnItemJson)
  expect(block?.kind).toBe('user')
  await render({ user: block as Extract<ChatBlock, { kind: 'user' }>, blocks: [] })
  expect(container.querySelector('[data-goal-round-source]')).not.toBeNull()
  expect(container.textContent).toContain(i18n.t('common:goalRoundSourceTriggered', { round: 3 }))
  expect(container.textContent).toContain('actual-goal')
  expect(container.querySelector('[data-user-message-bubble]')).toBeNull()
  expect(container.querySelector('textarea')).toBeNull()
  expect(container.textContent).not.toContain('actual internal continuation')
  expect(container.querySelector('[data-goal-round-source]')!.querySelector('button')).toBeNull()
  await render({ user, blocks: [] })
  expect(container.querySelector('[data-user-message-bubble]')).not.toBeNull()
  expect(container.querySelector('[data-goal-round-source]')).toBeNull()
})
it('does not guess Goal attribution from malformed source or ordinary prompt prose', async () => {
  await render({ user: { ...user, source: { kind: 'goal', goalId: 'goal', round: -1 } as never }, blocks: [] })
  expect(container.textContent).toContain(i18n.t('common:goalRoundSourceUnavailable'))
  expect(container.querySelector('[data-user-message-bubble]')).toBeNull()
  await render({ user: { ...user, text: '<goal_round>Human-authored text</goal_round>' }, blocks: [] })
  expect(container.querySelector('[data-user-message-bubble]')).not.toBeNull()
  expect(container.querySelector('[data-goal-round-source]')).toBeNull()
})
it('shows one global running tail only after a bound native start, never from pending gates or queued sends', async () => {
  useChatStore.setState({ activeThreadId: 'thread', currentTurnId: 'turn', currentTurnUserId: user.id,
    blocks: [user, { kind: 'approval', id: 'approval', approvalId: 'approval', summary: 'Approve', status: 'pending', meta: { turnId: 'turn' } }],
    busy: false, runtimeConnection: 'ready' })
  const draw = () => act(async () => root.render(createElement(MessageTimeline, { blocks: useChatStore.getState().blocks,
    live: '', activeThreadId: 'thread', runtimeConnection: 'ready', onRetryConnection() {}, onOpenSettings() {} })))
  await draw()
  expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
  await act(async () => useChatStore.setState({ busy: true, currentTurnId: pendingActiveStreamTurnId('thread', user.id) }))
  await draw(); expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
  const sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId: 'thread' })
  await act(async () => dispatchAnalytixRuntimeEvents([{ kind: 'turn_started', seq: 1, threadId: 'thread', turnId: 'turn',
    timestamp: new Date(Date.now() - 2000).toISOString() }], sink, async () => undefined))
  await draw(); expect(container.querySelectorAll('[data-analytix-live-progress]')).toHaveLength(1)
  await act(async () => clearActiveStream('thread', 'turn'))
  await draw(); expect(container.querySelector('[data-analytix-live-progress]')).toBeNull()
})


it('carries user action recency through the actual turn and nested steering render path', async () => {
  const steering: Extract<ChatBlock, { kind: 'user' }> = { ...user, id: 'steering-input', text: 'Later user steering' }
  const turn: ThreadTurn = { user, blocks: [steering] }
  await render(turn, { tailReveal: 'always' })
  const mainRow = container.querySelector('[data-clock="start"]')!.closest('[data-actions-reveal]')!
  expect(mainRow.getAttribute('data-actions-reveal')).toBe('hover')
  const steeringBubble = [...container.querySelectorAll('[data-user-message-bubble]')].find(node => node.textContent?.includes('Later user steering'))!
  expect(steeringBubble.closest('[data-actions-reveal]')!.getAttribute('data-actions-reveal')).toBe('always')
  await render(turn, { tailReveal: 'hover' })
  expect([...container.querySelectorAll('[data-clock="start"]')].map(node => node.closest('[data-actions-reveal]')!.getAttribute('data-actions-reveal'))).toEqual(['hover', 'hover'])
  await render({ user, blocks: [] }, { tailReveal: 'always' })
  expect(container.querySelector('[data-clock="start"]')!.closest('[data-actions-reveal]')!.getAttribute('data-actions-reveal')).toBe('always')
})


it.each(['completed', 'failed', 'aborted'] as const)('uses authoritative %s only for the process header, preserving running leaves', async turnStatus => {
  const previousMode = readWorkDetailsPreference()
  try {
    await act(async () => writeWorkDetailsPreference('standard'))
    const running: ToolBlock = { ...tool, status: 'running' }
    const turn = { user, blocks: [running] }
    await render(turn, { processing: true })
    const header = container.querySelector<HTMLButtonElement>('[data-process-group] button[data-process-activity]')!
    expect(header.querySelector('[data-shimmer="true"]')).not.toBeNull()
    await render(turn, { processing: true, turnStatus })
    expect(header.querySelector('[data-shimmer="true"]')).toBeNull()
    expect(header.textContent).not.toContain('read public fixture')
    await act(async () => header.click())
    expect(container.querySelector('[data-chat-seat="tool"] [data-state="running"]')).not.toBeNull()
    expect(turn.blocks[0].status).toBe('running')
  } finally { await act(async () => writeWorkDetailsPreference(previousMode)) }
})

it('passes the extracted reply boundary to the real Pane without closing later tool work', async () => {
  const previousMode = readWorkDetailsPreference()
  try {
    await act(async () => writeWorkDetailsPreference('standard'))
    const before = { ...tool, id: 'before-extracted-reply', status: 'running' as const }
    const after = { ...tool, id: 'after-extracted-reply', status: 'running' as const }
    const reply: ChatBlock = { kind: 'assistant', id: 'extracted-reply', text: 'Actual final reply' }
    await render({ user, blocks: [before, reply, after] })
    expect([...container.querySelectorAll('[data-process-group]')].map(group => group.getAttribute('data-process-group')))
      .toEqual(['public:before-extracted-reply', 'public:after-extracted-reply'])
    expect(container.querySelectorAll('.ds-chat-answer')).toHaveLength(1)
    expect(container.querySelector('.ds-chat-answer')?.textContent).toContain('Actual final reply')
  } finally { await act(async () => writeWorkDetailsPreference(previousMode)) }
})
