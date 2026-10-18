// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsPatch, type AppSettingsV1 } from '@shared/app-settings'
import { createRuntimeStatusPublicV1 } from '@shared/analytix-runtime-status'
import type { AnalytixRuntimeStatusPayload } from '@shared/analytix-api'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore } from '../store/chat-store'
import { SETTINGS_CHANGED_EVENT, emitRendererSettingsChanged } from '../lib/keyboard-shortcut-settings'
import { adoptPerformanceUsageHostReadback } from './chat/presentation/chat/TurnUsagePanel'
import { mergeSettings } from './settings-utils'
import { settingsPatchSchema } from '../../../main/ipc/app-ipc-schemas'
import i18n from '../i18n'

const sectionProbe = vi.hoisted(() => ({
  current: null as { form: AppSettingsV1; update: (patch: AppSettingsPatch) => void } | null,
  saveModelPreferences: null as ((partial: AppSettingsPatch) => Promise<AppSettingsV1>) | null
}))

vi.mock('./settings-section-ui-theme', () => ({ UiThemeSettingsSection: () => null }))
vi.mock('./settings-sections', async () => {
  const { GeneralSettingsSection } = await import('./settings-section-general')
  const { ComputerUseSettingsSection } = await import('./settings-section-computer-use')
  const { createElement: h } = await import('react')
  const empty = () => null
  const sections = Object.fromEntries([
    'Agents', 'ArchivedThreads', 'Browser', 'Claw', 'ComputerUse', 'ImageGeneration',
    'KeyboardShortcuts', 'LlmDebug', 'Worktree', 'MediaGeneration', 'Memory',
    'Permissions', 'Providers', 'SpeechToText', 'Updates', 'Write'
  ].map(name => [`${name}SettingsSection`, empty]))
  return { ...sections, GeneralSettingsSection: (props: Parameters<typeof GeneralSettingsSection>[0]) => {
    // Observe the existing section callback while retaining the real General UI.
    sectionProbe.saveModelPreferences = props.ctx.saveModelPreferences
    sectionProbe.current = { form: props.ctx.form, update: props.ctx.update }
    return h(GeneralSettingsSection, props)
  }, ComputerUseSettingsSection: (props: Parameters<typeof ComputerUseSettingsSection>[0]) => {
    sectionProbe.current = { form: props.ctx.form, update: props.ctx.update }
    return h(ComputerUseSettingsSection, props)
  } }
})
import { SettingsView } from './SettingsView'

