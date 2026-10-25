// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock, ToolBlock } from '../../../../../agent/types'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import type { UserInputLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import i18n from '../../../../../i18n'
import { useChatStore } from '../../../../../store/chat-store'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { ToolCallTree } from '../ToolCallTree'
import { ToolRow } from '../ToolRow'
import { GateResponseContext } from '../../../message-timeline-bubbles'
import rowCss from './ask-question-row.module.css'
import { presentationPolicyFor } from '../chat-view-model'
import { processActivityFor } from '../ToolViews'
import { ToolRawIO } from './ToolRawIO'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
const requestId = 'input_' + 'a'.repeat(64), callId = 'actual-machine-call'
const selector = { threadId: 'thread-row', turnId: 'turn-source', itemId: 'item_' + requestId, requestId }
const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
const tool: ToolBlock = { kind: 'tool', id: 'actual-tool-item', toolKind: 'tool_call', status: 'running', summary: 'Safe tool summary', meta: { callId, turnId: selector.turnId, sourceItemId: 'actual-tool-item', toolName: 'ask_user_question' } }
const input: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: selector.itemId, requestId, status: 'pending', meta: { turnId: selector.turnId }, questions: [{ id: requestId + '_1', header: 'Projected header', question: 'Projected question', multiSelect: false, options: [] }] }
const source = (): UserInputLocalDisplayV1 => ({ schemaVersion: 1, ...selector, status: 'pending', callId, timedWait: timing, answerState: 'pending', questions: [{ id: requestId + '_1', header: 'Synthetic original header', question: 'Synthetic original question', multiSelect: false, options: [] }] })
const queuedSource = (): UserInputLocalDisplayV1 => ({ ...source(), status: 'continued', answerState: 'available',
  submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic queued answer' }] },
  lateReply: { clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', turnId: 'old-target-turn', state: 'queued' } })
let root: Root, container: HTMLDivElement, display: UserInputLocalDisplayV1
let previous: ReturnType<typeof useChatStore.getState>, previousApi: PropertyDescriptor | undefined
const select = vi.fn(), listeners = new Set<(viewId: string | null) => void>()
const shimmerCopySelector = '[aria-hidden="true"][inert]'
function shimmerText(copy: Element): string {
  return [copy.textContent ?? '', ...[...copy.querySelectorAll('[data-shimmer-text]')].map(node => node.getAttribute('data-shimmer-text') ?? '')].join('\n')
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en')
  previous = useChatStore.getState(); previousApi = Object.getOwnPropertyDescriptor(window, 'analytix'); display = source(); select.mockClear()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    openUserInputLocalDisplay: vi.fn(async () => ({ ok: true, display })), closeUserInputLocalDisplay: vi.fn(async () => undefined),
    onUserInputLocalDisplayInvalidated: (listener: (id: string | null) => void) => { listeners.add(listener); return () => listeners.delete(listener) }, onRuntimeStatus: () => () => undefined
  } } })
  useChatStore.setState({ activeThreadId: selector.threadId, blocks: [tool, input], selectUserInputPanel: select })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); listeners.clear(); useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals(); if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix') })
