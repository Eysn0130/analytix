import type {
  CoreAttachmentContentResponseJson,
  CoreAttachmentMetadataJson,
  CoreAttachmentTextFallbackJson,
  CoreCheckpointRewindApplyResultJson,
  CoreCheckpointRewindPlanJson,
  CoreMemoryDiagnosticsJson,
  CoreMemoryRecordJson,
  CoreRuntimeInfoJson,
  CoreRuntimeSkillsResponseJson,
  CoreRuntimeToolDiagnosticsJson,
  CoreModelExecutionRefJson,
  CoreModelExecutionSourceJson,
  CoreCaseProjectDetailResponseJson,
  CoreAcceptedFinalPublicViewJson,
  CoreAcceptedFinalTerminalReasonJson,
  CoreThreadSummaryResponseJson,
  CoreThreadSummaryTaskMutationResponseJson,
  CoreThreadSummaryTaskOutputResponseJson
} from './analytix-contract'
import type { ApprovalPolicy, SandboxMode } from '@shared/app-settings'
import type { ModelReasoningEffort as SharedModelReasoningEffort } from '@shared/app-settings'
import type { PublicProjectionRevokedEvent } from '../../../../packages/runtime/src/contracts/events.js'
import type { ModelReasoningEffort } from '../../../../packages/runtime/src/contracts/capabilities.js'

export type ToolItemKind = 'tool_call' | 'command_execution' | 'file_change' | 'subagent'
export type RuntimeErrorSeverity = 'info' | 'warning' | 'error'

export type AttachmentReference = {
  id: string
  kind?: 'image' | 'document'
  name?: string
  mimeType?: string
  byteSize?: number
  width?: number
  height?: number
  pageCount?: number
  documentText?: string
  textPreview?: string
  truncated?: boolean
  previewUrl?: string
  localFilePath?: string
  FilePath?: string
}

export type GeneratedFileReference = {
  id?: string
  name?: string
  mimeType?: string
  byteSize?: number
  width?: number
  height?: number
  previewUrl?: string
  path?: string
  relativePath?: string
  absolutePath?: string
  localFilePath?: string
  FilePath?: string
}

export type UserFileReference = {
  path: string
  relativePath: string
  name: string
  kind?: 'file' | 'directory'
}

