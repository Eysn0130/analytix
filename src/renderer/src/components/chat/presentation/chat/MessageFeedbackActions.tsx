/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Tooltip } from '../Tooltip'
import { messageFeedbackController, type MessageFeedbackError } from './message-feedback-client'
import { MessageFeedbackDialog } from './MessageFeedbackDialog'
import { MessageFeedbackIcon, MessageFeedbackNoticeIcon } from './MessageFeedbackIcons'
import styles from './MessageFeedbackActions.module.css'

export function MessageFeedbackActions({ threadId, messageId }: { threadId: string; messageId: string }) {
  const { t } = useTranslation('common')
  const controller = useMemo(() => messageFeedbackController(threadId), [threadId])
  const owner = useRef(Symbol('message-feedback-row')).current
  const subscribe = useMemo(() => (listener: () => void) => controller.subscribe(listener, owner), [controller, owner])
  const state = useSyncExternalStore(subscribe, controller.getSnapshot, controller.getSnapshot)
  const host = useRef<HTMLSpanElement>(null)
  const [toast, setToast] = useState<{ x: number; kind: 'recorded' | MessageFeedbackError } | null>(null)
  const [fading, setFading] = useState(false)
  const [choosing, setChoosing] = useState(false)
  const current = state.items[messageId]
  const acknowledgement = state.acknowledgement?.owner === owner ? state.acknowledgement : null
  const error = state.error && (!state.errorMessageId || state.errorMessageId === messageId) &&
    (!state.errorOwner || state.errorOwner === owner) ? state.error : null
  const dialogFailure = state.dialog?.owner === owner ? error : null
  useEffect(() => {
    if (!acknowledgement && !dialogFailure) { setToast(null); return }
    const column = host.current?.closest('[data-analytix-conversation-body]')
    const composer = column?.querySelector('.ds-floating-composer')
    const bounds = (composer ?? host.current?.closest('[data-analytix-chat-view]'))?.getBoundingClientRect()
    setToast({ x: bounds ? bounds.left + bounds.width / 2 : window.innerWidth / 2,
      kind: dialogFailure ?? 'recorded' })
    setFading(false)
    const hold = dialogFailure ? 6000 : 3000
    const fade = setTimeout(() => setFading(true), hold)
    const close = setTimeout(() => { setToast(null); if (acknowledgement) controller.clearAcknowledgement(owner, acknowledgement.serial) }, hold + 1000)
    return () => { clearTimeout(fade); clearTimeout(close) }
  }, [acknowledgement, dialogFailure, controller, owner])
  return <span ref={host} className={styles.host} data-message-feedback={messageId}
    onMouseEnter={() => { void controller.ensure() }} onFocus={() => { void controller.ensure() }}>
    {(['positive', 'negative'] as const).map(rating => {
      const active = current?.rating === rating
      const label = t(active ? 'messageFeedback.remove' : rating === 'positive' ? 'messageFeedback.like' : 'messageFeedback.dislike')
      return <Tooltip key={rating} label={label} side="bottom"><button type="button" className={styles.action}
        disabled={state.pending || choosing} aria-label={label} aria-pressed={active}
        data-active={active || undefined} onClick={async event => {
          const opener = event.currentTarget
          setChoosing(true)
          try { await controller.rate(messageId, rating, opener, owner) } finally { setChoosing(false) }
        }}>
        <MessageFeedbackIcon negative={rating === 'negative'} filled={active} /></button></Tooltip>
    })}
    {error && !state.dialog ? <span className={styles.failure} role="alert">{t(`messageFeedback.error.${error}`)}</span> : null}
    {state.dialog?.owner === owner ? <MessageFeedbackDialog key={messageId} controller={controller}
      dialog={state.dialog} pending={state.pending} error={error} /> : null}
    {toast ? createPortal(<div className={styles.toast} data-fading={fading || undefined} role={toast.kind === 'recorded' ? 'status' : 'alert'}
      style={{ left: toast.x }}><span className={styles.noticeIcon} data-success={toast.kind === 'recorded' || undefined}>
        <MessageFeedbackNoticeIcon success={toast.kind === 'recorded'} /></span>
      {t(toast.kind === 'recorded' ? 'messageFeedback.recorded' : `messageFeedback.error.${toast.kind}`)}</div>, document.body) : null}
  </span>
}
