import type { AppSettingsPatch, AppSettingsV1 } from '@shared/app-settings'
import {
  childInputRequestSchemaV1, childInputResponseSchemaV1, childInputReceiptMatchesV1,
  type ChildInputRequestV1, type ChildInputSubmitRequestV1, type ChildInputResponseV1, type ChildInputReceiptV1
} from '@shared/child-input-contract'
import type {
  AnalytixRuntimeApi,
  AnalytixSettingsApi,
  AcceptedFinalSseAckBindingV1,
  RuntimeRequestResult,
  RuntimeSseReplayOptionsV1,
  SseEndPayload,
  SseErrorPayload,
  SseEventPayload
} from '@shared/analytix-api'
import {
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_KILL_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_LIST_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_RESTART_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_RESUME_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH,
  ANALYTIX_RUNTIME_TASK_JOBS_WAIT_PATH
} from '@shared/analytix-endpoints'
import { desktopQueryCache } from '../lib/desktop-query-cache'

const SETTINGS_QUERY_KEY = ['settings'] as const
const SETTINGS_STALE_TIME_MS = 5 * 60 * 1000

function decodeChildInputResponse(body: string): ChildInputResponseV1 {
  try {
    const parsed = childInputResponseSchemaV1.safeParse(JSON.parse(body) as unknown)
    if (parsed.success) return parsed.data
  } catch { /* Keep untrusted response contents out of UI errors. */ }
  throw new Error('Child message response was not confirmed.')
}

export class ChildInputRejectedError extends Error {
  constructor(readonly code: string) { super('Child message was rejected. The draft has been kept.') }
}

class RendererRuntimeClient {
  private settingsApi(): Pick<AnalytixSettingsApi, 'getSettings' | 'setSettings'> {
    return window.analytix.settings
  }

  private runtimeApi(): Pick<
    AnalytixRuntimeApi,
    | 'openUserInputWait'
    | 'closeUserInputWait'
    | 'onUserInputWaitChanged'
    | 'runtimeRequest'
    | 'restartRuntime'
    | 'startSse'
    | 'ackSseEvent'
    | 'stopSse'
    | 'onSseEvent'
    | 'onSseEnd'
    | 'onSseError'
    | 'onRuntimeStatus'
  > {
    return window.analytix.runtime
  }

  async getSettings(options?: { forceRefresh?: boolean }): Promise<AppSettingsV1> {
    if (options?.forceRefresh) {
      this.invalidateSettings()
    }
    return desktopQueryCache.fetchQuery({
      queryKey: SETTINGS_QUERY_KEY,
      staleTimeMs: SETTINGS_STALE_TIME_MS,
      fetcher: () => this.settingsApi().getSettings()
    })
  }

  // Recovery callers own readback adoption; leave cached reads and writes untouched.
  readSettingsFromHost(): Promise<AppSettingsV1> {
    return this.settingsApi().getSettings()
  }

  async setSettings(partial: AppSettingsPatch): Promise<AppSettingsV1> {
    const settings = await this.settingsApi().setSettings(partial)
    desktopQueryCache.setQueryData(SETTINGS_QUERY_KEY, settings, SETTINGS_STALE_TIME_MS)
    desktopQueryCache.broadcastInvalidation(SETTINGS_QUERY_KEY)
    return settings
  }