export type RuntimeChildMetadata = {
  kind?: string
  parentThreadId: string
  parentTurnId: string
  parentToolCallId?: string
  childId: string
  childRunId?: string
  childThreadId?: string
  childTurnId?: string
  childLabel?: string
  childName?: string
  childStatus: 'queued' | 'running' | 'pause_requested' | 'paused' | 'resume_requested' | 'resuming' | 'completed' | 'failed' | 'aborted' | 'interrupted' | 'killed'
  childSeq?: number
  childModel?: string
  childProviderId?: string
  childEndpointFormat?: string
  childModelVariant?: string
  childModelSource?: CoreModelExecutionSourceJson
  childModelExecution?: CoreModelExecutionRefJson
  childEffort?: ModelReasoningEffort
  effort?: ModelReasoningEffort
  childProfile?: string
  childProfileMode?: string
  childProfileDescription?: string
  childProfileColor?: string
  childProfileIcon?: string
  childToolPolicy?: string
  jobId?: string
  returnFormat?: 'summary' | 'evidence' | 'transcriptRef'
  tokenBudget?: number
  timeBudgetMs?: number
  budgetExceeded?: boolean
  evidenceBundleStatus?: 'parsed' | 'not_found' | string
  evidenceCount?: number
  continueFrom?: string
  forkFrom?: string
  prefixReused?: boolean
  inheritedHistoryItems?: number
  toolInvocations?: number
  evidenceLedgered?: boolean
  durationMs?: number
  queuedMs?: number
  totalTokens?: number
  cachedTokens?: number
  cacheHitTokens?: number
  cacheMissTokens?: number
  cacheHitRate?: number | null
  cacheableTokenHitRate?: number | null
  totalInputTokenHitRate?: number | null
  costUsd?: number
  costCny?: number
  priceConfigured?: boolean
  cacheSavingsUsd?: number
  cacheSavingsCny?: number
  tokenEconomySavingsTokens?: number
  tokenEconomySavingsUsd?: number
  tokenEconomySavingsCny?: number
  background?: boolean
  heartbeatStatus?: string
  lastHeartbeatAt?: string
  heartbeatAgeMs?: number
  leaseOwner?: string
  leaseExpiresAt?: string
  leaseExpired?: boolean
  staleAfterMs?: number
  stalled?: boolean
  orphaned?: boolean
  recoveryStatus?: string
  recoveryAttempt?: number
  recoveryUpdatedAt?: string
  deliveryId?: string
  deliveryStatus?: string
  deliveryItemId?: string
  completionDeliveryAt?: string
  completionDeadLetterAt?: string
  completionDeliveryAttempt?: number
  parallelGroupId?: string
  parallelIndex?: number
  steerMessageId?: string
  steerStatus?: 'queued' | 'admitted' | 'rejected' | 'expired' | string
  pendingSteers?: number
  admittedSteers?: number
  steerCount?: number
  canAcceptSteer?: boolean
  lastSteerAt?: string
  pauseRequestId?: string
  pauseStatus?: 'requested' | 'paused' | 'rejected' | 'expired' | 'resumed' | 'resume_requested' | string
  canPause?: boolean
  canResume?: boolean
  paused?: boolean
  pauseCount?: number
  pauseRequestedAt?: string
  lastPausedAt?: string
  lastResumedAt?: string
  resumeTokenIssuedAt?: string
  resumeTokenExpiresAt?: string
  isolationMode?: 'worktree' | string
  worktreePath?: string
  worktreeBranch?: string
  baseCommit?: string
  currentCommit?: string
  mergeStatus?: 'not_requested' | string
  mergeDecisionId?: string
  mergeDecision?: string
  mergeDecisionCreatedAt?: string
  cleanupReceiptId?: string
  cleanupAcceptDecisionId?: string
  cleanupRemoved?: boolean
  cleanupRetainedReason?: string
  acceptDecisionId?: string
  acceptApprovalId?: string
  appliedPatchDigest?: string
  conflictReportId?: string
  parentHeadAtConflict?: string
  childHeadAtConflict?: string
  sourcePatchDigest?: string
  conflictSummary?: string
  conflictFileCount?: number
  conflictFiles?: Array<{ path: string; status?: string }>
  repairReviewId?: string
  repairConflictReportId?: string
  repairPatchDigest?: string
  repairExpectedParentHead?: string
  repairDryRunStatus?: string
  repairRejectedReason?: string
  repairChangedFileCount?: number
  repairChangedFiles?: Array<{ path: string; status?: string }>
  repairDecisionId?: string
  repairApprovalId?: string
  repairAppliedPatchDigest?: string
  childTodoListId?: string
  childTodoScope?: 'child' | 'projection' | string
  childTodoCount?: number
  childTodoCompletedCount?: number
  childTodoInProgressCount?: number
  childTodoPendingCount?: number
  childTodoFailedCount?: number
  childTodoBlockedCount?: number
  childTodoCanceledCount?: number
  childTodoProjectionId?: string
  childTodoProjectionStatus?: 'proposed' | 'accepted' | 'rejected' | 'superseded' | string
  childTodoProjectionSummary?: string
  childTodoProjectionItemCount?: number
  childTodoProjectionEvidenceCount?: number
  childTodoProjectionMappedCount?: number
  childTodoProjectionCompletedMappedCount?: number
  childTodoProjectionDecisionId?: string
  childTodoProjectionDecision?: 'accepted' | 'rejected' | string
  childTodoProjectionApprovalId?: string
  childTodoProjectionAcceptedItemCount?: number
  childTodoProjectionSkippedItemCount?: number
  childTodoProjectionAcceptedAt?: string
  childTodoProjectionRejectedAt?: string
  diffSummary?: string
  changedFileCount?: number
  changedFiles?: Array<{ path: string; status?: string }>
}

export type WebCitationSource = {
  sourceId?: string
  url?: string
  title?: string
  retrievedAt?: string
}

export type RuntimeDisclosureMetadata = {
  turnId?: string
  displayText?: string
  delivery?: 'steer'
  clientUserMessageId?: string
  steeringStatus?: 'pending' | 'admitted' | 'accepted'
  admittedSeq?: number
  attachmentIds?: string[]
  attachments?: AttachmentReference[]
  fileReferences?: UserFileReference[]
  workspaceCheckpointId?: string
  generatedFiles?: GeneratedFileReference[]
  activeSkillIds?: string[]
  injectedMemoryIds?: string[]
  skillInjectionBytes?: number
  child?: RuntimeChildMetadata
  diagnostics?: RuntimeJobDiagnosticsMetadata
  providerError?: RuntimeProviderErrorDiagnosticsMetadata
  providerRetry?: RuntimeProviderRetryDiagnosticsMetadata
  providerRecovery?: RuntimeProviderRecoveryDiagnosticsMetadata
  sources?: WebCitationSource[]
}

export type RuntimeProviderRecoveryDiagnosticsMetadata = {
  kind?: string
  attempt?: number
  maxAttempt?: number
  recoveryExhausted?: boolean
}

export type RuntimeProviderRetryDiagnosticsMetadata = {
  attempt?: number
  maxAttempt?: number
}

