import { captureRewindResendPayload } from './rewind-resend-payload'
import { composerDraftKey, emptyComposerDraft } from './composer-drafts'
import { AttachmentPublicMetadata } from '../../../../packages/runtime/src/contracts/attachments.js'
import { UserInputActionSelectorSchemaV1 } from '../../../../packages/runtime/src/contracts/approvals.js'
import { countedGoalRefV1 } from '../../../../packages/runtime/src/contracts/threads.js'
import { projectAttachmentReferencesForPublicSurfaces } from '../agent/attachment-public'
import { getThreadBinding } from './chat-store-thread-binding'
import { withImageThreadNavigation } from '../write/image-thread-navigation'
import type { ChatBlock, ThreadGoal, ThreadGoalStatus, ThreadTodoList, ThreadTodoStatus, UserInputAnswer, UserInputSubmission } from '../agent/types'
import { getProvider } from '../agent/registry'
import { restoreThreadDisplayHistory } from '../agent/analytix-runtime'
import { mergeGoalHistoryBlocks, reconcileTurnUsageById } from '../agent/analytix-mapper'
import { clearActiveStream } from '../thread/streaming/active-stream-store'
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
import type { ChatState, ChatStoreGet, ChatStoreSet, ThreadSelectionReceipt, TurnForkSnapshot, TurnForkSource } from './chat-store-types'
import { queueMessagesPatch, queuePausePatch, queuedMessagesForThread, retireQueuedThreads } from './chat-store-queue'
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
  findUserInputBlockIndex,
  findReusableEmptyThreadId,
  forgetUserInputDraftInterruptionsForThread,
  markDeletedUserInputDraftThread,
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
    markUserInputUncertain: true,
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
    loadThreadDetail: async (id) => {
      const provider = getProvider()
      const detail = await provider.getThreadDetail(id)
      if (!isCurrent()) throw new Error('Interrupted thread selection changed.')
      return restoreThreadDisplayHistory(provider, id, detail)
    },
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

function submittedUserInputDisplay(
  block: Extract<ChatBlock, { kind: 'user_input' }>,
  answers: UserInputSubmission[]
): UserInputAnswer[] {
  return answers.map(answer => {
    if (!('kind' in answer)) return {
      id: answer.id,
      label: projectOrdinaryPublicText(answer.label),
      value: projectOrdinaryPublicText(answer.value)
    }
    if (answer.kind === 'skip') return { id: answer.id, label: i18n.t('common:userInputSkipped'), value: '' }
    const question = block.questions.find(question => question.id === answer.id)
    const selected = answer.optionIds.map(id => question?.options.find(option => option.id === id)?.label ?? '')
      .filter(Boolean).map(label => projectOrdinaryPublicText(label))
    // Typed custom text is a private submission, never a renderer history/export field.
    const customStatus = answer.customText?.trim() ? i18n.t('common:userInputSubmitted') : ''
    return {
      id: answer.id,
      label: selected.join(', ') || customStatus,
      value: [...selected, customStatus].filter(Boolean).join('\n')
    }
  })
}

