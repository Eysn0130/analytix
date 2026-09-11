import { createElement, type ComponentProps } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { NormalizedThread } from '../../agent/types'
import type { SddDraftHistoryItem } from '../../sdd/sdd-draft-history'
import { sidebarWorkspaceCollapseKey } from '../../lib/sidebar-collapsed-workspaces'
import {
	  buildSidebarPinnedThreadOrder,
	  buildSidebarCaseProjectGroups,
	  buildSidebarDraftWorkspacePaths,
	  SidebarProjectsSection,
	  buildExpandedSidebarDraftWorkspacePaths,
  buildSidebarWorkspaceGroups,
  compareSidebarThreadsWithPins,
  DataAnalysisWorkspaceRows,
  filterEmptySddAssistantThreadsFromSidebar,
  filterSddDraftHistoryItems,
  getNextSidebarWorkspaceThreadPage,
  getSidebarWorkspaceVisibleThreadLimit,
  isSidebarCaseProjectCollapsed,
  isSidebarWorkspaceCollapsed,
  mergeSidebarWorkspaceGroupsWithDraftHistory,
  SddDraftHistoryRows,
  ThreadRenameDialog
} from './SidebarProjectsSection'
import sidebarProjectsSectionSource from './SidebarProjectsSection.tsx?raw'

function thread(overrides: Partial<NormalizedThread> & Pick<NormalizedThread, 'id' | 'workspace'>): NormalizedThread {
  return {
    id: overrides.id,
    title: overrides.title ?? overrides.id,
    updatedAt: overrides.updatedAt ?? '2026-06-01T00:00:00.000Z',
    model: overrides.model ?? 'reasonix',
    mode: overrides.mode ?? 'agent',
    workspace: overrides.workspace,
    ...(overrides.preview ? { preview: overrides.preview } : {}),
    ...(overrides.latestTurnId ? { latestTurnId: overrides.latestTurnId } : {}),
    ...(overrides.status ? { status: overrides.status } : {}),
    ...(overrides.archived !== undefined ? { archived: overrides.archived } : {})
  }
}

function draft(overrides: Partial<SddDraftHistoryItem> & Pick<SddDraftHistoryItem, 'id' | 'title'>): SddDraftHistoryItem {
  const folder = overrides.id.replace(/[^a-z0-9-]/gi, '').slice(0, 36).padEnd(36, '0')
  return {
    id: overrides.id,
    workspaceRoot: overrides.workspaceRoot ?? '/tmp/app',
    relativePath: overrides.relativePath ?? `.analytixsdd/draft/${folder}/requirement.md`,
    createdAt: overrides.createdAt ?? '2026-01-01T00:00:00.000Z',
    updatedAt: overrides.updatedAt ?? '2026-01-02T00:00:00.000Z',
    title: overrides.title,
    source: overrides.source ?? 'remembered',
    ...(overrides.chatThreadIds ? { chatThreadIds: overrides.chatThreadIds } : {}),
    ...(overrides.searchText ? { searchText: overrides.searchText } : {})
  }
}

function renderSidebarProjectsSection(
  overrides: Partial<ComponentProps<typeof SidebarProjectsSection>> = {}
): string {
  const noop = vi.fn()
  return renderToStaticMarkup(
    createElement(SidebarProjectsSection, {
      threads: [],
      caseProjects: [],
      caseProjectIndexStatus: 'ready',
      caseProjectThreadsById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectExpandedById: {},
      activeView: 'chat',
      activeThreadId: null,
      runtimeReady: true,
      searchQuery: '',
      showArchived: false,
      workspaceRoot: '',
      workspaceRoots: [],
      busy: false,
      watchTurnCompletion: {},
      unreadThreadIds: {},
      locale: 'en',
      onPickWorkspace: noop,
      onRemoveWorkspace: vi.fn(async () => undefined),
      onCreateThreadInWorkspace: noop,
      onOpenRequirementDraft: noop,
      activeDataAnalysisEntry: null,
      onOpenDataAnalysis: noop,
      onSelectThread: noop,
      onRenameThread: vi.fn(async () => undefined),
      onArchiveThread: vi.fn(async () => undefined),
      onDeleteThread: vi.fn(async () => undefined),
      onRestoreThread: vi.fn(async () => undefined),
      onOpenSideChat: noop,
      onForkThread: noop,
      onOpenSchedule: noop,
      onSearchQueryChange: noop,
      onSetCaseProjectExpanded: noop,
      t: (key: string) => key,
      ...overrides
    })
  )
}

