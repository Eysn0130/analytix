// @vitest-environment jsdom
import { act, createElement, Profiler, useContext, useEffect, useRef, useState, type ComponentProps, type ContextType } from 'react'
import { File as NodeFile } from 'node:buffer'
import { createRoot, type Root } from 'react-dom/client'
import { createPortal } from 'react-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WriteRetrievalResult } from '@shared/write-retrieval'
import { defaultAnalytixRuntimeSettings, type AppSettingsV1 } from '@shared/app-settings'
import type { NormalizedThread, ThreadEventSink } from '../agent/types'
import type { InputBar } from './chat/InputBar'
import type { CoreThreadSummaryResponseJson, CoreThreadSummarySubagentJson } from '../agent/analytix-contract'
import { SkillChildInspectionContext, type SkillChildInspectionRequest } from './chat/presentation/chat/toolviews/skill-child-inspection'
import { invalidateSharedThreadSummary } from './summary/thread-summary-resource'
import { markPublicProjectionRevoked, resetPublicProjectionRevocationsForTests } from '../lib/public-projection-revocation'

// Only presentation leaves and external IPC/Provider I/O are replaced. The
// mounted Workbench, plan controller, composer draft hook and stores are real.
const io = vi.hoisted(() => ({
  realComposer: false,
  realWorkspaceChrome: false,
  realDocument: false,
  realFileTree: false,
  realPreview: false,
  wordMounts: 0, wordUnmounts: 0, docxEditor: vi.fn(),
  composer: null as ComponentProps<typeof InputBar> | null,
  summaryMounted: vi.fn(),
  summaryUnmounted: vi.fn(),
  skillInspect: null as ContextType<typeof SkillChildInspectionContext>,
  inspector: null as ComponentProps<typeof import('./summary/SubagentInspectorPanel').SubagentInspectorPanel> | null,
  sddNext: null as (() => void) | null,
  document: null as { onSubmitPrompt: (value: string, references?: import('../office/native-reference-store').NativeReference[]) => void; onPrepareDraft: (value:string,references:import('../office/native-reference-store').NativeReference[]) => boolean } | null,
  provider: { connect: vi.fn(), createThread: vi.fn(), sendUserMessage: vi.fn(), subscribeThreadEvents: vi.fn(), listThreads: vi.fn(), getThreadDetail: vi.fn(), getThreadSummary: vi.fn(), uploadAttachment: vi.fn(), getRuntimeInfo: vi.fn(), renameThread: vi.fn() },
  packageHost: vi.fn(), writeFile: vi.fn(), browser: vi.fn(), retrieve: vi.fn(), settings: vi.fn(), canvas: vi.fn(), objects: vi.fn(), office: vi.fn(), read: vi.fn(), directory: vi.fn(), checkpoint: vi.fn(),
  t: (key: string) => key
}))
vi.mock('../agent/registry', () => ({ getProvider: () => io.provider }))
vi.mock('react-i18next', async (importOriginal) => ({
  ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: io.t, i18n: { language: 'en' } })
}))
vi.mock('./chat/InputBar', async importOriginal => {
  const actual = await importOriginal<typeof import('./chat/InputBar')>()
  return ({
  InputBar: (props: ComponentProps<typeof InputBar>) => {
    io.composer = props
    return io.realComposer ? createElement(actual.InputBar, props) : null
  }
})
})
// PDF rendering is a presentation leaf with its own browser-backed suite.
vi.mock('./write/WritePdfViewer', () => ({ WritePdfViewer: () => null }))
vi.mock('./PluginMarketplaceView',()=>({PluginMarketplaceView:()=>createElement('div',{'data-synthetic-plugins':''},'Plugins')}))
vi.mock('@superdoc/react',()=>({SuperDocEditor:(props:{document:File;onReady:(value:unknown)=>void;onEditorUpdate:()=>void})=>{
  const [body,setBody]=useState(''),latest=useRef(props);latest.current=props
  const source=useRef(props.document)
  useEffect(()=>{
    let live=true;io.wordMounts++
    void source.current.text().then(text=>{
      if(!live)return
      setBody(text)
      latest.current.onReady({superdoc:{ui:{selection:{observe:()=>()=>{}},viewport:{observe:()=>()=>{},getRect:()=>({found:false,rects:[]})}}}})
    })
    return()=>{live=false;io.wordUnmounts++}
  },[])
  return createElement('div',{'data-synthetic-word-body':'',contentEditable:true,suppressContentEditableWarning:true,
    onInput:(event:{currentTarget:HTMLElement})=>{setBody(event.currentTarget.textContent??'');latest.current.onEditorUpdate()}},body)
}}))
vi.mock('./schedule/ScheduleTasksView', () => ({ ScheduleTasksView: () => null }))
vi.mock('./chat/Sidebar', () => ({ Sidebar: () => null }))
vi.mock('./chat/WorkbenchTopBar', async importOriginal => {
  const actual = await importOriginal<typeof import('./chat/WorkbenchTopBar')>()
  return { WorkbenchTopBar: (props: ComponentProps<typeof actual.WorkbenchTopBar>) => io.realWorkspaceChrome
    ? createElement(actual.WorkbenchTopBar, props)
    : props.summaryAvailable ? createElement('button', { id: 'workbench-summary-toggle', onClick: props.onToggleSummary }, 'Summary') : null }
})
vi.mock('./chat/AssistantMarkdown', () => ({ preloadAssistantMarkdownRenderer: () => null }))
vi.mock('./chat/ChatFileTreePanel', async importOriginal => { const actual=await importOriginal<typeof import('./chat/ChatFileTreePanel')>();return {ChatFileTreePanel:(props:ComponentProps<typeof actual.ChatFileTreePanel>)=>io.realFileTree?createElement(actual.ChatFileTreePanel,props):null} })
vi.mock('./chat/SideConversationPanel', () => ({ SideConversationPanel: () => null }))
vi.mock('./SessionHeader', () => ({
  SessionHeader: () => null,
  SessionHeaderTabsSeat: ({ seatRef }: ComponentProps<typeof import('./SessionHeader').SessionHeaderTabsSeat>) =>
    createElement('div', { ref: seatRef, 'data-analytix-session-header-tabs': '' })
}))
vi.mock('./shell/ShellNavigationControls', () => ({ ShellNavigationControls: () => null }))
vi.mock('./DevPreviewLaunchCard', () => ({ DevPreviewLaunchCard: () => null }))
vi.mock('./RuntimeBanner', () => ({ RuntimeBanner: () => null }))
vi.mock('./brand/AnalytixLoadingPage', () => ({ AnalytixLoadingPage: () => null }))
vi.mock('./workbench/RightPanelIslands', async () => {
  const { WorkspaceFilePreviewPanel: ActualPreview } = await import('./WorkspaceFilePreviewPanel')
  const { DocumentWorkspacePanel: ActualDocument } = await import('./workbench/DocumentWorkspacePanel')
  const { WorkspaceViewerHeader } = await import('./workbench/WorkspaceViewerHeader')
  return ({ TerminalWorkspacePanel: (props: { visible: boolean; focusRequest: number; onCollapse: () => void }) => createElement('div', { id: 'workbench-workspace-terminal' },
    createElement('div', { className: 'xterm' }, createElement('textarea', { className: 'xterm-helper-textarea', 'data-synthetic-terminal': '', 'data-focus-request': props.focusRequest, hidden: !props.visible })),
    props.visible ? createPortal(createElement('button', { 'data-workbench-terminal-menu': '', onClick: props.onCollapse }, 'Close all terminals'), document.body) : null), ChangeInspectorIsland: () => null, DevBrowserPanelIsland: () => null, PlanPanel: () => null, SddAssistantPanelIsland: () => null,
    SubagentInspectorPanelIsland: (props: NonNullable<typeof io.inspector>) => {
      io.inspector = props
      useEffect(() => () => { if (io.inspector === props) io.inspector = null }, [props])
      return createElement('div', { 'data-synthetic-child-inspector': props.selectedKey })
    }, ThreadSummaryPanelIsland: (props: { titleId?: string; onCollapse: () => void }) => {
      useEffect(() => { io.summaryMounted(); return () => { io.summaryUnmounted() } }, [])
      return createElement('div', null, createElement('h2', { id: props.titleId }, 'Summary'), createElement('button', { 'data-synthetic-summary-close': '', onClick: props.onCollapse }, 'Close'))
    }, TodoPanel: () => null, WorkspaceFilePreviewPanel: (props: ComponentProps<typeof ActualPreview>) => io.realPreview ? createElement(ActualPreview, props) : null,
    DocumentWorkspacePanel: (props: ComponentProps<typeof ActualDocument> & NonNullable<typeof io.document>) => {
      io.document = props
      if (io.realDocument) return createElement(ActualDocument, props)
      return io.realWorkspaceChrome && props.visible && props.viewerControls
        ? createElement(WorkspaceViewerHeader, { title: props.activeTab?.title ?? 'Document', viewerControls: props.viewerControls })
        : null
    }, preloadRightPanelIsland: () => null }) })

vi.mock('./sdd/SddDraftEditorView', () => ({ SddDraftEditorView: (props: { onNext: () => void }) => { io.sddNext = props.onNext; return null } }))
vi.mock('./workbench/ChatTimelineIsland', async () => {
  const { SkillChildInspectionContext } = await import('./chat/presentation/chat/toolviews/skill-child-inspection')
  return { ChatTimelineIsland: () => { io.skillInspect = useContext(SkillChildInspectionContext); return null }, useDevPreviewUrls: () => [] }
})
import { Workbench } from './Workbench'
import { WorkspaceFilePreviewPanel } from './WorkspaceFilePreviewPanel'
import * as codeHighlighting from '../lib/code-highlighting'
import { ImagePreviewLightbox } from './chat/ImagePreviewLightbox'
import { invalidateThreadDetailCache } from '../lib/thread-detail-cache'
import { useChatStore } from '../store/chat-store'
import { useNativeReferenceStore, type CanvasNativeReference, type OfficeNativeReference, type ImageRegionNativeReference, type BrowserNativeReference, type NativeReference } from '../office/native-reference-store'
import { useDocxEditorStore } from '../office/docx-editor-store'
import type {DocxEditorView} from '@shared/docx-editor'
import { useNativeOfficeStore } from '../office/native-office-store'
import type { NativeOfficeView } from '@shared/native-office'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../store/workspace-tabs-store'
import { useGuiPlanStore } from '../plan/plan-store'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { composerDraftKey } from '../store/composer-drafts'
import { createSddDraft, useSddDraftStore } from '../sdd/sdd-draft-store'
import { markSddAssistantThread } from '../sdd/sdd-thread-registry'
import { createOrdinaryHistoryReplay, isOrdinaryDisplayHistoryBlock } from '../agent/analytix-mapper'
import { clearBusyWatchdog, stopTurnCompletionPoll } from '../store/chat-store-schedulers'
import * as schedulers from '../store/chat-store-schedulers'
import * as runtime from '../store/chat-store-runtime'
import { appendActiveStreamDeltas, clearActiveStream, getActiveStreamSnapshotFor } from '../thread/streaming/active-stream-store'

const readyDocuments = () => ({packageId:'analytix-documents' as const,packageVersion:'1.0.0',displayName:'Documents',
  origin:'development-source' as const,publishable:false as const,materialized:true,generationId:'a'.repeat(64),
  activationState:'recorded' as const,desiredState:'enabled' as const,activationRevision:1,activationId:'b'.repeat(64),available:true,
  operations:['create-object','open-object','close-object']})

const workspace = '/synthetic/canvas-send'
const thread = (id: string): NormalizedThread => ({ id, title: id,
  updatedAt: '2026-09-17T00:00:00.000Z', model: 'synthetic-model', mode: 'agent', workspace, status: 'running' })
const reference = (): Omit<CanvasNativeReference, 'id'> => ({ kind: 'canvas', threadId: 'a', workspace,
  path: workspace + '/private.canvas', objectId: 'a'.repeat(64), revision: 'b'.repeat(64),
  label: 'PRIVATE-LABEL', text: '', sessionId: 'c'.repeat(48), scopeId: 'd'.repeat(48),
  editable: true, selectedIds: ['PRIVATE-NODE'] })
const selectionResponse = () => ({ ok: true, selection: { sessionId: 'c'.repeat(48), threadId: 'a',
  scopeId: 'd'.repeat(48), baseRevision: 'b'.repeat(64), selectedIds: ['PRIVATE-NODE'], editable: true } })
const officeReference = (): OfficeNativeReference => ({kind:'office',id:'local-office',threadId:'a',workspace,path:workspace+'/PRIVATE-OFFICE.docx',objectId:'1'.repeat(64),revision:'2'.repeat(64),scopeId:'a'.repeat(48),editable:false,label:'PRIVATE-OFFICE-LABEL',text:'PRIVATE OFFICE QUOTE',note:'PRIVATE OFFICE NOTE',selection:{kind:'text',documentId:'1'.repeat(64),version:'2'.repeat(64),changeSequence:0,token:'office-selection-token',text:'PRIVATE OFFICE QUOTE',scope:'session-text-range-at-version-and-change-sequence'}})
const officeView = (reference = officeReference()): NativeOfficeView => ({objectId:reference.objectId,path:reference.path,revision:reference.revision,kind:'docx',status:'ready',dirty:false,editing:false,changeSequence:0,selection:reference.selection,scope:{scopeId:reference.scopeId!,threadId:reference.threadId!,selectionToken:reference.selection.token!,baseRevision:reference.revision,changeSequence:reference.selection.changeSequence,editable:false,parts:[]}})
const editableOfficeReference = (): OfficeNativeReference => {
  const reference = officeReference()
  return {...reference,editable:true,selection:{...reference.selection,capture:{capturedCharacters:reference.text.length,totalCharacters:reference.text.length,truncated:false,unit:'utf-16',complete:true}}}
}
const editableOfficeView = (reference = editableOfficeReference()): NativeOfficeView => {
  const view = officeView(reference)
  return {...view,editing:true,scope:{...view.scope!,editable:true}}
}
async function mountDocumentActionOwner() {
  const view: NativeOfficeView = {objectId:'9'.repeat(64),path:workspace+'/document-actions.docx',
    revision:'8'.repeat(64),kind:'docx',status:'ready',dirty:false,editing:false,changeSequence:0}
  const id = workspaceObjectTabId(workspace,view.path)
  const before = structuredClone(draft())
  const references = structuredClone(useNativeReferenceStore.getState().references)
  io.office.mockResolvedValue({ok:true,view})
  await act(async () => {
    useNativeOfficeStore.setState({target:{workspace,path:view.path},view,views:{}})
    useWorkspaceTabsStore.getState().openTab({id,kind:'document',mode:'documents',title:'Document actions',workspaceRoot:workspace,path:view.path})
  })
  await settle()
  expect(useWorkspaceTabsStore.getState().activeTabId).toBe(id)
  expect(io.document).not.toBeNull()
  expect(draft()).toEqual(before)
  expect(useNativeReferenceStore.getState().references).toEqual(references)
}
async function mountOfficeDraft(view = editableOfficeView()) {
  await act(async () => {
    useNativeReferenceStore.setState({references:[]})
    useNativeOfficeStore.setState({target:{workspace,path:view.path},view,views:{}})
    useWriteWorkspaceStore.setState({workspaceRoot:workspace,activeFilePath:view.path,saveStatus:'saved'})
    useWorkspaceTabsStore.getState().openTab({id:workspaceObjectTabId(workspace,view.path),kind:'document',mode:'documents',title:'Current Office',workspaceRoot:workspace,path:view.path})
    composer().setInput('')
  })
  io.office.mockResolvedValue({ok:true,view})
  await settle()
  expect(io.document).not.toBeNull()
}
async function attachOffice() {
  await act(async () => {useNativeReferenceStore.setState({references:[]});useNativeOfficeStore.setState({target:{workspace,path:officeReference().path},view:officeView(),views:{}});useNativeReferenceStore.getState().add(officeReference())})
  io.office.mockResolvedValue({ok:true,view:officeView()})
}
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
import { WORKSPACE_FILE_PREVIEW_EVENT } from '../lib/workspace-file-preview'
const initialChat = useChatStore.getInitialState()
const initialWrite = useWriteWorkspaceStore.getInitialState()
const initialPlan = useGuiPlanStore.getInitialState()
const initialSdd = useSddDraftStore.getInitialState()
const scrollIntoViewDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')

beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  vi.stubGlobal('__ANALYTIX_DOCX_WORKER_PATHS__',{document:'superdoc-engine/assets/document-fixture.js',collaboration:'superdoc-engine/assets/collaboration-fixture.js',reviewIndex:'superdoc-engine/assets/review-fixture.js'})
  // jsdom has no native ResizeObserver; geometry behavior has its own focused suite.
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.resetAllMocks()
  io.provider.createThread = vi.fn()
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
  streams = []
  clearActiveStream()
  io.realComposer = false
  io.realWorkspaceChrome = false
  io.realDocument = false
  io.realFileTree = false
  io.realPreview = false
  io.wordMounts=0;io.wordUnmounts=0
  useDocxEditorStore.setState(useDocxEditorStore.getInitialState(),true)
  io.document = null
  io.sddNext = null
  io.skillInspect = null; io.inspector = null
  invalidateSharedThreadSummary(); resetPublicProjectionRevocationsForTests()
  useWorkspaceTabsStore.setState(useWorkspaceTabsStore.getInitialState(), true)
  localStorage.clear()
  useChatStore.setState({ ...initialChat, threads: [thread('a'), thread('b')], activeThreadId: 'a',
    workspaceRoot: workspace, runtimeConnection: 'ready', composerModel: 'synthetic-model' }, true)
  useWriteWorkspaceStore.setState(initialWrite, true)
  useGuiPlanStore.setState(initialPlan, true)
  useSddDraftStore.setState(initialSdd, true)
  useNativeReferenceStore.setState({ references: [], drafts: {} })
  useNativeOfficeStore.setState(useNativeOfficeStore.getInitialState(),true)
  io.settings.mockResolvedValue({ workspaceRoot: workspace, runtime: defaultAnalytixRuntimeSettings() } as AppSettingsV1)
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockImplementation(() => io.settings())
  io.provider.getRuntimeInfo.mockResolvedValue({ capabilities: {
    attachments: { available: true }, model: { id: 'synthetic-model', inputModalities: ['text'], contextWindowTokens: 128000 },
    mcp: { toolCount: 0 }, skills: { discoveredSkills: 0 }, web: { fetch: { available: false }, search: { available: false } }
  } })
  io.provider.listThreads.mockResolvedValue([thread('a'), thread('b')])
  io.provider.renameThread.mockResolvedValue(thread('a'))
  io.writeFile.mockResolvedValue({ ok: true })
  io.provider.sendUserMessage.mockResolvedValue({ turnId: 'turn-a', userMessageItemId: 'message-a' })
  io.provider.subscribeThreadEvents.mockImplementation(() => new Promise<void>(resolve => streams.push(resolve)))
  io.canvas.mockImplementation(async () => selectionResponse())
  io.read.mockResolvedValue({ ok: true, content: 'SYNTHETIC-PUBLIC-FILE' })
  io.directory.mockResolvedValue({ ok: true, entries: [] })
  io.packageHost.mockResolvedValue({ok:true,packages:[readyDocuments()]})
  io.checkpoint.mockResolvedValue({ ok: false, reason: 'not_git_repo', message: 'synthetic fixture' })
  Object.assign(window, { analytix: {
    workspace: { onThreadHandoffEvent: () => () => {}, getThreadHandoffOperations: async () => ({ ok: true, operations: [] }),
      createGitCheckpoint: io.checkpoint },
    write: { retrieveWriteContext: io.retrieve },
    files: { read: io.read, write: io.writeFile, listDirectory: io.directory, getPathForFile: () => workspace + '/synthetic.pdf',
      readLocalPdfText: async () => ({ ok: true, path: workspace + '/synthetic.pdf', text: 'Synthetic attachment text', pageCount: 1 }) }, canvas: { request: io.canvas }, objects: { request: io.objects }, browserSelection: { request: io.browser }, office:{request:io.office,onChange:()=>()=>{}},
    packageHost:{request:io.packageHost},
    docxEditor:{request:io.docxEditor,onCapture:()=>()=>{}},
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
    useSddDraftStore.setState(initialSdd, true)
    streams.forEach(resolve => resolve())
  })
  clearBusyWatchdog()
  stopTurnCompletionPoll()
  clearActiveStream()
  invalidateSharedThreadSummary(); resetPublicProjectionRevocationsForTests()
  element.remove()
  if (scrollIntoViewDescriptor) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', scrollIntoViewDescriptor)
  else delete (HTMLElement.prototype as { scrollIntoView?: unknown }).scrollIntoView
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const skillRequest = (): SkillChildInspectionRequest => ({ parentThreadId: 'a', parentTurnId: 'turn-a',
  parentToolCallId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
  child: { jobId: 'job-child', childThreadId: 'child-skill', childTurnId: 'turn-child', status: 'completed',
    background: false, outputWithheld: true } })
const skillAgent = (): CoreThreadSummarySubagentJson => ({ schemaVersion: 1, id: 'run:job-child', key: 'run:job-child',
  parentThreadId: 'a', parentTurnId: 'turn-a', parentToolCallId: skillRequest().parentToolCallId, childRunId: 'job-child',
  childId: 'job-child', taskJobId: 'job-child', taskKind: 'subagent', childThreadId: 'child-skill', childTurnId: 'turn-child',
  status: 'active', rawStatus: 'running', outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
  factAnswerAllowed: false, evidenceAuthority: false, canContinueParent: false, canReadOutput: false,
  canOpenThread: true, canKill: false, canRestart: false, updatedAt: '2026-10-10T00:00:00Z' })
const skillSummary = (subagents = [skillAgent()]): CoreThreadSummaryResponseJson => ({ threadId: 'a',
  generatedAt: '2026-10-10T00:00:00Z', latestSeq: 1, subagents, tasks: [], outputs: [], sources: [], sideChats: [], backgroundProcesses: [] })

describe('Workbench protected Skill child entry', () => {
  it('opens only the summary-admitted real child in the existing Inspector and clears on source retirement', async () => {
    const actual = skillAgent(), other = { ...actual, id: 'run:other', key: 'run:other', childRunId: 'other', taskJobId: 'other', childId: 'other', childThreadId: 'other-thread' }
    io.provider.getThreadSummary.mockResolvedValue(skillSummary([other, actual]))
    const controller = new AbortController()
    let opened = false
    await act(async () => { opened = await io.skillInspect!(skillRequest(), controller.signal) })
    expect(opened).toBe(true); expect(io.provider.getThreadSummary).toHaveBeenCalledExactlyOnceWith('a')
    expect(io.inspector?.selectedKey).toBe(actual.key); expect(io.inspector?.subagents).toEqual([actual])
    expect(useChatStore.getState().activeThreadId).toBe('a')
    expect(io.provider.getThreadDetail).not.toHaveBeenCalled()
    await act(async () => controller.abort())
    expect(io.inspector).toBeNull(); expect(element.querySelector('[data-synthetic-child-inspector]')).toBeNull()
  })
  it.each(['absent', 'unknown'] as const)('accepts the actual event-merged identity with %s taskKind and nullable captured childTurnId', async kind => {
    const actual = skillAgent(); delete actual.taskJobId
    if (kind === 'absent') delete actual.taskKind; else actual.taskKind = 'unknown'
    const request = skillRequest(); request.child.childTurnId = null
    io.provider.getThreadSummary.mockResolvedValue(skillSummary([actual]))
    await act(async () => { expect(await io.skillInspect!(request, new AbortController().signal)).toBe(true) })
    expect(io.inspector?.selectedKey).toBe(actual.key)
  })
  it.each(['missing', 'parent', 'call', 'job', 'child', 'turn', 'capability', 'ambiguous', 'duplicate-key'] as const)(
    'rejects %s relation without Inspector first-row fallback', async variant => {
      const actual = skillAgent()
      if (variant === 'parent') actual.parentThreadId = 'other-parent'
      if (variant === 'call') actual.parentToolCallId = 'foreign-call'
      if (variant === 'job') actual.childRunId = 'foreign-job'
      if (variant === 'child') actual.childThreadId = 'foreign-child'
      if (variant === 'turn') actual.childTurnId = 'foreign-turn'
      if (variant === 'capability') actual.canOpenThread = false
      const agents = variant === 'missing' ? [] : variant === 'ambiguous' ? [actual, { ...actual, id: 'duplicate', key: 'duplicate' }] :
        variant === 'duplicate-key' ? [{ ...actual, childThreadId: 'wrong-first-child' }, actual] : [actual]
      io.provider.getThreadSummary.mockResolvedValue(skillSummary(agents))
      await act(async () => { expect(await io.skillInspect!(skillRequest(), new AbortController().signal)).toBe(false) })
      expect(io.inspector).toBeNull(); expect(element.querySelector('[data-synthetic-child-inspector]')).toBeNull()
    })
  it.each(['abort', 'parent-switch', 'revoked'] as const)('rejects a late summary after %s', async variant => {
    let finish!: (value: CoreThreadSummaryResponseJson) => void
    io.provider.getThreadSummary.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const controller = new AbortController()
    let pending!: Promise<boolean>
    await act(async () => { pending = io.skillInspect!(skillRequest(), controller.signal) })
    await act(async () => {
      if (variant === 'abort') controller.abort()
      if (variant === 'parent-switch') useChatStore.setState({ activeThreadId: 'b' })
      if (variant === 'revoked') markPublicProjectionRevoked('child-skill')
      finish(skillSummary())
      expect(await pending).toBe(false)
    })
    expect(io.inspector).toBeNull()
  })
  it('starts a fresh summary read after parent A to B to A rather than reusing the old in-flight request', async () => {
    let first!: (value: CoreThreadSummaryResponseJson) => void, second!: (value: CoreThreadSummaryResponseJson) => void
    io.provider.getThreadSummary.mockImplementationOnce(() => new Promise(resolve => { first = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { second = resolve }))
    let old!: Promise<boolean>, current!: Promise<boolean>
    await act(async () => { old = io.skillInspect!(skillRequest(), new AbortController().signal) })
    await act(async () => useChatStore.setState({ activeThreadId: 'b' }))
    await act(async () => useChatStore.setState({ activeThreadId: 'a' }))
    await act(async () => { current = io.skillInspect!(skillRequest(), new AbortController().signal) })
    expect(io.provider.getThreadSummary).toHaveBeenCalledTimes(2)
    await act(async () => { first(skillSummary()); expect(await old).toBe(false) })
    expect(io.inspector).toBeNull()
    await act(async () => { second(skillSummary()); expect(await current).toBe(true) })
    expect(io.inspector?.selectedKey).toBe(skillAgent().key)
  })
})

describe('Workbench Canvas consumer path', () => {
  it.each([false, true])('uses primary SDD context while retaining display history (selection changes: %s)', async selectionChanges => {
    const sdd = createSddDraft({ id: '12345678-1234-1234-1234-123456789abc', workspaceRoot: workspace })
    const timestamp = '2026-10-08T00:00:00Z'
    const replay = createOrdinaryHistoryReplay('a', 4)
    replay.accept([{ kind: 'item_created', seq: 1, timestamp, threadId: 'a', turnId: 'compressed-turn',
      itemId: 'compressed-user', item: { id: 'compressed-user', threadId: 'a', turnId: 'compressed-turn',
        kind: 'user_message', role: 'user', status: 'completed', createdAt: timestamp, text: 'COMPRESSED-DISPLAY-ONLY' } },
      { kind: 'compaction_completed', seq: 2, timestamp, threadId: 'a', turnId: 'compact-turn', summary: 'Context compacted' },
      { kind: 'cursor_advanced', seq: 4, timestamp, threadId: 'a' }])
    const primary = { thread: thread('a'), latestSeq: 4, threadStatus: 'completed', blocks: [
      { kind: 'user' as const, id: 'primary-user', text: 'PRIMARY-CONTEXT', meta: { turnId: 'primary-turn' } },
      { kind: 'assistant' as const, id: 'primary-assistant', text: 'PRIMARY-ANSWER', meta: { turnId: 'primary-turn' } },
      { kind: 'compaction' as const, id: 'compact-item', status: 'success' as const, summary: 'Context compacted', meta: { turnId: 'compact-turn' } }
    ] }
    const displayed = replay.mergeSnapshot(primary.blocks)
    const pending = deferred<typeof primary>()
    io.provider.getThreadDetail.mockReturnValueOnce(pending.promise)
    await act(async () => {
      markSddAssistantThread(sdd, 'a')
      useNativeReferenceStore.setState({ references: [], drafts: {} })
      useChatStore.setState({ blocks: displayed, lastSeq: 4, busy: false })
      useSddDraftStore.getState().setActiveDraft(sdd, '# Synthetic requirement')
    })
    await settle()
    expect(io.sddNext).not.toBeNull()
    await act(async () => io.sddNext!())
    await settle()
    expect(io.provider.getThreadDetail).toHaveBeenCalledWith('a')
    if (selectionChanges) await act(async () => useChatStore.setState({ activeThreadId: 'b' }))
    pending.resolve(primary)
    await settle()
    expect(useChatStore.getState().blocks.some(block => isOrdinaryDisplayHistoryBlock(block) && block.id === 'compressed-user')).toBe(true)
    if (selectionChanges) {
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
      expect(useSddDraftStore.getState().operationStatus).toBe('error')
    } else {
      expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
      const prompt = io.provider.sendUserMessage.mock.calls[0][1]
      expect(prompt).toContain('PRIMARY-CONTEXT'); expect(prompt).toContain('PRIMARY-ANSWER')
      expect(prompt).not.toContain('COMPRESSED-DISPLAY-ONLY')
    }
  })

  it('keeps modal Tab focus in the actual lightbox without consuming the plan shortcut', async () => {
    const modalHost = document.createElement('div'); document.body.append(modalHost)
    const modalRoot = createRoot(modalHost), close = vi.fn()
    let modalRootMounted = true
    const beforeTabs = useWorkspaceTabsStore.getState()
    const tabs = { open: beforeTabs.open, activeTabId: beforeTabs.activeTabId, ids: beforeTabs.tabs.map(tab => tab.id) }
    const key = async (target: Element, shiftKey = false, isComposing = false) => {
      const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, isComposing, bubbles: true, cancelable: true })
      await act(async () => target.dispatchEvent(event))
      return event
    }
    try {
      await act(async () => modalRoot.render(createElement(ImagePreviewLightbox, { open: true,
        src: 'data:image/png;base64,synthetic', alt: 'Synthetic modal', downloadHref: 'data:image/png;base64,synthetic', onClose: close })))
      const dialog = document.querySelector<HTMLElement>('[role="dialog"][aria-modal="true"]')!
      const download = dialog.querySelector<HTMLAnchorElement>('a[download]')!
      const buttons = [...dialog.querySelectorAll<HTMLButtonElement>('button')]
      const last = buttons.at(-1)!, middle = buttons[1]!, previous = buttons[0]!
      expect(composer().mode).toBe('agent')
      download.focus(); await key(download, true)
      expect(document.activeElement).toBe(last)
      expect(composer().mode).toBe('agent')
      last.focus(); await key(last)
      expect(document.activeElement).toBe(download)
      // jsdom has no native sequential Tab navigation; the middle event must reach the browser unconsumed.
      middle.focus(); const middleTab = await key(middle, true)
      expect(middleTab.defaultPrevented).toBe(false)
      expect(document.activeElement).toBe(middle)
      expect(previous).not.toBe(middle)
      const composing = await key(download, true, true)
      expect(composing.defaultPrevented).toBe(false)
      expect(composer().mode).toBe('agent')
      const afterTabs = useWorkspaceTabsStore.getState()
      expect({ open: afterTabs.open, activeTabId: afterTabs.activeTabId, ids: afterTabs.tabs.map(tab => tab.id) }).toEqual(tabs)
      await act(async () => dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', isComposing: true, bubbles: true, cancelable: true })))
      expect(close).not.toHaveBeenCalled()
      await act(async () => dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
      expect(close).toHaveBeenCalledOnce()
      await act(async () => modalRoot.unmount())
      modalRootMounted = false
      await act(async () => composer().setMode('plan'))
      const executionMenu = document.createElement('div'); executionMenu.dataset.composerExecutionMenu = ''
      const executionButton = document.createElement('button'); executionMenu.append(executionButton); document.body.append(executionMenu)
      try {
        const executionTab = await key(executionButton, true)
        expect(executionTab.defaultPrevented).toBe(false)
        expect(composer().mode).toBe('plan')
      } finally { executionMenu.remove() }
      const outside = document.createElement('button'); document.body.append(outside)
      try {
        const ime = await key(outside, true, true)
        expect(ime.defaultPrevented).toBe(false); expect(composer().mode).toBe('plan')
        const ordinary = await key(outside, true)
        expect(ordinary.defaultPrevented).toBe(true); expect(composer().mode).toBe('agent')
      } finally { outside.remove() }
    } finally { if (modalRootMounted) await act(async () => modalRoot.unmount()); modalHost.remove() }
  })

  it.each([{ key: 'Tab', shiftKey: false }, { key: 'Tab', shiftKey: true }, { key: 'Escape' }, { key: 'Tab', shiftKey: true, isComposing: true }, { key: 'Escape', isComposing: true }, { key: 'Tab', shiftKey: true, keyCode: 229 }])('lets the actual ordinary composer menu own $key through Workbench capture ($shiftKey/$isComposing/$keyCode)', async options => {
    const rangeRects = Object.getOwnPropertyDescriptor(Range.prototype, 'getClientRects')
    const rangeBounds = Object.getOwnPropertyDescriptor(Range.prototype, 'getBoundingClientRect')
    Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
    Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
    const frames = new Map<number, FrameRequestCallback>()
    let nextFrame = 0
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++nextFrame, callback); return nextFrame })
    vi.stubGlobal('cancelAnimationFrame', (id: number) => { frames.delete(id) })
    const flushFrames = async () => {
      for (let pass = 0; pass < 3 && frames.size > 0; pass++) {
        const pending = [...frames.keys()]
        await act(async () => {
          for (const id of pending) {
            const callback = frames.get(id)
            frames.delete(id)
            callback?.(0)
          }
        })
      }
    }
    try {
      io.realComposer = true
      await act(async () => { composer().setMode('plan'); root.render(createElement(Workbench)) })
      await flushFrames()
      const trigger = element.querySelector<HTMLButtonElement>('button[aria-label="composerMenuTitle"]')!
      expect(trigger).not.toBeNull()
      await act(async () => trigger.click())
      await flushFrames()
      const menu = element.querySelector<HTMLElement>('[role="menu"][aria-label="composerMenuTitle"]')!
      expect(menu).not.toBeNull(); expect(menu.contains(document.activeElement)).toBe(true)
      const event = new KeyboardEvent('keydown', { ...options, bubbles: true, cancelable: true })
      await act(async () => document.activeElement!.dispatchEvent(event))
      const composing = options.isComposing || options.keyCode === 229
      expect(event.defaultPrevented).toBe(options.key === 'Escape' && !composing); expect(composer().mode).toBe('plan')
      if (composing) expect(menu.contains(document.activeElement)).toBe(true)
      else {
        expect(element.querySelector('[role="menu"][aria-label="composerMenuTitle"]')).toBeNull()
        // Native sequential navigation to the Editor is verified in the actual
        // Workbench browser fixture; jsdom stops at the exit trigger.
        expect(document.activeElement).toBe(trigger)
      }
    } finally {
      io.realComposer = false
      await act(async () => root.render(createElement(Workbench)))
      frames.clear()
      vi.unstubAllGlobals()
      if (rangeRects) Object.defineProperty(Range.prototype, 'getClientRects', rangeRects); else Reflect.deleteProperty(Range.prototype, 'getClientRects')
      if (rangeBounds) Object.defineProperty(Range.prototype, 'getBoundingClientRect', rangeBounds); else Reflect.deleteProperty(Range.prototype, 'getBoundingClientRect')
    }
  })

  it('keeps the real input editor and current draft through hero, history settling and restored content', async () => {
    const rangeRects = Object.getOwnPropertyDescriptor(Range.prototype, 'getClientRects')
    const rangeBounds = Object.getOwnPropertyDescriptor(Range.prototype, 'getBoundingClientRect')
    Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
    Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
    vi.stubGlobal('requestAnimationFrame', (fn: FrameRequestCallback) => setTimeout(() => fn(performance.now()), 0))
    vi.stubGlobal('cancelAnimationFrame', clearTimeout)
    const restored = completedDetail('a')
    const history = deferred<typeof restored>()
    let navigation: Promise<void> | undefined
    try {
      io.realComposer = true
      await act(async () => root.render(createElement(Workbench)))
      const body = element.querySelector<HTMLElement>('[data-analytix-conversation-body]')!
      const inputOwner = element.querySelector<HTMLElement>('[data-analytix-input-bar]')!
      const editor = inputOwner.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
      expect(editor).not.toBeNull()
      expect(body.dataset.phase).toBe('hero')
      expect(inputOwner.dataset.inputLayout).toBe('hero')
      const original = draft()
      io.provider.getThreadDetail.mockReturnValueOnce(history.promise)
      invalidateThreadDetailCache('a')
      await act(async () => { navigation = useChatStore.getState().selectThread('a') })
      expect(body.dataset.phase).toBe('settling')
      expect(element.querySelector('[data-analytix-input-bar]')).toBe(inputOwner)
      expect(inputOwner.querySelector('.ProseMirror')).toBe(editor)
      expect(draft().input).toBe(original.input)
      expect(draft().inputRevision).toBe(original.inputRevision)

      history.resolve(restored)
      await act(async () => { await navigation })
      await settle()
      expect(body.dataset.phase).toBe('active')
      expect(inputOwner.dataset.inputLayout).toBe('composer')
      expect(element.querySelector('[data-analytix-input-bar]')).toBe(inputOwner)
      expect(inputOwner.querySelector('.ProseMirror')).toBe(editor)
      expect(editor.textContent).toBe(original.input)
      expect(draft().input).toBe(original.input)
      expect(draft().inputRevision).toBe(original.inputRevision)
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    } finally {
      history.resolve(restored)
      await act(async () => { await navigation })
      io.realComposer = false
      await act(async () => root.render(createElement(Workbench)))
      vi.unstubAllGlobals()
      if (rangeRects) Object.defineProperty(Range.prototype, 'getClientRects', rangeRects); else Reflect.deleteProperty(Range.prototype, 'getClientRects')
      if (rangeBounds) Object.defineProperty(Range.prototype, 'getBoundingClientRect', rangeBounds); else Reflect.deleteProperty(Range.prototype, 'getBoundingClientRect')
    }
  })

  it('moves the same resident input on a real first send and preserves newer typing through acknowledgement', async () => {
    const rangeRects = Object.getOwnPropertyDescriptor(Range.prototype, 'getClientRects')
    const rangeBounds = Object.getOwnPropertyDescriptor(Range.prototype, 'getBoundingClientRect')
    Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
    Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
    vi.stubGlobal('requestAnimationFrame', (fn: FrameRequestCallback) => setTimeout(() => fn(performance.now()), 0))
    vi.stubGlobal('cancelAnimationFrame', clearTimeout)
    const fresh = { ...thread('brand-first-send'), status: 'idle' as const,
      executionPolicyVersion: 2 as const, approvalPolicy: 'on-request' as const, sandboxMode: 'workspace-write' as const }
    const receipt = deferred<{ turnId: string; userMessageItemId: string }>()
    Object.assign(io.provider, { createThread: vi.fn().mockResolvedValueOnce(fresh) })
    io.provider.listThreads.mockResolvedValue([fresh, thread('a'), thread('b')])
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    try {
      io.realComposer = true
      await act(async () => {
        useNativeReferenceStore.setState({ references: [], drafts: {} })
        await useChatStore.getState().createThread()
        root.render(createElement(Workbench))
      })
      await act(async () => composer().setInput('First visible submission'))
      const body = element.querySelector<HTMLElement>('[data-analytix-conversation-body]')!
      const inputOwner = element.querySelector<HTMLElement>('[data-analytix-input-bar]')!
      const editor = inputOwner.querySelector<HTMLElement>('.ProseMirror')!
      expect(editor).not.toBeNull()
      expect(body.dataset.phase).toBe('hero')
      await act(async () => {
        editor.focus()
        editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true }))
      })
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
      expect(body.dataset.phase).toBe('hero')
      await act(async () => inputOwner.querySelector<HTMLButtonElement>('.ds-composer-primary-action-button')!.click())
      await settle()
      expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
      expect(io.provider.sendUserMessage.mock.calls[0][0]).toBe(fresh.id)
      expect(body.dataset.phase).toBe('active')
      expect(inputOwner.dataset.inputLayout).toBe('composer')
      expect(element.querySelector('[data-analytix-input-bar]')).toBe(inputOwner)
      expect(inputOwner.querySelector('.ProseMirror')).toBe(editor)
      await act(async () => { composer().setInput('Keep newer typing'); await new Promise(resolve => setTimeout(resolve, 20)) })
      expect(document.activeElement).toBe(editor)
      receipt.resolve({ turnId: 'brand-first-turn', userMessageItemId: 'brand-first-user' })
      await settle()
      expect(composer().input).toBe('Keep newer typing')
      expect(editor.textContent).toBe('Keep newer typing')
      expect(inputOwner.querySelector('.ProseMirror')).toBe(editor)
      expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    } finally {
      receipt.resolve({ turnId: 'brand-first-turn', userMessageItemId: 'brand-first-user' })
      await settle()
      Reflect.deleteProperty(io.provider, 'createThread')
      io.realComposer = false
      await act(async () => root.render(createElement(Workbench)))
      vi.unstubAllGlobals()
      if (rangeRects) Object.defineProperty(Range.prototype, 'getClientRects', rangeRects); else Reflect.deleteProperty(Range.prototype, 'getClientRects')
      if (rangeBounds) Object.defineProperty(Range.prototype, 'getBoundingClientRect', rangeBounds); else Reflect.deleteProperty(Range.prototype, 'getBoundingClientRect')
    }
  })

  it('owns the summary independently of file tabs and the right dock lifecycle', async () => {
    await act(async () => useWorkspaceTabsStore.getState().openTab({
      id: 'synthetic-file', kind: 'file', mode: 'file', title: 'Synthetic file', path: 'synthetic.txt', workspaceRoot: workspace
    }))
    await act(async () => useWorkspaceTabsStore.getState().setFocused(true))
    const before = useWorkspaceTabsStore.getState()
    await act(async () => document.getElementById('workbench-summary-toggle')!.click())
    expect(io.summaryMounted).toHaveBeenCalledTimes(1)
    expect(useWorkspaceTabsStore.getState()).toBe(before)
    const summary = document.querySelector('[data-thread-summary-surface]')
    const tab = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true })
    const beforeMode = composer().mode
    await act(async () => summary!.querySelector('button')!.dispatchEvent(tab))
    expect(tab.defaultPrevented).toBe(false)
    expect(composer().mode).toBe(beforeMode)
    await act(async () => useWorkspaceTabsStore.getState().setOpen(false))
    expect(io.summaryUnmounted).not.toHaveBeenCalled()
    expect(document.querySelector('[data-thread-summary-surface]')).toBe(summary)
    await act(async () => useWorkspaceTabsStore.getState().setOpen(true))
    const reopened = useWorkspaceTabsStore.getState()
    await act(async () => document.querySelector<HTMLButtonElement>('[data-synthetic-summary-close]')!.click())
    expect(io.summaryUnmounted).toHaveBeenCalledTimes(1)
    expect(useWorkspaceTabsStore.getState()).toBe(reopened)
    expect(document.activeElement).toBe(document.getElementById('workbench-summary-toggle'))
  })

  it('holds the actual send owner until its selected attachment upload completes', async () => {
    let complete!: (value: object) => void
    io.provider.uploadAttachment.mockImplementation(() => new Promise(resolve => { complete = resolve }))
    const file = new File(['synthetic'], 'synthetic.pdf', { type: 'application/pdf' })
    Object.assign(file, { arrayBuffer: async () => new TextEncoder().encode('synthetic').buffer })
    await act(async () => {
      useNativeReferenceStore.setState({ references: [] })
      composer().onPickAttachments!([file])
    })
    await settle()
    expect(io.provider.uploadAttachment).toHaveBeenCalledOnce()
    expect(composer().attachmentUploadBusy).toBe(true)
    try {
      await send()
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    } finally {
      await act(async () => complete({ id: 'uploaded-pdf', name: 'synthetic.pdf', mimeType: 'application/pdf', byteSize: 9, pageCount: 1 }))
      await settle()
    }
    expect(composer().attachmentUploadBusy).toBe(false)
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(io.provider.sendUserMessage.mock.calls[0][2]).toMatchObject({ attachmentIds: ['uploaded-pdf'] })
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


  it.each(['revoke', 'thread'])('rejects %s during retained editor-request retrieval', async change => {
    const retrieved = deferred<WriteRetrievalResult>()
    io.retrieve.mockReturnValueOnce(retrieved.promise)
    await mountDocumentActionOwner()
    // This no-reference override covers the retained editor request. Its Write
    // context must belong to the conversation; native Word actions pass references.
    await act(async () => useWriteWorkspaceStore.setState({workspaceRoot:workspace}))
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

  it('sends current Office discussion scopes through the ordinary composer using only opaque context', async () => {
    await attachOffice(); await send()
    expect(io.canvas).not.toHaveBeenCalled()
    expect(io.office).toHaveBeenCalledTimes(2)
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    const message = io.provider.sendUserMessage.mock.calls[0][1]
    expect(message).toContain('Core scopeId: ' + officeReference().scopeId)
    expect(message).toContain('native_selection_read')
    for (const privateValue of ['PRIVATE-OFFICE','PRIVATE OFFICE QUOTE','PRIVATE OFFICE NOTE','office-selection-token','2'.repeat(64)]) expect(message).not.toContain(privateValue)
    expect(io.provider.sendUserMessage.mock.calls[0][2].submissionIsCurrent).toBeTypeOf('function')
    expect(draft().input).toBe('')
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })
  it.each(['settings','checkpoint'])('refuses a stale readonly Office scope after late %s I/O and preserves the draft', async boundary => {
    await attachOffice()
    const settings = deferred<AppSettingsV1>(), checkpoint = deferred<{ok:false;reason:'not_git_repo';message:string}>()
    if (boundary === 'settings') io.settings.mockReturnValueOnce(settings.promise)
    else io.checkpoint.mockReturnValueOnce(checkpoint.promise)
    await send()
    expect(io.office).toHaveBeenCalledOnce()
    io.office.mockResolvedValue({ok:false,view:officeView(),error:'stale_selection'})
    if (boundary === 'settings') settings.resolve({workspaceRoot:workspace} as AppSettingsV1)
    else checkpoint.resolve({ok:false,reason:'not_git_repo',message:'synthetic fixture'})
    await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(useChatStore.getState().blocks).toEqual([])
    expect(useChatStore.getState().busy).toBe(false)
  })
  it('refuses an Office draft send after navigation during final settings preparation', async () => {
    await attachOffice()
    const settings = deferred<AppSettingsV1>();io.settings.mockReturnValueOnce(settings.promise)
    await send()
    await act(async () => useChatStore.getState().setRoute('schedule'))
    settings.resolve({workspaceRoot:workspace} as AppSettingsV1);await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
  })
  it('rejects an unscoped Office quote before preparing or clearing the composer', async () => {
    await attachOffice()
    await act(async () => useNativeReferenceStore.getState().revokeScopes(officeReference().objectId))
    await send()
    expect(io.office).not.toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
  })
  it('preserves the draft and avoids queueing a readonly Office submission while busy', async () => {
    await attachOffice()
    await act(async () => useChatStore.setState({busy:true}))
    await send()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(useChatStore.getState().queuedMessages).toEqual([])
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
  })
  it('keeps later user input and a replacement Office reference after an older send succeeds', async () => {
    await attachOffice()
    const receipt = deferred<{turnId:string;userMessageItemId:string}>()
    io.provider.sendUserMessage.mockReturnValueOnce(receipt.promise)
    await send()
    await act(async () => {composer().setInput('Later user input');useNativeReferenceStore.getState().add({...officeReference(),scopeId:'b'.repeat(48)})})
    const replacement = useNativeReferenceStore.getState().references[0]
    receipt.resolve({turnId:'turn-a',userMessageItemId:'message-a'});await settle()
    expect(draft().input).toBe('Later user input')
    expect(useNativeReferenceStore.getState().references).toEqual([replacement])
  })
  it('prepares a readonly discussion draft once, preserves typed input and waits for user send', async () => {
    await attachOffice()
    await act(async () => {useWriteWorkspaceStore.setState({workspaceRoot:workspace,activeFilePath:officeReference().path,saveStatus:'saved'});composer().setInput('')})
    await settle()
    expect(io.document).not.toBeNull()
    await act(async () => {expect(io.document!.onPrepareDraft('Suggest changes',[officeReference()])).toBe(true)})
    expect(draft().input).toBe('Suggest changes')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    await act(async () => {composer().setInput('My own instructions');io.document!.onPrepareDraft('Suggest changes',[officeReference()]);io.document!.onPrepareDraft('Suggest changes',[officeReference()])})
    expect(draft().input).toBe('My own instructions')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    const prepareDraft = io.document!.onPrepareDraft
    await act(async () => useWorkspaceTabsStore.getState().openTab({id:'different-native',kind:'document',mode:'documents',title:'Different',workspaceRoot:workspace,path:workspace+'/other.docx'}))
    await act(async () => {expect(prepareDraft('Old target callback',[officeReference()])).toBe(false)})
    expect(draft().input).toBe('My own instructions')
    await act(async () => useChatStore.getState().setRoute('schedule'))
    await act(async () => {expect(prepareDraft('Stale callback',[officeReference()])).toBe(false)})
    expect(draft().input).toBe('My own instructions')
  })

  it('admits an editable Office draft without sending, then manual send carries proposal permission and only opaque context', async () => {
    const reference = editableOfficeReference(), view = editableOfficeView(reference)
    await mountOfficeDraft(view)
    await act(async () => {expect(io.document!.onPrepareDraft('Suggest local edits',[reference])).toBe(true)})
    expect(draft().input).toBe('Suggest local edits')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(useNativeReferenceStore.getState().references[0].editable).toBe(true)
    expect(io.office).not.toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    await act(async () => {
      composer().setInput('My own instructions')
      expect(io.document!.onPrepareDraft('Suggest local edits',[reference])).toBe(true)
      expect(io.document!.onPrepareDraft('Suggest local edits',[reference])).toBe(true)
    })
    expect(draft().input).toBe('My own instructions')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    await send()
    expect(io.office).toHaveBeenCalledTimes(2)
    expect(io.office).toHaveBeenCalledWith({action:'proposals',objectId:reference.objectId,scopeId:reference.scopeId})
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    const call = io.provider.sendUserMessage.mock.calls[0], message = call[1]
    expect(message).toContain('My own instructions')
    expect(message).toContain('[Office selection reference]')
    expect(message).toContain('Core scopeId: ' + reference.scopeId)
    expect(message).toContain('native_selection_propose')
    expect(message).toContain('wait for explicit user acceptance')
    expect(message).not.toContain('[Office discussion reference]')
    const publicPayload = JSON.stringify({call,blocks:useChatStore.getState().blocks})
    for (const privateValue of ['PRIVATE-OFFICE','PRIVATE OFFICE QUOTE','PRIVATE OFFICE NOTE','office-selection-token',reference.revision]) expect(publicPayload).not.toContain(privateValue)
    expect(call[2].submissionIsCurrent).toBeTypeOf('function')
    expect(draft().input).toBe('')
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })

  it('keeps an explicitly readonly draft discussion-only through manual send', async () => {
    const reference = officeReference(), view = officeView(reference)
    await mountOfficeDraft(view)
    await act(async () => {expect(io.document!.onPrepareDraft('Discuss suggested edits',[reference])).toBe(true)})
    expect(useNativeReferenceStore.getState().references[0].editable).toBe(false)
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    await send()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    const message = io.provider.sendUserMessage.mock.calls[0][1]
    expect(message).toContain('[Office discussion reference]')
    expect(message).toContain('does not authorize native_selection_propose or file edits')
    expect(message).not.toContain('[Office selection reference]')
  })
  it('rejects an old document callback even when its valid scope remains retained after switching files', async () => {
    const reference = editableOfficeReference(), previous = editableOfficeView(reference)
    await mountOfficeDraft(previous)
    const prepare = io.document!.onPrepareDraft
    const next:NativeOfficeView = {...previous,objectId:'3'.repeat(64),path:workspace+'/OTHER.docx',scope:undefined,selection:undefined}
    await act(async () => {
      useNativeOfficeStore.setState({target:{workspace,path:next.path},view:next,views:{retained:previous}})
      useWorkspaceTabsStore.getState().openTab({id:workspaceObjectTabId(workspace,next.path),kind:'document',mode:'documents',title:'Other',workspaceRoot:workspace,path:next.path})
      composer().setInput('Keep new file instructions')
    })
    await act(async () => {expect(prepare('Old file request',[reference])).toBe(false)})
    expect(draft().input).toBe('Keep new file instructions')
    expect(useNativeReferenceStore.getState().references).toEqual([])
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
  })

  it.each(['unscoped','scope-id','scope-thread','token','revision','sequence','permission','implicit-permission','loading','saving','incomplete'] as const)(
    'refuses a %s Office draft before changing typed input or adding a chip', async variant => {
      const reference = editableOfficeReference(), view = editableOfficeView(reference)
      if (variant === 'unscoped') delete reference.scopeId
      if (variant === 'scope-id') reference.scopeId = 'invalid-scope'
      if (variant === 'scope-thread') view.scope!.threadId = 'foreign-thread'
      if (variant === 'token') reference.selection.token = 'foreign-selection-token'
      if (variant === 'revision') reference.revision = 'f'.repeat(64)
      if (variant === 'sequence') reference.selection.changeSequence++
      if (variant === 'permission') view.scope!.editable = false
      if (variant === 'implicit-permission') delete reference.editable
      if (variant === 'loading') view.status = 'loading'
      if (variant === 'saving') view.saving = true
      if (variant === 'incomplete') reference.selection.capture!.complete = false
      await mountOfficeDraft(view)
      await act(async () => {
        composer().setInput('Keep typed instructions')
        expect(io.document!.onPrepareDraft('Discarded request',[reference])).toBe(false)
      })
      expect(draft().input).toBe('Keep typed instructions')
      expect(useNativeReferenceStore.getState().references).toEqual([])
      expect(io.office).not.toHaveBeenCalled()
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    })

  it.each(['preparation','final-send'] as const)('preserves an editable draft when Core rejects its %s scope check', async boundary => {
    const reference = editableOfficeReference(), view = editableOfficeView(reference)
    await mountOfficeDraft(view)
    await act(async () => {expect(io.document!.onPrepareDraft('Suggest local edits',[reference])).toBe(true)})
    if (boundary === 'final-send') io.office.mockResolvedValueOnce({ok:true,view})
    io.office.mockResolvedValue({ok:false,view,error:'stale_selection'})
    await send()
    expect(io.office).toHaveBeenCalledTimes(boundary === 'final-send' ? 2 : 1)
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Suggest local edits')
    expect(useNativeReferenceStore.getState().references).toHaveLength(1)
    expect(useNativeReferenceStore.getState().references[0].editable).toBe(true)
    expect(useChatStore.getState()).toMatchObject({busy:false,blocks:[]})
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
  it('releases a new-thread submission after acknowledgement while its index refresh is pending', async () => {
    const listing = deferred<NormalizedThread[]>()
    const first = { ...thread('fresh-one'), title: 'First named thread' }
    const second = { ...thread('fresh-two'), title: 'Second named thread' }
    Object.assign(io.provider, { createThread: vi.fn().mockResolvedValueOnce(first).mockResolvedValueOnce(second) })
    io.provider.listThreads.mockResolvedValueOnce([first, thread('a'), thread('b')])
      .mockReturnValueOnce(listing.promise).mockResolvedValue([first, second, thread('a'), thread('b')])
    io.provider.sendUserMessage.mockResolvedValueOnce({ turnId: 'fresh-turn-one', userMessageItemId: 'fresh-user-one' })
      .mockResolvedValueOnce({ turnId: 'fresh-turn-two', userMessageItemId: 'fresh-user-two' })
    try {
      await act(async () => {
        useNativeReferenceStore.setState({ references: [], drafts: {} })
        await useChatStore.getState().createThread()
      })
      await act(async () => composer().setInput('First new thread request'))
      await send()
      expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
      expect(io.provider.listThreads).toHaveBeenCalledTimes(2)
      expect(composer().input).toBe('')
      const sink = io.provider.subscribeThreadEvents.mock.calls[0][2] as ThreadEventSink
      await act(async () => { await sink.onTurnComplete({ threadId: first.id, turnId: 'fresh-turn-one', seq: 20 }) })
      await act(async () => { await useChatStore.getState().createThread() })
      await act(async () => composer().setInput('Second new thread request'))
      await send()
      expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(2)
      expect(io.provider.sendUserMessage.mock.calls.map(([id]) => id)).toEqual([first.id, second.id])
      expect(composer().input).toBe('')
    } finally {
      listing.resolve([first, thread('a'), thread('b')])
      await settle()
      Reflect.deleteProperty(io.provider, 'createThread')
    }
  })

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
    await mountDocumentActionOwner()
    const before = structuredClone(draft())
    const refs = structuredClone(useNativeReferenceStore.getState().references)
    // Explicit actions own their references without requiring composer chip membership.
    await act(async () => useNativeReferenceStore.setState({ references: [] }))
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

describe('mounted Workbench protected source navigation', () => {
  const protectedRead = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  const target = { path: workspace + '/private.md', workspaceRoot: workspace, line: 2, preview: 'source' as const, protectedRead }
  it('keeps a real Present selection in its selector-only tab while ordinary Office selection still registers its document tab', async () => {
    const path = workspace + '/private.docx'
    const selector = { ...protectedRead, index: 0 }
    io.office.mockResolvedValue({ ok: true, view: null })
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, {
      detail: { path, workspaceRoot: workspace, preview: 'source', protectedPresent: selector }
    })))
    await settle()
    const before = useWorkspaceTabsStore.getState().tabs
    const protectedTab = before.find(tab => tab.protectedPresent)!
    expect(protectedTab).toMatchObject({ kind: 'file', preview: 'source', protectedPresent: selector })
    expect(protectedTab.path).toBeUndefined()
    expect(protectedTab.workspaceRoot).toBeUndefined()
    await act(async () => { await useNativeOfficeStore.getState().selectPresent(workspace, path, selector) })
    await settle()
    expect(useNativeOfficeStore.getState().target?.protectedPresent).toEqual(selector)
    expect(useWorkspaceTabsStore.getState().tabs).toEqual(before)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(protectedTab.id)
    expect(useWorkspaceTabsStore.getState().tabs.some(tab => tab.kind === 'document' && tab.path === path)).toBe(false)

    await act(async () => { await useNativeOfficeStore.getState().select(workspace, path) })
    await settle()
    const ordinaryId = workspaceObjectTabId(workspace, path)
    expect(useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === ordinaryId)).toMatchObject({
      kind: 'document', mode: 'documents', path, workspaceRoot: workspace
    })
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(ordinaryId)
    expect(useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === protectedTab.id)).toEqual(protectedTab)
  })
  it('uses the real source panel without Write open/persistence/completion, raw transfer, Markdown resources or model requests', async () => {
    io.realPreview = true
    io.read.mockResolvedValue({ ok: true, path: target.path, content: '# private\n![remote](https://synthetic.invalid/image)\n<script>private</script>', size: 100, truncated: false, line: 2 })
    const openFile = vi.spyOn(useWriteWorkspaceStore.getState(), 'openFile')
    const initialize = vi.spyOn(useWriteWorkspaceStore.getState(), 'initializeWorkspace')
    const clipboard = vi.fn(), fetch = vi.fn()
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboard } })
    vi.stubGlobal('fetch', fetch)
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: target })))
    await settle()
    expect(openFile).not.toHaveBeenCalled(); expect(initialize).not.toHaveBeenCalled()
    expect(useWriteWorkspaceStore.getState().activeFilePath).toBeNull()
    expect(io.read).toHaveBeenCalledWith(expect.objectContaining({ protectedRead, preview: 'source', line: 2 }))
    expect(element.querySelector('[data-line="2"]')).not.toBeNull()
    expect(element.querySelector('[data-protected-source-preview] img')).toBeNull()
    const source = element.querySelector('[data-protected-source-preview]')!
    const filename = source.querySelector<HTMLButtonElement>('[data-workspace-viewer-header] button[aria-haspopup="menu"]')!
    expect(filename).not.toBeNull()
    await act(async () => filename.click())
    const menu = document.getElementById(filename.getAttribute('aria-controls')!)!
    expect(menu).not.toBeNull()
    expect(menu.getAttribute('role')).toBe('menu')
    const copy = [...menu.querySelectorAll<HTMLButtonElement>('button')]
      .find(button => button.getAttribute('aria-label') === 'filePreviewCopyContent')
    expect(copy).toBeDefined()
    expect(copy!.disabled).toBe(true)
    const openEditor = menu.querySelector<HTMLButtonElement>('[aria-label="filePreviewOpenEditor"]')
    expect(openEditor).not.toBeNull()
    expect(openEditor!.disabled).toBe(true)
    for (const name of ['copy', 'cut', 'dragstart']) {
      const event = new Event(name, { bubbles: true, cancelable: true }); source.dispatchEvent(event); expect(event.defaultPrevented).toBe(true)
    }
    expect(clipboard).not.toHaveBeenCalled(); expect(fetch).not.toHaveBeenCalled()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled(); expect(io.retrieve).not.toHaveBeenCalled()
    const tab = useWorkspaceTabsStore.getState().tabs.find(tab => tab.preview === 'source')!
    expect(tab).toMatchObject({ title: 'private.md', protectedRead, sourceLine: 2 })
    expect(tab.path).toBeUndefined(); expect(tab.workspaceRoot).toBeUndefined()
    expect(JSON.stringify(localStorage)).not.toContain('private.md')
    vi.unstubAllGlobals()
  })
  it('aligns the authoritative source line with Go CR-only line coordinates', async () => {
    io.realPreview = true
    io.read.mockResolvedValue({ ok: true, path: target.path, content: 'first\rsecond\rthird', size: 18, truncated: false, line: 2 })
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: target })))
    await settle()
    const panel = element.querySelector('[data-protected-source-preview]')!
    expect(panel.querySelectorAll('[data-line]')).toHaveLength(3)
    expect(panel.querySelector('[data-line="2"]')?.classList.contains('is-active')).toBe(true)
    expect(panel.querySelectorAll('.ds-file-preview-code-html .line')[1]?.textContent).toBe('second')
  })
  it('keeps protection through tab selection and clears source bodies/refs on thread change including late reads', async () => {
    io.realPreview = true
    const read = deferred<{ ok: true; path: string; content: string; size: number; truncated: boolean; line: number }>()
    io.read.mockReturnValue(read.promise)
    const openFile = vi.spyOn(useWriteWorkspaceStore.getState(), 'openFile')
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT, { detail: target })))
    await settle()
    const tab = useWorkspaceTabsStore.getState().tabs.find(tab => tab.preview === 'source')!
    await act(async () => { useWorkspaceTabsStore.getState().showSelector() })
    // Click the real tab control to exercise Workbench's target reconstruction.
    await act(async () => { (element.querySelector('[role="tab"]') as HTMLButtonElement).click() })
    expect(useWorkspaceTabsStore.getState().tabs.find(item => item.id === tab.id)?.preview).toBe('source')
    expect(openFile).not.toHaveBeenCalled()
    await act(async () => { useChatStore.setState({ activeThreadId: 'b' }) })
    await act(async () => read.resolve({ ok: true, path: target.path, content: 'LATE_PRIVATE_BODY', size: 17, truncated: false, line: 2 }))
    await settle()
    expect(useWorkspaceTabsStore.getState().tabs.some(tab => tab.preview === 'source')).toBe(false)
    expect(element.textContent).not.toContain('LATE_PRIVATE_BODY')
    expect(openFile).not.toHaveBeenCalled()
  })
  it('masks protected body and highlighted HTML on every source-to-ordinary/null commit', async () => {
    const ordinary = { path: workspace + '/ordinary.txt', workspaceRoot: workspace }
    io.read.mockImplementation(async value => ({ ok: true, path: value.path,
      content: value.protectedRead ? 'PROTECTED_SOURCE_BODY' : 'ORDINARY_BODY', size: 21, truncated: false }))
    const ordinaryHighlight = deferred<string>()
    vi.spyOn(codeHighlighting, 'highlightCodeHtml').mockImplementation(content => content === 'PROTECTED_SOURCE_BODY'
      ? Promise.resolve('<pre>PROTECTED_SOURCE_HIGHLIGHT</pre>') : ordinaryHighlight.promise)
    const commits: { protected: boolean; text: string }[] = []
    const renderPanel = async (next: typeof target | typeof ordinary | null) => {
      await act(async () => root.render(createElement(Profiler, { id: 'source-retirement', onRender: () => {
        commits.push({ protected: Boolean(element.querySelector('[data-protected-source-preview]')), text: element.textContent ?? '' })
      } }, createElement(WorkspaceFilePreviewPanel, { target: next, workspaceRoot: workspace, onClose: vi.fn() }))))
      await settle()
    }
    await renderPanel(target)
    expect(element.textContent).toContain('PROTECTED_SOURCE_HIGHLIGHT')
    commits.length = 0
    await renderPanel(ordinary)
    expect(element.textContent).toContain('ORDINARY_BODY')
    expect(commits.filter(commit => !commit.protected).some(commit => /PROTECTED_SOURCE/.test(commit.text))).toBe(false)
    await renderPanel(null)
    expect(element.textContent).not.toContain('ORDINARY_BODY')
    expect(element.textContent).not.toContain('PROTECTED_SOURCE')
    await act(async () => ordinaryHighlight.resolve('<pre>LATE_ORDINARY_HIGHLIGHT</pre>'))
    expect(element.textContent).not.toContain('LATE_ORDINARY_HIGHLIGHT')
  })
  it('keeps ordinary Markdown in file preview without opening or registering the retired Write adapter', async () => {
    const initialize = vi.spyOn(useWriteWorkspaceStore.getState(),'initializeWorkspace')
    const openFile = vi.spyOn(useWriteWorkspaceStore.getState(),'openFile')
    const flushSave = vi.spyOn(useWriteWorkspaceStore.getState(),'flushSave')
    await act(async () => window.dispatchEvent(new CustomEvent(WORKSPACE_FILE_PREVIEW_EVENT,{detail:{path:workspace+'/ordinary.md',workspaceRoot:workspace,line:2}})))
    await settle()
    const id=workspaceObjectTabId(workspace,workspace+'/ordinary.md')
    expect(useWorkspaceTabsStore.getState().tabs.find(tab=>tab.id===id)).toMatchObject({kind:'file',mode:'file',path:workspace+'/ordinary.md'})
    expect(initialize).not.toHaveBeenCalled();expect(openFile).not.toHaveBeenCalled();expect(flushSave).not.toHaveBeenCalled()
    await act(async()=>useWriteWorkspaceStore.setState({workspaceRoot:workspace,activeFilePath:workspace+'/retained.md',activeFileKind:'text',saveStatus:'dirty',fileContent:'Retained local draft'}))
    expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.path===workspace+'/retained.md')).toBe(false)
    expect(useWriteWorkspaceStore.getState().fileContent).toBe('Retained local draft')
  })
})


