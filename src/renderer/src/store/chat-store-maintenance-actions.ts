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
  userBlockId: string | null
): Promise<boolean> {
  return reconcileTerminalTurnFromThreadDetail({
    threadId,
    turnId,
    userBlockId,
    terminalStatus: 'aborted',
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
  const forkActiveThreadWithOptions = async (options: { turnId?: string } = {}): Promise<void> => {
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
      await get().selectThread(forked.id)
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
  }

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

  archiveThread: async (threadId, archived) => {
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
      if (archivingActive) {
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
          ...(archivingActive ? clearedThreadSelection() : {}),
          error: null
        }
      })
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

  resumeSessionIntoThread: async (sessionId, options) => {
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
      await get().selectThread(result.threadId)
      return result.threadId
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return null
    }
  },

  deleteThread: async (threadId) => {
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
      } catch {
        /* best-effort; the slot can be reclaimed later from Settings */
      }
    }
    try {
      await p.deleteThread(targetId)
      saveWriteThreadRegistry(forgetWriteThread(targetId))
      saveThreadForkRegistry(forgetThreadFork(targetId))
      if (wtRecord) saveThreadWorktreeRegistry(forgetThreadWorktree(targetId))
      if (deletingActive) {
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
          ...(deletingActive ? clearedThreadSelection() : {}),
          error: null
        }
      })
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

  rewindAndResend: async (userBlockId, newText) => {
    const trimmed = newText.trim()
    if (!trimmed) return
    const state = get()
    if (state.busy) {
      set({ error: i18n.t('common:rewindBusyError') })
      return
    }
    const idx = state.blocks.findIndex((b) => b.id === userBlockId && b.kind === 'user')
    if (idx < 0) return
    const target = state.blocks[idx]
    const turnId = target?.kind === 'user' && typeof target.meta?.turnId === 'string'
      ? target.meta.turnId
      : ''
    const activeThreadId = state.activeThreadId
    const provider = getProvider()
    if (!activeThreadId || !turnId || typeof provider.rewindThread !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return
    }
    const activeThread = state.threads.find((thread) => thread.id === activeThreadId)
    if (activeThread?.historyAuthority === 'case_boundary_only_v1') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return
    }
    const checkpointId =
      target?.kind === 'user' && typeof target.meta?.workspaceCheckpointId === 'string'
        ? target.meta.workspaceCheckpointId.trim()
        : ''
    if (checkpointId) {
      const restored = await window.analytix.workspace.restoreGitCheckpoint({ checkpointId }).catch((error) => ({
        ok: false as const,
        reason: 'error' as const,
        message: error instanceof Error ? error.message : String(error)
      }))
      if (!restored.ok) {
        set({ error: restored.message })
        return
      }
    }

    try {
      await provider.rewindThread(activeThreadId, turnId)
    } catch (e) {
      set({ error: formatRuntimeError(e) })
      return
    }

    // The runtime has now removed the target turn and everything after it
    // from canonical history. Mirror that cut locally before sending the
    // edited prompt as a fresh turn.
    const trimmedBlocks = state.blocks.slice(0, idx)

    const droppedUserIds = state.blocks
      .slice(idx)
      .filter((b) => b.kind === 'user')
      .map((b) => b.id)
    const turnStartedAtByUserId = { ...state.turnStartedAtByUserId }
    const turnDurationByUserId = { ...state.turnDurationByUserId }
    for (const id of droppedUserIds) {
      delete turnStartedAtByUserId[id]
      delete turnDurationByUserId[id]
    }

    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    clearBusyWatchdog()

    set({
      blocks: trimmedBlocks,
      liveAssistant: '',
      currentTurnId: null,
      currentTurnUserId: null,
      turnStartedAtByUserId,
      turnDurationByUserId,
      queuedMessages: [],
      queuedMessagesPausedReason: null,
      error: null
    })

    await get().sendMessage(trimmed)
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
    const { blocks } = get()
    const block = blocks.find((b) => b.id === blockId)
    if (!block || block.kind !== 'approval' || block.status !== 'pending') return
    const p = getProvider()
    if (typeof p.submitApprovalDecision !== 'function') {
      set({ error: i18n.t('common:runtimeFeatureUnsupported') })
      return
    }
    try {
      await p.submitApprovalDecision(
        block.approvalId,
        decision === 'allow' ? 'allow' : 'deny',
        false
      )
      set((s) => ({
        blocks: s.blocks.map((b) =>
          b.id === blockId && b.kind === 'approval'
            ? { ...b, status: decision === 'allow' ? ('allowed' as const) : ('denied' as const) }
            : b
        )
      }))
    } catch (e) {
      const msg = formatRuntimeError(e)
      void window.analytix.logs.error('approval', 'Failed to submit approval decision', {
        message: msg,
        blockId
      }).catch(() => undefined)
      set((s) => ({
        error: msg,
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {}),
        blocks: s.blocks.map((b) =>
          b.id === blockId && b.kind === 'approval'
            ? { ...b, status: 'error' as const, errorMessage: msg }
            : b
        )
      }))
    }
  },

  resolveUserInput: async (blockId, action) => {
    const { blocks } = get()
    const block = blocks.find((b) => b.id === blockId)
    if (!block || block.kind !== 'user_input' || block.status !== 'pending') return
    const p = getProvider()
    try {
      if (action.kind === 'submit') {
        const publicAnswers = action.answers.map((answer) => ({
          ...answer,
          label: projectOrdinaryPublicText(answer.label),
          value: projectOrdinaryPublicText(answer.value)
        }))
        if (typeof p.submitUserInputResponse !== 'function') {
          throw new Error(i18n.t('common:runtimeUserInputUnsupported'))
        }
        await p.submitUserInputResponse(block.requestId, action.answers)
        if (get().busy) armBusyWatchdog(set, get)
        set((s) => ({
          blocks: s.blocks.map((b) =>
            b.id === blockId && b.kind === 'user_input'
              ? { ...b, status: 'submitted' as const, answers: publicAnswers }
              : b
          )
        }))
        return
      }

      if (typeof p.cancelUserInput !== 'function') {
        throw new Error(i18n.t('common:runtimeUserInputUnsupported'))
      }
      await p.cancelUserInput(block.requestId)
      set((s) => ({
        blocks: s.blocks.map((b) =>
          b.id === blockId && b.kind === 'user_input'
            ? { ...b, status: 'cancelled' as const }
            : b
        )
      }))
    } catch (e) {
      const msg = formatRuntimeError(e)
      void window.analytix.logs.error('user-input', 'Failed to resolve user input', {
        message: msg,
        blockId
      }).catch(() => undefined)
      set((s) => ({
        error: msg,
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {}),
        blocks: s.blocks.map((b) =>
          b.id === blockId && b.kind === 'user_input'
            ? { ...b, status: 'error' as const, errorMessage: msg }
            : b
        )
      }))
    }
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
    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    quarantineInterruptedTurn(set, get, activeThreadId, currentTurnId, currentTurnUserId)
    if (get().queuedMessages.length > 0) {
      set({ queuedMessagesPausedReason: 'interrupted' })
    }
    try {
      if (!pendingRuntimeTurn) {
        await p.interruptTurn(activeThreadId, currentTurnId, { discard: options?.discard === true })
      }
    } catch (e) {
      const msg = formatRuntimeError(e)
      void window.analytix.logs.error('interrupt', 'Failed to interrupt turn', { message: msg }).catch(() => undefined)
      set({
        error: msg,
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
    if (get().activeThreadId === activeThreadId && sseAbortRef.current === null) {
      const reconciled = await reconcileInterruptedTurn(
        set,
        get,
        activeThreadId,
        currentTurnId,
        currentTurnUserId
      )
      if (reconciled) {
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
