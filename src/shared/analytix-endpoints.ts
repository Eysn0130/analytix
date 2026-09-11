/**
 * Analytix HTTP endpoint path templates. The renderer and the main
 * process IPC allow-list both derive their paths from this table, so
 * adding a new endpoint is a one-file change.
 *
 * `*TEMPLATE` constants carry the `{id}` / `{turn}` placeholders
 * literally. `*PATH(...)` builders perform the URL encoding and
 * return a concrete path for runtime use.
 */

export const ANALYTIX_HEALTH_PATH = '/health'
export const ANALYTIX_HEALTH_TEMPLATE = '/health'

export const ANALYTIX_RUNTIME_INFO_PATH = '/v1/runtime/info'
export const ANALYTIX_RUNTIME_INFO_TEMPLATE = '/v1/runtime/info'

export const ANALYTIX_RUNTIME_TOOLS_PATH = '/v1/runtime/tools'
export const ANALYTIX_RUNTIME_TOOLS_TEMPLATE = '/v1/runtime/tools'

export const ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH = '/v1/runtime/tool-executions/observe'
export const ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_TEMPLATE = '/v1/runtime/tool-executions/observe'

export const ANALYTIX_RUNTIME_TASK_JOBS_LIST_PATH = '/v1/runtime/task-jobs/list'
export const ANALYTIX_RUNTIME_TASK_JOBS_LIST_TEMPLATE = '/v1/runtime/task-jobs/list'
export const ANALYTIX_RUNTIME_TASK_JOBS_WAIT_PATH = '/v1/runtime/task-jobs/wait'
export const ANALYTIX_RUNTIME_TASK_JOBS_WAIT_TEMPLATE = '/v1/runtime/task-jobs/wait'
export const ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_PATH = '/v1/runtime/task-jobs/output'
export const ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_TEMPLATE = '/v1/runtime/task-jobs/output'
export const ANALYTIX_RUNTIME_TASK_JOBS_KILL_PATH = '/v1/runtime/task-jobs/kill'
export const ANALYTIX_RUNTIME_TASK_JOBS_KILL_TEMPLATE = '/v1/runtime/task-jobs/kill'
export const ANALYTIX_RUNTIME_TASK_JOBS_RESTART_PATH = '/v1/runtime/task-jobs/restart'
export const ANALYTIX_RUNTIME_TASK_JOBS_RESTART_TEMPLATE = '/v1/runtime/task-jobs/restart'
export const ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_PATH = '/v1/runtime/task-jobs/recover'
export const ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_TEMPLATE = '/v1/runtime/task-jobs/recover'
export const ANALYTIX_RUNTIME_TASK_JOBS_STEER_PATH = '/v1/runtime/task-jobs/steer'
export const ANALYTIX_RUNTIME_TASK_JOBS_STEER_TEMPLATE = '/v1/runtime/task-jobs/steer'
export const ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_PATH = '/v1/runtime/task-jobs/pause'
export const ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_TEMPLATE = '/v1/runtime/task-jobs/pause'
export const ANALYTIX_RUNTIME_TASK_JOBS_RESUME_PATH = '/v1/runtime/task-jobs/resume'
export const ANALYTIX_RUNTIME_TASK_JOBS_RESUME_TEMPLATE = '/v1/runtime/task-jobs/resume'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_PATH = '/v1/runtime/task-jobs/isolation-review'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_TEMPLATE = '/v1/runtime/task-jobs/isolation-review'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_PATH = '/v1/runtime/task-jobs/isolation-reject'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_TEMPLATE = '/v1/runtime/task-jobs/isolation-reject'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_PATH = '/v1/runtime/task-jobs/isolation-cleanup'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_TEMPLATE = '/v1/runtime/task-jobs/isolation-cleanup'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_PATH = '/v1/runtime/task-jobs/isolation-accept'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_TEMPLATE = '/v1/runtime/task-jobs/isolation-accept'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_PATH = '/v1/runtime/task-jobs/isolation-conflict-report'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_TEMPLATE = '/v1/runtime/task-jobs/isolation-conflict-report'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_PATH = '/v1/runtime/task-jobs/isolation-repair-check'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_TEMPLATE = '/v1/runtime/task-jobs/isolation-repair-check'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_PATH = '/v1/runtime/task-jobs/isolation-repair-accept'
export const ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_TEMPLATE = '/v1/runtime/task-jobs/isolation-repair-accept'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PATH = '/v1/runtime/task-jobs/child-todos'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_TEMPLATE = '/v1/runtime/task-jobs/child-todos'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_PATH = '/v1/runtime/task-jobs/child-todos/project'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_TEMPLATE = '/v1/runtime/task-jobs/child-todos/project'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_PATH = '/v1/runtime/task-jobs/child-todos/reject'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_TEMPLATE = '/v1/runtime/task-jobs/child-todos/reject'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_PATH = '/v1/runtime/task-jobs/child-todos/accept'
export const ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_TEMPLATE = '/v1/runtime/task-jobs/child-todos/accept'