const imageReference = (): Omit<ImageRegionNativeReference, 'id'> => ({ kind: 'image-region', threadId: 'a', workspace,
  path: workspace + '/synthetic.png', objectId: '3'.repeat(64), revision: '4'.repeat(64), label: 'Synthetic region', text: '',
  sessionId: '5'.repeat(48), scopeId: '6'.repeat(48), editable: false, annotationRevision: '7'.repeat(64),
  regionId: '8'.repeat(48), width: 640, height: 480, region: { x: 10, y: 20, width: 80, height: 60 } })
const browserReference = (): Omit<BrowserNativeReference, 'id'> => {
  const browserScope = { scopeId: 'a'.repeat(48), documentId: 'b'.repeat(48), selectionId: 'c'.repeat(48), threadId: 'a', workspace }
  return { kind: 'browser-selection', threadId: 'a', workspace, objectId: browserScope.documentId,
    revision: browserScope.selectionId, scopeId: browserScope.scopeId, browserScope,
    label: 'Synthetic browser selection', text: '', path: '', editable: false }
}
async function attachIntentReference(kind: 'office' | 'canvas' | 'image-region' | 'browser-selection'): Promise<NativeReference> {
  if (kind === 'office') await attachOffice()
  else await act(async () => {
    useNativeReferenceStore.setState({ references: [] })
    useNativeReferenceStore.getState().add(kind === 'canvas' ? reference() : kind === 'image-region' ? imageReference() : browserReference())
  })
  io.browser.mockResolvedValue({ ok: true })
  const image = imageReference()
  io.objects.mockResolvedValue({ ok: true, scope: { kind: 'image-region', sessionId: image.sessionId, scopeId: image.scopeId,
    objectId: image.objectId, threadId: image.threadId, sourceRevision: image.revision, annotationRevision: image.annotationRevision,
    regionId: image.regionId, region: image.region, width: image.width, height: image.height,
    purpose: 'discuss', editable: false, current: true } })
  return structuredClone(useNativeReferenceStore.getState().references[0])
}

