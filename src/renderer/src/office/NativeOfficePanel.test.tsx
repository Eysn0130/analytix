// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NativeOfficePanel } from './NativeOfficePanel'
import { useNativeOfficeStore } from './native-office-store'
import { useNativeReferenceStore } from './native-reference-store'
import type { NativeOfficeRequest, NativeOfficeView, NativeOfficeSelection } from '@shared/native-office'

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
  useNativeReferenceStore.setState({ references: [] })
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
    expect(container.querySelector('.office-preview-state[role="status"]')).toBeNull()
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


const annotationSelection: NativeOfficeSelection = { kind:'text', documentId:view.objectId, version:view.revision, changeSequence:0, token:'selection-token', scope:'session-text-range-at-version-and-change-sequence', text:'Synthetic selection', capture:{capturedCharacters:19,totalCharacters:19,unit:'utf-16',truncated:false,complete:true} }
const annotationView = { ...view, editing:true, changeSequence:0, selection:annotationSelection }
async function renderAnnotation(next: NativeOfficeView, props: Partial<Parameters<typeof NativeOfficePanel>[0]> = {}) {
  request.mockImplementation(async (input: NativeOfficeRequest) => ({ ok:true, view: input.action === 'reference' ? { ...next, scope:{scopeId:'a'.repeat(48), editable:true, threadId:'thread'} } : next }))
  useNativeOfficeStore.setState({ target:{workspace:'/synthetic',path:next.path}, view:next, error:null })
  await act(async () => root!.render(createElement(NativeOfficePanel, {visible:true,threadId:'thread',...props})))
}
function namedButton(label: string): HTMLButtonElement {
  return [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.getAttribute('aria-label') === label || button.textContent === label)!
}
async function clickNamed(label: string) { await act(async () => namedButton(label).click()) }
async function fillNote(value: string) {
  const input = container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input,value)
    input.dispatchEvent(new Event('input',{bubbles:true}))
  })
}
describe('native document annotation workflow', () => {
  it('starts annotation without exposing any manual Office editing controls', async () => {
    await renderAnnotation(view)
    await clickNamed('标注文档')
    expect(request.mock.calls.some(([input]) => input.action === 'annotate')).toBe(true)
    for (const label of ['编辑文档','撤销','重做','字体','字号','局部替换内容']) expect(container.querySelector(`[aria-label="${label}"]`)).toBeNull()
    expect(container.textContent).toContain('点击“标注”')
  })
  it('adds a versioned scoped annotation with an optional note without replacing the composer or sending', async () => {
    const setInput = vi.fn(), onFocusConversation = vi.fn()
    await renderAnnotation(annotationView,{setInput,onFocusConversation})
    await fillNote('请澄清这一段')
    await clickNamed('加入对话')
    expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({editable:true,threadId:'thread',selectionToken:'selection-token'})
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({note:'请澄清这一段',revision:view.revision,editable:true,threadId:'thread'})
    expect(setInput).not.toHaveBeenCalled()
    expect(onFocusConversation).toHaveBeenCalledOnce()
    expect(container.textContent).toContain('尚未发送')
    expect(container.querySelector('[aria-label="nativeOfficeEditor"]')?.querySelector('[aria-label="标注备注"]')).toBeNull()
  })
  it('keeps readonly references discussion-only and does not invent a writable scope', async () => {
    await renderAnnotation({...view,selection:{...annotationSelection,scope:'current-view-text-only; no verified structural offset or durable anchor'}})
    expect(container.textContent).toContain('仅供讨论')
    await clickNamed('加入对话')
    expect(request.mock.calls.some(([input]) => input.action === 'reference')).toBe(false)
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({editable:false})
    expect(useNativeReferenceStore.getState().references[0].scopeId).toBeUndefined()
  })
  it('does not reuse a frozen selection after the document version changes', async () => {
    await renderAnnotation(annotationView)
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,revision:'c'.repeat(64),selection:{...annotationSelection,text:''}}}))
    expect(namedButton('加入对话')).toBeUndefined()
    expect(container.textContent).toContain('请在文档中选择')
  })
  it('keeps AI quick actions as annotation notes without adding a manual replacement flow', async () => {
    const setInput = vi.fn()
    await renderAnnotation(annotationView,{setInput,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'请润色这段话'}]})
    await clickNamed('润色')
    expect(useNativeReferenceStore.getState().references[0].note).toBe('请润色这段话')
    expect(setInput).not.toHaveBeenCalled()
    expect(request.mock.calls.some(([input]) => ['replace','format','undo','redo'].includes(input.action))).toBe(false)
  })
  it('does not add an annotation after its scope request finishes on another tab', async () => {
    await renderAnnotation(annotationView)
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'reference' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:annotationView}))
    await act(async () => namedButton('加入对话').click())
    const other = {...view,objectId:'c'.repeat(64),path:'/synthetic/other.docx'}
    await act(async () => useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:other.path},view:other}))
    await act(async () => finish({ok:true,view:{...annotationView,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread'}}}))
    expect(useNativeReferenceStore.getState().references).toHaveLength(0)
  })
})


