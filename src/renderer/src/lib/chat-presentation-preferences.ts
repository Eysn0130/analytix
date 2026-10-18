import { useSyncExternalStore } from 'react'
import { browserStorage, readBrowserStorageItem, removeBrowserStorageItem, writeBrowserStorageItem } from './browser-storage'

export const WORK_DETAILS_STORAGE_KEY = 'analytix.workDetails'
export const WORK_DETAILS_MODES = ['compact', 'standard', 'detailed', 'verbose'] as const
export type WorkDetailsMode = typeof WORK_DETAILS_MODES[number]
function defaultWorkDetailsMode(): WorkDetailsMode {
  // The browser preview also exposes window.analytix, including on macOS.
  const desktop = typeof window !== 'undefined' && Boolean(window.analytix) &&
    typeof document !== 'undefined' && document.documentElement.dataset.bridge !== 'browser-preview'
  return desktop ? 'standard' : 'detailed'
}
export function normalizeWorkDetailsMode(value: unknown): WorkDetailsMode {
  if (value === 'normal' || value === 'expanded') return 'detailed'
  return WORK_DETAILS_MODES.find(mode => mode === value) ?? defaultWorkDetailsMode()
}
export function readWorkDetailsPreference(): WorkDetailsMode {
  return normalizeWorkDetailsMode(readBrowserStorageItem(WORK_DETAILS_STORAGE_KEY))
}
let current: WorkDetailsMode | undefined
const listeners = new Set<() => void>()
export function writeWorkDetailsPreference(mode: WorkDetailsMode): void {
  current = normalizeWorkDetailsMode(mode)
  writeBrowserStorageItem(WORK_DETAILS_STORAGE_KEY, current)
  for (const listener of listeners) listener()
}
function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  const changed = (event: StorageEvent) => {
    if (event.key !== null && event.key !== WORK_DETAILS_STORAGE_KEY) return
    current = readWorkDetailsPreference()
    listener()
  }
  window.addEventListener('storage', changed)
  return () => { listeners.delete(listener); window.removeEventListener('storage', changed) }
}
export function useWorkDetailsPreference(): WorkDetailsMode {
  return useSyncExternalStore(subscribe, () => current ??= readWorkDetailsPreference(), defaultWorkDetailsMode)
}

export const CONVERSATION_WIDTH_STORAGE_KEY = 'analytix.chatContentWidth'
export function normalizeConversationWidth(value: unknown): number | null {
  if (typeof value !== 'number' && typeof value !== 'string') return null
  const width = Number(value)
  return Number.isFinite(width) && width > 0 ? width : null
}
export function readConversationWidthPreference(): number | null {
  return normalizeConversationWidth(readBrowserStorageItem(CONVERSATION_WIDTH_STORAGE_KEY))
}
let currentWidth: number | null | undefined
const widthListeners = new Set<() => void>()
let widthStorageListenerAttached = false
function ensureWidthStorageListener(): void {
  if (widthStorageListenerAttached || typeof window === 'undefined') return
  window.addEventListener('storage', (event: StorageEvent) => {
    if (event.key !== null && event.key !== CONVERSATION_WIDTH_STORAGE_KEY) return
    // This presentation owner outlives its consumers, including route changes.
    // Unlike a missing value, an unavailable read must keep the memory value.
    const storage = browserStorage()
    if (storage === null) return
    try { currentWidth = normalizeConversationWidth(storage.getItem(CONVERSATION_WIDTH_STORAGE_KEY)) }
    catch { return }
    for (const listener of widthListeners) listener()
  })
  widthStorageListenerAttached = true
}
export function writeConversationWidthPreference(width: number | null): void {
  ensureWidthStorageListener()
  currentWidth = normalizeConversationWidth(width)
  if (currentWidth === null) removeBrowserStorageItem(CONVERSATION_WIDTH_STORAGE_KEY)
  else writeBrowserStorageItem(CONVERSATION_WIDTH_STORAGE_KEY, String(currentWidth))
  for (const listener of widthListeners) listener()
}
function subscribeWidth(listener: () => void): () => void {
  ensureWidthStorageListener()
  widthListeners.add(listener)
  return () => { widthListeners.delete(listener) }
}
export function useConversationWidthPreference(): number | null {
  return useSyncExternalStore(subscribeWidth, () => {
    if (currentWidth === undefined) currentWidth = readConversationWidthPreference()
    return currentWidth
  }, () => null)
}
