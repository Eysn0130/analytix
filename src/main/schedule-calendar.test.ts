import { describe, expect, it } from 'vitest'
import { scheduleCalendarNextRunAt } from './schedule-calendar'
import { computeScheduleNextRunAt } from './schedule-runtime-helpers'
import type { ScheduledTaskScheduleV1, ScheduledTaskV1 } from '../shared/app-settings'

const rule = (extra: Partial<ScheduledTaskScheduleV1> = {}): ScheduledTaskScheduleV1 => ({ kind: 'daily', everyMinutes: 60, timeOfDay: '02:30', atTime: '', timeZone: 'America/New_York', ...extra })
const next = (schedule: ScheduledTaskScheduleV1, floor: string) => scheduleCalendarNextRunAt(schedule, new Date(floor))
describe('Schedule calendar recurrence', () => {
  it('skips a daily DST gap without shifting its wall-clock time', () => {
    expect(next(rule(), '2026-03-08T05:00:00Z')).toBe('2026-03-09T06:30:00.000Z')
  })
  it('uses the earlier overlap once, including a floor between its two instants', () => {
    const daily = rule({ timeOfDay: '01:30' })
    expect(next(daily, '2026-11-01T04:00:00Z')).toBe('2026-11-01T05:30:00.000Z')
    expect(next(daily, '2026-11-01T05:45:00Z')).toBe('2026-11-02T06:30:00.000Z')
  })
  it('uses the rule calendar for a half-hour gap and a skipped local day', () => {
    expect(next(rule({ timeZone: 'Australia/Lord_Howe', timeOfDay: '02:15' }), '2026-10-03T13:00:00Z')).toBe('2026-10-04T15:15:00.000Z')
    expect(next(rule({ timeZone: 'Pacific/Apia', timeOfDay: '09:00' }), '2011-12-30T00:00:00Z')).toBe('2011-12-30T19:00:00.000Z')
  })
  it('uses ISO weekdays and skips a weekly gap to the next selected date', () => {
    expect(next(rule({ kind: 'weekly', weekdays: [7] }), '2026-03-08T05:00:00Z')).toBe('2026-03-15T06:30:00.000Z')
    expect(next(rule({ kind: 'weekly', weekdays: [1, 2, 3, 4, 5], timeZone: 'UTC', timeOfDay: '09:00' }), '2026-01-02T10:00:00Z')).toBe('2026-01-05T09:00:00.000Z')
  })
  it('uses numeric cron local times with the same DST gap/overlap behavior', () => {
    expect(next(rule({ kind: 'cron', expression: '30 2 * * *' }), '2026-03-08T05:00:00Z')).toBe('2026-03-09T06:30:00.000Z')
    expect(next(rule({ kind: 'cron', expression: '30 1 * * *' }), '2026-11-01T05:45:00Z')).toBe('2026-11-02T06:30:00.000Z')
  })
  it('preserves Vixie DOM/DOW OR, star-step AND and Sunday 0/7', () => {
    const cron = (expression: string) => rule({ kind: 'cron', timeZone: 'UTC', expression })
    expect(next(cron('0 9 1 * 1'), '2026-01-01T10:00:00Z')).toBe('2026-01-05T09:00:00.000Z')
    expect(next(cron('0 9 */2 * 1'), '2026-01-05T10:00:00Z')).toBe('2026-01-19T09:00:00.000Z')
    expect(next(cron('0 9 * * 7'), '2026-01-03T10:00:00Z')).toBe('2026-01-04T09:00:00.000Z')
  })
  it('bounds valid but impossible cron rules and exhausted UTC years', () => {
    expect(() => next(rule({ kind: 'cron', timeZone: 'UTC', expression: '0 0 30 2 *' }), '2026-01-01T00:00:00Z')).toThrow('No future occurrence')
    expect(() => next(rule({ timeZone: 'UTC', timeOfDay: '09:00' }), '9999-12-31T23:59:59.999Z')).toThrow('No future occurrence')
  })
  it('keeps legacy daily host-local and manual/interval/at behavior', () => {
    const oldRule = { kind: 'daily' as const, everyMinutes: 60, timeOfDay: '09:00', atTime: '' }
    const from = new Date(2026, 0, 3, 10), expected = new Date(from); expected.setHours(9, 0, 0, 0); expected.setDate(expected.getDate() + 1)
    const task = { enabled: true, schedule: oldRule } as ScheduledTaskV1
    expect(computeScheduleNextRunAt(task, from)).toBe(expected.toISOString())
    expect(computeScheduleNextRunAt({ ...task, schedule: { ...oldRule, kind: 'manual' } }, from)).toBe('')
    expect(computeScheduleNextRunAt({ ...task, schedule: { ...oldRule, kind: 'interval', everyMinutes: 5 } }, from)).toBe(new Date(from.getTime() + 300000).toISOString())
    expect(computeScheduleNextRunAt({ ...task, schedule: { ...oldRule, kind: 'at', atTime: '2026-02-01T00:00:00Z' } }, from)).toBe('2026-02-01T00:00:00Z')
  })
})
