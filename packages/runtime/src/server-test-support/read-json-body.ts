import type { AnalytixErrorBody } from '../contracts/errors.js'
import { jsonResponse, type JsonResponse } from './response.js'

export type ReadJsonBodyResult =
  | { ok: true; value: unknown }
  | { ok: false; response: JsonResponse }

export async function readJsonBody(request: Request): Promise<ReadJsonBodyResult> {
  if (request.body === null) return { ok: true, value: {} }
  const text = await request.text()
  if (!text) return { ok: true, value: {} }
  try {
    return { ok: true, value: JSON.parse(text) }
  } catch {
    const body: AnalytixErrorBody = {
      code: 'validation_error',
      message: 'The request did not satisfy the runtime contract.'
    }
    return { ok: false, response: jsonResponse(body, 400) }
  }
}
