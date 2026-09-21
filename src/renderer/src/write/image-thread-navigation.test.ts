import { afterEach, beforeEach, expect, test, vi, type MockInstance } from 'vitest'
import { useWriteWorkspaceStore } from './write-workspace-store'
import { isImageThreadNavigationFrozen, withImageThreadNavigation } from './image-thread-navigation'
import { createThreadActions } from '../store/chat-store-thread-actions'
import { createNavigationActions } from '../store/chat-store-navigation-actions'
import { createMaintenanceActions } from '../store/chat-store-maintenance-actions'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { buildThreadEventSink } from '../store/chat-store-runtime'
import { readThreadWorktreeRegistry, saveThreadWorktreeRegistry, markThreadWorktree } from '../lib/thread-worktree-registry'
import { readThreadForkRegistry } from '../lib/thread-fork-registry'
import { clearBusyWatchdog, stopTurnCompletionPoll } from '../store/chat-store-schedulers'
import type { ChatState, ChatStoreSet } from '../store/chat-store-types'
import type { ImageRegionEditor } from './image-region-session'
const registry = vi.hoisted(() => ({ getProvider: vi.fn() }))
vi.mock('../agent/registry', () => ({ getProvider: registry.getProvider }))
vi.mock('electron', () => ({ clipboard: {} }))
const thread = (id: string) => ({ id, title: id, workspace: '/workspace', updatedAt: '2026-09-20T00:00:00Z', status: 'idle', mode: 'agent', model: '' })
const imageEditor = (): ImageRegionEditor => ({ workspace: '/workspace', path: '/workspace/image.png', threadId: 'thr-ime',
  snapshot: null, annotation: null, region: null, note: '保留组合输入', dirty: true, pending: null, loading: false,
  revoked: false, stale: false, composing: false, status: 'dirty', error: null })