describe.each(['office', 'canvas', 'image-region', 'browser-selection'] as const)('ordinary %s reference send intent', kind => {
  it.each(['settings', 'checkpoint'] as const)('cancels removal during %s while Core still admits the frozen scope', async boundary => {
    await attachIntentReference(kind); await addAssets()
    const before = structuredClone(draft())
    const settings = deferred<AppSettingsV1>(), checkpoint = deferred<{ ok: false; reason: 'not_git_repo'; message: string }>()
    if (boundary === 'settings') io.settings.mockReturnValueOnce(settings.promise)
    else io.checkpoint.mockReturnValueOnce(checkpoint.promise)
    await send()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    const remove = element.querySelector<HTMLButtonElement>('button[aria-label="nativeReferenceRemove"]')!
    expect(remove).not.toBeNull()
    await act(async () => remove.click())
    if (boundary === 'settings') settings.resolve({ workspaceRoot: workspace } as AppSettingsV1)
    else checkpoint.resolve({ ok: false, reason: 'not_git_repo', message: 'synthetic fixture' })
    await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft()).toEqual(before)
    expect(useChatStore.getState().blocks).toEqual([])
    expect(useChatStore.getState().busy).toBe(false)
    expect(useNativeReferenceStore.getState().references).toEqual([])
  })
  it('rejects replacement with the same scope and keeps the replacement and draft', async () => {
    const frozen = await attachIntentReference(kind)
    const settings = deferred<AppSettingsV1>(); io.settings.mockReturnValueOnce(settings.promise)
    await send()
    await act(async () => {
      useNativeReferenceStore.getState().remove(frozen.id)
      useNativeReferenceStore.getState().add(frozen)
    })
    const replacement = useNativeReferenceStore.getState().references[0]
    expect(replacement.id).not.toBe(frozen.id); expect(replacement.scopeId).toBe(frozen.scopeId)
    settings.resolve({ workspaceRoot: workspace } as AppSettingsV1); await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft().input).toBe('Discuss selected objects')
    expect(useNativeReferenceStore.getState().references).toEqual([replacement])
    expect(useChatStore.getState().blocks).toEqual([])
  })
  it('rejects removal during file preparation before any scope can be sent', async () => {
    const frozen = await attachIntentReference(kind); await addAssets()
    const before = structuredClone(draft()), read = deferred<{ ok: true; content: string }>()
    io.read.mockReturnValueOnce(read.promise)
    await send()
    await act(async () => useNativeReferenceStore.getState().remove(frozen.id))
    read.resolve({ ok: true, content: 'Synthetic file' }); await settle()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    expect(draft()).toEqual(before); expect(useChatStore.getState().blocks).toEqual([])
  })
  it('preserves a different reference added after freezing without including it in this send', async () => {
    const frozen = await attachIntentReference(kind)
    const settings = deferred<AppSettingsV1>(); io.settings.mockReturnValueOnce(settings.promise)
    await send()
    let added!: NativeReference
    await act(async () => {
      const next = browserReference()
      added = useNativeReferenceStore.getState().add({ ...next, objectId: 'e'.repeat(48), scopeId: 'f'.repeat(48),
        browserScope: { ...next.browserScope, documentId: 'e'.repeat(48), scopeId: 'f'.repeat(48) } })
    })
    settings.resolve({ workspaceRoot: workspace } as AppSettingsV1); await settle()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    expect(io.provider.sendUserMessage.mock.calls[0][1]).toContain('Core scopeId: ' + frozen.scopeId)
    expect(io.provider.sendUserMessage.mock.calls[0][1]).not.toContain('Core scopeId: ' + added.scopeId)
    expect(useNativeReferenceStore.getState().references).toEqual([added])
  })
})

