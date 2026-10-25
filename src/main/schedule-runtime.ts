import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import { randomUUID } from 'node:crypto'
import { URL } from 'node:url'
import { mkdir } from 'node:fs/promises'
import type {
  AppSettingsV1,
  ClawImChannelV1,
  ScheduleReasoningEffort,
  ScheduleRunMode,
  ScheduleRunResult,
  ScheduleRuntimeStatus,
  ScheduleTaskFromTextResult,
  ScheduledTaskV1
} from '../shared/app-settings'
import {
  scheduleTaskEditMatches,
  type ScheduleTaskCreateInput,
  type ScheduleTaskEditPatch,
  type ScheduleTaskUpdateRequest,
  type ScheduleTaskDeleteRequest,
  type ScheduleTaskWriteResult,
  type ScheduleTaskDeleteResult
} from '../shared/schedule-task-mutation'
import { validateScheduleCalendarRule } from '../../packages/runtime/src/contracts/schedule-rule'
import { scheduleTaskCreatePayloadSchema, scheduleTaskEditPatchSchema } from './ipc/app-ipc-schemas'
import {
  DEFAULT_SCHEDULE_MODEL,
  DEFAULT_SCHEDULE_REASONING_EFFORT,
  buildClawRuntimePrompt,
  buildScheduleRuntimePrompt,
  normalizeScheduleReasoningEffort
} from '../shared/app-settings'
import {
  buildClawScheduledTaskDetectionPrompt,
  buildScheduledTaskFromDetectedRequest,
  detectClawScheduledTaskRequest,
  looksLikeClawScheduledTaskCandidate
} from './claw-scheduled-task-detector'
import {
  SCHEDULER_INTERVAL_MS,
  TASK_RESPONSE_TIMEOUT_MS,
  asString,
  computeScheduleNextRunAt,
  normalizeScheduleTaskOrigin,
  normalizeScheduleDeliveryRecord,
  canonicalScheduleUTC,
  requestScheduleAdmission,
  scheduleAdmissionRequestSchemaV1,
  type ScheduleAdmissionRequestV1,
  type ScheduleAdmissionResponseV1,
  scheduleTaskReadView,
  scheduleTaskReceiptView,
  scheduleNativeOriginSchemaV1,
  type ScheduleTaskOriginV1,
  type StoredScheduledTaskV1,
  hasEnabledScheduledTask,
  internalUrl,
  isRunningStatus,
  latestAssistantText,
  nestedRecord,
  normalizeTaskModel,
  parseJsonObject,
  readRequestBody,
  runtimeErrorMessage,
  sleep,
  writeJson,
  type RunPromptOptions,
  type ScheduleRuntimeDeps,
  type ThreadDetailJson,
  type ThreadRecordJson
} from './schedule-runtime-helpers'

export { computeScheduleNextRunAt } from './schedule-runtime-helpers'

export function scheduledThreadTitle(title: string): string {
  const trimmed = title.trim()
  const prefix = '[Scheduled task]'
  const suffix = Array.from(trimmed).slice(0, 4).join('')
  return suffix ? `${prefix} ${suffix}` : prefix
}

const SCHEDULE_APPROVAL_POLICY = 'never'
const SCHEDULE_SANDBOX_MODE = 'workspace-write'

const SCHEDULE_REASONING_EFFORT_SET = new Set<ScheduleReasoningEffort>([
  'auto',
  'off',
  'low',
  'medium',
  'high',
  'max'
])

type ScheduleModelConfig = {
  providerId: string
  model: string
  reasoningEffort: ScheduleReasoningEffort
}

function isScheduleReasoningEffort(value: string): value is ScheduleReasoningEffort {
  return SCHEDULE_REASONING_EFFORT_SET.has(value as ScheduleReasoningEffort)
}

function resolveReasoningForModel(
  reasoningEffort: ScheduleReasoningEffort | string | null | undefined
): ScheduleReasoningEffort {
  const requested = normalizeScheduleReasoningEffort(reasoningEffort)
  return isScheduleReasoningEffort(requested) ? requested : DEFAULT_SCHEDULE_REASONING_EFFORT
}

export class ScheduleRuntime {
  private readonly deps: ScheduleRuntimeDeps
  private scheduler: ReturnType<typeof setInterval> | null = null
  private server: Server | null = null
  private serverKey = ''
  private runningTaskIds = new Set<string>()
  private powerSaveBlockerId: number | null = null

  constructor(deps: ScheduleRuntimeDeps) {
    this.deps = deps
  }

  private resolveScheduleModelConfig(
    settings: AppSettingsV1,
    input: {
      providerId?: string | null
      model?: string | null
      reasoningEffort?: ScheduleReasoningEffort | string | null
    }
  ): ScheduleModelConfig {
    const requestedProviderId = input.providerId?.trim() || ''
    const scheduleProviderId = settings.schedule.providerId?.trim() || ''
    const model = normalizeTaskModel(input.model ?? '') ?? DEFAULT_SCHEDULE_MODEL
    return {
      providerId: requestedProviderId || scheduleProviderId,
      model,
      reasoningEffort: resolveReasoningForModel(input.reasoningEffort)
    }
  }

  sync(settings: AppSettingsV1): void {
    this.syncInternalServer(settings)
    this.startScheduler()
    this.syncPowerSaveBlocker(settings)
    void this.ensureNextRuns()
  }

  stop(): void {
    if (this.scheduler) {
      clearInterval(this.scheduler)
      this.scheduler = null
    }
    this.closeInternalServer()
    this.stopPowerSaveBlocker()
  }

