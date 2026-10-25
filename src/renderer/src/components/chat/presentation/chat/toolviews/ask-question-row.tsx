/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { createContext, useContext, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../../../../agent/types'
import { useChatStore } from '../../../../../store/chat-store'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { GateResponseContext } from '../../../message-timeline-bubbles'
import { DshCloseOutlineRegular, DshQuestionOutlineRegular } from '../../DshToolIcons'
import { ToolRow } from '../ToolRow'
import { GenericToolCard } from '../GenericToolCard'
import type { ToolViewProps } from '../ToolViews'
import { useUserInputProtectedTransfer, useUserInputLocalDisplay, useUserInputLocalAssociation } from '../useUserInputLocalDisplay'
import css from './ask-question-row.module.css'

type Input = Extract<ChatBlock, { kind: 'user_input' }>
export const QuestionAssociationContext = createContext<Record<string, string>>({})
export function QuestionAssociationLeaf({ input, threadId, anchors, onMatch }: { input: Input; threadId: string; anchors: ChatBlock[]; onMatch: (id: string, toolId: string | null) => void }) {
  const targets = anchors.flatMap(block => block.kind === 'tool' && block.toolKind === 'tool_call' &&
    typeof block.meta?.callId === 'string' && typeof block.meta.turnId === 'string' && ['user_input', 'request_user_input', 'ask_user_question'].includes(String(block.meta.toolName))
    ? [{ kind: 'tool' as const, id: block.id, turnId: block.meta.turnId, callId: block.meta.callId }] : [])
  const ids = useUserInputLocalAssociation(typeof input.meta?.turnId === 'string' ? { threadId, turnId: input.meta.turnId, itemId: input.id, requestId: input.requestId } : null, input.status, targets)
  const sole = ids.length === 1 ? ids[0] : null
  useEffect(() => { onMatch(input.id, sole); return () => onMatch(input.id, null) }, [input.id, sole, onMatch])
  return null
}
// The compact transcript reads only this leaf's validated private receipt.
function QuestionRowPresentation({ input, threadId, callId, props }: { input: Input; threadId: string; callId: string; props: ToolViewProps }) {
  const { t } = useTranslation('common'), secondary = useContext(GateResponseContext)
  const select = useChatStore(state => state.selectUserInputPanel)
  const [open, setOpen] = useState(false), rootRef = useRef<HTMLDivElement>(null)
  const runningCapture = props.block.status === 'running' && props.visible ? props.capture : undefined
  const privateOpenIntent = useRef(false), privateIntentGeneration = useRef(props.privateGeneration)
  useLayoutEffect(() => {
    if (!props.visible || privateIntentGeneration.current !== props.privateGeneration) privateOpenIntent.current = false
    privateIntentGeneration.current = props.privateGeneration
    if (privateOpenIntent.current && open && runningCapture?.expandable && !runningCapture.expanded) runningCapture.setOpen(true)
  }, [props.visible, props.privateGeneration, open, runningCapture])
  const toggle = () => {
    const next = !open
    privateOpenIntent.current = props.visible && props.block.status === 'running' && next
    setOpen(next); runningCapture?.setOpen(next)
  }
  const local = useUserInputLocalDisplay(typeof input.meta?.turnId === 'string' ? { threadId, turnId: input.meta.turnId, itemId: input.id, requestId: input.requestId } : null, input.status, props.visible)
  const display = local.display?.callId === callId ? local.display : null
  const [discarding, setDiscarding] = useState(false), [discardError, setDiscardError] = useState('')
  const commandEpoch = useRef(0), commandLock = useRef(false)
  const localRevision = useRef(local.revision); localRevision.current = local.revision
  useLayoutEffect(() => {
    const epochCounter = commandEpoch
    epochCounter.current++; commandLock.current = false; setDiscarding(false); setDiscardError('')
    return () => { epochCounter.current++ }
  }, [input.id, input.requestId, threadId, callId, local.privacyRevision])
  const discard = async () => {
    const delivery = display?.lateReply, api = window.analytix?.runtime
    if (secondary || display?.readOnly || !local.canTransfer() || input.status !== 'continued' || display?.status !== 'continued' || delivery?.state !== 'queued' ||
        !api?.discardUserInputLateReply || commandLock.current || discarding) return
    const epoch = commandEpoch.current, revision = local.revision
    commandLock.current = true; setDiscarding(true); setDiscardError('')
    const current = () => commandEpoch.current === epoch && localRevision.current === revision
    try {
      await api.discardUserInputLateReply({ threadId: display.threadId, turnId: display.turnId, itemId: display.itemId,
        requestId: display.requestId, action: 'discardLateReply', clientUserMessageId: delivery.clientUserMessageId, expectedTurnId: delivery.turnId })
      if (current()) local.refresh(revision)
    } catch {
      if (current()) { setOpen(true); setDiscardError(t('userInputLateDiscardFailed')); local.refresh(revision) }
    } finally {
      if (commandEpoch.current === epoch) { commandLock.current = false; setDiscarding(false) }
    }
  }
  useUserInputProtectedTransfer(rootRef, Boolean(display), local.canTransfer)
  if (!display) return <GenericToolCard {...props} />
  const lateState = display.lateReply?.state
  const answerable = !display.readOnly && (input.status === 'pending' || input.status === 'continued') && lateState !== 'queued' && lateState !== 'accepted'
  const submission = display.submission
  const answerLines = (id: string): string[] => {
    if (submission?.format === 'legacy') {
      const answer = submission.answers.find(item => item.questionId === id)
      return answer ? [answer.value] : []
    }
    const answer = submission?.answers.find(item => item.id === id), question = display.questions.find(item => item.id === id)
    if (!question || !answer || answer.kind === 'skip') return []
    return [...answer.optionIds.map(optionId => question.options.find(option => option.id === optionId)!.label), ...(answer.customText === undefined ? [] : [answer.customText])]
  }
  const answeredSummary = t('userInputAnsweredCount', { answered: display.questions.filter(question => answerLines(question.id).length > 0).length, total: display.questions.length })
  const summary = submission && (lateState === 'queued' || lateState === 'accepted' || input.status === 'submitted')
    ? lateState === 'queued' ? `${answeredSummary} · ${t('userInputLateQueued')}` : answeredSummary
    : t(input.status === 'continued' ? 'userInputContinued' : input.status === 'cancelled' ? 'userInputCancelled' : input.status === 'error' ? 'userInputFailed' : input.status === 'submitted' ? 'userInputOriginalAnswerUnavailable' : 'userInputPending')
  return <div ref={rootRef} className={css.row} data-question-call-id={callId} data-question-request-id={input.requestId}
    data-question-thread-id={threadId} data-question-late-state={lateState} data-question-inline-owned={secondary ? undefined : true} data-protected-user-input>
    <ToolRow icon={<DshQuestionOutlineRegular />} title={t('userInputTitle')} summary={summary} contentClassName={css.inlineContent}
      toolName="ask_user_question" running={input.status === 'pending'} expanded={open} expandable onToggle={toggle} keepSummaryWhenOpen
      actions={input.status === 'cancelled' && !display.readOnly ? undefined : <><button type="button" className={css.action} onClick={event => {
        event.stopPropagation()
        if (secondary) secondary.onOpenOwnerThread?.(input.requestId)
        else if (answerable) select(input.requestId)
        else if (!display.readOnly && lateState === 'queued') setOpen(value => !value)
        else select(input.requestId, 'review')
      }} onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') event.stopPropagation() }}
        disabled={Boolean(secondary && !secondary.onOpenOwnerThread)}><DshQuestionOutlineRegular />{t(answerable ? 'dshRespond' : 'userInputReview')}</button>
        {!secondary && !display.readOnly && input.status === 'continued' && display.status === 'continued' && lateState === 'queued' ? <button type="button" className={css.action}
          disabled={discarding || !window.analytix?.runtime.discardUserInputLateReply}
          onClick={event => { event.stopPropagation(); void discard() }}
          onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') event.stopPropagation() }}><DshCloseOutlineRegular />{t('userInputLateDiscard')}</button> : null}</>}>
      {discardError ? <p className={css.verdict} role="alert">{discardError}</p> : null}
      {open && runningCapture?.expanded ? runningCapture.body : null}
      {submission ? <dl className={css.card}>{display.questions.map(question => <div key={question.id} className={css.item}>
        <dt className={css.question}>{question.question}</dt><dd className={css.answer}>{answerLines(question.id).length
          ? answerLines(question.id).map((answer, index) => <span key={index} className={css.answerLine}>{answer}</span>)
          : <span className={css.skipped}>{t('userInputSkipped')}</span>}</dd>
      </div>)}</dl> : <div className={css.card}><p className={css.verdict}>{summary}</p><ul className={css.questionList}>
        {display.questions.map(question => <li key={question.id} className={css.unansweredQuestion}>{question.question}</li>)}
      </ul></div>}
    </ToolRow>
  </div>
}
// callId is supplied by the actual top-level tool machine anchor. Private
// display establishes association; request names or result prose never do.
export function AskQuestionRow({ callId, ...props }: ToolViewProps & { callId: string }) {
  const threadId = useContext(ProtectedToolResultScope)
  const allBlocks = useChatStore(state => state.blocks), activeThreadId = useChatStore(state => state.activeThreadId)
  const inputs = threadId === activeThreadId ? allBlocks.filter((block): block is Input => block.kind === 'user_input' && block.meta?.turnId === props.block.meta?.turnId) : []
  const associations = useContext(QuestionAssociationContext)
  const associated = inputs.filter(input => associations[input.id] === props.block.id)
  const input = associated.length === 1 ? associated[0] : undefined
  return !input || !threadId ? <GenericToolCard {...props} /> : <QuestionRowPresentation key={input.id + ':' + callId} input={input} threadId={threadId} callId={callId} props={props} />
}
