import { readBrowserStorageItem, writeBrowserStorageItem } from './browser-storage'

/** Theme preference: 'default' or a UI plugin id. */
export const UI_MODE_STORAGE_KEY = 'analytix.uiMode'

export const UI_MODE_DEFAULT = 'default'
export const UI_MODE_MASCOT = 'mascot'

const UI_MODE_PATTERN = /^[a-z0-9][a-z0-9-]{1,39}$/

export function readUiModePreference(): string {
  const stored = readBrowserStorageItem(UI_MODE_STORAGE_KEY)?.trim().toLowerCase()
  if (stored === UI_MODE_MASCOT) {
    return UI_MODE_DEFAULT
  }
  if (stored && (stored === UI_MODE_DEFAULT || UI_MODE_PATTERN.test(stored))) {
    return stored
  }
  return UI_MODE_DEFAULT
}

export function writeUiModePreference(mode: string): void {
  writeBrowserStorageItem(UI_MODE_STORAGE_KEY, mode)
}
