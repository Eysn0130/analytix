/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { memo, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TextShimmer } from '../../TextShimmer'
import { formatRunDuration, liveElapsed, LIVE_RUN_CLOCK_INTERVAL_MS } from './model'
import { RunningWhaleTail } from './RunningWhaleTail'
import css from './TurnState.module.css'

/** Mounted only for the current session’s admitted running authority; no history clock. */
export const RunningStatus = memo(function RunningStatus({ startedAt, hasOutput }: { startedAt?: number; hasOutput: boolean }) {
  const { t } = useTranslation('common')
  const [clock, setClock] = useState(() => ({ startedAt, now: Date.now() }))
  const now = clock.startedAt === startedAt ? clock.now : Date.now()
  useEffect(() => {
    if (startedAt === undefined || !Number.isFinite(startedAt) || startedAt > Date.now()) return
    setClock({ startedAt, now: Date.now() })
    const timer = window.setInterval(() => setClock({ startedAt, now: Date.now() }), LIVE_RUN_CLOCK_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [startedAt])
  const elapsed = liveElapsed(startedAt, now)
  const label = elapsed === undefined ? t('dshRunning') : t('dshRunningFor', { duration: formatRunDuration(elapsed, t).map(part => part.text).join('') })
  return <div className={css.running} data-analytix-live-progress>
    <span role="status" aria-live="polite" aria-atomic="true" className="sr-only">{t('dshRunning')}</span>
    {hasOutput ? <span className={css.runningDivider} aria-hidden /> : null}
    <span className={css.runningContent} aria-live={elapsed === undefined ? undefined : 'off'} title={elapsed === undefined ? undefined : t('dshRunningEstimate')}>
      <RunningWhaleTail /><TextShimmer active className={css.runningText}>{label}</TextShimmer>
    </span>
  </div>
})
