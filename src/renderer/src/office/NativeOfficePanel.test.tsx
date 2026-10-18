// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NativeOfficePanel } from './NativeOfficePanel'
import { useNativeOfficeStore, nativeOfficeTargetKey } from './native-office-store'
import { useNativeReferenceStore } from './native-reference-store'
import type { NativeOfficeActionMenu, NativeOfficeFormat, NativeOfficeMenuTarget, NativeOfficeRequest, NativeOfficeView, NativeOfficeSelection, NativeOfficePlayback } from '@shared/native-office'
import { nativeOfficeActionChoiceSchema, nativeOfficeActionMenuSchema } from '@shared/native-office'

vi.mock('react-i18next', async importOriginal => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key })
}))

const view: NativeOfficeView = { objectId: 'a'.repeat(64), path: '/synthetic/report.docx', kind: 'docx', revision: 'b'.repeat(64), dirty: false, status: 'ready' }
let root: Root | null, container: HTMLDivElement
let rect: { x: number; y: number; width: number; height: number }
let request: ReturnType<typeof vi.fn>, unsubscribe: ReturnType<typeof vi.fn<() => void>>
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
  unsubscribe = vi.fn<() => void>()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { office: { request, onChange: () => unsubscribe } } })
  useNativeOfficeStore.setState({ target: null, view, error: null, proposalInFlight:false, annotationInputFrozen:false, manualDiscardAttempts:{}, manualOwners:{} })
  useNativeReferenceStore.setState({ references: [], drafts: {} })
  container = document.createElement('div'); container.className = 'ds-right-sidebar-pane'; document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root?.unmount()); root = null
  container.remove(); useNativeOfficeStore.setState({ target: null, view: null, error: null })
  Reflect.deleteProperty(window, 'analytix'); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('native preview dock geometry', () => {
  it.each([null, 'engine_unavailable', 'forbidden'])('keeps supplied file actions usable while preview is unavailable (%s)', async error => {
    useNativeOfficeStore.setState({ target: null, view: null, error })
    const openFile = vi.fn()
    await act(async () => root!.render(createElement(NativeOfficePanel, {
      visible: true,
      fileActions: createElement('button', { onClick: openFile }, 'Open file')
    })))
    const toolbar = container.querySelector('[role="toolbar"]')!
    const button = Array.from(toolbar.querySelectorAll('button')).find(button => button.textContent === 'Open file')!
    await act(async () => button.click())
    expect(openFile).toHaveBeenCalledOnce()
    expect(container.querySelector<HTMLButtonElement>('[aria-label="标注文档"]')!.disabled).toBe(true)
    const state = container.querySelector('[data-native-office-state]')!
    expect(state.contains(toolbar)).toBe(false)
    expect(state.parentElement).toBe(container.querySelector('[aria-label="nativeOfficeEditor"]')!.parentElement)
  })

  it.each(['docx', 'pptx'] as const)('keeps %s controls bound to existing native actions', async kind => {
    const preview = { ...view, kind }
    useNativeOfficeStore.setState({ target: { workspace: '/synthetic', path: preview.path }, view: preview })
    request.mockImplementation(async (input: NativeOfficeRequest) => ({ ok: true,
      view: input.action === 'annotate' ? { ...preview, editing: true } : preview }))
    await render(true)
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="标注文档"]')!.click())
    expect(request.mock.calls.find(([input]) => input.action === 'annotate')?.[0]).toEqual({
      action: 'annotate', objectId: view.objectId, revision: view.revision, expectedChangeSequence: 0
    })
    expect(container.querySelector<HTMLButtonElement>('[aria-label="标注文档"]')!.getAttribute('aria-pressed')).toBe('true')
    expect(boundsCalls().at(-1)).toEqual({ action: 'bounds', bounds: rect, appearance })
  })

  it('keeps annotate unavailable when a failed native view retains its metadata', async () => {
    useNativeOfficeStore.setState({ target: null, view: { ...view, status: 'error' }, error: 'engine_unavailable' })
    await render(true)
    const annotate = container.querySelector<HTMLButtonElement>('[aria-label="标注文档"]')!
    expect(annotate.disabled).toBe(true)
    await act(async () => annotate.click())
    expect(request.mock.calls.some(([input]) => input.action === 'annotate')).toBe(false)
    expect(container.querySelector('[data-native-office-state][role="alert"]')).not.toBeNull()
  })

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
    expect(container.querySelector('[data-native-office-state][role="status"]')).toBeNull()
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

  it.each(['/synthetic', '/synthetic-home/.analytix/default_workspace'])('shows a typed refusal without inferring its cause from %s', async workspace => {
    useNativeOfficeStore.setState({ target: { workspace, path: `${workspace}/report.docx` }, view: null })
    request.mockResolvedValueOnce({ ok: false, view: null, error: 'forbidden' })
    await render(true)
    const alert = container.querySelector('[role="alert"]')!
    expect(alert.textContent).toContain('nativeOfficeForbidden')
    expect(alert.textContent).not.toContain('nativeOfficeOperationFailed')
    expect(useNativeOfficeStore.getState().error).toBe('forbidden')
    expect(request.mock.calls.filter(call => call[0].action === 'open')).toHaveLength(1)
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


const annotationSelection: Extract<NativeOfficeSelection,{kind:'text'}> = { kind:'text', documentId:view.objectId, version:view.revision, changeSequence:0, token:'selection-token', scope:'session-text-range-at-version-and-change-sequence', text:'Synthetic selection', capture:{capturedCharacters:19,totalCharacters:19,unit:'utf-16',truncated:false,complete:true} }
const annotationView = { ...view, editing:true, changeSequence:0, selection:annotationSelection }
async function renderAnnotation(next: NativeOfficeView, props: Partial<Parameters<typeof NativeOfficePanel>[0]> = {}) {
  request.mockImplementation(async (input: NativeOfficeRequest) => {
    if (input.action === 'viewControl') return {ok:false,error:'unavailable',view:useNativeOfficeStore.getState().view}
    if (['beginManualEdit','format','discardManualEdit'].includes(input.action)) return {ok:false,error:'native_input_unavailable',view:next}
    if (input.action === 'reference' && input.saveFirst && next.dirty) return {ok:false,error:'unsaved_changes',view:next}
    return {ok:true,view:input.action === 'reference' ? {...next,scope:{scopeId:'a'.repeat(48),editable:input.editable,threadId:input.threadId,selectionToken:input.selectionToken,baseRevision:input.revision,changeSequence:input.expectedChangeSequence,parts:[]}} : next}
  })
  useNativeOfficeStore.setState({ target:{workspace:'/synthetic',path:next.path}, view:next, error:null })
  await act(async () => root!.render(createElement(NativeOfficePanel, {visible:true,threadId:'thread',...props})))
}
function namedButton(label: string): HTMLButtonElement {
  return [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.getAttribute('aria-label') === label || button.textContent === label)!
}
async function clickNamed(label: string) { await act(async () => namedButton(label).click()) }
async function openNotes() {
  if (namedButton('编辑标注备注')) await clickNamed('编辑标注备注')
}
async function fillNote(value: string) {
  await openNotes()
  const input = container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input,value)
    input.dispatchEvent(new Event('input',{bubbles:true}))
  })
}
describe('native document annotation workflow', () => {
  it('lets view controls replace a nearby menu without waiting for its cancellation reply',async()=>{
    let menuRequested!:(target:any)=>void
    const choices:Array<(choice:{actionId:string|null})=>void>=[]
    window.analytix.office.onMenuRequested=handler=>{menuRequested=handler;return ()=>{}}
    const menu=vi.fn(()=>new Promise<{actionId:string|null}>(resolve=>{choices.push(resolve)}));window.analytix.office.showActionMenu=menu
    await renderAnnotation({...annotationView,previewView:{zoom:100,fit:false,page:null,pages:null}})
    await act(async()=>menuRequested({objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,pointer:{interactionId:'interaction_id',selectionToken:annotationSelection.token!}}))
    await clickNamed('预览视图');expect(menu).toHaveBeenCalledTimes(2)
    await act(async()=>choices[0]({actionId:'quote'}))
    expect(request.mock.calls.some(([r])=>r.action==='reference')).toBe(false)
    await act(async()=>choices[1]({actionId:'view-zoom-25'}))
    expect(request.mock.calls.some(([r])=>r.action==='viewControl'&&r.control==='set-zoom'&&r.zoom===25)).toBe(true)
  })

  it('keeps the keyboard/native menu usable while a cancelled nearby menu reply is still pending',async()=>{
    let menuRequested!:(target:any)=>void
    const choices:Array<(choice:{actionId:string|null})=>void>=[]
    window.analytix.office.onMenuRequested=handler=>{menuRequested=handler;return ()=>{}}
    const menu=vi.fn(()=>new Promise<{actionId:string|null}>(resolve=>{choices.push(resolve)}));window.analytix.office.showActionMenu=menu
    await renderAnnotation(annotationView)
    const target={objectId:view.objectId,revision:view.revision,expectedChangeSequence:0}
    await act(async()=>menuRequested({...target,pointer:{interactionId:'interaction_id',selectionToken:annotationSelection.token!}}))
    await act(async()=>menuRequested(target));expect(menu).toHaveBeenCalledTimes(2)
    await act(async()=>choices[0]({actionId:null}))
    await act(async()=>menuRequested(target));expect(menu).toHaveBeenCalledTimes(2)
    await act(async()=>choices[1]({actionId:'quote'}))
    expect(request.mock.calls.filter(([r])=>r.action==='reference')).toHaveLength(1)
  })

  it('uses the fresh pointer token even when the menu intent arrives before the next React render',async()=>{
    let menuRequested!:(target:any)=>void
    window.analytix.office.onMenuRequested=handler=>{menuRequested=handler;return ()=>{}}
    const menu=vi.fn(async()=>({actionId:'quote'}));window.analytix.office.showActionMenu=menu
    await renderAnnotation(annotationView)
    const fresh={...annotationSelection,token:'fresh_selection',text:annotationSelection.text}
    const next={...annotationView,selection:fresh}
    request.mockImplementation(async(input:NativeOfficeRequest)=>({ok:true,view:input.action==='reference'?{...next,scope:{scopeId:'a'.repeat(48),editable:false,threadId:'thread',selectionToken:input.selectionToken,baseRevision:view.revision,changeSequence:0,parts:[]}}:next}))
    await act(async()=>{
      useNativeOfficeStore.getState().receive(next)
      menuRequested({objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,pointer:{interactionId:'interaction_id',selectionToken:fresh.token}})
    })
    expect(menu).toHaveBeenCalledWith(expect.objectContaining({pointer:{interactionId:'interaction_id',selectionToken:fresh.token}}))
    expect(request.mock.calls.find(([r])=>r.action==='reference')?.[0]).toMatchObject({selectionToken:fresh.token,editable:false})
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({selection:{token:fresh.token}})
    expect(namedButton('nativeActionMore')).toBeDefined()
    expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
  })
  it('rejects a pointer intent for a stale token and a choice after the live native selection changes',async()=>{
    let menuRequested!:(target:any)=>void,choose!:(choice:{actionId:string|null})=>void
    window.analytix.office.onMenuRequested=handler=>{menuRequested=handler;return ()=>{}}
    const menu=vi.fn(()=>new Promise<{actionId:string|null}>(resolve=>{choose=resolve}));window.analytix.office.showActionMenu=menu
    await renderAnnotation(annotationView)
    const target={objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,pointer:{interactionId:'interaction_id',selectionToken:'wrong_token'}}
    await act(async()=>menuRequested(target));expect(menu).not.toHaveBeenCalled()
    await act(async()=>menuRequested({...target,pointer:{...target.pointer,selectionToken:annotationSelection.token!}}))
    await act(async()=>useNativeOfficeStore.getState().receive({...annotationView,selection:{...annotationSelection,token:'same_text_other_range'}}))
    await act(async()=>choose({actionId:'quote'}))
    expect(request.mock.calls.some(([r])=>r.action==='reference')).toBe(false)
  })

  it('opens compatible menu ids for existing custom actions and starts editing from preview in one click', async () => {
    const onSubmitPrompt = vi.fn()
    window.analytix.office.showActionMenu = vi.fn(async payload => {
      expect(nativeOfficeActionMenuSchema.safeParse(payload).success).toBe(true)
      return {actionId:'task:0'}
    })
    await renderAnnotation({...annotationView,editing:false},{onSubmitPrompt,quickActions:[{id:'中文 custom action',label:'润色',mode:'edit',prompt:'请润色'}]})
    request.mockImplementation(async (input:NativeOfficeRequest) => ({ok:true,view:input.action === 'reference'
      ? {...annotationView,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,parts:[]}} : annotationView}))
    await clickNamed('nativeActionMore')
    expect(request.mock.calls.map(([input])=>input.action).filter(action=>action==='annotate'||action==='reference')).toEqual(['annotate','reference'])
    expect(onSubmitPrompt).toHaveBeenCalledExactlyOnceWith('请润色',[expect.objectContaining({editable:true,selection:annotationSelection})])
  })
  it('uses the same action handler for a native menu choice without sending on menu open', async () => {
    const onSubmitPrompt=vi.fn()
    let menuRequested!: (target:any)=>void, choose!: (choice:{actionId:string|null})=>void
    window.analytix.office.onMenuRequested=handler=>{menuRequested=handler;return ()=>{}}
    window.analytix.office.showActionMenu=vi.fn(()=>new Promise<{actionId:string|null}>(resolve=>{choose=resolve}))
    await renderAnnotation(annotationView,{onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'请润色'}]})
    await act(async()=>menuRequested({objectId:view.objectId,revision:view.revision,expectedChangeSequence:0}))
    expect(onSubmitPrompt).not.toHaveBeenCalled()
    expect(request.mock.calls.some(([input])=>input.action==='reference')).toBe(false)
    await act(async()=>choose({actionId:'task:0'}))
    expect(onSubmitPrompt).toHaveBeenCalledExactlyOnceWith('请润色',[expect.objectContaining({selection:annotationSelection})])
  })
  it('drops a native menu choice if the selection changes while the menu is open', async () => {
    const onSubmitPrompt=vi.fn()
    let choose!: (choice:{actionId:string|null})=>void
    window.analytix.office.showActionMenu=vi.fn(()=>new Promise<{actionId:string|null}>(resolve=>{choose=resolve}))
    await renderAnnotation(annotationView,{onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'请润色'}]})
    await clickNamed('nativeActionMore')
    await act(async()=>useNativeOfficeStore.getState().receive({...annotationView,selection:{...annotationSelection,token:'different-token',text:'Another'}}))
    await act(async()=>choose({actionId:'task:0'}))
    expect(onSubmitPrompt).not.toHaveBeenCalled()
  })
  it('keeps annotation separate from native manual editing and formatting', async () => {
    await renderAnnotation(view)
    await clickNamed('标注文档')
    expect(request.mock.calls.some(([input]) => input.action === 'annotate')).toBe(true)
    for (const label of ['编辑文档','撤销','重做','字体','字号','局部替换内容']) expect(container.querySelector(`[aria-label="${label}"]`)).toBeNull()
    expect(namedButton('nativeOfficeManualEdit')).toBeDefined()
    expect(request.mock.calls.some(([input]) => input.action === 'beginManualEdit' || input.action === 'format')).toBe(false)
    expect(container.textContent).toContain('点击“标注”')
  })
  it('adds a versioned scoped annotation with an optional note without replacing the composer or sending', async () => {
    const setInput = vi.fn(), onFocusConversation = vi.fn()
    await renderAnnotation(annotationView,{setInput,onFocusConversation})
    await openNotes()
    await fillNote('请澄清这一段')
    await clickNamed('加入对话')
    expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({editable:false,threadId:'thread',selectionToken:'selection-token'})
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({note:'请澄清这一段',revision:view.revision,editable:false,threadId:'thread'})
    expect(setInput).not.toHaveBeenCalled()
    expect(onFocusConversation).toHaveBeenCalledOnce()
    expect(container.textContent).toContain('尚未发送')
    expect(container.querySelector('[aria-label="nativeOfficeEditor"]')?.querySelector('[aria-label="标注备注"]')).toBeNull()
  })
  it('keeps readonly references discussion-only and does not invent a writable scope', async () => {
    await renderAnnotation({...view,selection:{...annotationSelection,scope:'current-view-text-only; no verified structural offset or durable anchor'}})
    await openNotes()
    expect(container.textContent).toContain('仅供讨论')
    await clickNamed('加入对话')
    expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({editable:false,selectionToken:annotationSelection.token})
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({editable:false,scopeId:'a'.repeat(48)})
  })
  it('does not reuse a frozen selection after the document version changes', async () => {
    await renderAnnotation(annotationView)
    await openNotes()
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,revision:'c'.repeat(64),selection:{...annotationSelection,text:''}}}))
    expect(namedButton('加入对话')).toBeUndefined()
    expect(container.textContent).toContain('请在文档中选择')
  })
  it('dispatches an explicit quick task with only its frozen reference, preserving the composer', async () => {
    const setInput = vi.fn(), onSubmitPrompt = vi.fn()
    await renderAnnotation(annotationView,{setInput,onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'请润色这段话'}]})
    await openNotes()
    expect(onSubmitPrompt).not.toHaveBeenCalled()
    await clickNamed('润色')
    expect(onSubmitPrompt).toHaveBeenCalledExactlyOnceWith('请润色这段话', [expect.objectContaining({threadId:'thread',selection:annotationSelection,editable:true})])
    expect(setInput).not.toHaveBeenCalled()
    expect(request.mock.calls.some(([input]) => ['replace','format','undo','redo'].includes(input.action))).toBe(false)
  })
  it('does not consume or evict existing chips when a quick task acquires a new scope', async () => {
    const onSubmitPrompt = vi.fn()
    await renderAnnotation(annotationView,{onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'润色'}]})
    await openNotes()
    for (let i=0;i<8;i++) useNativeReferenceStore.getState().add({threadId:'thread',workspace:'/synthetic',path:view.path,objectId:view.objectId,revision:view.revision,selection:annotationSelection,label:`ref${i}`,text:`draft${i}`,scopeId:'b'.repeat(48),editable:true})
    const before = useNativeReferenceStore.getState().references.map(reference => reference.id)
    await clickNamed('润色')
    expect(onSubmitPrompt).toHaveBeenCalledOnce()
    expect(useNativeReferenceStore.getState().references.map(reference => reference.id)).toEqual(before)
    expect(useNativeReferenceStore.getState().references.every(reference => !reference.editable && !reference.scopeId)).toBe(true)
  })
  it('rejects an old scope response before it can restore an old document or dispatch a task', async () => {
    const onSubmitPrompt = vi.fn()
    await renderAnnotation(annotationView,{onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'润色'}]})
    await openNotes()
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'reference' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:annotationView}))
    await act(async () => namedButton('润色').click())
    const changed = {...annotationView,revision:'c'.repeat(64),selection:undefined}
    await act(async () => useNativeOfficeStore.getState().receive(changed))
    await act(async () => finish({ok:true,view:{...annotationView,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,parts:[]}}}))
    expect(onSubmitPrompt).not.toHaveBeenCalled()
    expect(useNativeOfficeStore.getState().view?.revision).toBe(changed.revision)
  })
  it('keeps polling single-flight across collapse and rejects responses superseded by an event', async () => {
    vi.useFakeTimers()
    try {
      const scoped = {...annotationView,scope:{scopeId:'a'.repeat(48),threadId:'thread',editable:true} as NativeOfficeView['scope']}
      let finish!: (value:unknown) => void
      request.mockImplementation((input:NativeOfficeRequest) => input.action === 'proposals' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:scoped}))
      useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:view.path},view:scoped})
      const renderPanel = async (visible:boolean) => { await act(async () => root!.render(createElement(NativeOfficePanel,{visible,threadId:'thread'}))) }
      await renderPanel(true)
      await act(async () => vi.advanceTimersByTimeAsync(6000))
      expect(request.mock.calls.filter(([input]) => input.action === 'proposals')).toHaveLength(1)
      await renderPanel(false); await renderPanel(true)
      expect(request.mock.calls.filter(([input]) => input.action === 'proposals')).toHaveLength(1)
      await act(async () => root!.unmount())
      root = createRoot(container)
      await renderPanel(true)
      expect(request.mock.calls.filter(([input]) => input.action === 'proposals')).toHaveLength(1)
      await act(async () => finish({ok:true,view:scoped}))
      await act(async () => vi.advanceTimersByTimeAsync(2000))
      expect(request.mock.calls.filter(([input]) => input.action === 'proposals')).toHaveLength(2)
      const newer = {...scoped,canUndo:true}
      await act(async () => useNativeOfficeStore.getState().receive(newer))
      await act(async () => finish({ok:true,view:scoped}))
      expect(useNativeOfficeStore.getState().view?.canUndo).toBe(true)
    } finally { vi.useRealTimers() }
  })
  it('retains a note across collapse and a stale document version without silently rebinding it', async () => {
    await renderAnnotation(annotationView)
    await openNotes()
    await fillNote('不要丢失这条备注')
    await act(async () => root!.render(createElement(NativeOfficePanel,{visible:false,threadId:'thread'})))
    await act(async () => root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'thread'})))
    await openNotes()
    expect(container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')?.value).toBe('不要丢失这条备注')
    await act(async () => useNativeOfficeStore.getState().receive({...annotationView,revision:'c'.repeat(64),selection:undefined}))
    await openNotes()
    expect(container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')?.value).toBe('不要丢失这条备注')
    expect(namedButton('加入对话').disabled).toBe(true)
  })
  it('does not add an annotation after its scope request finishes on another tab', async () => {
    await renderAnnotation(annotationView)
    await openNotes()
    useNativeReferenceStore.getState().add({threadId:'other-thread',workspace:'/synthetic',path:view.path,objectId:view.objectId,revision:view.revision,selection:annotationSelection,label:'old',text:'keep text',note:'keep note',scopeId:'b'.repeat(48),editable:true})
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'reference' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:annotationView}))
    await act(async () => namedButton('加入对话').click())
    const other = {...view,objectId:'c'.repeat(64),path:'/synthetic/other.docx'}
    await act(async () => useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:other.path},view:other}))
    await act(async () => finish({ok:true,view:{...annotationView,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,parts:[]}}}))
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({threadId:'other-thread',text:'keep text',note:'keep note',editable:false})
    expect(useNativeReferenceStore.getState().references[0].scopeId).toBeUndefined()
  })
})


