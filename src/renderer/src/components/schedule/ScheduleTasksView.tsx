import type { ReactElement, ReactNode } from 'react'
import { isValidScheduleTimeZone, isValidScheduleTimeOfDay, validateScheduleCalendarRule } from '../../../../../packages/runtime/src/contracts/schedule-rule'
import { useModalFocus } from '../../hooks/use-modal-focus'
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Brain,
  CalendarClock,
  ChevronDown,
  Clock3,
  Folder,
  FolderOpen,
  MessageSquare,
  MoreHorizontal,
  PencilLine,
  Play,
  Plus,
  Power,
  Timer,
  Trash2,
  X
} from '../../design/AnalytixUiIcons'
import {
  DEFAULT_SCHEDULE_MODEL,
  DEFAULT_SCHEDULE_REASONING_EFFORT,
  SCHEDULE_REASONING_EFFORT_IDS,
  getAnalytixRuntimeSettings,
  getModelProviderSettings,
  isComposerChatModelId,
  listNonTextModelIds,
  mergeScheduleSettings,
  modelProfileSupportsTextChat,
  modelProviderModelProfile,
  normalizeScheduleSettings,
  type AppSettingsV1,
  type ClawImChannelV1,
  type ModelProviderModelProfileV1,
  type ScheduleKind,
  type ScheduleReasoningEffort,
  type ScheduleRuntimeStatus,
  type ScheduleSettingsV1,
  type ScheduleTaskMessageKey,
  type ScheduledTaskV1
} from '@shared/app-settings'
import { buildScheduleTaskUpdate, scheduleTaskEditable, type ScheduleTaskEditPatch } from '@shared/schedule-task-mutation'
import { scheduleTaskCatalog } from './schedule-task-catalog'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { confirmDialog } from '../../lib/confirm-dialog'
import { desktopQueryCache } from '../../lib/desktop-query-cache'
import { formatWorkspacePickerError } from '../../lib/format-workspace-picker-error'
import { usePersistentStringState } from '../../lib/use-persistent-string-state'
import { ScheduleDefaultsDialog } from './ScheduleDefaultsDialog'

type Props = {
  leftSidebarCollapsed: boolean
  onOpenThread?: (threadId: string) => void
  onOpenTaskDetail?: (taskId: string, title: string) => void
}

type TaskFilter = 'all' | 'enabled' | 'running' | 'done'
type ScheduleClientMode = 'code' | 'im'
type ScheduleModelProviderOption = {
  providerId: string
  label: string
  modelIds: string[]
  modelProfiles?: Record<string, ModelProviderModelProfileV1>
}
type TaskDialogState =
  | { mode: 'create'; draft: ScheduledTaskV1 }
  | { mode: 'edit'; taskId: string; baseline: ScheduledTaskV1; draft: ScheduledTaskV1 }

const SCHEDULE_FILTERS: TaskFilter[] = ['all', 'enabled', 'running', 'done']
const SCHEDULE_FILTER_STORAGE_KEY = 'analytix.schedule.filter.v1'
const SCHEDULE_STATUS_STALE_MS = 4_000
const SCHEDULE_KIND_OPTIONS: ScheduleKind[] = ['daily', 'weekly', 'cron', 'at', 'interval', 'manual']
const SCHEDULE_REASONING_OPTIONS: ScheduleReasoningEffort[] = [...SCHEDULE_REASONING_EFFORT_IDS]
const TIME_HOURS = Array.from({ length: 24 }, (_item, index) => String(index).padStart(2, '0'))
const TIME_MINUTES = Array.from({ length: 60 }, (_item, index) => String(index).padStart(2, '0'))
function modelIdsEqual(left: string, right: string): boolean {
  return left.trim().toLowerCase() === right.trim().toLowerCase()
}

function firstConcreteScheduleModel(modelIds: readonly string[], fallback = DEFAULT_SCHEDULE_MODEL): string {
  return modelIds.find((model) => model.trim() && model.trim().toLowerCase() !== 'auto') ?? fallback
}

export function scheduleModelProviderOptions(settings: AppSettingsV1): ScheduleModelProviderOption[] {
  const nonTextModelIds = listNonTextModelIds(settings)
  return getModelProviderSettings(settings).providers
    .map((provider) => {
      const modelIds = provider.models
        .map((model) => model.trim())
        .filter((model) =>
          isComposerChatModelId(model, nonTextModelIds) &&
          modelProfileSupportsTextChat(modelProviderModelProfile(provider, model))
        )
      return {
        providerId: provider.id,
        label: provider.name.trim() || provider.id,
        modelIds,
        modelProfiles: provider.modelProfiles
      }
    })
    .filter((provider) => provider.modelIds.length > 0)
}

export function resolveScheduleModelSelection(
  providers: readonly ScheduleModelProviderOption[],
  providerId: string | undefined,
  model: string | undefined
): { providerId: string; model: string } {
  const requestedProviderId = providerId?.trim() ?? ''
  const requestedModel = model?.trim() ?? ''
  const providerById = providers.find((provider) => provider.providerId === requestedProviderId)
  const providerByModel = requestedModel && requestedModel.toLowerCase() !== 'auto'
    ? providers.find((provider) => provider.modelIds.some((id) => modelIdsEqual(id, requestedModel)))
    : undefined
  const provider = providerById ?? providerByModel ?? providers[0]
  if (!provider) {
    return {
      providerId: requestedProviderId,
      model: requestedModel && requestedModel.toLowerCase() !== 'auto' ? requestedModel : DEFAULT_SCHEDULE_MODEL
    }
  }
  const selectedModel =
    requestedModel &&
    requestedModel.toLowerCase() !== 'auto' &&
    provider.modelIds.some((id) => modelIdsEqual(id, requestedModel))
      ? requestedModel
      : firstConcreteScheduleModel(provider.modelIds)
  return {
    providerId: provider.providerId,
    model: selectedModel
  }
}

function preferredScheduleProviderId(
  settings: AppSettingsV1,
  providers: readonly ScheduleModelProviderOption[],
  configuredProviderId: string | undefined
): string {
  const configured = configuredProviderId?.trim() ?? ''
  if (providers.some((provider) => provider.providerId === configured)) return configured
  const runtimeProviderId = getAnalytixRuntimeSettings(settings).providerId.trim()
  if (providers.some((provider) => provider.providerId === runtimeProviderId)) return runtimeProviderId
  return providers[0]?.providerId ?? ''
}

function isScheduleReasoningEffort(value: string): value is ScheduleReasoningEffort {
  return SCHEDULE_REASONING_OPTIONS.includes(value as ScheduleReasoningEffort)
}

function scheduleReasoningOptionsForModel(
  profile: Pick<ModelProviderModelProfileV1, 'reasoning'> | undefined
): ScheduleReasoningEffort[] {
  const supported = profile?.reasoning?.supportedEfforts
    .filter(isScheduleReasoningEffort) ?? []
  if (supported.length === 0) return SCHEDULE_REASONING_OPTIONS
  return SCHEDULE_REASONING_OPTIONS.filter((effort) => supported.includes(effort))
}

