/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useEffect, useState } from 'react'

export function useCalendarDay(enabled: boolean): number {
  const dayStart = (now: number): number => { const date = new Date(now); return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime() }
  const [day, setDay] = useState(() => dayStart(Date.now()))
  useEffect(() => {
    if (!enabled) return
    let timer: ReturnType<typeof setTimeout>
    const arm = (): void => {
      const now = Date.now(), date = new Date(now)
      setDay(dayStart(now))
      timer = setTimeout(arm, Math.max(1, new Date(date.getFullYear(), date.getMonth(), date.getDate() + 1).getTime() - now))
    }
    arm()
    return () => clearTimeout(timer)
  }, [enabled])
  return day
}

export function messageClock(time: string | undefined, day: number, t: (key: string, values?: Record<string, unknown>) => string): string | undefined {
  if (!time) return undefined
  const date = new Date(time), now = new Date(day)
  if (!Number.isFinite(date.getTime())) return undefined
  const clock = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
  if (date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate()) return clock
  const values = { y: date.getFullYear(), m: date.getMonth() + 1, d: date.getDate() }
  return `${t(date.getFullYear() === now.getFullYear() ? 'dshClockMonthDay' : 'dshClockYearMonthDay', values)} ${clock}`
}