it('adds a discussion annotation for a textless slide shape without treating its description as editable text', async () => {
  const shapeSelection: NativeOfficeSelection = { kind:'shapes',documentId:view.objectId,version:view.revision,changeSequence:0,token:'selection-token',scope:'page-and-shape-index-at-version-and-change-sequence',capture:{capturedCharacters:0,totalCharacters:0,unit:'utf-16',truncated:false,complete:true},shapes:[{pageIndex:1,shapeIndex:2,name:'Synthetic graphic',type:'image',text:''}] }
  await renderAnnotation({...annotationView,path:'/synthetic/deck.pptx',kind:'pptx',selection:shapeSelection})
  await openNotes()
  await fillNote('讨论这张图的位置')
  await clickNamed('加入对话')
  const reference = useNativeReferenceStore.getState().references[0]
  expect(reference).toMatchObject({editable:false,note:'讨论这张图的位置'})
  expect(reference.text).toContain('第2页 · 形状3')
  expect(reference.text).toContain('未捕获文本')
  expect(reference.scopeId).toBe('a'.repeat(48))
  expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({editable:false})
})


it('retains the bounded selection and note when native blur reports an empty capture', async () => {
  await renderAnnotation(annotationView)
  await openNotes()
  await fillNote('保留这条备注')
  await act(async () => useNativeOfficeStore.getState().receive({...annotationView,selection:{kind:'unavailable',documentId:view.objectId,version:view.revision,changeSequence:0,scope:'engine selection interface unavailable in this view'}}))
  expect(container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')?.value).toBe('保留这条备注')
  await clickNamed('加入对话')
  expect(useNativeReferenceStore.getState().references[0]).toMatchObject({note:'保留这条备注',selection:annotationSelection,editable:false})
})
it('keeps an initially empty selection unavailable for annotation', async () => {
  await renderAnnotation({...annotationView,selection:{...annotationSelection,text:''}})
  await openNotes()
  expect(namedButton('加入对话')).toBeUndefined()
})

describe('compact native review toolbar', () => {
  it('shows annotation, manual edit and view controls, and adds undo only when Main permits it', async () => {
    await renderAnnotation(annotationView,{fileActions:createElement('select',{'aria-label':'打开'},createElement('option',{},'打开'))})
    const actions = () => [...container.querySelectorAll<HTMLButtonElement>('[role="toolbar"] button')].map(button => button.getAttribute('aria-label')).sort()
    expect(actions()).toEqual(['标注文档','nativeActionMore','编辑标注备注','nativeOfficeManualEdit','预览视图'].sort())
    expect(namedButton('标注文档').disabled).toBe(true)
    expect(namedButton('标注文档').getAttribute('aria-pressed')).toBe('true')
    expect(namedButton('nativeOfficeManualEdit').disabled).toBe(false)
    expect(namedButton('nativeOfficeManualEdit').getAttribute('aria-pressed')).toBe('false')
    expect(namedButton('nativeOfficeManualSave')).toBeUndefined()
    expect(namedButton('nativeOfficeManualDiscard')).toBeUndefined()
    expect(container.querySelector<HTMLButtonElement>('[data-native-view-controls]')!.disabled).toBe(false)
    expect(container.querySelector('[role="toolbar"] [aria-label="打开"]')).not.toBeNull()
    expect(container.querySelector('[aria-label="保存文档"]')).toBeNull()
    expect(container.querySelector('[aria-label="撤销修改"]')).toBeNull()
    await act(async () => useNativeOfficeStore.setState({view:{...annotationView,canUndo:true}}))
    expect(actions()).toEqual(['标注文档','nativeActionMore','编辑标注备注','nativeOfficeManualEdit','撤销修改','预览视图'].sort())
    expect(namedButton('撤销修改').disabled).toBe(false)
    await clickNamed('撤销修改')
    expect(request.mock.calls.some(([input]) => input.action === 'undoChange' && input.threadId === 'thread')).toBe(true)
    expect(request.mock.calls.some(([input]) => input.action === 'save')).toBe(false)
  })
  it('renders protected-local raw before/after text and applies a proposal without a separate Save command', async () => {
    const scope: NonNullable<NativeOfficeView['scope']> = {scopeId:'a'.repeat(48),threadId:'thread',selectionToken:'selection-token',baseRevision:view.revision,changeSequence:0,editable:true,parts:[{kind:'literal',text:'Before'},{kind:'protected',protectedRef:`protected_${'c'.repeat(48)}`}]}
    const proposal: NonNullable<NativeOfficeView['proposals']>[number] = {proposalId:'b'.repeat(48),status:'proposed',parts:[{kind:'literal',text:'After'},{kind:'protected',protectedRef:`protected_${'c'.repeat(48)}`}]}
    const localReviews=[{proposalId:proposal.proposalId,beforeText:'Before 原始字段',afterText:'After 原始字段'}]
    await renderAnnotation({...annotationView,scope,proposals:[proposal],localReviews})
    expect(container.querySelector('del[aria-label="修改前"]')?.textContent).toBe('Before 原始字段')
    expect(container.querySelector('ins[aria-label="修改后"]')?.textContent).toBe('After 原始字段')
    expect(container.textContent).not.toContain('[保留原始字段]')
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


it('disables proposal application when its protected-local raw review is missing',async()=>{
  const scope:NonNullable<NativeOfficeView['scope']>={scopeId:'a'.repeat(48),threadId:'thread',selectionToken:'selection-token',baseRevision:view.revision,changeSequence:0,editable:true,parts:[{kind:'literal',text:'Projected before'}]}
  const proposal:NonNullable<NativeOfficeView['proposals']>[number]={proposalId:'b'.repeat(48),status:'proposed',parts:[{kind:'protected',protectedRef:`protected_${'c'.repeat(48)}`}]}
  await renderAnnotation({...annotationView,scope,proposals:[proposal],localReviews:[]})
  expect(namedButton('应用修改').disabled).toBe(true)
  expect(container.textContent).toContain('原文暂不可核实')
  expect(container.textContent).not.toContain('[保留原始字段]')
  await clickNamed('应用修改')
  expect(request.mock.calls.some(([input])=>input.action==='acceptProposal')).toBe(false)
  await act(async()=>useNativeOfficeStore.setState({view:{...annotationView,scope,proposals:[proposal],localReviews:[{proposalId:'d'.repeat(48),beforeText:'Unrelated raw text',afterText:'Other proposal'}]}}))
  expect(namedButton('应用修改').disabled).toBe(true)
  expect(container.textContent).not.toContain('Unrelated raw text')
})


it('shows only thread-owned durable recovery and routes cancellation and undo retry explicitly',async()=>{
  const pending:NonNullable<NonNullable<NativeOfficeView['recovery']>['pending']>={changeId:'e'.repeat(64),threadId:'thread',proposalId:'f'.repeat(48),baseRevision:view.revision,revision:'',status:'prepared',beforeText:'原文私有字段',afterText:'修改后私有字段',saveOperationId:'save_core_0001',undoOperationId:'undo_core_0001',canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-09-15T00:00:00Z',savedAt:''}
  await renderAnnotation({...annotationView,recovery:{current:null,pending}})
  expect(container.querySelector('[aria-label="已记录的修改前"]')?.textContent).toBe('原文私有字段')
  await clickNamed('取消未保存的修改')
  expect(request.mock.calls.find(([input])=>input.action==='cancelChange')?.[0]).toMatchObject({threadId:'thread',changeId:pending.changeId,objectId:view.objectId,revision:view.revision})
  await act(async()=>useNativeOfficeStore.setState({view:{...annotationView,recovery:{current:null,pending:{...pending,threadId:'other-thread'}}}}))
  expect(container.textContent).not.toContain('原文私有字段')
  expect(namedButton('取消未保存的修改')).toBeUndefined()
  const current={...pending,status:'unknown' as const,revision:view.revision,canCancel:false,canRetryUndo:true}
  await act(async()=>useNativeOfficeStore.setState({view:{...annotationView,canUndo:true,recovery:{current,pending:null}}}))
  expect(namedButton('完成撤销').disabled).toBe(false)
  await clickNamed('完成撤销')
  expect(request.mock.calls.find(([input])=>input.action==='undoChange')?.[0]).toMatchObject({threadId:'thread',objectId:view.objectId,revision:view.revision})
})


it('never renders a proposal raw review carried by another thread scope',async()=>{
  const scope:NonNullable<NativeOfficeView['scope']>={scopeId:'a'.repeat(48),threadId:'other-thread',selectionToken:'selection-token',baseRevision:view.revision,changeSequence:0,editable:true,parts:[{kind:'literal',text:'Projected before'}]}
  const proposal:NonNullable<NativeOfficeView['proposals']>[number]={proposalId:'b'.repeat(48),status:'proposed',parts:[{kind:'literal',text:'Projected after'}]}
  await renderAnnotation({...annotationView,scope,proposals:[proposal],localReviews:[{proposalId:proposal.proposalId,beforeText:'foreign-thread-private-before',afterText:'foreign-thread-private-after'}]})
  expect(container.textContent).not.toContain('foreign-thread-private-before')
  expect(container.textContent).not.toContain('foreign-thread-private-after')
  expect(namedButton('应用修改')?.disabled ?? true).toBe(true)
})


it('offers explicit resume only for the owning thread and capability, while queries never submit it',async()=>{
  const pending:NonNullable<NonNullable<NativeOfficeView['recovery']>['pending']>={changeId:'e'.repeat(64),threadId:'thread',proposalId:'f'.repeat(48),baseRevision:view.revision,revision:'c'.repeat(64),status:'unknown',beforeText:'Before',afterText:'After',saveOperationId:'save_core_0001',undoOperationId:'undo_core_0001',canUndo:false,canCancel:false,canRetryUndo:false,canResume:true,createdAt:'2026-09-15T00:00:00Z',savedAt:''}
  const resumable={...annotationView,recovery:{current:null,pending}}
  await renderAnnotation(resumable)
  expect(namedButton('继续保存此修改').disabled).toBe(false)
  expect(request.mock.calls.some(([input])=>input.action==='resumeChange')).toBe(false)
  await act(async()=>useNativeOfficeStore.setState({view:resumable,error:'unknown'}))
  await clickNamed('查询结果')
  expect(request.mock.calls.some(([input])=>input.action==='saveStatus')).toBe(true)
  expect(request.mock.calls.some(([input])=>input.action==='resumeChange')).toBe(false)
  await clickNamed('继续保存此修改')
  expect(request.mock.calls.filter(([input])=>input.action==='resumeChange')).toEqual([[{action:'resumeChange',threadId:'thread',changeId:pending.changeId,objectId:view.objectId,revision:view.revision,expectedChangeSequence:0}]])
  await act(async()=>useNativeOfficeStore.setState({view:{...annotationView,recovery:{current:null,pending:{...pending,canResume:false}}}}))
  expect(namedButton('继续保存此修改')).toBeUndefined()
  await act(async()=>useNativeOfficeStore.setState({view:{...annotationView,recovery:{current:null,pending:{...pending,threadId:'other-thread'}}}}))
  expect(namedButton('继续保存此修改')).toBeUndefined()
})

function persistedAnnotation(note:string, threadId='thread'): NonNullable<NativeOfficeView['annotation']> {
  return {objectId:view.objectId,threadId,note,draftRevision:'c'.repeat(64),sourceRevision:view.revision,updatedAt:'2026-09-15T00:00:00Z'}
}
function noteInput() { return container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')! }
const annotationDraftKey=JSON.stringify(['thread','/synthetic',view.path])
describe('persisted native annotation races',()=>{
  it('restores only note text without selection, token, editable scope or automatic save',async()=>{
    await renderAnnotation({...view,annotation:persistedAnnotation('恢复的备注')})
    await openNotes()
    expect(noteInput().value).toBe('恢复的备注')
    expect(namedButton('加入对话').disabled).toBe(true)
    expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].selection).toBeUndefined()
    expect(useNativeOfficeStore.getState().view?.scope).toBeUndefined()
    expect(useNativeOfficeStore.getState().view?.editing).not.toBe(true)
    expect(request.mock.calls.some(([input])=>['annotationSave','annotate','reference'].includes(input.action))).toBe(false)
  })
  it('keeps newer input when a delayed annotation load arrives while a save is pending',async()=>{
    await renderAnnotation(annotationView)
    await openNotes()
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='annotationSave'?new Promise(()=>{}):Promise.resolve({ok:true,view:annotationView}))
    await fillNote('刚输入的新备注')
    await act(async()=>useNativeOfficeStore.getState().receive({...annotationView,annotation:persistedAnnotation('较早读取的备注')},null))
    expect(noteInput().value).toBe('刚输入的新备注')
    expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].dirty).toBe(true)
    expect(request.mock.calls.filter(([input])=>input.action==='annotationSave').map(([input])=>input)).toEqual([{action:'annotationSave',objectId:view.objectId,threadId:'thread',note:'刚输入的新备注',sourceRevision:view.revision}])
  })
  it('does not replace newer acknowledged input with an older save acknowledgement',async()=>{
    await renderAnnotation(annotationView)
    await openNotes()
    const replies=new Map<string,(value:unknown)=>void>()
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='annotationSave'?new Promise(resolve=>replies.set(input.note,resolve)):Promise.resolve({ok:true,view:annotationView}))
    await fillNote('first');await fillNote('latest')
    await act(async()=>replies.get('latest')!({ok:true,view:{...annotationView,annotation:persistedAnnotation('latest')}}))
    expect(noteInput().value).toBe('latest')
    expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].dirty).toBe(false)
    await act(async()=>replies.get('first')!({ok:true,view:{...annotationView,annotation:persistedAnnotation('first')}}))
    expect(noteInput().value).toBe('latest')
    expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].note).toBe('latest')
    expect(request.mock.calls.filter(([input])=>input.action==='annotationSave')).toHaveLength(2)
  })
  it('isolates a switched thread from old saves and foreign annotation loads',async()=>{
    await renderAnnotation(annotationView)
    await openNotes()
    let finish!:(value:unknown)=>void
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='annotationSave'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:annotationView}))
    await fillNote('thread-one-private-note')
    const next={...annotationView,annotation:persistedAnnotation('thread-two-note','thread-two')}
    request.mockImplementation(async()=>({ok:true,view:next}))
    await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'thread-two'})))
    await openNotes()
    expect(noteInput().value).toBe('thread-two-note')
    await act(async()=>finish({ok:true,view:{...annotationView,annotation:persistedAnnotation('thread-one-private-note')}}))
    expect(noteInput().value).toBe('thread-two-note')
    await act(async()=>useNativeOfficeStore.getState().receive({...annotationView,annotation:persistedAnnotation('foreign private load')},null))
    expect(noteInput().value).toBe('thread-two-note')
    expect(container.textContent).not.toContain('thread-one-private-note')
    expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].note).toBe('thread-one-private-note')
    expect(request.mock.calls.filter(([input])=>input.action==='annotationSave')).toHaveLength(1)
  })
})