export type RuntimeProviderErrorDiagnosticsMetadata = {
  providerId?: string
  family?: string
  endpointFormat?: string
  status?: number
  kind?: string
  hasApiKey?: boolean
  authStatus?: string
  retryable?: boolean
  attempt?: number
  failureStage?:
    | 'request_validation'
    | 'request_build'
    | 'pre_send_body_audit'
    | 'telemetry_begin'
    | 'transport_before_observed_send'
    | 'transport_after_observed_send'
    | 'local_admission_config'
    | 'callback_projection'
    | 'unclassified'
  dispatchState?: 'not_sent' | 'sent' | 'indeterminate'
}

/** Renderer-safe cache telemetry: hashes, counters and booleans only. */
export type RuntimeCacheDiagnosticsMetadata = {
  prefixHash?: string
  prefixChanged?: boolean
  toolSourceChanged?: boolean
  systemHash?: string
  modeHash?: string
  prefixItemsHash?: string
  toolsHash?: string
  toolSchemaTokens?: number
  toolCount?: number
  toolSourcesHash?: string
  cacheTelemetrySupported?: boolean
  firstTokenLatencyMs?: number
  durationMs?: number
  cacheHitTokens?: number
  cacheMissTokens?: number
}

export type RuntimeJobNotificationKind =
  | 'background_job_completion'
  | 'background_job_auto_continue'
  | 'background_job_delivery'
  | 'thread_summary_subagent'
  | 'thread_summary_task'

export type RuntimeJobKindCode =
  | 'task'
  | 'parallel_task'
  | 'planner'
  | 'background-shell'
  | 'bash'
  | 'subagent'
  | 'child-run'
  | 'parallel-child-run'
  | 'command'
  | 'process'
  | 'unknown'

export type RuntimeJobStatusCode =
  | 'queued'
  | 'running'
  | 'pause_requested'
  | 'paused'
  | 'resume_requested'
  | 'resuming'
  | 'completed'
  | 'failed'
  | 'aborted'
  | 'interrupted'
  | 'killed'
  | 'canceled'
  | 'timeout'
  | 'starting'
  | 'pending'
  | 'delivered'
  | 'retry'
  | 'recovering'
  | 'recovered'
  | 'dead_letter'
  | 'dead_lettered'
  | 'skipped'
  | 'stopped'
  | 'missing'
  | 'archived'
  | 'unknown'

export type RuntimeJobDiagnosticsMetadata = {
  notificationKind?: RuntimeJobNotificationKind
  jobId?: string
  childRunId?: string
  childThreadId?: string
  childTurnId?: string
  kind?: RuntimeJobKindCode
  status?: RuntimeJobStatusCode
  terminal?: boolean
  background?: boolean
  canContinueParent?: boolean
  lateCompletionSuppressed?: boolean
  autoContinueParent?: boolean
  autoContinueStatus?: 'pending' | 'started' | 'skipped' | 'failed' | 'completed'
  autoContinueTurnId?: string
  deliveryId?: string
  deliveryStatus?: 'pending' | 'retry' | 'delivered' | 'dead_letter' | 'failed'
  deliveryItemId?: string
  completionDeliveryAttempt?: number
  parentThreadId?: string
  parentTurnId?: string
  ageMs?: number
  idleMs?: number
  heartbeatStatus?: 'running' | 'healthy' | 'stale' | 'stalled' | 'lease_expired' | 'paused' | 'completed' | 'recovered' | 'unknown'
  stalled?: boolean
  staleAfterMs?: number
  stalledAfterMs?: number
  heartbeatAgeMs?: number
  leaseExpired?: boolean
  orphaned?: boolean
  recoveryStatus?: 'pending' | 'recovering' | 'recovered' | 'failed' | 'dead_lettered'
  recoveryAttempt?: number
  paused?: boolean
  pauseStatus?: 'pause_requested' | 'paused' | 'resume_requested' | 'resuming' | 'running' | 'failed'
  pauseRequestId?: string
  warningCode?: 'task_job_stalled' | 'task_job_stale' | 'slow-output'
}

/** Host-projected tool metadata; raw arguments/output/diagnostic text is forbidden. */
export type ToolBlockMeta = Record<string, unknown> & {
  rewindPlan?: CoreCheckpointRewindPlanJson
}

export type UserInputOption = {
  label: string
  description: string
}

export type UserInputQuestion = {
  header: string
  id: string
  question: string
  options: UserInputOption[]
}

export type UserInputAnswer = {
  id: string
  label: string
  value: string
}

