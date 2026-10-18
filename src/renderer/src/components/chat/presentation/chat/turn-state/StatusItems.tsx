/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useEffect, useId, useState, type ReactNode } from 'react'
import { ChevronDown, ChevronRight } from '../../../../../design/AnalytixUiIcons'
import { DshApiOutlineRegular } from '../../DshToolIcons'
import { TextShimmer } from '../../TextShimmer'
import { finiteCounter, retryPhase, type RetryPhase, type Translate } from './model'
import css from './TurnState.module.css'

/** Only an actual projected summary supplies an expandable detail. */
export function CompactionItem({ title, summary, detail, state, open, onToggle }: {
  title: string; summary?: string; detail?: ReactNode; state: 'running' | 'success' | 'error' | 'unknown'; open: boolean; onToggle: () => void
}) {
  const bodyId = useId()
  const expandable = detail !== undefined && detail !== null
  const shownOpen = expandable && open
  return <div className={css.compactionRow} data-analytix-compaction data-state={state}>
    <span className="sr-only" role="status" aria-live="polite" aria-atomic="true">{title}</span>
    <button type="button" className={css.compactionButton} disabled={!expandable}
      aria-expanded={expandable ? shownOpen : undefined} aria-controls={expandable ? bodyId : undefined} onClick={onToggle}>
      <span className={css.compactionLeading} aria-hidden>
        <span className={css.compactionContextIcon}><DshApiOutlineRegular /></span>
        <span className={css.compactionDisclosureIcon}>{shownOpen ? <ChevronDown /> : <ChevronRight />}</span>
      </span>
      <TextShimmer active={state === 'running'} className={css.compactionTitle}>{title}</TextShimmer>
      {summary ? <><span className={css.compactionSep} aria-hidden /><span className={css.compactionSummary}>{summary}</span></> : null}
    </button>
    {shownOpen ? <div id={bodyId} className={css.compactionBody}>{detail}</div> : null}
  </div>
}

/** Countdown displays the Host deadline; reaching zero never changes the supplied phase. */
export function RetryItem({ message, code, attempt, maxAttempt, phase, delayMs, deadlineUnixMs, detail, t }: {
  message: string; code?: string; attempt?: number; maxAttempt?: number; phase?: RetryPhase; delayMs?: number; deadlineUnixMs?: number; detail?: ReactNode; t: Translate
}) {
  const state = retryPhase(phase)
  const current = finiteCounter(attempt), maximum = finiteCounter(maxAttempt), delay = finiteCounter(delayMs)
  const deadline = finiteCounter(deadlineUnixMs)
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (state !== 'scheduled' || deadline === undefined || deadline === 0) return
    setNow(Date.now())
    if (deadline <= Date.now()) return
    const timer = setInterval(() => {
      const next = Date.now()
      setNow(next)
      if (next >= deadline) clearInterval(timer)
    }, 1000)
    return () => clearInterval(timer)
  }, [state, deadline])
  const remainingMs = state === 'scheduled' && deadline !== undefined && deadline > 0
    ? Math.max(0, deadline - now) : undefined
  const label = t(state === 'scheduled' ? 'dshRetryScheduled' : state === 'started' ? 'dshRetryStarted'
    : state === 'cancelled' ? 'dshRetryCancelled' : 'dshRetryUnknown')
  return <details className={css.retryRow} data-analytix-retry data-retry-state={state} data-active={state === 'scheduled' || undefined}>
    <summary className={css.retrySummary}>
      <span className={css.retryText} role="status" aria-live="polite" aria-atomic="true">
        <TextShimmer active={state === 'scheduled'}>{label}</TextShimmer>
        {remainingMs === undefined ? null : <span data-retry-deadline-unix-ms={deadline}> · {t('dshSeconds', { seconds: Math.ceil(remainingMs / 1000) })}</span>}
        {current === undefined ? null : <span> · {t('dshRetryRequestAttempt', { attempt: current, max: maximum ?? '?' })}</span>}
      </span>
    </summary>
    <div className={css.retryDetails}>
      {message ? <p className={css.detailText}>{message}</p> : null}
      {code ? <code>{code}</code> : null}
      {delay === undefined ? null : <div><span className={css.retryDetailLabel}>{t('dshRetryDelay')} </span>{t('dshMilliseconds', { milliseconds: delay })}</div>}
      {detail}
    </div>
  </details>
}

/** Persistent public error feedback; the body is independent of tool I/O. */
export function TurnErrorItem({ title, message, code, detail, detailLabel, open, onToggle }: {
  title: string; message: string; code?: string; detail?: ReactNode; detailLabel: string; open: boolean; onToggle: () => void
}) {
  const bodyId = useId()
  return <div data-analytix-turn-error>
    <div className={css.turnErrorRow} role="status" aria-live="polite" aria-atomic="true">
      <span className={css.turnErrorDot} aria-hidden />
      <div className={css.turnErrorCopy}><span className={css.turnErrorTitle}>{title}</span><span className={css.turnErrorMessage}>{message}</span></div>
      {code ? <code className={css.turnErrorCode}>{code}</code> : null}
    </div>
    {detail ? <button type="button" className={css.detailButton} data-disclosure-row aria-expanded={open}
      aria-controls={bodyId} onClick={onToggle}><ChevronRight aria-hidden className={open ? css.detailChevronOpen : undefined} />{detailLabel}</button> : null}
    {open && detail ? <div id={bodyId} className={css.statusDetails}>{detail}</div> : null}
  </div>
}

/** Kept separate from errors. No current ChatBlock field proves this turn end. */
export function TurnMaxTokensItem({ t }: { t: Translate }) {
  return <div className={css.turnErrorRow} role="status" data-analytix-turn-max-tokens>
    <span className={`${css.turnErrorDot} ${css.warningDot}`} aria-hidden />
    <div className={css.turnErrorCopy}><span className={css.maxTokensTitle}>{t('dshMaxTokens')}</span>
      <span className={css.turnErrorMessage}>{t('dshMaxTokensHint')}</span></div>
  </div>
}