  async status(): Promise<ScheduleRuntimeStatus> {
    const settings = await this.deps.store.load()
    return {
      internalServerRunning: this.server !== null,
      internalUrl: internalUrl(settings),
      runningTaskIds: [...this.runningTaskIds],
      powerSaveBlockerActive: this.isPowerSaveBlockerActive()
    }
  }

  async runTask(taskId: string): Promise<ScheduleRunResult> {
    const settings = await this.deps.store.load()
    const task = settings.schedule.tasks.find((item) => item.id === taskId)
    if (!task) return { ok: false, message: 'Task not found.' }
    return this.runTaskInternal(task, false)
  }

  async createScheduledTaskFromText(
    text: string,
    options: {
      workspaceRoot?: string | null
      clawChannelId?: string | null
      providerId?: string | null
      modelHint?: string | null
      reasoningEffort?: ScheduleReasoningEffort | null
      mode?: ScheduleRunMode | null
    } = {}
  ): Promise<ScheduleTaskFromTextResult> {
    const settings = await this.deps.store.load()
    try {
      const clawChannel = this.resolveClawChannel(settings, options.clawChannelId)
      const modelConfig = this.resolveScheduleModelConfig(settings, {
        providerId: options.providerId ?? settings.schedule.providerId,
        model: options.modelHint?.trim() || clawChannel?.model.trim() || settings.schedule.model || DEFAULT_SCHEDULE_MODEL,
        reasoningEffort: options.reasoningEffort ?? DEFAULT_SCHEDULE_REASONING_EFFORT
      })
      if (!looksLikeClawScheduledTaskCandidate(text)) return { kind: 'noop' }
      const workspaceRoot =
        options.workspaceRoot?.trim() ||
        (clawChannel ? this.resolveClawChannelWorkspaceRoot(settings, clawChannel) : this.resolveDefaultWorkspaceRoot(settings))
      const detection = await this.runPrompt(settings, {
        prompt: buildClawScheduledTaskDetectionPrompt(text, new Date()),
        title: '[Scheduled task] Detect',
        workspaceRoot,
        model: modelConfig.model,
        reasoningEffort: modelConfig.reasoningEffort,
        mode: 'plan',
        waitForResult: true,
        responseTimeoutMs: TASK_RESPONSE_TIMEOUT_MS,
        maxModelSteps: 1,
        deleteThreadAfterResult: true
      })
      const request = detection.ok
        ? await detectClawScheduledTaskRequest(text, detection.text ?? '')
        : null
      if (!request) return { kind: 'noop' }
      const task: ScheduledTaskV1 = buildScheduledTaskFromDetectedRequest({
        request,
        workspaceRoot,
        providerId: modelConfig.providerId,
        model: modelConfig.model,
        reasoningEffort: modelConfig.reasoningEffort,
        mode: options.mode ?? settings.schedule.mode,
        id: randomUUID()
      })
      task.clawChannelId = clawChannel?.id ?? ''
      if (task.schedule.kind === 'daily') {
        task.schedule = { ...task.schedule, timeZone: new Intl.DateTimeFormat().resolvedOptions().timeZone }
        task.nextRunAt = computeScheduleNextRunAt(task, new Date())
      }
      await this.createTask(task)
      return {
        kind: 'created',
        taskId: task.id,
        title: task.title,
        scheduleAt: request.scheduleAt,
        confirmationText: request.confirmationText
      }
    } catch (error) {
      void error
      this.deps.logError('schedule-task', 'Failed to create scheduled task from text', {
        operation: 'detect-and-create'
      })
      return { kind: 'error', message: 'Failed to create scheduled task.' }
    }
  }

  private async mutateTasks<T>(mutator: (latest: AppSettingsV1) => { tasks: ScheduledTaskV1[]; enabled?: boolean; value: T }): Promise<{ settings: AppSettingsV1; value: T; changed: boolean }> {
    const result = await this.deps.store.mutateScheduleTasks(mutator)
    if (result.changed) this.deps.onCommittedChange?.()
    return result
  }

  async listTasks(): Promise<ScheduledTaskV1[]> {
    const settings = await this.deps.store.loadFresh()
    return settings.schedule.tasks.map(scheduleTaskReadView)
  }

  async readTaskHistory(request: import('../shared/schedule-task-mutation').ScheduleTaskHistoryRequest): Promise<import('../shared/schedule-task-mutation').ScheduleTaskHistoryResult> {
    const settings = await this.deps.store.load()
    const task = settings.schedule.tasks.find(item => item.id === request.taskId) as StoredScheduledTaskV1 | undefined
    if (!task) return { kind: 'not_found', message: 'Task unavailable.' }
    const records = task.hostDelivery?.records ?? []
    const end = request.before === undefined ? records.length : records.findIndex(item => item.attemptId === request.before)
    if (end < 0) return { kind: 'conflict', message: 'History changed. Refresh the records.' }
    const start = Math.max(0, end - 20), page = records.slice(start, end).reverse()
    return { kind: 'success', records: page, ...(start > 0 ? { nextBefore: page[page.length - 1].attemptId } : {}),
      deliveryKind: task.hostDelivery ? 'origin-delivery' : 'legacy-headless' }
  }

  async createTask(task: ScheduledTaskV1): Promise<ScheduledTaskV1> {
    const result = await this.mutateTasks((latest) => ({
      enabled: true,
      tasks: [...latest.schedule.tasks, task],
      value: task
    }))
    this.sync(result.settings)
    return result.settings.schedule.tasks.find((item) => item.id === task.id)!
  }

