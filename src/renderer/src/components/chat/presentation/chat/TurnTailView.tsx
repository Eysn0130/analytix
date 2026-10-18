/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCcw } from '../../../../design/AnalytixUiIcons'
import type { ChatBlock, ThreadUsageSnapshot } from '../../../../agent/types'
import type { ProviderBillingV1 } from '../../../../../../../packages/runtime/src/contracts/usage'
import { CopyFeedbackButton } from '../../message-timeline-bubbles'
import { Tooltip } from '../Tooltip'
import { DshBranchOutlineRegular } from '../DshToolIcons'
import { TurnUsagePanel, turnAccountingFromBilling, type PerformanceUsageMode } from './TurnUsagePanel'
import { RunningStatus } from './turn-state/RunningStatus'
import { MessageFeedbackActions } from './MessageFeedbackActions'
import { messageClock, useCalendarDay } from './message-chrome'
import styles from './turn-state/TurnState.module.css'

type Action = { busy: boolean; run: () => void; unavailable?: boolean }

export function TurnTailView({ processing, running = false, startedAt, finalBlock, billing, fork, rollback, hasOutput,
  mode = 'detailed', reveal = 'always', ended = false, feedback, ownerKey }: {
  processing: boolean; running?: boolean; startedAt?: number; finalBlock?: Extract<ChatBlock, { kind: 'assistant' }>
  usage?: ThreadUsageSnapshot; billing?: ProviderBillingV1; fork?: Action; rollback?: Action; hasOutput: boolean
  mode?: PerformanceUsageMode; reveal?: 'always' | 'hover'; ended?: boolean; ownerKey?: string | null
  feedback?: { threadId: string; messageId: string }
}) {
  const { t } = useTranslation('common'), reasonId = useId()
  const clockTime = finalBlock?.createdAt ?? finalBlock?.acceptedFinalProjectionTerminal?.createdAt
  const day = useCalendarDay(Boolean(!processing && ended && clockTime && Number.isFinite(Date.parse(clockTime))))
  const accounting = ended ? turnAccountingFromBilling(billing) : undefined
  const clock = messageClock(clockTime, day, t)
  const unavailable = fork?.busy || fork?.unavailable === true
  const branchReason = fork?.busy ? t('forkingThread') : t('dshBranchUnavailable')
  return <div className={styles.tail} data-actions-reveal={reveal}>
    {running ? <RunningStatus startedAt={startedAt} hasOutput={hasOutput} /> : !processing && ended && finalBlock?.text.trim() ?
      <div className={styles.actions} data-clock="end">
        <CopyFeedbackButton text={finalBlock.text} iconOnly chrome="turn" className={styles.action} />
        {feedback ? <MessageFeedbackActions key={`${feedback.threadId}:${feedback.messageId}`} threadId={feedback.threadId} messageId={feedback.messageId} /> : null}
        {rollback ? <button type="button" disabled={rollback.busy} onClick={rollback.run}
          aria-label={t('rollbackWorkspaceFromAssistantResponse')} title={rollback.busy ? t('rollingBackWorkspace') : t('rollbackWorkspace')}
          className={styles.action}><RotateCcw /></button> : null}
        {fork ? <Tooltip label={unavailable ? branchReason : t('forkFromAssistantResponse')} side="bottom"><button type="button" aria-disabled={unavailable || undefined} data-unavailable={unavailable || undefined}
          onClick={unavailable ? undefined : fork.run} aria-label={t('forkFromAssistantResponse')}
          aria-describedby={unavailable ? reasonId : undefined}
          className={styles.action}><DshBranchOutlineRegular /></button></Tooltip> : null}
        {fork && unavailable ? <span id={reasonId} className={styles.visuallyHidden}>{branchReason}</span> : null}
        <span className={styles.endInfo}>
          {accounting ? <TurnUsagePanel billing={billing!} mode={mode} ownerKey={ownerKey} /> : null}
          {clock ? <span className={styles.timeEnd}>{clock}</span> : null}
        </span>
      </div> : null}
  </div>
}
