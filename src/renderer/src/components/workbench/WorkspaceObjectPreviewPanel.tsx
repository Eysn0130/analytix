import { lazy, Suspense, useEffect, useRef, type ComponentProps, type ReactElement } from 'react'
import { NativeOfficePanel } from '../../office/NativeOfficePanel'
import { useNativeOfficeStore, nativeOfficeTargetKey } from '../../office/native-office-store'
import { useTranslation } from 'react-i18next'
import { useChatStore } from '../../store/chat-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../../store/workspace-tabs-store'
import { openCanvasWorkspaceObject } from '../../canvas/canvas-workspace-open'

const FilePreview = lazy(() => import('../WorkspaceFilePreviewPanel').then(module => ({ default: module.WorkspaceFilePreviewPanel })))
const CanvasPreview = lazy(() => import('../../canvas/CanvasWorkspacePanel').then(module => ({ default: module.CanvasWorkspacePanel })))

function PresentOfficePreview({target,workspaceRoot,visible,className,viewerControls}: {target:NonNullable<ComponentProps<typeof FilePreview>['target']>;workspaceRoot:string;visible:boolean;className?:string;viewerControls?:ComponentProps<typeof FilePreview>['viewerControls']}): ReactElement {
  const nativeTarget = useNativeOfficeStore(state => state.target)
  const selector = target.protectedPresent!
  const latestTarget = useRef({workspaceRoot,path:target.path,selector}); latestTarget.current = {workspaceRoot,path:target.path,selector}
  const identity = nativeOfficeTargetKey({workspace:workspaceRoot,path:target.path,protectedPresent:selector})
  const ready = !!nativeTarget?.protectedPresent && nativeOfficeTargetKey(nativeTarget) === identity
  useEffect(() => {
    if (!visible) return
    let current = true
    const owner = latestTarget.current
    void useNativeOfficeStore.getState().selectPresent(owner.workspaceRoot,owner.path,owner.selector,() => current)
    return () => { current = false; void useNativeOfficeStore.getState().closePresent(owner.selector) }
  },[identity,visible])
  return <div className={`h-full min-h-0 flex flex-col ${className ?? ''}`}>
    {ready ? <NativeOfficePanel visible={visible} presentSelector={selector} viewerControls={viewerControls} /> : <p role="status" className="p-4 text-sm text-ds-muted">正在打开文档…</p>}
  </div>
}

/** Object-specific view selection inside the existing file tab, not a new mode. */
export function WorkspaceObjectPreviewPanel(props: ComponentProps<typeof FilePreview>): ReactElement {
  const { t } = useTranslation('common')
  const threadId = useChatStore(state => state.activeThreadId)
  const workspace = useChatStore(state => state.threads.find(thread => thread.id === state.activeThreadId)?.workspace || state.workspaceRoot)
  const tab = useWorkspaceTabsStore(state => state.tabs.find(item => item.id === state.activeTabId) ?? null)
  const open = useWorkspaceTabsStore(state => state.open && !state.selectorOpen)
  const routePresented = useChatStore(state => state.route !== 'plugins' && state.route !== 'settings')
  const visible = routePresented && open && props.visible !== false
  const target = props.target
  const root = target?.workspaceRoot || props.workspaceRoot
  const ownsTarget = !!target && tab?.id === workspaceObjectTabId(root, target.path)
  const ownsPresent = !!target?.protectedPresent && !!tab?.protectedPresent &&
    tab.id === 'protected-present:' + target.protectedPresent.viewId + ':' + target.protectedPresent.index &&
    nativeOfficeTargetKey({workspace:root,path:target.path,protectedPresent:tab.protectedPresent}) === nativeOfficeTargetKey({workspace:root,path:target.path,protectedPresent:target.protectedPresent})
  // Ordinary PNG viewing remains unchanged unless the user chose Canvas explicitly.
  const presentOffice = !!target?.protectedPresent && /\.(?:docx|pptx)$/i.test(target.path)
  const canvas = !!target && target.preview !== 'source' && !target.protectedRead && !target.protectedPresent &&
    (/\.canvas$/i.test(target.path) || ownsTarget && tab?.preview === 'canvas')
  return <Suspense fallback={<p role="status" className="p-4 text-sm text-ds-muted">{t('loading')}</p>}>
    {presentOffice && target ? <PresentOfficePreview target={target} workspaceRoot={root} visible={visible && ownsPresent} className={props.className} viewerControls={visible ? props.viewerControls : undefined} /> : canvas ? <CanvasPreview threadId={threadId} workspaceRoot={workspace} activeTab={ownsTarget ? tab : null}
      visible={visible && ownsTarget} viewerControls={visible ? props.viewerControls : undefined} onOpenObject={openCanvasWorkspaceObject} /> : <FilePreview {...props} visible={visible} viewerControls={visible ? props.viewerControls : undefined} />}
  </Suspense>
}
