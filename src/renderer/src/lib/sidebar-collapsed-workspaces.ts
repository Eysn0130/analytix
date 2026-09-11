const SIDEBAR_COLLAPSED_WORKSPACES_KEY = 'analytix:sidebar-collapsed-workspaces:v1'
const SIDEBAR_COLLAPSED_WORKSPACES_CHANGED = 'analytix:sidebar-collapsed-workspaces-changed'

function safeWindow(): Window | null {
  return typeof window === 'undefined' ? null : window
}

export function sidebarWorkspaceCollapseKey(workspacePath: string): string {
  return workspacePath.trim()
}

function normalizeCollapsedWorkspaces(value: unknown): Record<string, boolean> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const entries = Object.entries(value)
    .filter(([key, collapsed]) => key.trim().length > 0 && typeof collapsed === 'boolean')
    .map(([key, collapsed]) => [key, collapsed] as const)
  return Object.fromEntries(entries)
}

export function readSidebarCollapsedWorkspaces(): Record<string, boolean> {
  const win = safeWindow()
  if (!win?.localStorage) return {}
  try {
    const raw = win.localStorage.getItem(SIDEBAR_COLLAPSED_WORKSPACES_KEY)
    if (!raw) return {}
    return normalizeCollapsedWorkspaces(JSON.parse(raw))
  } catch {
    return {}
  }
}

export function writeSidebarCollapsedWorkspaces(collapsed: Record<string, boolean>): void {
  const win = safeWindow()
  if (!win?.localStorage) return
  try {
    const normalized = normalizeCollapsedWorkspaces(collapsed)
    win.localStorage.setItem(SIDEBAR_COLLAPSED_WORKSPACES_KEY, JSON.stringify(normalized))
    win.dispatchEvent(new CustomEvent(SIDEBAR_COLLAPSED_WORKSPACES_CHANGED))
  } catch {
    // Ignore storage failures; this is a local UI preference.
  }
}

export function subscribeSidebarCollapsedWorkspaces(listener: () => void): () => void {
  const win = safeWindow()
  if (!win) return () => undefined
  const handleChange = (): void => listener()
  const handleStorage = (event: StorageEvent): void => {
    if (event.key === SIDEBAR_COLLAPSED_WORKSPACES_KEY) listener()
  }
  win.addEventListener(SIDEBAR_COLLAPSED_WORKSPACES_CHANGED, handleChange)
  win.addEventListener('storage', handleStorage)
  return () => {
    win.removeEventListener(SIDEBAR_COLLAPSED_WORKSPACES_CHANGED, handleChange)
    win.removeEventListener('storage', handleStorage)
  }
}
