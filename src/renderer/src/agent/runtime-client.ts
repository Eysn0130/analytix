import type { AppSettingsPatch, AppSettingsV1 } from '@shared/app-settings'
import type {
  AnalytixRuntimeApi,
  AnalytixSettingsApi,
  AcceptedFinalSseAckBindingV1,
  RuntimeRequestResult,
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

class RendererRuntimeClient {
  private settingsApi(): Pick<AnalytixSettingsApi, 'getSettings' | 'setSettings'> {
    return window.analytix.settings
  }

  private runtimeApi(): Pick<
    AnalytixRuntimeApi,
    | 'runtimeRequest'
    | 'restartRuntime'
    | 'startSse'
    | 'ackSseEvent'
    | 'stopSse'
    | 'onSseEvent'
    | 'onSseEnd'
    | 'onSseError'
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

  async setSettings(partial: AppSettingsPatch): Promise<AppSettingsV1> {
    const settings = await this.settingsApi().setSettings(partial)
    desktopQueryCache.setQueryData(SETTINGS_QUERY_KEY, settings, SETTINGS_STALE_TIME_MS)
    desktopQueryCache.broadcastInvalidation(SETTINGS_QUERY_KEY)
    return settings
  }

  invalidateSettings(): void {
    desktopQueryCache.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY, exact: true })
  }

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

  steerTaskJob(body: Record<string, unknown>): Promise<RuntimeRequestResult> {
    return this.runtimeRequest(ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH, 'POST', JSON.stringify(body))
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

  startSse(threadId: string, sinceSeq: number, streamId?: string): Promise<{ streamId: string }> {
    return this.runtimeApi().startSse(threadId, sinceSeq, streamId)
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
