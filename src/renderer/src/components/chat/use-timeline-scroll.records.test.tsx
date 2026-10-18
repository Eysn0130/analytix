// @vitest-environment jsdom
import { act, createElement, useRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useTimelineScroll, type UseTimelineScrollResult } from './use-timeline-scroll'

let root: Root, container: HTMLDivElement, controls: UseTimelineScrollResult
let frames: Map<number, FrameRequestCallback>, serial: number
function Surface({ threadId, contentKey = 'initial', suspended = false, userTurnKey = '' }: { threadId: string; contentKey?: string; suspended?: boolean; userTurnKey?: string }) {
  const ref = useRef<HTMLDivElement>(null)
  controls = useTimelineScroll({ containerRef: ref, activeThreadId: threadId, pageSize: 18, autoCollapseThreshold: 24, totalTurns: 100, busy: false, suspended,
    scrollDeps: { contentKey, streaming: false, userTurnKey } })
  return createElement('div', { ref, 'data-scroll': true })
}
const draw = async (threadId = 'record-nav-hook', contentKey = 'initial', suspended = false, userTurnKey = '') => act(async () => root.render(createElement(Surface, { threadId, contentKey, suspended, userTurnKey })))
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  frames = new Map(); serial = 0
  vi.stubGlobal('requestAnimationFrame', (run: FrameRequestCallback) => { frames.set(++serial, run); return serial })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
it('cancels an already queued prepend frame and pauses content/resize following before precise record navigation', async () => {
  await draw()
  await act(async () => { const queued = [...frames.values()]; frames.clear(); queued.forEach(run => run(0)) })
  const scroller = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 600 })
  scroller.scrollTop = 100
  await act(async () => controls.loadEarlierTurns({ userInitiated: true }))
  const oldPrepend = [...frames.values()].at(-1)!
  expect(oldPrepend).toBeDefined()
  await act(async () => controls.pauseFollowForNavigation())
  scroller.scrollTop = 730
  await act(async () => oldPrepend(0))
  expect(scroller.scrollTop).toBe(730)
  await draw('record-nav-hook', 'arrived-result')
  await act(async () => { controls.preserveBottomOnResize(); for (const run of [...frames.values()]) run(0) })
  expect(scroller.scrollTop).toBe(730)
})
it('expands a real history window without creating a prepend snapshot for exact navigation', async () => {
  await draw('record-nav-expand')
  const scroller = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 600 })
  await act(async () => { const queued = [...frames.values()]; frames.clear(); queued.forEach(run => run(0)); controls.pauseFollowForNavigation() })
  await act(async () => controls.loadEarlierTurns({ userInitiated: true, preserveScroll: false }))
  expect(controls.visibleTurnCount).toBe(36); expect(frames.size).toBe(0)
  await act(async () => controls.loadEarlierTurns({ userInitiated: true, preserveScroll: false }))
  expect(controls.visibleTurnCount).toBe(54)
})
it('fences a queued prepend across a thread change even if an old browser callback is delivered', async () => {
  await draw('record-nav-old')
  const oldScroller = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(oldScroller, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(oldScroller, 'clientHeight', { configurable: true, get: () => 600 })
  await act(async () => { const queued = [...frames.values()]; frames.clear(); queued.forEach(run => run(0)) })
  await act(async () => controls.loadEarlierTurns({ userInitiated: true }))
  const old = [...frames.values()].at(-1)!
  await draw('record-nav-new')
  const scroller = container.querySelector<HTMLElement>('[data-scroll]')!; scroller.scrollTop = 312
  await act(async () => old(0))
  expect(scroller.scrollTop).toBe(312)
})

it('freezes chat sampling and callbacks while another view owns the scroller, then restores the saved chat viewport', async () => {
  await draw('view-suspend-scroll')
  const scroller = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 600 })
  await draw('view-suspend-scroll', 'queued-growth')
  const oldScroll = [...frames.values()].at(-1)!
  expect(oldScroll).toBeDefined()
  scroller.scrollTop = 640
  await act(async () => controls.pauseForViewChange())
  const visible = controls.visibleTurnCount
  await draw('view-suspend-scroll', 'alternate-view', true, 'new-user-while-away')
  scroller.scrollTop = 130
  await act(async () => {
    scroller.dispatchEvent(new Event('scroll'))
    controls.loadEarlierTurns({ userInitiated: true })
    controls.collapseEarlierTurns()
    controls.scrollToBottom()
    controls.snapToBottomIfFollowing()
    controls.snapToBottomIfNearLive()
    controls.preserveBottomOnResize()
    controls.pauseFollowForNavigation()
    controls.pauseForViewChange()
    oldScroll(0)
  })
  expect(controls.visibleTurnCount).toBe(visible)
  expect(scroller.scrollTop).toBe(130)
  await draw('view-suspend-scroll', 'back-to-chat', false, 'new-user-while-away')
  expect(scroller.scrollTop).toBe(640)
  await act(async () => oldScroll(0))
  expect(scroller.scrollTop).toBe(640)
  await draw('view-suspend-scroll', 'later-content', false, 'new-user-while-away')
  await act(async () => { controls.preserveBottomOnResize(); for (const run of [...frames.values()]) run(0) })
  expect(scroller.scrollTop).toBe(640)
})
it('retires queued prepend work and never saves the alternate viewport during suspended unmount', async () => {
  await draw('view-suspend-unmount')
  const scroller = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 600 })
  scroller.scrollTop = 450
  await act(async () => controls.loadEarlierTurns({ userInitiated: true }))
  const oldPrepend = [...frames.values()].at(-1)!
  expect(oldPrepend).toBeDefined()
  await act(async () => controls.pauseForViewChange())
  await draw('view-suspend-unmount', 'away', true)
  scroller.scrollTop = 910
  await act(async () => { oldPrepend(0); scroller.dispatchEvent(new Event('scroll')) })
  expect(scroller.scrollTop).toBe(910)
  await act(async () => root.render(null))
  await draw('view-suspend-unmount', 'back', true)
  const replacement = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(replacement, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(replacement, 'clientHeight', { configurable: true, get: () => 600 })
  replacement.scrollTop = 77
  await draw('view-suspend-unmount', 'back', false)
  expect(replacement.scrollTop).toBe(450)
  expect(controls.visibleTurnCount).toBe(36)
  await act(async () => oldPrepend(0))
  expect(replacement.scrollTop).toBe(450)
})
it('retains the original chat viewport while suspended thread ownership changes', async () => {
  await draw('view-suspend-thread-a')
  const scroller = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 600 })
  scroller.scrollTop = 510
  await act(async () => controls.pauseForViewChange())
  await draw('view-suspend-thread-a', 'away-a', true)
  scroller.scrollTop = 24
  await draw('view-suspend-thread-b', 'away-b', true)
  scroller.scrollTop = 900
  await act(async () => root.render(null))
  await draw('view-suspend-thread-a', 'back-a', true)
  const replacement = container.querySelector<HTMLElement>('[data-scroll]')!
  Object.defineProperty(replacement, 'scrollHeight', { configurable: true, get: () => 10_000 })
  Object.defineProperty(replacement, 'clientHeight', { configurable: true, get: () => 600 })
  await draw('view-suspend-thread-a', 'back-a', false)
  expect(replacement.scrollTop).toBe(510)
})
