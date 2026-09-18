// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WriteRetrievalResult } from '@shared/write-retrieval'
import type { AppSettingsV1 } from '@shared/app-settings'
import type { NormalizedThread } from '../agent/types'
import type { FloatingComposerIsland } from './workbench/FloatingComposerIsland'

// Only presentation leaves and external IPC/Provider I/O are replaced. The
// mounted Workbench, plan controller, composer draft hook and stores are real.
const io = vi.hoisted(() => ({
  composer: null as ComponentProps<typeof FloatingComposerIsland> | null,
  document: null as { onSubmitPrompt: (value: string) => void } | null,
  provider: { sendUserMessage: vi.fn(), subscribeThreadEvents: vi.fn(), listThreads: vi.fn() },
  retrieve: vi.fn(), settings: vi.fn(), canvas: vi.fn(), read: vi.fn(), directory: vi.fn(), checkpoint: vi.fn(),
  t: (key: string) => key
}))
vi.mock('../agent/registry', () => ({ getProvider: () => io.provider }))
vi.mock('react-i18next', async (importOriginal) => ({
  ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: io.t })
}))
vi.mock('./workbench/FloatingComposerIsland', () => ({
  FloatingComposerIsland: (props: ComponentProps<typeof FloatingComposerIsland>) => {
    io.composer = props
    return null
  }
}))
vi.mock('./chat/Sidebar', () => ({ Sidebar: () => null }))
vi.mock('./chat/WorkbenchTopBar', () => ({ WorkbenchTopBar: () => null }))
vi.mock('./chat/AnimatedWorkLogo', () => ({ MascotCameoLayer: () => null, CameoCelebrationLayer: () => null }))
vi.mock('./chat/AssistantMarkdown', () => ({ preloadAssistantMarkdownRenderer: () => null }))
vi.mock('./chat/ChatFileTreePanel', () => ({ ChatFileTreePanel: () => null }))
vi.mock('./chat/SideConversationPanel', () => ({ SideConversationPanel: () => null }))
vi.mock('./SessionHeader', () => ({ SessionHeader: () => null }))
vi.mock('./shell/ShellNavigationControls', () => ({ ShellNavigationControls: () => null }))
vi.mock('./DevPreviewLaunchCard', () => ({ DevPreviewLaunchCard: () => null }))
vi.mock('./RuntimeBanner', () => ({ RuntimeBanner: () => null }))
vi.mock('./brand/AnalytixLoadingPage', () => ({ AnalytixLoadingPage: () => null }))
vi.mock('./workbench/RightPanelIslands', () => ({ ChangeInspectorIsland: () => null, DevBrowserPanelIsland: () => null, PlanPanel: () => null, SddAssistantPanelIsland: () => null, SubagentInspectorPanelIsland: () => null, ThreadSummaryPanelIsland: () => null, TodoPanel: () => null, WorkspaceFilePreviewPanel: () => null, DocumentWorkspacePanel: (props: { onSubmitPrompt: (value: string) => void }) => { io.document = props; return null }, preloadRightPanelIsland: () => null }))

vi.mock('./workbench/ChatTimelineIsland', () => ({ ChatTimelineIsland: () => null, useDevPreviewUrls: () => [] }))
import { Workbench } from './Workbench'
import { useChatStore } from '../store/chat-store'
import { useNativeReferenceStore, type CanvasNativeReference } from '../office/native-reference-store'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
import { useGuiPlanStore } from '../plan/plan-store'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { composerDraftKey } from '../store/composer-drafts'
import { clearBusyWatchdog, stopTurnCompletionPoll } from '../store/chat-store-schedulers'

const workspace = '/synthetic/canvas-send'
const thread = (id: string): NormalizedThread => ({ id, title: id,
  updatedAt: '2026-09-17T00:00:00.000Z', model: 'synthetic-model', mode: 'agent', workspace, status: 'running' })
const reference = (): Omit<CanvasNativeReference, 'id'> => ({ kind: 'canvas', threadId: 'a', workspace,
  path: workspace + '/private.canvas', objectId: 'a'.repeat(64), revision: 'b'.repeat(64),
  label: 'PRIVATE-LABEL', text: '', sessionId: 'c'.repeat(48), scopeId: 'd'.repeat(48),
  editable: true, selectedIds: ['PRIVATE-NODE'] })
const selectionResponse = () => ({ ok: true, selection: { sessionId: 'c'.repeat(48), threadId: 'a',
  scopeId: 'd'.repeat(48), baseRevision: 'b'.repeat(64), selectedIds: ['PRIVATE-NODE'], editable: true } })
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const composer = () => {
  expect(io.composer).not.toBeNull()
  return io.composer!
}
const draft = (id = 'a') => useChatStore.getState().composerDrafts[composerDraftKey(workspace, id)]
async function settle() { await act(async () => { for (let n = 0; n < 30; n++) await Promise.resolve() }) }
async function send() { await act(async () => { composer().onSend() }); await settle() }
let root: Root
let element: HTMLDivElement
let streams: (() => void)[]
const initialChat = useChatStore.getInitialState()
const initialWrite = useWriteWorkspaceStore.getInitialState()
const initialPlan = useGuiPlanStore.getInitialState()

beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  vi.resetAllMocks()
  streams = []
  io.document = null
  useWorkspaceTabsStore.setState(useWorkspaceTabsStore.getInitialState(), true)
  localStorage.clear()
  useChatStore.setState({ ...initialChat, threads: [thread('a'), thread('b')], activeThreadId: 'a',
    workspaceRoot: workspace, runtimeConnection: 'ready', composerModel: 'synthetic-model' }, true)
  useWriteWorkspaceStore.setState(initialWrite, true)
  useGuiPlanStore.setState(initialPlan, true)
  useNativeReferenceStore.setState({ references: [], drafts: {} })
  io.settings.mockResolvedValue({ workspaceRoot: workspace } as AppSettingsV1)
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockImplementation(() => io.settings())
  io.provider.listThreads.mockResolvedValue([thread('a'), thread('b')])
  io.provider.sendUserMessage.mockResolvedValue({ turnId: 'turn-a', userMessageItemId: 'message-a' })
  io.provider.subscribeThreadEvents.mockImplementation(() => new Promise<void>(resolve => streams.push(resolve)))
  io.canvas.mockImplementation(async () => selectionResponse())
  io.read.mockResolvedValue({ ok: true, content: 'SYNTHETIC-PUBLIC-FILE' })
  io.directory.mockResolvedValue({ ok: true, entries: [] })
  io.checkpoint.mockResolvedValue({ ok: false, reason: 'not_git_repo', message: 'synthetic fixture' })
  Object.assign(window, { analytix: {
    workspace: { onThreadHandoffEvent: () => () => {}, getThreadHandoffOperations: async () => ({ ok: true, operations: [] }),
      createGitCheckpoint: io.checkpoint },
    write: { retrieveWriteContext: io.retrieve },
    files: { read: io.read, listDirectory: io.directory }, canvas: { request: io.canvas },
    logs: { error: async () => {} }
  } })
  element = document.createElement('div')
  document.body.append(element)
  root = createRoot(element)
  await act(async () => root.render(createElement(Workbench)))
  await act(async () => { composer().setInput('Discuss selected objects'); useNativeReferenceStore.getState().add(reference()) })
})
afterEach(async () => {
  await act(async () => {
    root.unmount()
    useChatStore.setState({ ...initialChat, busy: false }, true)
    streams.forEach(resolve => resolve())
  })
  clearBusyWatchdog()
  stopTurnCompletionPoll()
  element.remove()
  vi.restoreAllMocks()
})

