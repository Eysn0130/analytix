// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defaultAnalytixRuntimeSettings, type AppSettingsV1 } from '@shared/app-settings'
import type { ChatBlock, NormalizedThread, ThreadExecutionPolicy } from '../../agent/types'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import * as dialogs from '../../lib/confirm-dialog'
import { useChatStore } from '../../store/chat-store'
import type { ChatState } from '../../store/chat-store-types'
import { useComposerExecutionPolicy } from './use-composer-execution-policy'

const readOnly: ThreadExecutionPolicy = { approvalPolicy: 'never', sandboxMode: 'read-only' }
const workspaceWrite: ThreadExecutionPolicy = { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' }
const full: ThreadExecutionPolicy = { approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }
const thread = (id: string, policy = readOnly, version: 2 | null = 2): NormalizedThread => ({
  id, title: id, workspace: '/synthetic/workspace', model: 'synthetic-model', mode: 'agent', updatedAt: '2026-10-20T00:00:00Z',
  ...policy, executionPolicyVersion: version ?? undefined
})
const settings = (policy = readOnly): AppSettingsV1 => ({ runtime: { ...defaultAnalytixRuntimeSettings(), ...policy } } as AppSettingsV1)
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
let host: HTMLDivElement
let root: Root
let original: ChatState
let current: ReturnType<typeof useComposerExecutionPolicy>
let createThread: ReturnType<typeof vi.fn<ChatState['createThread']>>
let updatePolicy: ReturnType<typeof vi.fn<ChatState['updateThreadExecutionPolicy']>>
function Probe() {
  current = useComposerExecutionPolicy()
  return createElement('output', null, JSON.stringify({ value: current.value, legacy: current.legacy, applying: current.applying, unavailable: current.unavailable }))
}
async function render() { await act(async () => root.render(createElement(Probe))) }
async function choose(policy: ThreadExecutionPolicy) { await act(async () => current.onChange(policy)) }
function commitPolicy(id: string, policy: ThreadExecutionPolicy) {
  useChatStore.setState(state => ({ threads: state.threads.map(item => item.id === id
    ? { ...item, ...policy, executionPolicyVersion: 2, executionPolicyUncertain: false } : item) }))
}
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  original = useChatStore.getState()
  createThread = vi.fn<ChatState['createThread']>(async () => undefined)
  updatePolicy = vi.fn<ChatState['updateThreadExecutionPolicy']>(async (id, policy) => { commitPolicy(id, policy); return true })
  useChatStore.setState({ activeThreadId: 'a', threads: [thread('a'), thread('b')], workspaceRoot: '/synthetic/workspace',
    route: 'chat', runtimeConnection: 'ready', busy: false, error: null, createThread, updateThreadExecutionPolicy: updatePolicy })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings())
  vi.spyOn(rendererRuntimeClient, 'setSettings').mockRejectedValue(new Error('Global settings must not be written'))
  vi.spyOn(dialogs, 'confirmDialog').mockResolvedValue(true)
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); useChatStore.setState(original, true)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('real thread permission owner', () => {
  it('reads the current thread pair and waits for store-confirmed metadata rather than an optimistic check', async () => {
    useChatStore.setState({ threads: [thread('a', full), thread('b')] })
    const response = deferred<boolean>()
    updatePolicy.mockImplementationOnce(async (id, policy) => { const ok = await response.promise; if (ok) commitPolicy(id, policy); return ok })
    await render(); await choose(workspaceWrite)
    expect(current.value).toEqual(full)
    expect(current.applying).toBe(true)
    expect(current.unavailable).toBe(true)
    expect(updatePolicy).toHaveBeenCalledExactlyOnceWith('a', workspaceWrite)
    expect(rendererRuntimeClient.getSettings).not.toHaveBeenCalled()
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
    await act(async () => response.resolve(true))
    expect(current.value).toEqual(workspaceWrite)
    expect(current.applying).toBe(false)
    expect(current.unavailable).toBe(false)
  })
  it('keeps a caller-observed unversioned full-access pair Custom and does not seal it locally after a failed store result', async () => {
    useChatStore.setState({ threads: [thread('a', full, null)] })
    updatePolicy.mockResolvedValueOnce(false)
    await render()
    expect(current.legacy).toBe(true)
    await choose(full)
    expect(dialogs.confirmDialog).toHaveBeenCalledOnce()
    expect(updatePolicy).toHaveBeenCalledExactlyOnceWith('a', full)
    expect(current.legacy).toBe(true)
    expect(useChatStore.getState().threads[0].executionPolicyVersion).toBeUndefined()
  })
  it('does not PATCH after full-access confirmation is cancelled', async () => {
    vi.mocked(dialogs.confirmDialog).mockResolvedValueOnce(false)
    await render(); await choose(full)
    expect(dialogs.confirmDialog).toHaveBeenCalledOnce()
    expect(createThread).not.toHaveBeenCalled()
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(current.value).toEqual(readOnly)
    expect(current.applying).toBe(false)
  })
  it('updates real next-turn metadata while the busy turn and its stream remain unchanged', async () => {
    const blocks: ChatBlock[] = [{ kind: 'user', id: 'running-user', text: 'Synthetic running prompt' }]
    useChatStore.setState({ busy: true, currentTurnId: 'running-turn', currentTurnUserId: 'running-user', blocks })
    await render(); await choose(workspaceWrite)
    expect(updatePolicy).toHaveBeenCalledExactlyOnceWith('a', workspaceWrite)
    expect(current.value).toEqual(workspaceWrite)
    expect(useChatStore.getState().busy).toBe(true)
    expect(useChatStore.getState().currentTurnId).toBe('running-turn')
    expect(useChatStore.getState().currentTurnUserId).toBe('running-user')
    expect(useChatStore.getState().blocks).toBe(blocks)
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
  })
  it('rejects partial or non-preset patches instead of merging them into ambient policy', async () => {
    await render()
    await act(async () => { current.onChange({ sandboxMode: 'danger-full-access' }); current.onChange({ approvalPolicy: 'untrusted', sandboxMode: 'danger-full-access' }) })
    expect(dialogs.confirmDialog).not.toHaveBeenCalled()
    expect(updatePolicy).not.toHaveBeenCalled()
  })
  it('ignores duplicate selections while native confirmation owns the pending operation', async () => {
    const confirmation = deferred<boolean>()
    vi.mocked(dialogs.confirmDialog).mockReturnValueOnce(confirmation.promise)
    await render(); await choose(full); await choose(workspaceWrite)
    expect(dialogs.confirmDialog).toHaveBeenCalledOnce()
    expect(updatePolicy).not.toHaveBeenCalled()
    await act(async () => confirmation.resolve(true))
    expect(updatePolicy).toHaveBeenCalledExactlyOnceWith('a', full)
  })
  it('retires a native confirmation across a batched A to B to A selection cycle', async () => {
    const confirmation = deferred<boolean>()
    vi.mocked(dialogs.confirmDialog).mockReturnValueOnce(confirmation.promise)
    await render(); await choose(full)
    await act(async () => { useChatStore.setState({ activeThreadId: 'b' }); useChatStore.setState({ activeThreadId: 'a' }) })
    expect(current.applying).toBe(false)
    await act(async () => confirmation.resolve(true))
    expect(updatePolicy).not.toHaveBeenCalled()
  })
  it.each(['workspace', 'route', 'runtime'] as const)('retires a confirmation when %s changes and returns before React renders', async kind => {
    const confirmation = deferred<boolean>()
    vi.mocked(dialogs.confirmDialog).mockReturnValueOnce(confirmation.promise)
    await render(); await choose(full)
    await act(async () => {
      if (kind === 'workspace') { useChatStore.setState({ workspaceRoot: '/synthetic/other' }); useChatStore.setState({ workspaceRoot: '/synthetic/workspace' }) }
      if (kind === 'route') { useChatStore.setState({ route: 'settings' }); useChatStore.setState({ route: 'chat' }) }
      if (kind === 'runtime') { useChatStore.setState({ runtimeConnection: 'offline' }); useChatStore.setState({ runtimeConnection: 'ready' }) }
    })
    await act(async () => confirmation.resolve(true))
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(current.applying).toBe(false)
  })
  it('lets an issued A mutation persist by ID without clearing the newer B operation or replacing B metadata', async () => {
    const a = deferred<boolean>(), b = deferred<boolean>()
    updatePolicy.mockImplementation(async (id, policy) => { const ok = await (id === 'a' ? a.promise : b.promise); if (ok) commitPolicy(id, policy); return ok })
    await render(); await choose(workspaceWrite)
    await act(async () => useChatStore.setState({ activeThreadId: 'b' }))
    await choose(full)
    expect(current.applying).toBe(true)
    await act(async () => a.resolve(true))
    expect(useChatStore.getState().threads.find(item => item.id === 'a')).toMatchObject(workspaceWrite)
    expect(current.value).toEqual(readOnly)
    expect(current.applying).toBe(true)
    await act(async () => b.resolve(true))
    expect(current.value).toEqual(full)
    expect(current.applying).toBe(false)
  })
  it('keeps the real last pair and the store uncertainty flag after a lost policy outcome', async () => {
    updatePolicy.mockImplementationOnce(async () => {
      useChatStore.setState(state => ({ threads: state.threads.map(item => item.id === 'a' ? { ...item, executionPolicyUncertain: true } : item) }))
      return false
    })
    await render(); await choose(workspaceWrite)
    expect(current.value).toEqual(readOnly)
    expect(current.unavailable).toBe(true)
    expect(current.applying).toBe(false)
  })
  it('keeps unknown metadata unavailable but permits an explicit preset to repair its actual owner', async () => {
    useChatStore.setState({ threads: [{ ...thread('a'), executionPolicyUncertain: true }] })
    await render()
    expect(current.unavailable).toBe(true)
    await choose(readOnly)
    expect(updatePolicy).toHaveBeenCalledExactlyOnceWith('a', readOnly)
    expect(current.unavailable).toBe(false)
  })
  it('does not substitute runtime defaults for missing thread fields', async () => {
    const missing = thread('a'); delete missing.approvalPolicy; delete missing.sandboxMode
    useChatStore.setState({ threads: [missing] })
    await render()
    expect(current.value).toBeNull()
    expect(current.unavailable).toBe(true)
    expect(rendererRuntimeClient.getSettings).not.toHaveBeenCalled()
    await choose(workspaceWrite)
    expect(current.value).toEqual(workspaceWrite)
    expect(current.unavailable).toBe(false)
  })
  it('retires a native confirmation on unmount without issuing a policy mutation', async () => {
    const confirmation = deferred<boolean>()
    vi.mocked(dialogs.confirmDialog).mockReturnValueOnce(confirmation.promise)
    await render(); await choose(full)
    await act(async () => root.render(null))
    await act(async () => confirmation.resolve(true))
    expect(updatePolicy).not.toHaveBeenCalled()
  })
})

