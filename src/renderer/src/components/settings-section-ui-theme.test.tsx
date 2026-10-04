// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { UiThemeSettingsSection } from './settings-section-ui-theme'
import { SettingsSidebar } from './SettingsSidebar'
import { useUiPluginStore } from '../store/ui-plugin-store'

const initial = useUiPluginStore.getState()
const labels: Record<string, string> = {
  uiThemeSection: 'UI themes', uiThemeTitle: 'Theme plugins', uiThemeDesc: 'Customize colors',
  uiThemeDefaultTitle: 'Default theme', uiThemeDefaultSubtitle: 'Analytix colors',
  uiPluginInstall: 'Install plugin folder', uiPluginActivate: 'Use', uiPluginActive: 'Active',
  uiPluginRemove: 'Remove plugin', uiPluginInstallFailed: 'Install failed'
}
const t = (key: string) => labels[key] ?? key
beforeEach(() => { Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true }); localStorage.clear() })
afterEach(() => { useUiPluginStore.setState(initial); document.getElementById('ax-ui-plugin-tokens')?.remove(); vi.restoreAllMocks() })

it('keeps real install/activate/remove/reset callbacks in neutral theme management, without animal previews', async () => {
  const plugin = { manifest: { id: 'synthetic-theme', name: 'Synthetic theme', version: '1.0.0', figures: {}, tokens: { light: { '--ds-accent': '#123456' } } }, previewDataUrl: 'data:image/png;base64,animal' }
  const listUiPlugins = vi.fn().mockResolvedValue({ plugins: [plugin] })
  const loadUiPlugin = vi.fn().mockResolvedValue({ ok: true, manifest: plugin.manifest, figures: {} })
  const installUiPlugin = vi.fn().mockResolvedValue({ canceled: true })
  const removeUiPlugin = vi.fn().mockResolvedValue({ ok: true })
  Object.assign(window, { analytix: { app: { listUiPlugins, loadUiPlugin, installUiPlugin, removeUiPlugin } } })
  useUiPluginStore.setState({ ...initial, initialized: false })
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const click = async (name: string) => { await act(async () => { [...host.querySelectorAll('button')].find(b => b.textContent?.trim() === name || b.getAttribute('aria-label') === name)!.click() }) }
  try {
    await act(async () => root.render(createElement(UiThemeSettingsSection, { ctx: { t } })))
    expect(host.textContent).toContain('Synthetic theme'); expect(host.querySelector('img')).toBeNull()
    await click('Use'); expect(loadUiPlugin).toHaveBeenCalledWith('synthetic-theme')
    expect(document.getElementById('ax-ui-plugin-tokens')?.textContent).toContain('--ds-accent')
    await click('Install plugin folder'); expect(installUiPlugin).toHaveBeenCalledOnce(); expect(host.textContent).not.toContain('Install failed')
    await click('Remove plugin'); expect(removeUiPlugin).toHaveBeenCalledWith('synthetic-theme')
    expect(useUiPluginStore.getState().uiMode).toBe('default'); expect(document.getElementById('ax-ui-plugin-tokens')).toBeNull()
    await act(async () => useUiPluginStore.setState({ busy: true, lastError: 'Synthetic error' }))
    expect([...host.querySelectorAll('button')].every(b => b.disabled)).toBe(true)
    expect(host.textContent).toContain('Synthetic error')
  } finally { await act(async () => root.unmount()); host.remove() }
})

it('removes the dedicated pet settings navigation while keeping General discoverable', async () => {
  const host = document.createElement('div'); const root = createRoot(host)
  try {
    await act(async () => root.render(createElement(SettingsSidebar, { category: 'general', goBack: () => {}, setCategory: () => {}, t })))
    expect(host.textContent).not.toContain('easterEgg')
    expect(host.querySelector('[aria-current="page"]')?.textContent).toContain('general')
  } finally { await act(async () => root.unmount()) }
})

it('shows real pending removal and failure from the existing IPC callback', async () => {
  let rejectRemoval: (error: Error) => void = () => {}
  const removeUiPlugin = vi.fn(() => new Promise<{ ok: boolean }>((_resolve, reject) => { rejectRemoval = reject }))
  const plugin = { manifest: { id: 'theme-to-remove', name: 'Theme to remove', version: '1.0.0', figures: {} } }
  Object.assign(window, { analytix: { app: { listUiPlugins: async () => ({ plugins: [plugin] }), removeUiPlugin } } })
  useUiPluginStore.setState({ ...initial, initialized: true, installed: [plugin] as any })
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  try {
    await act(async () => root.render(createElement(UiThemeSettingsSection, { ctx: { t } })))
    await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="Remove plugin"]')!.click())
    expect(removeUiPlugin).toHaveBeenCalledExactlyOnceWith('theme-to-remove')
    expect([...host.querySelectorAll('button')].every(b => b.disabled)).toBe(true)
    await act(async () => { rejectRemoval(new Error('Synthetic removal failure')); await Promise.resolve() })
    expect(useUiPluginStore.getState().busy).toBe(false)
    expect(host.textContent).toContain('Synthetic removal failure')
  } finally { await act(async () => root.unmount()); host.remove() }
})
