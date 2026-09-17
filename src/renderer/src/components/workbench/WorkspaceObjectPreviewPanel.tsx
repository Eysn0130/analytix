import { lazy, Suspense, type ComponentProps, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { useChatStore } from '../../store/chat-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../../store/workspace-tabs-store'
import { openCanvasWorkspaceObject } from '../../canvas/canvas-workspace-open'

const FilePreview = lazy(() => import('../WorkspaceFilePreviewPanel').then(module => ({ default: module.WorkspaceFilePreviewPanel })))
const CanvasPreview = lazy(() => import('../../canvas/CanvasWorkspacePanel').then(module => ({ default: module.CanvasWorkspacePanel })))

/** Object-specific view selection inside the existing file tab, not a new mode. */
export function WorkspaceObjectPreviewPanel(props: ComponentProps<typeof FilePreview>): ReactElement {
  const { t } = useTranslation('common')
  const threadId = useChatStore(state => state.activeThreadId)
  const workspace = useChatStore(state => state.threads.find(thread => thread.id === state.activeThreadId)?.workspace || state.workspaceRoot)
  const tab = useWorkspaceTabsStore(state => state.tabs.find(item => item.id === state.activeTabId) ?? null)
  const open = useWorkspaceTabsStore(state => state.open && !state.selectorOpen)
  const target = props.target
  const root = target?.workspaceRoot || props.workspaceRoot
  const ownsTarget = !!target && tab?.id === workspaceObjectTabId(root, target.path)
  // Ordinary PNG viewing remains unchanged unless the user chose Canvas explicitly.
  const canvas = !!target && (/\.canvas$/i.test(target.path) || ownsTarget && tab?.preview === 'canvas')
  return <Suspense fallback={<p role="status" className="p-4 text-sm text-ds-muted">{t('loading')}</p>}>
    {canvas ? <CanvasPreview threadId={threadId} workspaceRoot={workspace} activeTab={ownsTarget ? tab : null}
      visible={open && ownsTarget} onOpenObject={openCanvasWorkspaceObject} /> : <FilePreview {...props} />}
  </Suspense>
}
