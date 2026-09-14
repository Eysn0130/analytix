// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NativeOfficePanel } from './NativeOfficePanel'
import { useNativeOfficeStore } from './native-office-store'
import type { NativeOfficeRequest, NativeOfficeView } from '@shared/native-office'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

const view: NativeOfficeView = { objectId: 'a'.repeat(64), path: '/synthetic/report.docx', kind: 'docx', revision: 'b'.repeat(64), dirty: false, status: 'ready' }
let root: Root | null, container: HTMLDivElement
let rect: { x: number; y: number; width: number; height: number }
let request: ReturnType<typeof vi.fn>, unsubscribe: ReturnType<typeof vi.fn>
const observers: FakeResizeObserver[] = []
class FakeResizeObserver {
  readonly elements = new Set<Element>()
  disconnected = false
  constructor(private readonly callback: ResizeObserverCallback) { observers.push(this) }
  observe(element: Element) { this.elements.add(element) }
  unobserve(element: Element) { this.elements.delete(element) }
  disconnect() { this.disconnected = true; this.elements.clear() }
  // A queued callback may arrive after disconnect; the effect must reject it.
  fire(target: Element) { this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver) }
}
const appearance = { theme: 'light', reducedMotion: false }
const boundsCalls = () => request.mock.calls.map(call => call[0] as NativeOfficeRequest).filter(call => call.action === 'bounds')
async function render(visible: boolean) { await act(async () => root!.render(createElement(NativeOfficePanel, { visible }))) }
async function fire(observer: FakeResizeObserver, target: Element) { await act(async () => observer.fire(target)) }

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', FakeResizeObserver)
  document.documentElement.dataset.theme = 'light'
  document.documentElement.dataset.motionReduced = 'false'
  rect = { x: 1400, y: 90, width: 640, height: 620 }
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(() => ({ ...rect, top: rect.y, left: rect.x, right: rect.x + rect.width, bottom: rect.y + rect.height, toJSON: () => ({ ...rect }) } as DOMRect))
  observers.length = 0
  request = vi.fn(async (_input: NativeOfficeRequest) => ({ ok: true, view }))
  unsubscribe = vi.fn()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { office: { request, onChange: () => unsubscribe } } })
  useNativeOfficeStore.setState({ target: null, view, error: null })
  container = document.createElement('div'); container.className = 'ds-right-sidebar-pane'; document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root?.unmount()); root = null
  container.remove(); useNativeOfficeStore.setState({ target: null, view: null, error: null })
  Reflect.deleteProperty(window, 'analytix'); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('native preview dock geometry', () => {
  it('synchronizes resolved theme and reduced motion without waiting for geometry changes', async () => {
    await render(true)
    await act(async () => {
      document.documentElement.dataset.theme = 'dark'
      document.documentElement.dataset.motionReduced = 'true'
    })
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance: { theme: 'dark', reducedMotion: true } })
  })

  it('shows loading immediately without moving the native surface bounds', async () => {
    useNativeOfficeStore.setState({ target: { workspace: '/synthetic', path: view.path }, view: null })
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render(true)
    expect(container.querySelector('[role="status"]')?.textContent).toContain('nativeOfficeStatus_loading')
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull()
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
    await act(async () => finish({ ok: true, view }))
    expect(container.querySelector('[role="status"]')).toBeNull()
    expect(container.querySelector('[aria-busy="true"]')).toBeNull()
  })

  it('keeps a failed preview visible as a recoverable state', async () => {
    useNativeOfficeStore.setState({ target: { workspace: '/synthetic', path: view.path }, view: null })
    request.mockResolvedValueOnce({ ok: false, view: null, error: 'engine_unavailable' })
    await render(true)
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('nativeOfficeOperationFailed')
    const retry = container.querySelector<HTMLButtonElement>('[aria-label="nativeOfficeRetry"]')!
    await act(async () => retry.click())
    expect(request.mock.calls.filter(call => call[0].action === 'open')).toHaveLength(2)
    expect(container.querySelector('[role="alert"]')).toBeNull()
  })

  it('remeasures position when the dock animates but the surface dimensions stay fixed', async () => {
    await render(true)
    const observer = observers.at(-1)!, surface = container.querySelector('[aria-label="nativeOfficeEditor"]')!
    expect(observer.elements.has(surface)).toBe(true)
    expect(observer.elements.has(container)).toBe(true)
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
    rect = { ...rect, x: 760 }
    await fire(observer, container)
    expect(boundsCalls()).toHaveLength(2)
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: { x: 760, y: 90, width: 640, height: 620 }, appearance })
    await fire(observer, container); await fire(observer, surface)
    expect(boundsCalls()).toHaveLength(2)
  })

  it('covers focus-mode ancestor size changes and window position changes without repeated IPC', async () => {
    await render(true)
    rect = { x: 8, y: 48, width: 1300, height: 690 }
    await fire(observers.at(-1)!, container)
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
    rect = { ...rect, x: 28 }
    await act(async () => window.dispatchEvent(new Event('resize')))
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
    const count = boundsCalls().length
    await act(async () => window.dispatchEvent(new Event('resize')))
    expect(boundsCalls()).toHaveLength(count)
  })

  it('does not attach hidden previews and measures a fresh position when restored', async () => {
    await render(false)
    expect(request.mock.calls.every(call => call[0].action === 'hide')).toBe(true)
    expect(observers).toHaveLength(0)
    await render(true)
    const observer = observers.at(-1)!, surface = container.querySelector('[aria-label="nativeOfficeEditor"]')!
    await render(false)
    const count = boundsCalls().length
    expect(observer.disconnected).toBe(true)
    rect = { ...rect, x: 500 }
    await fire(observer, surface)
    await act(async () => window.dispatchEvent(new Event('resize')))
    expect(boundsCalls()).toHaveLength(count)
    await render(true)
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
  })

  it('disconnects observers/listeners and ignores queued callbacks after unmount', async () => {
    await render(true)
    const observer = observers.at(-1)!, surface = container.querySelector('[aria-label="nativeOfficeEditor"]')!
    await act(async () => root!.unmount()); root = null
    const count = request.mock.calls.length
    expect(observer.disconnected).toBe(true); expect(unsubscribe).toHaveBeenCalledTimes(1)
    rect = { ...rect, x: 300 }
    await fire(observer, surface)
    await act(async () => window.dispatchEvent(new Event('resize')))
    expect(request).toHaveBeenCalledTimes(count)
  })

  it('still observes its own bounds outside the dock', async () => {
    container.className = ''
    await render(true)
    const observer = observers.at(-1)!, surface = container.querySelector('[aria-label="nativeOfficeEditor"]')!
    expect([...observer.elements]).toEqual([surface])
    rect = { ...rect, width: 720 }
    await fire(observer, surface)
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
  })
})
