import type {
  ScheduleReasoningEffort,
  ScheduleRunMode,
  ScheduledTaskV1
} from '../shared/app-settings'
import {
  DEFAULT_SCHEDULE_MODEL,
  DEFAULT_SCHEDULE_REASONING_EFFORT
} from '../shared/app-settings'

const SCHEDULED_TASK_CANDIDATE_RE =
  /(?:提醒|定时|闹钟|通知|叫我|叫醒|稍后|之后|到点|分钟后|小时后|秒后|天后|明天|后天|今晚|later|remind|reminder|alarm|timer|schedule|scheduled|tomorrow|tonight|in\s+\d+\s+(?:seconds?|minutes?|hours?|days?|weeks?))/iu

const ISO_WITH_TIMEZONE_RE = /(?:[zZ]|[+-]\d{2}:\d{2})$/u

type DetectionPayload = {
  shouldCreateTask?: boolean
  scheduleAt?: string
  reminderBody?: string
  taskName?: string
}

export type ParsedClawScheduledTaskRequest = {
  kind: 'create'
  sourceText: string
  reminderBody: string
  runAt: Date
  scheduleAt: string
  taskName: string
  taskPrompt: string
  confirmationText: string
}

function normalizeReminderBody(value: string): string {
  return value
    .trim()
    .replace(/^[,，:：\s]+/u, '')
    .replace(/[。！？!?~～\s]+$/u, '')
    .replace(/^(?:一下|一声|一下子)\s*/u, '')
    .trim()
}

function normalizeReminderName(value: string): string {
  const normalized = value
    .replace(/[。！？!?]/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!normalized) return 'Reminder'
  const compact = normalized.length > 20 ? normalized.slice(0, 20).trim() : normalized
  return /(?:提醒|reminder)$/iu.test(compact) ? compact : `${compact} reminder`
}

function buildTaskPrompt(body: string): string {
  if (!body) return '⏰ Reminder'
  if (body.startsWith('⏰')) return body
  if (/^提醒[:：]?/u.test(body)) return `⏰ ${body}`
  if (/^remind(?:er)?[:：]?\s*/iu.test(body)) return `⏰ ${body}`
  return `⏰ Reminder: ${body}`
}

function formatRelativeDelayLabel(now: Date, runAt: Date): string {
  const diffMs = Math.max(0, runAt.getTime() - now.getTime())
  const minuteMs = 60_000
  const hourMs = 3_600_000
  const dayMs = 86_400_000
  if (diffMs < minuteMs) {
    const seconds = Math.max(1, Math.round(diffMs / 1000))
    return `${seconds}s later`
  }
  if (diffMs < hourMs) {
    const minutes = Math.max(1, Math.round(diffMs / minuteMs))
    return `${minutes}min later`
  }
  if (diffMs < dayMs) {
    const hours = Math.max(1, Math.round(diffMs / hourMs))
    return `${hours}h later`
  }
  const days = Math.max(1, Math.round(diffMs / dayMs))
  return `${days}d later`
}

function formatConfirmationText(scheduleAt: string, runAt: Date, body: string, now: Date): string {
  const delayLabel = formatRelativeDelayLabel(now, runAt)
  const localText = runAt.toLocaleString()
  return `Scheduled. I will handle "${body}" at ${localText} (${delayLabel}).`
}

function extractFirstJsonObject(text: string): string | null {
  const trimmed = text.trim()
  if (!trimmed) return null
  const fencedMatch = trimmed.match(/```(?:json)?\s*([\s\S]*?)```/iu)
  const candidate = fencedMatch?.[1]?.trim() || trimmed
  const start = candidate.indexOf('{')
  const end = candidate.lastIndexOf('}')
  if (start === -1 || end === -1 || end <= start) return null
  return candidate.slice(start, end + 1)
}

