// @vitest-environment jsdom
import { act, createElement, Fragment } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ThreadHandoffOperation } from '@shared/thread-handoff'
import { ConversationWidthControls, conversationPresentationPhase, resolveConversationContentWidth, type ConversationPresentationPhase } from './ConversationWidthControls'
import { ThreadHandoffProgressModal } from './above-composer/ThreadHandoffProgressModal'
import { CONVERSATION_WIDTH_STORAGE_KEY, normalizeConversationWidth, readConversationWidthPreference, useConversationWidthPreference, writeConversationWidthPreference } from '../../lib/chat-presentation-preferences'
import { useChatStore } from '../../store/chat-store'

let body: HTMLDivElement
let root: Root | null
let column: number
let frames: Map<number, FrameRequestCallback>
let resize: ResizeObserverCallback
let disconnect: ReturnType<typeof vi.fn>
let cancelFrame: ReturnType<typeof vi.fn>
let capture: WeakMap<HTMLElement, number>
let previousHandoffState: { threadHandoffOperations: ThreadHandoffOperation[]; activeThreadHandoffOperationId: string | null }

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const { threadHandoffOperations, activeThreadHandoffOperationId } = useChatStore.getState()
  previousHandoffState = { threadHandoffOperations, activeThreadHandoffOperationId }
  useChatStore.setState({ threadHandoffOperations: [], activeThreadHandoffOperationId: null })
  column = 1200
  frames = new Map()
  capture = new WeakMap()
  let frameId = 0
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => { frames.set(++frameId, callback); return frameId }))
  cancelFrame = vi.fn((id: number) => frames.delete(id))
  vi.stubGlobal('cancelAnimationFrame', cancelFrame)
  disconnect = vi.fn()
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback: ResizeObserverCallback) { resize = callback }
    observe = vi.fn()
    disconnect = disconnect
  })
  writeConversationWidthPreference(null)
  body = document.createElement('div')
  body.dataset.analytixConversationBody = ''
  Object.defineProperty(body, 'offsetWidth', { get: () => column })
  document.body.append(body)
  root = createRoot(body)
  await render()
})
afterEach(async () => {
  if (root) await act(async () => root!.unmount())
  useChatStore.setState(previousHandoffState)
  body.remove()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  writeConversationWidthPreference(null)
})
async function render(phase: ConversationPresentationPhase = 'active', container: HTMLDivElement | null = body, reservedInlineEnd = 0) {
  await act(async () => root!.render(createElement(Fragment, null,
    createElement('div', { 'data-analytix-message-timeline-scroller': '', style: { lineHeight: '20px' } }),
    createElement('div', { 'data-composer': '' }, createElement(ThreadHandoffProgressModal)),
    createElement(ConversationWidthControls, { container, phase, label: 'Resize conversation', reservedInlineEnd }))))
  for (const handle of body.querySelectorAll<HTMLElement>('[data-width-handle]')) {
    handle.setPointerCapture = vi.fn((id: number) => capture.set(handle, id))
    handle.hasPointerCapture = vi.fn((id: number) => capture.get(handle) === id)
    handle.releasePointerCapture = vi.fn(() => capture.delete(handle))
  }
}
function handle(side: 'left' | 'right' = 'right') { return body.querySelector<HTMLElement>(`[data-width-handle=${side}]`)! }
function axis() { return body.style.getPropertyValue('--ax-chat-content-axis') }
async function pointer(type: string, x: number, options: { side?: 'left' | 'right'; id?: number; button?: number; y?: number } = {}) {
  const event = new MouseEvent(type, { bubbles: true, cancelable: true, button: options.button ?? 0, clientX: x, clientY: options.y ?? 100 })
  Object.defineProperty(event, 'pointerId', { value: options.id ?? 7 })
  await act(async () => handle(options.side).dispatchEvent(event))
  return event
}
async function flushFrames() {
  const callbacks = [...frames.values()]; frames.clear()
  await act(async () => callbacks.forEach(callback => callback(0)))
}
async function measure(width: number) {
  column = width
  await act(async () => resize([], {} as ResizeObserver))
}

