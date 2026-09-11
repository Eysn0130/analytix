import {
  ApplyCheckpointRewindPlanRequestSchema,
  CheckpointRewindApplyResponseSchema,
  CheckpointRewindPlanResponseSchema,
  CreateCheckpointRewindPlanRequestSchema,
  type CheckpointRewindApplyResponse,
  type CheckpointRewindPlanResponse
} from '../../contracts/checkpoints.js'
import type { CheckpointRewindService } from '../../services-test-support/checkpoint-rewind-service.js'
import { readJsonBody } from '../read-json-body.js'
import { jsonResponse, type JsonResponse } from '../response.js'
import { ERRORS } from './runtime-error.js'

export async function createCheckpointRewindPlan(
  service: CheckpointRewindService,
  threadId: string,
  checkpointId: string,
  request: Request
): Promise<JsonResponse | Response> {
  const body = await readJsonBody(request)
  if (!body.ok) return body.response

  const parsed = CreateCheckpointRewindPlanRequestSchema.safeParse(body.value)
  if (!parsed.success) {
    return ERRORS.validation('invalid checkpoint rewind plan body', parsed.error.issues)
  }

  const result = await service.createPlan({
    threadId,
    checkpointId,
    scope: parsed.data.scope
  })
  if (!result.ok) {
    if (result.status === 404) return ERRORS.notFound(result.message)
    return ERRORS.conflict(result.message)
  }

  const payload: CheckpointRewindPlanResponse = CheckpointRewindPlanResponseSchema.parse({
    plan: result.plan
  })
  return jsonResponse(payload)
}

export async function applyCheckpointRewindPlan(
  service: CheckpointRewindService,
  threadId: string,
  checkpointId: string,
  request: Request
): Promise<JsonResponse | Response> {
  const body = await readJsonBody(request)
  if (!body.ok) return body.response

  const parsed = ApplyCheckpointRewindPlanRequestSchema.safeParse(body.value)
  if (!parsed.success) {
    return ERRORS.validation('invalid checkpoint rewind apply body', parsed.error.issues)
  }

  const result = await service.applyPlan({
    threadId,
    checkpointId,
    request: parsed.data
  })
  if (!result.ok) {
    if (result.status === 404) return ERRORS.notFound(result.message)
    if (result.apply) {
      const payload: CheckpointRewindApplyResponse = CheckpointRewindApplyResponseSchema.parse({
        apply: result.apply
      })
      return jsonResponse(payload, 409)
    }
    return ERRORS.conflict(result.message)
  }

  const payload: CheckpointRewindApplyResponse = CheckpointRewindApplyResponseSchema.parse({
    apply: result.apply
  })
  return jsonResponse(payload)
}
