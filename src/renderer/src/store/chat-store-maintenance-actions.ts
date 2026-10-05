import { captureRewindResendPayload } from './rewind-resend-payload'
import { composerDraftKey, emptyComposerDraft } from './composer-drafts'
import { AttachmentPublicMetadata } from '../../../../packages/runtime/src/contracts/attachments.js'
import { projectAttachmentReferencesForPublicSurfaces } from '../agent/attachment-public'
import { getThreadBinding } from './chat-store-thread-binding'
import { withImageThreadNavigation } from '../write/image-thread-navigation'
import type { ChatBlock, ThreadGoal, ThreadGoalStatus, ThreadTodoList, ThreadTodoStatus } from '../agent/types'
import { getProvider } from '../agent/registry'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { applyTheme, applyUiFontScale } from '../lib/apply-theme'
import { confirmDialog } from '../lib/confirm-dialog'
import { formatWorkspacePickerError } from '../lib/format-workspace-picker-error'
import { formatRuntimeError } from '../lib/format-runtime-error'
import {
  getDefaultThreadTitle,
  shouldAutoTitleThread
} from '../lib/thread-title'
import { filterThreadsForSidebar } from '../lib/thread-sidebar-visibility'
import {
  enrichThreadsWithForkInfo,
  forgetThreadFork,
  hydrateThreadForkRegistry,
  markThreadFork,
  readThreadForkRegistry,
  saveThreadForkRegistry
} from '../lib/thread-fork-registry'
import {
  forgetThreadWorktree,
  readThreadWorktreeRegistry,
  saveThreadWorktreeRegistry
} from '../lib/thread-worktree-registry'
import { workspaceLabelFromPath } from '../lib/workspace-label'
import { isInternalTemporaryWorkspace, normalizeWorkspaceRoot } from '../lib/workspace-path'
import { buildClawRuntimePrompt } from '@shared/app-settings'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import type { ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'
import {
  activeClawChannel,
  compactCodeWorkspaceRoots,
  forgetCodeWorkspaceRoot,
  hydrateBlockModelLabels,
  isClawThread,
  optimisticUserModelLabel,
  readCodeWorkspaceRoots,
  readStoredComposerModel,
  rememberCodeWorkspaceRoots,
  rememberTurnModel
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
  WRITE_ASSISTANT_THREAD_TITLE,
  forgetWriteThread,
  hydrateWriteThreadRegistry,
  isWriteThreadId,
  pruneWriteThreadRegistry,
  readWriteThreadRegistry,
  saveWriteThreadRegistry,
  writeWorkspaceForThreadId
} from '../write/write-thread-registry'
import {
  clearBusyWatchdog,
  resetBusyRecoveryAttempts,
  scheduleStartupRuntimeProbe,
  stopTurnCompletionPoll
} from './chat-store-schedulers'
import {
  armBusyWatchdog,
  buildThreadEventSink,
  clearWatchedCompletionNotification,
  forkedMessageCount,
  forkedTurnCount,
  isCodeThread,
  latestThread,
  looksLikeActiveTurnError,
  readWriteWorkspaceRoots,
  quarantineTerminalTurnDraft,
  reconcileTerminalTurnFromThreadDetail,
  rememberPendingClawFeishuMirror,
  runtimeErrorDetail,
  runtimeStreamRecoveringMessage,
  shouldOpenSettingsForError,
  syncTurnCompletionPoll,
  watchTurnCompletionNotification
} from './chat-store-runtime'
import {
  extractPlanTodos,
  mergePlanTodosForRenderer,
  sameTodoWriteItems,
  threadTodoWriteItems
} from '../plan/plan-todo-sync'
import { isPendingActiveStreamTurnId } from '../thread/streaming/active-stream-store'

type SseAbortRef = { current: AbortController | null }

type StoreActionContext = {
  set: ChatStoreSet
  get: ChatStoreGet
  sseAbortRef: SseAbortRef
}

function releaseThreadWorktreeIfNeeded(threadId: string | null): void {
  if (!threadId || typeof window === 'undefined') return
  if (typeof window.analytix?.workspace?.releaseWorktree !== 'function') return
  const record = readThreadWorktreeRegistry().worktrees[threadId]
  if (!record) return
  void window.analytix.workspace
    .releaseWorktree({
      projectPath: record.projectPath,
      poolIndex: record.poolIndex
    })
    .catch(() => undefined)
  saveThreadWorktreeRegistry(forgetThreadWorktree(threadId))
}

function applyGoalSnapshot(
  set: ChatStoreSet,
  threadId: string,
  goal: ThreadGoal | null,
  updatedAt = new Date().toISOString()
): void {
  set((s) => ({
    activeThreadGoal: s.activeThreadId === threadId ? goal : s.activeThreadGoal,
    threads: s.threads.map((thread) =>
      thread.id === threadId
        ? { ...thread, goal, updatedAt: goal?.updatedAt ?? updatedAt }
        : thread
    )
  }))
}

function applyTodosSnapshot(
  set: ChatStoreSet,
  threadId: string,
  todos: ThreadTodoList | null,
  updatedAt = new Date().toISOString()
): void {
  set((s) => ({
    activeThreadTodos: s.activeThreadId === threadId ? todos : s.activeThreadTodos,
    threads: s.threads.map((thread) =>
      thread.id === threadId
        ? { ...thread, todos, updatedAt: todos?.updatedAt ?? updatedAt }
        : thread
    )
  }))
}

function quarantineInterruptedTurn(
  set: ChatStoreSet,
  get: ChatStoreGet,
  threadId: string,
  turnId: string,
  userBlockId: string | null
): void {
  resetBusyRecoveryAttempts()
  clearBusyWatchdog()
  quarantineTerminalTurnDraft({
    threadId,
    turnId,
    userBlockId,
    busy: false,
    set,
    get
  })
}

async function reconcileInterruptedTurn(
  set: ChatStoreSet,
  get: ChatStoreGet,
  threadId: string,
  turnId: string,
  userBlockId: string | null,
  isCurrent: () => boolean
): Promise<boolean> {
  return reconcileTerminalTurnFromThreadDetail({
    threadId,
    turnId,
    userBlockId,
    terminalStatus: 'aborted',
    isCurrent,
    loadThreadDetail: (id) => getProvider().getThreadDetail(id),
    set,
    get
  })
}

function blockRuntimeTurnId(block: ChatBlock): string {
  if (!('meta' in block)) return ''
  const turnId = block.meta?.turnId
  return typeof turnId === 'string' ? turnId.trim() : ''
}

function forkBlocksThroughTurn(blocks: ChatBlock[], turnId: string): ChatBlock[] {
  const targetTurnId = turnId.trim()
  if (!targetTurnId) return blocks
  const out: ChatBlock[] = []
  let foundTarget = false
  for (const block of blocks) {
    const currentTurnId = blockRuntimeTurnId(block)
    if (foundTarget && block.kind === 'user' && currentTurnId && currentTurnId !== targetTurnId) {
      break
    }
    out.push(block)
    if (currentTurnId === targetTurnId) {
      foundTarget = true
    }
  }
  return foundTarget ? out : blocks
}

export function createMaintenanceActions(
  { set, get, sseAbortRef }: StoreActionContext
): Pick<ChatState, 'renameActiveThread' | 'renameThread' | 'archiveThread' | 'compactActiveThread' | 'forkActiveThread' | 'forkThreadFromTurn' | 'setActiveThreadGoal' | 'setActiveThreadGoalStatus' | 'clearActiveThreadGoal' | 'setActiveThreadTodoStatus' | 'clearActiveThreadTodos' | 'syncPlanTodosFromMarkdown' | 'resumeSessionIntoThread' | 'deleteThread' | 'rewindAndResend' | 'rollbackWorkspaceToCheckpoint' | 'resolveApproval' | 'resolveUserInput' | 'interrupt'> {
  const binding = getThreadBinding(sseAbortRef)
  const resolvingGates = new Set<string>()
  let editInFlight = false
  type GateBlock = Extract<ChatBlock, { kind: 'approval' | 'user_input' }>
  const gateKey = (block: GateBlock): string =>
    `${block.kind}:${block.kind === 'approval' ? block.approvalId : block.requestId}`

  const resolveGate = async (
    block: GateBlock,
    invoke: () => Promise<void>,
    success: Partial<GateBlock>,
    supported: boolean
  ): Promise<void> => {
    const threadId = get().activeThreadId
    const generation = binding.generation
    const key = gateKey(block)
    if (!threadId || block.status !== 'pending' || resolvingGates.has(key)) return
    const belongs = (candidate: ChatBlock): candidate is GateBlock =>
      (candidate.kind === 'approval' || candidate.kind === 'user_input') &&
      candidate.id === block.id && gateKey(candidate) === key
    const current = (): boolean => get().activeThreadId === threadId && binding.generation === generation
    const pending = (): boolean => current() && get().blocks.some(candidate => belongs(candidate) && candidate.status === 'pending')
    resolvingGates.add(key)
    set(state => ({ pendingGateResolutionIds: { ...state.pendingGateResolutionIds, [key]: true } }))
    try {
      await invoke()
      if (!current()) return
      set(state => ({ blocks: state.blocks.map(candidate => {
        if (!belongs(candidate)) return candidate
        if (candidate.status === 'pending') return { ...candidate, ...success, errorMessage: undefined } as GateBlock
        // A public resolved event may arrive before its HTTP acknowledgement.
        // Attach only this successful answer's projection to the same outcome.
        if (candidate.kind === 'user_input' && candidate.status === 'submitted' && success.status === 'submitted') {
          return { ...candidate, ...success, errorMessage: undefined } as GateBlock
        }
        return candidate
      }) }))
      if (success.status === 'submitted' && current() && get().busy) armBusyWatchdog(set, get)
    } catch (error) {
      if (!pending()) return
      const message = formatRuntimeError(error)
      // An ambiguous POST failure is not permission to repeat a decision.
      // Reconcile just this gate from the existing public detail authority.
      let authoritative: GateBlock | undefined
      if (supported) {
        try {
          const detail = await getProvider().getThreadDetail(threadId)
          if (!pending()) return
          if ((detail.latestSeq ?? 0) >= (get().lastSeq ?? 0)) {
            authoritative = detail.blocks.find(belongs) as GateBlock | undefined
          }
        } catch {
          // Keep an explicit error when authority cannot be recovered.
        }
      }
      if (!pending()) return
      if (authoritative && authoritative.status !== 'pending' && authoritative.status !== 'error') {
        const settled = authoritative
        set(state => ({ blocks: state.blocks.map(candidate => belongs(candidate)
          ? { ...candidate, status: settled.status, errorMessage: undefined } as GateBlock
          : candidate) }))
        return
      }
      void window.analytix.logs.error(block.kind === 'approval' ? 'approval' : 'user-input', 'Failed to resolve pending request', {
        message, blockId: block.id
      }).catch(() => undefined)
      set(state => ({
        error: message,
        ...(shouldOpenSettingsForError(error) ? { route: 'settings' as const, settingsSection: 'agents' as const } : {}),
        blocks: state.blocks.map(candidate => belongs(candidate) && candidate.status === 'pending'
          ? { ...candidate, status: authoritative?.status === 'pending' ? 'pending' : 'error', errorMessage: message } as GateBlock
          : candidate)
      }))
    } finally {
      resolvingGates.delete(key)
      set(state => {
        const next = { ...state.pendingGateResolutionIds }
        delete next[key]
        return { pendingGateResolutionIds: next }
      })
    }
  }

  const forkActiveThreadWithOptions = async (options: { turnId?: string } = {}): Promise<void> => withImageThreadNavigation(undefined, async navigationCurrent => {
    const { activeThreadId, busy, blocks } = get()
    if (!activeThreadId) return
    if (busy) {
      set({ error: i18n.t('common:threadActionBusy') })
      return
    }
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const p = getProvider()
    if (typeof p.forkThread !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return
    }
    const turnId = options.turnId?.trim()
    try {
      const parentThread =
        get().threads.find((thread) => thread.id === activeThreadId) ?? {
          id: activeThreadId,
          title: activeThreadId.slice(0, 8)
        }
      const forked = turnId
        ? await p.forkThread(activeThreadId, { turnId })
        : await p.forkThread(activeThreadId)
      const fallbackBlocks = turnId ? forkBlocksThroughTurn(blocks, turnId) : blocks
      saveThreadForkRegistry(
        markThreadFork(
          forked.id,
          parentThread,
          {
            createdAt: forked.forkedAt ?? new Date().toISOString(),
            forkedFromMessageCount: forked.forkedFromMessageCount ?? forkedMessageCount(fallbackBlocks),
            forkedFromTurnCount: forked.forkedFromTurnCount ?? forkedTurnCount(fallbackBlocks)
          },
          readThreadForkRegistry()
        )
      )
      await get().refreshThreads()
      if (!navigationCurrent()) return
      await get().selectThread(forked.id)
      if (!navigationCurrent()) return
    } catch (e) {
      if (!navigationCurrent()) return
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
  })

  return {
  renameActiveThread: async (title) => {
    const { activeThreadId } = get()
    if (!activeThreadId) return
    await get().renameThread(activeThreadId, title)
  },

  renameThread: async (threadId, title) => {
    const targetId = threadId.trim()
    const nextTitle = title.trim()
    if (!targetId || !nextTitle) return
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const p = getProvider()
    try {
      await p.renameThread(targetId, nextTitle)
      set((s) => ({
        threads: s.threads.map((thread) =>
          thread.id === targetId ? { ...thread, title: nextTitle } : thread
        ),
        error: null
      }))
      await get().refreshThreads()
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
  },

  archiveThread: async (threadId, archived) => withImageThreadNavigation(undefined, async navigationCurrent => {
    const targetId = threadId.trim()
    if (!targetId) return
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const { activeThreadId } = get()
    const p = getProvider()
    const archivingActive = archived && activeThreadId === targetId
    try {
      if (typeof p.archiveThread === 'function') {
        await p.archiveThread(targetId, archived)
      } else if (archived) {
        await p.deleteThread(targetId)
      } else {
        throw new Error(i18n.t('common:runtimeFeatureUnsupported'))
      }
      if (archivingActive && navigationCurrent()) {
        binding.generation += 1
        sseAbortRef.current?.abort()
        sseAbortRef.current = null
        clearBusyWatchdog()
      }
      set((s) => {
        const w = { ...s.watchTurnCompletion }
        const u = { ...s.unreadThreadIds }
        if (archived) {
          delete w[targetId]
          delete u[targetId]
          clearWatchedCompletionNotification(targetId)
        }
        return {
          threads: s.threads.map((thread) =>
            thread.id === targetId ? { ...thread, archived } : thread
          ),
          watchTurnCompletion: w,
          unreadThreadIds: u,
          ...(archivingActive && navigationCurrent() ? clearedThreadSelection() : {}),
          error: null
        }
      })
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
    }
  }),

  compactActiveThread: async (reason) => {
    const { activeThreadId, busy } = get()
    if (!activeThreadId) return
    if (busy) {
      set({ error: i18n.t('common:threadActionBusy') })
      return
    }
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const p = getProvider()
    if (typeof p.compactThread !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return
    }
    try {
      await p.compactThread(activeThreadId, reason)
      await get().refreshThreads()
      await get().selectThread(activeThreadId)
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
  },

  forkActiveThread: async () => {
    await forkActiveThreadWithOptions()
  },

  forkThreadFromTurn: async (turnId) => {
    const targetTurnId = turnId.trim()
    if (!targetTurnId) return
    await forkActiveThreadWithOptions({ turnId: targetTurnId })
  },

  setActiveThreadGoal: async (objective, options) => {
    const trimmed = objective.trim()
    if (!trimmed) return false
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    let { activeThreadId } = get()
    if (!activeThreadId) {
      await get().createThread({ materialize: true })
      activeThreadId = get().activeThreadId
    }
    if (!activeThreadId) return false
    const p = getProvider()
    if (typeof p.setThreadGoal !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return false
    }
    try {
      const goal = await p.setThreadGoal(activeThreadId, {
        objective: trimmed,
        status: 'active',
        ...(options?.research
          ? {
              research: {
                enabled: true as const,
                ...(options.requirements?.length ? { requirements: options.requirements } : {})
              }
            }
          : {})
      })
      applyGoalSnapshot(set, activeThreadId, goal)
      await get().refreshThreads()
      return get().sendMessage(goal.objective, 'agent', {
        displayText: i18n.t('common:goalUserMessage', { objective: goal.objective })
      })
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    }
  },

  setActiveThreadGoalStatus: async (status: ThreadGoalStatus) => {
    const { activeThreadId } = get()
    if (!activeThreadId) return false
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    const p = getProvider()
    if (typeof p.setThreadGoal !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return false
    }
    try {
      const goal = await p.setThreadGoal(activeThreadId, { status })
      applyGoalSnapshot(set, activeThreadId, goal)
      await get().refreshThreads()
      return true
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    }
  },

  clearActiveThreadGoal: async () => {
    const { activeThreadId } = get()
    if (!activeThreadId) return false
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    const p = getProvider()
    if (typeof p.clearThreadGoal !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return false
    }
    try {
      const cleared = await p.clearThreadGoal(activeThreadId)
      if (cleared) {
        applyGoalSnapshot(set, activeThreadId, null)
      }
      await get().refreshThreads()
      return cleared
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    }
  },

  setActiveThreadTodoStatus: async (todoId: string, status: ThreadTodoStatus) => {
    const { activeThreadId, activeThreadTodos } = get()
    if (!activeThreadId || !activeThreadTodos) return false
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    const p = getProvider()
    if (typeof p.setThreadTodos !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return false
    }
    const target = activeThreadTodos.items.find((item) => item.id === todoId)
    if (!target) return false
    if (target.status === status) return true
    if (
      status === 'failed' || status === 'canceled' ||
      target.status === 'completed' || target.status === 'failed' || target.status === 'canceled'
    ) {
      set({ error: i18n.t('common:todoExplicitTransitionRequired') })
      return false
    }
    try {
      const nextItems = activeThreadTodos.items.map((item) => {
        if (item.id === todoId) return { ...item, status }
        if (status === 'in_progress' && item.status === 'in_progress') {
          return { ...item, status: 'pending' as const }
        }
        return item
      })
      const todos = await p.setThreadTodos(activeThreadId, threadTodoWriteItems({
        ...activeThreadTodos,
        items: nextItems
      }))
      applyTodosSnapshot(set, activeThreadId, todos)
      return true
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    }
  },

  clearActiveThreadTodos: async () => {
    const { activeThreadId, activeThreadTodos } = get()
    if (!activeThreadId) return false
    if (activeThreadTodos?.items.some((item) => item.status === 'failed' || item.status === 'canceled')) {
      set({ error: i18n.t('common:todoTerminalAuditRetained') })
      return false
    }
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    const p = getProvider()
    if (typeof p.clearThreadTodos !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return false
    }
    try {
      const cleared = await p.clearThreadTodos(activeThreadId)
      if (cleared) applyTodosSnapshot(set, activeThreadId, null)
      return cleared
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    }
  },

  syncPlanTodosFromMarkdown: async (plan, markdown) => {
    const { activeThreadId, activeThreadTodos } = get()
    if (!activeThreadId) return false
    if (get().runtimeConnection !== 'ready') return false
    const p = getProvider()
    if (typeof p.setThreadTodos !== 'function') return false
    const now = new Date().toISOString()
    const planItems = extractPlanTodos({
      markdown,
      threadId: activeThreadId,
      planId: plan.id,
      relativePath: plan.relativePath,
      now
    })
    const nextTodos = mergePlanTodosForRenderer({
      threadId: activeThreadId,
      existing: activeThreadTodos,
      planItems,
      now
    })
    const currentWriteItems = activeThreadTodos ? threadTodoWriteItems(activeThreadTodos) : []
    const nextWriteItems = threadTodoWriteItems(nextTodos)
    if (sameTodoWriteItems(currentWriteItems, nextWriteItems)) return true
    try {
      const todos = await p.setThreadTodos(activeThreadId, nextWriteItems)
      applyTodosSnapshot(set, activeThreadId, todos)
      return true
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    }
  },

  resumeSessionIntoThread: async (sessionId, options) => withImageThreadNavigation(null, async navigationCurrent => {
    const id = sessionId.trim()
    if (!id) return null
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return null
    }
    const p = getProvider()
    if (typeof p.resumeSession !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return null
    }
    try {
      const result = await p.resumeSession(id, options)
      await get().refreshThreads()
      if (!navigationCurrent()) return null
      await get().selectThread(result.threadId)
      if (!navigationCurrent()) return null
      return result.threadId
    } catch (e) {
      if (!navigationCurrent()) return null
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return null
    }
  }),

  deleteThread: async (threadId) => withImageThreadNavigation(undefined, async navigationCurrent => {
    const targetId = threadId.trim()
    if (!targetId) return
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const { activeThreadId } = get()
    const p = getProvider()
    const deletingActive = activeThreadId === targetId
    const wtRecord = readThreadWorktreeRegistry().worktrees[targetId]
    if (wtRecord && typeof window.analytix?.workspace?.releaseWorktree === 'function') {
      try {
        await window.analytix.workspace.releaseWorktree({
          projectPath: wtRecord.projectPath,
          poolIndex: wtRecord.poolIndex
        })
        saveThreadWorktreeRegistry(forgetThreadWorktree(targetId))
        if (!navigationCurrent()) return
      } catch {
        if (!navigationCurrent()) return
        /* best-effort; the slot can be reclaimed later from Settings */
      }
    }
    try {
      await p.deleteThread(targetId)
      saveWriteThreadRegistry(forgetWriteThread(targetId))
      saveThreadForkRegistry(forgetThreadFork(targetId))
      if (wtRecord) saveThreadWorktreeRegistry(forgetThreadWorktree(targetId))
      if (deletingActive && navigationCurrent()) {
        binding.generation += 1
        sseAbortRef.current?.abort()
        sseAbortRef.current = null
        clearBusyWatchdog()
      }
      set((s) => {
        const w = { ...s.watchTurnCompletion }
        delete w[targetId]
        clearWatchedCompletionNotification(targetId)
        const u = { ...s.unreadThreadIds }
        delete u[targetId]
        return {
          threads: s.threads.filter((thread) => thread.id !== targetId),
          watchTurnCompletion: w,
          unreadThreadIds: u,
          ...(deletingActive && navigationCurrent() ? clearedThreadSelection() : {}),
          error: null
        }
      })
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
    }
  }),

  rewindAndResend: async (userBlockId, newText) => {
    const trimmed = newText.trim()
    if (!trimmed || editInFlight) return
    const state = get()
    if (state.busy) { set({ error: i18n.t('common:rewindBusyError') }); return }
    if (state.runtimeConnection !== 'ready') { set({ error: i18n.t('common:runtimeActionNeedsConnection') }); return }
    const idx = state.blocks.findIndex(block => block.id === userBlockId && block.kind === 'user')
    if (idx < 0) return
    const target = state.blocks[idx]
    if (target.kind !== 'user') return
    // File-context prompts have a separate visible text and remain uneditable.
    if (typeof target.meta?.displayText === 'string' && target.meta.displayText.trim()) {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') }); return
    }
    const turnId = typeof target.meta?.turnId === 'string' ? target.meta.turnId : ''
    const activeThreadId = state.activeThreadId
    const provider = getProvider()
    const activeThread = state.threads.find(thread => thread.id === activeThreadId)
    if (!activeThreadId || !turnId || typeof provider.rewindThread !== 'function' || activeThread?.historyAuthority === 'case_boundary_only_v1') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') }); return
    }
    const workspace = normalizeWorkspaceRoot(activeThread?.workspace) || normalizeWorkspaceRoot(state.workspaceRoot)
    let generation = binding.generation
    let submissionGeneration = binding.submissionGeneration
    let rewindIssued = false
    let sending = false
    const isCurrent = (): boolean => get().activeThreadId === activeThreadId && get().runtimeConnection === 'ready' &&
      binding.generation === generation && binding.submissionGeneration === submissionGeneration && (sending || !get().busy) &&
      get().threads.find(thread => thread.id === activeThreadId)?.historyAuthority !== 'case_boundary_only_v1' &&
      (rewindIssued || get().blocks.some(block => block.id === userBlockId && block.kind === 'user' && block.meta?.turnId === turnId)) &&
      (normalizeWorkspaceRoot(get().threads.find(thread => thread.id === activeThreadId)?.workspace) || normalizeWorkspaceRoot(get().workspaceRoot)) === workspace
    let payload: ReturnType<typeof captureRewindResendPayload>
    try { payload = captureRewindResendPayload(target.meta) } catch {
      set({ error: i18n.t('common:rewindResendInvalidPayload') }); return
    }
    const draftKey = composerDraftKey(workspace, activeThreadId)
    const originalDraft = state.composerDrafts?.[draftKey] ?? emptyComposerDraft
    const checkpointId = typeof target.meta?.workspaceCheckpointId === 'string' ? target.meta.workspaceCheckpointId.trim() : ''
    editInFlight = true
    let rewound = false
    let sent = false
    try {
      // This read checks current Go authority before either destructive operation.
      // StartTurn will still plan fresh context-bound use; no old receipt is reused.
      if (payload.attachmentIds.length) {
        if (!workspace || typeof provider.getAttachmentMetadata !== 'function') {
          set({ error: i18n.t('common:rewindResendAttachmentUnavailable') }); return
        }
        const captured = new Map(payload.attachments.map(reference => [reference.id, reference]))
        const attachments = [] as typeof payload.attachments
        for (const id of payload.attachmentIds) {
          let metadata
          try { metadata = AttachmentPublicMetadata.parse(await provider.getAttachmentMetadata(id, { threadId: activeThreadId, workspace })) } catch {
            if (isCurrent()) set({ error: i18n.t('common:rewindResendAttachmentUnavailable') })
            return
          }
          if (!isCurrent()) return
          if (metadata.id !== id) { set({ error: i18n.t('common:rewindResendAttachmentUnavailable') }); return }
          attachments.push(captured.get(id) ?? projectAttachmentReferencesForPublicSurfaces([metadata])[0])
        }
        payload = { ...payload, attachments }
      }
      if (!isCurrent()) return
      if (checkpointId) {
        let restored
        try { restored = await window.analytix.workspace.restoreGitCheckpoint({ checkpointId }) } catch {
          if (isCurrent()) set({ error: i18n.t('common:rewindResendFailed') })
          return
        }
        if (!isCurrent()) return
        if (!restored.ok) { set({ error: i18n.t('common:rewindResendFailed') }); return }
      }
      rewindIssued = true
      try { await provider.rewindThread(activeThreadId, turnId); rewound = true } catch {
        if (isCurrent()) set({ error: i18n.t('common:rewindResendFailed') })
        return
      }
      if (!isCurrent()) return
      // Our rewind SSE may already have removed the target. Mirror the captured
      // cut only after its successful HTTP reply, without requiring it to remain.
      const turnStartedAtByUserId = { ...get().turnStartedAtByUserId }
      const turnDurationByUserId = { ...get().turnDurationByUserId }
      for (const block of state.blocks.slice(idx)) {
        if (block.kind === 'user') { delete turnStartedAtByUserId[block.id]; delete turnDurationByUserId[block.id] }
      }
      generation = ++binding.generation
      sseAbortRef.current?.abort()
      sseAbortRef.current = null
      clearBusyWatchdog()
      set({ blocks: state.blocks.slice(0, idx), liveAssistant: '', currentTurnId: null, currentTurnUserId: null,
        turnStartedAtByUserId, turnDurationByUserId, queuedMessages: [], queuedMessagesPausedReason: null, error: null })
      sending = true
      try {
        const pending = get().sendMessage(trimmed, undefined, { ...payload,
          submissionGuard: { isCurrent, validateBeforeSend: async () => isCurrent() } })
        // An existing-thread send binds its new subscription synchronously before
        // its first await. Hand off exactly that generation; later changes revoke it.
        generation = binding.generation
        submissionGeneration = binding.submissionGeneration
        sent = await pending
      } catch { /* Preserve a retryable draft; never expose an unsafe error body. */ }
      if (!sent && isCurrent()) set({ error: i18n.t('common:rewindResendFailed') })
    } finally {
      // A successful rewind is irreversible locally. Preserve the original-key
      // draft even when navigation or lost-ACK recovery replaced the subscription.
      // Object identity protects newer text, attachments and file references.
      const owner = get().threads.find(thread => thread.id === activeThreadId)
      if (rewound && !sent && owner && owner.historyAuthority !== 'case_boundary_only_v1' && normalizeWorkspaceRoot(owner.workspace) === workspace) {
        get().updateComposerDraft?.(draftKey, current => current !== originalDraft ? current : ({
          ...current, input: trimmed, inputRevision: current.inputRevision + 1, attachments: payload.attachments,
          fileReferences: payload.fileReferences.map(reference => ({ path: reference.path, relativePath: reference.relativePath,
            name: reference.name, ...(reference.kind ? { type: reference.kind } : {}) }))
        }))
      }
      editInFlight = false
    }
  },

  rollbackWorkspaceToCheckpoint: async (checkpointId) => {
    const targetCheckpointId = checkpointId.trim()
    if (!targetCheckpointId) {
      set({ error: i18n.t('common:rollbackWorkspaceMissingCheckpoint') })
      return
    }
    if (get().busy) {
      set({ error: i18n.t('common:rollbackWorkspaceBusyError') })
      return
    }
    const confirmed = await confirmDialog(
      i18n.t('common:rollbackWorkspaceConfirm'),
      i18n.t('common:rollbackWorkspaceConfirmDetail')
    )
    if (!confirmed) return
    if (get().busy) {
      set({ error: i18n.t('common:rollbackWorkspaceBusyError') })
      return
    }
    let restored = await window.analytix.workspace.restoreGitCheckpoint({
      checkpointId: targetCheckpointId
    }).catch((error) => ({
      ok: false as const,
      reason: 'error' as const,
      message: error instanceof Error ? error.message : String(error)
    }))
    if (!restored.ok && restored.reason === 'partial') {
      const partialConfirmed = await confirmDialog(
        i18n.t('common:rollbackWorkspacePartialConfirm'),
        restored.message
      )
      if (!partialConfirmed) return
      restored = await window.analytix.workspace.restoreGitCheckpoint({
        checkpointId: targetCheckpointId,
        allowPartialRestore: true
      }).catch((error) => ({
        ok: false as const,
        reason: 'error' as const,
        message: error instanceof Error ? error.message : String(error)
      }))
    }
    if (!restored.ok) {
      set({ error: restored.message })
      return
    }
    set({ error: null })
  },

  resolveApproval: async (blockId, decision) => {
    const block = get().blocks.find(candidate => candidate.id === blockId)
    if (!block || block.kind !== 'approval' || block.status !== 'pending') return
    const provider = getProvider()
    const supported = typeof provider.submitApprovalDecision === 'function'
    await resolveGate(block, async () => {
      if (!supported) throw new Error(i18n.t('common:runtimeFeatureUnsupported'))
      await provider.submitApprovalDecision!(block.approvalId, decision === 'allow' ? 'allow' : 'deny', false)
    }, { status: decision === 'allow' ? 'allowed' : 'denied' }, supported)
  },

  resolveUserInput: async (blockId, action) => {
    const block = get().blocks.find(candidate => candidate.id === blockId)
    if (!block || block.kind !== 'user_input' || block.status !== 'pending') return
    const provider = getProvider()
    const supported = action.kind === 'submit'
      ? typeof provider.submitUserInputResponse === 'function'
      : typeof provider.cancelUserInput === 'function'
    const publicAnswers = action.kind === 'submit' ? action.answers.map(answer => ({
      ...answer, label: projectOrdinaryPublicText(answer.label), value: projectOrdinaryPublicText(answer.value)
    })) : undefined
    await resolveGate(block, async () => {
      if (!supported) throw new Error(i18n.t('common:runtimeUserInputUnsupported'))
      if (action.kind === 'submit') await provider.submitUserInputResponse!(block.requestId, action.answers)
      else await provider.cancelUserInput!(block.requestId)
    }, action.kind === 'submit' ? { status: 'submitted', answers: publicAnswers } : { status: 'cancelled' }, supported)
  },

  interrupt: async (options) => {
    const { activeThreadId, currentTurnId, currentTurnUserId } = get()
    if (!activeThreadId || !currentTurnId) return
    const pendingRuntimeTurn = isPendingActiveStreamTurnId(currentTurnId)
    const p = getProvider()
    // Settle the UI before notifying the runtime: a slow or hung
    // interruptTurn must not keep the stop button unresponsive. The event
    // stream is aborted first because onDeltas/onTool flip `busy` back on
    // while the backend turn is still streaming.
    binding.generation += 1
    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    quarantineInterruptedTurn(set, get, activeThreadId, currentTurnId, currentTurnUserId)
    if (get().queuedMessages.length > 0) {
      set({ queuedMessagesPausedReason: 'interrupted' })
    }
    const interruptGeneration = binding.generation
    const ownsInterrupt = () => get().activeThreadId === activeThreadId && binding.generation === interruptGeneration
    try {
      if (!pendingRuntimeTurn) {
        await p.interruptTurn(activeThreadId, currentTurnId, { discard: options?.discard === true })
      }
    } catch (e) {
      if (!ownsInterrupt()) return
      const msg = formatRuntimeError(e)
      void window.analytix.logs.error('interrupt', 'Failed to interrupt turn', { message: msg }).catch(() => undefined)
      set({
        error: msg,
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
    if (ownsInterrupt() && sseAbortRef.current === null) {
      const reconciled = await reconcileInterruptedTurn(
        set,
        get,
        activeThreadId,
        currentTurnId,
        currentTurnUserId,
        ownsInterrupt
      )
      if (reconciled && ownsInterrupt()) {
        void get().refreshThreads()
        if (get().queuedMessages.length === 0) {
          releaseThreadWorktreeIfNeeded(activeThreadId)
        }
        void get().drainQueuedMessages()
      }
    }
  }
  }
}
