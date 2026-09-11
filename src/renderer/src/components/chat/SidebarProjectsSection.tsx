import type { CSSProperties, FormEvent, MouseEvent as ReactMouseEvent, ReactElement, SetStateAction } from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { LucideIcon } from 'lucide-react'
import {
  Archive,
  BarChart3,
  ClipboardList,
  Clock3,
  Copy,
  Database,
  FileText,
  Folder,
  FolderPlus,
  FolderOpen,
  GitBranch,
  GitFork,
  Loader2,
  MessageCirclePlus,
  Network,
  PencilLine,
  Plus,
  RotateCcw,
  Search,
  Sparkles,
  SquareStack,
  Trash2
} from 'lucide-react'
import type { CaseProjectIndexStatus, NormalizedCaseProject, NormalizedThread } from '../../agent/types'
import { getProvider } from '../../agent/registry'
import { ActionMenuItem, ActionMenuSeparator } from '../common/ActionMenu'
import { confirmDialog } from '../../lib/confirm-dialog'
import { formatRelativeTime } from '../../lib/format-relative-time'
import { readPinnedThreadIds, subscribePinnedThreadIds, togglePinnedThreadId } from '../../lib/pinned-threads'
import {
  readSidebarCollapsedWorkspaces,
  sidebarWorkspaceCollapseKey,
  subscribeSidebarCollapsedWorkspaces,
  writeSidebarCollapsedWorkspaces
} from '../../lib/sidebar-collapsed-workspaces'
import { buildThreadMarkdown } from '../../lib/thread-markdown'
import {
  buildAnalytixThreadLink,
  buildCodexCompatibleThreadLink
} from '../../lib/thread-links'
import { workspaceLabelFromPath } from '../../lib/workspace-label'
import { deleteSddDraft } from '../../sdd/sdd-draft-actions'
import { listSddDraftHistory, type SddDraftHistoryItem } from '../../sdd/sdd-draft-history'
import { isEmptySddAssistantThreadCandidate } from '../../sdd/sdd-thread-registry'
import { useSddDraftStore, type SddDraft } from '../../sdd/sdd-draft-store'
import {
  isClawWorkspacePath,
  isInternalDeepSeekGuiWorkspace,
  isInternalTemporaryWorkspace,
  normalizeWorkspaceRoot,
  workspaceRootIdentityKey
} from '../../lib/workspace-path'
import {
  SidebarCollapseMotion,
  SidebarFlowCollapse,
  SidebarIconButton,
  SidebarSearchField,
  SidebarTreeRow
} from '../sidebar/SidebarPrimitives'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'
import { preloadCaseOverviewForWorkspace } from '../../data-analysis/services/analysis/stats-case-overview-resource'

type SidebarProjectsSectionProps = {
  threads: NormalizedThread[]
  caseProjects?: NormalizedCaseProject[]
  caseProjectIndexStatus?: CaseProjectIndexStatus
  caseProjectThreadsById?: Record<string, NormalizedThread[]>
  caseProjectLoadingById?: Record<string, boolean>
  caseProjectErrorsById?: Record<string, string | null>
  caseProjectExpandedById?: Record<string, boolean>
  activeView: 'chat' | 'write' | 'claw'
  activeThreadId: string | null
  runtimeReady: boolean
  searchQuery: string
  showArchived: boolean
  workspaceRoot: string
  workspaceRoots: string[]
  busy: boolean
  watchTurnCompletion: Record<string, boolean>
  unreadThreadIds: Record<string, boolean>
  locale: string
  onPickWorkspace: () => void
  onRemoveWorkspace: (workspacePath: string) => Promise<void>
  onCreateThreadInWorkspace: (workspacePath: string) => void
  onOpenRequirementDraft: (draft: SddDraft) => void
  activeDataAnalysisEntry: {
    workspaceRoot: string
    itemId: DataAnalysisSidebarItemId
  } | null
  onOpenDataAnalysis: (workspacePath: string, itemId: DataAnalysisSidebarItemId) => void
  onSelectThread: (threadId: string) => void
  onRenameThread: (threadId: string, title: string) => Promise<void>
  onArchiveThread: (threadId: string) => Promise<void>
  onDeleteThread: (threadId: string) => Promise<void>
  onRestoreThread: (threadId: string) => Promise<void>
  onOpenSideChat: (threadId: string) => Promise<void> | void
  onForkThread: (threadId: string) => Promise<void> | void
  onOpenSchedule: () => void
  onSearchQueryChange: (query: string) => void
  onSetCaseProjectExpanded?: (caseProjectId: string, expanded: boolean) => void
  t: (k: string, opts?: Record<string, unknown>) => string
}

export type SidebarWorkspaceGroup = [workspacePath: string, threads: NormalizedThread[]]

type ThreadContextMenuState = {
  thread: NormalizedThread
  x: number
  y: number
}

type WorkspaceContextMenuState = {
  workspacePath: string
  x: number
  y: number
}

type DraftContextMenuState = {
  draft: SddDraftHistoryItem
  x: number
  y: number
}

type SidebarContextSubmenu = 'copy' | 'branch' | null

export type RenameThreadDialogState = {
  thread: NormalizedThread
  value: string
  submitting: boolean
}

export type DataAnalysisSidebarItemId = 'import' | 'cleaning' | 'stats' | 'visual'

type DataAnalysisSidebarItem = {
  id: DataAnalysisSidebarItemId
  labelKey: string
  titleKey: string
  Icon: LucideIcon
}

const SDD_DRAFT_HISTORY_PAGE_SIZE = 3
const SDD_DRAFT_HISTORY_LOAD_LIMIT = 40
export const SIDEBAR_WORKSPACE_THREAD_LIMIT = 5
export const SIDEBAR_WORKSPACE_THREAD_EXPAND_STEP = 10

const DATA_ANALYSIS_SIDEBAR_ITEMS: DataAnalysisSidebarItem[] = [
  {
    id: 'import',
    labelKey: 'sidebarDataAnalysisImport',
    titleKey: 'sidebarDataAnalysisImportTitle',
    Icon: Database
  },
  {
    id: 'cleaning',
    labelKey: 'sidebarDataAnalysisCleaning',
    titleKey: 'sidebarDataAnalysisCleaningTitle',
    Icon: Sparkles
  },
  {
    id: 'stats',
    labelKey: 'sidebarDataAnalysisStats',
    titleKey: 'sidebarDataAnalysisStatsTitle',
    Icon: BarChart3
  },
  {
    id: 'visual',
    labelKey: 'sidebarDataAnalysisVisual',
    titleKey: 'sidebarDataAnalysisVisualTitle',
    Icon: Network
  }
]

export function getSidebarWorkspaceVisibleThreadLimit(totalThreads: number, expandedPage = 0): number {
  const collapsedLimit = Math.min(totalThreads, SIDEBAR_WORKSPACE_THREAD_LIMIT)
  if (totalThreads <= SIDEBAR_WORKSPACE_THREAD_LIMIT || expandedPage <= 0) return collapsedLimit
  return Math.min(
    totalThreads,
    SIDEBAR_WORKSPACE_THREAD_LIMIT + expandedPage * SIDEBAR_WORKSPACE_THREAD_EXPAND_STEP
  )
}

export function getNextSidebarWorkspaceThreadPage(totalThreads: number, expandedPage = 0): number {
  if (totalThreads <= SIDEBAR_WORKSPACE_THREAD_LIMIT) return 0
  const currentPage = Math.max(0, Math.floor(expandedPage))
  const visibleLimit = getSidebarWorkspaceVisibleThreadLimit(totalThreads, currentPage)
  if (visibleLimit >= totalThreads) return 0
  return currentPage + 1
}

function isSidebarProjectWorkspacePath(workspacePath: string): boolean {
  const normalized = normalizeWorkspaceRoot(workspacePath)
  if (!normalized) return false
  if (isInternalTemporaryWorkspace(normalized)) return false
  if (isInternalDeepSeekGuiWorkspace(normalized)) return false
  if (isClawWorkspacePath(normalized)) return false
  return true
}

function compareWorkspacePathsByActive(a: string, b: string, selectedWorkspace: string): number {
  const selectedWorkspaceKey = workspaceRootIdentityKey(selectedWorkspace)
  const aKey = workspaceRootIdentityKey(a)
  const bKey = workspaceRootIdentityKey(b)
  if (aKey === selectedWorkspaceKey && bKey !== selectedWorkspaceKey) return -1
  if (bKey === selectedWorkspaceKey && aKey !== selectedWorkspaceKey) return 1
  return a.localeCompare(b)
}

function sortWorkspacePathsByActive(workspacePaths: string[], selectedWorkspace: string): string[] {
  return [...workspacePaths].sort((a, b) => compareWorkspacePathsByActive(a, b, selectedWorkspace))
}

export function buildSidebarWorkspaceGroups(options: {
  threads: NormalizedThread[]
  searchQuery: string
  showArchived: boolean
  workspaceRoot: string
  workspaceRoots: string[]
}): SidebarWorkspaceGroup[] {
  const map = new Map<string, { workspacePath: string, threads: NormalizedThread[] }>()
  const selectedWorkspace = normalizeWorkspaceRoot(options.workspaceRoot)
  const selectedWorkspaceKey = workspaceRootIdentityKey(selectedWorkspace)
  const query = options.searchQuery.trim().toLowerCase()

  const upsertWorkspace = (workspacePath: string, threads: NormalizedThread[] = []): void => {
    const normalized = normalizeWorkspaceRoot(workspacePath)
    const key = workspaceRootIdentityKey(normalized)
    if (!key) return
    const existing = map.get(key)
    if (existing) {
      existing.threads.push(...threads)
      if (key === selectedWorkspaceKey && normalized === selectedWorkspace) {
        existing.workspacePath = normalized
      }
      return
    }
    map.set(key, { workspacePath: normalized, threads: [...threads] })
  }

  for (const th of options.threads) {
    if (isInternalTemporaryWorkspace(th.workspace)) continue
    if (isInternalDeepSeekGuiWorkspace(th.workspace)) continue
    if (isClawWorkspacePath(th.workspace)) continue
    if ((th.archived === true) !== options.showArchived) continue
    const key = normalizeWorkspaceRoot(th.workspace)
    if (!key) continue
    if (query) {
      const haystack = [th.title, th.preview, key, workspaceLabelFromPath(key)]
        .filter(Boolean)
        .join('\n')
        .toLowerCase()
      if (!haystack.includes(query)) continue
    }
    upsertWorkspace(key, [th])
  }

  if (selectedWorkspace && !map.has(selectedWorkspaceKey)) {
    upsertWorkspace(selectedWorkspace)
  }
  if (!query && !options.showArchived) {
    for (const workspacePath of options.workspaceRoots) {
      const key = normalizeWorkspaceRoot(workspacePath)
      if (!key || map.has(workspaceRootIdentityKey(key))) continue
      if (isInternalTemporaryWorkspace(key)) continue
      if (isInternalDeepSeekGuiWorkspace(key)) continue
      if (isClawWorkspacePath(key)) continue
      upsertWorkspace(key)
    }
  }

  return Array.from(map.values()).map(({ workspacePath, threads }): SidebarWorkspaceGroup => [workspacePath, threads]).sort(([a], [b]) => {
    const aKey = workspaceRootIdentityKey(a)
    const bKey = workspaceRootIdentityKey(b)
    if (aKey === selectedWorkspaceKey && bKey !== selectedWorkspaceKey) return -1
    if (bKey === selectedWorkspaceKey && aKey !== selectedWorkspaceKey) return 1
    return a.localeCompare(b)
  })
}

