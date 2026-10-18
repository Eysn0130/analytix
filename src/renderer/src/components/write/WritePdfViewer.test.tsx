// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { WritePdfViewer } from './WritePdfViewer'

const pdfjs = vi.hoisted(() => ({ getDocument: vi.fn(), worker: { workerSrc: '' }, layers: [] as Array<{ scale: number; cancel: ReturnType<typeof vi.fn> }> }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('pdfjs-dist/build/pdf.worker.mjs?url', () => ({ default: 'synthetic-pdf-worker-url' }))
vi.mock('pdfjs-dist/build/pdf.mjs', () => ({
  GlobalWorkerOptions: pdfjs.worker,
  getDocument: pdfjs.getDocument,
  TextLayer: class {
    cancel = vi.fn()
    constructor(private readonly input: { container: HTMLElement; textContentSource: { items: Array<{ str: string }> }; viewport: { scale: number } }) {
      pdfjs.layers.push({ scale: input.viewport.scale, cancel: this.cancel })
    }
    async render() {
      for (const item of this.input.textContentSource.items) {
        const span = document.createElement('span'); span.textContent = item.str
        this.input.container.append(span)
      }
    }
  }
}))

type MockPage = ReturnType<typeof makePage>
type SelectionCallback = Parameters<typeof WritePdfViewer>[0]['onSelectionChange']
function makePage(pageNumber: number, text: string) {
  return {
    getViewport: vi.fn(({ scale }: { scale: number }) => ({ width: 600 * scale, height: 800 * scale, scale, userUnit: 1 })),
    render: vi.fn(() => ({ promise: Promise.resolve(), cancel: vi.fn() })),
    getTextContent: vi.fn(async () => ({ items: [{ str: text }, { str: '   ' }] })),
    cleanup: vi.fn(), pageNumber
  }
}
function makeDocument(texts = ['Alpha needle', 'Beta plain', 'Gamma NEEDLE']) {
  const pages = texts.map((text, index) => makePage(index + 1, text))
  return { numPages: pages.length, getPage: vi.fn(async (page: number) => pages[page - 1]), destroy: vi.fn(), pages }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(finish => { resolve = finish })
  return { promise, resolve }
}
const observers: FakeResizeObserver[] = []
class FakeResizeObserver {
  disconnected = false
  elements = new Set<Element>()
  constructor(private readonly callback: ResizeObserverCallback) { observers.push(this) }
  observe(element: Element) { this.elements.add(element) }
  unobserve(element: Element) { this.elements.delete(element) }
  disconnect() { this.disconnected = true }
  fire() { this.callback([], this as unknown as ResizeObserver) }
}

let container: HTMLDivElement, root: Root | null, pdf: ReturnType<typeof makeDocument>
let loadTask: { promise: Promise<ReturnType<typeof makeDocument>>; destroy: ReturnType<typeof vi.fn> }
let width: number, pageScrollOffset: number, onSelectionChange: Mock<SelectionCallback>, scrollIntoView: ReturnType<typeof vi.fn>
let rangeRectsDescriptor: PropertyDescriptor | undefined, rangeBoundsDescriptor: PropertyDescriptor | undefined
let scrollDescriptor: PropertyDescriptor | undefined
const props = () => ({ filePath: '/synthetic/report.pdf', dataBase64: 'JVBERg==', size: 4, mtimeMs: 1, workspaceRoot: '/synthetic', onSelectionChange })
async function draw(overrides: Partial<Parameters<typeof WritePdfViewer>[0]> = {}) {
  await act(async () => { root!.render(createElement(WritePdfViewer, { ...props(), ...overrides })) })
}
const button = (label: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.getAttribute('aria-label') === label)!
const pageInput = () => container.querySelector<HTMLInputElement>('[aria-label="writePdfPageInput"]')!
const zoom = () => document.querySelector<HTMLSelectElement>('[data-write-pdf-zoom]')!
const canvas = () => container.querySelector<HTMLCanvasElement>('canvas')!
const scroller = () => container.querySelector<HTMLDivElement>('[data-write-pdf-scroller]')!
async function click(label: string) { await act(async () => button(label).click()) }
async function chooseZoom(value: string) {
  await act(async () => { zoom().value = value; zoom().dispatchEvent(new Event('change', { bubbles: true })) })
}
async function fill(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
async function enterPage(value: string) {
  await fill(pageInput(), value)
  await act(async () => pageInput().form!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })))
}
async function syncMouseSelection() {
  const previousCalls = onSelectionChange.mock.calls.length
  await act(async () => {
    // Native selectionchange is asynchronous; the viewer then coalesces its snapshot in the next turn.
    scroller().dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    await vi.waitFor(() => expect(onSelectionChange.mock.calls.length).toBeGreaterThan(previousCalls))
  })
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', FakeResizeObserver)
  observers.length = 0; pdfjs.layers.length = 0; pdfjs.getDocument.mockReset()
  width = 632; pageScrollOffset = 0; pdf = makeDocument(); onSelectionChange = vi.fn<SelectionCallback>()
  loadTask = { promise: Promise.resolve(pdf), destroy: vi.fn() }; pdfjs.getDocument.mockReturnValue(loadTask)
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ setTransform: vi.fn() } as unknown as CanvasRenderingContext2D)
  scrollIntoView = vi.fn()
  vi.stubGlobal('devicePixelRatio', 2)
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    const page = Number((this as HTMLElement).dataset.writePdfPage ?? (this.firstElementChild as HTMLElement | null)?.dataset.writePdfPage)
    return page ? new DOMRect(24, 100 + (page - 1) * 900 - pageScrollOffset, 600, 800) : new DOMRect(0, 0, width, 600)
  })
  scrollDescriptor = Object.getOwnPropertyDescriptor(Element.prototype, 'scrollIntoView')
  Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, value: scrollIntoView })
  const computedStyle = window.getComputedStyle.bind(window)
  vi.spyOn(window, 'getComputedStyle').mockImplementation(element => element.hasAttribute('data-write-pdf-scroller')
    ? { paddingLeft: '16px', paddingRight: '16px' } as CSSStyleDeclaration : computedStyle(element))
  rangeRectsDescriptor = Object.getOwnPropertyDescriptor(Range.prototype, 'getClientRects')
  rangeBoundsDescriptor = Object.getOwnPropertyDescriptor(Range.prototype, 'getBoundingClientRect')
  Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [new DOMRect(40, 120, 100, 20)] })
  Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect(40, 120, 100, 20) })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root?.unmount()); root = null
  container.remove(); window.getSelection()?.removeAllRanges()
  if (rangeRectsDescriptor) Object.defineProperty(Range.prototype, 'getClientRects', rangeRectsDescriptor)
  else Reflect.deleteProperty(Range.prototype, 'getClientRects')
  if (rangeBoundsDescriptor) Object.defineProperty(Range.prototype, 'getBoundingClientRect', rangeBoundsDescriptor)
  else Reflect.deleteProperty(Range.prototype, 'getBoundingClientRect')
  if (scrollDescriptor) Object.defineProperty(Element.prototype, 'scrollIntoView', scrollDescriptor)
  else Reflect.deleteProperty(Element.prototype, 'scrollIntoView')
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('PDF viewer direct interaction', () => {
  it('configures the worker, decodes bounded input and fits content without scaling chrome', async () => {
    await draw()
    expect(pdfjs.worker.workerSrc).toBe('synthetic-pdf-worker-url')
    expect(pdfjs.getDocument).toHaveBeenCalledWith({ data: new Uint8Array([37, 80, 68, 70]), isEvalSupported: false })
    expect(zoom().value).toBe('fit')
    expect(canvas().style.width).toBe('600px')
    expect(canvas().width).toBe(1200)
    expect(pdfjs.layers.at(-1)?.scale).toBe(1)
    expect(container.querySelector<HTMLElement>('.textLayer')!.style.getPropertyValue('--total-scale-factor')).toBe('1')
    const toolbar = container.querySelector<HTMLElement>('[role="toolbar"]')!
    expect(toolbar.style.transform).toBe('')
    const observer = observers.find(observer => observer.elements.has(scroller()))!
    width = 332
    await act(async () => observer.fire())
    expect(canvas().style.width).toBe('300px')
    expect(pdfjs.layers.at(-1)?.scale).toBe(.5)
    expect(container.querySelector<HTMLElement>('.textLayer')!.style.getPropertyValue('--total-scale-factor')).toBe('0.5')
  })

  it('renders precise presets and preserves fixed zoom on resize and incremental zoom limits', async () => {
    await draw()
    for (const value of [.25, .5, 1, 2]) {
      await chooseZoom(String(value))
      expect(canvas().style.width).toBe(`${600 * value}px`)
      expect(pdfjs.layers.at(-1)?.scale).toBe(value)
    }
    width = 332
    await act(async () => observers.find(observer => observer.elements.has(scroller()))!.fire())
    expect(canvas().style.width).toBe('1200px')
    await chooseZoom('fit')
    expect(canvas().style.width).toBe('300px')
    await chooseZoom('in'); expect(zoom().value).toBe('0.6')
    await chooseZoom('out'); expect(zoom().value).toBe('0.5')
    await chooseZoom('0.25')
    expect(zoom().querySelector<HTMLOptionElement>('option[value="out"]')!.disabled).toBe(true)
  })

  it('only portals zoom, preserves inline fallback and removes the portal on unmount', async () => {
    const target = document.createElement('div'); document.body.append(target)
    try {
      await draw({ zoomControlTarget: target })
      expect(target.querySelector('[data-write-pdf-zoom]')).toBe(zoom())
      expect(container.querySelector('[data-write-pdf-zoom]')).toBeNull()
      expect(pageInput()).not.toBeNull()
      await chooseZoom('2'); expect(canvas().style.width).toBe('1200px')
      await draw({ zoomControlTarget: null }); expect(container.querySelector('[data-write-pdf-zoom]')).toBe(zoom())
      await draw({ zoomControlTarget: target })
      await act(async () => root!.unmount()); root = null
      expect(target.childElementCount).toBe(0)
      expect(loadTask.destroy).toHaveBeenCalledOnce()
      expect(pdf.destroy).toHaveBeenCalledOnce()
    } finally { target.remove() }
  })

  it('clamps page navigation and restores the current page for invalid input', async () => {
    await draw()
    expect(button('writePdfPrevPage').disabled).toBe(true)
    await click('writePdfNextPage'); expect(pageInput().value).toBe('2')
    await enterPage('999'); expect(pageInput().value).toBe('3')
    expect(button('writePdfNextPage').disabled).toBe(true)
    await enterPage('invalid'); expect(pageInput().value).toBe('3')
    await enterPage('-2'); expect(pageInput().value).toBe('1')
    expect(scrollIntoView).toHaveBeenLastCalledWith({ block: 'start', behavior: 'smooth' })
  })

  it('updates the page input from scroll geometry through a coalesced animation frame', async () => {
    await draw()
    let frame!: FrameRequestCallback
    const animationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frame = callback; return 7 })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    pageScrollOffset = 900
    await act(async () => { scroller().dispatchEvent(new Event('scroll')); scroller().dispatchEvent(new Event('scroll')) })
    expect(animationFrame).toHaveBeenCalledOnce()
    await act(async () => frame(0))
    expect(pageInput().value).toBe('2')
  })

  it('searches real text-layer content by matching page, cycles and consumes Escape with focus return', async () => {
    await draw(); await click('writePdfSearchPlaceholder')
    const input = container.querySelector<HTMLInputElement>('[role="search"] input')!
    expect(document.activeElement).toBe(input)
    await fill(input, 'NEEDLE')
    expect(pageInput().value).toBe('1')
    expect(container.querySelector('[role="search"] [role="status"]')!.textContent).toBe('1/2')
    await click('writePdfNextMatch'); expect(pageInput().value).toBe('3')
    await click('writePdfNextMatch'); expect(pageInput().value).toBe('1')
    await click('writePdfPrevMatch'); expect(pageInput().value).toBe('3')
    const escaped = vi.fn(); document.addEventListener('keydown', escaped)
    try {
      await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    } finally { document.removeEventListener('keydown', escaped) }
    expect(container.querySelector('[role="search"]')).toBeNull()
    expect(document.activeElement).toBe(button('writePdfSearchPlaceholder'))
    expect(escaped).not.toHaveBeenCalled()
  })

  it('reports text-layer selection with page coordinates, retains it on focus loss and clears it on zoom', async () => {
    await draw()
    const text = container.querySelector('.textLayer span')!.firstChild!
    const range = document.createRange(); range.selectNodeContents(text)
    window.getSelection()!.removeAllRanges(); window.getSelection()!.addRange(range)
    expect(range.collapsed).toBe(false)
    expect(window.getSelection()!.toString()).toBe('Alpha needle')
    expect(container.firstElementChild!.contains(window.getSelection()!.anchorNode)).toBe(true)
    expect(container.firstElementChild!.contains(window.getSelection()!.focusNode)).toBe(true)
    await syncMouseSelection()
    const selection = onSelectionChange.mock.calls.at(-1)![0]
    expect(selection).toMatchObject({ text: 'Alpha needle', sourceKind: 'pdf', pageStart: 1, pageEnd: 1,
      rects: [{ page: 1, x: 16, y: 20, width: 100, height: 20 }], anchorRect: { left: 40, top: 120 } })
    expect(container.querySelector('[data-live-selection]')).not.toBeNull()
    const count = onSelectionChange.mock.calls.length
    await act(async () => { window.getSelection()!.removeAllRanges(); document.dispatchEvent(new Event('selectionchange')) })
    expect(container.querySelector('[data-live-selection]')).toBeNull()
    expect(onSelectionChange).toHaveBeenCalledTimes(count)
    expect(container.querySelector('[aria-hidden="true"] span')).not.toBeNull()
    await chooseZoom('2')
    expect(onSelectionChange.mock.calls.at(-1)![0]).toMatchObject({ text: '', sourceKind: 'pdf' })
    expect(container.querySelector('[aria-hidden="true"] span')).toBeNull()
  })

  it('rejects selections crossing outside the viewer root', async () => {
    await draw()
    const outside = document.createTextNode('Outside'); document.body.append(outside)
    try {
      const inside = container.querySelector('.textLayer span')!.firstChild!
      const range = document.createRange(); range.setStart(inside, 0); range.setEnd(outside, 7)
      window.getSelection()!.removeAllRanges(); window.getSelection()!.addRange(range)
      await syncMouseSelection()
      expect(onSelectionChange.mock.calls.at(-1)![0]).toMatchObject({ text: '', ranges: [] })
    } finally { outside.remove() }
  })

  it('destroys a late old document without replacing the current file', async () => {
    const old = makeDocument(['OLD']), pending = deferred<typeof old>(), oldTask = { promise: pending.promise, destroy: vi.fn() }
    pdfjs.getDocument.mockReturnValueOnce(oldTask)
    await draw()
    await draw({ filePath: '/synthetic/current.pdf', mtimeMs: 2 })
    await act(async () => pending.resolve(old))
    expect(oldTask.destroy).toHaveBeenCalledOnce(); expect(old.destroy).toHaveBeenCalledOnce()
    expect(container.querySelectorAll('[data-write-pdf-page]')).toHaveLength(3)
    expect(container.textContent).not.toContain('OLD')
  })

  it('rejects an old canvas completion after switching files', async () => {
    const pending = deferred<void>(), cancel = vi.fn()
    pdf.pages[0].render.mockReturnValueOnce({ promise: pending.promise, cancel })
    await draw()
    const next = makeDocument(['CURRENT FILE'])
    pdfjs.getDocument.mockReturnValueOnce({ promise: Promise.resolve(next), destroy: vi.fn() })
    await draw({ filePath: '/synthetic/current.pdf', mtimeMs: 2 })
    await act(async () => pending.resolve())
    expect(cancel).toHaveBeenCalledOnce()
    expect(pdf.pages[0].getTextContent).not.toHaveBeenCalled()
    expect(container.querySelectorAll('[data-write-pdf-page]')).toHaveLength(1)
    expect(container.textContent).toContain('CURRENT FILE')
    expect(container.textContent).not.toContain('Alpha needle')
  })

  it('shows load failure and the scanned-page limitation without pretending search has text', async () => {
    pdfjs.getDocument.mockReturnValueOnce({ promise: Promise.reject(new Error('synthetic failure')), destroy: vi.fn() })
    await draw()
    expect(container.querySelector('[role="alert"]')!.textContent).toBe('writePdfLoadFailed')
    expect(button('writePdfSearchPlaceholder').disabled).toBe(true)
    const scanned = makeDocument([''])
    pdfjs.getDocument.mockReturnValueOnce({ promise: Promise.resolve(scanned), destroy: vi.fn() })
    await draw({ mtimeMs: 2 })
    expect(container.textContent).toContain('writePdfNoTextLayer')
    await click('writePdfSearchPlaceholder')
    await fill(container.querySelector<HTMLInputElement>('[role="search"] input')!, 'needle')
    expect(button('writePdfNextMatch').disabled).toBe(true)
  })

  it('cancels old rendering and drops late text after a scale or file change', async () => {
    const pending = deferred<Awaited<ReturnType<MockPage['getTextContent']>>>()
    pdf.pages[0].getTextContent.mockReturnValueOnce(pending.promise)
    await draw(); await chooseZoom('2')
    const oldRender = pdf.pages[0].render.mock.results[0].value
    expect(oldRender.cancel).toHaveBeenCalledOnce()
    await act(async () => pending.resolve({ items: [{ str: 'STALE TEXT' }] }))
    expect(container.textContent).not.toContain('STALE TEXT')
    const next = makeDocument(['CURRENT FILE']); pdfjs.getDocument.mockReturnValueOnce({ promise: Promise.resolve(next), destroy: vi.fn() })
    await draw({ filePath: '/synthetic/next.pdf', mtimeMs: 2 })
    expect(container.querySelectorAll('[data-write-pdf-page]')).toHaveLength(1)
    expect(container.textContent).toContain('CURRENT FILE')
    expect(pdf.destroy).toHaveBeenCalledOnce()
    expect(pdfjs.layers.some(layer => layer.cancel.mock.calls.length > 0)).toBe(true)
  })
})
