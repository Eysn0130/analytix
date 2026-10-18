import type { AppSettingsPatch, AppSettingsV1, ScheduledTaskV1 } from './app-settings'

export const SCHEDULE_TASK_EDITABLE_FIELDS = [
  'title', 'enabled', 'prompt', 'workspaceRoot', 'clawChannelId', 'providerId',
  'model', 'reasoningEffort', 'mode', 'schedule'
] as const
export type ScheduleTaskEditableField = typeof SCHEDULE_TASK_EDITABLE_FIELDS[number]
export type ScheduleTaskEditable = Pick<ScheduledTaskV1, ScheduleTaskEditableField>
export type ScheduleTaskEditPatch = Omit<Partial<ScheduleTaskEditable>, 'schedule'> & { schedule?: Partial<ScheduledTaskV1['schedule']> }
export type ScheduleTaskCreateInput = Omit<ScheduleTaskEditable, 'workspaceRoot' | 'clawChannelId' | 'providerId' | 'model' | 'reasoningEffort' | 'mode' | 'enabled' | 'schedule'> &
  Partial<Pick<ScheduleTaskEditable, 'workspaceRoot' | 'clawChannelId' | 'providerId' | 'model' | 'reasoningEffort' | 'mode' | 'enabled'>> & {
    schedule: Partial<ScheduledTaskV1['schedule']> & Pick<ScheduledTaskV1['schedule'], 'kind'>
  }
export type ScheduleTaskUpdateRequest = {
  taskId: string
  patch: ScheduleTaskEditPatch
  expected: ScheduleTaskEditPatch
}
export type ScheduleTaskDeleteRequest = { taskId: string; expected: ScheduleTaskEditable }
export type ScheduleTaskFailure = { kind: 'unavailable' | 'not_found' | 'conflict' | 'error'; message: string }
export type ScheduleTaskListResult = { kind: 'success'; tasks: ScheduledTaskV1[] } | ScheduleTaskFailure
export type ScheduleTaskWriteResult = { kind: 'success'; task: ScheduledTaskV1 } | ScheduleTaskFailure
export type ScheduleTaskDeleteResult = { kind: 'success'; taskId: string } | ScheduleTaskFailure

const RELATED_FIELDS: readonly (readonly ScheduleTaskEditableField[])[] = [
  ['enabled', 'schedule'], ['providerId', 'model', 'reasoningEffort'], ['workspaceRoot', 'clawChannelId']
]

export function scheduleTaskValuesEqual(left: unknown, right: unknown): boolean {
  if (left === right) return true
  if (!left || !right || typeof left !== 'object' || typeof right !== 'object') return false
  const a = left as Record<string, unknown>
  const b = right as Record<string, unknown>
  const keys = Object.keys(a)
  return keys.length === Object.keys(b).length && keys.every((key) =>
    Object.prototype.hasOwnProperty.call(b, key) && scheduleTaskValuesEqual(a[key], b[key]))
}

export function scheduleTaskEditable(task: ScheduledTaskV1): ScheduleTaskEditable {
  return Object.fromEntries(SCHEDULE_TASK_EDITABLE_FIELDS.map((key) =>
    [key, key === 'schedule' ? { ...task.schedule } : task[key]])) as ScheduleTaskEditable
}

export function scheduleTaskExpectedFields(patch: ScheduleTaskEditPatch): ScheduleTaskEditableField[] {
  const fields = new Set(Object.keys(patch) as ScheduleTaskEditableField[])
  for (const group of RELATED_FIELDS) {
    if (group.some((field) => fields.has(field))) for (const field of group) fields.add(field)
  }
  return [...fields]
}

export function buildScheduleTaskUpdate(
  baseline: ScheduledTaskV1,
  draft: ScheduleTaskEditable
): ScheduleTaskUpdateRequest {
  const patch = Object.fromEntries(SCHEDULE_TASK_EDITABLE_FIELDS
    .filter((key) => !scheduleTaskValuesEqual(baseline[key], draft[key]))
    .map((key) => [key, draft[key]])) as ScheduleTaskEditPatch
  const expected = Object.fromEntries(scheduleTaskExpectedFields(patch)
    .map((key) => [key, baseline[key]])) as ScheduleTaskEditPatch
  return { taskId: baseline.id, patch, expected }
}

export function scheduleTaskEditMatches(task: ScheduledTaskV1, expected: ScheduleTaskEditPatch): boolean {
  return Object.entries(expected).every(([key, value]) =>
    scheduleTaskValuesEqual(task[key as ScheduleTaskEditableField], value))
}

/** Settings snapshots echo tasks for reads; task writes have a separate Host boundary. */
export function ordinarySettingsPatch(settings: AppSettingsV1 | AppSettingsPatch): AppSettingsPatch {
  const { schedule, ...rest } = settings
  if (!schedule) return rest
  const { tasks: _tasks, ...defaults } = schedule
  void _tasks
  return { ...rest, schedule: defaults }
}

export function rejectScheduleTaskArrayWrite(patch: unknown): void {
  if (!patch || typeof patch !== 'object') return
  const schedule = (patch as Record<string, unknown>).schedule
  if (schedule && typeof schedule === 'object' && Object.prototype.hasOwnProperty.call(schedule, 'tasks')) {
    throw new Error('Schedule tasks must be changed through the schedule task API.')
  }
}

/** Immutable saved delivery facts. Only Main may append a validated durable Core ACK. */
export type ScheduleTaskDeliveryRecord = Readonly<{
  attemptId: string
  scheduledAt: string
  deliveredAt: string
  messageId: string
  prompt?: string
}>
export type ScheduleTaskHistoryRequest = { taskId: string; limit: 20; before?: string }
export type ScheduleTaskHistoryResult = {
  kind: 'success'
  records: readonly ScheduleTaskDeliveryRecord[]
  nextBefore?: string
  deliveryKind: 'origin-delivery' | 'legacy-headless'
} | ScheduleTaskFailure
