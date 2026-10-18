// @vitest-environment jsdom
import { act, createElement, useEffect, useRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import type { CoreThreadSummarySubagentJson } from '../../agent/analytix-contract'
import type { AgentProvider, AcceptedFinalProjectionBatch, ChatBlock, ThreadEventSink } from '../../agent/types'
import type { ProcessPaneAuthorizeRequestV1, ProcessPaneReadRequestV1 } from '../../../../../packages/runtime/src/contracts/protected-process-local-display'
import { dispatchAnalytixRuntimeEvents, generalTerminalProjectionBatchFromRuntime, turnUsageEntryFromBatch } from '../../agent/analytix-mapper'
import { AnalytixRuntimeProvider } from '../../agent/analytix-runtime'
import { resetPublicProjectionRevocationsForTests } from '../../lib/public-projection-revocation'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { ConversationUsageV1Schema, ThreadUsageResponseSchema } from '../../../../../packages/runtime/src/contracts/usage'
import { CompactionEvent } from '../../../../../packages/runtime/src/contracts/events'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { useChatStore } from '../../store/chat-store'
import { clearActiveStream } from '../../thread/streaming/active-stream-store'
import i18n from '../../i18n'
import { MessageBubble } from '../chat/message-timeline-bubbles'
import { SubagentInspectorPanel } from './SubagentInspectorPanel'
import type { MessageTimeline } from '../chat/MessageTimeline'
import type { InputBar } from '../chat/InputBar'
const io = vi.hoisted(() => ({
  runtimeProvider: null as AgentProvider | null,
  detail: vi.fn(), streams: [] as Array<{ threadId: string; sink: ThreadEventSink; signal: AbortSignal; close: () => void }>,
  openOwner: vi.fn(),
  realTimeline: false,
  timeline: null as ComponentProps<typeof MessageTimeline> | null,
  timelineFrames: [] as Array<{ threadId: ComponentProps<typeof MessageTimeline>['activeThreadId']; text: string }>,
  realComposer: false,
  voice: null as Parameters<typeof import('../chat/use-voice-dictation').useVoiceDictation>[0] | null,
  composer: null as ComponentProps<typeof InputBar> | null
}))
vi.mock('../../agent/registry', () => ({ getProvider: () => io.runtimeProvider ?? ({
  getThreadDetail: io.detail,
  subscribeThreadEvents: (threadId: string, _seq: number, sink: ThreadEventSink, signal: AbortSignal) => {
    return new Promise<void>(resolve => { io.streams.push({ threadId, sink, signal, close: resolve }) })
  }
}) }))
vi.mock('../chat/use-voice-dictation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../chat/use-voice-dictation')>()
  return { ...actual, useVoiceDictation(options: Parameters<typeof actual.useVoiceDictation>[0]) {
    io.voice = options
    return actual.useVoiceDictation(options)
  } }
})
vi.mock('../chat/InputBar', async () => {
  const actual = await vi.importActual<typeof import('../chat/InputBar')>('../chat/InputBar')
  return { InputBar: (props: ComponentProps<typeof InputBar>) => {
    io.composer = props
    return io.realComposer ? createElement(actual.InputBar, props) : null
  } }
})
vi.mock('../chat/MessageTimeline', async () => {
  const actual = await vi.importActual<typeof import('../chat/MessageTimeline')>('../chat/MessageTimeline')
  return { MessageTimeline: (props: ComponentProps<typeof MessageTimeline>) => {
    const latest = useRef(props)
    latest.current = props
    useEffect(() => () => { if (io.timeline === latest.current) io.timeline = null }, [])
    io.timeline = props
    io.timelineFrames.push({ threadId: props.activeThreadId, text: JSON.stringify(props.blocks) })
    return io.realTimeline ? createElement(actual.MessageTimeline, props) : createElement('div', {}, props.blocks.map(block => createElement(MessageBubble, { key: block.id, block })))
  } }
})
const timestamp = '2026-10-02T00:00:00Z'
const user = { kind: 'user' as const, id: 'user-child', text: 'Synthetic request', meta: { turnId: 'turn-child' } }
type Detail = Awaited<ReturnType<AgentProvider['getThreadDetail']>>
type PublicSsePayload = Parameters<Parameters<Window['analytix']['runtime']['onSseEvent']>[0]>[0]
const snapshot = (latestSeq = 10): Detail => ({ blocks: [user], latestSeq, latestTurnId: 'turn-child', latestUserMessageId: user.id, threadStatus: 'running' })
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
async function render(selectedKey = 'child-a', visible = true, subagents = [agent('child-a'), agent('child-b')]) {
  await act(async () => root.render(createElement(SubagentInspectorPanel, {
    visible, subagents, selectedKey, runtimeConnection: 'ready', composerModel: 'synthetic-model',
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
  resetPublicProjectionRevocationsForTests(); io.runtimeProvider = null
  io.realTimeline = false; io.realComposer = false; io.composer = null; io.voice = null
  io.openOwner.mockReset(); io.streams = []; io.timeline = null; io.timelineFrames = []; io.detail.mockReset(); io.detail.mockResolvedValue(snapshot())
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); clearActiveStream(); resetPublicProjectionRevocationsForTests(); useChatStore.setState(previous)
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
function inspectorCompactionEvent(threadId = 'child-a', seq = 11) {
  const facts = { threadId, turnId: 'turn-child', compactionId: 'compact-live', summaryKind: 'conversation' as const,
    summary: 'INSPECTOR_ORDINARY_SUMMARY', replacedTokens: 1234, auto: false,
    pinnedConstraints: ['user: preserve recent turns'], sourceDigest: 'a'.repeat(64),
    digestMarker: `sha256:${'a'.repeat(12)}`, sourceItemIds: ['source-child'],
    reasoningExcluded: true as const, reasoningExclusionProof: `sha256:${'b'.repeat(64)}`,
    summaryDigest: 'c'.repeat(64), checkpointDigest: 'd'.repeat(64), shadowedItemCount: 3,
    tokenCountKind: 'estimated' as const, tokenEstimatorVersion: 1 as const,
    createdAt: timestamp, finishedAt: '2026-10-02T00:00:02Z' }
  const item = { ...facts, id: facts.compactionId, kind: 'compaction', role: 'system', status: 'completed', schemaVersion: 5,
    toolPayloadsExcluded: true, caseFactsExcluded: true, providerHistoryProjectionVersion: 3 }
  return CompactionEvent.parse({ ...facts, kind: 'compaction_completed', schemaVersion: 3, seq, timestamp: facts.finishedAt,
    itemId: facts.compactionId, turn: { id: facts.turnId, threadId, status: 'completed', prompt: '/compact',
      createdAt: timestamp, startedAt: timestamp, finishedAt: facts.finishedAt,
      items: [item], steering: [], attachmentIds: [], activeSkillIds: [], injectedMemoryIds: [] } })
}

function inspectorCompactionButton() {
  return container.querySelector<HTMLButtonElement>('[data-analytix-compaction] button')!
}

describe('Inspector compaction summary live disclosure', () => {
  it('opens the mapped ordinary summary through the actual Compaction leaf and replays without duplication', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    const event = inspectorCompactionEvent()
    await act(async () => dispatchAnalytixRuntimeEvents([event], sink, async () => {}))
    const button = inspectorCompactionButton()
    expect(button.disabled).toBe(false)
    expect(button.textContent).toContain(i18n.t('common:dshCompactionShadowedItems', { count: 3 }))
    expect(button.textContent).toContain(i18n.t('common:dshCompactionEstimatedTokens', { count: 1234 }))
    expect(button.textContent).not.toContain(i18n.t('common:dshCompactionSummaryUnavailable'))
    expect(button.getAttribute('aria-expanded')).toBe('false')
    await act(async () => button.click())
    const bodyId = button.getAttribute('aria-controls')!
    expect(document.getElementById(bodyId)?.textContent).toBe('INSPECTOR_ORDINARY_SUMMARY')
    await act(async () => dispatchAnalytixRuntimeEvents([event], sink, async () => {}))
    expect(io.timeline!.blocks.filter(block => block.kind === 'compaction')).toHaveLength(1)
    expect(document.getElementById(bodyId)?.textContent).toBe('INSPECTOR_ORDINARY_SUMMARY')
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it('keeps legacy notices and summary-free failures unexpandable without inventing summary facts', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    const legacy = CompactionEvent.parse({ kind: 'compaction_completed', seq: 11, timestamp,
      threadId: 'child-a', turnId: 'turn-child', itemId: 'compact-legacy', summary: 'LEGACY_NOTICE_ONLY' })
    await act(async () => dispatchAnalytixRuntimeEvents([legacy], sink, async () => {}))
    const legacyButton = inspectorCompactionButton()
    expect(legacyButton.disabled).toBe(true)
    expect(legacyButton.getAttribute('aria-controls')).toBeNull()
    expect(legacyButton.textContent).toContain(i18n.t('common:dshCompactionSummaryUnavailable'))
    expect(io.timeline!.blocks.find(block => block.id === 'compact-legacy')).not.toHaveProperty('summaryKind', 'conversation')
    const failed = CompactionEvent.parse({ kind: 'compaction_failed', schemaVersion: 3, seq: 12, timestamp,
      threadId: 'child-a', turnId: 'turn-child', itemId: 'compact-missing', compactionId: 'compact-missing',
      auto: false, reasonCode: 'summary_unavailable', finishedAt: timestamp })
    await act(async () => dispatchAnalytixRuntimeEvents([failed], sink, async () => {}))
    const buttons = container.querySelectorAll<HTMLButtonElement>('[data-analytix-compaction] button')
    expect(buttons).toHaveLength(2)
    expect(Array.from(buttons).every(button => button.disabled && !button.hasAttribute('aria-controls'))).toBe(true)
    expect(io.timeline!.blocks.find(block => block.id === 'compact-missing')).toMatchObject({ detail: undefined, summaryKind: undefined })
  })

  it('clears missing repeated-summary facts and retires the old body when Inspector selection changes', async () => {
    await render(); await settle()
    const oldSink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    await act(async () => dispatchAnalytixRuntimeEvents([inspectorCompactionEvent()], oldSink, async () => {}))
    const button = inspectorCompactionButton()
    expect(button.disabled).toBe(false)
    await act(async () => button.click())
    expect(document.getElementById(button.getAttribute('aria-controls')!)?.textContent).toBe('INSPECTOR_ORDINARY_SUMMARY')
    const notice = CompactionEvent.parse({ kind: 'compaction_completed', seq: 12, timestamp,
      threadId: 'child-a', turnId: 'turn-child', itemId: 'compact-live', summary: 'REPLAY_NOTICE_WITHOUT_SUMMARY_FACTS' })
    await act(async () => dispatchAnalytixRuntimeEvents([notice, notice], oldSink, async () => {}))
    expect(io.timeline!.blocks.filter(block => block.kind === 'compaction')).toHaveLength(1)
    expect(inspectorCompactionButton().disabled).toBe(true)
    expect(container.textContent).not.toContain('INSPECTOR_ORDINARY_SUMMARY')
    await act(async () => dispatchAnalytixRuntimeEvents([inspectorCompactionEvent('child-a', 13)], oldSink, async () => {}))
    const restoredButton = inspectorCompactionButton()
    expect(restoredButton.disabled).toBe(false)
    expect(restoredButton.getAttribute('aria-expanded')).toBe('true')
    expect(document.getElementById(restoredButton.getAttribute('aria-controls')!)?.textContent).toBe('INSPECTOR_ORDINARY_SUMMARY')
    await render('child-b'); await settle()
    expect(container.textContent).not.toContain('INSPECTOR_ORDINARY_SUMMARY')
    await act(async () => dispatchAnalytixRuntimeEvents([inspectorCompactionEvent('child-a', 14)], oldSink, async () => {}))
    expect(io.timeline!.activeThreadId).toBe('child-b')
    expect(container.textContent).not.toContain('INSPECTOR_ORDINARY_SUMMARY')
    const newSink = io.streams.find(stream => stream.threadId === 'child-b')!.sink
    await act(async () => dispatchAnalytixRuntimeEvents([{ ...notice, threadId: 'child-b', seq: 11 }], newSink, async () => {}))
    expect(inspectorCompactionButton().disabled).toBe(true)
    expect(container.textContent).not.toContain('INSPECTOR_ORDINARY_SUMMARY')
  })
})

describe('Inspector child loading and empty localization', () => {
  it('uses English labels through the selected child entry and responds to a locale change', async () => {
    await i18n.changeLanguage('en')
    let finish!: (value: Detail) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render(); await settle()
    expect(io.detail).toHaveBeenCalledWith('child-a')
    expect(container.textContent).toContain('Loading')
    expect(container.textContent).not.toContain('正在加载子智能体')
    expect(io.timeline).toBeNull()
    await act(async () => finish({ ...snapshot(0), blocks: [], latestTurnId: undefined,
      latestUserMessageId: undefined, threadStatus: 'idle' }))
    await settle()
    expect(container.textContent).toContain('No work yet')
    expect(container.textContent).toContain('The child agent thread has been created, but there are no messages to display yet.')
    expect(container.textContent).not.toContain('暂无工作内容')
    expect(container.textContent).not.toContain('子智能体线程已经创建，但还没有可展示的消息。')
    await act(async () => { await i18n.changeLanguage('zh') })
    expect(container.textContent).toContain('暂无工作内容')
    expect(container.textContent).toContain('子智能体线程已经创建，但还没有可展示的消息。')
    expect(container.textContent).not.toContain('No work yet')
    expect(io.detail).toHaveBeenCalledTimes(1)
  })
})

describe('inspector public delivery and snapshot ownership', () => {
  it('commits an existing runtime general terminal batch through the actual mapper', async () => {
    await render(); await settle()
    const stream = io.streams.find(stream => stream.threadId === 'child-a')!
    await act(async () => { await dispatchAnalytixRuntimeEvents([generalBatch()], stream.sink, async () => {}) })
    expect(io.timeline!.blocks.find(block => block.id === 'terminal-child')).toMatchObject({
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'child-a', turnId: 'turn-child',
        itemId: 'terminal-child', firstSeq: 11, lastSeq: 13, status: 'completed' }
    })
    const committed = io.timeline!.blocks
    await act(async () => { await dispatchAnalytixRuntimeEvents([generalBatch()], stream.sink, async () => {}) })
    expect(io.timeline!.blocks).toBe(committed)
    await act(async () => {
      for (const start of [{ turnId: 'turn-child', seq: 14 }, { turnId: 'old-different-turn', seq: 13 },
        { turnId: 'unbound-cursor' }, { turnId: '', seq: 14 }]) stream.sink.onTurnStarted?.(start)
      stream.sink.onDeltas([{ kind: 'agent_message', text: 'LATE_UNPUBLISHED_TEXT', seq: 14 }])
    })
    expect(io.timeline!.blocks.filter(block => block.id === 'terminal-child')).toHaveLength(1)
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('LATE_UNPUBLISHED_TEXT')
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    await act(async () => {
      stream.sink.onTurnStarted?.({ turnId: 'next-child-turn', seq: 15 })
      stream.sink.onDeltas([{ kind: 'agent_message', text: 'NEW_AUTHORIZED_TURN', seq: 16, turnId: 'next-child-turn' }])
    })
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: true, currentTurnId: 'next-child-turn' })
    expect(useChatStore.getState().blocks).toEqual([])
  })
  it('keeps a newer public gate when an older detail request finishes', async () => {
    await render(); await settle()
    const stream = io.streams.find(stream => stream.threadId === 'child-a')!
    let finish!: (value: Detail) => void
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
    await act(async () => {
      for (const start of [{ turnId: batch.turnId, seq: 14 }, { turnId: 'old-different-turn', seq: batch.lastSeq },
        { turnId: 'unbound-cursor' }, { turnId: '', seq: 14 }]) sink.onTurnStarted?.(start)
      sink.onDeltas([{ kind: 'agent_message', text: 'LATE_UNPUBLISHED_TEXT', seq: 14 }])
    })
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('LATE_UNPUBLISHED_TEXT')
    await act(async () => { sink.onTurnStarted?.({ turnId: 'next-accepted-child', seq: 15 }) })
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: true, currentTurnId: 'next-accepted-child' })
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
    let finish!: (value: Detail) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render('child-a'); await render('child-b'); await settle(); await render('child-a'); await settle()
    await act(async () => finish({ ...snapshot(10), blocks: [{ ...user, text: 'OBSOLETE A SNAPSHOT' }] }))
    expect(container.textContent).not.toContain('OBSOLETE A SNAPSHOT')
    expect(io.detail.mock.calls.filter(call => call[0] === 'child-a')).toHaveLength(2)
  })
  it('never mounts previous child text beneath a newly selected child while its detail is pending', async () => {
    io.detail.mockResolvedValueOnce({ ...snapshot(), blocks: [{ ...user, text: 'ACTUAL CHILD A BODY' }] })
    await render(); await settle()
    expect(container.textContent).toContain('ACTUAL CHILD A BODY')
    let finish!: (value: Detail) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    io.timelineFrames = []
    await render('child-b')
    expect(io.timelineFrames.some(frame => frame.text.includes('ACTUAL CHILD A BODY'))).toBe(false)
    expect(container.textContent).not.toContain('ACTUAL CHILD A BODY')
    await act(async () => finish({ ...snapshot(), blocks: [{ ...user, id: 'child-b-user', text: 'ACTUAL CHILD B BODY' }] }))
    await settle()
    expect(container.textContent).toContain('ACTUAL CHILD B BODY')
    expect(io.timeline?.activeThreadId).toBe('child-b')
  })
  it('does not read a child whose summary has no open-thread capability', async () => {
    await render('child-a', true, [{ ...agent('child-a'), canOpenThread: false }]); await settle()
    expect(io.detail).not.toHaveBeenCalled(); expect(io.timeline).toBeNull()
  })
  it('restarts a running second turn after same-seq detail despite previous terminal usage', async () => {
    await render(); await settle()
    const stream = io.streams.find(stream => stream.threadId === 'child-a')!
    const batch = acceptedFinalProjectionBatch()
    await act(async () => { await stream.sink.onAcceptedFinalBatch!(batch) }); await settle()
    await act(async () => {
      stream.sink.onSeq(14)
      stream.sink.onUserMessage({ itemId: 'user-next', turnId: 'turn-next', text: 'Synthetic second request', createdAt: timestamp })
    })
    io.detail.mockResolvedValueOnce({ ...snapshot(14), blocks: io.timeline!.blocks, latestTurnId: 'turn-next',
      latestUserMessageId: 'user-next', threadStatus: 'running' })
    const subscriptions = io.streams.length
    await act(async () => stream.close()); await settle()
    expect(io.streams.length).toBe(subscriptions + 1)
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: true, currentTurnId: 'turn-next', currentTurnUserId: 'user-next' })
    expect(io.timeline!.blocks.some(block => block.id === batch.assistant.id)).toBe(true)
  })
  it('queues one facts read after a pre-terminal GET without reconciling committed delivery', async () => {
    vi.useFakeTimers()
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    let finish!: (value: Detail) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
      .mockResolvedValueOnce({ ...snapshot(13), blocks: [], threadStatus: 'completed',
        authoritativeTurnTimings: { 'turn-child': { turnId: 'turn-child', startedAt: 1000, finishedAt: 4200 } },
        authoritativeTurnStatuses: { 'turn-child': 'completed' } })
    await act(async () => vi.advanceTimersByTime(1000))
    expect(io.detail).toHaveBeenCalledTimes(2)
    const batch = acceptedFinalProjectionBatch()
    await act(async () => { expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt) })
    expect(io.detail).toHaveBeenCalledTimes(2)
    await act(async () => finish(snapshot(10))); await settle()
    expect(io.detail).toHaveBeenCalledTimes(3)
    expect(io.timeline!.runtimeStateOverride?.authoritativeTurnTimings?.['turn-child']).toMatchObject({ startedAt: 1000, finishedAt: 4200 })
    expect(io.timeline!.runtimeStateOverride?.lastTurnUsage).toEqual(batch.usage)
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ acceptedFinalProjectionReceipt: batch.receipt })
    await act(async () => { expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt) })
    expect(io.detail).toHaveBeenCalledTimes(3)
  })
  it('retains facts ownership across an error to ready subscription restart', async () => {
    vi.useFakeTimers()
    await render(); await settle()
    io.detail.mockRejectedValueOnce(new Error('Synthetic pre-terminal snapshot failure'))
    await act(async () => vi.advanceTimersByTime(1000)); await settle()
    const sink = io.streams.filter(stream => stream.threadId === 'child-a').at(-1)!.sink
    let finish!: (value: unknown) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const batch = acceptedFinalProjectionBatch()
    await act(async () => { expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt) }); await settle()
    await act(async () => finish({ ...snapshot(13), blocks: [], threadStatus: 'completed',
      authoritativeTurnTimings: { 'turn-child': { turnId: 'turn-child', startedAt: 1000, finishedAt: 4200 } },
      authoritativeTurnStatuses: { 'turn-child': 'completed' } }))
    expect(io.timeline!.runtimeStateOverride?.authoritativeTurnTimings?.['turn-child']).toMatchObject({ startedAt: 1000, finishedAt: 4200 })
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ acceptedFinalProjectionReceipt: batch.receipt })
  })
  it('preserves accepted delivery and replay when the optional terminal facts GET rejects', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    const batch = acceptedFinalProjectionBatch()
    io.detail.mockRejectedValueOnce(new Error('Synthetic facts read failure'))
    await act(async () => { expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt) }); await settle()
    expect(io.timeline!.runtimeStateOverride?.lastTurnUsage).toEqual(batch.usage)
    await act(async () => { expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt) })
    expect(io.timeline!.blocks.filter(block => block.id === batch.assistant.id)).toHaveLength(1)
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ acceptedFinalProjectionReceipt: batch.receipt })
    expect(io.timeline!.runtimeStateOverride?.authoritativeTurnStatuses?.['turn-child']).toBe('completed')
    expect(io.detail).toHaveBeenCalledTimes(2)
  })
  it('refreshes same-seq terminal facts without replacing committed receipt, blocks or usage', async () => {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    const batch = acceptedFinalProjectionBatch()
    io.detail.mockResolvedValueOnce({ ...snapshot(batch.lastSeq), blocks: [user], threadStatus: 'completed',
      authoritativeTurnTimings: { 'turn-child': { turnId: 'turn-child', startedAt: 1000, finishedAt: 4200 } },
      authoritativeTurnStatuses: { 'turn-child': 'completed' } })
    await act(async () => { expect(await sink.onAcceptedFinalBatch!(batch)).toEqual(batch.receipt) }); await settle()
    expect(io.timeline!.runtimeStateOverride?.authoritativeTurnTimings?.['turn-child']).toMatchObject({ startedAt: 1000, finishedAt: 4200 })
    expect(io.timeline!.runtimeStateOverride?.authoritativeTurnStatuses?.['turn-child']).toBe('completed')
    expect(io.timeline!.runtimeStateOverride?.lastTurnUsage).toEqual(batch.usage)
    expect(io.timeline!.blocks.filter(block => block.id === batch.assistant.id)).toHaveLength(1)
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ acceptedFinalProjectionReceipt: batch.receipt })
  })

})

