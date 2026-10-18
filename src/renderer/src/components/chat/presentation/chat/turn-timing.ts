import type { AuthoritativeTurnStatus, AuthoritativeTurnStatuses, AuthoritativeTurnTiming, AuthoritativeTurnTimings } from '../../../../agent/types'

/** Renderer-only projection of terminal Go timestamps. Creation/item times are
 * deliberately not substitutes for a turn's execution start or settlement. */
export function authoritativeTurnTimings(turns: readonly {
  id: string; status: string; startedAt?: string; finishedAt?: string
  items?: readonly { id: string; kind: string }[]
}[]): AuthoritativeTurnTimings {
  const timings: AuthoritativeTurnTimings = {}
  for (const turn of turns) {
    if (!['completed', 'failed', 'aborted'].includes(turn.status)) continue
    if (!turn.startedAt || !turn.finishedAt) continue
    const startedAt = Date.parse(turn.startedAt)
    const finishedAt = Date.parse(turn.finishedAt)
    if (!Number.isFinite(startedAt) || !Number.isFinite(finishedAt) || finishedAt < startedAt) continue
    timings[turn.id] = { turnId: turn.id, startedAt, finishedAt,
      userItemId: turn.items?.find(item => item.kind === 'user_message')?.id }
  }
  return timings
}

export function settledDuration(timing: AuthoritativeTurnTiming | undefined): number | undefined {
  if (!timing || !Number.isFinite(timing.startedAt) || !Number.isFinite(timing.finishedAt) ||
    timing.finishedAt < timing.startedAt) return undefined
  return timing.finishedAt - timing.startedAt
}

/** Status is already public in the existing validated Go detail response. */
export function authoritativeTurnStatuses(turns: readonly { id: string; status: string }[]): AuthoritativeTurnStatuses {
  const statuses: AuthoritativeTurnStatuses = {}
  for (const turn of turns) {
    if (['queued', 'running', 'completed', 'failed', 'aborted'].includes(turn.status)) {
      statuses[turn.id] = turn.status as AuthoritativeTurnStatus
    }
  }
  return statuses
}