export type NormalizedThread = {
  id: string
  title: string
  updatedAt: string
  model: string
  providerId?: string
  mode: string
  workspace?: string
  status?: string
  approvalPolicy?: ApprovalPolicy
  sandboxMode?: SandboxMode
  archived?: boolean
  preview?: string
  messageCount?: number
  turnCount?: number
  historyAuthority?: 'case_boundary_only_v1'
  latestTurnId?: string
  latestTurnStatus?: string
  relation?: 'primary' | 'fork' | 'side'
  parentThreadId?: string
  forkedFromThreadId?: string
  forkedFromTitle?: string
  forkedAt?: string
  forkedFromMessageCount?: number
  forkedFromTurnCount?: number
  goal?: ThreadGoal | null
  todos?: ThreadTodoList | null
}

export type NormalizedCaseProject = {
  id: string
  name: string
  rootPath: string
  updatedAt: string
  threadCount: number
  runningCount: number
  archivedCount: number
  lastThreadId?: string
  lastPreview?: string
  dataSizeEstimate?: number
  status?: string
}

export type CaseProjectIndexStatus = 'ready' | 'building'

export type NormalizedCaseProjectListResult = {
  caseProjects: NormalizedCaseProject[]
  indexStatus: CaseProjectIndexStatus
}

export type ThreadGoalStatus =
  | 'active'
  | 'paused'
  | 'blocked'
  | 'usageLimited'
  | 'budgetLimited'
  | 'complete'

export type ThreadGoalEvidenceEntry = {
  id: string
  turnId?: string
  toolCallId?: string
  requirementId?: string
  step: string
  evidence: string[]
  summary?: string
  createdAt: string
}

export type ThreadGoalResearch = {
  enabled: true
  stateRelativePath: string
  taskSpecPath: string
  progressPath: string
  findingsPath: string
  directionsTriedPath: string
  iterationLogPath: string
  requirementCount: number
}

export type ThreadGoal = {
  threadId: string
  objective: string
  status: ThreadGoalStatus
  tokenBudget?: number | null
  tokensUsed: number
  timeUsedSeconds: number
  evidenceLedger?: ThreadGoalEvidenceEntry[]
  research?: ThreadGoalResearch
  blockedReason?: string
  blockedCount?: number
  blockedTurnId?: string
  strictCompletion?: boolean
  selfCheckRequired?: boolean
  selfCheckCompleted?: boolean
  selfCheckTurnId?: string
  createdAt: string
  updatedAt: string
}

export type ThreadTodoStatus = 'pending' | 'in_progress' | 'completed' | 'failed' | 'canceled'
export type ThreadTodoStatusReasonCode =
  | 'execution_failed'
  | 'dependency_failed'
  | 'verification_failed'
  | 'timeout'
  | 'tool_failed'
  | 'subagent_failed'
  | 'runtime_failed'
  | 'user_canceled'
  | 'parent_canceled'
  | 'runtime_canceled'
  | 'superseded'

export type ThreadPlanTodoSource = {
  kind: 'plan'
  planId?: string
  relativePath?: string
  ordinal?: number
  contentHash?: string
}

export type ThreadChildTodoSource = {
  kind: 'child'
  parentThreadId?: string
  childThreadId?: string
  childRunId?: string
  jobId?: string
  projectionId?: string
}

export type ThreadManualTodoSource = {
  kind: 'manual'
}

export type ThreadTodoSource = ThreadPlanTodoSource | ThreadChildTodoSource | ThreadManualTodoSource

export type ThreadTodoItem = {
  id: string
  content: string
  status: ThreadTodoStatus
  statusReasonCode?: ThreadTodoStatusReasonCode
  source?: ThreadTodoSource
  note?: string
  createdAt: string
  updatedAt: string
}

export type ThreadTodoList = {
  threadId: string
  turnId?: string
  items: ThreadTodoItem[]
  updatedAt: string
}

export type RuntimeConnectionStatus = 'idle' | 'checking' | 'ready' | 'offline'

export type ThreadListOptions = {
  limit?: number
  search?: string
  includeArchived?: boolean
  archivedOnly?: boolean
  includeSide?: boolean
  summary?: boolean
}

export type ToolBlock = {
  kind: 'tool'
  id: string
  createdAt?: string
  summary: string
  status: 'running' | 'success' | 'error'
  toolKind?: ToolItemKind
  /** Closed host message or validated public tool-result projection only. */
  detail?: string
  /** Resolved file path for file_change items, when known */
  filePath?: string
  /** Optional structured metadata, e.g. { exit_code, duration_ms, command } */
  meta?: ToolBlockMeta
}

export type CompactionBlock = {
  kind: 'compaction'
  id: string
  createdAt?: string
  summary: string
  status: 'running' | 'success' | 'error'
  detail?: string
  auto?: boolean
  messagesBefore?: number
  messagesAfter?: number
  meta?: RuntimeDisclosureMetadata
}

export type ReviewTarget =
  | { kind: 'uncommittedChanges' }
  | { kind: 'baseBranch'; branch: string }
  | { kind: 'commit'; sha: string }
  | { kind: 'custom'; instructions: string }

