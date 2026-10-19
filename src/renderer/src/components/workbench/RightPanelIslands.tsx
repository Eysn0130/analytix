import type { ComponentProps, ReactElement } from 'react'
import { lazy, useEffect, useState } from 'react'
import { extractLatestTurnDevPreviewUrls } from '../../lib/dev-preview-detection'
import { useChatStore } from '../../store/chat-store'
import { useChatTimelinePanelState, useDevPreviewUrls } from './ChatTimelineIsland'

const loadTerminalPanel = () => import('../terminal/TerminalPanel').then(module => ({ default: module.TerminalPanel }))
const TerminalPanel = lazy(loadTerminalPanel)

const loadDocumentWorkspacePanel = () => import('./DocumentWorkspacePanel').then((module) => ({ default: module.DocumentWorkspacePanel }))
export const DocumentWorkspacePanel = lazy(loadDocumentWorkspacePanel)

const loadChangeInspector = () =>
  import('../ChangeInspector').then((module) => ({ default: module.ChangeInspector }))

const loadSddAssistantPanel = () =>
  import('../sdd/SddAssistantPanel').then((module) => ({ default: module.SddAssistantPanel }))

const loadDevBrowserPanel = () =>
  import('../DevBrowserPanel').then((module) => ({ default: module.DevBrowserPanel }))

const loadWorkspaceFilePreviewPanel = () =>
  import('./WorkspaceObjectPreviewPanel').then((module) => ({
    default: module.WorkspaceObjectPreviewPanel
  }))

const loadQuestionPlanPreview = () => import('./QuestionPlanPreview').then(module => ({ default: module.QuestionPlanPreview }))
export const QuestionPlanPreview = lazy(loadQuestionPlanPreview)

const loadScheduleTaskDetail = () => import('../schedule/ScheduleTaskDetail').then(module => ({ default: module.ScheduleTaskDetail }))
export const ScheduleTaskDetail = lazy(loadScheduleTaskDetail)

const loadPlanPanel = () =>
  import('../plan/PlanPanel').then((module) => ({ default: module.PlanPanel }))

const loadTodoPanel = () =>
  import('../todo/TodoPanel').then((module) => ({ default: module.TodoPanel }))

const loadThreadSummaryPanel = () =>
  import('../summary/ThreadSummaryPanel').then((module) => ({ default: module.ThreadSummaryPanel }))

const loadSubagentInspectorPanel = () =>
  import('../summary/SubagentInspectorPanel').then((module) => ({ default: module.SubagentInspectorPanel }))

const ChangeInspector = lazy(loadChangeInspector)
const SddAssistantPanel = lazy(loadSddAssistantPanel)
const DevBrowserPanel = lazy(loadDevBrowserPanel)
export const WorkspaceFilePreviewPanel = lazy(loadWorkspaceFilePreviewPanel)
export const PlanPanel = lazy(loadPlanPanel)
export const TodoPanel = lazy(loadTodoPanel)
export const ThreadSummaryPanelIsland = lazy(loadThreadSummaryPanel)
export const SubagentInspectorPanelIsland = lazy(loadSubagentInspectorPanel)

/** Closing the workspace tab hides this view; only explicit session actions dispose PTYs. */
export function TerminalWorkspacePanel({ opened, visible, ...props }: ComponentProps<typeof TerminalPanel> & {
  opened: boolean; visible: boolean
}): ReactElement | null {
  const presented = opened && visible
  const [retained, setRetained] = useState(presented)
  useEffect(() => { if (presented) setRetained(true) }, [presented])
  return presented || retained ? (
    <div id="workbench-workspace-terminal" className="h-full min-h-0" hidden={!visible} inert={!visible}>
      <TerminalPanel {...props} interactive={visible} className="h-full w-full" />
    </div>
  ) : null
}

export type RightPanelIslandPreloadTarget =
  | 'documents'
  | 'files'
  | 'todo'
  | 'changes'
  | 'terminal'
  | 'browser'
  | 'file'
  | 'plan'
  | 'summary'
  | 'sdd-ai'
  | 'child-agent'
  | 'schedule-task'

export function preloadRightPanelIsland(
  target: RightPanelIslandPreloadTarget | null | undefined
): void {
  switch (target) {
    case 'terminal':
      void loadTerminalPanel()
      break
    case 'schedule-task':
      void loadScheduleTaskDetail()
      break
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
      void loadQuestionPlanPreview()
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
    default:
      break
  }
}

type SddAssistantPanelIslandProps = Omit<
  ComponentProps<typeof SddAssistantPanel>,
  'blocks' | 'hasLiveStream'
>

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
  onCollapse,
  onSubmitPrompt
}: {
  preferredUrl?: string | null
  className?: string
  onCollapse: () => void
  onSubmitPrompt?: ComponentProps<typeof DevBrowserPanel>['onSubmitPrompt']
}): ReactElement {
  const detectedUrls = useDevPreviewUrls(extractLatestTurnDevPreviewUrls)
  return (
    <DevBrowserPanel
      detectedUrls={detectedUrls}
      preferredUrl={preferredUrl}
      className={className}
      onCollapse={onCollapse}
      onSubmitPrompt={onSubmitPrompt}
    />
  )
}
