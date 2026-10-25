// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock } from '../../agent/types'
import {
  userInputLocalDisplaySchemaV1, userInputLocalViewRequestSchemaV1,
  type UserInputLocalDisplayV1, type UserInputLocalQuestionV1,
  type UserInputLocalViewRequestV1, type UserInputLocalViewResultV1
} from '../../../../../packages/runtime/src/contracts/user-input-local-display'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { MessageBubble } from './message-timeline-bubbles'

type InputBlock = Extract<ChatBlock, { kind: 'user_input' }>
const inputId = (hex = 'a') => 'input_' + hex.repeat(64)
const questionId = (hex = 'a', ordinal = 1) => inputId(hex) + '_' + ordinal
const optionId = (hex = 'a', ordinal = 1, option = 1) => questionId(hex, ordinal) + '_option_' + option
function fixture(hex = 'a', questions: UserInputLocalQuestionV1[] = [{
  id: questionId(hex), header: 'Synthetic question', question: 'Choose a synthetic option', multiSelect: false,
  options: [{ id: optionId(hex), label: 'Alpha', description: 'Synthetic option' }]
}], threadId = 'synthetic-thread') {
  const display: UserInputLocalDisplayV1 = { schemaVersion: 1, threadId, turnId: 'synthetic-turn-' + hex,
    itemId: 'item_' + inputId(hex), requestId: inputId(hex), status: 'pending', answerState: 'pending', questions }
  // The public block supplies identities and safe projection only. These words
  // deliberately differ from the receipt so public fallback cannot pass.
  const block: InputBlock = { kind: 'user_input', id: display.itemId, requestId: display.requestId, status: 'pending',
    meta: { turnId: display.turnId }, questions: questions.map(question => ({
      id: question.id, header: 'Projected header', question: 'Projected question', multiSelect: question.multiSelect,
      options: question.options.map(option => ({ id: option.id, label: 'Projected option', description: '' }))
    })) }
  return { block, display }
}
const freeform = (hex = 'a', question = 'Choose a synthetic option'): UserInputLocalQuestionV1[] => [{
  id: questionId(hex), header: 'Synthetic question', question, multiSelect: false, options: []
}]
const displayKey = (value: { threadId: string; turnId: string; itemId: string; requestId: string }) =>
  JSON.stringify([value.threadId, value.turnId, value.itemId, value.requestId])
let displays: Map<string, UserInputLocalDisplayV1>
let invalidated: Set<(viewId: string | null) => void>
const emitInvalidated = (viewId: string | null) => { for (const handler of invalidated) handler(viewId) }
function bind(display: UserInputLocalDisplayV1) {
  const closed = userInputLocalDisplaySchemaV1.parse(display)
  displays.set(displayKey(closed), closed)
}
function makeApi() {
  return {
    openUserInputLocalDisplay: vi.fn(async (request: UserInputLocalViewRequestV1): Promise<UserInputLocalViewResultV1> => {
      const parsed = userInputLocalViewRequestSchemaV1.safeParse(request)
      if (!parsed.success) return { ok: false, code: 'unavailable' }
      const display = displays.get(displayKey(parsed.data))
      return display ? { ok: true, display } : { ok: false, code: 'unavailable' }
    }),
    closeUserInputLocalDisplay: vi.fn(async (viewId: string) => { emitInvalidated(viewId) }),
    invalidateUserInputLocalDisplay: vi.fn(async () => { emitInvalidated(null) }),
    onUserInputLocalDisplayInvalidated: vi.fn((handler: (viewId: string | null) => void) => {
      invalidated.add(handler); return () => { invalidated.delete(handler) }
    }),
    onRuntimeStatus: vi.fn((_handler: (payload: { state: string }) => void) => () => undefined),
    runtimeRequest: vi.fn()
  }
}
let root: Root, container: HTMLDivElement
let previous: ReturnType<typeof useChatStore.getState>
let previousApi: PropertyDescriptor | undefined
let api: ReturnType<typeof makeApi>
const resolve = vi.fn<(...args: unknown[]) => Promise<void>>(async () => {})
async function render(value: ChatBlock, threadId?: string | null) {
  await act(async () => root.render(createElement(MessageBubble, { block: value, threadId })))
}
const gateButton = (key: string) => [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === i18n.t('common:' + key)) ??
  [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.getAttribute('aria-label') === i18n.t('common:' + key))!
