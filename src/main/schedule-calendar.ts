/* Pure calendar target calculation adapted from DeepSeek Harness
 * packages/schedule/schedule/src/domain.ts at 5badb15009ae1756c3afe0ae0cef1faafc290ccc
 * (blob 28748ace0aa02b61de09987d3543890761f0a47a).
 * Copyright (c) 2026 DeepSeek. MIT; complete notice: THIRD_PARTY_NOTICES.md.
 * Analytix changes: detached pure validators, native four-field compatibility,
 * HH:mm compatibility and no Session/store/runtime dependency; Temporal stays in Main.
 */

import { Temporal } from '@js-temporal/polyfill'
import { ScheduleInputError, parseCronExpression, validateScheduleCalendarRule, type ParsedCronExpression } from '../../packages/runtime/src/contracts/schedule-rule'
import type { ScheduledTaskScheduleV1 } from '../shared/app-settings'

const MIN_FOUR_DIGIT_YEAR_MS = Date.parse('0001-01-01T00:00:00.000Z')
const MAX_FOUR_DIGIT_YEAR_MS = Date.parse('9999-12-31T23:59:59.999Z')
const CRON_SEARCH_HORIZON_YEARS = 400
/** Resolve only the earlier overlap instant; a gap fails the local-field round trip. */
function localInstant(local: Temporal.PlainDateTime, timeZone: string): number | undefined {
  const zoned = local.toZonedDateTime(timeZone, { disambiguation: 'earlier' })
  return zoned.toPlainDateTime().equals(local) ? zoned.epochMilliseconds : undefined
}

/** Project an explicit instant into a calendar date in the rule's zone. */
function localDate(epoch: number, timeZone: string): Temporal.PlainDate {
  return Temporal.Instant.fromEpochMilliseconds(epoch).toZonedDateTimeISO(timeZone).toPlainDate()
}

/** Recognize one of the rule's explicit ISO weekdays on a local calendar date. */
type WeekdayMatch = (date: Temporal.PlainDate) => boolean

/** Skip every local date; used by a rule that selects by time alone. */
const EVERY_DATE: WeekdayMatch = () => true

/** Select the explicit ISO weekdays of one normalized weekly rule. */
function weekdaySet(weekdays: readonly number[]): WeekdayMatch {
  const selected = new Set(weekdays)
  return date => selected.has(date.dayOfWeek)
}

/** Find the first actual occurrence after now and, when supplied, after a delivered date. */
function nextMatchingTarget(
  time: Temporal.PlainTime,
  timeZone: string,
  now: number,
  matches: WeekdayMatch,
  afterDate?: Temporal.PlainDate,
): string | undefined {
  let date = localDate(now, timeZone)
  const lastDate = localDate(MAX_FOUR_DIGIT_YEAR_MS, 'UTC').add({ days: 1 })
  if (afterDate !== undefined && Temporal.PlainDate.compare(date, afterDate) <= 0) {
    date = afterDate.add({ days: 1 })
  }
  for (; Temporal.PlainDate.compare(date, lastDate) <= 0; date = date.add({ days: 1 })) {
    if (!matches(date)) continue
    const target = localInstant(date.toPlainDateTime(time), timeZone)
    if (target === undefined) continue
    if (target > MAX_FOUR_DIGIT_YEAR_MS) return undefined
    if (target >= MIN_FOUR_DIGIT_YEAR_MS && target > now) return new Date(target).toISOString()
  }
  return undefined
}

/**
 * Build the local-date predicate of one parsed cron rule under Vixie day-of-month/day-of-week semantics.
 *
 * The star flag selects the branch only; it never excuses a field's matched values.
 * When either field text starts with `*`, a local date must satisfy BOTH fields, so a
 * stepped star such as `*` followed by `/2` still restricts the dates it matches. A bare
 * `*` matches every value, which makes that field's match always true and degrades the
 * AND to the other field, exactly like Vixie's `DOM_STAR`/`DOW_STAR` test. Only when
 * neither field text starts with `*` does either field matching suffice.
 * @param parsed - Parsed cron rule with canonical field text and matched values.
 * @returns Whether one local date matches the rule.
 */
