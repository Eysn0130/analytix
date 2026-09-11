import type { WorktreeInfo } from './worktree'

export type ThreadHandoffDirection = 'to-worktree' | 'to-local' | 'to-host-worktree'

export type ThreadHandoffStatus = 'queued' | 'running' | 'success' | 'warning' | 'error'

export type ThreadHandoffStepStatus = 'pending' | 'running' | 'done' | 'failed'

export type ThreadHandoffStepId =
  | 'prepare-host-transfer'
  | 'transfer-host-artifacts'
  | 'create-new-worktree'
  | 'reuse-existing-worktree'
  | 'stash-source-changes'
  | 'checkout-local-branch'
  | 'stash-target-worktree-changes'
  | 'checkout-worktree-branch'
  | 'detach-worktree-branch'
  | 'apply-changes-to-worktree'
  | 'apply-changes-to-local'
  | 'switching-thread'
  | 'rolling-back-changes'

export type ThreadHandoffStep = {
  id: ThreadHandoffStepId
  status: ThreadHandoffStepStatus
}

export type ThreadHandoffExecOutput = {
  command?: string
  output: string
}

export type ThreadHandoffRequest =
  | {
      direction: 'to-worktree'
      sourceThreadId: string
      sourceWorkspace: string
      sourceBranch?: string
      localBranch?: string
      worktreeBranch?: string
      worktreeRoot?: string
    }
  | {
      direction: 'to-local'
      sourceThreadId: string
      sourceWorkspace: string
      localWorkspace: string
      projectPath: string
      poolIndex?: number
      sourceBranch?: string
      localBranch?: string
      worktreeRoot?: string
    }
  | {
      direction: 'to-host-worktree'
      sourceThreadId: string
      sourceWorkspace: string
      destinationHostId: string
      destinationLabel: string
      destinationWorkspaceRoot: string
      sourceBranch?: string
      worktreeBranch?: string
    }

export type ThreadHandoffOperation = {
  id: string
  direction: ThreadHandoffDirection
  status: ThreadHandoffStatus
  sourceThreadId: string
  targetThreadId: string | null
  sourceWorkspace: string
  targetWorkspace: string | null
  sourceBranch: string | null
  localBranch: string | null
  worktreeBranch: string | null
  request: ThreadHandoffRequest
  steps: ThreadHandoffStep[]
  errorMessage: string | null
  warningMessage: string | null
  execOutput: ThreadHandoffExecOutput | null
  hasUnseenTerminalState: boolean
  worktree: WorktreeInfo | null
  createdAt: string
  updatedAt: string
}

export type ThreadHandoffEvent = {
  operation: ThreadHandoffOperation
}

export type ThreadHandoffStartResult = {
  operation: ThreadHandoffOperation
}

export type ThreadHandoffCompleteSwitchRequest = {
  operationId: string
  targetThreadId?: string
}

export function threadHandoffInitialStepIds(
  request: ThreadHandoffRequest,
  existingWorktree = false
): ThreadHandoffStepId[] {
  if (request.direction === 'to-host-worktree') {
    return [
      'prepare-host-transfer',
      'transfer-host-artifacts',
      existingWorktree ? 'reuse-existing-worktree' : 'create-new-worktree',
      'apply-changes-to-worktree',
      'switching-thread'
    ]
  }
  if (request.direction === 'to-local') {
    return [
      'stash-source-changes',
      'detach-worktree-branch',
      'checkout-local-branch',
      'apply-changes-to-local',
      'switching-thread'
    ]
  }
  return [
    existingWorktree ? 'reuse-existing-worktree' : 'create-new-worktree',
    'stash-source-changes',
    'checkout-local-branch',
    'stash-target-worktree-changes',
    'checkout-worktree-branch',
    'apply-changes-to-worktree',
    'switching-thread'
  ]
}

export function threadHandoffStepsWithRollback(
  steps: ThreadHandoffStep[]
): ThreadHandoffStep[] {
  const failedIndex = steps.findIndex((step) => step.status === 'failed')
  if (failedIndex === -1) return steps
  const visible = steps.slice(0, failedIndex + 1)
  return failedIndex === steps.length - 1
    ? visible
    : [...visible, { id: 'rolling-back-changes', status: 'running' }]
}
