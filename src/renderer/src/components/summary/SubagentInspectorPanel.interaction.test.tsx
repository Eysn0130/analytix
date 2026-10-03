// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import type { CoreThreadSummarySubagentJson } from '../../agent/analytix-contract'
import type { AcceptedFinalProjectionBatch, ThreadEventSink } from '../../agent/types'
import { dispatchAnalytixRuntimeEvents } from '../../agent/analytix-mapper'
import { useChatStore } from '../../store/chat-store'
import { clearActiveStream } from '../../thread/streaming/active-stream-store'
import i18n from '../../i18n'
import { MessageBubble } from '../chat/message-timeline-bubbles'
import { SubagentInspectorPanel } from './SubagentInspectorPanel'
import type { MessageTimeline } from '../chat/MessageTimeline'
const io = vi.hoisted(() => ({
  detail: vi.fn(), streams: [] as Array<{ threadId: string; sink: ThreadEventSink; signal: AbortSignal }>,
  openOwner: vi.fn(),
  timeline: null as ComponentProps<typeof MessageTimeline> | null
}))
vi.mock('../../agent/registry', () => ({ getProvider: () => ({
  getThreadDetail: io.detail,
  subscribeThreadEvents: (threadId: string, _seq: number, sink: ThreadEventSink, signal: AbortSignal) => {
    io.streams.push({ threadId, sink, signal }); return new Promise<void>(() => {})
  }
}) }))
vi.mock('../chat/FloatingComposer', () => ({ FloatingComposer: () => null }))
vi.mock('../chat/MessageTimeline', () => ({ MessageTimeline: (props: ComponentProps<typeof MessageTimeline>) => {
  io.timeline = props
  return createElement('div', {}, props.blocks.map(block => createElement(MessageBubble, { key: block.id, block })))
} }))
const timestamp = '2026-10-02T00:00:00Z'
const user = { kind: 'user' as const, id: 'user-child', text: 'Synthetic request', meta: { turnId: 'turn-child' } }
const snapshot = (latestSeq = 10) => ({ blocks: [user], latestSeq, latestTurnId: 'turn-child', latestUserMessageId: user.id, threadStatus: 'running' })
function agent(id: string): CoreThreadSummarySubagentJson {
  return { schemaVersion: 1, id, key: id, childThreadId: id, parentThreadId: 'parent', status: 'active', rawStatus: 'running',
    outputWithheld: true, outputTrustStatus: 'untrusted_child_output', factAnswerAllowed: false, evidenceAuthority: false,
    canContinueParent: false, canReadOutput: false, canOpenThread: true, canKill: false, canRestart: false, updatedAt: timestamp }
}
function generalBatch() {
  return { schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1', kind: 'general_terminal_batch',
    batchDigest: 'a'.repeat(64), threadId: 'child-a', turnId: 'turn-child', seq: 13, firstSeq: 11, lastSeq: 13,
    timestamp, generalTerminalCommitId: 'b'.repeat(64), generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: 'c'.repeat(64), eventManifestDigest: 'd'.repeat(64), projectedEventsDigest: 'e'.repeat(64),
    transportAuthority: 'host_batch_digest_v1', evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
    events: [
      { kind: 'item_completed', seq: 11, timestamp, threadId: 'child-a', turnId: 'turn-child', itemId: 'terminal-child',
        item: { id: 'terminal-child', turnId: 'turn-child', threadId: 'child-a', role: 'assistant', status: 'completed',
          createdAt: timestamp, finishedAt: timestamp, kind: 'assistant_text',
          text: '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。' } },
      { kind: 'usage', seq: 12, timestamp, threadId: 'child-a', turnId: 'turn-child', model: 'synthetic-model',
        usage: { promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3, cacheHitRate: null,
          cacheableTokenHitRate: null, totalInputTokenHitRate: null, cacheMissReasons: [], cacheSuggestions: [],
          costUsd: 0, costCny: 0, priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0, tokenEconomySavingsTokens: 0, turns: 1 },
        cacheDiagnostics: {}, usageFinalStatus: 'completed' },
      { kind: 'turn_completed', seq: 13, timestamp, threadId: 'child-a', turnId: 'turn-child', status: 'completed', terminalReason: 'success' }
    ],
    eventManifest: [
      { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
    ] }
}
function acceptedFinalProjectionBatch(
  overrides: Partial<AcceptedFinalProjectionBatch> = {}
): AcceptedFinalProjectionBatch {
  const publicationCommitId = 'f'.repeat(64)
  return {
    batchId: '7'.repeat(64),
    threadId: 'child-a',
    turnId: 'turn-child',
    publicationCommitId,
    firstSeq: 11,
    lastSeq: 13,
    receipt: {
      schemaVersion: 1,
      batchId: '7'.repeat(64),
      threadId: 'child-a',
      turnId: 'turn-child',
      publicationCommitId,
      lastSeq: 13
    },
    assistant: {
      kind: 'assistant',
      id: 'item-final',
      createdAt: '2026-07-18T00:00:00Z',
      text: 'verified boundary answer',
      acceptedFinalView: {
        schemaVersion: 3,
        acceptedFinalDigest: publicationCommitId,
        publicationState: 'accepted',
        variant: 'GeneralGuidanceAnswer',
        terminalReason: 'success',
        blockerCode: '',
        coverageStatus: 'guidance_only',
        checkedScopeDigest: '',
        missingScopeCount: 0,
        claimCount: 0,
        claimTypes: [],
        receiptMetadata: {
          projection: 'masked_metadata_only', count: 0, setDigest: '8'.repeat(64), citations: []
        },
        noHitWording: '',
        acceptedAt: '2026-07-18T00:00:00Z'
      },
      meta: { turnId: 'turn-child' }
    },
    usage: {
      inputTokens: 10,
      outputTokens: 3,
      reasoningTokens: 0,
      cachedTokens: 5,
      cacheMissTokens: 5,
      cacheHitRate: 0.5,
      totalTokens: 13,
      costUsd: null,
      costCny: null,
      priceConfigured: false,
      tokenEconomySavingsTokens: 5,
      turns: 1
    },
    terminal: {
      status: 'completed',
      createdAt: '2026-07-18T00:00:00Z',
      acceptedFinalDigest: publicationCommitId,
      terminalReason: 'success'
    },
    ...overrides
  }
}

let root: Root, container: HTMLDivElement
let previous: ReturnType<typeof useChatStore.getState>
async function render(selectedKey = 'child-a') {
  await act(async () => root.render(createElement(SubagentInspectorPanel, {
    subagents: [agent('child-a'), agent('child-b')], selectedKey, runtimeConnection: 'ready', composerModel: 'synthetic-model',
    composerPickList: ['synthetic-model'], composerReasoningEffort: 'medium', setComposerModel() {}, setComposerReasoningEffort() {},
    onSelectSubagent() {}, onOpenThreadRequest: io.openOwner, onCollapse() {}, onRetryConnection() {}, onOpenSettings() {}, tabbedWorkspace: true
  })))
}
async function settle() { await act(async () => { for (let n = 0; n < 15; n++) await Promise.resolve() }) }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  await i18n.changeLanguage('zh')
  previous = useChatStore.getState()
  useChatStore.setState({ activeThreadId: 'parent', blocks: [], route: 'chat' })
  io.openOwner.mockReset(); io.streams = []; io.timeline = null; io.detail.mockReset(); io.detail.mockResolvedValue(snapshot())
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); clearActiveStream(); useChatStore.setState(previous)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
describe('inspector public delivery and snapshot ownership', () => {
  it('commits an existing runtime general terminal batch through the actual mapper', async () => {
    await render(); await settle()
    const stream = io.streams.find(stream => stream.threadId === 'child-a')!
    await act(async () => { await dispatchAnalytixRuntimeEvents([generalBatch()], stream.sink, async () => {}) })
    expect(io.timeline!.blocks.some(block => block.id === 'terminal-child')).toBe(true)
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    expect(useChatStore.getState().blocks).toEqual([])
  })
  it('keeps a newer public gate when an older detail request finishes', async () => {
    await render(); await settle()
    const stream = io.streams.find(stream => stream.threadId === 'child-a')!
    let finish!: (value: ReturnType<typeof snapshot>) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => { stream.sink.onTurnComplete({ seq: 11, turnId: 'turn-child' }) })
    await act(async () => { stream.sink.onSeq(12); stream.sink.onUserInput({ itemId: 'input-child-item', requestId: 'input-child', questions: [{ id: 'q', header: 'Synthetic question', question: 'Synthetic question', options: [] }], turnId: 'turn-child' }) })
    expect(io.timeline!.blocks.some(block => block.kind === 'user_input')).toBe(true)
    await act(async () => { finish(snapshot(11)) })
    expect(io.timeline!.blocks.some(block => block.kind === 'user_input')).toBe(true)
  })
  it('commits and idempotently replays an accepted projection before returning its exact receipt', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    const batch = acceptedFinalProjectionBatch()
    await act(async () => {
      expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt)
      expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt)
    })
    expect(io.timeline!.blocks.filter(block => block.id === batch.assistant.id)).toHaveLength(1)
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ acceptedFinalProjectionReceipt: batch.receipt })
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    expect(useChatStore.getState().blocks).toEqual([])
  })
  it('rejects an invalid receipt and a closed selection without granting a receipt', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    const batch = acceptedFinalProjectionBatch()
    await act(async () => { await expect(sink.onAcceptedFinalBatch!({ ...batch, receipt: { ...batch.receipt, lastSeq: 14 } })).rejects.toThrow('binding') })
    expect(io.timeline!.blocks.some(block => block.id === batch.assistant.id)).toBe(false)
    await render('child-b'); await settle()
    await expect(sink.onAcceptedFinalBatch!(batch)).rejects.toThrow('inactive')
    expect(useChatStore.getState().blocks).toEqual([])
  })
  it('opens the explicit owning thread for a pending gate, without invoking the main responder', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    await act(async () => sink.onUserInput({ itemId: 'input-owned', requestId: 'request-owned', turnId: 'turn-child',
      questions: [{ id: 'q', header: 'Synthetic question', question: 'Synthetic question', options: [] }] }))
    const button = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent?.includes(i18n.t('common:respondInOwnedThread')))!
    expect(button).toBeDefined()
    expect(container.querySelectorAll('textarea')).toHaveLength(0)
    expect(container.textContent).toContain(i18n.t('common:pendingRequestNotResolved'))
    await act(async () => button.click())
    expect(io.openOwner).toHaveBeenCalledWith('child-a', 'request-owned')
    expect(useChatStore.getState().blocks).toEqual([])
  })
  it('rejects the old A detail promise after selection A to B to A', async () => {
    let finish!: (value: ReturnType<typeof snapshot>) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render('child-a'); await render('child-b'); await settle(); await render('child-a'); await settle()
    await act(async () => finish({ ...snapshot(10), blocks: [{ ...user, text: 'OBSOLETE A SNAPSHOT' }] }))
    expect(container.textContent).not.toContain('OBSOLETE A SNAPSHOT')
    expect(io.detail.mock.calls.filter(call => call[0] === 'child-a')).toHaveLength(2)
  })
})
