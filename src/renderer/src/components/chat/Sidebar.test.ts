import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { Sidebar } from './Sidebar'
import sidebarSource from './Sidebar.tsx?raw'
import sidebarClawDialogSource from './SidebarClawDialog.tsx?raw'

function renderSidebar(activeView: 'chat' | 'write' | 'claw' | 'schedule' = 'chat'): string {
  const noop = vi.fn()
  return renderToStaticMarkup(
	    createElement(Sidebar, {
		      threads: [],
		      caseProjects: [],
		      caseProjectIndexStatus: 'ready',
		      caseProjectThreadsById: {},
	      caseProjectLoadingById: {},
	      caseProjectErrorsById: {},
	      caseProjectExpandedById: {},
	      activeThreadId: null,
      activeView,
      connectPhoneSidebarOpen: false,
      pluginsActive: false,
      runtimeReady: true,
      threadSearch: '',
	      showArchivedThreads: false,
	      onThreadSearchChange: noop,
	      onSetCaseProjectExpanded: noop,
	      onSelectThread: noop,
      onRenameThread: vi.fn(async () => undefined),
      onArchiveThread: vi.fn(async () => undefined),
      onDeleteThread: vi.fn(async () => undefined),
      onRestoreThread: vi.fn(async () => undefined),
      onOpenSideChat: noop,
      onForkThread: noop,
      onNewChat: noop,
      onNewChatInWorkspace: noop,
      onNewRequirement: noop,
      onOpenRequirementDraft: noop,
      activeDataAnalysis: null,
      onOpenDataAnalysis: noop,
      onOpenSettings: noop,
      onOpenPlugins: noop,
      focusModeEnabled: false,
      onFocusModeChange: noop,
      onToggleConnectPhone: noop,
      onCodeOpen: noop,
      onWriteOpen: noop,
      onScheduleOpen: noop
    })
  )
}

const forbiddenTopLevelEntrypointLabels = [
  'Workflow',
  'Create Loop',
  'Subagent',
  'Subagents',
  'AutoResearch',
  'Auto Research',
  'MCP-indexer',
  'MCP Indexer'
]

describe('Sidebar route entries', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
    useChatStore.setState({
      workspaceRoot: '',
      codeWorkspaceRoots: [],
      clawChannels: [],
      activeClawChannelId: '',
      busy: false,
      watchTurnCompletion: {},
      unreadThreadIds: {}
    })
  })

  it('keeps Workflow/Create Loop out of the top-level sidebar entry set', () => {
    const html = renderSidebar()

    expect(html).toContain('Plugins')
    expect(html).toContain('Schedule')
    for (const label of forbiddenTopLevelEntrypointLabels) {
      expect(html, `${label} must not render as a top-level sidebar entry`).not.toContain(label)
    }
  })

  it('renders a direct Settings footer without mounting Hub account behavior', () => {
    const html = renderSidebar()

    expect(html).toContain('Settings')
    expect(sidebarSource).not.toContain("import { SidebarAccountMenu } from './SidebarAccountMenu'")
    expect(sidebarSource).not.toContain('<SidebarAccountMenu')
    expect(sidebarSource).toContain("onOpenSettings('general')")
  })

  it('portals the WeChat setup dialog above sidebar clipping layers', () => {
    expect(sidebarSource).toContain("import { createPortal } from 'react-dom'")
    expect(sidebarSource).toContain('document.body')
    expect(sidebarSource).toContain('createPortal(')
    expect(sidebarClawDialogSource).toContain('fixed inset-0 z-[100]')
  })
})
