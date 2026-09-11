import type { AttachmentReference } from './types'

/**
 * Keep attachment bytes and extracted text out of optimistic blocks, queues,
 * runtime events, history, and exports. The host resolves the attachment ID
 * inside the frozen turn security context when preparing provider input.
 */
export function projectAttachmentReferencesForPublicSurfaces(
  attachments: readonly AttachmentReference[]
): AttachmentReference[] {
  return attachments.map((attachment) => ({
    id: attachment.id,
    ...(attachment.kind ? { kind: attachment.kind } : {}),
    ...(attachment.name ? { name: attachment.name } : {}),
    ...(attachment.mimeType ? { mimeType: attachment.mimeType } : {}),
    ...(attachment.byteSize !== undefined ? { byteSize: attachment.byteSize } : {}),
    ...(attachment.width !== undefined ? { width: attachment.width } : {}),
    ...(attachment.height !== undefined ? { height: attachment.height } : {}),
    ...(attachment.pageCount !== undefined ? { pageCount: attachment.pageCount } : {}),
    ...(attachment.truncated !== undefined ? { truncated: attachment.truncated } : {})
  }))
}
