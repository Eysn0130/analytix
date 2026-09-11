import { type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThreadHandoffDirection, ThreadHandoffOperation, ThreadHandoffStep } from '@shared/thread-handoff'
import { ProgressStepRow } from './ProgressStepRow'

type Props = {
  compact?: boolean
  direction: ThreadHandoffDirection
  localBranch?: string | null
  sourceBranch?: string | null
  step: ThreadHandoffStep
  worktreeBranch?: string | null
}

function stepLabel(
  t: ReturnType<typeof useTranslation>['t'],
  stepId: ThreadHandoffStep['id'],
  direction: ThreadHandoffDirection,
  localBranch?: string | null,
  sourceBranch?: string | null,
  worktreeBranch?: string | null
): string {
  switch (stepId) {
    case 'rolling-back-changes':
      return t('threadHandoffStepRollingBackChanges')
    case 'prepare-host-transfer':
      return t('threadHandoffStepPrepareHostTransfer')
    case 'transfer-host-artifacts':
      return t('threadHandoffStepTransferHostArtifacts')
    case 'create-new-worktree':
      return t('threadHandoffStepCreateNewWorktree')
    case 'reuse-existing-worktree':
      return t('threadHandoffStepReuseExistingWorktree')
    case 'stash-source-changes':
      return t('threadHandoffStepStashSourceChanges')
    case 'checkout-local-branch':
      return t('threadHandoffStepCheckoutLocalBranch', { branch: localBranch || sourceBranch || t('threadHandoffBranchFallback') })
    case 'stash-target-worktree-changes':
      return t('threadHandoffStepStashTargetWorktreeChanges')
    case 'checkout-worktree-branch':
      return t('threadHandoffStepCheckoutWorktreeBranch', { branch: worktreeBranch || sourceBranch || t('threadHandoffBranchFallback') })
    case 'detach-worktree-branch':
      return t('threadHandoffStepDetachWorktreeBranch')
    case 'apply-changes-to-worktree':
      return t('threadHandoffStepApplyChangesToWorktree')
    case 'apply-changes-to-local':
      return t('threadHandoffStepApplyChangesToLocal')
    case 'switching-thread':
      if (direction === 'to-worktree') return t('threadHandoffStepMoveThreadToWorktree')
      if (direction === 'to-host-worktree') return t('threadHandoffStepMoveThreadToHostWorktree')
      return t('threadHandoffStepMoveThreadToLocal')
  }
}

export function ThreadHandoffStepRow({
  compact = false,
  direction,
  localBranch,
  sourceBranch,
  step,
  worktreeBranch
}: Props): ReactElement {
  const { t } = useTranslation('common')
  return (
    <ProgressStepRow compact={compact} status={step.status}>
      {stepLabel(t, step.id, direction, localBranch, sourceBranch, worktreeBranch)}
    </ProgressStepRow>
  )
}

export function ThreadHandoffOperationStepRow({
  compact = false,
  operation,
  step
}: {
  compact?: boolean
  operation: ThreadHandoffOperation
  step: ThreadHandoffStep
}): ReactElement {
  return (
    <ThreadHandoffStepRow
      compact={compact}
      direction={operation.direction}
      localBranch={operation.localBranch}
      sourceBranch={operation.sourceBranch}
      step={step}
      worktreeBranch={operation.worktreeBranch}
    />
  )
}