describe('inspector user-input identity through the actual subscription', () => {
  function request(itemId: string, requestId: string, question = 'Synthetic question'): Parameters<ThreadEventSink['onUserInput']>[0] {
    return { itemId, requestId, turnId: 'turn-child', questions: [
      { id: `${requestId}_1`, header: 'Synthetic question', question, options: [], multiSelect: false }
    ] }
  }
  function inputs() {
    return io.timeline!.blocks.filter((block): block is Extract<ChatBlock, { kind: 'user_input' }> => block.kind === 'user_input')
  }
  async function pendingPair() {
    await render(); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
    await act(async () => {
      sink.onUserInput(request('input-item-a', 'request-a', 'Original question A'))
      sink.onUserInput(request('input-item-b', 'request-b', 'Original question B'))
    })
    expect(inputs()).toMatchObject([
      { id: 'input-item-a', requestId: 'request-a', status: 'pending' },
      { id: 'input-item-b', requestId: 'request-b', status: 'pending' }
    ])
    return sink
  }

  it('keeps the current inspector turn when a colliding question requests a foreign turn', async () => {
    const sink = await pendingPair()
    const blocksBefore = io.timeline!.blocks
    const runtimeBefore = io.timeline!.runtimeStateOverride
    expect(runtimeBefore).toMatchObject({ busy: true, currentTurnId: 'turn-child' })
    await act(async () => sink.onUserInput({
      ...request('input-item-a', 'foreign-request', 'REJECTED_FOREIGN_TURN_QUESTION'), turnId: 'foreign-turn'
    }))
    expect(io.timeline!.blocks).toEqual(blocksBefore)
    expect(io.timeline!.runtimeStateOverride?.currentTurnId).toBe(runtimeBefore?.currentTurnId)
    expect(io.timeline!.runtimeStateOverride).toEqual(runtimeBefore)
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it.each([
    { itemId: 'input-item-a', requestId: 'request-b' },
    { itemId: 'input-item-a', requestId: 'foreign-request' },
    { itemId: 'foreign-item', requestId: 'request-a' }
  ])('leaves both inspector requests pending for mismatched status identities %j', async identity => {
    const sink = await pendingPair()
    const before = io.timeline!.blocks
    await act(async () => sink.onUserInputStatus({ ...identity, status: 'cancelled', errorMessage: 'MISMATCHED_STATUS' }))
    expect(io.timeline!.blocks).toEqual(before)
    expect(inputs().map(block => block.status)).toEqual(['pending', 'pending'])
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('MISMATCHED_STATUS')
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it.each([
    { itemId: 'input-item-a', requestId: 'request-a' },
    { itemId: 'request-a' },
    { itemId: 'request-a', requestId: 'request-a' }
  ])('cancels each inspector request separately with a paired or legacy alias identity %j', async identity => {
    const sink = await pendingPair()
    await act(async () => sink.onUserInputStatus({ ...identity, status: 'cancelled' }))
    expect(inputs()).toMatchObject([
      { id: 'input-item-a', requestId: 'request-a', status: 'cancelled' },
      { id: 'input-item-b', requestId: 'request-b', status: 'pending' }
    ])
    await act(async () => sink.onUserInputStatus({ itemId: 'input-item-b', requestId: 'request-b', status: 'cancelled' }))
    expect(inputs().map(block => block.status)).toEqual(['cancelled', 'cancelled'])
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it.each([
    { itemId: 'input-item-a', requestId: 'foreign-request' },
    { itemId: 'foreign-item', requestId: 'request-a' },
    { itemId: 'input-item-a', requestId: 'request-b' },
    { itemId: 'user-child', requestId: 'foreign-request' }
  ])('rejects an inspector request identity collision and its late resolution %j', async identity => {
    const sink = await pendingPair()
    const before = io.timeline!.blocks
    await act(async () => sink.onUserInput(request(identity.itemId, identity.requestId, 'COLLIDING_QUESTION')))
    expect(io.timeline!.blocks).toEqual(before)
    await act(async () => sink.onUserInputStatus({ ...identity, status: 'submitted' }))
    expect(io.timeline!.blocks).toEqual(before)
    expect(inputs().map(block => block.status)).toEqual(['pending', 'pending'])
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('COLLIDING_QUESTION')
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it.each(['submitted', 'cancelled'] as const)('keeps a %s inspector request terminal when its pending request is replayed', async status => {
    const sink = await pendingPair()
    await act(async () => sink.onUserInputStatus({ itemId: 'input-item-a', requestId: 'request-a', status }))
    const before = io.timeline!.blocks
    await act(async () => sink.onUserInput(request('input-item-a', 'request-a', 'LATE_PENDING_QUESTION')))
    expect(io.timeline!.blocks).toEqual(before)
    expect(inputs()).toMatchObject([
      { id: 'input-item-a', requestId: 'request-a', status },
      { id: 'input-item-b', requestId: 'request-b', status: 'pending' }
    ])
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('LATE_PENDING_QUESTION')
  })

  it.each([
    { original: 'submitted', late: 'cancelled' },
    { original: 'submitted', late: 'error' },
    { original: 'cancelled', late: 'submitted' },
    { original: 'cancelled', late: 'error' }
  ] as const)('ignores a late $late status for an inspector request already $original', async ({ original, late }) => {
    const sink = await pendingPair()
    await act(async () => sink.onUserInputStatus({ itemId: 'input-item-a', requestId: 'request-a', status: original }))
    const before = io.timeline!.blocks
    await act(async () => sink.onUserInputStatus({ itemId: 'input-item-a', requestId: 'request-a', status: late,
      answers: [{ id: 'late-answer', label: 'Late answer', value: 'LATE_WRONG_ANSWER' }], errorMessage: 'LATE_WRONG_STATUS' }))
    expect(io.timeline!.blocks).toEqual(before)
    expect(inputs()).toMatchObject([
      { id: 'input-item-a', requestId: 'request-a', status: original },
      { id: 'input-item-b', requestId: 'request-b', status: 'pending' }
    ])
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('LATE_WRONG_')
  })

  it('refreshes a matching pending inspector request without replacing another request', async () => {
    const sink = await pendingPair()
    await act(async () => sink.onUserInput(request('input-item-a', 'request-a', 'Updated question A')))
    expect(inputs()).toHaveLength(2)
    expect(inputs()).toMatchObject([
      { id: 'input-item-a', requestId: 'request-a', status: 'pending', questions: [{ question: 'Updated question A' }] },
      { id: 'input-item-b', requestId: 'request-b', status: 'pending', questions: [{ question: 'Original question B' }] }
    ])
  })

  it('ignores a late user-input resolution from the closed inspector selection', async () => {
    const oldSink = await pendingPair()
    await render('child-b'); await settle()
    const sink = io.streams.find(stream => stream.threadId === 'child-b')!.sink
    await act(async () => sink.onUserInput(request('input-item-a', 'request-a', 'Current child B question')))
    const before = io.timeline!.blocks
    await act(async () => oldSink.onUserInputStatus({ itemId: 'input-item-a', requestId: 'request-a', status: 'cancelled' }))
    expect(io.timeline!.blocks).toEqual(before)
    expect(inputs()).toMatchObject([{ id: 'input-item-a', requestId: 'request-a', status: 'pending' }])
    await act(async () => sink.onUserInputStatus({ itemId: 'input-item-a', requestId: 'request-a', status: 'cancelled' }))
    expect(inputs()).toMatchObject([{ id: 'input-item-a', requestId: 'request-a', status: 'cancelled' }])
    expect(useChatStore.getState().blocks).toEqual([])
  })
})

describe('inspector draft replacement through the real timeline and composer', () => {
  it.each(['same', 'next_turn', 'item', 'text', 'turn', 'cursor'] as const)(
    'retains only the committed terminal association across a %s detail GET', async variant => {
      vi.useFakeTimers()
      vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
      vi.spyOn(rendererRuntimeClient, 'getSettings').mockRejectedValue(new Error('Synthetic settings unavailable'))
      vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
      vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
      vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
      // jsdom has no Range layout API; supply synthetic selection geometry
      // while retaining the actual ProseMirror focus/selection handlers.
      const rangeDescriptors = new Map(['getClientRects', 'getBoundingClientRect'].map(key =>
        [key, Object.getOwnPropertyDescriptor(Range.prototype, key)] as const))
      Object.defineProperties(Range.prototype, {
        getClientRects: { configurable: true, value: () => [new DOMRect(100, 100, 1, 16)] },
        getBoundingClientRect: { configurable: true, value: () => new DOMRect(100, 100, 1, 16) }
      })
      const observers: Array<{ callback: ResizeObserverCallback; targets: Set<Element> }> = []
      vi.stubGlobal('ResizeObserver', class {
        readonly targets = new Set<Element>()
        constructor(readonly callback: ResizeObserverCallback) { observers.push(this) }
        observe(target: Element) { this.targets.add(target) }
        unobserve(target: Element) { this.targets.delete(target) }
        disconnect() { this.targets.clear() }
      })
      const descriptor = Object.getOwnPropertyDescriptor(window, 'analytix')
      const scopes = new Map<string, { threadId: string; turnId: string; contextDigest: string; contextEpoch: string; processIncarnation: string }>()
      const expanded = new Map<string, Set<string>>()
      let revision = 2, privateText = 'INSPECTOR_PRIVATE_REV_1', terminal = false, producing = false
      const authorize = vi.fn(async (request: ProcessPaneAuthorizeRequestV1) => {
        const viewerId = crypto.randomUUID()
        const scope = { threadId: request.threadId, turnId: request.turnId, contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'child-incarnation' }
        scopes.set(viewerId, scope)
        return { status: 'authorized', schemaVersion: 2, viewerId, revision: '1', scope,
          presence: { nativeBlockCount: 0, toolCaptureCount: 0, assistantDraftCount: producing && request.turnId === 'turn-child' && !terminal ? 1 : 0 } }
      })
      Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
        authorizeProcessPane: authorize,
        readProcessPane: vi.fn(async (request: ProcessPaneReadRequestV1) => {
          const scope = scopes.get(request.viewerId)!
          const hasDraft = producing && scope.turnId === 'turn-child' && !terminal
          return { status: 'snapshot', schemaVersion: 2, revision: String(revision), scope,
            units: hasDraft ? [{ unitId: 'child-draft-unit', producerId: 'child-draft-producer', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }] : [],
            blocks: hasDraft ? [{ blockId: 'child-draft-1', kind: 'assistant_draft', state: 'running', attempt: 1,
              source: { kind: 'assistant_draft', unitId: 'child-draft-unit', producerId: 'child-draft-producer', providerSequence: '1', attempt: 1 },
              contentClassification: 'unclassified_inline', classification: 'source_exact_local', durationMs: 0, truncated: false,
              ...(expanded.get(request.viewerId)?.has('child-draft-1') ? {
                draftSegments: [{ offsetBytes: '0', text: privateText }], draftBytes: String(new TextEncoder().encode(privateText).length)
              } : {}) }] : [] }
        }),
        setProcessPaneExpanded: vi.fn(async (request: { viewerId: string; expandedBlockIds: string[] }) => {
          expanded.set(request.viewerId, new Set(request.expandedBlockIds)); return { status: 'ok' }
        }),
        closeProcessPane: vi.fn(async () => ({ status: 'ok' })),
        openProcessCapture: async () => ({ status: 'unavailable' }),
        onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
      } } })
      const frames = async () => { await act(async () => { await vi.advanceTimersByTimeAsync(0) }); await settle() }
      const measureRows = async (height: number) => {
        await act(async () => {
          for (const observer of observers) {
            const entries = [...observer.targets].filter(target => target.hasAttribute('data-analytix-timeline-row'))
              .map(target => ({ target, contentRect: new DOMRect(0, 0, 600, height) }) as ResizeObserverEntry)
            if (entries.length) observer.callback(entries, observer as unknown as ResizeObserver)
          }
        })
        await frames()
      }
      try {
        io.realTimeline = true; io.realComposer = true
        io.detail.mockResolvedValue({ ...snapshot(8), blocks: [], latestTurnId: null,
          latestUserMessageId: null, threadStatus: 'idle' })
        await render(); await settle(); await frames()
        // Leave a draft in the real idle composer before the observed turn starts.
        await act(async () => io.composer!.setInput('UNSENT_PRODUCT_COMPOSER'))
        const editor = container.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror[contenteditable="true"]')!
        expect(editor).not.toBeNull()
        expect(editor.textContent).toBe('UNSENT_PRODUCT_COMPOSER')
        await act(async () => editor.focus())
        expect(document.activeElement).toBe(editor)
        const sink = io.streams.find(stream => stream.threadId === 'child-a')!.sink
        producing = true
        io.detail.mockResolvedValue(snapshot())
        await act(async () => {
          sink.onTurnStarted?.({ turnId: 'turn-child', seq: 9 })
          sink.onSeq(10)
          sink.onUserMessage({ itemId: user.id, turnId: 'turn-child', text: user.text, createdAt: timestamp })
        })
        await frames()
        expect(authorize).toHaveBeenCalled()
        const seat = container.querySelector<HTMLElement>('[data-analytix-assistant-response-seat]')!
        expect(seat.textContent).toContain('INSPECTOR_PRIVATE_REV_1')
        const draftSeat = seat.querySelector('[data-chat-seat="child-draft-unit:assistant_draft"]')
        const scroller = container.querySelector<HTMLElement>('[data-analytix-message-timeline-scroller]')!
        let scrollHeight = 1600
        Object.defineProperties(scroller, {
          clientHeight: { configurable: true, get: () => 400 },
          scrollHeight: { configurable: true, get: () => scrollHeight }
        })
        const readEarlier = async () => {
          await act(async () => {
            scroller.scrollTop = scrollHeight - 400
            scroller.dispatchEvent(new Event('scroll'))
            scroller.scrollTop = 137
            scroller.dispatchEvent(new Event('scroll'))
          })
          await frames()
          expect(container.querySelector('[data-analytix-assistant-response-seat]'), 'existing response seat after the next turn starts').toBe(seat)
        }
        await readEarlier()
        const assertReaderAndInput = (editable: boolean) => {
          expect(container.querySelector('[data-analytix-message-timeline-scroller]')).toBe(scroller)
          expect(scroller.scrollTop).toBe(137)
          expect(container.querySelector('[data-composer-prompt-editor] .ProseMirror')).toBe(editor)
          expect(editor.textContent).toBe('UNSENT_PRODUCT_COMPOSER')
          expect(io.composer!.input).toBe('UNSENT_PRODUCT_COMPOSER')
          expect(editor.getAttribute('contenteditable')).toBe(String(editable))
          expect(io.composer!.busy).toBe(!editable)
          if (editable) expect(document.activeElement).toBe(editor)
        }
        assertReaderAndInput(false)
        privateText = 'INSPECTOR_PRIVATE_REV_2'; revision++
        await act(async () => { await vi.advanceTimersByTimeAsync(500) })
        scrollHeight = 1800; await measureRows(900)
        expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
        expect(seat.querySelector('[data-chat-seat="child-draft-unit:assistant_draft"]')).toBe(draftSeat)
        expect(seat.textContent).toContain('INSPECTOR_PRIVATE_REV_2')
        expect(seat.textContent).not.toContain('INSPECTOR_PRIVATE_REV_1')
        assertReaderAndInput(false)
        terminal = true; revision++
        await act(async () => { await dispatchAnalytixRuntimeEvents([generalBatch()], sink, async () => {}) })
        await settle(); await measureRows(1200)
        const committed = io.timeline!.blocks.find((block): block is Extract<ChatBlock, { kind: 'assistant' }> => block.kind === 'assistant' && block.id === 'terminal-child')!
        const receipt = committed.generalTerminalProjectionReceipt!
        expect(receipt).toBeDefined()
        expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
        expect(seat.textContent).toContain(committed.text)
        expect(container.querySelector('[data-protected-assistant-draft]')).toBeNull()
        expect(io.composer!.busy).toBe(false)
        expect(io.timeline!.runtimeStateOverride?.currentTurnId).toBeNull()
        expect(editor.getAttribute('contenteditable')).toBe('true')
        await act(async () => editor.focus())
        assertReaderAndInput(true)
        expect(JSON.stringify(io.timeline!.blocks)).not.toContain('INSPECTOR_PRIVATE_')
        expect(useChatStore.getState().blocks).toEqual([])
        expect(useChatStore.getState().liveAssistant).not.toContain('INSPECTOR_PRIVATE_')

        const { generalTerminalProjectionReceipt: _receipt, ...historyItem } = committed
        let incoming: Extract<ChatBlock, { kind: 'assistant' }> = historyItem
        if (variant === 'item' || variant === 'cursor') incoming = { ...historyItem, id: 'wrong-terminal-item',
          generalTerminalProjectionReceipt: { ...receipt, itemId: 'wrong-terminal-item' } }
        if (variant === 'text') incoming = { ...historyItem, text: 'WRONG_TERMINAL_TEXT', generalTerminalProjectionReceipt: receipt }
        if (variant === 'turn') incoming = { ...historyItem, meta: { ...historyItem.meta, turnId: 'wrong-terminal-turn' }, generalTerminalProjectionReceipt: receipt }
        const nextUser = { ...user, id: 'user-next', text: 'Synthetic second request', meta: { turnId: 'turn-next' } }
        if (variant === 'next_turn') {
          await act(async () => {
            sink.onTurnStarted?.({ turnId: 'turn-next', seq: 14 })
            sink.onSeq(15)
            sink.onUserMessage({ itemId: nextUser.id, turnId: 'turn-next', text: nextUser.text, createdAt: timestamp })
          })
          await frames()
          // A new user turn intentionally becomes visible. Establish the
          // reader's position again before testing that turn's newer GET.
          await readEarlier()
          assertReaderAndInput(false)
        }
        const requestSeq = variant === 'next_turn' ? 16 : 14
        incoming = { ...incoming, meta: { ...incoming.meta, factHistoryState: 'retained_snapshot' } }
        io.detail.mockResolvedValueOnce({ ...snapshot(variant === 'cursor' ? 12 : requestSeq),
          blocks: [user, incoming, ...(variant === 'next_turn' ? [nextUser] : [])],
          latestTurnId: variant === 'next_turn' ? 'turn-next' : 'turn-child',
          latestUserMessageId: variant === 'next_turn' ? nextUser.id : user.id,
          threadStatus: variant === 'next_turn' ? 'running' : 'completed',
          authoritativeTurnStatuses: { 'turn-child': 'completed' } })
        const reads = io.detail.mock.calls.length
        // The fresh GET must advance the already-covered cursor. Advertising
        // its future cursor here would hit the same-cursor preservation path
        // and bypass the newer-GET replacement defect under test.
        const coveredSeq = variant === 'cursor' ? requestSeq : variant === 'next_turn' ? 15 : 13
        await act(async () => { await sink.onSnapshotRequired?.({ threadId: 'child-a', seq: coveredSeq, highestSeq: coveredSeq, reason: 'synthetic detail refresh' }) })
        await settle(); await measureRows(1300)
        expect(io.detail.mock.calls.length).toBe(reads + 1)
        const refreshed = io.timeline!.blocks.find(block => block.id === incoming.id)
        if (variant === 'item' || variant === 'text' || variant === 'turn') {
          expect(refreshed).toMatchObject({ kind: 'assistant', meta: { factHistoryState: 'retained_snapshot' } })
          expect(refreshed?.kind === 'assistant' ? refreshed.generalTerminalProjectionReceipt : null).toBeUndefined()
          expect(seat.textContent).not.toContain(incoming.text)
        } else {
          expect(io.timeline!.blocks.find(block => block.id === committed.id)).toMatchObject({ generalTerminalProjectionReceipt: receipt })
          if (variant !== 'cursor') expect(refreshed).toMatchObject({ meta: { factHistoryState: 'retained_snapshot' } })
          expect(container.querySelector('[data-analytix-assistant-response-seat]')).toBe(seat)
          expect(seat.textContent).toContain(committed.text)
        }
        if (variant === 'cursor') expect(refreshed).toBeUndefined()
        expect(container.textContent).not.toContain('INSPECTOR_PRIVATE_')
        assertReaderAndInput(variant !== 'next_turn')
      } finally {
        await act(async () => root.render(null))
        if (descriptor) Object.defineProperty(window, 'analytix', descriptor); else Reflect.deleteProperty(window, 'analytix')
        for (const [key, previous] of rangeDescriptors) {
          if (previous) Object.defineProperty(Range.prototype, key, previous); else Reflect.deleteProperty(Range.prototype, key)
        }
      }
    }
  )
})


it('automatically leases inspector identity and clears its actual protected leaf on hiding or selection change', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(window, 'analytix')
  const scope = { threadId: 'child-a', turnId: 'turn-child', contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'child-incarnation' }
  const authorize = vi.fn(async () => ({ status: 'authorized', schemaVersion: 2, viewerId: '11111111-1111-4111-8111-111111111111', revision: '1', scope, presence: { nativeBlockCount: 1, toolCaptureCount: 0 } }))
  const close = vi.fn(async () => ({ status: 'ok' }))
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    authorizeProcessPane: authorize, closeProcessPane: close,
    readProcessPane: vi.fn(async (request: { draftsOnly?: boolean }) => ({ status: 'snapshot', schemaVersion: 2, revision: '1', scope,
      units: request.draftsOnly ? [] : [{ unitId: 'child-unit', producerId: 'child-producer', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }],
      blocks: request.draftsOnly ? [] : [{ blockId: 'block', kind: 'native_reasoning', source: { kind: 'native', unitId: 'child-unit', producerId: 'child-producer', providerSequence: '1', attempt: 1, nativeIndex: 0, sourceOrdinal: '1' },
        state: 'completed', attempt: 1, nativeIndex: 0, preview: 'PRIVATE_INSPECTOR_PREVIEW', durationMs: 1, truncated: false, classification: 'credential_redacted' }] })),
    setProcessPaneExpanded: async () => ({ status: 'ok' }), openProcessCapture: async () => ({ status: 'unavailable' }),
    onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
  } } })
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  try {
    io.realTimeline = true; await render(); await settle()
    expect(io.timeline?.processPaneRoute).toMatchObject({ surface: 'inspector', visible: true })
    expect(authorize).toHaveBeenCalledTimes(1)
    const process = container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')
    if (process) await act(async () => process.click())
    const group = container.querySelector<HTMLElement>('[data-process-group] > div:first-child > button[data-disclosure-row]')
    if (group) await act(async () => group.click())
    expect(container.textContent).toContain('PRIVATE_INSPECTOR_PREVIEW')
    await render('child-a', false); expect(container.textContent).not.toContain('PRIVATE_INSPECTOR_PREVIEW'); expect(close).toHaveBeenCalled()
    await render(); await settle(); expect(authorize).toHaveBeenCalledTimes(2)
    await render('child-b'); await settle()
    expect(container.textContent).not.toContain('PRIVATE_INSPECTOR_PREVIEW')
    expect(io.timeline?.processPaneRoute?.selectionKey).toContain('child-b')
    expect(authorize.mock.calls.length).toBeGreaterThanOrEqual(2)
  } finally {
    await act(async () => root.render(null))
    if (descriptor) Object.defineProperty(window, 'analytix', descriptor); else Reflect.deleteProperty(window, 'analytix')
  }
})


