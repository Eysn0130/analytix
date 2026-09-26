import { createHash } from 'node:crypto'
import { resolve } from 'node:path'
import { expandHomePath } from '../services/workspace-service'
import { workspaceReadPath, workspaceReadRequestSchema, workspaceReadResponseSchema,
  type WorkspaceReadFile } from '../../../packages/runtime/src/contracts/workspace-read'

type Transport = (path: string, body: string) => Promise<{ ok: boolean; status: number; body: string }>
export type WriteRetrievalSource = {
  threadId: string
  key: string
  workspaceRoot: string
  current: () => Promise<boolean>
  scan: (includePdf: boolean) => Promise<WorkspaceReadFile[]>
}

export async function authorizeWriteRetrievalSource(transport: Transport, threadId: string | undefined,
  expectedWorkspace: string | undefined, ownerCurrent: () => boolean): Promise<WriteRetrievalSource | null> {
  if (!ownerCurrent() || !threadId) return null
  const read = async (request: unknown) => {
    if (!ownerCurrent()) throw new Error('write_read_unavailable')
    const response = await transport(workspaceReadPath, JSON.stringify(workspaceReadRequestSchema.parse(request)))
    if (!ownerCurrent() || !response.ok || response.status !== 200 || Buffer.byteLength(response.body) > 24 * 1024 * 1024) throw new Error('write_read_unavailable')
    const parsed = workspaceReadResponseSchema.parse(JSON.parse(response.body))
    if (!parsed.ok || parsed.snapshot.threadId !== threadId) throw new Error('write_read_unavailable')
    return parsed.snapshot
  }
  try {
    const scope = await read({ action: 'authorize', threadId })
    // The renderer can only assert a root; it cannot select what Core scans.
    if (!expectedWorkspace || resolve(expandHomePath(expectedWorkspace)) !== resolve(expandHomePath(scope.workspace))) return null
    const same = (value: typeof scope) => value.binding === scope.binding && value.workspace === scope.workspace
    const current = async () => {
      try { return same(await read({ action: 'validate', threadId, binding: scope.binding })) }
      catch { return false }
    }
    return {
      threadId, key: `${threadId}:${scope.binding}`, workspaceRoot: scope.workspace, current,
      scan: async includePdf => {
        const snapshot = await read({ action: 'scan', threadId, binding: scope.binding, includePdf })
        if (!same(snapshot)) throw new Error('write_read_unavailable')
        let bytes = 0
        const paths = new Set<string>()
        for (const file of snapshot.files ?? []) {
          const content = Buffer.from(file.content, 'base64')
          bytes += content.length
          if (paths.has(file.path) || bytes > 16 * 1024 * 1024 || (!includePdf && file.kind === 'pdf') ||
            (file.kind === 'text' && content.length > 600000) ||
            createHash('sha256').update(content).digest('hex') !== file.revision) throw new Error('write_read_unavailable')
          paths.add(file.path)
        }
        if (!await current()) throw new Error('write_read_unavailable')
        return snapshot.files ?? []
      }
    }
  } catch { return null }
}
