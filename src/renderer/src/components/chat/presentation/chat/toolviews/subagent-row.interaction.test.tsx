// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToolCallTurnItem, ToolResultTurnItem, type PublicToolResultProjectionV1 } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import { toolResultLocalViewResultSchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalViewResultV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { CoreChildRuntimeMetadataJson, CoreTurnItemJson } from '../../../../../agent/analytix-contract'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import type { ToolBlock } from '../../../../../agent/types'
import i18n from '../../../../../i18n'
import { useChatStore } from '../../../../../store/chat-store'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { ToolCallTree } from '../ToolCallTree'
import { presentationPolicyFor } from '../chat-view-model'
import { ToolRawIO } from './ToolRawIO'
import { childInputRequestSchemaV1 } from '@shared/child-input-contract'

const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const projection = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
  code: 'tool_completed', messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } satisfies PublicToolResultProjectionV1
function mapped(name = 'task', child?: CoreChildRuntimeMetadataJson, failed = false): ToolBlock {
  const item: CoreTurnItemJson = { id: selector.resultItemId, threadId: 'thread', turnId: 'turn', kind: 'tool_result', role: 'tool',
    status: failed ? 'failed' : 'completed', toolName: name, toolKind: 'subagent', callId: selector.callId, createdAt: '2026-10-11T00:00:00Z',
    output: failed ? { ...projection, status: 'failed', code: 'tool_failed', messageKey: 'tool_failed' } : projection, isError: failed }
  expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item, child)
  if (block?.kind !== 'tool') throw new Error('Fixture must use the real mapper')
  expect(block.meta?.localResult).toEqual(selector)
  return block
}
function captured(name = 'task') {
  return { schemaVersion: 1, resultItemId: selector.resultItemId, snapshotDigest: 'c'.repeat(64), toolName: name, canNavigate: false,
    capture: { kind: 'job', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
      job: { operation: 'spawn', total: 1, truncated: false, receipt: null, jobs: [{ jobId: 'job-a', kind: 'task', status: 'running',
        name: 'PRIVATE_NAME', label: 'PRIVATE_LABEL', output: { availability: 'unavailable' } }] } },
    inspect: { inputJSON: '{"prompt":"PRIVATE_PROMPT"}', inputTruncated: false, schemaTruncated: true, callEventAt: null, resultEventAt: null, durationMs: null } } satisfies ToolResultLocalDisplayV1
}
const nativeChild: CoreChildRuntimeMetadataJson = { parentThreadId: 'thread', parentTurnId: 'turn', parentToolCallId: selector.callId,
  childId: 'child-a', jobId: 'job-a', childRunId: 'job-a', childThreadId: 'child-thread', childTurnId: 'child-turn', childStatus: 'running',
  kind: 'subagent', background: true, canAcceptSteer: true, canPause: true }
