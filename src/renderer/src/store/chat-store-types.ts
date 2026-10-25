import type {
	  AttachmentReference,
	  ChatBlock,
  AcceptedFinalProjectionReceiptV1,
  AcceptedFinalProjectionTerminalV1,
  GeneralTerminalProjectionReceiptV1,
	  NormalizedCaseProject,
	  NormalizedThread,
	  RuntimeConnectionStatus,
	  CaseProjectIndexStatus,
	  ReviewTarget,
  ThreadGoal,
  ThreadGoalStatus,
  ThreadExecutionPolicy,
  ThreadTodoList,
  ThreadTodoStatus,
  ThreadUsageSnapshot,
  TurnUsageById,
  UserFileReference,
  UserInputSubmission
} from '../agent/types'
import type { AnalytixRuntimeStatusPayload } from '@shared/analytix-api'
import type { ThreadHandoffOperation, ThreadHandoffRequest } from '@shared/thread-handoff'
import type {
  ClawImAgentProfileV1,
  ClawImChannelV1,
  ClawImPlatformAccountV1,
  ClawImProvider,
  ClawImSettingsV1,
  ClawModel
} from '@shared/app-settings'
import type { ModelProviderModelGroup } from '@shared/analytix-api'
import type { ModelReasoningEffort } from '@shared/app-settings'
import type { ComposerDraft } from './composer-drafts'

/** Unconfirmed Question save metadata only; original bytes stay in the private leaf/Host. */
export type UserInputDraftInterruption = { revision: string | null; unsent: boolean }

/** A completed renderer selection; its owner can never be revived by ID reuse. */
export type ThreadSelectionReceipt = { threadId: string; isCurrent: () => boolean }

/** Renderer owner fence; a committed endSeq also binds the Host event prefix. */
export type TurnForkBoundary = {
  threadId: string
  turnId: string
  finalItemId: string
  status: 'completed' | 'aborted'
  /** Only a committed terminal receipt supplies this cursor; never a Host CAS. */
  endSeq?: number
  proof: { kind: 'general'; receipt: GeneralTerminalProjectionReceiptV1 } |
    { kind: 'accepted'; receipt: AcceptedFinalProjectionReceiptV1; terminal: AcceptedFinalProjectionTerminalV1 } | null
}
export type TurnForkSnapshot = {
  blocks: ChatBlock[]
  busy: boolean
  latestSeq: number
  title?: string
  rewoundTurnIds?: Record<string, true>
}
export type TurnForkOwner = {
  isCurrent: () => boolean
  getSnapshot: () => TurnForkSnapshot | null
}
export type TurnForkSource = TurnForkBoundary & TurnForkOwner

export type QueuedMessagePauseReason = 'interrupted' | 'failed' | null

export type QueuedUserMessage = {
  threadId: string
  /** A local receipt owns this entry while dispatch is pending. Never persisted. */
  sending?: boolean
  id: string
  text: string
  displayText?: string
  requestedSkillIds?: string[]
  mode?: string
  model?: string
  providerId?: string
  modelLabel?: string
  reasoningEffort?: ModelReasoningEffort
  attachmentIds?: string[]
  attachments?: AttachmentReference[]
  fileReferences?: UserFileReference[]
  /**
   * Optional GUI plan context forwarded to Analytix. The renderer
   * attaches it for plan/refine turns so the runtime can advertise
   * the native `create_plan` tool and gate the write to the reserved
   * plan artifact.
   */
  guiPlan?: {
    operation: 'draft' | 'refine'
    workspaceRoot: string
    relativePath: string
    planId: string
    sourceRequest?: string
    title?: string
  }
}

/**
 * GUI plan context attached to a send-message call. Mirrors the
 * Analytix `GuiPlanContextSchema` and is forwarded to the runtime
 * request body so plan/refine turns are scoped to a reserved path.
 */
export type GuiPlanMessageContext = {
  operation: 'draft' | 'refine'
  workspaceRoot: string
  relativePath: string
  planId: string
  sourceRequest?: string
  title?: string
}

export type SendMessageOverrides = {
  /** One renderer submission; steering remains best-effort and Host-authorized. */
  deliveryIntent?: 'queue' | 'steer'
  /** Renderer-local, one-submission fence. Core remains the authority. Never
   * serialize this callback, forward it to a Provider, or retain it in a queue. */
  submissionGuard?: {
    isCurrent: () => boolean
    validateBeforeSend: () => Promise<boolean>
  }
  queued?: QueuedUserMessage
  model?: string
  providerId?: string
  modelLabel?: string
  reasoningEffort?: ModelReasoningEffort
  displayText?: string
  requestedSkillIds?: string[]
  guiPlan?: GuiPlanMessageContext
  attachmentIds?: string[]
  attachments?: AttachmentReference[]
  fileReferences?: UserFileReference[]
}