it('keeps source actions separate from summary-only details and closes details on revision, removal and owner changes', async () => {
  await attachOffice()
  const source = Array.from(element.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent?.includes('PRIVATE-OFFICE-LABEL'))!
  const trigger = element.querySelector<HTMLButtonElement>('button[aria-label="nativeReferenceDetails"]')!
  expect(source).not.toBe(trigger); expect(source.querySelector('button')).toBeNull()
  expect(element.textContent).not.toContain('PRIVATE OFFICE QUOTE')
  await act(async () => { trigger.focus(); trigger.click() })
  let panel = document.querySelector<HTMLElement>('[role="dialog"][aria-label="nativeReferenceDetails"]')!
  expect(panel.parentElement).toBe(document.body)
  expect(panel.textContent).not.toContain('PRIVATE OFFICE QUOTE')
  expect(panel.textContent).not.toContain('PRIVATE OFFICE NOTE')
  const selection = window.getSelection()!
  selection.selectAllChildren(panel)
  expect(selection.toString()).not.toMatch(/PRIVATE OFFICE (QUOTE|NOTE)/)
  selection.removeAllRanges()
  expect(document.activeElement).toBe(trigger)
  await act(async () => source.click())
  expect(useWorkspaceTabsStore.getState().tabs.some(tab => tab.path === officeReference().path)).toBe(true)
  await act(async () => useNativeReferenceStore.setState(state => ({ references: state.references.map(r => ({ ...r, revision: '9'.repeat(64) })) })))
  expect(document.querySelector('[role="dialog"][aria-label="nativeReferenceDetails"]')).toBeNull()
  await act(async () => element.querySelector<HTMLButtonElement>('button[aria-label="nativeReferenceDetails"]')!.click())
  panel = document.querySelector<HTMLElement>('[role="dialog"][aria-label="nativeReferenceDetails"]')!
  expect(panel).not.toBeNull()
  await act(async () => element.querySelector<HTMLButtonElement>('button[aria-label="nativeReferenceRemove"]')!.click())
  expect(document.querySelector('[role="dialog"][aria-label="nativeReferenceDetails"]')).toBeNull()
  await attachOffice()
  await act(async () => element.querySelector<HTMLButtonElement>('button[aria-label="nativeReferenceDetails"]')!.click())
  await act(async () => useChatStore.setState({ activeThreadId: 'b' }))
  expect(document.querySelector('[role="dialog"][aria-label="nativeReferenceDetails"]')).toBeNull()
})