it('freeze notification disables notes synchronously and thaw restores note input without native edit authority',async()=>{
  let freeze!:(frozen:boolean)=>void
  const unsubscribeFreeze=vi.fn()
  window.analytix.office.onAnnotationInputFreeze=handler=>{freeze=handler;handler(false);return unsubscribeFreeze}
  await renderAnnotation({...view,annotation:persistedAnnotation('readonly note')})
  await openNotes()
  await act(async()=>freeze(true))
  expect(noteInput().disabled).toBe(true)
  await fillNote('blocked keystroke')
  expect(request.mock.calls.filter(([input])=>input.action==='annotationSave')).toHaveLength(0)
  expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].note).toBe('readonly note')
  await act(async()=>freeze(false))
  expect(noteInput().disabled).toBe(false)
  await fillNote('after thaw')
  expect(request.mock.calls.filter(([input])=>input.action==='annotationSave').map(([input])=>input.note)).toEqual(['after thaw'])
  expect(useNativeOfficeStore.getState().view?.editing).not.toBe(true)
  expect(useNativeOfficeStore.getState().view?.scope).toBeUndefined()
  expect(request.mock.calls.some(([input])=>['annotate','reference','acceptProposal'].includes(input.action))).toBe(false)
  await act(async()=>{root!.unmount();root=null})
  expect(unsubscribeFreeze).toHaveBeenCalledOnce()
})


describe('Request Modify draft preparation', () => {
  it('admits an explicit edit from preview and prepares its original scoped selection without sending or applying', async () => {
    const onPrepareDraft = vi.fn(() => true), onSubmitPrompt = vi.fn(), onFocusConversation = vi.fn()
    await renderAnnotation({...annotationView,editing:false},{onPrepareDraft,onSubmitPrompt,onFocusConversation})
    await openNotes()
    await fillNote('PRIVATE LOCAL NOTE')
    const recaptured = {...annotationView,selection:{...annotationSelection,token:'recaptured-token'}}
    request.mockImplementation(async (input:NativeOfficeRequest) => ({ok:true,view:input.action === 'reference'
      ? {...recaptured,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:input.selectionToken,baseRevision:view.revision,changeSequence:0,parts:[]}}
      : recaptured}))
    await clickNamed('请求修改')
    expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({editable:true,selectionToken:annotationSelection.token,threadId:'thread'})
    expect(request.mock.calls.map(([input]) => input.action).filter(action=>action==='annotate'||action==='reference')).toEqual(['annotate','reference'])
    expect(request.mock.calls.some(([input]) => ['save','acceptProposal'].includes(input.action))).toBe(false)
    expect(onPrepareDraft).toHaveBeenCalledExactlyOnceWith('请根据所选内容提出修改建议。',[expect.objectContaining({editable:true,scopeId:'a'.repeat(48),selection:annotationSelection,note:'PRIVATE LOCAL NOTE'})])
    expect(onSubmitPrompt).not.toHaveBeenCalled()
    expect(onFocusConversation).toHaveBeenCalledOnce()
    expect(container.textContent).toContain('尚未发送')
  })
  it.each(['hide','target','thread','selection'] as const)('drops a late scope after %s changes', async change => {
    const onPrepareDraft = vi.fn(() => true), onFocusConversation = vi.fn()
    await renderAnnotation(annotationView,{onPrepareDraft,onFocusConversation})
    await openNotes()
    let finish!: (value: unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'reference' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:annotationView}))
    await act(async () => namedButton('请求修改').click())
    if (change === 'target') await act(async () => useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:'/synthetic/other.docx'},view:{...view,objectId:'c'.repeat(64),path:'/synthetic/other.docx'}}))
    else if (change === 'selection') await act(async () => useNativeOfficeStore.getState().receive({...annotationView,selection:{...annotationSelection,token:'different-token',text:'New selection'}}))
    else await act(async () => root!.render(createElement(NativeOfficePanel,{visible:change !== 'hide',threadId:change === 'thread' ? 'other' : 'thread',onPrepareDraft,onFocusConversation})))
    await act(async () => finish({ok:true,view:{...annotationView,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,parts:[]}}}))
    expect(onPrepareDraft).not.toHaveBeenCalled()
    expect(onFocusConversation).not.toHaveBeenCalled()
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })
  it('rejects a readonly reply to an explicit modify request without upgrading it', async () => {
    const onPrepareDraft = vi.fn(() => true)
    await renderAnnotation(annotationView,{onPrepareDraft})
    await openNotes()
    request.mockImplementation(async () => ({ok:true,view:{...annotationView,scope:{scopeId:'a'.repeat(48),editable:false,threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,parts:[]}}}))
    await clickNamed('请求修改')
    expect(onPrepareDraft).not.toHaveBeenCalled()
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })
  it.each(['draft','quick'] as const)('shares edit admission for %s and accepts its normal pre-reply recapture', async mode => {
    const onPrepareDraft = vi.fn(() => true), onSubmitPrompt = vi.fn()
    await renderAnnotation({...annotationView,editing:false},{onPrepareDraft,onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'请润色'}]})
    await openNotes()
    const recaptured = {...annotationView,selection:{...annotationSelection,token:'recaptured-token'}}
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'annotate' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:input.action === 'reference'
      ? {...recaptured,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:input.selectionToken,baseRevision:view.revision,changeSequence:0,parts:[]}}
      : recaptured}))
    await act(async () => namedButton(mode === 'draft' ? '请求修改' : '润色').click())
    await act(async () => useNativeOfficeStore.getState().receive(recaptured))
    await act(async () => finish({ok:true,view:recaptured}))
    expect(request.mock.calls.find(([input])=>input.action==='reference')?.[0]).toMatchObject({editable:true,selectionToken:annotationSelection.token})
    expect(mode === 'draft' ? onPrepareDraft : onSubmitPrompt).toHaveBeenCalledOnce()
    expect(mode === 'draft' ? onSubmitPrompt : onPrepareDraft).not.toHaveBeenCalled()
    expect(request.mock.calls.some(([input])=>['save','acceptProposal'].includes(input.action))).toBe(false)
  })
  it.each(['draft','quick'] as const)('drops %s after a newer selection even when its own recapture became the latest view', async mode => {
    const onPrepareDraft = vi.fn(() => true), onSubmitPrompt = vi.fn()
    await renderAnnotation({...annotationView,editing:false},{onPrepareDraft,onSubmitPrompt,quickActions:[{id:'polish',label:'润色',mode:'edit',prompt:'请润色'}]})
    await openNotes()
    const recaptured = {...annotationView,selection:{...annotationSelection,token:'recaptured-token'}}
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'annotate' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:recaptured}))
    await act(async () => namedButton(mode === 'draft' ? '请求修改' : '润色').click())
    // Identical text can belong to another range; only the opaque token identifies it.
    await act(async () => useNativeOfficeStore.getState().receive({...annotationView,selection:{...annotationSelection,token:'user-changed-token'}}))
    await act(async () => useNativeOfficeStore.getState().receive(recaptured))
    await act(async () => finish({ok:true,view:recaptured}))
    expect(request.mock.calls.some(([input])=>input.action==='reference')).toBe(false)
    expect(onPrepareDraft).not.toHaveBeenCalled();expect(onSubmitPrompt).not.toHaveBeenCalled()
  })
  it('does not restore expired edit permission from a late scope receipt', async () => {
    const onPrepareDraft = vi.fn(() => true)
    await renderAnnotation(annotationView,{onPrepareDraft})
    await openNotes()
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'reference' ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:annotationView}))
    await act(async () => namedButton('请求修改').click())
    await act(async () => useNativeOfficeStore.getState().receive({...annotationView,editing:false}))
    await act(async () => finish({ok:true,view:{...annotationView,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,parts:[]}}}))
    expect(onPrepareDraft).not.toHaveBeenCalled()
    expect(useNativeOfficeStore.getState().view?.editing).toBe(false)
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })
  it.each(['annotate','reference'] as const)('shows a current %s refusal without restoring its older view or preparing a draft', async stage => {
    const onPrepareDraft = vi.fn(() => true)
    await renderAnnotation({...annotationView,editing:stage==='reference'},{onPrepareDraft})
    await openNotes()
    const current = useNativeOfficeStore.getState().view
    request.mockImplementation(async (input:NativeOfficeRequest) => input.action === stage
      ? {ok:false,error:'package_changed',view:{...annotationView,revision:'c'.repeat(64)}}
      : {ok:true,view:annotationView})
    await clickNamed('请求修改')
    expect(useNativeOfficeStore.getState().error).toBe('package_changed')
    expect(useNativeOfficeStore.getState().view).toBe(current)
    expect(container.querySelector('[role="alert"]')).not.toBeNull()
    expect(onPrepareDraft).not.toHaveBeenCalled()
    expect(useNativeReferenceStore.getState().references).toEqual([])
    expect(request.mock.calls.some(([input])=>['save','acceptProposal'].includes(input.action))).toBe(false)
  })
  it.each(['incomplete','unanchored'] as const)('keeps %s captures discussion-only rather than inventing edit admission', async capability => {
    const selected:NativeOfficeSelection = capability === 'incomplete'
      ? {...annotationSelection,capture:{...annotationSelection.capture!,complete:false,truncated:true}}
      : {...annotationSelection,scope:'current-view-text-only; no verified structural offset or durable anchor'}
    const onPrepareDraft = vi.fn(() => true)
    await renderAnnotation({...annotationView,editing:false,selection:selected},{onPrepareDraft})
    await openNotes()
    expect(namedButton('请求修改').disabled).toBe(true)
    await clickNamed('请求修改');await clickNamed('加入对话')
    expect(request.mock.calls.some(([input])=>input.action==='annotate')).toBe(false)
    expect(request.mock.calls.find(([input])=>input.action==='reference')?.[0]).toMatchObject({editable:false})
    expect(onPrepareDraft).not.toHaveBeenCalled()
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({editable:false})
  })
})


