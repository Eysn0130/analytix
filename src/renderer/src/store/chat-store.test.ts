import { afterEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_ANALYTIX_MODEL } from '@shared/app-settings'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetModules()
})

async function coldStore(saved: Record<string, string> = {}) {
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => saved[key] ?? null,
    setItem: (key: string, value: string) => { saved[key] = value }
  })
  vi.resetModules()
  return (await import('./chat-store')).useChatStore.getState()
}

describe('chat store defaults', () => {
  it('keeps the default unbound until a Provider is configured', async () => {
    const state = await coldStore()
    expect(state.composerModel).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(state.composerProviderId).toBe('')
    expect(state.composerModelGroups).toEqual([])
  })

  it('restores the saved selection before the Runtime catalog arrives', async () => {
    const state = await coldStore({
      'analytix.composerModel': 'deepseek-v4-pro',
      'analytix.composerProviderId': 'chosen-provider'
    })
    expect(state.composerModel).toBe('deepseek-v4-pro')
    expect(state.composerProviderId).toBe('chosen-provider')
    expect(state.composerModelGroups).toEqual([])
    expect(state.runtimeConnection).toBe('idle')
  })
})