function liveInspectorStream(threadId = 'child-a') {
  const stream = io.streams.slice().reverse().find(item => item.threadId === threadId && !item.signal.aborted)
  if (!stream) throw new Error(`Missing live inspector stream: ${threadId}`)
  return stream
}
function footerGeneralBatch() {
  const batch = generalBatch(), text = 'Historical ordinary child answer'
  return { ...batch, events: batch.events.map(event => event.kind === 'item_completed'
    ? { ...event, item: { ...event.item, text, ordinaryResult: { schemaVersion: 1, purpose: 'analytix.ordinary-result/v1',
      projectionVersion: 'analytix.ordinary-output-projection/v1', logicalEffect: 'ordinary', ordinaryWork: true,
      candidateOrigin: 'provider_ordinary_only', evidenceAuthority: false, citationAuthority: false,
      factAnswerAllowed: false, text, textSha256: '6'.repeat(64), resultDigest: '7'.repeat(64) } } }
    : event.kind === 'usage' ? { ...event, cacheDiagnostics: { providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1', providerAttemptTelemetryValid: true,
      providerLogicalCallCount: 1, providerAttemptCount: 2,
      providerAttemptStatuses: { succeeded: 2, failed: 0, cancelled: 0, timedOut: 0, streamAborted: 0 },
      providerAttemptInputTokens: { complete: true, knownObservationCount: 2, observationCount: 2, value: 12 },
      providerAttemptOutputTokens: { complete: true, knownObservationCount: 2, observationCount: 2, value: 4 },
      providerAttemptCacheHitTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
      providerAttemptCacheMissTokens: { complete: false, knownObservationCount: 0, observationCount: 2 },
      providerAttemptCacheRate: { known: false, numerator: 0, denominator: 0 } } } : event) }
}
function normalInspectorFooterUsage() {
  const count = (reportedValue: number, knownAttemptCount = 2, complete = true) => ({ reportedValue, knownAttemptCount, attemptCount: 2, complete })
  const unknown = count(0, 0, false)
  const billing = { inventoryComplete: true, closed: true, attemptCount: 2, settledAttemptCount: 2,
    inputTokens: count(12), outputTokens: count(4), totalTokens: count(16),
    uncachedInputTokens: count(12), cacheReadTokens: unknown, cacheWriteTokens: unknown, reasoningTokens: unknown }
  const conversationUsage = ConversationUsageV1Schema.parse({ schemaVersion: 'conversation-usage.v1', billing, turns: [{ turnId: 'turn-child', billing }] })
  const retained = { input_tokens: 999, output_tokens: 99, reasoning_tokens: 0, cached_tokens: 0, cache_miss_tokens: 999,
    total_tokens: 1098, cost_usd: 0, cost_cny: 0, price_configured: false, cost_estimate_status: 'unknown', cost_known_currencies: [],
    cache_savings_usd: 0, cache_savings_cny: 0, token_economy_savings_tokens: 0, token_economy_savings_usd: 0,
    token_economy_savings_cny: 0, turns: 1, cache_hit_rate: null }
  return ThreadUsageResponseSchema.parse({ group_by: 'thread', buckets: [{ ...retained, thread_id: 'child-a', conversationUsage }],
    totals: { ...retained, thread_count: 1 } })
}
function mappedFooterBatch() {
  const batch = generalTerminalProjectionBatchFromRuntime(footerGeneralBatch())
  if (!batch) throw new Error('Invalid synthetic Footer batch fixture')
  return batch
}