it('opens a real guide entry without replacing the dirty document or sending its content', async () => {
  const owned = { id: workspaceObjectTabId(workspace, workspace + '/owned.docx'), kind: 'document' as const,
    mode: 'documents' as const, title: 'owned.docx', workspaceRoot: workspace, path: workspace + '/owned.docx', dirty: true }
  await act(async () => { useWorkspaceTabsStore.getState().openTab(owned); useWorkspaceTabsStore.getState().showSelector() })
  const writes = io.writeFile.mock.calls.length, sends = io.provider.sendUserMessage.mock.calls.length
  const files = element.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="files"]')!
  expect(files).not.toBeNull()
  await act(async () => files.click())
  expect(useWorkspaceTabsStore.getState().selectorOpen).toBe(false)
  expect(useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === owned.id)).toEqual(owned)
  expect(useWorkspaceTabsStore.getState().tabs.find(tab => tab.id === useWorkspaceTabsStore.getState().activeTabId)?.mode).toBe('files')
  const retained = useWorkspaceTabsStore.getState().tabs
  await act(async () => element.querySelector<HTMLButtonElement>('#workbench-workspace-collapse')!.click())
  expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: false, tabs: retained })
  await act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', {detail:'toggle-workspace'})))
  expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, tabs: retained })
  expect(io.writeFile).toHaveBeenCalledTimes(writes)
  expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(sends)
})

it('returns to an existing document tab without a retired documents creation entry', async () => {
  const owned = { id: workspaceObjectTabId(workspace, workspace + '/owned.docx'), kind: 'document' as const,
    mode: 'documents' as const, title: 'owned.docx', workspaceRoot: workspace, path: workspace + '/owned.docx', dirty: true }
  await act(async () => { useWorkspaceTabsStore.getState().openTab(owned); useWorkspaceTabsStore.getState().showSelector() })
  expect(element.querySelector('[data-sidebar-right-guide-entry="documents"]')).toBeNull()
  await act(async () => [...element.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(tab=>tab.title===owned.title)!.click())
  expect(useWorkspaceTabsStore.getState()).toMatchObject({selectorOpen:false,activeTabId:owned.id,tabs:[owned]})
  expect(io.writeFile).not.toHaveBeenCalled()
  expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
})


it.each([
  { name: 'automatic fullscreen', width: 680, focused: false, collapse: true },
  { name: 'automatic fullscreen after manual maximize', width: 680, focused: true, collapse: true },
  { name: 'manual fullscreen in a wide window', width: 1600, focused: true, collapse: false }
])('DSH workspace restore: $name', async ({ width, focused, collapse }) => {
  const viewport = Object.getOwnPropertyDescriptor(window, 'innerWidth')
  const owned = { id: workspaceObjectTabId(workspace, workspace + '/owned.docx'), kind: 'document' as const,
    mode: 'documents' as const, title: 'owned.docx', workspaceRoot: workspace, path: workspace + '/owned.docx', dirty: true }
  try {
    io.realWorkspaceChrome = true
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
    await act(async () => {
      useWorkspaceTabsStore.setState({ tabs: [owned], activeTabId: owned.id, selectorOpen: false, open: true, focused })
      window.dispatchEvent(new Event('resize'))
    })
    const writes = io.writeFile.mock.calls.length, sends = io.provider.sendUserMessage.mock.calls.length
    const restore = element.querySelector<HTMLButtonElement>('[data-workspace-viewer-header][data-viewer-controls="true"] button[aria-label="workbenchDock"][aria-pressed="true"]')!
    expect(restore).not.toBeNull()
    expect(restore.disabled).toBe(false)
    expect(element.querySelector('#workbench-right-workspace')?.getAttribute('data-auto-overlay')).toBe(collapse ? 'true' : null)
    restore.focus()
    await act(async () => restore.click())
    await act(async () => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: !collapse, focused: false, tabs: [owned] })
    if (collapse) {
      expect(document.activeElement?.id).toBe('workbench-workspace-expand')
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1600 })
      await act(async () => window.dispatchEvent(new Event('resize')))
      expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: false, focused: false, tabs: [owned] })
    }
    expect(io.writeFile).toHaveBeenCalledTimes(writes)
    expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(sends)
  } finally {
    if (viewport) Object.defineProperty(window, 'innerWidth', viewport)
    else Reflect.deleteProperty(window, 'innerWidth')
  }
})

function wordCreationReceipt(input:{operationId:string;fileName:string}) {return {operationId:input.operationId,workspace,path:workspace+'/'+input.fileName,revision:'a'.repeat(64),kind:'docx' as const,byteSize:128,savedAt:'2026-10-15T00:00:00Z'}}
async function showWordCreate(fileName='created.docx') {
  await act(async()=>useWorkspaceTabsStore.getState().showSelector())
  await act(async()=>element.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.click())
  const input=element.querySelector<HTMLInputElement>('form[aria-label="workspaceNewWord"] input')!
  await act(async()=>{
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,fileName)
    input.dispatchEvent(new Event('input',{bubbles:true}))
  })
}
async function submitWordCreate() {await act(async()=>element.querySelector<HTMLButtonElement>('form[aria-label="workspaceNewWord"] button[type="submit"]')!.click());await settle()}
const createRequests=()=>io.office.mock.calls.map(call=>call[0]).filter(input=>input.action==='create')

describe('Workbench real Word creation and regeneration entry',()=>{
  it('creates from the guide through the frozen DTO and selects only the receipt file while retaining a dirty tab',async()=>{
    const owned={id:workspaceObjectTabId(workspace,workspace+'/dirty.docx'),kind:'document' as const,mode:'documents' as const,title:'dirty.docx',workspaceRoot:workspace,path:workspace+'/dirty.docx',dirty:true}
    await act(async()=>useWorkspaceTabsStore.getState().openTab(owned))
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:true,view:null,creation:wordCreationReceipt(input)}:{ok:true,view:null})
    await showWordCreate();await submitWordCreate()
    const calls=createRequests();expect(calls).toHaveLength(1)
    expect(calls[0]).toMatchObject({threadId:'a',fileName:'created.docx'})
    expect(calls[0]).not.toHaveProperty('workspace');expect(calls[0]).not.toHaveProperty('path')
    const tabs=useWorkspaceTabsStore.getState()
    expect(tabs.activeTabId).toBe(workspaceObjectTabId(workspace,workspace+'/created.docx'))
    expect(tabs.tabs.find(tab=>tab.id===owned.id)).toEqual(owned)
    expect(tabs.tabs.find(tab=>tab.id===tabs.activeTabId)).toMatchObject({workspaceRoot:workspace,path:workspace+'/created.docx',mode:'documents'})
    expect(useNativeOfficeStore.getState().view).toBeNull()
    expect(io.writeFile).not.toHaveBeenCalled();expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
  })
  it('does not duplicate an unresolved create and retries its original operation ID',async()=>{
    const response=deferred<unknown>()
    io.office.mockImplementation(async(input)=>input.action==='create'?response.promise:{ok:true,view:null})
    await showWordCreate()
    await act(async()=>{const button=element.querySelector<HTMLButtonElement>('form button[type="submit"]')!;button.click();button.click()})
    expect(createRequests()).toHaveLength(1)
    const original=createRequests()[0]
    response.resolve({ok:false,error:'unknown',view:null});await settle()
    expect(element.querySelector<HTMLInputElement>('form input')!.disabled).toBe(true)
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:true,view:null,creation:wordCreationReceipt(input)}:{ok:true,view:null})
    await submitWordCreate()
    expect(createRequests()).toHaveLength(2);expect(createRequests()[1].operationId).toBe(original.operationId)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(workspaceObjectTabId(workspace,workspace+'/created.docx'))
  })
  it.each(['thread','workspace','tab'] as const)('keeps a late receipt out of the new %s tabs while retaining its original attempt',async change=>{
    const response=deferred<unknown>()
    io.office.mockImplementation(async(input)=>input.action==='create'?response.promise:{ok:true,view:null})
    await showWordCreate();await act(async()=>element.querySelector<HTMLButtonElement>('form button[type="submit"]')!.click())
    const input=createRequests()[0]
    await act(async()=>{
      if(change==='thread')useChatStore.setState({activeThreadId:'b'})
      else if(change==='workspace')useChatStore.setState({threads:[{...thread('a'),workspace:'/synthetic/other-workspace'},thread('b')]})
      else useWorkspaceTabsStore.getState().openTab({id:'other-tool',kind:'tool',mode:'changes',title:'Other'})
    })
    const active=useWorkspaceTabsStore.getState().activeTabId,tabs=useWorkspaceTabsStore.getState().tabs
    const officeCalls=io.office.mock.calls.length
    response.resolve({ok:true,view:null,creation:wordCreationReceipt(input)});await settle()
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(active)
    expect(useWorkspaceTabsStore.getState().tabs).toEqual(tabs)
    expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.path===workspace+'/created.docx')).toBe(false)
    expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({input:{threadId:'a',operationId:input.operationId},phase:'created',creation:wordCreationReceipt(input)})
    expect(useNativeOfficeStore.getState().creationAttempt?.presented).not.toBe(true)
    expect(useNativeOfficeStore.getState().target).toBeNull()
    expect(io.office.mock.calls.slice(officeCalls).some(([request])=>['hide','open','create'].includes(request.action))).toBe(false)
    expect(createRequests()).toHaveLength(1)
  })
  it('does not activate an old-workspace receipt when the same thread retries from a different workspace',async()=>{
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:false,error:'unknown',view:null}:{ok:true,view:null})
    await showWordCreate();await submitWordCreate()
    const original=createRequests()[0]
    await act(async()=>useChatStore.setState({threads:[{...thread('a'),workspace:'/synthetic/new-workspace'},thread('b')]}))
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:true,view:null,creation:wordCreationReceipt(input)}:{ok:true,view:null})
    const active=useWorkspaceTabsStore.getState().activeTabId
    await submitWordCreate()
    expect(createRequests()[1].operationId).toBe(original.operationId)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(active)
    expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.workspaceRoot===workspace&&tab.path===workspace+'/created.docx')).toBe(false)
    expect(useNativeOfficeStore.getState().creationAttempt?.creation?.workspace).toBe(workspace)
    expect(element.querySelector('[data-word-open-created]')).toBeNull()
    expect(useNativeOfficeStore.getState().target).toBeNull()
  })
  it.each(['thread','workspace'] as const)('allows an explicit new create in a different %s after the previous receipt is confirmed',async change=>{
    const response=deferred<unknown>()
    io.office.mockImplementation(async(input)=>input.action==='create'?response.promise:{ok:true,view:null})
    await showWordCreate();await act(async()=>element.querySelector<HTMLButtonElement>('form button[type="submit"]')!.click())
    const original=createRequests()[0]
    const nextWorkspace=change==='workspace'?'/synthetic/new-workspace':workspace
    await act(async()=>{
      if(change==='thread')useChatStore.setState({activeThreadId:'b'})
      else useChatStore.setState({threads:[{...thread('a'),workspace:nextWorkspace},thread('b')]})
    })
    response.resolve({ok:true,view:null,creation:wordCreationReceipt(original)});await settle()
    expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.path===workspace+'/created.docx')).toBe(false)
    expect(useNativeOfficeStore.getState().creationAttempt?.phase).toBe('created')
    expect(element.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="word"]')!.disabled).toBe(false)
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:true,view:null,creation:{...wordCreationReceipt(input),workspace:nextWorkspace,path:nextWorkspace+'/'+input.fileName}}:{ok:true,view:null})
    await showWordCreate('second.docx');await submitWordCreate()
    const calls=createRequests();expect(calls).toHaveLength(2)
    expect(calls[1]).toMatchObject({threadId:change==='thread'?'b':'a',fileName:'second.docx'})
    expect(calls[1].operationId).not.toBe(original.operationId)
    expect(calls.filter(input=>input.fileName==='created.docx')).toHaveLength(1)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(workspaceObjectTabId(nextWorkspace,nextWorkspace+'/second.docx'))
    expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.path===workspace+'/created.docx')).toBe(false)
    expect(io.writeFile).not.toHaveBeenCalled()
  })
  it('explicitly opens the retained receipt only after returning to its owner, without creating again',async()=>{
    io.realDocument=true
    io.docxEditor.mockResolvedValue({ok:false,view:null,error:'unavailable'})
    const response=deferred<unknown>()
    io.office.mockImplementation(async(input)=>input.action==='create'?response.promise
      :{ok:true,view:null})
    await showWordCreate();await act(async()=>element.querySelector<HTMLButtonElement>('form button[type="submit"]')!.click())
    const input=createRequests()[0]
    await act(async()=>useChatStore.setState({activeThreadId:'b'}))
    response.resolve({ok:false,error:'engine_unavailable',view:null,creation:wordCreationReceipt(input)});await settle()
    expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.path===workspace+'/created.docx')).toBe(false)
    expect(element.querySelector('[data-word-open-created]')).toBeNull()
    expect(io.office.mock.calls.some(([request])=>request.action==='open')).toBe(false)
    expect(io.docxEditor).not.toHaveBeenCalled()
    await act(async()=>{useChatStore.setState({activeThreadId:'a'});useWorkspaceTabsStore.getState().showSelector()});await settle()
    io.packageHost.mockResolvedValue({ok:false,code:'unavailable',message:'Synthetic not ready'})
    const lists=io.packageHost.mock.calls.length
    const open=element.querySelector<HTMLButtonElement>('[data-word-open-created]')!
    expect(open).not.toBeNull()
    await act(async()=>{open.click();open.click()});await settle()
    expect(createRequests()).toHaveLength(1)
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(workspaceObjectTabId(workspace,workspace+'/created.docx'))
    expect(io.docxEditor).toHaveBeenCalledExactlyOnceWith({action:'open',workspace,path:workspace+'/created.docx',threadId:'a'})
    expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({input:{operationId:input.operationId},presented:true})
    expect(io.packageHost).toHaveBeenCalledTimes(lists)
  })
  it.each(['missing','mismatch','forbidden'] as const)('does not fabricate a successful object tab after %s',async failure=>{
    io.office.mockImplementation(async(input)=>input.action==='create'?failure==='forbidden'?{ok:false,error:'forbidden',view:null}
      :{ok:true,view:null,...(failure==='missing'?{}:{creation:{...wordCreationReceipt(input),operationId:'wrong_operation_001'}})}:{ok:true,view:null})
    await showWordCreate();const tabs=useWorkspaceTabsStore.getState().tabs;await submitWordCreate()
    expect(useWorkspaceTabsStore.getState().tabs).toEqual(tabs)
    expect(useNativeOfficeStore.getState().target).toBeNull();expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    if(failure==='forbidden')expect(element.textContent).toContain('nativeOfficeForbidden')
  })
  it('keeps the created file after DOCX open failure and the document panel retry sends only open',async()=>{
    io.realDocument=true
    io.docxEditor.mockResolvedValue({ok:false,view:null,error:'unavailable'})
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:false,error:'engine_unavailable',view:null,creation:wordCreationReceipt(input)}
      :{ok:true,view:null})
    await showWordCreate();await submitWordCreate()
    expect(useWorkspaceTabsStore.getState().activeTabId).toBe(workspaceObjectTabId(workspace,workspace+'/created.docx'))
    const retry=[...element.querySelectorAll<HTMLButtonElement>('[data-docx-open-error] button')].find(button=>button.textContent==='重试')!
    expect(retry).not.toBeNull();await act(async()=>retry.click());await settle()
    expect(createRequests()).toHaveLength(1)
    const opens=io.docxEditor.mock.calls.map(call=>call[0]).filter(input=>input.action==='open')
    expect(opens.length).toBeGreaterThanOrEqual(2)
    for(const input of opens)expect(input).toMatchObject({workspace,path:workspace+'/created.docx',threadId:'a'})
    expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({phase:'created',presented:true})
    expect(io.office.mock.calls.some(([request])=>request.action==='open')).toBe(false)
  })
  it.each(['unset','list-failure','missing-operation'] as const)('blocks only new creation for %s readiness without changing retained state',async kind=>{
    const pkg=readyDocuments(),unset={...pkg,activationState:'unset',activationRevision:0,available:false}
    Reflect.deleteProperty(unset,'desiredState');Reflect.deleteProperty(unset,'activationId')
    io.packageHost.mockResolvedValue(kind==='list-failure'?{ok:false,code:'unavailable',message:'Synthetic'}
      :{ok:true,packages:kind==='unset'?[unset]:[{...pkg,operations:['open-object','close-object']}]})
    const retained=useNativeOfficeStore.getState().creationAttempt
    await showWordCreate();const tabs=useWorkspaceTabsStore.getState().tabs
    expect(element.querySelector<HTMLButtonElement>('form button[type="submit"]')!.disabled).toBe(true)
    expect(element.textContent).toContain(kind==='unset'?'workspaceWordDocumentsNotEnabled':kind==='list-failure'?'workspaceWordDocumentsUnknown':'workspaceWordDocumentsNotReady')
    expect(createRequests()).toEqual([]);expect(useNativeOfficeStore.getState().creationAttempt).toBe(retained)
    expect(useWorkspaceTabsStore.getState().tabs).toEqual(tabs)
    expect(io.packageHost.mock.calls.every(([input])=>input.action==='list')).toBe(true)
  })
  it('refreshes before fresh submission and exposes plugin navigation after a readiness change',async()=>{
    await showWordCreate()
    io.packageHost.mockResolvedValue({ok:true,packages:[{...readyDocuments(),desiredState:'disabled',available:false}]})
    await submitWordCreate()
    expect(createRequests()).toEqual([]);expect(useNativeOfficeStore.getState().creationAttempt).toBeNull()
    expect(element.textContent).toContain('workspaceWordDocumentsNotEnabled')
    await act(async()=>element.querySelector<HTMLButtonElement>('[data-word-open-plugins]')!.click())
    expect(useChatStore.getState().route).toBe('plugins')
    expect(io.packageHost.mock.calls.every(([input])=>input.action==='list')).toBe(true)
  })
  it.each(['thread','workspace','tab','route'] as const)('does not create after a delayed readiness check loses its %s submission owner',async change=>{
    await showWordCreate()
    const response=deferred<unknown>();io.packageHost.mockImplementationOnce(()=>response.promise)
    await act(async()=>{const submit=element.querySelector<HTMLButtonElement>('form button[type="submit"]')!;submit.click();submit.click()})
    expect(io.packageHost).toHaveBeenCalledTimes(2)
    await act(async()=>{
      if(change==='thread')useChatStore.setState({activeThreadId:'b'})
      if(change==='workspace')useChatStore.setState({threads:[{...thread('a'),workspace:'/synthetic/other'},thread('b')]})
      if(change==='tab')useWorkspaceTabsStore.getState().openTab({id:'another-tab',kind:'tool',mode:'files',title:'Other'})
      if(change==='route')useChatStore.setState({route:'plugins'})
    })
    response.resolve({ok:true,packages:[readyDocuments()]});await settle()
    expect(createRequests()).toEqual([]);expect(useNativeOfficeStore.getState().creationAttempt).toBeNull()
  })
  it.each([['package_unavailable','workspaceWordDocumentsNotReady'],['package_changed','workspaceWordDocumentsChanged'],
    ['invalid_request','workspaceWordInvalidRequest'],['capacity','workspaceWordCapacity'],['engine_unavailable','workspaceWordUnknown']])(
    'renders the actual %s creation refusal without generic filename advice',async(error,key)=>{
      io.office.mockImplementation(async(input)=>input.action==='create'?{ok:false,error,view:null}:{ok:true,view:null})
      await showWordCreate();await submitWordCreate()
      expect(element.textContent).toContain(key);expect(element.textContent).not.toContain('workspaceWordCreateFailed')
      expect(useWorkspaceTabsStore.getState().tabs.some(tab=>tab.path===workspace+'/created.docx')).toBe(false)
    })
  it('replays an unknown create with its original identity despite unavailable readiness and keeps a replay refusal unresolved',async()=>{
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:false,error:'unknown',view:null}:{ok:true,view:null})
    await showWordCreate();await submitWordCreate();const original=createRequests()[0]
    io.packageHost.mockResolvedValue({ok:false,code:'unavailable',message:'Synthetic'})
    io.office.mockImplementation(async(input)=>input.action==='create'?{ok:false,error:'package_unavailable',view:null}:{ok:true,view:null})
    await submitWordCreate()
    expect(createRequests()).toHaveLength(2);expect(createRequests()[1].operationId).toBe(original.operationId)
    // Readiness may refresh its list; create/replay mutations are counted above.
    expect(io.packageHost.mock.calls.map(([input])=>input.action).filter(action=>action!=='list')).toEqual([])
    expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({phase:'unknown',input:{operationId:original.operationId,fileName:original.fileName}})
    expect(element.textContent).toContain('workspaceWordUnknown')
    expect(element.querySelector<HTMLInputElement>('form input')!.disabled).toBe(true)
  })
  it('uses the existing Agent submit path for a fresh editable regeneration without replacing typed input',async()=>{
    const reference=editableOfficeReference(),view=editableOfficeView(reference)
    await mountOfficeDraft(view)
    await act(async()=>composer().setInput('Keep my typed draft'))
    await act(async()=>io.document!.onSubmitPrompt('Generate a fresh proposal',[reference]));await settle()
    expect(io.provider.sendUserMessage).toHaveBeenCalledOnce()
    const message=io.provider.sendUserMessage.mock.calls[0][1]
    expect(message).toContain('Generate a fresh proposal');expect(message).toContain('Core scopeId: '+reference.scopeId)
    expect(message).toContain('native_selection_propose');expect(message).toContain('wait for explicit user acceptance')
    expect(message).not.toContain(reference.text);expect(message).not.toContain(reference.note!)
    expect(draft().input).toBe('Keep my typed draft')
    expect(io.office.mock.calls.every(([input])=>input.action==='proposals')).toBe(true)
  })
})


