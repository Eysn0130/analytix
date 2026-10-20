// @vitest-environment jsdom
import { act, createElement, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { presentationPolicyFor } from '../components/chat/presentation/chat/chat-view-model'
import { useDisclosure } from '../components/chat/presentation/chat/use-disclosure'

let preferences: typeof import('./chat-presentation-preferences')
let storageListeners: Set<EventListenerOrEventListenerObject>
function selectEnvironment(environment: 'desktop' | 'browser-preview' | 'browser') {
  vi.stubGlobal('analytix', environment === 'browser' ? undefined : { app: { platform: 'darwin' } })
  if (environment === 'browser-preview') document.documentElement.dataset.bridge = 'browser-preview'
  else delete document.documentElement.dataset.bridge
}
beforeEach(async () => {
  storageListeners = new Set()
  const addEventListener = window.addEventListener.bind(window)
  vi.spyOn(window, 'addEventListener').mockImplementation((type, listener, options) => {
    if (type === 'storage') storageListeners.add(listener)
    addEventListener(type, listener, options)
  })
  vi.resetModules()
  preferences = await import('./chat-presentation-preferences')
})
afterEach(() => {
  for (const listener of storageListeners) window.removeEventListener('storage', listener)
  localStorage.removeItem(preferences.WORK_DETAILS_STORAGE_KEY)
  localStorage.removeItem(preferences.TRAJECTORY_DURATION_STORAGE_KEY)
  delete document.documentElement.dataset.bridge
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it.each(['desktop', 'browser-preview', 'browser'] as const)('uses the %s default for missing, null and invalid preferences without writing', environment => {
  selectEnvironment(environment)
  const fallback = environment === 'desktop' ? 'standard' : 'detailed'
  const { WORK_DETAILS_STORAGE_KEY, normalizeWorkDetailsMode, readWorkDetailsPreference } = preferences
  expect(readWorkDetailsPreference()).toBe(fallback)
  expect(localStorage.getItem(WORK_DETAILS_STORAGE_KEY)).toBeNull()
  for (const value of [undefined, null, '', 'FULL', 'verbose ', '{"body":"private"}', 1]) {
    expect(normalizeWorkDetailsMode(value)).toBe(fallback)
  }
  localStorage.setItem(WORK_DETAILS_STORAGE_KEY, 'broken')
  expect(readWorkDetailsPreference()).toBe(fallback)
  expect(localStorage.getItem(WORK_DETAILS_STORAGE_KEY)).toBe('broken')
})
it.each(['desktop', 'browser-preview'] as const)('preserves saved modes and reads legacy modes as detailed without migrating them in %s', environment => {
  selectEnvironment(environment)
  const { WORK_DETAILS_STORAGE_KEY, WORK_DETAILS_MODES, readWorkDetailsPreference, writeWorkDetailsPreference } = preferences
  for (const mode of WORK_DETAILS_MODES) {
    localStorage.setItem(WORK_DETAILS_STORAGE_KEY, mode)
    expect(readWorkDetailsPreference()).toBe(mode)
    expect(localStorage.getItem(WORK_DETAILS_STORAGE_KEY)).toBe(mode)
    writeWorkDetailsPreference(mode)
    expect(readWorkDetailsPreference()).toBe(mode)
    expect(localStorage.getItem(WORK_DETAILS_STORAGE_KEY)).toBe(mode)
  }
  for (const legacy of ['normal', 'expanded']) {
    localStorage.setItem(WORK_DETAILS_STORAGE_KEY, legacy)
    expect(readWorkDetailsPreference()).toBe('detailed')
    expect(localStorage.getItem(WORK_DETAILS_STORAGE_KEY)).toBe(legacy)
  }
})
it('retains the four official presentation policies', () => {
  expect(preferences.WORK_DETAILS_MODES.map(mode => presentationPolicyFor(mode))).toEqual([
    { mode: 'compact', foldCompletedTurns: true, stepGrouping: 'collapsed', liveProcessDetail: false, settledReasoningPreview: false },
    { mode: 'standard', foldCompletedTurns: true, stepGrouping: 'collapsed', liveProcessDetail: true, settledReasoningPreview: true },
    { mode: 'detailed', foldCompletedTurns: true, stepGrouping: 'history', liveProcessDetail: true, settledReasoningPreview: true },
    { mode: 'verbose', foldCompletedTurns: false, stepGrouping: 'none', liveProcessDetail: false, settledReasoningPreview: true }
  ])
})
it.each(['desktop', 'browser-preview'] as const)('keeps the chosen mode and manual fold when late settings rerender the same %s view', async environment => {
  selectEnvironment(environment)
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const { WORK_DETAILS_STORAGE_KEY, useWorkDetailsPreference, writeWorkDetailsPreference } = preferences
  const write = vi.spyOn(Storage.prototype, 'setItem')
  let hydrate!: () => void
  const settings = new Promise<void>(resolve => { hydrate = resolve })
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  function Probe() {
    const mode = useWorkDetailsPreference()
    const disclosure = useDisclosure('thread:turn:settled', true)
    const [ready, setReady] = useState(false)
    useEffect(() => { void settings.then(() => setReady(true)) }, [])
    return createElement('button', { onClick: disclosure.toggle, 'data-mode': mode, 'data-ready': ready }, disclosure.open ? 'open' : 'collapsed')
  }
  try {
    await act(async () => root.render(createElement(Probe)))
    const button = host.querySelector('button')!
    expect(button.dataset.mode).toBe(environment === 'desktop' ? 'standard' : 'detailed')
    expect(localStorage.getItem(WORK_DETAILS_STORAGE_KEY)).toBeNull()
    expect(write).not.toHaveBeenCalled()
    await act(async () => { writeWorkDetailsPreference('compact'); button.click() })
    expect(button.textContent).toBe('collapsed')
    await act(async () => hydrate())
    expect(button.dataset.ready).toBe('true')
    expect(button.dataset.mode).toBe('compact')
    expect(button.textContent).toBe('collapsed')
    expect(write).toHaveBeenCalledExactlyOnceWith(WORK_DETAILS_STORAGE_KEY, 'compact')
  } finally { await act(async () => root.unmount()); host.remove() }
})
it('synchronizes three mounted consumers in one window and handles external storage/reset', async () => {
  selectEnvironment('desktop')
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const { WORK_DETAILS_STORAGE_KEY, useWorkDetailsPreference, writeWorkDetailsPreference } = preferences
  writeWorkDetailsPreference('detailed')
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  function Probe({ surface }: { surface: string }) { return createElement('span', { 'data-surface': surface }, useWorkDetailsPreference()) }
  try {
    await act(async () => root.render(createElement('div', {}, ['main', 'side', 'inspector'].map(surface => createElement(Probe, { key: surface, surface })))))
    await act(async () => writeWorkDetailsPreference('compact'))
    expect([...host.querySelectorAll('span')].map(node => node.textContent)).toEqual(['compact', 'compact', 'compact'])
    localStorage.setItem(WORK_DETAILS_STORAGE_KEY, 'standard')
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: WORK_DETAILS_STORAGE_KEY })))
    expect(host.textContent).toBe('standardstandardstandard')
    localStorage.removeItem(WORK_DETAILS_STORAGE_KEY)
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: null })))
    expect(host.textContent).toBe('standardstandardstandard')
  } finally { await act(async () => root.unmount()); host.remove() }
})