  // The second parameter is Main-internal and must come from authenticated native
  // outer metadata. It is never read from renderer or MCP business arguments.
  async createTaskFromInput(input: ScheduleTaskCreateInput, origin?: ScheduleTaskOriginV1): Promise<ScheduledTaskV1> {
    validateScheduleCalendarRule(input.schedule)
    const settings = await this.deps.store.load()
    const clawChannel = this.resolveClawChannel(settings, input.clawChannelId)
    const modelConfig = this.resolveScheduleModelConfig(settings, {
      providerId: input.providerId ?? settings.schedule.providerId,
      model: input.model?.trim() || clawChannel?.model.trim() || settings.schedule.model || DEFAULT_SCHEDULE_MODEL,
      reasoningEffort: input.reasoningEffort ?? DEFAULT_SCHEDULE_REASONING_EFFORT
    })
    const now = new Date().toISOString()
    const task: ScheduledTaskV1 = {
      id: randomUUID(),
      title: input.title.trim() || 'New scheduled task',
      enabled: input.enabled !== false,
      prompt: input.prompt,
      workspaceRoot:
        input.workspaceRoot?.trim() ||
        (clawChannel ? this.resolveClawChannelWorkspaceRoot(settings, clawChannel) : this.resolveDefaultWorkspaceRoot(settings)),
      clawChannelId: clawChannel?.id ?? '',
      providerId: modelConfig.providerId,
      model: modelConfig.model,
      reasoningEffort: modelConfig.reasoningEffort,
      mode: input.mode ?? settings.schedule.mode,
      schedule: {
        kind: input.schedule.kind,
        everyMinutes: typeof input.schedule.everyMinutes === 'number' ? input.schedule.everyMinutes : 60,
        timeOfDay: input.schedule.timeOfDay?.trim() || '09:00',
        atTime: input.schedule.atTime?.trim() || '',
        ...(Object.prototype.hasOwnProperty.call(input.schedule, 'timeZone') ? { timeZone: input.schedule.timeZone } : {}),
        ...(input.schedule.weekdays ? { weekdays: [...input.schedule.weekdays].sort((a, b) => a - b) } : {}),
        ...(input.schedule.expression ? { expression: input.schedule.expression } : {})
      },
      createdAt: now,
      updatedAt: now,
      lastRunAt: '',
      nextRunAt: '',
      lastStatus: 'idle',
      lastMessage: 'schedule_task_idle',
      lastThreadId: ''
    }
    if (origin !== undefined) (task as StoredScheduledTaskV1).hostDelivery = { schemaVersion: 1, origin: normalizeScheduleTaskOrigin(origin), records: [] }
    task.nextRunAt = computeScheduleNextRunAt(task, new Date(now))
    const saved = await this.createTask(task)
    await this.ensureNextRuns()
    return scheduleTaskReceiptView(saved)
  }

  async updateTaskConfiguration(request: ScheduleTaskUpdateRequest): Promise<ScheduleTaskWriteResult> {
    const patch = scheduleTaskEditPatchSchema.parse(request.patch)
    const result = await this.mutateTasks<ScheduleTaskWriteResult>((latest) => {
      const task = latest.schedule.tasks.find((item) => item.id === request.taskId)
      if (!task) return { tasks: latest.schedule.tasks, value: { kind: 'not_found', message: 'Task not found.' } }
      if (!scheduleTaskEditMatches(task, request.expected)) {
        return { tasks: latest.schedule.tasks, value: { kind: 'conflict', message: 'Task configuration changed. Refresh and try again.' } }
      }
      if (Object.keys(patch).length === 0) return { tasks: latest.schedule.tasks, value: { kind: 'success', task } }
      const next = this.applyTaskEdit(task, patch)
      return { tasks: latest.schedule.tasks.map((item) => item.id === task.id ? next : item), value: { kind: 'success', task: next } }
    })
    if (result.changed) this.sync(result.settings)
    if (result.value.kind === 'success') {
      return { kind: 'success', task: scheduleTaskReadView(result.settings.schedule.tasks.find((task) => task.id === request.taskId)!) }
    }
    return result.value
  }

  private applyTaskEdit(task: ScheduledTaskV1, patch: ScheduleTaskEditPatch): ScheduledTaskV1 {
    return {
      ...task, ...patch,
      schedule: patch.schedule ? {
        kind: patch.schedule.kind!, everyMinutes: patch.schedule.everyMinutes ?? task.schedule.everyMinutes,
        timeOfDay: patch.schedule.timeOfDay ?? task.schedule.timeOfDay, atTime: patch.schedule.atTime ?? task.schedule.atTime,
        ...(Object.prototype.hasOwnProperty.call(patch.schedule, 'timeZone') ? { timeZone: patch.schedule.timeZone } : {}),
        ...(patch.schedule.weekdays ? { weekdays: [...patch.schedule.weekdays].sort((a, b) => a - b) } : {}),
        ...(patch.schedule.expression ? { expression: patch.schedule.expression } : {})
      } : task.schedule,
      ...(Object.prototype.hasOwnProperty.call(patch, 'enabled') || patch.schedule ? { nextRunAt: '' } : {}),
      updatedAt: new Date().toISOString()
    }
  }

