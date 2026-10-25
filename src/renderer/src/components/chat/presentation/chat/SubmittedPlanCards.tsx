/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../../../agent/types'
import { extractMarkdownPlainText } from '../../../../../../shared/markdown/plain-text'
import type { UserInputLocalSelectorV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { userInputLocalSelectorSchemaV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { questionPlanWorkspaceTabId, useWorkspaceTabsStore } from '../../../../store/workspace-tabs-store'
import { DshMarkdownFileIcon } from '../DeliverableIcons'
import { questionPlanReview } from './question-plan-review'
import { requestUserInputLocalReopen, useUserInputLocalDisplay, useUserInputProtectedTransfer } from './useUserInputLocalDisplay'
import css from './SubmittedPlanCards.module.css'

type QuestionBlock = Extract<ChatBlock, { kind: 'user_input' }>

/** Only the original signed private Question can identify a submitted plan. */
function SubmittedPlanCard({ selector, status, presented }: {
  selector: UserInputLocalSelectorV1; status: QuestionBlock['status']; presented: boolean
}) {
  const { t } = useTranslation('common'), root = useRef<HTMLDivElement>(null)
  // Keep only the verified opaque call identity for an explicit reread after
  // retirement. The title and Markdown remain in this mounted native lease.
  const tabId = questionPlanWorkspaceTabId(selector)
  const tab = useWorkspaceTabsStore(state => state.tabs.find(candidate => candidate.id === tabId))
  const verifiedCallId = useRef<string | null>(tab?.questionPlan?.callId ?? null)
  const [authorityRetired, setAuthorityRetired] = useState(false)
  const [ordinaryQuestion, setOrdinaryQuestion] = useState(false)
  const tabRetired = Boolean(tab?.questionPlanRetired)
  const local = useUserInputLocalDisplay(selector, status, presented && !authorityRetired && !tabRetired && !ordinaryQuestion, () => {
    setAuthorityRetired(true)
    if (verifiedCallId.current) useWorkspaceTabsStore.getState().retireQuestionPlanReview({ selector, callId: verifiedCallId.current })
  })
  const review = questionPlanReview(local.display)
  const valid = Boolean(review?.callId && (!verifiedCallId.current || review.callId === verifiedCallId.current))
  useLayoutEffect(() => {
    if (valid && review?.callId) verifiedCallId.current = review.callId
    // A signed ordinary Question is no submitted plan. Retire its raw lease so
    // unrelated requests cannot occupy every native seat before a real plan.
    else if (local.phase === 'ready' && local.display && !review) setOrdinaryQuestion(true)
  }, [valid, review?.callId, local.phase, local.display, review])
  useUserInputProtectedTransfer(root, valid, local.canTransfer)
  const callId = valid ? review!.callId : verifiedCallId.current
  if (!presented || !callId) return null
  const title = valid ? extractMarkdownPlainText(review!.plan, { mode: 'first-line' }) : t('userInputPlanPreview')
  const open = () => {
    if (!presented || valid && !local.canTransfer()) return
    setAuthorityRetired(false)
    useWorkspaceTabsStore.getState().openQuestionPlanReview({ selector, callId }, t('userInputPlanPreview'))
    requestUserInputLocalReopen(selector)
  }
  return <div ref={root} data-protected-user-input={valid ? true : undefined}>
    <button type="button" className={css.card} data-plan-card={callId}
      aria-label={t('submittedPlanOpenNamed', { title })} onClick={open}>
      <span className={css.cardIcon}><DshMarkdownFileIcon size={20} /></span>
      <span className={css.cardDetails}>
        <span className={css.cardTitle}>{title}</span>
        <span className={css.cardDescription}>{t('submittedPlanDocument')}</span>
      </span>
      <span className={css.cardOpen}>{t('submittedPlanOpen')}</span>
    </button>
  </div>
}

/** Invocation order comes from this Turn, never the current GUI file plan. */
export function SubmittedPlanCards({ threadId, turnId, blocks, presented = true }: {
  threadId: string | null; turnId: string | null; blocks: ChatBlock[]; presented?: boolean
}) {
  if (!threadId || !turnId) return null
  const seen = new Set<string>()
  const requests = blocks.flatMap(block => {
    if (block.kind !== 'user_input' || block.meta?.turnId !== turnId || block.status === 'error') return []
    const selector = userInputLocalSelectorSchemaV1.safeParse({ threadId, turnId, itemId: block.id, requestId: block.requestId })
    if (!selector.success) return []
    const key = JSON.stringify(selector.data)
    if (seen.has(key)) return []
    seen.add(key)
    return [{ key, selector: selector.data, status: block.status }]
  })
  if (!requests.length) return null
  return <div className={css.cards} data-plan-artifacts>
    {requests.map(request => <SubmittedPlanCard key={request.key} selector={request.selector} status={request.status} presented={presented} />)}
  </div>
}