function resolveScheduleReasoningSelection(
  value: ScheduleReasoningEffort | undefined,
  profile: Pick<ModelProviderModelProfileV1, 'reasoning'> | undefined
): ScheduleReasoningEffort {
  const options = scheduleReasoningOptionsForModel(profile)
  if (value && options.includes(value)) return value
  const defaultEffort = profile?.reasoning?.defaultEffort
  if (defaultEffort && isScheduleReasoningEffort(defaultEffort) && options.includes(defaultEffort)) {
    return defaultEffort
  }
  return options.includes(DEFAULT_SCHEDULE_REASONING_EFFORT)
    ? DEFAULT_SCHEDULE_REASONING_EFFORT
    : options[0] ?? DEFAULT_SCHEDULE_REASONING_EFFORT
}

function scheduleModelProfileForSelection(
  provider: ScheduleModelProviderOption | null | undefined,
  model: string
): ModelProviderModelProfileV1 | undefined {
  return provider?.modelProfiles ? modelProviderModelProfile({ modelProfiles: provider.modelProfiles }, model) : undefined
}

function nowIso(): string {
  return new Date().toISOString()
}

export function scheduleDefaultTimeZone(): string {
  const zone = new Intl.DateTimeFormat().resolvedOptions().timeZone
  return isValidScheduleTimeZone(zone) ? zone : 'UTC'
}

/** Called only by real timing controls; opening/saving ordinary edits never migrates a rule. */
export function editScheduleTiming(schedule: ScheduledTaskV1['schedule'], patch: Partial<ScheduledTaskV1['schedule']>): ScheduledTaskV1['schedule'] {
  if (Object.entries(patch).every(([key, value]) => JSON.stringify(schedule[key as keyof typeof schedule]) === JSON.stringify(value))) return schedule
  const next = { ...schedule, ...patch }
  if (next.kind === 'daily' || next.kind === 'weekly' || next.kind === 'cron') {
    if (!Object.prototype.hasOwnProperty.call(next, 'timeZone')) next.timeZone = scheduleDefaultTimeZone()
    if (next.kind === 'weekly') next.weekdays ??= [1, 2, 3, 4, 5]
    else delete next.weekdays
    if (next.kind === 'cron') next.expression ??= '0 9 * * *'
    else delete next.expression
  } else {
    delete next.timeZone; delete next.weekdays; delete next.expression
  }
  return next
}

export function newScheduledTask(workspaceRoot: string, defaults?: Partial<ScheduledTaskV1>): ScheduledTaskV1 {
  const now = nowIso()
  const schedule: ScheduledTaskV1['schedule'] = { kind: 'daily', everyMinutes: 60, timeOfDay: '09:00', atTime: '', ...defaults?.schedule }
  if ((schedule.kind === 'daily' || schedule.kind === 'weekly' || schedule.kind === 'cron') && !Object.prototype.hasOwnProperty.call(schedule, 'timeZone')) schedule.timeZone = scheduleDefaultTimeZone()
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `schedule-${Date.now()}`,
    title: '',
    enabled: true,
    prompt: '',
    workspaceRoot,
    clawChannelId: '',
    providerId: '',
    model: DEFAULT_SCHEDULE_MODEL,
    reasoningEffort: DEFAULT_SCHEDULE_REASONING_EFFORT,

    createdAt: now,
    updatedAt: now,
    lastRunAt: '',
    nextRunAt: '',
    lastStatus: 'idle',
    lastMessage: 'schedule_task_idle',
    lastThreadId: '',
    ...defaults,
    schedule,
    mode: 'agent'
  }
}

export function dateTimeLocalValueFromIso(value: string): string {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ''
  const pad = (part: number): string => String(part).padStart(2, '0')
  return [
    date.getFullYear(),
    '-',
    pad(date.getMonth() + 1),
    '-',
    pad(date.getDate()),
    'T',
    pad(date.getHours()),
    ':',
    pad(date.getMinutes())
  ].join('')
}

export function isoFromDateTimeLocalValue(value: string): string {
  if (!value.trim()) return ''
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toISOString() : ''
}

export function scheduleTaskSummary(
  task: Pick<ScheduledTaskV1, 'schedule'>,
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  if (task.schedule.kind === 'at') {
    return t('scheduleAt', {
      datetime: task.schedule.atTime ? new Date(task.schedule.atTime).toLocaleString() : '-'
    })
  }
  if (task.schedule.kind === 'interval') {
    return t('scheduleEvery', { minutes: task.schedule.everyMinutes })
  }
  if (task.schedule.kind === 'daily') {
    return t('scheduleDailyAt', { time: task.schedule.timeOfDay }) + (task.schedule.timeZone ? ` · ${task.schedule.timeZone}` : '')
  }
  if (task.schedule.kind === 'weekly') return t('scheduleWeeklyAt', { days: task.schedule.weekdays?.map(day => t(`scheduleWeekday_${day}`)).join(', '), time: task.schedule.timeOfDay, zone: task.schedule.timeZone })
  if (task.schedule.kind === 'cron') return t('scheduleCronAt', { expression: task.schedule.expression, zone: task.schedule.timeZone })
  return t('scheduleManual')
}

function scheduleReasoningLabel(
  value: ScheduleReasoningEffort,
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  return t(`scheduleReasoning_${value}`)
}

function clawChannelDisplayName(channel: ClawImChannelV1): string {
  return channel.agentProfile.name.trim() || channel.label.trim() || channel.provider
}

export function scheduleImProviderLabel(
  channel: Pick<ClawImChannelV1, 'provider' | 'platformAccount'>,
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  if (channel.provider === 'telegram') return t('scheduleImProviderTelegram')
  if (channel.provider === 'weixin') return t('scheduleImProviderWeixin')
  if (channel.platformAccount?.kind === 'feishu' && channel.platformAccount.domain === 'lark') {
    return t('scheduleImProviderLark')
  }
  return t('scheduleImProviderFeishu')
}

export function scheduleImChannelOptionLabel(
  channel: ClawImChannelV1,
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  return `${clawChannelDisplayName(channel)} (${scheduleImProviderLabel(channel, t)})`
}

function configuredScheduleImChannels(channels: ClawImChannelV1[]): ClawImChannelV1[] {
  return channels.filter((channel) => channel.enabled)
}

export function preferredScheduleImChannel(channels: ClawImChannelV1[]): ClawImChannelV1 | null {
  const configured = configuredScheduleImChannels(channels)
  return configured.find((channel) => channel.provider === 'weixin') ??
    configured.find((channel) => channel.provider === 'feishu') ??
    configured[0] ??
    null
}

export function scheduledTaskClawLabel(
  task: Pick<ScheduledTaskV1, 'clawChannelId'>,
  channels: ClawImChannelV1[],
  t: (key: string, values?: Record<string, unknown>) => string
): string {
  const channelId = task.clawChannelId.trim()
  if (!channelId) return ''
  const channel = channels.find((item) => item.id === channelId)
  return channel
    ? t('scheduleClawAgentLabel', { name: scheduleImChannelOptionLabel(channel, t) })
    : t('scheduleClawAgentMissing')
}