  async updateTaskById(taskId: string, input: ScheduleTaskEditPatch): Promise<ScheduledTaskV1 | null> {
    const patch = scheduleTaskEditPatchSchema.parse(input)
    const result = await this.mutateTasks((latest) => {
      const task = latest.schedule.tasks.find((item) => item.id === taskId)
      if (!task) return { tasks: latest.schedule.tasks, value: null }
      const next = this.applyTaskEdit(task, patch)
      return { tasks: latest.schedule.tasks.map((item) => item.id === taskId ? next : item), value: next }
    })
    if (result.changed) this.sync(result.settings)
    return result.value ? result.settings.schedule.tasks.find((item) => item.id === taskId)! : null
  }

  async deleteTask(request: ScheduleTaskDeleteRequest): Promise<ScheduleTaskDeleteResult> {
    const result = await this.mutateTasks<ScheduleTaskDeleteResult>((latest) => {
      const task = latest.schedule.tasks.find((item) => item.id === request.taskId)
      if (!task) return { tasks: latest.schedule.tasks, value: { kind: 'not_found', message: 'Task not found.' } }
      if (!scheduleTaskEditMatches(task, request.expected)) {
        return { tasks: latest.schedule.tasks, value: { kind: 'conflict', message: 'Task configuration changed. Refresh and try again.' } }
      }
      return { tasks: latest.schedule.tasks.filter((item) => item.id !== request.taskId), value: { kind: 'success', taskId: request.taskId } }
    })
    if (result.changed) this.sync(result.settings)
    return result.value
  }

  async deleteTaskById(taskId: string): Promise<boolean> {
    const result = await this.mutateTasks((latest) => ({
      tasks: latest.schedule.tasks.some((item) => item.id === taskId)
        ? latest.schedule.tasks.filter((item) => item.id !== taskId) : latest.schedule.tasks,
      value: latest.schedule.tasks.some((item) => item.id === taskId)
    }))
    if (result.changed) this.sync(result.settings)
    return result.value
  }

  private startScheduler(): void {
    if (this.scheduler) return
    this.scheduler = setInterval(() => {
      void this.tick()
    }, SCHEDULER_INTERVAL_MS)
    this.scheduler.unref?.()
    void this.tick()
  }

  private async tick(): Promise<void> {
    await this.ensureNextRuns()
    const fresh = await this.deps.store.load()
    const now = Date.now()
    for (const task of fresh.schedule.tasks) {
      if (this.runningTaskIds.has(task.id)) continue
      // An unresolved submission is queried even after disable/manual edits.
      // Query is read-only and must never create a replacement attempt.
      if ((task as StoredScheduledTaskV1).hostDelivery?.pending) {
        void this.runTaskInternal(task, true)
        continue
      }
      if (!fresh.schedule.enabled || !task.enabled || task.schedule.kind === 'manual') continue
      const dueAt = Date.parse(task.nextRunAt)
      if (!Number.isFinite(dueAt) || dueAt > now) continue
      void this.runTaskInternal(task, true)
    }
  }

  private async ensureNextRuns(): Promise<void> {
    const result = await this.mutateTasks((latest) => {
      if (!latest.schedule.enabled) return { tasks: latest.schedule.tasks, value: undefined }
      let changed = false
      const now = new Date()
      const tasks = latest.schedule.tasks.map((task) => {
        if ((task as StoredScheduledTaskV1).hostDelivery?.pending) return task
        const wasInterrupted = task.lastStatus === 'running' && !this.runningTaskIds.has(task.id)
        if (!task.enabled || task.schedule.kind === 'manual' || this.runningTaskIds.has(task.id)) {
          if (!wasInterrupted) return task
          changed = true
          return {
            ...task,
            ...(task.schedule.kind === 'at' ? { enabled: false } : {}),
            nextRunAt: task.schedule.kind === 'at' ? '' : task.nextRunAt,
            lastStatus: 'error' as const,
            lastMessage: 'schedule_task_interrupted' as const,
            updatedAt: now.toISOString()
          }
        }
        if (task.nextRunAt && !wasInterrupted) return task
        changed = true
        return {
          ...task,
          nextRunAt: computeScheduleNextRunAt(task, now),
          ...(wasInterrupted
            ? {
                lastStatus: 'error' as const,
                lastMessage: 'schedule_task_interrupted' as const,
                updatedAt: now.toISOString()
              }
            : {})
        }
      })
      return { tasks: changed ? tasks : latest.schedule.tasks, value: undefined }
    })
    this.syncPowerSaveBlocker(result.settings)
  }

  private async updateTask(
    taskId: string,
    updater: (task: ScheduledTaskV1, settings: AppSettingsV1) => ScheduledTaskV1
  ): Promise<AppSettingsV1> {
    const result = await this.mutateTasks((latest) => ({
      tasks: latest.schedule.tasks.some((task) => task.id === taskId)
        ? latest.schedule.tasks.map((task) => task.id === taskId ? updater(task, latest) : task)
        : latest.schedule.tasks,
      value: undefined
    }))
    this.syncPowerSaveBlocker(result.settings)
    return result.settings
  }