let root: Root, container: HTMLDivElement, display: ToolResultLocalDisplayV1
let previousApi: PropertyDescriptor | undefined, previousHidden: PropertyDescriptor | undefined, previousStore: ReturnType<typeof useChatStore.getState>
let invalidators: Set<(id: string | null) => void>
let api: { runtimeRequest: ReturnType<typeof vi.fn>; openToolResultLocalDisplay: ReturnType<typeof vi.fn>; closeToolResultLocalDisplay: ReturnType<typeof vi.fn>; onToolResultLocalDisplayInvalidated: ReturnType<typeof vi.fn>; onRuntimeStatus: ReturnType<typeof vi.fn> }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix'); previousHidden = Object.getOwnPropertyDescriptor(document, 'hidden')
  Object.defineProperty(document, 'hidden', { configurable: true, value: false })
  previousStore = useChatStore.getState(); useChatStore.setState({ workspaceRoot: '', activeThreadId: 'thread' })
  display = captured(); invalidators = new Set()
  api = { runtimeRequest: vi.fn(childInputHostFixture), openToolResultLocalDisplay: vi.fn(async (): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async () => undefined), onRuntimeStatus: vi.fn(() => () => undefined),
    onToolResultLocalDisplayInvalidated: vi.fn((handler: (id: string | null) => void) => { invalidators.add(handler); return () => invalidators.delete(handler) }) }
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); useChatStore.setState(previousStore)
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix')
  if (previousHidden) Object.defineProperty(document, 'hidden', previousHidden); else Reflect.deleteProperty(document, 'hidden')
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
async function draw(block: ToolBlock, props: Partial<Parameters<typeof ToolCallTree>[0]> = {}) {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(ToolCallTree, {
    blocks: [block], processing: false, policy: presentationPolicyFor('detailed'), ...props
  }))))
  if (props.visible !== false && props.publicVisible !== false) {
    for (const header of container.querySelectorAll<HTMLButtonElement>('[data-process-group] > div:first-child button[aria-expanded="false"]')) {
      await act(async () => header.click())
    }
  }
}
const row = () => container.querySelector<HTMLElement>('[data-subagent-row] [data-disclosure-row]')!
const click = async (element: HTMLElement) => { await act(async () => element.click()) }
function runningInputFixture() {
  const call = { callId: selector.callId, toolCallItemId: 'actual-running-call-item', startedEventSeq: '12' }
  const item: CoreTurnItemJson = { id: call.toolCallItemId, threadId: 'thread', turnId: 'turn', role: 'assistant',
    status: 'running', createdAt: '2026-10-11T00:00:00Z', kind: 'tool_call', toolName: 'task', callId: call.callId, toolKind: 'subagent',
    arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } }
  expect(ToolCallTurnItem.safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item, nativeChild)
  if (block?.kind !== 'tool') throw new Error('Fixture must use the actual running mapper')
  const raw: ProcessDisplayBlockV1 = { blockId: 'subagent-raw', kind: 'tool_input', state: 'running', attempt: 0,
    source: { kind: 'tool', unitId: 'subagent-unit', ...call }, durationMs: 0, truncated: false, classification: 'source_exact_local',
    input: { availability: 'available', inputJSON: ' {"sequence":9007199254740993,"prompt":"Actual"} ' } }
  let expanded = false
  const setOpen = vi.fn((value: boolean) => { expanded = value })
  const local = { threadId: 'thread', turnId: 'turn', units: [{ unitId: 'subagent-unit', producerId: 'producer-subagent',
    providerSequence: '1', placement: 'anchored' as const, calls: [call], precedingCalls: [] }], identities: [raw], blocks: [raw], render: () => null,
    capture: () => ({ blockId: raw.blockId, preview: null, body: createElement(ToolRawIO, { input: raw.input }), expanded, expandable: true, setOpen }) }
  return { block, local, waiting: { ...local, identities: [], blocks: [] }, raw, setOpen }
}
async function openRunningGroup() {
  await act(async () => {
    for (const title of container.querySelectorAll<HTMLButtonElement>('button[data-process-activity][aria-expanded="false"]')) title.click()
  })
}
describe('shared running subagent input', () => {
  it.each([false, true])('honors actual row expansion when capture arrives (initially present: %s)', async initiallyPresent => {
    const { block, local, waiting, raw, setOpen } = runningInputFixture()
    await draw(block, { local: initiallyPresent ? local : waiting }); await openRunningGroup()
    const publicControls = container.querySelector('[data-subagent-native-controls]')
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    await click(row())
    if (!initiallyPresent) { expect(setOpen).not.toHaveBeenCalled(); await draw(block, { local }) }
    expect(setOpen).toHaveBeenLastCalledWith(true)
    await draw(block, { local })
    expect(container.querySelector('[data-tool-raw-input]')?.textContent).toBe(raw.input?.availability === 'available' ? raw.input.inputJSON : undefined)
    expect(container.querySelector('[data-subagent-native-controls]')).toBe(publicControls)
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await click(row()); expect(setOpen).toHaveBeenLastCalledWith(false)
    await draw(block, { local }); expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
  })
  it.each(['fold', 'generation', 'visibility'] as const)('retires %s intent before a late running capture arrives', async retirement => {
    const { block, local, waiting, setOpen } = runningInputFixture()
    await draw(block, { local: waiting }); await openRunningGroup(); await click(row())
    const generation = retirement === 'generation' ? 1 : 0
    if (retirement === 'fold') await click(row())
    if (retirement === 'generation') await draw(block, { local: waiting, privateGeneration: generation })
    if (retirement === 'visibility') await draw(block, { local: waiting, visible: false, publicVisible: true })
    await draw(block, { local, privateGeneration: generation })
    await act(async () => container.querySelector('[data-subagent-public-controls]')!.dispatchEvent(new Event('beforematch')))
    await draw(block, { local, privateGeneration: generation })
    expect(setOpen).not.toHaveBeenCalled()
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    expect(container.querySelector('[data-subagent-native-controls]')).not.toBeNull()
  })
  it('keeps searchable public disclosure from granting a late private capture', async () => {
    const { block, local, waiting, setOpen } = runningInputFixture()
    await draw(block, { local: waiting }); await openRunningGroup()
    await act(async () => container.querySelector('[data-subagent-public-controls]')!.dispatchEvent(new Event('beforematch')))
    expect(row().getAttribute('aria-expanded')).toBe('true')
    await draw(block, { local })
    expect(setOpen).not.toHaveBeenCalled()
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
  })
})
describe('actual mapper to DSH subagent row and protected lease', () => {
  it('keeps actual failed child indices in shared OUT while public child controls retain their owner', async () => {
    const original = captured('parallel_tasks')
    display = { ...original, capture: { ...original.capture, status: 'failed' }, inspect: { ...original.inspect!,
      outputError: { availability: 'available', entries: [{ message: 'first failure', classification: 'source_exact_local', parallelIndex: 1 },
        { message: 'third failure', classification: 'source_exact_local', parallelIndex: 3 }] } } }
    expect(toolResultLocalViewResultSchemaV1.safeParse({ ok: true, display }).success).toBe(true)
    await draw(mapped('parallel_tasks', nativeChild, true)); await click(row())
    expect([...container.querySelectorAll('[data-tool-raw-error]')].map(element => element.getAttribute('data-parallel-index'))).toEqual(['1', '3'])
    const publicInput = container.querySelector('[data-subagent-public-controls] input')
    await act(async () => { for (const invalidate of invalidators) invalidate(null) })
    expect(container.querySelector('[data-tool-raw-error]')).toBeNull()
    expect(container.querySelector('[data-subagent-public-controls] input')).toBe(publicInput)
  })
  it.each(['task', 'delegate_task', 'parallel_tasks'])('renders the %s real start receipt and retains Inspect', async name => {
    display = captured(name)
    expect(toolResultLocalViewResultSchemaV1.safeParse({ ok: true, display }).success).toBe(true)
    const inspect = vi.fn(); await draw(mapped(name), { onInspect: inspect })
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    expect(container.textContent).toContain(name === 'parallel_tasks' ? 'PRIVATE_NAME' : 'PRIVATE_PROMPT')
    await click(row())
    expect(container.querySelector('[data-subagent-start-receipt]')).not.toBeNull()
    expect(container.textContent).toContain('Started'); expect(container.textContent).toContain('job-a')
    expect(container.textContent).toContain('Output unavailable'); expect(container.textContent).not.toContain('Reply')
    expect(container.querySelector('[data-tool-io]')).toBeNull()
    await click([...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Inspect')!)
    expect(inspect).toHaveBeenCalledWith('turn', selector.callId)
    expect(useChatStore.getState().threads).toEqual(previousStore.threads)
  })
  it('keeps public running controls, focus and steer draft stable across private revoke, fold and replacement', async () => {
    const block = mapped('task', nativeChild)
    await draw(block); await click(row())
    expect(container.querySelector('[data-subagent-row] [data-state]')?.getAttribute('data-state')).toBe('running')
    const input = container.querySelector<HTMLInputElement>('[data-subagent-public-controls] input')!
    expect(input).not.toBeNull(); expect(input.closest('[data-protected-tool-result]')).toBeNull()
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => { input.focus(); setter.call(input, 'PUBLIC_STEER_DRAFT'); input.dispatchEvent(new Event('input', { bubbles: true })) })
    await act(async () => { for (const invalidate of invalidators) invalidate(null) })
    expect(container.textContent).not.toMatch(/PRIVATE_(NAME|LABEL|PROMPT)/)
    expect(container.querySelector('input')).toBe(input); expect(document.activeElement).toBe(input)
    await draw(block, { visible: false, publicVisible: false, resetKey: 'folded', privateGeneration: 1 })
    expect(container.querySelector('input')).toBe(input); expect(input.value).toBe('PUBLIC_STEER_DRAFT')
    expect(document.activeElement).toBe(input)
    await draw(block, { resetKey: 'unfolded', privateGeneration: 2 })
    expect(container.querySelector('input')).toBe(input); expect(input.value).toBe('PUBLIC_STEER_DRAFT')
  })
  it('retains a failed outer call even when the returned job and public child completed', async () => {
    const block = mapped('task', { ...nativeChild, childStatus: 'completed' }, true)
    await draw(block); await click(row())
    expect(container.querySelector('[data-subagent-row] [data-state]')?.getAttribute('data-state')).toBe('error')
    expect(container.querySelector('[data-subagent-start-receipt]')?.textContent).not.toContain('Started')
    expect(container.querySelector('[data-subagent-public-controls] input')).not.toBeNull()
  })
  it('reports a truncated empty record without inventing child rows or a reply', async () => {
    if (display.capture.kind !== 'job') throw new Error('fixture')
    display = { ...display, capture: { ...display.capture, job: { operation: 'spawn', jobs: [], total: 3, truncated: true, receipt: null } } }
    await draw(mapped()); await click(row())
    expect(container.textContent).toContain('Receipt unavailable'); expect(container.textContent).toContain('Showing 0 of 3 jobs')
    expect(container.querySelectorAll('[data-subagent-start-receipt] li')).toHaveLength(0)
    expect(container.textContent).not.toMatch(/Started|PRIVATE_PROMPT|Reply/)
  })
  it('keeps absent typed results inspectable without constructing a Reply from input', async () => {
    display = { ...display, capture: { kind: 'inspect', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } }
    await draw(mapped()); await click(row())
    expect(container.textContent).not.toContain('PRIVATE_PROMPT'); expect(container.querySelector('[data-subagent-start-receipt]')).toBeNull()
    await click([...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Inspect')!)
    expect(container.querySelector('[data-tool-result-inspector]')).not.toBeNull()
  })
  it('clears hidden private content and rejects a late Host completion after the leaf retires', async () => {
    let resolve!: (value: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise<ToolResultLocalViewResultV1>(done => { resolve = done }))
    const block = mapped(); await draw(block)
    await draw(block, { visible: false, publicVisible: false, privateGeneration: 1 })
    await act(async () => resolve({ ok: true, display }))
    expect(container.textContent).not.toMatch(/PRIVATE_(NAME|LABEL|PROMPT)/)
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledOnce()
  })
  it('clears the actual mounted private summary on document hide without remounting public controls', async () => {
    await draw(mapped('task', nativeChild)); await click(row())
    const input = container.querySelector('input')!
    await act(async () => {
      Object.defineProperty(document, 'hidden', { configurable: true, value: true })
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(container.textContent).not.toMatch(/PRIVATE_(NAME|LABEL|PROMPT)/)
    expect(container.querySelector('input')).toBe(input)
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it('retains real public child navigation and excludes controls when no native child was published', async () => {
    await draw(mapped()); expect(container.querySelector('[data-subagent-native-controls]')).toBeNull()
    await draw(mapped('task', nativeChild)); await click(row())
    expect(container.querySelector('button[aria-label="Open child session"]')).not.toBeNull()
    expect(container.querySelector('button[aria-label="Pause at safe boundary"]')).not.toBeNull()
  })
})

// The real mapper/ToolCallTree/control/client path uses the closed Go response shape.
const hostCaps = { send: 'steer' as 'steer' | 'start' | 'disabled', stop: true,
  runId: 'job-a', childThreadId: 'child-thread', childTurnId: 'child-turn' }
const hostReply = (value: unknown, ok = true, status = 200) => ({ ok, status, body: JSON.stringify(value) })
function childInputHostFixture(_path: string, method: string, body: string) {
  expect(_path).toBe('/v1/runtime/task-jobs/steer'); expect(method).toBe('POST')
  const request = childInputRequestSchemaV1.parse(JSON.parse(body) as unknown)
  if (request.mode === 'inspect') return Promise.resolve(hostReply({ schemaVersion: 1, capabilities: hostCaps }))
  return Promise.resolve(hostReply({ schemaVersion: 1, capabilities: hostCaps, receipt: {
    action: 'steer', admission: 'queued', clientMessageId: request.clientMessageId, jobId: request.expectedRunId,
    childThreadId: hostCaps.childThreadId, childTurnId: request.expectedChildTurnId, messageId: 'message-1'
  } }))
}
const messageInput = () => container.querySelector<HTMLInputElement>('[data-subagent-public-controls] input')!
async function typeChildMessage(text: string) {
  const input = messageInput()
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  await act(async () => { input.focus(); setter.call(input, text); input.dispatchEvent(new Event('input', { bubbles: true })) })
}
async function sendChildMessage() {
  await act(async () => { messageInput().closest('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })) })
}
const sentIntents = () => api.runtimeRequest.mock.calls.map(call => JSON.parse(call[2] as string) as Record<string, string>)
describe('continuable input through the actual native subagent row', () => {
  it.each(['queued', 'admitted'])('inspects and confirms a running child %s in the same run', async admission => {
    api.runtimeRequest.mockImplementation(async (path, method, body) => {
      const response = await childInputHostFixture(path, method, body)
      const parsed = JSON.parse(response.body)
      if (parsed.receipt) parsed.receipt.admission = admission
      return hostReply(parsed)
    })
    await draw(mapped('task', nativeChild)); await click(row()); await typeChildMessage('Continue carefully'); await sendChildMessage()
    const [inspect, submit] = sentIntents()
    expect(inspect).toEqual({ mode: 'inspect', threadId: 'thread', jobId: 'job-a' })
    expect(submit).toMatchObject({ mode: 'start_or_steer', threadId: 'thread', jobId: 'job-a',
      expectedRunId: 'job-a', expectedChildTurnId: 'child-turn', message: 'Continue carefully' })
    expect(submit.clientMessageId).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    expect(messageInput().value).toBe('')
  })
  it('keeps an unknown draft and retries the exact envelope even when current run capabilities changed', async () => {
    let sent = 0
    api.runtimeRequest.mockImplementation(async (path, method, body) => {
      const response = await childInputHostFixture(path, method, body)
      const parsed = JSON.parse(response.body)
      if (parsed.receipt && ++sent === 1) return hostReply({ schemaVersion: 1,
        capabilities: { send: 'disabled', stop: false }, code: 'child_input_unknown' }, false, 503)
      if (parsed.receipt) parsed.capabilities = { ...hostCaps, runId: 'newer-job', childTurnId: 'newer-turn' }
      return hostReply(parsed)
    })
    await draw(mapped('task', nativeChild)); await click(row()); await typeChildMessage('Keep my draft'); await sendChildMessage()
    const input = messageInput(); expect(input.value).toBe('Keep my draft'); expect(document.activeElement).toBe(input)
    expect(container.querySelector('button[aria-label="Retry unconfirmed child message"]')).not.toBeNull()
    await sendChildMessage()
    const intents = sentIntents(); expect(intents).toHaveLength(3); expect(intents[2]).toEqual(intents[1])
    expect(input.value).toBe(''); expect(document.activeElement).toBe(input)
  })
  it('uses Host start capability for completed/cold history without changing the old result', async () => {
    const caps = { ...hostCaps, send: 'start', stop: false }
    api.runtimeRequest.mockImplementation(async (_path, _method, body) => {
      const request = childInputRequestSchemaV1.parse(JSON.parse(body) as unknown)
      return hostReply({ schemaVersion: 1, capabilities: caps, ...(request.mode === 'start_or_steer' ? { receipt: {
        action: 'start', admission: 'admitted', clientMessageId: request.clientMessageId,
        jobId: 'job-next', childThreadId: 'child-thread', childTurnId: 'turn-next', messageId: 'message-next'
      } } : {}) })
    })
    const block = mapped('task', { ...nativeChild, childStatus: 'completed', canAcceptSteer: false, heartbeatStatus: 'recovered' })
    const history = JSON.stringify(block)
    await draw(block); await click(row()); await typeChildMessage('New child turn'); await sendChildMessage()
    expect(sentIntents()[1]).toMatchObject({ mode: 'start_or_steer', expectedRunId: 'job-a', expectedChildTurnId: 'child-turn' })
    expect(JSON.stringify(block)).toBe(history); expect(messageInput().value).toBe('')
    expect(container.textContent).toContain('Started a new turn in this child session')
    expect(container.querySelector('button[aria-label="Open child session"]')).not.toBeNull()
  })
  it('keeps a stale-anchor rejection draft and only re-inspects after another explicit submission', async () => {
    api.runtimeRequest.mockImplementation(async (_path, _method, body) => {
      const request = childInputRequestSchemaV1.parse(JSON.parse(body) as unknown)
      if (request.mode === 'inspect') return hostReply({ schemaVersion: 1, capabilities: hostCaps })
      expect(request.expectedRunId).toBe('job-a'); expect(request.expectedChildTurnId).toBe('child-turn')
      return hostReply({ schemaVersion: 1, capabilities: { ...hostCaps, runId: 'changed-job', childTurnId: 'changed-turn' }, code: 'conflict' }, false, 409)
    })
    await draw(mapped('task', nativeChild)); await click(row()); await typeChildMessage('Retain after conflict'); await sendChildMessage()
    expect(messageInput().value).toBe('Retain after conflict'); expect(sentIntents()).toHaveLength(2)
    expect(container.textContent).not.toContain('Started a new turn')
    await sendChildMessage(); expect(sentIntents().map(intent => intent.mode)).toEqual(['inspect', 'start_or_steer', 'inspect', 'start_or_steer'])
  })
  it.each(['cross-parent', 'wrong-child', 'disabled'])('does not submit after %s inspection', async failure => {
    api.runtimeRequest.mockImplementation(async () => failure === 'cross-parent'
      ? hostReply({ schemaVersion: 1, capabilities: { send: 'disabled', stop: false }, code: 'child_input_unknown' }, false, 503)
      : hostReply({ schemaVersion: 1, capabilities: failure === 'disabled'
        ? { ...hostCaps, send: 'disabled', reasonCode: 'paused' }
        : { ...hostCaps, childThreadId: 'other-child' } }))
    await draw(mapped('task', nativeChild)); await click(row()); await typeChildMessage('Keep unavailable draft'); await sendChildMessage()
    expect(messageInput().value).toBe('Keep unavailable draft'); expect(sentIntents()).toHaveLength(1)
  })
  it('retires a late inspection before start and keeps a late receipt from clearing edited draft or focus', async () => {
    let completeInspect!: (value: ReturnType<typeof hostReply>) => void
    api.runtimeRequest.mockImplementationOnce(() => new Promise(resolve => { completeInspect = resolve }))
    const block = mapped('task', nativeChild)
    await draw(block); await click(row()); await typeChildMessage('Old view draft'); await sendChildMessage()
    await act(async () => useChatStore.setState({ activeThreadId: 'other-thread' }))
    await act(async () => useChatStore.setState({ activeThreadId: 'thread' }))
    await act(async () => completeInspect(hostReply({ schemaVersion: 1, capabilities: hostCaps })))
    expect(sentIntents()).toHaveLength(1); expect(messageInput().value).toBe('Old view draft')
    expect(messageInput().closest('form')!.querySelector('button')!.disabled).toBe(false)
    let completeSubmit!: (value: ReturnType<typeof hostReply>) => void
    let request!: Record<string, string>
    api.runtimeRequest.mockImplementation(async (path, method, body) => {
      request = JSON.parse(body)
      if (request.mode === 'inspect') return childInputHostFixture(path, method, body)
      return new Promise(resolve => { completeSubmit = resolve })
    })
    await sendChildMessage(); const submitted = { ...request }; const input = messageInput()
    await typeChildMessage('Newer draft')
    await act(async () => { for (const invalidate of invalidators) invalidate(null) })
    await act(async () => completeSubmit(hostReply({ schemaVersion: 1, capabilities: hostCaps, receipt: {
      action: 'steer', admission: 'queued', clientMessageId: submitted.clientMessageId, jobId: 'job-a',
      childThreadId: 'child-thread', childTurnId: 'child-turn', messageId: 'message-1'
    } })))
    expect(messageInput()).toBe(input); expect(input.value).toBe('Newer draft'); expect(document.activeElement).toBe(input)
  })
})

it('keeps the original key when a confirmed receipt arrives after an A-B-A view retirement', async () => {
  let complete!: (value: ReturnType<typeof hostReply>) => void
  let pendingBody = ''
  api.runtimeRequest.mockImplementation(async (path, method, body) => {
    const request = JSON.parse(body)
    if (request.mode === 'inspect') return childInputHostFixture(path, method, body)
    pendingBody = body
    return new Promise(resolve => { complete = resolve })
  })
  await draw(mapped('task', nativeChild)); await click(row()); await typeChildMessage('Original input'); await sendChildMessage()
  await act(async () => { useChatStore.setState({ activeThreadId: 'other-thread' }) })
  await act(async () => { useChatStore.setState({ activeThreadId: 'thread' }) })
  const receipt = await childInputHostFixture('/v1/runtime/task-jobs/steer', 'POST', pendingBody)
  await act(async () => complete(receipt))
  expect(messageInput().value).toBe('Original input')
  api.runtimeRequest.mockImplementation(childInputHostFixture)
  await sendChildMessage()
  const intents = sentIntents(); expect(intents).toHaveLength(3); expect(intents[2]).toEqual(intents[1])
  expect(messageInput().value).toBe('')
})

it.each(['forbidden', 'input_cancelled'])('reconciles an unknown envelope after later %s without losing draft', async code => {
  let send = 0
  api.runtimeRequest.mockImplementation(async (path, method, body) => {
    const response = await childInputHostFixture(path, method, body)
    if (JSON.parse(body).mode === 'inspect') return response
    send++
    if (send <= 2) return hostReply({ schemaVersion: 1, capabilities: { send: 'disabled', stop: false },
      code: send === 1 ? 'child_input_unknown' : code }, false, send === 1 ? 503 : code === 'forbidden' ? 403 : 409)
    return response
  })
  await draw(mapped('task', nativeChild)); await click(row()); await typeChildMessage('Uncertain original'); await sendChildMessage()
  await sendChildMessage()
  expect(messageInput().value).toBe('Uncertain original')
  expect(Boolean(container.querySelector('button[aria-label="Retry unconfirmed child message"]'))).toBe(code === 'forbidden')
  await sendChildMessage()
  const intents = sentIntents()
  expect(intents[2]).toEqual(intents[1])
  if (code === 'forbidden') {
    expect(intents).toHaveLength(4); expect(intents[3]).toEqual(intents[1])
  } else {
    expect(intents).toHaveLength(5); expect(intents[3].mode).toBe('inspect')
    expect(intents[4].clientMessageId).not.toBe(intents[1].clientMessageId)
  }
  expect(messageInput().value).toBe('')
})
