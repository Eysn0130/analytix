/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { useId } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { MessageFeedbackCategoryV1Schema } from '../../../../../../../packages/runtime/src/contracts/message-feedback'
import { useModalFocus } from '../../../../hooks/use-modal-focus'
import type { MessageFeedbackController, MessageFeedbackDialogState, MessageFeedbackError } from './message-feedback-client'
import styles from './MessageFeedbackDialog.module.css'

export function MessageFeedbackDialog({ controller, dialog, pending, error }: {
  controller: MessageFeedbackController; dialog: MessageFeedbackDialogState; pending: boolean; error: MessageFeedbackError | null
}) {
  const { t } = useTranslation('common')
  const titleId = useId(), detailId = useId(), errorId = useId()
  const focus = useModalFocus(controller.close, !pending, true, dialog.opener)
  // Reuse the existing chat presentation token scope across the body portal.
  return createPortal(<div className={styles.backdrop} data-analytix-chat-view data-analytix-message-feedback-overlay onMouseDown={event => {
    if (event.target === event.currentTarget) controller.close()
  }}>
    <form ref={focus} className={styles.dialog} role="dialog" aria-modal="true" aria-labelledby={titleId}
      tabIndex={-1} onSubmit={event => { event.preventDefault(); void controller.submit() }}>
      <header className={styles.header}><h2 id={titleId}>{t('messageFeedback.title')}</h2>
        <button type="button" className={styles.close} disabled={pending} aria-label={t('messageFeedback.close')}
          onClick={controller.close}>×</button></header>
      <div>
        <div className={styles.categories} role="group" aria-label={t('messageFeedback.categories')}>
          {MessageFeedbackCategoryV1Schema.options.map(category => <button key={category} type="button" disabled={pending}
            aria-pressed={dialog.category === category} data-active={dialog.category === category || undefined}
            className={styles.chip} onClick={() => controller.edit({ category: dialog.category === category ? undefined : category })}>
            {t(`messageFeedback.category.${category}`)}</button>)}
        </div>
        <textarea id={detailId} data-modal-autofocus className={styles.detail} disabled={pending}
          aria-label={t('messageFeedback.detail')} aria-describedby={error ? errorId : undefined}
          placeholder={t('messageFeedback.hint')} value={dialog.note} onChange={event => controller.edit({ note: event.target.value })} />
        {error ? <p id={errorId} className={styles.error} role="alert">{t(`messageFeedback.error.${error}`)}</p> : null}
      </div>
      <button type="submit" className={styles.submit} disabled={pending}>{t(pending ? 'messageFeedback.submitting' : 'messageFeedback.submit')}</button>
    </form>
  </div>, document.body)
}