describe('Inspector historical Footer usage owners', () => {
  it('restores the actual historical Footer through the real bounded public replay path', async () => {
    const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
    const body = normalInspectorFooterUsage()
    const runtimeRequest = vi.fn<Window['analytix']['runtime']['runtimeRequest']>(async (path, method) => {
      expect(path).toBe('/v1/usage?group_by=thread&thread_id=child-a'); expect(method).toBe('GET')
      return { ok: true, status: 200, body: JSON.stringify(body) }
    })
    rendererRuntimeClient.invalidateSettings()
    let onData: Parameters<Window['analytix']['runtime']['onSseEvent']>[0] | undefined
    let onEnd: Parameters<Window['analytix']['runtime']['onSseEnd']>[0] | undefined
    const startSse: Window['analytix']['runtime']['startSse'] = vi.fn(async (threadId, _since, streamId, replay) => {
      expect(threadId).toBe('child-a'); expect(replay).toEqual({ replayThroughSeq: 16 })
      queueMicrotask(() => {
        onData?.({ streamId: streamId!, events: [{ kind: 'item_created', seq: 1, threadId: 'child-a', turnId: 'turn-child', timestamp,
          itemId: user.id, item: { id: user.id, threadId: 'child-a', turnId: 'turn-child', role: 'user', kind: 'user_message',
            status: 'completed', createdAt: timestamp, text: user.text } }] })
        onData?.({ streamId: streamId!, events: [footerGeneralBatch() as PublicSsePayload['events'][number]] })
        onData?.({ streamId: streamId!, events: [{ kind: 'cursor_advanced', seq: 16, threadId: 'child-a', timestamp, reason: 'restricted_content_removed' }] })
        onEnd?.({ streamId: streamId! })
      })
      return { streamId: streamId! }
    })
    Object.defineProperty(window, 'analytix', { configurable: true, value: { settings: {
      getSettings: async () => normalizeAppSettings({ performanceUsageMode: 'detailed' } as AppSettingsV1)
    }, runtime: { runtimeRequest,
      startSse, stopSse: vi.fn(async () => undefined), ackSseEvent: vi.fn(async () => true),
      onSseEvent: (handler: NonNullable<typeof onData>) => { onData = handler; return () => { onData = undefined } },
      onSseEnd: (handler: NonNullable<typeof onEnd>) => { onEnd = handler; return () => { onEnd = undefined } },
      onSseError: () => () => undefined,
      authorizeProcessPane: vi.fn(async () => ({ status: 'unavailable' })),
      closeProcessPane: vi.fn(async () => ({ status: 'ok' })),
      onProcessPaneInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
    } } })
    try {
      const provider = new AnalytixRuntimeProvider()
      vi.spyOn(provider, 'getThreadDetail').mockImplementation(io.detail)
      vi.spyOn(provider, 'subscribeThreadEvents').mockImplementation((threadId, _seq, sink, signal) =>
        new Promise<void>(resolve => { io.streams.push({ threadId, sink, signal, close: resolve }) }))
      io.runtimeProvider = provider; io.realTimeline = true
      io.detail.mockResolvedValue({ ...snapshot(16), blocks: [{ kind: 'compaction', id: 'compact-primary', summary: 'Context compacted',
        status: 'success', meta: { turnId: 'compact-turn' } }], threadStatus: 'idle', latestTurnId: 'compact-turn' })
      await render(); await settle()
      expect(startSse).toHaveBeenCalledOnce()
      expect(io.timeline!.runtimeStateOverride?.turnUsageById?.['turn-child']).toEqual(turnUsageEntryFromBatch(mappedFooterBatch()))
      const footer = container.querySelector('[data-clock="end"]')!
      expect(footer).not.toBeNull()
      expect(footer.querySelector(`button[aria-label="${i18n.t('common:copyMessage')}"]`)).not.toBeNull()
      const usage = footer.querySelector<HTMLButtonElement>('button[aria-haspopup="dialog"]')!
      expect(usage.textContent).toContain('16'); expect(usage.textContent).not.toContain('1098')
      expect(runtimeRequest).toHaveBeenCalled()
      await act(async () => usage.click())
      expect(document.querySelector('[data-turn-usage-details]')).not.toBeNull()
      expect(useChatStore.getState().blocks).toEqual([])
    } finally {
      await act(async () => root.render(null))
      rendererRuntimeClient.invalidateSettings()
      if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge); else Reflect.deleteProperty(window, 'analytix')
    }
  })
  it('retries an unavailable whole-log GET after real same-cursor Inspector snapshot recovery', async () => {
    const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix'), body = normalInspectorFooterUsage()
    let ready = false
    const request = vi.fn<Window['analytix']['runtime']['runtimeRequest']>(async (path, method) => {
      expect(path).toBe('/v1/usage?group_by=thread&thread_id=child-a'); expect(method).toBe('GET')
      return ready ? { ok: true, status: 200, body: JSON.stringify(body) } : { ok: false, status: 503, body: '' }
    })
    rendererRuntimeClient.invalidateSettings()
    Object.defineProperty(window, 'analytix', { configurable: true, value: { settings: {
      getSettings: async () => normalizeAppSettings({ performanceUsageMode: 'detailed' } as AppSettingsV1)
    }, runtime: { runtimeRequest: request } } })
    try {
      io.detail.mockResolvedValue({ ...snapshot(0), latestTurnId: null, latestUserMessageId: null, threadStatus: 'idle' })
      await render(); await settle()
      expect(request).toHaveBeenCalled()
      expect(container.querySelector('[data-composer-stat="usage"]')).toBeNull()
      const before = request.mock.calls.length, stream = liveInspectorStream()
      ready = true
      await act(async () => stream.sink.onSnapshotRequired?.({ threadId: 'child-a', seq: 0, highestSeq: 0 }))
      await settle()
      expect(request.mock.calls.length).toBeGreaterThan(before)
      const pill = container.querySelector('[data-composer-stat="usage"]')!
      expect(pill.textContent).toContain('16'); expect(pill.textContent).not.toContain('1098')
      // A usage-only snapshot does not manufacture an ended response Footer.
      expect(container.querySelector('[data-clock="end"]')).toBeNull()
    } finally {
      await act(async () => root.render(null)); rendererRuntimeClient.invalidateSettings()
      if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge); else Reflect.deleteProperty(window, 'analytix')
    }
  })
  it('adds missing usage on a same-cursor refresh while preserving the committed body and receipt', async () => {
    await render(); await settle()
    const stream = liveInspectorStream()
    await act(async () => dispatchAnalytixRuntimeEvents([footerGeneralBatch()], stream.sink, async () => {})); await settle()
    const blocks = io.timeline!.blocks
    const historical = { ...turnUsageEntryFromBatch(mappedFooterBatch()), turnId: 'earlier', firstSeq: 1, lastSeq: 3 }
    io.detail.mockResolvedValue({ ...snapshot(13), blocks: [user], threadStatus: 'completed',
      turnUsageById: { earlier: historical, 'turn-child': turnUsageEntryFromBatch(mappedFooterBatch()) }, turnUsageThroughSeq: 13 })
    await act(async () => stream.sink.onSnapshotRequired?.({ threadId: 'child-a', seq: 13, highestSeq: 13, reason: 'synthetic usage recovery' }))
    await settle()
    expect(io.timeline!.blocks).toBe(blocks)
    expect(io.timeline!.runtimeStateOverride?.turnUsageById?.earlier).toEqual(historical)
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ generalTerminalProjectionReceipt: { batchDigest: 'a'.repeat(64) } })
  })
  it('cuts the owner synchronously, waits out an old GET, and cannot revive a rewound turn in a new closure', async () => {
    const retained = { kind: 'user' as const, id: 'retained-user', text: 'Earlier request', meta: { turnId: 'constructor' } }
    const historical = { ...turnUsageEntryFromBatch(mappedFooterBatch()), turnId: 'constructor', firstSeq: 1, lastSeq: 3 }
    io.detail.mockResolvedValue({ ...snapshot(), blocks: [retained, user], turnUsageById: { constructor: historical },
      authoritativeTurnTimings: { constructor: { turnId: 'constructor', startedAt: 1, finishedAt: 2 } }, authoritativeTurnStatuses: { constructor: 'completed' } })
    await render(); await settle()
    const stream = liveInspectorStream()
    await act(async () => dispatchAnalytixRuntimeEvents([footerGeneralBatch()], stream.sink, async () => {})); await settle()
    let finish!: (detail: Detail) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => { void stream.sink.onSnapshotRequired?.({ threadId: 'child-a', seq: 13, highestSeq: 13, reason: 'synthetic pending detail' }) })
    const reads = io.detail.mock.calls.length
    const fresh: Detail = { ...snapshot(14), blocks: [retained], threadStatus: 'idle', latestTurnId: 'constructor', latestUserMessageId: retained.id,
      turnUsageById: { constructor: historical }, turnUsageThroughSeq: 14,
      authoritativeTurnTimings: { constructor: { turnId: 'constructor', startedAt: 1, finishedAt: 2 } }, authoritativeTurnStatuses: { constructor: 'completed' as const } }
    io.detail.mockResolvedValue(fresh)
    await act(async () => stream.sink.onThreadRewound?.({ threadId: 'child-a', turnId: 'turn-child', removedTurnIds: ['turn-child'],
      removedTurns: 1, remainingTurns: 1, seq: 14 }))
    expect(io.timeline!.blocks).toEqual([retained]); expect(io.timeline!.runtimeStateOverride?.turnUsageById).toEqual({ constructor: historical })
    expect(io.timeline!.runtimeStateOverride?.authoritativeTurnStatuses?.constructor).toBe('completed')
    await act(async () => stream.sink.onUserInputStatus({ itemId: 'old-input-item', requestId: 'old-request', status: 'submitted' }))
    expect(io.timeline!.blocks).toEqual([retained])
    await act(async () => finish({ ...snapshot(13), blocks: [retained, user], turnUsageById: { 'turn-child': turnUsageEntryFromBatch(mappedFooterBatch()) } }))
    await settle(); expect(io.detail.mock.calls.length).toBe(reads + 1)
    expect(io.timeline!.blocks).toEqual([retained]); expect(io.timeline!.runtimeStateOverride?.turnUsageById?.['turn-child']).toBeUndefined()
    await act(async () => stream.sink.onError(new Error('Synthetic reconnect boundary')))
    await settle()
    const latest = liveInspectorStream()
    expect(latest.signal).not.toBe(stream.signal); expect(stream.signal.aborted).toBe(true)
    await act(async () => latest.sink.onTurnStarted?.({ turnId: 'old-unremoved-turn', seq: 13 }))
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: false, currentTurnId: null })
    await act(async () => { latest.sink.onTurnStarted?.({ turnId: 'turn-child', seq: 15 });
      latest.sink.onDeltas([{ kind: 'agent_message', text: 'REMOVED_TEXT', seq: 16, turnId: 'turn-child' }]);
      latest.sink.onTurnStarted?.({ turnId: 'new-child-turn', seq: 17 }) })
    expect(io.timeline!.runtimeStateOverride).toMatchObject({ busy: true, currentTurnId: 'new-child-turn' })
    expect(JSON.stringify(io.timeline!.blocks)).not.toContain('REMOVED_TEXT')
  })
  it('revokes the actual usage owner before a late detail can restore it', async () => {
    io.detail.mockResolvedValue({ ...snapshot(), turnUsageById: { 'turn-child': turnUsageEntryFromBatch(mappedFooterBatch()) } })
    await render(); await settle(); const stream = liveInspectorStream()
    expect(io.timeline!.runtimeStateOverride?.turnUsageById).toEqual({ 'turn-child': turnUsageEntryFromBatch(mappedFooterBatch()) })
    let finish!: (detail: Detail) => void
    io.detail.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => { void stream.sink.onSnapshotRequired?.({ threadId: 'child-a', seq: 11, highestSeq: 11, reason: 'synthetic pending detail' }) })
    await act(async () => stream.sink.onPublicProjectionRevoked?.({ schemaVersion: 1, kind: 'public_projection_revoked', threadId: 'child-a', historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected', action: 'purge_case_projection', terminal: true }))
    expect(io.timeline).toBeNull()
    expect(container.textContent).toContain(i18n.t('common:runtimeFinalSnapshotUnavailable'))
    expect(container.textContent).not.toContain(user.text)
    expect(container.querySelector('[data-clock="end"],[data-turn-usage-details]')).toBeNull()
    await act(async () => finish({ ...snapshot(12), turnUsageById: { 'turn-child': turnUsageEntryFromBatch(mappedFooterBatch()) } }))
    await settle(); expect(io.timeline).toBeNull()
    expect(container.textContent).not.toContain(user.text)
    expect(container.querySelector('[data-clock="end"],[data-turn-usage-details]')).toBeNull()
  })
})


