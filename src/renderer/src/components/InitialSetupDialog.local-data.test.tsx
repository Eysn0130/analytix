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
})
