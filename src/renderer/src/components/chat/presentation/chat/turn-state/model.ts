/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
export const LIVE_RUN_CLOCK_INTERVAL_MS = 1000

export type Translate = (key: string, options?: Record<string, unknown>) => string
export type RunDurationPart = { readonly text: string; readonly numeric: boolean }

/** Pinned DSH whole-second clock fragments, with the current locale seat. */
export function formatRunDuration(ms: number, t: Translate): RunDurationPart[] {
  if (!Number.isFinite(ms)) return []
  const total = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor(total / 60) % 60
  const seconds = total % 60
  const parts: RunDurationPart[] = []
  if (hours > 0) parts.push({ text: String(hours), numeric: true }, { text: t('dshDurationHourUnit'), numeric: false })
  if (total >= 60) parts.push({ text: String(minutes), numeric: true }, { text: t('dshDurationMinuteUnit'), numeric: false })
  parts.push({ text: String(seconds), numeric: true }, { text: t('dshDurationSecondUnit'), numeric: false })
  return parts
}

export type TurnProcessStatus = 'completed' | 'failed' | 'aborted'
export function turnProcessPresentation({ open, hasContent, durationMs, status, collapsible = true }: {
  open: boolean; hasContent: boolean; durationMs?: number; status?: TurnProcessStatus; collapsible?: boolean
}) {
  // ChatView/ProtectedProcessPane owns live/failed/aborted/interleaved
  // expansion, including a protected history display request. Do not override it.
  const canCollapse = hasContent && collapsible
  const shownOpen = hasContent && (!collapsible || open)
  const duration = status === 'completed' && durationMs !== undefined && Number.isFinite(durationMs) && durationMs >= 0
    ? Math.max(1000, durationMs) : undefined
  const labelKey = status === 'failed' ? 'dshProcessFailed' : status === 'aborted' ? 'dshTurnStopped'
    : status !== 'completed' ? 'dshTurnProcess' : duration === undefined ? 'processed' : 'dshTurnTook'
  return { canCollapse, open: shownOpen, duration, labelKey,
    announcementKey: status === 'completed' ? 'processed' : labelKey }
}

export type RetryPhase = 'scheduled' | 'started' | 'cancelled' | 'unknown'
export function retryPhase(value: unknown): RetryPhase {
  return value === 'scheduled' || value === 'started' || value === 'cancelled' ? value : 'unknown'
}

export function finiteCounter(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined
}

/** Creation/item clocks and public text are never substitutes for execution facts. */
export function liveElapsed(startedAt: number | undefined, now: number): number | undefined {
  if (startedAt === undefined || !Number.isFinite(startedAt) || !Number.isFinite(now) || startedAt > now) return undefined
  return Math.max(1000, now - startedAt)
}