export function validateScheduledTaskDraft(
  task: ScheduledTaskV1,
  t: (key: string, values?: Record<string, unknown>) => string,
  now = new Date()
): string | null {
  if (!task.title.trim()) return t('scheduleTaskNameRequired')
  if (task.title.trim().length > 50) return t('scheduleTaskNameTooLong')
  if (!task.prompt.trim()) return t('scheduleTaskPromptRequired')
  if (task.prompt.length > 8_000) return t('scheduleTaskPromptTooLong')
  if (task.schedule.kind === 'interval' && (!Number.isFinite(task.schedule.everyMinutes) || task.schedule.everyMinutes < 1)) {
    return t('scheduleIntervalInvalid')
  }
  if ((task.schedule.kind === 'daily' || task.schedule.kind === 'weekly') && !isValidScheduleTimeOfDay(task.schedule.timeOfDay)) {
    return t('scheduleDailyTimeInvalid')
  }
  try { validateScheduleCalendarRule(task.schedule, true) }
  catch {
    if ((task.schedule.kind === 'daily' || task.schedule.kind === 'weekly' || task.schedule.kind === 'cron') && !isValidScheduleTimeZone(task.schedule.timeZone)) return t('scheduleTimeZoneInvalid')
    return t(task.schedule.kind === 'weekly' ? 'scheduleWeekdaysInvalid' : 'scheduleCronInvalid')
  }
  if (task.schedule.kind === 'at') {
    const runAt = Date.parse(task.schedule.atTime)
    if (!Number.isFinite(runAt)) return t('scheduleAtTimeInvalid')
    if (task.enabled && runAt <= now.getTime()) return t('scheduleAtTimePast')
  }
  return null
}

export function filterScheduledTasks(tasks: ScheduledTaskV1[], filter: TaskFilter): ScheduledTaskV1[] {
  const filtered = tasks.filter((task) => {
    if (filter === 'enabled') return task.enabled
    if (filter === 'running') return task.lastStatus === 'running'
    if (filter === 'done') return task.lastStatus === 'success' || task.lastStatus === 'error'
    return true
  })
  return [...filtered].sort((a, b) => {
    const aNext = Date.parse(a.nextRunAt)
    const bNext = Date.parse(b.nextRunAt)
    if (Number.isFinite(aNext) && Number.isFinite(bNext)) return aNext - bNext
    if (Number.isFinite(aNext)) return -1
    if (Number.isFinite(bNext)) return 1
    return Date.parse(b.updatedAt) - Date.parse(a.updatedAt)
  })
}

export function scheduledTaskLastThreadId(task: Pick<ScheduledTaskV1, 'lastThreadId'>): string {
  return task.lastThreadId.trim()
}

type ScheduleTaskMessageTranslationKey =
  | 'scheduleStatus_idle'
  | 'scheduleStatus_running'
  | 'scheduleStatus_success'
  | 'scheduleStatus_error'
  | 'scheduleDeliveryAccepted'
  | 'scheduleDeliveryPending'

const SCHEDULE_TASK_MESSAGE_TRANSLATIONS: Readonly<
  Partial<Record<ScheduleTaskMessageKey, ScheduleTaskMessageTranslationKey>>
> = {
  schedule_task_idle: 'scheduleStatus_idle',
  schedule_task_running: 'scheduleStatus_running',
  schedule_task_started: 'scheduleStatus_running',
  schedule_task_completed: 'scheduleStatus_success',
  schedule_task_accepted: 'scheduleDeliveryAccepted',
  schedule_task_failed: 'scheduleStatus_error',
  schedule_task_interrupted: 'scheduleStatus_error'
}

export function scheduledTaskMessageTranslationKey(
  task: Pick<ScheduledTaskV1, 'lastMessage' | 'lastStatus' | 'deliveryKind'>
): ScheduleTaskMessageTranslationKey {
  if (task.deliveryKind === 'origin-delivery') {
    if (task.lastStatus === 'success') return 'scheduleDeliveryAccepted'
    if (task.lastStatus === 'running') return 'scheduleDeliveryPending'
  }
  const key = task.lastMessage
  if (key) {
    const translation = SCHEDULE_TASK_MESSAGE_TRANSLATIONS[key]
    if (translation) return translation
  }
  if (task.lastStatus === 'running') return 'scheduleStatus_running'
  if (task.lastStatus === 'success') return 'scheduleStatus_success'
  if (task.lastStatus === 'error') return 'scheduleStatus_error'
  return 'scheduleStatus_idle'
}

function formatDateTime(value: string, fallback: string): string {
  if (!value.trim()) return fallback
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return fallback
  return date.toLocaleString()
}

function statusTone(status: ScheduledTaskV1['lastStatus']): string {
  if (status === 'running') return 'bg-amber-500/15 text-amber-900 dark:text-amber-100'
  if (status === 'success') return 'bg-emerald-500/15 text-emerald-800 dark:text-emerald-100'
  if (status === 'error') return 'bg-red-500/15 text-red-700 dark:text-red-100'
  return 'bg-ds-subtle text-ds-muted'
}

