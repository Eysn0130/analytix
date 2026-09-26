import { createHash } from 'node:crypto'
import {
  objectEditingPath, objectExportBindingSchema, objectExportSnapshotRequestSchema,
  objectExportSnapshotResponseSchema, type ObjectExportBinding, type ObjectExportSnapshot
} from '../../../packages/runtime/src/contracts/object-editing'

type Transport = (path: string, body: string) => Promise<{ ok: boolean; status: number; body: string }>
const sameBinding = (binding: ObjectExportBinding, snapshot: ObjectExportSnapshot): boolean =>
  binding.sessionId === snapshot.sessionId && binding.objectId === snapshot.objectId &&
  binding.threadId === snapshot.threadId && binding.baseRevision === snapshot.baseRevision &&
  binding.draftVersion === snapshot.draftVersion

/** Main-only resolution. No Renderer paths, settings workspace or returned hash
 * grants authority; every effect rechecks the same live Core snapshot. */
export function createWriteExportSnapshotResolver(transport: Transport) {
  return async (input: ObjectExportBinding, ownerCurrent: () => boolean) => {
    // Pick identity fields rather than accepting an export's format/typography
    // as part of the protected Core request.
    const binding = objectExportBindingSchema.safeParse({ sessionId: input.sessionId, objectId: input.objectId,
      threadId: input.threadId, baseRevision: input.baseRevision, draftVersion: input.draftVersion })
    if (!binding.success) return null
    const read = async (): Promise<ObjectExportSnapshot | null> => {
      if (!ownerCurrent()) return null
      try {
        const request = objectExportSnapshotRequestSchema.parse({ action: 'export-snapshot', ...binding.data })
        const response = await transport(objectEditingPath, JSON.stringify(request))
        if (!response.ok || response.status !== 200 || !ownerCurrent() || Buffer.byteLength(response.body) > 12 * 1024 * 1024) return null
        const result = objectExportSnapshotResponseSchema.safeParse(JSON.parse(response.body))
        if (!result.success || !sameBinding(binding.data, result.data.snapshot)) return null
        const snapshot = result.data.snapshot
        if (createHash('sha256').update(snapshot.content, 'utf8').digest('hex') !== snapshot.contentDigest) return null
        return snapshot
      } catch { return null }
    }
    const snapshot = await read()
    if (!snapshot) return null
    return { snapshot, authorityCurrent: async (): Promise<boolean> => {
      const current = await read()
      return current !== null && current.contentDigest === snapshot.contentDigest &&
        current.content === snapshot.content && current.workspace === snapshot.workspace && current.path === snapshot.path
    } }
  }
}
