// @vitest-environment jsdom
import { act, createElement, StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  UserInputLocalSelectorV1, UserInputLocalDisplayV1, UserInputLocalViewRequestV1, UserInputLocalViewResultV1
} from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { useUserInputLocalDisplay, useUserInputLocalAssociation } from './useUserInputLocalDisplay'

const context = vi.hoisted(() => ({ pending: false, listeners: new Set<() => void>() }))
vi.mock('../../../../data-analysis/services/analysis/stats-case-overview-resource', () => ({
  sharedActiveCaseContextTransitionPending: () => context.pending,
  subscribeBeforeSharedActiveCaseContextChange: (listener: () => void) => {
    context.listeners.add(listener); return () => context.listeners.delete(listener)
  }
}))
const requestId = 'input_' + 'a'.repeat(64)
const selector: UserInputLocalSelectorV1 = { threadId: 'thread-a', turnId: 'turn-a', itemId: 'item_' + requestId, requestId }
function display(target = selector): UserInputLocalDisplayV1 {
  return { schemaVersion: 1, ...target, status: 'pending', answerState: 'pending', questions: [{
    id: target.requestId + '_1', header: 'Synthetic original header', question: 'Synthetic original question',
    detail: '**Synthetic original detail**', multiSelect: false,
    options: [{ id: target.requestId + '_1_option_1', label: 'Synthetic original option', description: 'Synthetic original description' }]
  }] }
}
let invalidate: ((id: string | null) => void) | undefined
const sseListeners = new Set<(value: { events: Record<string, unknown>[] }) => void>()
let runtimeStatus: ((value: { state: string }) => void) | undefined
function makeApi() {
  const invalidationListeners = new Set<(id: string | null) => void>()
  invalidate = id => { for (const listener of [...invalidationListeners]) listener(id) }
  return {
    openUserInputLocalDisplay: vi.fn(async (_request: UserInputLocalViewRequestV1): Promise<UserInputLocalViewResultV1> => ({ ok: true, display: display() })),
    closeUserInputLocalDisplay: vi.fn(async (_id: string) => undefined),
    onUserInputLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidationListeners.add(listener); return () => { invalidationListeners.delete(listener) } }),
    onRuntimeStatus: vi.fn((listener: (value: { state: string }) => void) => { runtimeStatus = listener; return () => { runtimeStatus = undefined } }),
    onSseEvent: vi.fn((listener: (value: { events: Record<string, unknown>[] }) => void) => { sseListeners.add(listener); return () => sseListeners.delete(listener) }),
    runtimeRequest: vi.fn()
  }
}
let root: Root, container: HTMLDivElement, api: ReturnType<typeof makeApi>
let previousApi: PropertyDescriptor | undefined
let current: ReturnType<typeof useUserInputLocalDisplay>
function Probe({ target = selector, status = 'pending' }: { target?: UserInputLocalSelectorV1 | null; status?: string }) {
  current = useUserInputLocalDisplay(target, status)
  return createElement('p', { 'data-phase': current.phase }, current.display?.questions[0].question ?? '')
}
async function render(target: UserInputLocalSelectorV1 | null = selector, status = 'pending', strict = false) {
  const node = createElement(Probe, { target, status })
  await act(async () => root.render(strict ? createElement(StrictMode, null, node) : node))
}
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  context.pending = false; context.listeners.clear(); sseListeners.clear(); invalidate = undefined; runtimeStatus = undefined
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  api = makeApi(); Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi)
  else Reflect.deleteProperty(window, 'analytix')
})


function continuedDisplay(): UserInputLocalDisplayV1 {
  return { ...display(), status: 'continued', answerState: 'unavailable',
    timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }, continuedReason: 'user_input_continued' }
}
function completedUserNotice(kind = 'item_created'): Record<string, unknown> {
  return { kind, threadId: selector.threadId, turnId: 'turn-idle-wake', itemId: 'late-user-item',
    item: { kind: 'user_message', role: 'user', status: 'completed' } }
}
async function broadcast(event: Record<string, unknown>) {
  await act(async () => { for (const listener of [...sseListeners]) listener({ events: [event] }) })
}

