import { z } from 'zod'
import { scheduleCalendarNextRunAt } from './schedule-calendar'
import type { IncomingMessage, ServerResponse } from 'node:http'
import type {
  AppSettingsV1,
  ClawImChannelV1,
  ScheduleReasoningEffort,
  ScheduleRunMode,
  ScheduledTaskV1
} from '../shared/app-settings'
import type { JsonSettingsStore } from './settings-store'
import { sanitizePublicSerializedText } from '../shared/public-runtime-content'

export type RuntimeRequestResult = { ok: boolean; status: number; body: string }

export type RuntimeRequestFn = (
  settings: AppSettingsV1,
  pathAndQuery: string,
  init: { method?: string; body?: string; headers?: Record<string, string> }
) => Promise<RuntimeRequestResult>

export type PowerSaveBlockerLike = {
  start: (type: 'prevent-app-suspension' | 'prevent-display-sleep') => number
  stop: (id: number) => void
  isStarted: (id: number) => boolean
}

export type ScheduleRuntimeDeps = {
  store: JsonSettingsStore
  runtimeRequest: RuntimeRequestFn
  localDisplayRequest?: (settings: AppSettingsV1, path: string, body: string) => Promise<RuntimeRequestResult>
  logError: (category: string, message: string, detail?: unknown) => void
  onCommittedChange?: () => void
  powerSaveBlocker?: PowerSaveBlockerLike
}

export type ThreadRecordJson = {
  id: string
  status?: string
}

export type TurnRecordJson = {
  id: string
  status?: string
  error?: string | null
  items?: TurnItemJson[]
}

export type TurnItemJson = {
  kind: string
  turnId?: string
  text?: string | null
  summary?: string
  detail?: string | null
}

export type ThreadDetailJson = {
  thread?: ThreadRecordJson
  id?: string
  status?: string
  turns?: TurnRecordJson[]
  items?: TurnItemJson[]
}

export type RunPromptOptions = {
  prompt: string
  title: string
  workspaceRoot: string
  model: string
  reasoningEffort: ScheduleReasoningEffort
  mode: ScheduleRunMode
  clawChannel?: ClawImChannelV1 | null
  waitForResult: boolean
  responseTimeoutMs: number
}

export const SCHEDULER_INTERVAL_MS = 30_000
export const INTERNAL_BODY_LIMIT_BYTES = 1_000_000
export const TASK_RESPONSE_TIMEOUT_MS = 30 * 60_000

export function parseJsonObject(raw: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(raw) as unknown
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : null
  } catch {
    return null
  }
}

export function runtimeErrorMessage(result: RuntimeRequestResult, fallback: string): string {
  void result
  return fallback
}

export function isRunningStatus(status: string | undefined): boolean {
  return status === 'queued' || status === 'in_progress' || status === 'started' || status === 'running'
}

export function latestAssistantText(
  detail: ThreadDetailJson,
  options: { turnId?: string } = {}
): string {
  const turnId = options.turnId?.trim()
  const items = turnId
    ? threadItems(detail).filter((item) => item.turnId === turnId)
    : threadItems(detail)
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index]
    if (item.kind !== 'assistant_text' && item.kind !== 'agent_message') continue
    const text = sanitizePublicSerializedText(item.text ?? item.detail ?? item.summary ?? '')
    if (text) return text
  }
  return ''
}

