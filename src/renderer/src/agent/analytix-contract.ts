import { GUI_PLAN_CREATE_PLAN_TOOL_NAME } from '@shared/gui-plan'
import type {
  ThreadSummaryBackgroundProcess,
  ThreadSummaryOutput,
  ThreadSummaryResponse,
  ThreadSummarySideChat,
  ThreadSummarySource,
  ThreadSummarySubagent,
  ThreadSummaryTask,
  ThreadSummaryTaskMutationResponse,
  ThreadSummaryTaskOutputResponse
} from '../../../../packages/runtime/src/contracts/threads.js'
import type { RuntimeInfoResponse } from '../../../../packages/runtime/src/contracts/runtime-info.js'
import type {
  AcceptedFinalPublicView,
  AcceptedFinalPublicViewV2,
  AcceptedFinalRecordV3,
  AcceptedFinalRecordV5,
  BoundaryAcceptedFinalRecordV3,
  BoundaryAcceptedFinalRecordV5,
  EvidenceAuthorityWitnessBindingV1,
  FactFinalWitnessAdmissionV1,
  WitnessedFactAcceptedFinalRecordV4,
  WitnessedFactAcceptedFinalRecordV5
} from '../../../../packages/runtime/src/contracts/items.js'
import type {
  AttachmentDiagnosticsResponseV2,
  MemoryDiagnosticsResponseV2,
  RuntimeToolsResponse
} from '../../../../packages/runtime/src/contracts/runtime-tools.js'
import type {
  PublicRuntimeSkillV2,
  RuntimeSkillsResponseV2
} from '../../../../packages/runtime/src/contracts/runtime-skills.js'
import type { ModelReasoningEffort } from '../../../../packages/runtime/src/contracts/capabilities.js'
import type {
  CaseProjectDetailResponseV1,
  CaseProjectListResponseV1,
  CaseProjectSummaryV1,
  CaseProjectThreadsResponseV1
} from '../../../../packages/runtime/src/contracts/case-projects.js'
import type {
  AcceptedFinalDeliveryBatchV2
} from '../../../../packages/runtime/src/contracts/events.js'

export type CoreThreadStatus = 'idle' | 'running' | 'archived' | 'deleted'
export type CoreTurnStatus = 'queued' | 'running' | 'completed' | 'failed' | 'aborted'
export type CoreItemStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'aborted'
  | 'allowed'
  | 'denied'
  | 'expired'
  | string

export type CoreThreadSummaryJson = {
  id: string
  title: string
  workspace?: string
  model: string
  providerId?: string
  mode: string
  status: CoreThreadStatus
  approvalPolicy?: string
  sandboxMode?: string
  relation?: 'primary' | 'fork' | 'side'
  parentThreadId?: string
  forkedFromThreadId?: string
  forkedFromTitle?: string
  forkedAt?: string
  forkedFromMessageCount?: number
  forkedFromTurnCount?: number
  preview?: string
  messageCount?: number
  turnCount?: number
  latestTurnId?: string
  /** Host projection status; case_boundary_only_v1 contains only host-admitted public records. */
  historyAuthority?: 'case_boundary_only_v1'
  goal?: CoreThreadGoalJson | null
  todos?: CoreThreadTodoListJson | null
  createdAt: string
  updatedAt: string
}

export type CoreCaseProjectJson = CaseProjectSummaryV1
export type CoreCaseProjectListResponseJson = CaseProjectListResponseV1
export type CoreCaseProjectThreadsResponseJson = CaseProjectThreadsResponseV1
export type CoreCaseProjectDetailResponseJson = CaseProjectDetailResponseV1

export type CoreThreadJson = CoreThreadSummaryJson & {
  turns?: CoreTurnJson[]
  latestSeq?: number
  usage?: CoreUsageSnapshotJson
  pendingApprovalIds?: string[]
  pendingUserInputIds?: string[]
  /** Main-verified current delivery groups; V3 alone is never authority. */
  acceptedFinalDelivery?: AcceptedFinalDeliveryBatchV2
  acceptedFinalDeliveries?: AcceptedFinalDeliveryBatchV2[]
}

export type CoreThreadSummaryItemStateJson = 'active' | 'done' | 'terminal' | 'inactive'

