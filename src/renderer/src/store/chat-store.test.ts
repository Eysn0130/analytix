import { describe, expect, it } from 'vitest'
import { DEFAULT_ANALYTIX_MODEL } from '@shared/app-settings'
import { useChatStore } from './chat-store'

describe('chat store defaults', () => {
  it('starts the composer on the Analytix default model', () => {
    expect(useChatStore.getState().composerModel).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(useChatStore.getState().composerModel).toBe('deepseek-v4-flash')
  })
})
