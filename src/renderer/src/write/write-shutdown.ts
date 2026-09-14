import type { WriteShutdownHandler, WriteShutdownResult } from '@shared/write-shutdown'
import { useWriteWorkspaceStore } from './write-workspace-store'

type InputSurface = { composing: () => boolean; freeze: (frozen: boolean) => void }
const surfaces = new Set<InputSurface>()
let inputFrozen = false
export const isWriteShutdownFrozen = () => inputFrozen

/** Register the actual editor instance, not its asynchronously updated props. */
export function registerWriteShutdownInput(surface: InputSurface): () => void {
  surfaces.add(surface)
  surface.freeze(inputFrozen)
  return () => { surfaces.delete(surface) }
}

export function createWriteShutdownHandler(): WriteShutdownHandler {
  let active: { requestId: string; release: () => void; result: Promise<WriteShutdownResult> } | undefined
  const thaw = () => {
    const previous = active
    active = undefined
    previous?.release()
    inputFrozen = false
    let restored = true
    for (const surface of surfaces) {
      try { surface.freeze(false) } catch { restored = false }
    }
    return restored
  }
  return async request => {
    if (request.phase === 'cancel') {
      if (active?.requestId === request.requestId && !thaw()) return { result: 'blocked', reason: 'unavailable' }
      return { result: 'ready' }
    }
    if (active?.requestId === request.requestId) return active.result
    if (active) return { result: 'blocked', reason: 'unavailable' }
    // Do not blur or reconfigure an editor with unfinished composition text.
    if ([...surfaces].some(surface => surface.composing())) return { result: 'blocked', reason: 'composing' }
    try {
      inputFrozen = true
      for (const surface of surfaces) surface.freeze(true)
      const hold = useWriteWorkspaceStore.getState().beginShutdown()
      const attempt = { requestId: request.requestId, release: hold.release, result: Promise.resolve<WriteShutdownResult>({ result: 'blocked', reason: 'unavailable' }) }
      active = attempt
      attempt.result = hold.save().then((result): WriteShutdownResult => {
        if (active !== attempt) return { result: 'blocked', reason: 'unavailable' }
        if (result.result !== 'ready') thaw()
        return result
      }).catch(() => {
        if (active === attempt) thaw()
        return { result: 'blocked', reason: 'unavailable' }
      })
      return attempt.result
    } catch {
      thaw()
      return { result: 'blocked', reason: 'unavailable' }
    }
  }
}