const composing = (value: boolean) => useWriteWorkspaceStore.getState().setImageRegionComposing(value)
let provider: {
  createThread: ReturnType<typeof vi.fn>; getThreadDetail: ReturnType<typeof vi.fn>; subscribeThreadEvents: ReturnType<typeof vi.fn>
  forkThread: ReturnType<typeof vi.fn>; resumeSession: ReturnType<typeof vi.fn>; deleteThread: ReturnType<typeof vi.fn>
  archiveThread: ReturnType<typeof vi.fn>; listThreads: ReturnType<typeof vi.fn>
}
let settings: MockInstance<typeof rendererRuntimeClient.getSettings>, saveSettings: MockInstance<typeof rendererRuntimeClient.setSettings>, pickDirectory: ReturnType<typeof vi.fn>
function harness() {
  let state = { activeThreadId: 'thr-ime', runtimeConnection: 'ready', threads: [thread('thr-ime')], workspaceRoot: '/workspace',
    route: 'chat', blocks: [], clawChannels: [], busy: false, codeWorkspaceRoots: [], watchTurnCompletion: {}, unreadThreadIds: {},
    composerModel: '', composerProviderId: '', refreshThreads: vi.fn(async () => undefined), currentTurnId: null, currentTurnUserId: null,
    lastSeq: 0, threadSearch: '', showArchivedThreads: false } as unknown as ChatState
  const set: ChatStoreSet = partial => { state = { ...state, ...(typeof partial === 'function' ? partial(state) : partial) } }
  const sseAbortRef = { current: new AbortController() as AbortController | null }
  const context = { set, get: () => state, sseAbortRef }
  const threadActions = createThreadActions(context), navigation = createNavigationActions(context), maintenance = createMaintenanceActions(context)
  state = { ...state, ...threadActions, ...navigation, ...maintenance, refreshThreads: vi.fn(async () => undefined) }
  return { get: () => state, set, navigation, sseAbortRef }
}
beforeEach(() => {
  useWriteWorkspaceStore.setState({ imageRegionEditor: imageEditor(), shutdownFrozen: false })
  provider = {
    createThread: vi.fn(async () => thread('thr-created')), getThreadDetail: vi.fn(async () => ({ blocks: [], latestSeq: 0, threadStatus: 'idle' })),
    subscribeThreadEvents: vi.fn(async () => undefined), forkThread: vi.fn(async () => thread('thr-fork')),
    resumeSession: vi.fn(async () => ({ threadId: 'thr-resumed' })), deleteThread: vi.fn(async () => undefined),
    archiveThread: vi.fn(async () => undefined), listThreads: vi.fn(async () => [])
  }
  registry.getProvider.mockReturnValue(provider)
  settings = vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({ workspaceRoot: '/workspace', write: { workspaces: [] } } as never)
  saveSettings = vi.spyOn(rendererRuntimeClient, 'setSettings').mockResolvedValue({ workspaceRoot: '/next' } as never)
  pickDirectory = vi.fn(async () => ({ canceled: false, path: '/next' }))
  const storage = new Map<string, string>()
  const localStorage = { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => { storage.set(key, value) }, removeItem: (key: string) => { storage.delete(key) } }
  vi.stubGlobal('window', { localStorage, analytix: { workspace: { pickDirectory }, settings: { setSettings: saveSettings } } })
})
afterEach(() => {
  expect(isImageThreadNavigationFrozen()).toBe(false)
  useWriteWorkspaceStore.setState({ imageRegionEditor: null })
  clearBusyWatchdog(); stopTurnCompletionPoll(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
const leaving = [
  ['select', (s: ChatState) => s.selectThread('thr-next')],
  ['live', (s: ChatState) => s.subscribeThreadEventsLive('thr-next')],
  ['new', (s: ChatState) => s.createThread()],
  ['materialize', (s: ChatState) => s.createThread({ materialize: true, forceNew: true })],
  ['code', (s: ChatState) => s.openCode()],
  ['workspace-picker', (s: ChatState) => s.chooseWorkspace()],
  ['workspace-select', (s: ChatState) => s.selectWorkspaceRoot('/next')],
  ['workspace-clear', (s: ChatState) => s.clearWorkspace()],
  ['workspace-delete', (s: ChatState) => s.deleteWorkspace('/workspace')],
  ['fork', (s: ChatState) => s.forkActiveThread()],
  ['fork-turn', (s: ChatState) => s.forkThreadFromTurn('turn-1')],
  ['resume', (s: ChatState) => s.resumeSessionIntoThread('session-1')],
  ['archive', (s: ChatState) => s.archiveThread('thr-ime', true)],
  ['delete', (s: ChatState) => s.deleteThread('thr-ime')]
] as const
test.each(leaving)('active image IME rejects %s before side effects', async (_name, action) => {
  const h = harness(), before = h.get(), stream = h.sseAbortRef.current
  composing(true)
  await action(h.get())
  expect(h.get()).toBe(before)
  expect(stream?.signal.aborted).toBe(false)
  expect(registry.getProvider).not.toHaveBeenCalled()
  expect(settings).not.toHaveBeenCalled(); expect(saveSettings).not.toHaveBeenCalled(); expect(pickDirectory).not.toHaveBeenCalled()
  expect(useWriteWorkspaceStore.getState().imageRegionEditor).toMatchObject({ composing: true, note: '保留组合输入' })
})
test.each(['select', 'create', 'fork', 'resume', 'delete', 'archive', 'workspace'] as const)('composition starting during %s await permanently cancels selection and preserves input', async kind => {
  const h = harness()
  let finish!: () => void
  if (kind === 'create') {
    settings.mockImplementationOnce(() => new Promise(resolve => {
      finish = () => resolve({ workspaceRoot: '/workspace' } as Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>)
    }))
  } else if (kind === 'workspace') {
    saveSettings.mockImplementationOnce(() => new Promise(resolve => {
      finish = () => resolve({ workspaceRoot: '/next' } as Awaited<ReturnType<typeof rendererRuntimeClient.setSettings>>)
    }))
  } else {
    const target = kind === 'select' ? provider.getThreadDetail : kind === 'fork' ? provider.forkThread :
      kind === 'resume' ? provider.resumeSession : kind === 'delete' ? provider.deleteThread : provider.archiveThread
    target.mockImplementationOnce(() => new Promise(resolve => {
      finish = () => resolve(kind === 'select' ? { blocks: [], latestSeq: 0, threadStatus: 'idle' } :
        kind === 'fork' ? thread('thr-fork') : kind === 'resume' ? { threadId: 'thr-resumed' } : undefined)
    }))
  }
  const originalStream = h.sseAbortRef.current
  const task = kind === 'select' ? h.get().selectThread('thr-pending') : kind === 'create' ? h.get().createThread() :
    kind === 'fork' ? h.get().forkActiveThread() : kind === 'resume' ? h.get().resumeSessionIntoThread('session-pending') :
    kind === 'delete' ? h.get().deleteThread('thr-ime') : kind === 'archive' ? h.get().archiveThread('thr-ime', true) : h.get().selectWorkspaceRoot('/next')
  expect(isImageThreadNavigationFrozen()).toBe(true)
  composing(true)
  expect(isImageThreadNavigationFrozen()).toBe(false)
  // Ending composition cannot revive the navigation that began before it.
  composing(false)
  finish(); await task
  expect(h.get().activeThreadId).toBe('thr-ime')
  if (kind === 'select') { expect(originalStream?.signal.aborted).toBe(false); expect(h.sseAbortRef.current).toBe(originalStream) }
  expect(useWriteWorkspaceStore.getState().imageRegionEditor?.note).toBe('保留组合输入')
})
test('background refresh updates the list but defers active selection cleanup while the image draft owns input', async () => {
  const h = harness()
  h.get().threads = [{ ...thread('thr-ime'), relation: 'side', parentThreadId: 'parent' } as never]
  provider.listThreads.mockResolvedValue([thread('parent')])
  provider.getThreadDetail.mockResolvedValue({ blocks: [{ kind: 'user', id: 'u', text: 'hello' }], latestSeq: 0, threadStatus: 'idle' })
  composing(true)
  await h.navigation.refreshThreads()
  expect(h.get().threads.map(t => t.id)).toContain('parent')
  expect(h.get().activeThreadId).toBe('thr-ime')
  expect(useWriteWorkspaceStore.getState().imageRegionEditor?.composing).toBe(true)
})
test('nested holds release only their own freeze and throwing actions always thaw', async () => {
  await expect(withImageThreadNavigation(undefined, async () => {
    expect(isImageThreadNavigationFrozen()).toBe(true)
    await withImageThreadNavigation(undefined, async () => { expect(isImageThreadNavigationFrozen()).toBe(true) })
    expect(isImageThreadNavigationFrozen()).toBe(true)
    throw Error('synthetic failure')
  })).rejects.toThrow('synthetic failure')
  expect(isImageThreadNavigationFrozen()).toBe(false)
})
test.each(['acquisition', 'creation'] as const)('cancellation after worktree %s retains completed resource ownership', async boundary => {
  const h = harness()
  let finish!: () => void
  const acquired = { path: '/workspace-pool/1', branch: 'analytix-1' }
  const acquireWorktree = vi.fn(async () => acquired), releaseWorktree = vi.fn(async () => undefined)
  Object.assign(window.analytix.workspace, { findAvailableWorktreePoolIndex: vi.fn(async () => 1), acquireWorktree, releaseWorktree })
  if (boundary === 'acquisition') acquireWorktree.mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve(acquired) }))
  else provider.createThread.mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve(thread('created-owner')) }))
  const task = h.get().createThread({ useWorktreePool: true })
  await vi.waitFor(() => expect(finish).toBeTypeOf('function'))
  composing(true); finish(); await task
  expect(h.get().activeThreadId).toBe('thr-ime')
  if (boundary === 'acquisition') {
    expect(releaseWorktree).toHaveBeenCalledExactlyOnceWith({ projectPath: '/workspace', poolIndex: 1 })
    expect(provider.createThread).not.toHaveBeenCalled()
  } else {
    expect(releaseWorktree).not.toHaveBeenCalled()
    expect(readThreadWorktreeRegistry().worktrees['created-owner']).toMatchObject({ projectPath: '/workspace', poolIndex: 1, worktreePath: acquired.path })
    expect(h.get().threads.some(t => t.id === 'created-owner')).toBe(true)
  }
})
test('a completed fork keeps its registry even when composition cancels selecting it', async () => {
  const h = harness()
  let finish!: () => void
  provider.forkThread.mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve(thread('fork-registered')) }))
  const task = h.get().forkActiveThread()
  composing(true); finish(); await task
  expect(readThreadForkRegistry().forks['fork-registered']).toMatchObject({ parentThreadId: 'thr-ime' })
  expect(h.get().refreshThreads).toHaveBeenCalled()
  expect(h.get().activeThreadId).toBe('thr-ime')
})
test('completed deletion cleans registry and list while preserving the composing selection', async () => {
  const h = harness()
  saveThreadWorktreeRegistry(markThreadWorktree('thr-ime', { projectPath: '/workspace', poolIndex: 2, worktreePath: '/pool/2', branch: 'analytix-2' }))
  let finish!: () => void
  provider.deleteThread.mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve(undefined) }))
  const task = h.get().deleteThread('thr-ime')
  composing(true); finish(); await task
  expect(readThreadWorktreeRegistry().worktrees['thr-ime']).toBeUndefined()
  expect(h.get().threads).toEqual([])
  expect(h.get().activeThreadId).toBe('thr-ime')
  expect(h.get().refreshThreads).toHaveBeenCalled()
})
test('public projection revocation remains immediate during a navigation input hold', async () => {
  const h = harness(), id = 'thr-security-image-ime'
  h.set({ activeThreadId: id, threads: [{ ...thread(id), historyAuthority: 'case_boundary_only_v1' } as never],
    blocks: [{ kind: 'assistant', id: 'a', text: 'REVOKED_CONTENT' } as never], sideConversations: {},
    sidePanel: { open: false, activeSideId: null }, caseProjectThreadsById: {}, threadHandoffOperations: [] })
  const sink = buildThreadEventSink(h.set, h.get, { threadId: id })
  await withImageThreadNavigation(undefined, async () => {
    expect(isImageThreadNavigationFrozen()).toBe(true)
    await sink.onPublicProjectionRevoked?.({ schemaVersion: 1, kind: 'public_projection_revoked', threadId: id,
      historyAuthority: 'case_boundary_only_v1', code: 'case_public_authority_rejected', action: 'purge_case_projection', terminal: true })
    expect(h.get().activeThreadId).toBeNull()
    expect(h.get().blocks).toEqual([])
    expect(h.get().threads).toEqual([])
    expect(useWriteWorkspaceStore.getState().imageRegionEditor?.note).toBe('保留组合输入')
  })
})
