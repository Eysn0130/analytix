import { useEffect, useRef, useState, type ReactElement } from 'react'
import { Files, Maximize2, Minimize2, PanelRightClose, RotateCcw, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useWriteWorkspaceStore, writeBasenameFromPath } from '../../write/write-workspace-store'
import { WriteSidebar } from '../write/WriteSidebar'
import { WriteWorkspaceView } from '../write/WriteWorkspaceView'

export function DocumentWorkspacePanel({ input, setInput, onSubmitPrompt, focused, onToggleFocus, onCollapse, onOpenSettings, onFocusConversation }: {
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
  const activeFilePath = useWriteWorkspaceStore((state) => state.activeFilePath)
  const workspaceRoot = useWriteWorkspaceStore((state) => state.workspaceRoot)
  const filesButton = useRef<HTMLButtonElement>(null)
  const [filesOpen, setFilesOpen] = useState(!activeFilePath)
  const [recentlyClosed, setRecentlyClosed] = useState<{ root: string; path: string } | null>(null)
  useEffect(() => { if (activeFilePath) setFilesOpen(false) }, [activeFilePath])
  const closeDocument = async (): Promise<void> => {
    if (!activeFilePath) return
    if (!(await useWriteWorkspaceStore.getState().openWorkspaceHome(workspaceRoot))) return
    setRecentlyClosed({ root: workspaceRoot, path: activeFilePath })
    setFilesOpen(true)
  }
  const FocusIcon = focused ? Minimize2 : Maximize2
  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col bg-ds-card" aria-label={t('workbenchDocuments')} onKeyDown={(event) => {
        if (event.key === 'Escape' && filesOpen) {
          event.preventDefault()
          event.stopPropagation()
          setFilesOpen(false)
          filesButton.current?.focus()
        }
      }}>
      <header className="flex h-11 shrink-0 items-center gap-2 border-b border-ds-border-muted px-3">
        <button ref={filesButton} type="button" className="ds-toolbar-icon-button" aria-label={t('rightPanelFiles')} aria-expanded={filesOpen} onClick={() => setFilesOpen(!filesOpen)}><Files className="h-4 w-4" /></button>
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{activeFilePath ? writeBasenameFromPath(activeFilePath) : t('workbenchDocuments')}</span>
        {activeFilePath ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchCloseDocument')} onClick={() => void closeDocument()}><X className="h-4 w-4" /></button> : null}
        {recentlyClosed?.root === workspaceRoot ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchReopenDocument')} onClick={() => void useWriteWorkspaceStore.getState().openFile(recentlyClosed.root, recentlyClosed.path)}><RotateCcw className="h-4 w-4" /></button> : null}
        <button type="button" className="ds-toolbar-icon-button" aria-label={t(focused ? 'workbenchDock' : 'workbenchFocus')} onClick={onToggleFocus}><FocusIcon className="h-4 w-4" /></button>
        <button type="button" className="ds-toolbar-icon-button" aria-label={t('workbenchCollapse')} onClick={onCollapse}><PanelRightClose className="h-4 w-4" /></button>
      </header>
      <div className="relative flex min-h-0 flex-1">
        {filesOpen ? <div className="absolute inset-y-0 left-0 z-20 w-60 border-r border-ds-border bg-ds-card shadow-lg"><WriteSidebar /></div> : null}
        <WriteWorkspaceView leftSidebarCollapsed={false} input={input} setInput={setInput} onSubmitPrompt={onSubmitPrompt} onOpenAgentSettings={onOpenSettings} onFocusConversation={onFocusConversation} />
      </div>
    </section>
  )
}
