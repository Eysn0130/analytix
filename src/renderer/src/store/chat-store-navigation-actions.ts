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
import { filterRevokedPublicProjectionThreads } from '../lib/public-projection-revocation'
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
  resolveLocalProviderReadiness
} from '../account/local-provider-readiness'
import type { ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'
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
  reconcileOptimisticUserBlock,
  threadSnapshotLooksRunning,
  threadBelongsToWorkspace
} from './chat-store-runtime-helpers'
import {
  forgetWriteThread,
  hydrateWriteThreadRegistry,
  isWriteThreadId,
  pruneWriteThreadRegistry,
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
let searchRefreshTimer: ReturnType<typeof setTimeout> | null = null
const defaultThreadRefreshLimit = 50
const archivedThreadRefreshLimit = 200
const caseProjectRefreshLimit = 100
const caseProjectThreadLoadLimit = 200
const runtimeRecoveryDelaysMs = [250, 1000, 3000] as const

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
  const forkRegistry = hydrateThreadForkRegistry(
    replaceThreads ? sidebarThreads : mergeThreadsById(get().threads, sidebarThreads),
    readThreadForkRegistry()
  )
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
      get().threads.filter((thread) => !shouldHideThreadFromSidebarByLineage(thread)),
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
  const writeRegistry = hydrateWriteThreadRegistry(
    displayThreads,
    writeWorkspaceRoots,
    pruneWriteThreadRegistry(displayThreads, readWriteThreadRegistry())
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
      if (v && validIds.has(k)) {
        w[k] = true
      } else {
        clearWatchedCompletionNotification(k)
      }
    }
    const u: Record<string, boolean> = {}
    for (const [k, v] of Object.entries(s.unreadThreadIds)) {
      if (v && validIds.has(k)) u[k] = true
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
  const caseProjectLoadOwners = new Map<string, symbol>()
  let caseProjectIndexRefreshTimer: ReturnType<typeof setTimeout> | null = null
  let caseProjectRefreshGeneration = 0
  let runtimeProbeGeneration = 0
  let activeUserRuntimeProbeGeneration = 0
  let runtimeRecoveryTimer: ReturnType<typeof setTimeout> | null = null
  let runtimeRecoveryAttempt = 0
  let runtimeRecoveryEpoch = 0

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
      runtimeRecoveryTimer != null ||
      runtimeRecoveryAttempt >= runtimeRecoveryDelaysMs.length
    ) {
      return
    }
    const epoch = runtimeRecoveryEpoch
    const delay = runtimeRecoveryDelaysMs[runtimeRecoveryAttempt]
    runtimeRecoveryTimer = setTimeout(() => {
      runtimeRecoveryTimer = null
      if (
        epoch !== runtimeRecoveryEpoch ||
        get().runtimeConnection !== 'offline' ||
        activeUserRuntimeProbeGeneration !== 0
      ) {
        return
      }
      runtimeRecoveryAttempt += 1
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
    if (mode === 'user') {
      clearRuntimeRecovery()
      activeUserRuntimeProbeGeneration = probeGeneration
      // A later ready state cannot revive work from before this connection check.
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
        throw new Error(providerReadiness.kind === 'recovery'
          ? providerReadiness.message : 'Choose a Provider in Settings to finish setup.')
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
      const msg = formatRuntimeError(e)
      const detail = runtimeErrorDetail(e)
      const needsSettings = shouldOpenSettingsForError(e)
      if (mode === 'user') {
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
      } else {
        if (prev === 'ready') {
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
        applyTheme(settings.theme)
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
          error: needsProviderRecovery ? readiness.message : null,
          runtimeErrorDetail: null
        })
        if (readiness.kind !== 'ready') return
        const initialPick = get().composerPickList
        const fromStorage = readStoredComposerModel(initialPick)
        if (fromStorage) {
          set({ composerModel: fromStorage })
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
    const { activeThreadId } = get()
    const p = getProvider()
    const workspaceThreads = get().threads.filter((thread) =>
      threadBelongsToWorkspace(thread, normalizedPath)
    )
    const deletingActive = workspaceThreads.some((th) => th.id === activeThreadId)
    if (deletingActive) {
      sseAbortRef.current?.abort()
      sseAbortRef.current = null
      clearBusyWatchdog()
    }
    try {
      const removeIds = new Set<string>()
      for (const th of workspaceThreads) {
        await p.deleteThread(th.id)
        removeIds.add(th.id)
        if (!navigationCurrent()) break
      }
      const codeWorkspaceRoots = removeIds.size === workspaceThreads.length
        ? forgetCodeWorkspaceRoot(get().codeWorkspaceRoots, normalizedPath) : get().codeWorkspaceRoots
      set((s) => {
        const w = { ...s.watchTurnCompletion }
        const u = { ...s.unreadThreadIds }
        for (const tid of removeIds) {
          delete w[tid]
          delete u[tid]
          clearWatchedCompletionNotification(tid)
        }
        return {
          threads: s.threads.filter(
            (thread) => !removeIds.has(thread.id)
          ),
          codeWorkspaceRoots,
          watchTurnCompletion: w,
          unreadThreadIds: u,
          ...(deletingActive && navigationCurrent() ? clearedThreadSelection() : {}),
          error: null
        }
      })
      // If the deleted workspace is the current workspaceRoot, clear it.
      if (navigationCurrent() && normalizeWorkspaceRoot(get().workspaceRoot) === normalizedPath) {
        try {
          if (typeof window.analytix?.settings?.setSettings === 'function') {
            const next = await rendererRuntimeClient.setSettings({ workspaceRoot: '' })
            if (!navigationCurrent()) return
            set({
              workspaceRoot: normalizeWorkspaceRoot(next.workspaceRoot),
              codeWorkspaceRoots: get().codeWorkspaceRoots,
              workspaceLabel: workspaceLabelFromPath('')
            })
          }
        } catch {
          if (!navigationCurrent()) return
          /* silently keep workspaceRoot if settings clear fails */
        }
      }
      await get().refreshThreads()
      if (!navigationCurrent()) return
    } catch (e) {
      if (!navigationCurrent()) return
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      await get().refreshThreads()
      if (!navigationCurrent()) return
    }
	  }),

  refreshCaseProjects: async (options = {}) => {
    if (options.isCurrent?.() === false || get().runtimeConnection !== 'ready') return
    const p = getProvider()
    if (typeof p.listCaseProjects !== 'function') return
    // A newer admitted read owns the existing timer slot, even while its I/O is
    // pending. An obsolete timer/result must neither block nor replace its chain.
    invalidateCaseProjectRefresh()
    const generation = caseProjectRefreshGeneration
    const isCurrent = () => generation === caseProjectRefreshGeneration &&
      options.isCurrent?.() !== false && get().runtimeConnection === 'ready'
    try {
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
	    if (!projectId || get().runtimeConnection !== 'ready') return
	    const p = getProvider()
	    if (typeof p.listCaseProjectThreads !== 'function') return
	    if (!options.force && get().caseProjectThreadsById[projectId]) return
	    if (get().caseProjectLoadingById[projectId]) return
	    const loadOwner = Symbol('case-project-load')
      caseProjectLoadOwners.set(projectId, loadOwner)
      const isCurrent = () => caseProjectLoadOwners.get(projectId) === loadOwner && options.isCurrent?.() !== false
      set((s) => ({
	      caseProjectLoadingById: { ...s.caseProjectLoadingById, [projectId]: true },
	      caseProjectErrorsById: { ...s.caseProjectErrorsById, [projectId]: null }
	    }))
	    try {
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
      const isCurrent = () => options.isCurrent?.() !== false
      if (!isCurrent()) return
	    if (get().runtimeConnection !== 'ready') return
	    const p = getProvider()
	    try {
	      const searchQuery = get().threadSearch.trim()
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
        await get().refreshCaseProjects(options)
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
      } catch {
        if (!isCurrent()) return
        rawThreads = await p.listThreads()
      }
      if (!isCurrent()) return
      const threads = rawThreads.map((thread) => ({
        ...thread,
        workspace: normalizeWorkspaceRoot(thread.workspace)
      }))
	      const sidebarThreads = await filterThreadsForSidebar(threads, p)
	      await reconcileThreadListSideEffects(
	        { set, get, sseAbortRef },
	        {
	          provider: p,
	          rawThreads: threads,
	          sidebarThreads,
	          replaceThreads: true,
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
	    set({ threadSearch: query })
	    if (searchRefreshTimer != null) {
	      clearTimeout(searchRefreshTimer)
	      searchRefreshTimer = null
	    }
	    if (get().runtimeConnection !== 'ready') return
	    searchRefreshTimer = setTimeout(() => {
	      searchRefreshTimer = null
	      void get().refreshThreads()
	    }, query.trim() ? 250 : 0)
	  },

  setShowArchivedThreads: (show) => {
    set({ showArchivedThreads: show })
    if (show && get().runtimeConnection === 'ready') {
      void get().refreshThreads()
    }
  },
  }
}
