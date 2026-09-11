import { create } from 'zustand'
import type {
  MergeResult,
  SyncResult,
  WorktreeChanges,
  WorktreePoolStatus
} from '@shared/worktree'

interface WorktreeStoreState {
  poolStatus: WorktreePoolStatus | null
  loading: boolean
  error: string | null
  lastMergeResult: MergeResult | null
  lastSyncResult: SyncResult | null
  setPoolStatus: (status: WorktreePoolStatus | null) => void
  setLoading: (loading: boolean) => void
  setError: (error: string | null) => void
  setLastMergeResult: (result: MergeResult | null) => void
  setLastSyncResult: (result: SyncResult | null) => void
  reset: () => void
}

export const useWorktreeStore = create<WorktreeStoreState>((set) => ({
  poolStatus: null,
  loading: false,
  error: null,
  lastMergeResult: null,
  lastSyncResult: null,
  setPoolStatus: (poolStatus) => set({ poolStatus }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  setLastMergeResult: (lastMergeResult) => set({ lastMergeResult }),
  setLastSyncResult: (lastSyncResult) => set({ lastSyncResult }),
  reset: () =>
    set({
      poolStatus: null,
      loading: false,
      error: null,
      lastMergeResult: null,
      lastSyncResult: null
    })
}))

export type { WorktreeChanges }