it.each(['dirty','unknown'] as const)('retains a legacy %s draft when its document tab is closed until explicit recovery',async status=>{
  const path=workspace+'/retained.md',id=workspaceObjectTabId(workspace,path)
  const flush=vi.spyOn(useWriteWorkspaceStore.getState(),'flushSave')
  const close=vi.spyOn(useWriteWorkspaceStore.getState(),'openWorkspaceHome')
  await act(async()=>{
    useWriteWorkspaceStore.setState({workspaceRoot:workspace,activeFilePath:path,activeFileKind:'text',saveStatus:status,fileContent:'Retained original owner',
      pendingSave:status==='unknown'?{operationId:'legacy_save_001',objectId:'a'.repeat(64),baseRevision:'b'.repeat(64),content:'Retained original owner'}:null})
    useWorkspaceTabsStore.getState().openTab({id,kind:'document',mode:'documents',title:'retained.md',path,workspaceRoot:workspace,dirty:true})
  })
  await act(async()=>window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut',{detail:'close-tab'})))
  expect(useWorkspaceTabsStore.getState().tabs.find(tab=>tab.id===id)).toMatchObject({id,path,dirty:true})
  expect(useWriteWorkspaceStore.getState().fileContent).toBe('Retained original owner')
  expect(flush).not.toHaveBeenCalled();expect(close).not.toHaveBeenCalled()
})


describe('composer closure actual intake and delivery', () => {
  const pdf = (name: string) => {
    const file = new File(['synthetic'], name, { type: 'application/pdf' })
    Object.assign(file, { arrayBuffer: async () => new TextEncoder().encode('synthetic').buffer })
    return file
  }
  const ready = (id: string) => ({ id, name: id + '.pdf', mimeType: 'application/pdf', byteSize: 9, pageCount: 1 })
  it('retains partial batch success and failed files until an explicit retry succeeds', async () => {
    await act(async () => useNativeReferenceStore.setState({ references: [] }))
    io.provider.uploadAttachment.mockResolvedValueOnce(ready('first')).mockRejectedValueOnce(new Error('Synthetic second failure'))
    await act(async () => composer().onPickAttachments!([pdf('first.pdf'), pdf('second.pdf')]))
    await settle()
    expect(composer().attachments?.map(item => item.id)).toEqual(['first'])
    expect(composer().attachmentUploads).toHaveLength(1)
    expect(composer().attachmentUploads?.[0].status).toBe('failed')
    await send()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    const failed = composer().attachmentUploads![0]
    io.provider.uploadAttachment.mockResolvedValueOnce(ready('second'))
    await act(async () => composer().onRetryAttachmentUpload!(failed.id))
    await settle()
    expect(composer().attachmentUploads).toEqual([])
    expect(composer().attachments?.map(item => item.id)).toEqual(['first', 'second'])
    await send()
    expect(io.provider.sendUserMessage.mock.calls[0][2]).toMatchObject({ attachmentIds: ['first', 'second'] })
  })
  it('never revives a removed file from a late retry response', async () => {
    await act(async () => useNativeReferenceStore.setState({ references: [] }))
    io.provider.uploadAttachment.mockRejectedValueOnce(new Error('Synthetic failure'))
    await act(async () => composer().onPickAttachments!([pdf('retry.pdf')]))
    await settle()
    const failed = composer().attachmentUploads![0]
    const response = deferred<ReturnType<typeof ready>>()
    io.provider.uploadAttachment.mockReturnValueOnce(response.promise)
    await act(async () => composer().onRetryAttachmentUpload!(failed.id))
    await settle()
    expect(composer().attachmentUploads![0].attemptId).not.toBe(failed.attemptId)
    await act(async () => composer().onRemoveAttachmentUpload!(failed.id))
    await act(async () => response.resolve(ready('obsolete')))
    await settle()
    expect(composer().attachmentUploads).toEqual([])
    expect(composer().attachments).toEqual([])
  })
  it('materializes a first attachment thread and moves the latest text and references to that exact owner', async () => {
    const creation = deferred<NormalizedThread>()
    const fresh = { ...thread('first-attachment'), workspace: undefined, status: 'idle' as const }
    Object.assign(io.provider, { createThread: vi.fn(() => creation.promise) })
    io.provider.getThreadDetail.mockImplementation(async (id: string) => ({ ...completedDetail(id), blocks: [] }))
    io.provider.uploadAttachment.mockResolvedValueOnce(ready('first-upload'))
    try {
      await act(async () => { useNativeReferenceStore.setState({ references: [] }); await useChatStore.getState().createThread() })
      await act(async () => { composer().setInput('Before creation'); composer().onPickAttachments!([pdf('first.pdf')]); composer().onPickAttachments!([pdf('duplicate.pdf')]) })
      await settle()
      expect(io.provider.uploadAttachment).not.toHaveBeenCalled()
      expect(Reflect.get(io.provider, 'createThread')).toHaveBeenCalledOnce()
      expect(composer().attachmentUploads).toHaveLength(1)
      await act(async () => {
        composer().setInput('Newer typing while creating')
        composer().onAddFileReference!({ path: workspace + '/newer.md', relativePath: 'newer.md', name: 'newer.md' })
        creation.resolve(fresh)
      })
      await settle()
      expect(useChatStore.getState().activeThreadId).toBe(fresh.id)
      expect(io.provider.uploadAttachment.mock.calls[0][0]).toMatchObject({ threadId: fresh.id, workspace })
      expect(composer().input).toBe('Newer typing while creating')
      expect(composer().fileReferences?.[0].relativePath).toBe('newer.md')
      expect(composer().attachments?.[0].id).toBe('first-upload')
      expect(useChatStore.getState().composerDrafts[composerDraftKey(workspace, null)]).toBeUndefined()
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    } finally { creation.resolve(fresh); Reflect.deleteProperty(io.provider, 'createThread') }
  })
  it.each(['workspace', 'thread', 'workspace-aba'])('rejects a late materialization after navigating by %s', async navigation => {
    const creation = deferred<NormalizedThread>()
    const late = { ...thread('late-attachment'), status: 'idle' as const }
    Object.assign(io.provider, { createThread: vi.fn(() => creation.promise) })
    io.provider.getThreadDetail.mockImplementation(async (id: string) => completedDetail(id))
    try {
      await act(async () => { useNativeReferenceStore.setState({ references: [] }); await useChatStore.getState().createThread() })
      await act(async () => { composer().setInput('Original unsent input'); composer().onPickAttachments!([pdf('late.pdf')]) })
      await settle()
      await act(async () => {
        if (navigation === 'thread') await useChatStore.getState().selectThread('b')
        else {
          await useChatStore.getState().createThread({ workspaceRoot: '/synthetic/other' })
          if (navigation === 'workspace-aba') await useChatStore.getState().createThread({ workspaceRoot: workspace })
        }
      })
      const current = useChatStore.getState().activeThreadId
      await act(async () => creation.resolve(late))
      await settle()
      expect(useChatStore.getState().activeThreadId).toBe(current)
      expect(io.provider.uploadAttachment).not.toHaveBeenCalled()
      const original = useChatStore.getState().composerDrafts[composerDraftKey(workspace, null)]
      expect(original.input).toBe('Original unsent input')
      expect(original.attachmentUploads?.[0].status).toBe('failed')
      expect(useChatStore.getState().composerDrafts[composerDraftKey(workspace, late.id)]).toBeUndefined()
    } finally { creation.resolve(late); Reflect.deleteProperty(io.provider, 'createThread') }
  })
  it('passes an explicit queue intent to the actual busy store without attempting steer', async () => {
    const steer = vi.fn()
    Object.assign(io.provider, { steerUserMessage: steer })
    try {
      await act(async () => { useNativeReferenceStore.setState({ references: [] }); useChatStore.setState({ busy: true, currentTurnId: 'active-turn' }); composer().setInput('Queue this follow-up') })
      await act(async () => composer().onSend('queue'))
      await settle()
      expect(steer).not.toHaveBeenCalled()
      expect(useChatStore.getState().queuedMessages).toEqual([expect.objectContaining({ text: 'Queue this follow-up', threadId: 'a' })])
      expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
    } finally { Reflect.deleteProperty(io.provider, 'steerUserMessage') }
  })
  it.each(['queue', 'steer'] as const)('keeps the busy native-reference fence for %s intent', async intent => {
    const refs = useNativeReferenceStore.getState().references
    await act(async () => { useChatStore.setState({ busy: true, currentTurnId: 'active-turn' }); composer().onSend(intent) })
    await settle()
    expect(useChatStore.getState().queuedMessages).toEqual([])
    expect(useNativeReferenceStore.getState().references).toEqual(refs)
    expect(draft().input).toBe('Discuss selected objects')
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
  })
  it('binds the main capacity to the actual selected model profile and hides an unknown model', async () => {
    const profile = (contextWindowTokens: number) => ({ contextWindowTokens, inputModalities: ['text' as const],
      outputModalities: ['text' as const], supportsToolCalling: true, messageParts: [] })
    await act(async () => useChatStore.setState({ composerProviderId: 'one', composerModel: 'small', composerModelGroups: [
      { providerId: 'one', label: 'One', modelIds: ['small', 'large'], modelProfiles: { small: profile(4096), large: profile(32768) } }
    ] }))
    expect(composer().contextWindowTokens).toBe(4096)
    await act(async () => composer().onComposerModelChange('large', 'one'))
    expect(composer().contextWindowTokens).toBe(32768)
    await act(async () => composer().onComposerModelChange('unknown', 'one'))
    expect(composer().contextWindowTokens).toBeUndefined()
  })
})

it.each(['plugins','schedule'] as const)('retains unsaved Word body and the same upstream instance across %s and return',async route=>{
  vi.stubGlobal('File',NodeFile)
  // Drain route preloads while their fixture modules and environment are alive.
  await Promise.all([import('./PluginMarketplaceView'),import('./schedule/ScheduleTasksView')])
  io.office.mockResolvedValue({ok:true,view:null})
  io.realDocument=true
  let view:DocxEditorView={editorId:'00000000-0000-4000-8000-000000000001',workspace,path:workspace+'/retained.docx',threadId:'a',objectId:'a'.repeat(64),revision:'b'.repeat(64),
    version:0,phase:'editing',dirty:false,content:{encoding:'base64',kind:'docx',byteLength:18,sha256:'b'.repeat(64),data:btoa('Core original body')},proposals:[],reviews:[]}
  io.docxEditor.mockImplementation(async request=>{
    if(request.action==='changed')view={...view,version:request.version,dirty:true}
    return {ok:true,view}
  })
  await act(async()=>useWorkspaceTabsStore.getState().openTab({id:workspaceObjectTabId(workspace,view.path),kind:'document',mode:'documents',title:'Word',workspaceRoot:workspace,path:view.path}))
  await settle()
  await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0))})
  const editor=element.querySelector<HTMLElement>('[data-synthetic-word-body]')!
  expect(editor).not.toBeNull();expect(editor.textContent).toBe('Core original body')
  await act(async()=>{editor.textContent='Unsaved edited Word body';editor.dispatchEvent(new Event('input',{bubbles:true}))})
  expect(editor.textContent).toBe('Unsaved edited Word body')
  expect(useDocxEditorStore.getState().views[view.editorId].dirty).toBe(true)
  await act(async()=>useChatStore.setState({route}))
  await settle()
  expect(editor.isConnected).toBe(true)
  if(route==='plugins')expect(editor.closest('[hidden]')).not.toBeNull()
  await act(async()=>useChatStore.setState({route:'chat'}))
  await settle()
  expect(element.querySelector('[data-synthetic-word-body]')).toBe(editor)
  expect(editor.textContent).toBe('Unsaved edited Word body')
  expect(element.querySelectorAll('[data-synthetic-word-body]')).toHaveLength(1)
  expect(io.wordMounts).toBe(1);expect(io.wordUnmounts).toBe(0)
})


