import { create } from 'zustand'
import type { NativeOfficeView } from '@shared/native-office'

/** Local presentation state only. Core owns sessions, activation and receipts. */
export const useNativeOfficeStore = create<{
  target: { workspace: string; path: string } | null
  view: NativeOfficeView | null
  error: string | null
  select: (workspace: string, path: string) => void
}>((set) => ({
  target: null, view: null, error: null,
  select: (workspace, path) => set({ target: { workspace, path }, error: null })
}))
