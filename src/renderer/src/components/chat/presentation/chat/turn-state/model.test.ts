import { describe, expect, it } from 'vitest'
import { finiteCounter, formatRunDuration, liveElapsed, retryPhase, turnProcessPresentation } from './model'

const units: Record<string, string> = { dshDurationHourUnit: 'h ', dshDurationMinuteUnit: 'm ', dshDurationSecondUnit: 's' }
const t = (key: string) => units[key]
const label = (ms: number) => formatRunDuration(ms, t).map(part => part.text).join('')

describe('pinned Turn state pure facts', () => {
  it.each([
    [-1, '0s'], [0, '0s'], [999, '0s'], [1000, '1s'], [3200, '3s'], [59_999, '59s'],
    [60_000, '1m 0s'], [3_599_999, '59m 59s'], [3_600_000, '1h 0m 0s'], [86_401_000, '24h 0m 1s']
  ])('floors localized full h/m/s for %s without applying the owner minimum', (ms, expected) => {
    expect(label(ms)).toBe(expected)
    expect(formatRunDuration(ms, t).filter(part => part.numeric).every(part => /^\d+$/.test(part.text))).toBe(true)
  })
  it.each([NaN, Infinity, -Infinity])('never formats nonfinite duration %s', ms => {
    expect(formatRunDuration(ms, t)).toEqual([])
  })
  it.each(['failed', 'aborted'] as const)('preserves the owning %s expansion policy and suppresses duration', status => {
    const view = turnProcessPresentation({ status, open: false, hasContent: true, collapsible: false, durationMs: 3200 })
    expect(view).toMatchObject({ open: true, canCollapse: false, duration: undefined,
      labelKey: status === 'failed' ? 'dshProcessFailed' : 'dshTurnStopped' })
  })
  it('uses a terminal-only one-second minimum while preserving nonfoldable/interleaved ownership', () => {
    expect(turnProcessPresentation({ status: 'completed', durationMs: 0, open: false, hasContent: true }).duration).toBe(1000)
    expect(turnProcessPresentation({ status: 'completed', open: false, hasContent: true, collapsible: false })).toMatchObject({ open: true, canCollapse: false })
    expect(turnProcessPresentation({ open: true, hasContent: false })).toMatchObject({ open: false, canCollapse: false })
  })
  it.each([undefined, NaN, Infinity, -1])('keeps invalid/missing settled duration %s unknown', durationMs => {
    expect(turnProcessPresentation({ status: 'completed', durationMs, open: false, hasContent: true })).toMatchObject({ duration: undefined, labelKey: 'processed' })
  })
  it('never infers completion from an elapsed value', () => {
    expect(turnProcessPresentation({ durationMs: 3200, open: false, hasContent: true })).toMatchObject({ duration: undefined, labelKey: 'dshTurnProcess' })
  })
  it.each(['scheduled', 'started', 'cancelled'] as const)('recognizes only supplied retry phase %s', phase => { expect(retryPhase(phase)).toBe(phase) })
  it.each([undefined, 'retrying', 'completed', {}, 1])('keeps unknown retry phase honest: %s', phase => { expect(retryPhase(phase)).toBe('unknown') })
  it.each([NaN, Infinity, -1, 1.5, '1', undefined])('rejects malformed count %s', value => { expect(finiteCounter(value)).toBeUndefined() })
  it('preserves real zero counts and validates starts without inventing a historical start', () => {
    expect(finiteCounter(0)).toBe(0)
    expect(liveElapsed(undefined, 10_000)).toBeUndefined()
    expect(liveElapsed(NaN, 10_000)).toBeUndefined()
    expect(liveElapsed(Infinity, 10_000)).toBeUndefined()
    expect(liveElapsed(11_000, 10_000)).toBeUndefined()
    expect(liveElapsed(10_000, 10_000)).toBe(1000)
    expect(liveElapsed(0, 3200)).toBe(3200)
  })
})
