import { describe, expect, it } from 'vitest'

import workbenchSource from './Workbench.tsx?raw'
import chatStoreAppActionsSource from '../store/chat-store-app-actions.ts?raw'
import chatStoreNavigationActionsSource from '../store/chat-store-navigation-actions.ts?raw'
import chatStoreTypesSource from '../store/chat-store-types.ts?raw'
import preloadSource from '../../../preload/index.ts?raw'
import browserAnalytixBridgeSource from '../lib/browser-analytix-bridge.ts?raw'
import pluginMarketplaceViewSource from './PluginMarketplaceView.tsx?raw'
import settingsShortcutsSource from './settings-section-shortcuts.tsx?raw'
import sidebarSource from './chat/Sidebar.tsx?raw'
import sidebarProjectsSectionSource from './chat/SidebarProjectsSection.tsx?raw'
import workspaceModeTabsSource from './chat/WorkspaceModeTabs.tsx?raw'
import scheduleTasksViewSource from './schedule/ScheduleTasksView.tsx?raw'
import sidebarPrimitivesSource from './sidebar/SidebarPrimitives.tsx?raw'
import shellNavigationControlsSource from './shell/ShellNavigationControls.tsx?raw'
import workbenchLayoutSource from './workbench-layout.ts?raw'
import workbenchShellSource from './workbench/WorkbenchShell.tsx?raw'
import writeSidebarSource from './write/WriteSidebar.tsx?raw'
import traySessionMenuSource from '../../../main/tray-session-menu.ts?raw'
import workflowCreateLoopViewSource from './workflow/WorkflowCreateLoopView.tsx?raw'
import createLoopRuntimeSource from '../workflow/create-loop-runtime.ts?raw'

const topLevelRouteSurfaceSource = [
  workbenchSource,
  pluginMarketplaceViewSource,
  sidebarSource,
  sidebarProjectsSectionSource,
  workspaceModeTabsSource,
  writeSidebarSource,
  shellNavigationControlsSource,
  chatStoreTypesSource,
  chatStoreAppActionsSource
].join('\n')

const forbiddenTopLevelRouteTokens = [
  "'workflow'",
  '"workflow"',
  "'create-loop'",
  '"create-loop"',
  "'createLoop'",
  '"createLoop"',
  "'subagent'",
  '"subagent"',
  "'subagents'",
  '"subagents"',
  "'auto-research'",
  '"auto-research"',
  "'autoResearch'",
  '"autoResearch"',
  "'mcp-indexer'",
  '"mcp-indexer"',
  "'mcpIndexer'",
  '"mcpIndexer"'
]

const forbiddenTopLevelEntrypointSymbols = [
  'WorkflowCreateLoopView',
  'AutoResearch',
  'MCPIndexer',
  'McpIndexer',
  'openWorkflow',
  'openCreateLoop',
  'openSubagent',
  'openSubagents',
  'openAutoResearch',
  'openMCPIndexer',
  'openMcpIndexer'
]

const quarantinedWorkflowEntrySurfaceSource = [
  topLevelRouteSurfaceSource,
  chatStoreNavigationActionsSource,
  preloadSource,
  browserAnalytixBridgeSource,
  traySessionMenuSource,
  settingsShortcutsSource
].join('\n')

const quarantinedWorkflowSymbols = [
  'WorkflowCreateLoopView',
  'runCreateLoopWorkflow',
  'findPendingWorkflowGate',
  'analytix-create-loop'
]

const forbiddenBrowserPreviewBridgeAliases = [
  'window.kun',
  'window.reasonix',
  'window.deepseek',
  'window.analytixGui',
  'kunGui',
  'reasonixGui'
]

async function readBaseShellSource(): Promise<string> {
  const nodeFs = 'node:fs/promises'
  const { readFile } = await import(/* @vite-ignore */ nodeFs)
  return readFile(new URL('../styles/base-shell.css', import.meta.url), 'utf8')
}

