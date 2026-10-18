// @vitest-environment jsdom
import { act, createElement, useState, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { chatBlockFromItem, dispatchAnalytixRuntimeEvent, mergeChatBlocks } from '../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../agent/analytix-contract'
import type { ChatBlock, ToolBlock } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import { buildThreadEventSink } from '../../store/chat-store-runtime'
import type { ChatState, ChatStoreSet } from '../../store/chat-store-types'
import { writeWorkDetailsPreference } from '../../lib/chat-presentation-preferences'
import i18n from '../../i18n'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1 } from '../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { MessageTimeline } from './MessageTimeline'
import { GateResponseContext } from './message-timeline-bubbles'
import type { TrajectoryPanelProps } from '../trajectory/TrajectoryPanel'
import type { TrajectoryToolIdentity } from '../trajectory/trajectory-adapter'

// The public Trajectory implementation has its own direct tests. Keep this suite
// at the real mode shell, record lookup, physical rows and protected lease seam.
let trajectoryProps: TrajectoryPanelProps | null = null
let trajectorySelect: ((identity: TrajectoryToolIdentity, inspect?: TrajectoryPanelProps['onInspectTool']) => void) | null = null
vi.mock('../trajectory/TrajectoryPanel', () => ({ TrajectoryPanel: function TrajectoryPanelFixture(props: TrajectoryPanelProps) {
  trajectoryProps = props
  const [selection, setSelection] = useState<TrajectoryToolIdentity | null>(null)
  const [query, setQuery] = useState('')
  trajectorySelect = (identity, inspect = props.onInspectTool) => { setSelection(identity); inspect?.(identity) }
  return createElement('section', { 'data-trajectory-shell-fixture': props.threadId },
    createElement('input', { 'data-trajectory-fixture-search': true, readOnly: true, value: query }),
    createElement('button', { onClick: () => setQuery('resident search') }, 'Fixture search'),
    selection ? createElement('aside', { 'data-trajectory-fixture-inspector': true },
      props.renderToolInspector?.(selection, () => setSelection(null))) : null)
} }))