  invalidateSettings(): void {
    desktopQueryCache.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY, exact: true })
  }

  openUserInputWait: AnalytixRuntimeApi['openUserInputWait'] = selector => this.runtimeApi().openUserInputWait(selector)
  closeUserInputWait: AnalytixRuntimeApi['closeUserInputWait'] = selector => this.runtimeApi().closeUserInputWait(selector)
  onUserInputWaitChanged: AnalytixRuntimeApi['onUserInputWaitChanged'] = handler => this.runtimeApi().onUserInputWaitChanged(handler)
  onRuntimeStatus: AnalytixRuntimeApi['onRuntimeStatus'] = handler => this.runtimeApi().onRuntimeStatus(handler)

  runtimeRequest(path: string, method?: string, body?: string): Promise<RuntimeRequestResult> {
    if (body === undefined) {
      if (method === undefined) return this.runtimeApi().runtimeRequest(path)
      return this.runtimeApi().runtimeRequest(path, method)
    }
    return this.runtimeApi().runtimeRequest(path, method, body)
  }

  listTaskJobs(body: Record<string, unknown> = {}): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_LIST_PATH, 'POST', JSON.stringify(body))
  }

  waitTaskJobs(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_WAIT_PATH, 'POST', JSON.stringify(body))
  }

  outputTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_PATH, 'POST', JSON.stringify(body))
  }

  restartTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_RESTART_PATH, 'POST', JSON.stringify(body))
  }

  recoverTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_PATH, 'POST', JSON.stringify(body))
  }

  steerTaskJob(body: ChildInputRequestV1): Promise<RuntimeRequestResult> {
    const request = childInputRequestSchemaV1.parse(body)
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH, 'POST', JSON.stringify(request))
  }

  async inspectChildInput(threadId: string, jobId: string, childThreadId?: string): Promise<ChildInputResponseV1['capabilities']> {
    const response = await this.steerTaskJob({ mode: 'inspect', threadId, jobId })
    const parsed = decodeChildInputResponse(response.body)
    if (!response.ok || response.status !== 200 || parsed.code || parsed.receipt ||
        childThreadId && parsed.capabilities.childThreadId !== childThreadId) {
      throw new Error('Child input inspection was not confirmed.')
    }
    return parsed.capabilities
  }

  async submitChildInput(request: ChildInputSubmitRequestV1, childThreadId: string): Promise<ChildInputReceiptV1> {
    const response = await this.steerTaskJob(request)
    const parsed = decodeChildInputResponse(response.body)
    const rejectionStatus: Record<string, number> = {
      validation_error: 400, forbidden: 403, not_found: 404,
      conflict: 409, new_turn_required: 409, input_cancelled: 409
    }
    if (!response.ok && parsed.code && rejectionStatus[parsed.code] === response.status && !parsed.receipt) {
      throw new ChildInputRejectedError(parsed.code)
    }
    if (!response.ok || response.status !== 200 || parsed.code || !parsed.receipt ||
        !childInputReceiptMatchesV1(parsed.receipt, request, childThreadId)) {
      throw new Error('Child message was not confirmed. Retry the same message to check its admission.')
    }
    return parsed.receipt
  }

  killTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_KILL_PATH, 'POST', JSON.stringify(body))
  }

  pauseTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_PATH, 'POST', JSON.stringify(body))
  }

  resumeTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_RESUME_PATH, 'POST', JSON.stringify(body))
  }

  reviewTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_PATH, 'POST', JSON.stringify(body))
  }

  rejectTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_PATH, 'POST', JSON.stringify(body))
  }

  cleanupTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_PATH, 'POST', JSON.stringify(body))
  }

  acceptTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_PATH, 'POST', JSON.stringify(body))
  }

  conflictReportTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_PATH, 'POST', JSON.stringify(body))
  }

  repairCheckTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_PATH, 'POST', JSON.stringify(body))
  }

  repairAcceptTaskJobIsolation(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_PATH, 'POST', JSON.stringify(body))
  }

  childTodosTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PATH, 'POST', JSON.stringify(body))
  }

  projectChildTodosTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_PATH, 'POST', JSON.stringify(body))
  }

  rejectChildTodoProjectionTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_PATH, 'POST', JSON.stringify(body))
  }

  acceptChildTodoProjectionTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_PATH, 'POST', JSON.stringify(body))
  }

  restartRuntime(): Promise<void> {
    return this.runtimeApi().restartRuntime()
  }

  startSse(threadId: string, sinceSeq: number, streamId?: string, replay?: RuntimeSseReplayOptionsV1): Promise<{ streamId: string }> {
    return replay
      ? this.runtimeApi().startSse(threadId, sinceSeq, streamId, replay)
      : this.runtimeApi().startSse(threadId, sinceSeq, streamId)
  }

  ackSseEvent(
    streamId: string,
    seq: number,
    acceptedFinal?: AcceptedFinalSseAckBindingV1
  ): Promise<boolean> {
    return acceptedFinal
      ? this.runtimeApi().ackSseEvent(streamId, seq, acceptedFinal)
      : this.runtimeApi().ackSseEvent(streamId, seq)
  }

  stopSse(streamId: string): Promise<boolean> {
    return this.runtimeApi().stopSse(streamId)
  }

  onSseEvent(handler: (payload: SseEventPayload) => void): () => void {
    return this.runtimeApi().onSseEvent(handler)
  }

  onSseEnd(handler: (payload: SseEndPayload) => void): () => void {
    return this.runtimeApi().onSseEnd(handler)
  }

  onSseError(handler: (payload: SseErrorPayload) => void): () => void {
    return this.runtimeApi().onSseError(handler)
  }
}

export const rendererRuntimeClient = new RendererRuntimeClient()
