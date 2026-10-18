import { trajectorySource } from '../components/trajectory/trajectory-source'
import { authoritativeTurnStatuses, authoritativeTurnTimings } from '../components/chat/presentation/chat/turn-timing'
import type {
  AuthoritativeTurnStatuses, AuthoritativeTurnTimings,
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
  TurnUsageById,
  UserInputSubmission, UserInputWaitState, UserInputWaitControls
} from './types'
import {
  getAnalytixRuntimeSettings,
  resolveModelProviderRequestSelection,
  type AppSettingsV1
} from '@shared/app-settings'
import { UserInputActionSelectorSchemaV1, type UserInputActionSelectorV1, UserInputLateReplyResponseSchemaV1, UserInputWaitScheduledResponseSchemaV1, UserInputMetadataResolutionResponse, UserInputResolutionResponse } from '../../../../packages/runtime/src/contracts/approvals'
import { userInputLocalSelectorSchemaV1, type UserInputLocalSelectorV1 } from '../../../../packages/runtime/src/contracts/user-input-local-display'
import { CompactResponse } from '../../../../packages/runtime/src/contracts/turns'
import type { UserInputWaitOwnerStateV1 } from '@shared/analytix-api'
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
  analytixAttachmentPath,
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
  createOrdinaryHistoryReplay,
  dispatchAnalytixRuntimeEvents,
  goalFromCore,
  goalHistoryBlocksFromCore,
  mergeGoalHistoryBlocks,
  mergeChatBlocks,
  todosFromCore,
  threadFromCore,
  usageFromCore,
  mergeTurnUsageById,
  turnUsageEntryFromBatch
} from './analytix-mapper'
import { rendererRuntimeClient } from './runtime-client'
import { isPublicSseIpcPayload } from '@shared/public-runtime-content'
import { isStrictPublicRuntimeSseIpcPayload } from '@shared/public-runtime-sse'
import {
  AssistantTextTurnItem,
  AcceptedFinalPublicViewV3Schema,
  acceptedFinalPublicViewsMatch
} from '../../../../packages/runtime/src/contracts/items.js'
import { PublicProjectionRevokedEvent as PublicProjectionRevokedEventSchema } from '../../../../packages/runtime/src/contracts/events.js'
import {
  ResumeThreadResponse as ResumeThreadResponseSchema,
  ThreadSummaryResponse as ThreadSummaryResponseSchema,
  ThreadSummaryTaskMutationResponse as ThreadSummaryTaskMutationResponseSchema,
  ThreadSummaryTaskOutputResponse
} from '../../../../packages/runtime/src/contracts/threads.js'
import { threadSummaryTaskIdentityMatchesV1 } from '../../../../packages/runtime/src/contracts/task-job-output.js'
import { AttachmentMetadataResponse } from '../../../../packages/runtime/src/contracts/attachments.js'
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
  isPublicProjectionRevoked,
  markPublicProjectionRevoked
} from '../lib/public-projection-revocation'

function userInputActionSelector(inputId: string, selector?: UserInputActionSelectorV1): UserInputActionSelectorV1 | undefined {
  // The absent form is the explicit original-owner compatibility command.
  // A supplied tuple must never fall back to that form after validation fails.
  if (selector === undefined) return undefined
  const parsed = UserInputActionSelectorSchemaV1.safeParse(selector)
  if (!parsed.success || parsed.data.requestId !== inputId) throw new Error('Question action owner is unavailable.')
  return parsed.data
}

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

// Public cancellation metadata describes one historical attempt. It carries
// no Host Question association, accepted reply, or executable composer state.
function discardedReplyHistoryBlocks(threadId: string, turn: CoreTurnJson): ChatBlock[] {
  return (turn.steering ?? []).flatMap(entry => {
    if (typeof entry === 'string' || entry.status !== 'cancelled' || entry.cancelReason !== 'user_input_discarded' ||
        !entry.id || !entry.cancelledAt || !Number.isFinite(Date.parse(entry.cancelledAt))) return []
    return [{ kind: 'system' as const, id: 'history:discard:' + JSON.stringify([threadId, turn.id, entry.id]),
      createdAt: entry.cancelledAt, text: '', code: 'history_user_input_discarded', meta: { turnId: turn.id } }]
  })
}

type ThreadTimingMaps = {
  turnDurationByUserId: Record<string, number>
}