export type CoreModelExecutionSourceJson =
  | 'thread'
  | 'subagent-profile'
  | 'explicit-input'
  | 'session'
  | 'runtime-default'

export type CoreModelExecutionRefJson = {
  providerId: string
  modelId: string
  variant?: string
  endpointFormat?: string
  baseUrlFingerprint?: string
  customFullEndpointFingerprint?: string
  capabilityFingerprint?: string
  source: CoreModelExecutionSourceJson
  resolvedAt: string
}

export type CoreThreadSummarySubagentJson = ThreadSummarySubagent
export type CoreThreadSummaryTaskJson = ThreadSummaryTask
export type CoreThreadSummaryOutputJson = ThreadSummaryOutput
export type CoreThreadSummarySourceJson = ThreadSummarySource
export type CoreThreadSummarySideChatJson = ThreadSummarySideChat
export type CoreThreadSummaryBackgroundProcessJson = ThreadSummaryBackgroundProcess
export type CoreThreadSummaryResponseJson = ThreadSummaryResponse

export type CoreThreadSummaryTaskOutputResponseJson = ThreadSummaryTaskOutputResponse

export type CoreThreadSummaryTaskMutationResponseJson = ThreadSummaryTaskMutationResponse

export type CoreCheckpointChangeKindJson = 'created' | 'modified' | 'deleted' | 'unknown'

export type CoreCheckpointChangedFileJson = {
  relativePath: string
  changeKind: CoreCheckpointChangeKindJson
  beforeHash?: string | null
  afterHash?: string | null
}

export type CoreCheckpointMetadataJson = {
  schemaVersion: 1
  checkpointId: string
  threadId: string
  turnId: string
  workspace: string
  createdAt: string
  status: 'captured' | 'rewind_planned' | 'rewound' | 'failed'
  changedFiles: CoreCheckpointChangedFileJson[]
  snapshotStorage?: 'runtime_private_cas'
}

export type CoreCheckpointRestoreFilePlanJson = {
  relativePath: string
  changeKind: CoreCheckpointChangeKindJson
  action:
    | 'restore_previous_version'
    | 'delete_created_file'
    | 'restore_deleted_file'
    | 'noop'
    | 'manual_review'
    | 'blocked'
  status: 'ready' | 'manual_review' | 'blocked'
  risk: 'low' | 'medium' | 'high'
  reason: string
  contentSource: 'checkpoint_metadata_only'
  beforeHash?: string | null
  afterHash?: string | null
}

export type CoreCheckpointConversationRewindPlanJson = {
  status: 'ready' | 'blocked'
  reason?: string
  boundaryTurnId?: string
  checkpointEventSeq?: number
  boundarySeq?: number
  retainedEventCount: number
  removedEventCount: number
  removedTurnIds: string[]
  projection: {
    latestSeq: number
    turnCount: number
    itemCount: number
    checkpointCount: number
  }
}

export type CoreCheckpointRewindPlanJson = {
  schemaVersion: 1
  planId: string
  checkpointId: string
  threadId: string
  planDigest: string
  workspace: string
  createdAt: string
  scope: 'code' | 'conversation' | 'combined'
  applyMode: 'plan_only'
  destructive: false
  checkpoint: CoreCheckpointMetadataJson
  files: CoreCheckpointRestoreFilePlanJson[]
  conversation?: CoreCheckpointConversationRewindPlanJson
  summary: {
    fileCount: number
    readyFileCount: number
    manualReviewFileCount: number
    blockedFileCount: number
    retainedEventCount: number
    removedEventCount: number
    removedTurnCount: number
    containsRawPrompt: false
    containsFullFileContent: false
    containsSecretValue: false
  }
}

export type CoreCheckpointRewindPlanResponseJson = {
  plan: CoreCheckpointRewindPlanJson
}

export type CoreCheckpointApplyFileSnapshotJson = {
  hash: string
  content: string
  encoding: 'utf8'
}

export type CoreCheckpointApplySnapshotEvidenceJson = {
  relativePath: string
  before?: CoreCheckpointApplyFileSnapshotJson
  after?: CoreCheckpointApplyFileSnapshotJson
}

