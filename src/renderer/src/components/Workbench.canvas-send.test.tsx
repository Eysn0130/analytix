// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WriteRetrievalResult } from '@shared/write-retrieval'
import type { AppSettingsV1 } from '@shared/app-settings'
import type { NormalizedThread, ThreadEventSink } from '../agent/types'
import type { FloatingComposerIsland } from './workbench/FloatingComposerIsland'

// Only presentation leaves and external IPC/Provider I/O are replaced. The
// mounted Workbench, plan controller, composer draft hook and stores are real.
const io = vi.hoisted(() => ({
  composer: null as ComponentProps<typeof FloatingComposerIsland> | null,
  summaryMounted: vi.fn(),
  document: null as { onSubmitPrompt: (value: string, references?: import('../office/native-reference-store').NativeReference[]) => void } | null,
  provider: { sendUserMessage: vi.fn(), subscribeThreadEvents: vi.fn(), listThreads: vi.fn(), getThreadDetail: vi.fn() },
  browser: vi.fn(), retrieve: vi.fn(), settings: vi.fn(), canvas: vi.fn(), read: vi.fn(), directory: vi.fn(), checkpoint: vi.fn(),
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
vi.mock('./workbench/RightPanelIslands', () => ({ ChangeInspectorIsland: () => null, DevBrowserPanelIsland: () => null, PlanPanel: () => null, SddAssistantPanelIsland: () => null, SubagentInspectorPanelIsland: () => null, ThreadSummaryPanelIsland: () => { io.summaryMounted(); return null }, TodoPanel: () => null, WorkspaceFilePreviewPanel: () => null, DocumentWorkspacePanel: (props: { onSubmitPrompt: (value: string, references?: import('../office/native-reference-store').NativeReference[]) => void }) => { io.document = props; return null }, preloadRightPanelIsland: () => null }))

vi.mock('./workbench/ChatTimelineIsland', () => ({ ChatTimelineIsland: () => null, useDevPreviewUrls: () => [] }))
import { Workbench } from './Workbench'
import { invalidateThreadDetailCache } from '../lib/thread-detail-cache'
import { useChatStore } from '../store/chat-store'
import { useNativeReferenceStore, type CanvasNativeReference } from '../office/native-reference-store'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
import { useGuiPlanStore } from '../plan/plan-store'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { composerDraftKey } from '../store/composer-drafts'
import { clearBusyWatchdog, stopTurnCompletionPoll } from '../store/chat-store-schedulers'
import * as schedulers from '../store/chat-store-schedulers'
import * as runtime from '../store/chat-store-runtime'
import { appendActiveStreamDeltas, clearActiveStream, getActiveStreamSnapshotFor } from '../thread/streaming/active-stream-store'

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
  clearActiveStream()
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
    files: { read: io.read, listDirectory: io.directory }, canvas: { request: io.canvas }, browserSelection: { request: io.browser },
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
  clearActiveStream()
  element.remove()
  vi.restoreAllMocks()
})

describe('Workbench Canvas consumer path', () => {
  it('stops mounting the summary poller when the right pane closes', async () => {
    await act(async () => useWorkspaceTabsStore.getState().openTab({
      id: 'tool:summary', kind: 'tool', mode: 'summary', title: 'Summary'
    }))
    expect(io.summaryMounted).toHaveBeenCalled()

    io.summaryMounted.mockClear()
    await act(async () => useWorkspaceTabsStore.getState().setOpen(false))
    expect(io.summaryMounted).not.toHaveBeenCalled()
  })

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


describe('Reaudit: actual selectThread A-B-A with a delayed send receipt', () => {
  it.each(['success', 'failure'])('keeps a newer running snapshot when the old %s arrives', async outcome => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => ({
      thread: thread(id), blocks: id === 'a' ? [{ id: 'newer-user-a', kind: 'user', text: 'newer request' }] : [],
      latestSeq: 12, threadStatus: id === 'a' ? 'running' : 'idle',
      latestTurnId: id === 'a' ? 'newer-turn-a' : null,
      latestUserMessageId: id === 'a' ? 'newer-user-a' : null,
      turnDurationByUserId: {}
    }))
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    invalidateThreadDetailCache('b')
    await act(async () => useChatStore.getState().selectThread('b'))
    expect(useChatStore.getState().activeThreadId).toBe('b')
    invalidateThreadDetailCache('a')
    await act(async () => useChatStore.getState().selectThread('a'))
    expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'a', busy: true,
      currentTurnId: 'newer-turn-a', currentTurnUserId: 'newer-user-a' })
    await act(async () => composer().setInput('new input after returning'))
    const snapshot = useChatStore.getState()
    if (outcome === 'success') receipt.resolve({ turnId: 'old-turn-a', userMessageItemId: 'old-user-a' })
    else receipt.reject(new Error('old request failed'))
    await settle()
    const state = useChatStore.getState()
    console.info('REAUDIT_OBSERVATION', JSON.stringify({scenario: 'newer-running', outcome,
      expected: {turn: snapshot.currentTurnId, user: snapshot.currentTurnUserId, busy: snapshot.busy, error:snapshot.error},
      actual: {turn:state.currentTurnId,user:state.currentTurnUserId,busy:state.busy,error:state.error}}))
    expect(state.currentTurnId).toBe('newer-turn-a')
    expect(state.currentTurnUserId).toBe('newer-user-a')
    expect(state.busy).toBe(true)
    expect(state.error).toBe(snapshot.error)
    expect(state.blocks).toEqual(snapshot.blocks)
    expect(draft().input).toBe('new input after returning')
  })
  it('does not resurrect a completed turn after actual selectThread loaded its idle snapshot', async () => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => ({
      thread: { ...thread(id), status: 'idle' },
      blocks: id === 'a' ? [
        { id: 'old-user-a', kind: 'user', text: 'Discuss selected objects', meta: {turnId: 'old-turn-a'} },
        { id: 'old-answer-a', kind: 'assistant', text: 'Completed before HTTP ack', meta: {turnId: 'old-turn-a'} }
      ] : [{ id: 'b-user', kind: 'user', text: 'Other completed conversation' }],
      latestSeq: 12, threadStatus: 'idle', latestTurnId: id === 'a' ? 'old-turn-a' : null,
      latestUserMessageId: id === 'a' ? 'old-user-a' : null, turnDurationByUserId: { 'old-user-a': 20 }
    }))
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    invalidateThreadDetailCache('b')
    await act(async () => useChatStore.getState().selectThread('b'))
    expect(useChatStore.getState().activeThreadId).toBe('b')
    invalidateThreadDetailCache('a')
    await act(async () => useChatStore.getState().selectThread('a'))
    expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'a', busy: false,
      currentTurnId: null, currentTurnUserId: null })
    const snapshot = useChatStore.getState()
    receipt.resolve({ turnId: 'old-turn-a', userMessageItemId: 'old-user-a' })
    await settle()
    const state = useChatStore.getState()
    console.info('REAUDIT_OBSERVATION', JSON.stringify({scenario: 'completed-before-ack',
      expected: {turn:null,user:null,busy:false},actual: {turn:state.currentTurnId,user:state.currentTurnUserId,busy:state.busy}}))
    expect(state.currentTurnId).toBeNull()
    expect(state.currentTurnUserId).toBeNull()
    expect(state.busy).toBe(false)
    expect(state.turnDurationByUserId).toEqual(snapshot.turnDurationByUserId)
  })
})


