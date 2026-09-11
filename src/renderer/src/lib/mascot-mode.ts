import { readBrowserStorageItem, writeBrowserStorageItem } from './browser-storage'

export const MASCOT_MODE_STORAGE_KEY = 'analytix.mascotMode'

export function readMascotModePreference(): boolean {
  const value = readBrowserStorageItem(MASCOT_MODE_STORAGE_KEY)?.trim().toLowerCase()
  return value === '1' || value === 'true' || value === 'on'
}

export function writeMascotModePreference(enabled: boolean): void {
  writeBrowserStorageItem(MASCOT_MODE_STORAGE_KEY, enabled ? '1' : '0')
}