const initialStore = useChatStore.getState()
const getSettings = vi.fn<() => Promise<AppSettingsV1>>()
const setSettings = vi.fn<(patch: AppSettingsPatch) => Promise<AppSettingsV1>>()
let root: Root | null, host: HTMLDivElement, stored: AppSettingsV1
let originalBridge: Window['analytix'] | undefined
let events: AppSettingsV1[]
let runtimeStatusHandler: ((status: AnalytixRuntimeStatusPayload) => void) | null
let choiceNotifications: number
let removeEventListener: () => void
async function mount() { root = createRoot(host); await act(async () => root!.render(createElement(SettingsView))) }
async function unmount() { if (root) { await act(async () => root!.unmount()); root = null } }
function usageButton(): HTMLButtonElement {
  const button = Array.from(host.querySelectorAll<HTMLButtonElement>('button'))
    .find(candidate => candidate.getAttribute('aria-label') === i18n.t('settings:performanceUsage'))
  expect(button, host.textContent ?? '').toBeDefined(); return button!
}
async function choose(label: 'Compact' | 'Detailed') {
  await act(async () => usageButton().click())
  const item = document.body.querySelector<HTMLButtonElement>(`[role="menuitem"][aria-label="${label}"]`)
  expect(item).not.toBeNull(); await act(async () => item!.click())
}
async function advanceSave() { await act(async () => vi.advanceTimersByTimeAsync(450)) }
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason: unknown) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
  vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 16))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  await i18n.changeLanguage('en'); vi.useFakeTimers()
  stored = normalizeAppSettings({ locale: 'en', workspaceRoot: '/synthetic/settings-usage', performanceUsageMode: 'detailed' } as AppSettingsV1)
  getSettings.mockReset().mockImplementation(async () => structuredClone(stored))
  setSettings.mockReset().mockImplementation(async patch => {
    stored = mergeSettings(stored, settingsPatchSchema.parse(patch) as AppSettingsPatch)
    return structuredClone(stored)
  })
  events = []; originalBridge = window.analytix; runtimeStatusHandler = null
  window.analytix = { settings: { getSettings, setSettings }, app: { invalidateQueryCache: vi.fn(async () => undefined) },
    runtime: { onRuntimeStatus: (handler: (status: AnalytixRuntimeStatusPayload) => void) => {
      runtimeStatusHandler = handler; return () => { runtimeStatusHandler = null }
    } }, write: {}, updates: {}, logs: { error: vi.fn(async () => undefined) } } as unknown as Window['analytix']
  rendererRuntimeClient.invalidateSettings()
  // Reset the actual shared display projection before observing this fixture's events.
  adoptPerformanceUsageHostReadback(stored)
  choiceNotifications = 0
  useChatStore.setState({ settingsSection: 'general', applyI18nFromSettings: vi.fn(async () => undefined),
    reloadUiSettings: vi.fn(async () => undefined), probeRuntime: vi.fn(async () => undefined), showTopNotice: vi.fn() })
  const onChanged = (event: Event) => events.push((event as CustomEvent<AppSettingsV1>).detail)
  const onChoice = () => { choiceNotifications += 1 }
  window.addEventListener(SETTINGS_CHANGED_EVENT, onChanged)
  window.addEventListener('analytix:performance-usage-choice', onChoice)
  removeEventListener = () => {
    window.removeEventListener(SETTINGS_CHANGED_EVENT, onChanged)
    window.removeEventListener('analytix:performance-usage-choice', onChoice)
  }
  sectionProbe.current = null
  sectionProbe.saveModelPreferences = null
  host = document.createElement('div'); document.body.append(host); await mount()
})
afterEach(async () => {
  await unmount(); removeEventListener(); host.remove(); rendererRuntimeClient.invalidateSettings()
  if (originalBridge) window.analytix = originalBridge; else Reflect.deleteProperty(window, 'analytix')
  sectionProbe.current = null
  sectionProbe.saveModelPreferences = null
  useChatStore.setState(initialStore); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks()
})

it.each(['en', 'zh'])('routes the real General configuration button to Providers in %s', async language => {
  await act(async () => { await i18n.changeLanguage(language) })
  const onboarding = vi.fn()
  await act(async () => useChatStore.setState({ openInitialSetup: onboarding, initialSetupOpen: false, initialSetupMode: 'required' }))
  expect(host.textContent).not.toContain(i18n.t('settings:onboardingPreview'))
  const button = [...host.querySelectorAll<HTMLButtonElement>('.ds-settings-content button')]
    .find(candidate => candidate.textContent?.trim() === i18n.t('settings:providers'))
  expect(button).toBeDefined()
  await act(async () => button!.click())
  expect(useChatStore.getState().settingsSection).toBe('providers')
  expect(host.querySelector('.ds-settings-sidebar [aria-current="page"]')?.textContent).toBe(i18n.t('settings:providers'))
  expect(onboarding).not.toHaveBeenCalled()
  expect(useChatStore.getState().initialSetupOpen).toBe(false)
  expect(useChatStore.getState().initialSetupMode).toBe('required')
})