describe('native readonly view controls in the unified toolbar',()=>{
  async function mountPreview(kind:'docx'|'pptx'='docx',missing=false) {
    let current:NativeOfficeView={...view,kind,path:kind==='pptx'?'/synthetic/deck.pptx':view.path,
      ...(missing?{}:{previewView:{zoom:100,fit:false,page:kind==='pptx'?1:null,pages:kind==='pptx'?3:null}})}
    const target={workspace:'/synthetic',path:current.path}
    const changed={handler:(_value:NativeOfficeView|null)=>{}}
    window.analytix.office.onChange=handler=>{changed.handler=next=>{if(next) current=next;handler(next)};return () => unsubscribe()}
    const menu=vi.fn(async(_input:Parameters<typeof window.analytix.office.showActionMenu>[0])=>({actionId:null as string|null}))
    window.analytix.office.showActionMenu=menu
    request.mockImplementation(async(input:NativeOfficeRequest)=>{
      if(input.action==='viewControl'&&current.previewView) {
        current={...current,previewView:{...current.previewView,
          ...(input.control==='set-zoom'?{zoom:input.zoom!,fit:false}:{}),
          ...(input.control==='fit'?{fit:true}:{}),
          ...(input.control==='next-page'?{page:3}:{}),...(input.control==='previous-page'?{page:1}:{})}}
      }
      return {ok:true,view:current}
    })
    useNativeOfficeStore.setState({target,view:current,error:null})
    await render(true)
    return {menu,changed,get current(){return current},button:()=>container.querySelector<HTMLButtonElement>('[data-native-view-controls]')!}
  }
  const clickMenu=async(button:HTMLButtonElement)=>{await act(async()=>{button.click();await new Promise(resolve=>setTimeout(resolve,0))})}
  it('uses the existing native menu and exact presets while displaying only acknowledged values',async()=>{
    const f=await mountPreview()
    for(const zoom of [25,50,100,200]) {
      f.menu.mockResolvedValueOnce({actionId:`view-zoom-${zoom}`})
      await clickMenu(f.button())
      expect(request).toHaveBeenCalledWith({action:'viewControl',objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,control:'set-zoom',zoom})
      expect(container.querySelector('[data-native-preview-zoom]')!.textContent).toBe(`${zoom}%`)
      expect(f.menu.mock.calls.at(-1)?.[0]).toMatchObject({objectId:view.objectId,revision:view.revision,actions:expect.arrayContaining([25,50,100,200].map(value=>({id:`view-zoom-${value}`,label:`${value}%`,enabled:true})))})
    }
    expect(request.mock.calls.some(([input])=>input.action==='annotate'||input.editable===true)).toBe(false)
    f.menu.mockResolvedValueOnce({actionId:'view-fit'});await clickMenu(f.button())
    expect(container.querySelector('[data-native-preview-zoom]')!.textContent).toBe('writeImageFitShort')
    expect(container.querySelector('details[data-native-view-controls]')).toBeNull()
  })
  it('keeps acknowledged readonly view controls available after internal AI edit admission',async()=>{
    const f=await mountPreview()
    await act(async()=>f.changed.handler({...f.current,editing:true}))
    expect(f.button().disabled).toBe(false)
    f.menu.mockResolvedValueOnce({actionId:'view-zoom-25'});await clickMenu(f.button())
    expect(request).toHaveBeenCalledWith(expect.objectContaining({action:'viewControl',control:'set-zoom',zoom:25}))
    expect(container.querySelector('[data-native-preview-zoom]')!.textContent).toBe('25%')
    expect(useNativeOfficeStore.getState().view?.editing).toBe(true)
  })
  it('reports actual PPTX page counts and refuses a disabled terminal-page action',async()=>{
    const f=await mountPreview('pptx')
    f.menu.mockResolvedValueOnce({actionId:'view-next-page'});await clickMenu(f.button())
    expect(container.querySelector('[data-native-preview-page]')!.textContent).toBe('3 / 3')
    const before=request.mock.calls.filter(([input])=>input.action==='viewControl').length
    f.menu.mockResolvedValueOnce({actionId:'view-next-page'});await clickMenu(f.button())
    expect(f.menu.mock.calls.at(-1)?.[0]).toMatchObject({actions:expect.arrayContaining([{id:'view-next-page',label:'下一页',enabled:false}])})
    expect(request.mock.calls.filter(([input])=>input.action==='viewControl')).toHaveLength(before)
  })
  it('keeps unavailable counts honest and retries only the real view reader',async()=>{
    const f=await mountPreview('pptx',true)
    expect(container.querySelector('[data-native-preview-page]')!.textContent).toBe('—')
    expect(container.querySelector('[data-native-preview-zoom]')!.textContent).toBe('视图不可用')
    f.menu.mockResolvedValueOnce({actionId:'view-read'});await clickMenu(f.button())
    expect(f.menu.mock.calls.at(-1)?.[0]).toMatchObject({actions:expect.arrayContaining([{id:'view-read',label:'重试视图',enabled:true},{id:'view-zoom-25',label:'25%',enabled:false}])})
    expect(request.mock.calls.some(([input])=>input.action==='viewControl'&&input.control==='read')).toBe(true)
  })
  it('restores focus after native menu cancellation without closing the viewer',async()=>{
    const f=await mountPreview();request.mockClear()
    let cancel!:(value:{actionId:string|null})=>void
    f.menu.mockImplementation(()=>new Promise(resolve=>{cancel=resolve}))
    await clickMenu(f.button());expect(f.button().disabled).toBe(true)
    const outside=document.createElement('input');document.body.append(outside);outside.focus()
    await act(async()=>cancel({actionId:null}))
    expect(f.button().disabled).toBe(false)
    expect(document.activeElement).toBe(f.button());outside.remove()
    expect(request).not.toHaveBeenCalled()
    expect(useNativeOfficeStore.getState().target?.path).toBe(view.path)
  })
  it('does not let an older control reply replace a newer view event',async()=>{
    const f=await mountPreview();let finish!:(value:unknown)=>void
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='viewControl'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:f.current}))
    f.menu.mockResolvedValueOnce({actionId:'view-zoom-25'});await clickMenu(f.button())
    await act(async()=>f.changed.handler({...f.current,previewView:{zoom:200,fit:false,page:null,pages:null}}))
    await act(async()=>finish({ok:true,view:{...f.current,previewView:{zoom:25,fit:false,page:null,pages:null}}}))
    expect(container.querySelector('[data-native-preview-zoom]')!.textContent).toBe('200%')
  })
  it('drops a pending old-tab control and preserves the new tab and its focus',async()=>{
    const f=await mountPreview();let finish!:(value:unknown)=>void
    const next:NativeOfficeView={...view,objectId:'d'.repeat(64),revision:'e'.repeat(64),path:'/synthetic/next.docx',previewView:{zoom:50,fit:false,page:null,pages:null}}
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='viewControl'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:input.action==='open'&&input.path===next.path?next:f.current}))
    f.menu.mockResolvedValueOnce({actionId:'view-zoom-25'});await clickMenu(f.button())
    await act(async()=>useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:next.path},view:next}))
    const focus=document.createElement('input');document.body.append(focus);focus.focus()
    await act(async()=>finish({ok:true,view:{...f.current,previewView:{zoom:25,fit:false,page:null,pages:null}}}))
    expect(useNativeOfficeStore.getState().view?.objectId).toBe(next.objectId)
    expect(container.querySelector('[data-native-preview-zoom]')!.textContent).toBe('50%')
    expect(document.activeElement).toBe(focus);focus.remove()
  })
})

describe('native PPT playback in the existing view toolbar', () => {
  const idle: NativeOfficePlayback = {generation:2,sequence:4,phase:'idle',slideIndex:null,slideCount:null}
  const running: NativeOfficePlayback = {generation:3,sequence:5,phase:'running',slideIndex:0,slideCount:3}
  async function mountPlayback(playback: NativeOfficePlayback | null = idle, extra: Partial<NativeOfficeView> = {}, protectedPresent?: NativeOfficeView['protectedPresent']) {
    let current: NativeOfficeView = {...view,kind:'pptx',path:'/synthetic/deck.pptx',
      previewView:{zoom:100,fit:false,page:3,pages:3},...extra,...(playback ? {playback} : {}),...(protectedPresent ? {protectedPresent} : {})}
    let changed!: (next: NativeOfficeView | null) => void
    window.analytix.office.onChange = handler => {changed = next => {if(next) current = next;handler(next)};return unsubscribe}
    const menu = vi.fn(async (_input: Parameters<typeof window.analytix.office.showActionMenu>[0]) => ({actionId:null as string | null}))
    window.analytix.office.showActionMenu = menu
    request.mockImplementation(async (input: NativeOfficeRequest) => {
      if (input.action === 'viewControl' && input.control === 'play-read' && !current.playback) current = {...current,playback:idle}
      return {ok:true,view:current}
    })
    useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:current.path,...(protectedPresent ? {protectedPresent} : {})},view:current,error:null})
    await act(async () => root!.render(createElement(NativeOfficePanel, {visible:true,...(protectedPresent ? {presentSelector:protectedPresent} : {})})))
    return {menu,get current(){return current},emit:changed,button:() => container.querySelector<HTMLButtonElement>('[data-native-view-controls]')!,
      choose:async (actionId:string) => {menu.mockResolvedValueOnce({actionId});await act(async () => {container.querySelector<HTMLButtonElement>('[data-native-view-controls]')!.click();await Promise.resolve()})}}
  }
  const playbackCalls = () => request.mock.calls.map(([input]) => input as NativeOfficeRequest)
    .filter(input => input.action === 'viewControl' && input.control.startsWith('play-'))

  it('initializes from the real play reader and waits for an acknowledged start', async () => {
    const f = await mountPlayback(null)
    expect(playbackCalls().some(input => input.action === 'viewControl' && input.control === 'play-read' && !('playbackGeneration' in input))).toBe(true)
    expect(container.querySelector('[data-native-playback-state]')!.textContent).toBe('放映未开始')
    let finish!: (value: unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'viewControl' && input.control === 'play-start'
      ? new Promise(resolve => {finish = resolve}) : Promise.resolve({ok:true,view:f.current}))
    await f.choose('view-play-start')
    expect(request).toHaveBeenCalledWith({action:'viewControl',objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,control:'play-start',playbackGeneration:2})
    expect(container.querySelector('[data-native-playback-state]')!.textContent).toBe('放映未开始')
    expect(container.querySelector('[data-native-playback-slide]')).toBeNull()
    await act(async () => finish({ok:true,view:{...f.current,playback:running}}))
    expect(container.querySelector('[data-native-playback-slide]')!.textContent).toBe('1 / 3')
    expect(container.querySelector('[data-native-playback-state]')!.textContent).toBe('放映中 · 1 / 3')
    expect(useNativeOfficeStore.getState().view?.previewView?.page).toBe(3)
    expect(document.activeElement).toBe(f.button())
  })

  it('finishes a delayed editor view read before initializing missing playback state', async () => {
    let current:NativeOfficeView={...view,kind:'pptx',path:'/synthetic/deck.pptx'}
    const target={workspace:'/synthetic',path:current.path}
    let finishRead!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => {
      if (input.action === 'viewControl' && input.control === 'read') return new Promise(resolve => {finishRead=resolve})
      if (input.action === 'viewControl' && input.control === 'play-read') current={...current,playback:idle}
      return Promise.resolve({ok:true,view:current})
    })
    useNativeOfficeStore.setState({target,view:current,error:null})
    await render(true)
    expect(finishRead).toBeDefined()
    expect(playbackCalls()).toHaveLength(0)
    current={...current,previewView:{zoom:50,fit:false,page:2,pages:3}}
    await act(async () => finishRead({ok:true,view:current}))
    expect(playbackCalls().some(input => input.action === 'viewControl' && input.control === 'play-read')).toBe(true)
    expect(useNativeOfficeStore.getState().view?.previewView).toMatchObject({zoom:50,page:2})
    expect(useNativeOfficeStore.getState().view?.playback).toEqual(idle)
  })

  it('keeps effects distinct from slides and sends the acknowledged generation for pause, resume and end', async () => {
    const f = await mountPlayback(running)
    await f.choose('view-play-next-effect')
    await f.choose('view-play-next-slide')
    expect(playbackCalls().map(input => input.action === 'viewControl' ? input.control : '')).toEqual(['play-next-effect','play-next-slide'])
    expect(playbackCalls().every(input => input.action === 'viewControl' && input.playbackGeneration === 3)).toBe(true)
    await f.choose('view-play-previous-slide')
    expect(f.menu.mock.calls.at(-1)?.[0].actions.find(item => item.id === 'view-play-previous-slide')?.enabled).toBe(false)
    expect(playbackCalls()).toHaveLength(2)
    await act(async () => f.emit({...f.current,playback:{...running,sequence:6,slideIndex:2}}))
    await f.choose('view-play-next-slide')
    expect(f.menu.mock.calls.at(-1)?.[0].actions.find(item => item.id === 'view-play-next-slide')?.enabled).toBe(false)
    expect(playbackCalls()).toHaveLength(2)
    await clickNamed('暂停放映')
    expect(playbackCalls().at(-1)).toMatchObject({control:'play-pause',playbackGeneration:3})
    expect(namedButton('继续放映')).toBeUndefined()
    await act(async () => f.emit({...f.current,playback:{...running,sequence:7,phase:'paused',slideIndex:2}}))
    await clickNamed('继续放映')
    expect(playbackCalls().at(-1)).toMatchObject({control:'play-resume',playbackGeneration:3})
    await clickNamed('结束放映')
    expect(playbackCalls().at(-1)).toMatchObject({control:'play-end',playbackGeneration:3})
    await act(async () => f.emit({...f.current,playback:{...idle,generation:4,sequence:8}}))
    expect(namedButton('结束放映')).toBeUndefined()
    expect(document.activeElement).toBe(f.button())
    await f.choose('view-play-current')
    expect(playbackCalls().at(-1)).toMatchObject({control:'play-current',playbackGeneration:4})
  })

  it('blocks zoom and annotation while playing without discarding editable metadata or notes', async () => {
    const selection:NativeOfficeSelection={kind:'shapes',documentId:view.objectId,version:view.revision,changeSequence:0,
      scope:'page-and-shape-index-at-version-and-change-sequence',shapes:[{pageIndex:2,shapeIndex:0,name:'shape',type:'text',text:'Synthetic text'}],token:'selection-token'}
    const f = await mountPlayback(running,{editing:true,selection,canUndo:true})
    await f.choose('view-zoom-25')
    expect(f.menu.mock.calls.at(-1)?.[0].actions.find(item => item.id === 'view-zoom-25')?.enabled).toBe(false)
    expect(request.mock.calls.some(([input]) => input.action === 'viewControl' && input.control === 'set-zoom')).toBe(false)
    expect(namedButton('标注文档').disabled).toBe(true)
    expect(namedButton('撤销修改').disabled).toBe(true)
    expect(container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')!.disabled).toBe(true)
    expect(useNativeOfficeStore.getState().view).toMatchObject({editing:true,selection,canUndo:true})
    expect(request.mock.calls.some(([input]) => ['annotate','reference','annotationSave','undoChange','save'].includes(input.action))).toBe(false)
  })

  it('refuses start and current on dirty documents and keeps saving controls disabled', async () => {
    const f = await mountPlayback(idle,{dirty:true})
    await f.choose('view-play-start');await f.choose('view-play-current')
    expect(f.menu.mock.calls.at(-1)?.[0].actions.filter(item => ['view-play-start','view-play-current'].includes(item.id)).every(item => !item.enabled)).toBe(true)
    expect(playbackCalls()).toHaveLength(0)
    await act(async () => f.emit({...f.current,saving:true}))
    expect(f.button().disabled).toBe(true)
    expect(useNativeOfficeStore.getState().view?.dirty).toBe(true)
  })

  it('preserves actual unsupported and error state when a playback reader rejects', async () => {
    const f = await mountPlayback({...idle,phase:'error'})
    request.mockRejectedValueOnce(new Error('transport unavailable'))
    await clickNamed('刷新放映状态')
    expect(container.querySelector('[data-native-playback-state]')!.textContent).toContain('放映失败')
    expect(useNativeOfficeStore.getState().view?.playback?.phase).toBe('error')
    expect(useNativeOfficeStore.getState().view?.previewView?.zoom).toBe(100)
    await act(async () => f.emit({...f.current,playback:{...idle,phase:'unsupported',sequence:5}}))
    await f.choose('view-play-start')
    expect(f.menu.mock.calls.at(-1)?.[0].actions.find(item => item.id === 'view-play-start')?.enabled).toBe(false)
    expect(container.querySelector('[data-native-playback-state]')!.textContent).toContain('不支持放映')
  })

  it('shows a recoverable read failure even when no view is returned', async () => {
    const f = await mountPlayback()
    request.mockResolvedValueOnce({ok:false,view:null,error:'unavailable'})
    await f.choose('view-play-read')
    expect(container.querySelector('[data-native-playback-state][role="alert"]')!.textContent).toContain('暂不可确认')
    expect(namedButton('刷新放映状态').disabled).toBe(false)
    expect(useNativeOfficeStore.getState().view?.previewView?.zoom).toBe(100)
  })

  it('uses the real Main timeout projection with cached idle state and requires reopening', async () => {
    const f = await mountPlayback(idle,{editing:true})
    request.mockResolvedValueOnce({ok:false,error:'presentation_start_timeout',view:{...f.current,status:'error',
      playback:idle,error:'presentation_start_timeout'}})
    await f.choose('view-play-start')
    expect(container.querySelector('[data-native-playback-state][role="alert"]')!.textContent).toContain('重新打开预览')
    expect(container.querySelector('[data-native-playback-state]')!.textContent).not.toContain('放映未开始')
    expect(useNativeOfficeStore.getState().view?.playback).toEqual(idle)
    expect(container.querySelector('[data-native-office-state][role="alert"]')!.textContent).toContain('重新打开预览')
    expect(namedButton('nativeOfficeRetry')).toBeDefined()
    expect(namedButton('刷新放映状态')).toBeUndefined()
  })

  it.each(['presentation_failed','presentation_unsupported'] as const)('prioritizes a Main %s error over a retained ready idle snapshot', async code => {
    const f = await mountPlayback()
    await act(async () => f.emit({...f.current,error:code}))
    const state = container.querySelector('[data-native-playback-state][role="alert"]')!
    expect(state.textContent).toContain(code === 'presentation_failed' ? '放映失败' : '不支持放映')
    expect(state.textContent).not.toContain('放映未开始')
    expect(useNativeOfficeStore.getState().view?.playback).toEqual(idle)
    expect(namedButton('刷新放映状态').disabled).toBe(false)
    await clickNamed('刷新放映状态')
    expect(playbackCalls().at(-1)).toMatchObject({control:'play-read'})
    expect(playbackCalls().at(-1)).not.toHaveProperty('playbackGeneration')
  })

  it('also displays a presentation error received through the current global error channel', async () => {
    await mountPlayback()
    await act(async () => useNativeOfficeStore.setState({error:'presentation_failed'}))
    expect(container.querySelector('[data-native-playback-state][role="alert"]')!.textContent).toContain('放映失败')
    expect(container.querySelector('[data-native-playback-state]')!.textContent).not.toContain('放映未开始')
    expect(namedButton('刷新放映状态').disabled).toBe(false)
  })

  it.each([false,true])('offers a real reopen for a failed Main surface even with retained editing=%s and no global error', async editing => {
    const f = await mountPlayback(idle,{editing})
    await act(async () => useNativeOfficeStore.setState({view:{...f.current,status:'error',error:'presentation_failed'},error:null}))
    expect(container.querySelector('[data-native-playback-state][role="alert"]')!.textContent).toContain('请重新打开预览')
    expect(namedButton('刷新放映状态')).toBeUndefined()
    expect(namedButton('nativeOfficeRetry')).toBeDefined()
    request.mockClear()
    await clickNamed('nativeOfficeRetry')
    expect(request.mock.calls.some(([input]) => input.action === 'open')).toBe(true)
    expect(useNativeOfficeStore.getState().view?.playback).toEqual(idle)
  })

  it('keeps the timeout reopen reachable from view metadata when no global error is present', async () => {
    const f = await mountPlayback()
    await act(async () => useNativeOfficeStore.setState({view:{...f.current,status:'error',error:'presentation_start_timeout'},error:null}))
    expect(container.querySelector('[data-native-office-state][role="alert"]')!.textContent).toContain('重新打开预览')
    expect(namedButton('nativeOfficeRetry')).toBeDefined()
    expect(namedButton('刷新放映状态')).toBeUndefined()
  })

  it('does not claim cached running playback on an errored surface and retains the real open retry', async () => {
    const f = await mountPlayback(running)
    await act(async () => f.emit({...f.current,status:'error',error:'engine_unavailable'}))
    expect(container.querySelector('[data-native-playback-state][role="alert"]')!.textContent).toContain('放映状态未确认，请重新打开预览')
    expect(container.querySelector('[data-native-playback-slide]')).toBeNull()
    expect(namedButton('暂停放映')).toBeUndefined()
    expect(namedButton('结束放映')).toBeUndefined()
    expect(namedButton('刷新放映状态')).toBeUndefined()
    expect(namedButton('nativeOfficeRetry')).toBeDefined()
    expect(useNativeOfficeStore.getState().view?.playback).toEqual(running)
  })

  it('does not replace a newer playback event or clear a successor after a late old-owner failure', async () => {
    const f = await mountPlayback(running)
    let finish!: (value:unknown) => void
    request.mockImplementation((input:NativeOfficeRequest) => input.action === 'viewControl'
      ? new Promise(resolve => {finish=resolve}) : Promise.resolve({ok:true,view:f.current}))
    await f.choose('view-play-next-effect')
    await act(async () => f.emit({...f.current,playback:{...running,sequence:10,slideIndex:1}}))
    await act(async () => finish({ok:true,view:{...f.current,playback:{...running,sequence:9,slideIndex:0}}}))
    expect(useNativeOfficeStore.getState().view?.playback).toMatchObject({sequence:10,slideIndex:1})
    await f.choose('view-play-next-effect')
    const next:NativeOfficeView={...view,objectId:'d'.repeat(64),revision:'e'.repeat(64),path:'/synthetic/next.docx',previewView:{zoom:50,fit:false,page:null,pages:null}}
    request.mockImplementation(async () => ({ok:true,view:next}))
    await act(async () => useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:next.path},view:next}))
    await act(async () => finish({ok:false,view:null,error:'presentation_failed'}))
    expect(useNativeOfficeStore.getState().view).toMatchObject({objectId:next.objectId,previewView:{zoom:50}})
    expect(container.querySelector('[data-native-playback-state]')).toBeNull()
  })

  it('allows bounded playback for protected Present while retaining read-only restrictions', async () => {
    const protectedPresent={threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),
      viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
    const notes={ordinary:{note:'preserved',dirty:true}}
    useNativeReferenceStore.setState({drafts:notes})
    const f = await mountPlayback(idle,{},protectedPresent)
    await f.choose('view-play-current')
    expect(playbackCalls().at(-1)).toMatchObject({control:'play-current',playbackGeneration:2})
    expect(namedButton('标注文档')).toBeUndefined()
    expect(container.querySelector('[aria-label="文档标注"]')).toBeNull()
    expect(request.mock.calls.some(([input]) => ['open','captureSelection','annotationSave','reference','proposals','annotate'].includes(input.action))).toBe(false)
    expect(useNativeReferenceStore.getState().drafts).toEqual(notes)
  })
})