const optionInput = (id = optionId()) => container.querySelector<HTMLInputElement>(`input[value="${id}"]`)!
async function typeCustom(value: string) {
  const textarea = container.querySelector<HTMLTextAreaElement>('textarea')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea, value)
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
const target = (block: InputBlock, threadId: string | null = 'synthetic-thread') => ({ itemId: block.id, requestId: block.requestId, threadId, turnId: block.meta?.turnId })
const expectNoPublicFallback = () => {
  expect(container.textContent).not.toContain('Projected question')
  expect(container.textContent).not.toContain('Projected option')
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('zh')
  previous = useChatStore.getState(); previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  displays = new Map(); invalidated = new Set(); api = makeApi(); resolve.mockReset(); resolve.mockResolvedValue(undefined)
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  useChatStore.setState({ activeThreadId: 'synthetic-thread', currentTurnId: 'unrelated-store-turn', blocks: [],
    route: 'chat', resolveUserInput: resolve, pendingGateResolutionIds: {} })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); useChatStore.setState(previous)
  document.getSelection()?.removeAllRanges(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi)
  else Reflect.deleteProperty(window, 'analytix')
})

describe('existing user input controls', () => {
  it.each(['cancelled', 'error'] as const)('does not present an unsubmitted local choice as an answer on %s', async status => {
    const value = fixture(); bind(value.display); await render(value.block)
    await act(async () => optionInput().click())
    expect(optionInput().checked).toBe(true)
    if (status === 'cancelled') bind({ ...value.display, status: 'cancelled', answerState: 'cancelled' })
    await render({ ...value.block, status })
    expect(container.querySelectorAll('input:checked')).toHaveLength(0)
    expect(container.querySelector('textarea')).toBeNull()
    expect(resolve).not.toHaveBeenCalled(); expectNoPublicFallback()
  })

  it('keeps the whole request disabled and named while the shared response is in flight', async () => {
    const value = fixture(); bind(value.display); await render(value.block)
    await act(async () => useChatStore.setState({ pendingGateResolutionIds: { ['user_input:' + value.block.requestId]: true } }))
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull()
    expect(container.querySelector('[role="radiogroup"]')?.getAttribute('aria-label')).toBe('Choose a synthetic option')
    expect(Array.from(container.querySelectorAll<HTMLButtonElement>('button')).every(button => button.disabled)).toBe(true)
    expect(container.textContent).toContain(i18n.t('common:requestResolving')); expectNoPublicFallback()
  })

  it.each(['freeform', 'other'] as const)('keeps IME, native 229 and key repeats from submitting the %s textarea', async kind => {
    const value = kind === 'freeform' ? fixture('a', freeform()) : fixture()
    bind(value.display); await render(value.block)
    if (kind === 'other') expect(container.querySelector('[data-question-custom] textarea')).not.toBeNull()
    await typeCustom('Synthetic answer')
    const textarea = container.querySelector('textarea')!
    expect(textarea.getAttribute('aria-label')).toBe('Choose a synthetic option')
    await act(async () => textarea.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true, cancelable: true })))
    await act(async () => textarea.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, isComposing: true, bubbles: true, cancelable: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, keyCode: 229, bubbles: true, cancelable: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, repeat: true, bubbles: true, cancelable: true })))
    expect(resolve).not.toHaveBeenCalled()
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true, cancelable: true })))
    expect(resolve).toHaveBeenCalledOnce()
    expect(resolve).toHaveBeenCalledWith(target(value.block), { kind: 'submit', answers: [
      { id: questionId(), kind: 'answer', optionIds: [], customText: 'Synthetic answer' }
    ] })
  })
})

describe('question ownership through the production MessageBubble entry', () => {
  it('submits one structured batch through the existing store action', async () => {
    const value = fixture('a', [...fixture().display.questions, {
      id: questionId('a', 2), header: 'Second', question: 'Synthetic second question', multiSelect: false, options: []
    }])
    bind(value.display); await render(value.block)
    await act(async () => optionInput().click())
    expect(resolve).not.toHaveBeenCalled()
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic second question')
    const skip = gateButton('userInputSkip')
    await act(async () => { skip.click(); skip.click() })
    expect(resolve).toHaveBeenCalledOnce()
    expect(resolve).toHaveBeenCalledWith(target(value.block), { kind: 'submit', answers: [
      { id: questionId(), kind: 'answer', optionIds: [optionId()] }, { id: questionId('a', 2), kind: 'skip' }
    ] })
    expectNoPublicFallback()
  })

  it.each(['thread', 'request'] as const)('does not carry a custom draft across a changed %s key', async owner => {
    const value = fixture('a', freeform()); bind(value.display); await render(value.block)
    await typeCustom('Synthetic scoped draft')
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic scoped draft')
    const threadId = owner === 'thread' ? 'other-synthetic-thread' : 'synthetic-thread'
    const next = fixture('b', freeform('b'), threadId); bind(next.display)
    if (owner === 'thread') {
      await act(async () => useChatStore.setState({ activeThreadId: threadId }))
      // The old pair does not belong to the newly selected thread.
      expect(container.querySelector('textarea')).toBeNull()
      expect(container.textContent).not.toContain('Synthetic scoped draft')
    }
    await render(next.block)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    expect(container.textContent).not.toContain('Synthetic scoped draft')
    expect(api.openUserInputLocalDisplay).toHaveBeenLastCalledWith({ threadId, turnId: next.display.turnId,
      itemId: next.block.id, requestId: next.block.requestId, viewId: expect.any(String) })
    expect(resolve).not.toHaveBeenCalled()
  })
})

