// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { WorkspaceTabs, WorkspaceToolSelector } from './WorkspaceTabs'
import { useWorkspaceTabsStore, type WorkspaceTab } from '../../store/workspace-tabs-store'

const readyDocuments = () => ({packageId:'analytix-documents' as const,packageVersion:'1.0.0',displayName:'Documents',
  origin:'development-source' as const,publishable:false as const,materialized:true,generationId:'a'.repeat(64),
  activationState:'recorded' as const,desiredState:'enabled' as const,activationRevision:1,activationId:'b'.repeat(64),available:true,
  operations:['create-object','open-object','close-object']})
const packageList=vi.fn()
let root: Root
let container: HTMLDivElement
const onSelect = vi.fn((id: string) => useWorkspaceTabsStore.getState().activateTab(id))
const onClose = vi.fn((id: string) => useWorkspaceTabsStore.getState().closeTab(id))
const a: WorkspaceTab = { id: 'doc-a', kind: 'document', mode: 'documents', title: '调查报告.docx' }
const b: WorkspaceTab = { id: 'doc-b', kind: 'document', mode: 'documents', title: '交易统计.xlsx', dirty: true }
function Harness() {
  const state = useWorkspaceTabsStore()
  return createElement(WorkspaceTabs, { tabs: state.tabs, activeTabId: state.activeTabId, selectorOpen: state.selectorOpen,
    focused: false, onSelect, onClose, onAdd: state.showSelector, onReorder: state.reorderTab, onCollapse: () => state.setOpen(false), onToggleFocus: () => {} })
}
const tabButtons = () => [...container.querySelectorAll<HTMLButtonElement>('[role="tab"]')]
async function key(target: HTMLElement, key: string, options: KeyboardEventInit = {}) {
  await act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options })) })
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('zh')
  packageList.mockReset().mockResolvedValue({ok:true,packages:[readyDocuments()]})
  Object.defineProperty(window,'analytix',{configurable:true,value:{packageHost:{request:packageList}}})
  onSelect.mockClear(); onClose.mockClear()
  useWorkspaceTabsStore.setState({ tabs: [a, b], activeTabId: a.id, selectorOpen: false, open: true, focused: false })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
  await act(async () => root.render(createElement(Harness)))
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); Reflect.deleteProperty(window,'analytix'); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
const c: WorkspaceTab = { id: 'doc-c', kind: 'document', mode: 'documents', title: '隔离的长标题_恢复后仍应显示关闭按钮.xlsx' }
function tabStripLayout(viewportWidth = 240, scale = 1) {
  const list = container.querySelector<HTMLDivElement>('[role="tablist"]')!
  const resizeCallbacks = new Set<() => void>()
  vi.stubGlobal('ResizeObserver', class {
    private notify: () => void
    constructor(callback: ResizeObserverCallback) {
      this.notify = () => callback([], this as unknown as ResizeObserver)
      resizeCallbacks.add(this.notify)
    }
    observe() {}
    disconnect() { resizeCallbacks.delete(this.notify) }
  })
  const layout = { viewportWidth, scale }
  const originalRect = HTMLElement.prototype.getBoundingClientRect
  const tabWidth = 170
  const tabRect = (tab: HTMLElement) => {
    const index = [...list.querySelectorAll('.workspace-tab')].indexOf(tab)
    return new DOMRect(50 + (index * (tabWidth + 8) - list.scrollLeft) * layout.scale, 20, tabWidth * layout.scale, 40 * layout.scale)
  }
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this === list) return new DOMRect(50, 20, layout.viewportWidth * layout.scale, 40 * layout.scale)
    if (this.matches('.workspace-tab')) return tabRect(this)
    if (this.matches('.workspace-tab-close')) {
      const rect = tabRect(this.parentElement!)
      return new DOMRect(rect.right - 28 * layout.scale, rect.y, 26 * layout.scale, 28 * layout.scale)
    }
    return originalRect.call(this)
  })
  Object.defineProperty(list, 'clientWidth', { configurable: true, get: () => layout.viewportWidth })
  list.scrollTo = vi.fn((options: ScrollToOptions | number) => {
    if (typeof options === 'object' && options.left !== undefined) list.scrollLeft = options.left
  }) as typeof list.scrollTo
  const visible = (id: string) => {
    const button = tabButtons().find(tab => tab.title === useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === id)?.title)!
    const wrapper = button.parentElement!
    const rect = wrapper.getBoundingClientRect(), viewport = list.getBoundingClientRect()
    const close = wrapper.querySelector<HTMLButtonElement>('.workspace-tab-close')!.getBoundingClientRect()
    return rect.left >= viewport.left - 0.5 && rect.right <= viewport.right + 0.5 && close.right <= viewport.right + 0.5
  }
  return { list, layout, visible, resize: () => resizeCallbacks.forEach(callback => callback()) }
}

