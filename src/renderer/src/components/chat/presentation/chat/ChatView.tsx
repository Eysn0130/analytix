/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
// MIT, upstream 5badb150; no upstream host, store or execution engine is imported.
import { memo, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { AuthoritativeTurnStatus, ChatBlock, ThreadUsageSnapshot } from '../../../../agent/types'
import type { ThreadTurn } from '../../../../thread/projection/thread-turns'
import type { TurnForkBoundary } from '../../../../store/chat-store-types'
import type { ProviderBillingV1 } from '../../../../../../../packages/runtime/src/contracts/usage'
import type { TurnSections } from '../../../../thread/projection/derive-turn-sections'
import { MessageBubble, UserMessageActionsRevealContext } from '../../message-timeline-bubbles'
import { ProtectedToolResultScope } from '../ProtectedToolResult'
import { ProtectedProcessPane, type ProcessPaneRoute } from './ProtectedProcessPane'
import { TurnTailView } from './TurnTailView'
import { TurnMaxTokensItem } from './turn-state/StatusItems'
import type { PerformanceUsageMode } from './TurnUsagePanel'
import { TurnStatusView } from './TurnStatusView'
import { AttachmentDeliveries } from './AttachmentDeliveries'
import { ScheduleTurnCard } from '../../../schedule/ScheduleTurnCard'
import { useDisclosure } from './use-disclosure'
import { hasProcessError, presentationPolicyFor } from './chat-view-model'
import { useWorkDetailsPreference } from '../../../../lib/chat-presentation-preferences'
import { GoalBar } from '../../above-composer/GoalBar'
import { PresentClosingContext, PresentClosingTail, usePresentHistory } from './PresentedFiles'
import { presentCallsForClosing } from './presented-files'
import styles from './ChatView.module.css'

type Action = { busy: boolean; run: () => void; unavailable?: boolean }
export type ChatViewProps = {
  threadId: string | null; turnId: string | null; turn: ThreadTurn; sections: TurnSections
  processing: boolean; running?: boolean; turnStatus?: AuthoritativeTurnStatus; runningStartedAt?: number; completedDurationMs?: number
  usage?: ThreadUsageSnapshot; billing?: ProviderBillingV1; performanceUsageMode?: PerformanceUsageMode; tailReveal?: 'always' | 'hover'; userNavigationItemId?: string; liveAnswer?: ReactNode
  fork?: Omit<Action, 'run'> & { run: (boundary: TurnForkBoundary) => void }; rollback?: Action; deliveries?: ReactNode
  processPaneRoute?: ProcessPaneRoute; runtimeAvailable?: boolean; processPaneAutoAcquire?: boolean
  onAssistantFinalized?: () => void
  onInspect?: (turnId: string, callId: string) => void
}

export const ChatView = memo(function ChatView({ threadId, turnId, turn, sections, processing,
  running = false, runningStartedAt, completedDurationMs, turnStatus, billing, performanceUsageMode, tailReveal, userNavigationItemId, liveAnswer,
  fork, rollback, deliveries, processPaneRoute, runtimeAvailable, processPaneAutoAcquire = true, onAssistantFinalized, onInspect }: ChatViewProps) {
  const { t } = useTranslation('common')
  const latestUserId = turn.blocks.slice().reverse().find(block => block.kind === 'user' && block.source === undefined)?.id ?? turn.user?.id
  const userActionsReveal = useMemo(() => ({ latestUserId, reveal: tailReveal ?? ('always' as const) }), [latestUserId, tailReveal])
  const policy = presentationPolicyFor(useWorkDetailsPreference())
  const pendingGate = sections.processBlocks.some(block =>
    (block.kind === 'approval' || block.kind === 'user_input') && block.status === 'pending')
  const error = sections.processBlocks.some(hasProcessError)
  // Pending gates are presented once by the composer. The transcript keeps a
  // compact owner-navigation marker so focus/navigation never answers another thread.
  const process = sections.processBlocks
  const assistantBlocks = [...sections.processBlocks, ...sections.assistantContentBlocks]
    .filter((block): block is Extract<ChatBlock, { kind: 'assistant' }> => block.kind === 'assistant')
  const finalBlock = assistantBlocks.slice().reverse().find(block => block.text.trim() !== '')
  const assistantIdentity = finalBlock?.assistantItemIdentity
  const feedback = threadId && turnId && assistantIdentity?.threadId === threadId &&
    assistantIdentity.turnId === turnId && assistantIdentity.itemId === finalBlock?.id
      ? { threadId, messageId: assistantIdentity.itemId } : undefined
  const finalIndex = finalBlock ? turn.blocks.map(block => block.id).lastIndexOf(finalBlock.id) : -1
  const hasLaterTranscript = finalIndex >= 0 && turn.blocks.slice(finalIndex + 1).some(block =>
    block.kind !== 'compaction' && block.kind !== 'goal' && (block.kind !== 'assistant' || block.text.trim() !== ''))
  const terminal = finalBlock?.acceptedFinalProjectionTerminal
  const generalReceipt = finalBlock?.generalTerminalProjectionReceipt
  const receipt = finalBlock?.acceptedFinalProjectionReceipt
  const acceptedTerminalBound = Boolean(terminal && finalBlock &&
    finalBlock.meta?.turnId === turnId && receipt?.schemaVersion === 1 && receipt.threadId === threadId && receipt.turnId === turnId &&
    /^[a-f0-9]{64}$/.test(receipt.batchId) && Number.isSafeInteger(receipt.lastSeq) && receipt.lastSeq > 0 &&
    receipt.publicationCommitId === terminal.acceptedFinalDigest && finalBlock.acceptedFinalView?.acceptedFinalDigest === terminal.acceptedFinalDigest)
  const generalTerminalBound = Boolean(finalBlock && !finalBlock.acceptedFinalView &&
    finalBlock.meta?.turnId === turnId && generalReceipt?.schemaVersion === 1 &&
    generalReceipt.threadId === threadId && generalReceipt.turnId === turnId && generalReceipt.itemId === finalBlock.id &&
    /^[a-f0-9]{64}$/.test(generalReceipt.batchDigest) &&
    Number.isSafeInteger(generalReceipt.firstSeq) && generalReceipt.firstSeq > 0 &&
    Number.isSafeInteger(generalReceipt.lastSeq) && generalReceipt.lastSeq === generalReceipt.firstSeq + 2)
  const emptyReceipt = turn.user?.generalTerminalProjectionReceipt
  const emptyTerminalBound = Boolean(!finalBlock && turn.user && emptyReceipt?.schemaVersion === 1 &&
    turn.user.meta?.turnId === turnId && emptyReceipt.threadId === threadId && emptyReceipt.turnId === turnId &&
    emptyReceipt.itemId === turn.user.id && emptyReceipt.status === 'completed' &&
    /^[a-f0-9]{64}$/.test(emptyReceipt.batchDigest) && Number.isSafeInteger(emptyReceipt.firstSeq) && emptyReceipt.firstSeq > 0 &&
    Number.isSafeInteger(emptyReceipt.lastSeq) && emptyReceipt.lastSeq === emptyReceipt.firstSeq + 1 && emptyReceipt.endReason?.kind === 'max-tokens')
  const status = acceptedTerminalBound ? terminal?.status : generalTerminalBound ? generalReceipt?.status : emptyTerminalBound ? emptyReceipt?.status : turnStatus
  const endReason = acceptedTerminalBound ? terminal?.endReason : generalTerminalBound ? generalReceipt?.endReason : emptyTerminalBound ? emptyReceipt?.endReason : undefined
  const settled = status === 'completed' || status === 'failed' || status === 'aborted'
  const acceptedTerminalAuthorized = !processing && settled && acceptedTerminalBound
  const generalTerminalAuthorized = !processing && generalTerminalBound && generalReceipt?.status === 'completed'
  const terminalAuthorized = acceptedTerminalAuthorized || generalTerminalAuthorized
  const alwaysOpen = status === 'failed' || status === 'aborted'
  const hasInterleavedInput = turn.blocks.some(block => block.kind === 'user')
  const unfolded = !policy.foldCompletedTurns && settled
  const disclosure = useDisclosure(`${threadId}:${turnId ?? turn.user?.id ?? 'live'}:${processing ? 'running' : 'settled'}`,
    processing || pendingGate || alwaysOpen || error && sections.assistantContentBlocks.length === 0)
  const processOpen = hasInterleavedInput || alwaysOpen || unfolded || disclosure.open
  const presentHistory = usePresentHistory(threadId, turnId, turn.blocks, processing, status, runtimeAvailable)
  const endCalls = presentCallsForClosing(threadId, turnId, turn.blocks, presentHistory, null, !processing && settled)
  const renderClosing = (closing: ChatBlock) => {
    const calls = presentCallsForClosing(threadId, turnId, turn.blocks, presentHistory, closing, !processing && settled)
    const final = closing.id === finalBlock?.id && !endCalls.length
    return calls.length || final && (sections.generatedFileBlocks.length || deliveries) ? <AttachmentDeliveries
      blocks={final ? sections.generatedFileBlocks : []} present={{ threadId, calls }}>{final ? deliveries : null}</AttachmentDeliveries> : null
  }
  // CSS follows the mounted seats, including independently updated private rows.
  return <ProtectedToolResultScope.Provider value={threadId}>
    <PresentClosingContext.Provider value={renderClosing}>
    <UserMessageActionsRevealContext.Provider value={userActionsReveal}>
    <div className={styles.chat} data-analytix-chat-view>
      {turn.user ? <div data-content-search-unit-key={userNavigationItemId}><MessageBubble block={turn.user} threadId={threadId} /></div> : null}
      <ProtectedProcessPane onInspect={onInspect} route={processPaneRoute ? { ...processPaneRoute, visible: processPaneRoute.visible && (processPaneAutoAcquire || disclosure.manuallyOpened) } : undefined} threadId={threadId} turnId={turnId} runtimeAvailable={runtimeAvailable}
        offerProcessEntry={!processPaneAutoAcquire} requestProcessDisplay={!processPaneAutoAcquire && !disclosure.manuallyOpened}
        onRequestProcessDisplay={() => disclosure.setOpen(true)} processOpen={processOpen} processBlocks={process} processReplyBoundaryAfterIds={sections.processReplyBoundaryAfterIds} processing={processing} policy={policy}
        status={settled ? status : undefined} collapsible={!hasInterleavedInput && !alwaysOpen && !unfolded} onToggle={disclosure.toggle}
        completedDurationMs={settled ? completedDurationMs : undefined} terminalAuthorized={terminalAuthorized}
        answer={<>{sections.assistantContentBlocks.map(block => <div key={block.id}><MessageBubble block={block}
          threadId={threadId} showActions={false} onAssistantFinalized={onAssistantFinalized} /><PresentClosingTail closing={block} /></div>)}{liveAnswer}</>} />
      {sections.compactionBlocks.map(block => <TurnStatusView key={block.id} block={block} />)}
      {(sections.goalBlocks ?? []).map(block => <GoalBar key={block.id} goal={block.goal} operation={block.operation} />)}
      {endCalls.length || !finalBlock && (sections.generatedFileBlocks.length || deliveries) ? <AttachmentDeliveries blocks={!finalBlock || endCalls.length ? sections.generatedFileBlocks : []} present={{ threadId, calls: endCalls }}>{!finalBlock || endCalls.length ? deliveries : null}</AttachmentDeliveries> : null}
      <ScheduleTurnCard threadId={threadId} turnId={turnId} blocks={turn.blocks} ended={!processing && settled} />
      {!processing && status === 'completed' && endReason?.kind === 'max-tokens' ? <TurnMaxTokensItem t={t} /> : null}
      <TurnTailView processing={processing} running={running} startedAt={runningStartedAt} finalBlock={finalBlock}
        ownerKey={threadId && turnId ? JSON.stringify([threadId, turnId]) : undefined}
        feedback={feedback}
        ended={settled && Boolean(threadId && turnId) && finalBlock?.meta?.turnId === turnId}
        billing={processing ? undefined : billing} mode={performanceUsageMode} reveal={hasLaterTranscript ? 'hover' : tailReveal}
        fork={fork ? { ...fork, unavailable: fork.unavailable || hasLaterTranscript || status === 'failed', run: () => {
          if (processing || hasLaterTranscript || !threadId || !turnId || !finalBlock || finalBlock.meta?.turnId !== turnId ||
            (status !== 'completed' && status !== 'aborted')) return
          fork.run({ threadId, turnId, finalItemId: finalBlock.id, status,
            ...(acceptedTerminalBound ? { endSeq: receipt!.lastSeq, proof: { kind: 'accepted' as const, receipt: { ...receipt! }, terminal: { ...terminal! } } }
              : generalTerminalBound ? { endSeq: generalReceipt!.lastSeq, proof: { kind: 'general' as const, receipt: { ...generalReceipt! } } } : { proof: null }) })
        } } : undefined} rollback={rollback}
        hasOutput />
    </div>
    </UserMessageActionsRevealContext.Provider>
    </PresentClosingContext.Provider>
  </ProtectedToolResultScope.Provider>
})