function cronDateMatch(parsed: ParsedCronExpression): WeekdayMatch {
  const months = new Set(parsed.months)
  const daysOfMonth = new Set(parsed.daysOfMonth)
  const daysOfWeek = new Set(parsed.daysOfWeek)
  const dayOfMonthRestricted = !parsed.dayOfMonthStar
  const dayOfWeekRestricted = !parsed.dayOfWeekStar
  return (date) => {
    if (!months.has(date.month)) return false
    const dayOfMonth = daysOfMonth.has(date.day)
    const dayOfWeek = daysOfWeek.has(date.dayOfWeek % 7)
    if (dayOfMonthRestricted && dayOfWeekRestricted) return dayOfMonth || dayOfWeek
    return dayOfMonth && dayOfWeek
  }
}

/** Enumerate one parsed cron rule's local times of day in ascending wall-clock order. */
function cronTimes(parsed: ParsedCronExpression): readonly Temporal.PlainTime[] {
  const times: Temporal.PlainTime[] = []
  for (const hour of parsed.hours) {
    for (const minute of parsed.minutes) times.push(Temporal.PlainTime.from({ hour, minute }))
  }
  return times
}

/**
 * Find the first strictly future cron occurrence without leaving the four-digit UTC year range.
 *
 * The walk stops at {@link CRON_SEARCH_HORIZON_YEARS} past the floor date when that
 * horizon precedes the four-digit ceiling: a rule with no match inside the horizon
 * never matches, so its search resolves to exhaustion instead of scanning to the ceiling.
 * @param parsed - Parsed cron rule.
 * @param timeZone - Explicit zone interpreting its local date and time.
 * @param floor - Instant every returned target must exceed.
 * @returns The first matching target, or exhaustion as undefined.
 */
function nextCronTarget(parsed: ParsedCronExpression, timeZone: string, floor: number): string | undefined {
  const times = cronTimes(parsed)
  const matches = cronDateMatch(parsed)
  const months = new Set(parsed.months)
  const firstDate = localDate(floor, timeZone)
  let date = firstDate
  // The floor's own local wall clock, so the first date converts only candidates
  // that can exceed it instead of its whole day of times.
  const floorTime = Temporal.Instant.fromEpochMilliseconds(floor).toZonedDateTimeISO(timeZone).toPlainTime()
  const ceiling = localDate(MAX_FOUR_DIGIT_YEAR_MS, 'UTC').add({ days: 1 })
  const horizon = date.add({ years: CRON_SEARCH_HORIZON_YEARS })
  const lastDate = Temporal.PlainDate.compare(horizon, ceiling) < 0 ? horizon : ceiling
  while (Temporal.PlainDate.compare(date, lastDate) <= 0) {
    if (!months.has(date.month)) {
      date = date.add({ months: 1 }).with({ day: 1 })
      continue
    }
    if (matches(date)) {
      for (const time of times) {
        if (date.equals(firstDate) && Temporal.PlainTime.compare(time, floorTime) < 0) continue
        const target = localInstant(date.toPlainDateTime(time), timeZone)
        if (target === undefined) continue
        if (target > MAX_FOUR_DIGIT_YEAR_MS) return undefined
        if (target >= MIN_FOUR_DIGIT_YEAR_MS && target > floor) return new Date(target).toISOString()
      }
    }
    date = date.add({ days: 1 })
  }
  return undefined
}

/** New explicit-zone rules only; callers retain the old host-local daily branch. */
export function scheduleCalendarNextRunAt(schedule: ScheduledTaskScheduleV1, from: Date): string {
  validateScheduleCalendarRule(schedule)
  const floor = from.getTime()
  if (!Number.isSafeInteger(floor) || floor < MIN_FOUR_DIGIT_YEAR_MS || floor > MAX_FOUR_DIGIT_YEAR_MS) {
    throw new ScheduleInputError('time_out_of_range', 'The scheduling instant must have a four-digit UTC year.')
  }
  const timeZone = schedule.timeZone!
  const target = schedule.kind === 'cron'
    ? nextCronTarget(parseCronExpression(schedule.expression!), timeZone, floor)
    : nextMatchingTarget(Temporal.PlainTime.from(schedule.timeOfDay), timeZone, floor,
      schedule.kind === 'weekly' ? weekdaySet(schedule.weekdays!) : EVERY_DATE)
  if (!target) throw new ScheduleInputError('time_out_of_range', 'No future occurrence is representable as a four-digit-year UTC instant.')
  return target
}
