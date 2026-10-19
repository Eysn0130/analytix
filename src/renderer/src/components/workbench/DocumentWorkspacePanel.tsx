import { useDocxEditorStore,isDocxPath,docxOpenKey } from '../../office/docx-editor-store'
import { useChatStore } from '../../store/chat-store'
import { DocxEditorPanel } from '../../office/DocxEditorPanel'
import { lazy, Suspense, useCallback, useEffect, useRef, useState, type ReactNode, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { isNativeOfficeFilePath } from '@shared/write-text-file'
import type { WorkspaceTab } from '../../store/workspace-tabs-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../../store/workspace-tabs-store'
import { NativeOfficePanel } from '../../office/NativeOfficePanel'
import type { NativeReference } from '../../office/native-reference-store'
import { useNativeOfficeStore } from '../../office/native-office-store'
import { useWriteWorkspaceStore, writeWorkspaceHasUnsettledDraft } from '../../write/write-workspace-store'
import { resolveWriteQuickActions } from '../../write/quick-actions'
import { WriteMarkdownEditor, type WriteMarkdownEditorHandle } from '../write/WriteMarkdownEditor'
import { WriteConflictReview } from '../write/WriteConflictReview'
import { Files, FolderOpen, ExternalLink, Info, Loader2, Download } from '../../design/AnalytixUiIcons'
import { WorkspaceViewerIconButton, WorkspaceViewerMenu, type WorkspaceViewerControls } from './WorkspaceViewerHeader'
import { ActionMenuItem } from '../common/ActionMenu'
import { Tooltip } from '../chat/presentation/Tooltip'
import './document-workspace.css'

const FilePreview = lazy(() => import('../WorkspaceFilePreviewPanel').then(module => ({default:module.WorkspaceFilePreviewPanel})))

/** Recovery only for an already retained text draft; this never creates a document. */
function LegacyDocumentRecovery({root,path,visible}: {root:string;path:string;visible:boolean}): ReactElement {
  const { t } = useTranslation('common')
  const state = useWriteWorkspaceStore()
  const editor = useRef<WriteMarkdownEditorHandle | null>(null)
  const mounted = useRef(true), shown = useRef(visible); shown.current = visible
  const inFlight = useRef(false)
  const [busy,setBusy] = useState(false)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  const owns = useCallback(() => {
    const current = useWriteWorkspaceStore.getState()
    return mounted.current && shown.current && current.activeFilePath === path &&
      workspaceObjectTabId(current.workspaceRoot,path) === workspaceObjectTabId(root,path)
  }, [root,path])
  const act = async (action: 'save' | 'discard'): Promise<void> => {
    const before = useWriteWorkspaceStore.getState()
    if (!owns() || inFlight.current || before.shutdownFrozen || before.exportInProgress || before.saveStatus === 'saving') return
    if (action === 'save' && (before.reviewActive || before.pendingAgentReview || before.fileTruncated ||
      before.saveStatus === 'conflict' || before.saveStatus === 'unknown' && !before.pendingSave)) return
    if (action === 'discard' && (before.pendingSave || before.saveStatus === 'unknown')) return
    inFlight.current = true; setBusy(true)
    try {
      if (action === 'save') await before.flushSave(root)
      else if (await before.syncActiveFileFromDisk(root,{path,force:true,animate:false,expectedContent:before.fileContent}) && owns()) {
        useWriteWorkspaceStore.getState().clearPendingAgentReview()
        useWriteWorkspaceStore.getState().setReviewActive(false)
      }
    } finally { inFlight.current = false; if (mounted.current) setBusy(false) }
  }
  useEffect(() => {
    if (!visible || !state.pendingAgentReview || !editor.current || !owns()) return
    const pending = state.pendingAgentReview, current = useWriteWorkspaceStore.getState()
    const started = editor.current.beginDiffReview({original:current.fileContent,nextDoc:pending.nextContent})
    if (started || current.fileContent === pending.nextContent) current.clearPendingAgentReview()
  }, [visible,state.pendingAgentReview,root,path,owns])
  const review = !!(state.reviewActive || state.reviewRecovery || state.pendingAgentReview)
  return <section data-legacy-document-recovery className="flex h-full min-h-0 flex-col gap-3 p-4" aria-label={t('workspaceLegacyDraftTitle')}>
    <p className="text-sm">{t('workspaceLegacyDraftDescription')}</p>
    <p className="break-all text-xs text-ds-muted">{path}</p>
    <div className="flex flex-wrap items-center gap-3" role="toolbar" aria-label={t('workspaceLegacyDraftTitle')}>
      <button type="button" disabled={busy || state.shutdownFrozen || state.exportInProgress || state.saveStatus === 'saving' || review || state.fileTruncated || state.saveStatus === 'conflict' || state.saveStatus === 'unknown' && !state.pendingSave}
        onClick={() => void act('save')}>{t(state.pendingSave ? 'workspaceLegacyDraftQuery' : 'save')}</button>
      <button type="button" disabled={busy || state.shutdownFrozen || state.exportInProgress || !!state.pendingSave || ['saving','unknown'].includes(state.saveStatus)}
        onClick={() => void act('discard')}>{t('workspaceLegacyDraftDiscard')}</button>
      <span role="status">{state.saveStatus}</span>
    </div>
    {state.fileError ? <p role="alert" className="text-xs text-ds-muted">{state.fileError}</p> : null}
    <WriteConflictReview />
    {review ? <WriteMarkdownEditor value={state.fileContent} workspaceRoot={root} filePath={path}
      appearance="source" livePreviewEnabled={false} readOnly={busy || !!state.pendingSave || state.fileTruncated || state.shutdownFrozen}
      completionModel="" completionEnabled={false} completionDebounceMs={0} completionMinAcceptScore={1}
      completionLongEnabled={false} completionLongDebounceMs={0} completionLongMinAcceptScore={1}
      handleRef={editor} reviewRecovery={state.reviewRecovery} onReviewSuspend={state.suspendReview}
      onReviewStateChange={active => { if (owns()) useWriteWorkspaceStore.getState().setReviewActive(active) }}
      onChange={value => { if (owns() && !inFlight.current) useWriteWorkspaceStore.getState().setFileContent(value) }}
      onSelectionChange={() => undefined} onSaveShortcut={() => { void act('save') }} />
      : <textarea aria-label={t('writeCurrentDraft')} className="min-h-0 flex-1 resize-none bg-transparent font-mono text-sm" readOnly value={state.fileContent} />}
  </section>
}

export function DocumentWorkspacePanel({threadId,activeTab,visible,onSubmitPrompt,onPrepareDraft,onFocusConversation,fileBrowser,viewerControls}: {
  fileBrowser:ReactNode;threadId?:string | null;activeTab?:WorkspaceTab | null;visible:boolean;viewerControls?:WorkspaceViewerControls
  onSubmitPrompt:(value:string,references?:NativeReference[]) => void
  onPrepareDraft?:(value:string,references:NativeReference[]) => boolean
  onFocusConversation:() => void
}): ReactElement {
  const { t } = useTranslation('common')
  const selectionAssist = useWriteWorkspaceStore(state => state.selectionAssist)
  const quickActions = resolveWriteQuickActions(selectionAssist.quickActions,t)
  const nativeTarget = useNativeOfficeStore(state => state.target)
  const activeFilePath = useWriteWorkspaceStore(state => state.activeFilePath)
  const workspaceRoot = useWriteWorkspaceStore(state => state.workspaceRoot)
  const unsettled = useWriteWorkspaceStore(writeWorkspaceHasUnsettledDraft)
  const mounted = useRef(true)
  const saveSequence = useRef(0), savePending = useRef<number | null>(null)
  const [saveState,setSaveState] = useState<{owner:string;sequence:number;phase:'saving' | 'saved' | 'canceled' | 'unavailable'} | null>(null)
  const fileActionsSelect = useRef<HTMLButtonElement>(null)
  const [fileActionError,setFileActionError] = useState(false)
  const [filesOpen,setFilesOpen] = useState(false)
  const displayedPath = activeTab ? activeTab.path ?? null : nativeTarget?.path ?? null
  const protectedPreview = activeTab?.preview === 'source' || activeTab?.protectedRead !== undefined || activeTab?.protectedPresent !== undefined
  const docxActive=!!displayedPath && !protectedPreview && isDocxPath(displayedPath)
  const shellVisible=useChatStore(state=>state.route!=='settings')
  const docxViews=useDocxEditorStore(state=>state.views)
  const [docxAttempt,setDocxAttempt]=useState(0)
  const [docxOpen,setDocxOpen]=useState<{owner:string;phase:'loading'|'error';message?:string}|null>(null)
  const docxOpening=useRef(false),docxSequence=useRef(0),lastDocxThread=useRef(threadId),expectedDocxThread=useRef<{threadId:string|null}|null>(null)
  const nativeActive = !docxActive && !!displayedPath && !protectedPreview && isNativeOfficeFilePath(displayedPath)
  const displayedRoot = activeTab?.workspaceRoot || (nativeActive ? nativeTarget?.workspace : null) || workspaceRoot
  const fileOwner = JSON.stringify([displayedRoot, displayedPath])
  const currentOwner = useRef({ owner: fileOwner, visible }); currentOwner.current = { owner: fileOwner, visible }
  const savePhase = saveState?.owner === fileOwner && saveState.sequence === saveSequence.current ? saveState.phase : null
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])
  useEffect(() => {
    setFileActionError(false)
    return () => { saveSequence.current++; savePending.current = null }
  }, [fileOwner, visible])
  const ownerIsCurrent = (owner: string): boolean => mounted.current && currentOwner.current.visible && currentOwner.current.owner === owner
  const openFile = (editorId: 'system' | 'file-manager'): void => {
    if (!displayedPath || !ownerIsCurrent(fileOwner)) return
    setFileActionError(false)
    void window.analytix.workspace.openEditorPath({ workspaceRoot: displayedRoot, path: displayedPath, editorId })
      .then(result => { if (ownerIsCurrent(fileOwner) && !result.ok) setFileActionError(true) })
      .catch(() => { if (ownerIsCurrent(fileOwner)) setFileActionError(true) })
  }
  const downloadFile = async (): Promise<void> => {
    if (docxActive || !displayedPath || !ownerIsCurrent(fileOwner) || savePending.current !== null) return
    const sequence = ++saveSequence.current
    savePending.current = sequence
    setSaveState({ owner: fileOwner, sequence, phase: 'saving' })
    const current = (): boolean => ownerIsCurrent(fileOwner) && saveSequence.current === sequence && savePending.current === sequence
    try {
      const result = await window.analytix.files.saveAs({ sourcePath: displayedPath, workspaceRoot: displayedRoot })
      const phase = result?.ok === true && result.path.trim() ? 'saved'
        : result?.ok === false && result.canceled === true ? 'canceled' : 'unavailable'
      if (current()) setSaveState({ owner: fileOwner, sequence, phase })
    } catch {
      if (current()) setSaveState({ owner: fileOwner, sequence, phase: 'unavailable' })
    } finally { if (savePending.current === sequence) savePending.current = null }
  }
  const downloadButton = (download:()=>void) => <WorkspaceViewerIconButton data-native-office-download shape="pill" label={t('generatedFileDownload')}
    disabled={!displayedPath || savePhase === 'saving'} aria-busy={savePhase === 'saving'} onClick={download}>
    {savePhase === 'saving' ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Download aria-hidden="true" />}
  </WorkspaceViewerIconButton>
  const downloadMessage = savePhase ? t(savePhase === 'saving' ? 'generatedFileSaving' : savePhase === 'saved' ? 'generatedFileSaved'
    : savePhase === 'canceled' ? 'userInputCancelled' : 'generatedFileSaveFailed') : null
  const downloadStatus = downloadMessage ? <Tooltip label={downloadMessage} side="bottom" portal>
    <span role="status" tabIndex={0} data-file-download-status data-native-office-file-status className="document-workspace-status">
      <Info aria-hidden="true" /><span className="sr-only">{downloadMessage}</span>
    </span></Tooltip> : null
  const renderFileTitle = (beforeOpen?:(open:()=>void)=>void,exportCurrent?:()=>void) => <WorkspaceViewerMenu label="打开" title={displayedPath?.split('/').at(-1) || t('filePreviewTitle')}
      triggerRef={fileActionsSelect} ownerKey={fileOwner} enabled={visible}>
      <ActionMenuItem icon={<ExternalLink />} label="用默认应用打开" onClick={()=>{
        if(beforeOpen)beforeOpen(()=>openFile('system'));else openFile('system')
      }} />
      <ActionMenuItem icon={<FolderOpen />} label="在文件夹中显示" onClick={()=>{
        if(beforeOpen)beforeOpen(()=>openFile('file-manager'));else openFile('file-manager')
      }} />
      <ActionMenuItem icon={<Files />} label="浏览工作区文件" onClick={()=>setFilesOpen(value=>!value)} />
      <ActionMenuItem icon={<Download />} label={t('generatedFileDownload')} disabled={!displayedPath || savePhase === 'saving'}
        onClick={exportCurrent ?? (()=>{void downloadFile()})} />
    </WorkspaceViewerMenu>
  const renderFileCommands = (exportCurrent?:()=>void) => <>
    {downloadButton(exportCurrent ?? (()=>{void downloadFile()}))}
    {downloadStatus}
    {fileActionError ? <Tooltip label="打开失败，请重试。" side="bottom" portal><span role="alert" tabIndex={0}
      data-native-office-file-status className="document-workspace-status"><Info aria-hidden="true" /><span className="sr-only">打开失败，请重试。</span></span></Tooltip> : null}
  </>
  const renderFileActions = (beforeOpen?:(open:()=>void)=>void,exportCurrent?:()=>void) => <>
    {renderFileTitle(beforeOpen,exportCurrent)}{renderFileCommands(exportCurrent)}
  </>
  // Tab selection precedes async native hide/select; old captures stay hidden.
  const nativeTabReady = !activeTab || !!(nativeTarget && activeTab.path && activeTab.workspaceRoot &&
    workspaceObjectTabId(activeTab.workspaceRoot,activeTab.path) === workspaceObjectTabId(nativeTarget.workspace,nativeTarget.path))
  useEffect(() => {
    if (!visible) return
    if (!displayedPath) { useWorkspaceTabsStore.getState().showSelector(); return }
    if (!nativeActive || !displayedRoot) return
    let current = true
    void useNativeOfficeStore.getState().select(displayedRoot,displayedPath,() =>
      current && mounted.current && currentOwner.current.visible && currentOwner.current.owner === fileOwner)
    return () => { current = false }
  }, [activeTab?.id,displayedPath,displayedRoot,fileOwner,nativeActive,visible])
  useEffect(()=>{
    if (!visible || !shellVisible || !docxActive || !displayedRoot || !displayedPath) return
    let live=true
    const sequence=++docxSequence.current,tabId=useWorkspaceTabsStore.getState().activeTabId,route=useChatStore.getState().route
    const expected={threadId:useChatStore.getState().activeThreadId};expectedDocxThread.current=expected
    const current=()=>{
      const tabs=useWorkspaceTabsStore.getState(),tab=tabs.tabs.find(item=>item.id===tabs.activeTabId)
      return live && mounted.current && ownerIsCurrent(fileOwner) && useChatStore.getState().route===route &&
        useChatStore.getState().activeThreadId===expected.threadId &&
        tabs.activeTabId===tabId && (!tab || JSON.stringify([tab.workspaceRoot,tab.path])===fileOwner)
    }
    docxOpening.current=true;setDocxOpen({owner:fileOwner,phase:'loading'})
    void useDocxEditorStore.getState().select(displayedRoot,displayedPath,expected.threadId ?? undefined,current,threadId=>{if(live && docxSequence.current===sequence)expected.threadId=threadId})
      .then(selected=>{
        if (!current()) return
        if (selected) {setDocxOpen(null);return}
        const code=useDocxEditorStore.getState().openErrors[docxOpenKey(displayedRoot,displayedPath)]
        const messages:Record<string,string>={forbidden:'当前权限不允许打开此 Word 文档。',unsaved_changes:'此文档仍由另一会话的工作副本持有，请先保存或关闭该副本。',
          package_changed:'Documents 插件已更新，请检查当前插件状态后重试。',unavailable:'Word 文档暂不可用，请检查本地运行时及 Documents 插件状态后重试。'}
        setDocxOpen({owner:fileOwner,phase:'error',message:code?messages[code] || code:'文档会话已变化，请重试。'})
      }).catch(()=>{if(current())setDocxOpen({owner:fileOwner,phase:'error',message:'Word 文档暂不可用，请重试。'})})
      .finally(()=>{if(docxSequence.current===sequence)docxOpening.current=false})
    return()=>{live=false}
  },[visible,shellVisible,docxActive,displayedRoot,displayedPath,fileOwner,docxAttempt])
  useEffect(()=>{
    if (lastDocxThread.current===threadId) return
    lastDocxThread.current=threadId
    // Our own materialization changes the chat owner before open completes.
    // A later user selection starts a new, separately fenced open attempt.
    if (!docxOpening.current || threadId!==expectedDocxThread.current?.threadId) setDocxAttempt(value=>value+1)
  },[threadId])
  useEffect(() => { if (nativeTarget) setFilesOpen(false) }, [nativeTarget])
  const closeLegacy = async (): Promise<void> => {
    if (!ownerIsCurrent(fileOwner) || !activeTab) return
    const owner = useWriteWorkspaceStore.getState()
    if (owner.activeFilePath === displayedPath && workspaceObjectTabId(owner.workspaceRoot,displayedPath ?? '') === workspaceObjectTabId(displayedRoot,displayedPath ?? '')) {
      if (writeWorkspaceHasUnsettledDraft(owner) || !(await owner.openWorkspaceHome(owner.workspaceRoot))) return
    }
    if (ownerIsCurrent(fileOwner)) useWorkspaceTabsStore.getState().closeTab(activeTab.id)
  }
  const retainedDraft = !!(!protectedPreview && displayedPath && displayedPath === activeFilePath && unsettled &&
    workspaceObjectTabId(workspaceRoot,displayedPath) === workspaceObjectTabId(displayedRoot,displayedPath))
  return <section className="document-workspace flex h-full min-h-0 min-w-0 flex-col bg-ds-card" inert={!visible} aria-label={t('workbenchDocuments')}
    onKeyDown={event => {
      if (!event.nativeEvent.isComposing && event.key === 'Escape' && filesOpen) {
        event.preventDefault();event.stopPropagation();setFilesOpen(false);fileActionsSelect.current?.focus()
      }
    }}>
    {Object.keys(docxViews).length ? <div className="relative flex min-h-0 flex-1" style={docxActive?undefined:{display:'none'}}>
      {filesOpen && visible && docxActive ? <div className="document-workspace-files relative w-52 shrink-0 border-r border-ds-border bg-ds-card">{fileBrowser}</div> : null}
      <div className="min-h-0 min-w-0 flex-1">{Object.values(docxViews).map(view=>{
        const viewVisible=visible && shellVisible && docxActive && view.threadId===threadId && view.workspace===displayedRoot && view.path===displayedPath
        return <DocxEditorPanel key={view.editorId} view={view} visible={viewVisible}
          fileActions={viewVisible?renderFileActions:undefined} viewerControls={viewVisible?viewerControls:undefined} onSubmitPrompt={onSubmitPrompt} onPrepareDraft={onPrepareDraft}/>
      })}</div>
    </div>:null}
    {docxActive && docxOpen?.owner===fileOwner ? docxOpen.phase==='loading'
      ? <p role="status" data-docx-opening>正在打开 Word 文档…</p>
      : <div role="alert" data-docx-open-error><p>{docxOpen.message}</p>
        <button type="button" onClick={()=>setDocxAttempt(value=>value+1)}>重试</button>
        <button type="button" onClick={()=>useChatStore.getState().setRoute('plugins')}>检查 Documents 插件</button></div>:null}
    {docxActive || !displayedPath || !nativeActive && !visible ? null : nativeActive ? <div className="relative flex min-h-0 flex-1">
      {filesOpen && visible ? <div className="document-workspace-files relative w-52 shrink-0 border-r border-ds-border bg-ds-card">{fileBrowser}</div> : null}
      <NativeOfficePanel viewerControls={viewerControls} titleControl={renderFileTitle()} fileActions={renderFileCommands()} quickActions={quickActions} threadId={threadId} visible={visible && nativeTabReady}
        onFocusConversation={onFocusConversation} onSubmitPrompt={onSubmitPrompt} onPrepareDraft={onPrepareDraft} />
    </div> : retainedDraft ? <LegacyDocumentRecovery key={fileOwner} root={displayedRoot} path={displayedPath} visible={visible} />
      : <Suspense fallback={<p role="status">{t('loading')}</p>}><FilePreview target={{path:displayedPath,workspaceRoot:displayedRoot,preview:activeTab?.preview === 'source' ? 'source' : undefined,protectedRead:activeTab?.protectedRead,protectedPresent:activeTab?.protectedPresent,line:activeTab?.sourceLine}}
        workspaceRoot={displayedRoot} visible={visible} viewerControls={visible ? viewerControls : undefined} tabbedWorkspace className="h-full min-h-0" onClose={() => { void closeLegacy() }} /></Suspense>}
  </section>
}
