// @vitest-environment jsdom
import { act, createElement, StrictMode, type ComponentProps, type RefObject } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsPatch, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore } from '../store/chat-store'
import { SETTINGS_CHANGED_EVENT } from '../lib/keyboard-shortcut-settings'
import { mergeSettings } from './settings-utils'
import i18n from '../i18n'
import { SETTINGS_TABS, SettingsSidebar } from './SettingsSidebar'

type NavigationContext = {
  form: AppSettingsV1
  update: (patch: AppSettingsPatch) => void
  openProviderSettings: () => void
  logsSectionRef: RefObject<HTMLDivElement | null>
  onProviderDraftDirtyChange: (dirty: boolean) => void
}
const capture = vi.hoisted(() => ({ current: null as NavigationContext | null,
  providerDirty: null as ((dirty: boolean) => void) | null }))

vi.mock('./settings-sections', async () => {
  const { createElement: h } = await import('react')
  const section = (name: string) => ({ ctx }: { ctx: NavigationContext }) => {
    capture.current = ctx
    return h('div', { 'data-test-settings-section': name })
  }
  const out = Object.fromEntries(['ArchivedThreads', 'Browser', 'Claw', 'ComputerUse', 'ImageGeneration',
    'KeyboardShortcuts', 'Worktree', 'Memory', 'Permissions', 'SpeechToText', 'Updates', 'Write']
    .map(name => [name + 'SettingsSection', section(name)]))
  return { ...out, GeneralSettingsSection: ({ ctx }: { ctx: NavigationContext }) => {
    capture.current = ctx
    return h('div', { 'data-test-settings-section': 'general' },
      h('input', { 'aria-label': 'Fixture draft', value: ctx.form.codePromptPrefix,
        onChange: (event: { target: { value: string } }) => ctx.update({ codePromptPrefix: event.target.value }) }),
      h('details', null, h('summary', null, 'Fixture diagnostics'),
        h('div', { ref: ctx.logsSectionRef, 'data-test-deep-link': 'logs' })),
      h('button', { 'data-test-provider-settings': '', onClick: ctx.openProviderSettings }, 'Fixture Providers'))
  }, ProvidersSettingsSection: ({ ctx }: { ctx: NavigationContext }) => {
    capture.current = ctx
    capture.providerDirty = ctx.onProviderDraftDirtyChange
    return h('div', { 'data-test-settings-section': 'providers' })
  }, AgentsSettingsSection: ({ ctx }: any) => {
    capture.current = ctx
    return h('div', { ref: ctx.agentsSectionRef },
      h('div', { ref: ctx.skillSectionRef, 'data-test-deep-link': 'skill' }),
      h('div', { ref: ctx.mcpSectionRef, 'data-test-deep-link': 'mcp' }))
  } }
})
vi.mock('./settings-section-account', async () => {
  const { createElement: h } = await import('react')
  return { AccountProfileSettingsSection: () => h('div', { 'data-test-settings-section': 'account' }) }
})
import { SettingsView } from './SettingsView'

const label = (key: string): string => i18n.t(`settings:${key}`)

describe('compact settings navigation', () => {
  let host: HTMLDivElement, root: Root
  const changeCategory = vi.fn()
  const render = async (category: ComponentProps<typeof SettingsSidebar>['category'] = 'general') => {
    await act(async () => root.render(createElement(SettingsSidebar, {
      category, setCategory: changeCategory, goBack: vi.fn(), t: label
    })))
  }
  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    host = document.createElement('div'); document.body.append(host); root = createRoot(host)
    changeCategory.mockClear()
  })
  afterEach(async () => {
    await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals()
  })
  it.each(['en', 'zh'])('offers four localized tabs and all real sections in %s', async language => {
    await i18n.changeLanguage(language)
    for (const tab of SETTINGS_TABS) {
      await render(tab.categories[0])
      const tabs = [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')]
      expect(tabs.map(button => button.textContent)).toEqual(SETTINGS_TABS.map(item => label(item.labelKey)))
      expect(host.querySelectorAll('[role="tab"][aria-selected="true"]')).toHaveLength(1)
      expect([...host.querySelectorAll<HTMLOptionElement>('select option')].map(option => option.value)).toEqual(tab.categories)
      expect(tabs.every(button => button.getAttribute('aria-controls') === 'ax-settings-tabpanel')).toBe(true)
    }
    expect(host.querySelector('input[type="search"]')).toBeNull()
    expect(host.textContent).not.toContain(label('deprecatedHubCompatibility'))
  })
  it('supports keyboard tab navigation and keeps archives and editable workspaces available', async () => {
    await render('general')
    const first = host.querySelector<HTMLButtonElement>('[role="tab"]')!
    await act(async () => first.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })))
    expect(changeCategory).toHaveBeenLastCalledWith('providers')
    await act(async () => first.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true })))
    expect(changeCategory).toHaveBeenLastCalledWith('agents')
    await render('archives')
    const select = host.querySelector<HTMLSelectElement>('select')!
    expect(select.value).toBe('archives')
    select.value = 'write'
    await act(async () => select.dispatchEvent(new Event('change', { bubbles: true })))
    expect(changeCategory).toHaveBeenLastCalledWith('write')
  })
})

