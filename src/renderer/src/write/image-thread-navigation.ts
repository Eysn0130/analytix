import { useWriteWorkspaceStore } from './write-workspace-store'

let holds = 0
const listeners = new Set<() => void>()
export const isImageThreadNavigationFrozen = (): boolean => holds > 0
export function subscribeImageThreadNavigation(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
const notify = () => { for (const listener of listeners) listener() }
export const deferImageThreadSelectionClear = (): boolean => {
  const editor = useWriteWorkspaceStore.getState().imageRegionEditor
  return holds > 0 || !!editor && (editor.composing || editor.dirty || !!editor.pending)
}
export const isImageNoteComposing = (): boolean => useWriteWorkspaceStore.getState().imageRegionEditor?.composing === true

/** A local input hold, not a shutdown or a Core permission. Nested navigation
 * shares the freeze; each action owns and releases its own hold. A composition
 * that starts after an await cancels the action permanently and thaws input so
 * its text can continue into the existing draft. */
export async function withImageThreadNavigation<T>(blocked: T, action: (current: () => boolean) => Promise<T>, enabled = true): Promise<T> {
  if (!enabled) return action(() => true)
  if (isImageNoteComposing()) return blocked
  let revoked = false, released = false
  const release = () => {
    if (released) return
    released = true
    holds--
    notify()
  }
  const current = () => !revoked && !isImageNoteComposing()
  holds++
  const unsubscribe = useWriteWorkspaceStore.subscribe(() => {
    if (!isImageNoteComposing()) return
    revoked = true
    release()
  })
  try {
    notify()
    if (!current()) return blocked
    return await action(current)
  } finally {
    unsubscribe()
    release()
  }
}