function timestampMs(value: unknown): number | undefined {
  if (typeof value !== 'string' || !value.trim()) return undefined
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : undefined
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
      if (live && block.status === 'error') {
        changed = true
        return { ...block, status: 'pending' }
      }
      if (!live && block.status === 'pending') {
        changed = true
        return { ...block, status: 'error', errorMessage: 'User input outcome is unavailable.' }
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
    // Explicit selections come from the Core Registry. Legacy Settings may
    // contain an older model catalog; Core owns validation of this pair.
    return { model, providerId: explicitProviderId }
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
type WaitBinding = {
  selector: UserInputLocalSelectorV1; state: UserInputWaitState; deadline: number | null
  listeners: Set<(state: UserInputWaitState) => void>; timer?: ReturnType<typeof setTimeout>; epoch: number
  focusIntent: boolean; engageIntent: boolean; holdIntent: boolean; closing: boolean
}
export class AnalytixRuntimeProvider implements AgentProvider {
  // Request metadata belongs to this existing provider, independent of which
  // thread/card is mounted. It never retains private question/answer bytes.
  private userInputWaits = new Map<string, WaitBinding>()
  private waitOff?: () => void
  private publishWait(binding: WaitBinding): void {
    for (const listener of binding.listeners) listener({ ...binding.state })
  }
  private stopWaitClock(binding: WaitBinding): void {
    clearTimeout(binding.timer)
    binding.timer = undefined
    if (binding.deadline !== null) binding.state.remainingMs = Math.max(0, binding.deadline - Date.now())
    binding.deadline = null
  }
  private runWaitClock(binding: WaitBinding): void {
    clearTimeout(binding.timer)
    if (binding.state.phase !== 'counting' || binding.deadline === null) return
    binding.state.remainingMs = Math.max(0, binding.deadline - Date.now())
    this.publishWait(binding)
    if (binding.state.remainingMs === 0) {
      binding.deadline = null
      binding.state.phase = 'continuing'
      this.publishWait(binding)
      const epoch = binding.epoch, key = JSON.stringify(binding.selector)
      const current = () => this.userInputWaits.get(key) === binding && binding.epoch === epoch && binding.state.phase === 'continuing'
      void this.continueUserInput(binding.selector.requestId, binding.selector).then(() => {
        if (!current()) return
        binding.state.phase = 'scheduled'; this.publishWait(binding)
      }).catch(() => {
        if (!current()) return
        binding.state.phase = 'error'; this.publishWait(binding)
      })
      return
    }
    binding.timer = setTimeout(() => this.runWaitClock(binding), Math.min(1000, binding.state.remainingMs))
  }
  private acceptWait(binding: WaitBinding, wait: UserInputWaitOwnerStateV1): void {
    // Selector-only EOF belongs to the old held reader until native close confirms.
    if (binding.closing) return
    if (wait.state === 'waiting') {
      // A repeat/remount frame cannot reset an engaged or scheduled claim.
      if (binding.state.phase !== 'connecting') return
      const phase = binding.engageIntent ? 'editing' : binding.holdIntent || wait.snapshot.remainingMs === null ? 'waiting' : binding.focusIntent ? 'focused' : 'counting'
      binding.state = { phase, remainingMs: binding.holdIntent ? null : wait.snapshot.remainingMs }
      binding.deadline = phase !== 'counting' || wait.snapshot.remainingMs === null ? null : wait.observedAt + wait.snapshot.remainingMs
      this.runWaitClock(binding)
    } else if (wait.state === 'ended' || wait.state === 'unavailable') {
      this.stopWaitClock(binding)
      binding.state.phase = wait.state === 'ended' ? 'ended' : 'error'
    }
    this.publishWait(binding)
    if ((wait.state === 'ended' || wait.state === 'unavailable') && !binding.listeners.size) {
      this.userInputWaits.delete(JSON.stringify(binding.selector))
      if (!this.userInputWaits.size) { this.waitOff?.(); this.waitOff = undefined }
    }
  }
  observeUserInputWait(selector: UserInputLocalSelectorV1, listener: (state: UserInputWaitState) => void): UserInputWaitControls {
    const parsed = userInputLocalSelectorSchemaV1.safeParse(selector)
    const none = { focus() {}, blur() {}, engage() {}, takeTime() {}, retry() {}, unsubscribe() {} }
    if (!parsed.success) { listener({ phase: 'error', remainingMs: null }); return none }
    const key = JSON.stringify(parsed.data)
    if (!this.waitOff) this.waitOff = rendererRuntimeClient.onUserInputWaitChanged(event => {
      const current = this.userInputWaits.get(JSON.stringify(event.selector))
      if (current) this.acceptWait(current, event.wait)
    })
    let binding = this.userInputWaits.get(key)
    if (!binding) {
      if (this.userInputWaits.size >= 32) { listener({ phase: 'error', remainingMs: null }); return none }
      binding = { selector: parsed.data, state: { phase: 'connecting', remainingMs: null }, deadline: null, listeners: new Set(), epoch: 0, focusIntent: false, engageIntent: false, holdIntent: false, closing: false }
      this.userInputWaits.set(key, binding)
      const attached = binding, epoch = binding.epoch
      void rendererRuntimeClient.openUserInputWait(parsed.data).then(wait => {
        if (this.userInputWaits.get(key) === attached && attached.epoch === epoch) this.acceptWait(attached, wait)
      }).catch(() => {
        if (this.userInputWaits.get(key) === attached && attached.epoch === epoch) this.acceptWait(attached, { state: 'unavailable' })
      })
    }
    const selected = binding
    selected.listeners.add(listener)
    listener({ ...selected.state })
    const current = () => this.userInputWaits.get(key) === selected
    return {
      focus: () => {
        if (!current()) return
        selected.focusIntent = true
        if (selected.state.phase !== 'counting') return
        this.stopWaitClock(selected); selected.state.phase = 'focused'; this.publishWait(selected)
      },
      blur: () => {
        if (!current()) return
        selected.focusIntent = false
        if (selected.state.phase !== 'focused') return
        selected.state.phase = 'counting'
        selected.deadline = Date.now() + (selected.state.remainingMs ?? 0)
        this.runWaitClock(selected)
      },
      engage: () => {
        if (!current() || !['connecting', 'counting', 'focused', 'waiting'].includes(selected.state.phase)) return
        selected.engageIntent = true
        if (selected.state.phase === 'connecting') return
        this.stopWaitClock(selected); selected.state.phase = 'editing'; this.publishWait(selected)
      },
      takeTime: () => {
        if (!current() || !['connecting', 'counting', 'focused', 'editing', 'waiting'].includes(selected.state.phase)) return
        selected.holdIntent = true
        if (selected.state.phase === 'connecting') return
        this.stopWaitClock(selected); selected.state.phase = 'waiting'; selected.state.remainingMs = null; this.publishWait(selected)
      },
      retry: () => {
        if (!current() || !['error', 'ended'].includes(selected.state.phase)) return
        const epoch = ++selected.epoch
        selected.closing = true
        selected.state = { phase: 'connecting', remainingMs: null }; this.publishWait(selected)
        void (async () => {
          await rendererRuntimeClient.closeUserInputWait(selected.selector)
          if (!current() || selected.epoch !== epoch) return
          selected.closing = false
          const wait = await rendererRuntimeClient.openUserInputWait(selected.selector)
          if (current() && selected.epoch === epoch) this.acceptWait(selected, wait)
        })().catch(() => {
          if (current() && selected.epoch === epoch) { selected.closing = false; this.acceptWait(selected, { state: 'unavailable' }) }
        })
      },
      unsubscribe: () => {
        selected.listeners.delete(listener)
        if (!selected.listeners.size && ['ended', 'error'].includes(selected.state.phase)) {
          this.userInputWaits.delete(key)
          if (!this.userInputWaits.size) { this.waitOff?.(); this.waitOff = undefined }
        }
      }
    }
  }
  settleUserInputWait(selector: UserInputLocalSelectorV1): void {
    const key = JSON.stringify(selector), binding = this.userInputWaits.get(key)
    if (!binding) return
    this.userInputWaits.delete(key); binding.epoch++; this.stopWaitClock(binding)
    binding.state.phase = 'ended'; this.publishWait(binding)
    void rendererRuntimeClient.closeUserInputWait(selector).catch(() => undefined)
    if (!this.userInputWaits.size) { this.waitOff?.(); this.waitOff = undefined }
  }
  async continueUserInput(inputId: string, selector?: UserInputActionSelectorV1): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(analytixUserInputPath(inputId), 'POST', JSON.stringify({ action: 'continue', selector: userInputActionSelector(inputId, selector) }))
    const scheduled = UserInputWaitScheduledResponseSchemaV1.safeParse(response.ok ? readRuntimeJson<unknown>(response.body, 'invalid wait acknowledgement') : null)
    if (!response.ok || response.status !== 202 || !scheduled.success || scheduled.data.inputId !== inputId) throw new Error('Question continuation could not be scheduled.')
    // Scheduling is not a terminal outcome. Keep the claim until real settlement.
  }
  async submitUserInputLateReply(inputId: string, answers: UserInputSubmission[], lateReply: import('../../../../packages/runtime/src/contracts/approvals').UserInputLateReplyRequestV1, selector?: UserInputActionSelectorV1): Promise<import('../../../../packages/runtime/src/contracts/approvals').UserInputLateReplyDeliveryV1> {
    const response = await rendererRuntimeClient.runtimeRequest(analytixUserInputPath(inputId), 'POST', JSON.stringify({ answers, lateReply, selector: userInputActionSelector(inputId, selector) }))
    if (!response.ok) {
      const error = readRuntimeError(response.body, 'Late answer could not be scheduled.')
      const failure = runtimeErrorToError(error)
      const tooLarge = response.status === 409 && error.code === 'user_input_late_reply_too_large'
      // A validation/size rejection precedes admission. Transport uncertainty
      // and conflicts retain the attempt identity for idempotent reconciliation.
      throw Object.assign(failure, { userInputLateReplyUnadmitted: response.status === 400 || response.status === 413 || tooLarge, userInputLateReplyTooLarge: tooLarge })
    }
    const result = UserInputLateReplyResponseSchemaV1.safeParse(readRuntimeJson<unknown>(response.body, 'invalid late answer response'))
    if (response.status !== 200 || !result.success || result.data.inputId !== inputId ||
        result.data.lateReply.clientUserMessageId !== lateReply.clientUserMessageId) throw new Error('Runtime returned an unrelated late answer.')
    // expectedTurnId is a compare-and-set anchor. Idle admission may return a
    // new durable target; only the authenticated closed Host receipt owns it.
    return result.data.lateReply
  }
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
    localDocument?: boolean
  }): Promise<NormalizedThread> {
    const settings = await rendererRuntimeClient.getSettings()
    const runtime = getAnalytixRuntimeSettings(settings)
    // Local ownership uses the existing Core thread contract, without selecting
    // or checking a model. The internal purpose never crosses HTTP.
    const selection = input.localDocument ? {} : requestModelSelectionFromSettings({
      settings,
      explicitProviderId: input.providerId,
      model: input.model?.trim() || runtime.model,
      runtimeProviderId: runtime.providerId
    })
    const response = await rendererRuntimeClient.runtimeRequest(
      ANALYTIX_THREADS_PATH,
      'POST',
      JSON.stringify({
        workspace: input.workspace || settings.workspaceRoot || '~',
        title: input.title,
        autoTitle: input.autoTitle,
        ...selection,
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
    authoritativeTurnTimings?: AuthoritativeTurnTimings
  authoritativeTurnStatuses?: AuthoritativeTurnStatuses
    pendingApprovalIds?: string[]
    pendingUserInputIds?: string[]
    usage?: ThreadUsageSnapshot
    turnUsageById?: TurnUsageById
    turnUsageThroughSeq?: number
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
    const retainedFactTurns = new Set(turns.filter(turn => turn.factHistoryState === 'retained_snapshot').map(turn => turn.id))
    const caseBound = thread.historyAuthority === 'case_boundary_only_v1'
    const acceptedProjectionByTurn = acceptedFinalProjectionsForTurns(
      turns,
      thread.acceptedFinalDeliveries
    )
    if (isPublicProjectionRevoked(threadId)) {
      throw new Error('Public case projection authority was revoked.')
    }
    const itemsWithContext = turns.flatMap((turn) =>
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
          item,
          ownerTurnId: turn.id,
          rendererContext: {
            workspaceCheckpointId: item.workspaceCheckpointId ?? turn.workspaceCheckpointId,
            attachmentIds: turn.attachmentIds,
            activeSkillIds: turn.activeSkillIds,
            injectedMemoryIds: turn.injectedMemoryIds,
            skillInjectionBytes: turn.skillInjectionBytes
          }
        }))
    )
    const items = itemsWithContext.map(({ item }) => item)
    const timelineItems = itemsWithContext.filter(({ item }) => item.kind !== 'assistant_reasoning')
    const timingMaps = deriveThreadTimingMaps(turns)
    const timelineItemsByTurn = new Map<string, typeof timelineItems>()
    for (const entry of timelineItems) {
      const group = timelineItemsByTurn.get(entry.ownerTurnId) ?? []
      group.push(entry)
      timelineItemsByTurn.set(entry.ownerTurnId, group)
    }
    const rawBlocks = mergeChatBlocks(turns.flatMap(turn => [
      ...(timelineItemsByTurn.get(turn.id) ?? []).flatMap<ChatBlock>(({ item, rendererContext }) => {
        const projection = item.turnId ? acceptedProjectionByTurn.get(item.turnId) : undefined
        if (projection?.assistant.id === item.id) {
          return [{
            ...projection.assistant,
            meta: { ...projection.assistant.meta, ...(retainedFactTurns.has(item.turnId!) ? { factHistoryState: 'retained_snapshot' } : {}) },
            acceptedFinalProjectionReceipt: projection.receipt,
            acceptedFinalProjectionTerminal: projection.terminal
          }]
        }
        const block = chatBlockFromItem(item, undefined, rendererContext)
        return block ? [block] : []
      }),
      ...discardedReplyHistoryBlocks(threadId, turn)
    ]))
    const pendingApprovalIds = validStringArray(thread.pendingApprovalIds)
    const pendingUserInputIds = validStringArray(thread.pendingUserInputIds)
    const snapshotBlocks = applyPendingGateProjection(rawBlocks, pendingApprovalIds, pendingUserInputIds)
    const latestSeq = thread.latestSeq ?? 0
    const blocks = mergeGoalHistoryBlocks(snapshotBlocks,goalHistoryBlocksFromCore(thread))
    if (isPublicProjectionRevoked(threadId)) throw new Error('Public case projection authority was revoked.')
    const latestTurn = turns.at(-1)
    const latestUserMessageId = [...items].reverse().find((item) => item.kind === 'user_message')?.id
    return {
      thread: threadFromCore(thread),
      blocks,
      latestSeq,
      threadStatus: thread.status ?? latestTurn?.status,
      latestTurnId: latestTurn?.id,
      latestUserMessageId,
      ...timingMaps,
      authoritativeTurnTimings: authoritativeTurnTimings(turns),
      authoritativeTurnStatuses: authoritativeTurnStatuses(turns),
      pendingApprovalIds,
      pendingUserInputIds,
      usage: thread.usage ? usageFromCore(thread.usage) : undefined,
      turnUsageById: Object.fromEntries([...acceptedProjectionByTurn].map(([turnId, projection]) =>
        [turnId, turnUsageEntryFromBatch(projection)])),
      goal: thread.goal ? goalFromCore(thread.goal) : null,
      todos: thread.todos ? todosFromCore(thread.todos) : null,
      historyAuthority: thread.historyAuthority,
      latestTurnAcceptedFinalDigest:
        latestTurn && acceptedProjectionByTurn.has(latestTurn.id)
          ? acceptedProjectionByTurn.get(latestTurn.id)?.publicationCommitId
          : undefined
    }
  }

  /** UI-only projection. The generic getter remains the current primary for model/workflow consumers. */
  async restoreThreadDisplayHistory(
    threadId: string,
    detail: Awaited<ReturnType<AgentProvider['getThreadDetail']>>,
    signal?: AbortSignal
  ): Promise<Awaited<ReturnType<AgentProvider['getThreadDetail']>>> {
    if (signal?.aborted) throw new Error('Public history replay was cancelled.')
    if (detail.thread && detail.thread.id !== threadId) throw new Error('Runtime thread response identity mismatch.')
    if (isPublicProjectionRevoked(threadId)) {
      trajectorySource.revoke(threadId)
      throw new Error('Public case projection authority was revoked.')
    }
    const releaseTrajectory = trajectorySource.retainThread(threadId)
    trajectorySource.setHistoryReader(threadId, detail.historyAuthority === 'case_boundary_only_v1' ? undefined
      : (throughSeq, accept, replaySignal) => this.replayPublicHistory(threadId, throughSeq, accept, replaySignal, 'trajectory-window'), detail.latestSeq)
    try {
      if (detail.historyAuthority === 'case_boundary_only_v1' || detail.latestSeq <= 0) {
        trajectorySource.bindThread(threadId)
        return detail
      }
      const needsHistoricalBlocks = detail.blocks.some(block => block.kind === 'compaction')
      try {
        const restored = await this.restoreOrdinaryHistory(threadId, detail.latestSeq, detail.blocks, signal, needsHistoricalBlocks)
        if (isPublicProjectionRevoked(threadId)) throw new Error('Public case projection authority was revoked.')
        if (!signal?.aborted) trajectorySource.bindThread(threadId)
        return { ...detail, blocks: restored.blocks,
          turnUsageById: mergeTurnUsageById(threadId, restored.turnUsageById, detail.turnUsageById),
          turnUsageThroughSeq: detail.latestSeq }
      } catch (error) {
        // Receipt/usage replay is optional. Preserve the primary history when it is
        // unavailable, without publishing an estimate or reviving revoked state.
        if (needsHistoricalBlocks || signal?.aborted || isPublicProjectionRevoked(threadId)) throw error
        trajectorySource.bindThread(threadId)
        return detail
      }
    } finally { releaseTrajectory() }
  }

  /** A separate finite stream: no chat dispatch, live binding, or shared ACK cursor. */
  private async replayPublicHistory(threadId: string, throughSeq: number, accept: (events: unknown[]) => void, signal?: AbortSignal, purpose: 'ordinary' | 'trajectory-window' = 'ordinary'): Promise<void> {
    const streamId = createSseStreamId()
    await new Promise<void>((resolve, reject) => {
      let settled = false
      let chain = Promise.resolve()
      let cursor = 0
      const finish = (error?: unknown): void => {
        if (settled) return
        settled = true
        offData(); offEnd(); offError()
        signal?.removeEventListener('abort', onAbort)
        void rendererRuntimeClient.stopSse(streamId).catch(() => undefined)
        if (error) reject(error)
        else resolve()
      }
      const offData = rendererRuntimeClient.onSseEvent(payload => {
        if (payload.streamId !== streamId || settled) return
        if (!isStrictPublicRuntimeSseIpcPayload(payload) || purpose === 'trajectory-window' && payload.events.some(event => event.threadId !== threadId)) {
          finish(new Error('Invalid public history delivery.'))
          return
        }
        chain = chain.then(async () => {
          if (settled) return
          // This terminal control has no seq. Validate/bind it before the ordinary
          // replay parser, and never let optional replay fallback revive its data.
          if (payload.events.some(event => event.kind === 'public_projection_revoked')) {
            const revoked = payload.events.length === 1
              ? PublicProjectionRevokedEventSchema.safeParse(payload.events[0])
              : null
            if (!revoked?.success || revoked.data.threadId !== threadId) {
              throw new Error('Invalid public history delivery.')
            }
            markPublicProjectionRevoked(threadId)
            trajectorySource.revoke(threadId)
            throw new Error('Public case projection authority was revoked.')
          }
          let next = payload.events.reduce<number>((max, event) => Math.max(max, (event as CoreRuntimeEventJson).seq ?? 0), 0)
          if (purpose === 'trajectory-window') {
            if (isPublicProjectionRevoked(threadId)) throw new Error('Public case projection authority was revoked.')
            next = cursor
            for (const event of payload.events) {
              const seq = (event as CoreRuntimeEventJson).seq
              if (event.kind === 'accepted_final_batch' || event.kind === 'snapshot_required' ||
                  typeof seq !== 'number' || !Number.isSafeInteger(seq) || seq <= next || seq > throughSeq ||
                  'firstSeq' in event && typeof event.firstSeq === 'number' && event.firstSeq <= next) {
                throw new Error('Invalid public history delivery.')
              }
              next = seq
            }
          }
          accept(payload.events)
          if (purpose === 'trajectory-window' && (settled || signal?.aborted)) return
          if (next > 0 && !await rendererRuntimeClient.ackSseEvent(streamId, next)) {
            throw new Error('Public history acknowledgement was rejected.')
          }
          cursor = next
        }).catch(finish)
      })
      const offError = rendererRuntimeClient.onSseError(payload => {
        if (payload.streamId === streamId) finish(new Error('Public history replay is unavailable.'))
      })
      const offEnd = rendererRuntimeClient.onSseEnd(payload => {
        if (payload.streamId === streamId) void chain.then(() => finish(purpose !== 'trajectory-window' || cursor === throughSeq ? undefined : new Error('Public history replay is incomplete.')), finish)
      })
      const onAbort = (): void => finish(new Error('Public history replay was cancelled.'))
      signal?.addEventListener('abort', onAbort, { once: true })
      if (signal?.aborted) { onAbort(); return }
      void rendererRuntimeClient.startSse(threadId, 0, streamId, { replayThroughSeq: throughSeq }).then(() => {
        // Cancellation can win while Main still awaits settings, before it owns
        // a controller. Stop again when that late start finally creates one.
        if (settled || signal?.aborted) void rendererRuntimeClient.stopSse(streamId).catch(() => undefined)
      }).catch(finish)
    })
  }

  private async restoreOrdinaryHistory(threadId: string, throughSeq: number, snapshot: ChatBlock[], signal?: AbortSignal, includeHistoricalBlocks = true): Promise<{ blocks: ChatBlock[]; turnUsageById: TurnUsageById }> {
    const projection = createOrdinaryHistoryReplay(threadId, throughSeq)
    const trajectoryReplay = trajectorySource.beginReplay(threadId, throughSeq)
    try {
      await this.replayPublicHistory(threadId, throughSeq, events => {
        projection.accept(events)
        trajectoryReplay.accept(events)
      }, signal)
      const restored = { blocks: projection.mergeSnapshot(snapshot, { threadId, includeHistoricalBlocks }), turnUsageById: projection.turnUsage() }
      trajectoryReplay.complete()
      return restored
    } catch (error) {
      trajectoryReplay.fail()
      throw error
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
      requestedSkillIds?: string[]
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
      submissionIsCurrent?: () => boolean
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
    if (options?.requestedSkillIds?.length) body.requestedSkillIds = [...options.requestedSkillIds]
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
    // Settings can yield after the store's preflight. Fence the actual request too.
    if (options?.submissionIsCurrent && !options.submissionIsCurrent()) {
      throw new Error('Submission ownership expired')
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
      const error = readRuntimeError(response.body, 'failed to rewind thread')
      // HTTP status and a canonical error body do not prove that Core made
      // no durable change. Preserve the error without inventing that receipt.
      throw runtimeErrorToError(error)
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

  async promoteSideThread(threadId: string): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadPath(threadId),
      'PATCH',
      JSON.stringify({ promoteSide: true })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'promote Side thread failed'))
    }
  }

  async archiveThread(threadId: string, archived: boolean): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadPath(threadId),
      'PATCH',
      JSON.stringify({ archived })
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
    readRuntimeSchema(response.body, 'Invalid compaction response.', CompactResponse)
  }

  async getThreadGoal(threadId: string): Promise<NonNullable<NormalizedThread['goal']> | null> {
    return (await this.getThreadGoalState(threadId)).goal
  }

  async getThreadGoalState(threadId: string): Promise<import('./types').ThreadGoalStateV1> {
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
    return { goal: body.goal ? goalFromCore(body.goal) : null, activation: body.activation ?? null,
      historyBlocks: goalHistoryBlocksFromCore({goalHistory:body.history}) }
  }

  async setThreadGoal(
    threadId: string,
    patch: {
      operation?: 'create' | 'edit' | 'pause' | 'resume'
      ref?: import('../../../../packages/runtime/src/contracts/goal').GoalRefV1
      maxGoalRounds?: number
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

  async clearThreadGoal(threadId: string, ref?: import('../../../../packages/runtime/src/contracts/goal').GoalRefV1): Promise<boolean> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixThreadGoalPath(threadId),
      ref ? 'POST' : 'DELETE',
      ref ? JSON.stringify({operation:'clear',ref}) : undefined
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

  async submitUserInputResponse(inputId: string, answers: UserInputSubmission[], selector?: UserInputActionSelectorV1): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixUserInputPath(inputId),
      'POST',
      JSON.stringify({ answers, selector: userInputActionSelector(inputId, selector) })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'request_user_input response failed'))
    }
    const schema = answers.some(answer => 'kind' in answer)
      ? UserInputMetadataResolutionResponse : UserInputResolutionResponse
    const resolution = schema.safeParse(readRuntimeJson<unknown>(response.body, 'invalid user input response'))
    if (!resolution.success || resolution.data.inputId !== inputId || resolution.data.status !== 'submitted') {
      throw new Error('runtime returned an unrelated user input resolution')
    }
  }

  async cancelUserInput(inputId: string, selector?: UserInputActionSelectorV1): Promise<void> {
    const response = await rendererRuntimeClient.runtimeRequest(
      analytixUserInputPath(inputId),
      'POST',
      JSON.stringify({ cancelled: true, selector: userInputActionSelector(inputId, selector) })
    )
    if (!response.ok) {
      throw runtimeErrorToError(readRuntimeError(response.body, 'request_user_input cancel failed'))
    }
    const resolution = UserInputMetadataResolutionResponse.safeParse(
      readRuntimeJson<unknown>(response.body, 'invalid user input cancellation')
    )
    if (!resolution.success || resolution.data.inputId !== inputId || resolution.data.status !== 'cancelled') {
      throw new Error('runtime returned an unrelated user input cancellation')
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

  async getAttachmentMetadata(
    attachmentId: string,
    options: { threadId: string; workspace: string }
  ): Promise<CoreAttachmentMetadataJson> {
    const query = buildQuery({ thread_id: options.threadId, workspace: options.workspace })
    const response = await rendererRuntimeClient.runtimeRequest(`${analytixAttachmentPath(attachmentId)}${query}`, 'GET')
    if (!response.ok) throw runtimeErrorToError(readRuntimeError(response.body, 'failed to load attachment metadata'))
    const { attachment } = readRuntimeSchema(response.body, 'runtime returned invalid attachment metadata', AttachmentMetadataResponse)
    if (attachment.id !== attachmentId) throw new Error('Attachment identity mismatch')
    return attachment
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
    options?: { relation?: 'primary' | 'fork' | 'side'; title?: string; turnId?: string; endSeq?: number }
  ): Promise<NormalizedThread> {
    const body: Record<string, unknown> = {}
    if (options?.relation) body.relation = options.relation
    if (options?.title) body.title = options.title
    if (options?.turnId) body.turnId = options.turnId
    if (options?.endSeq !== undefined) body.endSeq = options.endSeq
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
    const body = readRuntimeSchema(
      response.body,
      'runtime returned an invalid resume session response',
      ResumeThreadResponseSchema
    )
    return { threadId: body.thread_id, sessionId: body.session_id }
  }

  async subscribeThreadEvents(
    threadId: string,
    sinceSeq: number,
    sink: ThreadEventSink,
    signal: AbortSignal
  ): Promise<void> {
    if (!signal.aborted) trajectorySource.bindThread(threadId)
    const releaseTrajectory = signal.aborted ? () => undefined : trajectorySource.retainThread(threadId)
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
      const reportSseRejection = (kind: 'event' | 'ack_false' | 'ack_transport'): void => {
        const lifecycle = signal.aborted ? 'aborted' : settled ? 'closed' : 'active'
        try {
          void window.analytix?.logs?.error(`sse-${kind}-${lifecycle}`, 'SSE renderer rejection')
            .catch(() => undefined)
        } catch { /* Diagnostics must not change dispatch, error or finish behavior. */ }
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
            reportSseRejection('event')
            emitSseTransportError(
              'Runtime request failed (sse_event_rejected).',
              'sse_event_rejected'
            )
            finish()
            return
          }
          const task = dispatchChain.then(async () => {
            try {
              trajectorySource.revoke(threadId)
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
          reportSseRejection('event')
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
              let ackTransportRejected = false
              const acknowledged = await rendererRuntimeClient.ackSseEvent(
                streamId,
                maxSeq,
                acceptedFinalAck
              ).catch(() => { ackTransportRejected = true; return false })
              if (!acknowledged) {
                reportSseRejection(ackTransportRejected ? 'ack_transport' : 'ack_false')
                emitSseTransportError(
                  'Runtime request failed (sse_ack_rejected).',
                  'sse_ack_rejected'
                )
                finish()
              } else {
                // This observer owns no dispatch/ACK authority and retains only public facts.
                trajectorySource.observe(threadId, batch)
              }
            }
          } catch {
            reportSseRejection('event')
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
    }).finally(releaseTrajectory)
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

/** Enrich a primary detail only at an explicit UI store call site; never cache the enlarged transcript. */
export function restoreThreadDisplayHistory(
  provider: AgentProvider,
  threadId: string,
  detail: Awaited<ReturnType<AgentProvider['getThreadDetail']>>,
  signal?: AbortSignal
): Promise<Awaited<ReturnType<AgentProvider['getThreadDetail']>>> {
  return provider instanceof AnalytixRuntimeProvider
    ? provider.restoreThreadDisplayHistory(threadId, detail, signal)
    : Promise.resolve(detail)
}
