import { describe, expect, it, vi } from 'vitest'
import { createComposerController, createComposerViewState } from './composer-controller'

describe('composer controller contract', () => {
  it('keeps view state separate from imperative composer actions', () => {
    const viewState = createComposerViewState({
      input: 'hello',
      mode: 'agent',
      busy: false,
      runtimeReady: true,
      hasActiveThread: true,
      composerModel: 'm1',
      composerProviderId: 'p1',
      composerReasoningEffort: 'max'
    })
    const send = vi.fn()
    const controller = createComposerController({
      setInput: vi.fn(),
      setMode: vi.fn(),
      send,
      interrupt: vi.fn(),
      setModel: vi.fn(),
      setReasoningEffort: vi.fn()
    })

    controller.send()
    expect(send).toHaveBeenCalledOnce()
    expect(viewState).toMatchObject({
      input: 'hello',
      mode: 'agent',
      composerModel: 'm1',
      composerProviderId: 'p1'
    })
  })
})