describe('captured user-input resolution targets from MessageBubble', () => {
  it.each([undefined, 'owner-synthetic-thread'])('captures the displayed owner thread %s for submit and cancel', async threadId => {
    const owner = threadId === undefined ? 'synthetic-thread' : threadId
    const value = fixture('c', freeform('c', 'Synthetic owned question'), owner)
    bind(value.display); await render(value.block, threadId)
    await typeCustom('Synthetic owned answer')
    await act(async () => gateButton('userInputSubmit').click())
    expect(resolve).toHaveBeenCalledOnce()
    expect(resolve).toHaveBeenCalledWith(target(value.block, owner), { kind: 'submit', answers: [
      { id: questionId('c'), kind: 'answer', optionIds: [], customText: 'Synthetic owned answer' }
    ] })
    resolve.mockClear()
    await act(async () => gateButton('userInputCancel').click())
    expect(resolve).toHaveBeenCalledOnce()
    expect(resolve).toHaveBeenCalledWith(target(value.block, owner), { kind: 'cancel' })
  })

  it('does not fetch raw text or send any action on a null displayed thread', async () => {
    const value = fixture('c', freeform('c', 'Synthetic owned question')); bind(value.display)
    await render(value.block, null)
    expect(api.openUserInputLocalDisplay).not.toHaveBeenCalled()
    expect(container.querySelector('textarea')).toBeNull()
    expect(gateButton('userInputSubmit').disabled).toBe(true)
    await act(async () => gateButton('userInputSubmit').click())
    expect(resolve).not.toHaveBeenCalled()
    expect(gateButton('userInputCancel').disabled).toBe(true)
    await act(async () => gateButton('userInputCancel').click())
    expect(resolve).not.toHaveBeenCalled()
    expectNoPublicFallback()
  })

  it.each(['submit', 'cancel'] as const)('keeps an old displayed %s target when the store replaces the same item with another request', async action => {
    const value = fixture('d', freeform('d', 'Synthetic original question'), 'owner-synthetic-thread')
    bind(value.display); await render(value.block, 'owner-synthetic-thread')
    await typeCustom('Synthetic captured answer')
    const replacement = fixture('e', freeform('e', 'Synthetic replacement question'), 'owner-synthetic-thread')
    await act(async () => useChatStore.setState({ blocks: [{ ...replacement.block, id: value.block.id }] }))
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic original question')
    await act(async () => gateButton(action === 'submit' ? 'userInputSubmit' : 'userInputCancel').click())
    expect(resolve).toHaveBeenCalledOnce()
    expect(resolve.mock.calls[0]?.[0]).toEqual(target(value.block, 'owner-synthetic-thread'))
    expect(resolve.mock.calls[0]?.[1]).toEqual(action === 'submit' ? { kind: 'submit', answers: [
      { id: questionId('d'), kind: 'answer', optionIds: [], customText: 'Synthetic captured answer' }
    ] } : { kind: 'cancel' })
    expect(JSON.stringify(resolve.mock.calls)).not.toContain(replacement.block.requestId)
  })
})