it('keeps the official default axis independent of the dragged floor and edge budget', () => {
  expect([600, 1200, 1800].map(width => resolveConversationContentWidth(width, null))).toEqual([680, 768, 920])
  expect(resolveConversationContentWidth(1200, 50)).toBe(640)
  expect(resolveConversationContentWidth(1200, 1400)).toBe(1024)
  expect(resolveConversationContentWidth(500, 1400)).toBe(640)
  expect(axis()).toBe('')
})
it.each(['left', 'right'] as const)('captures the %s handle and applies outward 2×dx with one RAF and final pointer-up X', async side => {
  const direction = side === 'right' ? 1 : -1
  await pointer('pointerdown', 500, { side })
  expect(handle(side).hasPointerCapture(7)).toBe(true)
  await pointer('pointermove', 500 + direction * 10, { side, y: 130 })
  await pointer('pointermove', 500 + direction * 20, { side, y: 145 })
  expect(frames.size).toBe(1)
  expect(axis()).toBe('')
  expect(handle(side).style.getPropertyValue('--ax-width-handle-pointer-y')).toBe('145px')
  await flushFrames()
  expect(axis()).toBe('808px')
  await pointer('pointermove', 500 + direction * 25, { side })
  expect(frames.size).toBe(1)
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await pointer('pointerup', 500 + direction * 30, { side })
  expect(axis()).toBe('828px')
  expect(write).toHaveBeenCalledExactlyOnceWith(CONVERSATION_WIDTH_STORAGE_KEY, '828')
  expect(frames.size).toBe(0)
  expect(handle(side).hasAttribute('data-dragging')).toBe(false)
  expect(handle(side).releasePointerCapture).toHaveBeenCalledWith(7)
})
it('ignores a non-primary press and unrelated pointers', async () => {
  const press = await pointer('pointerdown', 500, { button: 2 })
  expect(press.defaultPrevented).toBe(false)
  expect(handle().setPointerCapture).not.toHaveBeenCalled()
  await pointer('pointerdown', 500)
  await pointer('pointermove', 700, { id: 8 })
  await pointer('pointerup', 700, { id: 8 })
  expect(frames.size).toBe(0)
  expect(handle().hasAttribute('data-dragging')).toBe(true)
  await pointer('pointercancel', 500)
})
it('does not overwrite a wider saved preference on press-only or Y-only movement', async () => {
  await act(async () => writeConversationWidthPreference(1100))
  await measure(900)
  expect(axis()).toBe('724px')
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await pointer('pointerdown', 500)
  await pointer('pointerup', 500)
  await pointer('pointerdown', 500)
  await pointer('pointermove', 500, { y: 300 })
  await flushFrames()
  await pointer('pointerup', 500)
  expect(write).not.toHaveBeenCalled()
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('1100')
  await measure(1500)
  expect(axis()).toBe('1100px')
})
it.each(['pointercancel', 'lostpointercapture'])('restores the saved value and abandons persistence on %s', async type => {
  await act(async () => writeConversationWidthPreference(850))
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await pointer('pointerdown', 500)
  await pointer('pointermove', 530)
  await flushFrames()
  expect(axis()).toBe('910px')
  await pointer('pointermove', 550)
  if (type === 'lostpointercapture') capture.delete(handle())
  await pointer(type, 550)
  expect(axis()).toBe('850px')
  expect(write).not.toHaveBeenCalled()
  expect(frames.size).toBe(0)
  await flushFrames()
  expect(axis()).toBe('850px')
})
it('restores the default after cancelling an unsaved drag and clears work on phase change', async () => {
  await pointer('pointerdown', 500)
  await pointer('pointermove', 530)
  await flushFrames()
  expect(axis()).toBe('828px')
  await pointer('pointercancel', 530)
  expect(axis()).toBe('')
  await pointer('pointerdown', 500)
  await pointer('pointermove', 600)
  await render('settling')
  expect(frames.size).toBe(0)
  expect(body.querySelector('[data-width-handle]')).toBeNull()
  expect(axis()).toBe('')
})
it('cancels the pending animation frame and disconnects measurement on unmount', async () => {
  await pointer('pointerdown', 500)
  const activeHandle = handle()
  await pointer('pointermove', 550)
  await act(async () => root!.unmount())
  root = null
  expect(frames.size).toBe(0)
  expect(cancelFrame).toHaveBeenCalled()
  expect(disconnect).toHaveBeenCalledOnce()
  expect(activeHandle.hasAttribute('data-dragging')).toBe(false)
  expect(activeHandle.hasPointerCapture(7)).toBe(false)
})
it('restores a visibly dragged saved width when phase retires the handles', async () => {
  await act(async () => writeConversationWidthPreference(850))
  await pointer('pointerdown', 500)
  await pointer('pointermove', 530)
  await flushFrames()
  expect(axis()).toBe('910px')
  await pointer('pointermove', 550)
  await render('settling')
  expect(axis()).toBe('850px')
  expect(frames.size).toBe(0)
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('850')
})
it('retires capture, queued work and the old measured axis when the body changes', async () => {
  await act(async () => writeConversationWidthPreference(850))
  await pointer('pointerdown', 500)
  const activeHandle = handle(), release = activeHandle.releasePointerCapture
  await pointer('pointermove', 530)
  await flushFrames()
  await pointer('pointermove', 550)
  const replacement = document.createElement('div')
  Object.defineProperty(replacement, 'offsetWidth', { value: 1500 })
  document.body.append(replacement)
  try {
    await render('active', replacement)
    expect(axis()).toBe('')
    expect(replacement.style.getPropertyValue('--ax-chat-content-axis')).toBe('850px')
    expect(frames.size).toBe(0)
    expect(release).toHaveBeenCalledWith(7)
    expect(capture.has(activeHandle)).toBe(false)
    expect(disconnect).toHaveBeenCalledOnce()
    await render('active', null)
    expect(replacement.style.getPropertyValue('--ax-chat-content-axis')).toBe('')
    expect(body.querySelector('[data-width-handle]')).toBeNull()
  } finally { replacement.remove() }
})
it('clamps the display on resize without rewriting preference, and restores it when widened', async () => {
  await act(async () => writeConversationWidthPreference(1000))
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await measure(850)
  expect(axis()).toBe('674px')
  await measure(500)
  expect(axis()).toBe('640px')
  await measure(1500)
  expect(axis()).toBe('1000px')
  expect(write).not.toHaveBeenCalled()
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('1000')
})
it('clamps a pinned summary reserve without changing the saved conversation axis', async () => {
  await act(async () => writeConversationWidthPreference(1000))
  column = 1500
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await render('active', body, 324)
  expect(axis()).toBe('1000px')
  column = 1200
  await measure(column)
  expect(axis()).toBe('700px')
  await render('active', body, 0)
  expect(axis()).toBe('1000px')
  expect(write).not.toHaveBeenCalled()
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('1000')
})
it('derives the default pinned axis from effective chat width and restores the CSS default when floating', async () => {
  column = 1500
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await render('active', body, 324)
  expect(axis()).toBe(`${1176 * 0.64}px`)
  await render('active', body, 0)
  expect(axis()).toBe('')
  expect(write).not.toHaveBeenCalled()
})
it('removes zero-space separators from tab order while retaining measurement and the saved preference', async () => {
  await act(async () => writeConversationWidthPreference(640))
  const write = vi.spyOn(Storage.prototype, 'setItem')
  handle().focus()
  expect(document.activeElement).toBe(handle())
  await measure(736)
  expect(axis()).toBe('640px')
  expect(body.querySelector('[role=separator]')).toBeNull()
  expect(body.querySelector('[tabindex="0"]')).toBeNull()
  expect(disconnect).not.toHaveBeenCalled()
  await measure(737)
  expect(body.querySelectorAll('[role=separator][tabindex="0"]')).toHaveLength(2)
  handle().focus()
  expect(document.activeElement).toBe(handle())
  await measure(500)
  expect(body.querySelector('[role=separator]')).toBeNull()
  await measure(1200)
  expect(body.querySelectorAll('[role=separator]')).toHaveLength(2)
  expect(axis()).toBe('640px')
  expect(write).not.toHaveBeenCalled()
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('640')
})
it('retires a captured drag when resize removes all usable handle space', async () => {
  await act(async () => writeConversationWidthPreference(850))
  const activeHandle = handle(), release = activeHandle.releasePointerCapture
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await pointer('pointerdown', 500)
  await pointer('pointermove', 530)
  await flushFrames()
  await pointer('pointermove', 550)
  await measure(736)
  expect(body.querySelector('[role=separator]')).toBeNull()
  expect(frames.size).toBe(0)
  expect(release).toHaveBeenCalledWith(7)
  expect(activeHandle.hasAttribute('data-dragging')).toBe(false)
  expect(write).not.toHaveBeenCalled()
  await measure(1200)
  expect(axis()).toBe('850px')
})
it('keeps a transient drag bounded during resize and commits the current measurement', async () => {
  await pointer('pointerdown', 500)
  await pointer('pointermove', 580)
  await flushFrames()
  expect(axis()).toBe('928px')
  await measure(900)
  expect(axis()).toBe('724px')
  await pointer('pointerup', 580)
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('724')
})
it.each(['hero', 'settling'] as const)('publishes saved geometry without exposing handles in %s', async phase => {
  await act(async () => writeConversationWidthPreference(850))
  await render(phase)
  expect(axis()).toBe('850px')
  expect(body.querySelector('[data-width-handle]')).toBeNull()
})
it('does not render handles without a mounted body', async () => {
  await render('active', null)
  expect(body.querySelector('[data-width-handle]')).toBeNull()
})
it.each([null, 850])('cancels capture and restores %s when a closed real handoff modal asynchronously reopens', async preference => {
  const operation: ThreadHandoffOperation = {
    id: 'width-handoff', direction: 'to-worktree', status: 'running', sourceThreadId: 'thread', targetThreadId: null,
    sourceWorkspace: '/fixture/source', targetWorkspace: null, sourceBranch: null, localBranch: null, worktreeBranch: null,
    request: { direction: 'to-worktree', sourceThreadId: 'thread', sourceWorkspace: '/fixture/source' }, steps: [],
    errorMessage: null, warningMessage: null, execOutput: null, hasUnseenTerminalState: false, worktree: null,
    createdAt: '2026-10-11T00:00:00.000Z', updatedAt: '2026-10-11T00:00:00.000Z'
  }
  await act(async () => useChatStore.getState().handleThreadHandoffEvent(operation))
  expect(body.querySelector('[data-analytix-thread-handoff-modal]')).not.toBeNull()
  expect(body.querySelector('[role=separator]')).toBeNull()
  await act(async () => useChatStore.getState().closeThreadHandoffOperation(operation.id))
  expect(body.querySelector('[data-analytix-thread-handoff-modal]')).toBeNull()
  await render()
  await act(async () => writeConversationWidthPreference(preference))
  const activeHandle = handle(), release = activeHandle.releasePointerCapture
  const write = vi.spyOn(Storage.prototype, 'setItem')
  await pointer('pointerdown', 500)
  await pointer('pointermove', 530)
  await flushFrames()
  expect(axis()).toBe(preference === null ? '828px' : '910px')
  await pointer('pointermove', 550)
  await act(async () => {
    await Promise.resolve()
    useChatStore.getState().handleThreadHandoffEvent(operation)
  })
  expect(body.querySelector('[data-analytix-thread-handoff-modal]')).not.toBeNull()
  expect(body.querySelector('[role=separator]')).toBeNull()
  expect(axis()).toBe(preference === null ? '' : '850px')
  expect(frames.size).toBe(0)
  expect(release).toHaveBeenCalledWith(7)
  expect(capture.has(activeHandle)).toBe(false)
  expect(activeHandle.hasAttribute('data-dragging')).toBe(false)
  await flushFrames()
  expect(write).not.toHaveBeenCalled()
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe(preference === null ? null : '850')
  await act(async () => useChatStore.getState().closeThreadHandoffOperation(operation.id))
  await render()
  expect(body.querySelectorAll('[role=separator]')).toHaveLength(2)
})
it('keeps modal suppression scoped to the measured main body', async () => {
  const otherModal = document.createElement('div')
  otherModal.dataset.analytixThreadHandoffModal = ''
  document.body.append(otherModal)
  try {
    await act(async () => { await Promise.resolve() })
    expect(body.querySelectorAll('[role=separator]')).toHaveLength(2)
  } finally { otherModal.remove() }
})
it('rejects a keyboard reset in the interval before modal observation retires the focused handle', async () => {
  await act(async () => writeConversationWidthPreference(850))
  const separator = handle()
  separator.focus()
  const marker = document.createElement('div')
  marker.dataset.analytixThreadHandoffModal = ''
  const write = vi.spyOn(Storage.prototype, 'removeItem')
  await act(async () => {
    body.querySelector('[data-composer]')!.append(marker)
    separator.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
  })
  expect(axis()).toBe('850px')
  expect(write).not.toHaveBeenCalled()
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('850')
  expect(body.querySelector('[role=separator]')).toBeNull()
  await act(async () => marker.remove())
})
it.each(['left', 'right'] as const)('retains separator keyboard access for the %s handle', async side => {
  const separator = handle(side)
  separator.focus()
  expect(document.activeElement).toBe(separator)
  expect(separator.getAttribute('role')).toBe('separator')
  expect(separator.getAttribute('aria-orientation')).toBe('vertical')
  expect(separator.getAttribute('aria-label')).toBe('Resize conversation')
  expect(separator.getAttribute('aria-valuenow')).toBe('768')
  expect(separator.getAttribute('aria-valuemin')).toBe('640')
  expect(separator.getAttribute('aria-valuemax')).toBe('1024')
  const outward = side === 'right' ? 'ArrowRight' : 'ArrowLeft'
  await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown', { key: outward, bubbles: true, cancelable: true })))
  expect(axis()).toBe('792px')
  await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown', { key: outward, shiftKey: true, bubbles: true, cancelable: true })))
  expect(axis()).toBe('856px')
  await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown', { key: outward, ctrlKey: true, bubbles: true })))
  expect(axis()).toBe('856px')
  await act(async () => separator.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
  expect(axis()).toBe('')
  expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBeNull()
})
it('forwards only vertical non-zoom wheel to this body scrollport in pixels, lines and pages', async () => {
  const scrollport = body.querySelector<HTMLElement>('[data-analytix-message-timeline-scroller]')!
  Object.defineProperty(scrollport, 'clientHeight', { value: 400 })
  scrollport.scrollBy = vi.fn()
  for (const [deltaMode, deltaY, expected] of [[0, 3, 3], [1, 2, 40], [2, -1, -400]]) {
    await act(async () => handle().dispatchEvent(new WheelEvent('wheel', { deltaMode, deltaY, bubbles: true })))
    expect(scrollport.scrollBy).toHaveBeenLastCalledWith({ top: expected })
  }
  scrollport.style.lineHeight = 'normal'
  await act(async () => handle().dispatchEvent(new WheelEvent('wheel', { deltaMode: 1, deltaY: 2, bubbles: true })))
  expect(scrollport.scrollBy).toHaveBeenLastCalledWith({ top: 32 })
  const zoom = new WheelEvent('wheel', { deltaY: 4, ctrlKey: true, bubbles: true, cancelable: true })
  await act(async () => { handle().dispatchEvent(zoom); handle().dispatchEvent(new WheelEvent('wheel', { deltaX: 5, bubbles: true })) })
  expect(zoom.defaultPrevented).toBe(false)
  expect(scrollport.scrollBy).toHaveBeenCalledTimes(4)
})

describe('single presentation preference owner', () => {
  it.each(['920', null, 'broken'])('observes external %s while all width consumers are unmounted', async value => {
    await act(async () => writeConversationWidthPreference(850))
    await act(async () => root!.render(null))
    if (value === null) localStorage.removeItem(CONVERSATION_WIDTH_STORAGE_KEY)
    else localStorage.setItem(CONVERSATION_WIDTH_STORAGE_KEY, value)
    const write = vi.spyOn(Storage.prototype, 'setItem')
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: value === null ? null : CONVERSATION_WIDTH_STORAGE_KEY })))
    await render()
    expect(axis()).toBe(value === '920' ? '920px' : '')
    expect(write).not.toHaveBeenCalled()
    expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe(value)
  })
  it('preserves memory across consumer absence and unavailable external storage reads', async () => {
    await act(async () => writeConversationWidthPreference(850))
    await act(async () => root!.render(null))
    const getter = vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new Error('denied') })
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: CONVERSATION_WIDTH_STORAGE_KEY })))
    await render()
    expect(axis()).toBe('850px')
    getter.mockRestore()
    await act(async () => root!.render(null))
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('unavailable') })
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: null })))
    await render()
    expect(axis()).toBe('850px')
  })
  it('rejects unavailable, damaged, non-finite and non-positive values without migration writes', () => {
    const write = vi.spyOn(Storage.prototype, 'setItem')
    for (const value of [undefined, null, '', ' ', 'bad', '{}', 'Infinity', 'NaN', '0', '-1', false, {}, Infinity, NaN, 0, -1]) {
      expect(normalizeConversationWidth(value)).toBeNull()
    }
    expect(normalizeConversationWidth('812.5')).toBe(812.5)
    expect(readConversationWidthPreference()).toBeNull()
    localStorage.setItem(CONVERSATION_WIDTH_STORAGE_KEY, 'broken')
    write.mockClear()
    expect(readConversationWidthPreference()).toBeNull()
    expect(localStorage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)).toBe('broken')
    expect(write).not.toHaveBeenCalled()
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('unavailable') })
    expect(readConversationWidthPreference()).toBeNull()
  })
  it('retains an in-memory preference when the storage getter or write is unavailable', async () => {
    const getter = vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new Error('denied') })
    expect(readConversationWidthPreference()).toBeNull()
    await act(async () => writeConversationWidthPreference(850))
    expect(axis()).toBe('850px')
    await pointer('pointerdown', 500)
    await pointer('pointermove', 520)
    await pointer('pointerup', 520)
    expect(axis()).toBe('890px')
    getter.mockRestore()
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota') })
    await act(async () => writeConversationWidthPreference(900))
    expect(axis()).toBe('900px')
  })
  it('synchronizes same-window consumers, external storage and reset', async () => {
    function Probe() { return createElement('span', null, String(useConversationWidthPreference())) }
    await act(async () => root!.render(createElement(Fragment, null, createElement(Probe), createElement(Probe))))
    await act(async () => writeConversationWidthPreference(812))
    expect(body.textContent).toBe('812812')
    localStorage.setItem(CONVERSATION_WIDTH_STORAGE_KEY, '920')
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: CONVERSATION_WIDTH_STORAGE_KEY })))
    expect(body.textContent).toBe('920920')
    localStorage.removeItem(CONVERSATION_WIDTH_STORAGE_KEY)
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: null })))
    expect(body.textContent).toBe('nullnull')
  })
})

describe('renderer presentation phase', () => {
  const empty = { threadId: 'thread', hasContent: false, historyLoading: false, summaryBlank: false }
  it('distinguishes blank history settling from already displayed content and known blank summaries', () => {
    expect(conversationPresentationPhase({ ...empty, threadId: null, historyLoading: true })).toBe('hero')
    expect(conversationPresentationPhase(empty)).toBe('hero')
    expect(conversationPresentationPhase({ ...empty, historyLoading: true })).toBe('settling')
    expect(conversationPresentationPhase({ ...empty, historyLoading: true, summaryBlank: true })).toBe('hero')
    expect(conversationPresentationPhase({ ...empty, hasContent: true, historyLoading: true })).toBe('active')
    expect(conversationPresentationPhase({ ...empty, hasContent: true })).toBe('active')
    const busyOnly = { ...empty, running: true }
    expect(conversationPresentationPhase(busyOnly)).toBe('hero')
  })
  it('waits for an explicitly supplied continuable parent readiness projection', () => {
    expect(conversationPresentationPhase({ ...empty, hasContent: true, parentAvailabilityPending: true })).toBe('settling')
    expect(conversationPresentationPhase({ ...empty, hasContent: true, parentAvailabilityPending: false })).toBe('active')
  })
})
