import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  state: { activeThreadId: 'thread-1' as string | null, workspaceRoot: '/workspace', openWrite: vi.fn() },
  flushSave: vi.fn(), select: vi.fn(), resolveArtifact: vi.fn()
}))
vi.mock('../store/chat-store', () => ({ useChatStore: { getState: () => mocks.state } }))
vi.mock('../write/write-workspace-store', () => ({ useWriteWorkspaceStore: { getState: () => ({ flushSave: mocks.flushSave }) } }))
vi.mock('./native-office-store', () => ({ useNativeOfficeStore: { getState: () => ({ select: mocks.select }) } }))
import { openGeneratedArtifact } from './open-generated-artifact'

const id = 'a'.repeat(64)
const response = { ok: true, artifact: { threadId: 'thread-1', artifactId: id, workspace: '/workspace', path: '/workspace/report.docx', kind: 'docx', revision: 'b'.repeat(64), byteSize: 2048, changed: false } }

beforeEach(() => {
  vi.clearAllMocks()
  mocks.state.activeThreadId = 'thread-1'
  mocks.state.workspaceRoot = '/workspace'
  mocks.flushSave.mockResolvedValue(true)
  mocks.select.mockResolvedValue(true)
  mocks.resolveArtifact.mockResolvedValue(response)
  vi.stubGlobal('window', { analytix: { objects: { resolveArtifact: mocks.resolveArtifact } } })
})

describe('open generated document in the main workspace', () => {
  it('rejects a deferred automatic open from a previous conversation scope', async () => {
    expect(await openGeneratedArtifact(id, { threadId: 'old-thread', workspace: '/workspace' })).toBe(false)
    expect(mocks.resolveArtifact).not.toHaveBeenCalled()
  })
  it('resolves the receipt and preserves pending text before opening the native object', async () => {
    expect(await openGeneratedArtifact(id)).toBe(true)
    expect(mocks.resolveArtifact).toHaveBeenCalledWith({ threadId: 'thread-1', artifactId: id })
    expect(mocks.flushSave).toHaveBeenCalledWith('/workspace')
    expect(mocks.select).toHaveBeenCalledWith('/workspace', '/workspace/report.docx', expect.any(Function))
    expect(mocks.state.openWrite).toHaveBeenCalledOnce()
  })
  it('does not switch documents when the existing document cannot be saved', async () => {
    mocks.flushSave.mockResolvedValue(false)
    expect(await openGeneratedArtifact(id)).toBe(false)
    expect(mocks.select).not.toHaveBeenCalled()
  })
  it('drops a reply after the user changes conversations', async () => {
    mocks.resolveArtifact.mockImplementation(async () => { mocks.state.activeThreadId = 'thread-2'; return response })
    expect(await openGeneratedArtifact(id)).toBe(false)
    expect(mocks.flushSave).not.toHaveBeenCalled()
    expect(mocks.select).not.toHaveBeenCalled()
  })
  it('drops a selection after the user changes workspace while saving', async () => {
    mocks.flushSave.mockImplementationOnce(async () => { mocks.state.workspaceRoot = '/other'; return true })
    expect(await openGeneratedArtifact(id)).toBe(false)
    expect(mocks.select).not.toHaveBeenCalled()
  })
  it('rejects a valid but different artifact returned by the bridge', async () => {
    mocks.resolveArtifact.mockResolvedValue({ ...response, artifact: { ...response.artifact, artifactId: 'c'.repeat(64) } })
    expect(await openGeneratedArtifact(id)).toBe(false)
    expect(mocks.select).not.toHaveBeenCalled()
  })
  it('invalidates the native store selection guard when a later open supersedes it', async () => {
    let guard: (() => boolean) | undefined
    mocks.select.mockImplementationOnce(async (_workspace, _path, isCurrent) => { guard = isCurrent; return true })
    expect(await openGeneratedArtifact(id)).toBe(true)
    expect(guard?.()).toBe(true)
    await openGeneratedArtifact(id)
    expect(guard?.()).toBe(false)
  })
})
