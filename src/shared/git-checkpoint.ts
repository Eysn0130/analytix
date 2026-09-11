export type GitCheckpointCreateResult =
  | {
      ok: true
      checkpointId: string
      repositoryRoot: string
      head: string
      currentBranch: string | null
      partial?: boolean
      skippedUntracked?: string[]
    }
  | {
      ok: false
      reason:
        | 'no_workspace'
        | 'not_git_repo'
        | 'git_unavailable'
        | 'conflict'
        | 'too_large'
        | 'partial'
        | 'timeout'
        | 'error'
      message: string
    }

export type GitCheckpointRestoreResult =
  | {
      ok: true
      checkpointId: string
      repositoryRoot: string
      head: string
      currentBranch: string | null
      rescueCheckpointId: string | null
      partial?: boolean
      skippedUntracked?: string[]
    }
  | {
      ok: false
      reason:
        | 'no_workspace'
        | 'not_git_repo'
        | 'git_unavailable'
        | 'not_found'
        | 'conflict'
        | 'partial'
        | 'error'
      message: string
      skippedUntracked?: string[]
    }
