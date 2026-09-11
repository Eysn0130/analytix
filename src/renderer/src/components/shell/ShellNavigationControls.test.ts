import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import shellNavigationControlsSource from './ShellNavigationControls.tsx?raw'
import { ShellNavigationControls } from './ShellNavigationControls'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key
  })
}))

describe('ShellNavigationControls', () => {
  it('renders shell-level sidebar, history, and collapsed new-chat controls', () => {
    const html = renderToStaticMarkup(
      createElement(ShellNavigationControls, {
        leftSidebarCollapsed: true,
        sidebarLabel: 'Expand sidebar',
        backLabel: 'Back',
        forwardLabel: 'Forward',
        newChatLabel: 'New Agent',
        canGoBack: true,
        canGoForward: false,
        newChatDisabled: true,
        onToggleSidebar: vi.fn(),
        onBack: vi.fn(),
        onForward: vi.fn(),
        onNewChat: vi.fn()
      })
    )

    expect(html).toContain('ds-shell-navigation-controls')
    expect(html).toContain('ds-shell-navigation-controls ds-no-drag')
    expect(html).toContain('data-left-sidebar-collapsed="true"')
    expect(html).toContain('ds-shell-sidebar-toggle')
    expect(html).toContain('aria-label="Back"')
    expect(html).toContain('aria-label="Forward"')
    expect(html).toContain('aria-label="New Agent"')
    expect(html).toContain('data-collapsed="true"')
    expect(html).toContain('ds-collapsed-new-chat-button')
    expect(html).toContain('disabled=""')
  })

  it('keeps the collapsed new-chat slot reserved before collapse completes', () => {
    const html = renderToStaticMarkup(
      createElement(ShellNavigationControls, {
        leftSidebarCollapsed: false,
        sidebarLabel: 'Collapse sidebar',
        backLabel: 'Back',
        forwardLabel: 'Forward',
        newChatLabel: 'New Agent',
        canGoBack: false,
        canGoForward: false,
        newChatDisabled: false,
        onToggleSidebar: vi.fn(),
        onBack: vi.fn(),
        onForward: vi.fn(),
        onNewChat: vi.fn()
      })
    )

    expect(html).toContain('data-left-sidebar-collapsed="false"')
    expect(html).toContain('ds-collapsed-new-chat-button')
    expect(html).toContain('aria-label="New Agent"')
    expect(html).toContain('aria-hidden="true"')
    expect(html).toContain('data-collapsed="false"')
    expect(html).toContain('tabindex="-1"')
  })

  it('keeps sidebar button clicks reachable while blocking drag-surface bubbling', () => {
    expect(shellNavigationControlsSource).toContain('function stopShellNavigationEvent')
    expect(shellNavigationControlsSource).toContain('event.stopPropagation()')
    expect(shellNavigationControlsSource).not.toContain('event.preventDefault()')
    expect(shellNavigationControlsSource).toContain('onPointerDown={stopShellNavigationEvent}')
    expect(shellNavigationControlsSource).toContain('onMouseDown={stopShellNavigationEvent}')
    expect(shellNavigationControlsSource).toContain('onClick={stopShellNavigationEvent}')
    expect(shellNavigationControlsSource).not.toContain('onClickCapture')
    expect(shellNavigationControlsSource).not.toContain('onPointerDownCapture')
    expect(shellNavigationControlsSource).toContain('onClick={onToggleSidebar}')
    expect(shellNavigationControlsSource).toContain('onClick={onBack}')
    expect(shellNavigationControlsSource).toContain('onClick={onForward}')
    expect(shellNavigationControlsSource).toContain('disabled={!canGoBack}')
    expect(shellNavigationControlsSource).toContain('disabled={!canGoForward}')
  })

  it('uses a CSS-reserved four-button slot instead of remeasuring during collapse', () => {
    expect(shellNavigationControlsSource).toContain('<ToolbarTooltip label={newChatLabel} hidden={!leftSidebarCollapsed}>')
    expect(shellNavigationControlsSource).toContain("data-collapsed={leftSidebarCollapsed ? 'true' : 'false'}")
    expect(shellNavigationControlsSource).toContain('disabled={!leftSidebarCollapsed || newChatDisabled}')
    expect(shellNavigationControlsSource).toContain('tabIndex={leftSidebarCollapsed ? undefined : -1}')
    expect(shellNavigationControlsSource).not.toContain('ResizeObserver')
    expect(shellNavigationControlsSource).not.toContain('getBoundingClientRect')
    expect(shellNavigationControlsSource).not.toContain('setProperty')
    expect(shellNavigationControlsSource).not.toContain('removeProperty')
    expect(shellNavigationControlsSource).not.toContain('useLayoutEffect')
  })

})
