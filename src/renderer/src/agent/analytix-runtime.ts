import type {
  AgentProvider,
  AcceptedFinalProjectionBatch,
  ChatBlock,
  NormalizedCaseProject,
  NormalizedCaseProjectListResult,
  NormalizedThread,
  ReviewTarget,
  ThreadEventSink,
  ThreadListOptions,
  ThreadUsageSnapshot,
  UserInputAnswer
} from './types'
import {
  getAnalytixRuntimeSettings,
  getModelProviderSettings,
  normalizeModelProviderId,
  resolveModelProviderRequestSelection,
  type AppSettingsV1
} from '@shared/app-settings'
import { validateOptionalModelReasoningEffortV1 } from '@shared/model-reasoning-effort'
import {
  ANALYTIX_ATTACHMENT_DIAGNOSTICS_PATH,
  ANALYTIX_ATTACHMENTS_PATH,
  ANALYTIX_HEALTH_PATH,
  ANALYTIX_MEMORY_DIAGNOSTICS_PATH,
  ANALYTIX_MEMORY_PATH,
  ANALYTIX_RUNTIME_INFO_PATH,
  ANALYTIX_RUNTIME_TOOLS_PATH,
  ANALYTIX_CASE_PROJECTS_PATH,
  ANALYTIX_SKILLS_PATH,
  ANALYTIX_THREADS_PATH,
  analytixCaseProjectDetailPath,
  analytixCaseProjectThreadsPath,
  analytixApprovalPath,
  analytixThreadCompactPath,
  analytixThreadCheckpointRewindApplyPath,
  analytixThreadCheckpointRewindPlanPath,
  analytixThreadEventsPath,
  analytixThreadForkPath,
  analytixThreadGoalPath,
  analytixThreadSummaryPath,
  analytixThreadSummaryTaskKillPath,
  analytixThreadSummaryTaskOutputPath,
  analytixThreadSummaryTaskRestartPath,
  analytixThreadRewindPath,
  analytixThreadReviewPath,
  analytixThreadTodosPath,
  analytixThreadInterruptPath,
  analytixThreadPath,
  analytixThreadSteerPath,
  analytixThreadTurnsPath,
  analytixAttachmentContentPath,
  analytixUserInputPath,
  analytixMemoryRecordPath,
  analytixSessionResumePath,
  normalizeThreadMode,
  type AnalytixThreadMode
} from '@shared/analytix-endpoints'
import { parseRuntimeErrorBody, runtimeErrorToError, type RuntimeError } from '@shared/runtime-error'
import type { AcceptedFinalSseAckBindingV1 } from '@shared/analytix-api'
import type {
  CoreAttachmentDiagnosticsJson,
  CoreAttachmentContentResponseJson,
  CoreAttachmentMetadataJson,
  CoreAttachmentTextFallbackJson,
  CoreAttachmentUploadResponseJson,
  CoreCaseProjectDetailResponseJson,
  CoreCaseProjectListResponseJson,
  CoreCaseProjectThreadsResponseJson,
  CoreCheckpointRewindApplyResponseJson,
  CoreCheckpointRewindApplyResultJson,
  CoreCheckpointRewindPlanJson,
  CoreCheckpointRewindPlanResponseJson,
  CoreMemoryDiagnosticsJson,
  CoreMemoryListResponseJson,
  CoreMemoryRecordJson,
  CoreResumeSessionResponseJson,
  CoreRuntimeInfoJson,
  CoreRuntimeEventJson,
  CoreRuntimeSkillsResponseJson,
  CoreRuntimeToolDiagnosticsJson,
  CoreStartReviewResponseJson,
  CoreClearThreadGoalResponseJson,
  CoreClearThreadTodosResponseJson,
  CoreStartTurnResponseJson,
  CoreThreadGoalResponseJson,
  CoreThreadJson,
  CoreTurnJson,
  CoreTurnItemJson,
  CoreThreadSummaryResponseJson,
  CoreThreadSummaryTaskMutationResponseJson,
  CoreThreadSummaryTaskOutputResponseJson,
  CoreThreadSummaryJson,
  CoreThreadTodosResponseJson
} from './analytix-contract'
import {
  buildQuery,
  acceptedFinalProjectionBatchFromRuntime,
  caseProjectFromCore,
  chatBlockFromItem,
  dispatchAnalytixRuntimeEvents,
  goalFromCore,
  mergeChatBlocks,
  todosFromCore,
  threadFromCore,
  usageFromCore
} from './analytix-mapper'
import { rendererRuntimeClient } from './runtime-client'
import { isPublicSseIpcPayload } from '@shared/public-runtime-content'
import {
  AssistantTextTurnItem,
  AcceptedFinalPublicViewV3Schema,
  acceptedFinalPublicViewsMatch
} from '../../../../packages/runtime/src/contracts/items.js'
import { PublicProjectionRevokedEvent as PublicProjectionRevokedEventSchema } from '../../../../packages/runtime/src/contracts/events.js'
import {
  ThreadSummaryResponse as ThreadSummaryResponseSchema,
  ThreadSummaryTaskMutationResponse as ThreadSummaryTaskMutationResponseSchema,
  ThreadSummaryTaskOutputResponse
} from '../../../../packages/runtime/src/contracts/threads.js'
import { threadSummaryTaskIdentityMatchesV1 } from '../../../../packages/runtime/src/contracts/task-job-output.js'
import { RuntimeInfoResponse as RuntimeInfoResponseSchema } from '../../../../packages/runtime/src/contracts/runtime-info.js'
import {
  AttachmentDiagnosticsResponseV2 as AttachmentDiagnosticsResponseSchema,
  MemoryDiagnosticsResponseV2 as MemoryDiagnosticsResponseSchema,
  RuntimeToolsResponse as RuntimeToolsResponseSchema
} from '../../../../packages/runtime/src/contracts/runtime-tools.js'
import { RuntimeSkillsResponseV2 as RuntimeSkillsResponseSchema } from '../../../../packages/runtime/src/contracts/runtime-skills.js'
import type { ZodType } from 'zod'
import {
  filterRevokedPublicProjectionThreads,
  isPublicProjectionRevoked
} from '../lib/public-projection-revocation'

