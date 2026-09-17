import { canvasHostRequestSchema, canvasHostResponseSchema, type CanvasHostRequest, type CanvasHostResponse, type CanvasDocument } from '../../../packages/runtime/src/contracts/canvas-host'
import { pluginPackageHostResponseSchema, type PluginPackageHostRequest, type PluginPackageHostResponse, type PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

const unavailable = (): CanvasHostResponse => ({ ok: false, code: 'unavailable' })

/** Each instance belongs to one app window and main frame. Renderer never gets
 * generic plugin invocation, another window's sessions, or export authority. */
export function createCanvasController(options: {
  packageHost: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse>
  current: () => boolean
}) {
  const sessions = new Map<string, { threadId: string; binding: PluginPackageView; object: Pick<CanvasDocument, 'objectId' | 'kind' | 'path'> }>()
  const sameObject = (left: Pick<CanvasDocument, 'objectId' | 'kind' | 'path'>, right: CanvasDocument): boolean =>
    left.objectId === right.objectId && left.kind === right.kind && left.path === right.path
  let tail: Promise<unknown> = Promise.resolve()
  const binding = async (): Promise<PluginPackageView | null> => {
    if (!options.current()) return null
    const parsed = pluginPackageHostResponseSchema.safeParse(await options.packageHost({ action: 'list' }))
    if (!options.current() || !parsed.success || !parsed.data.ok || !('packages' in parsed.data)) return null
    const packages = parsed.data.packages
    if (new Set(packages.map(pkg => pkg.packageId)).size !== packages.length) return null
    return packages.find(p => p.packageId === 'analytix-canvas' && p.available && p.desiredState === 'enabled') ?? null
  }
  const invoke = async (pkg: PluginPackageView, request: CanvasHostRequest) => {
    const { operation, ...input } = request
    return options.packageHost({ action: 'invoke', packageId: 'analytix-canvas', generationId: pkg.generationId,
      expectedRevision: pkg.activationRevision, contributionId: 'workspace-editor', operation, input })
  }
  const perform = async (payload: unknown): Promise<CanvasHostResponse> => {
    const parsed = canvasHostRequestSchema.safeParse(payload)
    if (!parsed.success) return { ok: false, code: 'invalid_request' }
    const request = parsed.data
    const owned = request.operation === 'open-object' ? null : sessions.get(request.sessionId)
    if (request.operation !== 'open-object' && (!owned || owned.threadId !== request.threadId)) return unavailable()
    const pkg = await binding()
    if (!pkg || !pkg.operations.includes(request.operation) || (owned &&
      (owned.binding.generationId !== pkg.generationId || owned.binding.activationRevision !== pkg.activationRevision))) return unavailable()
    if (request.operation === 'open-object' && sessions.size >= 64) return unavailable()
    const outer = pluginPackageHostResponseSchema.safeParse(await invoke(pkg, request))
    if (!outer.success || !outer.data.ok || !('output' in outer.data)) return unavailable()
    const result = canvasHostResponseSchema.safeParse(outer.data.output)
    if (!result.success) return unavailable()
    const response = result.data
    if (!options.current()) {
      if (request.operation === 'open-object' && response.ok && 'document' in response) {
        // Release an unclaimed in-memory handle; durable recovery remains owned
        // by Core. This does not authorize reading or applying in a new frame.
        await invoke(pkg, { operation: 'close-object', sessionId: response.document.sessionId, threadId: request.threadId }).catch(() => undefined)
      }
      return unavailable()
    }
    if (!response.ok) return response
    switch (request.operation) {
      case 'open-object':
        if (!('document' in response) || response.document.threadId !== request.threadId || response.document.kind !== request.kind) return unavailable()
        {
          const previous = sessions.get(response.document.sessionId)
          if (previous && (previous.threadId !== request.threadId || !sameObject(previous.object, response.document))) return unavailable()
          const { objectId, kind, path } = response.document
          sessions.set(response.document.sessionId, { threadId: request.threadId, binding: pkg, object: { objectId, kind, path } })
        }
        break
      case 'read-object':
        if (!('document' in response) || response.document.sessionId !== request.sessionId || response.document.threadId !== request.threadId || !owned || !sameObject(owned.object, response.document)) return unavailable()
        break
      case 'close-object':
        if (!('closed' in response)) return unavailable()
        sessions.delete(request.sessionId)
        break
      case 'propose-scene': case 'propose-image':
        if (!('proposal' in response) || response.proposal.baseRevision !== request.baseRevision || response.proposal.kind !== (request.operation === 'propose-scene' ? 'canvas' : 'png')) return unavailable()
        break
      case 'proposal-read':
        if (!('proposal' in response) || response.proposal.proposalId !== request.proposalId) return unavailable()
        break
      case 'proposal-reject':
        if (!('rejected' in response)) return unavailable()
        break
      case 'proposal-apply': case 'undo-change': case 'resume-change':
        if (!('receipt' in response)) return unavailable()
        break
      case 'object-recovery': case 'cancel-change':
        if (!('recovery' in response)) return unavailable()
        if ([response.recovery.current, response.recovery.pending].some(change => change && change.threadId !== request.threadId)) return unavailable()
        break
    }
    return response
  }
  return {
    async dispose(): Promise<void> {
      // Housekeeping does not read or mutate document content. Let an already
      // dispatched operation settle before releasing its memory handle.
      await tail
      const entries = [...sessions.entries()]
      sessions.clear()
      await Promise.allSettled(entries.map(([sessionId, owner]) => invoke(owner.binding,
        { operation: 'close-object', sessionId, threadId: owner.threadId })))
    },
    request(payload: unknown): Promise<CanvasHostResponse> {
      const result = tail.then(() => perform(payload)).catch(unavailable)
      tail = result
      return result
    }
  }
}
