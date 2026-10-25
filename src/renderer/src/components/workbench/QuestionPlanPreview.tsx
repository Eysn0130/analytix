/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useWorkspaceTabsStore, questionPlanWorkspaceTabId, type QuestionPlanTabOwner } from '../../store/workspace-tabs-store'
import { MarkdownText, type MarkdownLabels } from '../chat/presentation/markdown/MarkdownText'
import { questionPlanReview } from '../chat/presentation/chat/question-plan-review'
import { requestUserInputLocalReopen, useUserInputLocalDisplay, useUserInputProtectedTransfer } from '../chat/presentation/chat/useUserInputLocalDisplay'
import css from './QuestionPlanPreview.module.css'

export function QuestionPlanPreview({ owner, presented, className = '' }: {
  owner: QuestionPlanTabOwner; presented: boolean; className?: string
}) {
  const { t } = useTranslation('common'), root = useRef<HTMLElement>(null)
  // A tab retains identity only. Every visible mount obtains a fresh native
  // lease; hidden retained dock/overlay instances never acquire raw plan text.
  const id = questionPlanWorkspaceTabId(owner.selector)
  const retired = useWorkspaceTabsStore(state => Boolean(state.tabs.find(tab => tab.id === id)?.questionPlanRetired))
  const local = useUserInputLocalDisplay(owner.selector, 'pending', presented && !retired,
    () => useWorkspaceTabsStore.getState().retireQuestionPlanReview(owner))
  const retry = () => {
    useWorkspaceTabsStore.getState().openQuestionPlanReview(owner, t('userInputPlanPreview'))
    requestUserInputLocalReopen(owner.selector)
  }
  const review = questionPlanReview(local.display)
  const valid = review !== null && (owner.callId === null || review.callId === owner.callId)
  useUserInputProtectedTransfer(root, valid, local.canTransfer)
  const labels = useMemo<MarkdownLabels>(() => ({ code: {
    copyLabel: t('userInputCopy'), copiedLabel: t('userInputCopied'),
    toolbarLabels: { codeLabel: t('userInputCode'), wrapLabel: t('userInputWrap'), unwrapLabel: t('userInputUnwrap') }
  }, footnotes: t('markdownFootnotes'), table: {
    title: t('markdownTableTitle'), copy: t('markdownTableCopy'), copied: t('copySuccess'), copyFailed: t('copyFailed'),
    download: t('markdownTableDownload'), downloadFailed: t('markdownTableDownloadFailed'), fullscreen: t('markdownTableFullscreen'),
    close: t('markdownTableClose'), csv: t('markdownTableCsv'), tsv: t('markdownTableTsv'), markdown: t('markdownTableMarkdown'), exportHint: t('markdownTableExportHint')
  } }), [t])
  if (!presented) return null
  if (!valid || !review) return <div className={css.message} role="status">
    {local.phase === 'loading' ? t('requestResolving') : t('userInputOriginalQuestionUnavailable')}
    {retired || local.phase === 'unavailable' ? <button type="button" onClick={retry}>{t('protectedToolRetry')}</button> : null}
  </div>
  return <section ref={root} className={`${css.preview} ${className}`} data-protected-user-input
    data-plan-preview={review.callId ?? owner.selector.requestId} aria-label={review.question.question}>
    <div className={css.document}><MarkdownText text={review.plan} labels={labels} questionDetail={{
      copyCode: span => local.effect({ kind: 'copyCode', questionId: review.question.id, ...span }),
      openLink: span => local.effect({ kind: 'openDetailLink', questionId: review.question.id, ...span })
    }} /></div>
  </section>
}
