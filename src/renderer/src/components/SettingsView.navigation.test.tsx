// @vitest-environment jsdom
import { act, createElement, type ComponentProps, type RefObject } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsPatch, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore } from '../store/chat-store'
import { SETTINGS_CHANGED_EVENT } from '../lib/keyboard-shortcut-settings'
import i18n from '../i18n'
import { SettingsSidebar } from './SettingsSidebar'

type NavigationContext = { form: AppSettingsV1; update: (patch: AppSettingsPatch) => void; openProviderSettings: () => void; logsSectionRef: RefObject<HTMLDivElement | null> }
const capture = vi.hoisted(() => ({
  current: null as NavigationContext | null,
  providerDirty: null as ((dirty: boolean) => void) | null,
  retiredDebugRender: vi.fn(() => null)
}))

vi.mock('./settings-sections', async () => {
  const { createElement: h } = await import('react')
  const empty = () => null
  const out = Object.fromEntries(['ArchivedThreads','Browser','Claw','ComputerUse','ImageGeneration','KeyboardShortcuts','Worktree','Memory','Permissions','SpeechToText','Updates','Write'].map(n => [n + 'SettingsSection', empty]))
  return { ...out, GeneralSettingsSection: ({ ctx }: { ctx: NavigationContext }) => {
    capture.current = ctx
    return h('div', { 'data-test-settings-section': 'general', 'data-test-draft': ctx.form.codePromptPrefix },
      h('div', { ref: ctx.logsSectionRef, 'data-test-deep-link': 'logs' }),
      h('button', { 'data-test-provider-settings': '', onClick: ctx.openProviderSettings }, 'Fixture Providers'))
  }, ProvidersSettingsSection: ({ ctx }: { ctx: { onProviderDraftDirtyChange: (dirty: boolean) => void } }) => {
    capture.providerDirty = ctx.onProviderDraftDirtyChange
    return h('div', { 'data-test-settings-section': 'providers' })
  }, LlmDebugSettingsSection: capture.retiredDebugRender,
  AgentsSettingsSection: ({ ctx }: any) => h('div', { ref: ctx.agentsSectionRef }, h('div', { ref: ctx.skillSectionRef, 'data-test-deep-link': 'skill' }), h('div', { ref: ctx.mcpSectionRef, 'data-test-deep-link': 'mcp' })) }
})
vi.mock('./settings-section-account', async () => {
  const { createElement: h } = await import('react')
  return { AccountProfileSettingsSection: () => h('div', { 'data-test-settings-section': 'account' }) }
})
import { SettingsView } from './SettingsView'

describe('SettingsSidebar navigation', () => {
  let host: HTMLDivElement, root: ReturnType<typeof createRoot>
  const changeCategory = vi.fn()
  const t = (key: string): string => i18n.t(`settings:${key}`)
  const render = async (category: ComponentProps<typeof SettingsSidebar>['category'] = 'general') => {
    await act(async () => root.render(createElement(SettingsSidebar, { category, goBack: vi.fn(), setCategory: changeCategory, t })))
  }
  const search = async (value: string) => {
    const input = host.querySelector<HTMLInputElement>('input')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  const archives = () => [...host.querySelectorAll<HTMLButtonElement>('button')]
    .find(button => button.textContent?.trim() === t('archives'))
  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    await i18n.changeLanguage('en')
    changeCategory.mockClear()
    host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  })
  afterEach(async () => {
    await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals()
  })
  it.each(['en', 'zh'])('keeps three groups, Personal shortcuts and an independent Archives entry in %s', async language => {
    await i18n.changeLanguage(language); await render()
    const groups = [...host.querySelectorAll('section[aria-label]')]
    expect(groups.map(group => group.getAttribute('aria-label'))).toEqual([
      t('settingsNavPersonal'), t('settingsNavIntegrations'), t('settingsNavCoding')
    ])
    expect(groups[0].textContent).toContain(t('keyboardShortcuts'))
    expect(groups[2].textContent).not.toContain(t('keyboardShortcuts'))
    expect(host.querySelectorAll('nav button')).toHaveLength(14)
    expect(archives()?.closest('section')).toBeNull()
    expect(host.textContent).not.toContain(t('debug'))
    await act(async () => archives()!.click())
    expect(changeCategory).toHaveBeenCalledWith('archives')
    await render('archives')
    expect(archives()?.getAttribute('aria-current')).toBe('page')
  })
  it('finds Archives through search, reports an empty match and restores the utility when cleared', async () => {
    await render(); await search('archive')
    expect(archives()).toBeDefined()
    expect(host.textContent).not.toContain(t('settingsSearchEmpty'))
    await act(async () => archives()!.click())
    expect(changeCategory).toHaveBeenCalledWith('archives')
    await search('unmatched-fixture-query')
    expect(archives()).toBeUndefined()
    expect(host.textContent).toContain(t('settingsSearchEmpty'))
    const clear = host.querySelector<HTMLButtonElement>(`button[aria-label="${t('settingsSearchClear')}"]`)!
    await act(async () => clear.click())
    expect(host.querySelector<HTMLInputElement>('input')!.value).toBe('')
    expect(archives()).toBeDefined()
  })
  it.each([
    ['en', 'logs'], ['en', 'diagnostics'], ['en', 'debug'], ['en', 'Local error log'],
    ['zh', '排障'], ['zh', '本地错误日志']
  ])('finds General for %s log query %s without restoring Debug', async (language, query) => {
    await i18n.changeLanguage(language); await render(); await search(query)
    const entries = [...host.querySelectorAll<HTMLButtonElement>('nav button')]
    expect(entries).toHaveLength(1)
    expect(entries[0].textContent).toContain(t('general'))
    await act(async () => entries[0].click())
    expect(changeCategory).toHaveBeenCalledWith('general')
    expect(capture.retiredDebugRender).not.toHaveBeenCalled()
  })
})