export type InitialSetupMode = 'required' | 'preview'
export type SettingsRouteSection =
  | 'account'
  | 'general'
  | 'providers'
  | 'write'
  | 'imageGeneration'
  | 'mediaGeneration'
  | 'speechToText'
  | 'agents'
  | 'archives'
  | 'permissions'
  | 'browser'
  | 'computerUse'
  | 'worktree'
  | 'memory'
  | 'skill'
  | 'mcp'
  | 'shortcuts'

  | 'claw'
  | 'updates'
  | 'debug'
export type AppRoute = 'chat' | 'write' | 'settings' | 'plugins' | 'claw' | 'schedule'
export type PluginHostRoute = 'chat' | 'claw'
export type TopNoticeTone = 'info' | 'success' | 'error'

export type TopNotice = {
  id: string
  tone: TopNoticeTone
  message: string
  durationMs?: number
}

/**
 * A side conversation ("by-the-way") running alongside the active
 * thread. It owns its own timeline, composer, busy state, and SSE
 * subscription so it can stream in parallel with the main thread.
 *
 * The slice is namespaced under `sideConversations[threadId]` and
 * MUST NOT mutate any main-thread state (`activeThreadId`, `blocks`,
 * `busy`, etc.) — isolation is structural.
 */
export type SideConversation = {
  threadId: string
  parentThreadId: string
  title: string
  source?: 'side' | 'background-agent'
  createdAt: string
  /** Timestamp the snapshot was taken from the parent. */
  inheritedAt: string
  blocks: ChatBlock[]
  liveAssistant: string
  lastSeq: number
  input: string
  model: string
  reasoningEffort: ModelReasoningEffort
  busy: boolean
  turnId: string | null
  userItemId: string | null
  error: string | null
  /** Usage committed atomically with the latest accepted-final side batch. */
  lastTurnUsage?: ThreadUsageSnapshot | null
  /** Verified terminal usage indexed by the original runtime turn, within this side owner. */
  turnUsageById?: TurnUsageById
  /** Host-proven removals fence in-flight history within this owner. */
  rewoundTurnIds?: Record<string, true>
}

export type SidePanelState = {
  open: boolean
  activeSideId: string | null
}