it('keeps a live terminal commit made while the same-cursor real replay is pending', async () => {
  const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
  let onData: Parameters<Window['analytix']['runtime']['onSseEvent']>[0] | undefined
  let onEnd: Parameters<Window['analytix']['runtime']['onSseEnd']>[0] | undefined
  let pendingStreamId: string | undefined
  const startSse: Window['analytix']['runtime']['startSse'] = vi.fn(async (_threadId, _since, streamId, options) => {
    if (options?.replayThroughSeq === 10) queueMicrotask(() => {
      onData?.({ streamId: streamId!, events: [{ kind: 'cursor_advanced', seq: 10, threadId: 'child-a', timestamp, reason: 'restricted_content_removed' }] })
      onEnd?.({ streamId: streamId! })
    })
    else { expect(options).toEqual({ replayThroughSeq: 13 }); pendingStreamId = streamId }
    return { streamId: streamId! }
  })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    startSse, stopSse: vi.fn(async () => undefined), ackSseEvent: vi.fn(async () => true),
    onSseEvent: (handler: NonNullable<typeof onData>) => { onData = handler; return () => { onData = undefined } },
    onSseEnd: (handler: NonNullable<typeof onEnd>) => { onEnd = handler; return () => { onEnd = undefined } },
    onSseError: () => () => undefined
  } } })
  try {
    const provider = new AnalytixRuntimeProvider()
    vi.spyOn(provider, 'getThreadDetail').mockImplementation(io.detail)
    vi.spyOn(provider, 'subscribeThreadEvents').mockImplementation((threadId, _seq, sink, signal) =>
      new Promise<void>(resolve => { io.streams.push({ threadId, sink, signal, close: resolve }) }))
    io.runtimeProvider = provider
    await render(); await settle(); const stream = liveInspectorStream()
    io.detail.mockResolvedValue({ ...snapshot(13), blocks: [user], threadStatus: 'completed' })
    await act(async () => { void stream.sink.onSnapshotRequired?.({ threadId: 'child-a', seq: 10, highestSeq: 10, reason: 'synthetic same-cursor race' }) })
    await settle(); expect(pendingStreamId).toBeDefined()
    await act(async () => dispatchAnalytixRuntimeEvents([footerGeneralBatch()], stream.sink, async () => {}))
    const committed = io.timeline!.blocks
    await act(async () => {
      onData?.({ streamId: pendingStreamId!, events: [footerGeneralBatch() as PublicSsePayload['events'][number]] })
      onEnd?.({ streamId: pendingStreamId! })
    })
    await settle()
    expect(io.timeline!.blocks).toBe(committed)
    expect(io.timeline!.blocks.at(-1)).toMatchObject({ generalTerminalProjectionReceipt: { firstSeq: 11, lastSeq: 13 } })
    expect(io.timeline!.runtimeStateOverride?.turnUsageById?.['turn-child']).toEqual(turnUsageEntryFromBatch(mappedFooterBatch()))
    // Preserved general batch identity still provides the actual idempotent transport commit.
    await act(async () => dispatchAnalytixRuntimeEvents([footerGeneralBatch()], liveInspectorStream().sink, async () => {}))
    expect(io.timeline!.blocks).toBe(committed)
  } finally {
    await act(async () => root.render(null))
    if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge); else Reflect.deleteProperty(window, 'analytix')
  }
})