it('opens Present Office by selector alone and preserves ordinary notes without edit or transfer controls', async () => {
  const protectedPresent = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  const preview = {...view,protectedPresent}, notes = {ordinary:{note:'preserved',dirty:true}}
  useNativeReferenceStore.setState({drafts:notes})
  useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:view.path,protectedPresent},view:preview})
  request.mockImplementation(async () => ({ok:true,view:preview}))
  await act(async () => root!.render(createElement('div', {},createElement(NativeOfficePanel,{visible:false}),createElement(NativeOfficePanel,{visible:true,presentSelector:protectedPresent}))))
  expect(request.mock.calls.some(([input]) => input.action === 'hide')).toBe(false)
  expect(request.mock.calls.find(([input]) => input.action === 'openPresent')?.[0]).toEqual({action:'openPresent',protectedPresent,bounds:rect,appearance})
  expect(request.mock.calls.some(([input]) => ['open','captureSelection','annotationSave','reference','proposals','annotate'].includes(input.action))).toBe(false)
  expect(container.querySelector('[aria-label="标注文档"]')).toBeNull();expect(container.querySelector('[aria-label="文档标注"]')).toBeNull()
  expect(namedButton('nativeOfficeManualEdit')).toBeUndefined();expect(namedButton('nativeOfficeManualDiscard')).toBeUndefined();expect(namedButton('nativeOfficeSaveAndReference')).toBeUndefined()
  expect(container.querySelector('[aria-label="预览视图"]')).not.toBeNull();expect(useNativeReferenceStore.getState().drafts).toEqual(notes)
})

it('an unmounted Present Panel cannot hide or overwrite a successor when its open completes late',async()=>{
  const first={threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  const second={...first,viewId:'22222222-2222-4222-8222-222222222222'},firstView={...view,protectedPresent:first},secondView={...view,objectId:'f'.repeat(64),protectedPresent:second}
  let finish!:(value:{ok:boolean;view:NativeOfficeView})=>void
  request.mockImplementation(async(input:NativeOfficeRequest)=>input.action==='openPresent'&&input.protectedPresent.viewId===first.viewId?new Promise(resolve=>{finish=resolve}):{ok:true,view:secondView})
  useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:view.path,protectedPresent:first},view:firstView})
  await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,presentSelector:first})))
  expect(finish).toBeDefined();await act(async()=>root!.unmount());root=createRoot(container)
  useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:view.path,protectedPresent:second},view:secondView})
  await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,presentSelector:second})))
  const hides=request.mock.calls.filter(([input])=>input.action==='hide').length
  await act(async()=>finish({ok:true,view:firstView}))
  expect(request.mock.calls.filter(([input])=>input.action==='hide')).toHaveLength(hides)
  expect(useNativeOfficeStore.getState().view).toEqual(secondView)
})