export type ChatState = {
  route: AppRoute
  settingsReturnRoute: Exclude<AppRoute, 'settings'>
  pluginHostRoute: PluginHostRoute
  settingsSection: SettingsRouteSection
  initialSetupOpen: boolean
  initialSetupMode: InitialSetupMode
  workspaceRoot: string
  workspaceLabel: string
  runtimeConnection: RuntimeConnectionStatus
  runtimeStatus: AnalytixRuntimeStatusPayload | null
		  codeWorkspaceRoots: string[]
		  caseProjects: NormalizedCaseProject[]
		  caseProjectIndexStatus: CaseProjectIndexStatus
		  caseProjectThreadsById: Record<string, NormalizedThread[]>
	  caseProjectLoadingById: Record<string, boolean>
	  caseProjectErrorsById: Record<string, string | null>
	  caseProjectExpandedById: Record<string, boolean>
	  threads: NormalizedThread[]
  threadSearch: string
  threadSearchResults: { query: string; threads: NormalizedThread[] } | null
  showArchivedThreads: boolean
  activeThreadId: string | null
  activeThreadGoal: ThreadGoal | null
  activeGoalActivation?: import('../../../../packages/runtime/src/contracts/goal').GoalActivationV1 | null
  goalStateEpoch?: number
  activeThreadTodos: ThreadTodoList | null
  blocks: ChatBlock[]
  liveAssistant: string
  lastSeq: number
  usageRefreshKey: number
  /**
   * Latest turn's usage snapshot, tagged with the thread it belongs to. Used by
   * the context-capacity gauge: the last turn's prompt tokens ≈ what currently
   * occupies the window. Null until a live turn reports usage.
   */
  lastTurnUsage: { threadId: string; snapshot: ThreadUsageSnapshot } | null
  /** Display projection only; each entry retains its original thread and batch cursor. */
  turnUsageById: TurnUsageById
  /** Cleared on thread-owner changes; never inferred from a rejected candidate. */
  rewoundTurnIds?: Record<string, true>
  busy: boolean
  error: string | null
  topNotice: TopNotice | null
  runtimeErrorDetail: string | null
  currentTurnId: string | null
  currentTurnUserId: string | null
  turnStartedAtByUserId: Record<string, number>
  turnDurationByUserId: Record<string, number>
  inspectorSelectedId: string | null
  composerModel: string
  pendingGateResolutionIds: Record<string, true>
  userInputPanel?: { threadId: string; requestId: string | null; view?: 'review' }
  selectUserInputPanel: (requestId: string | null, view?: 'review') => void
  userInputDraftInterruptions?: Record<string, UserInputDraftInterruption>
  deletedUserInputDraftThreadIds?: Record<string, true>
  setUserInputDraftInterruption: (key: string, value: UserInputDraftInterruption | null) => void
  composerDrafts: Record<string, ComposerDraft>
  updateComposerDraft: (key: string, update: (draft: ComposerDraft) => ComposerDraft) => void
  composerProviderId: string
  composerPickList: string[]
  composerModelGroups: ModelProviderModelGroup[]
  disabledSkillIds: string[]
  queuedMessages: QueuedUserMessage[]
  queuedMessagePauses: Record<string, Exclude<QueuedMessagePauseReason, null>>
  threadHandoffOperations: ThreadHandoffOperation[]
  activeThreadHandoffOperationId: string | null
  watchTurnCompletion: Record<string, boolean>
  unreadThreadIds: Record<string, boolean>
  /**
   * Side conversations opened via `/btw`. The main thread selection
   * and subscription are never touched by these entries.
   */
  sideConversations: Record<string, SideConversation>
  sidePanel: SidePanelState
  clawChannels: ClawImChannelV1[]
  activeClawChannelId: string
  appendLocalClawTurn: (userText: string, replyText: string) => void
  setError: (message: string | null) => void
  showTopNotice: (notice: { tone?: TopNoticeTone; message: string; durationMs?: number; id?: string }) => void
  dismissTopNotice: (id?: string) => void
  setComposerModel: (modelId: string, providerId?: string) => void
  loadComposerModels: () => Promise<void>
  setRoute: (r: AppRoute) => void
  openWrite: () => Promise<void>
  openCode: () => Promise<void>
  openSettings: (section?: SettingsRouteSection) => void
  openPlugins: (host?: PluginHostRoute) => void
  openClaw: () => void
  openSchedule: () => void
  refreshClawChannels: () => Promise<void>
  addClawChannel: (
    provider: ClawImProvider,
    agentProfile?: Partial<ClawImAgentProfileV1>,
    platformAccount?: ClawImPlatformAccountV1,
    options?: {
      channelId?: string
      model?: string
      workspaceRoot?: string
      enabled?: boolean
      im?: Partial<ClawImSettingsV1>
      preserveRoute?: boolean
      mainFinalized?: boolean
    }
  ) => Promise<void>
  selectClawChannel: (channelId: string) => Promise<void>
  selectClawConversation: (channelId: string, threadId: string) => Promise<void>
  deleteClawChannel: (channelId: string) => Promise<void>
  resetClawChannelSession: (channelId: string) => Promise<void>
  setClawChannelModel: (channelId: string, model: string) => Promise<void>
  openInitialSetup: (mode?: InitialSetupMode) => void
  closeInitialSetup: () => void
  boot: () => Promise<void>
  probeRuntime: (mode?: 'user' | 'background', options?: { restart?: boolean }) => Promise<void>
  chooseWorkspace: (options?: { createThreadAfter?: boolean; selectThreadAfter?: boolean }) => Promise<string | null>
  selectWorkspaceRoot: (workspaceRoot: string) => Promise<string | null>
  clearWorkspace: () => Promise<void>
	  deleteWorkspace: (workspacePath: string) => Promise<void>
	  refreshCaseProjects: (options?: { isCurrent?: () => boolean }) => Promise<void>
	  loadCaseProjectThreads: (caseProjectId: string, options?: { force?: boolean; isCurrent?: () => boolean }) => Promise<void>
	  setCaseProjectExpanded: (caseProjectId: string, expanded: boolean) => void
	  refreshThreads: (options?: { isCurrent?: () => boolean }) => Promise<void>
  setThreadSearch: (query: string) => void
  setShowArchivedThreads: (show: boolean) => void
  createThread: (options?: {
    workspaceRoot?: string
    forceNew?: boolean
    /** Internal callers that need a persisted thread before the first prompt. */
    materialize?: boolean
    /** Local Word ownership needs Core connectivity, without model credentials. */
    localDocument?: boolean
    /** Renderer intake may only select while its originating draft is current. */
    isCurrent?: () => boolean
    onSelected?: (receipt: ThreadSelectionReceipt) => void
    beforeSelect?: (target: { threadId: string; workspace: string }) => boolean
    /** When true, acquire a worktree pool slot as the thread workspace. */
    useWorktreePool?: boolean
  }) => Promise<void>
  selectThread: (id: string, options?: { onSelected?: (receipt: ThreadSelectionReceipt) => void; localDocument?: boolean; isCurrent?: () => boolean }) => Promise<void>
  /**
   * Subscribe to a live thread immediately before fetching persisted history.
   * Used by Connect Phone streaming so deltas are visible without waiting for
   * the detail request round trip.
   */
  subscribeThreadEventsLive: (threadId: string) => Promise<void>
  recoverActiveTurn: () => Promise<boolean>
  sendMessage: (text: string, mode?: string, overrides?: SendMessageOverrides) => Promise<boolean>
  reviewActiveThread: (target: ReviewTarget) => Promise<boolean>
  drainQueuedMessages: (threadId?: string) => Promise<void>
  editQueuedMessage: (threadId: string, id: string, text: string) => void
  removeQueuedMessage: (threadId: string, id: string) => void
  reorderQueuedMessages: (threadId: string, ids: string[]) => void
  sendQueuedMessageNow: (threadId: string, id: string) => Promise<boolean>
  resumeInterruptedQueue: (threadId: string) => void
  setQueuedMessagesPausedReason: (reason: QueuedMessagePauseReason, threadId?: string) => void
  startThreadHandoff: (request: ThreadHandoffRequest) => Promise<boolean>
  retryThreadHandoff: (operationId: string) => Promise<boolean>
  cancelThreadHandoff: (operationId: string) => Promise<void>
  closeThreadHandoffOperation: (operationId: string) => Promise<void>
  hydrateThreadHandoffOperations: () => Promise<void>
  handleThreadHandoffEvent: (operation: ThreadHandoffOperation) => void
  /** Local payload handoff only; not a Core rewind or send acknowledgement. */
  rewindAndResend: (userBlockId: string, newText: string, onCaptured?: () => void) => Promise<void>
  rollbackWorkspaceToCheckpoint: (checkpointId: string) => Promise<void>
  interrupt: (options?: { discard?: boolean }) => Promise<void>
  renameActiveThread: (title: string) => Promise<void>
  renameThread: (threadId: string, title: string) => Promise<void>
  updateThreadExecutionPolicy: (threadId: string, policy: ThreadExecutionPolicy) => Promise<boolean>
  archiveThread: (threadId: string, archived: boolean) => Promise<void>
  compactActiveThread: (reason?: string) => Promise<void>
  forkActiveThread: (expected?: ThreadSelectionReceipt) => Promise<void>
  forkThreadFromTurn: (turnId: string, source?: TurnForkSource) => Promise<void>
  setActiveThreadGoal: (
    objective: string,
    options?: { research?: boolean; requirements?: string[]; maxGoalRounds?: number }
  ) => Promise<boolean>
  setActiveThreadGoalStatus: (status: ThreadGoalStatus) => Promise<boolean>
  clearActiveThreadGoal: () => Promise<boolean>
  setActiveThreadTodoStatus: (todoId: string, status: ThreadTodoStatus) => Promise<boolean>
  clearActiveThreadTodos: () => Promise<boolean>
  syncPlanTodosFromMarkdown: (
    plan: { id: string; relativePath: string },
    markdown: string
  ) => Promise<boolean>
  /**
   * Spawn a side conversation from the active thread. Available even
   * while the active thread is running. Does not change `activeThreadId`.
   * If `seedText` is provided, immediately sends it as the first turn.
   */
  spawnSideConversation: (seedText?: string) => Promise<string | null>
  /**
   * Open the side chat surface without creating an underlying side
   * thread. The first draft send will create the side thread.
   */
  openSideConversationDraft: () => void
  sendSideMessage: (sideId: string, text: string) => Promise<boolean>
  interruptSide: (sideId: string) => Promise<void>
  setSideInput: (sideId: string, text: string) => void
  setSideModel: (sideId: string, model: string) => void
  setSideReasoningEffort: (sideId: string, effort: ModelReasoningEffort) => void
  selectSideConversation: (sideId: string) => void
  openThreadInSidePanel: (
    threadId: string,
    options?: { parentThreadId?: string; title?: string; source?: 'side' | 'background-agent' }
  ) => Promise<boolean>
  setSidePanelOpen: (open: boolean) => void
  closeSideConversation: (sideId: string) => Promise<void>
  discardSideConversation: (sideId: string) => Promise<void>
  promoteSideConversation: (sideId: string) => Promise<void>
  resumeSessionIntoThread: (
    sessionId: string,
    options?: { model?: string; mode?: string }
  ) => Promise<string | null>
  deleteThread: (threadId: string) => Promise<void>
  resolveApproval: (blockId: string, decision: 'allow' | 'deny') => Promise<void>
  resolveUserInput: (
    target: { itemId: string; requestId: string; threadId: string | null; turnId: string | null },
    action: { kind: 'submit'; answers: UserInputSubmission[] } | { kind: 'cancel' } | { kind: 'late'; answers: UserInputSubmission[]; clientUserMessageId: string; expectedTurnId: string }
  ) => Promise<void>
  selectInspectorItem: (id: string | null) => void
  applyI18nFromSettings: (locale: 'en' | 'zh') => Promise<void>
  reloadUiSettings: () => Promise<void>
}

export type ChatStoreSet = (
  partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)
) => void

export type ChatStoreGet = () => ChatState
