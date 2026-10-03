// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useShellPanelMotion } from './useShellPanelMotion'

describe('shell panel motion', () => {
  let frames: Map<number, FrameRequestCallback>
  let nextFrame: number
  let now: number
  let state: ReturnType<typeof useShellPanelMotion>
  let host: HTMLDivElement
  let root: ReturnType<typeof createRoot>
  function Panel({ open }: { open: boolean }): null {
    state = useShellPanelMotion({ isVisible: open, size: 400 })
    return null
  }
  async function frame(at: number): Promise<void> {
    now = at
    const callbacks = [...frames.values()]
    frames.clear()
    await act(async () => callbacks.forEach(callback => callback(at)))
  }
  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    frames = new Map()
    nextFrame = 0
    now = 0
    document.documentElement.dataset.motionReduced = 'false'
    vi.spyOn(performance, 'now').mockImplementation(() => now)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      frames.set(++nextFrame, callback)
      return nextFrame
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id) })
    host = document.createElement('div')
    document.body.append(host)
    root = createRoot(host)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    host.remove()
    delete document.documentElement.dataset.motionReduced
    vi.restoreAllMocks()
  })
  it('stops a running close on the next frame when motion is reduced', async () => {
    await act(async () => root.render(createElement(Panel, { open: true })))
    await act(async () => root.render(createElement(Panel, { open: false })))
    await frame(100)
    expect(state.animatedSize).toBeGreaterThan(0)
    expect(state.animatedSize).toBeLessThan(400)
    document.documentElement.dataset.motionReduced = 'true'
    await frame(116)
    expect(state.animatedSize).toBe(0)
    expect(state.opacity).toBe(0)
    expect(state.isMounted).toBe(false)
    expect(frames.size).toBe(0)
  })
  it('reverses a partial close without jumping and reaches the reopened size', async () => {
    await act(async () => root.render(createElement(Panel, { open: true })))
    await act(async () => root.render(createElement(Panel, { open: false })))
    await frame(70)
    const partial = state.animatedSize
    await act(async () => root.render(createElement(Panel, { open: true })))
    expect(state.animatedSize).toBe(partial)
    await frame(300)
    expect(state.animatedSize).toBe(400)
    expect(state.opacity).toBe(1)
    expect(state.isMounted).toBe(true)
    expect(frames.size).toBe(0)
  })
  it('opens and closes immediately when motion is already reduced', async () => {
    document.documentElement.dataset.motionReduced = 'true'
    await act(async () => root.render(createElement(Panel, { open: false })))
    await act(async () => root.render(createElement(Panel, { open: true })))
    expect(state.animatedSize).toBe(400)
    expect(state.isMounted).toBe(true)
    await act(async () => root.render(createElement(Panel, { open: false })))
    expect(state.animatedSize).toBe(0)
    expect(state.isMounted).toBe(false)
    expect(frames.size).toBe(0)
  })
})