describe('manual workspace tabs', () => {
  it('shows task identity in the existing strip and omits the unavailable terminal from Schedule tools', async () => {
    const taskTab: WorkspaceTab = { id: 'schedule-task:"task"', kind: 'tool', mode: 'schedule-task', title: 'Current task', scheduleTaskId: 'task' }
    await act(async () => useWorkspaceTabsStore.getState().openTab(taskTab))
    expect(tabButtons().find(tab => tab.getAttribute('aria-selected') === 'true')?.textContent).toContain('Current task')
    await act(async () => root.render(createElement(WorkspaceToolSelector, {
      onOpen: vi.fn(), filesEnabled: true, sideChatEnabled: false, planEnabled: false,
      terminalAvailable: false
    })))
    expect(container.textContent).not.toContain(i18n.t('common:rightPanelTerminal'))
  })
  it('moves focus with arrows without opening a native object until Enter', async () => {
    await act(async () => tabButtons()[0].focus())
    await key(tabButtons()[0], 'ArrowRight')
    expect(document.activeElement).toBe(tabButtons()[1])
    expect(onSelect).not.toHaveBeenCalled()
    expect(tabButtons()[0].getAttribute('aria-selected')).toBe('true')
    await key(tabButtons()[1], 'Enter')
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(b.id)
    expect(tabButtons()[1].getAttribute('aria-selected')).toBe('true')
  })
  it('keeps close buttons outside tab buttons and exposes object state and full names', () => {
    expect(container.querySelector('button button')).toBeNull()
    expect(tabButtons()[1].getAttribute('aria-label')).toContain('交易统计.xlsx')
    expect(tabButtons()[1].getAttribute('aria-label')).toContain('未保存')
    expect(container.querySelectorAll('[role="tablist"]')).toHaveLength(1)
    expect(tabButtons().filter((tab) => tab.tabIndex === 0)).toHaveLength(1)
  })
  it('reorders with Alt+Shift+Arrow while keeping the active object', async () => {
    await key(tabButtons()[0], 'ArrowRight', { altKey: true, shiftKey: true })
    expect(useWorkspaceTabsStore.getState().tabs.map((tab) => tab.id)).toEqual([b.id, a.id])
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(a.id)
    expect(onSelect).not.toHaveBeenCalled()
  })
  it('does not close or activate during IME composition', async () => {
    await key(tabButtons()[0], 'Delete', { isComposing: true })
    await key(tabButtons()[0], 'Enter', { isComposing: true })
    expect(onClose).not.toHaveBeenCalled(); expect(onSelect).not.toHaveBeenCalled()
  })
  it('does not close twice when the shell already consumed the workspace shortcut', async () => {
    const consume = (event: KeyboardEvent): void => event.preventDefault()
    container.addEventListener('keydown', consume, true)
    try {
      await key(tabButtons()[0], 'w', { ctrlKey: true })
      expect(onClose).not.toHaveBeenCalled()
      expect(useWorkspaceTabsStore.getState().tabs.map((tab) => tab.id)).toEqual([a.id, b.id])
    } finally {
      container.removeEventListener('keydown', consume, true)
    }
  })
  it('closes a tab by keyboard and retains the workspace', async () => {
    await key(tabButtons()[0], 'Delete')
    expect(onClose).toHaveBeenCalledExactlyOnceWith(a.id)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, activeTabId: b.id })
  })
  it('reveals a newly opened tab including its close button without stealing editor focus or vertical scroll', async () => {
    const strip = tabStripLayout(240, 0.88)
    const editor = document.createElement('textarea'); container.append(editor); editor.focus()
    strip.list.scrollTop = 91; document.documentElement.scrollTop = 44
    await act(async () => useWorkspaceTabsStore.getState().openTab(c))
    expect(strip.visible(c.id)).toBe(true)
    expect(document.activeElement).toBe(editor)
    expect(strip.list.scrollTop).toBe(91)
    expect(document.documentElement.scrollTop).toBe(44)
    expect(useWorkspaceTabsStore.getState().tabs.map(tab => tab.id)).toEqual([a.id, b.id, c.id])
    document.documentElement.scrollTop = 0
  })
  it('reveals programmatic activation and a restored set of tabs', async () => {
    const strip = tabStripLayout()
    await act(async () => useWorkspaceTabsStore.getState().activateTab(b.id))
    expect(strip.visible(b.id)).toBe(true)
    await act(async () => useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true }))
    strip.list.scrollLeft = 0
    await act(async () => useWorkspaceTabsStore.setState({ tabs: [a, b, c], activeTabId: c.id, selectorOpen: false }))
    expect(strip.visible(c.id)).toBe(true)
    expect(tabButtons().filter(tab => tab.tabIndex === 0)).toHaveLength(1)
  })
  it('keeps the active tab and its close button visible after a narrow strip resize', async () => {
    const strip = tabStripLayout(280)
    await act(async () => useWorkspaceTabsStore.getState().openTab(c))
    strip.layout.viewportWidth = 180
    await act(async () => strip.resize())
    expect(strip.visible(c.id)).toBe(true)
  })
  it('preserves a manual horizontal scroll across unrelated tab metadata renders', async () => {
    const strip = tabStripLayout()
    await act(async () => useWorkspaceTabsStore.getState().openTab(c))
    strip.list.scrollLeft = 0
    await act(async () => useWorkspaceTabsStore.setState(state => ({ tabs: state.tabs.map(tab => ({ ...tab, loading: tab.id === c.id })) })))
    expect(strip.list.scrollLeft).toBe(0)
  })
  it('reveals the complete keyboard-focused tab without selecting it', async () => {
    const strip = tabStripLayout()
    await act(async () => useWorkspaceTabsStore.setState({ tabs: [a, b, c] }))
    await act(async () => tabButtons()[0].focus())
    await key(tabButtons()[0], 'End')
    expect(document.activeElement).toBe(tabButtons()[2])
    expect(strip.visible(c.id)).toBe(true)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(a.id)
    expect(onSelect).not.toHaveBeenCalled()
  })
  it('reveals the neighboring tab and restores focus after closing the active tab', async () => {
    const strip = tabStripLayout()
    await act(async () => useWorkspaceTabsStore.getState().openTab(c))
    await act(async () => tabButtons()[2].focus())
    await key(tabButtons()[2], 'Delete')
    await act(async () => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(b.id)
    expect(document.activeElement).toBe(tabButtons()[1])
    expect(strip.visible(b.id)).toBe(true)
  })
  it('preserves dirty-close rejection focus and order', async () => {
    tabStripLayout()
    await act(async () => useWorkspaceTabsStore.getState().activateTab(b.id))
    await act(async () => tabButtons()[1].focus())
    onClose.mockImplementationOnce(() => {})
    await key(tabButtons()[1], 'Delete')
    await act(async () => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
    expect(document.activeElement).toBe(tabButtons()[1])
    expect(useWorkspaceTabsStore.getState().tabs.map(tab => tab.id)).toEqual([a.id, b.id])
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(b.id)
  })
  it('reveals immediately under reduced motion even if CSS requests smooth scrolling', async () => {
    const strip = tabStripLayout()
    document.documentElement.dataset.motionReduced = 'true'
    strip.list.style.scrollBehavior = 'smooth'
    try {
      await act(async () => useWorkspaceTabsStore.getState().openTab(c))
      expect(strip.visible(c.id)).toBe(true)
      expect(strip.list.scrollTo).toHaveBeenCalledWith(expect.objectContaining({ behavior: 'instant' }))
    } finally { delete document.documentElement.dataset.motionReduced }
  })
  it('dispatches shared layout commands once and leaves collapse focus to its owner', async () => {
    const onCollapse = vi.fn()
    const onToggleFocus = vi.fn()
    const globalToggle = document.createElement('button')
    globalToggle.setAttribute('aria-controls', 'workbench-right-workspace')
    document.body.append(globalToggle)
    const stealFocus = vi.spyOn(globalToggle, 'focus')
    try {
      for (const focused of [false, true]) {
        await act(async () => root.render(createElement(WorkspaceTabs, {tabs:[a,b],activeTabId:a.id,selectorOpen:false,focused,onSelect,onClose,onReorder:vi.fn(),onAdd:vi.fn(),onToggleFocus,onCollapse})))
        const label = i18n.t(focused ? 'common:workbenchDock' : 'common:workbenchFocus')
        const focusButtons = [...container.querySelectorAll<HTMLButtonElement>('button')].filter(button => button.getAttribute('aria-label') === label)
        expect(focusButtons).toHaveLength(1)
        expect(focusButtons[0].getAttribute('aria-pressed')).toBe(String(focused))
        expect(focusButtons[0].disabled).toBe(false)
        expect(focusButtons[0].querySelector('svg')?.getAttribute('viewBox')).toBe('0 0 16 16')
        await act(async () => focusButtons[0].click())
      }
      expect(onToggleFocus).toHaveBeenCalledTimes(2)
      const collapseButtons = [...container.querySelectorAll<HTMLButtonElement>('button')].filter(button => button.getAttribute('aria-label') === i18n.t('common:workbenchCollapse'))
      expect(collapseButtons).toHaveLength(1)
      await act(async () => collapseButtons[0].click())
      await act(async () => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
      expect(onCollapse).toHaveBeenCalledExactlyOnceWith()
      expect(stealFocus).not.toHaveBeenCalled()
    } finally { globalToggle.remove() }
  })
  it('keeps layout controls out of the guide and drops every description for the real entry list', async () => {
    const onOpen = vi.fn()
    await act(async () => root.render(createElement(WorkspaceToolSelector, {
      onOpen, filesEnabled: true, sideChatEnabled: false, planEnabled: true
    })))
    expect(container.querySelector('h2')).toBeNull()
    expect(container.querySelector('.workspace-selector-layout')).toBeNull()
    expect(container.querySelectorAll('[data-sidebar-right-guide-entry]').length).toBeGreaterThan(4)
    expect(container.querySelector('.workspace-guide-entry-description')).toBeNull()
    expect(container.querySelectorAll('kbd')).toHaveLength(1)
    for (const [id, palette] of [
      ['files', '[fill="#FFBC4D"]'],
      ['browser', '[stroke="#539CFA"]'],
      ['terminal', '[stroke="#679EFE"]']
    ]) {
      const artwork = container.querySelector(`[data-sidebar-right-guide-entry="${id}"] svg`)
      expect(artwork?.getAttribute('viewBox')).toBe('0 0 36 36')
      expect(artwork?.getAttribute('width')).toBe('22')
      expect(artwork?.querySelector(palette)).not.toBeNull()
    }
    const terminal = container.querySelector('[data-sidebar-right-guide-entry="terminal"]')!
    expect(terminal.getAttribute('aria-keyshortcuts')).toBe('Control+` Meta+`')
    expect(container.querySelector('[data-sidebar-right-guide-entry="files"]')?.getAttribute('aria-keyshortcuts')).toBeNull()
    await act(async () => container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="sidechat"]')!.click())
    expect(onOpen).not.toHaveBeenCalled()
    await act(async () => container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="files"]')!.click())
    expect(onOpen).toHaveBeenCalledExactlyOnceWith('files')
  })
  it('keeps all tabs and dirty state when the panel folds and returns', async () => {
    const state = useWorkspaceTabsStore.getState()
    await act(async () => root.render(createElement(WorkspaceTabs, {tabs:[a,b],activeTabId:b.id,selectorOpen:false,focused:false,
      onSelect,onClose,onReorder:state.reorderTab,onAdd:state.showSelector,onToggleFocus:() => state.setFocused(true),onCollapse:() => state.setOpen(false)})))
    await act(async () => container.querySelector<HTMLButtonElement>('#workbench-workspace-collapse')!.click())
    expect(useWorkspaceTabsStore.getState()).toMatchObject({open:false,tabs:[a,b]})
    expect(onClose).not.toHaveBeenCalled()
    await act(async () => useWorkspaceTabsStore.getState().setOpen(true))
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([a,b])
    expect(useWorkspaceTabsStore.getState().tabs[1].dirty).toBe(true)
  })
  it('terminal selector dispatches one action to the workspace owner', async () => {
    const onOpen = vi.fn()
    await act(async () => root.render(createElement(WorkspaceToolSelector, { onOpen, filesEnabled: true, sideChatEnabled: false, planEnabled: false })))
    const button = container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="terminal"]')!
    await act(async () => button.click())
    expect(onOpen).toHaveBeenCalledExactlyOnceWith('terminal')
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([a, b])
  })
})