vi.mock('../../agent/registry', () => ({ getProvider: () => ({ getThreadDetail: async () => ({ authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} }) }) }))
const timestamp = '2026-10-09T00:00:00Z', callId = 'call_host_' + 'a'.repeat(64), resultId = 'item_result_' + 'b'.repeat(64)
const withheld = { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
  privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
const host = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed', code: 'tool_completed',
  messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
type FixtureToolOptions = { turnId?: string; callItemId?: string; resultItemId?: string; toolName?: 'get_goal' | 'list_jobs' }
function toolItems({ turnId = 'turn', callItemId = 'real-call-item', resultItemId = resultId, toolName = 'get_goal' }: FixtureToolOptions = {}): CoreTurnItemJson[] {
  const base = { threadId: 'thread', turnId, callId, toolName,
    toolKind: toolName === 'list_jobs' ? 'command_execution' as const : 'tool_call' as const, createdAt: timestamp }
  const output = toolName === 'list_jobs' ? { ...host, projectionKind: 'task_job_result',
    taskJob: { operation: 'list', result: { jobs: [], count: 0 } } } : host
  return [
    { ...base, id: callItemId, kind: 'tool_call', role: 'assistant', status: 'running', arguments: withheld },
    { ...base, id: resultItemId, kind: 'tool_result', role: 'tool', status: 'completed', output, isError: false }
  ]
}
function tool(options: FixtureToolOptions = {}): ToolBlock {
  const blocks = mergeChatBlocks(toolItems(options).map(item => chatBlockFromItem(item)!))
  if (blocks[0]?.kind !== 'tool') throw new Error('fixture must pass actual mapper'); return blocks[0]
}
const user: ChatBlock = { id: 'real-user', kind: 'user', text: 'ADMITTED_REQUEST', meta: { turnId: 'turn' } }
const snapshot: ToolResultLocalDisplayV1 = { schemaVersion: 1, toolName: 'get_goal', resultItemId: resultId, snapshotDigest: 'c'.repeat(64), canNavigate: false,
  capture: { kind: 'goal', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
    goal: { goal: { threadId: 'thread', objective: 'PRIVATE_BOUND_OBJECTIVE', status: 'active', tokensUsed: 2, timeUsedSeconds: 4,
      createdAt: timestamp, updatedAt: timestamp }, remainingTokens: null } } }
function jobSnapshot(resultItemId = resultId): ToolResultLocalDisplayV1 {
  return toolResultLocalDisplaySchemaV1.parse({ schemaVersion: 1, toolName: 'list_jobs', resultItemId, snapshotDigest: 'c'.repeat(64),
    capture: { kind: 'inspect', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false },
    inspect: { inputJSON: '{"original":"PRIVATE_BOUND_JOB_INPUT"}', schemaJSON: '{"name":"list_jobs","description":"PRIVATE_BOUND_JOB_SCHEMA","parameters":{"type":"object"}}',
      inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } })
}
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>
let scrollDescriptors: Record<string, PropertyDescriptor | undefined>
let api: ReturnType<typeof makeApi>, frames: Map<number, FrameRequestCallback>, serial: number
const invalidateListeners = new Set<(id: string | null) => void>()
const processPaneInvalidators = new Set<{ paneId: string; notify: () => void }>()
const runtimeListeners = new Set<(status: { state: string }) => void>()
function makeApi() {
  return { openToolResultLocalDisplay: vi.fn(async (_request: { viewId: string; turnId: string; callId: string; resultItemId: string }) => ({ ok: true, display: snapshot })), closeToolResultLocalDisplay: vi.fn(async () => undefined),
    onToolResultLocalDisplayInvalidated: (listener: (id: string | null) => void) => { invalidateListeners.add(listener); return () => { invalidateListeners.delete(listener) } },
    onProcessPaneInvalidated: (request: { paneId: string }, notify: () => void) => { const subscription = { paneId: request.paneId, notify }; processPaneInvalidators.add(subscription); return () => { processPaneInvalidators.delete(subscription) } },
    onRuntimeStatus: (listener: (status: { state: string }) => void) => { runtimeListeners.add(listener); return () => { runtimeListeners.delete(listener) } }, copyToolResultLocalDisplay: vi.fn(), saveToolResultLocalDisplay: vi.fn() }
}
const draw = async (blocks: ChatBlock[] = [user, tool()], activeThreadId: string | null = 'thread', overrides: Partial<ComponentProps<typeof MessageTimeline>> = {}) => {
  await act(async () => root.render(createElement(MessageTimeline, { blocks, live: '', activeThreadId, runtimeConnection: 'ready',
    onRetryConnection() {}, onOpenSettings() {}, conversationUsage: null,
    runtimeStateOverride: { busy: false, currentTurnId: null, currentTurnUserId: null, authoritativeTurnTimings: {}, authoritativeTurnStatuses: { turn: 'completed' } }, ...overrides })))
}
const flushFrames = async () => { for (let count = 0; frames.size && count < 8; count++) await act(async () => { const work = [...frames.values()]; frames.clear(); for (const run of work) run(performance.now()) }) }
const click = async (label: string) => {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent?.trim() === label || node.getAttribute('aria-label') === label)
  expect(button, label).toBeDefined(); await act(async () => { button!.focus(); button!.click() })
}
// Reveal the actual physical tool's public ancestors before requesting its
// protected leaf. A neighboring turn may reuse its callId, so DOM order is not identity.
const inspectConversationBlock = async (blockId: string) => {
  const seat = () => {
    const matches = [...container.querySelectorAll<HTMLElement>('[data-chat-seat]')].filter(node => node.dataset.chatSeat === blockId)
    expect(matches, 'one actual physical tool seat').toHaveLength(1)
    return matches[0]
  }
  const chat = seat().closest<HTMLElement>('[data-analytix-chat-view]')!
  expect(chat).not.toBeNull()
  const process = chat.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!
  expect(process).not.toBeNull()
  if (process.getAttribute('aria-expanded') !== 'true') await act(async () => { process.focus(); process.click() })
  const group = seat().closest<HTMLElement>('[data-process-group]')!
  expect(group).not.toBeNull()
  const groupTrigger = group.querySelector<HTMLButtonElement>(':scope > div > button[data-disclosure-row]')
  if (groupTrigger && groupTrigger.getAttribute('aria-expanded') !== 'true') await act(async () => { groupTrigger.focus(); groupTrigger.click() })
  const leaf = seat().querySelector<HTMLElement>('[data-recorded-domain-row],[data-recorded-job-row]')!
  expect(leaf?.dataset.analytixProcessBlockId).toBe(blockId)
  expect(leaf.closest('[hidden]'), 'the real public group is open').toBeNull()
  const row = leaf.querySelector<HTMLButtonElement>('[data-disclosure-row]')!
  expect(row).not.toBeNull()
  if (row.getAttribute('aria-expanded') !== 'true') await act(async () => { row.focus(); row.click() })
  const actions = [...seat().querySelectorAll<HTMLButtonElement>('button')].filter(node => node.textContent?.trim() === 'Inspect')
  expect(actions, 'the selected physical tool Inspect action').toHaveLength(1)
  await act(async () => { actions[0].focus(); actions[0].click() })
  await flushFrames()
}
const expandInspectorJSON = async () => {
  for (let count = 0; count < 16; count++) {
    const toggle = container.querySelector<HTMLButtonElement>('[data-tool-result-inspector] [role="tabpanel"] [data-json-node] button[aria-expanded="false"]')
    if (!toggle) return
    await act(async () => toggle.click())
  }
  throw new Error('fixture JSON expansion exceeded its bound')
}
const inspectFromConversation = async () => {
  const process = [...container.querySelectorAll<HTMLElement>('[data-analytix-turn-process]')].at(-1)!
  if (process && process.getAttribute('aria-expanded') !== 'true') await act(async () => process.click())
  const row = container.querySelector<HTMLElement>('[data-disclosure-row]')!
  if (row.getAttribute('aria-expanded') !== 'true') await act(async () => row.click())
  await click('Inspect'); await flushFrames()
}
beforeEach(async () => {
  trajectoryProps = null
  trajectorySelect = null
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  await i18n.changeLanguage('en'); await writeWorkDetailsPreference('verbose')
  frames = new Map(); serial = 0
  vi.stubGlobal('requestAnimationFrame', (run: FrameRequestCallback) => { frames.set(++serial, run); return serial })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('analytix', { runtime: api = makeApi() })
  scrollDescriptors = Object.fromEntries(['scrollTo', 'scrollIntoView'].map(key => [key, Object.getOwnPropertyDescriptor(HTMLElement.prototype, key)]))
  Object.defineProperty(HTMLElement.prototype, 'scrollTo', { configurable: true, value: vi.fn(function(this: HTMLElement, options: ScrollToOptions | number) { if (typeof options === 'object') this.scrollTop = options.top ?? this.scrollTop }) })
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
  previous = useChatStore.getState(); useChatStore.setState({ activeThreadId: 'thread', route: 'chat', workspaceRoot: '', busy: false, currentTurnId: null, currentTurnUserId: null })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); invalidateListeners.clear(); processPaneInvalidators.clear(); runtimeListeners.clear(); document.getSelection()?.removeAllRanges();
  for (const [key, descriptor] of Object.entries(scrollDescriptors)) { if (descriptor) Object.defineProperty(HTMLElement.prototype, key, descriptor); else Reflect.deleteProperty(HTMLElement.prototype, key) }
  useChatStore.setState(previous); writeWorkDetailsPreference('detailed'); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('main conversation top clearance', () => {
  const expectPadding = (padding: 'pt-4' | 'pt-8') => {
    const content = container.querySelector<HTMLElement>('[data-analytix-message-timeline-content]')!
    expect(content).not.toBeNull()
    expect(content.classList.contains(padding)).toBe(true)
    expect(content.classList.contains(padding === 'pt-4' ? 'pt-8' : 'pt-4')).toBe(false)
  }
  it('uses 16px only for the main Conversation and preserves Records and Trajectory clearance', async () => {
    await draw([user], 'thread', { edgeAlignedScroll: true }); await flushFrames()
    expectPadding('pt-4')
    await click('Records'); await flushFrames(); expectPadding('pt-8')
    await click('Trajectory'); await flushFrames(); expectPadding('pt-8')
    await click('Conversation'); await flushFrames(); expectPadding('pt-4')
  })
  it('keeps the main empty conversation at 16px', async () => {
    await draw([], null, { edgeAlignedScroll: true }); await flushFrames()
    expectPadding('pt-4')
  })
  it.each([
    { route: 'chat', edgeAlignedScroll: false },
    { route: 'write', edgeAlignedScroll: true },
    { route: 'claw', edgeAlignedScroll: true }
  ] as const)('retains 32px outside the main chat mount: %o', async ({ route, edgeAlignedScroll }) => {
    useChatStore.setState({ route })
    await draw([user], 'thread', { edgeAlignedScroll }); await flushFrames()
    expectPadding('pt-8')
  })
})

describe('main chat workspace entry ownership', () => {
  beforeEach(() => useChatStore.setState({ activeThreadId: null, route: 'chat', workspaceRoot: '' }))
  it('leaves the main empty hero brand and preview while the composer owns workspace selection', async () => {
    await draw([], null, { edgeAlignedScroll: true, conversationPhase: 'hero' }); await flushFrames()
    const hero = container.querySelector<HTMLElement>('[data-analytix-conversation-hero]')!
    expect(hero).not.toBeNull()
    expect(hero.textContent).toContain(i18n.t('common:conversationHeroTitle'))
    expect(hero.querySelector('.dsh-conversation-preview')?.textContent).toBe(i18n.t('common:conversationHeroPreview'))
    expect(hero.querySelector('p')).toBeNull(); expect(hero.querySelector('button')).toBeNull()
  })
  it.each([
    { route: 'chat', edgeAlignedScroll: false },
    { route: 'write', edgeAlignedScroll: true }
  ] as const)('keeps the existing workspace action without the main chat picker: %o', async ({ route, edgeAlignedScroll }) => {
    const chooseWorkspace = vi.fn(async () => null)
    useChatStore.setState({ route, chooseWorkspace })
    await draw([], null, { edgeAlignedScroll, conversationPhase: 'hero' }); await flushFrames()
    const hero = container.querySelector<HTMLElement>('[data-analytix-conversation-hero]')!
    expect(hero.querySelector('p')?.textContent).toBe(i18n.t('common:emptyHeroSubNoWorkspace'))
    const choose = hero.querySelector<HTMLButtonElement>('button')!
    expect(choose.textContent).toBe(i18n.t('common:selectWorkspace'))
    await act(async () => choose.click()); expect(chooseWorkspace).toHaveBeenCalledOnce()
  })
  it('keeps the sole workspace action in a secondary context even when edge aligned', async () => {
    const chooseWorkspace = vi.fn(async () => null)
    useChatStore.setState({ chooseWorkspace })
    await act(async () => root.render(createElement(GateResponseContext.Provider, { value: { onOpenOwnerThread: vi.fn() } },
      createElement(MessageTimeline, { blocks: [], live: '', activeThreadId: null, runtimeConnection: 'ready',
        onRetryConnection() {}, onOpenSettings() {}, conversationUsage: null, edgeAlignedScroll: true, conversationPhase: 'hero' }))))
    await flushFrames()
    const hero = container.querySelector<HTMLElement>('[data-analytix-conversation-hero]')!
    const choose = hero.querySelector<HTMLButtonElement>('button')!
    expect(choose.textContent).toBe(i18n.t('common:selectWorkspace'))
    await act(async () => choose.click()); expect(chooseWorkspace).toHaveBeenCalledOnce()
  })
})

describe('production conversation Inspect handoff', () => {
  it('switches to records, focuses and scrolls the exact physical row, then reuses one protected Overview', async () => {
    const subscriptions = vi.spyOn(document, 'addEventListener')
    await draw(); expect(container.querySelector('[data-conversation-composer-overlay]')).toBeNull()
    await inspectFromConversation()
    expect(container.querySelector('[data-conversation-composer-overlay]')).toBe(container.querySelector('[data-analytix-message-timeline-scroller]'))
    expect(container.querySelector('[data-analytix-chat-view]')).toBeNull()
    expect(container.querySelector('[data-conversation-source-item="real-user"]')?.textContent).toContain('ADMITTED_REQUEST')
    const row = container.querySelector<HTMLElement>('[data-conversation-call-key][aria-pressed="true"]')!
    expect(row.dataset.callItemId).toBe('real-call-item'); expect(row.dataset.resultItemId).toBe(resultId)
    expect(row.scrollIntoView).toHaveBeenCalledWith({ behavior: 'auto', block: 'center' })
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    const inspector = container.querySelector('[data-tool-result-inspector]')!
    expect(inspector.closest('[data-protected-tool-result]')).not.toBeNull()
    expect(inspector.textContent).toContain('real-call-item'); expect(inspector.textContent).toContain(resultId)
    expect(document.activeElement?.textContent).toBe('Summary')
    await click('Result')
    expect(inspector.isConnected, 'the original Inspector remains mounted').toBe(true)
    expect(container.querySelector('[data-tool-result-inspector]')).toBe(inspector)
    expect(subscriptions.mock.calls.some(([type]) => type === 'copy'), 'copy guard was subscribed').toBe(true)
    const range = document.createRange(); range.selectNodeContents(inspector); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    expect(document.getSelection()!.isCollapsed, 'selection is not collapsed').toBe(false)
    expect(range.intersectsNode(inspector), 'selection intersects current protected pane').toBe(true)
    for (const type of ['copy', 'cut', 'dragstart']) {
      const event = new Event(type, { bubbles: true, cancelable: true }); await act(async () => inspector.dispatchEvent(event)); expect(event.defaultPrevented, type).toBe(true)
    }
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    await click('Close details'); expect(document.activeElement).toBe(row)
    await act(async () => row.click()); expect(document.activeElement?.textContent).toBe('Summary')
  })
  it('elects overlay only for the current Records view and retires it on Conversation or thread replacement', async () => {
    await draw(); expect(container.querySelector('[data-conversation-composer-overlay]')).toBeNull()
    await click('Records'); await flushFrames()
    expect(container.querySelectorAll('[data-conversation-composer-overlay]')).toHaveLength(1)
    await click('Conversation'); await flushFrames()
    expect(container.querySelector('[data-conversation-composer-overlay]')).toBeNull()
    expect(container.querySelector('[data-analytix-chat-view]')).not.toBeNull()
    await click('Records'); await flushFrames()
    await draw([], 'other-thread')
    expect(container.querySelector('[data-conversation-composer-overlay]')).toBeNull()
    await draw()
    expect(container.querySelector('[data-conversation-composer-overlay]')).toBeNull()
  })
  it('binds Inspect to the actual call turn even when its physical chat group has a neighboring user', async () => {
    const next: ChatBlock = { kind: 'user', id: 'next-user', text: 'NEXT_USER', meta: { turnId: 'next-turn' } }
    await draw([user, next, tool()]); await inspectFromConversation()
    const row = container.querySelector<HTMLElement>('[data-conversation-call-key][aria-pressed="true"]')!
    expect(row.closest<HTMLElement>('[data-conversation-record-turn]')?.dataset.conversationRecordTurn).toBe('turn')
    expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain(resultId)
  })
  it('opens a normal Records Job row as admitted public Result without adding a private lease', async () => {
    api.openToolResultLocalDisplay.mockImplementation(async request => ({ ok: true, display: jobSnapshot(request.resultItemId) }))
    await draw([user, tool({ toolName: 'list_jobs' })]); await click('Records'); await flushFrames()
    const before = api.openToolResultLocalDisplay.mock.calls.length
    for (const [request] of api.openToolResultLocalDisplay.mock.calls) expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(request.viewId)
    const row = container.querySelector<HTMLButtonElement>('[data-conversation-call-key]')!
    await act(async () => row.click()); await flushFrames()
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    expect([...container.querySelectorAll('[role="tab"]')].map(tab => tab.textContent)).toEqual(['Summary', 'Result'])
    expect(container.querySelector('[data-tool-result-inspector]')?.closest('[data-protected-tool-result]')).toBeNull()
    await click('Result')
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('list')
    expect(container.textContent).not.toMatch(/PRIVATE_|Payload|Schema|Timing|Copy|Save/)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
  })
  it.each(['return', 'targeted-revoke'] as const)('does not reopen Job Inspect after %s when the same result gains its real call identity', async cause => {
    api.openToolResultLocalDisplay.mockImplementation(async request => ({ ok: true, display: jobSnapshot(request.resultItemId) }))
    const resultOnly = chatBlockFromItem(toolItems({ toolName: 'list_jobs' })[1])!
    await draw([user, resultOnly]); await inspectFromConversation()
    await click('Payload'); expect(container.textContent).toContain('PRIVATE_BOUND_JOB_INPUT')
    const before = api.openToolResultLocalDisplay.mock.calls.length
    const view = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    if (cause === 'return') await act(async () => container.querySelector<HTMLButtonElement>('[data-conversation-public-result]')!.click())
    else await act(async () => { for (const listener of invalidateListeners) listener(view) })
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(view)
    expect(container.textContent).not.toContain('PRIVATE_')
    await draw([user, tool({ toolName: 'list_jobs' })]); await flushFrames()
    expect(container.querySelector('[data-conversation-call-key][aria-pressed="true"]')?.getAttribute('data-call-item-id')).toBe('real-call-item')
    expect([...container.querySelectorAll('[role="tab"]')].map(tab => tab.textContent)).toEqual(['Summary', 'Result'])
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    expect(container.textContent).not.toContain('PRIVATE_')
  })
  it.each(['get_goal', 'list_jobs'] as const)('keeps the clicked %s native turn when another turn reuses the callId in the same chat group', async toolName => {
    const secondResultId = 'item_result_' + 'd'.repeat(64)
    let state: ChatState = { ...useChatStore.getState(), blocks: [user], activeThreadId: 'thread', busy: true,
      currentTurnId: 'turn', currentTurnUserId: user.id, liveAssistant: '' }
    const set: ChatStoreSet = patch => { state = { ...state, ...(typeof patch === 'function' ? patch(state) : patch) } }
    const sink = buildThreadEventSink(set, () => state, { threadId: 'thread' })
    // Real live ingress preserves both turns, including the second logical
    // block ID; a global merge by callId would hide this production counterexample.
    for (const item of [...toolItems({ toolName }), ...toolItems({ toolName, turnId: 'other-turn', callItemId: 'other-call-item', resultItemId: secondResultId })]) {
      await dispatchAnalytixRuntimeEvent({ kind: 'item_created', item }, sink, async () => undefined)
    }
    const tools = state.blocks.filter((block): block is ToolBlock => block.kind === 'tool')
    expect(tools).toHaveLength(2); expect(tools[0].id).not.toBe(tools[1].id)
    expect(tools.map(block => block.meta?.callId)).toEqual([callId, callId])
    expect(tools.map(block => block.meta?.turnId)).toEqual(['turn', 'other-turn'])
    api.openToolResultLocalDisplay.mockImplementation(async request => ({ ok: true, display: toolName === 'list_jobs'
      ? jobSnapshot(request.resultItemId) : { ...snapshot, resultItemId: request.resultItemId } }))
    await draw(state.blocks)
    const leaves = container.querySelectorAll<HTMLElement>('[data-recorded-domain-row],[data-recorded-job-row]')
    expect(leaves).toHaveLength(2)
    const target = tools.filter(block => block.meta?.turnId === 'other-turn' && block.meta.callId === callId && block.meta.toolResultItemId === secondResultId)
    expect(target).toHaveLength(1)
    await inspectConversationBlock(target[0].id)
    expect(container.querySelector('[data-analytix-chat-view]')).toBeNull()
    const selected = container.querySelector<HTMLElement>('[data-conversation-call-key][aria-pressed="true"]')!
    expect(selected.dataset.callItemId).toBe('other-call-item'); expect(selected.dataset.resultItemId).toBe(secondResultId)
    expect(selected.closest<HTMLElement>('[data-conversation-record-turn]')?.dataset.conversationRecordTurn).toBe('other-turn')
    expect(selected.scrollIntoView).toHaveBeenCalledWith({ behavior: 'auto', block: 'center' })
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain(secondResultId)
    expect(container.textContent).not.toMatch(/Waiting for the exact record to load|The call identity is ambiguous/)
    expect(api.openToolResultLocalDisplay).toHaveBeenLastCalledWith(expect.objectContaining({ turnId: 'other-turn', callId, resultItemId: secondResultId }))
    expect(container.querySelector('[data-tool-result-inspector]')?.closest('[data-protected-tool-result]')).not.toBeNull()
    if (toolName === 'list_jobs') {
      await click('Payload'); expect(container.textContent).toContain('PRIVATE_BOUND_JOB_INPUT')
      expect(container.textContent).not.toMatch(/Copy|Save/)
    }
  })
  it('leaves a legacy call pending until its exact admitted source arrives', async () => {
    const source = tool(), legacy = { ...source, meta: { ...source.meta, recordThreadId: undefined, toolCallIdentity: undefined, toolResultIdentity: undefined } }
    await draw([user, legacy]); await inspectFromConversation()
    expect(container.textContent).toContain('Waiting for the exact record to load'); expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    await draw([user, source]); await flushFrames()
    expect(container.querySelector('[data-conversation-call-key][aria-pressed="true"]')).not.toBeNull()
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
  })
  it('reveals the loaded historical target through the existing turn window and virtualizer before applying it', async () => {
    const source = tool(), legacy = { ...source, meta: { ...source.meta, recordThreadId: undefined } }
    await draw([user, legacy]); await inspectFromConversation()
    const history: ChatBlock[] = Array.from({ length: 70 }, (_, index) => ({ kind: 'user', id: 'history-user-' + index,
      text: 'Admitted later request', meta: { turnId: 'history-turn-' + index } }))
    await draw([user, source, ...history]); await flushFrames()
    expect(container.querySelector('[data-conversation-call-key][aria-pressed="true"]')?.getAttribute('data-result-item-id')).toBe(resultId)
    expect(container.querySelectorAll('[data-analytix-timeline-row="turn"]').length).toBeLessThan(71)
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
  })
  it('does not approximate an ambiguous call and can select an explicitly identified physical row', async () => {
    const source = tool(), legacy = { ...source, meta: { ...source.meta, recordThreadId: undefined } }
    await draw([user, legacy]); await inspectFromConversation()
    const duplicate: ToolBlock = { ...source, id: 'second-physical-row', meta: { ...source.meta, toolCallItemId: undefined, toolCallIdentity: undefined,
      toolResultItemId: 'second-result', toolResultIdentity: { threadId: 'thread', turnId: 'turn', callId, itemId: 'second-result' }, localResult: undefined, sourceItemId: 'second-result' } }
    await draw([user, source, duplicate]); await flushFrames()
    expect(container.textContent).toContain('The call identity is ambiguous'); expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    const exact = [...container.querySelectorAll<HTMLButtonElement>('[data-conversation-call-key]')].find(row => row.dataset.resultItemId === 'second-result')!
    await act(async () => exact.click()); await flushFrames()
    expect(container.querySelectorAll('[data-conversation-call-key][aria-pressed="true"]')).toHaveLength(1)
    expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain('second-result')
  })
  it('retires a late private reply when a newer exact call request selects public Overview', async () => {
    let complete: ((value: { ok: boolean; display: ToolResultLocalDisplayV1 }) => void) | undefined
    api.openToolResultLocalDisplay.mockImplementationOnce(async () => ({ ok: true, display: snapshot }))
      .mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
    const second = chatBlockFromItem({ threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'e'.repeat(64), id: 'second-call-item', toolName: 'get_goal', toolKind: 'tool_call',
      kind: 'tool_call', role: 'assistant', status: 'running', createdAt: timestamp, arguments: withheld })!
    await draw([user, tool(), second]); await inspectFromConversation()
    expect(complete).toBeDefined()
    const row = [...container.querySelectorAll<HTMLButtonElement>('[data-conversation-call-key]')].find(node => node.dataset.callItemId === 'second-call-item')!
    await act(async () => row.click()); await flushFrames()
    await act(async () => complete!({ ok: true, display: snapshot }))
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-call-id')).toBe('call_host_' + 'e'.repeat(64))
    expect(container.textContent).not.toContain('PRIVATE_BOUND_OBJECTIVE')
    expect(document.activeElement?.textContent).toBe('Summary')
  })
  it.each(['hidden', 'collapse', 'thread', 'revoke', 'targeted-revoke'] as const)('retires the selected request on %s and does not reopen it when visible again', async cause => {
    await draw(); await inspectFromConversation()
    const before = api.openToolResultLocalDisplay.mock.calls.length
    const selectedRow = container.querySelector('[data-conversation-call-key][aria-pressed="true"]')
    const currentViewId = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    if (cause === 'hidden') await act(async () => { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) })
    else if (cause === 'collapse') await act(async () => { container.style.visibility = 'collapse'; await Promise.resolve() })
    else if (cause === 'thread') await draw([], 'other-thread')
    else await act(async () => { for (const notify of [...invalidateListeners]) notify(cause === 'targeted-revoke' ? currentViewId : null) })
    await flushFrames(); expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    if (cause === 'targeted-revoke') expect(document.activeElement).not.toBe(selectedRow)
    await act(async () => { vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); container.style.visibility = ''; document.dispatchEvent(new Event('visibilitychange')) })
    await flushFrames(); expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    expect(container.querySelector('[data-conversation-call-key][aria-pressed="true"]')).toBeNull()
  })
})


