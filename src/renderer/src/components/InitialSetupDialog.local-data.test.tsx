// @vitest-environment jsdom

import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import { InitialSetupDialog } from './InitialSetupDialog'

const initialChatState = useChatStore.getState()
let root: Root
let container: HTMLDivElement

describe('InitialSetupDialog local data path', () => {
  beforeEach(async () => {
    const actEnvironment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    await i18n.changeLanguage('en')
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(
      normalizeAppSettings({ workspaceRoot: '/workspace/analytix' } as AppSettingsV1)
    )
    vi.spyOn(rendererRuntimeClient, 'setSettings')
    useChatStore.setState({
      ...initialChatState,
      initialSetupOpen: true,
      initialSetupMode: 'required',
      runtimeConnection: 'idle',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime: vi.fn(async () => undefined)
    })
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    useChatStore.setState(initialChatState)
    vi.restoreAllMocks()
    const actEnvironment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('dismisses required Provider setup explicitly while keeping model execution gated', async () => {
    await act(async () => {
      root.render(createElement(InitialSetupDialog))
      await Promise.resolve()
    })
    const entry = Array.from(container.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open local data tools'
    )
    expect(entry).toBeDefined()
    expect(entry?.disabled).toBe(false)
    await act(async () => entry?.click())
    const state = useChatStore.getState()
    expect(state.initialSetupOpen).toBe(false)
    expect(state.runtimeConnection).toBe('idle')
    expect(state.reloadUiSettings).toHaveBeenCalledTimes(1)
    expect(state.probeRuntime).not.toHaveBeenCalled()
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
  })
  it('names protected fields and keeps required Escape gated without saving', async () => {
    await act(async () => root.render(createElement(InitialSetupDialog)))
    const modal = container.querySelector<HTMLElement>('[role="dialog"]')!
    expect(modal.contains(document.activeElement)).toBe(true)
    const key = container.querySelector<HTMLInputElement>('input[type="password"]')!
    expect(container.querySelector(`label[for="${key.id}"]`)?.textContent).toContain('API Key')
    const show = container.querySelector<HTMLButtonElement>('button[aria-pressed="false"][aria-label="Show value"]')!
    expect(show).not.toBeNull()
    await act(async () => show.click())
    expect(key.type).toBe('text'); expect(show.getAttribute('aria-label')).toBe('Hide value')
    const base = container.querySelector<HTMLInputElement>('input[id$="-base-url"]')!
    expect(container.querySelector(`label[for="${base.id}"]`)?.textContent).toBeTruthy()
    await act(async () => modal.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(useChatStore.getState().initialSetupOpen).toBe(true)
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
    expect(useChatStore.getState().probeRuntime).not.toHaveBeenCalled()
  })
  it('restores the preview opener with Escape and handles loading to ready without losing the opener', async () => {
    let resolve!: (settings: AppSettingsV1) => void
    vi.mocked(rendererRuntimeClient.getSettings).mockImplementationOnce(() => new Promise(done => { resolve = done }))
    useChatStore.setState({ initialSetupMode: 'preview' })
    const opener = document.createElement('button'); document.body.append(opener); opener.focus()
    try {
      await act(async () => root.render(createElement(InitialSetupDialog)))
      expect(container.querySelector('[role="dialog"]')).toBe(document.activeElement)
      await act(async () => resolve(normalizeAppSettings({ workspaceRoot: '/synthetic/a' } as AppSettingsV1)))
      expect(container.querySelector('[role="dialog"]')?.contains(document.activeElement)).toBe(true)
      await act(async () => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
      expect(useChatStore.getState().initialSetupOpen).toBe(false)
      await act(async () => root.render(null)); expect(document.activeElement).toBe(opener)
      expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
    } finally { opener.remove() }
  })

})