function createSseStreamId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `sse-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function readRuntimeError(body: string, fallback: string): RuntimeError {
  return parseRuntimeErrorBody(body, fallback)
}

const ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON: Record<string, CoreTurnJson['status']> = {
  success: 'completed',
  source_unavailable: 'completed',
  semantic_failure: 'failed',
  provider_failure: 'failed',
  cancel: 'aborted',
  timeout: 'failed',
  stream_abort: 'failed',
  recovery: 'completed',
  approval: 'completed',
  user_input: 'completed',
  resume: 'completed',
  restart: 'aborted',
  report_fallback: 'failed',
  step_limit: 'failed',
  background_completion: 'completed',
  tool_failure: 'failed',
  approval_denied: 'completed',
  input_cancelled: 'completed'
}

function acceptedFinalProjectionMatchesTurn(
  turn: CoreTurnJson,
  projection: AcceptedFinalProjectionBatch
): boolean {
  const turnRecordValue = turn.acceptedFinal
  const turnView = turn.acceptedFinalView
  const assistants = (turn.items ?? []).filter((item) => item.kind === 'assistant_text')
  if (!turnView || assistants.length !== 1) return false
  const item = assistants[0]
  const itemRecordValue = item?.acceptedFinal
  const itemView = item?.acceptedFinalView
  const turnGenericView = AcceptedFinalPublicViewV3Schema.safeParse(turnView)
  const itemGenericView = AcceptedFinalPublicViewV3Schema.safeParse(itemView)
  if (turnRecordValue || !item || itemRecordValue || !turnGenericView.success || !itemGenericView.success ||
      !acceptedFinalPublicViewsMatch(turnView, itemView) ||
      ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[turnGenericView.data.terminalReason] !== turn.status ||
      turn.finishedAt !== turnGenericView.data.acceptedAt ||
      item.threadId !== turn.threadId || item.turnId !== turn.id ||
      item.role !== 'assistant' || item.status !== 'completed' ||
      item.finishedAt !== turnGenericView.data.acceptedAt) return false
  return projection.threadId === turn.threadId &&
    projection.turnId === turn.id &&
    projection.publicationCommitId === turnGenericView.data.acceptedFinalDigest &&
    projection.assistant.id === item.id &&
    projection.assistant.text === item.text &&
    projection.assistant.createdAt === (item.createdAt || item.finishedAt) &&
    projection.assistant.meta?.turnId === turn.id &&
    acceptedFinalPublicViewsMatch(projection.assistant.acceptedFinalView, turnView) &&
    projection.terminal.status === turn.status &&
    projection.terminal.createdAt === turnGenericView.data.acceptedAt &&
    projection.terminal.acceptedFinalDigest === turnGenericView.data.acceptedFinalDigest &&
    projection.terminal.terminalReason === turnGenericView.data.terminalReason
}

function acceptedFinalProjectionsForTurns(
  turns: CoreTurnJson[],
  deliveryValues: CoreThreadJson['acceptedFinalDeliveries']
): Map<string, AcceptedFinalProjectionBatch> {
  const projections = (deliveryValues ?? []).map(acceptedFinalProjectionBatchFromRuntime)
  if (projections.some((projection) => !projection)) return new Map()
  const acceptedTurns = turns.filter((turn) =>
    turn.acceptedFinal !== undefined || turn.acceptedFinalView !== undefined ||
    (turn.items ?? []).some((item) =>
      item.acceptedFinal !== undefined || item.acceptedFinalView !== undefined
    )
  )
  if (acceptedTurns.length !== projections.length) return new Map()

  const result = new Map<string, AcceptedFinalProjectionBatch>()
  const usedBatchIds = new Set<string>()
  for (const [index, turn] of acceptedTurns.entries()) {
    const projection = projections[index]
    if (!projection || !acceptedFinalProjectionMatchesTurn(turn, projection) ||
        usedBatchIds.has(projection.batchId)) return new Map()
    usedBatchIds.add(projection.batchId)
    result.set(turn.id, projection)
  }
  if (usedBatchIds.size !== projections.length) return new Map()
  return result
}

function normalizeApprovalPolicy(value: string | undefined): NormalizedThread['approvalPolicy'] {
  switch (value) {
    case 'always':
    case 'auto':
    case 'on-request':
    case 'untrusted':
    case 'suggest':
    case 'never':
      return value
    default:
      return undefined
  }
}

function runtimeEventString(event: CoreRuntimeEventJson, ...keys: Array<keyof CoreRuntimeEventJson>): string | undefined {
  for (const key of keys) {
    const value = event[key]
    if (typeof value === 'string' && value.trim().length > 0) return value
  }
  return undefined
}

function validStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  return value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
}

type ThreadTimingMaps = {
  turnDurationByUserId: Record<string, number>
}

function timestampMs(value: unknown): number | undefined {
  if (typeof value !== 'string' || !value.trim()) return undefined
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function timelineItemsFromThreadItems(items: CoreTurnItemJson[]): CoreTurnItemJson[] {
  return items.flatMap((item) => {
    if (item.kind === 'assistant_reasoning') return []
    return [{ ...item }]
  })
}

function deriveThreadTimingMaps(
  turns: Array<NonNullable<CoreThreadJson['turns']>[number]>
): ThreadTimingMaps {
  const userIdByTurn = new Map<string, string>()
  for (const turn of turns) {
    const userItem = (turn.items ?? []).find((item) => item.kind === 'user_message')
    if (userItem?.id) userIdByTurn.set(turn.id, userItem.id)
  }

  const turnDurationByUserId: Record<string, number> = {}
  for (const turn of turns) {
    const userId = userIdByTurn.get(turn.id)
    if (!userId) continue
    const start = timestampMs(turn.startedAt) ?? timestampMs(turn.createdAt)
    const finish =
      timestampMs(turn.finishedAt) ??
      Math.max(
        0,
        ...(turn.items ?? [])
          .map((item) => timestampMs(item.finishedAt) ?? timestampMs(item.createdAt) ?? 0)
      )
    if (typeof start === 'number' && typeof finish === 'number' && finish >= start) {
      turnDurationByUserId[userId] = finish - start
    }
  }

  return { turnDurationByUserId }
}

function applyPendingGateProjection(
  blocks: ChatBlock[],
  pendingApprovalIds?: string[],
  pendingUserInputIds?: string[]
): ChatBlock[] {
  const hasApprovalProjection = pendingApprovalIds !== undefined
  const hasUserInputProjection = pendingUserInputIds !== undefined
  if (!hasApprovalProjection && !hasUserInputProjection) return blocks
  const pendingApprovals = new Set(pendingApprovalIds ?? [])
  const pendingInputs = new Set(pendingUserInputIds ?? [])
  let changed = false
  const next = blocks.map((block): ChatBlock => {
    if (block.kind === 'approval' && hasApprovalProjection) {
      const live = pendingApprovals.has(block.approvalId) || pendingApprovals.has(block.id)
      if (live && block.status !== 'pending') {
        changed = true
        return { ...block, status: 'pending' }
      }
      if (!live && block.status === 'pending') {
        changed = true
        return {
          ...block,
          status: 'error',
          errorMessage: block.errorMessage ?? 'Approval is no longer pending.'
        }
      }
    }
    if (block.kind === 'user_input' && hasUserInputProjection) {
      const live = pendingInputs.has(block.requestId) || pendingInputs.has(block.id)
      if (live && block.status !== 'pending') {
        changed = true
        return { ...block, status: 'pending' }
      }
      if (!live && block.status === 'pending') {
        changed = true
        return { ...block, status: 'cancelled' }
      }
    }
    return block
  })
  return changed ? next : blocks
}

function readRuntimeJson<T>(body: string, fallback: string): T {
  try {
    return JSON.parse(body) as T
  } catch {
    throw runtimeErrorToError({ code: 'runtime_response_schema_invalid', message: fallback })
  }
}

function readRuntimeSchema<T>(body: string, fallback: string, schema: ZodType<T>): T {
  let value: unknown
  try {
    value = JSON.parse(body) as unknown
  } catch {
    throw runtimeErrorToError({ code: 'runtime_response_schema_invalid', message: fallback })
  }
  const parsed = schema.safeParse(value)
  if (!parsed.success) {
    throw runtimeErrorToError({ code: 'runtime_response_schema_invalid', message: fallback })
  }
  return parsed.data
}

function assertThreadSummaryTaskIdentityV1(actual: string, expected: string, fallback: string): void {
  if (!threadSummaryTaskIdentityMatchesV1(actual, expected)) {
    throw runtimeErrorToError({ code: 'runtime_response_schema_invalid', message: fallback })
  }
}

function requestModelSelectionFromSettings(options: {
  settings: AppSettingsV1
  explicitProviderId?: string
  model?: string
  runtimeProviderId?: string
}): { model: string; providerId?: string } {
  const model = options.model?.trim() ?? ''
  const explicitProviderId = options.explicitProviderId?.trim()
  if (model.toLowerCase() === 'auto') {
    return { model, providerId: explicitProviderId || undefined }
  }
  if (explicitProviderId) {
    const normalizedExplicitProviderId = normalizeModelProviderId(explicitProviderId)
    const providerKnown = getModelProviderSettings(options.settings).providers
      .some((provider) => provider.id === normalizedExplicitProviderId)
    if (!providerKnown) {
      return { model, providerId: explicitProviderId }
    }
  }
  const selection = resolveModelProviderRequestSelection(options.settings, {
    model,
    providerId: explicitProviderId,
    runtimeProviderId: options.runtimeProviderId
  })
  return {
    model: selection.model,
    providerId: selection.providerId
  }
}

/**
 * GUI-side adapter for the Analytix HTTP/SSE contract.
 *
 * The provider owns renderer orchestration only: HTTP calls, SSE
 * reconnection, and approval policy decisions. DTO and chat-block
 * mapping live in `analytix-contract.ts` and `analytix-mapper.ts`.
 */
export class AnalytixRuntimeProvider implements AgentProvider {
  readonly id = 'analytix' as const
  readonly displayName = 'Analytix'

  getCapabilities(): {
    interrupt: boolean
    stream: boolean
    approvals: boolean
    attachFiles: boolean
    review: boolean
  } {
    return { interrupt: true, stream: true, approvals: true, attachFiles: true, review: true }
  }

  async connect(): Promise<void> {
    const health = await rendererRuntimeClient.runtimeRequest(ANALYTIX_HEALTH_PATH, 'GET')
    if (!health.ok) {
      throw runtimeErrorToError(readRuntimeError(health.body, `runtime unhealthy (${health.status || 0})`))
    }
    const threads = await rendererRuntimeClient.runtimeRequest(`${ANALYTIX_THREADS_PATH}?limit=1`, 'GET')
    if (!threads.ok) {
      throw runtimeErrorToError(readRuntimeError(threads.body, `failed to list threads (${threads.status || 0})`))
    }
  }

  async listThreads(options: ThreadListOptions = {}): Promise<NormalizedThread[]> {
    const query = buildQuery({
      limit: options.limit ?? 50,
      search: options.search,
      include_archived: options.includeArchived,
      archived_only: options.archivedOnly,
      include: options.includeSide ? 'side' : undefined
    })
    const response = await rendererRuntimeClient.runtimeRequest(`${ANALYTIX_THREADS_PATH}${query}`, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to list threads'))
    }
    const body = readRuntimeJson<{ threads: CoreThreadSummaryJson[] }>(
      response.body,
      'runtime returned an invalid thread list response'
    )
    return filterRevokedPublicProjectionThreads(body.threads.map(threadFromCore))
  }

  async listCaseProjects(limit = 100): Promise<NormalizedCaseProjectListResult> {
    const query = buildQuery({ limit })
    const response = await rendererRuntimeClient.runtimeRequest(`${ANALYTIX_CASE_PROJECTS_PATH}${query}`, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to list case projects'))
    }
    const body = readRuntimeJson<CoreCaseProjectListResponseJson>(
      response.body,
      'runtime returned an invalid case project list response'
    )
    return {
      caseProjects: body.caseProjects.map(caseProjectFromCore),
      indexStatus: body.indexStatus === 'building' ? 'building' : 'ready'
    }
  }

  async listCaseProjectThreads(caseProjectId: string, limit = 200): Promise<NormalizedThread[]> {
    const query = buildQuery({ limit })
    const response = await rendererRuntimeClient.runtimeRequest(
      `${analytixCaseProjectThreadsPath(caseProjectId)}${query}`,
      'GET'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to list case project threads'))
    }
    const body = readRuntimeJson<CoreCaseProjectThreadsResponseJson>(
      response.body,
      'runtime returned an invalid case project thread response'
    )
    return filterRevokedPublicProjectionThreads(body.threads.map(threadFromCore))
  }

  async getCaseProjectDetail(caseProjectId: string, limit = 200): Promise<{
    project: NormalizedCaseProject | null
    threads: NormalizedThread[]
    raw: CoreCaseProjectDetailResponseJson
  }> {
    const query = buildQuery({ limit })
    const response = await rendererRuntimeClient.runtimeRequest(
      `${analytixCaseProjectDetailPath(caseProjectId)}${query}`,
      'GET'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load case project detail'))
    }
    const raw = readRuntimeJson<CoreCaseProjectDetailResponseJson>(
      response.body,
      'runtime returned an invalid case project detail response'
    )
    const threads = filterRevokedPublicProjectionThreads(raw.threads.map(threadFromCore))
    const allowedThreadIds = new Set(threads.map((thread) => thread.id))
    return {
      project: raw.project ? caseProjectFromCore(raw.project) : null,
      threads,
      raw: {
        ...raw,
        threads: raw.threads.filter((thread) => allowedThreadIds.has(thread.id))
      }
    }
  }

  async createThread(input: {
    workspace?: string
    title?: string
    autoTitle?: boolean
    mode?: AnalytixThreadMode
    model?: string
    providerId?: string
  }): Promise<NormalizedThread> {
    const settings = await rendererRuntimeClient.getSettings()
    const runtime = getAnalytixRuntimeSettings(settings)
    const rawModel = input.model?.trim() || runtime.model
    const selection = requestModelSelectionFromSettings({
      settings,
      explicitProviderId: input.providerId,
      model: rawModel,
      runtimeProviderId: runtime.providerId
    })
    const response = await rendererRuntimeClient.runtimeRequest(
      ANALYTIX_THREADS_PATH,
      'POST',
      JSON.stringify({
        workspace: input.workspace || settings.workspaceRoot || '~',
        title: input.title,
        autoTitle: input.autoTitle,
        model: selection.model,
        providerId: selection.providerId,
        mode: normalizeThreadMode(input.mode),
        approvalPolicy: runtime.approvalPolicy,
        sandboxMode: runtime.sandboxMode
      })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to create thread'))
    }
    return threadFromCore(readRuntimeJson<CoreThreadJson>(
      response.body,
      'runtime returned an invalid thread response'
    ))
  }

  async getThreadDetail(threadId: string): Promise<{
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
    goal?: NormalizedThread['goal']
    todos?: NormalizedThread['todos']
    historyAuthority?: CoreThreadJson['historyAuthority']
    latestTurnAcceptedFinalDigest?: string
  }> {
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const response = await rendererRuntimeClient.runtimeRequest(analytixThreadPath(threadId), 'GET')
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load thread'))
    }
    const thread = readRuntimeJson<CoreThreadJson>(
      response.body,
      'runtime returned an invalid thread response'
    )
    if (thread.id !== threadId) throw new Error('Runtime thread response identity mismatch.')
    const turns = Array.isArray(thread.turns) ? thread.turns : []
    const caseBound = thread.historyAuthority === 'case_boundary_only_v1'
    const acceptedProjectionByTurn = acceptedFinalProjectionsForTurns(
      turns,
      thread.acceptedFinalDeliveries
    )
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const items = turns.flatMap((turn) =>
      (turn.items ?? [])
        .filter((item) => {
          if (!caseBound || item.kind !== 'assistant_text' ||
              acceptedProjectionByTurn.get(turn.id)?.assistant.id === item.id) return true
          // Main verifies ordinary-result digests before IPC. This closed shape
          // preserves ordinary display only; it cannot grant accepted-final authority.
          const ordinary = AssistantTextTurnItem.safeParse(item)
          return ordinary.success && ordinary.data.ordinaryResult !== undefined &&
            ordinary.data.role === 'assistant' && ordinary.data.status === 'completed' &&
            ordinary.data.threadId === threadId && turn.threadId === threadId &&
            ordinary.data.turnId === turn.id
        })
        .map((item) => ({
          ...item,
          workspaceCheckpointId: item.workspaceCheckpointId ?? turn.workspaceCheckpointId,
          attachmentIds: turn.attachmentIds,
          activeSkillIds: turn.activeSkillIds,
          injectedMemoryIds: turn.injectedMemoryIds,
          skillInjectionBytes: turn.skillInjectionBytes
        }))
    )
    const timelineItems = timelineItemsFromThreadItems(items)
    const timingMaps = deriveThreadTimingMaps(turns)
    const rawBlocks = mergeChatBlocks(timelineItems.flatMap((item) => {
      const projection = item.turnId ? acceptedProjectionByTurn.get(item.turnId) : undefined
      if (projection?.assistant.id === item.id) {
        return [{
          ...projection.assistant,
          acceptedFinalProjectionReceipt: projection.receipt,
          acceptedFinalProjectionTerminal: projection.terminal
        }]
      }
      const block = chatBlockFromItem(item)
      return block ? [block] : []
    }))
    const pendingApprovalIds = validStringArray(thread.pendingApprovalIds)
    const pendingUserInputIds = validStringArray(thread.pendingUserInputIds)
    const blocks = applyPendingGateProjection(rawBlocks, pendingApprovalIds, pendingUserInputIds)
    const latestTurn = turns.at(-1)
    const latestUserMessageId = [...items].reverse().find((item) => item.kind === 'user_message')?.id
    return {
      thread: threadFromCore(thread),
      blocks,
      latestSeq: thread.latestSeq ?? 0,
      threadStatus: thread.status ?? latestTurn?.status,
      latestTurnId: latestTurn?.id,
      latestUserMessageId,
      ...timingMaps,
      pendingApprovalIds,
      pendingUserInputIds,
      usage: thread.usage ? usageFromCore(thread.usage) : undefined,
      goal: thread.goal ? goalFromCore(thread.goal) : null,
      todos: thread.todos ? todosFromCore(thread.todos) : null,
      historyAuthority: thread.historyAuthority,
      latestTurnAcceptedFinalDigest:
        latestTurn && acceptedProjectionByTurn.has(latestTurn.id)
          ? acceptedProjectionByTurn.get(latestTurn.id)?.publicationCommitId
          : undefined
    }
  }

  async getThreadSummary(threadId: string): Promise<CoreThreadSummaryResponseJson> {
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const response = await rendererRuntimeClient.runtimeRequest(analytixThreadSummaryPath(threadId), 'GET')
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load thread summary'))
    }
    const summary = readRuntimeSchema(
      response.body,
      'runtime returned an invalid thread summary response',
      ThreadSummaryResponseSchema
    )
    if (summary.threadId !== threadId) {
      throw runtimeErrorToError({
        code: 'runtime_response_schema_invalid',
        message: 'runtime returned an invalid thread summary response'
      })
    }
    return summary
  }

  async getThreadSummaryTaskOutput(
    threadId: string,
    taskId: string,
    options: { offset?: number; limit?: number } = {}
  ): Promise<CoreThreadSummaryTaskOutputResponseJson> {
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const query = buildQuery({
      offset: options.offset,
      limit: options.limit
    })
    const response = await rendererRuntimeClient.runtimeRequest(
      `${analytixThreadSummaryTaskOutputPath(threadId, taskId)}${query}`,
      'GET'
    )
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to read summary task output'))
    }
    const fallback = 'runtime returned an invalid summary task output response'
    const output = readRuntimeSchema(response.body, fallback, ThreadSummaryTaskOutputResponse)
    assertThreadSummaryTaskIdentityV1(output.taskId, taskId, fallback)
    return output
  }

  async killThreadSummaryTask(
    threadId: string,
    taskId: string
  ): Promise<CoreThreadSummaryTaskMutationResponseJson> {
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadSummaryTaskKillPath(threadId, taskId),
      'POST'
    )
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to kill summary task'))
    }
    const mutation = readRuntimeSchema(
      response.body,
      'runtime returned an invalid summary task mutation response',
      ThreadSummaryTaskMutationResponseSchema
    )
    assertThreadSummaryTaskIdentityV1(
      mutation.task.id,
      taskId,
      'runtime returned an invalid summary task mutation response'
    )
    return mutation
  }

  async restartThreadSummaryTask(
    threadId: string,
    taskId: string
  ): Promise<CoreThreadSummaryTaskMutationResponseJson> {
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadSummaryTaskRestartPath(threadId, taskId),
      'POST'
    )
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to restart summary task'))
    }
    const mutation = readRuntimeSchema(
      response.body,
      'runtime returned an invalid summary task mutation response',
      ThreadSummaryTaskMutationResponseSchema
    )
    assertThreadSummaryTaskIdentityV1(
      mutation.task.id,
      taskId,
      'runtime returned an invalid summary task mutation response'
    )
    return mutation
  }

  async sendUserMessage(
    threadId: string,
    text: string,
    options?: {
      mode?: AnalytixThreadMode
      model?: string
      providerId?: string
      reasoningEffort?: import('@shared/app-settings').ModelReasoningEffort
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
      fileReferences?: Array<{ path: string; relativePath: string; name: string; kind?: 'file' | 'directory' }>
      workspaceCheckpointId?: string
    }
  ): Promise<{ turnId: string; threadId: string; userMessageItemId?: string }> {
    const settings = await rendererRuntimeClient.getSettings()
    const runtime = getAnalytixRuntimeSettings(settings)
    const rawModel = options?.model?.trim() || undefined
    const selection = rawModel
      ? requestModelSelectionFromSettings({
          settings,
          explicitProviderId: options?.providerId,
          model: rawModel,
          runtimeProviderId: runtime.providerId
        })
      : { model: '', providerId: options?.providerId?.trim() || undefined }
    const model = rawModel ? selection.model : undefined
    const body: Record<string, unknown> = {
      prompt: text,
      async: true,
      model,
      providerId: selection.providerId,
      approvalPolicy: runtime.approvalPolicy,
      sandboxMode: runtime.sandboxMode
    }
    const reasoningEffort = validateOptionalModelReasoningEffortV1(options?.reasoningEffort)
    if (reasoningEffort) {
      body.reasoningEffort = reasoningEffort
    }
    if (options?.riskIntent === 'case') {
      body.riskIntent = 'case'
    }
    if (options?.displayText?.trim() && options.displayText.trim() !== text.trim()) {
      body.displayText = options.displayText.trim()
    }
    const mode = options?.mode
    if (mode === 'agent' || mode === 'plan') {
      body.mode = mode
    }
    if (options?.guiPlan) {
      body.guiPlan = {
        operation: options.guiPlan.operation,
        workspaceRoot: options.guiPlan.workspaceRoot,
        relativePath: options.guiPlan.relativePath,
        planId: options.guiPlan.planId,
        sourceRequest: options.guiPlan.sourceRequest,
        title: options.guiPlan.title
      }
    }
    if (options?.attachmentIds?.length) {
      body.attachmentIds = options.attachmentIds
    }
    if (options?.fileReferences?.length) {
      body.fileReferences = options.fileReferences
    }
    if (options?.workspaceCheckpointId?.trim()) {
      body.workspaceCheckpointId = options.workspaceCheckpointId.trim()
    }
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadTurnsPath(threadId),
      'POST',
      JSON.stringify(body)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to start turn'))
    }
    const parsed = readRuntimeJson<CoreStartTurnResponseJson>(
      response.body,
      'runtime returned an invalid turn response'
    )
    return {
      threadId: parsed.threadId,
      turnId: parsed.turnId,
      userMessageItemId: parsed.userMessageItemId
    }
  }

  async reviewThread(
    threadId: string,
    target: ReviewTarget,
    options?: { model?: string; providerId?: string }
  ): Promise<{ turnId: string; threadId: string; userMessageItemId?: string; reviewItemId?: string }> {
    const settings = await rendererRuntimeClient.getSettings()
    const runtime = getAnalytixRuntimeSettings(settings)
    const rawModel = options?.model?.trim() || ''
    const selection = rawModel
      ? requestModelSelectionFromSettings({
          settings,
          explicitProviderId: options?.providerId,
          model: rawModel,
          runtimeProviderId: runtime.providerId
        })
      : { model: '', providerId: options?.providerId?.trim() || undefined }
    const model = rawModel ? selection.model : ''
    const body: Record<string, unknown> = { target }
    if (model) {
      body.model = model
    }
    if (selection.providerId) {
      body.providerId = selection.providerId
    }
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadReviewPath(threadId),
      'POST',
      JSON.stringify(body)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to start review'))
    }
    const parsed = readRuntimeJson<CoreStartReviewResponseJson>(
      response.body,
      'runtime returned an invalid review response'
    )
    return {
      threadId: parsed.threadId,
      turnId: parsed.turnId,
      userMessageItemId: parsed.userMessageItemId,
      reviewItemId: parsed.reviewItemId
    }
  }

  async planCheckpointRewind(
    threadId: string,
    checkpointId: string,
    options?: { scope?: 'code' | 'conversation' | 'combined' }
  ) {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadCheckpointRewindPlanPath(threadId, checkpointId),
      'POST',
      JSON.stringify({ scope: options?.scope ?? 'combined' })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to create checkpoint rewind plan'))
    }
    const parsed = readRuntimeJson<CoreCheckpointRewindPlanResponseJson>(
      response.body,
      'runtime returned an invalid checkpoint rewind plan response'
    )
    return parsed.plan
  }

  async applyCheckpointRewind(
    threadId: string,
    checkpointId: string,
    plan: CoreCheckpointRewindPlanJson
  ): Promise<CoreCheckpointRewindApplyResultJson> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadCheckpointRewindApplyPath(threadId, checkpointId),
      'POST',
      JSON.stringify({
        plan,
        confirmation: {
          confirmed: true,
          destructive: true,
          phrase: 'APPLY_CHECKPOINT_REWIND'
        }
      })
    )
    if (!response.ok) {
      const parsed = readRuntimeJson<Partial<CoreCheckpointRewindApplyResponseJson>>(
        response.body,
        'runtime returned an invalid checkpoint rewind apply response'
      )
      if (parsed.apply) return parsed.apply
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to apply checkpoint rewind plan'))
    }
    const parsed = readRuntimeJson<CoreCheckpointRewindApplyResponseJson>(
      response.body,
      'runtime returned an invalid checkpoint rewind apply response'
    )
    return parsed.apply
  }

  async steerUserMessage(input: {
    threadId: string
    turnId: string
    text: string
    displayText?: string
    clientUserMessageId?: string
    expectedTurnId?: string
    attachmentIds?: string[]
    fileReferences?: Array<{ path: string; relativePath: string; name: string; kind?: 'file' | 'directory' }>
  }): Promise<{
    threadId: string
    turnId: string
    itemId?: string
    clientUserMessageId?: string
    admittedSeq?: number
  }> {
    const body: Record<string, unknown> = {
      text: input.text,
      expectedTurnId: input.expectedTurnId ?? input.turnId,
      delivery: 'steer'
    }
    if (input.displayText?.trim()) body.displayText = input.displayText.trim()
    if (input.clientUserMessageId?.trim()) body.clientUserMessageId = input.clientUserMessageId.trim()
    if (input.attachmentIds?.length) body.attachmentIds = input.attachmentIds
    if (input.fileReferences?.length) body.fileReferences = input.fileReferences
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadSteerPath(input.threadId, input.turnId),
      'POST',
      JSON.stringify(body)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to queue message'))
    }
    const parsed = readRuntimeJson<{
      ok?: boolean
      threadId?: string
      turnId?: string
      itemId?: string
      clientUserMessageId?: string
      admittedSeq?: number
    }>(response.body, 'runtime returned an invalid steer response')
    return {
      threadId: parsed.threadId ?? input.threadId,
      turnId: parsed.turnId ?? input.turnId,
      itemId: parsed.itemId,
      clientUserMessageId: parsed.clientUserMessageId,
      admittedSeq: parsed.admittedSeq
    }
  }

  async rewindThread(threadId: string, turnId: string) {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadRewindPath(threadId),
      'POST',
      JSON.stringify({ turnId })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to rewind thread'))
    }
    return readRuntimeJson<{
      threadId: string
      turnId: string
      removedTurns: number
      remainingTurns: number
      removedTurnIds?: string[]
    }>(response.body, 'runtime returned an invalid rewind response')
  }

  async interruptTurn(threadId: string, turnId: string, options?: { discard?: boolean }): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadInterruptPath(threadId, turnId),
      'POST',
      JSON.stringify({ discard: options?.discard === true })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to interrupt turn'))
    }
  }

  async renameThread(threadId: string, title: string): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadPath(threadId),
      'PATCH',
      JSON.stringify({ title })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'rename thread failed'))
    }
  }

  async updateThreadWorkspace(threadId: string, workspace: string): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadPath(threadId),
      'PATCH',
      JSON.stringify({ workspace })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'update thread workspace failed'))
    }
  }

  async updateThreadRelation(threadId: string, relation: NonNullable<NormalizedThread['relation']>): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadPath(threadId),
      'PATCH',
      JSON.stringify({ relation })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'update thread relation failed'))
    }
  }

  async archiveThread(threadId: string, archived: boolean): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadPath(threadId),
      'PATCH',
      JSON.stringify({ status: archived ? 'archived' : 'idle' })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'archive thread failed'))
    }
  }

  async deleteThread(threadId: string): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(analytixThreadPath(threadId), 'DELETE')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'delete thread failed'))
    }
  }

  async compactThread(threadId: string, reason?: string): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadCompactPath(threadId),
      'POST',
      JSON.stringify({ reason: reason?.trim() || undefined })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'compact thread failed'))
    }
  }

  async getThreadGoal(threadId: string): Promise<NonNullable<NormalizedThread['goal']> | null> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadGoalPath(threadId),
      'GET'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load thread goal'))
    }
    const body = readRuntimeJson<CoreThreadGoalResponseJson>(
      response.body,
      'runtime returned an invalid thread goal response'
    )
    return body.goal ? goalFromCore(body.goal) : null
  }

  async setThreadGoal(
    threadId: string,
    patch: {
      objective?: string
      status?: NonNullable<NormalizedThread['goal']>['status']
      tokenBudget?: number | null
      strictCompletion?: boolean
      selfCheckRequired?: boolean
      selfCheckCompleted?: boolean
      selfCheckTurnId?: string
      research?: { enabled: true; requirements?: string[] }
    }
  ): Promise<NonNullable<NormalizedThread['goal']>> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadGoalPath(threadId),
      'POST',
      JSON.stringify(patch)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to set thread goal'))
    }
    const body = readRuntimeJson<CoreThreadGoalResponseJson>(
      response.body,
      'runtime returned an invalid thread goal response'
    )
    if (!body.goal) {
      throw runtimeErrorToError({
        code: 'unknown',
        message: 'set thread goal returned an invalid response'
      })
    }
    return goalFromCore(body.goal)
  }

  async clearThreadGoal(threadId: string): Promise<boolean> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadGoalPath(threadId),
      'DELETE'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to clear thread goal'))
    }
    return readRuntimeJson<CoreClearThreadGoalResponseJson>(
      response.body,
      'runtime returned an invalid clear thread goal response'
    ).cleared
  }

  async getThreadTodos(threadId: string): Promise<NonNullable<NormalizedThread['todos']> | null> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadTodosPath(threadId),
      'GET'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load thread todos'))
    }
    const body = readRuntimeJson<CoreThreadTodosResponseJson>(
      response.body,
      'runtime returned an invalid thread todos response'
    )
    return body.todos ? todosFromCore(body.todos) : null
  }

  async setThreadTodos(
    threadId: string,
    todos: Parameters<NonNullable<AgentProvider['setThreadTodos']>>[1]
  ): Promise<NonNullable<NormalizedThread['todos']>> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadTodosPath(threadId),
      'POST',
      JSON.stringify({ todos })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to set thread todos'))
    }
    const body = readRuntimeJson<CoreThreadTodosResponseJson>(
      response.body,
      'runtime returned an invalid thread todos response'
    )
    if (!body.todos) {
      throw runtimeErrorToError({
        code: 'unknown',
        message: 'set thread todos returned an invalid response'
      })
    }
    return todosFromCore(body.todos)
  }

  async clearThreadTodos(threadId: string): Promise<boolean> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadTodosPath(threadId),
      'DELETE'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to clear thread todos'))
    }
    return readRuntimeJson<CoreClearThreadTodosResponseJson>(
      response.body,
      'runtime returned an invalid clear thread todos response'
    ).cleared
  }

  async submitApprovalDecision(
    approvalId: string,
    decision: 'allow' | 'deny'
  ): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixApprovalPath(approvalId),
      'POST',
      JSON.stringify({ decision })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'approval decision failed'))
    }
  }

  async submitUserInputResponse(inputId: string, answers: UserInputAnswer[]): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixUserInputPath(inputId),
      'POST',
      JSON.stringify({ answers })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'request_user_input response failed'))
    }
  }

  async cancelUserInput(inputId: string): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixUserInputPath(inputId),
      'POST',
      JSON.stringify({ cancelled: true })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'request_user_input cancel failed'))
    }
  }

  async getRuntimeInfo(): Promise<CoreRuntimeInfoJson> {
    const response = await rendererRuntimeClient.runtimeRequest(ANALYTIX_RUNTIME_INFO_PATH, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load runtime info'))
    }
    return readRuntimeSchema(
      response.body,
      'runtime returned an invalid runtime info response',
      RuntimeInfoResponseSchema
    )
  }

  async getToolDiagnostics(): Promise<CoreRuntimeToolDiagnosticsJson> {
    const response = await rendererRuntimeClient.runtimeRequest(ANALYTIX_RUNTIME_TOOLS_PATH, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load runtime diagnostics'))
    }
    return readRuntimeSchema(
      response.body,
      'runtime returned an invalid runtime diagnostics response',
      RuntimeToolsResponseSchema
    )
  }

  async listSkills(): Promise<CoreRuntimeSkillsResponseJson> {
    const response = await rendererRuntimeClient.runtimeRequest(ANALYTIX_SKILLS_PATH, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to list skills'))
    }
    return readRuntimeSchema(
      response.body,
      'runtime returned an invalid skills response',
      RuntimeSkillsResponseSchema
    )
  }

  async uploadAttachment(input: {
    name: string
    mimeType?: string
    dataBase64: string
    documentText?: string
    pageCount?: number
    textFallback?: CoreAttachmentTextFallbackJson
    threadId: string
    workspace: string
  }): Promise<CoreAttachmentMetadataJson> {
    const response = await rendererRuntimeClient.runtimeRequest(
      ANALYTIX_ATTACHMENTS_PATH,
      'POST',
      JSON.stringify(input)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'attachment upload failed'))
    }
    return readRuntimeJson<CoreAttachmentUploadResponseJson>(
      response.body,
      'runtime returned an invalid attachment upload response'
    ).attachment
  }

  async getAttachmentDiagnostics(): Promise<CoreAttachmentDiagnosticsJson> {
    const response = await rendererRuntimeClient.runtimeRequest(ANALYTIX_ATTACHMENT_DIAGNOSTICS_PATH, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load attachment diagnostics'))
    }
    return readRuntimeSchema(
      response.body,
      'runtime returned an invalid attachment diagnostics response',
      AttachmentDiagnosticsResponseSchema
    )
  }

  async getAttachmentContent(
    attachmentId: string,
    options: { threadId: string; workspace: string }
  ): Promise<CoreAttachmentContentResponseJson> {
    const query = buildQuery({
      thread_id: options.threadId,
      workspace: options.workspace
    })
    const response = await rendererRuntimeClient.runtimeRequest(
      `${analytixAttachmentContentPath(attachmentId)}${query}`,
      'GET'
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load attachment content'))
    }
    return readRuntimeJson<CoreAttachmentContentResponseJson>(
      response.body,
      'runtime returned an invalid attachment content response'
    )
  }

  async listMemories(options: { workspace?: string; includeDeleted?: boolean } = {}): Promise<CoreMemoryRecordJson[]> {
    const query = buildQuery({
      workspace: options.workspace,
      include_deleted: options.includeDeleted
    })
    const response = await rendererRuntimeClient.runtimeRequest(`${ANALYTIX_MEMORY_PATH}${query}`, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to list memories'))
    }
    return readRuntimeJson<CoreMemoryListResponseJson>(
      response.body,
      'runtime returned an invalid memory list response'
    ).memories ?? []
  }

  async createMemory(input: {
    content: string
    scope?: 'user' | 'workspace' | 'project'
    workspace?: string
    project?: string
    tags?: string[]
    confidence?: number
  }): Promise<CoreMemoryRecordJson> {
    const response = await rendererRuntimeClient.runtimeRequest(
      ANALYTIX_MEMORY_PATH,
      'POST',
      JSON.stringify(input)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to create memory'))
    }
    return readRuntimeJson<{ memory: CoreMemoryRecordJson }>(
      response.body,
      'runtime returned an invalid memory response'
    ).memory
  }

  async updateMemory(
    memoryId: string,
    patch: { content?: string; tags?: string[]; confidence?: number; disabled?: boolean }
  ): Promise<CoreMemoryRecordJson> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixMemoryRecordPath(memoryId),
      'PATCH',
      JSON.stringify(patch)
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to update memory'))
    }
    return readRuntimeJson<{ memory: CoreMemoryRecordJson }>(
      response.body,
      'runtime returned an invalid memory response'
    ).memory
  }

  async deleteMemory(memoryId: string): Promise<CoreMemoryRecordJson> {
    const response = await rendererRuntimeClient.runtimeRequest(analytixMemoryRecordPath(memoryId), 'DELETE')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to delete memory'))
    }
    return readRuntimeJson<{ memory: CoreMemoryRecordJson }>(
      response.body,
      'runtime returned an invalid memory response'
    ).memory
  }

  async getMemoryDiagnostics(): Promise<CoreMemoryDiagnosticsJson> {
    const response = await rendererRuntimeClient.runtimeRequest(ANALYTIX_MEMORY_DIAGNOSTICS_PATH, 'GET')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load memory diagnostics'))
    }
    return readRuntimeSchema(
      response.body,
      'runtime returned an invalid memory diagnostics response',
      MemoryDiagnosticsResponseSchema
    )
  }

  async forkThread(
    threadId: string,
    options?: { relation?: 'primary' | 'fork' | 'side'; title?: string; turnId?: string }
  ): Promise<NormalizedThread> {
    const body: Record<string, unknown> = {}
    if (options?.relation) body.relation = options.relation
    if (options?.title) body.title = options.title
    if (options?.turnId) body.turnId = options.turnId
    const url = analytixThreadForkPath(threadId)
    const response =
      Object.keys(body).length > 0
        ? await rendererRuntimeClient.runtimeRequest(url, 'POST', JSON.stringify(body))
        : await rendererRuntimeClient.runtimeRequest(url, 'POST')
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'fork thread failed'))
    }
    return threadFromCore(readRuntimeJson<CoreThreadJson>(
      response.body,
      'runtime returned an invalid thread response'
    ))
  }

  async resumeSession(
    sessionId: string,
    options?: { model?: string; providerId?: string; mode?: AnalytixThreadMode }
  ): Promise<{ threadId: string; sessionId: string }> {
    const settings = await rendererRuntimeClient.getSettings()
    const runtime = getAnalytixRuntimeSettings(settings)
    const rawModel = options?.model?.trim() || runtime.model
    const selection = requestModelSelectionFromSettings({
      settings,
      explicitProviderId: options?.providerId,
      model: rawModel,
      runtimeProviderId: runtime.providerId
    })
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixSessionResumePath(sessionId),
      'POST',
      JSON.stringify({
        workspace: settings.workspaceRoot || undefined,
        model: selection.model,
        providerId: selection.providerId,
        mode: options?.mode
      })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'resume session failed'))
    }
    const body = readRuntimeJson<CoreResumeSessionResponseJson>(
      response.body,
      'runtime returned an invalid resume session response'
    )
    const threadId = body.thread_id ?? body.threadId
    if (!threadId) {
      throw runtimeErrorToError({
        code: 'unknown',
        message: 'resume session returned an invalid response'
      })
    }
    return { threadId, sessionId: body.session_id ?? body.sessionId ?? sessionId }
  }

  async subscribeThreadEvents(
    threadId: string,
    sinceSeq: number,
    sink: ThreadEventSink,
    signal: AbortSignal
  ): Promise<void> {
    const streamId = createSseStreamId()
    await new Promise<void>(async (resolve) => {
      let settled = false
      const pendingDispatches = new Set<Promise<void>>()
      let dispatchChain: Promise<void> = Promise.resolve()
      const emitSseTransportError = (message: string, code: string, status?: number): void => {
        sink.onRuntimeError?.({
          itemId: `runtime_error_${threadId}_${streamId}_${code}`,
          createdAt: new Date().toISOString(),
          message,
          code,
          ...(status !== undefined ? { details: { status } } : {}),
          severity: 'error'
        })
        void sink.onError(new Error(JSON.stringify({
          code,
          message,
          ...(status !== undefined ? { details: { status } } : {}),
          severity: 'error'
        })), { terminal: false })
      }
      const finish = (): void => {
        if (settled) return
        settled = true
        offData()
        offEnd()
        offErr()
        signal.removeEventListener('abort', onAbort)
        void Promise.allSettled([...pendingDispatches]).then(() => resolve())
      }
      const offData = rendererRuntimeClient.onSseEvent((payload) => {
        if (payload.streamId !== streamId) return
        if (!isPublicSseIpcPayload(payload)) return
        const rawEvents = Array.isArray(payload.events) ? payload.events : []
        const containsRevocation = rawEvents.some((entry) =>
          entry !== null && typeof entry === 'object' &&
          (entry as { kind?: unknown }).kind === 'public_projection_revoked'
        )
        if (containsRevocation) {
          const parsed = rawEvents.length === 1
            ? PublicProjectionRevokedEventSchema.safeParse(rawEvents[0])
            : null
          if (!parsed?.success || parsed.data.threadId !== threadId) {
            emitSseTransportError(
              'Runtime request failed (sse_event_rejected).',
              'sse_event_rejected'
            )
            finish()
            return
          }
          const task = dispatchChain.then(async () => {
            try {
              await sink.onPublicProjectionRevoked?.(parsed.data)
            } finally {
              finish()
            }
          }).finally(() => {
            pendingDispatches.delete(task)
          })
          dispatchChain = task.catch(() => undefined)
          pendingDispatches.add(task)
          return
        }
        const containsAcceptedFinalBatch = rawEvents.some((entry) =>
          entry !== null && typeof entry === 'object' &&
          (entry as { kind?: unknown }).kind === 'accepted_final_batch'
        )
        const containsGeneralTerminalBatch = rawEvents.some((entry) =>
          entry !== null && typeof entry === 'object' &&
          (entry as { kind?: unknown }).kind === 'general_terminal_batch'
        )
        if ((containsAcceptedFinalBatch || containsGeneralTerminalBatch) && rawEvents.length !== 1) {
          emitSseTransportError(
            'Runtime request failed (sse_event_rejected).',
            'sse_event_rejected'
          )
          finish()
          return
        }
        const batch = rawEvents.map((entry): unknown =>
          entry && typeof entry === 'object' ? entry : {}
        )
        if (batch.length === 0) return
        let maxSeq: number | null = null
        for (const event of batch) {
          const seq = (event as { seq?: unknown }).seq
          if (typeof seq === 'number') {
            maxSeq = maxSeq === null ? seq : Math.max(maxSeq, seq)
          }
        }
        const task = dispatchChain.then(async () => {
          try {
            const committedReceipt = await dispatchAnalytixRuntimeEvents(batch, sink, (runtimeEvent, eventSink) =>
              this.handleApprovalRequest(runtimeEvent, eventSink)
            )
            let acceptedFinalAck: AcceptedFinalSseAckBindingV1 | undefined
            if (containsAcceptedFinalBatch) {
              if (!committedReceipt || committedReceipt.threadId !== threadId ||
                  committedReceipt.lastSeq !== maxSeq) {
                throw new Error('accepted-final renderer commit receipt is missing')
              }
              acceptedFinalAck = {
                batchId: committedReceipt.batchId,
                threadId: committedReceipt.threadId,
                turnId: committedReceipt.turnId,
                publicationCommitId: committedReceipt.publicationCommitId
              }
            } else if (committedReceipt) {
              throw new Error('ordinary SSE dispatch returned an accepted-final receipt')
            }
            if (containsAcceptedFinalBatch && signal.aborted) return
            if (maxSeq !== null) {
              if (!containsAcceptedFinalBatch) sink.onSeq(maxSeq)
              const acknowledged = await rendererRuntimeClient.ackSseEvent(
                streamId,
                maxSeq,
                acceptedFinalAck
              ).catch(() => false)
              if (!acknowledged) {
                emitSseTransportError(
                  'Runtime request failed (sse_ack_rejected).',
                  'sse_ack_rejected'
                )
                finish()
              }
            }
          } catch {
            emitSseTransportError(
              'Runtime request failed (sse_event_rejected).',
              'sse_event_rejected'
            )
            finish()
          }
        }).finally(() => {
          pendingDispatches.delete(task)
        })
        dispatchChain = task.catch(() => undefined)
        pendingDispatches.add(task)
      })
      const offErr = rendererRuntimeClient.onSseError(({ streamId: sid, message, status, code }) => {
        if (sid !== streamId) return
        emitSseTransportError(message ?? `sse error ${status ?? ''}`, code ?? 'sse_stream_error', status)
        finish()
      })
      const offEnd = rendererRuntimeClient.onSseEnd(({ streamId: sid }) => {
        if (sid !== streamId) return
        finish()
      })
      const onAbort = (): void => {
        void rendererRuntimeClient.stopSse(streamId)
        finish()
      }
      if (signal.aborted) {
        onAbort()
        return
      }
      signal.addEventListener('abort', onAbort, { once: true })
      try {
        await rendererRuntimeClient.startSse(threadId, sinceSeq, streamId)
      } catch (error) {
        if (signal.aborted) {
          finish()
          return
        }
        emitSseTransportError(error instanceof Error ? error.message : String(error), 'sse_start_failed')
        finish()
      }
    })
    void rendererRuntimeClient.stopSse(streamId)
  }

  private async handleApprovalRequest(event: CoreRuntimeEventJson, sink: ThreadEventSink): Promise<void> {
    const approvalId = runtimeEventString(event, 'approvalId', 'approval_id', 'itemId', 'item_id') ?? ''
    if (!approvalId) return
    const turnId = runtimeEventString(event, 'turnId', 'turn_id')
    const toolName = runtimeEventString(event, 'toolName', 'tool_name')
    try {
      const eventPolicy = normalizeApprovalPolicy(runtimeEventString(event, 'approvalPolicy', 'approval_policy'))
      const policy = eventPolicy ?? getAnalytixRuntimeSettings(await rendererRuntimeClient.getSettings()).approvalPolicy
      switch (policy) {
        case 'auto':
          await this.submitApprovalDecision(approvalId, 'allow')
          return
        case 'never':
          await this.submitApprovalDecision(approvalId, 'deny')
          return
        case 'on-request':
        case 'suggest':
        case 'untrusted':
          break
      }
    } catch {
      /* Fall through and render the approval card. */
    }
    sink.onApproval({
      approvalId,
      summary: event.summary ?? 'Approval required',
      toolName,
      ...(turnId ? { turnId } : {}),
      ...(turnId || event.child
        ? {
            meta: {
              ...(turnId ? { turnId } : {}),
              ...(event.child ? { child: event.child } : {})
            }
          }
        : {})
    })
  }
}

export { analytixThreadEventsPath }
