import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'

/** A navigation identity, never a copy of a task or a runtime authorization. */
export function scheduleTaskWorkspaceTabId(taskId: string): string {
  return 'schedule-task:' + JSON.stringify(taskId)
}

/** The list and every admitted creation card open the same real workspace tab. */
export function openTaskDetail(taskId: string, title: string): void {
  if (!taskId || taskId !== taskId.trim() || taskId.length > 512) return
  useWorkspaceTabsStore.getState().openTab({
    id: scheduleTaskWorkspaceTabId(taskId), kind: 'tool', mode: 'schedule-task', title,
    scheduleTaskId: taskId
  })
}
