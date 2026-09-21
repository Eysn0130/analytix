import { objectEditingResponseSchema, type ObjectEditingRequest, type ObjectEditingResponse } from '../../../../packages/runtime/src/contracts/object-editing'
import type { NativeReference } from '../office/native-reference-store'

export async function validateImageReferences(
  references: readonly NativeReference[],
  request: (request: ObjectEditingRequest) => Promise<ObjectEditingResponse>,
  current: () => boolean
): Promise<boolean> {
  for (const reference of references) {
    if (reference.kind !== 'image-region') continue
    if (!current() || !reference.scopeId) return false
    try {
      const response = objectEditingResponseSchema.safeParse(await request({ action: 'image-scope-read',
        sessionId: reference.sessionId, threadId: reference.threadId, scopeId: reference.scopeId }))
      if (!current() || !response.success || !response.data.ok || !('scope' in response.data)) return false
      const scope = response.data.scope
      if (!('kind' in scope) || scope.sessionId !== reference.sessionId || scope.scopeId !== reference.scopeId ||
        scope.threadId !== reference.threadId || scope.objectId !== reference.objectId || scope.sourceRevision !== reference.revision ||
        scope.annotationRevision !== reference.annotationRevision || scope.width !== reference.width || scope.height !== reference.height ||
        JSON.stringify(scope.region) !== JSON.stringify(reference.region)) return false
    } catch { return false }
  }
  return current()
}