describe('selected Inspector composer authority', () => {
  let rangeGeometry: Array<PropertyDescriptor | undefined>
  const settings = () => {
    const value = normalizeAppSettings({} as AppSettingsV1)
    value.runtime.speechToText.enabled = true
    return value
  }
  beforeEach(() => {
    io.realComposer = true
    io.detail.mockResolvedValue({ ...snapshot(0), latestTurnId: null, latestUserMessageId: null, threadStatus: 'idle' })
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings())
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {} })))
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
    vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
    rangeGeometry = ['getClientRects', 'getBoundingClientRect'].map(name => Object.getOwnPropertyDescriptor(Range.prototype, name))
    Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
    Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
  })
  afterEach(() => ['getClientRects', 'getBoundingClientRect'].forEach((name, index) => {
    const descriptor = rangeGeometry[index]
    if (descriptor) Object.defineProperty(Range.prototype, name, descriptor); else Reflect.deleteProperty(Range.prototype, name)
  }))
  function provider(send: AgentProvider['sendUserMessage'], interrupt = vi.fn(async () => undefined)) {
    io.runtimeProvider = { getThreadDetail: io.detail, sendUserMessage: send, interruptTurn: interrupt,
      subscribeThreadEvents: (threadId: string, _seq: number, sink: ThreadEventSink, signal: AbortSignal) =>
        new Promise<void>(resolve => {
          io.streams.push({ threadId, sink, signal, close: resolve })
          signal.addEventListener('abort', () => resolve())
        }) } as unknown as AgentProvider
  }
  it.each(['/compact literal', '/fork literal', '/archive literal'])('sends %s only to selected child and preserves model/voice controls', async text => {
    const send = vi.fn<AgentProvider['sendUserMessage']>(async threadId => ({ threadId, turnId: 'owned-turn' }))
    const primaryCompact = vi.fn(), primaryFork = vi.fn(), primaryArchive = vi.fn()
    useChatStore.setState({ compactActiveThread: primaryCompact, forkActiveThread: primaryFork, archiveThread: primaryArchive })
    provider(send); await render(); await settle()
    expect(io.composer).toMatchObject({ ownerThreadId: 'child-a', allowBusyFollowUp: false, voiceDictationEnabled: true,
      composerModel: 'synthetic-model', modelPickerMode: 'combobox' })
    expect(container.querySelector(`button[aria-label="${i18n.t('common:composerVoiceStart')}"]`)).not.toBeNull()
    await act(async () => io.composer!.setInput(text))
    const editor = container.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
    await act(async () => editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
    await settle()
    expect(send).toHaveBeenCalledExactlyOnceWith('child-a', expect.any(String), expect.objectContaining({ displayText: text, model: 'synthetic-model' }))
    expect(primaryCompact).not.toHaveBeenCalled(); expect(primaryFork).not.toHaveBeenCalled(); expect(primaryArchive).not.toHaveBeenCalled()
    expect(useChatStore.getState().activeThreadId).toBe('parent')
  })
  it('keeps busy Enter inert and Stop bound to the actual selected turn', async () => {
    io.detail.mockResolvedValue(snapshot())
    const send = vi.fn<AgentProvider['sendUserMessage']>(), stop = vi.fn(async () => undefined)
    provider(send, stop); await render(); await settle()
    await act(async () => io.composer!.setInput('Unsent child input'))
    const editor = container.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
    await act(async () => editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
    expect(send).not.toHaveBeenCalled(); expect(io.composer!.input).toBe('Unsent child input')
    await act(async () => container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:interrupt')}"]`)!.click())
    expect(stop).toHaveBeenCalledExactlyOnceWith('child-a', 'turn-child', undefined)
  })
  it('locks before settings await, keeps an already-clicked A send bound to A and fences its error from B', async () => {
    const send = vi.fn<AgentProvider['sendUserMessage']>().mockRejectedValueOnce(new Error('ERROR_A_ONLY'))
    provider(send); await render(); await settle()
    await act(async () => io.composer!.setInput('A saved input'))
    let resolve!: (value: AppSettingsV1) => void
    vi.mocked(rendererRuntimeClient.getSettings).mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const submit = io.composer!.onSend
    await act(async () => { submit(); submit() })
    expect(io.composer!.busy).toBe(true); expect(send).not.toHaveBeenCalled()
    await render('child-b'); await settle(); await act(async () => io.composer!.setInput('B retained input'))
    await act(async () => resolve(settings())); await settle()
    expect(send).toHaveBeenCalledOnce(); expect(send.mock.calls[0][0]).toBe('child-a')
    expect(container.textContent).not.toContain('ERROR_A_ONLY'); expect(io.composer!.input).toBe('B retained input')
    await render('child-a'); await settle(); expect(io.composer!.input).toBe('A saved input')
  })
  it('checks the same admitted voice ticket at insertion and delayed send after A → B → A', async () => {
    vi.useFakeTimers()
    const send = vi.fn<AgentProvider['sendUserMessage']>()
    provider(send); await render(); await settle()
    let current = true
    const voice = io.voice!
    await act(async () => voice.onText('A dictated draft', 'send', () => current))
    expect(io.composer!.input).toBe('A dictated draft')
    current = false
    await render('child-b'); await render('child-a')
    await act(async () => vi.advanceTimersByTime(0)); await settle()
    expect(send).not.toHaveBeenCalled(); expect(io.composer!.input).toBe('A dictated draft')
    await act(async () => io.voice!.onText('STALE_INSERT', 'insert', () => false))
    expect(io.composer!.input).toBe('A dictated draft')
  })
  it('retains current voice text without turning delayed Send into Stop when the same child becomes busy', async () => {
    vi.useFakeTimers()
    const send = vi.fn<AgentProvider['sendUserMessage']>(), stop = vi.fn(async () => undefined)
    provider(send, stop); await render(); await settle()
    await act(async () => io.voice!.onText('Owned voice draft', 'send', () => true))
    const stream = io.streams.find(value => value.threadId === 'child-a')!
    await act(async () => stream.sink.onTurnStarted?.({ threadId: 'child-a', turnId: 'turn-child', seq: 1, createdAt: timestamp }))
    expect(io.composer!.busy).toBe(true)
    await act(async () => vi.advanceTimersByTime(0)); await settle()
    expect(send).not.toHaveBeenCalled(); expect(stop).not.toHaveBeenCalled()
    expect(io.composer!.input).toBe('Owned voice draft')
  })
})
