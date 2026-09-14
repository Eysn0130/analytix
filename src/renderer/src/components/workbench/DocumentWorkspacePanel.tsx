import { NativeOfficePanel } from '../../office/NativeOfficePanel'
import { useNativeOfficeStore } from '../../office/native-office-store'
import { useEffect, useRef, useState, type ReactElement } from 'react'
import { FileText, Files, Maximize2, Minimize2, PanelRightClose, Presentation, RotateCcw, Sheet, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useWriteWorkspaceStore, writeBasenameFromPath } from '../../write/write-workspace-store'
import { WriteSidebar } from '../write/WriteSidebar'
import { WriteWorkspaceView } from '../write/WriteWorkspaceView'
import './document-workspace.css'

export function DocumentWorkspacePanel({ visible, input, setInput, onSubmitPrompt, focused, onToggleFocus, onCollapse, onOpenSettings, onFocusConversation }: {
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
  const nativeTarget = useNativeOfficeStore((s) => s.target)
  const nativeView = useNativeOfficeStore((s) => s.view)
  const activeFilePath = useWriteWorkspaceStore((state) => state.activeFilePath)
  const workspaceRoot = useWriteWorkspaceStore((state) => state.workspaceRoot)
  const filesButton = useRef<HTMLButtonElement>(null)
  const [filesOpen, setFilesOpen] = useState(!activeFilePath)
  const [recentlyClosed, setRecentlyClosed] = useState<{ root: string; path: string } | null>(null)
  useEffect(() => { if (activeFilePath) setFilesOpen(false) }, [activeFilePath])
  const displayedPath = nativeTarget?.path || nativeView?.path || activeFilePath
  useEffect(() => { if (nativeTarget) setFilesOpen(false) }, [nativeTarget])
  const closeDocument = async (): Promise<void> => {
    if (nativeTarget || nativeView) {
      const result = await window.analytix.office.request({ action: 'close' })
      if (!result.ok) { useNativeOfficeStore.setState({ error: result.error ?? 'unavailable', view: result.view }); return }
      useNativeOfficeStore.setState({ target: null, view: null, error: null })
      setFilesOpen(true)
      return
    }
    if (!activeFilePath) return
    if (!(await useWriteWorkspaceStore.getState().openWorkspaceHome(workspaceRoot))) return
    setRecentlyClosed({ root: workspaceRoot, path: activeFilePath })
    setFilesOpen(true)
  }
  const nativeKind = displayedPath?.split('.').at(-1)?.toLowerCase() ?? nativeView?.kind
  const FileIcon = nativeKind === 'xlsx' ? Sheet : nativeKind === 'pptx' ? Presentation : FileText
  const fileColor = nativeKind === 'xlsx' ? 'text-emerald-600 dark:text-emerald-400' : nativeKind === 'pptx' ? 'text-orange-600 dark:text-orange-400' : 'text-blue-600 dark:text-blue-400'
  return (
    <section className="document-workspace flex h-full min-h-0 min-w-0 flex-col bg-ds-card" inert={!visible} aria-label={t('workbenchDocuments')} onKeyDown={(event) => {
        if (event.key === 'Escape' && filesOpen) {
          event.preventDefault()
          event.stopPropagation()
          setFilesOpen(false)
          filesButton.current?.focus()
        }
      }}>
      <header className="document-workspace-header flex shrink-0 items-center gap-2 border-b border-ds-border-muted px-3">
        <button ref={filesButton} type="button" className="ds-toolbar-icon-button" aria-label={t('rightPanelFiles')} title={t('rightPanelFiles')} aria-expanded={filesOpen} aria-pressed={filesOpen} onClick={() => setFilesOpen(!filesOpen)}><Files className="h-4 w-4" /></button>
        {nativeTarget || nativeView ? <span className={`document-workspace-kind ${fileColor}`}><FileIcon className="h-4 w-4" aria-hidden="true" /></span> : null}
        <span title={displayedPath ? writeBasenameFromPath(displayedPath) : undefined} className="min-w-0 flex-1 truncate text-sm font-medium">{displayedPath ? writeBasenameFromPath(displayedPath) : t('workbenchDocuments')}</span>
        {displayedPath ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchCloseDocument')} title={t('workbenchCloseDocument')} onClick={() => void closeDocument()}><X className="h-4 w-4" /></button> : null}
        {recentlyClosed?.root === workspaceRoot ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchReopenDocument')} title={t('workbenchReopenDocument')} onClick={() => void useWriteWorkspaceStore.getState().openFile(recentlyClosed.root, recentlyClosed.path)}><RotateCcw className="h-4 w-4" /></button> : null}
        <button type="button" className="ds-toolbar-icon-button document-workspace-expand" aria-label={t(focused ? 'workbenchDock' : 'workbenchFocus')} title={t(focused ? 'workbenchDock' : 'workbenchFocus')} aria-pressed={focused} onClick={onToggleFocus}><span className="document-workspace-focus-icons" data-focused={focused} aria-hidden="true"><Maximize2 className="h-4 w-4" /><Minimize2 className="h-4 w-4" /></span></button>
        <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchCollapse')} title={t('workbenchCollapse')} onClick={() => { onCollapse(); onFocusConversation() }}><PanelRightClose className="h-4 w-4" /></button>
      </header>
      <div className="relative flex min-h-0 flex-1">
        {filesOpen ? <div className="document-workspace-files absolute inset-y-0 left-0 z-20 w-60 border-r border-ds-border bg-ds-card"><WriteSidebar /></div> : null}
        {nativeTarget || nativeView ? <NativeOfficePanel visible={visible && !filesOpen} /> : <WriteWorkspaceView leftSidebarCollapsed={false} input={input} setInput={setInput} onSubmitPrompt={onSubmitPrompt} onOpenAgentSettings={onOpenSettings} onFocusConversation={onFocusConversation} />}
      </div>
    </section>
  )
}