it('resets only the content owner when category changes, preserves same-category settings refresh and deep links', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  const initial = useChatStore.getState(), settings = normalizeAppSettings({
    workspaceRoot: '/synthetic/a',
    runtime: {
      textToSpeech: { model: 'speech-2.8-hd' },
      musicGeneration: { model: 'music-2.6' },
      videoGeneration: { model: 'MiniMax-Hailuo-2.3' }
    },
    provider: { providers: [{ id: 'legacy-media', name: 'Legacy media', baseUrl: 'https://media.invalid',
      endpointFormat: 'chat_completions', models: [], modelProfiles: {} }] }
  } as unknown as AppSettingsV1)
  const legacyProfile = Object.assign(settings.provider.providers[0], { apiKey: 'sk-test' })
  expect(legacyProfile.apiKey).toBe('sk-test')
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
    await act(async () => useChatStore.setState({ settingsSection: 'mcp' }))
    expect(host.querySelector('[data-test-deep-link="mcp"]')).not.toBeNull()
    expect(host.querySelector('.ds-settings-sidebar [aria-current="page"]')?.textContent).toBe(i18n.t('agents', { ns: 'settings' }))
    await act(async () => useChatStore.setState({ settingsSection: 'mediaGeneration' }))
    expect(host.querySelector('[data-test-settings-section="general"]')).not.toBeNull()
    expect(host.querySelector('.ds-settings-sidebar [aria-current="page"]')?.textContent).toBe(i18n.t('general', { ns: 'settings' }))
    await act(async () => useChatStore.setState({ settingsSection: 'debug' }))
    expect(host.querySelector('[data-test-settings-section="general"]')).not.toBeNull()
    expect(scroll.mock.contexts.at(-1)).toBe(host.querySelector('[data-test-deep-link="logs"]'))
    expect(capture.retiredDebugRender).not.toHaveBeenCalled()
    expect(settings.runtime.textToSpeech.model).toBe('speech-2.8-hd')
    expect(settings.runtime.musicGeneration.model).toBe('music-2.6')
    expect(settings.runtime.videoGeneration.model).toBe('MiniMax-Hailuo-2.3')
    for (const saved of ['speech-2.8-hd', 'music-2.6', 'MiniMax-Hailuo-2.3', 'sk-test']) expect(host.innerHTML).not.toContain(saved)
    expect(host.textContent).not.toContain(i18n.t('mediaGeneration', { ns: 'settings' }))
    expect(host.textContent).not.toContain(i18n.t('deprecatedHubCompatibility', { ns: 'settings' }))
    await act(async () => useChatStore.setState({ settingsSection: 'account' }))
    expect(host.querySelector('[data-test-settings-section="account"]')).not.toBeNull()
    expect(host.querySelector('.ds-settings-sidebar [aria-current="page"]')).toBeNull()
    await act(async () => useChatStore.setState({ settingsSection: 'general' }))
    expect(host.querySelector('[data-test-settings-section="general"]')).not.toBeNull()
    expect(host.querySelector('.ds-settings-sidebar [aria-current="page"]')?.textContent).toBe(i18n.t('general', { ns: 'settings' }))
    expect(host.textContent).not.toContain('Mascot modes')
  } finally { await act(async () => root.unmount()); host.remove(); useChatStore.setState(initial); raf.mockRestore(); vi.restoreAllMocks() }
})


