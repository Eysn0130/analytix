import { rendererRuntimeClient } from '../agent/runtime-client'
import type { WriteWorkspaceGet, WriteWorkspaceSet, WriteWorkspaceState } from './write-workspace-store-types'
import {
  compactWorkspaceRoots,
  normalizePath,
  normalizeWriteSettings,
  withResolvedInlineCompletionSettings
} from './write-workspace-store-helpers'

type WriteSettingsActions = Pick<
  WriteWorkspaceState,
  'loadWriteSettings' | 'selectWriteWorkspace' | 'addWriteWorkspace' | 'removeWriteWorkspace'
>

type WriteSettingsActionContext = {
  set: WriteWorkspaceSet
  get: WriteWorkspaceGet
}

export function applyWriteSettingsState(
  set: WriteWorkspaceSet,
  settings: Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>
): ReturnType<typeof withResolvedInlineCompletionSettings> {
  const write = withResolvedInlineCompletionSettings(normalizeWriteSettings(settings.write), settings)
  set({
    defaultWorkspaceRoot: write.defaultWorkspaceRoot,
    workspaceRoots: write.workspaces,
    inlineCompletion: write.inlineCompletion,
    selectionAssist: write.selectionAssist,
    agentPresets: write.agentPresets,
    // Key-free settings metadata is not evidence that a Registry credential
    // is committed. K7 will consult the public credentialConfigured projection.
    inlineCompletionApiReady: false,
    imageGenReady: false,
    prototypeReady: false,
    settingsError: null
  })
  return write
}

export function createWriteSettingsActions({ set, get }: WriteSettingsActionContext): WriteSettingsActions {
  return {
    loadWriteSettings: async () => {
      if (get().settingsLoading) return
      set({ settingsLoading: true, settingsError: null })
      try {
        const settings = await rendererRuntimeClient.getSettings({ forceRefresh: true })
        const write = applyWriteSettingsState(set, settings)
        set({ settingsLoading: false })
        await get().initializeWorkspace(write.activeWorkspaceRoot)
      } catch (error) {
        set({
          settingsLoading: false,
          settingsError: error instanceof Error ? error.message : String(error)
        })
      }
    },

    selectWriteWorkspace: async (workspaceRoot) => {
      const normalized = normalizePath(workspaceRoot)
      if (!normalized) return
      const roots = compactWorkspaceRoots([normalized, ...get().workspaceRoots])
      set({ workspaceRoots: roots })
      try {
        const settings = await rendererRuntimeClient.setSettings({
          write: {
            activeWorkspaceRoot: normalized,
            workspaces: roots
          }
        })
        const write = applyWriteSettingsState(set, settings)
        await get().initializeWorkspace(write.activeWorkspaceRoot)
      } catch (error) {
        set({ settingsError: error instanceof Error ? error.message : String(error) })
      }
    },

    addWriteWorkspace: async (workspaceRoot) => {
      const normalized = normalizePath(workspaceRoot)
      if (!normalized) return
      const roots = compactWorkspaceRoots([normalized, ...get().workspaceRoots])
      try {
        const settings = await rendererRuntimeClient.setSettings({
          write: {
            activeWorkspaceRoot: normalized,
            workspaces: roots
          }
        })
        const write = applyWriteSettingsState(set, settings)
        await get().initializeWorkspace(write.activeWorkspaceRoot)
      } catch (error) {
        set({ settingsError: error instanceof Error ? error.message : String(error) })
      }
    },

    removeWriteWorkspace: async (workspaceRoot) => {
      const normalized = normalizePath(workspaceRoot)
      if (!normalized) return
      const state = get()
      const fallback = state.defaultWorkspaceRoot ||
        state.workspaceRoots.find((item) => item !== normalized) ||
        state.workspaceRoot
      const roots = compactWorkspaceRoots([
        fallback,
        ...state.workspaceRoots.filter((item) => normalizePath(item) !== normalized)
      ])
      const activeWorkspaceRoot = normalizePath(state.workspaceRoot) === normalized
        ? fallback
        : state.workspaceRoot
      try {
        const settings = await rendererRuntimeClient.setSettings({
          write: {
            activeWorkspaceRoot,
            workspaces: roots
          }
        })
        const write = applyWriteSettingsState(set, settings)
        if (normalizePath(get().workspaceRoot) === normalized) {
          await get().initializeWorkspace(write.activeWorkspaceRoot)
        }
      } catch (error) {
        set({ settingsError: error instanceof Error ? error.message : String(error) })
      }
    }
  }
}
