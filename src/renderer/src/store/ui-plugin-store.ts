import { create } from 'zustand'
import {
  buildUiPluginTokenCss,
  type UiPluginLabelKey,
  type UiPluginListItem,
  type UiPluginManifestV1,
  type UiPluginRuntimeFigures
} from '@shared/ui-plugin'
import {
  UI_MODE_DEFAULT,
  UI_MODE_MASCOT,
  readUiModePreference,
  writeUiModePreference
} from '../lib/ui-mode'

/**
 * Declarative UI plugin authority: saved mode, validated manifest and theme token injection.
 */

export type UiPluginRuntime = {
  manifest: UiPluginManifestV1
  figures: UiPluginRuntimeFigures
}

type UiPluginState = {
  uiMode: string
  installed: UiPluginListItem[]
  activeRuntime: UiPluginRuntime | null
  busy: boolean
  initialized: boolean
  lastError: string | null
  initUiPlugins: () => Promise<void>
  refreshUiPlugins: () => Promise<void>
  activateUiMode: (mode: string) => Promise<void>
  installUiPluginFromDialog: () => Promise<{ ok: boolean; errors?: string[]; canceled?: boolean }>
  removeUiPluginById: (id: string) => Promise<void>
}

const TOKEN_STYLE_ELEMENT_ID = 'ax-ui-plugin-tokens'

function uiPluginApi(): Window['analytix']['app'] | null {
  if (typeof window === 'undefined') return null
  return window.analytix?.app ?? null
}

function applyUiModeDom(mode: string, runtime: UiPluginRuntime | null): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  root.removeAttribute('data-mascot-mode')
  if (runtime && mode === runtime.manifest.id) {
    root.setAttribute('data-ui-plugin', runtime.manifest.id)
  } else {
    root.removeAttribute('data-ui-plugin')
  }

  const css = runtime && mode === runtime.manifest.id ? buildUiPluginTokenCss(runtime.manifest) : ''
  let styleElement = document.getElementById(TOKEN_STYLE_ELEMENT_ID)
  if (!css) {
    styleElement?.remove()
    return
  }
  if (!styleElement) {
    styleElement = document.createElement('style')
    styleElement.id = TOKEN_STYLE_ELEMENT_ID
    document.head.appendChild(styleElement)
  }
  styleElement.textContent = css
}

export const useUiPluginStore = create<UiPluginState>((set, get) => ({
  uiMode: UI_MODE_DEFAULT,
  installed: [],
  activeRuntime: null,
  busy: false,
  initialized: false,
  lastError: null,

  initUiPlugins: async () => {
    if (get().initialized) return
    set({ initialized: true })
    const mode = readUiModePreference()
    if (mode === UI_MODE_DEFAULT) {
      set({ uiMode: mode })
      applyUiModeDom(mode, null)
      void get().refreshUiPlugins()
      return
    }
    // Load the saved plugin through the existing desktop authority; failure resets to default.
    applyUiModeDom(mode === UI_MODE_MASCOT ? UI_MODE_MASCOT : UI_MODE_DEFAULT, null)
    await get().activateUiMode(mode)
    void get().refreshUiPlugins()
  },

  refreshUiPlugins: async () => {
    const api = uiPluginApi()
    if (typeof api?.listUiPlugins !== 'function') return
    try {
      const result = await api.listUiPlugins()
      set({ installed: result.plugins })
    } catch (error) {
      set({ lastError: error instanceof Error ? error.message : String(error) })
    }
  },

  activateUiMode: async (mode: string) => {
    const normalized = mode.trim().toLowerCase()
    if (normalized === UI_MODE_DEFAULT) {
      writeUiModePreference(normalized)
      set({ uiMode: normalized, activeRuntime: null, lastError: null })
      applyUiModeDom(normalized, null)
      return
    }

    // Plugin manifests and tokens continue to come from the existing desktop authority.
    const api = uiPluginApi()
    if (typeof api?.loadUiPlugin !== 'function') {
      // Preserve the legacy mode compatibility without rendering decorative figures.
      if (normalized === UI_MODE_MASCOT) {
        writeUiModePreference(normalized)
        set({ uiMode: normalized, activeRuntime: null, lastError: null })
        applyUiModeDom(normalized, null)
      }
      return
    }
    set({ busy: true })
    try {
      const result = await api.loadUiPlugin(normalized)
      if (!result.ok) {
        set({
          busy: false,
          uiMode: UI_MODE_DEFAULT,
          activeRuntime: null,
          lastError: result.error
        })
        writeUiModePreference(UI_MODE_DEFAULT)
        applyUiModeDom(UI_MODE_DEFAULT, null)
        return
      }
      const runtime: UiPluginRuntime = { manifest: result.manifest, figures: result.figures }
      writeUiModePreference(normalized)
      set({ busy: false, uiMode: normalized, activeRuntime: runtime, lastError: null })
      applyUiModeDom(normalized, runtime)
    } catch (error) {
      set({
        busy: false,
        uiMode: UI_MODE_DEFAULT,
        activeRuntime: null,
        lastError: error instanceof Error ? error.message : String(error)
      })
      writeUiModePreference(UI_MODE_DEFAULT)
      applyUiModeDom(UI_MODE_DEFAULT, null)
    }
  },

  installUiPluginFromDialog: async () => {
    const api = uiPluginApi()
    if (typeof api?.installUiPlugin !== 'function') {
      return { ok: false, errors: ['桌面接口不可用'] }
    }
    set({ busy: true })
    try {
      const result = await api.installUiPlugin()
      set({ busy: false })
      if (result.canceled) return { ok: false, canceled: true }
      if (!result.ok) return { ok: false, errors: result.errors }
      await get().refreshUiPlugins()
      return { ok: true }
    } catch (error) {
      set({ busy: false })
      return { ok: false, errors: [error instanceof Error ? error.message : String(error)] }
    }
  },

  removeUiPluginById: async (id: string) => {
    const api = uiPluginApi()
    if (typeof api?.removeUiPlugin !== 'function') return
    if (get().uiMode === id) {
      await get().activateUiMode(UI_MODE_DEFAULT)
    }
    set({ busy: true, lastError: null })
    try {
      const result = await api.removeUiPlugin(id)
      if (!result.ok) set({ lastError: 'UI theme removal failed' })
    } catch (error) {
      set({ lastError: error instanceof Error ? error.message : String(error) })
    } finally {
      await get().refreshUiPlugins()
      set({ busy: false })
    }
  }
}))

/** 激活插件提供的进行中文案(按当前语言);未提供时返回 null */
export function useUiPluginWorkLabel(labelKey: UiPluginLabelKey, language: string): string | null {
  return useUiPluginStore((state) => {
    const labels = state.activeRuntime?.manifest.labels
    if (!labels) return null
    const locale = language.toLowerCase().startsWith('zh') ? 'zh' : 'en'
    return labels[locale]?.[labelKey] ?? null
  })
}