export type CoreCheckpointRewindApplyResultJson = {
  schemaVersion: 1
  applyId: string
  planId: string
  checkpointId: string
  threadId: string
  workspace: string
  createdAt: string
  scope: 'code' | 'conversation' | 'combined'
  status: 'applied' | 'blocked' | 'failed' | 'already_applied'
  destructive: true
  rescue?: {
    rescueId: string
    eventSeq?: number
    fileCount: number
  }
  files: Array<{
    relativePath: string
    action:
      | 'restore_previous_version'
      | 'delete_created_file'
      | 'restore_deleted_file'
      | 'noop'
      | 'manual_review'
      | 'blocked'
    status: 'applied' | 'noop' | 'manual_review' | 'blocked' | 'failed'
    reason: string
    beforeHash?: string | null
    afterHash?: string | null
    currentHash?: string | null
  }>
  conversation: {
    status: 'not_requested' | 'audit_recorded' | 'blocked' | 'already_applied'
    reason?: string
    boundaryTurnId?: string
    retainedEventCount?: number
    removedEventCount?: number
    removedTurnIds?: string[]
    auditEventSeq?: number
  }
  summary: {
    fileAppliedCount: number
    fileNoopCount: number
    fileManualReviewCount: number
    fileBlockedCount: number
    fileFailedCount: number
  }
  auditEventSeq?: number
}

export type CoreCheckpointRewindApplyResponseJson = {
  apply: CoreCheckpointRewindApplyResultJson
}

export type CoreAttachmentMetadataJson = {
  id: string
  name: string
  kind: 'image' | 'document'
  mimeType: string
  byteSize: number
  scope: 'thread'
  width?: number
  height?: number
  pageCount?: number
  truncated?: boolean
  createdAt: string
  updatedAt: string
}

export type CoreAttachmentTextFallbackJson = {
  dataBase64: string
  mimeType: string
  byteSize: number
  width?: number
  height?: number
  wasCompressed?: boolean
}

export type CoreAttachmentDiagnosticsJson = AttachmentDiagnosticsResponseV2

export type CoreMemoryRecordJson = {
  id: string
  content: string
  scope: 'user' | 'workspace' | 'project'
  workspace?: string
  project?: string
  sourceThreadId?: string
  sourceTurnId?: string
  tags: string[]
  confidence: number
  createdAt: string
  updatedAt: string
  disabledAt?: string
  deletedAt?: string
}

export type CoreThreadGoalStatusJson =
  | 'active'
  | 'paused'
  | 'blocked'
  | 'usageLimited'
  | 'budgetLimited'
  | 'complete'

export type CoreThreadGoalEvidenceEntryJson = {
  id: string
  turnId?: string
  toolCallId?: string
  requirementId?: string
  step: string
  evidence: string[]
  summary?: string
  createdAt: string
}

export type CoreThreadGoalResearchJson = {
  enabled: true
  stateRelativePath: string
  taskSpecPath: string
  progressPath: string
  findingsPath: string
  directionsTriedPath: string
  iterationLogPath: string
  requirementCount: number
}