// Synthetic frozen-contract consumers; these fixtures are not native capability evidence.
const manualAuthority:NonNullable<NativeOfficeView['manual']>={version:2,origin:'manual',changeId:'e'.repeat(64),baseRevision:view.revision,generation:7,frozen:false}
const manualPending:NonNullable<NonNullable<NativeOfficeView['recovery']>['pending']>={version:2,origin:'manual',changeId:manualAuthority.changeId,threadId:'thread',
  baseRevision:view.revision,revision:'',status:'prepared',saveOperationId:'save_manual_001',undoOperationId:'undo_manual_001',
  canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-10-15T00:00:00Z',savedAt:''}
const dirtyManual:NativeOfficeView={...annotationView,manual:manualAuthority,dirty:true,changeSequence:2,acknowledgedSequence:1,
  selection:{...annotationSelection,changeSequence:2},recovery:{current:null,pending:manualPending}}
const recapturedSelection:NativeOfficeSelection={...annotationSelection,version:'c'.repeat(64),changeSequence:0,token:'fresh-saved-token',text:'Saved native range'}
const recapturedView:NativeOfficeView={...annotationView,revision:recapturedSelection.version,dirty:false,changeSequence:0,acknowledgedSequence:0,selection:recapturedSelection,
  scope:{scopeId:'d'.repeat(48),threadId:'thread',selectionToken:recapturedSelection.token!,baseRevision:recapturedSelection.version,changeSequence:0,editable:false,parts:[]}}
const proposalScope:NonNullable<NativeOfficeView['scope']>={scopeId:'a'.repeat(48),threadId:'thread',selectionToken:annotationSelection.token!,baseRevision:view.revision,changeSequence:0,editable:true,parts:[]}
const proposed:NonNullable<NativeOfficeView['proposals']>[number]={proposalId:'b'.repeat(48),status:'proposed',parts:[{kind:'literal',text:'Projected suggestion'}]}
const proposalReview={proposalId:proposed.proposalId,beforeText:'PRIVATE BEFORE',afterText:'PRIVATE AFTER'}

describe('Word manual authority and explicit actions',()=>{
  it('keeps a refused native begin honest without adding another input or formatting surface',async()=>{
    const onPrepareDraft=vi.fn(()=>true),onSubmitPrompt=vi.fn()
    await renderAnnotation({...view,editing:false},{onPrepareDraft,onSubmitPrompt})
    await clickNamed('nativeOfficeManualEdit')
    expect(request.mock.calls.filter(([input])=>input.action==='beginManualEdit')).toEqual([[{action:'beginManualEdit',objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,threadId:'thread'}]])
    expect(useNativeOfficeStore.getState().view?.manual).toBeUndefined()
    expect(namedButton('nativeOfficeManualEdit').getAttribute('aria-pressed')).toBe('false')
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('nativeOfficeManualUnavailable')
    expect(container.querySelector('[data-native-office-state]')).toBeNull()
    expect(container.querySelector('[contenteditable="true"]')).toBeNull()
    expect(container.querySelector('[aria-label="字体"], [aria-label="字号"]')).toBeNull()
    expect(request.mock.calls.some(([input])=>['format','reference','save','acceptProposal'].includes(input.action))).toBe(false)
    expect(onPrepareDraft).not.toHaveBeenCalled();expect(onSubmitPrompt).not.toHaveBeenCalled()
  })
  it('does not treat an editing boolean as a successful manual begin',async()=>{
    await renderAnnotation({...view,editing:false})
    request.mockImplementation(async()=>({ok:true,view:annotationView}))
    await clickNamed('nativeOfficeManualEdit')
    expect(useNativeOfficeStore.getState().error).toBe('invalid_response')
    expect(useNativeOfficeStore.getState().view?.manual).toBeUndefined()
    expect(namedButton('nativeOfficeManualEdit').getAttribute('aria-pressed')).toBe('false')
  })
  it.each([false,true])('consumes explicit manual authority with frozen=%s',async frozen=>{
    await renderAnnotation({...view,editing:false})
    request.mockImplementation(async(input:NativeOfficeRequest)=>input.action==='beginManualEdit'
      ? {ok:true,view:{...annotationView,manual:{...manualAuthority,frozen}}}
      : {ok:true,view:useNativeOfficeStore.getState().view})
    await clickNamed('nativeOfficeManualEdit')
    expect(namedButton('nativeOfficeManualEdit').getAttribute('aria-pressed')).toBe(String(!frozen))
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(false)
    expect(request.mock.calls.some(([input])=>input.action==='format')).toBe(false)
  })
  it('keeps save, discard and undo as separate native requests',async()=>{
    await renderAnnotation(dirtyManual)
    request.mockImplementation(async()=>({ok:false,error:'composition_pending',view:dirtyManual}))
    await clickNamed('nativeOfficeManualSave')
    expect(request.mock.calls.filter(([input])=>input.action==='save')).toEqual([[{action:'save',objectId:view.objectId,revision:view.revision,expectedChangeSequence:2}]])
    expect(useNativeOfficeStore.getState().view?.dirty).toBe(true)
    expect(container.textContent).toContain('nativeOfficeCompositionPending')
    await clickNamed('nativeOfficeManualDiscard')
    expect(request.mock.calls.filter(([input])=>input.action==='discardManualEdit')).toEqual([[{action:'discardManualEdit',objectId:view.objectId,revision:view.revision,expectedChangeSequence:2,threadId:'thread',changeId:manualAuthority.changeId}]])
    expect(request.mock.calls.some(([input])=>input.action==='undoChange'||input.action==='acceptProposal')).toBe(false)
  })
  it('queries an unknown operation once without another save, decision, discard or undo',async()=>{
    const unknownSave:NativeOfficeView={...dirtyManual,status:'error',saving:true,error:'unknown',manual:{...manualAuthority,frozen:true}}
    await renderAnnotation(unknownSave)
    await openNotes()
    await act(async()=>useNativeOfficeStore.setState({error:'unknown'}))
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(true)
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    expect(namedButton('nativeOfficeSaveAndReference').disabled).toBe(true)
    let finish!:(value:unknown)=>void
    request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
    await act(async()=>{namedButton('查询结果').click();namedButton('查询结果').click()})
    expect(request.mock.calls.filter(([input])=>input.action==='saveStatus')).toEqual([[{action:'saveStatus',objectId:view.objectId}]])
    await act(async()=>finish({ok:false,error:'unknown',view:unknownSave}))
    expect(useNativeOfficeStore.getState().view?.dirty).toBe(true)
    expect(request.mock.calls.some(([input])=>['save','resumeChange','acceptProposal','discardManualEdit','undoChange'].includes(input.action))).toBe(false)
  })
  it('asks Host to settle native input when the cached manual view is still clean',async()=>{
    const clean:NativeOfficeView={...dirtyManual,dirty:false,changeSequence:0,acknowledgedSequence:0,selection:annotationSelection}
    await renderAnnotation(clean)
    request.mockImplementation(async()=>({ok:false,error:'composition_pending',view:clean}))
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(false)
    await clickNamed('nativeOfficeManualSave')
    expect(request.mock.calls.filter(([input])=>input.action==='save')).toEqual([[{action:'save',objectId:view.objectId,revision:view.revision,expectedChangeSequence:0}]])
    expect(request.mock.calls.some(([input])=>['discardManualEdit','undoChange','beginManualEdit'].includes(input.action))).toBe(false)
  })
  it('allows one explicit first save of a frozen prefix discovered during close, including an error view',async()=>{
    const frozen:NativeOfficeView={...dirtyManual,status:'error',error:'unsaved_changes',manual:{...manualAuthority,frozen:true}}
    await renderAnnotation(frozen)
    let finish!:(value:unknown)=>void
    request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(false)
    await act(async()=>{namedButton('nativeOfficeManualSave').click();namedButton('nativeOfficeManualSave').click()})
    expect(request.mock.calls.filter(([input])=>input.action==='save')).toEqual([[{action:'save',objectId:view.objectId,revision:view.revision,expectedChangeSequence:2}]])
    await act(async()=>finish({ok:true,view:recapturedView}))
    expect(useNativeOfficeStore.getState().view?.dirty).toBe(false)
    expect(request.mock.calls.some(([input])=>['beginManualEdit','reference','undoChange'].includes(input.action))).toBe(false)
  })
  it('queries Core before first discard when begin authority did not carry cancellability',async()=>{
    const noRecovery:NativeOfficeView={...dirtyManual,recovery:undefined}
    await renderAnnotation(noRecovery)
    request.mockImplementation(async(input:NativeOfficeRequest)=>input.action==='saveStatus'
      ? {ok:true,view:dirtyManual} : {ok:true,view:{...view,editing:false}})
    await clickNamed('nativeOfficeManualDiscard')
    expect(request.mock.calls.filter(([input])=>input.action==='saveStatus'||input.action==='discardManualEdit')).toEqual([
      [{action:'saveStatus',objectId:view.objectId}],
      [{action:'discardManualEdit',objectId:view.objectId,revision:view.revision,expectedChangeSequence:2,threadId:'thread',changeId:manualAuthority.changeId}]
    ])
    expect(request.mock.calls.some(([input])=>input.action==='save'||input.action==='undoChange')).toBe(false)
  })
  it.each(['absent','other-change','other-thread','not-cancellable'] as const)('does not discard after a Core query returns %s authority',async kind=>{
    await renderAnnotation({...dirtyManual,recovery:undefined})
    const pending=kind==='absent'?null:{...manualPending,...(kind==='other-change'?{changeId:'f'.repeat(64)}:kind==='other-thread'?{threadId:'other-thread'}:{canCancel:false})}
    request.mockImplementation(async()=>({ok:true,view:{...dirtyManual,recovery:{current:null,pending}}}))
    await clickNamed('nativeOfficeManualDiscard')
    expect(request.mock.calls.filter(([input])=>input.action==='saveStatus')).toHaveLength(1)
    expect(request.mock.calls.some(([input])=>input.action==='discardManualEdit')).toBe(false)
  })
  it('keeps frozen failed export recovery queryable and allows only a Core-cancellable discard',async()=>{
    const failed:NativeOfficeView={...dirtyManual,status:'error',error:'save_failed',saving:false,manual:{...manualAuthority,frozen:true}}
    await renderAnnotation(failed)
    request.mockImplementation(async(input:NativeOfficeRequest)=>input.action==='discardManualEdit'
      ? {ok:true,view:{...view,editing:false}} : {ok:true,view:failed})
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(false)
    await clickNamed('查询结果')
    expect(request.mock.calls.filter(([input])=>input.action==='saveStatus')).toEqual([[{action:'saveStatus',objectId:view.objectId}]])
    await clickNamed('nativeOfficeManualDiscard')
    expect(request.mock.calls.filter(([input])=>input.action==='discardManualEdit')).toHaveLength(1)
    expect(request.mock.calls.some(([input])=>input.action==='save'||input.action==='undoChange')).toBe(false)
  })
  it('keeps unknown save queryable when only the Host view carries its error',async()=>{
    const unknown:NativeOfficeView={...dirtyManual,status:'error',saving:true,error:'unknown',manual:{...manualAuthority,frozen:true}}
    await renderAnnotation(unknown)
    expect(useNativeOfficeStore.getState().error).toBeNull()
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(true)
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    expect(namedButton('取消未保存的修改').disabled).toBe(true)
    expect(namedButton('查询结果').disabled).toBe(false)
    request.mockImplementation(async()=>({ok:false,error:'unknown',view:unknown}))
    await clickNamed('查询结果')
    expect(request.mock.calls.filter(([input])=>input.action==='saveStatus')).toHaveLength(1)
    expect(request.mock.calls.some(([input])=>input.action==='save'||input.action==='discardManualEdit')).toBe(false)
  })
  it.each([false,true])('retains only the same explicit discard attempt after its reply is lost (transport=%s)',async transport=>{
    await renderAnnotation(dirtyManual)
    const lost:NativeOfficeView={...dirtyManual,status:'error',error:'unknown',saving:false,manual:{...manualAuthority,frozen:true},recovery:{current:null,pending:null}}
    request.mockImplementation(async(input:NativeOfficeRequest)=>{
      if(input.action==='discardManualEdit') {
        if(transport) {useNativeOfficeStore.getState().receive(lost);throw new Error('synthetic lost cancel reply')}
        return {ok:false,error:'unknown',view:lost}
      }
      return {ok:false,error:'unknown',view:lost}
    })
    await clickNamed('nativeOfficeManualDiscard')
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(true)
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(false)
    await clickNamed('查询结果')
    expect(request.mock.calls.filter(([input])=>input.action==='discardManualEdit')).toHaveLength(1)
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(false)
    request.mockImplementation(async()=>({ok:true,view:{...view,editing:false}}))
    await clickNamed('nativeOfficeManualDiscard')
    const cancels=request.mock.calls.filter(([input])=>input.action==='discardManualEdit')
    expect(cancels).toHaveLength(2);expect(cancels[1]).toEqual(cancels[0])
    expect(request.mock.calls.some(([input])=>input.action==='save'||input.action==='undoChange')).toBe(false)
  })
  it('retains the exact lost cancellation across a Panel unmount and remount',async()=>{
    await renderAnnotation(dirtyManual)
    const lost:NativeOfficeView={...dirtyManual,status:'error',error:'unknown',saving:false,manual:{...manualAuthority,frozen:true},recovery:{current:null,pending:null}}
    request.mockImplementation(async()=>({ok:false,error:'unknown',view:lost}))
    await clickNamed('nativeOfficeManualDiscard')
    await act(async()=>root!.unmount());root=createRoot(container)
    await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'thread'})))
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(false)
    request.mockImplementation(async()=>({ok:true,view:{...view,editing:false}}))
    await clickNamed('nativeOfficeManualDiscard')
    const cancels=request.mock.calls.filter(([input])=>input.action==='discardManualEdit')
    expect(cancels).toHaveLength(2);expect(cancels[1]).toEqual(cancels[0])
    expect(useNativeOfficeStore.getState().manualDiscardAttempts).toEqual({})
  })
  it('does not turn an arbitrary unknown manual state into a new cancellation attempt',async()=>{
    const unknown:NativeOfficeView={...dirtyManual,status:'error',error:'unknown',saving:false,manual:{...manualAuthority,frozen:true},recovery:{current:null,pending:null}}
    await renderAnnotation(unknown)
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    request.mockImplementation(async()=>({ok:false,error:'unknown',view:unknown}))
    await clickNamed('查询结果')
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    expect(request.mock.calls.some(([input])=>input.action==='discardManualEdit'||input.action==='save')).toBe(false)
  })
  it.each(['thread','target','change'] as const)('does not reuse a lost cancellation attempt after its %s owner changes',async owner=>{
    await renderAnnotation(dirtyManual)
    const lost:NativeOfficeView={...dirtyManual,status:'error',error:'unknown',saving:false,manual:{...manualAuthority,frozen:true},recovery:{current:null,pending:null}}
    request.mockImplementation(async()=>({ok:false,error:'unknown',view:lost}))
    await clickNamed('nativeOfficeManualDiscard')
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(false)
    let ownerView=lost
    request.mockImplementation(async()=>({ok:false,error:'unknown',view:ownerView}))
    if(owner==='thread') {
      await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'other-thread'})))
    } else if(owner==='target') {
      const other={...lost,objectId:'f'.repeat(64),path:'/other/report.docx'};ownerView=other
      await act(async()=>useNativeOfficeStore.setState({target:{workspace:'/other',path:other.path},view:other,error:'unknown'}))
    } else {
      await act(async()=>useNativeOfficeStore.getState().receive({...lost,manual:{...manualAuthority,changeId:'f'.repeat(64),frozen:true}},'unknown'))
    }
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    expect(request.mock.calls.filter(([input])=>input.action==='discardManualEdit')).toHaveLength(1)
  })
  it('keeps committed manual records on the explicit Undo path',async()=>{
    const committed={...manualPending,status:'committed' as const,revision:'c'.repeat(64),canCancel:false,canUndo:true,savedAt:'2026-10-15T00:01:00Z'}
    await renderAnnotation({...dirtyManual,dirty:false,canUndo:true,manual:{...manualAuthority,frozen:true},recovery:{pending:null,current:committed}})
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(true)
    expect(namedButton('撤销修改').disabled).toBe(false)
    expect(request.mock.calls.some(([input])=>input.action==='discardManualEdit'||input.action==='undoChange')).toBe(false)
  })
  it.each([false,true])('does not acquire references from a frozen manual prefix (dirty=%s)',async dirty=>{
    await renderAnnotation({...dirtyManual,dirty,manual:{...manualAuthority,frozen:true}})
    await openNotes()
    if(dirty)expect(namedButton('nativeOfficeSaveAndReference').disabled).toBe(true)
    else expect(namedButton('加入对话').disabled).toBe(true)
    expect(namedButton('请求修改').disabled).toBe(true)
    expect(request.mock.calls.some(([input])=>input.action==='reference'||input.action==='format'||input.action==='beginManualEdit')).toBe(false)
  })
  it.each([false,true])('resumes only the advertised original candidate after save query has settled (saving=%s)',async saving=>{
    const pending={...manualPending,status:'unknown' as const,revision:'c'.repeat(64),canCancel:false,canResume:true}
    const recoverable:NativeOfficeView={...dirtyManual,status:'error',error:'unknown',saving,recovery:{current:null,pending}}
    await renderAnnotation(recoverable)
    expect(namedButton('继续保存此修改').disabled).toBe(saving)
    await clickNamed('继续保存此修改')
    expect(request.mock.calls.filter(([input])=>input.action==='resumeChange')).toEqual(saving?[]:[[{action:'resumeChange',objectId:view.objectId,revision:view.revision,expectedChangeSequence:2,threadId:'thread',changeId:manualAuthority.changeId}]])
    expect(request.mock.calls.some(([input])=>input.action==='save'||input.action==='discardManualEdit')).toBe(false)
  })
  it('rejects a stale discard click when an unresolved save arrives before React rerenders',async()=>{
    await renderAnnotation(dirtyManual)
    const discard=namedButton('nativeOfficeManualDiscard')
    await act(async()=>{
      useNativeOfficeStore.getState().receive({...dirtyManual,saving:true,error:'unknown',manual:{...manualAuthority,frozen:true}},'unknown')
      discard.click()
    })
    expect(request.mock.calls.some(([input])=>input.action==='discardManualEdit'||input.action==='saveStatus'||input.action==='save')).toBe(false)
  })
  it('does not reinterpret a failed save as Undo when earlier changes can be undone',async()=>{
    await renderAnnotation({...annotationView,canUndo:true,dirty:true})
    await act(async()=>useNativeOfficeStore.setState({error:'save_failed'}))
    await clickNamed('重试')
    expect(request.mock.calls.filter(([input])=>input.action==='save')).toHaveLength(1)
    expect(request.mock.calls.some(([input])=>input.action==='undoChange')).toBe(false)
  })
})