  /** Freeze before dispatch. Unknown outcomes are recovered only by exact query. */
  private async runOriginTask(taskId: string, scheduled: boolean): Promise<ScheduleRunResult> {
    if (this.runningTaskIds.has(taskId)) return { ok: false, message: 'Task is already running.' }
    this.runningTaskIds.add(taskId)
    try {
      const frozen = await this.mutateTasks<{ request: ScheduleAdmissionRequestV1; query: boolean } | null>(latest => {
        const task = latest.schedule.tasks.find(item => item.id === taskId) as StoredScheduledTaskV1 | undefined
        if (!task?.hostDelivery) return { tasks: latest.schedule.tasks, value: null }
        if (task.hostDelivery.pending) return { tasks: latest.schedule.tasks, value: { request: task.hostDelivery.pending, query: true } }
        const now = new Date()
        if (scheduled && (!latest.schedule.enabled || !task.enabled || task.schedule.kind === 'manual' || !task.nextRunAt || Date.parse(task.nextRunAt) > now.getTime())) return { tasks: latest.schedule.tasks, value: null }
        const target = scheduled ? task.nextRunAt : now.toISOString()
        let scheduledAt: string
        try { scheduledAt = canonicalScheduleUTC(target) }
        catch { scheduledAt = canonicalScheduleUTC(new Date(target).toISOString()) }
        const model = normalizeTaskModel(task.model)
        const request = scheduleAdmissionRequestSchemaV1.parse({
          schemaVersion: 1, kind: 'submit', taskId, origin: task.hostDelivery.origin,
          attemptId: randomUUID(), scheduledAt, prompt: task.prompt,
          execution: { ...(task.providerId?.trim() ? { providerId: task.providerId.trim() } : {}),
            ...(model ? { model } : {}), reasoningEffort: task.reasoningEffort, mode: task.mode }
        })
        const next: StoredScheduledTaskV1 = { ...task, nextRunAt: '', lastStatus: 'running',
          lastMessage: 'schedule_task_running', updatedAt: now.toISOString(),
          hostDelivery: { ...task.hostDelivery, pending: request } }
        return { tasks: latest.schedule.tasks.map(item => item.id === taskId ? next : item), value: { request, query: false } }
      })
      if (!frozen.value) return { ok: false, message: 'Task is not available for delivery.' }
      const request = frozen.value.request
      const outcome = await requestScheduleAdmission(this.deps, frozen.settings,
        { ...request, kind: frozen.value.query ? 'query' : 'submit' })
      const committed = await this.settleOriginAttempt(request, outcome)
      if (!committed) return { ok: false, message: 'Task changed before delivery was recorded.' }
      if (outcome.status === 'accepted' && outcome.receipt) return {
        ok: true, threadId: outcome.receipt.threadId, turnId: outcome.receipt.turnId, message: 'Accepted by the original session.'
      }
      const messages = {
        unknown: 'Delivery acceptance is unknown. The saved attempt will be queried.',
        not_found: 'No acceptance receipt is available. The saved attempt will be queried.',
        rejected: 'Original-session delivery was rejected.',
        conflict: 'Original-session delivery identity conflicts with the saved attempt.',
        invalid: 'Original-session delivery request was rejected as invalid.',
        accepted: 'Delivery acceptance is unavailable.'
      }
      return { ok: false, message: messages[outcome.status] }
    } catch {
      // If dispatch might have happened, the durable pending request survives.
      return { ok: false, message: 'Original-session delivery is unavailable.' }
    } finally { this.runningTaskIds.delete(taskId) }
  }

  private async settleOriginAttempt(request: ScheduleAdmissionRequestV1, outcome: ScheduleAdmissionResponseV1): Promise<boolean> {
    const result = await this.mutateTasks(latest => {
      const task = latest.schedule.tasks.find(item => item.id === request.taskId) as StoredScheduledTaskV1 | undefined
      if (!task?.hostDelivery?.pending || JSON.stringify(task.hostDelivery.pending) !== JSON.stringify(request)) return { tasks: latest.schedule.tasks, value: false }
      // No receipt means no History, last-run timestamp or inferred message.
      if (outcome.status === 'unknown' || outcome.status === 'not_found') return { tasks: latest.schedule.tasks, value: true }
      const { pending: _pending, ...state } = task.hostDelivery
      void _pending
      const now = new Date()
      const receipt = outcome.receipt
      const accepted = outcome.status === 'accepted' && receipt !== undefined
      const record = accepted ? normalizeScheduleDeliveryRecord({ attemptId: receipt.attemptId,
        scheduledAt: receipt.scheduledAt, deliveredAt: receipt.deliveredAt,
        messageId: receipt.messageId, prompt: request.prompt }) : undefined
      const next: StoredScheduledTaskV1 = { ...task,
        ...(task.schedule.kind === 'at' ? { enabled: false } : {}),
        nextRunAt: task.schedule.kind === 'at' ? '' : computeScheduleNextRunAt(task, now),
        lastStatus: accepted ? 'success' : 'error',
        lastMessage: accepted ? 'schedule_task_accepted' : 'schedule_task_failed',
        ...(accepted ? { lastRunAt: receipt.deliveredAt, lastThreadId: receipt.threadId } : {}),
        updatedAt: now.toISOString(), hostDelivery: { ...state, records: record ? [...state.records, record] : state.records }
      }
      return { tasks: latest.schedule.tasks.map(item => item.id === request.taskId ? next : item), value: true }
    })
    this.syncPowerSaveBlocker(result.settings)
    return result.value
  }