function threadItems(detail: ThreadDetailJson): TurnItemJson[] {
  const turns = Array.isArray(detail.turns) ? detail.turns : []
  const singleTurnId = turns.length === 1 ? turns[0].id : ''
  const topLevelItems = Array.isArray(detail.items)
    ? detail.items.map((item) => ({ ...item, turnId: item.turnId || singleTurnId || undefined }))
    : []
  const turnItems = turns.flatMap((turn) =>
    Array.isArray(turn.items)
      ? turn.items.map((item) => ({ ...item, turnId: item.turnId || turn.id }))
      : []
  )
  return [...topLevelItems, ...turnItems]
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export function normalizeTaskModel(model: string): string | undefined {
  const trimmed = model.trim()
  return trimmed && trimmed.toLowerCase() !== 'auto' ? trimmed : undefined
}


export function computeScheduleNextRunAt(task: ScheduledTaskV1, from: Date): string {
  if (!task.enabled || task.schedule.kind === 'manual') return ''
  if (task.schedule.kind === 'at') {
    return task.schedule.atTime.trim()
  }
  if (task.schedule.kind === 'interval') {
    return new Date(from.getTime() + task.schedule.everyMinutes * 60_000).toISOString()
  }

  if (task.schedule.kind === 'weekly' || task.schedule.kind === 'cron' || Object.prototype.hasOwnProperty.call(task.schedule, 'timeZone')) {
    return scheduleCalendarNextRunAt(task.schedule, from)
  }

  const [hourRaw, minuteRaw] = task.schedule.timeOfDay.split(':')
  const hour = Number(hourRaw)
  const minute = Number(minuteRaw)
  const next = new Date(from)
  next.setSeconds(0, 0)
  next.setHours(Number.isFinite(hour) ? hour : 9, Number.isFinite(minute) ? minute : 0, 0, 0)
  if (next.getTime() <= from.getTime()) {
    next.setDate(next.getDate() + 1)
  }
  return next.toISOString()
}

export function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function nestedRecord(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

export function writeJson(res: ServerResponse, status: number, body: unknown): void {
  const payload = JSON.stringify(body)
  res.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': Buffer.byteLength(payload)
  })
  res.end(payload)
}

export async function readRequestBody(req: IncomingMessage): Promise<string> {
  let size = 0
  const chunks: Buffer[] = []
  for await (const chunk of req) {
    const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)
    size += buffer.length
    if (size > INTERNAL_BODY_LIMIT_BYTES) {
      throw new Error('Request body is too large.')
    }
    chunks.push(buffer)
  }
  return Buffer.concat(chunks).toString('utf8')
}

export function internalUrl(settings: AppSettingsV1): string {
  return `http://127.0.0.1:${settings.schedule.internal.port}`
}

export function hasEnabledScheduledTask(settings: AppSettingsV1): boolean {
  return settings.schedule.tasks.some((task) => task.enabled && task.schedule.kind !== 'manual')
}


// Persisted only by Main, never accepted as editable task input. Shape validation
// here does not authenticate an origin; native creation must bind Host metadata.
export type ScheduleTaskOriginV1 = Readonly<{ threadId: string; turnId: string; toolCallId: string; grantId: string }>
export type ScheduleTaskHostDeliveryV1 = Readonly<{
  schemaVersion: 1
  origin: ScheduleTaskOriginV1
  records: readonly import('../shared/schedule-task-mutation').ScheduleTaskDeliveryRecord[]
  pending?: ScheduleAdmissionRequestV1
}>
export type StoredScheduledTaskV1 = ScheduledTaskV1 & { hostDelivery?: ScheduleTaskHostDeliveryV1 }

function scheduleStoredId(value: unknown): string {
  if (typeof value !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/.test(value) || value.includes('..')) throw new Error('Invalid saved Schedule identity.')
  return value
}
function scheduleStoredObject(value: unknown, keys: readonly string[], optional: readonly string[] = []): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid saved Schedule record.')
  const record = value as Record<string, unknown>
  if (keys.some(key => !Object.hasOwn(record, key)) || Object.keys(record).some(key => !keys.includes(key) && !optional.includes(key))) throw new Error('Invalid saved Schedule record fields.')
  return record
}
export function normalizeScheduleTaskOrigin(value: unknown): ScheduleTaskOriginV1 {
  const record = scheduleStoredObject(value, ['threadId', 'turnId', 'toolCallId', 'grantId'])
  return scheduleNativeOriginSchemaV1.parse(record)
}
export function normalizeScheduleDeliveryRecord(value: unknown): import('../shared/schedule-task-mutation').ScheduleTaskDeliveryRecord {
  const record = scheduleStoredObject(value, ['attemptId', 'scheduledAt', 'deliveredAt', 'messageId'], ['prompt'])
  const time = (value: unknown) => { if (typeof value !== 'string' || !/^(?!0000)\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) || !Number.isFinite(Date.parse(value)) || new Date(value).toISOString().slice(0, 19) !== value.slice(0, 19)) throw new Error('Invalid saved Schedule delivery time.'); return value }
  if (Object.hasOwn(record, 'prompt') && typeof record.prompt !== 'string') throw new Error('Invalid saved Schedule delivery prompt.')
  return { attemptId: scheduleStoredId(record.attemptId), scheduledAt: time(record.scheduledAt), deliveredAt: time(record.deliveredAt), messageId: scheduleStoredId(record.messageId), ...(typeof record.prompt === 'string' ? { prompt: record.prompt } : {}) }
}
export function normalizeScheduleTaskHostDelivery(value: unknown): ScheduleTaskHostDeliveryV1 {
  const record = scheduleStoredObject(value, ['schemaVersion', 'origin', 'records'], ['pending'])
  if (record.schemaVersion !== 1 || !Array.isArray(record.records)) throw new Error('Invalid saved Schedule delivery state.')
  const records = record.records.map(normalizeScheduleDeliveryRecord)
  if (new Set(records.map(item => item.attemptId)).size !== records.length) throw new Error('Duplicate saved Schedule delivery identity.')
  const origin = normalizeScheduleTaskOrigin(record.origin)
  const pending = Object.hasOwn(record, 'pending') ? scheduleAdmissionRequestSchemaV1.parse(record.pending) : undefined
  if (pending && (pending.kind !== 'submit' || JSON.stringify(pending.origin) !== JSON.stringify(origin) || records.some(item => item.attemptId === pending.attemptId))) throw new Error('Invalid saved Schedule attempt binding.')
  return { schemaVersion: 1, origin, records, ...(pending ? { pending } : {}) }
}

