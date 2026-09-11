import {
  DEFAULT_SCHEDULE_INTERNAL_PORT,
  DEFAULT_SCHEDULE_MODEL,
  SCHEDULE_TASK_MESSAGE_KEYS,
  type ScheduleSettingsPatchV1,
  type ScheduleSettingsV1,
  type ScheduleTaskMessageKey,
  type ScheduleTaskStatus,
  type ScheduledTaskV1
} from './app-settings-types'
import {
  compactStrings,
  normalizeAtTime,
  normalizeBoolean,
  normalizePositiveInteger,
  normalizeRunMode,
  normalizeScheduleKind,
  normalizeScheduleReasoningEffort,
  normalizeStatus,
  normalizeTimeOfDay
} from './app-settings-normalizers'

const SCHEDULE_TASK_MESSAGE_KEY_SET = new Set<ScheduleTaskMessageKey>(
  SCHEDULE_TASK_MESSAGE_KEYS
)

const SCHEDULE_TASK_MESSAGE_KEYS_BY_STATUS: Readonly<
  Record<ScheduleTaskStatus, readonly ScheduleTaskMessageKey[]>
> = {
  idle: ['schedule_task_idle'],
  running: ['schedule_task_running', 'schedule_task_started'],
  success: ['schedule_task_completed'],
  error: ['schedule_task_failed', 'schedule_task_interrupted']
}

export function isScheduleTaskMessageKey(value: unknown): value is ScheduleTaskMessageKey {
  return typeof value === 'string' &&
    SCHEDULE_TASK_MESSAGE_KEY_SET.has(value as ScheduleTaskMessageKey)
}

export function scheduleTaskMessageKeyForStatus(
  status: ScheduleTaskStatus
): ScheduleTaskMessageKey {
  return SCHEDULE_TASK_MESSAGE_KEYS_BY_STATUS[status][0]
}

export function normalizeScheduleTaskMessageKey(
  value: unknown,
  status: ScheduleTaskStatus
): ScheduleTaskMessageKey {
  if (isScheduleTaskMessageKey(value) && SCHEDULE_TASK_MESSAGE_KEYS_BY_STATUS[status].includes(value)) {
    return value
  }
  return scheduleTaskMessageKeyForStatus(status)
}

export function scheduleSettingsNeedsMessageKeyMigration(
  input: ScheduleSettingsPatchV1 | undefined
): boolean {
  if (!Array.isArray(input?.tasks)) return false
  return input.tasks.some((candidate) => {
    if (!candidate || typeof candidate !== 'object') return true
    const status = normalizeStatus(candidate.lastStatus)
    return normalizeScheduleTaskMessageKey(candidate.lastMessage, status) !== candidate.lastMessage
  })
}

export function normalizeScheduledTask(
  task: Partial<ScheduledTaskV1>,
  index: number,
  now: string
): ScheduledTaskV1 {
  const schedule = task.schedule
  const model = normalizeScheduleModel(task.model)
  const lastStatus = normalizeStatus(task.lastStatus)
  return {
    id: typeof task.id === 'string' && task.id.trim() ? task.id.trim() : `task-${index + 1}`,
    title: typeof task.title === 'string' && task.title.trim() ? task.title.trim() : `Task ${index + 1}`,
    enabled: normalizeBoolean(task.enabled, true),
    prompt: typeof task.prompt === 'string' ? task.prompt : '',
    workspaceRoot: typeof task.workspaceRoot === 'string' ? task.workspaceRoot.trim() : '',
    clawChannelId: typeof task.clawChannelId === 'string' ? task.clawChannelId.trim() : '',
    providerId: typeof task.providerId === 'string' ? task.providerId.trim() : '',
    model,
    reasoningEffort: normalizeScheduleReasoningEffort(task.reasoningEffort),
    mode: normalizeRunMode(task.mode),
    schedule: {
      kind: normalizeScheduleKind(schedule?.kind),
      everyMinutes: normalizePositiveInteger(schedule?.everyMinutes, 60, 1, 10_080),
      timeOfDay: normalizeTimeOfDay(schedule?.timeOfDay),
      atTime: normalizeAtTime(schedule?.atTime)
    },
    createdAt: typeof task.createdAt === 'string' && task.createdAt ? task.createdAt : now,
    updatedAt: typeof task.updatedAt === 'string' && task.updatedAt ? task.updatedAt : now,
    lastRunAt: typeof task.lastRunAt === 'string' ? task.lastRunAt : '',
    nextRunAt: typeof task.nextRunAt === 'string' ? task.nextRunAt : '',
    lastStatus,
    lastMessage: normalizeScheduleTaskMessageKey(task.lastMessage, lastStatus),
    lastThreadId: typeof task.lastThreadId === 'string' ? task.lastThreadId : ''
  }
}

export function defaultScheduleSettings(): ScheduleSettingsV1 {
  return {
    enabled: false,
    defaultWorkspaceRoot: '',
    providerId: '',
    model: DEFAULT_SCHEDULE_MODEL,
    mode: 'agent',
    promptPrefix: '',
    skills: {
      defaultNames: [],
      extraDirs: [],
      disabledDirs: []
    },
    keepAwake: false,
    internal: {
      port: DEFAULT_SCHEDULE_INTERNAL_PORT,
      secret: ''
    },
    tasks: []
  }
}

export function normalizeScheduleSettings(
  input: ScheduleSettingsPatchV1 | undefined
): ScheduleSettingsV1 {
  const defaults = defaultScheduleSettings()
  const source = input ?? {}
  const skills = source.skills ?? defaults.skills
  const internal = source.internal ?? defaults.internal
  const now = new Date().toISOString()
  return {
    enabled: normalizeBoolean(source.enabled, defaults.enabled),
    defaultWorkspaceRoot:
      typeof source.defaultWorkspaceRoot === 'string' ? source.defaultWorkspaceRoot.trim() : '',
    providerId: typeof source.providerId === 'string' ? source.providerId.trim() : '',
    model: normalizeScheduleModel(source.model),
    mode: normalizeRunMode(source.mode),
    promptPrefix: typeof source.promptPrefix === 'string' ? source.promptPrefix : '',
    skills: {
      defaultNames: compactStrings(skills.defaultNames),
      extraDirs: compactStrings(skills.extraDirs),
      disabledDirs: compactStrings(skills.disabledDirs)
    },
    keepAwake: normalizeBoolean(source.keepAwake, defaults.keepAwake),
    internal: {
      port: normalizePositiveInteger(internal.port, defaults.internal.port, 1024, 65_535),
      secret: typeof internal.secret === 'string' ? internal.secret.trim() : ''
    },
    tasks: Array.isArray(source.tasks)
      ? source.tasks.map((task, index) => normalizeScheduledTask(task as Partial<ScheduledTaskV1>, index, now))
      : []
  }
}

function normalizeScheduleModel(value: unknown): string {
  if (typeof value !== 'string') return DEFAULT_SCHEDULE_MODEL
  const trimmed = value.trim()
  return trimmed && trimmed.toLowerCase() !== 'auto' ? trimmed : DEFAULT_SCHEDULE_MODEL
}

export function mergeScheduleSettings(
  current: ScheduleSettingsV1,
  patch: ScheduleSettingsPatchV1 | undefined
): ScheduleSettingsV1 {
  if (!patch) return normalizeScheduleSettings(current)
  return normalizeScheduleSettings({
    ...current,
    ...patch,
    skills: {
      ...current.skills,
      ...(patch.skills ?? {})
    },
    internal: {
      ...current.internal,
      ...(patch.internal ?? {})
    },
    tasks: patch.tasks ?? current.tasks
  })
}