export function buildSidebarCaseProjectGroups(options: {
  caseProjects: NormalizedCaseProject[]
  caseProjectThreadsById: Record<string, NormalizedThread[]>
  threads?: NormalizedThread[]
  workspaceRoot: string
  workspaceRoots: string[]
}): SidebarWorkspaceGroup[] {
  const selectedWorkspace = normalizeWorkspaceRoot(options.workspaceRoot)
  const selectedWorkspaceKey = workspaceRootIdentityKey(selectedWorkspace)
  const map = new Map<string, { workspacePath: string, threads: NormalizedThread[] }>()
  const upsertWorkspace = (workspacePath: string, threads: NormalizedThread[] = []): void => {
    const normalized = normalizeWorkspaceRoot(workspacePath)
    if (!isSidebarProjectWorkspacePath(normalized)) return
    const key = workspaceRootIdentityKey(normalized)
    if (!key) return
    const existing = map.get(key)
    if (existing) {
      existing.threads.push(...threads)
      if (key === selectedWorkspaceKey && normalized === selectedWorkspace) {
        existing.workspacePath = normalized
      }
      return
    }
    map.set(key, { workspacePath: normalized, threads: [...threads] })
  }
  const upsertThread = (thread: NormalizedThread): void => {
    const workspacePath = normalizeWorkspaceRoot(thread.workspace)
    if (!isSidebarProjectWorkspacePath(workspacePath)) return
    upsertWorkspace(workspacePath, [thread])
  }

  for (const project of options.caseProjects) {
    upsertWorkspace(project.rootPath, options.caseProjectThreadsById[project.id] ?? [])
  }
  for (const thread of options.threads ?? []) {
    upsertThread(thread)
  }
  if (selectedWorkspace && !map.has(selectedWorkspaceKey)) {
    upsertWorkspace(selectedWorkspace)
  }
  for (const workspacePath of options.workspaceRoots) {
    const normalized = normalizeWorkspaceRoot(workspacePath)
    const key = workspaceRootIdentityKey(normalized)
    if (!key || map.has(key)) continue
    upsertWorkspace(normalized)
  }

  return Array.from(map.values())
    .map(({ workspacePath, threads }): SidebarWorkspaceGroup => {
      const deduped = new Map<string, NormalizedThread>()
      for (const thread of threads) {
        deduped.set(thread.id, thread)
      }
      return [workspacePath, Array.from(deduped.values())]
    })
    .sort(([a], [b]) => compareWorkspacePathsByActive(a, b, selectedWorkspace))
}

export function buildSidebarDraftWorkspacePaths(options: {
  threads: NormalizedThread[]
  workspaceRoot: string
  workspaceRoots: string[]
}): string[] {
  const map = new Map<string, string>()
  const selectedWorkspace = normalizeWorkspaceRoot(options.workspaceRoot)

  const upsertWorkspace = (workspacePath: string): void => {
    const normalized = normalizeWorkspaceRoot(workspacePath)
    if (!isSidebarProjectWorkspacePath(normalized)) return
    const key = workspaceRootIdentityKey(normalized)
    if (!key) return
    const previous = map.get(key)
    if (!previous || normalized === selectedWorkspace) {
      map.set(key, normalized)
    }
  }

  upsertWorkspace(selectedWorkspace)
  for (const workspacePath of options.workspaceRoots) {
    upsertWorkspace(workspacePath)
  }
  for (const thread of options.threads) {
    upsertWorkspace(thread.workspace ?? '')
  }

  return sortWorkspacePathsByActive([...map.values()], selectedWorkspace)
}

export function isSidebarWorkspaceCollapsed(
  collapsed: Record<string, boolean>,
  workspacePath: string,
  activeThreadWorkspace = ''
): boolean {
  const collapseKey = sidebarWorkspaceCollapseKey(workspacePath)
  if (Object.prototype.hasOwnProperty.call(collapsed, collapseKey)) {
    return collapsed[collapseKey] === true
  }
  const activeKey = workspaceRootIdentityKey(normalizeWorkspaceRoot(activeThreadWorkspace))
  if (!activeKey) return true
  return workspaceRootIdentityKey(workspacePath) !== activeKey
}

export function isSidebarCaseProjectCollapsed(
  expandedById: Record<string, boolean>,
  caseProjectId: string,
  hasActiveThread = false
): boolean {
  const projectId = caseProjectId.trim()
  if (!projectId) return true
  if (Object.prototype.hasOwnProperty.call(expandedById, projectId)) {
    return expandedById[projectId] !== true
  }
  return !hasActiveThread
}

export function buildExpandedSidebarDraftWorkspacePaths(options: {
  workspacePaths: string[]
  collapsed: Record<string, boolean>
  activeThreadWorkspace?: string
}): string[] {
  return options.workspacePaths.filter((workspacePath) =>
    !isSidebarWorkspaceCollapsed(options.collapsed, workspacePath, options.activeThreadWorkspace)
  )
}

export function filterSddDraftHistoryItems(
  items: SddDraftHistoryItem[],
  searchQuery: string,
  workspacePath = ''
): SddDraftHistoryItem[] {
  const query = searchQuery.trim().toLowerCase()
  if (!query) return items
  const workspaceLabel = workspacePath ? workspaceLabelFromPath(workspacePath) : ''
  return items.filter((item) => {
    const haystack = [
      item.title,
      item.relativePath,
      item.absolutePath,
      item.searchText,
      workspacePath,
      workspaceLabel
    ]
      .filter(Boolean)
      .join('\n')
      .toLowerCase()
    return haystack.includes(query)
  })
}

export function mergeSidebarWorkspaceGroupsWithDraftHistory(options: {
  groups: SidebarWorkspaceGroup[]
  draftHistoryByWorkspace: Record<string, SddDraftHistoryItem[]>
  workspaceRoot: string
}): SidebarWorkspaceGroup[] {
  const selectedWorkspace = normalizeWorkspaceRoot(options.workspaceRoot)
  const map = new Map<string, SidebarWorkspaceGroup>()

  const upsertGroup = (workspacePath: string, threads: NormalizedThread[] = []): void => {
    const normalized = normalizeWorkspaceRoot(workspacePath)
    if (!isSidebarProjectWorkspacePath(normalized)) return
    const key = workspaceRootIdentityKey(normalized)
    if (!key) return
    const previous = map.get(key)
    if (previous) {
      previous[1].push(...threads)
      if (normalized === selectedWorkspace) previous[0] = normalized
      return
    }
    map.set(key, [normalized, [...threads]])
  }

  for (const [workspacePath, threads] of options.groups) {
    upsertGroup(workspacePath, threads)
  }
  for (const [workspacePath, items] of Object.entries(options.draftHistoryByWorkspace)) {
    if (items.length > 0) upsertGroup(workspacePath)
  }

  return Array.from(map.values()).sort(([a], [b]) => compareWorkspacePathsByActive(a, b, selectedWorkspace))
}

export function filterEmptySddAssistantThreadsFromSidebar(
  threads: NormalizedThread[],
  draftHistory: SddDraftHistoryItem[]
): NormalizedThread[] {
  const draftThreadIds = new Set<string>()
  for (const draft of draftHistory) {
    for (const threadId of draft.chatThreadIds ?? []) {
      if (threadId.trim()) draftThreadIds.add(threadId.trim())
    }
  }
  if (draftThreadIds.size === 0) return [...threads]
  return threads.filter((thread) =>
    !draftThreadIds.has(thread.id) || !isEmptySddAssistantThreadCandidate(thread)
  )
}

export type SidebarPinnedThreadOrder = ReadonlyMap<string, number>

export function buildSidebarPinnedThreadOrder(pinnedThreadIds: Set<string>): SidebarPinnedThreadOrder {
  const order = new Map<string, number>()
  let index = 0
  for (const id of pinnedThreadIds) {
    order.set(id, index)
    index += 1
  }
  return order
}

export function compareSidebarThreadsWithPins(
  a: NormalizedThread,
  b: NormalizedThread,
  pinnedThreadOrder: SidebarPinnedThreadOrder
): number {
  const aPinnedIndex = pinnedThreadOrder.get(a.id)
  const bPinnedIndex = pinnedThreadOrder.get(b.id)
  const aPinned = aPinnedIndex !== undefined
  const bPinned = bPinnedIndex !== undefined

  if (aPinned !== bPinned) return aPinned ? -1 : 1
  if (aPinned && bPinned && aPinnedIndex !== bPinnedIndex) return aPinnedIndex - bPinnedIndex
  return Date.parse(b.updatedAt) - Date.parse(a.updatedAt)
}

function sddDraftHistoryForWorkspace(
  draftHistoryByWorkspace: Record<string, SddDraftHistoryItem[]>,
  workspacePath: string
): SddDraftHistoryItem[] {
  const exact = draftHistoryByWorkspace[workspacePath]
  if (exact) return exact
  const targetKey = workspaceRootIdentityKey(workspacePath)
  if (!targetKey) return []
  for (const [path, history] of Object.entries(draftHistoryByWorkspace)) {
    if (workspaceRootIdentityKey(path) === targetKey) return history
  }
  return []
}

function contextMenuPosition(
  event: ReactMouseEvent<HTMLDivElement>,
  width: number,
  height: number
): { x: number; y: number } {
  return {
    x: Math.max(8, Math.min(event.clientX, window.innerWidth - width - 8)),
    y: Math.max(8, Math.min(event.clientY, window.innerHeight - height - 8))
  }
}

function sidebarRevealStyle(index: number, open: boolean): CSSProperties {
  return {
    transitionDelay: open ? `${Math.min(index * 16, 96)}ms` : '0ms'
  }
}