function parseDetectionPayload(raw: string): DetectionPayload | null {
  const json = extractFirstJsonObject(raw)
  if (!json) return null
  try {
    const parsed = JSON.parse(json) as DetectionPayload
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function normalizeDetectedRequest(
  payload: DetectionPayload | null,
  sourceText: string,
  now = new Date()
): ParsedClawScheduledTaskRequest | null {
  if (!payload?.shouldCreateTask) return null
  const scheduleAt = typeof payload.scheduleAt === 'string' ? payload.scheduleAt.trim() : ''
  if (!scheduleAt || !ISO_WITH_TIMEZONE_RE.test(scheduleAt)) return null
  const runAt = new Date(scheduleAt)
  if (!Number.isFinite(runAt.getTime()) || runAt.getTime() <= now.getTime()) return null
  const reminderBody = normalizeReminderBody(typeof payload.reminderBody === 'string' ? payload.reminderBody : '')
  if (!reminderBody) return null
  const taskName =
    typeof payload.taskName === 'string' && payload.taskName.trim()
      ? normalizeReminderName(payload.taskName)
      : normalizeReminderName(reminderBody)
  return {
    kind: 'create',
    sourceText,
    reminderBody,
    runAt,
    scheduleAt,
    taskName,
    taskPrompt: buildTaskPrompt(reminderBody),
    confirmationText: formatConfirmationText(scheduleAt, runAt, reminderBody, now)
  }
}

export function buildClawScheduledTaskDetectionPrompt(sourceText: string, now: Date): string {
  return [
    'You are a structured extractor for one-shot reminder requests.',
    `Current local datetime: ${now.toISOString()}.`,
    'Return JSON only. No markdown. No prose.',
    'Decide whether the user is explicitly asking to create a one-time scheduled reminder or delayed task.',
    'If yes, return: {"shouldCreateTask":true,"scheduleAt":"ISO8601 with explicit timezone offset","reminderBody":"short reminder content","taskName":"short task name"}',
    'If no, return: {"shouldCreateTask":false}',
    'Rules:',
    '- Only return true when the user explicitly wants a future one-shot reminder/task.',
    '- `scheduleAt` must be a future absolute timestamp with timezone offset.',
    '- `reminderBody` should be concise and describe what should happen at that time.',
    '- If the time is ambiguous, missing, or recurring, return false.',
    'User request:',
    sourceText
  ].join('\n\n')
}

export function looksLikeClawScheduledTaskCandidate(text: string): boolean {
  return SCHEDULED_TASK_CANDIDATE_RE.test(text.trim())
}

export function detectClawScheduledTaskRequest(
  sourceText: string,
  detectorResponse: string,
  now = new Date()
): Promise<ParsedClawScheduledTaskRequest | null> {
  if (!looksLikeClawScheduledTaskCandidate(sourceText)) return Promise.resolve(null)
  return Promise.resolve(normalizeDetectedRequest(parseDetectionPayload(detectorResponse), sourceText, now))
}

export function buildScheduledTaskFromDetectedRequest(options: {
  request: ParsedClawScheduledTaskRequest
  workspaceRoot: string
  providerId?: string
  model: string
  reasoningEffort?: ScheduleReasoningEffort
  mode: ScheduleRunMode
  id: string
  now?: string
}): ScheduledTaskV1 {
  const now = options.now ?? new Date().toISOString()
  return {
    id: options.id,
    title: options.request.taskName,
    enabled: true,
    prompt: options.request.taskPrompt,
    workspaceRoot: options.workspaceRoot.trim(),
    clawChannelId: '',
    providerId: options.providerId?.trim() ?? '',
    model: options.model.trim() || DEFAULT_SCHEDULE_MODEL,
    reasoningEffort: options.reasoningEffort ?? DEFAULT_SCHEDULE_REASONING_EFFORT,
    mode: options.mode,
    schedule: {
      kind: 'at',
      everyMinutes: 60,
      timeOfDay: '09:00',
      atTime: options.request.scheduleAt
    },
    createdAt: now,
    updatedAt: now,
    lastRunAt: '',
    nextRunAt: options.request.scheduleAt,
    lastStatus: 'idle',
    lastMessage: '',
    lastThreadId: ''
  }
}