const currentSink = (): ThreadEventSink => io.provider.subscribeThreadEvents.mock.calls.at(-1)![2]
function runningDetail(id: string) {
  return { thread: thread(id), blocks: [{ id: `newer-user-${id}`, kind: 'user' as const, text: 'Newer request' }],
    latestSeq: 20, threadStatus: 'running', latestTurnId: `newer-turn-${id}`,
    latestUserMessageId: `newer-user-${id}`, turnDurationByUserId: {} }
}
async function select(id: string) {
  invalidateThreadDetailCache(id)
  await act(async () => useChatStore.getState().selectThread(id))
  expect(useChatStore.getState().activeThreadId).toBe(id)
}
function completedDetail(id: string) {
  return { thread: { ...thread(id), status: 'idle' as const }, blocks: [
    { id: 'stable-user', kind: 'user' as const, text: 'Discuss selected objects', meta: { turnId: 'stable-turn' } },
    { id: 'answer', kind: 'assistant' as const, text: 'Completed answer', meta: { turnId: 'stable-turn' } }
  ], latestSeq: 20, threadStatus: 'idle', latestTurnId: 'stable-turn',
  latestUserMessageId: 'stable-user', turnDurationByUserId: { 'stable-user': 50 } }
}
function receiptSideEffects() {
  return { arm: vi.spyOn(runtime, 'armBusyWatchdog'), clear: vi.spyOn(schedulers, 'clearBusyWatchdog'),
    poll: vi.spyOn(runtime, 'syncTurnCompletionPoll'), subscriptions: io.provider.subscribeThreadEvents.mock.calls.length,
    refreshes: io.provider.listThreads.mock.calls.length }
}
function expectNoReceiptSideEffects(before: ReturnType<typeof receiptSideEffects>) {
  expect(before.arm).not.toHaveBeenCalled()
  expect(before.clear).not.toHaveBeenCalled()
  expect(before.poll).not.toHaveBeenCalled()
  expect(io.provider.subscribeThreadEvents).toHaveBeenCalledTimes(before.subscriptions)
  expect(io.provider.listThreads).toHaveBeenCalledTimes(before.refreshes)
}