export const ANALYTIX_SKILLS_PATH = '/v1/skills'
export const ANALYTIX_SKILLS_TEMPLATE = '/v1/skills'

export const ANALYTIX_ATTACHMENTS_PATH = '/v1/attachments'
export const ANALYTIX_ATTACHMENTS_TEMPLATE = '/v1/attachments'
export const ANALYTIX_ATTACHMENT_DIAGNOSTICS_PATH = '/v1/attachments/diagnostics'
export const ANALYTIX_ATTACHMENT_DIAGNOSTICS_TEMPLATE = '/v1/attachments/diagnostics'
export const ANALYTIX_ATTACHMENT_TEMPLATE = '/v1/attachments/{id}'
export function analytixAttachmentPath(attachmentId: string): string {
  return `/v1/attachments/${encodeURIComponent(attachmentId)}`
}
export const ANALYTIX_ATTACHMENT_CONTENT_TEMPLATE = '/v1/attachments/{id}/content'
export function analytixAttachmentContentPath(attachmentId: string): string {
  return `${analytixAttachmentPath(attachmentId)}/content`
}

export const ANALYTIX_MEMORY_PATH = '/v1/memory'
export const ANALYTIX_MEMORY_TEMPLATE = '/v1/memory'
export const ANALYTIX_MEMORY_DIAGNOSTICS_PATH = '/v1/memory/diagnostics'
export const ANALYTIX_MEMORY_DIAGNOSTICS_TEMPLATE = '/v1/memory/diagnostics'
export const ANALYTIX_MEMORY_RECORD_TEMPLATE = '/v1/memory/{id}'
export function analytixMemoryRecordPath(memoryId: string): string {
  return `/v1/memory/${encodeURIComponent(memoryId)}`
}

export const ANALYTIX_PROVIDER_REGISTRY_PATH = '/v1/provider-registry'
export const ANALYTIX_PROVIDER_REGISTRY_TEMPLATE = '/v1/provider-registry'
export const ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH =
  '/v1/provider-registry/portable-manifest'
export const ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_TEMPLATE =
  '/v1/provider-registry/portable-manifest'
export const ANALYTIX_PROVIDER_REGISTRY_RECOVER_PATH = '/v1/provider-registry/recover'
export const ANALYTIX_PROVIDER_REGISTRY_RECOVER_TEMPLATE = '/v1/provider-registry/recover'
export const ANALYTIX_PROVIDER_REGISTRY_PROVIDER_TEMPLATE = '/v1/provider-registry/providers/{id}'
export function analytixProviderRegistryProviderPath(providerId: string): string {
  return `${ANALYTIX_PROVIDER_REGISTRY_PATH}/providers/${encodeURIComponent(providerId)}`
}
export const ANALYTIX_PROVIDER_REGISTRY_SELECT_TEMPLATE =
  '/v1/provider-registry/providers/{id}/select'
export function analytixProviderRegistrySelectPath(providerId: string): string {
  return `${analytixProviderRegistryProviderPath(providerId)}/select`
}
export const ANALYTIX_PROVIDER_REGISTRY_DISCONNECT_TEMPLATE =
  '/v1/provider-registry/providers/{id}/disconnect'
export function analytixProviderRegistryDisconnectPath(providerId: string): string {
  return `${analytixProviderRegistryProviderPath(providerId)}/disconnect`
}
export const ANALYTIX_PROVIDER_REGISTRY_CREDENTIAL_TEMPLATE =
  '/v1/provider-registry/providers/{id}/credential'
