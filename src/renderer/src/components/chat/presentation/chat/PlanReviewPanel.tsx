/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useMemo, type KeyboardEvent, type Ref } from 'react'
import { useTranslation } from 'react-i18next'
import { extractMarkdownPlainText } from '../../../../../../shared/markdown/plain-text'
import { DshChevronRightOutlineRegular, DshEditOutlineRegular } from '../DshToolIcons'
import type { questionPlanReview } from './question-plan-review'
import css from './PlanReviewPanel.module.css'

function syncSpinner(element: SVGSVGElement | null) {
  if (!element) return
  const pin = () => { for (const animation of element.getAnimations?.({ subtree: true }) ?? []) animation.startTime = 0 }
  pin(); element.addEventListener('animationstart', pin)
}

type Props = {
  review: NonNullable<ReturnType<typeof questionPlanReview>>
  requestId: string; sectionRef: Ref<HTMLElement>; busy: boolean; disabled: boolean; feedback: string
  onApprove: () => void; onDiscuss: () => void; onOpenPlan: () => void
  onKeyDown: (event: KeyboardEvent<HTMLElement>) => void
}
export function PlanReviewPanel({ review, requestId, sectionRef, busy, disabled, feedback, onApprove, onDiscuss, onOpenPlan, onKeyDown }: Props) {
  const { t } = useTranslation('common')
  const summary = useMemo(() => {
    const title = extractMarkdownPlainText(review.plan, { mode: 'first-line' })
    const description = extractMarkdownPlainText(review.plan, { mode: 'first-paragraph' })
    return { title, description: description === title ? '' : description }
  }, [review.plan])
  return <div className={css.frame} data-plan-review-key={requestId}>
    <section ref={sectionRef} className={css.card} data-protected-user-input data-gate-request-id={requestId}
      aria-label={review.question.question} aria-busy={busy} onKeyDown={onKeyDown}>
      <div className={css.strip}>
        {busy ? <svg ref={syncSpinner} className={css.spinner} data-state="ongoing" width={14} height={14} viewBox="0 0 24 24" aria-hidden="true">
          <g className={css.spinnerMotion}><circle className={css.spinnerTrack} cx="12" cy="12" r="9.5" /><circle className={css.spinnerArc} cx="12" cy="12" r="9.5" /></g>
        </svg> : <span className={css.dot} data-state="warning" aria-hidden="true" />}
        {t('userInputPlanHeader')}
        <div className={css.previewActions}><button type="button" className={css.reviewLink}
          aria-label={t('userInputPlanOpen')} title={t('userInputPlanOpen')} onClick={onOpenPlan}>
          {t('userInputPlanFull')}<DshChevronRightOutlineRegular size={14} />
        </button></div>
      </div>
      <div className={css.summary}><h3 className={css.title}>{summary.title}</h3>
        {summary.description !== '' ? <p className={css.description}>{summary.description}</p> : null}
      </div>
      <div className={css.footer}><div className={css.feedback} role={feedback ? 'alert' : 'status'}>{feedback}</div>
        <div className={css.actions}><button type="button" className={`${css.button} ${css.outline} ${css.discuss}`} disabled={busy} onClick={onDiscuss}>
          <span className={css.icon}><DshEditOutlineRegular size={14} /></span>{t('userInputPlanDiscuss')}
        </button><button type="button" className={`${css.button} ${css.primary}`} title={review.approve.description} disabled={disabled} onClick={onApprove}>{t('userInputPlanApprove')}</button></div>
      </div>
    </section>
  </div>
}
