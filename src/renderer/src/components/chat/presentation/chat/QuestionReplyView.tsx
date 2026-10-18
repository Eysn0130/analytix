/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../../../agent/types'
import type { UserInputLocalDisplayV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { useUserInputProtectedTransfer, useUserInputLocalDisplay, useUserInputLocalAssociation } from './useUserInputLocalDisplay'
import { DshChevronDownOutlineRegular, DshChevronRightOutlineRegular } from '../DshToolIcons'
import { Tooltip } from '../Tooltip'
import { DshCopyOutlineRegular, DshCheckOutlineRegular } from '../DshToolIcons'
import css from './QuestionReplyView.module.css'
type Input = Extract<ChatBlock, { kind: 'user_input' }>
type User = Extract<ChatBlock, { kind: 'user' }>
export function userInputReplyMatches(display: UserInputLocalDisplayV1 | null, block: User): boolean {
  const reply = display?.historyReply ?? (display?.lateReply?.state === 'accepted' ? display.lateReply : null)
  return Boolean(display?.callId && display.submission && reply &&
    block.id && (!display.historyReply || display.historyReply.itemId === block.id) && block.meta?.steeringStatus === 'accepted' &&
    block.meta.clientUserMessageId === reply.clientUserMessageId && block.meta.turnId === reply.turnId)
}
function answerValues(display: UserInputLocalDisplayV1, questionId: string): string[] {
  if (display.submission?.format === 'legacy') {
    const answer = display.submission.answers.find(item => item.questionId === questionId)
    return answer ? [answer.value] : []
  }
  if (display.submission?.format !== 'structured') return []
  const answer = display.submission.answers.find(answer => answer.id === questionId)
  const question = display.questions.find(question => question.id === questionId)
  if (!question || !answer || answer.kind === 'skip') return []
  return [...answer.optionIds.map(id => question.options.find(option => option.id === id)!.label), ...(answer.customText?.trim() ? [answer.customText.trim()] : [])]
}
function Candidate({ input, block, threadId, onMatch }: { input: Input; block: User; threadId: string; onMatch: (id: string, match: boolean) => void }) {
  const ids = useUserInputLocalAssociation(typeof input.meta?.turnId === 'string' ? { threadId, turnId: input.meta.turnId, itemId: input.id, requestId: input.requestId } : null, input.status,
    block.meta?.steeringStatus === 'accepted' && typeof block.meta.turnId === 'string' && typeof block.meta.clientUserMessageId === 'string'
      ? [{ kind: 'reply', id: block.id, turnId: block.meta.turnId, clientUserMessageId: block.meta.clientUserMessageId }] : [])
  const matches = ids.includes(block.id)
  useLayoutEffect(() => { onMatch(input.id, matches); return () => onMatch(input.id, false) }, [input.id, matches, onMatch])
  return null
}
function ReplyBody({ input, block, threadId, fallback }: { input: Input; block: User; threadId: string; fallback: ReactNode }) {
  const { t, i18n } = useTranslation('common')
  const local = useUserInputLocalDisplay(typeof input.meta?.turnId === 'string' ? { threadId, turnId: input.meta.turnId, itemId: input.id, requestId: input.requestId } : null, input.status)
  const display = local.display, matches = userInputReplyMatches(display, block)
  const [open, setOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    setCopied(false)
    return () => { if (copyTimer.current !== null) clearTimeout(copyTimer.current) }
  }, [local.revision])
  const copy = () => {
    const reply = display?.historyReply ?? display?.lateReply
    if (copied || !matches || !reply) return
    void local.effect({ kind: 'copyReply', locale: i18n.language.startsWith('zh') ? 'zh' : 'en', clientUserMessageId: reply.clientUserMessageId, expectedTurnId: reply.turnId }).then(ok => {
      if (!ok) return
      setCopied(true); copyTimer.current = setTimeout(() => setCopied(false), 1000)
    })
  }
  const rootRef = useRef<HTMLDivElement>(null)
  useUserInputProtectedTransfer(rootRef, Boolean(matches && display), local.canTransfer)
  if (!matches || !display) return <>{fallback}</>
  // Original local transcript remains leaf-owned. Named Copy derives its
  // exact contents in native Main; no raw public metadata or decorative twin.
  return <div ref={rootRef} className={css.row} role="group" aria-label={t('userInputReplyLabel')} data-reply-outcome="answered" data-question-reply={display.callId} data-question-reply-item-id={block.id} data-protected-user-input>
    <div className={css.bubble}><button type="button" className={css.toggle} aria-label={`${t('userInputReplyLabel')} · ${t(open ? 'userInputReplyCollapse' : 'userInputReplyExpand')}`} aria-expanded={open} onClick={() => setOpen(value => !value)}>
      <span className={css.labelRow}><span className={css.caret}>{open ? <DshChevronDownOutlineRegular /> : <DshChevronRightOutlineRegular />}</span><span className={css.label}>{t('userInputReplyLabel')}</span></span>
      {!open ? <span className={css.summary}>{display.questions.flatMap(question => answerValues(display, question.id)).join(', ') || t('userInputSkipped')}</span> : null}
    </button>
    {open ? <dl className={css.details}>{display.questions.map(question => <div key={question.id}>
      <dt className={css.question}>{question.header && question.header !== question.question ? <span className={css.header}>{question.header}</span> : null}<span className={css.questionText}>{question.question}</span>
        {question.detail ? <span className={css.detail}>{question.detail}</span> : null}
        {question.options.length ? <ul className={css.options}>{question.options.map(option => <li key={option.id}>{option.label}{option.description ? ' — ' + option.description : ''}</li>)}</ul> : null}
      </dt><dd className={css.answer}><span className={css.answerLabel}>{t('userInputReplyAnswer')}</span>{answerValues(display, question.id).join(', ') || t('userInputSkipped')}</dd>
    </div>)}</dl> : null}</div>
    <div className={css.actions}>
      {block.createdAt && Number.isFinite(Date.parse(block.createdAt)) ? <span className={css.time}>{new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(block.createdAt))}</span> : null}
      <Tooltip label={t(copied ? 'userInputCopied' : 'userInputCopy')} side="bottom" portal><button type="button" className={css.action} aria-label={t(copied ? 'userInputCopied' : 'userInputCopy')} onClick={copy}>
        {copied ? <DshCheckOutlineRegular /> : <DshCopyOutlineRegular />}
      </button></Tooltip>
    </div>
  </div>
}
export function QuestionReplyView({ block, requests, threadId, fallback }: { block: User; requests: Input[]; threadId: string; fallback: ReactNode }) {
  const [matches, setMatches] = useState<Record<string, true>>({})
  const [onMatch] = useState(() => (id: string, match: boolean) => setMatches(previous => {
    if (Boolean(previous[id]) === match) return previous
    const next = { ...previous }; if (match) next[id] = true; else delete next[id]; return next
  }))
  const matched = requests.filter(input => matches[input.id])
  const sole = matched.length === 1 ? matched[0] : null
  return <>{requests.map(input => <Candidate key={input.id + ':' + block.id} input={input} block={block} threadId={threadId} onMatch={onMatch} />)}{sole ? <ReplyBody key={sole.id + ':' + block.id} input={sole} block={block} threadId={threadId} fallback={fallback} /> : fallback}</>
}
