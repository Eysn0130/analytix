import { describe, expect, it } from 'vitest'
import { canonicalizeCronExpression, parseCronExpression, validateScheduleCalendarRule, normalizeScheduleWeekdays, isValidScheduleTimeZone } from './schedule-rule'
describe('Schedule calendar strict rule contract', () => {
  it('preserves star identity when canonicalizing cron fields', () => {
    for (const expression of ['0 9 */2 * 1', '0 9 1-31 * 1', '0 9 5,* * 1', '0 9 *,5 * 1']) {
      const before = parseCronExpression(expression), after = parseCronExpression(canonicalizeCronExpression(expression))
      expect(after).toEqual(before)
    }
    expect(parseCronExpression('0 9 1-31 * 1').dayOfMonthStar).toBe(false)
    expect(parseCronExpression('0 9 5,* * 1').dayOfMonthStar).toBe(false)
    expect(parseCronExpression('0 9 *,5 * 1').dayOfMonthStar).toBe(true)
  })
  it('rejects unsupported cron dialect, invalid ranges and unsafe steps', () => {
    for (const expression of ['@daily', '0 0 * * MON', '0 0 L * *', '0 0 * * 1#2', '0 0 * * * *', '60 0 * * *', '0 24 * * *', '0 0 * * 8', '0 0 5/2 * *', '0 0 5-1 * *', '*/0 * * * *', '0,,1 0 * * *', '*/9007199254740992 * * * *']) expect(() => parseCronExpression(expression)).toThrow()
  })
  it('requires a valid explicit zone and unique ISO weekday set', () => {
    for (const zone of ['', 'EST', '+08:00', 'Missing/Zone', ' UTC ']) expect(isValidScheduleTimeZone(zone)).toBe(false)
    expect(isValidScheduleTimeZone('America/New_York')).toBe(true)
    expect(normalizeScheduleWeekdays([7, 1, 5])).toEqual([1, 5, 7])
    for (const days of [[], [0], [8], [1, 1], ['1'], [1.5]]) expect(() => normalizeScheduleWeekdays(days)).toThrow()
    expect(() => validateScheduleCalendarRule({ kind: 'daily', timeOfDay: '09:00' })).toThrow()
    expect(() => validateScheduleCalendarRule({ kind: 'daily', timeOfDay: '09:00' }, true)).not.toThrow()
    expect(() => validateScheduleCalendarRule({ kind: 'daily', timeOfDay: '09:00', timeZone: '' }, true)).toThrow()
    expect(() => validateScheduleCalendarRule({ kind: 'manual', timeZone: 'UTC' })).toThrow()
  })
})