  private async runTaskInternal(task: ScheduledTaskV1, scheduled: boolean): Promise<ScheduleRunResult> {
    if ((task as StoredScheduledTaskV1).hostDelivery) return this.runOriginTask(task.id, scheduled)
    if (this.runningTaskIds.has(task.id)) {
      return { ok: false, message: 'Task is already running.' }
    }
    if (scheduled && (!task.enabled || task.schedule.kind === 'manual')) {
      return { ok: false, message: 'Task is not scheduled.' }
    }
    if (!task.prompt.trim()) {
      return { ok: false, message: 'Task prompt is empty.' }
    }

    this.runningTaskIds.add(task.id)
    await this.updateTask(task.id, (current) => ({
      ...current,
      lastStatus: 'running',
      lastMessage: 'schedule_task_running',
      nextRunAt: '',
      updatedAt: new Date().toISOString()
    }))

    try {
      const settings = await this.deps.store.load()
      const clawChannel = this.resolveTaskClawChannel(settings, task)
      const modelConfig = this.resolveScheduleModelConfig(settings, {
        providerId: task.providerId,
        model: task.model,
        reasoningEffort: task.reasoningEffort
      })
      const result = await this.runPrompt(settings, {
        prompt: task.prompt,
        title: scheduledThreadTitle(task.title),
        workspaceRoot: this.resolveTaskWorkspaceRoot(settings, task, clawChannel),
        model: modelConfig.model,
        reasoningEffort: modelConfig.reasoningEffort,
        mode: task.mode,
        clawChannel,
        waitForResult: false,
        responseTimeoutMs: TASK_RESPONSE_TIMEOUT_MS
      })
      if (!result.ok) {
        const finishedAt = new Date()
        await this.updateTask(task.id, (current) => ({
          ...current,
          ...(current.schedule.kind === 'at' ? { enabled: false } : {}),
          lastRunAt: finishedAt.toISOString(),
          nextRunAt: current.schedule.kind === 'at' ? '' : computeScheduleNextRunAt(current, finishedAt),
          lastStatus: 'error',
          lastMessage: 'schedule_task_failed',
          updatedAt: finishedAt.toISOString()
        }))
        this.runningTaskIds.delete(task.id)
        return result
      }

      const startedAt = new Date()
      await this.updateTask(task.id, (current) => ({
        ...current,
        lastRunAt: startedAt.toISOString(),
        nextRunAt: '',
        lastStatus: 'running',
        lastMessage: 'schedule_task_started',
        lastThreadId: result.threadId,
        updatedAt: startedAt.toISOString()
      }))
      void this.monitorTaskTurn(task.id, result.threadId, result.turnId ?? '')
      return result
    } catch (error) {
      void error
      const finishedAt = new Date()
      await this.updateTask(task.id, (current) => ({
        ...current,
        lastRunAt: finishedAt.toISOString(),
        nextRunAt: computeScheduleNextRunAt(current, finishedAt),
        lastStatus: 'error',
        lastMessage: 'schedule_task_failed',
        updatedAt: finishedAt.toISOString()
      }))
      this.runningTaskIds.delete(task.id)
      return { ok: false, message: 'Scheduled task failed.' }
    }
  }

  private async monitorTaskTurn(taskId: string, threadId: string, turnId: string): Promise<void> {
    try {
      const settings = await this.deps.store.load()
      const task = settings.schedule.tasks.find((item) => item.id === taskId)
      await this.waitForAssistantText(
        settings,
        threadId,
        turnId,
        TASK_RESPONSE_TIMEOUT_MS,
        task?.workspaceRoot || this.resolveDefaultWorkspaceRoot(settings)
      )
      const finishedAt = new Date()
      await this.updateTask(taskId, (current) => ({
        ...current,
        ...(current.schedule.kind === 'at' ? { enabled: false } : {}),
        nextRunAt: current.schedule.kind === 'at' ? '' : computeScheduleNextRunAt(current, finishedAt),
        lastStatus: 'success',
        lastMessage: 'schedule_task_completed',
        lastThreadId: threadId,
        updatedAt: finishedAt.toISOString()
      }))
    } catch (error) {
      void error
      const finishedAt = new Date()
      await this.updateTask(taskId, (current) => ({
        ...current,
        ...(current.schedule.kind === 'at' ? { enabled: false } : {}),
        nextRunAt: current.schedule.kind === 'at' ? '' : computeScheduleNextRunAt(current, finishedAt),
        lastStatus: 'error',
        lastMessage: 'schedule_task_failed',
        lastThreadId: threadId || current.lastThreadId,
        updatedAt: finishedAt.toISOString()
      }))
      this.deps.logError('schedule-task', 'Scheduled task failed', { taskId })
    } finally {
      this.runningTaskIds.delete(taskId)
    }
  }