describe('explicit save and reference',()=>{
  it('passes saveFirst and preserves the current Host refusal without a capsule or send',async()=>{
    const onPrepareDraft=vi.fn(()=>true),onSubmitPrompt=vi.fn(),onFocusConversation=vi.fn()
    await renderAnnotation(dirtyManual,{onPrepareDraft,onSubmitPrompt,onFocusConversation})
    await openNotes()
    expect(namedButton('加入对话')).toBeUndefined()
    expect(namedButton('请求修改').disabled).toBe(true)
    await clickNamed('nativeOfficeSaveAndReference')
    expect(request.mock.calls.filter(([input])=>input.action==='reference')).toEqual([[{action:'reference',objectId:view.objectId,revision:view.revision,expectedChangeSequence:2,selectionToken:annotationSelection.token!,threadId:'thread',editable:false,saveFirst:true}]])
    expect(useNativeOfficeStore.getState().error).toBe('unsaved_changes')
    expect(useNativeReferenceStore.getState().references).toEqual([])
    expect(onPrepareDraft).not.toHaveBeenCalled();expect(onSubmitPrompt).not.toHaveBeenCalled();expect(onFocusConversation).not.toHaveBeenCalled()
    expect(request.mock.calls.some(([input])=>['save','acceptProposal','beginManualEdit'].includes(input.action))).toBe(false)
  })
  it.each([false,true])('uses the new capture after successful synthetic save (published before response=%s)',async publishFirst=>{
    const onPrepareDraft=vi.fn(()=>true),onSubmitPrompt=vi.fn(),onFocusConversation=vi.fn()
    await renderAnnotation(dirtyManual,{onPrepareDraft,onSubmitPrompt,onFocusConversation})
    await openNotes()
    request.mockImplementation(async(input:NativeOfficeRequest)=>{
      if(input.action==='reference') {if(publishFirst)useNativeOfficeStore.getState().receive(recapturedView);return {ok:true,view:recapturedView}}
      return {ok:true,view:useNativeOfficeStore.getState().view}
    })
    await clickNamed('nativeOfficeSaveAndReference')
    expect(useNativeReferenceStore.getState().references).toEqual([expect.objectContaining({revision:recapturedView.revision,selection:recapturedSelection,scopeId:recapturedView.scope!.scopeId,text:'Saved native range',editable:false})])
    expect(useNativeOfficeStore.getState().view?.revision).toBe(recapturedView.revision)
    expect(onFocusConversation).toHaveBeenCalledOnce();expect(onPrepareDraft).not.toHaveBeenCalled();expect(onSubmitPrompt).not.toHaveBeenCalled()
  })
  it.each(['old-token','old-scope','dirty-result'] as const)('rejects a claimed save success with %s',async failure=>{
    await renderAnnotation(dirtyManual)
    await openNotes()
    const bad=failure==='old-token'?{...recapturedView,selection:{...recapturedSelection,token:annotationSelection.token},scope:{...recapturedView.scope!,selectionToken:annotationSelection.token!}}
      :failure==='old-scope'?{...recapturedView,scope:{...recapturedView.scope!,baseRevision:view.revision}}:{...recapturedView,dirty:true}
    request.mockImplementation(async()=>({ok:true,view:bad}))
    await clickNamed('nativeOfficeSaveAndReference')
    expect(useNativeReferenceStore.getState().references).toEqual([])
    expect(useNativeOfficeStore.getState().view?.revision).toBe(view.revision)
  })
  it.each(['thread','target','hide','intermediate-selection','new-edit'] as const)('drops a delayed saved reference after %s changes',async change=>{
    await renderAnnotation(dirtyManual)
    await openNotes()
    let finish!:(value:unknown)=>void
    request.mockImplementation(async(input:NativeOfficeRequest)=>input.action==='reference'?new Promise(resolve=>{finish=resolve}):{ok:true,view:useNativeOfficeStore.getState().view})
    await act(async()=>namedButton('nativeOfficeSaveAndReference').click())
    if(change==='target')await act(async()=>useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:'/synthetic/other.docx'},view:{...view,path:'/synthetic/other.docx',objectId:'f'.repeat(64)}}))
    else if(change==='thread'||change==='hide')await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:change!=='hide',threadId:change==='thread'?'other':'thread'})))
    else if(change==='new-edit')await act(async()=>useNativeOfficeStore.getState().receive({...dirtyManual,changeSequence:3,selection:{...annotationSelection,changeSequence:3,token:'newer-edit-token'}}))
    else await act(async()=>{useNativeOfficeStore.getState().receive({...dirtyManual,selection:{...annotationSelection,changeSequence:2,token:'different-selection-token'}});useNativeOfficeStore.getState().receive(dirtyManual)})
    await act(async()=>finish({ok:true,view:recapturedView}))
    expect(useNativeReferenceStore.getState().references).toEqual([])
    expect(useNativeOfficeStore.getState().view?.revision).not.toBe(recapturedView.revision)
  })
})

describe('proposal decision phases and manual recovery records',()=>{
  it('does not accept or reject an approved proposal that has not been applied',async()=>{
    await renderAnnotation({...annotationView,scope:proposalScope,proposals:[{...proposed,status:'approved'}],localReviews:[proposalReview]},{onPrepareDraft:vi.fn(()=>true),onSubmitPrompt:vi.fn()})
    expect(namedButton('应用修改')).toBeUndefined();expect(namedButton('拒绝')).toBeUndefined()
    expect(namedButton('nativeOfficeRegenerate')).toBeUndefined()
    expect(container.textContent).toContain('nativeOfficeProposalApproved')
    expect(container.textContent).not.toContain('已保存的修改')
    expect(request.mock.calls.some(([input])=>input.action==='acceptProposal'||input.action==='rejectProposal')).toBe(false)
  })
  it('rechecks live approval even when a proposed button has not rerendered yet',async()=>{
    await renderAnnotation({...annotationView,scope:proposalScope,proposals:[proposed],localReviews:[proposalReview]})
    const previousButton=namedButton('应用修改')
    await act(async()=>{
      useNativeOfficeStore.setState({view:{...annotationView,scope:proposalScope,proposals:[{...proposed,status:'approved'}],localReviews:[proposalReview]}})
      previousButton.click()
    })
    expect(request.mock.calls.some(([input])=>input.action==='acceptProposal')).toBe(false)
  })
  it('serializes a proposed decision before React rerenders and then shows approved',async()=>{
    await renderAnnotation({...annotationView,scope:proposalScope,proposals:[proposed],localReviews:[proposalReview]})
    let finish!:(value:unknown)=>void
    request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
    await act(async()=>{namedButton('应用修改').click();namedButton('应用修改').click()})
    expect(request.mock.calls.filter(([input])=>input.action==='acceptProposal')).toHaveLength(1)
    await act(async()=>finish({ok:true,view:{...annotationView,scope:proposalScope,proposals:[{...proposed,status:'approved'}],localReviews:[proposalReview]}}))
    expect(namedButton('应用修改')).toBeUndefined();expect(namedButton('拒绝')).toBeUndefined()
  })
  it.each(['proposed','rejected'] as const)('regenerates %s through the real submit callback with a fresh scope, never an approval replay',async status=>{
    const onPrepareDraft=vi.fn(()=>true),onSubmitPrompt=vi.fn()
    await renderAnnotation({...annotationView,scope:proposalScope,proposals:[{...proposed,status}],localReviews:[proposalReview]},{onPrepareDraft,onSubmitPrompt})
    request.mockImplementation(async(input:NativeOfficeRequest)=>input.action==='reference'
      ? {ok:true,view:{...annotationView,scope:{...proposalScope,scopeId:'c'.repeat(48)}}}
      : {ok:true,view:useNativeOfficeStore.getState().view})
    await clickNamed('nativeOfficeRegenerate')
    expect(onSubmitPrompt).toHaveBeenCalledExactlyOnceWith('nativeOfficeRegeneratePrompt',[expect.objectContaining({scopeId:'c'.repeat(48),editable:true,selection:annotationSelection})])
    expect(onPrepareDraft).not.toHaveBeenCalled()
    expect(request.mock.calls.filter(([input])=>input.action==='reference')).toHaveLength(1)
    expect(request.mock.calls.some(([input])=>['acceptProposal','save','resumeChange'].includes(input.action))).toBe(false)
    expect(JSON.stringify(onSubmitPrompt.mock.calls)).not.toContain(proposalReview.beforeText)
    expect(JSON.stringify(onSubmitPrompt.mock.calls)).not.toContain(proposalReview.afterText)
  })
  it('offers regeneration with the actual submit callback and no draft callback',async()=>{
    const onSubmitPrompt=vi.fn()
    await renderAnnotation({...annotationView,scope:proposalScope,proposals:[proposed],localReviews:[proposalReview]},{onSubmitPrompt})
    expect(namedButton('nativeOfficeRegenerate').disabled).toBe(false)
    await clickNamed('nativeOfficeRegenerate')
    expect(onSubmitPrompt).toHaveBeenCalledOnce()
    expect(request.mock.calls.some(([input])=>input.action==='acceptProposal'||input.action==='save')).toBe(false)
  })
  it.each([false,true])('does not regenerate proposal A for native selection B (before rerender=%s)',async beforeRerender=>{
    const onPrepareDraft=vi.fn(()=>true),onSubmitPrompt=vi.fn()
    await renderAnnotation({...annotationView,scope:proposalScope,proposals:[proposed],localReviews:[proposalReview]},{onPrepareDraft,onSubmitPrompt})
    const previousButton=namedButton('nativeOfficeRegenerate')
    const next={...annotationView,scope:proposalScope,proposals:[proposed],localReviews:[proposalReview],selection:{...annotationSelection,token:'selection-b-token'}}
    await act(async()=>{
      useNativeOfficeStore.getState().receive(next)
      if(beforeRerender)previousButton.click()
    })
    expect(namedButton('nativeOfficeRegenerate').disabled).toBe(true)
    if(!beforeRerender)await clickNamed('nativeOfficeRegenerate')
    expect(request.mock.calls.some(([input])=>input.action==='reference'||input.action==='acceptProposal')).toBe(false)
    expect(onPrepareDraft).not.toHaveBeenCalled();expect(onSubmitPrompt).not.toHaveBeenCalled()
  })
  it('renders a typed manual record without inventing proposal diff or approval state',async()=>{
    const pending:NonNullable<NonNullable<NativeOfficeView['recovery']>['pending']>={version:2,origin:'manual',changeId:manualAuthority.changeId,threadId:'thread',
      baseRevision:view.revision,revision:'',status:'prepared',saveOperationId:'save_manual_001',undoOperationId:'undo_manual_001',
      canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-10-15T00:00:00Z',savedAt:''}
    await renderAnnotation({...annotationView,recovery:{current:null,pending}})
    expect(container.textContent).toContain('nativeOfficeManualDraft');expect(container.textContent).toContain('nativeOfficeManualRecord')
    expect(container.querySelector('[aria-label="已记录的修改前"], [aria-label="已记录的修改后"]')).toBeNull()
    expect(container.textContent).not.toContain('已批准')
    await clickNamed('取消未保存的修改')
    expect(request.mock.calls.filter(([input])=>input.action==='discardManualEdit')).toEqual([[{action:'discardManualEdit',objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,threadId:'thread',changeId:pending.changeId}]])
    await act(async()=>useNativeOfficeStore.setState({view:{...annotationView,recovery:{current:null,pending:{...pending,canCancel:false}}}}))
    expect(namedButton('取消未保存的修改')).toBeUndefined();expect(namedButton('继续保存此修改')).toBeUndefined()
  })
  it('retains a conflict draft without offering a save or implicit undo',async()=>{
    await renderAnnotation(dirtyManual)
    await openNotes()
    await fillNote('Synthetic draft retained across conflict')
    await act(async()=>useNativeOfficeStore.setState({error:'conflict'}))
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(true)
    expect(namedButton('nativeOfficeManualDiscard').disabled).toBe(true)
    expect(container.querySelector<HTMLInputElement>('[aria-label="标注备注"]')?.value).toBe('Synthetic draft retained across conflict')
    expect(useNativeOfficeStore.getState().view?.dirty).toBe(true)
    expect(request.mock.calls.some(([input])=>['save','undoChange','discardManualEdit'].includes(input.action))).toBe(false)
  })
})


// The guest control tests own the actual native DOM event -> typed choice half.
// This mounted Panel suite drives that same facade choice through format -> state.
type FormatMenuChoice = ReturnType<typeof nativeOfficeActionChoiceSchema.parse>
function nativeFormatView(sequence=0,token=`format_selection_${sequence}`):NativeOfficeView {
  return {...annotationView,manual:manualAuthority,dirty:sequence>0,changeSequence:sequence,selection:{...annotationSelection,
    changeSequence:sequence,token,rangeLease:{generation:manualAuthority.generation,sequence,token},
    formatState:{fontFamily:null,fontSize:null,bold:null,italic:false,underline:false}}}
}
function formatMenuChoice(current:NativeOfficeView,format:NativeOfficeFormat):FormatMenuChoice {
  if(current.selection?.kind!=='text'||!current.manual)throw new Error('Expected a native manual text selection')
  return nativeOfficeActionChoiceSchema.parse({actionId:null,changeId:current.manual.changeId,rangeLease:current.selection.rangeLease,format})
}
async function formatPanelHarness(initialSelection = true, props: Partial<Parameters<typeof NativeOfficePanel>[0]> = {}) {
  let menuRequested:(target:NativeOfficeMenuTarget)=>void=()=>{throw new Error('Panel menu consumer not mounted')}
  let publish:(next:NativeOfficeView|null)=>void=()=>{throw new Error('Panel view consumer not mounted')}
  const choices:Array<(choice:FormatMenuChoice)=>void>=[]
  const menu=vi.fn((_target:NativeOfficeActionMenu)=>new Promise<FormatMenuChoice>(resolve=>choices.push(resolve)))
  window.analytix.office.onMenuRequested=handler=>{menuRequested=handler;return ()=>{}}
  window.analytix.office.onChange=handler=>{publish=handler;return ()=>{}}
  window.analytix.office.showActionMenu=menu
  await renderAnnotation({...view,editing:false},props)
  // Host retains the successful begin view independently of Renderer open clearing its store.
  const hostView=nativeFormatView()
  if (!initialSelection) delete hostView.selection
  request.mockImplementation(async(input:NativeOfficeRequest)=>['beginManualEdit','open'].includes(input.action)
    ? {ok:true,view:hostView}: {ok:true,view:useNativeOfficeStore.getState().view})
  await clickNamed('nativeOfficeManualEdit')
  expect(useNativeOfficeStore.getState().manualOwners[view.objectId]).toMatchObject({threadId:'thread',changeId:manualAuthority.changeId,generation:manualAuthority.generation})
  let interaction=0
  const target=(current:NativeOfficeView):NativeOfficeMenuTarget=>({objectId:current.objectId,revision:current.revision,expectedChangeSequence:current.changeSequence??0,
    pointer:{interactionId:`format_interaction_${++interaction}`,selectionToken:current.selection!.token!}})
  return {menu,choices,publish,target,menuRequested,
    open:async(current=useNativeOfficeStore.getState().view!)=>{await act(async()=>menuRequested(target(current)))},
    choose:async(choice:FormatMenuChoice,index=choices.length-1)=>{await act(async()=>{choices[index](choice);for(let i=0;i<12;i++)await Promise.resolve()})}}
}
const formatCalls=()=>request.mock.calls.map(([input])=>input as NativeOfficeRequest).filter(input=>input.action==='format')

describe('native selection format facade -> Panel -> typed format -> fresh state',()=>{
  it.each<NativeOfficeFormat>([{kind:'font-family',value:'Noto Sans CJK SC'},{kind:'font-size',value:13.5},
    {kind:'bold',value:true},{kind:'italic',value:true},{kind:'underline',value:true}])('dispatches the actual %j choice with its original lease',async format=>{
    const harness=await formatPanelHarness(),before=useNativeOfficeStore.getState().view!,after=nativeFormatView(1)
    if(after.selection?.kind!=='text'||!after.selection.formatState)throw new Error('Expected format readback')
    if(format.kind==='font-family')after.selection.formatState.fontFamily=format.value
    else if(format.kind==='font-size')after.selection.formatState.fontSize=format.value
    else after.selection.formatState[format.kind]=format.value
    request.mockImplementation(async(input:NativeOfficeRequest)=>({ok:true,view:input.action==='format'?after:useNativeOfficeStore.getState().view}))
    await harness.open()
    expect(harness.menu.mock.calls[0][0].formatting).toEqual({changeId:manualAuthority.changeId,rangeLease:before.selection?.kind==='text'?before.selection.rangeLease:undefined,
      formatState:{fontFamily:null,fontSize:null,bold:null,italic:false,underline:false},labels:{fontFamily:'nativeOfficeFormatFontFamily',fontSize:'nativeOfficeFormatFontSize',bold:'nativeOfficeFormatBold',italic:'nativeOfficeFormatItalic',underline:'nativeOfficeFormatUnderline',mixed:'nativeOfficeFormatMixed'}})
    expect(formatCalls()).toEqual([])
    await harness.choose(formatMenuChoice(before,format))
    expect(formatCalls()).toEqual([{action:'format',objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,changeId:manualAuthority.changeId,
      rangeLease:{generation:manualAuthority.generation,sequence:0,token:'format_selection_0'},format}])
    expect(useNativeOfficeStore.getState().view).toEqual(after)
    expect(request.mock.calls.some(([input])=>['save','reference','acceptProposal'].includes(input.action))).toBe(false)
  })
  it('keeps formatting available for a dirty current manual session',async()=>{
    const harness=await formatPanelHarness(),current={...nativeFormatView(),dirty:true}
    await act(async()=>harness.publish(current))
    request.mockImplementation(async()=>({ok:true,view:nativeFormatView(1)}))
    await harness.open();expect(harness.menu.mock.calls[0][0].formatting).toBeDefined()
    await harness.choose(formatMenuChoice(current,{kind:'bold',value:true}));expect(formatCalls()).toHaveLength(1)
  })
  it.each(['annotation','lease','state','token','generation','sequence','unbound','frozen','saving','unknown','protected'] as const)(
    'does not offer format controls for %s authority',async kind=>{
      const harness=await formatPanelHarness(),next=nativeFormatView()
      if(next.selection?.kind!=='text')throw new Error('Expected text')
      if(kind==='annotation')delete next.manual
      if(kind==='lease')delete next.selection.rangeLease
      if(kind==='state')delete next.selection.formatState
      if(kind==='token')next.selection.token='foreign_selection_token'
      if(kind==='generation')next.selection.rangeLease!.generation++
      if(kind==='sequence')next.selection.rangeLease!.sequence++
      if(kind==='frozen')next.manual={...manualAuthority,frozen:true}
      if(kind==='saving')next.saving=true
      if(kind==='protected'){
        const selector={threadId:'thread',turnId:'turn',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
        next.protectedPresent=selector;next.editing=false;next.dirty=false;delete next.manual;delete next.selection
        await act(async()=>useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:next.path,protectedPresent:selector},view:next}))
      }else await act(async()=>useNativeOfficeStore.getState().receive(next,kind==='unknown'?'unknown':null))
      if(kind==='unbound')await act(async()=>useNativeOfficeStore.setState({manualOwners:{}}))
      await harness.open(kind==='protected'?nativeFormatView():next)
      expect(harness.menu.mock.calls.at(-1)?.[0].formatting).toBeUndefined()
      expect(formatCalls()).toEqual([])
    })
  it.each(['thread','target','change','generation','token','frozen','saving','unknown'] as const)(
    'rejects an old native control choice after %s changes',async kind=>{
      const harness=await formatPanelHarness(),before=useNativeOfficeStore.getState().view!,next=nativeFormatView()
      await harness.open()
      if(kind==='thread')await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'other'})))
      else await act(async()=>{
        if(kind==='target')useNativeOfficeStore.setState({target:{workspace:'/synthetic',path:view.path}})
        if(kind==='change')next.manual={...manualAuthority,changeId:'f'.repeat(64)}
        if(kind==='generation')next.manual={...manualAuthority,generation:manualAuthority.generation+1}
        if(kind==='token'&&next.selection?.kind==='text'){next.selection.token='same_text_new_token';next.selection.rangeLease={generation:manualAuthority.generation,sequence:0,token:'same_text_new_token'}}
        if(kind==='frozen')next.manual={...manualAuthority,frozen:true}
        if(kind==='saving')next.saving=true
        if(kind!=='target')useNativeOfficeStore.getState().receive(next,kind==='unknown'?'unknown':null)
      })
      await harness.choose(formatMenuChoice(before,{kind:'bold',value:true}))
      expect(formatCalls()).toEqual([])
    })
  it('retains successful begin ownership across Panel remount but never transfers it to another thread',async()=>{
    const harness=await formatPanelHarness()
    await act(async()=>root!.unmount());root=createRoot(container)
    await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'thread'})))
    await harness.open();expect(harness.menu.mock.calls.at(-1)?.[0].formatting).toBeDefined()
    await harness.choose({actionId:null})
    await act(async()=>root!.render(createElement(NativeOfficePanel,{visible:true,threadId:'other'})))
    await harness.open();expect(harness.menu.mock.calls.at(-1)?.[0].formatting).toBeUndefined()
  })
  it.each(['success','failure'] as const)('does not replace a newer selection with a late format %s',async outcome=>{
    const harness=await formatPanelHarness(),before=useNativeOfficeStore.getState().view!,after=nativeFormatView(1),newer=nativeFormatView(2,'user_newer_selection')
    let finish!:(value:{ok:boolean;view:NativeOfficeView})=>void,fail!:(error:Error)=>void
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='format'?new Promise((resolve,reject)=>{finish=resolve;fail=reject}):Promise.resolve({ok:true,view:useNativeOfficeStore.getState().view}))
    await harness.open();await harness.choose(formatMenuChoice(before,{kind:'bold',value:true}))
    await act(async()=>harness.publish(newer))
    await act(async()=>{if(outcome==='success')finish({ok:true,view:after});else fail(new Error('old format unavailable'))})
    expect(useNativeOfficeStore.getState().view).toEqual(newer)
    expect(useNativeOfficeStore.getState().error).toBeNull()
  })
  it('does not republish an identical format result already delivered by the actual onChange consumer',async()=>{
    const harness=await formatPanelHarness(),before=useNativeOfficeStore.getState().view!,after=nativeFormatView(1)
    let finish!:(value:{ok:true;view:NativeOfficeView})=>void
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='format'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:useNativeOfficeStore.getState().view}))
    await harness.open();await harness.choose(formatMenuChoice(before,{kind:'bold',value:true}))
    await act(async()=>harness.publish(after));const received=useNativeOfficeStore.getState().receiveSequence
    await act(async()=>finish({ok:true,view:after}))
    expect(useNativeOfficeStore.getState().receiveSequence).toBe(received)
    expect(useNativeOfficeStore.getState().view).toEqual(after)
  })
  it('accepts a fresh native request after format completion with the new range lease',async()=>{
    const harness=await formatPanelHarness(),before=useNativeOfficeStore.getState().view!,after=nativeFormatView(1)
    let finish!:(value:{ok:true;view:NativeOfficeView})=>void
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='format'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:useNativeOfficeStore.getState().view}))
    await harness.open();await harness.choose(formatMenuChoice(before,{kind:'bold',value:true}))
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(true)
    await act(async()=>finish({ok:true,view:after}))
    expect(useNativeOfficeStore.getState().view).toEqual(after)
    expect(namedButton('nativeOfficeManualSave').disabled).toBe(false)
    await harness.open(after)
    expect(harness.menu).toHaveBeenCalledTimes(2)
    expect(harness.menu.mock.calls[1][0]).toMatchObject({expectedChangeSequence:1,formatting:{rangeLease:{sequence:1,token:'format_selection_1'}}})
  })
  it('serializes choices and drains only the fresh continuation after the format fence releases',async()=>{
    const harness=await formatPanelHarness(),before=useNativeOfficeStore.getState().view!,after=nativeFormatView(1)
    let finish!:(value:{ok:true;view:NativeOfficeView})=>void
    request.mockImplementation((input:NativeOfficeRequest)=>input.action==='format'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:useNativeOfficeStore.getState().view}))
    await harness.open();await harness.open()
    await act(async()=>{harness.choices[0](formatMenuChoice(before,{kind:'bold',value:true}));harness.choices[1](formatMenuChoice(before,{kind:'italic',value:true}));for(let i=0;i<12;i++)await Promise.resolve()})
    expect(formatCalls()).toHaveLength(1)
    await act(async()=>harness.menuRequested(harness.target(after)))
    expect(harness.menu).toHaveBeenCalledTimes(2)
    await act(async()=>finish({ok:true,view:after}))
    expect(harness.menu).toHaveBeenCalledTimes(3)
    expect(harness.menu.mock.calls[2][0].formatting?.rangeLease).toEqual(after.selection?.kind==='text'?after.selection.rangeLease:undefined)
    request.mockImplementation(async()=>({ok:true,view:nativeFormatView(2)}))
    await harness.choose(formatMenuChoice(after,{kind:'font-size',value:14}))
    expect(formatCalls()).toHaveLength(2)
    expect(formatCalls()[1]).toMatchObject({expectedChangeSequence:1,rangeLease:{sequence:1,token:'format_selection_1'}})
    const count=harness.menu.mock.calls.length
    await harness.open(before);expect(harness.menu).toHaveBeenCalledTimes(count)
  })
})


