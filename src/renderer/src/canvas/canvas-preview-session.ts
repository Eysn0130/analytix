import type { CanvasDocument, CanvasHostRequest, CanvasHostResponse } from '../../../../packages/runtime/src/contracts/canvas-host'

export type CanvasPreviewTarget = { threadId: string; workspace: string; path: string; kind: 'canvas' | 'png' }
type Request = (request: CanvasHostRequest) => Promise<CanvasHostResponse>
type Owned = {
  target: CanvasPreviewTarget
  document: Omit<CanvasDocument, 'content'>
  selected: string[]
}
const targetKey = (target: CanvasPreviewTarget): string => JSON.stringify([target.threadId, target.workspace, target.path, target.kind])
const sameDocument = (owned: Owned, document: CanvasDocument): boolean =>
  owned.document.sessionId === document.sessionId && owned.document.threadId === document.threadId &&
  owned.document.objectId === document.objectId && owned.document.kind === document.kind && owned.document.path === document.path

/** Two bounded preview handles, not an authorization or persistence owner.
 * Hiding a view suspends presentation; reopening MUST reread Core authority and
 * bytes. Only IDs/local selection are retained, never a cached document body.
 * Core owns durable proposals/recovery and Main owns window teardown. */
export function createCanvasPreviewSession(request: Request) {
  let generation = 0
  const owned = new Map<string, Owned>() // insertion order is least-recently-used
  let tail: Promise<unknown> = Promise.resolve()
  const call = async (input: CanvasHostRequest): Promise<CanvasHostResponse | null> => {
    try { return await request(input) } catch { return null }
  }
  const release = async (key: string): Promise<boolean> => {
    const entry = owned.get(key)
    if (!entry) return true
    const result = await call({ operation: 'close-object', sessionId: entry.document.sessionId, threadId: entry.document.threadId })
    if (!result?.ok || !('closed' in result)) return false
    owned.delete(key)
    return true
  }
  return {
    open(target: CanvasPreviewTarget): Promise<CanvasDocument | null> {
      const ticket = ++generation
      const input = { ...target }, key = targetKey(input)
      const result = tail.then(async () => {
        if (ticket !== generation) return null
        // An old-thread handle can never become a new-thread selection grant.
        for (const [previous, entry] of owned) {
          if (entry.target.threadId !== input.threadId && !(await release(previous))) return null
          if (ticket !== generation) return null
        }
        const retained = owned.get(key)
        if (retained) {
          const response = await call({ operation: 'read-object', sessionId: retained.document.sessionId, threadId: input.threadId })
          if (ticket !== generation || !response?.ok || !('document' in response) || !sameDocument(retained, response.document)) return null
          const { content: _content, ...document } = response.document
          if (document.revision !== retained.document.revision) retained.selected = []
          retained.document = document
          owned.delete(key); owned.set(key, retained)
          return response.document
        }
        if (owned.size >= 2) {
          const oldest = owned.keys().next().value
          if (oldest === undefined || !(await release(oldest))) return null
        }
        if (ticket !== generation) return null
        const response = await call({ operation: 'open-object', threadId: input.threadId,
          kind: input.kind, object: { workspace: input.workspace, path: input.path } })
        if (!response?.ok || !('document' in response)) return null
        const document = response.document
        if (document.threadId !== input.threadId || document.kind !== input.kind) return null
        // Canonical filesystem aliases may resolve to an already owned handle.
        // Never leave two cache entries that could close each other's session.
        const shared = [...owned].find(([, entry]) => entry.document.sessionId === document.sessionId)
        if (shared && !sameDocument(shared[1], document)) return null
        if (shared) owned.delete(shared[0])
        const { content: _content, ...identity } = document
        owned.set(key, { target: input, document: identity, selected: shared?.[1].selected ?? [] })
        if (ticket !== generation) { if (!shared) await release(key); return null }
        return document
      })
      tail = result
      return result
    },
    suspend(): void { ++generation },
    selected(target: CanvasPreviewTarget): string[] { return [...(owned.get(targetKey(target))?.selected ?? [])] },
    select(target: CanvasPreviewTarget, ids: readonly string[]): void {
      const entry = owned.get(targetKey(target))
      if (entry && ids.length <= 64 && new Set(ids).size === ids.length && ids.every(id => /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(id))) entry.selected = [...ids]
    },
    closeObject(workspace: string, path: string): Promise<boolean> {
      ++generation
      const result = tail.then(async () => {
        let complete = true
        for (const [key, entry] of owned) {
          if (entry.target.workspace === workspace && entry.target.path === path && !(await release(key))) complete = false
        }
        return complete
      })
      tail = result
      return result
    },
    close(target?: CanvasPreviewTarget): Promise<boolean> {
      ++generation
      const key = target && targetKey({ ...target })
      const result = tail.then(async () => {
        let complete = true
        for (const candidate of key === undefined ? [...owned.keys()] : [key]) {
          if (!(await release(candidate))) complete = false
        }
        return complete
      })
      tail = result
      return result
    }
  }
}
