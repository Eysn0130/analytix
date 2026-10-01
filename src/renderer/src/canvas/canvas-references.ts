import { canvasHostResponseSchema, type CanvasHostRequest, type CanvasHostResponse } from '../../../../packages/runtime/src/contracts/canvas-host'
import type { NativeReference } from '../office/native-reference-store'

/** The frozen composer snapshot is only UI intent, never authority. Failure is
 * propagated before Send so draft text, attachments and quotations stay intact. */
export async function validateCanvasReferences(
  references: readonly NativeReference[],
  request: (request: CanvasHostRequest) => Promise<CanvasHostResponse>,
  current: () => boolean
): Promise<boolean> {
  for (const reference of references) {
    if (reference.kind !== 'canvas') continue
    if (!current() || !reference.scopeId || !reference.editable) return false
    try {
      const response = canvasHostResponseSchema.safeParse(await request({ operation: 'validate-selection',
        sessionId: reference.sessionId, threadId: reference.threadId, scopeId: reference.scopeId }))
      if (!current() || !response.success || !response.data.ok || !('selection' in response.data)) return false
      const selection = response.data.selection
      if (selection.scopeId !== reference.scopeId || selection.sessionId !== reference.sessionId ||
        selection.threadId !== reference.threadId || selection.baseRevision !== reference.revision ||
        JSON.stringify(selection.selectedIds) !== JSON.stringify(reference.selectedIds)) return false
    } catch { return false }
  }
  return current()
}