describe('settings modal draft lifecycle', () => {
  const initialStore = useChatStore.getState()
  let host: HTMLDivElement, root: Root | null, bridge: Window['analytix'] | undefined
  let stored: AppSettingsV1, writes: AppSettingsPatch[], failWrite: boolean
  let writeGate: Promise<void> | null, releaseWrite: (() => void) | undefined
  let opener: HTMLButtonElement
  const setRoute = vi.fn()
  const context = () => { expect(capture.current).not.toBeNull(); return capture.current! }
  const button = (key: string) => key === 'settingsResetAppearance'
    ? host.querySelector<HTMLButtonElement>('.ax-settings-reset')!
    : [...host.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent?.trim() === label(key))!
  const click = async (key: string) => act(async () => button(key).click())
  const edit = async (patch: AppSettingsPatch) => act(async () => context().update(patch))
  const key = async (value: string) => act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: value, bubbles: true })))
  const unmount = async () => { if (root) { await act(async () => root!.unmount()); root = null } }
  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
    vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
    await i18n.changeLanguage('en'); vi.useFakeTimers()
    stored = normalizeAppSettings({ theme: 'light', locale: 'en', workspaceRoot: '/fixture/settings-modal' } as AppSettingsV1)
    writes = []; failWrite = false; writeGate = null; releaseWrite = undefined
    capture.current = null; capture.providerDirty = null; setRoute.mockClear(); bridge = window.analytix
    window.analytix = { app: {}, write: {}, updates: {},
      logs: { error: vi.fn(async () => undefined) } } as unknown as Window['analytix']
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockImplementation(async () => structuredClone(stored))
    vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost').mockImplementation(async () => structuredClone(stored))
    vi.spyOn(rendererRuntimeClient, 'setSettings').mockImplementation(async patch => {
      writes.push(structuredClone(patch))
      if (writeGate) await writeGate
      if (failWrite) throw new Error('Fixture private failure')
      stored = mergeSettings(stored, patch)
      return structuredClone(stored)
    })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(fn => { fn(0); return 1 })
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    useChatStore.setState({ route: 'settings', settingsSection: 'general', settingsReturnRoute: 'chat', setRoute,
      applyI18nFromSettings: vi.fn(async () => undefined), reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime: vi.fn(async () => undefined), showTopNotice: vi.fn() })
    opener = document.createElement('button'); opener.textContent = 'Open settings'; document.body.append(opener); opener.focus()
    host = document.createElement('div'); document.body.append(host); root = createRoot(host)
    await act(async () => root!.render(createElement(SettingsView)))
  })
  afterEach(async () => {
    await unmount(); host.remove(); opener.remove()
    if (bridge) window.analytix = bridge
    else Reflect.deleteProperty(window, 'analytix')
    useChatStore.setState(initialStore); capture.current = null
    vi.clearAllTimers(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  })
  it('starts clean after the StrictMode effect replay and closes without a discard prompt', async () => {
    await unmount()
    root = createRoot(host)
    await act(async () => root!.render(createElement(StrictMode, null, createElement(SettingsView))))
    expect(host.querySelector('[role="status"]')?.textContent).toBe('')
    expect(button('settingsSavePreferences').disabled).toBe(true)
    await key('Escape')
    expect(host.querySelector('[role="alert"]')).toBeNull()
    expect(setRoute).toHaveBeenCalledWith('chat')
    expect(writes).toEqual([])
  })
  it('previews accent, cancels without writes and preserves a failed accent draft for retry', async () => {
    await edit({ accentColor: 'orange' })
    expect(document.documentElement.dataset.accent).toBe('orange')
    expect(writes).toEqual([])
    await click('settingsCancel')
    expect(document.documentElement.dataset.accent).toBe('blue')
    expect(writes).toEqual([])
    await unmount(); root = createRoot(host)
    await act(async () => root!.render(createElement(SettingsView)))
    await edit({ accentColor: 'teal' }); failWrite = true
    await click('settingsSavePreferences')
    expect(context().form.accentColor).toBe('teal')
    expect(document.documentElement.dataset.accent).toBe('teal')
    expect(stored.accentColor).toBe('blue')
    expect(host.querySelector('[role="status"]')?.textContent).toBe(label('settingsSaveFailed'))
    failWrite = false; await click('settingsSavePreferences')
    expect(stored.accentColor).toBe('teal')
    await click('settingsCancel')
    expect(document.documentElement.dataset.accent).toBe('teal')
  })
  it('holds edits beyond the old debounce and saves once only on the explicit action', async () => {
    await edit({ codePromptPrefix: 'Pending draft' })
    await act(async () => vi.advanceTimersByTimeAsync(2500))
    expect(writes).toEqual([])
    expect(host.querySelector('[role="status"]')?.textContent).toBe(label('settingsUnsavedChanges'))
    await click('settingsSavePreferences')
    expect(writes).toEqual([{ codePromptPrefix: 'Pending draft' }])
    expect(button('settingsSavePreferences').disabled).toBe(true)
    await click('settingsCancel')
    expect(setRoute).toHaveBeenCalledWith('chat')
    await act(async () => vi.advanceTimersByTimeAsync(3000))
    expect(writes).toHaveLength(1)
  })
  it.each(['chat', 'write', 'claw', 'schedule', 'plugins'] as const)('Cancel discards without writing and returns to %s', async destination => {
    await act(async () => useChatStore.setState({ settingsReturnRoute: destination }))
    await edit({ theme: 'dark', locale: 'zh', codePromptPrefix: 'Discard this' })
    expect(document.documentElement.dataset.theme).toBe('dark')
    await click('settingsCancel')
    expect(setRoute).toHaveBeenCalledWith(destination)
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(useChatStore.getState().applyI18nFromSettings).toHaveBeenLastCalledWith('en')
    await act(async () => vi.advanceTimersByTimeAsync(2000))
    expect(writes).toEqual([])
  })
  it.each(['Escape', 'close'])('asks before discarding via %s, restores editing focus and never flushes', async close => {
    await edit({ codePromptPrefix: 'Keep until confirmed' })
    const input = host.querySelector<HTMLInputElement>('input[aria-label="Fixture draft"]')!; input.focus()
    if (close === 'Escape') await key('Escape')
    else await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${label('settingsClose')}"]`)!.click())
    expect(document.activeElement).toBe(button('settingsKeepEditing'))
    expect(setRoute).not.toHaveBeenCalled(); expect(writes).toEqual([])
    await key('Escape')
    expect(document.activeElement).toBe(input)
    expect(context().form.codePromptPrefix).toBe('Keep until confirmed')
    await key('Escape'); await click('settingsDiscard')
    expect(setRoute).toHaveBeenCalledWith('chat')
    await act(async () => vi.advanceTimersByTimeAsync(1000))
    expect(writes).toEqual([])
  })
  it('retains a failed draft and retries using the same explicit action', async () => {
    failWrite = true; await edit({ codePromptPrefix: 'Retry me' }); await click('settingsSavePreferences')
    expect(context().form.codePromptPrefix).toBe('Retry me')
    expect(host.querySelector('[role="status"]')?.textContent).toBe(label('settingsSaveFailed'))
    expect(host.textContent).not.toContain('Fixture private failure')
    failWrite = false; await click('settingsSavePreferences')
    expect(stored.codePromptPrefix).toBe('Retry me'); expect(writes).toHaveLength(2)
  })
  it('prevents close while a dispatched Save settles and does not enqueue duplicate writes', async () => {
    writeGate = new Promise<void>(resolve => { releaseWrite = resolve })
    await edit({ codePromptPrefix: 'In flight' }); await click('settingsSavePreferences')
    expect(button('settingsCancel').disabled).toBe(true)
    await key('Escape')
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${label('settingsClose')}"]`)!.click())
    expect(setRoute).not.toHaveBeenCalled(); expect(writes).toHaveLength(1)
    await act(async () => { releaseWrite!(); await writeGate })
    await click('settingsCancel')
    expect(setRoute).toHaveBeenCalledTimes(1)
    await act(async () => vi.advanceTimersByTimeAsync(2000))
    expect(writes).toHaveLength(1)
  })
  it('keeps Host changes and Provider preferences when saving an unrelated local draft', async () => {
    await edit({ codePromptPrefix: 'My change' })
    const preference = { id: 'fixture', baseUrl: 'https://provider.invalid/v1',
      endpointFormat: 'chat_completions' as const, modelProfiles: {} }
    const refreshed = { ...stored, workspaceRoot: '/fixture/new-host-root', provider: { ...stored.provider, modelPreferences: [preference] } } as AppSettingsV1
    stored = refreshed
    await act(async () => window.dispatchEvent(new CustomEvent(SETTINGS_CHANGED_EVENT, { detail: refreshed })))
    expect(context().form.codePromptPrefix).toBe('My change')
    await click('settingsSavePreferences')
    expect(writes).toEqual([{ codePromptPrefix: 'My change' }])
    expect(stored.workspaceRoot).toBe('/fixture/new-host-root')
    expect(stored.provider.modelPreferences).toEqual([preference])
  })
  it('resets appearance only and preserves provider credentials and workspace choices', async () => {
    await edit({ theme: 'dark', accentColor: 'orange', uiFontScale: 'large', workspaceRoot: '/fixture/edited-root' })
    const provider = structuredClone(context().form.provider)
    await click('settingsResetAppearance')
    expect(context().form.accentColor).toBe('blue')
    expect(context().form.theme).toBe('light'); expect(context().form.uiFontScale).toBe('small')
    expect(context().form.provider).toEqual(provider)
    expect(context().form.workspaceRoot).toBe('/fixture/edited-root'); expect(writes).toEqual([])
  })
  it('maps existing deep links to real sections and opens collapsed diagnostics', async () => {
    const owner = host.querySelector<HTMLDivElement>('.ax-settings-content')!
    owner.scrollTop = 1000
    await act(async () => useChatStore.getState().openSettings('write'))
    expect(owner.scrollTop).toBe(0)
    expect(host.querySelector('[data-test-settings-section="Write"]')).not.toBeNull()
    await act(async () => useChatStore.getState().openSettings('skill'))
    expect(host.querySelector('[data-test-deep-link="skill"]')).not.toBeNull()
    await act(async () => useChatStore.getState().openSettings('mcp'))
    expect(host.querySelector('[data-test-deep-link="mcp"]')).not.toBeNull()
    await act(async () => useChatStore.getState().openSettings('debug'))
    expect(host.querySelector<HTMLDetailsElement>('details')!.open).toBe(true)
    expect(host.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe(label('settingsTabGeneral'))
    await act(async () => useChatStore.getState().openSettings('account'))
    expect(host.querySelector('[data-test-settings-section="account"]')).not.toBeNull()
    await act(async () => useChatStore.getState().openSettings('mediaGeneration'))
    expect(host.querySelector('[data-test-settings-section="general"]')).not.toBeNull()
  })
  it('blocks a Provider draft from disappearing on tab navigation', async () => {
    await act(async () => useChatStore.getState().openSettings('providers'))
    await act(async () => capture.providerDirty!(true))
    await act(async () => host.querySelector<HTMLButtonElement>('#ax-settings-tab-workspace')!.click())
    expect(host.querySelector('[data-test-settings-section="providers"]')).not.toBeNull()
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({ tone: 'info', message: label('modelProviderUnsavedChanges') })
    expect(writes).toEqual([])
  })
  it('uses one focus owner, traps Tab and returns focus after the modal unmounts', async () => {
    expect(host.querySelectorAll('[role="dialog"][aria-modal="true"]')).toHaveLength(1)
    const cancel = button('settingsCancel'); cancel.focus()
    await act(async () => cancel.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true })))
    expect(document.activeElement).toBe(host.querySelector(`button[aria-label="${label('settingsClose')}"]`))
    await unmount()
    expect(document.activeElement).toBe(opener)
  })
})