describe('SettingsView departure requires the current draft to be saved', () => {
  const initial = useChatStore.getState()
  let root: ReturnType<typeof createRoot>, host: HTMLDivElement, bridge: Window['analytix'] | undefined
  let stored: AppSettingsV1, failed: boolean, writes: AppSettingsPatch[]
  let writeGate: Promise<void> | null, releaseWrite: (() => void) | undefined
  let reloadGate: Promise<void> | null, releaseReload: (() => void) | undefined
  const chat = vi.fn(async () => undefined), write = vi.fn(async () => undefined), claw = vi.fn(),
    schedule = vi.fn(), plugins = vi.fn(), setRoute = vi.fn(), onboarding = vi.fn()
  const destinations = () => chat.mock.calls.length + write.mock.calls.length + claw.mock.calls.length +
    schedule.mock.calls.length + plugins.mock.calls.length + setRoute.mock.calls.length + onboarding.mock.calls.length
  const context = () => { expect(capture.current).not.toBeNull(); return capture.current! }
  const edit = async (value: string) => act(async () => context().update({ codePromptPrefix: value }))
  const click = async (entry: 'back' | 'chat' | 'plugins' | 'schedule' | 'providers') => {
    await act(async () => {
      if (entry === 'providers') host.querySelector<HTMLButtonElement>('[data-test-provider-settings]')!.click()
      else if (entry === 'back') [...host.querySelectorAll<HTMLButtonElement>('.ds-settings-sidebar button')]
        .find(button => button.textContent?.trim() === i18n.t('settings:back'))!.click()
      else {
        const key = entry === 'chat' ? 'chatNavigationLabel' : entry
        host.querySelector<HTMLButtonElement>(`.ds-navigation-rail button[aria-label="${i18n.t('common:' + key)}"]`)!.click()
      }
    })
  }
  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
    vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
    await i18n.changeLanguage('en'); vi.useFakeTimers()
    stored = normalizeAppSettings({ workspaceRoot: '/fixture/settings-departure' } as AppSettingsV1)
    failed = false; writes = []; writeGate = null; reloadGate = null; releaseWrite = undefined; releaseReload = undefined
    capture.current = null; capture.providerDirty = null; bridge = window.analytix
    window.analytix = { app: {}, write: {}, updates: {}, settings: { getSettings: async () => stored },
      logs: { error: vi.fn(async () => undefined) } } as unknown as Window['analytix']
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockImplementation(async () => stored)
    vi.spyOn(rendererRuntimeClient, 'setSettings').mockImplementation(async partial => {
      writes.push(structuredClone(partial))
      if (writeGate) await writeGate
      if (failed) throw new Error('Fixture save failed')
      stored = { ...stored, ...partial } as AppSettingsV1
      return stored
    })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(fn => { fn(0); return 1 })
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    useChatStore.setState({ route: 'settings', settingsSection: 'general', settingsReturnRoute: 'chat',
      openCode: chat, openWrite: write, openClaw: claw, openSchedule: schedule, openPlugins: plugins,
      setRoute, openInitialSetup: onboarding, applyI18nFromSettings: vi.fn(async () => undefined),
      reloadUiSettings: vi.fn(async () => { if (reloadGate) await reloadGate }),
      showTopNotice: vi.fn(), probeRuntime: vi.fn(async () => undefined) })
    host = document.createElement('div'); document.body.append(host); root = createRoot(host)
    await act(async () => root.render(createElement(SettingsView)))
    expect(capture.current).not.toBeNull()
  })
  afterEach(async () => {
    await act(async () => root.unmount()); host.remove(); capture.current = null
    if (bridge) window.analytix = bridge
    else Reflect.deleteProperty(window, 'analytix')
    useChatStore.setState(initial); vi.clearAllTimers(); vi.useRealTimers()
    vi.clearAllMocks(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  })
  it.each(['chat', 'write', 'claw', 'schedule', 'plugins'] as const)('retains a failed draft before returning to %s and retries successfully', async destination => {
    useChatStore.setState({ settingsReturnRoute: destination })
    failed = true; await edit('Retained failed draft'); await click('back')
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('Retained failed draft')
    expect(host.querySelector('[role="status"]')?.textContent).toBe('Fixture save failed')
    failed = false; await click('back')
    expect(destinations()).toBe(1); expect(writes.at(-1)?.codePromptPrefix).toBe('Retained failed draft')
    const selected = destination === 'chat' ? chat : destination === 'write' ? write : destination === 'claw' ? claw
      : destination === 'schedule' ? schedule : setRoute
    expect(selected).toHaveBeenCalledTimes(1)
  })
  it.each(['chat', 'plugins', 'schedule'] as const)('blocks %s departure on a failed save and preserves a retry', async entry => {
    failed = true; await edit('Rail draft'); await click(entry)
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('Rail draft')
    failed = false; await click(entry)
    expect(destinations()).toBe(1)
  })
  it('opens Providers from the General configuration entry without opening setup', async () => {
    await click('providers')
    expect(useChatStore.getState().route).toBe('settings')
    expect(useChatStore.getState().settingsSection).toBe('providers')
    expect(host.querySelector('[data-test-settings-section="providers"]')).not.toBeNull()
    expect(writes).toHaveLength(0)
    expect(onboarding).not.toHaveBeenCalled()
    expect(destinations()).toBe(0)
  })
  it('keeps a failed General draft until the Providers entry can save it and retry', async () => {
    failed = true; await edit('Provider entry retained draft'); await click('providers')
    expect(useChatStore.getState().settingsSection).toBe('general')
    expect(host.querySelector('[data-test-settings-section="providers"]')).toBeNull()
    expect(context().form.codePromptPrefix).toBe('Provider entry retained draft')
    expect(host.querySelector('[role="status"]')?.textContent).toBe('Fixture save failed')
    failed = false; await click('providers')
    expect(writes.at(-1)?.codePromptPrefix).toBe('Provider entry retained draft')
    expect(stored.codePromptPrefix).toBe('Provider entry retained draft')
    expect(useChatStore.getState().settingsSection).toBe('providers')
    expect(host.querySelector('[data-test-settings-section="providers"]')).not.toBeNull()
    expect(onboarding).not.toHaveBeenCalled()
    expect(destinations()).toBe(0)
  })
  it('offers an explicit save retry without discarding the draft or duplicating an accepted save on Back', async () => {
    failed = true; await edit('Draft to retry'); await click('back')
    const retry = [...host.querySelectorAll<HTMLButtonElement>('button')]
      .find(button => button.textContent?.trim() === i18n.t('settings:retrySave'))!
    expect(retry).not.toBeUndefined()
    failed = false; await act(async () => retry.click())
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('Draft to retry')
    expect(writes).toHaveLength(2)
    await click('back'); expect(destinations()).toBe(1); expect(writes).toHaveLength(2)
  })
  it('keeps an edit made during SET and its debounce instead of leaving on the old reply', async () => {
    writeGate = new Promise<void>(done => { releaseWrite = done })
    await edit('First draft'); await click('back'); expect(writes).toHaveLength(1)
    await edit('New draft during save')
    await act(async () => { releaseWrite!() })
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('New draft during save')
    writeGate = null
    await act(async () => { await vi.advanceTimersByTimeAsync(450) })
    expect(writes.at(-1)?.codePromptPrefix).toBe('New draft during save')
    await click('back'); expect(destinations()).toBe(1)
  })
  it('retains an edit and rejects stale Host refresh during the awaited UI reload', async () => {
    reloadGate = new Promise<void>(done => { releaseReload = done })
    await edit('Saved before reload'); await click('back')
    await edit('New edit during reload')
    await act(async () => window.dispatchEvent(new CustomEvent(SETTINGS_CHANGED_EVENT, { detail: stored })))
    expect(context().form.codePromptPrefix).toBe('New edit during reload')
    await act(async () => { releaseReload!() })
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('New edit during reload')
  })
  it('ignores a rejected old reload after a new edit without changing its saving status', async () => {
    let rejectReload!: (reason: unknown) => void
    reloadGate = new Promise<void>((_resolve, reject) => { rejectReload = reject })
    // The existing background reload succeeds; hold only this explicit departure reload.
    vi.mocked(useChatStore.getState().reloadUiSettings).mockResolvedValueOnce(undefined)
    await edit('Saved before old reload'); await click('back')
    await edit('New draft during old reload')
    await act(async () => rejectReload(new Error('Fixture old reload failure')))
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('New draft during old reload')
    expect(host.textContent).toContain(i18n.t('settings:applying'))
    expect(host.textContent).not.toContain(i18n.t('settings:applyFailed'))
    expect(useChatStore.getState().showTopNotice).not.toHaveBeenCalled()
  })
  it('reports a current departure reload failure without offering an ineffective save retry', async () => {
    let rejectReload!: (reason: unknown) => void
    reloadGate = new Promise<void>((_resolve, reject) => { rejectReload = reject })
    vi.mocked(useChatStore.getState().reloadUiSettings).mockResolvedValueOnce(undefined)
    await edit('Accepted draft'); await click('back')
    await act(async () => rejectReload(new Error('Fixture departure reload failure')))
    expect(destinations()).toBe(0); expect(context().form.codePromptPrefix).toBe('Accepted draft')
    expect(host.textContent).not.toContain(i18n.t('settings:applyFailed'))
    expect([...host.querySelectorAll('button')].some(button => button.textContent?.trim() ===
      i18n.t('settings:retrySave'))).toBe(false)
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({
      tone: 'error', message: 'Fixture departure reload failure'
    })
    reloadGate = null; await click('back')
    expect(destinations()).toBe(1); expect(writes).toHaveLength(1)
  })
  it('does not use a retired Settings entry to navigate after late success', async () => {
    writeGate = new Promise<void>(done => { releaseWrite = done })
    await edit('Entry to retire'); await click('back')
    await act(async () => root.render(null))
    await act(async () => { releaseWrite!() })
    expect(destinations()).toBe(0)
  })
  it('keeps an invalid port on the current page without attempting a write', async () => {
    await act(async () => context().update({ runtime: { port: 0 } }))
    await click('back')
    expect(writes).toHaveLength(0); expect(destinations()).toBe(0)
    expect(context().form.runtime.port).toBe(0)
  })
  it('honors only the latest explicit departure while the save queue drains', async () => {
    writeGate = new Promise<void>(done => { releaseWrite = done })
    await edit('One current draft'); await click('plugins'); await click('chat')
    await act(async () => { releaseWrite!() })
    expect(plugins).not.toHaveBeenCalled(); expect(chat).toHaveBeenCalledTimes(1)
  })
  it('synchronizes accepted categories so repeated external Provider navigation remains actionable', async () => {
    const sidebarClick = async (key: string) => act(async () => {
      [...host.querySelectorAll<HTMLButtonElement>('.ds-settings-sidebar button')]
        .find(button => button.textContent?.trim() === i18n.t(`settings:${key}`))!.click()
    })
    await sidebarClick('providers')
    expect(useChatStore.getState().settingsSection).toBe('providers')
    await sidebarClick('general')
    expect(useChatStore.getState().settingsSection).toBe('general')
    await act(async () => useChatStore.getState().openSettings('providers'))
    expect(host.querySelector('[data-test-settings-section="providers"]')).not.toBeNull()
    await act(async () => capture.providerDirty!(true))
    await act(async () => useChatStore.getState().openSettings('mcp'))
    expect(useChatStore.getState().settingsSection).toBe('providers')
    expect(host.querySelector('[data-test-deep-link="mcp"]')).toBeNull()
    await act(async () => capture.providerDirty!(false))
    await act(async () => useChatStore.getState().openSettings('mcp'))
    expect(useChatStore.getState().settingsSection).toBe('mcp')
    expect(host.querySelector('[data-test-deep-link="mcp"]')).not.toBeNull()
  })

  it('keeps an unsaved Provider draft when sidebar utilities or the legacy Debug link request another page', async () => {
    const sidebarClick = async (key: string) => act(async () => {
      [...host.querySelectorAll<HTMLButtonElement>('.ds-settings-sidebar button')]
        .find(button => button.textContent?.trim() === i18n.t(`settings:${key}`))!.click()
    })
    await sidebarClick('providers')
    expect(capture.providerDirty).not.toBeNull()
    await act(async () => capture.providerDirty!(true))
    await sidebarClick('archives'); await sidebarClick('keyboardShortcuts')
    await act(async () => useChatStore.setState({ settingsSection: 'debug' }))
    expect(host.querySelector('[data-test-settings-section="providers"]')).not.toBeNull()
    expect(host.querySelector('[data-test-deep-link="logs"]')).toBeNull()
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({
      tone: 'info', message: i18n.t('settings:modelProviderUnsavedChanges')
    })
    expect(capture.retiredDebugRender).not.toHaveBeenCalled()
  })
})
