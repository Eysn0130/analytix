// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore } from '../store/chat-store'
import { SETTINGS_CHANGED_EVENT } from '../lib/keyboard-shortcut-settings'
import i18n from '../i18n'

vi.mock('./settings-sections', async () => {
  const { createElement: h } = await import('react')
  const empty = () => null
  const out = Object.fromEntries(['ArchivedThreads','Browser','Claw','ComputerUse','General','ImageGeneration','KeyboardShortcuts','LlmDebug','Worktree','MediaGeneration','Memory','Permissions','Providers','SpeechToText','Updates','Write'].map(n => [n + 'SettingsSection', empty]))
  return { ...out, AgentsSettingsSection: ({ ctx }: any) => h('div', { ref: ctx.agentsSectionRef }, h('div', { ref: ctx.skillSectionRef, 'data-test-deep-link': 'skill' }), h('div', { ref: ctx.mcpSectionRef, 'data-test-deep-link': 'mcp' })) }
})
import { SettingsView } from './SettingsView'

it('resets only the content owner when category changes, preserves same-category settings refresh and deep links', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  const initial = useChatStore.getState(), settings = normalizeAppSettings({ workspaceRoot: '/synthetic/a' } as AppSettingsV1)
  Object.assign(window, { analytix: { app: {}, write: {}, updates: {} } })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings)
  const scroll = vi.fn(); Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: scroll })
  const raf = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(fn => { fn(0); return 1 })
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const click = async (text: string) => act(async () => { [...host.querySelectorAll('button')].find(b => b.textContent?.trim() === text)!.click() })
  try {
    useChatStore.setState({ settingsSection: 'general' })
    await act(async () => root.render(createElement(SettingsView)))
    const owner = host.querySelector<HTMLDivElement>('.ds-settings-content')!; expect(owner).not.toBeNull()
    owner.scrollTop = 1196
    await click(i18n.t('write', { ns: 'settings' })); expect(owner.scrollTop).toBe(0)
    owner.scrollTop = 420
    await act(async () => window.dispatchEvent(new CustomEvent(SETTINGS_CHANGED_EVENT, { detail: { ...settings, codePromptPrefix: 'Synthetic retained draft' } })))
    expect(owner.scrollTop).toBe(420)
    await act(async () => useChatStore.setState({ settingsSection: 'skill' }))
    expect(owner.scrollTop).toBe(0); expect(scroll).toHaveBeenCalled()
    expect(host.querySelector('[data-test-deep-link="skill"]')).not.toBeNull()
    await act(async () => useChatStore.setState({ settingsSection: 'easterEgg' as any }))
    expect(host.querySelector('.ds-settings-sidebar [aria-current="page"]')?.textContent).toBe(i18n.t('general', { ns: 'settings' }))
    expect(host.textContent).not.toContain('Mascot modes')
  } finally { await act(async () => root.unmount()); host.remove(); useChatStore.setState(initial); raf.mockRestore(); vi.restoreAllMocks() }
})