export type ReviewFinding = {
  title: string
  body: string
  confidenceScore: number
  priority: number
  codeLocation: {
    absoluteFilePath: string
    lineRange: { start: number; end: number }
  }
}

export type ReviewOutput = {
  findings: ReviewFinding[]
  overallCorrectness: 'patch is correct' | 'patch is incorrect'
  overallExplanation: string
  overallConfidenceScore: number
}

export type ReviewBlock = {
  kind: 'review'
  id: string
  createdAt?: string
  title: string
  status: 'running' | 'success' | 'error'
  target?: ReviewTarget
  reviewText?: string
  output?: ReviewOutput
}

export type ChatBlock =
  | {
      kind: 'user'
      id: string
      createdAt?: string
      text: string
      modelLabel?: string
      managedBy?: 'claw'
      meta?: RuntimeDisclosureMetadata
    }
  | {
      kind: 'assistant'
      id: string
      createdAt?: string
      text: string
      acceptedFinalView?: CoreAcceptedFinalPublicViewJson
      /**
       * Renderer-local proof that this exact accepted-final batch was
       * committed to the store before the main process was ACKed. The batch
       * id is authority-bound to the complete answer/error/usage/terminal
       * payload; this value is never synthesized from visible assistant text.
       */
      acceptedFinalProjectionReceipt?: AcceptedFinalProjectionReceiptV1
      acceptedFinalProjectionTerminal?: AcceptedFinalProjectionTerminalV1
      meta?: RuntimeDisclosureMetadata
    }
  | ToolBlock
  | CompactionBlock
  | ReviewBlock
  | {
      kind: 'system'
      id: string
      createdAt?: string
      text: string
      code?: string
      detail?: string
      severity?: RuntimeErrorSeverity
      meta?: RuntimeDisclosureMetadata
    }
  | {
      kind: 'approval'
      id: string
      createdAt?: string
      approvalId: string
      summary: string
      toolName?: string
      status: 'pending' | 'allowed' | 'denied' | 'error'
      errorMessage?: string
      meta?: RuntimeDisclosureMetadata
    }
  | {
      kind: 'user_input'
      id: string
      createdAt?: string
      requestId: string
      questions: UserInputQuestion[]
      status: 'pending' | 'submitted' | 'cancelled' | 'error'
      answers?: UserInputAnswer[]
      errorMessage?: string
      meta?: RuntimeDisclosureMetadata
    }

export type ApprovalRequestPayload = {
  approvalId: string
  summary: string
  toolName?: string
  turnId?: string
  meta?: RuntimeDisclosureMetadata
}

export type ApprovalStatusPayload = {
  approvalId: string
  itemId: string
  status: 'allowed' | 'denied' | 'error'
  errorMessage?: string
}

export type ToolEventPayload = {
  itemId: string
  turnId?: string
  summary: string
  status: 'running' | 'success' | 'error'
  toolKind?: ToolItemKind
  detail?: string
  filePath?: string
  meta?: Record<string, unknown>
}

export type RuntimeStatusEventPayload = {
  kind:
    | 'tool_result_upload_wait'
    | 'tool_catalog_changed'
    | 'tool_storm_suppressed'
    | 'pipeline_stage'
    | 'compaction_summary_fallback'
  itemId: string
  turnId?: string
  createdAt?: string
  message?: string
  toolResultCount?: number
  changeKind?: 'additive' | 'breaking'
  toolName?: string
  callId?: string
  stage?: string
  label?: string
  attempt?: number
  maxAttempt?: number
  child?: RuntimeChildMetadata
  meta?: Record<string, unknown>
}

export type RuntimeErrorEventPayload = {
  itemId: string
  createdAt?: string
  message: string
  code?: string
  details?: unknown
  severity?: RuntimeErrorSeverity
}

export type CompactionEventPayload = {
  itemId: string
  turnId?: string
  seq?: number
  summary: string
  status: 'running' | 'success' | 'error'
  detail?: string
  auto?: boolean
  messagesBefore?: number
  messagesAfter?: number
  createdAt?: string
  meta?: RuntimeDisclosureMetadata
}

export type ReviewEventPayload = {
  itemId: string
  createdAt?: string
  title: string
  status: 'running' | 'success' | 'error'
  target?: ReviewTarget
  reviewText?: string
  output?: ReviewOutput
}

export type UserInputRequestPayload = {
  itemId: string
  requestId: string
  questions: UserInputQuestion[]
  turnId?: string
  meta?: RuntimeDisclosureMetadata
}

export type UserInputStatusPayload = {
  itemId: string
  requestId?: string
  status: 'submitted' | 'cancelled' | 'error'
  answers?: UserInputAnswer[]
  errorMessage?: string
}