describe('Workbench Canvas consumer path', () => {
  it('does not send on attachment; ordinary success clears only the submitted draft and Canvas scope', async () => {
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(io.provider.sendUserMessage.mock.calls[0][0]).toBe('a')
    const prompt = io.provider.sendUserMessage.mock.calls[0][1] as string
    expect(prompt).toContain('d'.repeat(48))
    for (const value of [workspace, 'private.canvas', 'PRIVATE-NODE', 'PRIVATE-LABEL', 'a'.repeat(64), 'b'.repeat(64)]) {
      expect(prompt).not.toContain(value)
    }
    expect(draft().input).toBe('')
    expect(useNativeReferenceStore.getState().references).toHaveLength(0)
  })
  it('sends a Canvas plan through the actual plan controller and clears on acknowledged success', async () => {
    await act(async () => composer().setMode('plan'))
    await send()
    expect(io.directory).toHaveBeenCalled()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(io.provider.sendUserMessage.mock.calls[0][2]).toMatchObject({ mode: 'plan', guiPlan: { operation: 'draft', workspaceRoot: workspace } })
    const metadata = io.provider.sendUserMessage.mock.calls[0][2].guiPlan
    expect(metadata.sourceRequest).toBe('Discuss selected objects')
    expect(JSON.stringify(metadata)).not.toContain('d'.repeat(48))
    expect(draft().input).toBe('')
    expect(useNativeReferenceStore.getState().references).toHaveLength(0)
  })

  it.each(['deny', 'exception', 'read-false', 'read-exception', 'not-ready', 'provider-failure'])(
    'retains all draft assets and scope on %s without a duplicate submission', async failure => {
      await addAssets()
      const before = structuredClone(draft())
      const refs = structuredClone(useNativeReferenceStore.getState().references)
      if (failure === 'deny') io.canvas.mockResolvedValue({ ok: false, error: 'denied' })
      if (failure === 'exception') io.canvas.mockRejectedValue(new Error('synthetic Core failure'))
      if (failure === 'read-false') io.read.mockResolvedValue({ ok: false, message: 'synthetic file denied' })
      if (failure === 'read-exception') io.read.mockRejectedValue(new Error('synthetic read failure'))
      if (failure === 'not-ready') await act(async () => useChatStore.setState({ runtimeConnection: 'idle' }))
      if (failure === 'provider-failure') io.provider.sendUserMessage.mockRejectedValue(new Error('synthetic send rejected'))
      await send()
      expect(draft()).toEqual(before)
      expect(useNativeReferenceStore.getState().references).toEqual(refs)
      expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(failure === 'provider-failure' ? 1 : 0)
      expect(useChatStore.getState().queuedMessages).toEqual([])
    }
  )

  it.each(['revoke', 'thread', 'workspace'])('rejects %s during the actual composer file read', async change => {
    await addAssets()
    const read = deferred<{ ok: true; content: string }>()
    io.read.mockReturnValueOnce(read.promise)
    await send()
    expect(io.read).toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    if (change === 'revoke') {
      await act(async () => useNativeReferenceStore.getState().revokeScopes(reference().objectId))
      io.canvas.mockResolvedValue({ ok: false, error: 'scope revoked' })
    } else if (change === 'thread') {
      await switchThread()
    } else {
      await act(async () => useChatStore.setState({ workspaceRoot: '/synthetic/other',
        threads: [{ ...thread('a'), workspace: '/synthetic/other' }, thread('b')] }))
    }
    read.resolve({ ok: true, content: 'SYNTHETIC-PUBLIC-FILE' })
    await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(draft().attachments).toHaveLength(1)
    expect(draft().fileReferences).toHaveLength(1)
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    if (change === 'thread') expect(useChatStore.getState().error).toBe('new-thread-error')
  })

  it('does not retarget a pending Canvas plan when the thread changes during plan directory I/O', async () => {
    await act(async () => composer().setMode('plan'))
    const directory = deferred<{ ok: true; entries: [] }>()
    io.directory.mockReturnValueOnce(directory.promise)
    await send()
    expect(io.directory).toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    await switchThread()
    directory.resolve({ ok: true, entries: [] })
    await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(useChatStore.getState().error).toBe('new-thread-error')
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it.each(['settings', 'checkpoint'])('rechecks Core after late %s I/O, not just in Workbench preparation', async boundary => {
    const settings = deferred<AppSettingsV1>()
    const checkpoint = deferred<{ ok: false; reason: 'not_git_repo'; message: string }>()
    if (boundary === 'settings') io.settings.mockReturnValueOnce(settings.promise)
    else io.checkpoint.mockReturnValueOnce(checkpoint.promise)
    await send()
    expect(io.canvas).toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    io.canvas.mockResolvedValue({ ok: false, error: 'scope revoked after Workbench preparation' })
    if (boundary === 'settings') settings.resolve({ workspaceRoot: workspace } as AppSettingsV1)
    else checkpoint.resolve({ ok: false, reason: 'not_git_repo', message: 'synthetic fixture' })
    await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(useChatStore.getState().busy).toBe(false)
    expect(useChatStore.getState().blocks).toEqual([])
  })

  it('retains later edits, attachments, files and a replacement Canvas reference after a late success', async () => {
    await addAssets()
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    await act(async () => {
      composer().setInput('later input')
      useChatStore.getState().updateComposerDraft(composerDraftKey(workspace, 'a'), current => ({ ...current,
        attachments: [...current.attachments, { id: 'attachment-next', kind: 'document', name: 'next.txt' }],
        fileReferences: [...current.fileReferences, { path: workspace + '/next.txt', name: 'next.txt', relativePath: 'next.txt' }] }))
      useNativeReferenceStore.getState().add({ ...reference(), scopeId: 'e'.repeat(48) })
    })
    receipt.resolve({ turnId: 'turn-a', userMessageItemId: 'message-a' })
    await settle()
    expect(draft().input).toBe('later input')
    expect(draft().attachments.map(item => item.id)).toEqual(['attachment-next'])
    expect(draft().fileReferences.map(item => item.name)).toEqual(['next.txt'])
    expect(useNativeReferenceStore.getState().references.map(item => item.scopeId)).toEqual(['e'.repeat(48)])
  })

  it('does not write a late successful send receipt into a different thread', async () => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    await switchThread()
    receipt.resolve({ turnId: 'turn-a', userMessageItemId: 'message-a' })
    await settle()
    expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'b', currentTurnId: null,
      currentTurnUserId: null, blocks: [], error: 'new-thread-error', busy: false })
    expect(draft().input).toBe('')
    expect(draft('b').input).toBe('new thread draft')
  })

  it('does not duplicate one Canvas submission while its file preparation is still pending', async () => {
    await addAssets()
    const read = deferred<{ ok: true; content: string }>()
    io.read.mockReturnValue(read.promise)
    await send()
    await send()
    read.resolve({ ok: true, content: 'SYNTHETIC-PUBLIC-FILE' })
    await settle()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(useChatStore.getState().queuedMessages).toEqual([])
    expect(io.read).toHaveBeenCalledTimes(1)
  })


  it.each(['revoke', 'thread'])('rejects %s during real document-action retrieval', async change => {
    const retrieved = deferred<WriteRetrievalResult>()
    io.retrieve.mockReturnValueOnce(retrieved.promise)
    await act(async () => useWriteWorkspaceStore.setState({ workspaceRoot: workspace,
      activeFilePath: workspace + '/synthetic.md', saveStatus: 'saved' }))
    expect(io.document).not.toBeNull()
    await act(async () => io.document!.onSubmitPrompt('Discuss selected objects'))
    await settle()
    expect(io.retrieve).toHaveBeenCalledOnce()
    if (change === 'thread') await switchThread()
    else {
      await act(async () => useNativeReferenceStore.getState().revokeScopes(reference().objectId))
      io.canvas.mockResolvedValue({ ok: false, error: 'revoked during retrieval' })
    }
    retrieved.resolve({ ok: false, message: 'Synthetic optional context unavailable' })
    await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    if (change === 'thread') expect(useChatStore.getState().error).toBe('new-thread-error')
  })

  it('preserves a Canvas submission when the real send owner refuses a busy turn; no unvalidated queue', async () => {
    await addAssets()
    const before = structuredClone(draft())
    await act(async () => useChatStore.setState({ busy: true, currentTurnId: 'existing-turn' }))
    await send()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(useChatStore.getState().queuedMessages).toEqual([])
    expect(draft()).toEqual(before)
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
  })

  it('retains the draft when final Core validation throws after preparation succeeded', async () => {
    io.canvas.mockResolvedValueOnce(selectionResponse()).mockRejectedValueOnce(new Error('late Core unavailable'))
    await send()
    expect(io.canvas).toHaveBeenCalledTimes(2)
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useChatStore.getState()).toMatchObject({ busy: false, blocks: [] })
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
  })

  it('does not pollute a new thread on a late Provider failure', async () => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    await send()
    await switchThread()
    receipt.reject(new Error('synthetic old send failed'))
    await settle()
    expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'b', busy: false, blocks: [],
      currentTurnId: null, currentTurnUserId: null, error: 'new-thread-error' })
    expect(draft().input).toBe('Discuss selected objects')
    expect(draft('b').input).toBe('new thread draft')
  })

  it('keeps Office discussion snapshots on the original ordinary send path', async () => {
    await act(async () => {
      useNativeReferenceStore.setState({ references: [] })
      useNativeReferenceStore.getState().add({ kind: 'office', threadId: 'a', workspace,
        path: workspace + '/synthetic.docx', objectId: '1'.repeat(64), revision: '2'.repeat(64),
        label: 'synthetic.docx', text: 'SYNTHETIC OFFICE QUOTE', note: 'SYNTHETIC NOTE',
        selection: { kind: 'text', documentId: '1'.repeat(64), version: '2'.repeat(64),
          changeSequence: 0, text: 'SYNTHETIC OFFICE QUOTE',
          scope: 'current-view-text-only; no verified structural offset or durable anchor' } })
    })
    await send()
    expect(io.canvas).not.toHaveBeenCalled()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    const message = io.provider.sendUserMessage.mock.calls[0][1]
    expect(message).toContain('[原生文档引用] synthetic.docx')
    expect(message).toContain('SYNTHETIC OFFICE QUOTE')
    expect(message).toContain('SYNTHETIC NOTE')
    expect(io.provider.sendUserMessage.mock.calls[0][2].submissionGuard).toBeUndefined()
    expect(draft().input).toBe('')
    expect(useNativeReferenceStore.getState().references).toHaveLength(0)
  })

})

async function addAssets() {
  await act(async () => useChatStore.getState().updateComposerDraft(composerDraftKey(workspace, 'a'), current => ({
    ...current, attachments: [{ id: 'attachment-a', kind: 'document', name: 'synthetic.txt', documentText: 'SYNTHETIC-PUBLIC-DOCUMENT' }],
    fileReferences: [{ path: workspace + '/public.txt', name: 'public.txt', relativePath: 'public.txt' }]
  })))
}
async function switchThread() {
  await act(async () => useChatStore.setState({ activeThreadId: 'b', busy: false, blocks: [],
    currentTurnId: null, currentTurnUserId: null, error: 'new-thread-error' }))
  await act(async () => composer().setInput('new thread draft'))
}
