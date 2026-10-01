import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkspaceTabsStore, workspaceObjectTabId, type WorkspaceTab } from './workspace-tabs-store'

const documentTab = (name: string): WorkspaceTab => ({ id: workspaceObjectTabId('/workspace', `/workspace/${name}`), kind: 'document', mode: 'documents', title: name, path: `/workspace/${name}`, workspaceRoot: '/workspace' })
beforeEach(() => useWorkspaceTabsStore.setState({ open: false, selectorOpen: true, activeTabId: null, tabs: [], focused: false }))

describe('right workspace lifecycle', () => {
  it('opens an empty selector and closes the last tab without folding the workspace', () => {
    const state = useWorkspaceTabsStore.getState()
    state.toggleOpen()
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: true, tabs: [] })
    const tab = documentTab('report.docx')
    state.openTab(tab)
    state.closeTab(tab.id)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: true, activeTabId: null, tabs: [] })
  })
  it('deduplicates an object across preview and document adapter entry points', () => {
    const tab = documentTab('report.docx')
    const state = useWorkspaceTabsStore.getState()
    state.openTab({ ...tab, mode: 'file', kind: 'file' })
    state.openTab({ ...tab, dirty: true })
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([{ ...tab, dirty: true }])
  })
  it('folding and restoring keeps the active object and dirty metadata', () => {
    const state = useWorkspaceTabsStore.getState()
    const a = { ...documentTab('a.docx'), dirty: true }
    const b = documentTab('b.xlsx')
    state.openTab(a); state.openTab(b); state.setFocused(true); state.toggleOpen()
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: false, focused: false, activeTabId: b.id, tabs: [a, b] })
    state.toggleOpen()
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: false, activeTabId: b.id })
  })
  it('plus preserves the last selected object without creating a tool instance', () => {
    const state = useWorkspaceTabsStore.getState()
    const tab = documentTab('a.docx')
    state.openTab(tab); state.showSelector()
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ activeTabId: tab.id, tabs: [tab], selectorOpen: true })
    state.activateTab(tab.id)
    expect(useWorkspaceTabsStore.getState().selectorOpen).toBe(false)
  })
  it('closing a background tab does not change the selected object', () => {
    const state = useWorkspaceTabsStore.getState()
    const a = documentTab('a.docx'); const b = documentTab('b.docx')
    state.openTab(a); state.openTab(b); state.closeTab(a.id)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(b.id)
  })
  it('chooses the next neighbor after close and keeps activation through reorder', () => {
    const state = useWorkspaceTabsStore.getState()
    const a = documentTab('a.docx'); const b = documentTab('b.docx'); const c = documentTab('c.docx')
    state.openTab(a); state.openTab(b); state.openTab(c); state.activateTab(b.id)
    state.closeTab(b.id)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(c.id)
    state.reorderTab(c.id, 0)
    expect(useWorkspaceTabsStore.getState().tabs.map((tab) => tab.id)).toEqual([c.id, a.id])
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(c.id)
  })
  it('status patches never activate a background tab or expand a collapsed dock', () => {
    const state = useWorkspaceTabsStore.getState()
    const a = documentTab('a.docx'); const b = documentTab('b.docx')
    state.openTab(a); state.openTab(b); state.setOpen(false)
    state.patchTab(a.id, { dirty: true, error: true })
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: false, activeTabId: b.id })
    expect(useWorkspaceTabsStore.getState().tabs[0]).toMatchObject({ dirty: true, error: true })
  })
})