describe('normal General performance and usage preference', () => {
  it('saves through the actual Settings queue and restores Host readback', async () => {
    expect(usageButton().textContent).toContain('Detailed')
    await choose('Compact')
    expect(usageButton().textContent).toContain('Compact'); expect(setSettings).not.toHaveBeenCalled()
    await advanceSave()
    expect(setSettings).toHaveBeenCalledTimes(1)
    expect(setSettings.mock.calls[0]![0].performanceUsageMode).toBe('compact')
    expect(events).toHaveLength(1); expect(events[0]!.performanceUsageMode).toBe('compact')
    expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).performanceUsageMode).toBe('compact')
    await unmount(); await mount()
    expect(usageButton().textContent).toContain('Compact')
  })
  it('recovers a failed desktop save from fresh Host readback and retries through the real menu', async () => {
    setSettings.mockRejectedValueOnce(new Error('Synthetic settings persistence failure'))
    await choose('Compact'); await advanceSave()
    expect(usageButton().textContent).toContain('Detailed'); expect(events).toHaveLength(0)
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalled()
    expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).performanceUsageMode).toBe('detailed')
    await choose('Compact'); await advanceSave()
    expect(setSettings).toHaveBeenCalledTimes(2); expect(events).toHaveLength(1)
    expect(events[0]!.performanceUsageMode).toBe('compact')
    expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).performanceUsageMode).toBe('compact')
  })
  it('drains a failed write while recovery is pending and ignores its stale result', async () => {
    const oldHost = structuredClone(stored)
    const recovery = deferred<AppSettingsV1>()
    const readSettingsFromHost = vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost')
    const invalidateSettings = vi.spyOn(rendererRuntimeClient, 'invalidateSettings')
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockImplementationOnce(() => recovery.promise)
    setSettings.mockRejectedValueOnce(new Error('Synthetic settings persistence failure'))
    await choose('Compact'); await advanceSave()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(readSettingsFromHost).toHaveBeenCalledTimes(1)
    expect(invalidateSettings).not.toHaveBeenCalled()
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('detailed')
    expect(usageButton().textContent).toContain('Compact'); expect(events).toHaveLength(0)

    await choose('Detailed'); await choose('Compact'); await advanceSave()
    // Recovery is still unresolved: the second normal write already settled.
    expect(setSettings).toHaveBeenCalledTimes(2)
    expect(stored.performanceUsageMode).toBe('compact')
    expect(events).toHaveLength(1); expect(events[0]!.performanceUsageMode).toBe('compact')
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
    const notificationsBefore = choiceNotifications
    await act(async () => recovery.resolve(oldHost))
    expect(usageButton().textContent).toContain('Compact')
    expect(choiceNotifications).toBe(notificationsBefore); expect(events).toHaveLength(1)
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(invalidateSettings).not.toHaveBeenCalled()
  })
  it('rejects an older recovery request even when queued explicit writes share the same draft', async () => {
    const oldHost = structuredClone(stored)
    const older = deferred<void>(), recovery = deferred<AppSettingsV1>(), latest = deferred<void>()
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockImplementationOnce(() => recovery.promise)
    setSettings.mockImplementationOnce(async () => {
      await older.promise
      throw new Error('Synthetic older write failure')
    }).mockRejectedValueOnce(new Error('Synthetic queued write failure'))
      .mockImplementationOnce(async patch => {
        await latest.promise
        stored = mergeSettings(stored, settingsPatchSchema.parse(patch) as AppSettingsPatch)
        return structuredClone(stored)
      })
    await choose('Compact'); await advanceSave()
    expect(setSettings).toHaveBeenCalledTimes(1)
    expect(sectionProbe.saveModelPreferences).not.toBeNull()
    let first!: Promise<AppSettingsV1>, second!: Promise<AppSettingsV1>
    await act(async () => {
      // Use the same real SettingsView queue callback as model-preference saves.
      // Both explicit writes will begin against this final form/draft generation.
      first = sectionProbe.saveModelPreferences!({ provider: { modelPreferences: [] } })
      void first.catch(() => undefined)
      second = sectionProbe.saveModelPreferences!({ provider: { modelPreferences: [] } })
      void second.catch(() => undefined)
    })
    await act(async () => older.resolve(undefined))
    expect(setSettings).toHaveBeenCalledTimes(3)
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(events).toHaveLength(0)
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('detailed')
    const notificationsBefore = choiceNotifications
    await act(async () => recovery.resolve(oldHost))
    // Entry, generation and failedForm still match: only the write identity is newer.
    expect(usageButton().textContent).toContain('Compact')
    expect(choiceNotifications).toBe(notificationsBefore)
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('detailed')
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    await act(async () => { latest.resolve(undefined); await second })
    await expect(first).rejects.toThrow('Synthetic queued write failure')
    expect(usageButton().textContent).toContain('Compact')
    expect(events).toHaveLength(1); expect(events[0]!.performanceUsageMode).toBe('compact')
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
  })
  it('preserves a newer accepted external form at the same draft generation', async () => {
    const oldHost = structuredClone(stored)
    const recovery = deferred<AppSettingsV1>()
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockImplementationOnce(() => recovery.promise)
    setSettings.mockRejectedValueOnce(new Error('Synthetic settings persistence failure'))
    await choose('Compact'); await advanceSave()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    await act(async () => {
      const accepted = await rendererRuntimeClient.setSettings({ performanceUsageMode: 'compact' })
      emitRendererSettingsChanged(accepted)
    })
    expect(events).toHaveLength(1); expect(events[0]!.performanceUsageMode).toBe('compact')
    const notificationsBefore = choiceNotifications
    await act(async () => recovery.resolve(oldHost))
    expect(usageButton().textContent).toContain('Compact')
    expect(choiceNotifications).toBe(notificationsBefore); expect(events).toHaveLength(1)
    expect(setSettings).toHaveBeenCalledTimes(2)
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
  })
  it('retires a pending recovery when the normal Settings entry unmounts', async () => {
    const oldHost = structuredClone(stored)
    const recovery = deferred<AppSettingsV1>()
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockImplementationOnce(() => recovery.promise)
    setSettings.mockRejectedValueOnce(new Error('Synthetic settings persistence failure'))
    await choose('Compact'); await advanceSave()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    await unmount()
    const notificationsBefore = choiceNotifications
    await act(async () => recovery.resolve(oldHost))
    expect(choiceNotifications).toBe(notificationsBefore); expect(events).toHaveLength(0)
    expect(setSettings).toHaveBeenCalledTimes(1)
  })
  it('preserves the original error when recovery rejects and allows a new normal save', async () => {
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockRejectedValueOnce(new Error('Synthetic recovery read failure'))
    setSettings.mockRejectedValueOnce(new Error('Synthetic settings persistence failure'))
    await choose('Compact'); await advanceSave()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(usageButton().textContent).toContain('Compact'); expect(events).toHaveLength(0)
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({
      tone: 'error', message: 'Synthetic settings persistence failure'
    })
    await choose('Detailed'); await advanceSave()
    expect(setSettings).toHaveBeenCalledTimes(2); expect(events).toHaveLength(1)
    expect(events[0]!.performanceUsageMode).toBe('detailed')
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('detailed')
  })
  it('clears failed pending display choice on successful normal entry readback and permits retry', async () => {
    getSettings.mockRejectedValueOnce(new Error('Synthetic recovery read failure'))
    setSettings.mockRejectedValueOnce(new Error('Synthetic settings persistence failure'))
    await choose('Compact'); await advanceSave()
    expect(usageButton().textContent).toContain('Compact'); expect(events).toHaveLength(0)
    await unmount()
    const readsBefore = getSettings.mock.calls.length
    // Normal entry GET succeeds after both the earlier SET and its recovery GET failed.
    await mount()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(usageButton().textContent).toContain('Detailed'); expect(events).toHaveLength(0)
    await choose('Compact'); await advanceSave()
    expect(setSettings).toHaveBeenCalledTimes(2); expect(events).toHaveLength(1)
    expect(events[0]!.performanceUsageMode).toBe('compact')
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
  })
  it('converges ordinary cached reads after Host persistence followed by IPC failure', async () => {
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('detailed')
    const recovery = deferred<AppSettingsV1>()
    const readSettingsFromHost = vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost')
    const invalidateSettings = vi.spyOn(rendererRuntimeClient, 'invalidateSettings')
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockImplementationOnce(() => recovery.promise)
    setSettings.mockImplementationOnce(async patch => {
      stored = mergeSettings(stored, settingsPatchSchema.parse(patch) as AppSettingsPatch)
      throw new Error('Synthetic failure after Host persistence')
    })
    await choose('Compact'); await advanceSave()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(readSettingsFromHost).toHaveBeenCalledTimes(1)
    expect(invalidateSettings).not.toHaveBeenCalled()
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('detailed')
    expect(stored.performanceUsageMode).toBe('compact')
    await act(async () => recovery.resolve(structuredClone(stored)))
    expect(invalidateSettings).toHaveBeenCalledTimes(1)
    expect(usageButton().textContent).toContain('Compact'); expect(events).toHaveLength(0)
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({
      tone: 'error', message: 'Synthetic failure after Host persistence'
    })
    // No forceRefresh: recovery itself must retire the previously cached Detailed value.
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 2)
    expect(setSettings).toHaveBeenCalledTimes(1); expect(events).toHaveLength(0)
  })
  it('reconciles a changed Host preference on a fresh normal Settings mount', async () => {
    await choose('Compact'); await advanceSave()
    await unmount()
    // Synthetic Host changes while this normal entry is absent; no display event is injected.
    stored = mergeSettings(stored, { performanceUsageMode: 'detailed' })
    const readsBefore = getSettings.mock.calls.length
    await mount()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    expect(usageButton().textContent).toContain('Detailed')
    expect(setSettings).toHaveBeenCalledTimes(1); expect(events).toHaveLength(1)
  })
  it('preserves an accepted normal form when an older initial read rejects', async () => {
    await unmount()
    const initial = deferred<AppSettingsV1>()
    const readsBefore = getSettings.mock.calls.length
    getSettings.mockImplementationOnce(() => initial.promise)
    await mount()
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1)
    await act(async () => {
      const accepted = await rendererRuntimeClient.setSettings({ performanceUsageMode: 'compact' })
      emitRendererSettingsChanged(accepted)
    })
    expect(usageButton().textContent).toContain('Compact'); expect(events).toHaveLength(1)
    const notificationsBefore = choiceNotifications
    await act(async () => initial.reject(new Error('Synthetic stale initial read failure')))
    expect(usageButton().textContent).toContain('Compact')
    expect(host.textContent).not.toContain('Synthetic stale initial read failure')
    expect(events).toHaveLength(1); expect(choiceNotifications).toBe(notificationsBefore)
    expect((await rendererRuntimeClient.getSettings()).performanceUsageMode).toBe('compact')
    expect(getSettings).toHaveBeenCalledTimes(readsBefore + 1); expect(setSettings).toHaveBeenCalledTimes(1)
  })
  it('closes and refocuses a current-mode selection without writing unchanged settings', async () => {
    await choose('Detailed'); await advanceSave()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(usageButton())
    expect(setSettings).not.toHaveBeenCalled(); expect(events).toHaveLength(0)
  })
  it('uses real menu keyboard and pointer dismissal without default focus theft', async () => {
    const trigger = usageButton()
    await act(async () => { trigger.focus(); trigger.click() })
    expect(document.activeElement).toBe(trigger)
    await act(async () => trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })))
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Compact')
    await act(async () => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true, cancelable: true })))
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Detailed')
    await act(async () => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(document.querySelector('[role="menu"]')).toBeNull(); expect(document.activeElement).toBe(trigger)
    await act(async () => trigger.click())
    await act(async () => document.body.dispatchEvent(new Event('pointerdown', { bubbles: true })))
    expect(document.querySelector('[role="menu"]')).toBeNull(); expect(setSettings).not.toHaveBeenCalled()
  })
})