export type UserMessageEventPayload = {
  itemId: string
  turnId?: string
  createdAt?: string
  text: string
  modelLabel?: string
  managedBy?: 'claw'
  meta?: RuntimeDisclosureMetadata
}

export type TurnStartedEventPayload = {
  threadId?: string
  turnId: string
  createdAt?: string
  seq?: number
}

export type TurnCompletedEventPayload = {
  threadId?: string
  turnId?: string
  createdAt?: string
  seq?: number
  acceptedFinalDigest?: string
  terminalReason?: CoreAcceptedFinalTerminalReasonJson
}

export type SnapshotRequiredEventPayload = {
  threadId?: string
  createdAt?: string
  seq?: number
  sinceSeq?: number
  highestSeq?: number
  replayEventCount?: number
  reason?: string
}

export type TurnSteeredEventPayload = {
  turnId?: string
  createdAt?: string
  text?: string
  clientUserMessageId?: string
  admittedSeq?: number
}

export type ThreadDeltaEvent = {
  text: string
  kind: 'agent_message'
  seq?: number
  turnId?: string
}

export type ThreadErrorOptions = {
  terminal?: boolean
  threadId?: string
  turnId?: string
  seq?: number
  status?: 'failed' | 'aborted'
  acceptedFinalDigest?: string
  terminalReason?: CoreAcceptedFinalTerminalReasonJson
}

/** Cumulative usage/cost for a Analytix thread. */
export type ThreadUsageSnapshot = {
  model?: string
  providerId?: string
  effort?: ModelReasoningEffort
  usageSource?: string
  childRunId?: string
  cacheDiagnostics?: RuntimeCacheDiagnosticsMetadata
  inputTokens: number
  outputTokens: number
  reasoningTokens: number
  cachedTokens: number
  cacheMissTokens: number
  cacheHitRate: number | null
  cacheableTokenHitRate?: number | null
  totalInputTokenHitRate?: number | null
  totalTokens: number
  costUsd: number | null
  costCny: number | null
  priceConfigured: boolean
  cacheSavingsUsd?: number
  cacheSavingsCny?: number
  tokenEconomySavingsTokens: number
  turns: number
}

/**
 * Renderer projection of one main-process-verified accepted-final delivery
 * batch. The assistant answer, usage, terminal state, and cursor are committed
 * by the store as one indivisible state transition.
 */
export type AcceptedFinalProjectionBatch = {
  batchId: string
  threadId: string
  turnId: string
  publicationCommitId: string
  firstSeq: number
  lastSeq: number
  receipt: AcceptedFinalProjectionReceiptV1
  assistant: Extract<ChatBlock, { kind: 'assistant' }>
  terminalError?: Extract<ChatBlock, { kind: 'system' }>
  usage: ThreadUsageSnapshot
  terminal: AcceptedFinalProjectionTerminalV1
}

export type AcceptedFinalProjectionTerminalV1 = {
  status: 'completed' | 'failed' | 'aborted'
  createdAt: string
  acceptedFinalDigest: string
  terminalReason: CoreAcceptedFinalTerminalReasonJson
}

/**
 * Renderer projection of one ordinary terminal transport batch. Unlike an
 * accepted-final projection it carries no receipt and grants no evidence,
 * citation, fact-answer, or publication authority.
 */
export type GeneralTerminalProjectionBatch = {
  batchDigest: string
  threadId: string
  turnId: string
  firstSeq: number
  lastSeq: number
  terminalItem?: Extract<ChatBlock, { kind: 'assistant' | 'system' }>
  usage: ThreadUsageSnapshot
  terminal: GeneralTerminalProjectionTerminalV1
}

export type GeneralTerminalProjectionTerminalV1 = {
  status: 'completed' | 'failed' | 'aborted'
  createdAt: string
  terminalReason: CoreAcceptedFinalTerminalReasonJson
}

/**
 * Versioned renderer commit receipt. It deliberately mirrors the exact fields
 * required by the privileged accepted-final SSE ACK. `batchId` is the signed
 * identity of the complete sealed delivery, so matching only assistant text is
 * never sufficient for replay or acknowledgement.
 */
export type AcceptedFinalProjectionReceiptV1 = {
  schemaVersion: 1
  batchId: string
  threadId: string
  turnId: string
  publicationCommitId: string
  lastSeq: number
}