async function draw(blocks: ChatBlock[] = [tool, input], openGroup = true, props: Partial<Parameters<typeof ToolCallTree>[0]> = {}) {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: selector.threadId }, createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('detailed'), ...props }))))
  if (openGroup) await act(async () => {
    for (const title of container.querySelectorAll<HTMLButtonElement>('button[data-process-activity][aria-expanded="false"]')) title.click()
  })
}
function rowAction(key: string): HTMLButtonElement {
  const row = container.querySelector('[data-question-call-id]')!
  const buttons = [...row.querySelectorAll<HTMLButtonElement>('button')].filter(button => button.textContent === i18n.t('common:' + key))
  expect(buttons).toHaveLength(1)
  expect(buttons[0].closest(shimmerCopySelector)).toBeNull()
  return buttons[0]
}
async function drawQueued() {
  display = queuedSource()
  const blocks: ChatBlock[] = [{ ...tool, status: 'success' }, { ...input, status: 'continued', timedWait: timing }]
  useChatStore.setState({ blocks }); await draw(blocks)
}
describe('actual ask-question row association', () => {
  it.each(['user_input', 'request_user_input', 'ask_user_question'])('dispatches the actual mapper %s call to one Host-associated question row', async toolName => {
    const mapped = chatBlockFromItem({ id: tool.id, threadId: selector.threadId, turnId: selector.turnId, role: 'assistant',
      status: 'running', createdAt: '2026-10-09T00:00:00Z', kind: 'tool_call', toolName, callId, toolKind: 'tool_call',
      arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
        privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } })
    if (mapped?.kind !== 'tool') throw new Error('fixture must reach the actual mapper tool row')
    expect(mapped.meta?.toolName).toBe(toolName); expect(processActivityFor(mapped)).toBe('questions')
    useChatStore.setState({ blocks: [mapped, input] }); await draw([mapped, input])
    expect(container.querySelectorAll('[data-question-call-id]')).toHaveLength(1)
    expect(container.querySelector('[data-gate-request-id]')).toBeNull()
    await act(async () => container.querySelector<HTMLElement>('[data-question-call-id] [data-disclosure-row]')!.click())
    expect(container.textContent).toContain('Synthetic original question')
    expect(container.textContent).not.toContain('Projected question')
    display = { ...source(), callId: 'different-host-call' }
    for (const invalidate of [...listeners]) await act(async () => invalidate(null))
    expect(container.querySelector('[data-question-call-id]')).toBeNull()
    expect(container.querySelector('[data-gate-request-id]')).not.toBeNull()
  })
  it.each([false, true])('expands shared running IN in the single paired row without repeating Respond (capture initially present: %s)', async initiallyPresent => {
    const call = { callId, toolCallItemId: tool.id, startedEventSeq: '12' }
    const raw: ProcessDisplayBlockV1 = { blockId: 'question-raw', kind: 'tool_input', state: 'running', attempt: 0,
      source: { kind: 'tool', unitId: 'question-unit', ...call }, durationMs: 0, truncated: false, classification: 'source_exact_local',
      input: { availability: 'available', inputJSON: ' {"sequence":9007199254740993,"question":"Actual"} ' } }
    let expanded = false
    const setOpen = vi.fn((open: boolean) => { expanded = open })
    const local = { threadId: selector.threadId, turnId: selector.turnId, units: [{ unitId: 'question-unit', producerId: 'producer-question',
      providerSequence: '1', placement: 'anchored' as const, calls: [call], precedingCalls: [] }], identities: [raw], blocks: [raw], render: () => null,
      capture: () => ({ blockId: raw.blockId, preview: null, body: createElement(ToolRawIO, { input: raw.input }), expanded, expandable: true, setOpen }) }
    await draw([tool, input], true, { local: initiallyPresent ? local : undefined })
    expect(container.querySelectorAll('[data-question-call-id]')).toHaveLength(1)
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    const row = container.querySelector<HTMLElement>('[data-question-call-id] [data-disclosure-row]')!
    await act(async () => row.click())
    if (!initiallyPresent) { expect(setOpen).not.toHaveBeenCalled(); await draw([tool, input], true, { local }) }
    expect(setOpen).toHaveBeenLastCalledWith(true)
    await draw([tool, input], true, { local })
    expect(container.querySelector('[data-tool-raw-input]')?.textContent).toBe(raw.input?.availability === 'available' ? raw.input.inputJSON : undefined)
    const calls = setOpen.mock.calls.length
    await act(async () => rowAction('dshRespond').click())
    expect(select).toHaveBeenCalledOnce(); expect(select).toHaveBeenCalledWith(requestId)
    expect(setOpen).toHaveBeenCalledTimes(calls)
    await act(async () => row.click()); expect(setOpen).toHaveBeenLastCalledWith(false)
    await draw([tool, input], true, { local })
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    expect(container.querySelectorAll('[data-question-call-id]')).toHaveLength(1)
  })

  it('clears Question raw expansion intent on generation retirement before a late capture arrives', async () => {
    await draw()
    const row = container.querySelector<HTMLElement>('[data-question-call-id] [data-disclosure-row]')!
    await act(async () => row.click())
    await draw([tool, input], true, { privateGeneration: 1 })
    const call = { callId, toolCallItemId: tool.id, startedEventSeq: '12' }
    const raw: ProcessDisplayBlockV1 = { blockId: 'late-question-raw', kind: 'tool_input', state: 'running', attempt: 0,
      source: { kind: 'tool', unitId: 'question-unit', ...call }, durationMs: 0, truncated: false, classification: 'source_exact_local' }
    const setOpen = vi.fn()
    const local = { threadId: selector.threadId, turnId: selector.turnId, units: [{ unitId: 'question-unit', producerId: 'producer-question',
      providerSequence: '1', placement: 'anchored' as const, calls: [call], precedingCalls: [] }], identities: [raw], blocks: [raw], render: () => null,
      capture: () => ({ blockId: raw.blockId, preview: null, body: 'LATE_QUESTION_RAW', expanded: false, expandable: true, setOpen }) }
    await draw([tool, input], true, { local, privateGeneration: 1 })
    expect(setOpen).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('LATE_QUESTION_RAW')
    expect(container.querySelectorAll('[data-question-call-id]')).toHaveLength(1)
  })
  it('keeps folded process raw leaves closed and reads the real question only after the actual group title opens', async () => {
    await draw([tool, input], false)
    expect(container.querySelector('[data-question-inline-owned]')).toBeNull()
    expect(container.textContent).not.toContain('Synthetic original question')
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-process-activity]')!.click())
    expect(container.querySelector('[data-question-inline-owned="true"]')?.closest('[hidden],[inert]')).toBeNull()
    await act(async () => container.querySelector<HTMLElement>('[data-question-inline-owned="true"] [data-disclosure-row]')!.click())
    expect(container.textContent).toContain('Synthetic original question')
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-process-activity]')!.click())
    expect(container.querySelector('[data-question-inline-owned]')).toBeNull()
    expect(container.textContent).not.toContain('Synthetic original question')
  })

  it('joins only the actual top-level machine callId and removes only its paired gate row', async () => {
    await draw(); expect(container.querySelectorAll('[data-question-call-id]')).toHaveLength(1)
    expect(container.querySelector('[data-gate-request-id]')).toBeNull()
    const row = container.querySelector<HTMLElement>('[data-question-call-id]')!
    expect(row.dataset.questionCallId).toBe(callId)
    expect(row.dataset.questionRequestId).toBe(requestId)
    expect(row.dataset.questionThreadId).toBe(selector.threadId)
    expect(row.dataset.questionInlineOwned).toBe('true')
    const header = row.querySelector('[data-disclosure-row]')!
    const content = [...header.children].find(node => node.classList.contains(rowCss.inlineContent))
    expect(content).toBeDefined()
    const respond = rowAction('dshRespond')
    expect(respond.parentElement?.previousElementSibling).toBe(content)
    expect(content?.contains(respond)).toBe(false)
    await act(async () => respond.click()); expect(select).toHaveBeenCalledOnce(); expect(select).toHaveBeenCalledWith(requestId)
    expect(header.getAttribute('aria-expanded')).toBe('false')
  })
  it('keeps expanded original questions outside an actual running shimmer copy', async () => {
    await draw()
    const row = container.querySelector<HTMLElement>('[data-question-call-id]')!
    await act(async () => row.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
    expect(row.querySelector('li')?.textContent).toContain('Synthetic original question')
    const copies = [...row.querySelectorAll(shimmerCopySelector)]
    expect(copies.length).toBeGreaterThan(0)
    for (const copy of copies) {
      expect(shimmerText(copy)).not.toContain('Synthetic original question')
      expect(shimmerText(copy)).not.toContain('Synthetic original header')
    }
    rowAction('dshRespond'); expect(select).not.toHaveBeenCalled()
  })
  it('keeps the fallback when the same machine anchor exists only in another Tree', async () => {
    await draw([input]); expect(container.querySelector('[data-gate-request-id]')?.getAttribute('data-gate-request-id')).toBe(requestId)
    expect(container.querySelector('[data-question-call-id]')).toBeNull()
  })
  it.each([null, 'other-machine-call'])('does not derive private association from tool name or result prose when callId is %s', async value => {
    display = { ...source(), callId: value }; await draw()
    expect(container.querySelector('[data-question-call-id]')).toBeNull(); expect(container.querySelector('[data-gate-request-id]')).not.toBeNull()
  })
  it('offers a real queued discard action without selecting a full composer or assuming the receipt is current delivery', async () => {
    display = { ...source(), status: 'continued', answerState: 'available', submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic queued answer' }] }, lateReply: { clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', turnId: 'old-target-turn', state: 'queued' } }
    const continued = { ...input, status: 'continued' as const, timedWait: timing }, settledTool = { ...tool, status: 'success' as const }
    let finish!: () => void
    const discard = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    Object.assign(window.analytix.runtime, { discardUserInputLateReply: discard })
    useChatStore.setState({ blocks: [settledTool, continued] }); await draw([settledTool, continued])
    const button = () => [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === i18n.t('common:userInputLateDiscard'))!
    await act(async () => { button().click(); button().click() }); expect(discard).toHaveBeenCalledOnce()
    expect(discard).toHaveBeenCalledWith({ ...selector, action: 'discardLateReply', clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', expectedTurnId: 'old-target-turn' })
    await act(async () => finish())
    expect(container.textContent).toContain(i18n.t('common:userInputLateQueued')); expect(select).not.toHaveBeenCalled()
    expect(button().disabled).toBe(false)
  })
  it('renders a queued compact transcript privately and keeps queued distinct from accepted', async () => {
    display = { ...source(), status: 'continued', answerState: 'available', submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic queued answer' }] }, lateReply: { clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', turnId: 'different-current-turn', state: 'queued' } }
    const continued = { ...input, status: 'continued' as const, timedWait: timing }, settledTool = { ...tool, status: 'success' as const }
    useChatStore.setState({ blocks: [settledTool, continued] }); await draw([settledTool, continued])
    expect(container.textContent).toContain(i18n.t('common:userInputLateQueued'))
    const review = rowAction('userInputReview'); rowAction('userInputLateDiscard')
    await act(async () => review.click()); expect(container.querySelector('dl')?.textContent).toContain('Synthetic queued answer')
    expect(select).not.toHaveBeenCalled()
    expect(container.querySelector('textarea')).toBeNull(); expect(container.querySelector('[data-question-reply-item-id]')).toBeNull()
    expect(container.querySelectorAll(shimmerCopySelector)).toHaveLength(0)
    expect([...container.querySelectorAll(shimmerCopySelector)].some(node => shimmerText(node).includes('Synthetic queued answer'))).toBe(false)
    const text = container.querySelector('dt')!.firstChild!, range = document.createRange(); range.selectNodeContents(text); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    expect(document.getSelection()!.rangeCount).toBe(1)
    expect(document.getSelection()!.isCollapsed).toBe(false)
    expect(document.getSelection()!.toString()).toContain('Synthetic original')
    expect(document.getSelection()!.getRangeAt(0).intersectsNode(container.querySelector('[data-question-inline-owned="true"]')!)).toBe(true)
    const copy = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(copy); expect(copy.defaultPrevented).toBe(false)
    document.getSelection()?.removeAllRanges()
  })
  it('keeps a secondary queued row navigation-only without publishing an owned inline entry', async () => {
    display = queuedSource()
    const blocks: ChatBlock[] = [{ ...tool, status: 'success' }, { ...input, status: 'continued', timedWait: timing }]
    useChatStore.setState({ blocks })
    const owner = vi.fn()
    await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: selector.threadId },
      createElement(GateResponseContext.Provider, { value: { onOpenOwnerThread: owner } },
        createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('detailed') })))))
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-process-activity][aria-expanded="false"]')!.click())
    const row = container.querySelector<HTMLElement>('[data-question-call-id]')!
    expect(row.dataset.questionCallId).toBe(callId); expect(row.dataset.questionRequestId).toBe(requestId)
    expect(row.dataset.questionThreadId).toBe(selector.threadId); expect(row.hasAttribute('data-question-inline-owned')).toBe(false)
    expect([...row.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:userInputLateDiscard'))).toBe(false)
    await act(async () => rowAction('userInputReview').click())
    expect(owner).toHaveBeenCalledWith(requestId); expect(select).not.toHaveBeenCalled(); expect(row.querySelector('dl')).toBeNull()
  })
  it('shows a failed closed-row discard and rereads Host without making the queued reply answerable', async () => {
    const discard = vi.fn(async () => { throw new Error('Synthetic discard failure') })
    Object.assign(window.analytix.runtime, { discardUserInputLateReply: discard })
    await drawQueued()
    const open = vi.mocked(window.analytix.runtime.openUserInputLocalDisplay), calls = open.mock.calls.length
    await act(async () => rowAction('userInputLateDiscard').click())
    expect(discard).toHaveBeenCalledOnce(); expect(open.mock.calls.length).toBeGreaterThan(calls)
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(i18n.t('common:userInputLateDiscardFailed'))
    expect(container.textContent).toContain(i18n.t('common:userInputLateQueued')); expect(container.querySelector('dl')?.textContent).toContain('Synthetic queued answer')
    expect([...container.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:dshRespond'))).toBe(false)
    expect(rowAction('userInputLateDiscard').disabled).toBe(false); expect(select).not.toHaveBeenCalled()
  })
  it.each(['discarded', 'accepted'] as const)('uses Host %s readback after discard instead of treating the RPC acknowledgement as delivery', async state => {
    let finish!: () => void
    Object.assign(window.analytix.runtime, { discardUserInputLateReply: vi.fn(() => new Promise<void>(resolve => { finish = resolve })) })
    await drawQueued(); await act(async () => rowAction('userInputLateDiscard').click())
    display = { ...display, lateReply: { ...display.lateReply!, state } }
    await act(async () => finish())
    expect([...container.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:userInputLateDiscard'))).toBe(false)
    expect(select).not.toHaveBeenCalled(); expect(container.querySelector('[data-question-reply-item-id]')).toBeNull()
    if (state === 'discarded') {
      await act(async () => rowAction('dshRespond').click()); expect(select).toHaveBeenCalledWith(requestId)
    } else {
      rowAction('userInputReview')
      expect([...container.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:dshRespond'))).toBe(false)
    }
  })
  it.each(['thread change', 'privacy revoke'] as const)('does not reopen the former owner from a delayed discard after %s', async reason => {
    let finish!: () => void
    Object.assign(window.analytix.runtime, { discardUserInputLateReply: vi.fn(() => new Promise<void>(resolve => { finish = resolve })) })
    await drawQueued(); await act(async () => rowAction('userInputLateDiscard').click())
    await act(async () => {
      if (reason === 'thread change') useChatStore.setState({ activeThreadId: 'thread-other', blocks: [] })
      else listeners.forEach(listener => listener(null))
    })
    const open = vi.mocked(window.analytix.runtime.openUserInputLocalDisplay), calls = open.mock.calls.length
    await act(async () => finish())
    expect(open).toHaveBeenCalledTimes(calls); expect(select).not.toHaveBeenCalled()
    expect(container.querySelector('[data-question-call-id]')).toBeNull(); expect(container.textContent).not.toContain('Synthetic queued answer')
  })
})

describe('ordinary ToolRow default sizing', () => {
  it.each([
    { running: false, error: false, expected: 'ok' },
    { running: true, error: false, expected: 'running' },
    { running: false, error: true, expected: 'error' }
  ])('preserves the default $expected row without the question-only sizing class', async ({ running, error, expected }) => {
    const action = vi.fn()
    await act(async () => root.render(createElement(ToolRow, { title: 'Synthetic ordinary tool', summary: 'Synthetic ordinary summary', toolName: 'shell', running, error,
      actions: createElement('button', { type: 'button', onClick: action }, 'Synthetic ordinary action') })))
    const row = container.querySelector<HTMLElement>('[data-tool="shell"]')!
    expect(row.dataset.state).toBe(expected)
    expect(row.querySelectorAll(shimmerCopySelector)).toHaveLength(running ? 1 : 0)
    expect([...row.querySelectorAll('*')].some(node => node.classList.contains(rowCss.inlineContent))).toBe(false)
    const buttons = [...row.querySelectorAll<HTMLButtonElement>('button')].filter(button => button.textContent === 'Synthetic ordinary action')
    expect(buttons).toHaveLength(1); expect(buttons[0].closest(shimmerCopySelector)).toBeNull()
    await act(async () => buttons[0].click()); expect(action).toHaveBeenCalledOnce(); expect(row.dataset.state).toBe(expected)
  })
})

describe('settled Question row actions', () => {
  it('selects the full readonly review through actual store metadata', async () => {
    const queued = queuedSource()
    display = { ...queued, status: 'submitted', lateReply: null }
    const blocks: ChatBlock[] = [{ ...tool, status: 'success' }, { ...input, status: 'submitted' }]
    useChatStore.setState({ blocks }); await draw(blocks)
    await act(async () => rowAction('userInputReview').click())
    expect(select).toHaveBeenCalledWith(requestId, 'review')
  })
  it('shows no Review/Respond action on a cancelled question', async () => {
    display = { ...source(), status: 'cancelled', answerState: 'cancelled' }
    const blocks: ChatBlock[] = [{ ...tool, status: 'success' }, { ...input, status: 'cancelled' }]
    useChatStore.setState({ blocks }); await draw(blocks)
    const row = container.querySelector('[data-question-call-id]')!
    expect([...row.querySelectorAll('button')].some(button => [i18n.t('common:userInputReview'), i18n.t('common:dshRespond')].includes(button.textContent ?? ''))).toBe(false)
  })
})


it.each(['cancelled', 'submitted', 'continued'] as const)('offers only full Review for actual Host readOnly %s with the retained machine association', async status => {
  display = { ...source(), readOnly: true, status, answerState: status === 'cancelled' ? 'cancelled' : 'unavailable',
    timedWait: null, continuedReason: null, draftRevision: null, draft: null, lateReply: null }
  const discard = vi.fn(); Object.assign(window.analytix.runtime, { discardUserInputLateReply: discard })
  const blocks: ChatBlock[] = [{ ...tool, status: 'success' }, { ...input, status }]
  useChatStore.setState({ blocks }); await draw(blocks)
  const row = container.querySelector('[data-question-call-id]')!
  expect(row.getAttribute('data-question-call-id')).toBe(callId)
  expect([...row.querySelectorAll('button')].some(button => [i18n.t('common:dshRespond'), i18n.t('common:userInputLateDiscard')].includes(button.textContent ?? ''))).toBe(false)
  await act(async () => rowAction('userInputReview').click())
  expect(select).toHaveBeenCalledWith(requestId, 'review'); expect(discard).not.toHaveBeenCalled()
  expect(container.querySelector('textarea')).toBeNull()
})
