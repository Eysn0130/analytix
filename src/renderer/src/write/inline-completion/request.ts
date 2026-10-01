import type { WriteInlineCompletionRequest, WriteInlineCompletionResult } from '@shared/write-inline-completion'
import { useWriteWorkspaceStore } from '../write-workspace-store'

// SDD/Plan own their editor state outside WriteWorkspaceStore. Their path is an
// assertion to Core, which resolves the object/version in the primary workspace.
// An existing workspace session is never silently downgraded to that path route.
export async function requestDocumentInlineCompletion(
  payload: WriteInlineCompletionRequest,
  signal: AbortSignal
): Promise<WriteInlineCompletionResult> {
  const unavailable = { ok: false as const, message: 'Inline completion owner is unavailable.' }
  const api = window.analytix?.write
  if (signal.aborted || !payload.threadId || !payload.currentFilePath ||
      typeof api?.requestWriteInlineCompletion !== 'function' || typeof api.cancelWriteInlineCompletion !== 'function') return unavailable
  const snapshot = useWriteWorkspaceStore.getState()
  const workspaceSession = snapshot.workspaceRoot === payload.workspaceRoot && snapshot.activeFilePath === payload.currentFilePath
    ? snapshot.objectSession : null
  const document = workspaceSession
    ? { sessionId: workspaceSession.sessionId, objectId: workspaceSession.objectId, baseRevision: workspaceSession.revision }
    : { path: payload.currentFilePath }
  const requestId = crypto.randomUUID()
  let revoked = false
  let cancellationSent = false
  const current = () => {
    if (revoked || signal.aborted) return false
    if (!workspaceSession) return true
    const state = useWriteWorkspaceStore.getState()
    return state.workspaceRoot === snapshot.workspaceRoot && state.activeFilePath === snapshot.activeFilePath &&
      state.objectSession?.sessionId === workspaceSession.sessionId && state.objectSession.objectId === workspaceSession.objectId &&
      state.objectSession.revision === workspaceSession.revision && !state.shutdownFrozen
  }
  const cancel = () => {
    revoked = true
    if (cancellationSent) return
    cancellationSent = true
    void api.cancelWriteInlineCompletion({ requestId }).catch(() => undefined)
  }
  signal.addEventListener('abort', cancel, { once: true })
  // A saved revision, close, navigation or shutdown also revokes a workspace
  // session while the editor's own document/selection signal handles typing.
  const unsubscribe = useWriteWorkspaceStore.subscribe(() => { if (!current()) cancel() })
  try {
    if (!current()) return unavailable
    const result = await api.requestWriteInlineCompletion({ ...payload, requestId, document })
    return current() ? result : unavailable
  } finally {
    signal.removeEventListener('abort', cancel)
    unsubscribe()
  }
}
