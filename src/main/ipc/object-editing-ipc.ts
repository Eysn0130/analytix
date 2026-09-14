import {
  objectEditingPath, objectEditingRequestSchema, objectEditingResponseSchema,
  type ObjectEditingResponse
} from '../../../packages/runtime/src/contracts/object-editing'

type Transport = (path: string, body: string) => Promise<{ ok: boolean; status: number; body: string }>

/** This transport is Main-only. Never pass private object responses through the
 * ordinary runtime proxy, history projection or a third-party plugin frame. */
export function createObjectEditingHandler(transport: Transport) {
  return async (payload: unknown): Promise<ObjectEditingResponse> => {
    const parsed = objectEditingRequestSchema.safeParse(payload)
    if (!parsed.success) return { ok: false, code: 'invalid_request', message: 'Invalid object request.' }
    const request = parsed.data
    try {
      const response = await transport(objectEditingPath, JSON.stringify(request))
      const result = objectEditingResponseSchema.safeParse(JSON.parse(response.body))
      if (result.success && result.data.ok === response.ok) {
        const value = result.data
        const receipt = 'receipt' in value ? value.receipt : undefined
        if (receipt && (!('operationId' in request) || receipt.operationId !== request.operationId)) throw new Error('Mismatched operation receipt')
        if (!value.ok || (request.action === 'open' && 'document' in value) ||
            (request.action === 'close' && 'closed' in value) ||
            ((request.action === 'commit' || request.action === 'status') && receipt)) return value
      }
    } catch { /* An uncertain commit must retain its operation identity. */ }
    if (request.action === 'commit') return {
      ok: false, code: 'persistence_failure', message: 'Save result is unknown. Check the operation before retrying.',
      receipt: { operationId: request.operationId, revision: '', status: 'unknown', savedAt: '' }
    }
    return { ok: false, code: 'unavailable', message: 'Object service is unavailable.' }
  }
}
