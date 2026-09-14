// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { WorkspaceTabs, WorkspaceToolSelector } from './WorkspaceTabs'
import { useWorkspaceTabsStore, type WorkspaceTab } from '../../store/workspace-tabs-store'

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
  onSelect.mockClear(); onClose.mockClear()
  useWorkspaceTabsStore.setState({ tabs: [a, b], activeTabId: a.id, selectorOpen: false, open: true, focused: false })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
  await act(async () => root.render(createElement(Harness)))
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals()
})
describe('manual workspace tabs', () => {
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
  it('closes a tab by keyboard and retains the workspace', async () => {
    await key(tabButtons()[0], 'Delete')
    expect(onClose).toHaveBeenCalledExactlyOnceWith(a.id)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, activeTabId: b.id })
  })
  it('terminal selector dispatches one action and creates no right-side terminal tab', async () => {
    const onOpen = vi.fn()
    await act(async () => root.render(createElement(WorkspaceToolSelector, { onOpen, filesEnabled: true, sideChatEnabled: false, planEnabled: false })))
    const button = [...container.querySelectorAll('button')].find((item) => item.textContent === i18n.t('common:rightPanelTerminal'))!
    await act(async () => button.click())
    expect(onOpen).toHaveBeenCalledExactlyOnceWith('terminal')
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([a, b])
  })
})