describe('Workbench route surface', () => {
  it('keeps private document text out of user-message prompts', () => {
    expect(workbenchSource).not.toContain('buildComposerDocumentContextPrompt')
    expect(workbenchSource).not.toContain('attachment.documentText')
  })

  it('does not expose Workflow/Create Loop as a main workbench stage', () => {
    expect(workbenchSource).toContain("route === 'schedule'")
    expect(workbenchSource).not.toContain("route === 'workflow'")
    expect(workbenchSource).not.toContain('WorkflowCreateLoopView')
  })

  it('keeps Kun-absent orchestration features out of top-level routes', () => {
    expect(chatStoreTypesSource).toContain(
      "export type AppRoute = 'chat' | 'write' | 'settings' | 'plugins' | 'claw' | 'schedule'"
    )
    expect(sidebarSource).toContain("activeView: 'chat' | 'write' | 'claw' | 'schedule'")
    expect(workbenchSource).toContain('<PluginMarketplaceView leftSidebarCollapsed={leftSidebarCollapsed} />')
    expect(workbenchSource).toContain('<ScheduleTasksView')

    for (const token of forbiddenTopLevelRouteTokens) {
      expect(topLevelRouteSurfaceSource.includes(token), `${token} must not be a top-level route token`).toBe(false)
    }
    for (const symbol of forbiddenTopLevelEntrypointSymbols) {
      expect(
        topLevelRouteSurfaceSource.includes(symbol),
        `${symbol} must not be exposed as a top-level entrypoint`
      ).toBe(false)
    }
  })

  it('keeps dormant Workflow/Create Loop code quarantined from app entry surfaces', () => {
    expect(workflowCreateLoopViewSource).toContain('WorkflowCreateLoopView')
    expect(createLoopRuntimeSource).toContain('runCreateLoopWorkflow')

    for (const symbol of quarantinedWorkflowSymbols) {
      expect(
        quarantinedWorkflowEntrySurfaceSource.includes(symbol),
        `${symbol} must remain quarantined from top-level app entry surfaces`
      ).toBe(false)
    }
  })

  it('keeps SDD assistant thread creation on the selected composer provider and model', () => {
    expect(workbenchSource).toContain('const model = writeAssistantModel.trim()')
    expect(workbenchSource).toContain('const providerId = resolvedWriteAssistantProviderId.trim()')
    expect(workbenchSource).toMatch(
      /provider\.createThread\(\{[\s\S]*?mode: 'agent',[\s\S]*?\.\.\.\(model \? \{ model \} : \{\}\),[\s\S]*?\.\.\.\(providerId \? \{ providerId \} : \{\}\)/
    )
  })

  it('keeps the browser preview bridge on the analytix facade only', () => {
    expect(browserAnalytixBridgeSource).toContain('window.analytix = createBrowserAnalytixApi')
    expect(browserAnalytixBridgeSource).toContain("document.documentElement.dataset.bridge = 'browser-preview'")
    expect(browserAnalytixBridgeSource).toContain("runtimeRequest: (path, method, body) => runtimeRequestViaProxy")
    expect(browserAnalytixBridgeSource).toContain('/v1/threads/${encodeURIComponent(threadId)}/events')

    for (const alias of forbiddenBrowserPreviewBridgeAliases) {
      expect(
        browserAnalytixBridgeSource.includes(alias),
        `${alias} must not be exposed by the browser preview bridge`
      ).toBe(false)
    }
  })

  it('keeps sidebar navigation controls at the workbench shell level', () => {
    expect(workbenchSource).toContain('ShellNavigationControls')
    expect(workbenchSource).not.toContain('ds-shell-header-drag-slot')
    expect(workbenchSource).toContain('navigateShellHistoryBack')
    expect(workbenchSource).toContain('--ds-shell-navigation-sidebar-width')
    expect(workbenchSource).toMatch(/<WorkbenchShell ref=\{shellRef\} style=\{workbenchShellStyle\}>\s+<ShellNavigationControls/)
    expect(workbenchShellSource).toContain('ds-workbench-shell ds-no-drag')
    expect(workbenchShellSource).toContain('ds-no-drag ds-stage-surface')
    expect(workbenchShellSource).not.toContain('ds-drag ds-stage-surface')
    expect(workbenchShellSource).not.toContain('ds-workbench-shell ds-drag')
    expect(workbenchSource).toContain('ds-chat-stage ds-no-drag')
    expect(workbenchSource).not.toContain('ds-chat-stage ds-drag')
    expect(workbenchSource).toContain('chat-topbar ds-chat-shell-header ds-topbar-surface')
    expect(workbenchSource).toContain('className="chat-topbar-drag-region"')
    expect(workbenchSource).toContain('<PluginMarketplaceView leftSidebarCollapsed={leftSidebarCollapsed} />')
    expect(pluginMarketplaceViewSource).toContain('leftSidebarCollapsed?: boolean')
    expect(pluginMarketplaceViewSource).toContain('ds-plugin-marketplace-tabs flex items-center gap-1')
    expect(pluginMarketplaceViewSource).toContain("data-left-sidebar-collapsed={leftSidebarCollapsed ? 'true' : 'false'}")
    expect(pluginMarketplaceViewSource.indexOf('ds-plugin-marketplace-tabs')).toBeLessThan(
      pluginMarketplaceViewSource.indexOf('mx-auto w-full max-w-[960px]')
    )
    expect(workbenchSource).toMatch(
      /<section[\s\S]*?className="ds-chat-stage ds-no-drag[\s\S]*?<header className="chat-topbar ds-chat-shell-header[\s\S]*?<div className=\{`\$\{stageInsetClass\}/
    )
    expect(scheduleTasksViewSource).toContain('ds-no-drag flex h-full')
    expect(scheduleTasksViewSource).not.toContain('ds-drag flex h-full')
    expect(workbenchSource).not.toContain('toggleChatTopbarSidebar')
    expect(shellNavigationControlsSource).toContain('ds-collapsed-new-chat-button')
    expect(shellNavigationControlsSource).toContain('ShellGuiUpdateAction')
    expect(shellNavigationControlsSource).toContain('shellGuiUpdateActionMode(leftSidebarCollapsed)')
    expect(shellNavigationControlsSource).toContain('<ToolbarTooltip label={newChatLabel} hidden={!leftSidebarCollapsed}>')
    expect(shellNavigationControlsSource).toContain("data-collapsed={leftSidebarCollapsed ? 'true' : 'false'}")
    expect(shellNavigationControlsSource).toContain('disabled={!leftSidebarCollapsed || newChatDisabled}')
    expect(shellNavigationControlsSource).not.toContain("setProperty('--ds-shell-collapsed-header-slot-width'")
    expect(shellNavigationControlsSource).not.toContain('ResizeObserver')
    expect(shellNavigationControlsSource).not.toContain('getBoundingClientRect')
    expect(shellNavigationControlsSource).toContain('onPointerDown')
    expect(shellNavigationControlsSource).toContain('onMouseDown')
    expect(shellNavigationControlsSource).toContain('onClick')
    expect(shellNavigationControlsSource).not.toContain('onClickCapture')
    expect(shellNavigationControlsSource).not.toContain('event.preventDefault()')
    expect(workbenchLayoutSource.match(/left: clampWidth\(leftWidth, SIDEBAR_HARD_MIN, LEFT_PANEL_MAX\)/g)).toHaveLength(2)
    expect(workbenchLayoutSource).not.toContain('left: clampWidth(leftWidth, LEFT_PANEL_MIN, LEFT_PANEL_MAX)')
    expect(sidebarPrimitivesSource).toContain('ds-no-drag ds-sidebar-shell')
    expect(sidebarPrimitivesSource).not.toContain('ds-drag ds-sidebar-shell')
  })

  it('anchors the return-to-bottom button above the full composer surface', () => {
    const anchorIndex = workbenchSource.indexOf('data-analytix-return-to-bottom-anchor')
    const composerIndex = workbenchSource.indexOf('<FloatingComposerIsland')

    expect(anchorIndex).toBeGreaterThanOrEqual(0)
    expect(composerIndex).toBeGreaterThan(anchorIndex)
    expect(workbenchSource).toContain('data-analytix-return-to-bottom-button')
    expect(workbenchSource).toContain('bottom-[calc(100%+1.5rem)]')
  })

  it('keeps shell navigation clear of native titlebar controls', async () => {
    const baseShellSource = await readBaseShellSource()

    expect(baseShellSource).toContain('--ds-window-controls-safe-block: calc(50px / var(--ds-ui-scale))')
    expect(baseShellSource).toContain('--ds-macos-traffic-light-left: 16px')
    expect(baseShellSource).toContain('--ds-macos-traffic-light-top: 16px')
    expect(baseShellSource).toContain('--ds-macos-traffic-light-width: 66px')
    expect(baseShellSource).toContain('--ds-macos-traffic-light-gap: 18px')
    expect(baseShellSource).toContain('--ds-window-controls-safe-inset: calc(')
    expect(baseShellSource).toContain('/ var(--ds-ui-scale)')
    expect(baseShellSource).toContain('--ds-shell-navigation-control-gap: 4px')
    expect(baseShellSource).toContain('--ds-shell-navigation-base-control-width: calc(')
    expect(baseShellSource).toContain('(var(--ax-toolbar-button-size) * 3) + (var(--ds-shell-navigation-control-gap) * 2)')
    expect(baseShellSource).toContain('--ds-shell-navigation-collapsed-control-width: calc(')
    expect(baseShellSource).toContain('(var(--ax-toolbar-button-size) * 4) + (var(--ds-shell-navigation-control-gap) * 3)')
    expect(baseShellSource).toContain('--ds-shell-navigation-fixed-control-width: var(--ds-shell-navigation-base-control-width)')
    expect(baseShellSource).toContain('--ds-shell-collapsed-controls-right: calc(')
    expect(baseShellSource).toContain('--ds-shell-collapsed-header-title-gap: var(--ds-shell-navigation-control-gap)')
    expect(baseShellSource).toContain('--ds-shell-collapsed-header-slot-width: max(')
    expect(baseShellSource).toContain('var(--ds-shell-collapsed-controls-right) - var(--ds-shell-topbar-content-left-padding, 20px)')
    expect(baseShellSource).toContain('--ds-shell-navigation-top: calc((var(--ax-chat-topbar-shell-height) - var(--ax-toolbar-button-size)) / 2)')
    expect(baseShellSource).toMatch(/:root\[data-platform='darwin'\] \.ds-workbench-shell \{[\s\S]*?--ds-shell-collapsed-controls-right: calc\([\s\S]*?var\(--ds-titlebar-safe-header-left\) \+ var\(--ds-shell-navigation-collapsed-control-width\)[\s\S]*?\);[\s\S]*?\}/)
    expect(baseShellSource).toMatch(/:root\[data-platform='darwin'\] \.ds-workbench-shell \{[\s\S]*?--ds-shell-navigation-top: calc\([\s\S]*?\(var\(--ds-macos-traffic-light-top\) \/ var\(--ds-ui-scale\) \/ 2\)[\s\S]*?\(\(var\(--ax-chat-topbar-shell-height\) - var\(--ax-toolbar-button-size\)\) \/ 2\)[\s\S]*?\);[\s\S]*?\}/)
    expect(baseShellSource).toContain('left: var(--ds-titlebar-safe-header-left)')
    expect(baseShellSource).toContain('--ds-shell-navigation-hit-slop: 0px')
    expect(baseShellSource).toContain('top: var(--ds-shell-navigation-top)')
    expect(baseShellSource).toContain('z-index: 220')
    expect(baseShellSource).toContain('.ds-native-window-controls-hitbox')
    expect(baseShellSource).toContain('.ds-sidebar-titlebar-spacer')
    expect(baseShellSource).toContain('.ds-sidebar-titlebar-spacer .ds-titlebar-safe-block')
    expect(baseShellSource).toMatch(/\.ds-sidebar-titlebar-spacer \{[\s\S]*?pointer-events: none;[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(/\.ds-sidebar-titlebar-row,[\s\S]*?\.ds-sidebar-titlebar-spacer \.ds-titlebar-safe-block \{[\s\S]*?pointer-events: none;[\s\S]*?-webkit-app-region: no-drag;/)
    expect(baseShellSource).toContain('min-height: var(--ax-toolbar-button-size)')
    expect(baseShellSource).toMatch(/html,\s+body,\s+#root \{[\s\S]*?-webkit-app-region: no-drag;/)
    expect(baseShellSource).toContain('.ds-shell-navigation-controls .ds-toolbar-tooltip-anchor')
    expect(baseShellSource).toContain('.ds-shell-navigation-controls button')
    expect(baseShellSource).toContain('.ds-shell-navigation-controls *')
    expect(baseShellSource).toContain('-webkit-app-region: no-drag')
    expect(baseShellSource).not.toContain('.ds-shell-header-drag-slot')
    expect(baseShellSource).toMatch(/\.ds-shell-navigation-controls \{[\s\S]*?pointer-events: auto;[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(/\.ds-shell-navigation-controls \{[\s\S]*?margin: calc\(-1 \* var\(--ds-shell-navigation-hit-slop\)\);[\s\S]*?padding: var\(--ds-shell-navigation-hit-slop\);/)
    expect(baseShellSource).toMatch(/:root\[data-platform='darwin'\] \.ds-shell-navigation-controls \{\s+--ds-shell-navigation-hit-slop: 10px;\s+\}/)
    expect(baseShellSource).toMatch(/\.ds-shell-navigation-controls \.ds-toolbar-tooltip-anchor \{[\s\S]*?pointer-events: auto;[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(/\.ds-shell-navigation-controls button \{[\s\S]*?pointer-events: auto;[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(/\.ds-shell-navigation-controls,\s+\.ds-shell-navigation-controls \* \{[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(
      /\.ds-collapsed-new-chat-button\[data-collapsed='false'\] \{\s+visibility: hidden;\s+pointer-events: none;\s+\}/
    )
    expect(baseShellSource).not.toContain('view-transition-name: sidebar-trigger')
    expect(baseShellSource).toMatch(/\.chat-topbar \{[\s\S]*?pointer-events: none;[\s\S]*?-webkit-app-region: drag;[\s\S]*?app-region: drag;/)
    expect(baseShellSource).toMatch(/\.ds-windows-titlebar \{[\s\S]*?z-index: 300;/)
    expect(baseShellSource).toMatch(/\.ds-windows-menu \{[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(/\.ds-windows-menu-popover \{[\s\S]*?z-index: 310;[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(/\.ds-windows-menu \*,[\s\S]*?\.ds-window-controls \* \{[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toContain('.ds-topbar-surface,')
    expect(baseShellSource).toContain('.chat-topbar-actions,')
    expect(baseShellSource).toMatch(/\.ds-topbar-surface,[\s\S]*?\.chat-workbench-topbar \{[\s\S]*?pointer-events: none;[\s\S]*?-webkit-app-region: drag;[\s\S]*?app-region: drag;/)
    expect(baseShellSource).toContain('.chat-topbar-drag-region')
    expect(baseShellSource).toMatch(/\.ds-chat-shell-header\.chat-topbar \{[\s\S]*?margin-top: 0;[\s\S]*?border-radius: 0;[\s\S]*?-webkit-app-region: initial;[\s\S]*?app-region: initial;/)
    expect(baseShellSource).toMatch(/\.ds-chat-shell-header\.ds-topbar-surface,[\s\S]*?\.ds-chat-shell-header \.chat-workbench-topbar \{[\s\S]*?-webkit-app-region: initial;[\s\S]*?app-region: initial;/)
    expect(baseShellSource).toMatch(/:root\[data-platform='darwin'\] \.ds-chat-shell-header \.chat-topbar-drag-region \{[\s\S]*?left: var\(--ds-shell-collapsed-header-slot-width\);[\s\S]*?right: clamp\(220px, 24vw, 360px\);[\s\S]*?z-index: 1;[\s\S]*?pointer-events: auto;[\s\S]*?-webkit-app-region: drag;[\s\S]*?app-region: drag;/)
    expect(baseShellSource).toMatch(/\.ds-chat-shell-header \.ds-toolbar-tooltip-anchor,[\s\S]*?\.ds-chat-shell-header :is\(button, input, textarea, select, option, a, summary, \[role='button'\], \[role='menu'\], \[role='menuitem'\], \[role='tab'\], \[contenteditable='true'\]\) \{[\s\S]*?position: relative;[\s\S]*?z-index: 2;/)
    expect(baseShellSource).toContain(".ds-sidebar-titlebar-spacer :is(button, input, textarea, select, option, a, summary, [role='button'], [role='menu'], [role='menuitem'], [role='tab'], [contenteditable='true'])")
    expect(baseShellSource).toContain('.chat-topbar .ds-toolbar-tooltip-anchor')
    expect(baseShellSource).toContain(".chat-topbar :is(button, input, textarea, select, option, a, summary, [role='button'], [role='menu'], [role='menuitem'], [role='tab'], [contenteditable='true'])")
    expect(baseShellSource).toMatch(/\.chat-topbar :is\(button, input, textarea, select, option, a, summary, \[role='button'\], \[role='menu'\], \[role='menuitem'\], \[role='tab'\], \[contenteditable='true'\]\) \{[\s\S]*?pointer-events: auto;[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toContain('Windows/Linux have a dedicated desktop titlebar; workbench chrome must not mask clicks.')
    expect(baseShellSource).toMatch(/:root:not\(\[data-platform='darwin'\]\) \.ds-sidebar-titlebar-spacer,[\s\S]*?:root:not\(\[data-platform='darwin'\]\) \.chat-workbench-topbar \{[\s\S]*?-webkit-app-region: no-drag;[\s\S]*?app-region: no-drag;/)
    expect(baseShellSource).toMatch(
      /\.ds-shell-controls-safe-inset \{\s+padding-left: var\(--ds-shell-collapsed-header-slot-width\);\s+\}/
    )
    expect(baseShellSource).toMatch(/\.chat-topbar-grid \{[\s\S]*?--ds-shell-topbar-content-left-padding: 0\.75rem;/)
    expect(baseShellSource).toMatch(/\.ds-shell-topbar-grid \{[\s\S]*?--ds-shell-topbar-content-left-padding: 0\.75rem;/)
    expect(baseShellSource).toMatch(/@media \(min-width: 640px\) \{[\s\S]*?--ds-shell-topbar-content-left-padding: 1rem;/)
    expect(baseShellSource).toMatch(/@media \(min-width: 768px\) \{[\s\S]*?--ds-shell-topbar-content-left-padding: 1\.25rem;/)
    expect(baseShellSource).toMatch(/\.ds-chat-shell-header\.chat-topbar \{[\s\S]*?margin-top: 0;[\s\S]*?border-radius: 0;[\s\S]*?\}/)
    expect(baseShellSource).not.toContain(
      'padding-left: calc(var(--ds-window-controls-safe-inset) + var(--ds-shell-collapsed-header-slot-width))'
    )
    expect(baseShellSource).toMatch(/\.ds-shell-controls-safe-motion \{[\s\S]*?transition: padding-left var\(--ax-motion-sidebar-collapse\);/)
    expect(baseShellSource).toMatch(/\.ds-plugin-marketplace-tabs \{[\s\S]*?transition: padding-left var\(--ax-motion-sidebar-collapse\);/)
    expect(baseShellSource).toContain(":root[data-motion-reduced='true'] .ds-left-sidebar-pane")
    expect(baseShellSource).toContain(":root[data-motion-reduced='true'] .ds-shell-controls-safe-motion")
    expect(baseShellSource).toContain(":root[data-motion-reduced='true'] .ds-thread-summary-content-motion")
    expect(baseShellSource).not.toContain('@media (prefers-reduced-motion: reduce) {\n  .ds-left-sidebar-pane')
    expect(baseShellSource).not.toContain('@media (prefers-reduced-motion: reduce) {\n  .ds-thread-summary-content-motion')
    expect(baseShellSource).toMatch(
      /\.ds-plugin-marketplace-tabs\[data-left-sidebar-collapsed='true'\] \{\s+padding-left: var\(--ds-shell-navigation-sidebar-width\);\s+\}/
    )
    expect(baseShellSource).toContain(":root:not([data-platform='darwin']) .ds-shell-navigation-controls")
    expect(baseShellSource).toMatch(
      /:root:not\(\[data-platform='darwin'\]\) \.ds-shell-navigation-controls \{\s+top: 9px;\s+left: calc\(\(var\(--ds-shell-navigation-sidebar-width\) - var\(--ds-shell-navigation-fixed-control-width\)\) \/ 2\);\s+transform: none;\s+\}/
    )
    expect(baseShellSource).not.toContain(
      ".ds-shell-navigation-controls[data-left-sidebar-collapsed='true']"
    )
    expect(baseShellSource).toMatch(
      /:root:not\(\[data-platform='darwin'\]\) \.ds-shell-controls-safe-inset \{\s+padding-left: var\(--ds-shell-collapsed-header-slot-width\);\s+\}/
    )
  })
})