describe('DOCX selection keeps the native viewport geometry owner stable', () => {
  it.each([false,true])('publishes a fresh selection and menu before commit without an inflow card (truncated=%s)', async truncated => {
    const harness = await formatPanelHarness(false)
    const surface = container.querySelector<HTMLElement>('[aria-label="nativeOfficeEditor"]')!
    const viewport = surface.parentElement!, host = viewport.parentElement!
    const toolbar = host.querySelector<HTMLElement>('[role="toolbar"]')!
    expect([...host.children]).toEqual([toolbar,viewport])
    expect(container.querySelector('[aria-label="文档标注"]')).toBeNull()
    expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
    const next = nativeFormatView(0,'selection_after_drag')
    if (next.selection?.kind !== 'text') throw new Error('Expected text capture')
    next.selection.capture = {capturedCharacters:19,totalCharacters:truncated ? 38 : 19,unit:'utf-16',complete:!truncated,truncated}
    request.mockImplementation(async (input:NativeOfficeRequest) => ({ok:true,view:input.action === 'reference'
      ? {...next,scope:{scopeId:'a'.repeat(48),editable:false,threadId:'thread',selectionToken:input.selectionToken,baseRevision:view.revision,changeSequence:0,parts:[]}}
      : useNativeOfficeStore.getState().view}))
    await act(async () => {
      harness.publish(next)
      harness.menuRequested(harness.target(next))
      // This assertion precedes React's commit, through the real onChange facade.
      expect(harness.menu).toHaveBeenCalledTimes(1)
      expect(harness.menu.mock.calls[0][0]).toMatchObject({pointer:{selectionToken:'selection_after_drag'},
        formatting:{rangeLease:next.selection?.kind === 'text' ? next.selection.rangeLease : undefined,
          formatState:next.selection?.kind === 'text' ? next.selection.formatState : undefined}})
      expect(nativeOfficeActionMenuSchema.safeParse(harness.menu.mock.calls[0][0]).success).toBe(true)
    })
    // The menu stays pending across the commit. No constant rect mock can make
    // an inserted flow card pass these actual mounted structure assertions.
    expect(surface.parentElement).toBe(viewport)
    expect(viewport.parentElement).toBe(host)
    expect([...host.children]).toEqual([toolbar,viewport])
    expect(container.querySelector('[aria-label="文档标注"]')).toBeNull()
    expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
    const feedback = container.querySelector('[data-native-docx-feedback]')!
    expect(toolbar.contains(feedback)).toBe(true)
    expect(feedback.textContent?.includes('已捕获')).toBe(truncated)
    await harness.choose({actionId:'quote'})
    expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({selectionToken:'selection_after_drag',editable:false})
    expect(useNativeReferenceStore.getState().references[0]).toMatchObject({editable:false,selection:{token:'selection_after_drag',capture:{truncated}}})
    await openNotes()
    expect(toolbar.querySelector('[aria-label="标注备注"]')).not.toBeNull()
    expect([...host.children]).toEqual([toolbar,viewport])
    await clickNamed('关闭标注备注')
    expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
    expect([...host.children]).toEqual([toolbar,viewport])
  })

  it.each(['copy','request-modify'] as const)('keeps %s on the native menu without opening Renderer notes', async actionId => {
    const onPrepareDraft = vi.fn(() => true), onSubmitPrompt = vi.fn()
    const harness = await formatPanelHarness(false,{onPrepareDraft,onSubmitPrompt})
    const next = nativeFormatView(0,'normal_menu_selection')
    request.mockImplementation(async (input:NativeOfficeRequest) => ({ok:true,view:input.action === 'reference'
      ? {...next,scope:{scopeId:'a'.repeat(48),editable:true,threadId:'thread',selectionToken:input.selectionToken,baseRevision:view.revision,changeSequence:0,parts:[]}}
      : useNativeOfficeStore.getState().view}))
    const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator,'clipboard')
    const writeText = vi.fn(async (_text:string) => undefined)
    Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}})
    try {
      await act(async () => { harness.publish(next); harness.menuRequested(harness.target(next)) })
      expect(harness.menu.mock.calls[0][0].actions.find(action => action.id === actionId)?.enabled).toBe(true)
      expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
      await harness.choose({actionId})
      if (actionId === 'copy') {
        expect(writeText).toHaveBeenCalledExactlyOnceWith(annotationSelection.text)
        expect(request.mock.calls.some(([input]) => input.action === 'reference')).toBe(false)
      } else {
        expect(request.mock.calls.find(([input]) => input.action === 'reference')?.[0]).toMatchObject({selectionToken:'normal_menu_selection',editable:true})
        expect(onPrepareDraft).toHaveBeenCalledExactlyOnceWith('请根据所选内容提出修改建议。',[expect.objectContaining({editable:true,selection:{...next.selection}})])
        expect(onSubmitPrompt).not.toHaveBeenCalled()
      }
      expect(container.querySelector('[aria-label="文档标注"]')).toBeNull()
      expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
    } finally {
      if (clipboardDescriptor) Object.defineProperty(navigator,'clipboard',clipboardDescriptor)
      else Reflect.deleteProperty(navigator,'clipboard')
    }
  })
  it.each(['unknown','save_failed'] as const)('keeps %s visible and queryable without a selection or a new flow row', async error => {
    const harness = await formatPanelHarness(false)
    const surface = container.querySelector<HTMLElement>('[aria-label="nativeOfficeEditor"]')!
    const viewport = surface.parentElement!, host = viewport.parentElement!
    const toolbar = host.querySelector<HTMLElement>('[role="toolbar"]')!
    const failed = {...nativeFormatView(),selection:undefined,saving:error === 'unknown',error}
    await act(async () => harness.publish(failed))
    const feedback = toolbar.querySelector<HTMLElement>('[data-native-docx-feedback]')!
    expect(feedback.getAttribute('role')).toBe('alert')
    const message = error === 'unknown' ? '修改结果尚未确认' : '更新未完成'
    expect(feedback.textContent).toContain(message)
    expect(feedback.title).toContain(message)
    expect([...host.children]).toEqual([toolbar,viewport])
    expect(surface.parentElement).toBe(viewport)
    expect(namedButton('查询结果').disabled).toBe(false)
    expect(container.querySelector('[aria-label="标注备注"]')).toBeNull()
    expect(request.mock.calls.some(([input]) => input.action === 'saveStatus')).toBe(false)
    await clickNamed('查询结果')
    expect(request.mock.calls.filter(([input]) => input.action === 'saveStatus')).toEqual([[{action:'saveStatus',objectId:view.objectId}]])
    expect([...host.children]).toEqual([toolbar,viewport])
  })
  it.each([false,true])('preserves retained notes and the prior failed-view editing gate (editing=%s)', async editing => {
    const harness = await formatPanelHarness()
    await fillNote('保留的备注')
    const before = useNativeOfficeStore.getState().view!
    await act(async () => harness.publish({...before,status:'error',editing,error:'save_failed',annotationError:true,manual:editing ? before.manual : undefined}))
    const input = noteInput(), notes = input.closest('[data-native-docx-notes]')!
    expect(input.value).toBe('保留的备注')
    expect(input.disabled).toBe(!editing)
    expect(namedButton('加入对话').disabled).toBe(!editing)
    const retry = [...notes.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === '重试')!
    expect(retry.disabled).toBe(!editing)
    if (!editing) expect(namedButton('查询结果')).toBeUndefined()
    expect(container.querySelector('[data-native-docx-feedback]')?.textContent).toContain('更新未完成')
    const saves = request.mock.calls.filter(([value]) => value.action === 'annotationSave').length
    await fillNote('失败后输入')
    expect(useNativeReferenceStore.getState().drafts[annotationDraftKey].note).toBe(editing ? '失败后输入' : '保留的备注')
    expect(request.mock.calls.filter(([value]) => value.action === 'annotationSave')).toHaveLength(saves + (editing ? 1 : 0))
    expect(request.mock.calls.some(([value]) => ['annotate','reference','save','saveStatus'].includes(value.action))).toBe(false)
    expect(container.querySelector('[aria-label="文档标注"]')).toBeNull()
  })

})