describe('current-conversation Trajectory shell', () => {
  const identity = (patch: Partial<TrajectoryToolIdentity> = {}): TrajectoryToolIdentity => ({
    threadId: 'thread', turnId: 'turn', callId, toolCallItemId: 'real-call-item', toolResultItemId: resultId, ...patch
  })
  const scroller = () => container.querySelector<HTMLElement>('[data-analytix-message-timeline-scroller]')!
  const expectMode = (mode: 'Conversation' | 'Records' | 'Trajectory') => {
    const buttons = [...container.querySelectorAll<HTMLButtonElement>('button')].filter(button => ['Conversation', 'Records', 'Trajectory'].includes(button.textContent?.trim() ?? ''))
    expect(buttons).toHaveLength(3)
    expect(buttons.filter(button => button.getAttribute('aria-pressed') === 'true').map(button => button.textContent)).toEqual([mode])
  }
  it('offers the third mode for a real empty thread and resets it on thread replacement', async () => {
    await draw([], null)
    expect([...container.querySelectorAll('button')].some(button => button.textContent === 'Trajectory')).toBe(false)
    await draw([])
    expectMode('Conversation')
    await click('Trajectory')
    expectMode('Trajectory')
    expect(container.querySelector('[data-trajectory-shell-fixture]')?.getAttribute('data-trajectory-shell-fixture')).toBe('thread')
    expect(trajectoryProps?.threadId).toBe('thread')
    expect(trajectoryProps?.renderToolInspector).toBeTypeOf('function')
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await draw([], 'other-thread')
    expectMode('Conversation')
    expect(container.querySelector('[data-trajectory-shell-fixture]')).toBeNull()
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
  })
  it('keeps entering Trajectory on public data and unmounts the previous protected lease', async () => {
    await draw(); await inspectFromConversation()
    const lease = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    await click('Result'); await expandInspectorJSON()
    expect(container.textContent).toContain('PRIVATE_BOUND_OBJECTIVE')
    const before = api.openToolResultLocalDisplay.mock.calls.length
    await click('Trajectory'); await flushFrames()
    expectMode('Trajectory')
    expect(container.querySelector('[data-analytix-chat-view]')).toBeNull()
    expect(container.querySelector('[data-conversation-record-turn]')).toBeNull()
    expect(container.querySelector('[data-conversation-record-details]')).toBeNull()
    expect(container.querySelector('[data-protected-tool-result]')).toBeNull()
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(lease)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    await draw([user, tool()]); await flushFrames()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    expectMode('Trajectory')
  })
  it('keeps the same trajectory and search while its embedded Inspector uses the exact physical result', async () => {
    const secondResult = 'item_result_' + 'e'.repeat(64)
    api.openToolResultLocalDisplay.mockImplementation(async request => ({ ok: true, display: { ...snapshot, resultItemId: request.resultItemId } }))
    await draw([user, tool(), tool({ callItemId: 'second-call-item', resultItemId: secondResult })])
    await click('Trajectory')
    await click('Fixture search')
    const panel = container.querySelector('[data-trajectory-shell-fixture]')
    const before = api.openToolResultLocalDisplay.mock.calls.length
    await act(async () => trajectorySelect!(identity({ toolCallItemId: 'second-call-item', toolResultItemId: secondResult })))
    await flushFrames()
    expectMode('Trajectory')
    expect(container.querySelector('[data-trajectory-shell-fixture]')).toBe(panel)
    expect(container.querySelector<HTMLInputElement>('[data-trajectory-fixture-search]')?.value).toBe('resident search')
    expect(container.querySelector('[data-conversation-record-turn]')).toBeNull()
    const inspector = container.querySelector<HTMLElement>('[data-tool-result-inspector]')!
    expect(inspector.closest('[data-trajectory-fixture-inspector]')).not.toBeNull()
    expect(inspector.getAttribute('data-presentation')).toBe('embedded')
    expect(inspector.style.width).toBe('')
    expect(inspector.textContent).toContain('second-call-item')
    expect(inspector.textContent).toContain(secondResult)
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before + 1)
    expect(api.openToolResultLocalDisplay.mock.calls.at(-1)![0]).toMatchObject({ threadId: 'thread', turnId: 'turn', callId, resultItemId: secondResult })
    const lease = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    await click('Close details')
    expectMode('Trajectory')
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(container.querySelector('[data-trajectory-shell-fixture]')).toBe(panel)
    expect(container.querySelector<HTMLInputElement>('[data-trajectory-fixture-search]')?.value).toBe('resident search')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(lease)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('rejects foreign, incomplete, missing and ambiguous identities without a private read', async () => {
    await draw([user, tool(), tool()]); await click('Trajectory')
    const inspect = trajectoryProps!.onInspectTool!
    const before = api.openToolResultLocalDisplay.mock.calls.length
    for (const candidate of [identity({ threadId: 'foreign' }), identity({ toolResultItemId: undefined }),
      identity({ toolResultItemId: 'not-recorded' }), identity()]) {
      await act(async () => trajectorySelect!(candidate))
      expectMode('Trajectory')
      expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    }
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    // Retaining a callback from an unmounted view never grants a new selection,
    // including the same thread's trajectory -> conversation -> trajectory cycle.
    await click('Conversation'); await click('Trajectory')
    await draw([user, tool()])
    const beforeStale = api.openToolResultLocalDisplay.mock.calls.length
    await act(async () => trajectorySelect!(identity(), inspect))
    expectMode('Trajectory')
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(beforeStale)
  })
  it.each(['missing', 'duplicate', 'selector-removed', 'status-changed'] as const)('permanently retires one explicit Inspector when its source is %s', async cause => {
    const source = tool()
    await draw([user, source]); await click('Trajectory')
    await act(async () => trajectorySelect!(identity()))
    expect(container.querySelector('[data-tool-result-inspector]')).not.toBeNull()
    const before = api.openToolResultLocalDisplay.mock.calls.length
    const lease = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    const changed: ChatBlock[] = cause === 'missing' ? [user] : cause === 'duplicate' ? [user, source, source] : [user,
      cause === 'selector-removed' ? { ...source, meta: { ...source.meta, localResult: undefined } } : { ...source, status: 'error' }]
    await draw(changed)
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(lease)
    await draw([user, source])
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    await act(async () => trajectorySelect!(identity()))
    expect(container.querySelector('[data-tool-result-inspector]')).not.toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before + 1)
  })
  it('does not upgrade an already selected public result when a private selector later arrives', async () => {
    const source = tool(), publicOnly: ToolBlock = { ...source, meta: { ...source.meta, localResult: undefined } }
    await draw([user, publicOnly]); await click('Trajectory')
    const before = api.openToolResultLocalDisplay.mock.calls.length
    await act(async () => trajectorySelect!(identity()))
    expect(container.querySelector('[data-tool-result-inspector][data-presentation="embedded"]')).not.toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    await draw([user, source])
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
    await act(async () => trajectorySelect!(identity()))
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before + 1)
  })
  it.each(['hidden', 'ancestor-hidden', 'targeted-revoke', 'global-revoke', 'runtime-stopped'] as const)('retires embedded Job Inspect on %s without reopening after recovery', async cause => {
    api.openToolResultLocalDisplay.mockImplementation(async request => ({ ok: true, display: jobSnapshot(request.resultItemId) }))
    const blocks = [user, tool({ toolName: 'list_jobs' })]
    await draw(blocks); await click('Trajectory'); await click('Fixture search')
    await act(async () => trajectorySelect!(identity()))
    await click('Payload'); expect(container.textContent).toContain('PRIVATE_BOUND_JOB_INPUT')
    const before = api.openToolResultLocalDisplay.mock.calls.length
    const lease = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    await act(async () => {
      if (cause === 'hidden') { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) }
      else if (cause === 'ancestor-hidden') { container.hidden = true; await Promise.resolve() }
      else if (cause === 'runtime-stopped') { for (const notify of [...runtimeListeners]) notify({ state: 'stopped' }) }
      else { for (const notify of [...invalidateListeners]) notify(cause === 'targeted-revoke' ? lease : null) }
    })
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_BOUND_JOB_INPUT')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(lease)
    await act(async () => { vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); container.hidden = false; document.dispatchEvent(new Event('visibilitychange')) })
    await draw(blocks)
    expectMode('Trajectory')
    expect(container.querySelector<HTMLInputElement>('[data-trajectory-fixture-search]')?.value).toBe('resident search')
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(before)
  })
  it('keeps B mounted when A cleanup and its late private reply arrive after the newer explicit selection', async () => {
    const secondResult = 'item_result_' + 'd'.repeat(64)
    await draw([user, tool(), tool({ callItemId: 'second-call-item', resultItemId: secondResult })]); await click('Trajectory')
    let complete: ((value: { ok: boolean; display: ToolResultLocalDisplayV1 }) => void) | undefined
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
      .mockImplementation(async request => ({ ok: true, display: { ...snapshot, resultItemId: request.resultItemId } }))
    await act(async () => trajectorySelect!(identity()))
    expect(complete).toBeDefined()
    const leaseA = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
    await act(async () => trajectorySelect!(identity({ toolCallItemId: 'second-call-item', toolResultItemId: secondResult })))
    const inspectorB = container.querySelector('[data-tool-result-inspector]')!
    expect(inspectorB.textContent).toContain(secondResult)
    await act(async () => complete!({ ok: true, display: snapshot }))
    expectMode('Trajectory')
    expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
    expect(container.querySelector('[data-tool-result-inspector]')).toBe(inspectorB)
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(leaseA)
  })
  it('retains each mode viewport without sampling trajectory scroll into the chat position', async () => {
    await draw()
    const viewport = scroller()
    Object.defineProperty(viewport, 'scrollHeight', { configurable: true, get: () => 5_000 })
    Object.defineProperty(viewport, 'clientHeight', { configurable: true, get: () => 600 })
    viewport.scrollTop = 430
    await act(async () => viewport.dispatchEvent(new Event('scroll')))
    await click('Trajectory'); await flushFrames()
    expect(scroller()).toBe(viewport)
    expect(viewport.scrollTop).toBe(0)
    viewport.scrollTop = 180
    await act(async () => viewport.dispatchEvent(new Event('scroll')))
    await draw([user, tool(), { kind: 'assistant', id: 'new-public-answer', text: 'NEW_ADMITTED_ANSWER', meta: { turnId: 'turn' } }])
    await flushFrames()
    expect(viewport.scrollTop).toBe(180)
    await click('Conversation'); await flushFrames()
    expectMode('Conversation')
    expect(viewport.scrollTop).toBe(430)
    await click('Trajectory'); await flushFrames()
    expect(viewport.scrollTop).toBe(180)
    await draw([], 'replacement-thread'); await flushFrames()
    expectMode('Conversation')
    expect(container.querySelector('[data-trajectory-shell-fixture]')).toBeNull()
  })
})