/** Preserve only private Main state at every settings normalization boundary. */
export function preserveScheduleTaskHostDelivery(tasks: ScheduledTaskV1[], source: unknown, preferSource = false): StoredScheduledTaskV1[] {
  if (!Array.isArray(source)) return tasks
  const saved = new Map<string, unknown>()
  for (const item of source) {
    if (!item || typeof item !== 'object' || !Object.hasOwn(item, 'hostDelivery')) continue
    const raw = item as Record<string, unknown>
    saved.set(scheduleStoredId(raw.id), raw.hostDelivery)
  }
  return tasks.map(task => {
    const own = Object.hasOwn(task, 'hostDelivery') && !preferSource
    const value = own ? (task as StoredScheduledTaskV1).hostDelivery : saved.get(task.id)
    if (!own && !saved.has(task.id)) return task
    const hostDelivery = normalizeScheduleTaskHostDelivery(value)
    if (hostDelivery.pending && hostDelivery.pending.taskId !== task.id) throw new Error('Invalid saved Schedule task binding.')
    return { ...task, hostDelivery }
  })
}

/** Current management reads expose only original-thread navigation, never grants/history. */
export function scheduleTaskReadView(task: ScheduledTaskV1): ScheduledTaskV1 {
  const { hostDelivery, ...read } = task as StoredScheduledTaskV1
  return hostDelivery ? { ...read, originalThreadId: hostDelivery.origin.threadId, deliveryKind: 'origin-delivery', ...(hostDelivery.records.length ? { latestDeliveryAttemptId: hostDelivery.records[hostDelivery.records.length - 1].attemptId } : {}) }
    : { ...read, deliveryKind: 'legacy-headless' }
}
/** Historical tool receipts retain their established committed task shape. */
export function scheduleTaskReceiptView(task: ScheduledTaskV1): ScheduledTaskV1 {
  const { originalThreadId: _origin, deliveryKind: _kind, latestDeliveryAttemptId: _latest, ...receipt } = scheduleTaskReadView(task)
  void _origin; void _kind; void _latest
  return receipt
}
export function scheduleSettingsReadView(settings: AppSettingsV1): AppSettingsV1 {
  return { ...settings, schedule: { ...settings.schedule, tasks: settings.schedule.tasks.map(scheduleTaskReceiptView) } }
}


