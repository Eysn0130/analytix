// @vitest-environment jsdom
import { act, useEffect, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { ThreadSummaryPanelView } from './ThreadSummaryPanel'
import { ThreadSummarySurface, THREAD_SUMMARY_TITLE_ID, THREAD_SUMMARY_TRIGGER_ID } from './ThreadSummarySurface'

let root: Root, host: HTMLDivElement, chat: HTMLDivElement, fileButton: HTMLButtonElement
let bounds: { left: number; top: number; width: number; height: number }
const observers = new Set<() => void>()
const mounted = vi.fn(), unmounted = vi.fn(), reserve = vi.fn(), fileClick = vi.fn()
function Summary({ pinned, togglePin, close }: { pinned: boolean; togglePin: () => void; close: () => void }) {
  useEffect(() => { mounted(); return () => { unmounted() } }, [])
  return <ThreadSummaryPanelView state={{ status: 'idle', data: null, error: null }} busyTaskId={null} outputState={{ status: 'closed' }}
    titleId={THREAD_SUMMARY_TITLE_ID} pinPreference={pinned} onTogglePin={togglePin} onCollapse={close}
    onCloseOutput={() => {}} onOpenThread={() => {}} />
}
function Harness() {
  const [openIntent, setOpenIntent] = useState(false), [pinPreference, setPinPreference] = useState(true)
  const dismiss = (restoreFocus: boolean) => {
    setOpenIntent(false)
    if (restoreFocus) document.getElementById(THREAD_SUMMARY_TRIGGER_ID)?.focus({ preventScroll: true })
  }
  return <>
    <button id={THREAD_SUMMARY_TRIGGER_ID} aria-expanded={openIntent} onClick={() => setOpenIntent(open => !open)}>Summary</button>
    <ThreadSummarySurface container={chat} openIntent={openIntent} pinPreference={pinPreference} onReserveChange={reserve} onDismiss={dismiss}>
      <Summary pinned={pinPreference} togglePin={() => setPinPreference(pinned => !pinned)} close={() => dismiss(true)} />
    </ThreadSummarySurface>
  </>
}
const surface = () => document.querySelector<HTMLDivElement>('[data-thread-summary-surface]')
const trigger = () => document.getElementById(THREAD_SUMMARY_TRIGGER_ID) as HTMLButtonElement
const closeButton = () => surface()!.querySelector<HTMLButtonElement>('[aria-label="关闭摘要"]')!
async function resize(width: number, height = bounds.height) {
  bounds.width = width; bounds.height = height
  await act(async () => observers.forEach(notify => notify()))
}
async function key(options: KeyboardEventInit, consumed = false) {
  const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true, ...options })
  if (consumed) event.preventDefault()
  await act(async () => (surface() ?? document.body).dispatchEvent(event))
  return event
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class {
    notify: () => void
    constructor(callback: ResizeObserverCallback) { this.notify = () => callback([], this as unknown as ResizeObserver) }
    observe() { observers.add(this.notify) }
    disconnect() { observers.delete(this.notify) }
  })
  vi.stubGlobal('innerWidth', 1800); vi.stubGlobal('innerHeight', 900)
  bounds = { left: 80, top: 60, width: 1200, height: 600 }
  mounted.mockClear(); unmounted.mockClear(); reserve.mockClear(); fileClick.mockClear()
  await i18n.changeLanguage('zh')
  host = document.createElement('div'); chat = document.createElement('div'); fileButton = document.createElement('button')
  chat.getBoundingClientRect = () => new DOMRect(bounds.left, bounds.top, bounds.width, bounds.height)
  fileButton.textContent = 'Synthetic file'; fileButton.addEventListener('click', fileClick)
  document.body.append(host, chat, fileButton)
  root = createRoot(host)
  await act(async () => root.render(<Harness />))
  await act(async () => { trigger().focus(); trigger().click() })
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); chat.remove(); fileButton.remove(); observers.clear()
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('chat-owned thread summary', () => {
  it('floats an already open summary and restores pinning without remounting or losing scroll/focus', async () => {
    const card = surface()!, scroll = card.querySelector<HTMLElement>('.overflow-y-auto')!
    expect(card.dataset.mode).toBe('pinned'); expect(card.getAttribute('role')).toBe('region')
    expect(reserve).toHaveBeenLastCalledWith(324)
    scroll.scrollTop = 92; closeButton().focus()
    const focused = document.activeElement
    await resize(848)
    expect(surface()).toBe(card); expect(card.dataset.mode).toBe('floating')
    expect(card.getAttribute('role')).toBe('dialog'); expect(card.getAttribute('aria-modal')).toBe('false')
    expect(reserve).toHaveBeenLastCalledWith(0)
    expect(document.activeElement).toBe(focused); expect(scroll.scrollTop).toBe(92)
    expect(Number.parseFloat(card.style.left) + Number.parseFloat(card.style.width)).toBeLessThanOrEqual(bounds.left + bounds.width)
    await resize(1200)
    expect(surface()).toBe(card); expect(card.dataset.mode).toBe('pinned')
    expect(reserve).toHaveBeenLastCalledWith(324)
    expect(mounted).toHaveBeenCalledTimes(1); expect(unmounted).not.toHaveBeenCalled()
    expect(document.querySelectorAll('[data-thread-summary-surface]')).toHaveLength(1)
  })
  it('retains an explicit floating preference across resize and reopen', async () => {
    const pin = surface()!.querySelector<HTMLButtonElement>('[aria-pressed]')!
    await act(async () => pin.click())
    expect(pin.getAttribute('aria-pressed')).toBe('false'); expect(surface()!.dataset.mode).toBe('floating')
    await resize(700); await resize(1300)
    expect(surface()!.dataset.mode).toBe('floating'); expect(reserve).toHaveBeenLastCalledWith(0)
    await act(async () => closeButton().click())
    expect(surface()).toBeNull(); expect(document.activeElement).toBe(trigger())
    await act(async () => trigger().click())
    expect(surface()!.dataset.mode).toBe('floating')
    expect(surface()!.querySelector('[aria-pressed]')!.getAttribute('aria-pressed')).toBe('false')
  })
  it('enters an explicitly opened floating dialog but preserves viewer focus on automatic mode changes', async () => {
    fileButton.focus()
    await resize(700)
    expect(document.activeElement).toBe(fileButton)
    await act(async () => closeButton().click())
    expect(document.activeElement).toBe(trigger())
    await act(async () => trigger().click())
    expect(document.activeElement).toBe(surface())
    expect(surface()!.getAttribute('tabindex')).toBe('-1')
  })
  it('keeps a pinned card open on outside clicks and Escape', async () => {
    await key({})
    await act(async () => fileButton.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })))
    expect(surface()!.dataset.mode).toBe('pinned'); expect(unmounted).not.toHaveBeenCalled()
  })
  it('lets an outside file click complete without stealing its focus', async () => {
    await resize(700)
    const internal = new MouseEvent('pointerdown', { bubbles: true, cancelable: true })
    await act(async () => closeButton().dispatchEvent(internal))
    expect(surface()).not.toBeNull()
    const outside = new MouseEvent('pointerdown', { bubbles: true, cancelable: true })
    await act(async () => { fileButton.dispatchEvent(outside); fileButton.focus(); fileButton.click() })
    expect(outside.defaultPrevented).toBe(false); expect(fileClick).toHaveBeenCalledTimes(1)
    expect(surface()).toBeNull(); expect(document.activeElement).toBe(fileButton)
  })
  it('ignores composing and consumed Escape, then closes and restores the trigger', async () => {
    await resize(700)
    await key({ isComposing: true }); await key({ keyCode: 229 }); await key({}, true)
    expect(surface()).not.toBeNull()
    closeButton().focus()
    const windowEscape = vi.fn()
    window.addEventListener('keydown', windowEscape)
    expect((await key({})).defaultPrevented).toBe(true)
    window.removeEventListener('keydown', windowEscape)
    expect(windowEscape).not.toHaveBeenCalled()
    expect(surface()).toBeNull(); expect(document.activeElement).toBe(trigger())
  })
  it('bounds a low-height card inside chat while retaining its close and scrolling body', async () => {
    await resize(270, 140)
    expect(surface()!.style.width).toBe('246px'); expect(surface()!.style.maxHeight).toBe('116px')
    expect(surface()!.querySelector('.overflow-y-auto')).not.toBeNull(); expect(closeButton()).not.toBeNull()
    await act(async () => closeButton().click())
    expect(surface()).toBeNull(); expect(document.activeElement).toBe(trigger())
  })
})
