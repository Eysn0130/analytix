import { z } from 'zod'
import type { DurableTaskJobManager } from '../../delegation-test-support/job-manager.js'
import { jsonResponse, type JsonResponse } from '../response.js'
import { readJsonBody } from '../read-json-body.js'
import { ERRORS } from './runtime-error.js'
import {
  TaskJobKillResponseV1Schema,
  TaskJobOutputResponseV1Schema,
  TaskJobWaitResponseV1Schema,
  taskJobSummaryV1
} from '../../contracts/task-job-output.js'

const WaitTaskJobsRequest = z.object({
  jobIds: z.array(z.string().min(1)).default([]),
  threadId: z.string().min(1),
  timeoutMs: z.number().int().min(0).max(60_000).optional()
}).strict()

const ReadTaskJobOutputRequest = z.object({
  jobId: z.string().min(1),
  threadId: z.string().min(1),
  offset: z.number().int().nonnegative().optional(),
  limit: z.number().int().nonnegative().optional()
}).strict()

const KillTaskJobRequest = z.object({
  jobId: z.string().min(1),
  threadId: z.string().min(1),
  reason: z.string().min(1).optional()
}).strict()

export async function waitTaskJobs(
  manager: DurableTaskJobManager | undefined,
  request: Request
): Promise<JsonResponse> {
  if (!manager) return ERRORS.unavailable('task jobs are not available')
  const body = await readJsonBody(request)
  if (!body.ok) return body.response
  const parsed = WaitTaskJobsRequest.safeParse(body.value)
  if (!parsed.success) return ERRORS.validation('invalid task job wait request', parsed.error.flatten())
  const result = await manager.waitMetadataForParent(parsed.data.jobIds, {
    timeoutMs: parsed.data.timeoutMs,
    parentThreadId: parsed.data.threadId
  })
  if (!result.ok) return taskJobAccessError(result.reason, parsed.data.jobIds.join(', '))
  return jsonResponse(TaskJobWaitResponseV1Schema.parse({
    jobs: result.value.map((job) => taskJobSummaryV1({
      id: job.id,
      kind: job.kind,
      status: job.status,
      background: job.kind === 'parallel_task'
    }))
  }))
}

export async function readTaskJobOutput(
  manager: DurableTaskJobManager | undefined,
  request: Request
): Promise<JsonResponse> {
  if (!manager) return ERRORS.unavailable('task jobs are not available')
  const body = await readJsonBody(request)
  if (!body.ok) return body.response
  const parsed = ReadTaskJobOutputRequest.safeParse(body.value)
  if (!parsed.success) return ERRORS.validation('invalid task job output request', parsed.error.flatten())
  const output = await manager.metadataForParent(parsed.data.jobId, {
    parentThreadId: parsed.data.threadId
  })
  if (!output.ok) return taskJobAccessError(output.reason, parsed.data.jobId)
  return jsonResponse(TaskJobOutputResponseV1Schema.parse({
    schemaVersion: 1,
    availability: 'withheld',
    jobId: output.value.id,
    status: output.value.status,
    reasonCode: 'security_bound_child_output',
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false
  }))
}

export async function killTaskJob(
  manager: DurableTaskJobManager | undefined,
  request: Request
): Promise<JsonResponse> {
  if (!manager) return ERRORS.unavailable('task jobs are not available')
  const body = await readJsonBody(request)
  if (!body.ok) return body.response
  const parsed = KillTaskJobRequest.safeParse(body.value)
  if (!parsed.success) return ERRORS.validation('invalid task job kill request', parsed.error.flatten())
  const job = await manager.killMetadataForParent(parsed.data.jobId, {
    reason: parsed.data.reason,
    parentThreadId: parsed.data.threadId
  })
  if (!job.ok) return taskJobAccessError(job.reason, parsed.data.jobId)
  return jsonResponse(TaskJobKillResponseV1Schema.parse({
    job: taskJobSummaryV1({
      id: job.value.id,
      kind: job.value.kind,
      status: job.value.status,
      background: job.value.kind === 'parallel_task'
    })
  }))
}

function taskJobAccessError(reason: 'not_found' | 'forbidden', id: string): JsonResponse {
  if (reason === 'forbidden') return ERRORS.forbidden(`task job does not belong to this thread: ${id}`)
  return ERRORS.notFound(`task job not found: ${id}`)
}