describe('Workbench receipt ownership compatibility', () => {
  it.each(['agent', 'plan', 'no-user-id'] as const)('accepts SSE-normalized IDs before the %s HTTP acknowledgement', async mode => {
    const receipt = deferred<{ turnId: string; userMessageItemId?: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.listThreads.mockResolvedValue([{ ...thread('a'), title: 'Current synthetic conversation' },
      { ...thread('b'), title: 'Other synthetic conversation' }])
    await addAssets()
    if (mode === 'plan') await act(async () => composer().setMode('plan'))
    await send()
    const temporaryId = useChatStore.getState().currentTurnUserId!
    const started = useChatStore.getState().turnStartedAtByUserId[temporaryId]
    const sink = currentSink()
    await act(async () => {
      sink.onTurnStarted?.({ turnId: 'stable-turn', threadId: 'a', seq: 2, createdAt: new Date().toISOString() })
      sink.onUserMessage({ itemId: 'stable-user', turnId: 'stable-turn', text: 'Discuss selected objects', createdAt: new Date().toISOString() })
      // The stream store retains sequencing, but intentionally withholds raw text.
      appendActiveStreamDeltas({ threadId: 'a', turnId: 'stable-turn', lastSeq: 3 })
    })
    expect(useChatStore.getState()).toMatchObject({ busy: true, currentTurnId: 'stable-turn', currentTurnUserId: 'stable-user' })
    const stream = getActiveStreamSnapshotFor('a', 'stable-turn')
    expect(stream).toMatchObject({ threadId: 'a', turnId: 'stable-turn', lastSeq: 3, liveAssistant: '' })
    const arm = vi.spyOn(runtime, 'armBusyWatchdog')
    receipt.resolve({ turnId: 'stable-turn', ...(mode === 'no-user-id' ? {} : { userMessageItemId: 'stable-user' }) })
    await settle()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(useChatStore.getState()).toMatchObject({ busy: true, currentTurnId: 'stable-turn', currentTurnUserId: 'stable-user' })
    expect(useChatStore.getState().blocks.filter(block => block.kind === 'user')).toHaveLength(1)
    expect(getActiveStreamSnapshotFor('a', 'stable-turn')).toEqual(stream)
    expect(useChatStore.getState().turnStartedAtByUserId['stable-user']).toBe(started)
    expect(useChatStore.getState().turnStartedAtByUserId[temporaryId]).toBeUndefined()
    expect(arm).toHaveBeenCalledOnce()
    expect(draft().input).toBe('')
    expect(draft().attachments).toEqual([])
    expect(draft().fileReferences).toEqual([])
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })

  it('acknowledges a completed real SSE turn without reviving its state, stream or timers', async () => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => completedDetail(id))
    await addAssets()
    await send()
    const sink = currentSink()
    await act(async () => {
      sink.onUserMessage({ itemId: 'stable-user', turnId: 'stable-turn', text: 'Discuss selected objects', createdAt: new Date().toISOString() })
      await sink.onTurnComplete({ threadId: 'a', turnId: 'stable-turn', seq: 20 })
    })
    await settle()
    const snapshot = useChatStore.getState()
    expect(snapshot).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    const stream = getActiveStreamSnapshotFor('a')
    const effects = receiptSideEffects()
    receipt.resolve({ turnId: 'stable-turn', userMessageItemId: 'stable-user' })
    await settle()
    const state = useChatStore.getState()
    expect(state).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    expect(state.blocks).toEqual(snapshot.blocks)
    expect(state.turnDurationByUserId).toEqual(snapshot.turnDurationByUserId)
    expect(getActiveStreamSnapshotFor('a')).toEqual(stream)
    expectNoReceiptSideEffects(effects)
    expect(draft().input).toBe('')
    expect(draft().attachments).toEqual([])
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })

  it.each(['success', 'failure'])('keeps the actual B subscription and clears only acknowledged A assets on old %s', async outcome => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => runningDetail(id))
    await addAssets()
    const originalDraft = structuredClone(draft())
    await send()
    await select('b')
    await act(async () => composer().setInput('B must remain untouched'))
    const snapshot = useChatStore.getState()
    const stream = getActiveStreamSnapshotFor('b')
    const effects = receiptSideEffects()
    if (outcome === 'success') receipt.resolve({ turnId: 'old-turn', userMessageItemId: 'old-user' })
    else receipt.reject(new Error('old failure'))
    await settle()
    expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'b', busy: true,
      currentTurnId: snapshot.currentTurnId, currentTurnUserId: snapshot.currentTurnUserId, error: snapshot.error })
    expect(useChatStore.getState().blocks).toEqual(snapshot.blocks)
    expect(getActiveStreamSnapshotFor('b')).toEqual(stream)
    expectNoReceiptSideEffects(effects)
    expect(draft('b').input).toBe('B must remain untouched')
    if (outcome === 'success') {
      expect(draft('a').input).toBe('')
      expect(draft('a').attachments).toEqual([])
    } else expect(draft('a')).toEqual(originalDraft)
  })

  it.each(['success', 'failure'])('does not migrate a stale stream or touch the new watchdog/poll after A-B-A %s', async outcome => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => runningDetail(id))
    await send()
    const oldPending = useChatStore.getState().currentTurnId!
    const oldStream = getActiveStreamSnapshotFor('a', oldPending)
    await select('b')
    await select('a')
    const stream = getActiveStreamSnapshotFor('a')
    const effects = receiptSideEffects()
    const timing = useChatStore.getState().turnStartedAtByUserId
    const queue = useChatStore.getState().queuedMessages
    if (outcome === 'success') receipt.resolve({ turnId: 'old-turn', userMessageItemId: 'old-user' })
    else receipt.reject(new Error('old failure'))
    await settle()
    expect(getActiveStreamSnapshotFor('a')).toEqual(stream)
    expect(getActiveStreamSnapshotFor('a', oldPending)).toEqual(oldStream)
    expect(useChatStore.getState().turnStartedAtByUserId).toEqual(timing)
    expect(useChatStore.getState().queuedMessages).toEqual(queue)
    expectNoReceiptSideEffects(effects)
  })

  it('does not adopt a reselected snapshot even when its stable IDs match the late receipt', async () => {
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => ({ ...runningDetail(id),
      latestTurnId: 'stable-turn', latestUserMessageId: 'stable-user',
      blocks: [{ id: 'stable-user', kind: 'user', text: 'Authoritative snapshot', meta: { turnId: 'stable-turn' } }] }))
    await send()
    await select('b')
    await select('a')
    const snapshot = useChatStore.getState()
    const effects = receiptSideEffects()
    receipt.resolve({ turnId: 'stable-turn', userMessageItemId: 'stable-user' })
    await settle()
    expect(useChatStore.getState().blocks).toEqual(snapshot.blocks)
    expect(useChatStore.getState().turnStartedAtByUserId).toEqual(snapshot.turnStartedAtByUserId)
    expectNoReceiptSideEffects(effects)
    expect(draft().input).toBe('')
  })

  it.each(['success', 'failure'])('keeps a newer send owner on the same subscription and millisecond after old %s', async outcome => {
    vi.spyOn(Date, 'now').mockReturnValue(1789707000000)
    const oldReceipt = deferred<{ turnId: string; userMessageItemId: string }>()
    const newReceipt = deferred<{ turnId: string; userMessageItemId: string }>()
    io.provider.sendUserMessage.mockReturnValueOnce(oldReceipt.promise).mockReturnValueOnce(newReceipt.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => completedDetail(id))
    await send()
    const oldPending = useChatStore.getState().currentTurnId
    const sink = currentSink()
    await act(async () => {
      sink.onUserMessage({ itemId: 'stable-user', turnId: 'stable-turn', text: 'Discuss selected objects' })
      await sink.onTurnComplete({ threadId: 'a', turnId: 'stable-turn', seq: 20 })
    })
    await settle()
    expect(useChatStore.getState().busy).toBe(false)
    let pendingNew!: Promise<boolean>
    await act(async () => { pendingNew = useChatStore.getState().sendMessage('Newer ordinary request', 'agent') })
    await settle()
    expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(2)
    // Even equal optimistic IDs cannot give the first operation ownership again.
    expect(useChatStore.getState().currentTurnId).toBe(oldPending)
    const snapshot = useChatStore.getState()
    const effects = receiptSideEffects()
    if (outcome === 'success') oldReceipt.resolve({ turnId: 'stable-turn', userMessageItemId: 'stable-user' })
    else oldReceipt.reject(new Error('Old request rejected after a newer send'))
    await settle()
    expect(useChatStore.getState()).toMatchObject({ busy: true, currentTurnId: snapshot.currentTurnId,
      currentTurnUserId: snapshot.currentTurnUserId, error: snapshot.error })
    expect(useChatStore.getState().blocks).toEqual(snapshot.blocks)
    expectNoReceiptSideEffects(effects)
    newReceipt.resolve({ turnId: 'new-turn', userMessageItemId: 'new-user' })
    await act(async () => { expect(await pendingNew).toBe(true) })
    expect(useChatStore.getState()).toMatchObject({ busy: true, currentTurnId: 'new-turn', currentTurnUserId: 'new-user' })
  })

  it.each(['resolve', 'reject'])('fences a receipt-triggered refresh that finishes after A-B-A (%s)', async outcome => {
    const listing = deferred<NormalizedThread[]>()
    io.provider.listThreads.mockReturnValueOnce(listing.promise)
    io.provider.getThreadDetail.mockImplementation(async (id: string) => runningDetail(id))
    await send()
    expect(io.provider.listThreads).toHaveBeenCalledOnce()
    await select('b')
    await select('a')
    await act(async () => composer().setInput('Newer draft during old refresh'))
    const snapshot = useChatStore.getState()
    const effects = receiptSideEffects()
    if (outcome === 'resolve') listing.resolve([{ ...thread('a'), title: 'STALE LIST TITLE' }, thread('b')])
    else listing.reject(new Error('stale refresh failed'))
    await settle()
    expect(useChatStore.getState().threads).toEqual(snapshot.threads)
    expect(useChatStore.getState()).toMatchObject({ runtimeConnection: snapshot.runtimeConnection,
      activeThreadId: 'a', currentTurnId: snapshot.currentTurnId, currentTurnUserId: snapshot.currentTurnUserId, error: snapshot.error })
    expectNoReceiptSideEffects(effects)
    expect(draft().input).toBe('Newer draft during old refresh')
  })
})

