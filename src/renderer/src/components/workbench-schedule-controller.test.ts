import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
import { openTaskDetail, scheduleTaskWorkspaceTabId } from './workbench-schedule-controller'

beforeEach(() => useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, open: false, selectorOpen: true }))
describe('task-ID workspace navigation', () => {
  it('reopens the same task from either entry and retains independent tasks', () => {
    openTaskDetail('a', 'Creation title')
    openTaskDetail('b', 'Other task')
    useWorkspaceTabsStore.getState().setOpen(false)
    openTaskDetail('a', 'Current title')
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: false,
      activeTabId: scheduleTaskWorkspaceTabId('a'), tabs: [
        { scheduleTaskId: 'a', title: 'Current title', mode: 'schedule-task' },
        { scheduleTaskId: 'b', title: 'Other task' }
      ] })
  })
  it('rejects an absent or noncanonical task identity', () => {
    openTaskDetail('', 'Unknown'); openTaskDetail(' a ', 'Unknown')
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
  })
})