  private async runPrompt(
    settings: AppSettingsV1,
    options: RunPromptOptions & { deleteThreadAfterResult?: boolean; maxModelSteps?: number }
  ): Promise<ScheduleRunResult> {
    const workspace = options.workspaceRoot.trim() || this.resolveDefaultWorkspaceRoot(settings)
    if (workspace) {
      await mkdir(workspace, { recursive: true })
    }
    const model = normalizeTaskModel(options.model) ?? (settings.runtime.model.trim() || DEFAULT_SCHEDULE_MODEL)
    const create = await this.deps.runtimeRequest(settings, '/v1/threads', {
      method: 'POST',
      body: JSON.stringify({
        workspace,
        model,
        mode: options.mode,
        approvalPolicy: SCHEDULE_APPROVAL_POLICY,
        sandboxMode: SCHEDULE_SANDBOX_MODE,
        ...(options.title.trim() ? { title: options.title.trim() } : {})
      })
    })
    if (!create.ok) return { ok: false, message: runtimeErrorMessage(create, 'Failed to create thread.') }
    const thread = JSON.parse(create.body) as ThreadRecordJson
    try {
      const turnBody: Record<string, unknown> = {
        prompt: options.clawChannel
          ? buildClawRuntimePrompt(settings, options.prompt, { channel: options.clawChannel })
          : buildScheduleRuntimePrompt(settings, options.prompt),
        mode: options.mode,
        approvalPolicy: SCHEDULE_APPROVAL_POLICY,
        sandboxMode: SCHEDULE_SANDBOX_MODE,
        // Scheduled turns are headless — nobody can answer a user_input
        // prompt, and a turn that asks one hangs until the response timeout.
        disableUserInput: true
      }
      if (model) turnBody.model = model
      if (options.reasoningEffort) {
        turnBody.reasoningEffort = options.reasoningEffort
      }
      if (options.maxModelSteps !== undefined) {
        turnBody.maxModelSteps = options.maxModelSteps
      }
      const turn = await this.deps.runtimeRequest(
        settings,
        `/v1/threads/${encodeURIComponent(thread.id)}/turns`,
        { method: 'POST', body: JSON.stringify(turnBody) }
      )
      if (!turn.ok) return { ok: false, message: runtimeErrorMessage(turn, 'Failed to start turn.') }

      const parsedTurn = parseJsonObject(turn.body)
      const turnId = asString(nestedRecord(parsedTurn?.turn).id) || asString(parsedTurn?.turnId)
      if (!turnId) {
        return { ok: false, message: 'Failed to start turn: missing turn id.' }
      }
      if (!options.waitForResult) {
        return { ok: true, threadId: thread.id, turnId, message: 'Started' }
      }

      const text = await this.waitForAssistantText(settings, thread.id, turnId, options.responseTimeoutMs, workspace)
      return { ok: true, threadId: thread.id, turnId, text, message: text || 'Completed' }
    } finally {
      if (options.deleteThreadAfterResult) {
        await Promise.resolve(this.deps.runtimeRequest(
          settings,
          `/v1/threads/${encodeURIComponent(thread.id)}`,
          { method: 'DELETE' }
        )).catch(() => undefined)
      }
    }
  }

  private async waitForAssistantText(
    settings: AppSettingsV1,
    threadId: string,
    turnId: string,
    timeoutMs: number,
    workspaceRoot?: string
  ): Promise<string> {
    void workspaceRoot
    const deadline = Date.now() + timeoutMs
    let lastText = ''
    while (Date.now() < deadline) {
      await sleep(1_500)
      const detailRes = await this.deps.runtimeRequest(
        settings,
        `/v1/threads/${encodeURIComponent(threadId)}`,
        { method: 'GET' }
      )
      if (!detailRes.ok) {
        throw new Error(runtimeErrorMessage(detailRes, 'Failed to read thread result.'))
      }
      const detail = JSON.parse(detailRes.body) as ThreadDetailJson
      lastText = latestAssistantText(detail, { turnId }) || lastText
      const targetTurn = Array.isArray(detail.turns)
        ? detail.turns.find((turn) => turn.id === turnId)
        : undefined
      if (!targetTurn) continue
      if (isRunningStatus(targetTurn.status)) continue
      if (targetTurn.status === 'failed' || targetTurn.status === 'aborted') {
        throw new Error(`Agent turn ${targetTurn.status}.`)
      }
      if (targetTurn.status === 'completed' && lastText) return lastText
    }
    if (lastText) return lastText
    throw new Error('Timed out waiting for agent response.')
  }

  private resolveDefaultWorkspaceRoot(settings: AppSettingsV1): string {
    return settings.schedule.defaultWorkspaceRoot.trim() || settings.workspaceRoot
  }

  private resolveClawChannel(settings: AppSettingsV1, channelId: string | null | undefined): ClawImChannelV1 | null {
    const id = channelId?.trim()
    if (!id) return null
    return settings.claw.channels.find((channel) => channel.id === id) ?? null
  }

  private resolveTaskClawChannel(settings: AppSettingsV1, task: ScheduledTaskV1): ClawImChannelV1 | null {
    return this.resolveClawChannel(settings, task.clawChannelId)
  }

  private resolveClawChannelWorkspaceRoot(settings: AppSettingsV1, channel: ClawImChannelV1): string {
    return channel.workspaceRoot.trim() || settings.claw.im.workspaceRoot.trim() || this.resolveDefaultWorkspaceRoot(settings)
  }

  private resolveTaskWorkspaceRoot(
    settings: AppSettingsV1,
    task: ScheduledTaskV1,
    channel: ClawImChannelV1 | null
  ): string {
    return task.workspaceRoot.trim() ||
      (channel ? this.resolveClawChannelWorkspaceRoot(settings, channel) : this.resolveDefaultWorkspaceRoot(settings))
  }

  private syncInternalServer(settings: AppSettingsV1): void {
    const internal = settings.schedule.internal
    const key = `${internal.port}`
    if (this.server && this.serverKey === key) return
    this.closeInternalServer()

    const server = createServer((req, res) => {
      void this.handleInternalRequest(req, res)
    })
    server.on('error', (error) => {
      void error
      this.deps.logError('schedule-server', 'Schedule internal server failed', {
        operation: 'listen'
      })
      if (this.server === server) {
        this.closeInternalServer()
      }
    })
    server.listen(internal.port, '127.0.0.1')
    this.server = server
    this.serverKey = key
  }

  private closeInternalServer(): void {
    if (!this.server) return
    const server = this.server
    this.server = null
    this.serverKey = ''
    server.close()
  }