export type ThreadEventSink = {
  onSeq(seq: number): void
  onDeltas(deltas: ThreadDeltaEvent[]): void
  onTurnStarted?(ev: TurnStartedEventPayload): void
  onTurnSteered?(ev: TurnSteeredEventPayload): void
  onUserMessage(ev: UserMessageEventPayload): void
  onTool(ev: ToolEventPayload): void
  onCompaction(ev: CompactionEventPayload): void
  onReview?(ev: ReviewEventPayload): void
  onApproval(req: ApprovalRequestPayload): void
  onApprovalStatus?(ev: ApprovalStatusPayload): void
  onUserInput(req: UserInputRequestPayload): void
  onUserInputStatus(ev: UserInputStatusPayload): void
  onRuntimeStatus?(ev: RuntimeStatusEventPayload): void
  onRuntimeError?(ev: RuntimeErrorEventPayload): void
  onThreadLifecycle?(ev: { threadId: string; title?: string; status?: string; createdAt?: string; seq?: number }): void
  onThreadRewound?(ev: {
    threadId: string
    turnId: string
    removedTurns: number
    remainingTurns: number
    removedTurnIds: string[]
    seq?: number
  }): void
  onGoal(ev: { threadId: string; goal: ThreadGoal | null; cleared?: boolean; createdAt?: string }): void
  onTodos?(ev: { threadId: string; todos: ThreadTodoList | null; cleared?: boolean; createdAt?: string }): void
  onSnapshotRequired?(ev: SnapshotRequiredEventPayload): void | Promise<void>
  onPublicProjectionRevoked?(ev: PublicProjectionRevokedEvent): void | Promise<void>
  onAcceptedFinalBatch?(
    batch: AcceptedFinalProjectionBatch
  ): AcceptedFinalProjectionReceiptV1 | Promise<AcceptedFinalProjectionReceiptV1>
  onGeneralTerminalBatch?(batch: GeneralTerminalProjectionBatch): void | Promise<void>
  onTurnComplete(ev?: TurnCompletedEventPayload): void | Promise<void>
  onError(err: Error, options?: ThreadErrorOptions): void | Promise<void>
  /** Optional: cumulative usage update for the thread. */
  onUsage?(usage: ThreadUsageSnapshot): void
}