describe('Word user creation entry',()=>{
  it('opens the existing guide Word form and submits one valid basename without routing the old document entry',async()=>{
    const onOpen=vi.fn(),onCreateWord=vi.fn()
    await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen,onCreateWord,wordEnabled:true,filesEnabled:true,sideChatEnabled:false,planEnabled:false})))
    expect(container.querySelector('[data-sidebar-right-guide-entry="documents"]')).toBeNull()
    await act(async()=>container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
    const input=container.querySelector<HTMLInputElement>('[aria-label="'+i18n.t('common:workspaceWordFileName')+'"]')!
    expect(document.activeElement).toBe(input);expect(input.value).toBe(i18n.t('common:workspaceNewWordFileName'))
    await act(async()=>container.querySelector<HTMLButtonElement>('form button[type="submit"]')!.click())
    expect(onCreateWord).toHaveBeenCalledExactlyOnceWith(input.value);expect(onOpen).not.toHaveBeenCalled()
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([a,b])
  })
  it('keeps the Word entry disabled when its current thread/runtime gate is unavailable',async()=>{
    const onCreateWord=vi.fn()
    await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,wordEnabled:false,filesEnabled:true,sideChatEnabled:false,planEnabled:false})))
    const word=container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!
    expect(word.disabled).toBe(true);await act(async()=>word.click())
    expect(container.querySelector('form')).toBeNull();expect(onCreateWord).not.toHaveBeenCalled()
  })
  it('freezes an unresolved filename and retries the same requested name',async()=>{
    const onCreateWord=vi.fn()
    await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,wordEnabled:true,filesEnabled:true,sideChatEnabled:false,planEnabled:false,
      wordCreation:{phase:'unknown',fileName:'original.docx',message:'Unconfirmed'}})))
    await act(async()=>container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
    const input=container.querySelector<HTMLInputElement>('form input')!
    expect(input.value).toBe('original.docx');expect(input.disabled).toBe(true)
    await act(async()=>container.querySelector<HTMLButtonElement>('form button[type="submit"]')!.click())
    expect(onCreateWord).toHaveBeenCalledExactlyOnceWith('original.docx')
  })
  it('keeps invalid basenames out of the create callback even on direct form submission',async()=>{
    const onCreateWord=vi.fn()
    await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,wordEnabled:true,filesEnabled:true,sideChatEnabled:false,planEnabled:false})))
    await act(async()=>container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
    const input=container.querySelector<HTMLInputElement>('form input')!
    await act(async()=>{
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'unsafe/path.docx')
      input.dispatchEvent(new Event('input',{bubbles:true}))
    })
    expect(container.querySelector<HTMLButtonElement>('form button[type="submit"]')!.disabled).toBe(true)
    await act(async()=>container.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})))
    expect(onCreateWord).not.toHaveBeenCalled()
  })
  it('does not submit a pending operation',async()=>{
    const onCreateWord=vi.fn()
    await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,wordEnabled:true,filesEnabled:true,sideChatEnabled:false,planEnabled:false,
      wordCreation:{phase:'pending',fileName:'pending.docx',message:'Pending'}})))
    await act(async()=>container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
    const submit=container.querySelector<HTMLButtonElement>('form button[type="submit"]')!
    expect(submit.disabled).toBe(true);await act(async()=>submit.click());expect(onCreateWord).not.toHaveBeenCalled()
  })
})