it('defaults missing and invalid trajectory duration values without correcting storage', () => {
  const { TRAJECTORY_DURATION_STORAGE_KEY, readTrajectoryDurationPreference } = preferences
  const write = vi.spyOn(Storage.prototype, 'setItem')
  expect(readTrajectoryDurationPreference()).toBe(false)
  expect(write).not.toHaveBeenCalled()
  for (const value of ['', 'false', 'TRUE', '1', 'null', '{"duration":true}']) {
    localStorage.setItem(TRAJECTORY_DURATION_STORAGE_KEY, value)
    write.mockClear()
    expect(readTrajectoryDurationPreference()).toBe(false)
    expect(localStorage.getItem(TRAJECTORY_DURATION_STORAGE_KEY)).toBe(value)
    expect(write).not.toHaveBeenCalled()
  }
})

it('restores the trajectory duration choice after reloading the preference owner', async () => {
  const { TRAJECTORY_DURATION_STORAGE_KEY, writeTrajectoryDurationPreference } = preferences
  writeTrajectoryDurationPreference(true)
  expect(localStorage.getItem(TRAJECTORY_DURATION_STORAGE_KEY)).toBe('true')
  vi.resetModules()
  const restored = await import('./chat-presentation-preferences')
  expect(restored.readTrajectoryDurationPreference()).toBe(true)
  restored.writeTrajectoryDurationPreference(false)
  expect(localStorage.getItem(TRAJECTORY_DURATION_STORAGE_KEY)).toBe('false')
  expect(restored.readTrajectoryDurationPreference()).toBe(false)
})

