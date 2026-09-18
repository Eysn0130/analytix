import type { AgentProvider, AttachmentReference, ChatBlock, NormalizedThread, ReviewTarget, ThreadEventSink, UserFileReference } from '../agent/types'
import { projectAttachmentReferencesForPublicSurfaces } from '../agent/attachment-public'
import { getProvider } from '../agent/registry'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { applyTheme, applyUiFontScale } from '../lib/apply-theme'
import { formatWorkspacePickerError } from '../lib/format-workspace-picker-error'
import { formatRuntimeError } from '../lib/format-runtime-error'
import {
  getDefaultThreadTitle
} from '../lib/thread-title'
import { filterThreadsForSidebar } from '../lib/thread-sidebar-visibility'
import {
  invalidateThreadDetailCache,
  loadThreadDetailWithCache
} from '../lib/thread-detail-cache'
import { isPublicProjectionRevoked } from '../lib/public-projection-revocation'
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
  markThreadWorktree,
  readThreadWorktreeRegistry,
  saveThreadWorktreeRegistry
} from '../lib/thread-worktree-registry'
import { workspaceLabelFromPath } from '../lib/workspace-label'
import { isInternalTemporaryWorkspace, normalizeWorkspaceRoot } from '../lib/workspace-path'
import type { GitCheckpointCreateResult } from '@shared/git-checkpoint'
import { parseWorktreeHasChangesError } from '@shared/worktree'
import type { ThreadHandoffOperation } from '@shared/thread-handoff'
import { projectModelReasoningEffortV1 } from '@shared/model-reasoning-effort'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import type { ModelReasoningEffort } from '@shared/app-settings'
import {
  buildClawRuntimePrompt,
  buildCodeRuntimePrompt
} from '@shared/app-settings'
import type { ChatState, ChatStoreGet, ChatStoreSet, QueuedUserMessage } from './chat-store-types'
import {
  activeClawChannel,
  canonicalComposerModelForSelection,
  composerModelSelectable,
  compactCodeWorkspaceRoots,
  forgetCodeWorkspaceRoot,
  hydrateBlockModelLabels,
  isClawThread,
  optimisticUserModelLabel,
  providerIdForComposerModel,
  providerIdMatchesComposerModel,
  readCodeWorkspaceRoots,
  readThreadComposerSelection,
  rememberCodeWorkspaceRoots,
  rememberThreadComposerSelection,
  rememberTurnModel
} from './chat-store-helpers'
import {
  clearedThreadSelection,
  collectAssistantTextForTurn,
  findLatestUserBlockId,
  findReusableEmptyThreadId,
  reconcileOptimisticUserBlock,
  threadHasPendingRuntimeWork,
  threadSnapshotLooksRunning,
  threadBelongsToWorkspace
} from './chat-store-runtime-helpers'
import {
  WRITE_ASSISTANT_THREAD_TITLE,
  activeWriteThreadForWorkspace,
  forgetWriteThread,
  hydrateWriteThreadRegistry,
  isWriteThreadId,
  markWriteThread,
  pruneWriteThreadRegistry,
  readWriteThreadRegistry,
  saveWriteThreadRegistry,
  writeThreadBelongsToWorkspace,
  writeWorkspaceForThreadId
} from '../write/write-thread-registry'
import {
  clearBusyWatchdog,
  resetBusyRecoveryAttempts,
  scheduleStartupRuntimeProbe,
  stopTurnCompletionPoll
} from './chat-store-schedulers'
import {
  clearActiveStream,
  isPendingActiveStreamTurnId,
  migrateActiveStreamTurn,
  pendingActiveStreamTurnId,
  resetActiveStream
} from '../thread/streaming/active-stream-store'
import {
  armBusyWatchdog,
  buildThreadEventSink,
  clearWatchedCompletionNotification,
  finalizeTurnTiming,
  flushLiveBlocks,
  forkedMessageCount,
  forkedTurnCount,
  isCodeThread,
  latestThread,
  looksLikeActiveTurnError,
  readActiveWriteWorkspace,
  readWriteWorkspaceRoots,
  quarantineTerminalTurnDraft,
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

let drainingQueuedMessages = false
const finalizingThreadHandoffs = new Set<string>()
const CHECKPOINT_SEND_SOFT_TIMEOUT_MS = 650
const checkpointGitUnavailableWorkspaces = new Set<string>()

type SendCheckpointResult = GitCheckpointCreateResult | {
  ok: false
  reason: 'timeout'
  message: string
}

function checkpointWorkspaceKey(workspaceRoot: string): string {
  return workspaceRoot.replaceAll('\\', '/').toLowerCase()
}

async function createWorkspaceCheckpointForSend(params: {
  workspaceRoot: string
  threadId: string
  timeoutMs?: number
}): Promise<SendCheckpointResult> {
  const workspaceRoot = normalizeWorkspaceRoot(params.workspaceRoot)
  if (!workspaceRoot) {
    return {
      ok: false,
      reason: 'no_workspace',
      message: 'No workspace is available for Git checkpoint.'
    }
  }

  const workspaceKey = checkpointWorkspaceKey(workspaceRoot)
  if (checkpointGitUnavailableWorkspaces.has(workspaceKey)) {
    return {
      ok: false,
      reason: 'git_unavailable',
      message: 'Git checkpoint is unavailable for this workspace.'
    }
  }

  const createGitCheckpoint = typeof window !== 'undefined'
    ? window.analytix?.workspace?.createGitCheckpoint
    : undefined
  if (typeof createGitCheckpoint !== 'function') {
    return {
      ok: false,
      reason: 'git_unavailable',
      message: 'Git checkpoint bridge is unavailable.'
    }
  }

  const timeoutMs = Math.max(1, params.timeoutMs ?? CHECKPOINT_SEND_SOFT_TIMEOUT_MS)
  let timeoutId: ReturnType<typeof setTimeout> | null = null
  const checkpointPromise = createGitCheckpoint({
    workspaceRoot,
    threadId: params.threadId,
    timeoutMs
  }).catch((error): SendCheckpointResult => ({
    ok: false,
    reason: 'error',
    message: error instanceof Error ? error.message : String(error)
  }))
  const timeoutPromise = new Promise<SendCheckpointResult>((resolve) => {
    timeoutId = setTimeout(() => {
      resolve({
        ok: false,
        reason: 'timeout',
        message: `Git checkpoint did not finish within ${timeoutMs}ms.`
      })
    }, timeoutMs)
  })

  const result = await Promise.race([checkpointPromise, timeoutPromise])
  if (timeoutId != null) {
    clearTimeout(timeoutId)
  }
  if (!result.ok && result.reason === 'git_unavailable') {
    checkpointGitUnavailableWorkspaces.add(workspaceKey)
  }
  return result
}

function mergePersistedBlocksWithLiveOverlay(persisted: ChatBlock[], current: ChatBlock[]): ChatBlock[] {
  if (persisted.length === 0) return current
  if (current.length === 0) return persisted
  const seen = new Set(persisted.map((block) => block.id))
  const merged = [...persisted]
  for (const block of current) {
    if (seen.has(block.id)) continue
    if (isDuplicatePersistedAssistant(block, current, persisted)) continue
    merged.push(block)
    seen.add(block.id)
  }
  return merged
}

function blockTurnId(block: ChatBlock): string {
  return 'meta' in block && typeof block.meta?.turnId === 'string'
    ? block.meta.turnId.trim()
    : ''
}

function previousUserIdentity(block: ChatBlock, blocks: ChatBlock[]): string {
  const index = blocks.findIndex((candidate) => candidate === block || candidate.id === block.id)
  if (index < 0) return ''
  for (let cursor = index - 1; cursor >= 0; cursor -= 1) {
    const candidate = blocks[cursor]
    if (candidate?.kind === 'user') return `user:${candidate.id}`
  }
  return ''
}

function assistantMergeIdentities(block: ChatBlock, blocks: ChatBlock[]): string[] {
  const identities: string[] = []
  const turnId = blockTurnId(block)
  if (turnId) identities.push(`turn:${turnId}`)
  const userIdentity = previousUserIdentity(block, blocks)
  if (userIdentity) identities.push(userIdentity)
  return identities
}

function isLiveFlushAssistantBlock(block: ChatBlock): boolean {
  return (
    block.kind === 'assistant' &&
    (block.id.startsWith('a-') || block.id.startsWith('live_assistant_') || block.id === 'live-assistant')
  )
}

function isDuplicatePersistedAssistant(block: ChatBlock, current: ChatBlock[], persisted: ChatBlock[]): boolean {
  if (block.kind !== 'assistant') return false
  if (!isLiveFlushAssistantBlock(block)) return false
  const text = block.text.trim()
  if (!text) return false
  const identities = assistantMergeIdentities(block, current)
  if (identities.length === 0) return false
  return persisted.some((candidate) => {
    if (candidate.kind !== 'assistant' || candidate.text.trim() !== text) return false
    const candidateIdentities = assistantMergeIdentities(candidate, persisted)
    return candidateIdentities.some((identity) => identities.includes(identity))
  })
}

function fallbackComposerProviderIdForSend(state: ChatState): string {
  return state.route === 'claw' ? '' : state.composerProviderId.trim()
}

function composerSelectionForThread(
  state: ChatState,
  thread: Pick<NormalizedThread, 'id' | 'model' | 'providerId'> | null | undefined
): { model: string; providerId: string } | null {
  if (!thread) return null
  const pickList = state.composerPickList
  const stored = readThreadComposerSelection(thread.id)
  const storedModel = stored?.model.trim() ?? ''
  const threadModel = thread.model.trim()
  const model = composerModelSelectable(pickList, state.composerModelGroups, storedModel)
    ? storedModel
    : composerModelSelectable(pickList, state.composerModelGroups, threadModel)
      ? threadModel
      : ''
  if (!model) return null
  const canonicalModel = canonicalComposerModelForSelection(state.composerModelGroups, model) || model
  const storedProviderId =
    stored && providerIdMatchesComposerModel(state.composerModelGroups, stored.providerId, canonicalModel)
      ? stored.providerId
      : ''
  const threadProviderId =
    thread.providerId && providerIdMatchesComposerModel(state.composerModelGroups, thread.providerId, canonicalModel)
      ? thread.providerId
      : ''
  return {
    model: canonicalModel,
    providerId: storedProviderId || threadProviderId || providerIdForComposerModel(state.composerModelGroups, canonicalModel)
  }
}

function subscribeThreadEventsWithRecovery(
  provider: AgentProvider,
  threadId: string,
  sinceSeq: number,
  sink: ThreadEventSink,
  signal: AbortSignal,
  get: ChatStoreGet,
  sseAbortRef?: SseAbortRef,
  controller?: AbortController
): void {
  void provider.subscribeThreadEvents(threadId, sinceSeq, sink, signal)
    .catch(() => undefined)
    .then(() => {
      if (signal.aborted) return
      const state = get()
      if (state.activeThreadId !== threadId || !state.busy) return
      void state.recoverActiveTurn()
    })
    .finally(() => {
      if (sseAbortRef && controller && sseAbortRef.current === controller) {
        sseAbortRef.current = null
      }
    })
}

function upsertThreadHandoffOperation(
  operations: ThreadHandoffOperation[],
  operation: ThreadHandoffOperation
): ThreadHandoffOperation[] {
  const index = operations.findIndex((item) => item.id === operation.id)
  if (index === -1) return [...operations, operation]
  return operations.map((item, itemIndex) => itemIndex === index ? operation : item)
}

function threadHandoffIsTerminal(operation: ThreadHandoffOperation): boolean {
  return operation.status === 'success' || operation.status === 'warning' || operation.status === 'error'
}

function threadHandoffNeedsSwitchFinalize(operation: ThreadHandoffOperation): boolean {
  return operation.status === 'running'
    && Boolean(operation.targetWorkspace?.trim())
    && operation.steps.some((step) => step.id === 'switching-thread' && step.status === 'running')
}

type MidTurnSteerDraft = {
  id?: string
  text: string
  displayText?: string
  mode?: string
  model?: string
  providerId?: string
  modelLabel?: string
  reasoningEffort?: ModelReasoningEffort
  attachmentIds?: string[]
  attachments?: AttachmentReference[]
  fileReferences?: UserFileReference[]
  guiPlan?: QueuedUserMessage['guiPlan']
}

function createClientUserMessageId(now = Date.now()): string {
  return globalThis.crypto?.randomUUID?.() ?? `steer-${now}-${Math.random().toString(16).slice(2)}`
}

function requiresOrdinaryPublicProjection(text: string): boolean {
  return projectOrdinaryPublicText(text) !== text
}

function queuedMessageFromSteerDraft(draft: MidTurnSteerDraft, index: number, now = Date.now()): QueuedUserMessage {
  const id = draft.id?.trim() || `q-${now}-${index}`
  return {
    id,
    text: draft.text,
    ...(draft.displayText ? { displayText: draft.displayText } : {}),
    ...(draft.mode ? { mode: draft.mode } : {}),
    ...(draft.model ? { model: draft.model } : {}),
    ...(draft.providerId ? { providerId: draft.providerId } : {}),
    ...(draft.modelLabel ? { modelLabel: draft.modelLabel } : {}),
    ...(draft.reasoningEffort ? { reasoningEffort: draft.reasoningEffort } : {}),
    ...(draft.guiPlan ? { guiPlan: draft.guiPlan } : {}),
    ...(draft.attachmentIds?.length ? { attachmentIds: draft.attachmentIds } : {}),
    ...(draft.attachments?.length
      ? { attachments: projectAttachmentReferencesForPublicSurfaces(draft.attachments) }
      : {}),
    ...(draft.fileReferences?.length ? { fileReferences: draft.fileReferences } : {})
  }
}

async function steerActiveTurn(options: {
  set: ChatStoreSet
  get: ChatStoreGet
  provider: AgentProvider
  draft: MidTurnSteerDraft
  removeQueuedId?: string
  queueOnFailure?: boolean
}): Promise<boolean> {
  const { set, get, provider, draft, removeQueuedId, queueOnFailure = true } = options
  const activeThreadId = get().activeThreadId
  const turnId = get().currentTurnId
  if (!activeThreadId || !turnId || draft.guiPlan || typeof provider.steerUserMessage !== 'function') {
    return false
  }

  const now = Date.now()
  const clientUserMessageId = createClientUserMessageId(now)
  const visibleText = projectOrdinaryPublicText(draft.displayText?.trim() || draft.text)
  const publicAttachments = draft.attachments?.length
    ? projectAttachmentReferencesForPublicSurfaces(draft.attachments)
    : []
  set((s) => ({
    blocks: [
      ...s.blocks,
      {
        kind: 'user' as const,
        id: clientUserMessageId,
        createdAt: new Date(now).toISOString(),
        text: visibleText,
        ...(draft.modelLabel ? { modelLabel: draft.modelLabel } : {}),
        meta: {
          turnId,
          clientUserMessageId,
          steeringStatus: 'pending' as const,
          ...(draft.displayText && draft.displayText !== draft.text
            ? { displayText: projectOrdinaryPublicText(draft.displayText) }
            : {}),
          ...(draft.attachmentIds?.length ? { attachmentIds: draft.attachmentIds } : {}),
          ...(publicAttachments.length ? { attachments: publicAttachments } : {}),
          ...(draft.fileReferences?.length ? { fileReferences: draft.fileReferences } : {})
        }
      }
    ],
    queuedMessages: removeQueuedId
      ? s.queuedMessages.filter((message) => message.id !== removeQueuedId)
      : s.queuedMessages,
    queuedMessagesPausedReason: removeQueuedId && s.queuedMessages.length <= 1 ? null : s.queuedMessagesPausedReason,
    error: null
  }))

  try {
    const response = await provider.steerUserMessage({
      threadId: activeThreadId,
      turnId,
      text: draft.text,
      displayText: draft.displayText,
      clientUserMessageId,
      expectedTurnId: turnId,
      attachmentIds: draft.attachmentIds,
      fileReferences: draft.fileReferences
    })
    set((s) => ({
      blocks: s.blocks.map((block) =>
        block.kind === 'user' && block.id === clientUserMessageId
          ? {
              ...block,
              meta: {
                ...(block.meta ?? {}),
                steeringStatus: 'admitted' as const,
                ...(response.admittedSeq !== undefined ? { admittedSeq: response.admittedSeq } : {})
              }
            }
          : block
      ),
      error: null
    }))
    return true
  } catch (error) {
    set((s) => ({
      blocks: s.blocks.filter((block) => !(block.kind === 'user' && block.id === clientUserMessageId)),
      ...(queueOnFailure
        ? {
            queuedMessages: [
              ...s.queuedMessages,
              queuedMessageFromSteerDraft(draft, s.queuedMessages.length)
            ],
            queuedMessagesPausedReason: 'failed' as const
          }
        : {}),
      error: formatRuntimeError(error)
    }))
    return false
  }
}

export function createThreadActions(
  { set, get, sseAbortRef }: StoreActionContext
): Pick<ChatState, 'createThread' | 'recoverActiveTurn' | 'selectThread' | 'subscribeThreadEventsLive' | 'drainQueuedMessages' | 'editQueuedMessage' | 'removeQueuedMessage' | 'reorderQueuedMessages' | 'sendQueuedMessageNow' | 'resumeInterruptedQueue' | 'setQueuedMessagesPausedReason' | 'startThreadHandoff' | 'retryThreadHandoff' | 'cancelThreadHandoff' | 'closeThreadHandoffOperation' | 'hydrateThreadHandoffOperations' | 'handleThreadHandoffEvent' | 'sendMessage' | 'reviewActiveThread'> {
  // Receipt ownership is local to this action factory, never persisted. A later
  // send or subscription/snapshot replacement revokes the older completion.
  let currentReceiptOwner: symbol | null = null
  return {
  createThread: async (options = {}) => {
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    try {
      const settings = await rendererRuntimeClient.getSettings()
      const activeThread = get().activeThreadId
        ? get().threads.find((thread) => thread.id === get().activeThreadId)
        : null
      let workspaceRoot =
        normalizeWorkspaceRoot(options.workspaceRoot) ||
        (activeThread && !isInternalTemporaryWorkspace(activeThread.workspace)
          ? normalizeWorkspaceRoot(activeThread.workspace)
          : '') ||
        normalizeWorkspaceRoot(settings.workspaceRoot)
      if (!workspaceRoot) {
        await get().chooseWorkspace({ createThreadAfter: true })
        return
      }
      const codeWorkspaceRoots = rememberCodeWorkspaceRoots(get().codeWorkspaceRoots, [workspaceRoot])
      if (!options.useWorktreePool && !options.materialize) {
        sseAbortRef.current?.abort()
        sseAbortRef.current = null
        clearBusyWatchdog()
        const state = get()
        const nextWatch = { ...state.watchTurnCompletion }
        if (state.activeThreadId && state.busy) {
          nextWatch[state.activeThreadId] = true
          watchTurnCompletionNotification(state.activeThreadId)
        }
        set({
          ...clearedThreadSelection(),
          route: 'chat',
          workspaceRoot,
          workspaceLabel: workspaceLabelFromPath(workspaceRoot),
          codeWorkspaceRoots,
          watchTurnCompletion: nextWatch,
          error: null
        })
        syncTurnCompletionPoll(set, get)
        return
      }
      const p = getProvider()
      set({ codeWorkspaceRoots })
      const reusableThreadId = options.forceNew || options.useWorktreePool
        ? null
        : await findReusableEmptyThreadId(
            get(),
            p,
            workspaceRoot,
            (thread) => isCodeThread(thread, get().clawChannels)
          )
      if (reusableThreadId) {
        if (get().activeThreadId !== reusableThreadId) {
          await get().selectThread(reusableThreadId)
        } else {
          set({ error: null })
        }
        return
      }
      let acquiredWorktree: { projectPath: string; poolIndex: number; path: string; branch: string } | null = null
      if (options.useWorktreePool) {
        try {
          const poolIndex = await window.analytix.workspace.findAvailableWorktreePoolIndex({
            projectPath: workspaceRoot
          })
          if (poolIndex === null) {
            set({ error: i18n.t('common:worktreePoolFull') })
          } else {
            const acquireWithForce = async (force: boolean) =>
              window.analytix.workspace.acquireWorktree({
                projectPath: workspaceRoot,
                poolIndex,
                taskId: `pending-${Date.now()}`,
                force
              })
            let wt: { path: string; branch: string }
            try {
              wt = await acquireWithForce(false)
            } catch (acquireErr) {
              const acquireMsg = acquireErr instanceof Error ? acquireErr.message : String(acquireErr)
              if (parseWorktreeHasChangesError(acquireMsg)) {
                wt = await acquireWithForce(true)
              } else {
                throw acquireErr
              }
            }
            acquiredWorktree = {
              projectPath: workspaceRoot,
              poolIndex,
              path: wt.path,
              branch: wt.branch
            }
            workspaceRoot = wt.path
          }
        } catch {
          set({ error: i18n.t('common:worktreeAcquireFailed') })
        }
      }
      const t = await p.createThread({
        workspace: workspaceRoot,
        title: getDefaultThreadTitle(),
        mode: 'agent',
        ...(get().composerModel.trim() ? { model: get().composerModel.trim() } : {}),
        ...(get().composerProviderId.trim() ? { providerId: get().composerProviderId.trim() } : {})
      })
      // Register + activate optimistically before refreshing. A freshly created
      // Analytix thread may not be listed until the first message is written.
      // Setting it active first lets refreshThreads preserve it in the sidebar.
      set((s) => ({
        activeThreadId: t.id,
        codeWorkspaceRoots: rememberCodeWorkspaceRoots(s.codeWorkspaceRoots, [workspaceRoot, t.workspace]),
        threads: s.threads.some((thread) => thread.id === t.id) ? s.threads : [t, ...s.threads]
      }))
      await get().selectThread(t.id)
      if (acquiredWorktree) {
        saveThreadWorktreeRegistry(
          markThreadWorktree(t.id, {
            projectPath: acquiredWorktree.projectPath,
            poolIndex: acquiredWorktree.poolIndex,
            worktreePath: acquiredWorktree.path,
            branch: acquiredWorktree.branch,
            createdAt: new Date().toISOString()
          })
        )
      }
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

  recoverActiveTurn: async () => {
    const state = get()
    if (!state.activeThreadId) return false
    const { activeThreadId } = state
    if (isPublicProjectionRevoked(activeThreadId)) {
      set({ ...clearedThreadSelection(), error: i18n.t('common:runtimeFinalSnapshotUnavailable') })
      return false
    }
    const recoveryRequestSeq = state.lastSeq
    const p = getProvider()
    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    clearBusyWatchdog()
    set({ error: runtimeStreamRecoveringMessage() })
    try {
      const {
        blocks: rawBlocks,
        latestSeq,
        threadStatus,
        latestTurnId,
        latestUserMessageId,
        turnDurationByUserId = {},
        goal,
        todos,
        historyAuthority,
        latestTurnAcceptedFinalDigest
      } = await p.getThreadDetail(activeThreadId)
      if (isPublicProjectionRevoked(activeThreadId) || get().activeThreadId !== activeThreadId) {
        return false
      }
      const blocks = hydrateBlockModelLabels(activeThreadId, rawBlocks)
      const busy = threadSnapshotLooksRunning(blocks, threadStatus)
      const currentTurnUserId = busy
        ? state.currentTurnUserId ?? latestUserMessageId ?? findLatestUserBlockId(blocks)
        : null
      const stateTurnId = state.currentTurnId && !isPendingActiveStreamTurnId(state.currentTurnId)
        ? state.currentTurnId
        : null
      const currentTurnId = busy ? stateTurnId ?? latestTurnId ?? null : null
      const shouldReplaceLiveState = get().lastSeq <= recoveryRequestSeq
      const knownCaseThread =
        historyAuthority === 'case_boundary_only_v1' ||
        state.threads.some((thread) => (
          thread.id === activeThreadId && thread.historyAuthority === 'case_boundary_only_v1'
        ))
      const expectedTerminalTurnId = stateTurnId ?? latestTurnId ?? null
      const trustedCaseTerminal = Boolean(
        expectedTerminalTurnId &&
        latestTurnId?.trim() === expectedTerminalTurnId &&
        latestTurnAcceptedFinalDigest &&
        blocks.some((block) => (
          block.kind === 'assistant' && block.meta?.turnId?.trim() === expectedTerminalTurnId
        ))
      )
      if (shouldReplaceLiveState && state.busy && !busy && knownCaseThread && !trustedCaseTerminal) {
        const quarantined = quarantineTerminalTurnDraft({
          threadId: activeThreadId,
          turnId: expectedTerminalTurnId,
          userBlockId: state.currentTurnUserId,
          busy: false,
          eventSeq: latestSeq,
          set,
          get
        })
        if (!quarantined) clearActiveStream(activeThreadId)
        set({
          ...(!quarantined ? { blocks: blocks.filter((block) => block.kind !== 'assistant') } : {}),
          liveAssistant: '',
          busy: false,
          error: i18n.t('common:runtimeFinalSnapshotUnavailable'),
          runtimeErrorDetail: null,
          queuedMessagesPausedReason: 'failed'
        })
        return false
      }
      if (shouldReplaceLiveState) {
        if (busy && currentTurnId) {
          resetActiveStream(activeThreadId, {
            turnId: currentTurnId,
            lastSeq: latestSeq
          })
        } else {
          clearActiveStream(activeThreadId)
        }
      }

      set((s) => {
        const hasNewerLiveState = !shouldReplaceLiveState || s.lastSeq > recoveryRequestSeq
        return {
          activeThreadId,
          activeThreadGoal: goal ?? null,
          activeThreadTodos: todos ?? null,
          blocks: hasNewerLiveState ? s.blocks : blocks,
          lastSeq: Math.max(latestSeq, s.lastSeq),
          liveAssistant: hasNewerLiveState ? s.liveAssistant : '',
          error: hasNewerLiveState ? s.error : busy ? runtimeStreamRecoveringMessage() : null,
          busy: hasNewerLiveState ? s.busy : busy,
          currentTurnId: hasNewerLiveState ? s.currentTurnId : currentTurnId,
          currentTurnUserId: hasNewerLiveState ? s.currentTurnUserId : currentTurnUserId,
          turnDurationByUserId: hasNewerLiveState
            ? { ...turnDurationByUserId, ...s.turnDurationByUserId }
            : turnDurationByUserId,
          queuedMessages: s.queuedMessages
        }
      })

      const ac = new AbortController()
      sseAbortRef.current = ac
      const subscribeSeq = Math.max(latestSeq, get().lastSeq)
      const sink = buildThreadEventSink(set, get, { threadId: activeThreadId, signal: ac.signal, sinceSeq: subscribeSeq })
      subscribeThreadEventsWithRecovery(p, activeThreadId, subscribeSeq, sink, ac.signal, get, sseAbortRef, ac)
      if (get().busy) {
        armBusyWatchdog(set, get)
      } else {
        resetBusyRecoveryAttempts()
        if (get().queuedMessages.length > 0) {
          void get().drainQueuedMessages()
        }
      }
      return get().busy
    } catch (e) {
      if (isPublicProjectionRevoked(activeThreadId)) return false
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      if (state.busy) armBusyWatchdog(set, get)
      return state.busy
    }
  },

  selectThread: async (id) => {
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return
    }
    const prevId = get().activeThreadId
    const prevBusy = get().busy
    let nextWatch = { ...get().watchTurnCompletion }
    delete nextWatch[id]
    clearWatchedCompletionNotification(id)
    if (prevId && prevId !== id && prevBusy) {
      nextWatch[prevId] = true
      watchTurnCompletionNotification(prevId)
    }
    const nextUnread = { ...get().unreadThreadIds }
    delete nextUnread[id]

    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    const p = getProvider()
    try {
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()
      const threadSnap = get().threads.find((thread) => thread.id === id) ?? null
      const {
        thread: detailThread,
        blocks: rawBlocks,
        latestSeq,
        threadStatus,
        latestTurnId,
        latestUserMessageId,
        turnDurationByUserId = {},
        usage: threadUsage,
        goal,
        todos
      } = await loadThreadDetailWithCache(p, id, threadSnap)
      if (detailThread && detailThread.id !== id) throw new Error('Runtime thread response identity mismatch.')
      const blocks = hydrateBlockModelLabels(id, rawBlocks)
      const busy = threadSnapshotLooksRunning(blocks, threadStatus)
      const currentTurnUserId = busy
        ? latestUserMessageId ?? findLatestUserBlockId(blocks)
        : null
      if (busy && latestTurnId) {
        resetActiveStream(id, {
          turnId: latestTurnId,
          lastSeq: latestSeq
        })
      } else {
        clearActiveStream(id)
      }
      const currentThreads = get().threads
      const threads = detailThread
        ? currentThreads.some((thread) => thread.id === id)
          ? currentThreads.map((thread) => thread.id === id ? { ...thread, ...detailThread } : thread)
          : [...currentThreads, detailThread]
        : currentThreads
      const composerSelection = composerSelectionForThread(get(), detailThread ?? threadSnap)
      set({
        threads,
        watchTurnCompletion: nextWatch,
        unreadThreadIds: nextUnread,
        activeThreadId: id,
        activeThreadGoal: goal ?? null,
        activeThreadTodos: todos ?? null,
        blocks,
        lastSeq: latestSeq,
        liveAssistant: '',
        error: null,
        busy,
        currentTurnId: busy ? latestTurnId ?? null : null,
        currentTurnUserId,
        turnStartedAtByUserId: {},
        turnDurationByUserId,
        inspectorSelectedId: null,
        queuedMessages: [],
        queuedMessagesPausedReason: null,
        ...(composerSelection
          ? {
              composerModel: composerSelection.model,
              composerProviderId: composerSelection.providerId
            }
          : {})
      })
      syncTurnCompletionPoll(set, get)
      const ac = new AbortController()
      sseAbortRef.current = ac
      const sink = buildThreadEventSink(set, get, { threadId: id, signal: ac.signal, sinceSeq: latestSeq })
      subscribeThreadEventsWithRecovery(p, id, latestSeq, sink, ac.signal, get, sseAbortRef, ac)
      if (busy) armBusyWatchdog(set, get)
    } catch (e) {
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
  },

  subscribeThreadEventsLive: async (threadId) => {
    if (get().runtimeConnection !== 'ready') return
    const targetThreadId = threadId.trim()
    if (!targetThreadId) return
    if (isPublicProjectionRevoked(targetThreadId)) {
      set({ error: i18n.t('common:runtimeFinalSnapshotUnavailable') })
      return
    }
    // Match Kun's live entry path: the SSE stream must be open before the
    // detail fetch, otherwise a slow /v1/threads/{id} call hides live deltas.
    sseAbortRef.current?.abort()
    sseAbortRef.current = null
    const p = getProvider()
    const prevState = get()
    const keepExistingBlocks = prevState.activeThreadId === targetThreadId
    const knownBoundaryOnlyHistory = prevState.threads.some((thread) => (
      thread.id === targetThreadId && thread.historyAuthority === 'case_boundary_only_v1'
    ))
    resetActiveStream(targetThreadId, {
      turnId: keepExistingBlocks ? prevState.currentTurnId : null,
      liveAssistant: keepExistingBlocks && !knownBoundaryOnlyHistory ? prevState.liveAssistant : '',
      lastSeq: keepExistingBlocks ? prevState.lastSeq : 0
    })
    invalidateThreadDetailCache(targetThreadId)
    resetBusyRecoveryAttempts()
    clearBusyWatchdog()
    set({
      activeThreadId: targetThreadId,
      blocks: keepExistingBlocks && !knownBoundaryOnlyHistory ? prevState.blocks : [],
      lastSeq: keepExistingBlocks ? prevState.lastSeq : 0,
      liveAssistant: keepExistingBlocks && !knownBoundaryOnlyHistory ? prevState.liveAssistant : '',
      unreadThreadIds: { ...prevState.unreadThreadIds, [targetThreadId]: false },
      busy: true,
      currentTurnId: null,
      currentTurnUserId: null,
      turnStartedAtByUserId: {},
      turnDurationByUserId: {},
      inspectorSelectedId: null,
      queuedMessages: [],
      queuedMessagesPausedReason: null
    })
    const ac = new AbortController()
    sseAbortRef.current = ac
    const liveSinceSeq = keepExistingBlocks ? prevState.lastSeq : 0
    const publicSink = buildThreadEventSink(set, get, { threadId: targetThreadId, signal: ac.signal, sinceSeq: liveSinceSeq })
    const sink = knownBoundaryOnlyHistory
      ? {
          ...publicSink,
          onDeltas: (deltas: Parameters<typeof publicSink.onDeltas>[0]) => {
            const seq = deltas.reduce((highest, delta) => (
              typeof delta.seq === 'number' ? Math.max(highest, delta.seq) : highest
            ), 0)
            if (seq > 0) publicSink.onSeq(seq)
          }
        }
      : publicSink
    subscribeThreadEventsWithRecovery(p, targetThreadId, liveSinceSeq, sink, ac.signal, get, sseAbortRef, ac)
    armBusyWatchdog(set, get)
    try {
      const {
        blocks: rawBlocks,
        latestSeq,
        threadStatus,
        latestTurnId,
        latestUserMessageId,
        turnDurationByUserId = {},
        goal,
        todos,
        historyAuthority
      } = await p.getThreadDetail(targetThreadId)
      if (
        ac.signal.aborted ||
        isPublicProjectionRevoked(targetThreadId) ||
        get().activeThreadId !== targetThreadId
      ) return
      const blocks = hydrateBlockModelLabels(targetThreadId, rawBlocks)
      const busy = threadSnapshotLooksRunning(blocks, threadStatus)
      const currentTurnUserId = busy
        ? latestUserMessageId ?? findLatestUserBlockId(blocks)
        : null
      const liveStateBeforeDetail = get()
      const boundaryOnlyHistory = historyAuthority === 'case_boundary_only_v1'
      if (boundaryOnlyHistory) {
        resetActiveStream(targetThreadId, {
          turnId: latestTurnId ?? null,
          liveAssistant: '',
          lastSeq: Math.max(latestSeq, liveStateBeforeDetail.lastSeq)
        })
      }
      if (
        !boundaryOnlyHistory &&
        latestTurnId &&
        isPendingActiveStreamTurnId(liveStateBeforeDetail.currentTurnId) &&
        liveStateBeforeDetail.activeThreadId === targetThreadId
      ) {
        migrateActiveStreamTurn(targetThreadId, liveStateBeforeDetail.currentTurnId, latestTurnId)
      }
      set((s) => {
        if (s.activeThreadId !== targetThreadId || isPublicProjectionRevoked(targetThreadId)) {
          return {}
        }
        const hasNewerLiveState = s.lastSeq > latestSeq
        const liveStateTurnId =
          latestTurnId && isPendingActiveStreamTurnId(s.currentTurnId)
            ? latestTurnId
            : s.currentTurnId
        return {
          activeThreadGoal: boundaryOnlyHistory ? null : goal ?? null,
          activeThreadTodos: boundaryOnlyHistory ? null : todos ?? null,
          blocks: boundaryOnlyHistory
            ? blocks
            : hasNewerLiveState
              ? (keepExistingBlocks && prevState.blocks.length > 0
                ? s.blocks
                : mergePersistedBlocksWithLiveOverlay(blocks, s.blocks))
              : blocks,
          lastSeq: Math.max(latestSeq, s.lastSeq),
          liveAssistant: boundaryOnlyHistory ? '' : hasNewerLiveState ? s.liveAssistant : '',
          busy: boundaryOnlyHistory ? busy : hasNewerLiveState ? s.busy : busy,
          currentTurnId: boundaryOnlyHistory
            ? (busy ? latestTurnId ?? null : null)
            : hasNewerLiveState
              ? liveStateTurnId ?? latestTurnId ?? null
              : busy ? latestTurnId ?? null : null,
          currentTurnUserId: boundaryOnlyHistory
            ? currentTurnUserId
            : hasNewerLiveState
              ? s.currentTurnUserId ?? currentTurnUserId
              : currentTurnUserId,
          turnDurationByUserId: boundaryOnlyHistory
            ? turnDurationByUserId
            : hasNewerLiveState
              ? { ...turnDurationByUserId, ...s.turnDurationByUserId }
              : turnDurationByUserId
        }
      })
      if (!busy && get().queuedMessages.length > 0) {
        void get().drainQueuedMessages()
      }
    } catch (e) {
      if (ac.signal.aborted || isPublicProjectionRevoked(targetThreadId)) return
      set({
        error: formatRuntimeError(e),
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
    }
  },

  drainQueuedMessages: async () => {
    if (drainingQueuedMessages) return
    drainingQueuedMessages = true
    try {
      while (true) {
        const state = get()
        if (state.queuedMessagesPausedReason) return
        const queuedMessages = state.queuedMessages.filter((message) => !message.guiPlan)
        if (queuedMessages.length !== state.queuedMessages.length) {
          set({ queuedMessages })
        }
        const next = queuedMessages[0]
        if (!next || state.busy) return
        if (requiresOrdinaryPublicProjection(next.text)) {
          set((current) => ({
            queuedMessages: current.queuedMessages.filter((message) => message.id !== next.id),
            queuedMessagesPausedReason: 'failed',
            error: i18n.t('common:queuedMessageSensitiveInputBlocked')
          }))
          return
        }
        const started = await get().sendMessage(next.text, next.mode, { queued: next })
        if (!started) {
          set({ queuedMessagesPausedReason: 'failed' })
          return
        }
      }
    } finally {
      drainingQueuedMessages = false
    }
  },

  editQueuedMessage: (id, text) => {
    const trimmed = text.trim()
    if (!trimmed) return
    if (requiresOrdinaryPublicProjection(trimmed)) {
      set({ error: i18n.t('common:queuedMessageSensitiveInputBlocked') })
      return
    }
    set((s) => ({
      queuedMessages: s.queuedMessages.map((message) =>
        message.id === id
          ? {
              ...message,
              text: trimmed,
              displayText: message.displayText && message.displayText !== message.text
                ? message.displayText
                : trimmed
            }
          : message
      )
    }))
  },

  removeQueuedMessage: (id) =>
    set((s) => ({
      queuedMessages: s.queuedMessages.filter((message) => message.id !== id),
      queuedMessagesPausedReason: s.queuedMessages.length <= 1 ? null : s.queuedMessagesPausedReason
    })),

  reorderQueuedMessages: (ids) =>
    set((s) => {
      const order = new Map(ids.map((id, index) => [id, index]))
      const known = s.queuedMessages.filter((message) => order.has(message.id))
      const unknown = s.queuedMessages.filter((message) => !order.has(message.id))
      known.sort((left, right) => (order.get(left.id) ?? 0) - (order.get(right.id) ?? 0))
      return { queuedMessages: [...known, ...unknown] }
    }),

  sendQueuedMessageNow: async (id) => {
    const message = get().queuedMessages.find((item) => item.id === id)
    if (!message) return false
    if (requiresOrdinaryPublicProjection(message.text)) {
      set((state) => ({
        queuedMessages: state.queuedMessages.filter((item) => item.id !== id),
        queuedMessagesPausedReason: 'failed',
        error: i18n.t('common:queuedMessageSensitiveInputBlocked')
      }))
      return false
    }
    if (!get().busy) {
      set({ queuedMessagesPausedReason: null })
      return get().sendMessage(message.text, message.mode, { queued: message })
    }
    const provider = getProvider()
    if (message.guiPlan) {
      set({ queuedMessagesPausedReason: 'failed' })
      return false
    }
    const steered = await steerActiveTurn({
      set,
      get,
      provider,
      draft: message,
      removeQueuedId: id,
      queueOnFailure: true
    })
    if (!steered && !get().queuedMessagesPausedReason) {
      set({ queuedMessagesPausedReason: 'failed' })
    }
    return steered
  },

  resumeInterruptedQueue: () => {
    set({ queuedMessagesPausedReason: null })
    void get().drainQueuedMessages()
  },

  setQueuedMessagesPausedReason: (reason) => set({ queuedMessagesPausedReason: reason }),

  startThreadHandoff: async (request) => {
    if (get().queuedMessages.length > 0) {
      set({ error: i18n.t('common:threadHandoffBlockedQueuedMessages') })
      return false
    }
    if (get().threadHandoffOperations.some((operation) =>
      operation.sourceThreadId === request.sourceThreadId &&
      (operation.status === 'queued' || operation.status === 'running')
    )) {
      set({ error: i18n.t('common:threadHandoffAlreadyRunning') })
      return false
    }
    try {
      const result = await window.analytix.workspace.startThreadHandoff(request)
      set((s) => ({
        activeThreadHandoffOperationId: result.operation.id,
        threadHandoffOperations: upsertThreadHandoffOperation(s.threadHandoffOperations, result.operation),
        error: null
      }))
      return true
    } catch (error) {
      set({ error: formatRuntimeError(error) })
      return false
    }
  },

  retryThreadHandoff: async (operationId) => {
    try {
      const result = await window.analytix.workspace.retryThreadHandoff({ operationId })
      set((s) => ({
        activeThreadHandoffOperationId: result.operation.id,
        threadHandoffOperations: upsertThreadHandoffOperation(s.threadHandoffOperations, result.operation),
        error: null
      }))
      return true
    } catch (error) {
      set({ error: formatRuntimeError(error) })
      return false
    }
  },

  cancelThreadHandoff: async (operationId) => {
    try {
      const result = await window.analytix.workspace.cancelThreadHandoff({ operationId })
      set((s) => ({
        threadHandoffOperations: upsertThreadHandoffOperation(s.threadHandoffOperations, result.operation)
      }))
    } catch (error) {
      set({ error: formatRuntimeError(error) })
    }
  },

  closeThreadHandoffOperation: async (operationId) => {
    const operation = get().threadHandoffOperations.find((item) => item.id === operationId)
    if (operation && !threadHandoffIsTerminal(operation)) {
      set({ activeThreadHandoffOperationId: null })
      return
    }
    try {
      await window.analytix.workspace.removeThreadHandoff({ operationId })
    } catch {
      // The main operation may have been removed after a renderer reload.
    }
    set((s) => ({
      activeThreadHandoffOperationId: s.activeThreadHandoffOperationId === operationId
        ? null
        : s.activeThreadHandoffOperationId,
      threadHandoffOperations: s.threadHandoffOperations.filter((item) => item.id !== operationId)
    }))
  },

  hydrateThreadHandoffOperations: async () => {
    try {
      const result = await window.analytix.workspace.getThreadHandoffOperations()
      set({ threadHandoffOperations: result.operations })
      for (const operation of result.operations) {
        get().handleThreadHandoffEvent(operation)
      }
    } catch {
      // Handoff hydration is best-effort; normal chat boot should not fail.
    }
  },

  handleThreadHandoffEvent: (operation) => {
    set((s) => ({
      activeThreadHandoffOperationId: s.activeThreadHandoffOperationId ?? operation.id,
      threadHandoffOperations: upsertThreadHandoffOperation(s.threadHandoffOperations, operation)
    }))
    if (!threadHandoffNeedsSwitchFinalize(operation) || finalizingThreadHandoffs.has(operation.id)) return
    finalizingThreadHandoffs.add(operation.id)
    ;(async () => {
      const provider = getProvider()
      try {
        if (typeof provider.updateThreadWorkspace !== 'function') {
          throw new Error('Runtime provider does not support thread workspace updates.')
        }
        if (!operation.targetWorkspace) {
          throw new Error('Thread handoff did not resolve a target workspace.')
        }
        await provider.updateThreadWorkspace(operation.sourceThreadId, operation.targetWorkspace)
        if (operation.direction === 'to-worktree' && operation.worktree) {
          saveThreadWorktreeRegistry(
            markThreadWorktree(operation.sourceThreadId, {
              projectPath: operation.sourceWorkspace,
              poolIndex: operation.worktree.poolIndex,
              worktreePath: operation.worktree.path,
              branch: operation.worktree.branch,
              createdAt: new Date().toISOString()
            })
          )
        } else if (operation.direction === 'to-local') {
          saveThreadWorktreeRegistry(
            forgetThreadWorktree(operation.sourceThreadId, readThreadWorktreeRegistry())
          )
        }
        set((s) => ({
          threads: s.threads.map((thread) =>
            thread.id === operation.sourceThreadId
              ? { ...thread, workspace: operation.targetWorkspace ?? thread.workspace }
              : thread
          ),
          codeWorkspaceRoots: rememberCodeWorkspaceRoots(s.codeWorkspaceRoots, [operation.targetWorkspace])
        }))
        const result = await window.analytix.workspace.completeThreadHandoffSwitch({
          operationId: operation.id,
          targetThreadId: operation.sourceThreadId
        })
        set((s) => ({
          threadHandoffOperations: upsertThreadHandoffOperation(s.threadHandoffOperations, result.operation)
        }))
        void get().refreshThreads()
      } catch (error) {
        const message = formatRuntimeError(error)
        const result = await window.analytix.workspace.failThreadHandoffSwitch({
          operationId: operation.id,
          message
        }).catch(() => null)
        set((s) => ({
          error: message,
          ...(result
            ? { threadHandoffOperations: upsertThreadHandoffOperation(s.threadHandoffOperations, result.operation) }
            : {})
        }))
      } finally {
        finalizingThreadHandoffs.delete(operation.id)
      }
    })()
  },

  sendMessage: async (text, mode, overrides) => {
    const trimmedText = text.trim()
    if (!trimmedText) return false
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    const submissionGuard = overrides?.submissionGuard
    if (submissionGuard && !submissionGuard.isCurrent()) return false
    const p = getProvider()
    const hasPendingActiveTurn = threadHasPendingRuntimeWork(get().blocks)
    if (get().busy || hasPendingActiveTurn) {
      // The ordinary queue/steer path does not retain a send-time Core fence.
      // Never accept a transient Canvas scope there as an acknowledged send.
      if (submissionGuard) {
        set({ error: i18n.t('common:runtimeActiveTurn') })
        return false
      }
      if (overrides?.guiPlan) {
        set({ error: i18n.t('common:composerQueuePlaceholder') })
        return false
      }
      const now = Date.now()
      const activeThreadId = get().activeThreadId
      const threadSnap = activeThreadId
        ? get().threads.find((thread) => thread.id === activeThreadId)
        : undefined
      const clawModel = activeClawChannel(get())?.model
      const overrideModel = overrides?.model?.trim()
      const composerModel =
        overrideModel ?? (get().route === 'claw' && clawModel ? clawModel : get().composerModel.trim())
      const composerProviderId =
        overrides?.providerId?.trim() || fallbackComposerProviderIdForSend(get())
      const userModelChip =
        overrides?.modelLabel ?? optimisticUserModelLabel(composerModel, threadSnap?.model)
      const displayText = overrides?.displayText?.trim()
      const reasoningEffort = projectModelReasoningEffortV1(overrides?.reasoningEffort)
      const attachmentIds = overrides?.attachmentIds?.filter((id) => id.trim().length > 0)
      const attachments = projectAttachmentReferencesForPublicSurfaces(
        overrides?.attachments?.filter((attachment) => attachment.id.trim().length > 0) ?? []
      )
      const fileReferences = overrides?.fileReferences?.filter((reference) =>
        reference.path.trim() && reference.relativePath.trim() && reference.name.trim()
      )
      const draft: MidTurnSteerDraft = {
        text: trimmedText,
        ...(displayText ? { displayText } : {}),
        ...(mode ? { mode } : {}),
        ...(composerModel ? { model: composerModel } : {}),
        ...(composerProviderId ? { providerId: composerProviderId } : {}),
        ...(userModelChip ? { modelLabel: userModelChip } : {}),
        ...(reasoningEffort ? { reasoningEffort } : {}),
        ...(overrides?.guiPlan ? { guiPlan: overrides.guiPlan } : {}),
        ...(attachmentIds?.length ? { attachmentIds } : {}),
        ...(attachments?.length ? { attachments } : {}),
        ...(fileReferences?.length ? { fileReferences } : {})
      }
      if (!overrides?.guiPlan) {
        const steered = await steerActiveTurn({
          set,
          get,
          provider: p,
          draft,
          queueOnFailure: false
        })
        if (steered) return true
      }
      if (requiresOrdinaryPublicProjection(trimmedText)) {
        set({ error: i18n.t('common:queuedMessageSensitiveInputBlocked') })
        return false
      }
      set((s) => ({
        queuedMessages: [
          ...s.queuedMessages,
          queuedMessageFromSteerDraft(draft, s.queuedMessages.length, now)
        ],
        error: null
      }))
      // UI/runtime can briefly drift (busy=false while runtime still has an active turn).
      // Kick recovery so queued input drains as soon as the in-flight turn settles.
      if (!get().busy && hasPendingActiveTurn) {
        void get().recoverActiveTurn()
      }
      return true
    }
    const receiptOwner = Symbol('send-receipt')
    currentReceiptOwner = receiptOwner
    const now = Date.now()
    const queued = overrides?.queued
    const userBlockId = queued?.id ?? `u-${now}`
    const attachmentIds =
      queued?.attachmentIds ??
      overrides?.attachmentIds?.filter((id) => id.trim().length > 0) ??
      []
    const attachments = projectAttachmentReferencesForPublicSurfaces(
      queued?.attachments ??
      overrides?.attachments?.filter((attachment) => attachment.id.trim().length > 0) ??
      []
    )
    const fileReferences =
      queued?.fileReferences ??
      overrides?.fileReferences?.filter((reference) =>
        reference.path.trim() && reference.relativePath.trim() && reference.name.trim()
      ) ??
      []
    let activeThreadId = get().activeThreadId
    const rawDisplayText = queued?.displayText ?? overrides?.displayText?.trim() ?? trimmedText
    const displayText = projectOrdinaryPublicText(rawDisplayText)
    const userDisplayText = displayText !== projectOrdinaryPublicText(trimmedText) ? displayText : undefined
    const threadSnap = get().threads.find((thread) => thread.id === activeThreadId)
    const clawModel = activeClawChannel(get())?.model
    const overrideModel = overrides?.model?.trim()
    const composerModel =
      queued?.model ?? overrideModel ?? (get().route === 'claw' && clawModel ? clawModel : get().composerModel.trim())
    const composerProviderId =
      queued?.providerId ?? overrides?.providerId?.trim() ?? fallbackComposerProviderIdForSend(get())
    const reasoningEffort = projectModelReasoningEffortV1(queued?.reasoningEffort ?? overrides?.reasoningEffort)
    const userModelChip =
      queued?.modelLabel ?? overrides?.modelLabel ?? optimisticUserModelLabel(composerModel, threadSnap?.model)
    const previousBlocks = get().blocks
    const previousActiveThreadId = get().activeThreadId
    const previousLastSeq = get().lastSeq
    const previousCurrentTurnId = get().currentTurnId
    const previousCurrentTurnUserId = get().currentTurnUserId
    const previousTurnStartedAtByUserId = get().turnStartedAtByUserId
    const previousTurnDurationByUserId = get().turnDurationByUserId
    const previousQueuedMessages = get().queuedMessages
    const previousQueuedMessagesPausedReason = get().queuedMessagesPausedReason
    let provisionalTurnId = activeThreadId ? pendingActiveStreamTurnId(activeThreadId, userBlockId) : null
    resetBusyRecoveryAttempts()
    set((s) => ({
      busy: true,
      blocks: [
        ...s.blocks,
        {
          kind: 'user' as const,
          id: userBlockId,
          createdAt: new Date(now).toISOString(),
          text: displayText,
          ...(userModelChip ? { modelLabel: userModelChip } : {}),
          ...(provisionalTurnId || userDisplayText || attachmentIds.length || attachments.length || fileReferences.length
            ? {
                meta: {
                  ...(provisionalTurnId ? { turnId: provisionalTurnId } : {}),
                  ...(userDisplayText ? { displayText: userDisplayText } : {}),
                  ...(attachmentIds.length ? { attachmentIds } : {}),
                  ...(attachments.length ? { attachments } : {}),
                  ...(fileReferences.length ? { fileReferences } : {})
                }
              }
            : {})
        }
      ],
      liveAssistant: '',
      error: null,
      currentTurnId: provisionalTurnId ?? s.currentTurnId,
      currentTurnUserId: userBlockId,
      turnStartedAtByUserId: { ...s.turnStartedAtByUserId, [userBlockId]: now },
      queuedMessages: queued ? s.queuedMessages.filter((message) => message.id !== queued.id) : s.queuedMessages,
      queuedMessagesPausedReason: queued && s.queuedMessages.length <= 1 ? null : s.queuedMessagesPausedReason
    }))
    if (activeThreadId && provisionalTurnId) {
      resetActiveStream(activeThreadId, {
        turnId: provisionalTurnId,
        lastSeq: previousLastSeq,
        startedAt: now
      })
    }
    if (!activeThreadId) {
      try {
        const settings = await rendererRuntimeClient.getSettings()
        const workspaceRoot = normalizeWorkspaceRoot(get().workspaceRoot) || normalizeWorkspaceRoot(settings.workspaceRoot)
        if (!workspaceRoot) {
          set({
            blocks: previousBlocks,
            busy: false,
            currentTurnId: previousCurrentTurnId,
            currentTurnUserId: previousCurrentTurnUserId,
            turnStartedAtByUserId: previousTurnStartedAtByUserId,
            turnDurationByUserId: previousTurnDurationByUserId,
            queuedMessages: previousQueuedMessages,
            queuedMessagesPausedReason: previousQueuedMessagesPausedReason,
            error: i18n.t('common:workspaceRequiredToCreateThread')
          })
          return false
        }
        const codeWorkspaceRoots = rememberCodeWorkspaceRoots(get().codeWorkspaceRoots, [workspaceRoot])
        set({ codeWorkspaceRoots })
        const reusableThreadId = await findReusableEmptyThreadId(
          get(),
          p,
          workspaceRoot,
          (thread) => isCodeThread(thread, get().clawChannels)
        )
        const createdThread =
          reusableThreadId == null
            ? await p.createThread({
                workspace: workspaceRoot,
                autoTitle: true,
                mode: mode ?? 'agent',
                ...(composerModel ? { model: composerModel } : {}),
                ...(composerProviderId ? { providerId: composerProviderId } : {})
              })
            : null
        const threadId = reusableThreadId ?? createdThread?.id ?? null
        if (!threadId) {
          throw new Error('Failed to resolve target thread id.')
        }
        activeThreadId = threadId
        provisionalTurnId = pendingActiveStreamTurnId(activeThreadId, userBlockId)
        if (composerModel) {
          rememberThreadComposerSelection(threadId, composerModel, composerProviderId)
        }
        set((s) => ({
          activeThreadId: threadId,
          currentTurnId: provisionalTurnId,
          codeWorkspaceRoots: rememberCodeWorkspaceRoots(s.codeWorkspaceRoots, [workspaceRoot, createdThread?.workspace]),
          lastSeq: 0,
          inspectorSelectedId: null,
          threads:
            createdThread && !s.threads.some((thread) => thread.id === createdThread.id)
              ? [createdThread, ...s.threads]
              : s.threads
        }))
        resetActiveStream(threadId, {
          turnId: provisionalTurnId,
          lastSeq: 0,
          startedAt: now
        })
        void get().refreshThreads()
      } catch (e) {
        void window.analytix.logs.error('create-thread', 'Failed to create thread', {
          message: e instanceof Error ? e.message : String(e)
        }).catch(() => undefined)
        set({
          activeThreadId: previousActiveThreadId,
          blocks: previousBlocks,
          lastSeq: previousLastSeq,
          busy: false,
          currentTurnId: previousCurrentTurnId,
          currentTurnUserId: previousCurrentTurnUserId,
          turnStartedAtByUserId: previousTurnStartedAtByUserId,
          turnDurationByUserId: previousTurnDurationByUserId,
          queuedMessages: previousQueuedMessages,
          queuedMessagesPausedReason: previousQueuedMessagesPausedReason,
          error: formatRuntimeError(e),
          ...(shouldOpenSettingsForError(e)
            ? { route: 'settings' as const, settingsSection: 'agents' as const }
            : {})
        })
        if (activeThreadId && provisionalTurnId) clearActiveStream(activeThreadId, provisionalTurnId)
        return false
      }
    }
    clearBusyWatchdog()
    invalidateThreadDetailCache(activeThreadId)
    let submissionSubscription: AbortController | null = null
    let receiptSubscription: AbortController | null = null
    let acknowledged = false
    const ownsReceiptContext = (): boolean => currentReceiptOwner === receiptOwner &&
      get().activeThreadId === activeThreadId && get().runtimeConnection === 'ready' &&
      receiptSubscription !== null && !receiptSubscription.signal.aborted &&
      (sseAbortRef.current === receiptSubscription || sseAbortRef.current === null)
    const ownsRunningReceipt = (receipt?: { turnId: string; userMessageItemId?: string }): boolean => {
      const state = get()
      if (!ownsReceiptContext() || !state.busy) return false
      // SSE may already have replaced both optimistic IDs before HTTP returns.
      // Conversely, an idle snapshot or a different stable turn is not ours.
      const matchingTurn = state.currentTurnId === provisionalTurnId ||
        (receipt !== undefined && state.currentTurnId === receipt.turnId)
      const matchingUser = state.currentTurnUserId === userBlockId ||
        (receipt?.userMessageItemId !== undefined && state.currentTurnUserId === receipt.userMessageItemId) ||
        (receipt !== undefined && receipt.userMessageItemId === undefined &&
          state.currentTurnId === receipt.turnId && state.blocks.some(block =>
            block.kind === 'user' && block.id === state.currentTurnUserId && block.meta?.turnId === receipt.turnId))
      return matchingTurn && matchingUser
    }
    const refreshReceiptThreads = async (): Promise<void> => {
      const snapshot = get()
      const isCurrent = () => ownsReceiptContext() && get().busy === snapshot.busy &&
        get().currentTurnId === snapshot.currentTurnId && get().currentTurnUserId === snapshot.currentTurnUserId
      if (isCurrent()) await get().refreshThreads({ isCurrent })
    }
    const cancelPreparedSubmission = (): void => {
      const ownsState = ownsRunningReceipt()
      submissionSubscription?.abort()
      if (sseAbortRef.current === submissionSubscription) sseAbortRef.current = null
      if (ownsState && activeThreadId && provisionalTurnId) clearActiveStream(activeThreadId, provisionalTurnId)
      // A later thread/turn owns its own state. Remove only our unsent optimistic
      // block and timing entry, preserving other events that arrived meanwhile.
      set(state => {
        if (!ownsState) return state
        const { [userBlockId]: _started, ...started } = state.turnStartedAtByUserId
        const { [userBlockId]: _duration, ...durations } = state.turnDurationByUserId
        return { blocks: state.blocks.filter(block => block.id !== userBlockId), busy: false,
          currentTurnId: previousCurrentTurnId, currentTurnUserId: previousCurrentTurnUserId,
          turnStartedAtByUserId: started, turnDurationByUserId: durations }
      })
    }
    try {
      const seqAtSend = get().lastSeq
      if (!sseAbortRef.current || sseAbortRef.current.signal.aborted) {
        const ac = new AbortController()
        submissionSubscription = ac
        sseAbortRef.current = ac
        const sink = buildThreadEventSink(set, get, { threadId: activeThreadId, signal: ac.signal, sinceSeq: seqAtSend })
        subscribeThreadEventsWithRecovery(p, activeThreadId, seqAtSend, sink, ac.signal, get, sseAbortRef, ac)
      }
      receiptSubscription = sseAbortRef.current
      const channel = get().route === 'claw' ? activeClawChannel(get()) : null
      if (!channel && composerModel) {
        rememberThreadComposerSelection(activeThreadId, composerModel, composerProviderId)
      }
      const settingsPromise = rendererRuntimeClient.getSettings()
      let workspaceCheckpointId: string | undefined
      const checkpointThread = get().threads.find((thread) => thread.id === activeThreadId)
      const threadCheckpointWorkspaceRoot = normalizeWorkspaceRoot(checkpointThread?.workspace)
      let checkpointPromise = threadCheckpointWorkspaceRoot
        ? createWorkspaceCheckpointForSend({
            workspaceRoot: threadCheckpointWorkspaceRoot,
            threadId: activeThreadId
          })
        : null
      const settings = await settingsPromise
      const settingsCheckpointWorkspaceRoot = normalizeWorkspaceRoot(settings.workspaceRoot)
      const checkpointWorkspaceRoot = threadCheckpointWorkspaceRoot || settingsCheckpointWorkspaceRoot
      if (!checkpointPromise && checkpointWorkspaceRoot) {
        checkpointPromise = createWorkspaceCheckpointForSend({
          workspaceRoot: checkpointWorkspaceRoot,
          threadId: activeThreadId
        })
      }
      if (checkpointPromise && checkpointWorkspaceRoot) {
        const checkpoint = await checkpointPromise
        if (checkpoint.ok) {
          workspaceCheckpointId = checkpoint.checkpointId
        } else if (
          checkpoint.reason !== 'not_git_repo' &&
          checkpoint.reason !== 'no_workspace' &&
          checkpoint.reason !== 'git_unavailable' &&
          checkpoint.reason !== 'timeout'
        ) {
          void window.analytix.logs.error('git-checkpoint', 'Failed to create Git checkpoint', {
            message: checkpoint.message,
            reason: checkpoint.reason,
            workspaceRoot: checkpointWorkspaceRoot
          }).catch(() => undefined)
        }
      }
      let runtimeText: string
      if (channel) {
        runtimeText = buildClawRuntimePrompt(settings, trimmedText, { channel })
      } else {
        runtimeText = buildCodeRuntimePrompt(settings, trimmedText)
      }
      const runtimeDisplayText = channel ? displayText : (userDisplayText ?? trimmedText)
      if (submissionGuard) {
        let admitted = false
        try { admitted = await submissionGuard.validateBeforeSend() } catch { /* fail closed */ }
        if (!admitted || !submissionGuard.isCurrent() || get().activeThreadId !== activeThreadId || get().runtimeConnection !== 'ready') {
          cancelPreparedSubmission()
          return false
        }
      }
      const { turnId, userMessageItemId } = await p.sendUserMessage(activeThreadId, runtimeText, {
        mode,
        ...(composerModel ? { model: composerModel } : {}),
        ...(!channel && composerProviderId ? { providerId: composerProviderId } : {}),
        ...(reasoningEffort ? { reasoningEffort } : {}),
        ...(runtimeDisplayText ? { displayText: runtimeDisplayText } : {}),
        ...((queued?.guiPlan ?? overrides?.guiPlan) ? { guiPlan: queued?.guiPlan ?? overrides?.guiPlan } : {}),
        ...(attachmentIds.length ? { attachmentIds } : {}),
        ...(workspaceCheckpointId ? { workspaceCheckpointId } : {}),
        ...(fileReferences.length ? { fileReferences } : {})
      })
      acknowledged = true
      const receipt = { turnId, userMessageItemId }
      // Mirror the composer model selection against the runtime's stable
      // user_message item id so the badge survives page refresh / thread
      // re-selection. The runtime itself doesn't persist per-turn metadata.
      if (userMessageItemId && userModelChip) {
        rememberTurnModel(activeThreadId, userMessageItemId, userModelChip)
      }
      // Acknowledgement still clears the original composer snapshot, even when
      // navigation (including A -> B -> A) or completion revoked UI ownership.
      // Fence stream migration too: it can otherwise replace the latest bucket.
      if (!ownsRunningReceipt(receipt)) return true
      if (provisionalTurnId && isPendingActiveStreamTurnId(provisionalTurnId)) {
        migrateActiveStreamTurn(activeThreadId, provisionalTurnId, turnId)
      }
      const stableUserBlockId = userMessageItemId ?? get().currentTurnUserId ?? userBlockId
      if (stableUserBlockId && stableUserBlockId !== userBlockId) {
        set((s) => ({
          blocks: reconcileOptimisticUserBlock(
            s.blocks,
            userBlockId,
            stableUserBlockId,
            displayText,
            userModelChip
          ).map((block) =>
            block.kind === 'user' && block.id === stableUserBlockId
              ? {
                  ...block,
                  meta: {
                    ...(block.meta ?? {}),
                    turnId,
                    ...(workspaceCheckpointId ? { workspaceCheckpointId } : {})
                  }
                }
              : block
          ),
          currentTurnUserId: s.currentTurnUserId === userBlockId ? stableUserBlockId : s.currentTurnUserId,
          turnStartedAtByUserId: (() => {
            if (s.turnStartedAtByUserId[userBlockId] === undefined) return s.turnStartedAtByUserId
            const next = { ...s.turnStartedAtByUserId, [stableUserBlockId]: s.turnStartedAtByUserId[userBlockId] }
            delete next[userBlockId]
            return next
          })(),
          turnDurationByUserId: (() => {
            if (s.turnDurationByUserId[userBlockId] === undefined) return s.turnDurationByUserId
            const next = { ...s.turnDurationByUserId, [stableUserBlockId]: s.turnDurationByUserId[userBlockId] }
            delete next[userBlockId]
            return next
          })(),
        }))
      }
      set((s) => ({
        blocks: s.blocks.map((block) =>
          block.kind === 'user' && block.id === stableUserBlockId
            ? {
                ...block,
                meta: {
                  ...(block.meta ?? {}),
                  turnId,
                  ...(workspaceCheckpointId ? { workspaceCheckpointId } : {})
                }
              }
            : block
        ),
        currentTurnId: turnId
      }))
      if (channel && typeof window.analytix?.connectPhone?.mirrorChannelMessage === 'function') {
        const userMirror = await window.analytix.connectPhone.mirrorChannelMessage(
          activeThreadId,
          trimmedText,
          'user'
        )
        if (!ownsRunningReceipt(receipt)) return true
        if (userMirror.ok) {
          rememberPendingClawFeishuMirror(turnId, {
            threadId: activeThreadId,
            userBlockId: userMessageItemId ?? userBlockId,
            userText: trimmedText
          })
        }
      }
      if (!ownsRunningReceipt(receipt)) return true
      if (!sseAbortRef.current || sseAbortRef.current.signal.aborted) {
        const ac = new AbortController()
        receiptSubscription = ac
        sseAbortRef.current = ac
        const sink = buildThreadEventSink(set, get, { threadId: activeThreadId, signal: ac.signal, sinceSeq: seqAtSend })
        subscribeThreadEventsWithRecovery(p, activeThreadId, seqAtSend, sink, ac.signal, get, sseAbortRef, ac)
      }
      armBusyWatchdog(set, get)
      await refreshReceiptThreads()
      return true
    } catch (e) {
      // Post-acknowledgement housekeeping cannot turn a sent message into a
      // failed draft or roll back a turn that SSE has already established.
      if (acknowledged) return true
      if (!ownsRunningReceipt()) return false
      clearBusyWatchdog()
      void window.analytix.logs.error('send-message', 'Failed to send message', {
        message: e instanceof Error ? e.message : String(e),
        threadId: activeThreadId
      }).catch(() => undefined)
      if (looksLikeActiveTurnError(e)) {
        if (activeThreadId && provisionalTurnId) clearActiveStream(activeThreadId, provisionalTurnId)
        set({
          blocks: previousBlocks,
          busy: false,
          currentTurnId: previousCurrentTurnId,
          currentTurnUserId: previousCurrentTurnUserId,
          turnStartedAtByUserId: previousTurnStartedAtByUserId,
          turnDurationByUserId: previousTurnDurationByUserId,
          queuedMessages: previousQueuedMessages,
          error: i18n.t('common:runtimeActiveTurn')
        })
        await get().recoverActiveTurn()
        await refreshReceiptThreads()
        return false
      }
      if (activeThreadId && provisionalTurnId) clearActiveStream(activeThreadId, provisionalTurnId)
      set({
        error: formatRuntimeError(e),
        busy: false,
        currentTurnId: previousCurrentTurnId,
        currentTurnUserId: previousCurrentTurnUserId,
        turnStartedAtByUserId: previousTurnStartedAtByUserId,
        turnDurationByUserId: previousTurnDurationByUserId,
        queuedMessages: previousQueuedMessages,
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      await refreshReceiptThreads()
      return false
    } finally {
      if (currentReceiptOwner === receiptOwner) currentReceiptOwner = null
    }
  },

  reviewActiveThread: async (target: ReviewTarget) => {
    if (get().runtimeConnection !== 'ready') {
      set({ error: i18n.t('common:runtimeActionNeedsConnection') })
      return false
    }
    const p = getProvider()
    if (typeof p.reviewThread !== 'function') {
      set({ error: i18n.t('common:reviewUnavailable') })
      return false
    }
    if (get().busy || threadHasPendingRuntimeWork(get().blocks)) {
      set({ error: i18n.t('common:composerQueuePlaceholder') })
      return false
    }
    let activeThreadId = get().activeThreadId
    try {
      if (!activeThreadId) {
        const settings = await rendererRuntimeClient.getSettings()
        const workspaceRoot = normalizeWorkspaceRoot(settings.workspaceRoot)
        if (!workspaceRoot) {
          set({ error: i18n.t('common:workspaceRequiredToCreateThread') })
          return false
        }
        const codeWorkspaceRoots = rememberCodeWorkspaceRoots(get().codeWorkspaceRoots, [workspaceRoot])
        set({ codeWorkspaceRoots })
        const reusableThreadId = await findReusableEmptyThreadId(
          get(),
          p,
          workspaceRoot,
          (thread) => isCodeThread(thread, get().clawChannels)
        )
        const createdThread =
          reusableThreadId == null
            ? await p.createThread({
                workspace: workspaceRoot,
                title: i18n.t('common:slashCommandReviewTitle'),
                mode: 'agent',
                ...(get().composerModel.trim() ? { model: get().composerModel.trim() } : {}),
                ...(get().composerProviderId.trim() ? { providerId: get().composerProviderId.trim() } : {})
              })
            : null
        activeThreadId = reusableThreadId ?? createdThread?.id ?? null
        if (!activeThreadId) throw new Error('Failed to resolve target thread id.')
        set((s) => ({
          activeThreadId,
          codeWorkspaceRoots: rememberCodeWorkspaceRoots(s.codeWorkspaceRoots, [workspaceRoot, createdThread?.workspace]),
          lastSeq: 0,
          inspectorSelectedId: null,
          threads:
            createdThread && !s.threads.some((thread) => thread.id === createdThread.id)
              ? [createdThread, ...s.threads]
              : s.threads
        }))
      }
      const threadSnap = get().threads.find((thread) => thread.id === activeThreadId)
      const composerModel = get().composerModel.trim()
      const composerProviderId = get().composerProviderId.trim()
      const userModelChip = optimisticUserModelLabel(composerModel, threadSnap?.model)
      const seqAtSend = get().lastSeq
      resetBusyRecoveryAttempts()
      sseAbortRef.current?.abort()
      sseAbortRef.current = null
      clearBusyWatchdog()
      set({
        busy: true,
        liveAssistant: '',
        error: null,
        currentTurnId: null,
        currentTurnUserId: null
      })
      const { turnId, userMessageItemId } = await p.reviewThread(activeThreadId, target, {
        ...(composerModel ? { model: composerModel } : {}),
        ...(composerProviderId ? { providerId: composerProviderId } : {})
      })
      if (userMessageItemId && userModelChip) {
        rememberTurnModel(activeThreadId, userMessageItemId, userModelChip)
      }
      set({ currentTurnId: turnId })
      const ac = new AbortController()
      sseAbortRef.current = ac
      const sink = buildThreadEventSink(set, get, { threadId: activeThreadId, signal: ac.signal, sinceSeq: seqAtSend })
      subscribeThreadEventsWithRecovery(p, activeThreadId, seqAtSend, sink, ac.signal, get, sseAbortRef, ac)
      armBusyWatchdog(set, get)
      await get().refreshThreads()
      return true
    } catch (e) {
      clearBusyWatchdog()
      set({
        error: formatRuntimeError(e),
        busy: false,
        currentTurnId: null,
        currentTurnUserId: null,
        ...(shouldOpenSettingsForError(e)
          ? { route: 'settings' as const, settingsSection: 'agents' as const }
          : {})
      })
      await get().refreshThreads()
      return false
    }
  },
  }
}