describe('private question leaf lease', () => {
  it('opens the exact complete Core selector and closes only its own view', async () => {
    await render()
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledWith({ ...selector, viewId: expect.any(String) })
    expect(current.phase).toBe('ready'); expect(current.display).toEqual(display())
    expect(container.textContent).toBe('Synthetic original question')
    const id = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    await act(async () => root.unmount())
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalledWith(id)
    expect(api.runtimeRequest).not.toHaveBeenCalled()
  })

  it('requires the dedicated bridge and a valid complete pair without a generic fallback', async () => {
    await render(null)
    expect(current.phase).toBe('unavailable'); expect(api.openUserInputLocalDisplay).not.toHaveBeenCalled()
    await render({ ...selector, itemId: 'item_' + 'input_' + 'b'.repeat(64) })
    expect(current.display).toBeNull(); expect(api.openUserInputLocalDisplay).not.toHaveBeenCalled()
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { runtimeRequest: api.runtimeRequest } } })
    await render()
    expect(current.phase).toBe('unavailable'); expect(api.runtimeRequest).not.toHaveBeenCalled()
  })

  it.each(['thread', 'turn', 'pair'])('rejects a valid DTO from another %s binding', async kind => {
    const otherRequest = 'input_' + 'b'.repeat(64)
    const other = kind === 'thread' ? { ...selector, threadId: 'thread-b' }
      : kind === 'turn' ? { ...selector, turnId: 'turn-b' }
        : { ...selector, requestId: otherRequest, itemId: 'item_' + otherRequest }
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: display(other) })
    await render()
    expect(current.phase).toBe('unavailable'); expect(current.display).toBeNull()
    expect(container.textContent).toBe(''); expect(api.closeUserInputLocalDisplay).toHaveBeenCalled()
  })

  it('rejects additional fields and inconsistent original answer identities', async () => {
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display(), extra: 'Synthetic forbidden field' } } as never)
    await render(); expect(current.display).toBeNull()
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display(), status: 'submitted', answerState: 'available',
      submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [requestId + '_1_option_2'] }] }
    } } as never)
    await act(async () => current.refresh())
    expect(current.display).toBeNull(); expect(current.phase).toBe('unavailable')
  })

  it('drops the previous thread immediately and ignores its late response', async () => {
    let finish!: (value: UserInputLocalViewResultV1) => void
    api.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render(); const oldId = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    const next = { ...selector, threadId: 'thread-b' }
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: display(next) })
    await render(next)
    expect(current.display?.threadId).toBe('thread-b')
    await act(async () => finish({ ok: true, display: display() }))
    expect(current.display?.threadId).toBe('thread-b')
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalledWith(oldId)
  })

  it('ignores the retired pending read and refuses pending data after a terminal status', async () => {
    let finish!: (value: UserInputLocalViewResultV1) => void
    api.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render()
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display(), status: 'submitted', answerState: 'unavailable' } })
    await render(selector, 'submitted')
    await act(async () => finish({ ok: true, display: display() }))
    expect(current.display?.status).toBe('submitted')
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: display() })
    await act(async () => current.refresh())
    expect(current.phase).toBe('unavailable'); expect(current.display).toBeNull()
  })

  it('retires matching invalidation without reopening and rejects an old action refresh', async () => {
    await render(); const id = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    const revision = current.revision
    await act(async () => invalidate?.('another-view'))
    expect(current.display).not.toBeNull()
    await act(async () => invalidate?.(id))
    expect(current.display).toBeNull(); expect(api.openUserInputLocalDisplay).toHaveBeenCalledOnce()
    await act(async () => current.refresh(revision))
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledOnce()
    await act(async () => current.refresh())
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledTimes(2); expect(current.phase).toBe('ready')
    await act(async () => invalidate?.(null))
    expect(current.display).toBeNull(); expect(api.openUserInputLocalDisplay).toHaveBeenCalledTimes(2)
  })

  it.each(['hidden', 'runtime', 'case'])('retires raw state and its in-flight response on %s without automatic reopening', async kind => {
    let finish!: (value: UserInputLocalViewResultV1) => void
    api.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render()
    await act(async () => {
      if (kind === 'hidden') { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) }
      if (kind === 'runtime') runtimeStatus?.({ state: 'stopped' })
      if (kind === 'case') { context.pending = true; for (const listener of context.listeners) listener() }
    })
    await act(async () => finish({ ok: true, display: display() }))
    expect(current.display).toBeNull(); expect(container.textContent).toBe('')
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledOnce()
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalled()
  })

  it('keeps only the current StrictMode lease when the first setup resolves last', async () => {
    const completions: ((value: UserInputLocalViewResultV1) => void)[] = []
    api.openUserInputLocalDisplay.mockImplementation(() => new Promise(resolve => completions.push(resolve)))
    await render(selector, 'pending', true)
    expect(completions).toHaveLength(2)
    await act(async () => completions[1]({ ok: true, display: display() }))
    expect(current.display).not.toBeNull()
    await act(async () => completions[0]({ ok: true, display: { ...display(), questions: [{ ...display().questions[0], question: 'Synthetic obsolete question' }] } }))
    expect(container.textContent).toBe('Synthetic original question')
    const firstId = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalledWith(firstId)
  })

  it.each(['item_created', 'item_updated', 'item_completed', 'general_terminal_batch', 'accepted_final_batch'])('rereads continued Host display on %s metadata even when promotion uses a new turn', async kind => {
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: continuedDisplay() })
    await render(selector, 'continued')
    const oldView = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    const fresh = { ...continuedDisplay(), questions: [{ ...display().questions[0], question: 'Synthetic fresh Host display' }] }
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: fresh })
    await broadcast(kind.endsWith('_batch') ? { kind, threadId: selector.threadId, turnId: 'turn-idle-wake' } : completedUserNotice(kind))
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledTimes(2)
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalledWith(oldView)
    expect(current.display).toEqual(fresh)
    expect(container.textContent).toBe('Synthetic fresh Host display')
    expect(api.runtimeRequest).not.toHaveBeenCalled()
  })

  it.each([
    { ...completedUserNotice(), threadId: 'another-thread' },
    { ...completedUserNotice(), kind: 'item_started' },
    { ...completedUserNotice(), item: { kind: 'assistant_message', role: 'assistant', status: 'completed' } },
    { ...completedUserNotice(), item: { kind: 'user_message', role: 'assistant', status: 'completed' } },
    { ...completedUserNotice(), item: { kind: 'user_message', role: 'user', status: 'in_progress' } },
    { ...completedUserNotice(), item: null }
  ])('ignores a nonmatching or incomplete promotion notification %#', async event => {
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: continuedDisplay() })
    await render(selector, 'continued')
    await broadcast(event)
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledOnce()
    expect(current.display).toEqual(continuedDisplay())
  })

  it.each(['invalidation', 'hidden', 'runtime', 'case'])('does not grant authority through notifications after %s retirement', async kind => {
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: continuedDisplay() })
    await render(selector, 'continued')
    await act(async () => {
      if (kind === 'invalidation') invalidate?.(null)
      if (kind === 'hidden') { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) }
      if (kind === 'runtime') runtimeStatus?.({ state: 'stopped' })
      if (kind === 'case') { context.pending = true; for (const listener of context.listeners) listener() }
    })
    await broadcast(completedUserNotice())
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledOnce()
    expect(current.display).toBeNull()
  })

  it('retires notification reads on selector changes and removes the old listener', async () => {
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: continuedDisplay() })
    await render(selector, 'continued')
    let finish!: (value: UserInputLocalViewResultV1) => void
    api.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await broadcast(completedUserNotice())
    const next = { ...selector, threadId: 'next-thread' }
    const nextDisplay = { ...continuedDisplay(), ...next }
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: nextDisplay })
    await render(next, 'continued')
    await act(async () => finish({ ok: true, display: continuedDisplay() }))
    expect(current.display).toEqual(nextDisplay)
    expect(sseListeners.size).toBe(1)
    await act(async () => root.unmount())
    expect(sseListeners.size).toBe(0)
  })
})