describe('R07 reaudit: normal receipt follow-up lifetime', () => {
  it.each(['success', 'failure'])('continues a building case index after a current send returns (%s)', async outcome => {
    vi.useFakeTimers()
    const listCaseProjects = vi.fn()
      .mockResolvedValueOnce({ caseProjects: [], indexStatus: 'building' })
      .mockResolvedValue({ caseProjects: [], indexStatus: 'ready' })
    Object.assign(io.provider, { listCaseProjects })
    if (outcome === 'failure') io.provider.sendUserMessage.mockRejectedValueOnce(new Error('Synthetic current send rejected'))
    try {
      await send()
      expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
      expect(draft().input).toBe(outcome === 'success' ? '' : 'Discuss selected objects')
      expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'a', busy: outcome === 'success',
        currentTurnId: outcome === 'success' ? 'turn-a' : null,
        currentTurnUserId: outcome === 'success' ? 'message-a' : null, caseProjectIndexStatus: 'building' })
      expect(listCaseProjects).toHaveBeenCalledOnce()
      await act(async () => { await vi.advanceTimersByTimeAsync(1500) })
      await settle()
      console.info('R07_OBSERVATION', JSON.stringify({ scenario: 'normal-follow-up', outcome,
        listCalls: listCaseProjects.mock.calls.length, indexStatus: useChatStore.getState().caseProjectIndexStatus,
        activeThreadId: useChatStore.getState().activeThreadId, draft: draft().input }))
      expect(listCaseProjects).toHaveBeenCalledTimes(2)
      expect(useChatStore.getState().caseProjectIndexStatus).toBe('ready')
    } finally {
      Reflect.deleteProperty(io.provider, 'listCaseProjects')
      vi.useRealTimers()
    }
  })

  it('does not let an obsolete receipt timer suppress a newer independent index refresh', async () => {
    vi.useFakeTimers()
    const listCaseProjects = vi.fn()
      .mockResolvedValueOnce({ caseProjects: [], indexStatus: 'building' })
      .mockResolvedValueOnce({ caseProjects: [], indexStatus: 'building' })
      .mockResolvedValue({ caseProjects: [], indexStatus: 'ready' })
    Object.assign(io.provider, { listCaseProjects })
    try {
      await send()
      expect(draft().input).toBe('')
      expect(listCaseProjects).toHaveBeenCalledOnce()
      await act(async () => { await useChatStore.getState().refreshCaseProjects() })
      expect(listCaseProjects).toHaveBeenCalledTimes(2)
      expect(useChatStore.getState().caseProjectIndexStatus).toBe('building')
      await act(async () => { await vi.advanceTimersByTimeAsync(1500) })
      await settle()
      console.info('R07_OBSERVATION', JSON.stringify({ scenario: 'newer-independent-index-refresh',
        listCalls: listCaseProjects.mock.calls.length, indexStatus: useChatStore.getState().caseProjectIndexStatus }))
      expect(listCaseProjects).toHaveBeenCalledTimes(3)
      expect(useChatStore.getState().caseProjectIndexStatus).toBe('ready')
    } finally {
      Reflect.deleteProperty(io.provider, 'listCaseProjects')
      vi.useRealTimers()
    }
  })

  it('does not run an old receipt index timer after real A-B-A navigation replaced its subscription', async () => {
    vi.useFakeTimers()
    const listCaseProjects = vi.fn()
      .mockResolvedValueOnce({ caseProjects: [], indexStatus: 'building' })
      .mockResolvedValue({ caseProjects: [], indexStatus: 'ready' })
    Object.assign(io.provider, { listCaseProjects })
    io.provider.getThreadDetail.mockImplementation(async (id: string) => ({
      thread: thread(id), blocks: [{ kind: 'user', id: 'replacement-user-' + id, text: 'Replacement request' }],
      latestSeq: 42, threadStatus: 'running', latestTurnId: 'replacement-turn-' + id,
      latestUserMessageId: 'replacement-user-' + id, turnDurationByUserId: {}
    }))
    try {
      await send()
      expect(draft().input).toBe('')
      expect(listCaseProjects).toHaveBeenCalledOnce()
      invalidateThreadDetailCache('b')
      await act(async () => { await useChatStore.getState().selectThread('b') })
      expect(useChatStore.getState().activeThreadId).toBe('b')
      invalidateThreadDetailCache('a')
      await act(async () => { await useChatStore.getState().selectThread('a') })
      expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'a', currentTurnId: 'replacement-turn-a' })
      const snapshot = useChatStore.getState()
      await act(async () => { await vi.advanceTimersByTimeAsync(1500) })
      await settle()
      expect(listCaseProjects).toHaveBeenCalledOnce()
      expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'a', currentTurnId: 'replacement-turn-a',
        caseProjectIndexStatus: snapshot.caseProjectIndexStatus, error: snapshot.error })
    } finally {
      Reflect.deleteProperty(io.provider, 'listCaseProjects')
      vi.useRealTimers()
    }
  })
})