describe('unmaterialized hero permission selection', () => {
  beforeEach(() => useChatStore.setState({ activeThreadId: null, threads: [] }))
  it('reads defaults only for display and creates nothing until a preset is chosen', async () => {
    vi.mocked(rendererRuntimeClient.getSettings).mockResolvedValueOnce(settings(full))
    await render()
    expect(current.value).toEqual(full)
    expect(createThread).not.toHaveBeenCalled()
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
  })
  it('cancels full-access confirmation before creating a thread or writing metadata', async () => {
    vi.mocked(dialogs.confirmDialog).mockResolvedValueOnce(false)
    await render(); await choose(full)
    expect(createThread).not.toHaveBeenCalled()
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(useChatStore.getState().activeThreadId).toBeNull()
  })
  it('materializes or reuses a real empty thread and PATCHes only its exact current receipt', async () => {
    createThread.mockImplementationOnce(async options => {
      expect(options?.materialize).toBe(true)
      expect(options?.forceNew).toBeUndefined()
      const target = thread('real-target', full, null)
      expect(options?.beforeSelect?.({ threadId: target.id, workspace: target.workspace! })).toBe(true)
      useChatStore.setState({ activeThreadId: target.id, threads: [target] })
      options?.onSelected?.({ threadId: target.id, isCurrent: () => options.isCurrent?.() !== false })
    })
    await render(); await choose(workspaceWrite)
    expect(createThread).toHaveBeenCalledOnce()
    expect(updatePolicy).toHaveBeenCalledExactlyOnceWith('real-target', workspaceWrite)
    expect(current.value).toEqual(workspaceWrite)
    expect(current.unavailable).toBe(false)
  })
  it('confirms full access before materializing even when it equals the displayed hero default', async () => {
    const order: string[] = []
    vi.mocked(rendererRuntimeClient.getSettings).mockResolvedValueOnce(settings(full))
    vi.mocked(dialogs.confirmDialog).mockImplementationOnce(async () => { order.push('confirm'); return true })
    createThread.mockImplementationOnce(async options => {
      order.push('create')
      const target = thread('full-target', full, null)
      expect(options?.beforeSelect?.({ threadId: target.id, workspace: target.workspace! })).toBe(true)
      useChatStore.setState({ activeThreadId: target.id, threads: [target] })
      options?.onSelected?.({ threadId: target.id, isCurrent: () => options.isCurrent?.() !== false })
    })
    updatePolicy.mockImplementationOnce(async (id, policy) => { order.push('patch'); commitPolicy(id, policy); return true })
    await render()
    expect(current.value).toEqual(full)
    await choose(full)
    expect(order).toEqual(['confirm', 'create', 'patch'])
    expect(current.value).toEqual(full)
    expect(current.legacy).toBe(false)
  })
  it('does not PATCH a created target without a current selection receipt', async () => {
    createThread.mockImplementationOnce(async options => {
      const target = thread('real-target', full, null)
      expect(options?.beforeSelect?.({ threadId: target.id, workspace: target.workspace! })).toBe(true)
      useChatStore.setState({ activeThreadId: target.id, threads: [target] })
      options?.onSelected?.({ threadId: target.id, isCurrent: () => false })
    })
    await render(); await choose(workspaceWrite)
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(current.value).toEqual(full)
    expect(current.legacy).toBe(true)
  })
  it('rejects an unexpected creation workspace before allowing selection', async () => {
    createThread.mockImplementationOnce(async options => {
      expect(options?.beforeSelect?.({ threadId: 'wrong-target', workspace: '/synthetic/other' })).toBe(false)
    })
    await render(); await choose(readOnly)
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(useChatStore.getState().activeThreadId).toBeNull()
  })
  it('retires creation after a workspace cycle even if its eventual target returns to the original workspace', async () => {
    const response = deferred<void>()
    createThread.mockImplementationOnce(async options => {
      await response.promise
      expect(options?.isCurrent?.()).toBe(false)
      expect(options?.beforeSelect?.({ threadId: 'late-target', workspace: '/synthetic/workspace' })).toBe(false)
    })
    await render(); await choose(readOnly)
    await act(async () => { useChatStore.setState({ workspaceRoot: '/synthetic/other' }); useChatStore.setState({ workspaceRoot: '/synthetic/workspace' }) })
    await act(async () => response.resolve())
    expect(updatePolicy).not.toHaveBeenCalled()
  })
  it('keeps a failed defaults read unavailable and does not invent a thread policy', async () => {
    vi.mocked(rendererRuntimeClient.getSettings).mockRejectedValueOnce(new Error('Synthetic defaults unavailable'))
    await render()
    expect(current.value).toBeNull()
    expect(current.unavailable).toBe(true)
    expect(createThread).not.toHaveBeenCalled()
    expect(updatePolicy).not.toHaveBeenCalled()
  })
})
