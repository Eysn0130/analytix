import { create } from 'zustand'
import type {
  HubAccountSnapshot,
  HubLoginRequest,
  HubProfileEventSyncRequest,
  HubProfileEventSyncResult,
  HubProfileLocalSyncResult,
  HubProfileUpdateRequest,
  HubProfileUpdateResult,
  HubReferralResult,
  HubRegisterRequest,
  HubUsageResult
} from '@shared/hub-account'

type AccountLoadState = 'idle' | 'loading' | 'ready' | 'error'

type HubAccountStoreState = {
  snapshot: HubAccountSnapshot | null
  usage: HubUsageResult | null
  referral: HubReferralResult | null
  status: AccountLoadState
  error: string
  initialize: () => Promise<HubAccountSnapshot>
  refresh: () => Promise<HubAccountSnapshot>
  login: (request: HubLoginRequest) => Promise<HubAccountSnapshot>
  register: (request: HubRegisterRequest) => Promise<HubAccountSnapshot>
  logout: () => Promise<HubAccountSnapshot>
  updateProfile: (request: HubProfileUpdateRequest) => Promise<HubProfileUpdateResult | null>
  syncProfileEvents: (request: HubProfileEventSyncRequest) => Promise<HubProfileEventSyncResult | null>
  syncLocalProfileEvents: () => Promise<HubProfileLocalSyncResult | null>
  loadUsage: () => Promise<HubUsageResult | null>
  loadReferral: () => Promise<HubReferralResult | null>
  clearError: () => void
}

const EMPTY_SNAPSHOT: HubAccountSnapshot = {
  authenticated: false,
  gatewayConfigured: false,
  accountReady: false,
  source: 'none'
}

function accountApi() {
  return typeof window !== 'undefined' ? window.analytix?.account : undefined
}

function missingBridgeSnapshot(): HubAccountSnapshot {
  return {
    ...EMPTY_SNAPSHOT,
    error: 'Preload bridge missing: window.analytix.account is unavailable.'
  }
}