  private async handleInternalRequest(req: IncomingMessage, res: ServerResponse): Promise<void> {
    try {
      const settings = await this.deps.store.load()
      const url = new URL(req.url ?? '/', 'http://127.0.0.1')
      if (!url.pathname.startsWith('/schedule/internal/')) {
        writeJson(res, 404, { ok: false, message: 'Not found.' })
        return
      }
      if (req.method !== 'POST') {
        writeJson(res, 405, { ok: false, message: 'Method not allowed.' })
        return
      }
      const secret = settings.schedule.internal.secret.trim()
      if (secret) {
        const auth = req.headers.authorization ?? ''
        // 新名字 x-analytix-secret 优先;旧名字 x-analytix-secret 已配置
        // 在外部系统里,属于对外契约,必须长期兼容。
        const rawHeaderSecret = req.headers['x-analytix-secret'] ?? req.headers['x-analytix-secret']
        const headerSecret = Array.isArray(rawHeaderSecret) ? rawHeaderSecret[0] : rawHeaderSecret
        if (auth !== `Bearer ${secret}` && headerSecret !== secret) {
          writeJson(res, 401, { ok: false, message: 'Unauthorized.' })
          return
        }
      }

      if (url.pathname === '/schedule/internal/list') {
        const tasks = (await this.listTasks()).map(scheduleTaskReceiptView)
        writeJson(res, 200, { ok: true, tasks })
        return
      }

      const body = await readRequestBody(req)
      const payload = parseJsonObject(body)
      if (!payload) {
        writeJson(res, 400, { ok: false, message: 'Expected a JSON object.' })
        return
      }

      if (url.pathname === '/schedule/internal/create') {
        const originDeclared = Object.hasOwn(payload, 'origin')
        const marked = req.headers['x-analytix-schedule-origin'] === 'typed-v1'
        if (Object.keys(payload).some(key => key !== 'input' && key !== 'origin') || marked !== originDeclared) {
          writeJson(res, 400, { ok: false, message: 'Invalid native Schedule envelope.' }); return
        }
        let origin: ScheduleTaskOriginV1 | undefined
        if (originDeclared) {
          const secret = settings.schedule.internal.secret.trim()
          if (!secret || req.headers.authorization !== `Bearer ${secret}`) {
            writeJson(res, 401, { ok: false, message: 'Authenticated native Schedule helper is required.' }); return
          }
          const source = scheduleNativeOriginSchemaV1.safeParse(payload.origin)
          if (!source.success) { writeJson(res, 400, { ok: false, message: 'Invalid native Schedule origin.' }); return }
          origin = source.data
        }
        const parsed = scheduleTaskCreatePayloadSchema.safeParse(payload.input)
        if (!parsed.success) {
          writeJson(res, 400, { ok: false, message: 'Invalid task input.' })
          return
        }
        const saved = await this.createTaskFromInput(parsed.data, origin)
        writeJson(res, 200, { ok: true, task: scheduleTaskReceiptView(saved) })
        return
      }

      if (url.pathname === '/schedule/internal/update') {
        const taskId = asString(payload.taskId)
        const patch = nestedRecord(payload.patch)
        if (!taskId) {
          writeJson(res, 400, { ok: false, message: 'Missing taskId.' })
          return
        }
        const updated = await this.updateTaskById(taskId, patch as ScheduleTaskEditPatch)
        if (!updated) {
          writeJson(res, 404, { ok: false, message: 'Task not found.' })
          return
        }
        writeJson(res, 200, { ok: true, task: scheduleTaskReceiptView(updated) })
        return
      }

      if (url.pathname === '/schedule/internal/delete') {
        const taskId = asString(payload.taskId)
        if (!taskId) {
          writeJson(res, 400, { ok: false, message: 'Missing taskId.' })
          return
        }
        const removed = await this.deleteTaskById(taskId)
        writeJson(res, removed ? 200 : 404, removed ? { ok: true } : { ok: false, message: 'Task not found.' })
        return
      }

      writeJson(res, 404, { ok: false, message: 'Not found.' })
    } catch (error) {
      void error
      this.deps.logError('schedule-server', 'Schedule internal request failed', {
        operation: 'request'
      })
      writeJson(res, 500, { ok: false, message: 'Internal server error.' })
    }
  }

  private syncPowerSaveBlocker(settings: AppSettingsV1): void {
    const shouldKeepAwake =
      settings.schedule.keepAwake &&
      settings.schedule.enabled &&
      hasEnabledScheduledTask(settings)
    if (!shouldKeepAwake) {
      this.stopPowerSaveBlocker()
      return
    }
    if (this.isPowerSaveBlockerActive()) return
    const blocker = this.deps.powerSaveBlocker
    if (!blocker) return
    this.powerSaveBlockerId = blocker.start('prevent-app-suspension')
  }

  private stopPowerSaveBlocker(): void {
    const blocker = this.deps.powerSaveBlocker
    const id = this.powerSaveBlockerId
    this.powerSaveBlockerId = null
    if (!blocker || id == null) return
    try {
      if (blocker.isStarted(id)) blocker.stop(id)
    } catch (error) {
      void error
      this.deps.logError('schedule-power-save', 'Failed to stop power save blocker', {
        operation: 'stop'
      })
    }
  }

  private isPowerSaveBlockerActive(): boolean {
    const blocker = this.deps.powerSaveBlocker
    const id = this.powerSaveBlockerId
    if (!blocker || id == null) return false
    try {
      return blocker.isStarted(id)
    } catch {
      return false
    }
  }
}

export function createScheduleRuntime(deps: ScheduleRuntimeDeps): ScheduleRuntime {
  return new ScheduleRuntime(deps)
}