describe('private display admission and retirement through MessageBubble', () => {
  it('does not guess a missing producer turn from the current store turn', async () => {
    const value = fixture(); bind(value.display)
    await render({ ...value.block, meta: undefined })
    expect(api.openUserInputLocalDisplay).not.toHaveBeenCalled()
    expect(container.querySelector('input,textarea')).toBeNull()
    expect(gateButton('userInputSubmit').disabled).toBe(true)
    expect(resolve).not.toHaveBeenCalled(); expectNoPublicFallback()
  })

  it('fails closed without the dedicated bridge instead of requesting raw text through the generic facade', async () => {
    const value = fixture(); bind(value.display)
    const generic = vi.fn()
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { runtimeRequest: generic } } })
    await render(value.block)
    expect(api.openUserInputLocalDisplay).not.toHaveBeenCalled(); expect(generic).not.toHaveBeenCalled()
    expect(container.querySelector('input,textarea')).toBeNull()
    expect(gateButton('userInputSubmit').disabled).toBe(true)
    expect(resolve).not.toHaveBeenCalled(); expectNoPublicFallback()
  })

  it('rejects a schema-valid original question response from another Core pair', async () => {
    const value = fixture(), other = fixture('b')
    const foreign = userInputLocalDisplaySchemaV1.parse({ ...other.display, turnId: value.display.turnId })
    api.openUserInputLocalDisplay.mockResolvedValueOnce({ ok: true, display: foreign })
    await render(value.block)
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledWith({ threadId: value.display.threadId, turnId: value.display.turnId,
      itemId: value.block.id, requestId: value.block.requestId, viewId: expect.any(String) })
    expect(container.querySelector('input,textarea')).toBeNull()
    expect(gateButton('userInputSubmit').disabled).toBe(true)
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalled()
    expect(resolve).not.toHaveBeenCalled(); expectNoPublicFallback()
  })

  it('retires an old thread read and never shows its late original question in the new displayed owner', async () => {
    const value = fixture('a', freeform('a', 'Synthetic obsolete question'))
    const next = fixture('b', freeform('b', 'Synthetic new thread question'), 'other-synthetic-thread')
    bind(value.display); bind(next.display)
    let complete!: (value: UserInputLocalViewResultV1) => void
    api.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
    await render(value.block)
    const oldView = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    await act(async () => {
      useChatStore.setState({ activeThreadId: next.display.threadId })
      root.render(createElement(MessageBubble, { block: next.block }))
    })
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic new thread question')
    await act(async () => complete({ ok: true, display: value.display }))
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic new thread question')
    expect(container.textContent).not.toContain('Synthetic obsolete question')
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalledWith(oldView)
    expect(resolve).not.toHaveBeenCalled(); expectNoPublicFallback()
  })

  it.each(['lease', 'all'] as const)('clears raw text and drafts on %s invalidation and rejects a late retry response', async scope => {
    const value = fixture('a', freeform('a', 'Synthetic private question'))
    bind(value.display); await render(value.block)
    await typeCustom('Synthetic private draft')
    const firstView = api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    await act(async () => {
      if (scope === 'all') await api.invalidateUserInputLocalDisplay()
      else emitInvalidated(firstView)
    })
    expect(container.querySelector('textarea')).toBeNull()
    expect(container.textContent).not.toContain('Synthetic private question')
    expect(container.textContent).not.toContain('Synthetic private draft')
    let complete!: (value: UserInputLocalViewResultV1) => void
    api.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
    await act(async () => gateButton('protectedToolRetry').click())
    const calls = api.openUserInputLocalDisplay.mock.calls.length
    const retryView = api.openUserInputLocalDisplay.mock.calls[calls - 1][0].viewId
    await act(async () => {
      if (scope === 'all') await api.invalidateUserInputLocalDisplay()
      else emitInvalidated(retryView)
    })
    await act(async () => complete({ ok: true, display: value.display }))
    expect(container.querySelector('textarea')).toBeNull()
    expect(container.textContent).not.toContain('Synthetic private question')
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledTimes(calls)
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalledWith(retryView)
    await act(async () => gateButton('protectedToolRetry').click())
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    expect(gateButton('userInputSubmit').disabled).toBe(true)
    expect(resolve).not.toHaveBeenCalled(); expectNoPublicFallback()
  })
})


it('keeps the physical Question turn in a captured target after a store-only turn replacement', async () => {
  const value = fixture('f', freeform('f', 'Synthetic physical owner'))
  bind(value.display); await render(value.block)
  await typeCustom('Synthetic captured original answer')
  await act(async () => useChatStore.setState({ blocks: [{ ...value.block, meta: { turnId: 'replacement-store-turn' } }] }))
  await act(async () => gateButton('userInputSubmit').click())
  expect(resolve).toHaveBeenCalledWith(target(value.block), { kind: 'submit', answers: [
    { id: questionId('f'), kind: 'answer', optionIds: [], customText: 'Synthetic captured original answer' }
  ] })
  expect(resolve.mock.calls[0][0]).toMatchObject({ turnId: value.display.turnId })
  expect(resolve.mock.calls[0][0]).not.toMatchObject({ turnId: 'replacement-store-turn' })
})