describe('R07 follow-up consumers after the send promise settles', () => {
  it.each(['current', 'stale-resolve', 'stale-reject'])('checks delayed timer I/O against the actual receipt context (%s)', async outcome => {
    vi.useFakeTimers()
    const listing = deferred<{ caseProjects: never[]; indexStatus: 'ready' }>()
    const listCaseProjects = vi.fn()
      .mockResolvedValueOnce({ caseProjects: [], indexStatus: 'building' })
      .mockReturnValueOnce(listing.promise)
    Object.assign(io.provider, { listCaseProjects })
    io.provider.getThreadDetail.mockImplementation(async (id: string) => runningDetail(id))
    try {
      await send()
      expect(draft().input).toBe('')
      await act(async () => { await vi.advanceTimersByTimeAsync(1500) })
      expect(listCaseProjects).toHaveBeenCalledTimes(2)
      if (outcome !== 'current') {
        await select('b')
        await select('a')
      }
      await act(async () => composer().setInput('Later unsent draft'))
      const snapshot = useChatStore.getState()
      const effects = receiptSideEffects()
      if (outcome === 'stale-reject') listing.reject(new Error('Synthetic obsolete index response'))
      else listing.resolve({ caseProjects: [], indexStatus: 'ready' })
      await settle()
      expect(useChatStore.getState()).toMatchObject({ activeThreadId: 'a', busy: snapshot.busy,
        currentTurnId: snapshot.currentTurnId, currentTurnUserId: snapshot.currentTurnUserId,
        caseProjectIndexStatus: outcome === 'current' ? 'ready' : snapshot.caseProjectIndexStatus,
        runtimeConnection: snapshot.runtimeConnection, error: snapshot.error })
      expect(draft().input).toBe('Later unsent draft')
      expectNoReceiptSideEffects(effects)
      await act(async () => { await vi.advanceTimersByTimeAsync(1500) })
      expect(listCaseProjects).toHaveBeenCalledTimes(2)
    } finally {
      Reflect.deleteProperty(io.provider, 'listCaseProjects')
      vi.useRealTimers()
    }
  })

  it.each([false, true])('keeps normal idle housekeeping prewarm bounded by its receipt context (reselected=%s)', async reselected => {
    vi.useFakeTimers()
    // Named threads avoid the separate fallback-title visibility lookup.
    const warm = { ...thread('warm-r07'), title: 'Synthetic warm thread', status: 'idle' }
    invalidateThreadDetailCache(warm.id)
    io.provider.listThreads.mockResolvedValue([{ ...thread('a'), title: 'Named A' }, { ...thread('b'), title: 'Named B' }, warm])
    io.provider.getThreadDetail.mockImplementation(async (id: string) => runningDetail(id))
    io.provider.sendUserMessage.mockRejectedValueOnce(new Error('Synthetic current send failure'))
    try {
      await send()
      expect(useChatStore.getState().busy).toBe(false)
      expect(draft().input).toBe('Discuss selected objects')
      expect(io.provider.getThreadDetail).not.toHaveBeenCalled()
      if (reselected) {
        await select('b')
        await select('a')
      }
      await act(async () => { await vi.advanceTimersByTimeAsync(750) })
      const warmCalls = io.provider.getThreadDetail.mock.calls.filter(([id]) => id === warm.id)
      expect(warmCalls).toHaveLength(reselected ? 0 : 1)
      await act(async () => { await vi.advanceTimersByTimeAsync(750) })
      expect(io.provider.getThreadDetail.mock.calls.filter(([id]) => id === warm.id)).toHaveLength(reselected ? 0 : 1)
    } finally {
      invalidateThreadDetailCache(warm.id)
      vi.useRealTimers()
    }
  })
})