it('shares trajectory duration and observes storage changes while all consumers are unmounted', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const { TRAJECTORY_DURATION_STORAGE_KEY, useTrajectoryDurationPreference, writeTrajectoryDurationPreference } = preferences
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  function Probe({ surface }: { surface: string }) {
    return createElement('span', { 'data-surface': surface }, String(useTrajectoryDurationPreference()))
  }
  const draw = async () => act(async () => root.render(createElement('div', {}, ['first', 'second'].map(surface => createElement(Probe, { key: surface, surface })))))
  const values = () => [...host.querySelectorAll('span')].map(node => node.textContent)
  try {
    await draw()
    expect(values()).toEqual(['false', 'false'])
    await act(async () => writeTrajectoryDurationPreference(true))
    expect(values()).toEqual(['true', 'true'])
    localStorage.setItem(TRAJECTORY_DURATION_STORAGE_KEY, 'false')
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: 'unrelated.preference' })))
    expect(values()).toEqual(['true', 'true'])
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: TRAJECTORY_DURATION_STORAGE_KEY })))
    expect(values()).toEqual(['false', 'false'])
    await act(async () => root.render(null))
    localStorage.setItem(TRAJECTORY_DURATION_STORAGE_KEY, 'true')
    window.dispatchEvent(new StorageEvent('storage', { key: TRAJECTORY_DURATION_STORAGE_KEY }))
    await draw()
    expect(values()).toEqual(['true', 'true'])
    localStorage.removeItem(TRAJECTORY_DURATION_STORAGE_KEY)
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: null })))
    expect(values()).toEqual(['false', 'false'])
  } finally { await act(async () => root.unmount()); host.remove() }
})

it.each(['read-failure', 'unavailable-storage'] as const)('keeps trajectory duration usable in memory with %s', async failure => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const { TRAJECTORY_DURATION_STORAGE_KEY, useTrajectoryDurationPreference, writeTrajectoryDurationPreference } = preferences
  writeTrajectoryDurationPreference(true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  function Probe() { return createElement('span', {}, String(useTrajectoryDurationPreference())) }
  if (failure === 'read-failure') {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('storage read unavailable') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage write unavailable') })
  } else {
    vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new Error('storage unavailable') })
  }
  try {
    await act(async () => root.render(createElement(Probe)))
    expect(host.textContent).toBe('true')
    await act(async () => window.dispatchEvent(new StorageEvent('storage', { key: TRAJECTORY_DURATION_STORAGE_KEY })))
    expect(host.textContent).toBe('true')
    await act(async () => writeTrajectoryDurationPreference(false))
    expect(host.textContent).toBe('false')
    await act(async () => root.render(null))
    await act(async () => root.render(createElement(Probe)))
    expect(host.textContent).toBe('false')
  } finally {
    await act(async () => root.unmount()); host.remove()
    vi.restoreAllMocks()
  }
})
