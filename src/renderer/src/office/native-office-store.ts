import { create } from 'zustand'
import type { NativeOfficeView } from '@shared/native-office'

const key = (workspace: string, path: string) => JSON.stringify([workspace, path])
type Target = { workspace: string; path: string }
/** Protected-local presentation metadata only. Core owns sessions and receipts. */
export const useNativeOfficeStore = create<{
  target: Target | null
  view: NativeOfficeView | null
  views: Record<string, NativeOfficeView>
  error: string | null
  receiveSequence: number
  proposalInFlight: boolean
  annotationInputFrozen: boolean
  beginProposalPoll: () => boolean
  endProposalPoll: () => void
  select: (workspace: string, path: string, isCurrent?: () => boolean) => Promise<boolean>
  receive: (view: NativeOfficeView | null, error?: string | null) => void
  close: (workspace: string, path: string) => Promise<boolean>
}>((set, get) => ({
  target: null, view: null, views: {}, error: null, receiveSequence: 0,
  proposalInFlight: false, annotationInputFrozen: false,
  beginProposalPoll: () => {
    if (get().proposalInFlight) return false
    set({proposalInFlight:true})
    return true
  },
  endProposalPoll: () => set({proposalInFlight:false}),
  select: async (workspace, path, isCurrent = () => true) => {
    if (!isCurrent()) return false
    const old = get().target
    if (old?.workspace === workspace && old.path === path) return true
    if (typeof window !== 'undefined') await window.analytix?.office?.request({ action: 'hide' }).catch(() => undefined)
    if (!isCurrent()) return false
    set({ target: { workspace, path }, view: get().views[key(workspace, path)] ?? null, error: null })
    return true
  },
  receive: (view, error = null) => set(state => {
    const target = state.target
    // Late A updates must never replace B's title, content or selection.
    if (view && !target) return {}
    if (view && target && view.path !== target.path && view.path !== `${target.workspace.replace(/\/$/, '')}/${target.path}`) return {}
    return { view, error, receiveSequence:state.receiveSequence + 1, ...(view && target ? { views: {...state.views, [key(target.workspace, target.path)]: view} } : {}) }
  }),
  close: async (workspace, path) => {
    const identity = key(workspace, path), view = get().views[identity]
    if (view) {
      const result = await window.analytix.office.request({action:'close', objectId:view.objectId})
      if (!result.ok) { set({error:result.error ?? 'unavailable'}); return false }
    }
    set(state => {
      const views = {...state.views}; delete views[identity]
      return {views, ...(state.target?.workspace === workspace && state.target.path === path ? {target:null, view:null, error:null} : {})}
    })
    return true
  }
}))