export interface AgentProvider {
  readonly id: 'analytix'
  readonly displayName: string
  getCapabilities(): {
    interrupt: boolean
    stream: boolean
    approvals: boolean
    attachFiles: boolean
    review?: boolean
  }
  connect(): Promise<void>
  listThreads(options?: ThreadListOptions): Promise<NormalizedThread[]>
  listCaseProjects?(limit?: number): Promise<NormalizedCaseProject[] | NormalizedCaseProjectListResult>
  listCaseProjectThreads?(caseProjectId: string, limit?: number): Promise<NormalizedThread[]>
  getCaseProjectDetail?(caseProjectId: string, limit?: number): Promise<{
    project: NormalizedCaseProject | null
    threads: NormalizedThread[]
    raw: CoreCaseProjectDetailResponseJson
  }>
  createThread(input: { workspace?: string; title?: string; autoTitle?: boolean; mode?: string; model?: string; providerId?: string }): Promise<NormalizedThread>
  getThreadDetail(threadId: string): Promise<{
    thread?: NormalizedThread
    blocks: ChatBlock[]
    latestSeq: number
    threadStatus?: string
    latestTurnId?: string
    latestUserMessageId?: string
    turnDurationByUserId?: Record<string, number>
    pendingApprovalIds?: string[]
    pendingUserInputIds?: string[]
    usage?: ThreadUsageSnapshot
    goal?: ThreadGoal | null
    todos?: ThreadTodoList | null
    historyAuthority?: 'case_boundary_only_v1'
    latestTurnAcceptedFinalDigest?: string
  }>
  getThreadSummary?(threadId: string): Promise<CoreThreadSummaryResponseJson>
  getThreadSummaryTaskOutput?(
    threadId: string,
    taskId: string,
    options?: { offset?: number; limit?: number }
  ): Promise<CoreThreadSummaryTaskOutputResponseJson>
  killThreadSummaryTask?(threadId: string, taskId: string): Promise<CoreThreadSummaryTaskMutationResponseJson>
  restartThreadSummaryTask?(threadId: string, taskId: string): Promise<CoreThreadSummaryTaskMutationResponseJson>
  sendUserMessage(
    threadId: string,
    text: string,
    options?: {
      mode?: string
      model?: string
      providerId?: string
      reasoningEffort?: SharedModelReasoningEffort
      riskIntent?: 'case'
      displayText?: string
      guiPlan?: {
        operation: 'draft' | 'refine'
        workspaceRoot: string
        relativePath: string
        planId: string
        sourceRequest?: string
        title?: string
      }
      attachmentIds?: string[]
      fileReferences?: UserFileReference[]
      workspaceCheckpointId?: string
    }
  ): Promise<{ turnId: string; threadId: string; userMessageItemId?: string }>
  reviewThread?(
    threadId: string,
    target: ReviewTarget,
    options?: { model?: string; providerId?: string }
  ): Promise<{ turnId: string; threadId: string; userMessageItemId?: string; reviewItemId?: string }>
  planCheckpointRewind?(
    threadId: string,
    checkpointId: string,
    options?: { scope?: 'code' | 'conversation' | 'combined' }
  ): Promise<CoreCheckpointRewindPlanJson>
  applyCheckpointRewind?(
    threadId: string,
    checkpointId: string,
    plan: CoreCheckpointRewindPlanJson
  ): Promise<CoreCheckpointRewindApplyResultJson>
  getRuntimeInfo?(): Promise<CoreRuntimeInfoJson>
  getToolDiagnostics?(): Promise<CoreRuntimeToolDiagnosticsJson>
  listSkills?(): Promise<CoreRuntimeSkillsResponseJson>
  uploadAttachment?(input: {
    name: string
    mimeType?: string
    dataBase64: string
    documentText?: string
    pageCount?: number
    textFallback?: CoreAttachmentTextFallbackJson
    threadId: string
    workspace: string
  }): Promise<CoreAttachmentMetadataJson>
  getAttachmentContent?(
    attachmentId: string,
    options: { threadId: string; workspace: string }
  ): Promise<CoreAttachmentContentResponseJson>
  listMemories?(options?: { workspace?: string; includeDeleted?: boolean }): Promise<CoreMemoryRecordJson[]>
  createMemory?(input: {
    content: string
    scope?: 'user' | 'workspace' | 'project'
    workspace?: string
    project?: string
    tags?: string[]
    confidence?: number
  }): Promise<CoreMemoryRecordJson>
  updateMemory?(
    memoryId: string,
    patch: { content?: string; tags?: string[]; confidence?: number; disabled?: boolean }
  ): Promise<CoreMemoryRecordJson>
  deleteMemory?(memoryId: string): Promise<CoreMemoryRecordJson>
  getMemoryDiagnostics?(): Promise<CoreMemoryDiagnosticsJson>
  steerUserMessage?(input: {
    threadId: string
    turnId: string
    text: string
    displayText?: string
    clientUserMessageId?: string
    expectedTurnId?: string
    attachmentIds?: string[]
    fileReferences?: UserFileReference[]
  }): Promise<{
    threadId: string
    turnId: string
    itemId?: string
    clientUserMessageId?: string
    admittedSeq?: number
  }>
  rewindThread?(threadId: string, turnId: string): Promise<{
    threadId: string
    turnId: string
    removedTurns: number
    remainingTurns: number
    removedTurnIds?: string[]
  }>
  interruptTurn(threadId: string, turnId: string, options?: { discard?: boolean }): Promise<void>
  renameThread(threadId: string, title: string): Promise<void>
  updateThreadWorkspace?(threadId: string, workspace: string): Promise<void>
  updateThreadRelation?(threadId: string, relation: NonNullable<NormalizedThread['relation']>): Promise<void>
  archiveThread?(threadId: string, archived: boolean): Promise<void>
  deleteThread(threadId: string): Promise<void>
  compactThread?(threadId: string, reason?: string): Promise<void>
  getThreadGoal?(threadId: string): Promise<ThreadGoal | null>
  setThreadGoal?(
    threadId: string,
    patch: {
      objective?: string
      status?: ThreadGoalStatus
      tokenBudget?: number | null
      strictCompletion?: boolean
      selfCheckRequired?: boolean
      selfCheckCompleted?: boolean
      selfCheckTurnId?: string
      research?: { enabled: true; requirements?: string[] }
    }
  ): Promise<ThreadGoal>
  clearThreadGoal?(threadId: string): Promise<boolean>
  getThreadTodos?(threadId: string): Promise<ThreadTodoList | null>
  setThreadTodos?(
    threadId: string,
    todos: Array<{
      id?: string
      content: string
      status: ThreadTodoStatus
      statusReasonCode?: ThreadTodoStatusReasonCode
      source?: ThreadTodoSource
      note?: string
    }>
  ): Promise<ThreadTodoList>
  clearThreadTodos?(threadId: string): Promise<boolean>
  forkThread?(
    threadId: string,
    options?: { relation?: 'primary' | 'fork' | 'side'; title?: string; turnId?: string }
  ): Promise<NormalizedThread>
  resumeSession?(
    sessionId: string,
    options?: { model?: string; providerId?: string; mode?: string }
  ): Promise<{ threadId: string; sessionId: string }>
  subscribeThreadEvents(
    threadId: string,
    sinceSeq: number,
    sink: ThreadEventSink,
    signal: AbortSignal
  ): Promise<void>
  /** Runtime HTTP: POST /v1/approvals/{id} */
  submitApprovalDecision?(
    approvalId: string,
    decision: 'allow' | 'deny',
    remember?: boolean
  ): Promise<void>
  /** Runtime HTTP compatibility path for request_user_input responses. */
  submitUserInputResponse?(requestId: string, answers: UserInputAnswer[]): Promise<void>
  cancelUserInput?(requestId: string): Promise<void>
}
