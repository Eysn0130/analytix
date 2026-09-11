const PINNED_THREAD_IDS_KEY = 'analytix:pinned-thread-ids:v1'
const PINNED_THREAD_IDS_CHANGED = 'analytix:pinned-thread-ids-changed'

function safeWindow(): Window | null {
  return typeof window === 'undefined' ? null : window
}

export function readPinnedThreadIds(): Set<string> {
  const win = safeWindow()
  if (!win?.localStorage) return new Set()
  try {
    const raw = win.localStorage.getItem(PINNED_THREAD_IDS_KEY)
    if (!raw) return new Set()
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed.filter((value): value is string => typeof value === 'string' && value.trim().length > 0))
  } catch {
    return new Set()
  }
}

function writePinnedThreadIds(ids: Set<string>): void {
  const win = safeWindow()
  if (!win?.localStorage) return
  try {
    win.localStorage.setItem(PINNED_THREAD_IDS_KEY, JSON.stringify([...ids]))
    win.dispatchEvent(new CustomEvent(PINNED_THREAD_IDS_CHANGED))
  } catch {
    // Ignore storage failures; pinning is a local UI preference.
  }
}

export function togglePinnedThreadId(threadId: string): boolean {
  const target = threadId.trim()
  if (!target) return false
  const ids = readPinnedThreadIds()
  const pinned = !ids.has(target)
  if (pinned) ids.add(target)
  else ids.delete(target)
  writePinnedThreadIds(ids)
  return pinned
}

export function subscribePinnedThreadIds(listener: () => void): () => void {
  const win = safeWindow()
  if (!win) return () => undefined
  const handleChange = (): void => listener()
  const handleStorage = (event: StorageEvent): void => {
    if (event.key === PINNED_THREAD_IDS_KEY) listener()
  }
  win.addEventListener(PINNED_THREAD_IDS_CHANGED, handleChange)
  win.addEventListener('storage', handleStorage)
  return () => {
    win.removeEventListener(PINNED_THREAD_IDS_CHANGED, handleChange)
    win.removeEventListener('storage', handleStorage)
  }
}