it('adds a discussion annotation for a textless slide shape without treating its description as editable text', async () => {
  const shapeSelection: NativeOfficeSelection = { kind:'shapes',documentId:view.objectId,version:view.revision,changeSequence:0,token:'selection-token',scope:'page-and-shape-index-at-version-and-change-sequence',capture:{capturedCharacters:0,totalCharacters:0,unit:'utf-16',truncated:false,complete:true},shapes:[{pageIndex:1,shapeIndex:2,name:'Synthetic graphic',type:'image',text:''}] }
  await renderAnnotation({...annotationView,path:'/synthetic/deck.pptx',kind:'pptx',selection:shapeSelection})
  await fillNote('讨论这张图的位置')
  await clickNamed('加入对话')
  const reference = useNativeReferenceStore.getState().references[0]
  expect(reference).toMatchObject({editable:false,note:'讨论这张图的位置'})
  expect(reference.text).toContain('第2页 · 形状3')
  expect(reference.text).toContain('未捕获文本')
  expect(reference.scopeId).toBeUndefined()
  expect(request.mock.calls.some(([input]) => input.action === 'reference')).toBe(false)
})


it('retains the bounded selection and note when native blur reports an empty capture', async () => {
  await renderAnnotation(annotationView)
  await fillNote('保留这条备注')
  await act(async () => useNativeOfficeStore.getState().receive({...annotationView,selection:{kind:'unavailable',documentId:view.objectId,version:view.revision,changeSequence:0,scope:'engine selection interface unavailable in this view'}}))
  expect(container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')?.value).toBe('保留这条备注')
  await clickNamed('加入对话')
  expect(useNativeReferenceStore.getState().references[0]).toMatchObject({note:'保留这条备注',selection:annotationSelection,editable:true})
})
it('keeps an initially empty selection unavailable for annotation', async () => {
  await renderAnnotation({...annotationView,selection:{...annotationSelection,text:''}})
  expect(namedButton('加入对话')).toBeUndefined()
})

describe('compact native review toolbar', () => {
  it('shows only annotation before a change, and adds undo only when Main permits it', async () => {
    await renderAnnotation(annotationView,{fileActions:createElement('select',{'aria-label':'打开'},createElement('option',{},'打开'))})
    expect(container.querySelectorAll('[role="toolbar"] button')).toHaveLength(1)
    expect(container.querySelector('[role="toolbar"] [aria-label="打开"]')).not.toBeNull()
    expect(container.querySelector('[aria-label="保存文档"]')).toBeNull()
    expect(container.querySelector('[aria-label="撤销修改"]')).toBeNull()
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,canUndo:true}}))
    expect(container.querySelectorAll('[role="toolbar"] button')).toHaveLength(2)
    await clickNamed('撤销修改')
    expect(request.mock.calls.some(([input]) => input.action === 'undoChange')).toBe(true)
    expect(request.mock.calls.some(([input]) => input.action === 'save')).toBe(false)
  })
  it('renders protected before/after parts and applies a proposal without a separate Save command', async () => {
    const scope: NonNullable<NativeOfficeView['scope']> = {scopeId:'a'.repeat(48),threadId:'thread',selectionToken:'selection-token',baseRevision:view.revision,changeSequence:0,editable:true,parts:[{kind:'literal',text:'Before'},{kind:'protected',protectedRef:`protected_${'c'.repeat(48)}`}]}
    const proposal: NonNullable<NativeOfficeView['proposals']>[number] = {proposalId:'b'.repeat(48),status:'proposed',parts:[{kind:'literal',text:'After'},{kind:'protected',protectedRef:`protected_${'c'.repeat(48)}`}]}
    await renderAnnotation({...annotationView,scope,proposals:[proposal]})
    expect(container.querySelector('del[aria-label="修改前"]')?.textContent).toBe('Before[保留原始字段]')
    expect(container.querySelector('ins[aria-label="修改后"]')?.textContent).toBe('After[保留原始字段]')
    await clickNamed('应用修改')
    expect(request.mock.calls.find(([input]) => input.action === 'acceptProposal')?.[0]).toMatchObject({scopeId:scope.scopeId,proposalId:proposal.proposalId})
    expect(request.mock.calls.some(([input]) => input.action === 'save')).toBe(false)
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,scope,proposals:[proposal],appliedProposals:[proposal.proposalId],saving:true,dirty:true}}))
    expect(container.textContent).not.toContain('已更新')
    expect(container.textContent).toContain('结果待确认')
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,scope,proposals:[proposal],appliedProposals:[proposal.proposalId],canUndo:true}}))
    expect(container.textContent).toContain('已更新')
    expect(container.textContent).not.toContain('请保存')
  })
  it('offers save recovery only after failure or an unknown result', async () => {
    await renderAnnotation(annotationView)
    expect(namedButton('重试')).toBeUndefined()
    expect(namedButton('查询结果')).toBeUndefined()
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,dirty:true},error:'save_failed'}))
    await clickNamed('重试')
    expect(request.mock.calls.some(([input]) => input.action === 'save')).toBe(true)
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,saving:true,dirty:true},error:'unknown'}))
    await clickNamed('查询结果')
    expect(request.mock.calls.some(([input]) => input.action === 'saveStatus')).toBe(true)
  })
})