describe('local Word real file-list entry',()=>{
  const fileName='local-word-entry.docx',filePath=workspace+'/'+fileName,localId='thr_local_word_entry'
  function configure(){
    io.realFileTree=true;io.realDocument=true;io.realPreview=true;vi.stubGlobal('File',NodeFile)
    io.directory.mockResolvedValue({ok:true,entries:[{name:fileName,path:filePath,type:'file',ext:'.docx'}]})
    const owner={...thread(localId),workspace,model:'',providerId:undefined,status:'idle',messageCount:0}
    io.provider.connect.mockResolvedValue(undefined);io.provider.createThread.mockResolvedValue(owner)
    io.provider.getThreadDetail.mockResolvedValue({thread:owner,blocks:[],latestSeq:0,threadStatus:'idle'})
    io.office.mockResolvedValue({ok:true,view:null})
    const view:DocxEditorView={editorId:'00000000-0000-4000-8000-000000000019',workspace,path:filePath,threadId:localId,objectId:'a'.repeat(64),revision:'b'.repeat(64),version:0,phase:'ready',dirty:false,content:{encoding:'base64',kind:'docx',byteLength:18,sha256:'b'.repeat(64),data:btoa('Core original body')},proposals:[],reviews:[]}
    io.docxEditor.mockImplementation(async request=>({ok:true,view:{...view,threadId:request.threadId || useChatStore.getState().activeThreadId}}))
    return view
  }
  async function openFromList(){
    await act(async()=>{
      useNativeReferenceStore.setState({references:[]})
      useChatStore.setState({activeThreadId:null,threads:[],blocks:[],busy:false,currentTurnId:null,runtimeConnection:'offline'})
      useWorkspaceTabsStore.getState().showSelector()
    })
    await act(async()=>element.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="files"]')!.click());await settle()
    const label=[...element.querySelectorAll('span')].find(node=>node.textContent===fileName)!
    expect(label).toBeTruthy()
    const row=label.closest<HTMLButtonElement>('button')!
    expect(row).toBeTruthy();await act(async()=>row.click());await settle()
  }
  it('opens through the real file list with no active thread and Core but no Provider, keeping the empty composer centered and AI blocked',async()=>{
    configure();await openFromList()
    expect(io.provider.createThread).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({workspace,localDocument:true}))
    expect(io.provider.createThread.mock.calls[0][0]).not.toHaveProperty('model');expect(io.provider.createThread.mock.calls[0][0]).not.toHaveProperty('providerId')
    expect(useChatStore.getState().activeThreadId).toBe(localId);expect(useChatStore.getState().runtimeConnection).toBe('offline')
    expect(io.docxEditor).toHaveBeenCalledExactlyOnceWith({action:'open',workspace,path:filePath,threadId:localId})
    expect(element.querySelector('[data-synthetic-word-body]')).not.toBeNull();expect(io.wordMounts).toBe(1)
    expect(element.querySelector('[data-analytix-conversation-body]')?.getAttribute('data-phase')).toBe('hero')
    expect(element.textContent).not.toContain('filePreviewUnsupported')
    await act(async()=>{composer().setInput('This model request must remain blocked')});await send()
    expect(io.provider.sendUserMessage).not.toHaveBeenCalled();expect(useChatStore.getState().runtimeConnection).toBe('offline')
  })
  it('keeps an open refusal in the Word error surface and retries the same file without text fallback',async()=>{
    configure();io.docxEditor.mockResolvedValue({ok:false,view:null,error:'forbidden'})
    await openFromList()
    expect(element.querySelector('[data-docx-open-error]')?.textContent).toContain('当前权限不允许')
    expect(element.textContent).not.toContain('filePreviewUnsupported');expect(io.wordMounts).toBe(0)
    expect(useWorkspaceTabsStore.getState().tabs.find(tab=>tab.path===filePath)).toMatchObject({mode:'documents'})
    const view=configure();io.docxEditor.mockResolvedValue({ok:true,view})
    const retry=[...element.querySelectorAll<HTMLButtonElement>('[data-docx-open-error] button')].find(button=>button.textContent==='重试')!
    await act(async()=>retry.click());await settle()
    expect(element.querySelector('[data-docx-open-error]')).toBeNull();expect(element.querySelector('[data-synthetic-word-body]')).not.toBeNull()
    expect(io.provider.createThread).toHaveBeenCalledOnce()
  })
  it('keeps a real Core connection failure recoverable on the Word surface without creating a fake owner',async()=>{
    configure();io.provider.connect.mockRejectedValue(new Error(JSON.stringify({code:'runtime_unavailable',message:'SYNTHETIC_CORE_UNAVAILABLE'})))
    await openFromList()
    expect(element.querySelector('[data-docx-open-error]')).not.toBeNull();expect(element.textContent).not.toContain('filePreviewUnsupported')
    expect(io.provider.createThread).not.toHaveBeenCalled();expect(io.docxEditor).not.toHaveBeenCalled()
    expect(useChatStore.getState().activeThreadId).toBeNull();expect(useChatStore.getState().route).toBe('chat');expect(io.provider.sendUserMessage).not.toHaveBeenCalled()
  })
})

it('extracts initial skill requests from direct input before native context assembly', async () => {
  await addAssets();
  io.read.mockResolvedValue({ok:true,content:'/injected-from-file'});
  await act(async () => { composer().setInput('/review Discuss selected objects'); useNativeReferenceStore.getState().add(reference()) });
  await send();
  expect(io.provider.sendUserMessage).toHaveBeenCalledTimes(1);
  expect(io.provider.sendUserMessage.mock.calls[0][1]).toContain('/injected-from-file');
  expect(io.provider.sendUserMessage.mock.calls[0][2]).toMatchObject({requestedSkillIds:['review']});
});


it.each([360, 1600])('opens one terminal workspace tab from selector and repeated shortcuts at width %s, and restores close focus', async width => {
  const viewport = Object.getOwnPropertyDescriptor(window, 'innerWidth')
  io.realWorkspaceChrome = true
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  try {
    await act(async () => window.dispatchEvent(new Event('resize')))
    await act(async () => useWorkspaceTabsStore.getState().showSelector())
    const terminalEntry = element.querySelector<HTMLButtonElement>('[data-sidebar-right-guide-entry="terminal"]')!
    await act(async () => terminalEntry.click())
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: false, activeTabId: 'tool:terminal' })
    for (let n = 0; n < 3; n++) {
      await act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', { detail: 'toggle-terminal' })))
    }
    expect(useWorkspaceTabsStore.getState().tabs.filter(tab => tab.mode === 'terminal')).toHaveLength(1)
    expect(element.querySelector('#workbench-terminal-toggle')).toBeNull()
    expect(element.querySelector('#workbench-terminal-panel')).toBeNull()
    element.querySelector<HTMLTextAreaElement>('[data-synthetic-terminal]')!.focus()
    await act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', { detail: 'close-tab' })))
    await act(async () => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: true, tabs: [] })
    expect(document.activeElement?.hasAttribute('data-sidebar-right-guide-entry')).toBe(true)
    await act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', { detail: 'toggle-terminal' })))
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: false, activeTabId: 'tool:terminal' })
    expect(element.querySelector('#workbench-right-workspace')?.getAttribute('data-auto-overlay')).toBe(width === 360 ? 'true' : null)
    const closeAll = document.querySelector<HTMLButtonElement>('[data-workbench-terminal-menu]')!
    closeAll.focus()
    await act(async () => closeAll.click())
    await act(async () => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
    expect(useWorkspaceTabsStore.getState().open).toBe(false)
    expect(document.activeElement?.id).toBe('workbench-workspace-expand')
  } finally {
    if (viewport) Object.defineProperty(window, 'innerWidth', viewport)
    else Reflect.deleteProperty(window, 'innerWidth')
  }
})

it.each(['plugins', 'settings'] as const)('suspends terminal presentation on the real %s route and ignores terminal shortcuts until return', async route => {
  await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: '`', ctrlKey: true, bubbles: true })))
  const terminal = element.querySelector<HTMLTextAreaElement>('[data-synthetic-terminal]')!
  expect(terminal.hidden).toBe(false)
  expect(useWorkspaceTabsStore.getState().activeTabId).toBe('tool:terminal')
  const focusRequest = terminal.getAttribute('data-focus-request')
  const tabs = useWorkspaceTabsStore.getState().tabs
  await act(async () => useChatStore.setState({ route }))
  expect(terminal.hidden).toBe(true)
  expect(document.querySelector('[data-workbench-terminal-menu]')).toBeNull()
  await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: '`', ctrlKey: true, bubbles: true })))
  await act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', { detail: 'toggle-terminal' })))
  expect(useChatStore.getState().route).toBe(route)
  expect(useWorkspaceTabsStore.getState().tabs).toBe(tabs)
  expect(terminal.getAttribute('data-focus-request')).toBe(focusRequest)
  await act(async () => useChatStore.setState({ route: 'chat' }))
  expect(terminal.hidden).toBe(false)
  expect(useWorkspaceTabsStore.getState().tabs).toBe(tabs)
})

it.each(['plugins', 'settings'] as const)('does not open a terminal workspace tab from shortcuts while initially on %s', async route => {
  await act(async () => useChatStore.setState({ route }))
  await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: '`', ctrlKey: true, bubbles: true })))
  await act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', { detail: 'toggle-terminal' })))
  expect(useChatStore.getState().route).toBe(route)
  expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: false, activeTabId: null, tabs: [] })
  expect(document.querySelector('[data-workbench-terminal-menu]')).toBeNull()
  await act(async () => useChatStore.setState({ route: 'chat' }))
  expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(0)
})

it('passes Ctrl+W through the real window capture handler to shell input and preserves Cmd+W and tab-strip close', async () => {
  io.realWorkspaceChrome = true
  const openTerminal = async () => act(async () => window.dispatchEvent(new CustomEvent('analytix:workspace-shortcut', { detail: 'toggle-terminal' })))
  await openTerminal()
  const input = element.querySelector<HTMLTextAreaElement>('.xterm-helper-textarea')!
  const phases: { phase: number; prevented: boolean }[] = []
  const shellInput = vi.fn()
  const capture = (event: KeyboardEvent) => {
    if (event.key === 'w') phases.push({ phase: event.eventPhase, prevented: event.defaultPrevented })
  }
  const terminalKey = (event: KeyboardEvent) => {
    phases.push({ phase: event.eventPhase, prevented: event.defaultPrevented })
    if (event.ctrlKey && !event.metaKey && !event.defaultPrevented) {
      shellInput()
      event.preventDefault()
    }
  }
  window.addEventListener('keydown', capture, true)
  input.addEventListener('keydown', terminalKey)
  try {
    input.focus()
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', ctrlKey: true, bubbles: true, cancelable: true })))
    expect(phases).toEqual([{ phase: Event.CAPTURING_PHASE, prevented: false }, { phase: Event.AT_TARGET, prevented: false }])
    expect(shellInput).toHaveBeenCalledTimes(1)
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, selectorOpen: false, activeTabId: 'tool:terminal' })
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(1)

    phases.length = 0
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true, bubbles: true, cancelable: true })))
    expect(phases).toEqual([{ phase: Event.CAPTURING_PHASE, prevented: true }, { phase: Event.AT_TARGET, prevented: true }])
    expect(shellInput).toHaveBeenCalledTimes(1)
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(0)

    await openTerminal()
    const tab = element.querySelector<HTMLButtonElement>('.workspace-tab [role="tab"]')!
    await act(async () => tab.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', ctrlKey: true, bubbles: true, cancelable: true })))
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(0)
  } finally {
    window.removeEventListener('keydown', capture, true)
    input.removeEventListener('keydown', terminalKey)
  }
})
