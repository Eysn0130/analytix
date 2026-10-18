// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock } from '../../../../agent/types'
import type { UserInputLocalDisplayV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { isClosedPublicRuntimeSseEvent } from '../../../../../../shared/public-runtime-sse'
import i18n from '../../../../i18n'
import { QuestionReplyView, userInputReplyMatches } from './QuestionReplyView'
const requestId = 'input_' + 'b'.repeat(64), clientUserMessageId = '67eeac24-98da-4a49-a4ee-1d5b54574999'
const selector = { threadId: 'thread-reply', turnId: 'turn-original-question', itemId: 'item_' + requestId, requestId }
const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
const original = (): UserInputLocalDisplayV1 => ({ schemaVersion: 1, ...selector, status: 'continued', callId: 'actual-call', timedWait: timing, answerState: 'available',
  questions: [{ id: requestId + '_1', header: 'Synthetic header', question: 'Synthetic original question', multiSelect: false, options: [] }],
  submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic original reply' }] },
  lateReply: { clientUserMessageId, turnId: 'turn-steering-target', state: 'accepted' } })
const input: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: selector.itemId, requestId, status: 'continued', timedWait: timing, questions: [], meta: { turnId: selector.turnId } }
const user: Extract<ChatBlock, { kind: 'user' }> = { kind: 'user', id: 'actual-host-user-item', text: 'Safe public fallback', meta: { clientUserMessageId, turnId: 'turn-steering-target', steeringStatus: 'accepted' } }
let root: Root, container: HTMLDivElement, previousApi: PropertyDescriptor | undefined, display: UserInputLocalDisplayV1
const invalidators = new Set<(viewId: string | null) => void>()
type SseListener = Parameters<typeof window.analytix.runtime.onSseEvent>[0]
const events = new Set<SseListener>()
const openDisplay = vi.fn(async (_request: Parameters<typeof window.analytix.runtime.openUserInputLocalDisplay>[0]) => ({ ok: true as const, display }))
const onSseEvent = vi.fn((listener: SseListener) => { events.add(listener); return () => { events.delete(listener) } })
function promotionEvent(block: Extract<ChatBlock, { kind: 'user' }>): Parameters<SseListener>[0]['events'][number] {
  const turnId = String(block.meta?.turnId), timestamp = '2026-10-09T00:00:00Z'
  return { kind: 'item_created', seq: 17, timestamp, threadId: selector.threadId, turnId, itemId: block.id,
    item: { id: block.id, threadId: selector.threadId, turnId, role: 'user', status: 'completed', kind: 'user_message',
      text: block.text, clientUserMessageId, delivery: 'steer', createdAt: timestamp } }
}
async function emitPromotion(block: Extract<ChatBlock, { kind: 'user' }>) {
  const event = promotionEvent(block)
  // The bridge is synthetic; the real closed SSE validator and mounted leaves run.
  // This is not a Main/Go native delivery witness.
  expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
  expect(events.size).toBeGreaterThan(0)
  await act(async () => {
    for (const listener of [...events]) listener({ streamId: 'question-reply-promotion', events: [event] })
    for (let index = 0; index < 20; index++) await Promise.resolve()
  })
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); previousApi = Object.getOwnPropertyDescriptor(window, 'analytix'); display = original()
  openDisplay.mockReset(); openDisplay.mockImplementation(async () => ({ ok: true as const, display })); onSseEvent.mockClear()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { openUserInputLocalDisplay: openDisplay, closeUserInputLocalDisplay: vi.fn(async () => undefined), onUserInputLocalDisplayInvalidated: (listener: (id: string | null) => void) => { invalidators.add(listener); return () => { invalidators.delete(listener) } }, onSseEvent, onRuntimeStatus: () => () => undefined } } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); expect(events.size).toBe(0); container.remove(); events.clear(); invalidators.clear(); document.getSelection()?.removeAllRanges(); vi.restoreAllMocks(); vi.unstubAllGlobals(); if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix') })