export type CoreThreadGoalJson = {
  threadId: string
  objective: string
  status: CoreThreadGoalStatusJson
  tokenBudget?: number | null
  tokensUsed: number
  timeUsedSeconds: number
  evidenceLedger?: CoreThreadGoalEvidenceEntryJson[]
  research?: CoreThreadGoalResearchJson
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

export type CoreThreadGoalResponseJson = {
  goal: CoreThreadGoalJson | null
}

export type CoreClearThreadGoalResponseJson = {
  cleared: boolean
}

export type CoreThreadTodoStatusJson = 'pending' | 'in_progress' | 'completed' | 'failed' | 'canceled'
export type CoreThreadTodoStatusReasonCodeJson =
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

export type CoreThreadTodoSourceJson =
  | { kind: 'manual' }
  | {
      kind: 'plan'
      planId?: string
      relativePath?: string
      ordinal?: number
      contentHash?: string
    }
  | {
      kind: 'child'
      parentThreadId?: string
      childThreadId?: string
      childRunId?: string
      jobId?: string
      projectionId?: string
    }

export type CoreThreadTodoItemJson = {
  id: string
  content: string
  status: CoreThreadTodoStatusJson
  statusReasonCode?: CoreThreadTodoStatusReasonCodeJson
  source?: CoreThreadTodoSourceJson
  note?: string
  createdAt: string
  updatedAt: string
}

export type CoreThreadTodoListJson = {
  threadId: string
  turnId?: string
  items: CoreThreadTodoItemJson[]
  updatedAt: string
}

export type CoreThreadTodosResponseJson = {
  todos: CoreThreadTodoListJson | null
}

export type CoreClearThreadTodosResponseJson = {
  cleared: boolean
}

export type CoreMemoryDiagnosticsJson = MemoryDiagnosticsResponseV2

export type CoreRuntimeInfoJson = RuntimeInfoResponse
export type CoreRuntimeToolDiagnosticsJson = RuntimeToolsResponse
export type CoreRuntimeCommandDiagnosticJson = RuntimeToolsResponse['commands'][number]

export type CoreRuntimeSkillJson = PublicRuntimeSkillV2 & {
  description?: string
  version?: string
  root?: string
  triggers?: {
    commands?: string[]
    promptPatterns?: string[]
    fileTypes?: string[]
  }
  allowedTools?: string[]
}

export type CoreRuntimeSkillsResponseJson = RuntimeSkillsResponseV2

export type CoreChildRuntimeMetadataJson = {
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
  childToolPolicy?: 'readOnly' | 'inherit'
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
  cacheMissReasons?: string[]
  cacheSuggestions?: string[]
  cacheDiagnostics?: CoreCacheDiagnosticsJson
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

export type CoreWebSourceJson = {
  sourceId?: string
  url?: string
  title?: string
  retrievedAt?: string
}

export type CoreUserFileReferenceJson = {
  path: string
  relativePath: string
  name: string
  kind?: 'file' | 'directory'
}

export type CoreAcceptedFinalVariantJson =
  | 'EvidenceBackedAnswer'
  | 'PartialEvidenceAnswer'
  | 'VerifiedNoHitAnswer'
  | 'SourceUnavailableAnswer'
  | 'NeedsEvidenceAnswer'
  | 'GeneralGuidanceAnswer'

export type CoreAcceptedFinalTerminalReasonJson =
  | 'success'
  | 'source_unavailable'
  | 'semantic_failure'
  | 'provider_failure'
  | 'cancel'
  | 'timeout'
  | 'stream_abort'
  | 'recovery'
  | 'approval'
  | 'user_input'
  | 'resume'
  | 'restart'
  | 'report_fallback'
  | 'step_limit'
  | 'background_completion'
  | 'tool_failure'
  | 'approval_denied'
  | 'input_cancelled'

/** Public, PII-free proof issued by the Go host after final publication. */
export type CoreAcceptedFinalRecordV3Json = AcceptedFinalRecordV3

export type CoreBoundaryAcceptedFinalVariantJson = BoundaryAcceptedFinalRecordV5['variant']

export type CoreFactAcceptedFinalVariantJson = WitnessedFactAcceptedFinalRecordV5['variant']

export type CoreBoundaryAcceptedFinalRecordV3Json = BoundaryAcceptedFinalRecordV3

export type CoreEvidenceAuthorityWitnessBindingV1Json = EvidenceAuthorityWitnessBindingV1

export type CoreFactFinalWitnessAdmissionV1Json = FactFinalWitnessAdmissionV1

export type CoreWitnessedFactAcceptedFinalRecordV4Json = WitnessedFactAcceptedFinalRecordV4

export type CoreAcceptedFinalRecordV5Json = AcceptedFinalRecordV5

export type CoreBoundaryAcceptedFinalRecordV5Json = BoundaryAcceptedFinalRecordV5

export type CoreWitnessedFactAcceptedFinalRecordV5Json = WitnessedFactAcceptedFinalRecordV5

export type CoreAcceptedFinalClaimTypeJson =
  | 'amount'
  | 'count'
  | 'account'
  | 'entity'
  | 'direction'
  | 'date_range'
  | 'relationship'
  | 'quote'
  | 'device_identifier'
  | 'ownership'
  | 'control'
  | 'address'
  | 'change'
  | 'bid_certificate'
  | 'bid_edit_metadata'
  | 'legal_characterization'

/** Private/audit-only expansion of the signed V5 core; never a generic renderer contract. */
export type CoreAcceptedFinalPublicViewV2Json = AcceptedFinalPublicViewV2
/** Versioned accepted-final projection parsed at the Electron public boundary. */
export type CoreAcceptedFinalPublicViewJson = AcceptedFinalPublicView

export type CoreTurnJson = {
  factHistoryState?: 'retained_snapshot'
  id: string
  threadId: string
  status: CoreTurnStatus
  prompt: string
  model?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
  items?: CoreTurnItemJson[]
  /** Complete signed AcceptedFinalRecord V5 is host-private and never enters renderer state. */
  acceptedFinal?: never
  acceptedFinalView?: CoreAcceptedFinalPublicViewJson
  attachmentIds?: string[]
  workspaceCheckpointId?: string
  activeSkillIds?: string[]
  injectedMemoryIds?: string[]
  skillInjectionBytes?: number
  error?: string
}

export type CoreTurnItemJson = {
  id: string
  turnId: string
  threadId: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  status: CoreItemStatus
  createdAt: string
  finishedAt?: string
  kind: string
  text?: string
  displayText?: string
  delivery?: 'steer' | string
  clientUserMessageId?: string
  toolName?: string
  callId?: string
  toolKind?: 'tool_call' | 'command_execution' | 'file_change' | 'subagent'
  arguments?: unknown
  output?: unknown
  isError?: boolean
  approvalId?: string
  inputId?: string
  prompt?: string
  questions?: Array<{
    header: string
    id: string
    question: string
    options: Array<{ label: string; description: string }>
  }>
  summary?: string
  replacedTokens?: number
  auto?: boolean
  pinnedConstraints?: string[]
  sourceDigest?: string
  digestMarker?: string
  sourceItemIds?: string[]
  schemaVersion?: 2 | 3 | 4
  reasoningExcluded?: true
  reasoningExclusionProof?: string
  assistantProseExcluded?: true
  toolPayloadsExcluded?: true
  caseFactsExcluded?: true
  providerHistoryProjectionVersion?: 1 | 2
  caseHistoryProjectionVersion?: 1 | 2
  message?: string
  code?: string
  details?: unknown
  severity?: 'info' | 'warning' | 'error'
  attachmentIds?: string[]
  fileReferences?: CoreUserFileReferenceJson[]
  workspaceCheckpointId?: string
  activeSkillIds?: string[]
  injectedMemoryIds?: string[]
  skillInjectionBytes?: number
  target?: CoreReviewTargetJson
  title?: string
  reviewText?: string
  signature?: string
  /** Complete signed AcceptedFinalRecord V5 is host-private and never enters renderer state. */
  acceptedFinal?: never
  acceptedFinalView?: CoreAcceptedFinalPublicViewJson
}

export type CoreReviewTargetJson =
  | { kind: 'uncommittedChanges' }
  | { kind: 'baseBranch'; branch: string }
  | { kind: 'commit'; sha: string }
  | { kind: 'custom'; instructions: string }

export type CoreReviewFindingJson = {
  title: string
  body: string
  confidenceScore: number
  priority: number
  codeLocation: {
    absoluteFilePath: string
    lineRange: { start: number; end: number }
  }
}

export type CoreReviewOutputJson = {
  findings: CoreReviewFindingJson[]
  overallCorrectness: 'patch is correct' | 'patch is incorrect'
  overallExplanation: string
  overallConfidenceScore: number
}

/**
 * Structured plan metadata the renderer expects on a successful
 * `create_plan` tool result. Mirrors the Analytix output contract
 * so the Workbench can reload the saved plan file and update the
 * Plan panel without parsing assistant prose.
 */
export type CorePlanToolResultJson = {
  summary?: string
  plan_id: string
  workspace_root: string
  relative_path: string
  absolute_path?: string
  source_request?: string
  title?: string
  operation: 'draft' | 'refine'
  saved_at: string
  content_hash?: string
  byte_size?: number
}

export type CoreStartTurnResponseJson = {
  threadId: string
  turnId: string
  userMessageItemId?: string
}

export type CoreStartReviewResponseJson = CoreStartTurnResponseJson & {
  reviewItemId?: string
}

export type CoreAttachmentUploadResponseJson = {
  attachment: CoreAttachmentMetadataJson
}

export type CoreAttachmentContentResponseJson = {
  attachment: CoreAttachmentMetadataJson
  dataBase64: string
}

export type CoreMemoryListResponseJson = {
  memories: CoreMemoryRecordJson[]
}

/**
 * Optional plan context attached to a start-turn request. Carries the
 * reserved plan id, workspace root, and relative path the Analytix
 * should expose to the model via the `create_plan` tool.
 */
export type CoreStartTurnPlanContextJson = {
  operation: 'draft' | 'refine'
  workspaceRoot: string
  relativePath: string
  planId: string
  sourceRequest?: string
  title?: string
}

/**
 * Native Analytix plan tool name. Re-exported alongside the shared
 * constant for renderer consumers.
 */
export const CORE_PLAN_TOOL_NAME = GUI_PLAN_CREATE_PLAN_TOOL_NAME

export type CoreUsageSnapshotJson = {
  promptTokens?: number
  prompt_tokens?: number
  completionTokens?: number
  completion_tokens?: number
  reasoningTokens?: number
  reasoning_tokens?: number
  totalTokens?: number
  total_tokens?: number
  cachedTokens?: number
  cached_tokens?: number
  cacheHitTokens?: number
  cache_hit_tokens?: number
  cacheMissTokens?: number
  cache_miss_tokens?: number
  cacheHitRate?: number
  cache_hit_rate?: number
  cacheableTokenHitRate?: number | null
  cacheable_token_hit_rate?: number | null
  totalInputTokenHitRate?: number | null
  total_input_token_hit_rate?: number | null
  cacheMissReasons?: string[]
  cache_miss_reasons?: string[]
  cacheSuggestions?: string[]
  cache_suggestions?: string[]
  turns?: number
  costUsd?: number
  cost_usd?: number
  costCny?: number
  cost_cny?: number
  priceConfigured?: boolean
  price_configured?: boolean
  cacheSavingsUsd?: number
  cache_savings_usd?: number
  cacheSavingsCny?: number
  cache_savings_cny?: number
  tokenEconomySavingsTokens?: number
  token_economy_savings_tokens?: number
}

export type CoreCacheDiagnosticsJson = {
  prefixHash?: string
  prefixChanged?: boolean
  prefixChangeReasons?: string[]
  toolSourceChanged?: boolean
  toolSourceChangeReasons?: string[]
  systemHash?: string
  modeHash?: string
	  prefixItemsHash?: string
	  toolsHash?: string
	  toolSchemaTokens?: number
	  toolCount?: number
	  toolSourcesHash?: string
	  toolSourceIds?: string[]
	  route?: string
	  cacheTelemetrySupported?: boolean
	  firstTokenLatencyMs?: number
	  durationMs?: number
	  cacheHitTokens?: number
  cacheMissTokens?: number
  provider?: string
  providerId?: string
  endpointFormat?: string
  model?: string
}

export const CORE_RUNTIME_EVENT_KINDS = [
  'thread_created',
  'thread_updated',
  'thread_rewound',
  'turn_started',
  'turn_completed',
  'turn_failed',
  'turn_aborted',
  'turn_steered',
  'item_created',
  'item_updated',
  'item_completed',
  'assistant_text_delta',
  'tool_call_ready',
  'tool_progress',
  'tool_result_upload_wait',
  'tool_storm_suppressed',
  'tool_catalog_changed',
  'child_steer_queued',
  'child_steer_admitted',
  'child_steer_rejected',
  'child_pause_requested',
  'child_paused',
  'child_resume_requested',
  'child_resumed',
  'child_pause_rejected',
  'mcp_lifecycle_audit',
  'autoresearch_state_audit',
  'tool_call_started',
  'tool_call_finished',
  'approval_requested',
  'approval_resolved',
  'user_input_requested',
  'user_input_resolved',
  'compaction_started',
  'compaction_completed',
  'goal_updated',
  'goal_cleared',
  'goal_evidence_audit',
  'todos_updated',
  'todos_cleared',
  'checkpoint_captured',
  'checkpoint_rewind_rescue_created',
  'checkpoint_rewind_applied',
  'pipeline_stage',
  'usage',
  'error',
  'heartbeat',
  'snapshot_required'
] as const

export type CoreRuntimeEventKind = typeof CORE_RUNTIME_EVENT_KINDS[number]

export type CoreRuntimeEventJson = {
  kind?: CoreRuntimeEventKind
  seq?: number
  sinceSeq?: number
  since_seq?: number
  highestSeq?: number
  highest_seq?: number
  replayEventCount?: number
  replay_event_count?: number
  reason?: string
  timestamp?: string
  threadId?: string
  thread_id?: string
  turnId?: string
  turn_id?: string
  itemId?: string
  item_id?: string
  model?: string
  providerId?: string
  provider_id?: string
  effort?: ModelReasoningEffort
  usageSource?: string
  usage_source?: string
  childRunId?: string
  child_run_id?: string
  jobId?: string
  job_id?: string
  childThreadId?: string
  child_thread_id?: string
  childTurnId?: string
  child_turn_id?: string
  steerMessageId?: string
  steer_message_id?: string
  steerStatus?: 'queued' | 'admitted' | 'rejected' | 'expired' | string
  steer_status?: 'queued' | 'admitted' | 'rejected' | 'expired' | string
  pauseRequestId?: string
  pause_request_id?: string
  pauseStatus?: 'requested' | 'paused' | 'rejected' | 'expired' | 'resumed' | 'resume_requested' | string
  pause_status?: 'requested' | 'paused' | 'rejected' | 'expired' | 'resumed' | 'resume_requested' | string
  parentThreadId?: string
  parent_thread_id?: string
  sourceTurnId?: string
  source_turn_id?: string
  sourceToolCallId?: string
  source_tool_call_id?: string
  createdAt?: string
  admittedAt?: string
  admitted_at?: string
  pausedAt?: string
  paused_at?: string
  resumedAt?: string
  resumed_at?: string
  item?: CoreTurnItemJson
  acceptedFinalDigest?: string
  publicationCommitId?: string
  publicationEventId?: string
  publicationSlot?: string
  publicationPayloadDigest?: string
  terminalReason?: CoreAcceptedFinalTerminalReasonJson
  discard?: boolean
  cancelled?: boolean
  text?: string
  displayText?: string
  clientUserMessageId?: string
  client_user_message_id?: string
  admittedSeq?: number
  admitted_seq?: number
  content?: string
  delta?: string | { text?: string; content?: string }
  approvalId?: string
  approval_id?: string
  approvalPolicy?: string
  approval_policy?: string
  sandboxMode?: string
  sandbox_mode?: string
  toolName?: string
  tool_name?: string
  callId?: string
  call_id?: string
  readyCount?: number
  ready_count?: number
  partial?: boolean
  toolResultCount?: number
  tool_result_count?: number
  fingerprint?: string
  toolCount?: number
  changeKind?: 'additive' | 'breaking'
  toolNames?: string[]
  status?: string
  title?: string
  attempt?: number
  maxAttempt?: number
  stage?:
    | 'setup'
    | 'pre_start'
    | 'post_start'
    | 'input_received'
    | 'input_cached'
    | 'input_routed'
    | 'input_compressed'
    | 'input_remembered'
    | 'pre_send'
    | 'post_send'
    | 'response_received'
    | 'subagent_running'
    | 'subagent_completed'
    | 'subagent_failed'
    | 'subagent_killed'
    | 'subagent_aborted'
    | 'subagent_interrupted'
    | 'provider_retrying'
    | 'provider_error'
    | 'empty_final_recovered'
  label?: string
  details?: unknown
  summary?: string
  prompt?: string
  inputId?: string
  input_id?: string
  questions?: Array<{
    header: string
    id: string
    question: string
    options: Array<{ label: string; description: string }>
  }>
  replacedTokens?: number
  auto?: boolean
  pinnedConstraints?: string[]
  sourceDigest?: string
  digestMarker?: string
  sourceItemIds?: string[]
  schemaVersion?: 2
  reasoningExcluded?: true
  reasoningExclusionProof?: string
  usage?: CoreUsageSnapshotJson
  cacheDiagnostics?: CoreCacheDiagnosticsJson
  goal?: CoreThreadGoalJson | null
  todos?: CoreThreadTodoListJson | null
  cleared?: boolean
  rewindTurnId?: string
  removedTurns?: number
  remainingTurns?: number
  removedTurnIds?: string[]
  message?: string
  code?: string
  severity?: 'info' | 'warning' | 'error'
  terminal?: boolean
  fatal?: boolean
  recoverable?: boolean
  child?: CoreChildRuntimeMetadataJson
}

export type RuntimeErrorJson = {
  code?: string
  error?: string | { message?: string; status?: number }
  message?: string
  details?: unknown
  severity?: 'info' | 'warning' | 'error'
}
