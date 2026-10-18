import { describe, expect, it } from 'vitest'
import { authoritativeTurnStatuses, authoritativeTurnTimings, settledDuration } from './turn-timing'

describe('existing Go execution facts for central presentation', () => {
  it('settles only valid terminal start/end pairs and retains the real status independently', () => {
    const turns = [
      { id: 'completed', status: 'completed', startedAt: '2026-10-06T08:00:00Z', finishedAt: '2026-10-06T08:00:03.200Z', items: [{ id: 'u', kind: 'user_message' }] },
      { id: 'failed', status: 'failed', startedAt: '2026-10-06T08:00:00Z', finishedAt: '2026-10-06T08:00:01Z' },
      { id: 'aborted', status: 'aborted' },
      { id: 'running', status: 'running', startedAt: '2026-10-06T08:00:00Z', finishedAt: '2026-10-06T08:00:01Z' },
      { id: 'reversed', status: 'completed', startedAt: '2026-10-06T08:00:03Z', finishedAt: '2026-10-06T08:00:00Z' },
      { id: 'invalid', status: 'completed', startedAt: 'invalid', finishedAt: '2026-10-06T08:00:01Z' },
      { id: 'creation-only', status: 'completed', createdAt: '2026-10-06T08:00:00Z', finishedAt: '2026-10-06T08:00:01Z' },
      { id: 'unknown', status: 'unrecognized' }
    ]
    const timings = authoritativeTurnTimings(turns)
    expect(Object.keys(timings)).toEqual(['completed', 'failed'])
    expect(timings.completed.userItemId).toBe('u')
    expect(settledDuration(timings.completed)).toBe(3200)
    expect(settledDuration(timings.failed)).toBe(1000)
    expect(authoritativeTurnStatuses(turns)).toMatchObject({ aborted: 'aborted', running: 'running' })
    expect(authoritativeTurnStatuses(turns).unknown).toBeUndefined()
  })
  it('leaves unavailable or malformed final duration unknown', () => {
    expect(settledDuration(undefined)).toBeUndefined()
    expect(settledDuration({ turnId: 't', startedAt: 200, finishedAt: 100 })).toBeUndefined()
    expect(settledDuration({ turnId: 't', startedAt: NaN, finishedAt: 100 })).toBeUndefined()
  })
})
