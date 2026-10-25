/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { PublicToolResultProjectionV1, type PublicTaskJobResultDataV1 } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ToolResultLocalGoalDataV1, ToolResultLocalJobDataV1, ToolResultLocalSkillDataV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { TaskJobStatusV1, TaskJobSummaryV1 } from '../../../../../../../../packages/runtime/src/contracts/task-job-output'
import type { ToolBlock } from '../../../../../agent/types'
import type { ToolDetailItem, ToolDetailsModel } from './details-card-model'
export type RecordedToolTranslate = (key: string, options?: Record<string, unknown>) => string
export const recordedJobNames = { list_jobs: 'list', bash_output: 'output', kill_shell: 'kill', wait: 'wait', restart_job: 'restart' } as const
export type RecordedJobName = keyof typeof recordedJobNames
function statusBadge(status: TaskJobStatusV1, t: RecordedToolTranslate): NonNullable<ToolDetailItem['badge']> {
  return { label: t('dshRecordedTool.jobStatus.' + status), tone: status === 'completed' ? 'success'
    : status === 'failed' ? 'error' : ['aborted', 'interrupted', 'killed', 'canceled', 'timeout'].includes(status) ? 'warning'
      : ['queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming'].includes(status) ? 'info' : 'neutral' }
}
function jobItem(job: TaskJobSummaryV1, t: RecordedToolTranslate): ToolDetailItem {
  return { id: job.id, title: job.id, badge: statusBadge(job.status, t), fields: [
    { label: t('dshRecordedTool.job.kind'), value: t('dshRecordedTool.jobKind.' + job.kind) },
    { label: t('dshRecordedTool.job.background'), value: t('dshRecordedTool.' + (job.background ? 'yes' : 'no')) }
  ], description: t('dshRecordedTool.job.outputWithheld') }
}
/** Only the admitted successful, exact-name command result selects Job details. */
export function recordedJobData(block: ToolBlock): PublicTaskJobResultDataV1 | null {
  const p = PublicToolResultProjectionV1.safeParse(block.meta?.publicResult)
  const name = block.meta?.toolName
  if (!p.success || p.data.projectionKind !== 'task_job_result' || block.status !== 'success' ||
      block.toolKind !== 'command_execution' || typeof name !== 'string' || !Object.hasOwn(recordedJobNames, name) ||
      recordedJobNames[name as RecordedJobName] !== p.data.taskJob.operation ||
      typeof block.meta?.callId !== 'string' || !/^call_host_[a-f0-9]{64}$/.test(block.meta.callId) ||
      typeof block.meta?.sourceItemId !== 'string' || !/^item_result_[a-f0-9]{64}$/.test(block.meta.sourceItemId)) return null
  return p.data.taskJob
}
export function recordedJobCardModel(data: PublicTaskJobResultDataV1, t: RecordedToolTranslate): ToolDetailsModel {
  if (data.operation === 'output') {
    const result = data.result
    return { summary: result.jobId + ' · ' + statusBadge(result.status, t).label,
      items: [{ id: result.jobId, title: result.jobId, badge: statusBadge(result.status, t), fields: [], description: t('dshRecordedTool.job.outputWithheld') }] }
  }
  const jobs = data.operation === 'kill' ? [data.result.job] : data.result.jobs
  return { summary: data.operation === 'kill' ? jobs[0].id + ' · ' + statusBadge(jobs[0].status, t).label
    : t('dshRecordedTool.job.count', { count: jobs.length }), items: jobs.map(job => jobItem(job, t)), empty: t('dshRecordedTool.job.empty') }
}
/** Private Job facts and operation receipts are independent observations. */
export function protectedJobCardModel(data: ToolResultLocalJobDataV1, t: RecordedToolTranslate): ToolDetailsModel {
  const unavailable = t('dshToolInspect.unavailable')
  const items = data.jobs.map(job => ({ id: job.jobId, title: job.name ?? job.label ?? job.jobId,
    badge: statusBadge(job.status, t), fields: [
      { label: t('dshRecordedTool.job.name'), value: job.name ?? unavailable },
      { label: t('dshRecordedTool.job.label'), value: job.label ?? unavailable },
      { label: t('dshRecordedTool.job.id'), value: job.jobId },
      { label: t('dshRecordedTool.job.kind'), value: t('dshRecordedTool.jobKind.' + job.kind) }
    ] }))
  const receipt = data.receipt ? t('dshRecordedTool.job.receipt.' + data.receipt.operation + '.' + data.receipt.status)
    : data.operation === 'cancel' || data.operation === 'wait' ? t('dshRecordedTool.job.receiptUnavailable') : undefined
  return { items, summary: t('dshRecordedTool.job.count', { count: data.total }), expandedSummary: receipt,
    empty: t('dshRecordedTool.job.empty'), caption: [receipt,
      data.truncated ? t('dshRecordedTool.job.listTruncated', { count: items.length, total: data.total }) : undefined].filter(Boolean).join(' · ') || undefined }
}
/** Goal phase is native status. Absent DSH rounds/revision are never reconstructed. */
export function goalCardModel(data: ToolResultLocalGoalDataV1, t: RecordedToolTranslate): ToolDetailsModel {
  const goal = data.goal
  if (!goal) return { summary: t('dshRecordedTool.goal.empty'), items: [], empty: t('dshRecordedTool.goal.empty') }
  const fields = [
    { label: t('dshRecordedTool.goal.status'), value: t('dshRecordedTool.goalStatus.' + goal.status) },
    { label: t('dshRecordedTool.goal.tokensUsed'), value: String(goal.tokensUsed) },
    { label: t('dshRecordedTool.goal.timeUsed'), value: t('dshRecordedTool.goal.seconds', { count: goal.timeUsedSeconds }) }
  ]
  if (goal.id !== undefined) fields.unshift({ label: t('dshRecordedTool.goal.id'), value: goal.id })
  if (typeof goal.tokenBudget === 'number') fields.push({ label: t('dshRecordedTool.goal.budget'), value: String(goal.tokenBudget) })
  if (data.remainingTokens !== null) fields.push({ label: t('dshRecordedTool.goal.remaining'), value: String(data.remainingTokens) })
  if (goal.blockedReason !== undefined) fields.push({ label: t('dshRecordedTool.goal.reason'), value: goal.blockedReason })
  return { summary: goal.objective, items: [{ id: goal.id, title: goal.objective, badge: {
    label: t('dshRecordedTool.goalStatus.' + goal.status), tone: goal.status === 'complete' ? 'success' : goal.status === 'active' ? 'info' : 'warning'
  }, fields }] }
}
export function skillCardModel(data: ToolResultLocalSkillDataV1, body: string, t: RecordedToolTranslate) {
  const name = (data.resolved?.name ?? data.requestedName).split(/\r\n|\r|\n/)[0]
  const error = data.error?.message
  const summary = error === undefined ? name : error.split(/\r\n|\r|\n/)[0]
  const fields = data.resolved ? [{ label: t('dshRecordedTool.skill.id'), value: data.resolved.id },
    { label: t('dshRecordedTool.skill.runAs'), value: t('dshRecordedTool.skill.' + data.resolved.runAs) }] : []
  if (data.child) fields.push({ label: t('dshRecordedTool.job.id'), value: data.child.jobId },
    { label: t('dshRecordedTool.goal.status'), value: statusBadge(data.child.status, t).label })
  return { summary, instructions: data.resolved?.runAs === 'inline' ? body : null,
    details: { items: [{ title: name, fields, ...(data.error ? { description: data.error.message } : {}),
      ...(data.child ? { subtitle: t('dshRecordedTool.job.outputWithheld') } : {}) }] } satisfies ToolDetailsModel }
}
