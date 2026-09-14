import { resolveWriteQuickActions } from '../../write/quick-actions'
import type { WorkspaceTab } from '../../store/workspace-tabs-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../../store/workspace-tabs-store'
import { isNativeOfficeFilePath } from '@shared/write-text-file'
import { NativeOfficePanel } from '../../office/NativeOfficePanel'
import { useNativeOfficeStore } from '../../office/native-office-store'
import { useEffect, useRef, useState, type ReactNode, type ReactElement } from 'react'
import { Files, Maximize2, Minimize2, PanelRightClose, RotateCcw, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useWriteWorkspaceStore, writeBasenameFromPath } from '../../write/write-workspace-store'
import { WriteWorkspaceView } from '../write/WriteWorkspaceView'
import './document-workspace.css'

export function DocumentWorkspacePanel({ threadId, activeTab, visible, input, setInput, onSubmitPrompt, focused, onToggleFocus, onCollapse, onOpenSettings, onFocusConversation, fileBrowser }: {
  fileBrowser: ReactNode
  threadId?: string | null
  activeTab?: WorkspaceTab | null
  visible: boolean
  input: string
  setInput: (value: string) => void
  onSubmitPrompt: (value: string) => void
  focused: boolean
  onToggleFocus: () => void
  onCollapse: () => void
  onOpenSettings: () => void
  onFocusConversation: () => void
}): ReactElement {
  const { t } = useTranslation('common')
  const selectionAssist = useWriteWorkspaceStore(state => state.selectionAssist)
  const quickActions = resolveWriteQuickActions(selectionAssist.quickActions, t)
  const nativeTarget = useNativeOfficeStore((s) => s.target)
  const nativeView = useNativeOfficeStore((s) => s.view)
  const activeFilePath = useWriteWorkspaceStore((state) => state.activeFilePath)
  const workspaceRoot = useWriteWorkspaceStore((state) => state.workspaceRoot)
  const filesButton = useRef<HTMLButtonElement>(null)
  const fileActionsSelect = useRef<HTMLSelectElement>(null)
  const [fileActionError, setFileActionError] = useState(false)
  const [filesOpen, setFilesOpen] = useState(!activeFilePath)
  const [recentlyClosed, setRecentlyClosed] = useState<{ root: string; path: string } | null>(null)
  useEffect(() => { if (activeFilePath) setFilesOpen(false) }, [activeFilePath])
  const displayedPath = activeTab?.path ?? (activeTab ? null : nativeTarget?.path || nativeView?.path || activeFilePath)
  const nativeActive = !!displayedPath && /\.(docx|xlsx|pptx)$/i.test(displayedPath)
  useEffect(() => {
    if (!visible || !activeTab?.path || !activeTab.workspaceRoot) return
    if (isNativeOfficeFilePath(activeTab.path)) void useNativeOfficeStore.getState().select(activeTab.workspaceRoot, activeTab.path)
    else if (useWriteWorkspaceStore.getState().activeFilePath !== activeTab.path) void useWriteWorkspaceStore.getState().openFile(activeTab.workspaceRoot, activeTab.path)
  }, [activeTab?.id, activeTab?.path, activeTab?.workspaceRoot, visible])
  useEffect(() => { if (nativeTarget) setFilesOpen(false) }, [nativeTarget])
  const closeDocument = async (): Promise<void> => {
    if (nativeActive && nativeTarget) {
      if (!(await useNativeOfficeStore.getState().close(nativeTarget.workspace, nativeTarget.path))) return
      useWorkspaceTabsStore.getState().closeTab(workspaceObjectTabId(nativeTarget.workspace, nativeTarget.path))
      setFilesOpen(true)
      return
    }
    if (!activeFilePath) return
    if (!(await useWriteWorkspaceStore.getState().openWorkspaceHome(workspaceRoot))) return
    if (activeTab) useWorkspaceTabsStore.getState().closeTab(activeTab.id)
    setRecentlyClosed({ root: workspaceRoot, path: activeFilePath })
    setFilesOpen(true)
  }
  useEffect(() => { setFileActionError(false) }, [displayedPath])
  const fileActions = <>
    <select ref={fileActionsSelect} aria-label="打开" className="max-w-32 rounded bg-transparent px-1 py-1 text-xs" value="" onChange={event => {
      const action = event.target.value
      if (action === 'browse') { setFilesOpen(value => !value); return }
      if (!displayedPath || (action !== 'system' && action !== 'file-manager')) return
      setFileActionError(false)
      void window.analytix.workspace.openEditorPath({workspaceRoot:activeTab?.workspaceRoot || nativeTarget?.workspace || workspaceRoot,path:displayedPath,editorId:action})
        .then(result => { if (!result.ok) setFileActionError(true) })
        .catch(() => setFileActionError(true))
    }}>
      <option value="" disabled>打开</option>
      <option value="system">用默认应用打开</option>
      <option value="file-manager">在文件夹中显示</option>
      <option value="browse">浏览工作区文件</option>
    </select>
    {fileActionError ? <span role="alert" className="text-xs text-ds-muted">打开失败，请重试。</span> : null}
  </>
  return (
    <section className="document-workspace flex h-full min-h-0 min-w-0 flex-col bg-ds-card" inert={!visible} aria-label={t('workbenchDocuments')} onKeyDown={(event) => {
        if (!event.nativeEvent.isComposing && event.key === 'Escape' && filesOpen) {
          event.preventDefault()
          event.stopPropagation()
          setFilesOpen(false)
          if (nativeActive) fileActionsSelect.current?.focus()
          else filesButton.current?.focus()
        }
      }}>
      {!nativeActive ? <header className="document-workspace-header flex shrink-0 items-center gap-2 border-b border-ds-border-muted px-3">
        <button ref={filesButton} type="button" className="ds-toolbar-icon-button" aria-label={t('rightPanelFiles')} title={t('rightPanelFiles')} aria-expanded={filesOpen} aria-pressed={filesOpen} onClick={() => setFilesOpen(!filesOpen)}><Files className="h-4 w-4" /></button>
        <span title={displayedPath ? writeBasenameFromPath(displayedPath) : undefined} className="min-w-0 flex-1 truncate text-sm font-medium">{displayedPath ? writeBasenameFromPath(displayedPath) : t('workbenchDocuments')}</span>
        {displayedPath ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchCloseDocument')} title={t('workbenchCloseDocument')} onClick={() => void closeDocument()}><X className="h-4 w-4" /></button> : null}
        {recentlyClosed?.root === workspaceRoot ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchReopenDocument')} title={t('workbenchReopenDocument')} onClick={() => void useWriteWorkspaceStore.getState().openFile(recentlyClosed.root, recentlyClosed.path)}><RotateCcw className="h-4 w-4" /></button> : null}
        <button type="button" className="ds-toolbar-icon-button document-workspace-expand" aria-label={t(focused ? 'workbenchDock' : 'workbenchFocus')} title={t(focused ? 'workbenchDock' : 'workbenchFocus')} aria-pressed={focused} onClick={onToggleFocus}><span className="document-workspace-focus-icons" data-focused={focused} aria-hidden="true"><Maximize2 className="h-4 w-4" /><Minimize2 className="h-4 w-4" /></span></button>
        <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchCollapse')} title={t('workbenchCollapse')} onClick={() => { onCollapse(); onFocusConversation() }}><PanelRightClose className="h-4 w-4" /></button>
      </header> : null}
      <div className="relative flex min-h-0 flex-1">
        {filesOpen && visible ? <div className="document-workspace-files relative w-52 shrink-0 border-r border-ds-border bg-ds-card">{fileBrowser}</div> : null}
        {nativeActive ? <NativeOfficePanel fileActions={fileActions} quickActions={quickActions} threadId={threadId} visible={visible} onFocusConversation={onFocusConversation} setInput={(text) => setInput(input ? `${input}\n${text}` : text)} /> : <WriteWorkspaceView leftSidebarCollapsed={false} input={input} setInput={setInput} onSubmitPrompt={onSubmitPrompt} onOpenAgentSettings={onOpenSettings} onFocusConversation={onFocusConversation} />}
      </div>
    </section>
  )
}
