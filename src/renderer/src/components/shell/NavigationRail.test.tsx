// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { NavigationRail } from './NavigationRail'

describe('persistent product navigation', () => {
  it('preserves each existing destination handler and selected state independently of panels', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    await i18n.changeLanguage('en')
    const handlers = { onChat: vi.fn(), onPlugins: vi.fn(), onSchedule: vi.fn(), onSettings: vi.fn() }
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () => root.render(createElement(NavigationRail, { active: 'plugins', ...handlers })))
      const buttons = Array.from(container.querySelectorAll('button'))
      expect(buttons.map((button) => button.getAttribute('aria-label'))).toEqual(['Chat', 'Plugins', 'Scheduled tasks', 'Settings'])
      expect(container.querySelector('[aria-current="page"]')?.getAttribute('aria-label')).toBe('Plugins')
      for (const button of buttons) await act(async () => button.click())
      for (const handler of Object.values(handlers)) expect(handler).toHaveBeenCalledTimes(1)
      buttons[3].focus()
      expect(document.activeElement).toBe(buttons[3])
      await act(async () => root.render(createElement(NavigationRail, { active: 'settings', ...handlers })))
      expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1)
      expect(container.querySelector('[aria-current="page"]')?.getAttribute('aria-label')).toBe('Settings')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})