describe('Workbench Browser selected-text consumer path', () => {
  async function attachBrowser() {
    await act(async () => {
      useNativeReferenceStore.setState({ references: [] })
      const scope = { scopeId: 'a'.repeat(48), documentId: 'b'.repeat(48), selectionId: 'c'.repeat(48), threadId: 'a', workspace }
      useNativeReferenceStore.getState().add({ kind: 'browser-selection', threadId: 'a', workspace,
        objectId: scope.documentId, revision: scope.selectionId, scopeId: scope.scopeId, browserScope: scope,
        label: 'PRIVATE-BROWSER-TITLE', text: '', path: '', editable: false })
    })
    io.browser.mockResolvedValue({ ok: true })
  }
  it.each([false, true])('explicit Browser action preserves composer assets (revoked=%s)', async revoked => {
    await attachBrowser(); await addAssets()
    await act(async () => useWriteWorkspaceStore.setState({ workspaceRoot: workspace,
      activeFilePath: workspace + '/synthetic.md', saveStatus: 'saved' }))
    const before = structuredClone(draft())
    const refs = structuredClone(useNativeReferenceStore.getState().references)
    const settings = deferred<AppSettingsV1>(); io.settings.mockReturnValueOnce(settings.promise)
    await act(async () => io.document!.onSubmitPrompt('Explain the selected browser text', refs))
    await settle()
    expect(io.browser).toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    if (revoked) io.browser.mockResolvedValue({ ok: false })
    settings.resolve({ workspaceRoot: workspace } as AppSettingsV1); await settle()
    expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(revoked ? 0 : 1)
    expect(draft()).toEqual(before)
    expect(io.read).not.toHaveBeenCalled()
    if (!revoked) {
      expect(io.provider.sendUserMessage.mock.calls[0][0]).toBe('a')
      expect(io.provider.sendUserMessage.mock.calls[0][1]).toContain('Core scopeId: ' + 'a'.repeat(48))
      expect(io.provider.sendUserMessage.mock.calls[0][1]).not.toContain('PRIVATE-BROWSER-TITLE')
    }
  })
  it('quotes without sending and explicitly sends an opaque reference in the same conversation', async () => {
    await attachBrowser()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(io.provider.sendUserMessage.mock.calls[0][0]).toBe('a')
    expect(io.provider.sendUserMessage.mock.calls[0][1]).toContain('Core scopeId: ' + 'a'.repeat(48))
    expect(io.provider.sendUserMessage.mock.calls[0][1]).not.toContain('PRIVATE-BROWSER-TITLE')
    expect(io.provider.sendUserMessage.mock.calls[0][1]).not.toContain(workspace)
    expect(draft().input).toBe('')
  })
  it('rejects revoked Browser scope after settings I/O and preserves the draft', async () => {
    await attachBrowser()
    const settings = deferred<AppSettingsV1>(); io.settings.mockReturnValueOnce(settings.promise)
    await send(); expect(io.browser).toHaveBeenCalled(); expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    io.browser.mockResolvedValue({ ok: false }); settings.resolve({ workspaceRoot: workspace } as AppSettingsV1)
    await settle(); expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects'); expect(useNativeReferenceStore.getState().references).toHaveLength(1)
  })
})
