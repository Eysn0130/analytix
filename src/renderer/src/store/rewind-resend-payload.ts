import { AttachmentPublicMetadata } from '../../../../packages/runtime/src/contracts/attachments.js'
import { UserFileReferenceSchema } from '../../../../packages/runtime/src/contracts/items.js'
import { projectAttachmentReferencesForPublicSurfaces } from '../agent/attachment-public'
import type { AttachmentReference, RuntimeDisclosureMetadata, UserFileReference } from '../agent/types'

const publicReference = AttachmentPublicMetadata.pick({
  id: true, name: true, kind: true, mimeType: true, byteSize: true,
  width: true, height: true, pageCount: true, truncated: true
}).partial().required({ id: true })

/** Closed renderer-only snapshot. Never carry old turn receipts or private content. */
export function captureRewindResendPayload(meta: RuntimeDisclosureMetadata | undefined): {
  attachmentIds: string[]; attachments: AttachmentReference[]; fileReferences: UserFileReference[]
} {
  const boundedArray = (value: unknown): unknown[] => {
    if (value === undefined) return []
    if (!Array.isArray(value) || value.length > 4096) throw new Error('Invalid edit payload')
    return value
  }
  const attachmentIds = [...new Set(boundedArray(meta?.attachmentIds).map(value => {
    if (typeof value !== 'string' || !/^att_[0-9a-f]{24}$/.test(value.trim())) throw new Error('Invalid attachment ID')
    return value.trim()
  }))]
  const ids = new Set(attachmentIds)
  const byId = new Map<string, AttachmentReference>()
  for (const value of boundedArray(meta?.attachments)) {
    if (!value || typeof value !== 'object' || typeof (value as AttachmentReference).id !== 'string') throw new Error('Invalid attachment reference')
    const raw = value as AttachmentReference
    const fields = ['name', 'kind', 'mimeType', 'byteSize', 'width', 'height', 'pageCount', 'truncated'] as const
    const closed = { id: raw.id.trim(), ...Object.fromEntries(fields.filter(field => raw[field] !== undefined).map(field => [field, raw[field]])) }
    const reference = projectAttachmentReferencesForPublicSurfaces([publicReference.parse(closed)])[0]
    if (!ids.has(reference.id)) throw new Error('Inconsistent attachment reference')
    const previous = byId.get(reference.id)
    if (previous && JSON.stringify(previous) !== JSON.stringify(reference)) throw new Error('Conflicting attachment reference')
    byId.set(reference.id, reference)
  }
  const fileReferences = boundedArray(meta?.fileReferences).map(value => {
    if (!value || typeof value !== 'object') throw new Error('Invalid file reference')
    const reference = value as UserFileReference
    const trimmed = (input: unknown): string => typeof input === 'string' ? input.trim() : ''
    return UserFileReferenceSchema.strict().parse({ path: trimmed(reference.path), relativePath: trimmed(reference.relativePath),
      name: trimmed(reference.name), ...(reference.kind !== undefined ? { kind: reference.kind } : {}) })
  })
  return { attachmentIds, attachments: [...byId.values()], fileReferences }
}