describe('SidebarProjectsSection groups', () => {
  it('keeps the project scrollbar aligned near the sidebar divider', () => {
    expect(sidebarProjectsSectionSource).toContain('ds-sidebar-projects-scroll')
  })

  it('keeps remembered code workspaces visible even when the runtime lists only one workspace', () => {
    const groups = buildSidebarWorkspaceGroups({
      threads: [thread({ id: 'reasonix-current', workspace: '/Users/zxy/project-a' })],
      searchQuery: '',
      showArchived: false,
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: [
        '/Users/zxy/project-a',
        '/Users/zxy/project-b',
        '/Users/zxy/project-c'
      ]
    })

    expect(groups.map(([workspace]) => workspace)).toEqual([
      '/Users/zxy/project-a',
      '/Users/zxy/project-b',
      '/Users/zxy/project-c'
    ])
    expect(groups[1]?.[1]).toEqual([])
    expect(groups[2]?.[1]).toEqual([])
	  })

  it('builds case project parent groups without requiring global thread rows', () => {
    const groups = buildSidebarCaseProjectGroups({
      caseProjects: [{
        id: 'case_a',
        name: 'project-a',
        rootPath: '/Users/zxy/project-a',
        updatedAt: '2026-06-12T00:00:00.000Z',
        threadCount: 5000,
        runningCount: 0,
        archivedCount: 0,
        status: 'ready'
      }],
      caseProjectThreadsById: {},
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: []
    })

    expect(groups).toEqual([['/Users/zxy/project-a', []]])
  })

  it('merges recent runtime threads into case project groups', () => {
    const groups = buildSidebarCaseProjectGroups({
      caseProjects: [{
        id: 'case_a',
        name: 'project-a',
        rootPath: '/Users/zxy/project-a',
        updatedAt: '2026-06-12T00:00:00.000Z',
        threadCount: 2,
        runningCount: 0,
        archivedCount: 0,
        status: 'ready'
      }],
      caseProjectThreadsById: {
        case_a: [thread({ id: 'loaded-thread', workspace: '/Users/zxy/project-a' })]
      },
      threads: [thread({ id: 'active-thread', workspace: '/Users/zxy/project-a' })],
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: []
    })

    expect(groups).toHaveLength(1)
    expect(groups[0]?.[1].map((item) => item.id)).toEqual(['loaded-thread', 'active-thread'])
  })

  it('shows loading instead of an empty project list while the case-project index is building', () => {
    const html = renderSidebarProjectsSection({
      caseProjectIndexStatus: 'building'
    })

    expect(html).toContain('loading')
    expect(html).not.toContain('sidebarWorkspaceEmpty')
  })

  it('expands the active case project and shows its current thread row', () => {
    const html = renderSidebarProjectsSection({
      threads: [thread({ id: 'thr_active', workspace: '/cases/a' })],
      caseProjects: [{
        id: 'case_a',
        name: 'case-a',
        rootPath: '/cases/a',
        updatedAt: '2026-06-12T00:00:00.000Z',
        threadCount: 2,
        runningCount: 0,
        archivedCount: 0,
        status: 'ready'
      }],
      caseProjectThreadsById: {},
      activeThreadId: 'thr_active',
      workspaceRoot: '/cases/a',
      workspaceRoots: ['/cases/a']
    })

    expect(html).toContain('/cases/a')
    expect(html).toContain('thr_active')
  })

  it('shows the project-level spinner while a collapsed case project is loading', () => {
    const html = renderSidebarProjectsSection({
      caseProjects: [{
        id: 'case_a',
        name: 'case-a',
        rootPath: '/cases/a',
        updatedAt: '2026-06-12T00:00:00.000Z',
        threadCount: 2,
        runningCount: 0,
        archivedCount: 0,
        status: 'ready'
      }],
      caseProjectLoadingById: { case_a: true },
      workspaceRoot: '/cases/a',
      workspaceRoots: ['/cases/a']
    })

    expect(html).toContain('animate-spin text-accent')
    expect(html).not.toContain('sidebarDataAnalysisTitle')
  })

  it('does not show registry-only empty workspaces while searching or viewing archives', () => {
    const base = {
      threads: [thread({ id: 'reasonix-current', workspace: '/Users/zxy/project-a' })],
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: ['/Users/zxy/project-b']
    }

    expect(
      buildSidebarWorkspaceGroups({
        ...base,
        searchQuery: 'project',
        showArchived: false
      }).map(([workspace]) => workspace)
    ).toEqual(['/Users/zxy/project-a'])

    expect(
      buildSidebarWorkspaceGroups({
        ...base,
        searchQuery: '',
        showArchived: true
      }).map(([workspace]) => workspace)
    ).toEqual(['/Users/zxy/project-a'])
  })

  it('shows the default workspace while filtering write workspaces from code project groups', () => {
    const groups = buildSidebarWorkspaceGroups({
      threads: [
        thread({ id: 'code-current', workspace: '/Users/zxy/project-a' }),
        thread({ id: 'default-code', workspace: '/Users/zxy/.analytix/default_workspace' }),
        thread({ id: 'write-assistant', workspace: '~/.analytix/write_workspace' })
      ],
      searchQuery: '',
      showArchived: false,
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: [
        '/Users/zxy/project-a',
        '/Users/zxy/.analytix/default_workspace',
        '~/.analytix/write_workspace'
      ]
    })

    expect(groups.map(([workspace]) => workspace)).toEqual([
      '/Users/zxy/project-a',
      '/Users/zxy/.analytix/default_workspace'
    ])
    expect(groups[1]?.[1].map((item) => item.id)).toEqual(['default-code'])
  })

  it('sorts pinned threads by pinned order before falling back to recency', () => {
    const firstPinned = thread({
      id: 'pinned-old',
      workspace: '/tmp/app',
      updatedAt: '2026-06-01T00:00:00.000Z'
    })
    const secondPinned = thread({
      id: 'pinned-new',
      workspace: '/tmp/app',
      updatedAt: '2026-06-03T00:00:00.000Z'
    })
    const recentUnpinned = thread({
      id: 'recent-unpinned',
      workspace: '/tmp/app',
      updatedAt: '2026-06-05T00:00:00.000Z'
    })
    const olderUnpinned = thread({
      id: 'older-unpinned',
      workspace: '/tmp/app',
      updatedAt: '2026-06-02T00:00:00.000Z'
    })

    const pinnedThreadOrder = buildSidebarPinnedThreadOrder(new Set(['pinned-old', 'pinned-new']))
    const sorted = [recentUnpinned, secondPinned, olderUnpinned, firstPinned]
      .sort((a, b) => compareSidebarThreadsWithPins(a, b, pinnedThreadOrder))

    expect(sorted.map((item) => item.id)).toEqual([
      'pinned-old',
      'pinned-new',
      'recent-unpinned',
      'older-unpinned'
    ])
  })

  it('paginates project history with Analytix sidebar limits', () => {
    expect(getSidebarWorkspaceVisibleThreadLimit(4)).toBe(4)
    expect(getSidebarWorkspaceVisibleThreadLimit(25)).toBe(5)

    const firstPage = getNextSidebarWorkspaceThreadPage(25)
    expect(firstPage).toBe(1)
    expect(getSidebarWorkspaceVisibleThreadLimit(25, firstPage)).toBe(15)

    const secondPage = getNextSidebarWorkspaceThreadPage(25, firstPage)
    expect(secondPage).toBe(2)
    expect(getSidebarWorkspaceVisibleThreadLimit(25, secondPage)).toBe(25)

    expect(getNextSidebarWorkspaceThreadPage(25, secondPage)).toBe(0)
  })

  it('merges default workspace aliases into one sidebar group', () => {
    const groups = buildSidebarWorkspaceGroups({
      threads: [
        thread({ id: 'default-short', workspace: '~/.analytix/default_workspace' }),
        thread({ id: 'default-absolute', workspace: 'C:\\Users\\zxy\\.analytix\\default_workspace' })
      ],
      searchQuery: '',
      showArchived: false,
      workspaceRoot: 'C:\\Users\\zxy\\.analytix\\default_workspace',
      workspaceRoots: [
        '~/.analytix/default_workspace',
        'C:\\Users\\zxy\\.analytix\\default_workspace'
      ]
    })

    expect(groups).toHaveLength(1)
    expect(groups[0]?.[0]).toBe('C:\\Users\\zxy\\.analytix\\default_workspace')
    expect(groups[0]?.[1].map((item) => item.id)).toEqual(['default-short', 'default-absolute'])
  })

  it('loads requirement histories from all known project workspaces while searching', () => {
    const workspaces = buildSidebarDraftWorkspacePaths({
      threads: [
        thread({ id: 'code-current', workspace: '/Users/zxy/project-a' }),
        thread({ id: 'write-assistant', workspace: '~/.analytix/write_workspace' })
      ],
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: [
        '/Users/zxy/project-a',
        '/Users/zxy/project-b',
        '~/.analytix/write_workspace'
      ]
    })

    expect(workspaces).toEqual([
      '/Users/zxy/project-a',
      '/Users/zxy/project-b'
    ])
  })

  it('defaults project parents to collapsed and only expands the active project', () => {
    expect(isSidebarWorkspaceCollapsed({}, '/Users/zxy/project-a')).toBe(true)
    expect(isSidebarWorkspaceCollapsed({}, '/Users/zxy/project-a', '/Users/zxy/project-a')).toBe(false)
    expect(isSidebarWorkspaceCollapsed({}, '/Users/zxy/project-b', '/Users/zxy/project-a')).toBe(true)
    expect(isSidebarWorkspaceCollapsed({
      [sidebarWorkspaceCollapseKey('/Users/zxy/project-b')]: false
    }, '/Users/zxy/project-b', '/Users/zxy/project-a')).toBe(false)
  })

  it('keeps case projects collapsed unless expanded or holding the active thread', () => {
    expect(isSidebarCaseProjectCollapsed({}, 'case_a')).toBe(true)
    expect(isSidebarCaseProjectCollapsed({}, 'case_a', true)).toBe(false)
    expect(isSidebarCaseProjectCollapsed({ case_a: true }, 'case_a')).toBe(false)
    expect(isSidebarCaseProjectCollapsed({ case_a: false }, 'case_a', true)).toBe(true)
  })

  it('loads draft history only for expanded project parents', () => {
    const workspacePaths = [
      '/Users/zxy/project-a',
      '/Users/zxy/project-b',
      '/Users/zxy/project-c'
    ]

    expect(buildExpandedSidebarDraftWorkspacePaths({
      workspacePaths,
      collapsed: {},
      activeThreadWorkspace: '/Users/zxy/project-b'
    })).toEqual(['/Users/zxy/project-b'])

    expect(buildExpandedSidebarDraftWorkspacePaths({
      workspacePaths,
      collapsed: {
        [sidebarWorkspaceCollapseKey('/Users/zxy/project-a')]: false,
        [sidebarWorkspaceCollapseKey('/Users/zxy/project-b')]: true
      },
      activeThreadWorkspace: '/Users/zxy/project-b'
    })).toEqual(['/Users/zxy/project-a'])
  })

  it('merges requirement-only search matches into displayed groups', () => {
    const groups = buildSidebarWorkspaceGroups({
      threads: [thread({ id: 'reasonix-current', workspace: '/Users/zxy/project-a' })],
      searchQuery: 'checkout',
      showArchived: false,
      workspaceRoot: '/Users/zxy/project-a',
      workspaceRoots: ['/Users/zxy/project-a', '/Users/zxy/project-b']
    })
    const filteredDraftHistory = {
      '/Users/zxy/project-b': [draft({
        id: 'draft-checkout',
        title: 'Checkout requirement',
        workspaceRoot: '/Users/zxy/project-b'
      })]
    }

    const displayGroups = mergeSidebarWorkspaceGroupsWithDraftHistory({
      groups,
      draftHistoryByWorkspace: filteredDraftHistory,
      workspaceRoot: '/Users/zxy/project-a'
    })

    expect(displayGroups.map(([workspace]) => workspace)).toEqual([
      '/Users/zxy/project-a',
      '/Users/zxy/project-b'
    ])
  })

  it('filters requirement drafts by title, path, workspace, and content', () => {
    const items = [
      draft({ id: 'draft-login', title: 'Login requirement', searchText: 'Support passkey sign-in.' }),
      draft({ id: 'draft-export', title: 'Export requirement', searchText: 'Download reports as CSV.' })
    ]

    expect(filterSddDraftHistoryItems(items, 'passkey', '/tmp/app').map((item) => item.id)).toEqual(['draft-login'])
    expect(filterSddDraftHistoryItems(items, 'export', '/tmp/app').map((item) => item.id)).toEqual(['draft-export'])
    expect(filterSddDraftHistoryItems(items, 'tmp', '/tmp/app')).toHaveLength(2)
  })

  it('filters empty Requirement AI backing threads recorded in draft history', () => {
    const hidden = thread({
      id: 'thread-sdd-empty',
      title: 'Checkout requirement',
      workspace: '/tmp/app'
    })
    const visibleNormal = thread({
      id: 'thread-normal',
      title: 'Checkout requirement',
      workspace: '/tmp/app'
    })
    const visibleWithTurn = thread({
      id: 'thread-sdd-active-build',
      title: 'Checkout requirement',
      workspace: '/tmp/app',
      latestTurnId: 'turn-1'
    })
    const items = [
      draft({
        id: 'draft-checkout',
        title: 'Checkout requirement',
        chatThreadIds: ['thread-sdd-empty', 'thread-sdd-active-build']
      })
    ]

    expect(
      filterEmptySddAssistantThreadsFromSidebar([hidden, visibleNormal, visibleWithTurn], items)
        .map((item) => item.id)
    ).toEqual(['thread-normal', 'thread-sdd-active-build'])
  })
})

