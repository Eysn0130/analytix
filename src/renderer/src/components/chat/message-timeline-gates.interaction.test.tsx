// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock } from '../../agent/types'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { MessageBubble } from './message-timeline-bubbles'

let root: Root, container: HTMLDivElement
let previous: ReturnType<typeof useChatStore.getState>
const resolve = vi.fn(async () => {})
const block = (): Extract<ChatBlock, { kind: 'user_input' }> => ({ kind: 'user_input', id: 'input', requestId: 'request', status: 'pending',
  questions: [{ id: 'question', header: 'Synthetic question', question: 'Choose a synthetic option', options: [{ label: 'Alpha', description: 'Synthetic option' }] }] })
async function render(value: ChatBlock) { await act(async () => root.render(createElement(MessageBubble, { block: value }))) }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('zh'); previous = useChatStore.getState(); resolve.mockClear()
  useChatStore.setState({ activeThreadId: 'synthetic-thread', route: 'chat', resolveUserInput: resolve, pendingGateResolutionIds: {} })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); useChatStore.setState(previous); vi.unstubAllGlobals() })
describe('existing user input controls', () => {
  it.each(['cancelled', 'error'] as const)('does not present an unsubmitted local choice as an answer on %s', async status => {
    const value = block(); await render(value)
    const option = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent?.includes('Alpha'))!
    await act(async () => option.click())
    expect(option.getAttribute('aria-pressed')).toBe('true')
    await render({ ...value, status })
    expect(container.querySelectorAll('[class*="emerald"]')).toHaveLength(0)
    expect(resolve).not.toHaveBeenCalled()
  })
  it('keeps the whole request disabled and named while the shared response is in flight', async () => {
    const value = { ...block(), answers: [{ id: 'question', label: 'Alpha', value: 'Alpha' }] }
    await render(value)
    await act(async () => useChatStore.setState({ pendingGateResolutionIds: { 'user_input:request': true } }))
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull()
    expect(container.querySelector('[role="group"]')?.getAttribute('aria-label')).toBe('Choose a synthetic option')
    expect(Array.from(container.querySelectorAll<HTMLButtonElement>('button')).every(button => button.disabled)).toBe(true)
    expect(container.textContent).toContain(i18n.t('common:requestResolving'))
  })
  it.each(['freeform', 'other'] as const)('keeps IME, native 229 and key repeats from submitting the %s textarea', async kind => {
    const value = kind === 'freeform' ? { ...block(), questions: [{ ...block().questions[0], options: [] }], answers: [{ id: 'question', label: 'Answer', value: 'Synthetic answer' }] }
      : { ...block(), answers: [{ id: 'question', label: 'Other', value: 'Synthetic answer' }] }
    await render(value)
    if (kind === 'other') {
      const other = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent?.includes(i18n.t('common:userInputOther')))!
      await act(async () => other.click())
    }
    const textarea = container.querySelector('textarea')!
    expect(textarea.getAttribute('aria-label')).toBe('Choose a synthetic option')
    await act(async () => textarea.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true })))
    await act(async () => textarea.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, isComposing: true, bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, keyCode: 229, bubbles: true })))
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, repeat: true, bubbles: true })))
    expect(resolve).not.toHaveBeenCalled()
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true })))
    expect(resolve).toHaveBeenCalledOnce()
  })
})