export function analytixProviderRegistryCredentialPath(providerId: string): string {
  return `${analytixProviderRegistryProviderPath(providerId)}/credential`
}
export const ANALYTIX_PROVIDER_REGISTRY_PROBE_TEMPLATE =
  '/v1/provider-registry/providers/{id}/probe'
export function analytixProviderRegistryProbePath(providerId: string): string {
  return `${analytixProviderRegistryProviderPath(providerId)}/probe`
}
export const ANALYTIX_PROVIDER_REGISTRY_DISCOVER_MODELS_TEMPLATE =
  '/v1/provider-registry/providers/{id}/discover-models'
export function analytixProviderRegistryDiscoverModelsPath(providerId: string): string {
  return `${analytixProviderRegistryProviderPath(providerId)}/discover-models`
}
export const ANALYTIX_PROVIDER_REGISTRY_ACCOUNT_OBSERVATION_TEMPLATE =
  '/v1/provider-registry/providers/{id}/account-observation'
export function analytixProviderRegistryAccountObservationPath(providerId: string): string {
  return `${analytixProviderRegistryProviderPath(providerId)}/account-observation`
}

export const ANALYTIX_WORKSPACE_STATUS_PATH = '/v1/workspace/status'
export const ANALYTIX_WORKSPACE_STATUS_TEMPLATE = '/v1/workspace/status'
export function analytixWorkspaceStatusPath(workspacePath?: string): string {
  if (!workspacePath) return ANALYTIX_WORKSPACE_STATUS_PATH
  return `${ANALYTIX_WORKSPACE_STATUS_PATH}?path=${encodeURIComponent(workspacePath)}`
}

export const ANALYTIX_CASE_PROJECTS_PATH = '/v1/case-projects'
export const ANALYTIX_CASE_PROJECTS_TEMPLATE = '/v1/case-projects'
export const ANALYTIX_CASE_PROJECT_THREADS_TEMPLATE = '/v1/case-projects/{id}/threads'
export function analytixCaseProjectThreadsPath(caseProjectId: string): string {
  return `${ANALYTIX_CASE_PROJECTS_PATH}/${encodeURIComponent(caseProjectId)}/threads`
}
export const ANALYTIX_CASE_PROJECT_DETAIL_TEMPLATE = '/v1/case-projects/{id}/detail'
export function analytixCaseProjectDetailPath(caseProjectId: string): string {
  return `${ANALYTIX_CASE_PROJECTS_PATH}/${encodeURIComponent(caseProjectId)}/detail`
}

export const ANALYTIX_THREADS_PATH = '/v1/threads'
export const ANALYTIX_THREADS_TEMPLATE = '/v1/threads'

export const ANALYTIX_THREAD_TEMPLATE = '/v1/threads/{id}'
export function analytixThreadPath(threadId: string): string {
  return `/v1/threads/${encodeURIComponent(threadId)}`
}

export const ANALYTIX_THREAD_SUMMARY_TEMPLATE = '/v1/threads/{id}/summary'
export function analytixThreadSummaryPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/summary`
}

export const ANALYTIX_THREAD_SUMMARY_TASK_OUTPUT_TEMPLATE = '/v1/threads/{id}/summary/tasks/{task}/output'
export function analytixThreadSummaryTaskOutputPath(threadId: string, taskId: string): string {
  return `${analytixThreadSummaryPath(threadId)}/tasks/${encodeURIComponent(taskId)}/output`
}

export const ANALYTIX_THREAD_SUMMARY_TASK_KILL_TEMPLATE = '/v1/threads/{id}/summary/tasks/{task}/kill'
export function analytixThreadSummaryTaskKillPath(threadId: string, taskId: string): string {
  return `${analytixThreadSummaryPath(threadId)}/tasks/${encodeURIComponent(taskId)}/kill`
}

export const ANALYTIX_THREAD_SUMMARY_TASK_RESTART_TEMPLATE = '/v1/threads/{id}/summary/tasks/{task}/restart'
export function analytixThreadSummaryTaskRestartPath(threadId: string, taskId: string): string {
  return `${analytixThreadSummaryPath(threadId)}/tasks/${encodeURIComponent(taskId)}/restart`
}

export const ANALYTIX_THREAD_FORK_TEMPLATE = '/v1/threads/{id}/fork'
export function analytixThreadForkPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/fork`
}