async function draw(block = user) { await act(async () => root.render(createElement(QuestionReplyView, { block, requests: [input], threadId: selector.threadId, fallback: createElement('span', null, block.text) }))) }
describe('actual accepted Host question-reply consumer', () => {
  it('uses the actual public user item ID and exact accepted UUID/target association', async () => {
    expect(userInputReplyMatches(display, user)).toBe(true); await draw()
    expect(container.querySelector('[data-question-reply-item-id]')?.getAttribute('data-question-reply-item-id')).toBe(user.id)
    expect(container.textContent).toContain('Synthetic original reply'); expect(container.textContent).not.toContain(user.text)
    expect(user.meta?.turnId).not.toBe(selector.turnId)
  })
  it.each(['queued', 'discarded'] as const)('never manufactures a reply bubble for %s', async state => {
    display = { ...display, lateReply: { ...display.lateReply!, state } }; expect(userInputReplyMatches(display, user)).toBe(false); await draw()
    expect(container.querySelector('[data-question-reply-item-id]')).toBeNull(); expect(container.textContent).toBe(user.text)
  })
  it('rereads queued association only after completed user_message item_created, preserving the Host delivery UUID and new turn', async () => {
    const promoted = { ...user, meta: { ...user.meta, turnId: 'turn-host-promoted-reply' } }
    display = { ...display, lateReply: { ...display.lateReply!, state: 'queued' } }
    await draw(promoted)
    expect(container.querySelector('[data-question-reply-item-id]')).toBeNull(); expect(container.textContent).toBe(user.text)
    const reads = openDisplay.mock.calls.length
    display = { ...display, lateReply: { clientUserMessageId, turnId: promoted.meta.turnId, state: 'accepted' } }
    expect(openDisplay).toHaveBeenCalledTimes(reads)
    expect(container.querySelector('[data-question-reply-item-id]')).toBeNull()
    await emitPromotion(promoted)
    expect(openDisplay.mock.calls.length).toBeGreaterThan(reads)
    expect(openDisplay.mock.calls.every(([request]) => request.threadId === selector.threadId && request.turnId === selector.turnId && request.itemId === selector.itemId && request.requestId === requestId)).toBe(true)
    expect(container.querySelector('[data-question-reply-item-id]')?.getAttribute('data-question-reply-item-id')).toBe(promoted.id)
    expect(container.textContent).toContain('Synthetic original reply'); expect(container.textContent).not.toContain(user.text)
    expect(display.lateReply?.clientUserMessageId).toBe(clientUserMessageId)
    expect(display.lateReply?.turnId).not.toBe(selector.turnId)
  })
  it('does not regrant a revoked queued association when a valid promotion notification arrives', async () => {
    display = { ...display, lateReply: { ...display.lateReply!, state: 'queued' } }; await draw()
    await act(async () => [...invalidators].forEach(listener => listener(null)))
    const reads = openDisplay.mock.calls.length
    display = original(); await emitPromotion(user)
    expect(openDisplay).toHaveBeenCalledTimes(reads)
    expect(container.querySelector('[data-question-reply-item-id]')).toBeNull(); expect(container.textContent).toBe(user.text)
    expect(container.textContent).not.toContain('Synthetic original reply')
  })
  it.each(['hidden', 'owner switch'] as const)('does not publish a late accepted private read after %s', async retirement => {
    display = { ...display, lateReply: { ...display.lateReply!, state: 'queued' } }; await draw()
    let finish!: (result: { ok: true; display: UserInputLocalDisplayV1 }) => void
    const pending = new Promise<{ ok: true; display: UserInputLocalDisplayV1 }>(resolve => { finish = resolve })
    openDisplay.mockImplementationOnce(() => pending)
    display = original()
    const reads = openDisplay.mock.calls.length
    try {
      await emitPromotion(user)
      expect(openDisplay).toHaveBeenCalledTimes(reads + 1)
      await act(async () => {
        if (retirement === 'hidden') {
          vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
          document.dispatchEvent(new Event('visibilitychange'))
        } else root.render(createElement(QuestionReplyView, { block: user, requests: [], threadId: 'thread-other-owner', fallback: createElement('span', null, 'Other owner safe fallback') }))
      })
      await act(async () => { finish({ ok: true, display }); for (let index = 0; index < 20; index++) await Promise.resolve() })
      expect(container.querySelector('[data-question-reply-item-id]')).toBeNull()
      expect(container.textContent).not.toContain('Synthetic original reply')
      expect(container.textContent).toBe(retirement === 'hidden' ? user.text : 'Other owner safe fallback')
      expect(openDisplay).toHaveBeenCalledTimes(reads + 1)
    } finally { await act(async () => finish({ ok: true, display })) }
  })
  it.each([{ clientUserMessageId: 'other-uuid' }, { turnId: 'other-turn' }, { steeringStatus: 'admitted' as const }])('does not replace a public user item with unrelated private capture %j', fields => {
    expect(userInputReplyMatches(display, { ...user, meta: { ...user.meta, ...fields } })).toBe(false)
  })
  it('retires raw display on invalidation and restores only the ordinary safe fallback', async () => {
    await draw(); await act(async () => invalidators.forEach(listener => listener(null)))
    expect(container.querySelector('[data-question-reply-item-id]')).toBeNull(); expect(container.textContent).toBe(user.text)
    expect(container.textContent).not.toContain('Synthetic original reply')
  })
  it('allows authorized local selection without decorative raw twins', async () => {
    await draw(); await act(async () => container.querySelector('button')!.click())
    const text = container.querySelector('[data-question-reply-item-id] dt')!.lastChild!, range = document.createRange(); range.selectNodeContents(text); document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(copy); expect(copy.defaultPrevented).toBe(false)
    expect(container.querySelector('[data-shimmer-twin]')).toBeNull()
  })
})

