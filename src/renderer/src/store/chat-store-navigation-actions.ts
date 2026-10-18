import { withImageThreadNavigation, deferImageThreadSelectionClear } from '../write/image-thread-navigation'
import type { AgentProvider, NormalizedThread } from '../agent/types'
import { getProvider } from '../agent/registry'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { applyCursorSpotlight, applyMotionPreference, applyTheme, applyUiFontScale, applyWriteTypography } from '../lib/apply-theme'
import { formatWorkspacePickerError } from '../lib/format-workspace-picker-error'
import { formatRuntimeError } from '../lib/format-runtime-error'
import {
  getDefaultThreadTitle,
  shouldAutoTitleThread
} from '../lib/thread-title'
import {
  filterThreadsForSidebar,
  shouldHideThreadFromSidebarByLineage
} from '../lib/thread-sidebar-visibility'
import { filterRevokedPublicProjectionThreads, isPublicProjectionRevoked } from '../lib/public-projection-revocation'
import { schedulePrewarmThreadDetails } from '../lib/thread-detail-cache'
import {
  enrichThreadsWithForkInfo,
  forgetThreadFork,
  hydrateThreadForkRegistry,
  markThreadFork,
  readThreadForkRegistry,
  saveThreadForkRegistry
} from '../lib/thread-fork-registry'
import { workspaceLabelFromPath } from '../lib/workspace-label'
import { isInternalTemporaryWorkspace, normalizeWorkspaceRoot } from '../lib/workspace-path'
import { buildClawRuntimePrompt } from '@shared/app-settings'
import { parseRuntimeStatusPublicV1 } from '@shared/analytix-runtime-status'
import {
  checkLocalProviderReadiness,
  localProviderRecoveryReadiness,
  resolveLocalProviderReadiness,
  type LocalProviderReadiness
} from '../account/local-provider-readiness'
import type { ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'
import { retireQueuedThreads } from './chat-store-queue'
import { getThreadBinding } from './chat-store-thread-binding'
import {
  activeClawChannel,
  forgetCodeWorkspaceRoot,
  hydrateBlockModelLabels,
  isClawThread,
  optimisticUserModelLabel,
  readCodeWorkspaceRoots,
  readStoredComposerModel,
  rememberCodeWorkspaceRoots,
  rememberTurnModel,
  reconcileCodeWorkspaceRoots,
  saveCodeWorkspaceRoots
} from './chat-store-helpers'
import {
  clearedThreadSelection,
  collectAssistantTextForTurn,
  findLatestUserBlockId,
  findReusableEmptyThreadId,
  forgetUserInputDraftInterruptionsForThread,
  markDeletedUserInputDraftThread,
  reconcileOptimisticUserBlock,
  threadSnapshotLooksRunning,
  threadBelongsToWorkspace
} from './chat-store-runtime-helpers'
import {
  forgetWriteThread,
  hydrateWriteThreadRegistry,
  isWriteThreadId,
  readWriteThreadRegistry,
  saveWriteThreadRegistry,
  writeWorkspaceForThreadId
} from '../write/write-thread-registry'
import {
  isSddAssistantThread,
  readSddThreadRegistry
} from '../sdd/sdd-thread-registry'
import {
  clearBusyWatchdog,
  resetBusyRecoveryAttempts,
  stopTurnCompletionPoll
} from './chat-store-schedulers'
import {
  armBusyWatchdog,
  buildThreadEventSink,
  clearWatchedCompletionNotification,
  finalizeTurnTiming,
  flushLiveBlocks,
  forkedMessageCount,
  forkedTurnCount,
  isCodeThread,
  looksLikeActiveTurnError,
  readWriteWorkspaceRoots,
  rememberPendingClawFeishuMirror,
  runtimeErrorDetail,
  runtimeStreamRecoveringMessage,
  shouldOpenSettingsForError,
  syncTurnCompletionPoll,
  watchTurnCompletionNotification
} from './chat-store-runtime'

function localProviderReadinessMessage(readiness: Exclude<LocalProviderReadiness, { kind: 'ready' }>): string {
  if (readiness.kind === 'setup') return i18n.t('common:localProviderSetupRequired')
  return i18n.t(readiness.reason === 'credential_reentry_required'
    ? 'common:localProviderCredentialReentryRequired' : 'common:localProviderSavedConnectionUnavailable')
}

type SseAbortRef = { current: AbortController | null }

type StoreActionContext = {
  set: ChatStoreSet
  get: ChatStoreGet
  sseAbortRef: SseAbortRef
}

let bootPromise: Promise<void> | null = null
let clawChannelActivityUnsubscribe: (() => void) | null = null
let runtimeStatusUnsubscribe: (() => void) | null = null
let cancelScheduledThreadDetailPrewarm: (() => void) | null = null
const defaultThreadRefreshLimit = 50
const archivedThreadRefreshLimit = 200
const caseProjectRefreshLimit = 100
const caseProjectThreadLoadLimit = 200
const runtimeRecoveryDelaysMs = [250, 1000, 3000, 10_000, 30_000] as const

function mergeThreadsById(existing: NormalizedThread[], incoming: NormalizedThread[]): NormalizedThread[] {
  const map = new Map<string, NormalizedThread>()
  for (const thread of existing) {
    map.set(thread.id, thread)
  }
  for (const thread of incoming) {
    const previous = map.get(thread.id)
    map.set(thread.id, previous ? { ...previous, ...thread } : thread)
  }
  return Array.from(map.values()).sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
}

function pruneRecordToIds<T>(record: Record<string, T>, ids: Set<string>): Record<string, T> {
  return Object.fromEntries(Object.entries(record).filter(([id]) => ids.has(id)))
}

function forgetComposerDraftsForThread(drafts: ChatState['composerDrafts'] | undefined, threadId: string): ChatState['composerDrafts'] {
  return Object.fromEntries(Object.entries(drafts ?? {}).filter(([key]) => {
    try {
      const owner: unknown = JSON.parse(key)
      return !Array.isArray(owner) || owner.length !== 2 || owner[1] !== threadId
    } catch { return true }
  }))
}

async function reconcileThreadListSideEffects(
  { set, get, sseAbortRef }: StoreActionContext,
  options: {
    provider: AgentProvider
    rawThreads: NormalizedThread[]
    sidebarThreads: NormalizedThread[]
    replaceThreads: boolean
    allowClearSelection: boolean
    isCurrent?: () => boolean
  }
): Promise<{ displayThreads: NormalizedThread[]; enrichedThreads: NormalizedThread[] }> {
  const { provider, replaceThreads, allowClearSelection } = options
  const cancelled = () => options.isCurrent?.() === false
  if (cancelled()) return { displayThreads: [], enrichedThreads: [] }
  const rawThreads = filterRevokedPublicProjectionThreads(options.rawThreads)
  const sddThreadRegistry = readSddThreadRegistry()
  const sidebarThreads = filterRevokedPublicProjectionThreads(options.sidebarThreads)
    .filter((thread) => !isSddAssistantThread(thread, sddThreadRegistry))
  const sidebarIds = new Set(sidebarThreads.map(thread => thread.id))
  const hiddenIds = new Set([
    ...options.rawThreads.filter(thread => !sidebarIds.has(thread.id)),
    ...get().threads.filter(shouldHideThreadFromSidebarByLineage)
  ].map(thread => thread.id))
  const previousForkRegistry = readThreadForkRegistry()
  const hydratedForkRegistry = hydrateThreadForkRegistry(
    replaceThreads ? sidebarThreads : mergeThreadsById(get().threads, sidebarThreads),
    previousForkRegistry
  )
  // Bounded listings are positive observations, never deletion receipts for
  // records outside this page. Explicit visibility/revocation still wins.
  const forkRegistry = { version: 1 as const, forks: Object.fromEntries(Object.entries({
    ...previousForkRegistry.forks, ...hydratedForkRegistry.forks
  }).filter(([id, record]) => !hiddenIds.has(id) && !isPublicProjectionRevoked(id) && !isPublicProjectionRevoked(record.parentThreadId))) }
  const enrichedThreads = enrichThreadsWithForkInfo(sidebarThreads, forkRegistry)

  const activeId = get().activeThreadId
  const activeRawThread = activeId
    ? rawThreads.find((thread) => thread.id === activeId) ?? null
    : null
  const activeThreadIsSdd =
    isSddAssistantThread(activeRawThread, sddThreadRegistry) ||
    isSddAssistantThread(
      activeId ? get().threads.find((thread) => thread.id === activeId) ?? null : null,
      sddThreadRegistry
    )
  const activeThreadFilteredFromCodeSidebar =
    get().route === 'chat' &&
    activeId != null &&
    !activeThreadIsSdd &&
    rawThreads.some((thread) => thread.id === activeId) &&
    !sidebarThreads.some((thread) => thread.id === activeId)
  const preservedSddActiveThread =
    activeThreadIsSdd && activeId
      ? activeRawThread ?? get().threads.find((thread) => thread.id === activeId) ?? null
      : null

  let displayThreads = replaceThreads
    ? enrichedThreads
    : mergeThreadsById(
      filterRevokedPublicProjectionThreads(get().threads).filter((thread) =>
        !shouldHideThreadFromSidebarByLineage(thread) && !hiddenIds.has(thread.id)),
      enrichedThreads
    )
  const pendingActiveThread =
    activeId != null &&
    !activeThreadFilteredFromCodeSidebar &&
    !displayThreads.some((thread) => thread.id === activeId)
      ? get().threads.find((thread) =>
        thread.id === activeId && !shouldHideThreadFromSidebarByLineage(thread)
      ) ?? null
      : null
  if (pendingActiveThread) {
    displayThreads = [pendingActiveThread, ...displayThreads]
  }
  if (
    preservedSddActiveThread &&
    !displayThreads.some((thread) => thread.id === preservedSddActiveThread.id)
  ) {
    displayThreads = [preservedSddActiveThread, ...displayThreads]
  }

  const writeWorkspaceRoots = await readWriteWorkspaceRoots()
  if (cancelled()) return { displayThreads: [], enrichedThreads: [] }
  saveThreadForkRegistry(forkRegistry)
  displayThreads = filterRevokedPublicProjectionThreads(displayThreads)
  let previousWriteRegistry = readWriteThreadRegistry()
  for (const id of new Set([...hiddenIds, ...Object.values(previousWriteRegistry.workspaces).flatMap(record =>
    record.threadIds.filter(isPublicProjectionRevoked))])) {
    previousWriteRegistry = forgetWriteThread(id, previousWriteRegistry)
  }
  const writeRegistry = hydrateWriteThreadRegistry(
    displayThreads,
    writeWorkspaceRoots,
    previousWriteRegistry
  )
  saveWriteThreadRegistry(writeRegistry)
  displayThreads = displayThreads.map((thread) => {
    const writeWorkspace = writeWorkspaceForThreadId(thread.id, writeRegistry)
    return writeWorkspace ? { ...thread, workspace: writeWorkspace } : thread
  })
  const codeThreadWorkspaceRoots = filterRevokedPublicProjectionThreads([
    ...rawThreads,
    ...displayThreads
  ])
    .filter((thread) => isCodeThread(thread, get().clawChannels, writeRegistry))
    .map((thread) => thread.workspace)
  const codeWorkspaceRoots = reconcileCodeWorkspaceRoots({
    currentRoots: get().codeWorkspaceRoots,
    codeThreadWorkspaceRoots,
    writeWorkspaceRoots,
    preservedWorkspaceRoots: [get().workspaceRoot]
  })
  saveCodeWorkspaceRoots(codeWorkspaceRoots)

  const activeThreadId = get().activeThreadId
  const activeThread = activeThreadId
    ? displayThreads.find((thread) => thread.id === activeThreadId) ?? null
    : null
  const activeThreadIsManagedInCodeRoute =
    get().route === 'chat' &&
    activeThread != null &&
    isClawThread(activeThread, get().clawChannels)
  const shouldClearSelection =
    allowClearSelection && !deferImageThreadSelectionClear() &&
    activeThreadId != null &&
    !displayThreads.some((thread) => thread.id === activeThreadId)
  if (shouldClearSelection) {
    sseAbortRef.current?.abort()
    sseAbortRef.current = null
  }
  const validIds = new Set(displayThreads.map((t) => t.id))
  set((s) => {
    const w: Record<string, boolean> = {}
    for (const [k, v] of Object.entries(s.watchTurnCompletion)) {
      if (v && !isPublicProjectionRevoked(k) && (!hiddenIds.has(k) || validIds.has(k))) {
        w[k] = true
      } else {
        clearWatchedCompletionNotification(k)
      }
    }
    const u: Record<string, boolean> = {}
    for (const [k, v] of Object.entries(s.unreadThreadIds)) {
      if (v && !isPublicProjectionRevoked(k) && (!hiddenIds.has(k) || validIds.has(k))) u[k] = true
    }
    return {
      threads: displayThreads,
      codeWorkspaceRoots,
      watchTurnCompletion: w,
      unreadThreadIds: u,
      ...(shouldClearSelection ? clearedThreadSelection() : {})
    }
  })
  syncTurnCompletionPoll(set, get)
  cancelScheduledThreadDetailPrewarm?.()
  cancelScheduledThreadDetailPrewarm = schedulePrewarmThreadDetails(provider, displayThreads, {
    activeThreadId: get().activeThreadId,
    concurrency: 2,
    shouldSkip: () => cancelled() || get().busy || get().runtimeConnection !== 'ready'
  })
  if (activeThreadIsManagedInCodeRoute) {
    await get().openCode()
  }
  return {
    displayThreads,
    enrichedThreads: filterRevokedPublicProjectionThreads(enrichedThreads)
  }
}

export function createNavigationActions(
  { set, get, sseAbortRef }: StoreActionContext
): Pick<ChatState, 'openCode' | 'openWrite' | 'probeRuntime' | 'boot' | 'chooseWorkspace' | 'selectWorkspaceRoot' | 'clearWorkspace' | 'deleteWorkspace' | 'refreshCaseProjects' | 'loadCaseProjectThreads' | 'setCaseProjectExpanded' | 'refreshThreads' | 'setThreadSearch' | 'setShowArchivedThreads'> {
  const binding = getThreadBinding(sseAbortRef)
  let threadRefreshGeneration = 0
  let searchRefreshTimer: ReturnType<typeof setTimeout> | null = null
  let workspaceSelectionGeneration = 0
  let workspaceDeleteReadGeneration = 0
  const workspaceDeleteOwners = new Map<string, symbol>()
  const caseProjectLoadOwners = new Map<string, symbol>()
  let caseProjectIndexRefreshTimer: ReturnType<typeof setTimeout> | null = null
  let caseProjectRefreshGeneration = 0
  let runtimeProbeGeneration = 0
  let activeUserRuntimeProbeGeneration = 0
  let runtimeRecoveryTimer: ReturnType<typeof setTimeout> | null = null
  let runtimeRecoveryAttempt = 0
  let runtimeRecoveryEpoch = 0

  const invalidateThreadRefresh = (): void => {
    threadRefreshGeneration += 1
    if (searchRefreshTimer != null) {
      clearTimeout(searchRefreshTimer)
      searchRefreshTimer = null
    }
  }

  const invalidateCaseProjectRefresh = (): void => {
    caseProjectRefreshGeneration += 1
    if (caseProjectIndexRefreshTimer != null) {
      clearTimeout(caseProjectIndexRefreshTimer)
      caseProjectIndexRefreshTimer = null
    }
  }

  const clearRuntimeRecovery = (): void => {
    runtimeRecoveryEpoch += 1
    runtimeRecoveryAttempt = 0
    if (runtimeRecoveryTimer != null) {
      clearTimeout(runtimeRecoveryTimer)
      runtimeRecoveryTimer = null
    }
  }

  const scheduleRuntimeRecovery = (): void => {
    if (
      get().runtimeConnection !== 'offline' ||
      activeUserRuntimeProbeGeneration !== 0 ||
      runtimeRecoveryTimer != null
    ) {
      return
    }
    const epoch = runtimeRecoveryEpoch
    const delay = runtimeRecoveryDelaysMs[Math.min(runtimeRecoveryAttempt, runtimeRecoveryDelaysMs.length - 1)]
    runtimeRecoveryTimer = setTimeout(() => {
      runtimeRecoveryTimer = null
      if (
        epoch !== runtimeRecoveryEpoch ||
        get().runtimeConnection !== 'offline' ||
        activeUserRuntimeProbeGeneration !== 0
      ) {
        return
      }
      runtimeRecoveryAttempt = Math.min(runtimeRecoveryAttempt + 1, runtimeRecoveryDelaysMs.length - 1)
      const probeRuntime = get().probeRuntime
      if (typeof probeRuntime !== 'function') return
      void probeRuntime('background').then(() => {
        if (
          epoch !== runtimeRecoveryEpoch ||
          activeUserRuntimeProbeGeneration !== 0
        ) {
          return
        }
        if (get().runtimeConnection !== 'offline') return
        scheduleRuntimeRecovery()
      }).catch(() => {
        if (
          epoch !== runtimeRecoveryEpoch ||
          activeUserRuntimeProbeGeneration !== 0
        ) {
          return
        }
        scheduleRuntimeRecovery()
      })
    }, delay)
  }

  return {
  openCode: async () => withImageThreadNavigation(undefined, async () => {
    const state = get()
    const activeThread = state.activeThreadId
      ? state.threads.find((thread) => thread.id === state.activeThreadId) ?? null
      : null
    if (activeThread && isCodeThread(activeThread, state.clawChannels)) {
      set({ route: 'chat' })
      return
    }

    const openCleanCodeStage = (): void => {
      sseAbortRef.current?.abort()
      sseAbortRef.current = null
      clearBusyWatchdog()
      const nextWatch = { ...state.watchTurnCompletion }
      if (state.activeThreadId && state.busy) {
        nextWatch[state.activeThreadId] = true
        watchTurnCompletionNotification(state.activeThreadId)
      }
      set({
        ...clearedThreadSelection(),
        route: 'chat',
        watchTurnCompletion: nextWatch
      })
      syncTurnCompletionPoll(set, get)
    }

    openCleanCodeStage()
  }),

  // Compatibility entry: opening a document never selects or creates a thread.
  // Workbench consumes this legacy route by opening the right document surface.
  openWrite: async () => {
    set({ route: 'write' })
  },

  probeRuntime: async (mode = 'user', options) => {
    const probeGeneration = ++runtimeProbeGeneration
    const prev = get().runtimeConnection
    let readinessFailure: Exclude<LocalProviderReadiness, { kind: 'ready' }> | null = null
    if (mode === 'user') {
      clearRuntimeRecovery()
      activeUserRuntimeProbeGeneration = probeGeneration
      // A later ready state cannot revive work from before this connection check.
      invalidateThreadRefresh()
      invalidateCaseProjectRefresh()
      set({ runtimeConnection: 'checking' })
    }
    try {
      if (typeof window.analytix === 'undefined') {
        throw new Error(
          'Preload bridge missing (window.analytix). Restart the app or check BrowserWindow preload path.'
        )
      }
      const settings = await rendererRuntimeClient.getSettings({ forceRefresh: true })
      if (options?.restart) {
        await rendererRuntimeClient.restartRuntime()
      }
      const p = getProvider()
      await p.connect()
      const providerReadiness = await checkLocalProviderReadiness(
        (request) => window.analytix.providerRegistry.request(request)
      )
      if (providerReadiness.kind !== 'ready') {
        readinessFailure = providerReadiness
        throw new Error('Local Provider readiness requires recovery')
      }
      if (probeGeneration !== runtimeProbeGeneration &&
          probeGeneration !== activeUserRuntimeProbeGeneration) return
      if (mode === 'background' && activeUserRuntimeProbeGeneration !== 0) return
      set({ runtimeConnection: 'ready', error: null, runtimeErrorDetail: null })
      const preloadTasks: Promise<void>[] = [
        get().loadComposerModels().catch(() => {
          /* Model discovery is helpful for startup polish, but connection readiness owns the error state. */
        })
      ]
      if (prev !== 'ready' || mode === 'user') {
        preloadTasks.push(get().refreshThreads().catch(() => {
          /* refreshThreads sets state */
        }))
      }
      await Promise.all(preloadTasks)
      if (get().runtimeConnection === 'ready') clearRuntimeRecovery()
      if (mode === 'user' && activeUserRuntimeProbeGeneration === probeGeneration) {
        activeUserRuntimeProbeGeneration = 0
        if (get().runtimeConnection === 'offline') scheduleRuntimeRecovery()
      }
    } catch (e) {
      if (probeGeneration !== runtimeProbeGeneration &&
          probeGeneration !== activeUserRuntimeProbeGeneration) return
      if (mode === 'background' && activeUserRuntimeProbeGeneration !== 0) return
      if (mode === 'background' && get().runtimeConnection === 'checking') return
      if (mode === 'user' && activeUserRuntimeProbeGeneration === probeGeneration) {
        activeUserRuntimeProbeGeneration = 0
      }
      const msg = readinessFailure ? localProviderReadinessMessage(readinessFailure) : formatRuntimeError(e)
      const detail = readinessFailure ? null : runtimeErrorDetail(e)
      const needsSettings = shouldOpenSettingsForError(e)
      if (mode === 'user') {
        invalidateThreadRefresh()
        invalidateCaseProjectRefresh()
        stopTurnCompletionPoll()
        set({
          runtimeConnection: 'offline',
          error: msg,
          runtimeErrorDetail: detail,
          ...(readinessFailure?.kind === 'recovery'
            ? { route: 'settings' as const, settingsSection: 'providers' as const }
            : needsSettings
              ? { route: 'settings' as const, settingsSection: 'agents' as const }
              : {})
        })
      } else {
        if (prev === 'ready') {
          invalidateThreadRefresh()
          invalidateCaseProjectRefresh()
          stopTurnCompletionPoll()
          set({
            runtimeConnection: 'offline',
            error: msg,
            runtimeErrorDetail: detail,
            ...(needsSettings
              ? { route: 'settings' as const, settingsSection: 'agents' as const }
              : {})
          })
        }
        if (get().runtimeConnection === 'offline') scheduleRuntimeRecovery()
      }
      if (readinessFailure) {
        // Core is reachable even when model requests still require Provider setup.
        await get().refreshThreads({ isCurrent: () => probeGeneration === runtimeProbeGeneration &&
          (mode !== 'background' || activeUserRuntimeProbeGeneration === 0) })
      }
    }
  },

  boot: async () => {
    if (bootPromise) return bootPromise
    bootPromise = (async () => {
      try {
        if (typeof window.analytix === 'undefined') {
          set({
            error: formatRuntimeError(
              'Preload bridge missing (window.analytix). Restart the app or check BrowserWindow preload path.'
            ),
            runtimeConnection: 'offline',
            runtimeErrorDetail: 'Preload bridge missing (window.analytix). Restart the app or check BrowserWindow preload path.',
            initialSetupOpen: false,
            initialSetupMode: 'required'
          })
          return
        }
        const settings = await rendererRuntimeClient.getSettings({ forceRefresh: true })
        const workspaceRoot = normalizeWorkspaceRoot(settings.workspaceRoot)
        const writeWorkspaceRoots = [
          settings.write.defaultWorkspaceRoot,
          settings.write.activeWorkspaceRoot,
          ...settings.write.workspaces
        ]
        const codeWorkspaceRoots = reconcileCodeWorkspaceRoots({
          currentRoots: readCodeWorkspaceRoots(),
          codeThreadWorkspaceRoots: [workspaceRoot],
          writeWorkspaceRoots,
          preservedWorkspaceRoots: [workspaceRoot]
        })
        saveCodeWorkspaceRoots(codeWorkspaceRoots)
        const readiness = await (async () => {
          try {
            return resolveLocalProviderReadiness(
              await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' })
            )
          } catch {
            return localProviderRecoveryReadiness()
          }
        })()
        const needsInitialSetup = readiness.kind === 'setup'
        const needsProviderRecovery = readiness.kind === 'recovery'
        applyTheme(settings.theme, settings.accentColor)
        applyUiFontScale(settings.uiFontScale)
        applyMotionPreference(settings.motionPreference)
        applyCursorSpotlight(settings.cursorSpotlight !== false)
        if (settings.write?.typography) applyWriteTypography(settings.write.typography)
        await get().applyI18nFromSettings(settings.locale)
        if (!runtimeStatusUnsubscribe && typeof window.analytix.runtime.onRuntimeStatus === 'function') {
          runtimeStatusUnsubscribe = window.analytix.runtime.onRuntimeStatus((status) => {
            const publicStatus = parseRuntimeStatusPublicV1(status)
            if (!publicStatus) return
            set({ runtimeStatus: publicStatus })
            if (publicStatus.state === 'failed' || publicStatus.state === 'stopped') {
              // Terminal states reuse the main error banner, which carries
              // the full diagnostics UI (details, log path, settings).
              set({ error: publicStatus.message })
              void get().probeRuntime('background')
              return
            }
            if (publicStatus.state === 'running') {
              void get().probeRuntime('background')
              if (publicStatus.rolledBack) {
                // On-disk settings were restored by the rollback; refresh the cache.
                void rendererRuntimeClient.getSettings({ forceRefresh: true }).catch(() => null)
              }
            }
          })
        }
        if (!clawChannelActivityUnsubscribe && typeof window.analytix.connectPhone.onChannelActivity === 'function') {
          clawChannelActivityUnsubscribe = window.analytix.connectPhone.onChannelActivity(({ channelId, threadId }) => {
            void (async () => {
              const state = get()
              if (typeof window.analytix === 'undefined') return
              const settings = await rendererRuntimeClient.getSettings({ forceRefresh: true })
              const channels = settings.claw.channels
              const activeChannelId = channels.some(
                (channel) => channel.id === state.activeClawChannelId && channel.enabled
              )
                ? state.activeClawChannelId
                : channels.find((channel) => channel.enabled)?.id ?? ''
              set({
                disabledSkillIds: settings.disabledSkillIds,
                clawChannels: channels,
                activeClawChannelId: activeChannelId
              })
              void get().refreshThreads()
              if (state.route === 'claw' && state.activeClawChannelId === channelId) {
                if (state.activeThreadId !== threadId) {
                  await get().subscribeThreadEventsLive(threadId)
                } else {
                  await get().recoverActiveTurn()
                }
              }
            })()
          })
        }
        set({
          route: needsProviderRecovery ? 'settings' : 'chat',
          settingsSection: needsProviderRecovery ? 'providers' : get().settingsSection,
          initialSetupOpen: needsInitialSetup,
          initialSetupMode: 'required',
          workspaceRoot,
          codeWorkspaceRoots,
          workspaceLabel: workspaceLabelFromPath(workspaceRoot),
          disabledSkillIds: settings.disabledSkillIds,
          clawChannels: settings.claw.channels,
          activeClawChannelId: settings.claw.channels.find((channel) => channel.enabled)?.id ?? '',
          runtimeConnection: readiness.kind === 'ready' ? get().runtimeConnection : 'idle',
          error: readiness.kind === 'recovery' ? localProviderReadinessMessage(readiness) : null,
          runtimeErrorDetail: null
        })
        if (readiness.kind !== 'ready') {
          await get().refreshThreads()
          return
        }
        const initialPick = get().composerPickList
        const fromStorage = readStoredComposerModel(initialPick)
        if (fromStorage) {
          set({ composerModel: fromStorage })
        } else {
          set({ composerModel: readiness.model, composerProviderId: readiness.providerId })
        }
        await get().probeRuntime('user')
      } catch (e) {
        set({
          error: formatRuntimeError(e),
          runtimeErrorDetail: runtimeErrorDetail(e),
          runtimeConnection: 'offline',
          initialSetupOpen: false,
          initialSetupMode: 'required',
          ...(shouldOpenSettingsForError(e)
            ? { route: 'settings' as const, settingsSection: 'agents' as const }
            : {})
        })
      }
    })().finally(() => {
      bootPromise = null
    })
    return bootPromise
  },

  chooseWorkspace: async ({ createThreadAfter = false, selectThreadAfter = true } = {}) => withImageThreadNavigation(null, async navigationCurrent => {
    workspaceSelectionGeneration += 1
    try {
      const wasWriteRoute = get().route === 'write'
      if (typeof window.analytix === 'undefined' || typeof window.analytix.workspace.pickDirectory !== 'function') {
        throw new Error(i18n.t('common:workspacePickerUnavailable'))
      }
      const picked = await window.analytix.workspace.pickDirectory(get().workspaceRoot || undefined)
      if (!navigationCurrent()) return null
      if (picked.canceled || !picked.path) {
        if (createThreadAfter) {
          set({ error: i18n.t('common:workspaceRequiredToCreateThread') })
        }
        return null
      }
      const next = await rendererRuntimeClient.setSettings({ workspaceRoot: picked.path })
      if (!navigationCurrent()) return null
      const workspaceRoot = normalizeWorkspaceRoot(next.workspaceRoot)
      const codeWorkspaceRoots = rememberCodeWorkspaceRoots(get().codeWorkspaceRoots, [workspaceRoot])

      set({
        workspaceRoot,
        codeWorkspaceRoots,
        workspaceLabel: workspaceLabelFromPath(workspaceRoot),
        error: null
      })
      await get().refreshThreads()
      if (!navigationCurrent()) return null
      if (workspaceRoot) {
        if (!selectThreadAfter) return workspaceRoot
        if (wasWriteRoute) {
          await get().openWrite()
          if (!navigationCurrent()) return null
          return workspaceRoot
        }
        const workspaceThreads = get().threads
          .filter((thread) => isCodeThread(thread, get().clawChannels))
          .filter((thread) => threadBelongsToWorkspace(thread, workspaceRoot))
          .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))

        if (createThreadAfter || workspaceThreads.length === 0) {
          await get().createThread({ workspaceRoot })
          if (!navigationCurrent()) return null
        } else {
          const targetThreadId = workspaceThreads[0]?.id
          if (targetThreadId && get().activeThreadId !== targetThreadId) {
            await get().selectThread(targetThreadId)
            if (!navigationCurrent()) return null
          }
        }
      }
      return workspaceRoot
    } catch (e) {
      if (!navigationCurrent()) return null
      set({
        error: formatWorkspacePickerError(e)
      })
      return null
    }
  }),

  // Switch the active working directory to an already-known workspace (no native
  // picker). Persists the choice and lands on a clean new-conversation state for
  // that directory — typing then starts a fresh thread there. This backs the
  // workspace picker shown beneath the composer.
  selectWorkspaceRoot: async (workspaceRoot) => withImageThreadNavigation(null, async navigationCurrent => {
    workspaceSelectionGeneration += 1
    const normalized = normalizeWorkspaceRoot(workspaceRoot)
    if (!normalized) return null
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return null
    }
    // Already on this directory with an empty composer — nothing to switch.
    if (normalizeWorkspaceRoot(get().workspaceRoot) === normalized && !get().activeThreadId) {
      set({ route: 'chat', error: null })
      return normalized
    }
    try {
      const next = await rendererRuntimeClient.setSettings({ workspaceRoot: normalized })
      if (!navigationCurrent()) return null
      const persisted = normalizeWorkspaceRoot(next.workspaceRoot) || normalized
      sseAbortRef.current?.abort()
      sseAbortRef.current = null
      clearBusyWatchdog()
      resetBusyRecoveryAttempts()
      set((s) => ({
        ...clearedThreadSelection(),
        route: 'chat',
        workspaceRoot: persisted,
        workspaceLabel: workspaceLabelFromPath(persisted),
        codeWorkspaceRoots: rememberCodeWorkspaceRoots(s.codeWorkspaceRoots, [persisted]),
        error: null
      }))
      await get().refreshThreads()
      if (!navigationCurrent()) return null
      return persisted
    } catch (e) {
      if (!navigationCurrent()) return null
      set({ error: formatRuntimeError(e) })
      return null
    }
  }),

  clearWorkspace: async () => withImageThreadNavigation(undefined, async navigationCurrent => {
    workspaceSelectionGeneration += 1
    try {
      if (typeof window.analytix === 'undefined' || typeof window.analytix.settings.setSettings !== 'function') {
        return
      }
      const next = await rendererRuntimeClient.setSettings({ workspaceRoot: '' })
      if (!navigationCurrent()) return
      set({
        workspaceRoot: normalizeWorkspaceRoot(next.workspaceRoot),
        codeWorkspaceRoots: get().codeWorkspaceRoots,
        workspaceLabel: workspaceLabelFromPath(''),
        error: null
      })
      await get().refreshThreads()
      if (!navigationCurrent()) return
    } catch {
      if (!navigationCurrent()) return
      // silently ignore — the workspace will remain set
    }
  }),

  deleteWorkspace: async (workspacePath) => withImageThreadNavigation(undefined, async navigationCurrent => {
    const normalizedPath = normalizeWorkspaceRoot(workspacePath)
    if (!normalizedPath) return
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const p = getProvider()
    const deleteOwner = Symbol('workspace-delete')
    workspaceDeleteOwners.set(normalizedPath, deleteOwner)
    const readGeneration = ++workspaceDeleteReadGeneration
    const connectionEpoch = runtimeRecoveryEpoch
    const ownsIntent = () => workspaceDeleteOwners.get(normalizedPath) === deleteOwner && navigationCurrent() &&
      getProvider() === p && get().runtimeConnection === 'ready' && runtimeRecoveryEpoch === connectionEpoch
    const rootGeneration = workspaceSelectionGeneration
    let expectedWorkspaceRoot = normalizeWorkspaceRoot(get().workspaceRoot)
    const ownsWorkspace = () => ownsIntent() && rootGeneration === workspaceSelectionGeneration &&
      normalizeWorkspaceRoot(get().workspaceRoot) === expectedWorkspaceRoot
    const captureSelection = (current: () => boolean = ownsIntent): (() => boolean) => {
      const id = get().activeThreadId, controller = sseAbortRef.current, route = get().route
      const workspace = normalizeWorkspaceRoot(get().workspaceRoot), workspaceGeneration = workspaceSelectionGeneration
      const generation = binding.generation, submission = binding.submissionGeneration, interruption = binding.interruptionGeneration
      return () => current() && get().activeThreadId === id && get().route === route &&
        workspaceSelectionGeneration === workspaceGeneration && normalizeWorkspaceRoot(get().workspaceRoot) === workspace &&
        (sseAbortRef.current === controller || controller !== null && sseAbortRef.current === null && binding.naturallyRetired.has(controller)) &&
        binding.generation === generation && binding.submissionGeneration === submission && binding.interruptionGeneration === interruption
    }
    let expectedError = get().error
    const expectedErrorDetail = get().runtimeErrorDetail
    const captureErrorOwner = (current: () => boolean): (() => boolean) =>
      () => current() && ownsWorkspace() && get().error === expectedError && get().runtimeErrorDetail === expectedErrorDetail
    let ownsCompletion = captureSelection()
    let ownsError = captureErrorOwner(ownsCompletion)
    try {
      // The sidebar is paginated and can be a search projection. Only the
      // complete public listing determines this explicitly requested target.
      const listed = await p.listThreads({ limit: 0, includeArchived: true })
      if (!ownsIntent()) return
      const workspaceThreads = filterRevokedPublicProjectionThreads(listed).filter(thread =>
        !shouldHideThreadFromSidebarByLineage(thread) && threadBelongsToWorkspace(thread, normalizedPath))
      let confirmed = 0
      for (const th of workspaceThreads) {
        if (!ownsIntent()) break
        const activeTarget = get().activeThreadId === th.id
        const controller = sseAbortRef.current
        const requestOwnsSelection = captureSelection()
        ownsCompletion = requestOwnsSelection
        ownsError = captureErrorOwner(requestOwnsSelection)
        await p.deleteThread(th.id)
        confirmed += 1
        invalidateThreadRefresh()
        invalidateCaseProjectRefresh()
        saveThreadForkRegistry(forgetThreadFork(th.id))
        saveWriteThreadRegistry(forgetWriteThread(th.id))
        clearWatchedCompletionNotification(th.id)
        const detach = activeTarget && requestOwnsSelection()
        if (detach) {
          binding.generation += 1
          controller?.abort()
          sseAbortRef.current = null
          clearBusyWatchdog()
          resetBusyRecoveryAttempts()
        }
        const projectIds = new Set((get().caseProjects ?? []).filter(project =>
          normalizeWorkspaceRoot(project.rootPath) === normalizedPath).map(project => project.id))
        for (const id of projectIds) caseProjectLoadOwners.delete(id)
        // Each actual receipt retires only that thread's local owners. A later
        // failure cannot restore them or cancel an unrelated/new selection.
        set(s => {
          const watch = { ...s.watchTurnCompletion }, unread = { ...s.unreadThreadIds }
          delete watch[th.id]; delete unread[th.id]
          return {
            threads: s.threads.filter(thread => thread.id !== th.id),
            threadSearchResults: s.threadSearchResults ? { ...s.threadSearchResults,
              threads: s.threadSearchResults.threads.filter(thread => thread.id !== th.id) } : null,
            caseProjectThreadsById: Object.fromEntries(Object.entries(s.caseProjectThreadsById ?? {}).map(([id, threads]) =>
              [id, threads.filter(thread => thread.id !== th.id)])),
            caseProjectLoadingById: { ...s.caseProjectLoadingById, ...Object.fromEntries([...projectIds].map(id => [id, false])) },
            ...retireQueuedThreads(s, [th.id]),
            composerDrafts: forgetComposerDraftsForThread(s.composerDrafts, th.id),
            userInputDraftInterruptions: forgetUserInputDraftInterruptionsForThread(s.userInputDraftInterruptions, th.id),
            deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(s.deletedUserInputDraftThreadIds, th.id),
            ...(s.userInputPanel?.threadId === th.id ? { userInputPanel: undefined } : {}),
            watchTurnCompletion: watch,
            unreadThreadIds: unread,
            ...(detach ? clearedThreadSelection() : {})
          }
        })
        if (detach) ownsCompletion = captureSelection()
        syncTurnCompletionPoll(set, get)
        if (!ownsIntent()) break
      }
      if (confirmed !== workspaceThreads.length || !ownsWorkspace() || !ownsCompletion()) return
      ownsError = captureErrorOwner(ownsCompletion)
      const currentWorkspace = normalizeWorkspaceRoot(get().workspaceRoot)
      if (currentWorkspace === normalizedPath) {
        if (typeof window.analytix?.settings?.setSettings !== 'function') {
          throw new Error('Workspace settings are unavailable.')
        }
        const next = await rendererRuntimeClient.setSettings({ workspaceRoot: '' })
        if (!ownsWorkspace() || !ownsCompletion()) return
        const nextRoot = normalizeWorkspaceRoot(next.workspaceRoot)
        set({ workspaceRoot: nextRoot, workspaceLabel: workspaceLabelFromPath(nextRoot) })
        expectedWorkspaceRoot = nextRoot
        ownsCompletion = captureSelection()
        ownsError = captureErrorOwner(ownsCompletion)
      }
      const roots = forgetCodeWorkspaceRoot(get().codeWorkspaceRoots, normalizedPath)
      saveCodeWorkspaceRoots(roots)
      const clearOwnedError = ownsError()
      set({ codeWorkspaceRoots: roots, ...(clearOwnedError ? { error: null } : {}) })
      if (clearOwnedError) expectedError = null
      if (ownsError()) {
        // Completed mutation tickets are released below. The admitted index
        // read may keep polling while building, until a later intent/context
        // retires its primitive owner; it cannot retain the mutation ticket.
        const readerSelection = captureSelection(() => workspaceDeleteReadGeneration === readGeneration &&
          navigationCurrent() && getProvider() === p && get().runtimeConnection === 'ready' &&
          runtimeRecoveryEpoch === connectionEpoch && workspaceSelectionGeneration === rootGeneration &&
          normalizeWorkspaceRoot(get().workspaceRoot) === expectedWorkspaceRoot)
        await get().refreshThreads({ isCurrent: () => readerSelection() && get().error === expectedError &&
          get().runtimeErrorDetail === expectedErrorDetail })
      }
    } catch (e) {
      if (!ownsError()) return
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    } finally {
      if (workspaceDeleteOwners.get(normalizedPath) === deleteOwner) workspaceDeleteOwners.delete(normalizedPath)
    }
	  }),

  refreshCaseProjects: async (options = {}) => {
    if (options.isCurrent?.() === false) return
    const p = getProvider()
    if (typeof p.listCaseProjects !== 'function') return
    // A newer admitted read owns the existing timer slot, even while its I/O is
    // pending. An obsolete timer/result must neither block nor replace its chain.
    invalidateCaseProjectRefresh()
    const generation = caseProjectRefreshGeneration
    const connectionState = get().runtimeConnection
    const isCurrent = () => generation === caseProjectRefreshGeneration &&
      options.isCurrent?.() !== false && getProvider() === p && get().runtimeConnection === connectionState
    try {
      if (get().runtimeConnection !== 'ready') {
        await p.connect()
        if (!isCurrent()) return
      }
      const caseProjectResult = await p.listCaseProjects(caseProjectRefreshLimit)
      if (!isCurrent()) return
      const caseProjects = Array.isArray(caseProjectResult)
        ? caseProjectResult
        : caseProjectResult.caseProjects
      const caseProjectIndexStatus = Array.isArray(caseProjectResult)
        ? 'ready'
        : caseProjectResult.indexStatus
      const validProjectIds = new Set(caseProjects.map((project) => project.id))
      set((s) => ({
        caseProjects,
        caseProjectIndexStatus,
        caseProjectThreadsById: pruneRecordToIds(s.caseProjectThreadsById, validProjectIds),
        caseProjectLoadingById: pruneRecordToIds(s.caseProjectLoadingById, validProjectIds),
        caseProjectErrorsById: pruneRecordToIds(s.caseProjectErrorsById, validProjectIds),
        caseProjectExpandedById: pruneRecordToIds(s.caseProjectExpandedById, validProjectIds)
      }))
      if (caseProjectIndexStatus === 'building' && isCurrent()) {
        const timer = setTimeout(() => {
          if (caseProjectIndexRefreshTimer !== timer) return
          caseProjectIndexRefreshTimer = null
          if (isCurrent()) {
            void get().refreshCaseProjects(options).catch(() => {
              /* refreshCaseProjects stores state on success */
            })
          }
        }, 1500)
        caseProjectIndexRefreshTimer = timer
      }
    } catch (error) {
      // Do not turn an obsolete read failure into current reconnect/error state.
      if (isCurrent()) throw error
    }
  },

	  loadCaseProjectThreads: async (caseProjectId, options = {}) => {
	    const projectId = caseProjectId.trim()
      if (options.isCurrent?.() === false) return
	    if (!projectId) return
	    const p = getProvider()
	    if (typeof p.listCaseProjectThreads !== 'function') return
	    if (!options.force && get().caseProjectThreadsById[projectId]) return
	    if (get().caseProjectLoadingById[projectId]) return
	    const loadOwner = Symbol('case-project-load')
      caseProjectLoadOwners.set(projectId, loadOwner)
      const connectionState = get().runtimeConnection
      const isCurrent = () => caseProjectLoadOwners.get(projectId) === loadOwner &&
        options.isCurrent?.() !== false && getProvider() === p && get().runtimeConnection === connectionState
      set((s) => ({
	      caseProjectLoadingById: { ...s.caseProjectLoadingById, [projectId]: true },
	      caseProjectErrorsById: { ...s.caseProjectErrorsById, [projectId]: null }
	    }))
	    try {
        if (get().runtimeConnection !== 'ready') {
          await p.connect()
          if (!isCurrent()) return
        }
	      const rawThreads = await p.listCaseProjectThreads(projectId, caseProjectThreadLoadLimit)
        if (!isCurrent()) return
	      const threads = rawThreads.map((thread) => ({
	        ...thread,
	        workspace: normalizeWorkspaceRoot(thread.workspace)
	      }))
		      const sidebarThreads = (await filterThreadsForSidebar(threads, p))
		      const { enrichedThreads } = await reconcileThreadListSideEffects(
		        { set, get, sseAbortRef },
		        {
		          provider: p,
		          rawThreads: threads,
		          sidebarThreads,
		          replaceThreads: false,
		          allowClearSelection: false,
              isCurrent
		        }
		      )
		      if (!isCurrent()) return
          set((s) => ({
		        caseProjectThreadsById: {
		          ...s.caseProjectThreadsById,
		          [projectId]: filterRevokedPublicProjectionThreads(enrichedThreads)
		        },
		        caseProjectLoadingById: { ...s.caseProjectLoadingById, [projectId]: false },
		        caseProjectErrorsById: { ...s.caseProjectErrorsById, [projectId]: null }
		      }))
	    } catch (error) {
        if (!isCurrent()) return
	      set((s) => ({
	        caseProjectLoadingById: { ...s.caseProjectLoadingById, [projectId]: false },
	        caseProjectErrorsById: {
	          ...s.caseProjectErrorsById,
	          [projectId]: formatRuntimeError(error)
	        }
	      }))
      } finally {
        // Release only this request's loading marker, not a replacement load.
        if (caseProjectLoadOwners.get(projectId) === loadOwner) {
          caseProjectLoadOwners.delete(projectId)
          if (get().caseProjectLoadingById[projectId]) {
            set(state => ({ caseProjectLoadingById: { ...state.caseProjectLoadingById, [projectId]: false } }))
          }
        }
      }
	  },

	  setCaseProjectExpanded: (caseProjectId, expanded) => {
	    const projectId = caseProjectId.trim()
	    if (!projectId) return
	    set((s) => ({
	      caseProjectExpandedById: {
	        ...s.caseProjectExpandedById,
	        [projectId]: expanded
	      }
	    }))
	    if (expanded) {
	      void get().loadCaseProjectThreads(projectId)
	    }
	  },

	  refreshThreads: async (options = {}) => {
      if (options.isCurrent?.() === false) return
      const generation = ++threadRefreshGeneration
      const connectionState = get().runtimeConnection
      const searchQuery = get().threadSearch.trim()
      const showArchived = get().showArchivedThreads
      const p = getProvider()
      const isCurrent = () => generation === threadRefreshGeneration && options.isCurrent?.() !== false &&
        get().runtimeConnection === connectionState && get().threadSearch.trim() === searchQuery &&
        get().showArchivedThreads === showArchived && getProvider() === p
	    try {
        // Local inventory uses Core authority; this does not authorize model requests.
        if (get().runtimeConnection !== 'ready') {
          await p.connect()
          if (!isCurrent()) return
        }
	      const useCaseProjectSummaries =
	        typeof p.listCaseProjects === 'function' &&
	        !searchQuery &&
	        !get().showArchivedThreads
      if (useCaseProjectSummaries) {
        let listedThreads: NormalizedThread[] = []
        try {
          listedThreads = await p.listThreads({
            limit: defaultThreadRefreshLimit,
            search: undefined,
            includeArchived: true
          })
        } catch {
          listedThreads = []
        }
        if (!isCurrent()) return
        await get().refreshCaseProjects({ isCurrent })
        if (!isCurrent()) return
        const expandedProjectIds = Object.entries(get().caseProjectExpandedById)
          .filter(([, expanded]) => expanded)
          .map(([projectId]) => projectId)
        await Promise.all(expandedProjectIds.map((projectId) =>
          get().loadCaseProjectThreads(projectId, { force: true, isCurrent })
        ))
        if (!isCurrent()) return
        const retainedThreads = mergeThreadsById(
          get().threads,
          listedThreads
        ).map((thread) => ({
          ...thread,
          workspace: normalizeWorkspaceRoot(thread.workspace)
        }))
        const retainedSidebarThreads = await filterThreadsForSidebar(retainedThreads, p)
        await reconcileThreadListSideEffects(
          { set, get, sseAbortRef },
          {
            provider: p,
            rawThreads: retainedThreads,
            sidebarThreads: retainedSidebarThreads,
            replaceThreads: true,
            allowClearSelection: false,
            isCurrent
          }
        )
        return
      }
	      let rawThreads: NormalizedThread[]
	      try {
	        rawThreads = await p.listThreads({
	          limit: get().showArchivedThreads ? archivedThreadRefreshLimit : defaultThreadRefreshLimit,
	          search: searchQuery || undefined,
	          includeArchived: true
	        })
	      } catch (error) {
        if (!isCurrent()) return
        // An unfiltered inventory is never a substitute for a failed search.
        if (searchQuery) throw error
        rawThreads = await p.listThreads()
      }
      if (!isCurrent()) return
      const threads = rawThreads.map((thread) => ({
        ...thread,
        workspace: normalizeWorkspaceRoot(thread.workspace)
      }))
	      const sidebarThreads = await filterThreadsForSidebar(threads, p)
        if (!isCurrent()) return
        if (searchQuery) {
          const sddRegistry = readSddThreadRegistry()
          const writeRegistry = readWriteThreadRegistry()
          const matches = enrichThreadsWithForkInfo(
            filterRevokedPublicProjectionThreads(sidebarThreads).filter(thread => !isSddAssistantThread(thread, sddRegistry)),
            readThreadForkRegistry()
          ).map(thread => {
            const workspace = writeWorkspaceForThreadId(thread.id, writeRegistry)
            return workspace ? { ...thread, workspace } : thread
          })
          set({ threadSearchResults: { query: searchQuery, threads: matches } })
          return
        }
	      await reconcileThreadListSideEffects(
	        { set, get, sseAbortRef },
	        {
	          provider: p,
	          rawThreads: threads,
	          sidebarThreads,
	          replaceThreads: false,
	          allowClearSelection: true,
            isCurrent
	        }
	      )
	    } catch (e) {
        if (!isCurrent()) return
	      const refreshError = formatRuntimeError(e)
	      try {
	        await p.connect()
          if (!isCurrent()) return
	        set({ error: refreshError })
	      } catch (connectionError) {
          if (!isCurrent()) return
          invalidateThreadRefresh()
          invalidateCaseProjectRefresh()
	        stopTurnCompletionPoll()
	        set({
	          runtimeConnection: 'offline',
	          error: formatRuntimeError(connectionError),
	          ...(shouldOpenSettingsForError(connectionError)
	            ? { route: 'settings' as const, settingsSection: 'agents' as const }
	            : {})
	        })
	        scheduleRuntimeRecovery()
	      }
	    }
	  },

	  setThreadSearch: (query) => {
      invalidateThreadRefresh()
	    set({ threadSearch: query, threadSearchResults: null })
	    searchRefreshTimer = setTimeout(() => {
	      searchRefreshTimer = null
	      void get().refreshThreads()
	    }, query.trim() ? 250 : 0)
	  },

  setShowArchivedThreads: (show) => {
    invalidateThreadRefresh()
    set({ showArchivedThreads: show, threadSearchResults: null })
    if (show || get().threadSearch.trim()) {
      void get().refreshThreads()
    }
  },
  }
}