export function ScheduleTasksView({
  leftSidebarCollapsed,
  onOpenThread,
  onOpenTaskDetail
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const catalog = useSyncExternalStore(scheduleTaskCatalog.subscribe, scheduleTaskCatalog.getSnapshot)
  const [settings, setSettings] = useState<AppSettingsV1 | null>(null)
  const [status, setStatus] = useState<ScheduleRuntimeStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = usePersistentStringState<TaskFilter>(
    SCHEDULE_FILTER_STORAGE_KEY,
    'all',
    SCHEDULE_FILTERS
  )
  const dialogGeneration = useRef(0)
  const saveRequest = useRef(0)
  const pendingSaveGeneration = useRef<number | null>(null)
  const [dialogSaving, setDialogSaving] = useState(false)
  const pickerRequest = useRef(0)
  const [dialog, setDialog] = useState<TaskDialogState | null>(null)
  const [dialogError, setDialogError] = useState<string | null>(null)
  const [settingsDialogOpen, setSettingsDialogOpen] = useState(false)
  const load = useCallback(async (): Promise<void> => {
    const getScheduleStatus = window.analytix?.schedule?.getStatus
    try {
      const [nextSettings, nextStatus] = await Promise.all([
        rendererRuntimeClient.getSettings(),
        typeof getScheduleStatus === 'function'
          ? desktopQueryCache.fetchQuery({
              queryKey: ['schedule', 'status'],
              staleTimeMs: SCHEDULE_STATUS_STALE_MS,
              fetcher: () => getScheduleStatus()
            })
          : Promise.resolve(null)
      ])
      setSettings(nextSettings)
      setStatus(nextStatus)
      setError(null)
    } catch (loadError) {
      void loadError
      setError(t('scheduleStatus_error'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void load()
    const id = window.setInterval(() => { void load(); void scheduleTaskCatalog.refresh() }, 5_000)
    return () => window.clearInterval(id)
  }, [load])

  const catalogError = catalog.phase === 'error' || catalog.phase === 'unavailable'
  const schedule = settings ? normalizeScheduleSettings(settings.schedule) : null
  const tasks = catalog.tasks
  const clawChannels = settings?.claw.channels ?? []
  const modelProviders = useMemo(
    () => settings ? scheduleModelProviderOptions(settings) : [],
    [settings]
  )
  const runningTaskIds = useMemo(() => new Set(status?.runningTaskIds ?? []), [status])
  const visibleTasks = useMemo(() => filterScheduledTasks(tasks, filter), [filter, tasks])

  const persistSchedule = async (patch: Parameters<typeof mergeScheduleSettings>[1]): Promise<void> => {
    if (!settings || !patch) return
    const { tasks: _tasks, ...defaults } = patch
    void _tasks
    const saved = await rendererRuntimeClient.setSettings({ schedule: defaults })
    setSettings(saved)
    desktopQueryCache.invalidateQueries({ queryKey: ['schedule'], broadcast: true })
    await load()
  }

  const resolveDialogWorkspaceRoot = useCallback((workspaceRoot?: string): string => {
    const explicit = workspaceRoot?.trim() || ''
    if (explicit) return explicit
    return schedule?.defaultWorkspaceRoot.trim() || settings?.workspaceRoot.trim() || ''
  }, [schedule?.defaultWorkspaceRoot, settings?.workspaceRoot])

  const closeDialog = (): void => { dialogGeneration.current += 1; setDialog(null); setDialogError(null); setDialogSaving(false) }

  useEffect(() => () => { dialogGeneration.current += 1 }, [])

  const openCreateDialog = (): void => {
    dialogGeneration.current += 1
    setDialogSaving(false)
    const workspaceRoot = resolveDialogWorkspaceRoot()
    const selection = settings
      ? resolveScheduleModelSelection(
          modelProviders,
          preferredScheduleProviderId(settings, modelProviders, schedule?.providerId),
          schedule?.model || DEFAULT_SCHEDULE_MODEL
        )
      : { providerId: '', model: DEFAULT_SCHEDULE_MODEL }
    const selectedProvider = modelProviders.find((provider) => provider.providerId === selection.providerId) ?? null
    const selectedProfile = scheduleModelProfileForSelection(selectedProvider, selection.model)
    setDialog({ mode: 'create', draft: newScheduledTask(workspaceRoot, {
      providerId: selection.providerId,
      model: selection.model,
      reasoningEffort: resolveScheduleReasoningSelection(DEFAULT_SCHEDULE_REASONING_EFFORT, selectedProfile)
    }) })
    setDialogError(null)
  }

  const openEditDialog = (task: ScheduledTaskV1): void => {
    if (onOpenTaskDetail) { onOpenTaskDetail(task.id, task.title); return }
    dialogGeneration.current += 1
    setDialogSaving(false)
    setDialog({
      mode: 'edit',
      taskId: task.id,
      baseline: task,
      draft: {
        ...task,
        schedule: { ...task.schedule }
      }
    })
    setDialogError(null)
  }

  const pickDialogWorkspace = async (): Promise<void> => {
    if (!dialog) return
    const generation = dialogGeneration.current
    const request = ++pickerRequest.current
    const current = (): boolean => generation === dialogGeneration.current && request === pickerRequest.current
    try {
      if (typeof window.analytix?.workspace?.pickDirectory !== 'function') {
        throw new Error(t('workspacePickerUnavailable'))
      }
      const picked = await window.analytix.workspace.pickDirectory(resolveDialogWorkspaceRoot(dialog.draft.workspaceRoot) || undefined)
      if (!current() || picked.canceled || !picked.path) return
      onDraftChangeInDialog({ workspaceRoot: picked.path })
      setDialogError(null)
    } catch (error) {
      if (current()) setDialogError(formatWorkspacePickerError(error))
    }
  }

  const onDraftChangeInDialog = (patch: Partial<ScheduledTaskV1>): void => {
    setDialog((current) => current ? { ...current, draft: { ...current.draft, ...patch } } : current)
  }

  const saveDialog = async (): Promise<void> => {
    if (!dialog || !schedule || !settings || pendingSaveGeneration.current === dialogGeneration.current) return
    const validation = validateScheduledTaskDraft(dialog.draft, t)
    if (validation) {
      setDialogError(validation)
      return
    }
    const task = {
      ...dialog.draft,
      title: dialog.draft.title.trim(),
      workspaceRoot: dialog.mode === 'create'
        ? resolveDialogWorkspaceRoot(dialog.draft.workspaceRoot) : dialog.draft.workspaceRoot
    }
    const generation = dialogGeneration.current
    pendingSaveGeneration.current = generation
    setDialogSaving(true)
    const request = ++saveRequest.current
    const current = (): boolean => generation === dialogGeneration.current && request === saveRequest.current
    try {
      const api = window.analytix?.schedule
      const result = dialog.mode === 'create'
        ? await api.createTask(scheduleTaskEditable(task))
        : await api.updateTask(buildScheduleTaskUpdate(dialog.baseline, scheduleTaskEditable(task)))
      scheduleTaskCatalog.invalidate()
      if (!current()) return
      if (result.kind !== 'success') {
        setDialogError(result.message)
        return
      }
      closeDialog()
      setDialogError(null)
    } catch {
      if (current()) setDialogError(t('scheduleStatus_error'))
    } finally {
      if (pendingSaveGeneration.current === generation) pendingSaveGeneration.current = null
      if (current()) setDialogSaving(false)
    }
  }

  const updateTask = async (taskId: string, patch: ScheduleTaskEditPatch): Promise<void> => {
    const task = tasks.find((item) => item.id === taskId)
    if (!task) return
    try {
      const draft = { ...scheduleTaskEditable(task), ...patch, schedule: { ...task.schedule, ...patch.schedule } }
      const result = await window.analytix.schedule.updateTask(buildScheduleTaskUpdate(task, draft))
      if (result.kind !== 'success') setError(result.message)
      scheduleTaskCatalog.invalidate()
    } catch { setError(t('scheduleStatus_error')) }
  }

  const deleteTask = async (taskId: string): Promise<void> => {
    const task = tasks.find((item) => item.id === taskId)
    if (!task || !(await confirmDialog(t('scheduleDeleteConfirm')))) return
    try {
      const result = await window.analytix.schedule.deleteTask({ taskId, expected: scheduleTaskEditable(task) })
      if (result.kind !== 'success') setError(result.message)
      scheduleTaskCatalog.invalidate()
    } catch { setError(t('scheduleStatus_error')) }
  }

  const runTask = async (taskId: string): Promise<void> => {
    if (typeof window.analytix?.schedule?.runTask !== 'function') return
    const result = await window.analytix.schedule.runTask(taskId)
    if (!result.ok) {
      setError(t('scheduleStatus_error'))
      return
    }
    scheduleTaskCatalog.invalidate()
    await load()
  }

  const toggleKeepAwake = async (value: boolean): Promise<void> => {
    await persistSchedule({ keepAwake: value })
  }

  return (
    <div className="ds-no-drag flex h-full min-h-0 flex-col bg-ds-main">
      <div className="ds-stage-inset shrink-0">
        <header className="ds-topbar-surface relative z-10 mt-3 flex min-h-[46px] w-full items-stretch overflow-visible rounded-[24px]">
          <div className="grid w-full min-w-0 items-center gap-2.5 px-3 py-2 sm:px-4 md:pl-5 md:pr-2">
            <div
              className={`ds-shell-controls-safe-motion flex min-w-0 items-center gap-2.5 ${
                leftSidebarCollapsed ? 'ds-shell-controls-safe-inset' : ''
              }`}
            >
              <h1 className="min-w-0 flex-1 truncate text-[15px] font-medium text-ds-muted">
                {t('schedule')}
              </h1>
            </div>
          </div>
        </header>
      </div>

      <main className="ds-no-drag min-h-0 flex-1 overflow-y-auto px-6 pb-8 pt-8">
        <div className="mx-auto flex w-full max-w-[880px] flex-col gap-8">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-[14px] leading-6 text-ds-faint">
              {t('scheduleSubtitle')}
            </p>
            <div className="flex items-center gap-2">
              <select
                value={filter}
                onChange={(event) => setFilter(event.target.value as TaskFilter)}
                className="rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[13px] text-ds-ink shadow-sm outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/25"
              >
                {SCHEDULE_FILTERS.map((item) => (
                  <option key={item} value={item}>{t(`scheduleFilter_${item}`)}</option>
                ))}
              </select>
              <button
                type="button"
                onClick={() => setSettingsDialogOpen(true)}
                className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-ds-border bg-ds-card text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink"
                title={t('scheduleDefaultsTitle')}
                aria-label={t('scheduleDefaultsTitle')}
              >
                <MoreHorizontal className="h-4 w-4" strokeWidth={1.8} />
              </button>
              <button
                type="button"
                onClick={openCreateDialog}
                className="inline-flex items-center gap-2 rounded-xl bg-ds-userbubble px-4 py-2 text-[13px] font-semibold text-ds-userbubbleFg shadow-sm transition hover:opacity-90"
              >
                <Plus className="h-4 w-4" strokeWidth={2} />
                {t('scheduleNewTask')}
              </button>
            </div>
          </div>

          <div className="flex items-center justify-between gap-3 rounded-xl border border-ds-border bg-ds-card px-4 py-3">
            <div className="flex min-w-0 items-center gap-3">
              <Clock3 className="h-4 w-4 shrink-0 text-ds-muted" strokeWidth={1.75} />
              <span className="min-w-0 text-[14px] text-ds-ink">
                {t('scheduleAwakeNotice')}
              </span>
            </div>
            <label className="flex shrink-0 items-center gap-2 text-[13px] font-medium text-ds-muted">
              {t('scheduleKeepAwake')}
              <input
                type="checkbox"
                checked={Boolean(schedule?.keepAwake)}
                onChange={(event) => void toggleKeepAwake(event.target.checked)}
                className="sr-only"
              />
              <span className={`relative h-5 w-9 rounded-full transition ${schedule?.keepAwake ? 'bg-ds-ink' : 'bg-ds-border-strong'}`}>
                <span className={`absolute top-0.5 h-4 w-4 rounded-full shadow transition ${schedule?.keepAwake ? 'left-[18px] bg-[var(--ds-composer-primary-action-fg)]' : 'left-0.5 bg-ds-card'}`} />
              </span>
            </label>
          </div>

          {(error || catalogError) && tasks.length > 0 ? (
            <div role="alert" className="mb-3 text-[13px] text-red-700 dark:text-red-200">
              {error || t('scheduleStatus_error')}
              <button type="button" className="ml-3 underline" onClick={() => { setError(null); void scheduleTaskCatalog.refresh() }}>{t('retryConnection')}</button>
            </div>
          ) : null}
          {loading || (tasks.length === 0 && (catalog.phase === 'idle' || catalog.phase === 'loading')) ? (
            <div className="py-20 text-center text-[14px] text-ds-faint">{t('loading')}</div>
          ) : tasks.length === 0 && (error || catalogError) ? (
            <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-[13px] text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">
              {error || t('scheduleStatus_error')}
              <button type="button" className="ml-3 underline" onClick={() => { setError(null); void scheduleTaskCatalog.refresh() }}>{t('retryConnection')}</button>
            </div>
          ) : visibleTasks.length === 0 ? (
            <div className="flex min-h-[340px] items-center justify-center text-[13px] text-ds-faint">
              {tasks.length === 0 ? t('scheduleEmpty') : t('scheduleFilterEmpty')}
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {visibleTasks.map((task) => {
                const running = runningTaskIds.has(task.id) || task.lastStatus === 'running'
                const lastThreadId = scheduledTaskLastThreadId(task)
                const clawLabel = scheduledTaskClawLabel(task, clawChannels, t)
                const providerLabel = task.providerId
                  ? modelProviders.find((provider) => provider.providerId === task.providerId)?.label
                  : ''
                const modelLabel = providerLabel ? `${providerLabel} / ${task.model}` : task.model
                return (
                  <div
                    key={task.id}
                    className="rounded-xl border border-ds-border bg-ds-card px-4 py-4 shadow-sm"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 flex-1">
                        <div className="flex min-w-0 items-center gap-2">
                          <h2 className="truncate text-[15px] font-semibold text-ds-ink">
                            {onOpenTaskDetail ? <button type="button" className="truncate text-left hover:underline"
                              aria-label={t('dshSchedule.openTask', { title: task.title, defaultValue: 'Open {{title}}' })}
                              onClick={() => onOpenTaskDetail(task.id, task.title)}>{task.title || t('scheduleUntitled')}</button>
                              : task.title || t('scheduleUntitled')}
                          </h2>
                          <span className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${statusTone(task.lastStatus)}`}>
                            {task.deliveryKind === 'origin-delivery' ? t(scheduledTaskMessageTranslationKey(task)) : running ? t('scheduleStatus_running') : t(`scheduleStatus_${task.lastStatus}`)}
                          </span>
                        </div>
                        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[12px] text-ds-faint">
                          <span>{scheduleTaskSummary(task, t)}</span>
                          <span>{t('scheduleNextRun')}: {formatDateTime(task.nextRunAt, t('scheduleNotScheduled'))}</span>
                          <span>{t('scheduleLastRun')}: {formatDateTime(task.lastRunAt, t('scheduleNeverRun'))}</span>
                          {clawLabel ? <span>{clawLabel}</span> : null}
                          <span>{modelLabel} · {scheduleReasoningLabel(task.reasoningEffort, t)}</span>
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        {lastThreadId ? (
                          <button
                            type="button"
                            onClick={() => onOpenThread?.(lastThreadId)}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                            title={t('scheduleOpenLastThread')}
                            aria-label={t('scheduleOpenLastThread')}
                          >
                            <MessageSquare className="h-4 w-4" strokeWidth={1.8} />
                          </button>
                        ) : null}
                        <button
                          type="button"
                          onClick={() => void runTask(task.id)}
                          disabled={running}
                          className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-45"
                          title={t('scheduleRunNow')}
                          aria-label={t('scheduleRunNow')}
                        >
                          <Play className="h-4 w-4" strokeWidth={1.8} />
                        </button>
                        <button
                          type="button"
                          onClick={() => openEditDialog(task)}
                          className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                          title={t('scheduleEditTask')}
                          aria-label={t('scheduleEditTask')}
                        >
                          <PencilLine className="h-4 w-4" strokeWidth={1.8} />
                        </button>
                        <button
                          type="button"
                          onClick={() => void deleteTask(task.id)}
                          className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-ds-muted transition hover:bg-red-500/10 hover:text-red-600"
                          title={t('scheduleDeleteTask')}
                          aria-label={t('scheduleDeleteTask')}
                        >
                          <Trash2 className="h-4 w-4" strokeWidth={1.8} />
                        </button>
                        <label className="ml-1 inline-flex items-center">
                          <input
                            type="checkbox"
                            checked={task.enabled}
                            onChange={(event) => void updateTask(task.id, { enabled: event.target.checked })}
                            className="sr-only"
                          />
                          <span className={`relative h-5 w-9 rounded-full transition ${task.enabled ? 'bg-ds-ink' : 'bg-ds-border-strong'}`}>
                            <span className={`absolute top-0.5 h-4 w-4 rounded-full shadow transition ${task.enabled ? 'left-[18px] bg-[var(--ds-composer-primary-action-fg)]' : 'left-0.5 bg-ds-card'}`} />
                          </span>
                        </label>
                      </div>
                    </div>
                    {task.lastStatus !== 'idle' || task.lastRunAt ? (
                      <div className="mt-3 rounded-lg border border-ds-border-muted bg-ds-main/45 px-3 py-2.5">
                        <div className="mb-1.5 flex min-w-0 items-center justify-between gap-2">
                          <span className="min-w-0 truncate text-[12px] font-semibold text-ds-faint">
                            {task.lastStatus === 'error'
                              ? t('scheduleLastError')
                              : task.lastStatus === 'running'
                                ? t('scheduleCurrentStatus')
                                : t('scheduleLastResult')}
                          </span>
                        </div>
                        <div className="text-[12px] leading-5 text-ds-muted">
                          {t(scheduledTaskMessageTranslationKey(task))}
                        </div>
                      </div>
                    ) : null}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </main>

      {dialog ? (
        <ScheduleTaskDialog
          dialog={dialog}
          saving={dialogSaving}
          error={dialogError}
          onClose={closeDialog}
          onDraftChange={(draft) => setDialog((current) => current ? { ...current, draft } : current)}
          onPickWorkspace={() => void pickDialogWorkspace()}
          onSubmit={() => void saveDialog()}
          onOpenSettings={() => setSettingsDialogOpen(true)}
          clawChannels={clawChannels}
          defaultClawWorkspaceRoot={settings?.claw.im.workspaceRoot.trim() || ''}
          modelProviders={modelProviders}
          t={t}
        />
      ) : null}

      {settingsDialogOpen && schedule ? (
        <ScheduleDefaultsDialog
          schedule={schedule}
          modelProviders={modelProviders}
          onClose={() => setSettingsDialogOpen(false)}
          onSave={async (patch) => {
            await persistSchedule(patch)
            setSettingsDialogOpen(false)
          }}
          t={t}
        />
      ) : null}
    </div>
  )
}

function ScheduleTaskDialog({
  dialog,
  saving,
  error,
  onClose,
  onDraftChange,
  onPickWorkspace,
  onSubmit,
  onOpenSettings,
  clawChannels,
  defaultClawWorkspaceRoot,
  modelProviders,
  t
}: {
  dialog: TaskDialogState
  saving: boolean
  error: string | null
  onClose: () => void
  onDraftChange: (draft: ScheduledTaskV1) => void
  onPickWorkspace: () => void
  onSubmit: () => void
  onOpenSettings: () => void
  clawChannels: ClawImChannelV1[]
  defaultClawWorkspaceRoot: string
  modelProviders: ScheduleModelProviderOption[]
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  const dialogRef = useModalFocus(onClose)
  const draft = dialog.draft
  const updateDraft = (patch: Partial<ScheduledTaskV1>): void => {
    onDraftChange({ ...draft, ...patch })
  }
  const updateSchedule = (patch: Partial<ScheduledTaskV1['schedule']>): void => {
    onDraftChange({ ...draft, schedule: editScheduleTiming(draft.schedule, patch) })
  }
  const imChannels = configuredScheduleImChannels(clawChannels)
  const preferredImChannel = preferredScheduleImChannel(clawChannels)
  const selectedImChannel = imChannels.find((item) => item.id === draft.clawChannelId) ?? null
  const selectedImDisplayChannel = selectedImChannel ?? preferredImChannel
  const clientMode: ScheduleClientMode = selectedImChannel ? 'im' : 'code'
  const modelSelection = resolveScheduleModelSelection(modelProviders, draft.providerId, draft.model)
  const selectedModelProvider =
    modelProviders.find((provider) => provider.providerId === modelSelection.providerId) ?? null
  const selectedModelIds = selectedModelProvider?.modelIds ?? [modelSelection.model].filter(Boolean)
  const selectedModelProfile = scheduleModelProfileForSelection(selectedModelProvider, modelSelection.model)
  const reasoningOptions = scheduleReasoningOptionsForModel(selectedModelProfile)
  const reasoningSelection = resolveScheduleReasoningSelection(draft.reasoningEffort, selectedModelProfile)
  const updateModelProvider = (providerId: string): void => {
    const selection = resolveScheduleModelSelection(modelProviders, providerId, '')
    const provider = modelProviders.find((item) => item.providerId === selection.providerId) ?? null
    const profile = scheduleModelProfileForSelection(provider, selection.model)
    updateDraft({
      providerId: selection.providerId,
      model: selection.model,
      reasoningEffort: resolveScheduleReasoningSelection(draft.reasoningEffort, profile)
    })
  }
  const updateModel = (model: string): void => {
    const selection = resolveScheduleModelSelection(modelProviders, modelSelection.providerId, model)
    const provider = modelProviders.find((item) => item.providerId === selection.providerId) ?? null
    const profile = scheduleModelProfileForSelection(provider, selection.model)
    updateDraft({
      providerId: selection.providerId,
      model: selection.model,
      reasoningEffort: resolveScheduleReasoningSelection(draft.reasoningEffort, profile)
    })
  }
  const updateClawChannel = (channelId: string): void => {
    const channel = imChannels.find((item) => item.id === channelId)
    const selection = resolveScheduleModelSelection(
      modelProviders,
      draft.providerId,
      channel?.model.trim() || draft.model
    )
    const provider = modelProviders.find((item) => item.providerId === selection.providerId) ?? null
    const profile = scheduleModelProfileForSelection(provider, selection.model)
    updateDraft({
      clawChannelId: channel?.id ?? '',
      providerId: selection.providerId,
      model: selection.model,
      reasoningEffort: resolveScheduleReasoningSelection(draft.reasoningEffort, profile),
      ...(channel
        ? {
            workspaceRoot: channel.workspaceRoot.trim() || defaultClawWorkspaceRoot || draft.workspaceRoot
          }
        : {})
    })
  }
  const updateClientMode = (mode: ScheduleClientMode): void => {
    if (mode === 'code') {
      updateDraft({ clawChannelId: '' })
      return
    }
    if (selectedImChannel) return
    if (preferredImChannel) updateClawChannel(preferredImChannel.id)
  }
  const promptCount = draft.prompt.length
  const title = dialog.mode === 'create' ? t('scheduleCreateTask') : t('scheduleEditTask')

  return (
    <div
      className="ds-no-drag fixed inset-0 z-[90] flex items-center justify-center bg-black/58 px-4 py-2"
      onMouseDown={onClose}
    >
      <form
        ref={dialogRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby="schedule-task-dialog-title"
        onSubmit={(event) => {
          event.preventDefault()
          onSubmit()
        }}
        onMouseDown={(event) => event.stopPropagation()}
        className="flex max-h-[calc(100vh-1rem)] w-full max-w-[760px] flex-col overflow-hidden rounded-[22px] border border-white/55 bg-ds-card shadow-[var(--ax-shadow-modal)] dark:border-white/10"
      >
        <div className="flex shrink-0 items-center justify-between gap-4 border-b border-ds-border-muted px-6 py-3">
          <div className="min-w-0">
            <h2 id="schedule-task-dialog-title" className="truncate text-[17px] font-semibold text-ds-ink">
              {title}
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
            aria-label={t('close')}
            title={t('close')}
          >
            <X className="h-4 w-4" strokeWidth={1.7} />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
          <div className="grid gap-4">
            <ScheduleDialogSection
              icon={<Timer className="h-4 w-4" strokeWidth={1.8} />}
              title={t('scheduleTaskSectionContent')}
            >
              <label className="grid gap-2">
                <FieldLabel required>{t('scheduleTaskName')}</FieldLabel>
                <div className="relative">
                  <input
                    data-modal-autofocus
                    value={draft.title}
                    maxLength={50}
                    onChange={(event) => updateDraft({ title: event.target.value })}
                    placeholder={t('scheduleTaskNamePlaceholder')}
                    className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 pr-14 text-[14px] text-ds-ink outline-none transition placeholder:text-ds-faint focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                  />
                  <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[12px] text-ds-faint">
                    {draft.title.length}/50
                  </span>
                </div>
              </label>

              <label className="grid gap-2">
                <FieldLabel required>{t('scheduleTaskPrompt')}</FieldLabel>
                <div className="relative">
                  <textarea
                    value={draft.prompt}
                    maxLength={8_000}
                    onChange={(event) => updateDraft({ prompt: event.target.value })}
                    placeholder={t('scheduleTaskPromptPlaceholder')}
                    className="min-h-[108px] w-full resize-y rounded-xl border border-ds-border bg-ds-main/55 px-3 py-3 pb-8 text-[14px] leading-6 text-ds-ink outline-none transition placeholder:text-ds-faint focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                  />
                  <span className="pointer-events-none absolute bottom-3 right-3 text-[12px] text-ds-faint">
                    {promptCount}/8000
                  </span>
                </div>
              </label>
            </ScheduleDialogSection>

            <ScheduleDialogSection
              icon={<Brain className="h-4 w-4" strokeWidth={1.8} />}
              title={t('scheduleTaskSectionModel')}
            >
              <div className="grid gap-4">
                <div className="grid gap-2">
                  <FieldLabel>{t('scheduleClientMode')}</FieldLabel>
                  <div className="grid grid-cols-2 gap-2">
                    <SegmentButton
                      selected={clientMode === 'code'}
                      onClick={() => updateClientMode('code')}
                    >
                      {t('scheduleClientModeCode')}
                    </SegmentButton>
                    <SegmentButton
                      selected={clientMode === 'im'}
                      disabled={imChannels.length === 0}
                      onClick={() => updateClientMode('im')}
                    >
                      {t('scheduleClientModeIm')}
                    </SegmentButton>
                  </div>
                  {imChannels.length === 0 ? (
                    <p className="text-[12px] leading-5 text-ds-faint">{t('scheduleClientModeImUnavailable')}</p>
                  ) : null}
                </div>

                {clientMode === 'im' ? (
                  <label className="grid gap-2">
                    <FieldLabel>{t('scheduleImClient')}</FieldLabel>
                    <div className="relative">
                      <select
                        value={selectedImDisplayChannel?.id ?? ''}
                        onChange={(event) => updateClawChannel(event.target.value)}
                        aria-label={t('scheduleImClient')}
                        title={selectedImDisplayChannel ? scheduleImChannelOptionLabel(selectedImDisplayChannel, t) : undefined}
                        className="peer absolute inset-0 z-10 h-10 w-full cursor-pointer opacity-0"
                      >
                        {imChannels.map((channel) => (
                          <option key={channel.id} value={channel.id}>
                            {scheduleImChannelOptionLabel(channel, t)}
                          </option>
                        ))}
                      </select>
                      <div className="pointer-events-none flex h-10 w-full items-center gap-3 rounded-xl border border-ds-border bg-ds-main/55 px-3 pr-10 text-[14px] text-ds-ink outline-none transition peer-focus:border-accent/45 peer-focus:ring-2 peer-focus:ring-accent/15">
                        <span className="min-w-0 flex-1 truncate">
                          {selectedImDisplayChannel ? clawChannelDisplayName(selectedImDisplayChannel) : ''}
                        </span>
                        {selectedImDisplayChannel ? (
                          <span className="shrink-0 rounded-lg border border-ds-border-muted bg-ds-subtle px-2 py-0.5 text-[12px] font-semibold text-ds-muted">
                            {scheduleImProviderLabel(selectedImDisplayChannel, t)}
                          </span>
                        ) : null}
                      </div>
                      <ChevronDown
                        className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ds-faint"
                        strokeWidth={1.8}
                        aria-hidden="true"
                      />
                    </div>
                  </label>
                ) : null}

                <div className="grid gap-4 md:grid-cols-[minmax(0,0.9fr)_minmax(0,1fr)_minmax(0,2fr)]">
                  <label className="grid gap-2">
                    <FieldLabel required>{t('scheduleProvider')}</FieldLabel>
                    <select
                      value={modelSelection.providerId}
                      onChange={(event) => updateModelProvider(event.target.value)}
                      className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink outline-none transition focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                    >
                      {modelProviders.map((provider) => (
                        <option key={provider.providerId} value={provider.providerId}>
                          {provider.label}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label className="grid gap-2">
                    <FieldLabel required>{t('scheduleModel')}</FieldLabel>
                    <select
                      value={modelSelection.model}
                      onChange={(event) => updateModel(event.target.value)}
                      className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink outline-none transition focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                    >
                      {selectedModelIds.map((model) => (
                        <option key={model} value={model}>{model}</option>
                      ))}
                    </select>
                  </label>

                  <div className="grid gap-2">
                    <FieldLabel>{t('scheduleReasoning')}</FieldLabel>
                    <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                      {reasoningOptions.map((effort) => (
                        <SegmentButton
                          key={effort}
                          selected={reasoningSelection === effort}
                          onClick={() => updateDraft({ reasoningEffort: effort })}
                        >
                          {scheduleReasoningLabel(effort, t)}
                        </SegmentButton>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </ScheduleDialogSection>

            <ScheduleDialogSection
              icon={<CalendarClock className="h-4 w-4" strokeWidth={1.8} />}
              title={t('scheduleTaskSectionTiming')}
            >
              <div className="grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
                <div className="grid gap-2">
                  <FieldLabel required>{t('scheduleRunAt')}</FieldLabel>
                  <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                    {SCHEDULE_KIND_OPTIONS.map((kind) => (
                      <SegmentButton
                        key={kind}
                        selected={draft.schedule.kind === kind}
                        onClick={() => updateSchedule({ kind })}
                      >
                        {t(`scheduleKind_${kind}`)}
                      </SegmentButton>
                    ))}
                  </div>
                </div>

                {draft.schedule.kind === 'daily' || draft.schedule.kind === 'weekly' ? (
                  <div className="grid gap-2">
                    <FieldLabel>{t('scheduleDailyTime')}</FieldLabel>
                    <ScheduleTimePicker
                      value={draft.schedule.timeOfDay}
                      onChange={(timeOfDay) => updateSchedule({ timeOfDay })}
                      t={t}
                    />
                  </div>
                ) : draft.schedule.kind === 'cron' ? (
                  <label className="grid gap-2"><FieldLabel>{t('scheduleCronExpression')}</FieldLabel>
                    <input value={draft.schedule.expression ?? ''} onChange={event => updateSchedule({ expression: event.target.value })} aria-label={t('scheduleCronExpression')} className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink" />
                  </label>
                ) : draft.schedule.kind === 'at' ? (
                  <label className="grid gap-2">
                    <FieldLabel>{t('scheduleAtTime')}</FieldLabel>
                    <input
                      type="datetime-local"
                      value={dateTimeLocalValueFromIso(draft.schedule.atTime)}
                      onChange={(event) => updateSchedule({ atTime: isoFromDateTimeLocalValue(event.target.value) })}
                      className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink outline-none transition focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                    />
                  </label>
                ) : draft.schedule.kind === 'interval' ? (
                  <label className="grid gap-2">
                    <FieldLabel>{t('scheduleEveryMinutes')}</FieldLabel>
                    <input
                      type="number"
                      min={1}
                      max={10080}
                      value={draft.schedule.everyMinutes}
                      onChange={(event) => updateSchedule({ everyMinutes: Number(event.target.value) })}
                      className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink outline-none transition focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                    />
                  </label>
                ) : (
                  <div className="flex min-h-10 items-center rounded-xl bg-ds-subtle px-3 text-[13px] text-ds-muted">
                    {t('scheduleManualHint')}
                  </div>
                )}
              </div>
              {draft.schedule.kind === 'weekly' ? <fieldset className="mt-3 flex flex-wrap gap-3"><legend>{t('scheduleWeekdays')}</legend>
                {[1, 2, 3, 4, 5, 6, 7].map(day => <label key={day}><input type="checkbox" checked={draft.schedule.weekdays?.includes(day) ?? false} onChange={event => updateSchedule({ weekdays: event.target.checked ? [...(draft.schedule.weekdays ?? []), day].sort((a, b) => a - b) : draft.schedule.weekdays?.filter(value => value !== day) ?? [] })} /> {t(`scheduleWeekday_${day}`)}</label>)}
              </fieldset> : null}
              {['daily', 'weekly', 'cron'].includes(draft.schedule.kind) ? <label className="mt-3 grid gap-2"><FieldLabel>{t('scheduleTimeZone')}</FieldLabel>
                <input value={draft.schedule.timeZone ?? ''} placeholder={t('scheduleLegacyLocal')} onChange={event => updateSchedule({ timeZone: event.target.value })} aria-label={t('scheduleTimeZone')} className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink" />
              </label> : null}
            </ScheduleDialogSection>

            <ScheduleDialogSection
              icon={<Folder className="h-4 w-4" strokeWidth={1.8} />}
              title={t('scheduleTaskSectionEnvironment')}
            >
              <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_180px]">
                <label className="grid gap-2">
                  <FieldLabel>{t('scheduleWorkspace')}</FieldLabel>
                  <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_138px]">
                    <input
                      value={draft.workspaceRoot}
                      onChange={(event) => updateDraft({ workspaceRoot: event.target.value })}
                      placeholder={t('scheduleWorkspacePlaceholder')}
                      className="h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink outline-none transition placeholder:text-ds-faint focus:border-accent/45 focus:ring-2 focus:ring-accent/15"
                    />
                    <button
                      type="button"
                      onClick={onPickWorkspace}
                      className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-ds-border bg-ds-card px-3 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                    >
                      <FolderOpen className="h-4 w-4" strokeWidth={1.75} />
                      {draft.workspaceRoot.trim() ? t('changeWorkspace') : t('selectWorkspace')}
                    </button>
                  </div>
                </label>

                <div className="grid gap-2">
                  <FieldLabel>{t('scheduleTaskEnabled')}</FieldLabel>
                  <button
                    type="button"
                    onClick={() => updateDraft({ enabled: !draft.enabled })}
                    className="flex h-10 items-center justify-between gap-3 rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                    aria-pressed={draft.enabled}
                  >
                    <span className="inline-flex min-w-0 items-center gap-2">
                      <Power className="h-4 w-4 shrink-0" strokeWidth={1.8} />
                      <span className="truncate">{t('scheduleTaskEnabled')}</span>
                    </span>
                    <span className={`relative h-5 w-9 shrink-0 rounded-full transition ${draft.enabled ? 'bg-ds-ink' : 'bg-ds-border-strong'}`}>
                      <span className={`absolute top-0.5 h-4 w-4 rounded-full shadow transition ${draft.enabled ? 'left-[18px] bg-[var(--ds-composer-primary-action-fg)]' : 'left-0.5 bg-ds-card'}`} />
                    </span>
                  </button>
                </div>
              </div>
            </ScheduleDialogSection>
          </div>

          {error ? (
            <div className="mt-5 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-[13px] text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">
              {error}
            </div>
          ) : null}
        </div>

        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-ds-border-muted bg-ds-card px-6 py-3">
          <button
            type="button"
            onClick={onOpenSettings}
            className="inline-flex h-8 items-center gap-2 rounded-xl px-3 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
          >
            <MoreHorizontal className="h-4 w-4" strokeWidth={1.8} />
            {t('scheduleAdvancedSettings')}
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="h-8 rounded-xl border border-ds-border bg-ds-card px-4 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
            >
              {t('cancel')}
            </button>
            <button
              type="submit"
              disabled={saving}
              className="h-8 rounded-xl bg-ds-userbubble px-5 text-[13px] font-semibold text-ds-userbubbleFg transition hover:opacity-90"
            >
              {t('confirm')}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function ScheduleDialogSection({
  icon,
  title,
  children
}: {
  icon: ReactElement
  title: string
  children: ReactNode
}): ReactElement {
  return (
    <section className="grid gap-3 border-t border-ds-border-muted pt-4 first:border-t-0 first:pt-0">
      <div className="flex items-center gap-2 text-[13px] font-semibold text-ds-ink">
        <span className="inline-flex h-7 w-7 items-center justify-center rounded-lg bg-ds-subtle text-ds-muted">
          {icon}
        </span>
        <span>{title}</span>
      </div>
      {children}
    </section>
  )
}

function FieldLabel({
  children,
  required = false
}: {
  children: ReactNode
  required?: boolean
}): ReactElement {
  return (
    <span className="flex min-h-5 items-center gap-1 text-[13px] font-medium text-ds-ink">
      <span className="min-w-0 truncate">{children}</span>
      {required ? <span className="text-red-500">*</span> : null}
    </span>
  )
}

function SegmentButton({
  selected,
  disabled = false,
  onClick,
  children
}: {
  selected: boolean
  disabled?: boolean
  onClick: () => void
  children: ReactNode
}): ReactElement {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`h-9 min-w-0 rounded-xl border px-2.5 text-[12.5px] font-semibold transition ${
        selected
          ? 'border-accent/45 bg-accent/10 text-ds-ink shadow-sm'
          : disabled
            ? 'cursor-not-allowed border-ds-border bg-ds-subtle text-ds-faint opacity-60'
            : 'border-ds-border bg-ds-main/55 text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
      }`}
    >
      <span className="block truncate">{children}</span>
    </button>
  )
}

function ScheduleTimePicker({
  value,
  onChange,
  t
}: {
  value: string
  onChange: (value: string) => void
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  const [hour, minute] = splitTimeOfDay(value)
  const selectClass = 'h-10 w-full rounded-xl border border-ds-border bg-ds-main/55 px-3 text-[14px] text-ds-ink outline-none transition focus:border-accent/45 focus:ring-2 focus:ring-accent/15'

  return (
    <div className="grid grid-cols-2 gap-2">
      <select
        value={hour}
        onChange={(event) => onChange(`${event.target.value}:${minute}`)}
        className={selectClass}
        aria-label={t('scheduleTimeHour')}
      >
        {TIME_HOURS.map((item) => (
          <option key={item} value={item}>{item}</option>
        ))}
      </select>
      <select
        value={minute}
        onChange={(event) => onChange(`${hour}:${event.target.value}`)}
        className={selectClass}
        aria-label={t('scheduleTimeMinute')}
      >
        {TIME_MINUTES.map((item) => (
          <option key={item} value={item}>{item}</option>
        ))}
      </select>
    </div>
  )
}

function splitTimeOfDay(value: string): [string, string] {
  const match = /^(?<hour>[01]\d|2[0-3]):(?<minute>[0-5]\d)$/u.exec(value)
  return [match?.groups?.hour ?? '09', match?.groups?.minute ?? '00']
}