function messageFromError(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

export const useHubAccountStore = create<HubAccountStoreState>((set, get) => ({
  snapshot: null,
  usage: null,
  referral: null,
  status: 'idle',
  error: '',

  initialize: async () => {
    const api = accountApi()
    if (!api) {
      const snapshot = missingBridgeSnapshot()
      set({ snapshot, status: 'error', error: snapshot.error ?? '' })
      return snapshot
    }
    set({ status: 'loading', error: '' })
    try {
      const snapshot = await api.getSnapshot()
      set({ snapshot, status: 'ready', error: snapshot.error ?? '' })
      return snapshot
    } catch (error) {
      const snapshot = { ...EMPTY_SNAPSHOT, error: messageFromError(error) }
      set({ snapshot, status: 'error', error: snapshot.error ?? '' })
      return snapshot
    }
  },

  refresh: async () => {
    const api = accountApi()
    if (!api) {
      const snapshot = missingBridgeSnapshot()
      set({ snapshot, status: 'error', error: snapshot.error ?? '' })
      return snapshot
    }
    set({ status: 'loading', error: '' })
    try {
      const snapshot = await api.refresh()
      set({ snapshot, status: 'ready', error: snapshot.error ?? '' })
      return snapshot
    } catch (error) {
      const snapshot = { ...(get().snapshot ?? EMPTY_SNAPSHOT), error: messageFromError(error) }
      set({ snapshot, status: 'error', error: snapshot.error ?? '' })
      return snapshot
    }
  },

  login: async (request) => {
    const api = accountApi()
    if (!api) throw new Error(missingBridgeSnapshot().error)
    set({ status: 'loading', error: '' })
    try {
      const snapshot = await api.login(request)
      set({ snapshot, status: 'ready', usage: null, referral: null, error: snapshot.error ?? '' })
      return snapshot
    } catch (error) {
      set({ status: 'error', error: messageFromError(error) })
      throw error
    }
  },

  register: async (request) => {
    const api = accountApi()
    if (!api) throw new Error(missingBridgeSnapshot().error)
    set({ status: 'loading', error: '' })
    try {
      const snapshot = await api.register(request)
      set({ snapshot, status: 'ready', usage: null, referral: null, error: snapshot.error ?? '' })
      return snapshot
    } catch (error) {
      set({ status: 'error', error: messageFromError(error) })
      throw error
    }
  },

  logout: async () => {
    const api = accountApi()
    if (!api) {
      const snapshot = missingBridgeSnapshot()
      set({ snapshot, status: 'error', error: snapshot.error ?? '' })
      return snapshot
    }
    const snapshot = await api.logout()
    set({ snapshot, usage: null, referral: null, status: 'ready', error: snapshot.error ?? '' })
    return snapshot
  },

  updateProfile: async (request) => {
    const api = accountApi()
    if (!api) {
      set({ error: missingBridgeSnapshot().error ?? '' })
      return null
    }
    const result = await api.updateProfile(request)
    if (!result.ok) {
      set({
        error: result.message,
        snapshot: result.snapshot ?? get().snapshot
      })
      return null
    }
    set((state) => ({
      snapshot: state.snapshot
        ? {
            ...state.snapshot,
            checkedAt: new Date().toISOString(),
            user: result.user
          }
        : state.snapshot,
      usage: state.usage
        ? {
            ...state.usage,
            user: result.user
          }
        : state.usage,
      error: ''
    }))
    return { user: result.user }
  },

  syncProfileEvents: async (request) => {
    const api = accountApi()
    if (!api) {
      set({ error: missingBridgeSnapshot().error ?? '' })
      return null
    }
    const result = await api.syncProfileEvents(request)
    if (!result.ok) {
      set({
        error: result.message,
        snapshot: result.snapshot ?? get().snapshot
      })
      return null
    }
    set({ error: '' })
    return {
      acceptedCount: result.acceptedCount,
      syncedCount: result.syncedCount
    }
  },

  syncLocalProfileEvents: async () => {
    const api = accountApi()
    if (!api) {
      set({ error: missingBridgeSnapshot().error ?? '' })
      return null
    }
    const result = await api.syncLocalProfileEvents()
    if (!result.ok) {
      set({
        error: result.message,
        snapshot: result.snapshot ?? get().snapshot
      })
      return null
    }
    set({ error: '' })
    return {
      acceptedCount: result.acceptedCount,
      syncedCount: result.syncedCount,
      scannedThreadCount: result.scannedThreadCount,
      generatedEventCount: result.generatedEventCount
    }
  },

  loadUsage: async () => {
    const api = accountApi()
    if (!api) {
      set({ error: missingBridgeSnapshot().error ?? '' })
      return null
    }
    const result = await api.getUsage()
    if (!result.ok) {
      set({
        error: result.message,
        snapshot: result.snapshot ?? get().snapshot
      })
      return null
    }
    set({ usage: result, error: '' })
    return result
  },

  loadReferral: async () => {
    const api = accountApi()
    if (!api) {
      set({ error: missingBridgeSnapshot().error ?? '' })
      return null
    }
    const result = await api.getReferral()
    if (!result.ok) {
      set({
        error: result.message,
        snapshot: result.snapshot ?? get().snapshot
      })
      return null
    }
    set({ referral: result, error: '' })
    return result
  },

  clearError: () => set({ error: '' })
}))

export function productEditionLabel(snapshot: HubAccountSnapshot | null): string {
  const plan = snapshot?.entitlement?.plan ?? snapshot?.user?.plan ?? 'normal'
  if (plan === 'admin') return '管理员'
  return plan === 'normal' ? '普通版' : '专业版'
}

export function displayAccountName(snapshot: HubAccountSnapshot | null): string {
  return snapshot?.user?.displayName?.trim() || snapshot?.user?.fullName?.trim() || snapshot?.user?.email?.trim() || 'Analytix'
}

export function accountInitials(snapshot: HubAccountSnapshot | null): string {
  const source = displayAccountName(snapshot)
  const ascii = source.match(/[A-Za-z0-9]/gu)?.join('').slice(0, 2)
  if (ascii) return ascii.toUpperCase()
  return source.slice(0, 2).toUpperCase()
}
