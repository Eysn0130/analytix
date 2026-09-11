import type { ReactElement } from 'react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCcw } from 'lucide-react'
import type { CoreCheckpointRewindApplyResultJson } from '../agent/analytix-contract'
import type { AgentProvider, ToolBlock } from '../agent/types'
import { getProvider } from '../agent/registry'

type RewindPlan = NonNullable<ToolBlock['meta']>['rewindPlan']
type RewindPlanTranslator = (key: string, options?: Record<string, unknown>) => string
type RewindPlanPrompt = (message: string, defaultValue?: string) => string | null

export async function applyCheckpointRewindPlanWithConfirmation({
  plan,
  provider,
  prompt,
  t
}: {
  plan: RewindPlan
  provider: Pick<AgentProvider, 'applyCheckpointRewind'>
  prompt: RewindPlanPrompt
  t: RewindPlanTranslator
}): Promise<{ result: CoreCheckpointRewindApplyResultJson | null; message: string }> {
  if (!plan || !provider.applyCheckpointRewind) {
    return { result: null, message: t('rewindApplyUnavailable') }
  }
  const phrase = prompt(t('rewindApplyPrompt'), '')
  if (phrase !== 'APPLY_CHECKPOINT_REWIND') {
    return { result: null, message: t('rewindApplyPhraseMismatch') }
  }
  const result = await provider.applyCheckpointRewind(plan.threadId, plan.checkpointId, plan)
  return { result, message: messageForApplyResult(result, t) }
}

export function RewindPlanApplyControls({
  plan,
  compact = false
}: {
  plan: RewindPlan
  compact?: boolean
}): ReactElement | null {
  const { t } = useTranslation('common')
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<CoreCheckpointRewindApplyResultJson | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const disabledReason = useMemo(() => {
    if (!plan) return t('rewindApplyUnavailable')
    if (plan.summary.blockedFileCount > 0 || plan.summary.manualReviewFileCount > 0) {
      return t('rewindApplyNeedsReview')
    }
    if (plan.files.some((file) => isSnapshotBackedRestoreAction(file.action)) &&
      plan.checkpoint.snapshotStorage !== 'runtime_private_cas') {
      return t('rewindApplySnapshotsUnavailable')
    }
    return null
  }, [plan, t])

  if (!plan) return null

  const apply = async (): Promise<void> => {
    const provider = getProvider()
    setRunning(true)
    setMessage(null)
    try {
      const { result: next, message: nextMessage } = await applyCheckpointRewindPlanWithConfirmation({
        plan,
        provider,
        prompt: window.prompt.bind(window),
        t
      })
      setResult(next)
      setMessage(nextMessage)
    } catch (error) {
      setMessage(error instanceof Error ? error.message : String(error))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className={compact ? 'mt-2' : 'mt-3'}>
      <button
        type="button"
        onClick={() => void apply()}
        disabled={running || Boolean(disabledReason)}
        className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-[11px] font-medium transition ${
          disabledReason
            ? 'border-ds-border-muted bg-ds-card-muted text-ds-faint'
            : 'border-ds-border bg-ds-card text-ds-ink hover:bg-ds-hover'
        }`}
        title={disabledReason ?? t('rewindApplyButton')}
      >
        <RotateCcw className="h-3.5 w-3.5" strokeWidth={1.8} />
        {running ? t('rewindApplyRunning') : t('rewindApplyButton')}
      </button>
      <div className="mt-1.5 text-[11px] leading-5 text-ds-muted">
        {message ?? disabledReason ?? t('rewindApplyConfirmHint')}
      </div>
      {result ? (
        <div className="mt-1 text-[10px] leading-4 text-ds-faint">
          {t('rewindApplyResultCounts', {
            applied: result.summary.fileAppliedCount,
            noop: result.summary.fileNoopCount,
            blocked: result.summary.fileBlockedCount + result.summary.fileManualReviewCount + result.summary.fileFailedCount
          })}
        </div>
      ) : null}
    </div>
  )
}

function isSnapshotBackedRestoreAction(action: string): boolean {
  return action === 'restore_previous_version' || action === 'restore_deleted_file'
}

function messageForApplyResult(
  result: CoreCheckpointRewindApplyResultJson,
  t: RewindPlanTranslator
): string {
  switch (result.status) {
    case 'applied':
      return t('rewindApplyApplied')
    case 'already_applied':
      return t('rewindApplyAlready')
    case 'blocked':
      return result.files.find((file) => file.status === 'blocked' || file.status === 'manual_review')?.reason ??
        result.conversation.reason ??
        t('rewindApplyBlocked')
    case 'failed':
      return result.files.find((file) => file.status === 'failed')?.reason ?? t('rewindApplyFailed')
  }
}
