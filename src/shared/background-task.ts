export type BackgroundTaskStatus =
  | 'registered'
  | 'starting'
  | 'running'
  | 'completed'
  | 'failed'
  | 'stopped'
  | 'missing'
  | 'killed'

export type BackgroundTaskSource = 'summary-command' | 'manual' | 'restored-process'

export type BackgroundTaskRecord = {
  id: string
  threadId: string
  turnId?: string
  itemId?: string
  pid?: number | null
  processId?: string
  attemptId?: string
  status: BackgroundTaskStatus
  source: BackgroundTaskSource
  outputBytes?: number
  startedAt?: string
  updatedAt: string
  finishedAt?: string
  canKill: boolean
  canRestart: boolean
  canReadOutput: boolean
}

export type BackgroundTaskRegisterPayload = {
  record: Omit<BackgroundTaskRecord, 'updatedAt' | 'canKill' | 'canRestart' | 'canReadOutput'> & {
    updatedAt?: string
  }
}

export type BackgroundTaskThreadPayload = {
  threadId: string
}

export type BackgroundTaskActionPayload = {
  threadId: string
  taskId: string
}

export type BackgroundTaskOutputPayload = BackgroundTaskActionPayload & {
  offset?: number
  limit?: number
}

export type BackgroundTaskListResult = {
  tasks: BackgroundTaskRecord[]
}

export type BackgroundTaskMutationResult =
  | { ok: true; task: BackgroundTaskRecord }
  | { ok: false; message: string }

export type BackgroundTaskOutputResult =
  | {
      ok: true
      availability: 'withheld'
      reasonCode: 'electron_background_tasks_retired'
      outputWithheld: true
      canReadOutput: false
      factAnswerAllowed: false
      evidenceAuthority: false
    }
  | { ok: false; message: string }