describe('frozen inherited Question private display', () => {
  function inherited(status: 'submitted' | 'cancelled' | 'continued'): UserInputLocalDisplayV1 {
    return { ...display(), readOnly: true, status, answerState: status === 'cancelled' ? 'cancelled' : 'unavailable',
      timedWait: null, continuedReason: null, draftRevision: null, draft: null, lateReply: null }
  }
  it.each(['submitted', 'cancelled', 'continued'] as const)('admits exact readOnly %s without copying it into public state', async status => {
    const value = inherited(status)
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: value })
    await render(selector, status)
    expect(current.display).toEqual(value); expect(current.phase).toBe('ready')
    expect(api.runtimeRequest).not.toHaveBeenCalled()
  })
  it.each([
    { readOnly: false }, { readOnly: null }, { status: 'pending', answerState: 'pending' },
    { draftRevision: '11111111-1111-4111-8111-111111111111', draft: { currentPage: 0, answers: [] } },
    { lateReply: { clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', turnId: 'other-turn', state: 'queued' } }
  ])('rejects invalid history authority %j through the real private leaf parser', async amendment => {
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: { ...inherited('continued'), ...amendment } } as never)
    await render(selector, 'continued')
    expect(current.display).toBeNull(); expect(current.phase).toBe('unavailable')
    expect(api.runtimeRequest).not.toHaveBeenCalled()
  })
})