describe('Settings Host read and runtime rollback ownership', () => {
  const rollback = async (code: 'settings_rollback_ready' | 'settings_rollback_failed' | 'settings_apply_failed' = 'settings_rollback_ready') => {
    expect(runtimeStatusHandler).not.toBeNull()
    await act(async () => runtimeStatusHandler!(createRuntimeStatusPublicV1({ code,
      source: 'settings-apply-rollback', at: '2026-10-14T00:00:00.000Z',
      ...(code !== 'settings_apply_failed' ? { rolledBack: true } : {}) })))
  }

  const openComputerUse = async () => {
    await unmount()
    stored.runtime.computerUse = { ...stored.runtime.computerUse, enabled: true, mode: 'always' }
    rendererRuntimeClient.invalidateSettings()
    useChatStore.setState({ settingsSection: 'computerUse' })
    await mount()
  }
  const anyAppSwitch = () => host.querySelector<HTMLButtonElement>(
    `[role="switch"][aria-label="${i18n.t('settings:computerUseAnyApp')}"]`
  )!
  const saveRetries = () => [...host.querySelectorAll<HTMLButtonElement>('button')]
    .filter(button => button.textContent?.trim() === i18n.t('settings:retrySave'))
  const saveFeedback = () => [...host.querySelectorAll<HTMLElement>('[role="status"]')]
    .map(element => element.textContent).join('\n')

  it('keeps ComputerUse save failures visible across overview and Chrome detail with one effective Retry', async () => {
    await openComputerUse()
    setSettings.mockRejectedValueOnce(new Error('Synthetic ComputerUse save failure'))
    expect(anyAppSwitch().getAttribute('aria-checked')).toBe('true')
    await act(async () => anyAppSwitch().click())
    await advanceSave()
    await act(async () => vi.advanceTimersByTimeAsync(6000))
    expect(saveFeedback()).toContain('Synthetic ComputerUse save failure')
    expect(saveRetries()).toHaveLength(1)
    const manage = [...host.querySelectorAll<HTMLButtonElement>('button')]
      .find(button => button.textContent?.trim() === i18n.t('settings:computerUseManage'))!
    await act(async () => manage.click())
    expect(saveFeedback()).toContain('Synthetic ComputerUse save failure')
    expect(saveRetries()).toHaveLength(1)
    await act(async () => saveRetries()[0]!.click())
    expect(setSettings).toHaveBeenCalledTimes(2)
    expect(stored.runtime.computerUse.enabled).toBe(false)
    expect(saveFeedback()).toContain(i18n.t('settings:applied'))
    expect(saveRetries()).toHaveLength(0)
  })

  it('keeps accepted ComputerUse rollback errors visible without replaying the rolled-back target', async () => {
    await openComputerUse()
    const acceptedComputerUse = structuredClone(stored.runtime.computerUse)
    await act(async () => anyAppSwitch().click())
    await advanceSave()
    expect(stored.runtime.computerUse.enabled).toBe(false)
    stored.runtime.computerUse = acceptedComputerUse
    await rollback()
    await act(async () => vi.advanceTimersByTimeAsync(6000))
    expect(anyAppSwitch().getAttribute('aria-checked')).toBe('true')
    expect(saveFeedback()).toContain(i18n.t('settings:settingsRuntimeRolledBack'))
    expect(saveRetries()).toHaveLength(0)
    expect(setSettings).toHaveBeenCalledTimes(1)
    await act(async () => anyAppSwitch().click())
    await advanceSave()
    expect(setSettings).toHaveBeenCalledTimes(2)
    expect(stored.runtime.computerUse.enabled).toBe(false)
    expect(saveFeedback()).toContain(i18n.t('settings:applied'))
  })

  it.each(['resolve', 'reject'] as const)('never offers the rolled-back ComputerUse target while Host readback and language apply are pending (%s)', async outcome => {
    await openComputerUse()
    const acceptedComputerUse = structuredClone(stored.runtime.computerUse)
    const locale = deferred<void>(), readback = deferred<AppSettingsV1>()
    const applyLocale = vi.fn<() => Promise<void>>(async () => undefined).mockImplementationOnce(() => locale.promise)
    const openCode = vi.fn(async () => undefined)
    await act(async () => useChatStore.setState({ applyI18nFromSettings: applyLocale, openCode }))
    await act(async () => anyAppSwitch().click())
    await advanceSave()
    expect(stored.runtime.computerUse.enabled).toBe(false)
    expect(applyLocale).toHaveBeenCalledTimes(1)
    stored.runtime.computerUse = acceptedComputerUse
    getSettings.mockImplementationOnce(() => readback.promise)
    await rollback()
    expect(saveFeedback()).toContain(i18n.t('settings:settingsRuntimeRolledBack'))
    expect(saveRetries()).toHaveLength(0)
    const back = [...host.querySelectorAll<HTMLButtonElement>('button')]
      .find(button => button.textContent?.trim() === i18n.t('settings:back'))!
    expect(back).toBeDefined()
    await act(async () => back.click())
    await act(async () => outcome === 'resolve' ? locale.resolve() : locale.reject(new Error('Synthetic late language failure')))
    expect(saveRetries()).toHaveLength(0)
    expect(setSettings).toHaveBeenCalledTimes(1)
    expect(useChatStore.getState().reloadUiSettings).not.toHaveBeenCalled()
    expect(openCode).not.toHaveBeenCalled()
    await act(async () => back.click())
    expect(setSettings).toHaveBeenCalledTimes(1)
    await act(async () => readback.resolve(structuredClone(stored)))
    expect(anyAppSwitch().getAttribute('aria-checked')).toBe('true')
    expect(saveFeedback()).toContain(i18n.t('settings:settingsRuntimeRolledBack'))
    expect(saveRetries()).toHaveLength(0)
    await act(async () => back.click())
    expect(useChatStore.getState().reloadUiSettings).toHaveBeenCalledTimes(1)
    expect(openCode).toHaveBeenCalledTimes(1)
    expect(setSettings).toHaveBeenCalledTimes(1)
    setSettings.mockRejectedValueOnce(new Error('Synthetic new draft save failure'))
    await act(async () => anyAppSwitch().click())
    await advanceSave()
    expect(saveRetries()).toHaveLength(1)
    await act(async () => saveRetries()[0]!.click())
    expect(setSettings).toHaveBeenCalledTimes(3)
    expect(stored.runtime.computerUse.enabled).toBe(false)
    expect(saveFeedback()).toContain(i18n.t('settings:applied'))
  })

  it('offers safe initial-read failure and an uncached retry without reusing the failed cache read', async () => {
    await unmount()
    getSettings.mockRejectedValueOnce(new Error('Synthetic private read detail'))
    const direct = vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost')
    await mount()
    expect(host.querySelector('[role="alert"]')?.textContent).toBe(i18n.t('settings:settingsReadFailed'))
    expect(host.textContent).not.toContain('Synthetic private read detail')
    const retry = [...host.querySelectorAll<HTMLButtonElement>('button')]
      .find(button => button.textContent?.trim() === i18n.t('settings:settingsReadRetry'))!
    await act(async () => retry.click())
    expect(direct).toHaveBeenCalledTimes(1)
    expect(usageButton().textContent).toContain('Detailed')
  })

  it.each(['resolve', 'reject'] as const)('retains rollback state when an old language apply later %ss', async outcome => {
    const locale = deferred<void>()
    const applyLocale = vi.fn(() => locale.promise)
    await act(async () => useChatStore.setState({ applyI18nFromSettings: applyLocale }))
    const oldPort = stored.runtime.port
    await act(async () => sectionProbe.current!.update({ runtime: { port: oldPort + 1 }, codePromptPrefix: 'Retained unrelated field' }))
    await advanceSave()
    expect(stored.runtime.port).toBe(oldPort + 1)
    expect(applyLocale).toHaveBeenCalledTimes(1)
    stored = { ...stored, runtime: { ...stored.runtime, port: oldPort } }
    await rollback()
    expect(sectionProbe.current!.form.runtime.port).toBe(oldPort)
    expect(sectionProbe.current!.form.codePromptPrefix).toBe('Retained unrelated field')
    expect(host.textContent).toContain(i18n.t('settings:settingsRuntimeRolledBack'))
    expect(host.querySelector('[role="status"]')?.textContent).toContain(i18n.t('settings:settingsRuntimeRolledBack'))
    expect(host.querySelector('[role="status"]')?.getAttribute('aria-live')).toBe('polite')
    const reads = getSettings.mock.calls.length
    await act(async () => outcome === 'resolve' ? locale.resolve() : locale.reject(new Error('Synthetic late language failure')))
    expect(host.textContent).toContain(i18n.t('settings:settingsRuntimeRolledBack'))
    expect(host.textContent).not.toContain('Synthetic late language failure')
    expect(getSettings).toHaveBeenCalledTimes(reads)
    expect(useChatStore.getState().reloadUiSettings).not.toHaveBeenCalled()
    expect(events).toHaveLength(1)
  })

  it('retains a newer local draft when a rollback Host read arrives late', async () => {
    const oldPort = stored.runtime.port
    await act(async () => sectionProbe.current!.update({ runtime: { port: oldPort + 1 } }))
    await advanceSave()
    const restored = { ...stored, runtime: { ...stored.runtime, port: oldPort } }
    const read = deferred<AppSettingsV1>()
    getSettings.mockImplementationOnce(() => read.promise)
    await rollback()
    await act(async () => sectionProbe.current!.update({ runtime: { port: oldPort + 2 }, codePromptPrefix: 'New local draft' }))
    await act(async () => read.resolve(restored))
    expect(sectionProbe.current!.form.runtime.port).toBe(oldPort + 2)
    expect(sectionProbe.current!.form.codePromptPrefix).toBe('New local draft')
    expect(setSettings).toHaveBeenCalledTimes(1)
  })

  it('distinguishes restored-but-offline from unsuccessful durable restoration', async () => {
    await rollback('settings_rollback_failed')
    expect(host.textContent).toContain(i18n.t('settings:settingsRuntimeRollbackFailed'))
    const direct = vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost')
    await rollback('settings_apply_failed')
    expect(host.textContent).toContain(i18n.t('settings:settingsRuntimeRollbackFailed'))
    expect(direct).not.toHaveBeenCalled()
  })

  it('does not infer deletion from a Host profile addition rejected by a dirty form', async () => {
    await act(async () => sectionProbe.current!.update({ codePromptPrefix: 'Ordinary pending draft' }))
    const profile = { prompt: '', model: '', effort: '' as const, toolPolicy: 'readOnly' as const, tools: [] }
    stored.runtime.subagents.profiles = { host_added: profile }
    await act(async () => window.dispatchEvent(new CustomEvent(SETTINGS_CHANGED_EVENT, { detail: structuredClone(stored) })))
    expect(sectionProbe.current!.form.runtime.subagents.profiles.host_added).toBeUndefined()
    await advanceSave()
    expect(setSettings.mock.calls[0]![0].runtime?.subagents?.profiles?.host_added).toBeUndefined()
    expect(stored.runtime.subagents.profiles.host_added).toEqual(profile)
  })

  it('carries only explicit profile deletion through a full form save and a failed-save retry', async () => {
    const profile = { prompt: '', model: '', effort: '' as const, toolPolicy: 'readOnly' as const, tools: [] }
    stored.runtime.subagents.profiles = { removed: profile }
    await act(async () => window.dispatchEvent(new CustomEvent(SETTINGS_CHANGED_EVENT, { detail: structuredClone(stored) })))
    setSettings.mockRejectedValueOnce(new Error('Synthetic first save failure'))
    await act(async () => sectionProbe.current!.update({ runtime: { subagents: { profiles: { removed: null } } } }))
    await advanceSave()
    expect(setSettings.mock.calls[0]![0].runtime?.subagents?.profiles?.removed).toBeNull()
    await act(async () => sectionProbe.current!.update({ codePromptPrefix: 'Retry the current draft' }))
    await advanceSave()
    expect(setSettings.mock.calls[1]![0].runtime?.subagents?.profiles?.removed).toBeNull()
    expect(stored.runtime.subagents.profiles.removed).toBeUndefined()
  })
})