it('opens a retained creation through its explicit callback without issuing another create',async()=>{
  const onCreateWord=vi.fn(),onOpenCreatedWord=vi.fn()
  await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,onOpenCreatedWord,wordEnabled:false,filesEnabled:true,sideChatEnabled:false,planEnabled:false})))
  await act(async()=>container.querySelector<HTMLButtonElement>('[data-word-open-created]')!.click())
  expect(onOpenCreatedWord).toHaveBeenCalledOnce();expect(onCreateWord).not.toHaveBeenCalled()
})

it('keeps the Word form discoverable while real readiness loads, then offers existing plugin guidance',async()=>{
  let finish!:(value:unknown)=>void
  packageList.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  const onCreateWord=vi.fn(),onOpenWordPlugins=vi.fn()
  await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,onOpenWordPlugins,wordEnabled:true,filesEnabled:true,sideChatEnabled:false,planEnabled:false})))
  await act(async()=>container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
  expect(container.querySelector<HTMLInputElement>('form input')).not.toBeNull()
  expect(container.querySelector<HTMLButtonElement>('form button[type="submit"]')!.disabled).toBe(true)
  expect(container.querySelector('[data-word-readiness]')?.textContent).toBe(i18n.t('common:workspaceWordDocumentsChecking'))
  const pkg=readyDocuments(),unset={...pkg,activationState:'unset',activationRevision:0,available:false}
  Reflect.deleteProperty(unset,'desiredState');Reflect.deleteProperty(unset,'activationId')
  await act(async()=>finish({ok:true,packages:[unset]}))
  expect(container.querySelector('[data-word-readiness]')?.textContent).toBe(i18n.t('common:workspaceWordDocumentsNotEnabled'))
  await act(async()=>container.querySelector<HTMLButtonElement>('[data-word-open-plugins]')!.click())
  expect(onOpenWordPlugins).toHaveBeenCalledOnce();expect(onCreateWord).not.toHaveBeenCalled()
  expect(packageList).toHaveBeenCalledExactlyOnceWith({action:'list'})
  await act(async()=>container.querySelector<HTMLButtonElement>('[data-word-readiness-refresh]')!.click())
  expect(container.querySelector<HTMLButtonElement>('form button[type="submit"]')!.disabled).toBe(false)
})
it('allows the same unknown recovery while the new-create readiness query is unresolved',async()=>{
  packageList.mockImplementationOnce(()=>new Promise(()=>{}))
  const onCreateWord=vi.fn()
  await act(async()=>root.render(createElement(WorkspaceToolSelector,{onOpen:vi.fn(),onCreateWord,wordEnabled:true,filesEnabled:true,sideChatEnabled:false,planEnabled:false,
    wordCreation:{phase:'unknown',fileName:'original.docx',message:'Unconfirmed'}})))
  await act(async()=>container.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
  expect(container.querySelector<HTMLInputElement>('form input')!.disabled).toBe(true)
  const submit=container.querySelector<HTMLButtonElement>('form button[type="submit"]')!
  expect(submit.disabled).toBe(false);await act(async()=>submit.click())
  expect(onCreateWord).toHaveBeenCalledExactlyOnceWith('original.docx')
})