it.each(['thread', 'turn'] as const)('rejects inherited history returned for the wrong child %s tuple', async field => {
  const child = { ...selector, threadId: 'child-thread', turnId: 'child-seed-turn' }
  const foreign = { ...child, [field === 'thread' ? 'threadId' : 'turnId']: 'other-child-owner' }
  api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: { ...display(foreign), readOnly: true,
    status: 'continued', answerState: 'unavailable', timedWait: null, continuedReason: null, draftRevision: null, draft: null, lateReply: null } })
  await render(child, 'continued')
  expect(current.display).toBeNull(); expect(current.phase).toBe('unavailable')
  expect(api.closeUserInputLocalDisplay).toHaveBeenCalled(); expect(api.runtimeRequest).not.toHaveBeenCalled()
})


it('publishes only the exact historical reply IDs after confirmed close', async () => {
 const reply = { itemId: 'actual-child-reply', clientUserMessageId: '00000000-0000-4000-8000-000000000001', turnId: 'actual-reply-turn' }
 const history: UserInputLocalDisplayV1 = { ...display(), readOnly: true, callId: 'actual-call', status: 'continued', answerState: 'available',
   historyReply: reply, submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'skip' }] } }
 api.openUserInputLocalDisplay.mockResolvedValue({ ok: true, display: history })
 let finishClose!: () => void
 api.closeUserInputLocalDisplay.mockImplementationOnce(() => new Promise<undefined>(resolve => { finishClose = () => resolve(undefined) }))
 let ids: string[] = []
 function AssociationProbe() {
   ids = useUserInputLocalAssociation(selector, 'continued', [
     { kind: 'reply', id: reply.itemId, clientUserMessageId: reply.clientUserMessageId, turnId: reply.turnId },
     { kind: 'reply', id: 'wrong-item', clientUserMessageId: reply.clientUserMessageId, turnId: reply.turnId },
     { kind: 'reply', id: 'wrong-turn', clientUserMessageId: reply.clientUserMessageId, turnId: 'wrong-turn' },
     { kind: 'reply', id: 'wrong-attempt', clientUserMessageId: '00000000-0000-4000-8000-000000000002', turnId: reply.turnId }
   ])
   return createElement('p', null, ids.join(','))
 }
 await act(async () => root.render(createElement(AssociationProbe)))
 expect(ids).toEqual([])
 await act(async () => finishClose())
 expect(ids).toEqual([reply.itemId]); expect(api.runtimeRequest).not.toHaveBeenCalled()
})
