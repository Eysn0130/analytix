import type { CanvasDocument, CanvasHostRequest, CanvasHostResponse } from '../../../../packages/runtime/src/contracts/canvas-host'

export type CanvasPreviewTarget = { threadId: string; workspace: string; path: string; kind: 'canvas' | 'png' }
type Request = (request: CanvasHostRequest) => Promise<CanvasHostResponse>

/** One visible preview, not a persistence or authorization owner. Serialize release
 * before acquisition: Core may reuse a handle when reopening the same object. */
export function createCanvasPreviewSession(request: Request) {
  let generation = 0
  let owned: Pick<CanvasDocument, 'sessionId' | 'threadId'> | null = null
  let tail: Promise<unknown> = Promise.resolve()
  const call = async (input: CanvasHostRequest): Promise<CanvasHostResponse | null> => {
    try { return await request(input) } catch { return null }
  }
  const release = async (): Promise<boolean> => {
    if (!owned) return true
    const result = await call({ operation: 'close-object', ...owned })
    if (!result?.ok || !('closed' in result)) return false
    owned = null
    return true
  }
  return {
    open(target: CanvasPreviewTarget): Promise<CanvasDocument | null> {
      const ticket = ++generation
      // Copy the input before awaiting; never follow a caller-mutated target.
      const input = { ...target }
      const result = tail.then(async () => {
        if (ticket !== generation || !(await release()) || ticket !== generation) return null
        const response = await call({ operation: 'open-object', threadId: input.threadId,
          kind: input.kind, object: { workspace: input.workspace, path: input.path } })
        if (!response?.ok || !('document' in response)) return null
        const document = response.document
        // Main and Core enforce full ownership; never display a mismatched reply.
        if (document.threadId !== input.threadId || document.kind !== input.kind) return null
        owned = { sessionId: document.sessionId, threadId: document.threadId }
        if (ticket !== generation) { await release(); return null }
        return document
      })
      tail = result
      return result
    },
    close(): Promise<boolean> {
      ++generation
      const result = tail.then(release)
      tail = result
      return result
    }
  }
}