export function createMaintenanceActions(
  { set, get, sseAbortRef }: StoreActionContext
): Pick<ChatState, 'renameActiveThread' | 'renameThread' | 'archiveThread' | 'compactActiveThread' | 'forkActiveThread' | 'forkThreadFromTurn' | 'setActiveThreadGoal' | 'setActiveThreadGoalStatus' | 'clearActiveThreadGoal' | 'setActiveThreadTodoStatus' | 'clearActiveThreadTodos' | 'syncPlanTodosFromMarkdown' | 'resumeSessionIntoThread' | 'deleteThread' | 'rewindAndResend' | 'rollbackWorkspaceToCheckpoint' | 'resolveApproval' | 'resolveUserInput' | 'interrupt'> {
  const binding = getThreadBinding(sseAbortRef)
  const resolvingGates = new Set<string>()
  const renameOwners = new Map<string, number>()
  let renameGeneration = 0
  let editInFlight = false
  let goalMutationInFlight = false
  const refreshGoalAuthority = async (threadId: string, generation: number, fallback?: ThreadGoal | null): Promise<void> => {
    if (get().activeThreadId !== threadId || binding.generation !== generation) return
    const provider = getProvider()
    const epoch = (get().goalStateEpoch ?? 0) + 1
    set({ goalStateEpoch: epoch })
    const state = await provider.getThreadGoalState?.(threadId)
    const detail = provider.getThreadGoalState ? null : await provider.getThreadDetail(threadId)
    set(current => {
      if (current.activeThreadId !== threadId || current.goalStateEpoch !== epoch || binding.generation !== generation) return {}
      const goal = state ? state.goal : detail?.goal === undefined ? fallback !== undefined ? fallback : current.activeThreadGoal : detail.goal
      return { activeThreadGoal: goal, activeGoalActivation: state?.activation ?? null,
        goalStateEpoch: epoch + 1,
        blocks: mergeGoalHistoryBlocks(current.blocks, state?.historyBlocks ?? detail!.blocks.filter(block => block.kind === 'goal')),
        threads: current.threads.map(thread => thread.id === threadId ? { ...thread, goal } : thread) }
    })
  }
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
      candidate.id === block.id && gateKey(candidate) === key &&
      (block.kind !== 'user_input' || candidate.meta?.turnId === block.meta?.turnId)
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

  const forkActiveThreadWithOptions = async (options: { turnId?: string; source?: TurnForkSource; expected?: ThreadSelectionReceipt } = {}): Promise<void> => withImageThreadNavigation(undefined, async navigationCurrent => {
    const { activeThreadId, route, busy, blocks } = get()
    const expected = options.expected
    if (expected && (expected.threadId !== activeThreadId || !expected.isCurrent())) return
    const source = options.source
    const sourceThreadId = source?.threadId ?? expected?.threadId ?? activeThreadId
    if (!sourceThreadId) return
    const generation = binding.generation
    const submissionGeneration = binding.submissionGeneration
    const turnId = options.turnId?.trim()
    const sourceSnapshot = (): TurnForkSnapshot | null => {
      const snapshot = source?.getSnapshot()
      if (!source) return { blocks: get().blocks ?? blocks, busy: get().busy ?? busy, latestSeq: get().lastSeq }
      if (!snapshot || !source.isCurrent() || source.turnId !== turnId || snapshot.busy ||
        snapshot.rewoundTurnIds?.[source.turnId] === true) return null
      const closingIndex = snapshot.blocks.findIndex(block => block.kind === 'assistant' && block.id === source.finalItemId &&
        blockRuntimeTurnId(block) === source.turnId && block.text.trim() !== '')
      if (closingIndex < 0 || snapshot.blocks.slice(closingIndex + 1).some(block => blockRuntimeTurnId(block) === source.turnId &&
        block.kind !== 'compaction' && (block.kind !== 'assistant' || block.text.trim() !== ''))) return null
      if (source.endSeq !== undefined) {
        const closing = snapshot.blocks[closingIndex] as Extract<ChatBlock, { kind: 'assistant' }>
        const proof = source.proof
        if (!proof) return null
        if (proof.kind === 'general') {
          const current = closing.generalTerminalProjectionReceipt, captured = proof.receipt
          if (!current || closing.acceptedFinalView || current.schemaVersion !== captured.schemaVersion ||
            current.batchDigest !== captured.batchDigest || current.threadId !== captured.threadId || current.turnId !== captured.turnId ||
            current.itemId !== captured.itemId || current.firstSeq !== captured.firstSeq || current.lastSeq !== captured.lastSeq ||
            current.status !== captured.status) return null
        } else {
          const current = closing.acceptedFinalProjectionReceipt, captured = proof.receipt
          const currentTerminal = closing.acceptedFinalProjectionTerminal, capturedTerminal = proof.terminal
          if (!current || current.schemaVersion !== captured.schemaVersion || current.batchId !== captured.batchId ||
            current.threadId !== captured.threadId || current.turnId !== captured.turnId || current.lastSeq !== captured.lastSeq ||
            current.publicationCommitId !== captured.publicationCommitId || !currentTerminal ||
            currentTerminal.status !== capturedTerminal.status || currentTerminal.createdAt !== capturedTerminal.createdAt ||
            currentTerminal.acceptedFinalDigest !== capturedTerminal.acceptedFinalDigest || currentTerminal.terminalReason !== capturedTerminal.terminalReason ||
            closing.acceptedFinalView?.acceptedFinalDigest !== captured.publicationCommitId) return null
        }
        const receipt = closing.acceptedFinalProjectionReceipt ?? closing.generalTerminalProjectionReceipt
        if (!Number.isSafeInteger(source.endSeq) || source.endSeq <= 0 || snapshot.latestSeq < source.endSeq ||
          receipt?.threadId !== sourceThreadId || receipt.turnId !== source.turnId || receipt.lastSeq !== source.endSeq ||
          (closing.acceptedFinalProjectionReceipt ? closing.acceptedFinalProjectionTerminal?.status : closing.generalTerminalProjectionReceipt?.status) !== source.status) return null
      }
      return snapshot
    }
    const initialSnapshot = sourceSnapshot()
    if (!initialSnapshot) return
    if (initialSnapshot.busy) {
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
    const current = (): boolean => {
      const snapshot = sourceSnapshot()
      return navigationCurrent() && get().activeThreadId === activeThreadId && get().route === route &&
        (!expected || expected.isCurrent()) &&
        binding.generation === generation && binding.submissionGeneration === submissionGeneration &&
        get().runtimeConnection === 'ready' && getProvider() === p && snapshot !== null && !snapshot.busy
    }
    if (!current()) return
    try {
      const parentThread =
        get().threads.find((thread) => thread.id === sourceThreadId) ?? {
          id: sourceThreadId,
          title: initialSnapshot.title ?? sourceThreadId.slice(0, 8)
        }
      const forked = turnId
        ? await p.forkThread(sourceThreadId, { turnId, ...(source?.endSeq !== undefined ? { endSeq: source.endSeq } : {}) })
        : await p.forkThread(sourceThreadId)
      // A created Host fork survives owner retirement; never retry it automatically.
      if (!current()) return
      const sourceBlocks = sourceSnapshot()!.blocks
      const fallbackBlocks = turnId ? forkBlocksThroughTurn(sourceBlocks, turnId) : sourceBlocks
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
      if (expected) await get().refreshThreads({ isCurrent: current })
      else await get().refreshThreads()
      if (!current()) return
      await get().selectThread(forked.id)
    } catch (e) {
      if (!current()) return
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
    const owner = ++renameGeneration
    renameOwners.set(targetId, owner)
    if (get().runtimeConnection !== 'ready') {
      const error = new Error(i18n.t('common:runtimeActionNeedsConnection'))
      set({ error: error.message })
      if (renameOwners.get(targetId) === owner) renameOwners.delete(targetId)
      throw error
    }
    const p = getProvider()
    const { activeThreadId, route, error: originalError } = get()
    const generation = binding.generation
    const submissionGeneration = binding.submissionGeneration
    const ownsRename = (): boolean => renameOwners.get(targetId) === owner && getProvider() === p
    const ownsUi = (): boolean => getProvider() === p && get().activeThreadId === activeThreadId && get().route === route &&
      binding.generation === generation && binding.submissionGeneration === submissionGeneration &&
      get().runtimeConnection === 'ready'
    const current = (): boolean => ownsRename() && ownsUi()
    // A completed mutation releases its slot; admitted index reads may continue
    // under the same primitive owner until another rename or UI owner replaces it.
    const readCurrent = (): boolean => renameGeneration === owner && ownsUi()
    const recordError = (error: unknown, expectedError = originalError): void => {
      if (!current() || get().error !== expectedError) return
      set({
        error: formatRuntimeError(error),
        ...(shouldOpenSettingsForError(error)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
    try {
      try {
        await p.renameThread(targetId, nextTitle)
      } catch (error) {
        recordError(error)
        throw error
      }
      if (!ownsRename()) return
      const ownsError = current() && get().error === originalError
      set((s) => ({
        threads: s.threads.map((thread) =>
          thread.id === targetId ? { ...thread, title: nextTitle } : thread
        ),
        threadSearchResults: s.threadSearchResults
          ? { ...s.threadSearchResults, threads: s.threadSearchResults.threads.map((thread) =>
            thread.id === targetId ? { ...thread, title: nextTitle } : thread
          ) }
          : s.threadSearchResults,
        ...(ownsError && s.error === originalError ? { error: null } : {})
      }))
      if (!current() || !ownsError || get().error !== null) return
      const refreshError = get().error
      try {
        await get().refreshThreads({ isCurrent: () => readCurrent() && get().error === refreshError })
      } catch (error) {
        // The rename is already acknowledged; a list refresh cannot undo it.
        recordError(error, refreshError)
      }
    } finally {
      if (renameOwners.get(targetId) === owner) renameOwners.delete(targetId)
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
        set(s => ({ ...retireQueuedThreads(s, [targetId]), userInputDraftInterruptions: forgetUserInputDraftInterruptionsForThread(s.userInputDraftInterruptions, targetId),
          deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(s.deletedUserInputDraftThreadIds, targetId) }))
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
          threadSearchResults: s.threadSearchResults
            ? { ...s.threadSearchResults, threads: s.threadSearchResults.threads.map((thread) =>
              thread.id === targetId ? { ...thread, archived } : thread
            ) }
            : null,
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
    set({ busy: true })
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
    } finally {
      if (get().activeThreadId === activeThreadId) set({ busy: false })
    }
  },

  forkActiveThread: async (expected?: ThreadSelectionReceipt) => {
    await forkActiveThreadWithOptions({ expected })
  },

  forkThreadFromTurn: async (turnId, source) => {
    const targetTurnId = turnId.trim()
    if (!targetTurnId || source && (source.threadId.trim() !== source.threadId || !source.threadId ||
      source.turnId !== targetTurnId || (source.status !== 'completed' && source.status !== 'aborted'))) return
    await forkActiveThreadWithOptions({ turnId: targetTurnId, source })
  },

  setActiveThreadGoal: async (objective, options) => {
    if (goalMutationInFlight) return false
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
    const existing = get().activeThreadGoal
    const ref = countedGoalRefV1(existing)
    const generation = binding.generation
    goalMutationInFlight = true
    set(state => ({ goalStateEpoch: (state.goalStateEpoch ?? 0) + 1 }))
    try {
      const goal = await p.setThreadGoal(activeThreadId, {
        objective: trimmed,
        ...(ref ? { operation: 'edit' as const, ref } : !existing && !options?.research ? { operation: 'create' as const } : {}),
        ...(options?.maxGoalRounds !== undefined ? { maxGoalRounds: options.maxGoalRounds } : {}),
        ...(!ref && options?.research
          ? {
              research: {
                enabled: true as const,
                ...(options.requirements?.length ? { requirements: options.requirements } : {})
              }
            }
          : {})
      })
      await refreshGoalAuthority(activeThreadId, generation, goal)
      await get().refreshThreads()
      if (!existing && options?.research && get().activeThreadId === activeThreadId && binding.generation === generation) {
        return get().sendMessage(goal.objective, 'agent', {
          displayText: i18n.t('common:goalUserMessage', { objective: goal.objective })
        })
      }
      return true
    } catch (e) {
      await refreshGoalAuthority(activeThreadId, generation).catch(() => undefined)
      if (get().activeThreadId !== activeThreadId || binding.generation !== generation) return false
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    } finally { goalMutationInFlight = false }
  },

  setActiveThreadGoalStatus: async (status: ThreadGoalStatus) => {
    if (goalMutationInFlight) return false
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
    const ref = countedGoalRefV1(get().activeThreadGoal)
    if (ref && status !== 'active' && status !== 'paused') return false
    const generation = binding.generation
    goalMutationInFlight = true
    set(state => ({ goalStateEpoch: (state.goalStateEpoch ?? 0) + 1 }))
    try {
      const goal = await p.setThreadGoal(activeThreadId, ref ? { operation: status === 'active' ? 'resume' : 'pause', ref } : { status })
      await refreshGoalAuthority(activeThreadId, generation, goal)
      await get().refreshThreads()
      return true
    } catch (e) {
      await refreshGoalAuthority(activeThreadId, generation).catch(() => undefined)
      if (get().activeThreadId !== activeThreadId || binding.generation !== generation) return false
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    } finally { goalMutationInFlight = false }
  },

  clearActiveThreadGoal: async () => {
    if (goalMutationInFlight) return false
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
    const ref = countedGoalRefV1(get().activeThreadGoal)
    const generation = binding.generation
    goalMutationInFlight = true
    set(state => ({ goalStateEpoch: (state.goalStateEpoch ?? 0) + 1 }))
    try {
      const cleared = ref ? await p.clearThreadGoal(activeThreadId, ref) : await p.clearThreadGoal(activeThreadId)
      if (cleared) {
        await refreshGoalAuthority(activeThreadId, generation, null)
      }
      await get().refreshThreads()
      return cleared
    } catch (e) {
      await refreshGoalAuthority(activeThreadId, generation).catch(() => undefined)
      if (get().activeThreadId !== activeThreadId || binding.generation !== generation) return false
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      return false
    } finally { goalMutationInFlight = false }
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
          threadSearchResults: s.threadSearchResults
            ? { ...s.threadSearchResults, threads: s.threadSearchResults.threads.filter((thread) => thread.id !== targetId) }
            : null,
          ...retireQueuedThreads(s, [targetId]),
          userInputDraftInterruptions: forgetUserInputDraftInterruptionsForThread(s.userInputDraftInterruptions, targetId),
          deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(s.deletedUserInputDraftThreadIds, targetId),
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

  rewindAndResend: async (userBlockId, newText, onCaptured) => {
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
    let sent = false
    try {
      // The closed payload and original draft CAS now own any later failure.
      // This synchronous handoff is renderer-local, not a Core acknowledgement.
      onCaptured?.()
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
      try { await provider.rewindThread(activeThreadId, turnId) } catch {
        if (isCurrent()) set({ error: i18n.t('common:rewindResendOutcomeUnknown') })
        // A failed reply, including a canonical 404, may follow a durable cut.
        // Without a no-commit receipt, never resend or fabricate a history cut.
        // SSE owns any cut we have observed.
        return
      }
      if (!isCurrent()) return
      // Our rewind SSE may already have removed the target. Mirror the captured
      // cut only after its successful HTTP reply, without requiring it to remain.
      const removedTurnIds = new Set<string>([turnId])
      for (const block of state.blocks.slice(idx)) {
        if ('meta' in block && typeof block.meta?.turnId === 'string' && block.meta.turnId.trim()) removedTurnIds.add(block.meta.turnId.trim())
      }
      const rewoundTurnIds = { ...get().rewoundTurnIds,
        ...Object.fromEntries([...removedTurnIds].map(id => [id, true as const])) }
      const turnUsageById = reconcileTurnUsageById(activeThreadId, undefined, get().turnUsageById, undefined, rewoundTurnIds)
      for (const removedTurnId of removedTurnIds) clearActiveStream(activeThreadId, removedTurnId)
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
        turnStartedAtByUserId, turnDurationByUserId, turnUsageById, rewoundTurnIds, lastTurnUsage: null,
        ...queueMessagesPatch(get(), activeThreadId, []), ...queuePausePatch(get(), null, activeThreadId), error: null })
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
      // After local capture the bubble may close before async validation.
      // Keep its payload on the original draft while that owner still exists.
      // Core may have committed even without a reply or SSE; this draft is not
      // history or permission to resend. Object identity protects newer drafts.
      const owner = get().threads.find(thread => thread.id === activeThreadId)
      if (!sent && owner && owner.historyAuthority !== 'case_boundary_only_v1' && normalizeWorkspaceRoot(owner.workspace) === workspace) {
        const references = new Map(payload.attachments.map(reference => [reference.id, reference]))
        get().updateComposerDraft?.(draftKey, current => current !== originalDraft ? current : ({
          ...current, input: trimmed, inputRevision: current.inputRevision + 1,
          attachments: payload.attachmentIds.map(id => references.get(id) ?? { id }),
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

  resolveUserInput: async (target, action) => {
    const state = get()
    if (!target || typeof target !== 'object' || !target.threadId || target.threadId !== state.activeThreadId) return
    const index = findUserInputBlockIndex(state.blocks, target.itemId, target.requestId)
    const block = index < 0 ? undefined : state.blocks[index]
    if (!block || block.kind !== 'user_input') throw new Error(i18n.t('common:ownedThreadUnavailable'))
    const parsedSelector = UserInputActionSelectorSchemaV1.safeParse(target)
    if (!parsedSelector.success || parsedSelector.data.turnId !== block.meta?.turnId)
      throw new Error(i18n.t('common:ownedThreadUnavailable'))
    const selector = parsedSelector.data
    const provider = getProvider()
    if (action.kind === 'late') {
      const turnId = action.expectedTurnId
      if (block.status !== 'continued' || !turnId || isPendingActiveStreamTurnId(turnId) || !provider.submitUserInputLateReply) throw new Error(i18n.t('common:userInputLateUnavailable'))
      // The private leaf captures the current Host anchor once. A retry keeps
      // that anchor and UUID; Host owns admission and the actual delivery target.
      await provider.submitUserInputLateReply(block.requestId, action.answers, { clientUserMessageId: action.clientUserMessageId, expectedTurnId: turnId }, selector)
      // Terminal streams retire. Reuse the existing guarded snapshot/SSE
      // recovery for an idle wake; the receipt never fabricates busy or history.
      if (!state.busy && get().activeThreadId === target.threadId && get().route === 'chat' && !get().busy)
        void get().recoverActiveTurn()
      return
    }
    if (block.status !== 'pending') throw new Error(i18n.t('common:pendingRequestNotResolved'))
    const supported = action.kind === 'submit'
      ? typeof provider.submitUserInputResponse === 'function'
      : typeof provider.cancelUserInput === 'function'
    const publicAnswers = action.kind === 'submit'
      ? submittedUserInputDisplay(block, action.answers) : undefined
    await resolveGate(block, async () => {
      if (!supported) throw new Error(i18n.t('common:runtimeUserInputUnsupported'))
      if (action.kind === 'submit') await provider.submitUserInputResponse!(block.requestId, action.answers, selector)
      else await provider.cancelUserInput!(block.requestId, selector)
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
    binding.interruptionGeneration += 1
    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    quarantineInterruptedTurn(set, get, activeThreadId, currentTurnId, currentTurnUserId)
    if (queuedMessagesForThread(get(), activeThreadId).length > 0) {
      set(s => queuePausePatch(s, 'interrupted', activeThreadId))
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
        if (queuedMessagesForThread(get(), activeThreadId).length === 0) {
          releaseThreadWorktreeIfNeeded(activeThreadId)
        }
        void get().drainQueuedMessages(activeThreadId)
      }
    }
  }
  }
}
