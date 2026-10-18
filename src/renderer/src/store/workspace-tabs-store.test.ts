import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkspaceTabsStore, workspaceObjectTabId, type WorkspaceTab, questionPlanWorkspaceTabId, questionPlanReviewIdentity } from './workspace-tabs-store'
import { openTaskDetail, scheduleTaskWorkspaceTabId } from '../components/workbench-schedule-controller'

const documentTab = (name: string): WorkspaceTab => ({ id: workspaceObjectTabId('/workspace', `/workspace/${name}`), kind: 'document', mode: 'documents', title: name, path: `/workspace/${name}`, workspaceRoot: '/workspace' })
beforeEach(() => useWorkspaceTabsStore.setState({ open: false, selectorOpen: true, activeTabId: null, tabs: [], focused: false, openedQuestionPlanReviews: {} }))

describe('right workspace lifecycle', () => {
  it('retains taskId navigation through fold, reorder and close without carrying a task snapshot', () => {
    const state = useWorkspaceTabsStore.getState(), id = scheduleTaskWorkspaceTabId('task-a')
    openTaskDetail('task-a', 'Task A'); state.openTab(documentTab('ordinary.md')); state.activateTab(id)
    state.toggleOpen(); state.toggleOpen(); state.reorderTab(id, 1)
    expect(useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === id)).toEqual({ id, kind: 'tool', mode: 'schedule-task', title: 'Task A', scheduleTaskId: 'task-a' })
    state.closeTab(id)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, activeTabId: documentTab('ordinary.md').id })
  })
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

it('preserves only the protected label/reference/line across fold, reopen and tab selection', () => {
  const protectedRead = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
    viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64) }
  const tab: WorkspaceTab = { id: 'protected-read:' + protectedRead.viewId, kind: 'file', mode: 'file', title: 'private.md', preview: 'source', protectedRead, sourceLine: 9 }
  const state = useWorkspaceTabsStore.getState()
  state.openTab(tab); state.openTab(documentTab('ordinary.md')); state.activateTab(tab.id); state.toggleOpen(); state.toggleOpen()
  expect(useWorkspaceTabsStore.getState()).toMatchObject({ activeTabId: tab.id, open: true })
  expect(useWorkspaceTabsStore.getState().tabs[0]).toEqual(tab)
  state.closeTab(tab.id)
  expect(useWorkspaceTabsStore.getState().tabs.map(tab => tab.title)).toEqual(['ordinary.md'])
})

describe('private Question plan navigation in the existing workspace', () => {
  const requestId = 'input_' + 'd'.repeat(64)
  const owner = { selector: { threadId: 'thread-plan', turnId: 'turn-plan', itemId: 'item_' + requestId, requestId }, callId: 'actual-plan-call' }
  it('opens once automatically, preserves manual close, and explicitly reopens metadata only', () => {
    const state = useWorkspaceTabsStore.getState(), id = questionPlanWorkspaceTabId(owner.selector)
    state.openQuestionPlanReview(owner, 'Plan', true)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, activeTabId: id, selectorOpen: false })
    state.closeTab(id); state.setOpen(false); state.openQuestionPlanReview(owner, 'Plan', true)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: false, tabs: [] })
    state.openQuestionPlanReview(owner, 'Plan')
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([{ id, kind: 'tool', mode: 'plan', title: 'Plan', questionPlan: owner }])
    expect(useWorkspaceTabsStore.getState().openedQuestionPlanReviews[questionPlanReviewIdentity(owner)]).toBe(true)
  })
  it('retains true authority retirement through automatic open, fold and ordinary activation; explicit open requests a fresh read', () => {
    const state = useWorkspaceTabsStore.getState(), id = questionPlanWorkspaceTabId(owner.selector)
    state.openQuestionPlanReview(owner, 'Plan', true); state.retireQuestionPlanReview(owner)
    state.setOpen(false); state.setOpen(true); state.activateTab(id); state.openQuestionPlanReview(owner, 'Plan', true)
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBe(true)
    state.openQuestionPlanReview(owner, 'Plan')
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBeUndefined()
    state.retireQuestionPlanReview({ ...owner, callId: 'foreign-call' })
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBeUndefined()
  })
  it('keeps the GUI file plan separate and never inherits private metadata by shallow tab merge', () => {
    const state = useWorkspaceTabsStore.getState(); state.openQuestionPlanReview(owner, 'Plan')
    state.openTab({ id: 'tool:plan', kind: 'tool', mode: 'plan', title: 'GUI plan' })
    expect(useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === 'tool:plan')?.questionPlan).toBeUndefined()
    state.activateTab(questionPlanWorkspaceTabId(owner.selector)); expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(2)
  })
  it('scopes call identity by thread and uses request identity for nullable machine association', () => {
    const state = useWorkspaceTabsStore.getState(); state.openQuestionPlanReview(owner, 'Plan', true)
    state.openQuestionPlanReview({ ...owner, selector: { ...owner.selector, threadId: 'another-thread' } }, 'Plan', true)
    state.openQuestionPlanReview({ ...owner, callId: null }, 'Plan', true)
    expect(Object.keys(useWorkspaceTabsStore.getState().openedQuestionPlanReviews)).toHaveLength(3)
    expect(questionPlanReviewIdentity({ ...owner, callId: null })).toContain('review:' + requestId)
  })
})
