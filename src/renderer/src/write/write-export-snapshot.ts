import { objectExportBindingSchema, type ObjectExportBinding } from '../../../../packages/runtime/src/contracts/object-editing'
import { useWriteWorkspaceStore } from './write-workspace-store'

/** Explicit click capture only. Local typing continues while the export holds
 * autosave; no file write is started and no reply replaces the editor buffer. */
export async function captureWriteExport(threadId: string, currentThread: () => string | null): Promise<{ binding: ObjectExportBinding; release: () => void }> {
  const store = useWriteWorkspaceStore
  const frozen = store.getState()
  const hold = frozen.beginExport()
  try {
    await hold.settled
    const current = store.getState()
    const session = current.objectSession
    const sameDocument = (): boolean => {
      const next = store.getState()
      return currentThread() === threadId && next.workspaceRoot === frozen.workspaceRoot &&
        next.activeFilePath === frozen.activeFilePath && next.fileContent === frozen.fileContent &&
        next.objectSession?.sessionId === session?.sessionId && next.objectSession?.objectId === session?.objectId &&
        next.objectSession?.revision === session?.revision && !next.pendingSave && !next.reviewActive &&
        !next.pendingAgentReview && !next.fileLoading && !next.fileTruncated && next.saveStatus !== 'conflict'
    }
    if (!session || current.legacyObjectEditing || current.activeFileKind !== 'text' || !sameDocument()) throw new Error('The document export snapshot is unavailable or stale.')
    const previous = await window.analytix.objects.request({ action: 'draft-read', sessionId: session.sessionId })
    if (!sameDocument()) throw new Error('The document changed while preparing the export.')
    let expectedVersion = ''
    if (previous.ok && 'draft' in previous && previous.draft.objectId === session.objectId) expectedVersion = previous.draft.version
    else if (previous.ok || previous.code !== 'draft_stale') throw new Error('The document draft is unavailable.')
    const captured = await window.analytix.objects.request({ action: 'draft-update', sessionId: session.sessionId,
      baseRevision: session.revision, expectedVersion, content: frozen.fileContent })
    if (!sameDocument() || !captured.ok || !('draft' in captured) || captured.draft.objectId !== session.objectId ||
      captured.draft.baseRevision !== session.revision || captured.draft.content !== frozen.fileContent) throw new Error('The document changed while preparing the export.')
    const binding = objectExportBindingSchema.parse({ sessionId: session.sessionId, objectId: session.objectId,
      threadId, baseRevision: session.revision, draftVersion: captured.draft.version })
    return { binding, release: hold.release }
  } catch (error) { hold.release(); throw error }
}
