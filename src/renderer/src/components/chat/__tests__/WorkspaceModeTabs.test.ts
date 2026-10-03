import { createElement, type ComponentProps } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../../i18n'
import { Sidebar } from '../Sidebar'
import { NavigationRail } from '../../shell/NavigationRail'

// The accepted navigation contract replaces the old Code/Write tabs.
describe('unified sidebar navigation', () => {
  beforeEach(async () => { await i18n.changeLanguage('en') })
  it('keeps conversation actions without any mode tablist or replacement heading', () => {
    const noop = vi.fn()
    const props = { threads: [], caseProjects: [], caseProjectThreadsById: {}, caseProjectLoadingById: {},
      caseProjectErrorsById: {}, caseProjectExpandedById: {}, activeThreadId: null, activeView: 'chat',
      connectPhoneSidebarOpen: false, runtimeReady: true, threadSearch: '', showArchivedThreads: false,
      onNewChat: noop, onOpenSettings: noop, onOpenPlugins: noop, onFocusModeChange: noop,
      onToggleConnectPhone: noop, onThreadSearchChange: noop, onScheduleOpen: noop
    } as unknown as ComponentProps<typeof Sidebar>
    const html = renderToStaticMarkup(createElement('div', {},
      createElement(NavigationRail, { windowChrome: true, active: 'chat',
        onChat: noop, onPlugins: noop, onSchedule: noop, onSettings: noop }),
      createElement(Sidebar, props)
    ))
    expect(html).not.toContain('role="tablist"')
    expect(html).not.toContain('Code / Write')
    expect(html).not.toContain('>Workbench<')
    expect(html).not.toContain('>Office<')
    expect(html).toContain(i18n.t('common:newAgent'))
    expect(html).toContain(i18n.t('common:settings'))
    for (const key of ['chatNavigationLabel', 'plugins', 'schedule']) {
      expect(html).toContain(i18n.t(`common:${key}`))
    }
  })
})