export const SCHEDULE_ADMISSION_PATH_V1 = '/v1/local-display/schedule-admission'
const admissionId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
const hostCallId = z.string().regex(/^call_host_[a-f0-9]{64}$/)
export const scheduleNativeOriginSchemaV1 = z.object({ threadId: admissionId, turnId: admissionId, toolCallId: hostCallId, grantId: admissionId }).strict()
/** Match Go UTC RFC3339Nano: preserve nanoseconds and remove fractional trailing zeros. */
export function canonicalScheduleUTC(value: string): string {
  if (!/^(?!0000)\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) || !Number.isFinite(Date.parse(value)) || new Date(value).toISOString().slice(0, 19) !== value.slice(0, 19)) throw new Error('Invalid Schedule UTC instant.')
  return value.replace(/\.(\d+)Z$/, (_, fraction: string) => { const digits = fraction.replace(/0+$/, ''); return digits ? `.${digits}Z` : 'Z' })
}
const admissionTime = z.string().refine(value => { try { return canonicalScheduleUTC(value) === value } catch { return false } })
const executionSelection = z.string().min(1).refine(value => new TextEncoder().encode(value).length <= 255 && value.trim() === value && !/[\0\r\n]/.test(value))
export const scheduleAdmissionRequestSchemaV1 = z.object({
  schemaVersion: z.literal(1), kind: z.enum(['submit', 'query']), taskId: admissionId,
  origin: scheduleNativeOriginSchemaV1, attemptId: z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/),
  scheduledAt: admissionTime, prompt: z.string().refine(value => value.trim() !== '' && !value.includes('\0') && !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= 16 * 1024),
  execution: z.object({ providerId: executionSelection.optional(), model: executionSelection.optional(),
    reasoningEffort: z.enum(['auto', 'off', 'low', 'medium', 'high', 'max']).optional(), mode: z.enum(['agent', 'plan']).optional()
  }).strict()
}).strict().refine(value => new TextEncoder().encode(JSON.stringify(value)).length <= 32 * 1024)
export type ScheduleAdmissionRequestV1 = z.infer<typeof scheduleAdmissionRequestSchemaV1>
export const scheduleAdmissionReceiptSchemaV1 = z.object({ taskId: admissionId, attemptId: z.string(), messageId: admissionId, threadId: admissionId,
  turnId: admissionId, delivery: z.enum(['start', 'steer']), scheduledAt: admissionTime, deliveredAt: admissionTime
}).strict()
export type ScheduleAdmissionReceiptV1 = z.infer<typeof scheduleAdmissionReceiptSchemaV1>
const admissionResponse = z.object({ schemaVersion: z.literal(1), status: z.enum(['accepted', 'not_found', 'rejected', 'unknown', 'conflict', 'invalid']),
  code: z.enum(['schedule_admission_conflict', 'schedule_admission_unavailable']).optional(), receipt: scheduleAdmissionReceiptSchemaV1.optional()
}).strict().superRefine((value, ctx) => {
  const code = value.status === 'conflict' ? 'schedule_admission_conflict' : ['rejected', 'unknown'].includes(value.status) ? 'schedule_admission_unavailable' : undefined
  if (value.code !== code || (value.status === 'accepted') !== (value.receipt !== undefined)) ctx.addIssue({ code: 'custom', message: 'Invalid Schedule admission response.' })
})
export type ScheduleAdmissionResponseV1 = z.infer<typeof admissionResponse>
/** The private transport keeps source-exact non-2xx statuses; no public sanitizer. */
export async function requestScheduleAdmission(deps: ScheduleRuntimeDeps, settings: AppSettingsV1, request: ScheduleAdmissionRequestV1): Promise<ScheduleAdmissionResponseV1> {
  const frozen = scheduleAdmissionRequestSchemaV1.parse(request)
  const unavailable: ScheduleAdmissionResponseV1 = { schemaVersion: 1, status: 'unknown', code: 'schedule_admission_unavailable' }
  if (!deps.localDisplayRequest) return unavailable
  try {
    const response = await deps.localDisplayRequest(settings, SCHEDULE_ADMISSION_PATH_V1, JSON.stringify(frozen))
    if (new TextEncoder().encode(response.body).length > 4096 || !/"schemaVersion"\s*:\s*1(?=\s*[,}])/.test(response.body)) return unavailable
    const parsed = admissionResponse.parse(JSON.parse(response.body))
    const expectedHTTP = ['accepted', 'not_found'].includes(parsed.status) ? 200 : ['rejected', 'conflict'].includes(parsed.status) ? 409 : parsed.status === 'invalid' ? 400 : 503
    if (response.status !== expectedHTTP) return unavailable
    const receipt = parsed.receipt
    if (receipt && (receipt.taskId !== frozen.taskId || receipt.attemptId !== frozen.attemptId || receipt.threadId !== frozen.origin.threadId || receipt.scheduledAt !== frozen.scheduledAt)) return unavailable
    return parsed
  } catch { return unavailable }
}