describe('Reply real local operations and semantic state', () => {
  it('sends only bound native Copy metadata and confirms only its acknowledgement', async () => {
    let finish!: (value: { ok: boolean }) => void
    const effect = vi.fn((_request: unknown) => new Promise<{ ok: boolean }>(resolve => { finish = resolve }))
    Object.assign(window.analytix!.runtime, { performUserInputLocalEffect: effect })
    await draw()
    expect(container.querySelector('[role="group"]')?.getAttribute('data-reply-outcome')).toBe('answered')
    const copy = container.querySelector<HTMLButtonElement>('[aria-label="Copy"]')!
    expect(copy).not.toBeNull()
    await act(async () => copy.click())
    expect(effect).toHaveBeenCalledOnce()
    expect(effect.mock.calls[0][0]).toMatchObject({ selector, viewId: expect.any(String), effect: {
      kind: 'copyReply', locale: 'en', clientUserMessageId, expectedTurnId: 'turn-steering-target'
    } })
    expect(JSON.stringify(effect.mock.calls[0])).not.toContain('Synthetic original reply')
    expect(container.querySelector('[aria-label="Copied"]')).toBeNull()
    await act(async () => finish({ ok: true }))
    expect(container.querySelector('[aria-label="Copied"]')).not.toBeNull()
  })
  it('trims custom presentation without changing the original capture', async () => {
    display.submission = { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: '  custom answer  ' }] }
    await draw(); expect(container.querySelector('[data-question-reply-item-id]')?.textContent).toContain('custom answer')
    expect(container.querySelector('[data-question-reply-item-id]')?.textContent).not.toContain('  custom answer  ')
    expect(display.submission.answers[0]).toMatchObject({ customText: '  custom answer  ' })
  })
})


it('finds the actual accepted Reply after the 32nd request without changing resource budgets', async () => {
  const wrong = Array.from({ length: 39 }, (_, index) => {
    const id = 'input_' + (index + 1).toString(16).padStart(64, '0')
    return { ...input, id: 'item_' + id, requestId: id }
  })
  const open = vi.fn(async (request: { threadId: string; turnId: string; itemId: string; requestId: string }) => ({
    ok: true, display: request.requestId === requestId ? original() : { ...original(), threadId: request.threadId, turnId: request.turnId, itemId: request.itemId, requestId: request.requestId,
      questions: [{ ...original().questions[0], id: request.requestId + '_1' }],
      submission: { schemaVersion: 1, format: 'structured', answers: [{ id: request.requestId + '_1', kind: 'skip' }] },
      lateReply: { ...original().lateReply!, turnId: 'different-target' } }
  }))
  Object.assign(window.analytix!.runtime, { openUserInputLocalDisplay: open })
  await act(async () => root.render(createElement(QuestionReplyView, { block: user, requests: [...wrong, input],
    threadId: selector.threadId, fallback: createElement('span', null, user.text) })))
  expect(open.mock.calls.length).toBeGreaterThanOrEqual(40)
  expect(container.querySelector('[data-question-reply-item-id]')?.getAttribute('data-question-reply-item-id')).toBe(user.id)
  expect(container.textContent).toContain('Synthetic original reply')
})


describe('proven inherited reply consumer', () => {
 const historical = (): UserInputLocalDisplayV1 => ({ ...original(), readOnly: true, timedWait: null, lateReply: null,
   historyReply: { itemId: user.id, clientUserMessageId, turnId: String(user.meta!.turnId) } })
 it('restores original reply on the actual immutable child item', async () => {
   display = historical(); await draw()
   expect(userInputReplyMatches(display, user)).toBe(true)
   expect(container.querySelector('[data-question-reply-item-id]')?.getAttribute('data-question-reply-item-id')).toBe(user.id)
   expect(container.textContent).toContain('Synthetic original reply'); expect(container.textContent).not.toContain(user.text)
 })
 it.each(['itemId', 'clientUserMessageId', 'turnId'] as const)('falls back for a mismatched %s', async field => {
   display = historical(); display.historyReply = { ...display.historyReply!, [field]: field === 'clientUserMessageId' ? '00000000-0000-4000-8000-000000000001' : 'different-valid-id' }
   expect(userInputReplyMatches(display, user)).toBe(false); await draw()
   expect(container.querySelector('[data-question-reply-item-id]')).toBeNull(); expect(container.textContent).toBe(user.text)
 })
 it('keeps Copy metadata-only and waits for the existing Main ACK', async () => {
   display = historical()
   let finish!: (value: { ok: true }) => void
   const effect = vi.fn((_request: unknown) => new Promise<{ ok: true }>(resolve => { finish = resolve }))
   Object.assign(window.analytix.runtime, { performUserInputLocalEffect: effect })
   await draw(); await act(async () => (container.querySelector('button[aria-label="Copy"]') as HTMLButtonElement).click())
   expect(effect.mock.calls[0]?.[0]).toEqual(expect.objectContaining({ effect: { kind: 'copyReply', locale: 'en', clientUserMessageId, expectedTurnId: 'turn-steering-target' } }))
   expect(JSON.stringify(effect.mock.calls)).not.toContain('Synthetic original reply')
   expect(container.querySelector('button[aria-label="Copied"]')).toBeNull()
   await act(async () => finish({ ok: true }))
   expect(container.querySelector('button[aria-label="Copied"]')).not.toBeNull()
 })
})
