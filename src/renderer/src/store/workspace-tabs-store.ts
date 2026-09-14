import { create } from 'zustand'
import type { RightPanelMode } from '../components/chat/WorkbenchTopBar'

export type WorkspaceTab = {
  id: string
  kind: 'document' | 'file' | 'tool'
  mode: Exclude<RightPanelMode, null>
  title: string
  workspaceRoot?: string
  path?: string
  instanceId?: string
  dirty?: boolean
  loading?: boolean
  error?: boolean
}

/** Identities stay in protected local memory; layout storage never contains paths or content. */
export function workspaceObjectTabId(workspaceRoot: string, path: string): string {
  return `object:${JSON.stringify([workspaceRoot.replaceAll('\\', '/').replace(/\/+$/, ''), path.replaceAll('\\', '/')])}`
}

type WorkspaceTabsState = {
  open: boolean
  selectorOpen: boolean
  focused: boolean
  activeTabId: string | null
  tabs: WorkspaceTab[]
  openTab: (tab: WorkspaceTab) => void
  activateTab: (id: string) => void
  closeTab: (id: string) => void
  patchTab: (id: string, patch: Partial<Omit<WorkspaceTab, 'id'>>) => void
  reorderTab: (id: string, toIndex: number) => void
  showSelector: () => void
  toggleOpen: () => void
  setOpen: (open: boolean) => void
  setFocused: (focused: boolean) => void
}

export const useWorkspaceTabsStore = create<WorkspaceTabsState>((set) => ({
  open: false, selectorOpen: true, focused: false, activeTabId: null, tabs: [],
  openTab: (tab) => set((state) => ({
    open: true, selectorOpen: false, activeTabId: tab.id,
    tabs: state.tabs.some((item) => item.id === tab.id)
      ? state.tabs.map((item) => item.id === tab.id ? { ...item, ...tab } : item)
      : [...state.tabs, tab]
  })),
  activateTab: (id) => set((state) => state.tabs.some((tab) => tab.id === id)
    ? { activeTabId: id, selectorOpen: false, open: true }
    : state),
  closeTab: (id) => set((state) => {
    const index = state.tabs.findIndex((tab) => tab.id === id)
    if (index < 0) return state
    const tabs = state.tabs.filter((tab) => tab.id !== id)
    const activeTabId = state.activeTabId === id
      ? (tabs[Math.min(index, tabs.length - 1)]?.id ?? null)
      : state.activeTabId
    return { tabs, activeTabId, selectorOpen: tabs.length === 0 || state.selectorOpen }
  }),
  patchTab: (id, patch) => set((state) => ({ tabs: state.tabs.map((tab) => tab.id === id ? { ...tab, ...patch } : tab) })),
  reorderTab: (id, toIndex) => set((state) => {
    const fromIndex = state.tabs.findIndex((tab) => tab.id === id)
    if (fromIndex < 0) return state
    const tabs = [...state.tabs]
    const [tab] = tabs.splice(fromIndex, 1)
    tabs.splice(Math.max(0, Math.min(toIndex, tabs.length)), 0, tab)
    return { tabs }
  }),
  showSelector: () => set({ open: true, selectorOpen: true }),
  toggleOpen: () => set((state) => ({ open: !state.open, focused: false })),
  setOpen: (open) => set({ open, ...(!open ? { focused: false } : {}) }),
  setFocused: (focused) => set({ focused })
}))