export const ANALYTIX_THREAD_GOAL_TEMPLATE = '/v1/threads/{id}/goal'
export function analytixThreadGoalPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/goal`
}

export const ANALYTIX_THREAD_TODOS_TEMPLATE = '/v1/threads/{id}/todos'
export function analytixThreadTodosPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/todos`
}

export const ANALYTIX_THREAD_COMPACT_TEMPLATE = '/v1/threads/{id}/compact'
export function analytixThreadCompactPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/compact`
}

export const ANALYTIX_THREAD_REVIEW_TEMPLATE = '/v1/threads/{id}/review'
export function analytixThreadReviewPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/review`
}

export const ANALYTIX_THREAD_CHECKPOINT_REWIND_PLAN_TEMPLATE =
  '/v1/threads/{id}/checkpoints/{checkpoint}/rewind-plan'
export function analytixThreadCheckpointRewindPlanPath(threadId: string, checkpointId: string): string {
  return `${analytixThreadPath(threadId)}/checkpoints/${encodeURIComponent(checkpointId)}/rewind-plan`
}

export const ANALYTIX_THREAD_CHECKPOINT_REWIND_APPLY_TEMPLATE =
  '/v1/threads/{id}/checkpoints/{checkpoint}/rewind-apply'
export function analytixThreadCheckpointRewindApplyPath(threadId: string, checkpointId: string): string {
  return `${analytixThreadPath(threadId)}/checkpoints/${encodeURIComponent(checkpointId)}/rewind-apply`
}

export const ANALYTIX_THREAD_TURNS_TEMPLATE = '/v1/threads/{id}/turns'
export function analytixThreadTurnsPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/turns`
}

export const ANALYTIX_THREAD_REWIND_TEMPLATE = '/v1/threads/{id}/rewind'
export function analytixThreadRewindPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/rewind`
}

export const ANALYTIX_THREAD_STEER_TEMPLATE = '/v1/threads/{id}/turns/{turn}/steer'
export function analytixThreadSteerPath(threadId: string, turnId: string): string {
  return `${analytixThreadTurnsPath(threadId)}/${encodeURIComponent(turnId)}/steer`
}

export const ANALYTIX_THREAD_INTERRUPT_TEMPLATE = '/v1/threads/{id}/turns/{turn}/interrupt'
export function analytixThreadInterruptPath(threadId: string, turnId: string): string {
  return `${analytixThreadTurnsPath(threadId)}/${encodeURIComponent(turnId)}/interrupt`
}

export const ANALYTIX_THREAD_EVENTS_TEMPLATE = '/v1/threads/{id}/events'
export function analytixThreadEventsPath(threadId: string): string {
  return `${analytixThreadPath(threadId)}/events`
}

export const ANALYTIX_APPROVAL_TEMPLATE = '/v1/approvals/{id}'
export function analytixApprovalPath(approvalId: string): string {
  return `/v1/approvals/${encodeURIComponent(approvalId)}`
}

export const ANALYTIX_USER_INPUT_TEMPLATE = '/v1/user-inputs/{id}'
export function analytixUserInputPath(inputId: string): string {
  return `/v1/user-inputs/${encodeURIComponent(inputId)}`
}

export const ANALYTIX_SESSION_RESUME_TEMPLATE = '/v1/sessions/{id}/resume-thread'
export function analytixSessionResumePath(sessionId: string): string {
  return `/v1/sessions/${encodeURIComponent(sessionId)}/resume-thread`
}

export const ANALYTIX_USAGE_PATH = '/v1/usage'
export const ANALYTIX_USAGE_TEMPLATE = '/v1/usage'

export const ANALYTIX_DEBUG_LLM_ROUNDS_PATH = '/v1/debug/llm-rounds'
export const ANALYTIX_DEBUG_LLM_ROUNDS_TEMPLATE = '/v1/debug/llm-rounds'

/** Thread mode shared with the Analytix contract. */
export type AnalytixThreadMode = 'agent' | 'plan'

const THREAD_MODES: ReadonlySet<AnalytixThreadMode> = new Set<AnalytixThreadMode>(['agent', 'plan'])

export function isAnalytixThreadMode(value: unknown): value is AnalytixThreadMode {
  return typeof value === 'string' && (THREAD_MODES as Set<string>).has(value)
}

export function normalizeThreadMode(value: unknown): AnalytixThreadMode {
  return value === 'plan' ? 'plan' : 'agent'
}