it('releases an in-flight private lease on trajectory entry and discards its late reply', async () => {
  let complete: ((value: { ok: boolean; display: ToolResultLocalDisplayV1 }) => void) | undefined
  api.openToolResultLocalDisplay.mockImplementationOnce(async () => ({ ok: true, display: snapshot }))
    .mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
  await draw(); await inspectFromConversation()
  expect(complete).toBeDefined()
  const lease = api.openToolResultLocalDisplay.mock.calls.at(-1)![0].viewId
  await click('Trajectory')
  await act(async () => complete!({ ok: true, display: snapshot }))
  await flushFrames()
  expect(container.querySelector('[data-trajectory-shell-fixture]')).not.toBeNull()
  expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
  expect(container.textContent).not.toContain('PRIVATE_BOUND_OBJECTIVE')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(lease)
})
it('rebinds the live spacer observer after a trajectory round trip while the same response remains live', async () => {
  const observers: { targets: Element[]; disconnect: ReturnType<typeof vi.fn> }[] = []
  vi.stubGlobal('ResizeObserver', class {
    targets: Element[] = []
    disconnect = vi.fn()
    constructor() { observers.push(this) }
    observe(target: Element) { this.targets.push(target) }
    unobserve() {}
  })
  await draw([user, tool()], 'thread', { live: 'ADMITTED_LIVE_RESPONSE' })
  const first = container.querySelector('[data-analytix-response-spacer="live"]')!
  const owner = observers.find(observer => observer.targets.includes(first))!
  expect(owner).toBeDefined()
  await click('Trajectory')
  expect(owner.disconnect).toHaveBeenCalled()
  await click('Conversation')
  const replacement = container.querySelector('[data-analytix-response-spacer="live"]')!
  expect(replacement).not.toBe(first)
  expect(observers.some(observer => observer !== owner && observer.targets.includes(replacement))).toBe(true)
})