describe('ThreadRenameDialog', () => {
  it('renders an in-app rename form with the current thread title prefilled', () => {
    const html = renderToStaticMarkup(
      createElement(ThreadRenameDialog, {
        state: {
          thread: thread({
            id: 'thr_rename',
            title: 'Build rename dialog',
            workspace: '/Users/zxy/project-a'
          }),
          value: 'Build rename dialog',
          submitting: false
        },
        onClose: vi.fn(),
        onValueChange: vi.fn(),
        onSubmit: vi.fn(),
        t: (key: string) => key
      })
    )

    expect(html).toContain('role="dialog"')
    expect(html).toContain('sidebarThreadRename')
    expect(html).toContain('value="Build rename dialog"')
    expect(html).toContain('type="submit" disabled=""')
  })
})

describe('SddDraftHistoryRows', () => {
  it('renders requirement draft history fully collapsed by default', () => {
    const html = renderToStaticMarkup(
      createElement(SddDraftHistoryRows, {
        items: [
          draft({ id: 'draft-1', title: 'Requirement 1' }),
          draft({ id: 'draft-2', title: 'Requirement 2' }),
          draft({ id: 'draft-3', title: 'Requirement 3' }),
          draft({ id: 'draft-4', title: 'Requirement 4' })
        ],
        activeDraftId: '',
        onOpen: vi.fn(),
        t: (key: string, opts?: Record<string, unknown>) =>
          key === 'sddDraftHistoryOpen'
            ? `Open ${String(opts?.title)}`
            : key === 'sddDraftHistoryShowMore'
              ? `Show ${String(opts?.count)} more`
              : key
      })
    )

    expect(html).toContain('sddDraftHistoryTitle')
    expect(html).toContain('sddDraftHistoryExpand')
    expect(html).toContain('>4<')
    expect(html).not.toContain('Requirement 1')
    expect(html).not.toContain('Requirement 2')
    expect(html).not.toContain('Requirement 3')
    expect(html).not.toContain('Requirement 4')
    expect(html).not.toContain('Open Requirement 1')
    expect(html).not.toContain('Show 1 more')
  })
})

describe('DataAnalysisWorkspaceRows', () => {
  it('renders the project-level data analysis entry collapsed by default', () => {
    const html = renderToStaticMarkup(
      createElement(DataAnalysisWorkspaceRows, {
        activeItemId: null,
        onOpen: vi.fn(),
        t: (key: string) => key
      })
    )

    expect(html).toContain('sidebarDataAnalysisTitle')
    expect(html).toContain('sidebarDataAnalysisExpand')
    expect(html).toContain('>4<')
    expect(html).not.toContain('sidebarDataAnalysisImport')
    expect(html).not.toContain('sidebarDataAnalysisCleaning')
    expect(html).not.toContain('sidebarDataAnalysisStats')
    expect(html).not.toContain('sidebarDataAnalysisVisual')
  })
})
