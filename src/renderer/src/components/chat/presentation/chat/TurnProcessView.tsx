/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import { DshChevronDownOutlineRegular } from '../DshToolIcons'
import { formatRunDuration, turnProcessPresentation, type TurnProcessStatus } from './turn-state/model'
import css from './turn-state/TurnState.module.css'

/** Controlled Turn owner chrome, independent of tool-call rows. */
export function TurnProcessView({ open, onToggle, hasContent, durationMs, status, collapsible = true }: {
  open: boolean; onToggle: () => void; hasContent: boolean; durationMs?: number; collapsible?: boolean
  status?: TurnProcessStatus
}) {
  const { t } = useTranslation('common')
  const view = turnProcessPresentation({ open, hasContent, durationMs, status, collapsible })
  const content = <><span className={css.processLabel}>{t(view.labelKey)}
    {view.duration === undefined ? null : formatRunDuration(view.duration, t).map((part, index) =>
      <span key={index} className={part.numeric ? css.durationNumber : undefined}>{part.text}</span>)}
  </span>{view.canCollapse ? <DshChevronDownOutlineRegular className={css.processChevron} /> : null}</>
  return <>
    {status ? <span className="sr-only" role="status" aria-live="polite" aria-atomic="true">{t(view.announcementKey)}</span> : null}
    <button type="button" className={css.process} data-open={view.open || undefined} disabled={!view.canCollapse}
      data-analytix-turn-process aria-expanded={hasContent ? view.open : undefined}
      onClick={event => { if (view.canCollapse) { event.currentTarget.focus(); onToggle() } }}>
      {content}</button>
  </>
}
