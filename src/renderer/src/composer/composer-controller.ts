import type { ComposerReasoningEffort } from '../components/chat/FloatingComposerModelPicker'

export type ComposerViewState = {
  input: string
  mode: 'plan' | 'agent'
  busy: boolean
  runtimeReady: boolean
  hasActiveThread: boolean
  composerModel: string
  composerProviderId?: string
  composerReasoningEffort?: ComposerReasoningEffort
  contextWindowTokens?: number
  runtimeToolCount?: number
  runtimeSkillCount?: number
}

export type ComposerController = {
  setInput(value: string): void
  setMode(value: 'plan' | 'agent'): void
  send(): void
  interrupt(options?: { discard?: boolean }): void
  setModel(modelId: string, providerId?: string): void
  setReasoningEffort?(effort: ComposerReasoningEffort): void
}

export function createComposerViewState(input: ComposerViewState): ComposerViewState {
  return input
}

export function createComposerController(input: ComposerController): ComposerController {
  return input
}
