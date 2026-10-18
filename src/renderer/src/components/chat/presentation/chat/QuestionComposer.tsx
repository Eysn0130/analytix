/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock, UserInputSubmission } from '../../../../agent/types'
import { DshCheckOutlineRegular as Check, DshChevronDownOutlineRegular as ChevronDown, DshChevronLeftOutlineRegular as ChevronLeft, DshChevronRightOutlineRegular as ChevronRight, DshChevronUpOutlineRegular as ChevronUp, DshEditOutlineRegular as PencilLine, DshCloseOutlineRegular as X } from '../DshToolIcons'
import type { UserInputLocalSelectorV1, UserInputLocalDisplayV1, UserInputLocalDraftSnapshotV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { MarkdownText, type MarkdownLabels } from '../markdown/MarkdownText'
import { useUserInputTimedWait } from './useUserInputTimedWait'
import { useUserInputPrivateDraft } from './useUserInputPrivateDraft'
import { requestUserInputLocalReopen, useUserInputProtectedTransfer, useUserInputLocalDisplay } from './useUserInputLocalDisplay'
import { useChatStore } from '../../../../store/chat-store'
import { useWorkspaceTabsStore } from '../../../../store/workspace-tabs-store'
import { questionPlanReview } from './question-plan-review'
import { PlanReviewPanel } from './PlanReviewPanel'
import styles from './QuestionComposer.module.css'

type Props = {
  block: Extract<ChatBlock, { kind: 'user_input' }>; resolving?: boolean
  readOnly?: boolean; presented?: boolean; selector: UserInputLocalSelectorV1 | null; onOpenOwnerThread?: (requestId: string) => void
  onSubmit?: (answers: UserInputSubmission[]) => Promise<void>; onCancel?: () => Promise<void>
  onLateSubmit?: (answers: UserInputSubmission[], clientUserMessageId: string, expectedTurnId: string) => Promise<void>
  lateReplyTargetTurnId?: string | null; getLateReplyExpectedTurnId?: () => Promise<string | null>; onHide?: () => void
}
type QuestionDraft = { selected: string[]; customText: string; skipped: boolean }
type Question = UserInputLocalDisplayV1['questions'][number]
const EMPTY_QUESTIONS: Question[] = []
const emptyDraft = (): QuestionDraft => ({ selected: [], customText: '', skipped: false })
const draftFor = (records: Record<string, QuestionDraft>, id: string): QuestionDraft =>
  Object.prototype.hasOwnProperty.call(records, id) ? records[id] : emptyDraft()

function draftsFromSnapshot(questions: Question[], snapshot: UserInputLocalDraftSnapshotV1): Record<string, QuestionDraft> {
  return Object.fromEntries(questions.map(question => {
    const saved = snapshot.answers.find(answer => answer.questionId === question.id)
    return [question.id, saved
      ? { selected: [...saved.optionIds], customText: saved.customText, skipped: saved.skipped }
      : emptyDraft()]
  }))
}

function parseRecommendedLabel(label: string): { label: string; recommended: boolean } {
  const suffix = /\s*(?:\((?:recommended|推荐)\)|（(?:recommended|推荐)）)\s*$/i
  return suffix.test(label) ? { label: label.replace(suffix, ''), recommended: true } : { label, recommended: false }
}

function AnswerField({ value, inline, disabled, label, placeholder, onChange }: {
  value: string; inline: boolean; disabled: boolean; label: string; placeholder: string
  onChange: (value: string) => void
}) {
  return <div className={`${styles.field} ${inline ? styles.customInline : styles.customBlock}`}>
    <div aria-hidden className={styles.fieldMirror}>{`${value}\n`}</div>
    <textarea rows={1} className={styles.fieldInput} value={value} disabled={disabled}
      aria-label={label} placeholder={placeholder}
      onChange={event => onChange(event.target.value)} />
  </div>
}

function initialDrafts(questions: Question[], pending: boolean, readOnly: boolean): Record<string, QuestionDraft> {
  if (!pending || readOnly) return {}
  return Object.fromEntries(questions.map((question): [string, QuestionDraft] => {
    const first = question.options[0]
    const recommended = first && parseRecommendedLabel(first.label).recommended
    return [question.id, { ...emptyDraft(), selected: recommended ? [first.id] : [] }]
  }))
}

function recordedDraft(display: UserInputLocalDisplayV1 | null, question: Question | undefined): QuestionDraft | null {
  if (!display?.submission || !question) return null
  if (display.submission.format === 'structured') {
    const answer = display.submission.answers.find(item => item.id === question.id)
    return !answer ? null : answer.kind === 'skip' ? { ...emptyDraft(), skipped: true }
      : { selected: answer.optionIds, customText: answer.customText ?? '', skipped: false }
  }
  // Legacy records retain original text, but do not prove an option identity.
  return null
}

export function QuestionComposer({ block, resolving: foregroundResolving = false, readOnly: surfaceReadOnly = false, presented = true, selector,
  onOpenOwnerThread, onSubmit, onCancel, onLateSubmit, lateReplyTargetTurnId = null, getLateReplyExpectedTurnId, onHide }: Props) {
  const { t } = useTranslation('common')
  const resolving = block.status === 'pending' && foregroundResolving
  const boundSelector = selector && (selector.itemId !== block.id || selector.requestId !== block.requestId ||
    selector.turnId !== block.meta?.turnId) ? null : selector
  const local = useUserInputLocalDisplay(boundSelector, block.status, presented)
  const display = local.display
  const readOnly = surfaceReadOnly || display?.readOnly === true
  // A display lease alone does not grant actions. Wait for the exact Host DTO;
  // inherited history may keep the same request IDs with read-only authority.
  const writableDisplay = Boolean(display && !readOnly && local.canTransfer())
  const planReview = questionPlanReview(display)
  const questions = display?.questions ?? EMPTY_QUESTIONS
  const foregroundPending = block.status === 'pending' && (!display || display.status === 'pending')
  const continued = block.status === 'continued' && display?.status === 'continued'
  const lateState = continued ? display.lateReply?.state : undefined
  const pending = foregroundPending || Boolean(continued && display?.readOnly !== true && lateState !== 'queued' && lateState !== 'accepted')
  const timing = display?.timedWait ?? block.timedWait
  const lateAttempt = useRef<{ id: string; answers: string; expectedTurnId: string } | null>(null)
  const ownerKey = JSON.stringify([block.id, block.requestId, block.meta?.turnId ?? null, selector ?? null])
  const draftKey = ownerKey + ':' + local.privacyRevision
  const timed = useUserInputTimedWait(writableDisplay ? boundSelector : null, Boolean(writableDisplay && timing), block.status, {
    identity: draftKey, isCurrent: () => Boolean(presented && !readOnly && foregroundPending && display?.status === 'pending' && local.canTransfer())
  })
  const waitDisposition = useRef<'waiting' | 'editing' | undefined>(undefined)
  const restoredWaitOwner = useRef<string | null>(null)
  const restoreTimedWait = timed.restore
  const headingId = useId()
  const rootRef = useRef<HTMLElement>(null)
  const [minimized, setMinimized] = useState(false)
  const [drafts, setDrafts] = useState<Record<string, QuestionDraft>>({})
  const draftRef = useRef(drafts)
  const draftOwner = useRef(draftKey)
  const initializedDraftOwner = useRef<string | null>(null)
  const currentOwner = useRef(ownerKey)
  currentOwner.current = ownerKey
  const actionGeneration = useRef(0)
  const foregroundAction = useRef<{ owner: string; epoch: number } | null>(null)
  const [index, setIndex] = useState(0)
  const previousIndex = useRef(0)
  const previouslyPending = useRef(pending)
  const questionRef = useRef<HTMLFieldSetElement>(null)
  const answerSurface = useRef<HTMLDivElement>(null)
  const focusedQuestions = useRef(new Set<string>())
  const [waiting, setWaiting] = useState(false)
  const [error, setError] = useState('')
  const lock = useRef(false)
  const active = useRef(true)
  const composing = useRef(false)
  const compositionEnded = useRef(false)
  const question = questions[Math.min(index, Math.max(0, questions.length - 1))]
  const draft = question && draftOwner.current === draftKey && pending && !readOnly ? draftFor(drafts, question.id) : emptyDraft()
  const lateReplyReady = Boolean(lateReplyTargetTurnId || getLateReplyExpectedTurnId || lateAttempt.current)
  const lateAdmission = useRef({ draftKey, presented, readOnly, continued })
  lateAdmission.current = { draftKey, presented, readOnly, continued }
  const lateAdmissionGeneration = useRef(0)
  useLayoutEffect(() => { lateAdmissionGeneration.current += 1 }, [draftKey, presented, readOnly, continued])
  const canResolve = presented && writableDisplay && pending && !resolving && !waiting && (!continued || lateReplyReady)
  const answerable = canResolve && questions.length > 0
  const last = index >= questions.length - 1
  const editable = useRef(pending && !readOnly)
  const markdownLabels = useMemo<MarkdownLabels>(() => ({ code: {
    copyLabel: t('userInputCopy'), copiedLabel: t('userInputCopied'),
    toolbarLabels: { codeLabel: t('userInputCode'), wrapLabel: t('userInputWrap'), unwrapLabel: t('userInputUnwrap') }
  }, footnotes: t('markdownFootnotes'), table: {
    title: t('markdownTableTitle'), copy: t('markdownTableCopy'), copied: t('copySuccess'), copyFailed: t('copyFailed'),
    download: t('markdownTableDownload'), downloadFailed: t('markdownTableDownloadFailed'), fullscreen: t('markdownTableFullscreen'),
    close: t('markdownTableClose'), csv: t('markdownTableCsv'), tsv: t('markdownTableTsv'), markdown: t('markdownTableMarkdown'),
    exportHint: t('markdownTableExportHint')
  } }), [t])
  useLayoutEffect(() => {
    if (draftOwner.current !== draftKey) {
      lateAttempt.current = null; draftOwner.current = draftKey
      waitDisposition.current = undefined; restoredWaitOwner.current = null; restoreTimedWait(undefined)
      draftRef.current = {}; setDrafts({}); setIndex(0); setError('')
    }
    // A same-owner readonly/terminal transition must not reseed recommended
    // answers. A private leaf initializes once its actual question arrives.
    if (initializedDraftOwner.current === draftKey || !questions.length) return
    initializedDraftOwner.current = draftKey
    const next = display?.draft
      ? draftsFromSnapshot(questions, display.draft)
      : continued && display?.submission
      ? Object.fromEntries(questions.map(item => [item.id, recordedDraft(display, item) ?? emptyDraft()]))
      : initialDrafts(questions, pending, readOnly)
    draftRef.current = next
    setDrafts(next); setIndex(display?.draft?.currentPage ?? 0); setError('')
  }, [draftKey, questions, pending, readOnly, continued, display, restoreTimedWait])
  useLayoutEffect(() => {
    actionGeneration.current += 1
    foregroundAction.current = null
    lock.current = false
    setWaiting(false)
    lateAttempt.current = null
    return () => { actionGeneration.current += 1 }
  }, [ownerKey])
  useLayoutEffect(() => {
    const action = foregroundAction.current
    if (block.status !== 'continued' || !action || action.owner !== ownerKey || action.epoch !== actionGeneration.current) return
    // Only the actual Host transition retires this foreground submission.
    // Loading/null display changes and late-answer readbacks retain their locks.
    actionGeneration.current += 1
    foregroundAction.current = null
    lock.current = false; setWaiting(false); setError(t('userInputResubmit'))
  }, [block.status, ownerKey, t])
  useUserInputProtectedTransfer(rootRef, Boolean(display), local.canTransfer)
  useLayoutEffect(() => {
    if (previouslyPending.current && !pending) setIndex(0)
    previouslyPending.current = pending
  }, [pending])
  useLayoutEffect(() => {
    if (lateState !== 'discarded' || !display?.lateReply || lateAttempt.current?.id !== display.lateReply.clientUserMessageId) return
    lateAttempt.current = null
    const restored = display.draft
      ? draftsFromSnapshot(questions, display.draft)
      : Object.fromEntries(questions.map(item => [item.id, recordedDraft(display, item) ?? emptyDraft()]))
    draftRef.current = restored; setDrafts(restored); setIndex(display.draft?.currentPage ?? 0); setError('')
  }, [lateState, display, questions])
  useEffect(() => { active.current = true; return () => { active.current = false } }, [])
  useLayoutEffect(() => {
    editable.current = pending && writableDisplay
    if (readOnly || !foregroundPending && !continued && block.status !== 'pending' && block.status !== 'continued') {
      draftRef.current = {}
      setDrafts({})
      setError('')
    }
  }, [pending, readOnly, writableDisplay, foregroundPending, continued, block.status])
  useEffect(() => {
    if (previousIndex.current === index) return
    previousIndex.current = index
    const control = questionRef.current?.querySelector<HTMLElement>('input:not(:disabled),textarea:not(:disabled)')
    ;(control ?? questionRef.current)?.focus({ preventScroll: true })
  }, [index])
  const freeQuestionId = question && !question.options.length ? question.id : null
  const waitPhase = timed.state?.phase, waitFocus = timed.focus, waitBlur = timed.blur
  useEffect(() => {
    if (presented && foregroundPending && freeQuestionId && !focusedQuestions.current.has(freeQuestionId) &&
      (!waitPhase || ['waiting', 'editing'].includes(waitPhase))) {
      const input = rootRef.current?.querySelector<HTMLTextAreaElement>('textarea:not(:disabled)')
      if (input) { focusedQuestions.current.add(freeQuestionId); input.focus({ preventScroll: true }) }
    }
  }, [presented, foregroundPending, freeQuestionId, waitPhase])
  useEffect(() => {
    const refocus = () => { if (presented && !document.hidden && document.hasFocus() && answerSurface.current?.contains(document.activeElement)) waitFocus() }
    const visibility = () => { if (document.hidden) waitBlur(); else refocus() }
    window.addEventListener('focus', refocus); document.addEventListener('visibilitychange', visibility)
    if (!presented) waitBlur(); else refocus()
    return () => { window.removeEventListener('focus', refocus); document.removeEventListener('visibilitychange', visibility) }
  }, [presented, waitPhase, waitFocus, waitBlur])
  const restorePrivateDraft = useCallback((snapshot: UserInputLocalDraftSnapshotV1) => {
    const restored = draftsFromSnapshot(questions, snapshot)
    draftOwner.current = draftKey; initializedDraftOwner.current = draftKey
    draftRef.current = restored; setDrafts(restored); setIndex(snapshot.currentPage)
    if (restoredWaitOwner.current === draftKey || !presented || readOnly || !pending || !display || !['pending', 'continued'].includes(display.status) || !local.canTransfer()) return
    restoredWaitOwner.current = draftKey
    const baseline = initialDrafts(questions, true, false)
    const changed = questions.some(item => {
      const value = draftFor(restored, item.id), initial = draftFor(baseline, item.id)
      return value.selected.length !== initial.selected.length || value.selected.some((id, ordinal) => id !== initial.selected[ordinal]) || value.customText !== '' || value.skipped
    })
    waitDisposition.current = snapshot.wait ?? (changed ? 'editing' : undefined)
    // Continued retains display metadata, while only actual pending may replay
    // native controls. No draft choice can revive a settled business claim.
    if (foregroundPending && display.status === 'pending') restoreTimedWait(waitDisposition.current)
  }, [questions, draftKey, presented, readOnly, pending, foregroundPending, display, local, restoreTimedWait])
  const privateDraft = useUserInputPrivateDraft({ selector: boundSelector, display,
    // The draft owner retains its already verified live cursor across loading
    // and ordinary hide. Its Host guard still refuses an unverified new owner
    // or inherited history; new UI edits require writableDisplay above.
    editable: !readOnly && ['pending', 'continued'].includes(block.status) && lateState !== 'queued' && lateState !== 'accepted',
    presented, privacyRevision: local.privacyRevision, retirementReason: local.retirementReason, refresh: local.refresh, onRestore: restorePrivateDraft,
    isAuthorityRetired: local.isAuthorityRetired,
    readInterruption: key => useChatStore.getState().userInputDraftInterruptions?.[key] ?? null,
    recordInterruption: (key, value) => useChatStore.getState().setUserInputDraftInterruption(key, value) })
  const flushPrivateDraft = privateDraft.flush, canTransferPrivateDisplay = local.canTransfer
  useLayoutEffect(() => {
    // Store subscriptions run synchronously before React detaches the former
    // visible leaf. Admit its latest snapshot while native ownership is current.
    return useChatStore.subscribe((next, previous) => {
      if (!next.deletedUserInputDraftThreadIds?.[previous.activeThreadId ?? ''] &&
          (next.activeThreadId !== previous.activeThreadId || next.route !== previous.route) && canTransferPrivateDisplay()) flushPrivateDraft()
    })
  }, [flushPrivateDraft, canTransferPrivateDisplay])
  const snapshotDrafts = (records: Record<string, QuestionDraft>, page: number): UserInputLocalDraftSnapshotV1 => ({
    ...(waitDisposition.current === undefined ? {} : { wait: waitDisposition.current }),
    currentPage: page, answers: questions.map(item => {
      const value = draftFor(records, item.id)
      return { questionId: item.id, optionIds: [...value.selected], customText: value.customText, skipped: value.skipped }
    })
  })
  const [parked, setParked] = useState(false), previouslyPresented = useRef(presented)
  const hideOwner = useRef(onHide); hideOwner.current = onHide
  useLayoutEffect(() => {
    if (presented && !previouslyPresented.current) setParked(false)
    previouslyPresented.current = presented
  }, [presented])
  const inlineIdentity = display?.callId ? JSON.stringify([display.callId, display.threadId, display.requestId]) : null
  const mayPark = Boolean(presented && !readOnly && !resolving && !waiting && continued && lateState === 'queued' && onHide && inlineIdentity)
  useLayoutEffect(() => {
    if (!mayPark || !inlineIdentity) return
    const [callId, threadId, requestId] = JSON.parse(inlineIdentity) as [string, string, string]
    let complete = false
    const park = () => {
      if (complete) return true
      const rows = Array.from(document.querySelectorAll<HTMLElement>('[data-question-inline-owned="true"]')).filter(row =>
        row.isConnected && !row.closest('[hidden],[inert]') && row.dataset.questionCallId === callId &&
        row.dataset.questionThreadId === threadId && row.dataset.questionRequestId === requestId && row.dataset.questionLateState === 'queued')
      if (rows.length !== 1) return false
      complete = true; setParked(true); hideOwner.current?.(); return true
    }
    if (park()) return
    // The real inline row can finish its private read after this leaf. Observe
    // only DOM ownership availability; no new question state/authority is made.
    const observer = new MutationObserver(() => { if (park()) observer.disconnect() })
    observer.observe(document.body, { childList: true, subtree: true, attributes: true,
      attributeFilter: ['hidden', 'inert', 'data-question-inline-owned', 'data-question-call-id', 'data-question-thread-id', 'data-question-request-id', 'data-question-late-state'] })
    return () => observer.disconnect()
  }, [mayPark, inlineIdentity])
  const replaceDrafts = (next: Record<string, QuestionDraft>, page = index) => {
    draftRef.current = next
    setDrafts(next)
    privateDraft.change(snapshotDrafts(next, page))
    setError('')
  }
  const answered = (item: Question, value: QuestionDraft): boolean => {
    const custom = value.customText.trim().length > 0
    const optionIds = item.options.map(option => option.id)
    if (value.selected.some(id => !optionIds.includes(id)) ||
      new Set(value.selected).size !== value.selected.length ||
      item.multiSelect !== true && value.selected.length + Number(custom) > 1) return false
    return value.selected.length > 0 || custom
  }
  const completed = (item: Question, value: QuestionDraft): boolean =>
    value.skipped || answered(item, value)
  const engage = () => {
    if (waitDisposition.current !== undefined) return
    waitDisposition.current = 'editing'; restoredWaitOwner.current = draftKey; timed.engage()
  }
  const takeTime = () => {
    if (!presented || readOnly || !foregroundPending || lock.current) return
    waitDisposition.current = 'waiting'; restoredWaitOwner.current = draftKey; timed.takeTime()
    privateDraft.change(snapshotDrafts(draftRef.current, index))
  }
  const updateDraft = (update: (current: QuestionDraft) => QuestionDraft, page = index) => {
    if (!answerable || lock.current || !question) return
    engage()
    replaceDrafts({ ...draftRef.current, [question.id]: update(draftFor(draftRef.current, question.id)) }, page)
    if (page !== index) setIndex(page)
  }
  const choose = (identity: string) => {
    if (!answerable || lock.current || !question) return
    updateDraft(current => question.multiSelect === true
      ? { ...current, selected: current.selected.includes(identity)
        ? current.selected.filter(id => id !== identity) : [...current.selected, identity], skipped: false }
      : { ...emptyDraft(), selected: [identity] }, question.multiSelect !== true && !last ? index + 1 : index)
  }
  const editCustom = (customText: string) => updateDraft(current => ({
    ...current, selected: question.multiSelect === true ? current.selected : [],
    customText, skipped: false
  }))
  const act = async (kind: 'submit' | 'cancel', values = draftRef.current) => {
    if (!canResolve || !local.canTransfer() || lock.current || kind === 'submit' && !questions.length) return
    const action = kind === 'submit' ? continued ? onLateSubmit : onSubmit : onCancel
    if (!action) return
    const missing = kind === 'submit'
      ? questions.findIndex(item => !completed(item, draftFor(values, item.id))) : -1
    if (missing >= 0) { setIndex(missing); privateDraft.change(snapshotDrafts(values, missing)); setError(t('userInputIncomplete')); return }
    const epoch = ++actionGeneration.current
    const owner = ownerKey
    const displayRevision = local.revision
    const admissionGeneration = lateAdmissionGeneration.current
    privateDraft.flush()
    if (!continued) foregroundAction.current = { owner, epoch }
    lock.current = true; setWaiting(true); setError('')
    try {
      if (kind === 'submit') {
        const answers: UserInputSubmission[] = questions.map(item => {
          const value = draftFor(values, item.id)
          if (value.skipped) return { id: item.id, kind: 'skip' }
          return { id: item.id, kind: 'answer', optionIds: [...value.selected],
            ...(value.customText.trim() ? { customText: value.customText } : {}) }
        })
        if (continued) {
          const encoded = JSON.stringify(answers)
          if (lateAttempt.current && lateAttempt.current.answers !== encoded) throw new Error('Retry the original answer first.')
          const expectedTurnId = lateAttempt.current?.expectedTurnId ?? lateReplyTargetTurnId ?? await getLateReplyExpectedTurnId?.()
          const current = lateAdmission.current
          if (!active.current || currentOwner.current !== owner || actionGeneration.current !== epoch ||
              lateAdmissionGeneration.current !== admissionGeneration || current.draftKey !== draftKey || !current.presented || current.readOnly || !current.continued || !local.canTransfer()) return
          if (!expectedTurnId) throw new Error('No current Host answer anchor.')
          const attempt = lateAttempt.current ?? { id: crypto.randomUUID(), answers: encoded, expectedTurnId }
          lateAttempt.current = attempt
          await onLateSubmit?.(answers, attempt.id, attempt.expectedTurnId)
        } else await onSubmit?.(answers)
      } else await onCancel?.()
      if (active.current && currentOwner.current === owner && actionGeneration.current === epoch) {
        if (!continued) { draftRef.current = {}; setDrafts({}) }
        local.refresh(displayRevision)
      }
    } catch (failure) {
      if (active.current && currentOwner.current === owner && actionGeneration.current === epoch && editable.current) {
        if (continued && failure instanceof Error && 'userInputLateReplyUnadmitted' in failure && failure.userInputLateReplyUnadmitted === true) lateAttempt.current = null
        privateDraft.change(snapshotDrafts(draftRef.current, index)); privateDraft.flush()
        setError(t(continued && failure instanceof Error && 'userInputLateReplyTooLarge' in failure && failure.userInputLateReplyTooLarge === true ? 'userInputLateTooLarge' : 'userInputFailed')); if (continued) local.refresh(displayRevision)
      }
    }
    finally {
      if (foregroundAction.current?.owner === owner && foregroundAction.current.epoch === epoch) foregroundAction.current = null
      if (active.current && currentOwner.current === owner && actionGeneration.current === epoch) { lock.current = false; setWaiting(false) }
    }
  }
  const discardLateReply = async () => {
    const delivery = display?.lateReply
    if (!presented || !writableDisplay || !local.canTransfer() || resolving || waiting || lock.current || !continued || delivery?.state !== 'queued' || !boundSelector || !window.analytix?.runtime.discardUserInputLateReply) return
    const epoch = ++actionGeneration.current, owner = ownerKey, displayRevision = local.revision
    // The captured queued attempt may predate this mounted leaf. Only Host
    // discarded readback permits restoring it as a new editable attempt.
    lateAttempt.current = { id: delivery.clientUserMessageId, expectedTurnId: delivery.turnId, answers: JSON.stringify(display.submission?.answers ?? []) }
    lock.current = true; setWaiting(true); setError('')
    try {
      await window.analytix.runtime.discardUserInputLateReply({ ...boundSelector, action: 'discardLateReply', clientUserMessageId: delivery.clientUserMessageId, expectedTurnId: delivery.turnId })
      if (active.current && currentOwner.current === owner && actionGeneration.current === epoch) local.refresh(displayRevision)
    } catch {
      if (active.current && currentOwner.current === owner && actionGeneration.current === epoch) { setError(t('userInputLateDiscardFailed')); local.refresh(displayRevision) }
    } finally {
      if (active.current && currentOwner.current === owner && actionGeneration.current === epoch) { lock.current = false; setWaiting(false) }
    }
  }
  const continueFlow = () => {
    if (!answerable || lock.current || !question || !answered(question, draftFor(draftRef.current, question.id))) return
    if (!last) navigate(index + 1)
    else void act('submit')
  }
  const skip = () => {
    if (!answerable || lock.current || !question) return
    engage()
    const next = { ...draftRef.current, [question.id]: { ...emptyDraft(), skipped: true } }
    replaceDrafts(next, !last ? index + 1 : index)
    if (!last) setIndex(index + 1)
    else void act('submit', next)
  }
  const closeQuestion = () => {
    if (onHide && (display?.callId || readOnly)) {
      if (readOnly) { onHide(); return }
      if (lock.current) return
      // The ordinary Float keeps this same private leaf mounted. Its existing
      // writer flushes the latest chain while dismissal remains immediate;
      // native background/authority retirement still owns admission and cleanup.
      privateDraft.flush()
      onHide()
    }
    else if (foregroundPending) void act('cancel')
  }
  const keyDown = (event: KeyboardEvent<HTMLElement>) => {
    const target = event.target as HTMLElement
    const composingEvent = composing.current || compositionEnded.current || event.nativeEvent.isComposing || event.keyCode === 229
    if (event.defaultPrevented || event.repeat || composingEvent || event.altKey) {
      // Returning alone still permits the browser to activate a focused control.
      if ((event.repeat || composingEvent) && (event.key === 'Enter' || event.key === ' ') &&
        target.closest('button,input[type=radio],input[type=checkbox]')) event.preventDefault()
      return
    }
    if (target.closest('a,select,[contenteditable]')) return
    if (event.key === 'Escape' && !event.ctrlKey && !event.metaKey) { event.preventDefault(); closeQuestion(); return }
    if (target.closest('button')) return
    if (event.key === 'Enter' && !event.shiftKey &&
      target.closest('input[type=radio],input[type=checkbox]')) {
      event.preventDefault(); void act('submit'); return
    }
    if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); continueFlow() }
  }
  const navigate = (next: number) => {
    if (resolving || waiting || lock.current || next < 0 || next >= questions.length) return
    setIndex(next)
    if (writableDisplay && pending) privateDraft.change(snapshotDrafts(draftRef.current, next))
    setError('')
  }
  const detail = display?.questions.find(item => item.id === question?.id)?.detail
  const legacyCommitted = display?.submission?.format === 'legacy'
    ? display.submission.answers.find(item => item.questionId === question?.id) : undefined
  const recorded = recordedDraft(display, question)
  const review = recorded ?? emptyDraft()
  const shownDraft = pending && !readOnly ? draft : review
  const showCustom = pending && !readOnly || recorded !== null && review.customText !== ''
  const visibleStatus = display?.status ?? block.status
  const continuedLabel = display?.continuedReason === 'user_input_outcome_unknown_after_restart' ? 'userInputContinuedUnknown' : 'userInputContinued'
  const status = resolving || waiting ? t('requestResolving') : visibleStatus === 'submitted'
    ? t('userInputSubmitted') : visibleStatus === 'cancelled' ? t('userInputCancelled')
      : lateState === 'queued' ? t('userInputLateQueued') : lateState === 'accepted' ? t('userInputLateAccepted')
      : continued ? t(continuedLabel) : block.status === 'error' ? t('userInputFailed') : t('userInputPending')
  const draftFailure = privateDraft.interrupted ? t('userInputDraftInterrupted') : privateDraft.state.phase === 'error' ? t(privateDraft.state.code === 'capacity' ? 'userInputDraftCapacity'
    : privateDraft.state.code === 'conflict' ? 'userInputDraftConflict' : privateDraft.state.code === 'retired' ? 'userInputDraftRetired' : 'userInputDraftFailed') : ''
  useEffect(() => {
    if (!planReview || !boundSelector || !presented || readOnly || !pending || !local.canTransfer()) return
    useWorkspaceTabsStore.getState().openQuestionPlanReview({ selector: boundSelector, callId: planReview.callId }, t('userInputPlanPreview'), true)
  }, [planReview, boundSelector, presented, readOnly, pending, local, t])
  if (parked && !presented) return null
  if (planReview && !readOnly && pending) return <PlanReviewPanel review={planReview} requestId={block.requestId} sectionRef={rootRef}
    busy={resolving || waiting} disabled={!canResolve} feedback={block.errorMessage || error || draftFailure}
    onKeyDown={keyDown}
    onOpenPlan={() => {
      if (!boundSelector || !local.canTransfer()) return
      useWorkspaceTabsStore.getState().openQuestionPlanReview({ selector: boundSelector, callId: planReview.callId }, t('userInputPlanPreview'))
      requestUserInputLocalReopen(boundSelector)
    }}
    onApprove={() => { void act('submit', { [planReview.question.id]: { ...emptyDraft(), selected: [planReview.approve.id] } }) }}
    onDiscuss={() => { if (foregroundPending) void act('cancel'); else closeQuestion() }} />
  return <div className={styles.frame}><section ref={rootRef} data-protected-user-input={display ? true : undefined} className={`${styles.card} ${minimized ? styles.cardMinimized : ''}`}
    aria-labelledby={headingId} data-gate-request-id={block.requestId} tabIndex={-1} aria-busy={resolving || waiting}
    onKeyDown={keyDown} onKeyUp={() => { compositionEnded.current = false }}
    onCompositionStart={() => { composing.current = true }}
    onCompositionEnd={() => { composing.current = false; compositionEnded.current = true }}>
    <header className={styles.header}>
      <div className={styles.headingBlock}>
        {question?.header ? <div className={styles.eyebrow}>{question.header}</div> : null}
        <h2 className={styles.title} id={headingId}>{question?.question ?? t('userInputTitle')}</h2>
      </div>
      <div className={styles.headerActions}>
        {foregroundPending && timed.state && ['counting', 'focused'].includes(timed.state.phase) && timed.state.remainingMs !== null
          ? <span className={styles.waitStatus}>{t(timed.state.phase === 'focused' ? 'userInputPaused' : 'userInputCountdown', { seconds: Math.ceil(timed.state.remainingMs / 1000) })}</span> : null}
        {foregroundPending && timed.state && ['counting', 'focused'].includes(timed.state.phase)
          ? <button type="button" className={`${styles.secondary} ${styles.waitButton}`} onClick={takeTime}>{t('userInputTakeTime')}</button> : null}
        {foregroundPending && timed.state && timing?.timeoutSeconds !== -1 && ['waiting', 'editing'].includes(timed.state.phase)
          ? <span className={styles.waitStatus}>{t('userInputHeld')}</span> : null}
        {continued ? <span className={styles.waitStatus}>{t(lateState === 'queued' ? 'userInputLateQueued' : lateState === 'accepted' ? 'userInputLateAccepted' : continuedLabel)}</span> : null}
        <button type="button" className={styles.iconButton} aria-expanded={!minimized}
          aria-label={t(minimized ? 'userInputMaximize' : 'userInputMinimize')}
          title={t(minimized ? 'userInputMaximize' : 'userInputMinimize')} disabled={resolving || waiting}
          onClick={() => { if (!lock.current) setMinimized(current => !current) }}>
          {minimized ? <ChevronUp /> : <ChevronDown />}
        </button>
        {(onHide && (display?.callId || readOnly) || !readOnly && foregroundPending) ? <button type="button" className={styles.iconButton}
          aria-label={t(onHide && (display?.callId || readOnly) ? 'userInputHide' : 'userInputCancel')} title={t(onHide && (display?.callId || readOnly) ? 'userInputHide' : 'userInputCancel')}
          disabled={resolving || waiting || !(onHide && (display?.callId || readOnly)) && !writableDisplay}
          onClick={closeQuestion}><X /></button> : null}
      </div>
    </header>
    {!minimized ? <>
      <div ref={answerSurface} className={styles.body} data-question-scroll
        onFocusCapture={event => { if (presented && !event.currentTarget.contains(event.relatedTarget as Node | null)) timed.focus() }}
        onBlurCapture={event => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) { timed.blur(); privateDraft.flush() } }}>
        {question ? <div key={question.id}>
          {detail !== undefined ? <div className={styles.detail}>
            <MarkdownText text={detail} labels={markdownLabels} questionDetail={{
              copyCode: span => local.effect({ kind: 'copyCode', questionId: question.id, ...span }),
              openLink: span => local.effect({ kind: 'openDetailLink', questionId: question.id, ...span }),
            }} />
          </div> : null}
          <fieldset ref={questionRef} tabIndex={-1} className={styles.question}
            aria-labelledby={headingId} disabled={!answerable}>
          <div role={question.multiSelect ? 'group' : 'radiogroup'} aria-label={question.question} className={styles.options}>
            {<>
              {question.options.map((option, optionIndex) => {
                const identity = option.id
                const selected = shownDraft.selected.includes(identity)
                const display = parseRecommendedLabel(option.label)
                return <label key={option.id}
                  className={`${styles.option} ${selected && !question.multiSelect ? styles.optionSelected : ''}`}>
                  <input type={question.multiSelect ? 'checkbox' : 'radio'} name={block.requestId + ':' + question.id}
                    value={identity} aria-label={display.label} checked={selected} onChange={() => choose(identity)}
                    onClick={() => { if (selected && question.multiSelect !== true) choose(identity) }} />
                  {question.multiSelect ? <span className={`${styles.checkbox} ${selected ? styles.checkboxChecked : ''}`} aria-hidden>
                    {selected ? <Check size={12} /> : null}
                  </span> : <span className={styles.number} aria-hidden>{optionIndex + 1}</span>}
                  <span className={styles.optionCopy}><span className={styles.optionLine}>
                    <span className={styles.optionLabel}>{display.label}</span>
                    {display.recommended ? <span className={styles.badge}>{t('userInputRecommended')}</span> : null}
                    {option.description ? <span className={styles.description}>{option.description}</span> : null}
                  </span></span>
                </label>
              })}
              {showCustom ? question.options.length ? <div data-question-custom
                className={`${styles.customRow} ${shownDraft.customText !== '' ? styles.customRowActive : ''}`}>
                {question.multiSelect ? <span className={`${styles.checkbox} ${shownDraft.customText !== '' ? styles.checkboxChecked : ''}`} aria-hidden>
                  {shownDraft.customText !== '' ? <Check size={12} /> : null}
                </span> : <span className={styles.number} aria-hidden><PencilLine size={12} /></span>}
                <AnswerField inline value={shownDraft.customText} disabled={!answerable} label={question.question}
                  placeholder={t('userInputCustomPlaceholder')} onChange={editCustom} />
              </div> : <AnswerField inline={false} value={shownDraft.customText} disabled={!answerable} label={question.question}
                placeholder={t('userInputCustomPlaceholder')} onChange={editCustom} /> : null}
              {legacyCommitted ? <p className={`${styles.reviewNote} ${styles.answer}`}>{legacyCommitted.value}</p> : null}
              {shownDraft.skipped ? <p className={styles.reviewNote} role="status">{t('userInputSkipped')}</p> : null}
            </>}
          </div>
          </fieldset>
        </div> : null}
        {!display ? <div className={styles.reviewNote} role="status">
          <p>{local.phase === 'loading' ? t('requestResolving') : t('userInputOriginalQuestionUnavailable')}</p>
          {local.phase === 'unavailable' ? <button type="button" className={styles.secondary} disabled={resolving || waiting}
            onClick={() => local.refresh()}>{t('protectedToolRetry')}</button> : null}
        </div> : null}
        {continued && !readOnly && !lateReplyReady && lateState !== 'accepted' && lateState !== 'queued' ? <p className={styles.reviewNote} role="status">{t('userInputLateUnavailable')}</p> : null}
        {lateState === 'discarded' ? <p className={styles.reviewNote} role="status">{t('userInputLateDiscarded')}</p> : null}
        {display?.answerState === 'unavailable' ? <p className={styles.reviewNote} role="status">{t('userInputOriginalAnswerUnavailable')}</p> : null}
        {readOnly && pending ? <div className={styles.reviewNote}><p>{t('pendingRequestNotResolved')}</p>
          {onOpenOwnerThread ? <button type="button" className={styles.primary}
            onClick={() => onOpenOwnerThread(block.requestId)}>{t('respondInOwnedThread')}</button>
            : <p>{t('ownedThreadUnavailable')}</p>}</div> : null}
      </div>
      <footer className={styles.footer}>
        <div className={styles.pager}>
          <button type="button" aria-label={t('userInputPrevious')} disabled={index === 0 || resolving || waiting}
            className={styles.iconButton} onClick={() => navigate(index - 1)}><ChevronLeft /></button>
          <span className={styles.progress} aria-label={t('userInputQuestionProgress', { current: question ? index + 1 : 0, total: questions.length })}>
            {question ? index + 1 : 0} / {questions.length}
          </span>
          <button type="button" aria-label={t('userInputNext')} disabled={last || resolving || waiting}
            className={styles.iconButton} onClick={() => navigate(index + 1)}><ChevronRight /></button>
        </div>
        <div className={styles.feedback} role={block.errorMessage || error || draftFailure ? 'alert' : 'status'}>
          {block.errorMessage || error || draftFailure || (timed.state?.phase === 'continuing' ? t('userInputContinuePending')
            : timed.state?.phase === 'scheduled' ? t('userInputContinueScheduled')
            : timed.state?.phase === 'error' || timed.state?.phase === 'ended' ? t('userInputWaitUnknown')
            : resolving || waiting || !pending ? status : '')}
        </div>
        {privateDraft.interrupted && presented ? <button type="button" className={styles.secondary} disabled={resolving || waiting}
          onClick={() => local.refresh()}>{t('protectedToolRetry')}</button> : null}
        {presented && pending && !readOnly && privateDraft.state.phase === 'error'  && privateDraft.state.canRetry ? <div className={styles.footerActions}>
          <button type="button" className={styles.secondary} disabled={resolving || waiting} onClick={privateDraft.retry}>{t('userInputDraftRetry')}</button>
        </div> : null}
        {presented && continued && lateState === 'queued' && !readOnly ? <div className={styles.footerActions}>
          <button type="button" className={styles.secondary} disabled={resolving || waiting || !window.analytix?.runtime.discardUserInputLateReply} onClick={() => { void discardLateReply() }}>{t('userInputLateDiscard')}</button>
        </div> : null}
        {pending && !readOnly ? <div className={styles.footerActions}>
          {foregroundPending && timed.state && ['error', 'ended'].includes(timed.state.phase) ? <button type="button" className={styles.secondary} onClick={() => { local.refresh(); timed.retry() }}>{t('protectedToolRetry')}</button> : null}
          <button type="button" disabled={!answerable} className={styles.secondary} onClick={skip}>{t('userInputSkip')}</button>
          <button type="button" disabled={!answerable || !question || !answered(question, draft)} className={styles.primary}
            onClick={continueFlow}>{t(last ? continued ? 'userInputLateSubmit' : 'userInputSubmit' : 'userInputNext')}</button>
        </div> : null}
      </footer>
    </> : null}
  </section></div>
}
