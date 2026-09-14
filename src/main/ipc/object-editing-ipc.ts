import {
  objectEditingPath, objectEditingRequestSchema, objectEditingResponseSchema,
  type ObjectEditingRequest, type ObjectEditingResponse
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
        if (receipt && ((request.action !== 'commit' && request.action !== 'status') || receipt.operationId !== request.operationId)) throw new Error('Mismatched operation receipt')
        if (!value.ok || matchesObjectEditingSuccess(request, value)) return value
      }
    } catch { /* An uncertain commit must retain its operation identity. */ }
    if (request.action === 'commit') return {
      ok: false, code: 'persistence_failure', message: 'Save result is unknown. Check the operation before retrying.',
      receipt: { operationId: request.operationId, revision: '', status: 'unknown', savedAt: '' }
    }
    return { ok: false, code: 'unavailable', message: 'Object service is unavailable.' }
  }
}

// Validate only bindings present in this request. Core retains session/object
// authority; Main must not invent a second state store to infer missing fields.
function matchesObjectEditingSuccess(request: ObjectEditingRequest, value: Extract<ObjectEditingResponse, { ok: true }>): boolean {
  switch (request.action) {
    case 'open': return 'document' in value
    case 'commit':
    case 'status': return 'receipt' in value // Operation identity was checked above.
    case 'close': return 'closed' in value
    case 'draft-update':
      return 'draft' in value && value.draft.baseRevision === request.baseRevision && value.draft.content === request.content
    case 'draft-read': return 'draft' in value
    case 'scope-capture':
      return 'scope' in value && value.scope.threadId === request.threadId && value.scope.purpose === request.purpose &&
        value.scope.draftVersion === request.draftVersion && value.scope.current &&
        value.scope.range.start === request.range.start && value.scope.range.end === request.range.end
    case 'scope-read':
      return 'scope' in value && value.scope.scopeId === request.scopeId && value.scope.threadId === request.threadId &&
        value.scope.purpose === request.purpose && value.scope.draftVersion === request.draftVersion
    case 'scope-revoke': return 'revoked' in value
    case 'proposal-create':
      // Replaying create may return an already accepted/rejected proposal.
      return 'proposal' in value && value.proposal.scopeId === request.scopeId && value.proposal.draftVersion === request.draftVersion &&
        JSON.stringify(value.proposal.parts) === JSON.stringify(request.parts)
    case 'proposal-accept':
    case 'proposal-reject':
      // A decision's draftVersion is the version at application time. Accept
      // creates a version; rejecting a stale proposal may name a newer draft.
      return 'decision' in value && value.decision.proposalId === request.proposalId &&
        value.decision.status === (request.action === 'proposal-accept' ? 'accepted' : 'rejected')
  }
}
