import type { ComponentProps, ReactElement } from 'react'
import { lazy } from 'react'
import { extractLatestTurnDevPreviewUrls } from '../../lib/dev-preview-detection'
import { useChatStore } from '../../store/chat-store'
import { useChatTimelinePanelState, useDevPreviewUrls } from './ChatTimelineIsland'

const loadDocumentWorkspacePanel = () => import('./DocumentWorkspacePanel').then((module) => ({ default: module.DocumentWorkspacePanel }))
export const DocumentWorkspacePanel = lazy(loadDocumentWorkspacePanel)

const loadChangeInspector = () =>
  import('../ChangeInspector').then((module) => ({ default: module.ChangeInspector }))

const loadWriteAssistantPanel = () =>
  import('../write/WriteAssistantPanel').then((module) => ({ default: module.WriteAssistantPanel }))

const loadSddAssistantPanel = () =>
  import('../sdd/SddAssistantPanel').then((module) => ({ default: module.SddAssistantPanel }))

const loadDevBrowserPanel = () =>
  import('../DevBrowserPanel').then((module) => ({ default: module.DevBrowserPanel }))

const loadWorkspaceFilePreviewPanel = () =>
  import('./WorkspaceObjectPreviewPanel').then((module) => ({
    default: module.WorkspaceObjectPreviewPanel
  }))

const loadPlanPanel = () =>
  import('../plan/PlanPanel').then((module) => ({ default: module.PlanPanel }))

const loadTodoPanel = () =>
  import('../todo/TodoPanel').then((module) => ({ default: module.TodoPanel }))

const loadThreadSummaryPanel = () =>
  import('../summary/ThreadSummaryPanel').then((module) => ({ default: module.ThreadSummaryPanel }))

const loadSubagentInspectorPanel = () =>
  import('../summary/SubagentInspectorPanel').then((module) => ({ default: module.SubagentInspectorPanel }))

const ChangeInspector = lazy(loadChangeInspector)
const WriteAssistantPanel = lazy(loadWriteAssistantPanel)
const SddAssistantPanel = lazy(loadSddAssistantPanel)
const DevBrowserPanel = lazy(loadDevBrowserPanel)
export const WorkspaceFilePreviewPanel = lazy(loadWorkspaceFilePreviewPanel)
export const PlanPanel = lazy(loadPlanPanel)
export const TodoPanel = lazy(loadTodoPanel)
export const ThreadSummaryPanelIsland = lazy(loadThreadSummaryPanel)
export const SubagentInspectorPanelIsland = lazy(loadSubagentInspectorPanel)

export type RightPanelIslandPreloadTarget =
  | 'documents'
  | 'files'
  | 'todo'
  | 'changes'
  | 'browser'
  | 'file'
  | 'plan'
  | 'summary'
  | 'sdd-ai'
  | 'child-agent'
  | 'write-assistant'

export function preloadRightPanelIsland(
  target: RightPanelIslandPreloadTarget | null | undefined
): void {
  switch (target) {
    case 'documents':
      void loadDocumentWorkspacePanel()
      break
    case 'todo':
      void loadTodoPanel()
      break
    case 'changes':
      void loadChangeInspector()
      break
    case 'browser':
      void loadDevBrowserPanel()
      break
    case 'file':
      void loadWorkspaceFilePreviewPanel()
      break
    case 'plan':
      void loadPlanPanel()
      break
    case 'summary':
      void loadThreadSummaryPanel()
      break
    case 'sdd-ai':
      void loadSddAssistantPanel()
      break
    case 'child-agent':
      void loadSubagentInspectorPanel()
      break
    case 'write-assistant':
      void loadWriteAssistantPanel()
      break
    default:
      break
  }
}

type WriteAssistantPanelIslandProps = Omit<
  ComponentProps<typeof WriteAssistantPanel>,
  'blocks' | 'hasLiveStream'
>

type SddAssistantPanelIslandProps = Omit<
  ComponentProps<typeof SddAssistantPanel>,
  'blocks' | 'hasLiveStream'
>

export function WriteAssistantPanelIsland(props: WriteAssistantPanelIslandProps): ReactElement {
  const { blocks, hasLiveStream } = useChatTimelinePanelState()
  return (
    <WriteAssistantPanel
      {...props}
      blocks={blocks}
      hasLiveStream={hasLiveStream}
    />
  )
}

export function SddAssistantPanelIsland(props: SddAssistantPanelIslandProps): ReactElement {
  const { blocks, hasLiveStream } = useChatTimelinePanelState()
  return (
    <SddAssistantPanel
      {...props}
      blocks={blocks}
      hasLiveStream={hasLiveStream}
    />
  )
}

export function ChangeInspectorIsland({
  className,
  onCollapse
}: {
  className: string
  onCollapse: () => void
}): ReactElement {
  const blocks = useChatStore((s) => s.blocks)
  return <ChangeInspector blocks={blocks} className={className} onCollapse={onCollapse} />
}

export function DevBrowserPanelIsland({
  preferredUrl,
  className,
  onCollapse
}: {
  preferredUrl?: string | null
  className?: string
  onCollapse: () => void
}): ReactElement {
  const detectedUrls = useDevPreviewUrls(extractLatestTurnDevPreviewUrls)
  return (
    <DevBrowserPanel
      detectedUrls={detectedUrls}
      preferredUrl={preferredUrl}
      className={className}
      onCollapse={onCollapse}
    />
  )
}