export function SidebarProjectsSection({
  threads,
  caseProjects = [],
  caseProjectIndexStatus = 'ready',
  caseProjectThreadsById = {},
  caseProjectLoadingById = {},
  caseProjectErrorsById = {},
  caseProjectExpandedById = {},
  activeView,
  activeThreadId,
  runtimeReady,
  searchQuery,
  showArchived,
  workspaceRoot,
  workspaceRoots,
  busy,
  watchTurnCompletion,
  unreadThreadIds,
  locale,
  onPickWorkspace,
  onRemoveWorkspace,
  onCreateThreadInWorkspace,
  onOpenRequirementDraft,
  activeDataAnalysisEntry,
  onOpenDataAnalysis,
  onSelectThread,
  onRenameThread,
  onArchiveThread,
  onDeleteThread,
  onRestoreThread,
  onOpenSideChat,
  onForkThread,
  onOpenSchedule,
  onSearchQueryChange,
  onSetCaseProjectExpanded,
  t
}: SidebarProjectsSectionProps): ReactElement {
  const [collapsed, setCollapsedState] = useState<Record<string, boolean>>(() => readSidebarCollapsedWorkspaces())
  const [expandedWorkspacePages, setExpandedWorkspacePages] = useState<Record<string, number>>({})
  const [deletingThreadIds, setDeletingThreadIds] = useState<Record<string, boolean>>({})
  const [deletingDraftIds, setDeletingDraftIds] = useState<Record<string, boolean>>({})
  const [draftHistoryErrors, setDraftHistoryErrors] = useState<Record<string, string>>({})
  const [draftHistoryRefreshVersion, setDraftHistoryRefreshVersion] = useState(0)
  const [searchOpen, setSearchOpen] = useState(false)
  const [threadContextMenu, setThreadContextMenu] = useState<ThreadContextMenuState | null>(null)
  const [workspaceContextMenu, setWorkspaceContextMenu] = useState<WorkspaceContextMenuState | null>(null)
  const [draftContextMenu, setDraftContextMenu] = useState<DraftContextMenuState | null>(null)
  const [contextSubmenu, setContextSubmenu] = useState<SidebarContextSubmenu>(null)
  const [renameThreadDialog, setRenameThreadDialog] = useState<RenameThreadDialogState | null>(null)
  const [draftHistoryByWorkspace, setDraftHistoryByWorkspace] = useState<Record<string, SddDraftHistoryItem[]>>({})
  const [draftHistoryLoadingByWorkspace, setDraftHistoryLoadingByWorkspace] = useState<Record<string, boolean>>({})
  const [pinnedThreadIds, setPinnedThreadIds] = useState(() => readPinnedThreadIds())
  const pinnedThreadOrder = useMemo(() => buildSidebarPinnedThreadOrder(pinnedThreadIds), [pinnedThreadIds])
  const activeSddDraftId = useSddDraftStore((s) => s.activeDraft?.id ?? '')
  const setCollapsed = useCallback((updater: SetStateAction<Record<string, boolean>>): void => {
    setCollapsedState((current) => {
      const next = typeof updater === 'function'
        ? (updater as (value: Record<string, boolean>) => Record<string, boolean>)(current)
        : updater
      writeSidebarCollapsedWorkspaces(next)
      return next
    })
  }, [])

  useEffect(() => {
    return subscribePinnedThreadIds(() => setPinnedThreadIds(readPinnedThreadIds()))
  }, [])

  useEffect(() => {
    return subscribeSidebarCollapsedWorkspaces(() => setCollapsedState(readSidebarCollapsedWorkspaces()))
  }, [])

  const usingCaseProjects = (caseProjects.length > 0 || caseProjectIndexStatus === 'building') && !searchQuery.trim() && !showArchived
  const loadedCaseProjectThreads = useMemo(
    () => Object.values(caseProjectThreadsById).flat(),
    [caseProjectThreadsById]
  )
  const allKnownThreads = useMemo(
    () => [...threads, ...loadedCaseProjectThreads],
    [loadedCaseProjectThreads, threads]
  )
  const caseProjectRootPaths = useMemo(
    () => caseProjects.map((project) => project.rootPath),
    [caseProjects]
  )
  const caseProjectByWorkspaceKey = useMemo(() => {
    const map = new Map<string, NormalizedCaseProject>()
    for (const project of caseProjects) {
      const key = workspaceRootIdentityKey(project.rootPath)
      if (key) map.set(key, project)
    }
    return map
  }, [caseProjects])

  const groups = useMemo(() => {
    if (usingCaseProjects) {
      if (caseProjectIndexStatus === 'building' && caseProjects.length === 0) return []
      return buildSidebarCaseProjectGroups({
        caseProjects,
        caseProjectThreadsById,
        threads,
        workspaceRoot,
        workspaceRoots
      })
    }
    return buildSidebarWorkspaceGroups({
      threads,
      searchQuery,
      showArchived,
      workspaceRoot,
      workspaceRoots
    })
  }, [caseProjectIndexStatus, caseProjectThreadsById, caseProjects, searchQuery, showArchived, threads, usingCaseProjects, workspaceRoot, workspaceRoots])

  const draftHistoryWorkspacePaths = useMemo(() => {
    return buildSidebarDraftWorkspacePaths({
      threads: allKnownThreads,
      workspaceRoot,
      workspaceRoots: usingCaseProjects
        ? [...workspaceRoots, ...caseProjectRootPaths]
        : workspaceRoots
    })
  }, [allKnownThreads, caseProjectRootPaths, usingCaseProjects, workspaceRoot, workspaceRoots])
  const activeThreadWorkspace = useMemo(() => {
    return allKnownThreads.find((thread) => thread.id === activeThreadId)?.workspace ?? ''
  }, [activeThreadId, allKnownThreads])
  const activeThreadWorkspaceForCollapse = usingCaseProjects ? '' : activeThreadWorkspace
  const expandedDraftHistoryWorkspacePaths = useMemo(() => {
    if (usingCaseProjects) {
      return draftHistoryWorkspacePaths.filter((workspacePath) => {
        const project = caseProjectByWorkspaceKey.get(workspaceRootIdentityKey(workspacePath))
        if (project) {
          const hasActiveThread = activeThreadId != null &&
            workspaceRootIdentityKey(workspacePath) === workspaceRootIdentityKey(activeThreadWorkspace)
          return !isSidebarCaseProjectCollapsed(caseProjectExpandedById, project.id, hasActiveThread)
        }
        return !isSidebarWorkspaceCollapsed(collapsed, workspacePath, '')
      })
    }
    return buildExpandedSidebarDraftWorkspacePaths({
      workspacePaths: draftHistoryWorkspacePaths,
      collapsed,
      activeThreadWorkspace: activeThreadWorkspaceForCollapse
    })
  }, [
    activeThreadId,
    activeThreadWorkspace,
    activeThreadWorkspaceForCollapse,
    caseProjectByWorkspaceKey,
    caseProjectExpandedById,
    collapsed,
    draftHistoryWorkspacePaths,
    usingCaseProjects
  ])

  const filteredDraftHistoryByWorkspace = useMemo(() => {
    return Object.fromEntries(
      Object.entries(draftHistoryByWorkspace)
        .map(([path, history]) => [
          path,
          filterSddDraftHistoryItems(history, searchQuery, path)
        ] as const)
        .filter(([, history]) => history.length > 0)
    )
  }, [draftHistoryByWorkspace, searchQuery])

  const displayGroups = useMemo(() => {
    return mergeSidebarWorkspaceGroupsWithDraftHistory({
      groups,
      draftHistoryByWorkspace: filteredDraftHistoryByWorkspace,
      workspaceRoot
    })
  }, [filteredDraftHistoryByWorkspace, groups, workspaceRoot])

  const searchVisible = searchOpen || searchQuery.trim().length > 0
  const allGroupsCollapsed = displayGroups.length > 0 && displayGroups.every(([workspacePath]) => {
    const project = usingCaseProjects
      ? caseProjectByWorkspaceKey.get(workspaceRootIdentityKey(workspacePath))
      : undefined
    if (project) {
      const hasActiveThread = activeThreadId != null &&
        workspaceRootIdentityKey(workspacePath) === workspaceRootIdentityKey(activeThreadWorkspace)
      return isSidebarCaseProjectCollapsed(caseProjectExpandedById, project.id, hasActiveThread)
    }
    return isSidebarWorkspaceCollapsed(collapsed, workspacePath, activeThreadWorkspaceForCollapse)
  })
  const workspaceHistoryKey = expandedDraftHistoryWorkspacePaths.join('\n')
  const contextMenuPortalTarget = typeof document === 'undefined' ? null : document.body

  useEffect(() => {
    if (
      typeof window === 'undefined' ||
      typeof window.analytix?.files?.listDirectory !== 'function' ||
      typeof window.analytix?.files?.read !== 'function'
    ) {
      setDraftHistoryByWorkspace({})
      setDraftHistoryLoadingByWorkspace({})
      return
    }
    const workspacePaths = workspaceHistoryKey.split('\n').filter(Boolean)
    if (workspacePaths.length === 0) {
      setDraftHistoryByWorkspace({})
      setDraftHistoryLoadingByWorkspace({})
      return
    }
    let cancelled = false
    setDraftHistoryLoadingByWorkspace(Object.fromEntries(workspacePaths.map((path) => [path, true])))
    void Promise.all(
      workspacePaths.map(async (path) => {
        const history = await listSddDraftHistory({
          workspaceRoot: path,
          listWorkspaceDirectory: window.analytix.files.listDirectory,
          readWorkspaceFile: window.analytix.files.read,
          limit: SDD_DRAFT_HISTORY_LOAD_LIMIT
        }).catch(() => [])
        return [path, history] as const
      })
    ).then((entries) => {
      if (cancelled) return
      setDraftHistoryByWorkspace(Object.fromEntries(entries.filter(([, history]) => history.length > 0)))
      setDraftHistoryLoadingByWorkspace({})
    })
    return () => {
      cancelled = true
    }
  }, [draftHistoryRefreshVersion, workspaceHistoryKey])

  const closeContextMenus = useCallback((): void => {
    setThreadContextMenu(null)
    setWorkspaceContextMenu(null)
    setDraftContextMenu(null)
    setContextSubmenu(null)
  }, [])

  useEffect(() => {
    if (!threadContextMenu && !workspaceContextMenu && !draftContextMenu) return
    const close = (): void => closeContextMenus()
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') close()
    }
    window.addEventListener('pointerdown', close)
    window.addEventListener('scroll', close, true)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('pointerdown', close)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [closeContextMenus, draftContextMenu, threadContextMenu, workspaceContextMenu])

  const toggleAllGroups = (): void => {
    if (displayGroups.length === 0) return
    if (usingCaseProjects) {
      if (allGroupsCollapsed) return
      const nextCollapsed = { ...collapsed }
      const projectIdsToCollapse: string[] = []
      for (const [workspacePath] of displayGroups) {
        const project = caseProjectByWorkspaceKey.get(workspaceRootIdentityKey(workspacePath))
        if (project) {
          projectIdsToCollapse.push(project.id)
        } else {
          nextCollapsed[sidebarWorkspaceCollapseKey(workspacePath)] = true
        }
      }
      setCollapsed(nextCollapsed)
      for (const projectId of projectIdsToCollapse) {
        onSetCaseProjectExpanded?.(projectId, false)
      }
      return
    }
    if (allGroupsCollapsed) {
      setCollapsed(Object.fromEntries(displayGroups.map(([workspacePath]) => [
        sidebarWorkspaceCollapseKey(workspacePath),
        false
      ])))
      return
    }
    setCollapsed(Object.fromEntries(displayGroups.map(([workspacePath]) => [
      sidebarWorkspaceCollapseKey(workspacePath),
      true
    ])))
  }

  const handleDeleteThread = async (thread: NormalizedThread): Promise<void> => {
    const threadId = thread.id.trim()
    if (!threadId || deletingThreadIds[threadId]) return
    const confirmMessage = t('sidebarThreadDeleteConfirm', { title: thread.title })
    if (!(await confirmDialog(confirmMessage))) return
    setDeletingThreadIds((prev) => ({ ...prev, [threadId]: true }))
    try {
      await onDeleteThread(threadId)
    } finally {
      setDeletingThreadIds((prev) => {
        const next = { ...prev }
        delete next[threadId]
        return next
      })
    }
  }

  const handleArchiveThread = async (thread: NormalizedThread): Promise<void> => {
    const threadId = thread.id.trim()
    if (!threadId || deletingThreadIds[threadId]) return
    const confirmMessage = t('sidebarThreadArchiveConfirm', { title: thread.title })
    if (!(await confirmDialog(confirmMessage))) return
    setDeletingThreadIds((prev) => ({ ...prev, [threadId]: true }))
    try {
      await onArchiveThread(threadId)
    } finally {
      setDeletingThreadIds((prev) => {
        const next = { ...prev }
        delete next[threadId]
        return next
      })
    }
  }

  const handleRestoreThread = async (thread: NormalizedThread): Promise<void> => {
    const threadId = thread.id.trim()
    if (!threadId || deletingThreadIds[threadId]) return
    setDeletingThreadIds((prev) => ({ ...prev, [threadId]: true }))
    try {
      await onRestoreThread(threadId)
    } finally {
      setDeletingThreadIds((prev) => {
        const next = { ...prev }
        delete next[threadId]
        return next
      })
    }
  }

  const openRenameThreadDialog = (thread: NormalizedThread): void => {
    const threadId = thread.id.trim()
    if (!threadId || deletingThreadIds[threadId]) return
    setRenameThreadDialog({
      thread,
      value: thread.title,
      submitting: false
    })
  }

  const closeRenameThreadDialog = (): void => {
    setRenameThreadDialog((current) => current?.submitting ? current : null)
  }

  const submitRenameThreadDialog = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault()
    const dialog = renameThreadDialog
    if (!dialog || dialog.submitting) return
    const threadId = dialog.thread.id.trim()
    const nextTitle = dialog.value.trim()
    if (!threadId || deletingThreadIds[threadId]) return
    if (!nextTitle) return
    if (nextTitle === dialog.thread.title) {
      setRenameThreadDialog(null)
      return
    }
    setDeletingThreadIds((prev) => ({ ...prev, [threadId]: true }))
    setRenameThreadDialog((current) =>
      current?.thread.id === threadId ? { ...current, value: nextTitle, submitting: true } : current
    )
    try {
      await onRenameThread(threadId, nextTitle)
      setRenameThreadDialog(null)
    } catch {
      setRenameThreadDialog((current) =>
        current?.thread.id === threadId ? { ...current, submitting: false } : current
      )
    } finally {
      setDeletingThreadIds((prev) => {
        const next = { ...prev }
        delete next[threadId]
        return next
      })
    }
  }

  const copyText = async (text: string): Promise<void> => {
    if (!text.trim()) return
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      // Keep context-menu copy failures non-blocking; the top-level error surface
      // is reserved for runtime and workflow failures in this sidebar.
    }
  }

  const copyThreadMarkdown = async (thread: NormalizedThread): Promise<void> => {
    const threadId = thread.id.trim()
    if (!threadId || !runtimeReady) return
    try {
      const detail = await getProvider().getThreadDetail(threadId)
      await copyText(buildThreadMarkdown(
        thread,
        thread.workspace ? workspaceLabelFromPath(thread.workspace) : '',
        detail.blocks
      ))
    } catch {
      await copyText(threadId)
    }
  }

  const copyDraftContents = async (draft: SddDraftHistoryItem): Promise<void> => {
    if (typeof window === 'undefined' || typeof window.analytix?.files?.read !== 'function') return
    try {
      const result = await window.analytix.files.read({
        workspaceRoot: draft.workspaceRoot,
        path: draft.relativePath
      })
      if (result.ok) await copyText(result.content)
    } catch {
      // Non-blocking copy path; menu closes immediately like the reference app.
    }
  }

  const openPathWithEditor = (
    path: string,
    workspacePath: string,
    editorId: 'system' | 'finder'
  ): void => {
    if (typeof window === 'undefined' || typeof window.analytix?.workspace?.openEditorPath !== 'function') return
    void window.analytix.workspace.openEditorPath({
      path,
      workspaceRoot: workspacePath,
      editorId
    }).catch(() => undefined)
  }

  const openThreadContextMenu = (
    event: ReactMouseEvent<HTMLDivElement>,
    thread: NormalizedThread
  ): void => {
    event.preventDefault()
    event.stopPropagation()
    setWorkspaceContextMenu(null)
    setDraftContextMenu(null)
    setContextSubmenu(null)
    setThreadContextMenu({
      thread,
      ...contextMenuPosition(event, 216, 310)
    })
  }

  const openWorkspaceContextMenu = (
    event: ReactMouseEvent<HTMLDivElement>,
    workspacePath: string
  ): void => {
    event.preventDefault()
    event.stopPropagation()
    setThreadContextMenu(null)
    setDraftContextMenu(null)
    setContextSubmenu(null)
    setWorkspaceContextMenu({
      workspacePath,
      ...contextMenuPosition(event, 216, 210)
    })
  }

  const openDraftContextMenu = (
    event: ReactMouseEvent<HTMLDivElement>,
    draft: SddDraftHistoryItem
  ): void => {
    event.preventDefault()
    event.stopPropagation()
    setThreadContextMenu(null)
    setWorkspaceContextMenu(null)
    setContextSubmenu(null)
    setDraftContextMenu({
      draft,
      ...contextMenuPosition(event, 216, 250)
    })
  }

  const handleRemoveWorkspace = async (workspacePath: string): Promise<void> => {
    const confirmMessage = t('sidebarWorkspaceRemoveConfirm', { path: workspacePath })
    if (!(await confirmDialog(confirmMessage))) return
    await onRemoveWorkspace(workspacePath)
  }

  const handleDeleteRequirementDraft = async (draft: SddDraftHistoryItem): Promise<void> => {
    const draftId = draft.id.trim()
    if (!draftId || deletingDraftIds[draftId]) return
    const workspaceKey = draft.workspaceRoot
    const confirmMessage = t('sddDraftHistoryDeleteConfirm', { title: draft.title })
    if (!(await confirmDialog(confirmMessage))) return

    setDeletingDraftIds((prev) => ({ ...prev, [draftId]: true }))
    setDraftHistoryErrors((prev) => {
      const next = { ...prev }
      delete next[workspaceKey]
      return next
    })
    try {
      const result = await deleteSddDraft(draft)
      if (!result.ok) {
        setDraftHistoryErrors((prev) => ({
          ...prev,
          [workspaceKey]: t('sddDraftHistoryDeleteFailed', { message: result.message })
        }))
        return
      }
      setDraftHistoryByWorkspace((current) => {
        const next = Object.fromEntries(
          Object.entries(current)
            .map(([workspacePath, items]) => [
              workspacePath,
              items.filter((item) => item.id !== draftId)
            ] as const)
            .filter(([, items]) => items.length > 0)
        )
        return next
      })
      setDraftHistoryRefreshVersion((version) => version + 1)
    } finally {
      setDeletingDraftIds((prev) => {
        const next = { ...prev }
        delete next[draftId]
        return next
      })
    }
  }

  return (
    <div className="ds-no-drag flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-[38px] items-center justify-between px-2 pb-1.5 pt-3">
        <button
          type="button"
          onClick={toggleAllGroups}
          className="flex min-w-0 items-center gap-1.5 rounded-md px-2 py-1.5 text-[13px] text-ds-faint transition hover:bg-[var(--ds-sidebar-row-hover)] hover:text-ds-muted"
          title={t('sidebarProjects')}
          aria-label={t('sidebarProjects')}
        >
          <span className="truncate">{t('sidebarProjects')}</span>
          {allGroupsCollapsed ? (
            <AnalytixIconRegistry.icons.disclosureRight className="h-3 w-3 shrink-0" />
          ) : (
            <AnalytixIconRegistry.icons.disclosureDown className="h-3 w-3 shrink-0" />
          )}
        </button>
        <div className="flex shrink-0 items-center gap-1">
          <SidebarIconButton
            onClick={() => setSearchOpen((open) => !open)}
            active={searchVisible}
            className="h-7 w-7"
            title={t('sidebarSearchThreads')}
            ariaLabel={t('sidebarSearchThreads')}
          >
            <Search className="h-3.5 w-3.5" strokeWidth={1.85} />
          </SidebarIconButton>
          <SidebarIconButton
            onClick={onPickWorkspace}
            className="h-7 w-7"
            title={workspaceRoot ? t('changeWorkspace') : t('selectWorkspace')}
            ariaLabel={workspaceRoot ? t('changeWorkspace') : t('selectWorkspace')}
          >
            <FolderPlus className="h-3.5 w-3.5" strokeWidth={1.75} />
          </SidebarIconButton>
        </div>
      </div>

      <SidebarCollapseMotion
        open={searchVisible}
        className="ds-sidebar-search-motion"
        innerClassName="flex items-center gap-1 px-2"
      >
        <SidebarSearchField
          value={searchQuery}
          onChange={onSearchQueryChange}
          placeholder={t('sidebarSearchThreads')}
          clearLabel={t('clear')}
        />
      </SidebarCollapseMotion>

      <div className="ds-sidebar-projects-scroll min-h-0 flex-1 overflow-y-auto px-1 pb-2 pt-0.5">
        {displayGroups.length === 0 && usingCaseProjects && caseProjectIndexStatus === 'building' ? (
          <div className="flex min-h-[36px] items-center gap-2 px-3 py-2 text-[12.5px] text-ds-faint">
            <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" strokeWidth={1.9} />
            <span className="truncate">{t('loading')}</span>
          </div>
        ) : null}

        {displayGroups.length === 0 && !(usingCaseProjects && caseProjectIndexStatus === 'building') ? (
          <SidebarEmpty
            runtimeReady={runtimeReady}
            hasWorkspace={!!workspaceRoot}
            onPickWorkspace={onPickWorkspace}
            t={t}
          />
        ) : null}

        {displayGroups.map(([workspacePath, list]) => {
          const collapseKey = sidebarWorkspaceCollapseKey(workspacePath)
          const folderName = workspaceLabelFromPath(workspacePath)
          const workspaceContext = workspaceContextLabel(workspacePath, folderName)
          const caseProject = usingCaseProjects
            ? caseProjectByWorkspaceKey.get(workspaceRootIdentityKey(workspacePath))
            : undefined
          const caseProjectLoadedThreadCount = caseProject
            ? caseProjectThreadsById[caseProject.id]?.length ?? 0
            : 0
          const isCaseProjectLoading = caseProject ? caseProjectLoadingById[caseProject.id] === true : false
          const caseProjectError = caseProject ? caseProjectErrorsById[caseProject.id] ?? '' : ''
          const hasActiveThread = activeThreadId != null &&
            workspaceRootIdentityKey(workspacePath) === workspaceRootIdentityKey(activeThreadWorkspace)
          const isCollapsed = caseProject
            ? isSidebarCaseProjectCollapsed(caseProjectExpandedById, caseProject.id, hasActiveThread)
            : isSidebarWorkspaceCollapsed(collapsed, workspacePath, activeThreadWorkspaceForCollapse)
          const isDraftHistoryLoading = draftHistoryLoadingByWorkspace[workspacePath] === true
          const draftHistory = sddDraftHistoryForWorkspace(filteredDraftHistoryByWorkspace, workspacePath)
          const sortedThreads = filterEmptySddAssistantThreadsFromSidebar(list, draftHistory)
            .sort((a, b) => compareSidebarThreadsWithPins(a, b, pinnedThreadOrder))
          const workspaceExpandedPage = expandedWorkspacePages[workspacePath] ?? 0
          const visibleThreadLimit = getSidebarWorkspaceVisibleThreadLimit(
            sortedThreads.length,
            workspaceExpandedPage
          )
          const workspaceExpanded = visibleThreadLimit > SIDEBAR_WORKSPACE_THREAD_LIMIT
          const primaryThreads = sortedThreads.slice(0, SIDEBAR_WORKSPACE_THREAD_LIMIT)
          const overflowThreads = sortedThreads.slice(SIDEBAR_WORKSPACE_THREAD_LIMIT, visibleThreadLimit)
          const hasOverflow = sortedThreads.length > SIDEBAR_WORKSPACE_THREAD_LIMIT
          const hasMoreWorkspaceThreads = sortedThreads.length > visibleThreadLimit
          const draftRevealIndex = 1
          const threadRevealOffset = 1 + (draftHistory.length > 0 ? 1 : 0)
          const renderThread = (
            thread: NormalizedThread,
            index: number,
            revealOpen = !isCollapsed
          ): ReactElement => (
            <div
              key={thread.id}
              className="ds-sidebar-reveal-item"
              style={sidebarRevealStyle(threadRevealOffset + index, revealOpen)}
            >
              <ThreadRow
                thread={thread}
                active={(activeView === 'chat' || activeView === 'write') && activeThreadId === thread.id}
                deleting={deletingThreadIds[thread.id] === true}
                locale={locale}
                showRunning={
                  thread.status?.trim().toLowerCase() === 'running' ||
                  (activeThreadId === thread.id && busy) ||
                  watchTurnCompletion[thread.id] === true
                }
                showUnread={
                  unreadThreadIds[thread.id] === true && activeThreadId !== thread.id
                }
                pinned={pinnedThreadIds.has(thread.id)}
                onSelect={() => onSelectThread(thread.id)}
                onContextMenu={(event) => openThreadContextMenu(event, thread)}
                onRename={() => openRenameThreadDialog(thread)}
                onArchive={() => void handleArchiveThread(thread)}
                onDelete={() => void handleDeleteThread(thread)}
                onRestore={() => void handleRestoreThread(thread)}
              />
            </div>
          )
          const toggleWorkspace = (): void => {
            if (caseProject) {
              void preloadCaseOverviewForWorkspace(workspacePath).catch(() => {})
              onSetCaseProjectExpanded?.(caseProject.id, isCollapsed)
              return
            }
            setCollapsed((current) => ({
              ...current,
              [collapseKey]: !isSidebarWorkspaceCollapsed(current, workspacePath, activeThreadWorkspaceForCollapse)
            }))
          }
          return (
            <div key={workspacePath} className="mb-2">
              <SidebarTreeRow
                title={workspacePath}
                onClick={toggleWorkspace}
                className="min-h-[36px] text-[13.5px]"
                buttonClassName="items-center gap-2 px-2.5 py-2"
                actionsVisibility="hidden"
                actionsLayout="overlay"
                onContextMenu={(event) => openWorkspaceContextMenu(event, workspacePath)}
                actions={
                  <>
                    <SidebarIconButton
                      onClick={() => onCreateThreadInWorkspace(workspacePath)}
                      title={t('sidebarWorkspaceNewThread')}
                      ariaLabel={t('sidebarWorkspaceNewThread')}
                      className="h-6 w-6"
                      stopPropagation
                    >
                      <Plus className="h-3.5 w-3.5" strokeWidth={1.9} />
                    </SidebarIconButton>
                    <SidebarIconButton
                      onClick={() => void handleRemoveWorkspace(workspacePath)}
                      title={t('sidebarWorkspaceRemove')}
                      ariaLabel={t('sidebarWorkspaceRemove')}
                      tone="danger"
                      className="h-6 w-6"
                      stopPropagation
                    >
                      <Trash2 className="h-3.5 w-3.5" strokeWidth={1.9} />
                    </SidebarIconButton>
                  </>
                }
              >
                {isCaseProjectLoading ? (
                  <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-accent" strokeWidth={2} />
                ) : isCollapsed ? (
                  <Folder className="h-4 w-4 shrink-0 text-ds-muted" strokeWidth={1.75} />
                ) : (
                  <FolderOpen className="h-4 w-4 shrink-0 text-ds-muted" strokeWidth={1.75} />
                )}
                <span className="min-w-0 flex-1 truncate">{folderName}</span>
                {caseProject && caseProject.threadCount > 0 ? (
                  <span className="shrink-0 rounded-md bg-ds-card/70 px-1.5 py-0.5 text-[10.5px] text-ds-faint tabular-nums transition group-hover:opacity-0 group-focus-within:opacity-0">
                    {caseProjectLoadedThreadCount > 0
                      ? `${caseProjectLoadedThreadCount}/${caseProject.threadCount}`
                      : caseProject.threadCount}
                  </span>
                ) : null}
                {workspaceContext ? (
                  <span className="min-w-0 max-w-[42%] shrink truncate text-[12.5px] text-ds-faint transition group-hover:opacity-0 group-focus-within:opacity-0">
                    {workspaceContext}
                  </span>
                ) : null}
              </SidebarTreeRow>

              <SidebarCollapseMotion
                open={!isCollapsed}
                className="ds-sidebar-workspace-children"
                innerClassName="flex flex-col gap-[3px] pl-4"
              >
                <div
                  className="ds-sidebar-reveal-item pb-[3px]"
                  style={sidebarRevealStyle(0, !isCollapsed)}
                >
                  <DataAnalysisWorkspaceRows
                    activeItemId={
                      activeDataAnalysisEntry?.workspaceRoot === workspacePath
                        ? activeDataAnalysisEntry.itemId
                        : null
                    }
                    onOpen={(itemId) => onOpenDataAnalysis(workspacePath, itemId)}
                    t={t}
                  />
                </div>
                {isDraftHistoryLoading ? (
                  <div
                    className="ds-sidebar-reveal-item flex min-h-[28px] items-center gap-2 px-2.5 py-1.5 text-[12.5px] text-ds-faint"
                    style={sidebarRevealStyle(draftRevealIndex, !isCollapsed)}
                  >
                    <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" strokeWidth={1.9} />
                    <span className="truncate">{t('loading')}</span>
                  </div>
                ) : null}
                {isCaseProjectLoading ? (
                  <div
                    className="ds-sidebar-reveal-item flex min-h-[28px] items-center gap-2 px-2.5 py-1.5 text-[12.5px] text-ds-faint"
                    style={sidebarRevealStyle(threadRevealOffset, !isCollapsed)}
                  >
                    <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" strokeWidth={1.9} />
                    <span className="truncate">{t('loading')}</span>
                  </div>
                ) : null}
                {caseProjectError ? (
                  <div
                    className="ds-sidebar-reveal-item px-2.5 py-1.5 text-[12.5px] leading-5 text-ds-danger"
                    style={sidebarRevealStyle(threadRevealOffset, !isCollapsed)}
                  >
                    {caseProjectError}
                  </div>
                ) : null}
                {draftHistory.length > 0 ? (
                  <div
                    className="ds-sidebar-reveal-item pb-[3px]"
                    style={sidebarRevealStyle(draftRevealIndex, !isCollapsed)}
                  >
                    <SddDraftHistoryRows
                      items={draftHistory}
                      activeDraftId={activeSddDraftId}
                      deletingDraftIds={deletingDraftIds}
                      error={draftHistoryErrors[workspacePath] ?? ''}
                      onOpen={onOpenRequirementDraft}
                      onDelete={(draft) => void handleDeleteRequirementDraft(draft)}
                      onContextMenu={openDraftContextMenu}
                      t={t}
                    />
                  </div>
                ) : null}
                {sortedThreads.length === 0 && draftHistory.length === 0 && !isCaseProjectLoading ? (
                  <div
                    className="ds-sidebar-reveal-item flex items-center justify-between gap-2 px-2.5 py-1.5"
                    style={sidebarRevealStyle(threadRevealOffset, !isCollapsed)}
                  >
                    <div className="text-[12.5px] leading-5 text-ds-faint">
                      {searchQuery.trim()
                        ? t('sidebarSearchEmpty')
                        : showArchived
                          ? t('sidebarArchiveEmpty')
                          : t('sidebarWorkspaceEmpty')}
                    </div>
                    {!showArchived && !searchQuery.trim() ? (
                      <button
                        type="button"
                        onClick={() => onCreateThreadInWorkspace(workspacePath)}
                        className="shrink-0 rounded-md px-2 py-1 text-[12px] font-medium text-ds-faint transition hover:bg-[var(--ds-sidebar-row-hover)] hover:text-ds-ink"
                      >
                        {t('sidebarWorkspaceNewThread')}
                      </button>
                    ) : null}
                  </div>
                ) : (
                  <>
                    {primaryThreads.map((thread, index) => renderThread(thread, index))}
                    {hasOverflow ? (
                      <SidebarFlowCollapse
                        open={workspaceExpanded && !isCollapsed}
                        className="ds-sidebar-thread-overflow"
                        innerClassName="flex flex-col gap-[3px]"
                        preserveAfterOpen
                      >
                        {overflowThreads.map((thread, index) =>
                          renderThread(
                            thread,
                            primaryThreads.length + index,
                            workspaceExpanded && !isCollapsed
                          )
                        )}
                      </SidebarFlowCollapse>
                    ) : null}
                  </>
                )}
                {hasOverflow ? (
                  <div
                    className="ds-sidebar-reveal-item ml-1 flex gap-1 py-1"
                    role="listitem"
                    style={sidebarRevealStyle(
                      threadRevealOffset + primaryThreads.length + overflowThreads.length + 1,
                      !isCollapsed
                    )}
                  >
                    {hasMoreWorkspaceThreads ? (
                      <button
                        type="button"
                        onClick={() =>
                          setExpandedWorkspacePages((current) => {
                            const nextPage = getNextSidebarWorkspaceThreadPage(
                              sortedThreads.length,
                              current[workspacePath] ?? 0
                            )
                            if (nextPage <= 0) {
                              const next = { ...current }
                              delete next[workspacePath]
                              return next
                            }
                            return {
                              ...current,
                              [workspacePath]: nextPage
                            }
                          })
                        }
                        className="rounded-md px-2.5 py-1 text-[12.5px] leading-5 text-ds-faint transition hover:text-ds-ink"
                      >
                        {t('sidebarWorkspaceShowMore')}
                      </button>
                    ) : null}
                    {workspaceExpanded ? (
                      <button
                        type="button"
                        onClick={() =>
                          setExpandedWorkspacePages((current) => {
                            const next = { ...current }
                            delete next[workspacePath]
                            return next
                          })
                        }
                        className="rounded-md px-2.5 py-1 text-[12.5px] leading-5 text-ds-faint transition hover:text-ds-ink"
                      >
                        {t('sidebarWorkspaceShowLess')}
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </SidebarCollapseMotion>
            </div>
          )
        })}
      </div>

      {contextMenuPortalTarget && threadContextMenu ? createPortal(
        <ThreadContextMenu
          state={threadContextMenu}
          busy={deletingThreadIds[threadContextMenu.thread.id] === true}
          runtimeReady={runtimeReady}
          activePinned={pinnedThreadIds.has(threadContextMenu.thread.id)}
          submenu={contextSubmenu}
          onSubmenuChange={setContextSubmenu}
          onClose={closeContextMenus}
          onTogglePin={() => {
            togglePinnedThreadId(threadContextMenu.thread.id)
            setPinnedThreadIds(readPinnedThreadIds())
          }}
          onRename={() => openRenameThreadDialog(threadContextMenu.thread)}
          onArchive={() => void handleArchiveThread(threadContextMenu.thread)}
          onDelete={() => void handleDeleteThread(threadContextMenu.thread)}
          onRestore={() => void handleRestoreThread(threadContextMenu.thread)}
          onOpenSideChat={() => void onOpenSideChat(threadContextMenu.thread.id)}
          onCopyMarkdown={() => void copyThreadMarkdown(threadContextMenu.thread)}
          onCopyThreadId={() => void copyText(threadContextMenu.thread.id)}
          onCopyWorkspacePath={() => void copyText(threadContextMenu.thread.workspace ?? '')}
          onCopyDeeplink={() => void copyText(buildAnalytixThreadLink(threadContextMenu.thread.id))}
          onCopyCodexLink={() => void copyText(buildCodexCompatibleThreadLink(threadContextMenu.thread.id))}
          onFork={() => void onForkThread(threadContextMenu.thread.id)}
          onAddAutomation={onOpenSchedule}
          onOpenInNewWindow={() => {
            if (typeof window.analytix?.app?.openThreadInNewWindow !== 'function') return
            void window.analytix.app.openThreadInNewWindow(threadContextMenu.thread.id).catch(() => undefined)
          }}
          t={t}
        />,
        contextMenuPortalTarget
      ) : null}

      {contextMenuPortalTarget && workspaceContextMenu ? createPortal(
        <WorkspaceContextMenu
          state={workspaceContextMenu}
          onClose={closeContextMenus}
          onNewThread={() => onCreateThreadInWorkspace(workspaceContextMenu.workspacePath)}
          onOpenFolder={() =>
            openPathWithEditor(workspaceContextMenu.workspacePath, workspaceContextMenu.workspacePath, 'system')
          }
          onCopyPath={() => void copyText(workspaceContextMenu.workspacePath)}
          onReveal={() =>
            openPathWithEditor(workspaceContextMenu.workspacePath, workspaceContextMenu.workspacePath, 'finder')
          }
          onRemove={() => void handleRemoveWorkspace(workspaceContextMenu.workspacePath)}
          t={t}
        />,
        contextMenuPortalTarget
      ) : null}

      {contextMenuPortalTarget && draftContextMenu ? createPortal(
        <DraftContextMenu
          state={draftContextMenu}
          busy={deletingDraftIds[draftContextMenu.draft.id] === true}
          onClose={closeContextMenus}
          onOpen={() => onOpenRequirementDraft(draftContextMenu.draft)}
          onOpenEditor={() =>
            openPathWithEditor(
              draftContextMenu.draft.absolutePath ?? draftContextMenu.draft.relativePath,
              draftContextMenu.draft.workspaceRoot,
              'system'
            )
          }
          onCopyPath={() => void copyText(draftContextMenu.draft.absolutePath ?? draftContextMenu.draft.relativePath)}
          onCopyContents={() => void copyDraftContents(draftContextMenu.draft)}
          onReveal={() =>
            openPathWithEditor(
              draftContextMenu.draft.absolutePath ?? draftContextMenu.draft.relativePath,
              draftContextMenu.draft.workspaceRoot,
              'finder'
            )
          }
          onDelete={() => void handleDeleteRequirementDraft(draftContextMenu.draft)}
          t={t}
        />,
        contextMenuPortalTarget
      ) : null}

      {renameThreadDialog ? (
        <ThreadRenameDialog
          state={renameThreadDialog}
          onClose={closeRenameThreadDialog}
          onValueChange={(value) =>
            setRenameThreadDialog((current) => current ? { ...current, value } : current)
          }
          onSubmit={(event) => void submitRenameThreadDialog(event)}
          t={t}
        />
      ) : null}
    </div>
  )
}

type ThreadRowProps = {
  thread: NormalizedThread
  active: boolean
  deleting: boolean
  locale: string
  showRunning: boolean
  showUnread: boolean
  pinned: boolean
  onSelect: () => void
  onContextMenu: (event: ReactMouseEvent<HTMLDivElement>) => void
  onRename: () => void
  onArchive: () => void
  onDelete: () => void
  onRestore: () => void
}

export function DataAnalysisWorkspaceRows({
  activeItemId,
  onOpen,
  t
}: {
  activeItemId: DataAnalysisSidebarItemId | null
  onOpen: (itemId: DataAnalysisSidebarItemId) => void
  t: (k: string, opts?: Record<string, unknown>) => string
}): ReactElement {
  const [collapsed, setCollapsed] = useState(true)

  return (
    <div className="rounded-lg border border-transparent bg-[var(--ds-sidebar-row-hover)]/35 px-1 py-1">
      <SidebarTreeRow
        title={t('sidebarDataAnalysisTitle')}
        ariaLabel={collapsed ? t('sidebarDataAnalysisExpand') : t('sidebarDataAnalysisCollapse')}
        onClick={() => setCollapsed((current) => !current)}
        className="min-h-[28px]"
        buttonClassName="items-center gap-1.5 px-2 py-1.5"
      >
        {collapsed ? (
          <AnalytixIconRegistry.icons.disclosureRight className="h-3 w-3 shrink-0 text-ds-faint" />
        ) : (
          <AnalytixIconRegistry.icons.disclosureDown className="h-3 w-3 shrink-0 text-ds-faint" />
        )}
        <SquareStack className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.85} />
        <span className="min-w-0 flex-1 truncate text-[11.5px] font-medium text-ds-faint">
          {t('sidebarDataAnalysisTitle')}
        </span>
        <span className="shrink-0 rounded-md bg-ds-card/70 px-1.5 py-0.5 text-[10.5px] text-ds-faint tabular-nums">
          {DATA_ANALYSIS_SIDEBAR_ITEMS.length}
        </span>
      </SidebarTreeRow>
      <SidebarFlowCollapse
        open={!collapsed}
        className="ds-sidebar-data-analysis-children"
        innerClassName="flex flex-col gap-[2px] pt-1"
        preserveAfterOpen
      >
        {DATA_ANALYSIS_SIDEBAR_ITEMS.map((item, index) => {
          const Icon = item.Icon
          return (
            <div
              key={item.id}
              className="ds-sidebar-reveal-item"
              style={sidebarRevealStyle(index, !collapsed)}
            >
              <SidebarTreeRow
                active={activeItemId === item.id}
                activeVariant="outline"
                className="min-h-[30px]"
                buttonClassName="items-center gap-2 px-2 py-1.5"
                title={t(item.titleKey)}
                ariaLabel={t(item.titleKey)}
                onClick={() => onOpen(item.id)}
              >
                <span
                  className={`grid h-6 w-6 shrink-0 place-items-center rounded-md border transition ${
                    activeItemId === item.id
                      ? 'border-accent/25 bg-accent/10 text-accent shadow-[inset_0_1px_0_rgba(255,255,255,0.55)]'
                      : 'border-ds-border-muted bg-ds-card/70 text-ds-faint group-hover:border-accent/20 group-hover:bg-accent/10 group-hover:text-accent'
                  }`}
                  aria-hidden="true"
                >
                  <Icon className="h-3.5 w-3.5" strokeWidth={1.9} />
                </span>
                <span className="min-w-0 flex-1 truncate text-[13px] leading-4 text-ds-ink">
                  {t(item.labelKey)}
                </span>
              </SidebarTreeRow>
            </div>
          )
        })}
      </SidebarFlowCollapse>
    </div>
  )
}

export function SddDraftHistoryRows({
  items,
  activeDraftId,
  onOpen,
  onDelete,
  onContextMenu,
  deletingDraftIds = {},
  error = '',
  t
}: {
  items: SddDraftHistoryItem[]
  activeDraftId: string
  onOpen: (draft: SddDraft) => void
  onDelete?: (draft: SddDraftHistoryItem) => void
  onContextMenu?: (event: ReactMouseEvent<HTMLDivElement>, draft: SddDraftHistoryItem) => void
  deletingDraftIds?: Record<string, boolean>
  error?: string
  t: (k: string, opts?: Record<string, unknown>) => string
}): ReactElement | null {
  const itemKey = items.map((item) => item.id).join('\n')
  const [collapsed, setCollapsed] = useState(true)
  const [visibleCount, setVisibleCount] = useState(SDD_DRAFT_HISTORY_PAGE_SIZE)

  useEffect(() => {
    setCollapsed(true)
    setVisibleCount(SDD_DRAFT_HISTORY_PAGE_SIZE)
  }, [itemKey])

  if (items.length === 0) return null

  const visibleItems = items.slice(0, visibleCount)
  const remainingCount = Math.max(0, items.length - visibleItems.length)
  const nextCount = Math.min(SDD_DRAFT_HISTORY_PAGE_SIZE, remainingCount)

  return (
    <div className="rounded-lg border border-transparent bg-[var(--ds-sidebar-row-hover)]/35 px-1 py-1">
      <SidebarTreeRow
        title={t('sddDraftHistoryTitle')}
        ariaLabel={collapsed ? t('sddDraftHistoryExpand') : t('sddDraftHistoryCollapse')}
        onClick={() => setCollapsed((current) => !current)}
        className="min-h-[28px]"
        buttonClassName="items-center gap-1.5 px-2 py-1.5"
      >
        {collapsed ? (
          <AnalytixIconRegistry.icons.disclosureRight className="h-3 w-3 shrink-0 text-ds-faint" />
        ) : (
          <AnalytixIconRegistry.icons.disclosureDown className="h-3 w-3 shrink-0 text-ds-faint" />
        )}
        <span className="min-w-0 flex-1 truncate text-[11.5px] font-medium text-ds-faint">
          {t('sddDraftHistoryTitle')}
        </span>
        <span className="shrink-0 rounded-md bg-ds-card/70 px-1.5 py-0.5 text-[10.5px] text-ds-faint tabular-nums">
          {items.length}
        </span>
      </SidebarTreeRow>
      {error ? (
        <div className="px-2 py-1 text-[11.5px] leading-4 text-red-600 dark:text-red-300">
          {error}
        </div>
      ) : null}
      <SidebarFlowCollapse
        open={!collapsed}
        className="ds-sidebar-draft-children"
        innerClassName="flex flex-col gap-[2px] pt-1"
        preserveAfterOpen
      >
        {visibleItems.map((item, index) => (
          <div
            key={item.id}
            className="ds-sidebar-reveal-item"
            style={sidebarRevealStyle(index, !collapsed)}
          >
            <SidebarTreeRow
              active={activeDraftId === item.id}
              activeVariant="outline"
              actionsVisibility={deletingDraftIds[item.id] ? 'visible' : 'hidden'}
              actionsLayout="overlay"
              actions={
                onDelete ? (
                  <SidebarIconButton
                    onClick={() => onDelete(item)}
                    disabled={deletingDraftIds[item.id] === true}
                    tone="danger"
                    title={t('sddDraftHistoryDelete')}
                    ariaLabel={t('sddDraftHistoryDelete')}
                    stopPropagation
                  >
                    {deletingDraftIds[item.id] ? (
                      <Loader2 className="h-3 w-3 animate-spin" strokeWidth={2} />
                    ) : (
                      <Trash2 className="h-3 w-3" strokeWidth={1.9} />
                    )}
                  </SidebarIconButton>
                ) : null
              }
              className="min-h-[32px]"
              buttonClassName="items-center gap-2 px-2 py-1.5"
              title={item.relativePath}
              ariaLabel={t('sddDraftHistoryOpen', { title: item.title })}
              onClick={() => onOpen(item)}
              onContextMenu={(event) => onContextMenu?.(event, item)}
            >
              <span
                className={`grid h-7 w-7 shrink-0 place-items-center rounded-lg border transition ${
                  activeDraftId === item.id
                    ? 'border-accent/25 bg-accent/10 text-accent shadow-[inset_0_1px_0_rgba(255,255,255,0.55)]'
                    : 'border-ds-border-muted bg-ds-card/70 text-ds-faint group-hover:border-accent/20 group-hover:bg-accent/10 group-hover:text-accent'
                }`}
                aria-hidden="true"
              >
                <ClipboardList className="h-4 w-4" strokeWidth={1.9} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[13px] leading-4 text-ds-ink">{item.title}</span>
                <span className="block truncate text-[11.5px] leading-4 text-ds-faint">{item.relativePath}</span>
              </span>
              <span className="shrink-0 rounded-md bg-ds-card/70 px-1.5 py-0.5 text-[10.5px] text-ds-faint transition group-hover:opacity-0 group-focus-within:opacity-0">
                {item.source === 'remembered' ? t('sddDraftHistoryRemembered') : t('sddDraftHistoryDisk')}
              </span>
            </SidebarTreeRow>
          </div>
        ))}
        {remainingCount > 0 ? (
          <button
            type="button"
            onClick={() =>
              setVisibleCount((count) => Math.min(items.length, count + SDD_DRAFT_HISTORY_PAGE_SIZE))
            }
            className="ds-sidebar-reveal-item ml-1 mt-1 rounded-md px-2.5 py-1.5 text-[12.5px] text-ds-faint hover:bg-[var(--ds-sidebar-row-hover)] hover:text-ds-ink"
            style={sidebarRevealStyle(visibleItems.length, !collapsed)}
          >
            {t('sddDraftHistoryShowMore', { count: nextCount })}
          </button>
        ) : null}
      </SidebarFlowCollapse>
    </div>
  )
}

function ThreadRow({
  thread,
  active,
  deleting,
  locale,
  showRunning,
  showUnread,
  pinned,
  onSelect,
  onContextMenu,
  onRename,
  onArchive,
  onDelete,
  onRestore
}: ThreadRowProps): ReactElement {
  const { t } = useTranslation('common')
  const showUnreadDot = showUnread && !showRunning
  const archived = thread.archived === true
  const PinIcon = AnalytixIconRegistry.icons.pinFilled
  const forkedFromTitle = thread.forkedFromTitle?.trim() ?? ''
  const forked = Boolean(thread.forkedFromThreadId)
  const forkLabel = forked
    ? forkedFromTitle
      ? t('sidebarThreadForkedFrom', { title: forkedFromTitle })
      : t('sidebarThreadForked')
    : ''
  const updatedLabel = formatRelativeTime(thread.updatedAt, locale)
  const ariaLabel = [
    thread.title,
    updatedLabel,
    showRunning ? t('sidebarThreadRunning') : '',
    showUnreadDot ? t('sidebarThreadUnread') : '',
    forkLabel
  ].filter(Boolean).join(' — ')

  return (
    <SidebarTreeRow
      active={active}
      actionsVisibility={deleting ? 'visible' : 'hidden'}
      actionsLayout="overlay"
      actions={
        <>
          <SidebarIconButton
            onClick={archived ? onRestore : onArchive}
            disabled={deleting}
            tone="accent"
            title={archived ? t('sidebarThreadRestore') : t('sidebarThreadArchive')}
            ariaLabel={archived ? t('sidebarThreadRestore') : t('sidebarThreadArchive')}
            stopPropagation
          >
            {deleting ? (
              <Loader2 className="h-3 w-3 animate-spin" strokeWidth={2} />
            ) : archived ? (
              <RotateCcw className="h-3 w-3" strokeWidth={1.9} />
            ) : (
              <Archive className="h-3 w-3" strokeWidth={1.9} />
            )}
          </SidebarIconButton>
          <SidebarIconButton
            onClick={onDelete}
            disabled={deleting}
            tone="danger"
            title={t('sidebarThreadDelete')}
            ariaLabel={t('sidebarThreadDelete')}
            stopPropagation
          >
            <Trash2 className="h-3 w-3" strokeWidth={1.9} />
          </SidebarIconButton>
        </>
      }
      className="min-h-[34px]"
      buttonClassName="items-center gap-2 px-2.5 py-1.5"
      disabled={deleting}
      ariaLabel={ariaLabel}
      title={forkLabel ? `${thread.title}\n${forkLabel}` : thread.title}
      onClick={onSelect}
      onContextMenu={onContextMenu}
    >
      {pinned ? (
        <PinIcon
          className={`h-3.5 w-3.5 shrink-0 ${active ? 'text-accent' : 'text-ds-faint/90'}`}
        />
      ) : forked ? (
        <GitFork
          className={`h-3.5 w-3.5 shrink-0 ${active ? 'text-accent' : 'text-ds-faint/90'}`}
          strokeWidth={1.8}
        />
      ) : null}
      <span className="flex min-w-0 flex-1 items-center gap-1.5">
        <span
          className={`min-w-0 flex-1 truncate text-[13.5px] leading-5 ${
            showUnreadDot && !active ? 'font-semibold text-ds-ink' : 'text-ds-ink'
          }`}
        >
          {thread.title}
        </span>
        {forked ? (
          <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-accent/15 bg-accent/8 px-1.5 py-0.5 text-[10.5px] font-semibold leading-none text-accent">
            <GitFork className="h-2.5 w-2.5" strokeWidth={1.8} />
            {t('sidebarThreadForkBadge')}
          </span>
        ) : null}
        <span
          className={`ml-auto flex shrink-0 items-center gap-1.5 transition ${
            deleting ? 'opacity-0' : 'group-hover:opacity-0 group-focus-within:opacity-0'
          }`}
        >
          <span className="shrink-0 text-right text-[12px] leading-4 text-ds-faint tabular-nums">
            {updatedLabel}
          </span>
          <ThreadActivityDot
            running={showRunning}
            unread={showUnreadDot}
            unreadLabel={t('sidebarThreadUnread')}
          />
        </span>
      </span>
    </SidebarTreeRow>
  )
}

export function ThreadRenameDialog({
  state,
  onClose,
  onValueChange,
  onSubmit,
  t
}: {
  state: RenameThreadDialogState
  onClose: () => void
  onValueChange: (value: string) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  t: (k: string, opts?: Record<string, unknown>) => string
}): ReactElement {
  const nextTitle = state.value.trim()
  const unchanged = nextTitle === state.thread.title
  const canSubmit = Boolean(nextTitle) && !unchanged && !state.submitting

  useEffect(() => {
    if (state.submitting) return
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, state.submitting])

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="thread-rename-dialog-title"
      className="ds-no-drag fixed inset-0 z-[80] flex items-center justify-center bg-slate-950/18 px-4 backdrop-blur-[2px] dark:bg-black/35"
      onMouseDown={onClose}
    >
      <form
        onSubmit={onSubmit}
        onMouseDown={(event) => event.stopPropagation()}
        className="w-full max-w-sm rounded-[24px] border border-ds-border bg-ds-card p-5 shadow-[0_24px_72px_rgba(20,47,95,0.22)]"
      >
        <h2
          id="thread-rename-dialog-title"
          className="text-[18px] font-semibold tracking-[-0.035em] text-ds-ink"
        >
          {t('sidebarThreadRename')}
        </h2>
        <p className="mt-2 text-[13px] leading-6 text-ds-muted">
          {t('sidebarThreadRenamePrompt')}
        </p>
        <input
          autoFocus
          aria-label={t('sidebarThreadRenamePrompt')}
          disabled={state.submitting}
          value={state.value}
          onChange={(event) => onValueChange(event.target.value)}
          onFocus={(event) => event.currentTarget.select()}
          className="mt-4 w-full rounded-xl border border-ds-border bg-ds-main/65 px-3 py-2 text-[14px] text-ds-ink outline-none transition focus:border-accent/40 focus:ring-1 focus:ring-accent/25 disabled:cursor-wait disabled:opacity-70"
        />
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            disabled={state.submitting}
            onClick={onClose}
            className="rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-wait disabled:opacity-60"
          >
            {t('cancel')}
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            className="rounded-xl bg-accent px-3 py-2 text-[13px] font-semibold text-white transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-55"
          >
            {state.submitting ? t('loading') : t('confirm')}
          </button>
        </div>
      </form>
    </div>
  )
}

function ThreadContextMenu({
  state,
  busy,
  runtimeReady,
  activePinned,
  submenu,
  onSubmenuChange,
  onClose,
  onTogglePin,
  onRename,
  onArchive,
  onDelete,
  onRestore,
  onOpenSideChat,
  onCopyMarkdown,
  onCopyThreadId,
  onCopyWorkspacePath,
  onCopyDeeplink,
  onCopyCodexLink,
  onFork,
  onAddAutomation,
  onOpenInNewWindow,
  t
}: {
  state: ThreadContextMenuState
  busy: boolean
  runtimeReady: boolean
  activePinned: boolean
  submenu: SidebarContextSubmenu
  onSubmenuChange: (submenu: SidebarContextSubmenu) => void
  onClose: () => void
  onTogglePin: () => void
  onRename: () => void
  onArchive: () => void
  onDelete: () => void
  onRestore: () => void
  onOpenSideChat: () => void
  onCopyMarkdown: () => void
  onCopyThreadId: () => void
  onCopyWorkspacePath: () => void
  onCopyDeeplink: () => void
  onCopyCodexLink: () => void
  onFork: () => void
  onAddAutomation: () => void
  onOpenInNewWindow: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}): ReactElement {
  const archived = state.thread.archived === true
  const PinActionIcon = activePinned ? AnalytixIconRegistry.icons.pinFilled : AnalytixIconRegistry.icons.pin
  const run = (action: () => void): void => {
    onClose()
    action()
  }

  return (
    <div
      role="menu"
      aria-label={state.thread.title}
      className="ds-session-actions-menu ds-sidebar-context-menu ds-no-drag"
      style={{ left: state.x, top: state.y }}
      onPointerDown={(event) => event.stopPropagation()}
      onPointerLeave={() => onSubmenuChange(null)}
    >
      <ActionMenuItem
        icon={<PinActionIcon className="h-4 w-4" />}
        label={activePinned ? t('sessionActionUnpin') : t('sessionActionPin')}
        shortcut="⌥⌘P"
        active={activePinned}
        onClick={() => run(onTogglePin)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
      <ActionMenuItem
        icon={<PencilLine className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sessionActionRename')}
        shortcut="⌥⌘R"
        disabled={busy}
        title={busy ? t('threadActionBusy') : undefined}
        onClick={() => run(onRename)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
      <ActionMenuItem
        icon={
          archived
            ? <RotateCcw className="h-4 w-4" strokeWidth={1.9} />
            : <Archive className="h-4 w-4" strokeWidth={1.9} />
        }
        label={archived ? t('sessionActionRestore') : t('sessionActionArchive')}
        shortcut="⇧⌘A"
        disabled={busy}
        title={busy ? t('threadActionBusy') : undefined}
        onClick={() => run(archived ? onRestore : onArchive)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<MessageCirclePlus className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sessionActionOpenSideChat')}
        shortcut="⌥⌘S"
        disabled={!runtimeReady}
        title={!runtimeReady ? t('runtimeActionNeedsConnection') : undefined}
        onClick={() => run(onOpenSideChat)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
      <div className="ds-session-actions-menu-submenu-anchor">
        <ActionMenuItem
          icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
          label={t('sessionActionCopy')}
          hasSubmenu
          onClick={() => onSubmenuChange(submenu === 'copy' ? null : 'copy')}
          onPointerEnter={() => onSubmenuChange('copy')}
        />
        {submenu === 'copy' ? (
          <div className="ds-session-actions-submenu" role="menu" aria-label={t('sessionActionCopy')}>
            <ActionMenuItem
              icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sessionActionCopyMarkdown')}
              disabled={!runtimeReady}
              title={!runtimeReady ? t('runtimeActionNeedsConnection') : undefined}
              onClick={() => run(onCopyMarkdown)}
            />
            <ActionMenuItem
              icon={<SquareStack className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sessionActionCopyThreadId')}
              onClick={() => run(onCopyThreadId)}
            />
            <ActionMenuItem
              icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sessionActionCopyWorkspacePath')}
              onClick={() => run(onCopyWorkspacePath)}
            />
            <ActionMenuItem
              icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sessionActionCopyDeeplink')}
              onClick={() => run(onCopyDeeplink)}
            />
            <ActionMenuItem
              icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sessionActionCopyCodexLink')}
              onClick={() => run(onCopyCodexLink)}
            />
          </div>
        ) : null}
      </div>
      <div className="ds-session-actions-menu-submenu-anchor">
        <ActionMenuItem
          icon={<GitBranch className="h-4 w-4" strokeWidth={1.9} />}
          label={t('sessionActionBranch')}
          hasSubmenu
          disabled={!runtimeReady || busy}
          title={busy ? t('threadActionBusy') : !runtimeReady ? t('runtimeActionNeedsConnection') : undefined}
          onClick={() => onSubmenuChange(submenu === 'branch' ? null : 'branch')}
          onPointerEnter={() => onSubmenuChange('branch')}
        />
        {submenu === 'branch' ? (
          <div className="ds-session-actions-submenu" role="menu" aria-label={t('sessionActionBranch')}>
            <ActionMenuItem
              icon={<GitFork className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sessionActionForkCurrent')}
              disabled={!runtimeReady || busy}
              onClick={() => run(onFork)}
            />
          </div>
        ) : null}
      </div>
      <ActionMenuItem
        icon={<Clock3 className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sessionActionAddAutomation')}
        onClick={() => run(onAddAutomation)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<SquareStack className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sessionActionOpenInNewWindow')}
        disabled={!runtimeReady}
        title={!runtimeReady ? t('runtimeActionNeedsConnection') : undefined}
        onClick={() => run(onOpenInNewWindow)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<Trash2 className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarThreadDelete')}
        disabled={busy}
        danger
        title={busy ? t('threadActionBusy') : undefined}
        onClick={() => run(onDelete)}
        onPointerEnter={() => onSubmenuChange(null)}
      />
    </div>
  )
}

function WorkspaceContextMenu({
  state,
  onClose,
  onNewThread,
  onOpenFolder,
  onCopyPath,
  onReveal,
  onRemove,
  t
}: {
  state: WorkspaceContextMenuState
  onClose: () => void
  onNewThread: () => void
  onOpenFolder: () => void
  onCopyPath: () => void
  onReveal: () => void
  onRemove: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}): ReactElement {
  const run = (action: () => void): void => {
    onClose()
    action()
  }

  return (
    <div
      role="menu"
      aria-label={state.workspacePath}
      className="ds-session-actions-menu ds-sidebar-context-menu ds-no-drag"
      style={{ left: state.x, top: state.y }}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <ActionMenuItem
        icon={<Plus className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarWorkspaceNewThread')}
        onClick={() => run(onNewThread)}
      />
      <ActionMenuItem
        icon={<FolderOpen className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarContextOpenFolder')}
        onClick={() => run(onOpenFolder)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
        label={t('filePreviewCopyPath')}
        onClick={() => run(onCopyPath)}
      />
      <ActionMenuItem
        icon={<FolderOpen className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarContextRevealInFinder')}
        onClick={() => run(onReveal)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<Trash2 className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarWorkspaceRemove')}
        danger
        onClick={() => run(onRemove)}
      />
    </div>
  )
}

function DraftContextMenu({
  state,
  busy,
  onClose,
  onOpen,
  onOpenEditor,
  onCopyPath,
  onCopyContents,
  onReveal,
  onDelete,
  t
}: {
  state: DraftContextMenuState
  busy: boolean
  onClose: () => void
  onOpen: () => void
  onOpenEditor: () => void
  onCopyPath: () => void
  onCopyContents: () => void
  onReveal: () => void
  onDelete: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}): ReactElement {
  const run = (action: () => void): void => {
    onClose()
    action()
  }

  return (
    <div
      role="menu"
      aria-label={state.draft.title}
      className="ds-session-actions-menu ds-sidebar-context-menu ds-no-drag"
      style={{ left: state.x, top: state.y }}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <ActionMenuItem
        icon={<FileText className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarContextOpenDraft')}
        onClick={() => run(onOpen)}
      />
      <ActionMenuItem
        icon={<FolderOpen className="h-4 w-4" strokeWidth={1.9} />}
        label={t('filePreviewOpenEditor')}
        onClick={() => run(onOpenEditor)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<Copy className="h-4 w-4" strokeWidth={1.9} />}
        label={t('filePreviewCopyPath')}
        onClick={() => run(onCopyPath)}
      />
      <ActionMenuItem
        icon={<ClipboardList className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarContextCopyFileContents')}
        onClick={() => run(onCopyContents)}
      />
      <ActionMenuItem
        icon={<FolderOpen className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sidebarContextRevealInFinder')}
        onClick={() => run(onReveal)}
      />
      <ActionMenuSeparator />
      <ActionMenuItem
        icon={<Trash2 className="h-4 w-4" strokeWidth={1.9} />}
        label={t('sddDraftHistoryDelete')}
        disabled={busy}
        danger
        onClick={() => run(onDelete)}
      />
    </div>
  )
}

function workspaceContextLabel(workspacePath: string, folderName: string): string {
  const normalized = workspacePath.replace(/[/\\]+$/, '')
  const parts = normalized.split(/[/\\]/).filter(Boolean)
  if (parts.length < 2) return ''
  const parent = parts[parts.length - 2] ?? ''
  if (!parent || parent.toLowerCase() === folderName.toLowerCase()) return ''
  return parent
}

function ThreadActivityDot({
  running,
  unread,
  unreadLabel
}: {
  running: boolean
  unread: boolean
  unreadLabel: string
}): ReactElement | null {
  if (running) {
    return <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-accent" strokeWidth={2} />
  }

  if (unread) {
    return (
      <span
        className="block h-2 w-2 shrink-0 rounded-full bg-accent shadow-[0_0_0_1px_rgba(79,124,255,0.2)]"
        title={unreadLabel}
      />
    )
  }

  return null
}

type SidebarEmptyProps = {
  runtimeReady: boolean
  hasWorkspace: boolean
  onPickWorkspace: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}

function SidebarEmpty({
  runtimeReady,
  hasWorkspace,
  onPickWorkspace,
  t
}: SidebarEmptyProps): ReactElement {
  if (!hasWorkspace && runtimeReady) {
    return (
      <button
        type="button"
        onClick={onPickWorkspace}
        className="mx-1 mt-1 flex w-[calc(100%-0.5rem)] items-center gap-2 rounded-lg px-2 py-1.5 text-left text-ds-muted transition hover:bg-[var(--ds-sidebar-row-hover)] hover:text-ds-ink"
      >
        <FolderPlus className="h-4 w-4 shrink-0 text-accent" strokeWidth={1.75} />
        <span className="min-w-0 flex-1 truncate text-[14px] font-medium">
          {t('selectWorkspace')}
        </span>
      </button>
    )
  }

  return (
    <div className="mx-2 mt-2 rounded-lg px-2 py-2">
      <p className="text-[15px] font-medium text-ds-muted">{t('sidebarEmptyTitle')}</p>
      <p className="mt-1 text-[13px] leading-5 text-ds-faint">
        {runtimeReady ? t('sidebarEmptySub') : t('sidebarEmptySubOffline')}
      </p>
    </div>
  )
}
