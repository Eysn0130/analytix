import { z } from 'zod'
import {
  ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON,
  AcceptedFinalDigestSchema,
  AcceptedFinalTerminalReasonSchema,
  acceptedFinalDeliveryRequiresErrorItem,
  AcceptedFinalPublicAssistantTextTurnItemV3,
  PrivateAcceptedFinalAssistantTextTurnItem,
  OrdinaryResultSlotV1Schema,
  TurnItem
} from './items.js'
import {
  CheckpointMetadataSchema,
  CheckpointRewindApplyResultSchema,
  CheckpointRewindRescueRecordSchema
} from './checkpoints.js'
import { ThreadGoalSchema, ThreadTodoListSchema } from './threads.js'
import { UsageSnapshotSchema } from './usage.js'
import { RuntimeErrorSeverity } from './errors.js'
import { ApprovalPolicySchema, SandboxModeSchema } from './policy.js'
import { ModelReasoningEffort, SubagentToolPolicy } from './capabilities.js'
import { ModelExecutionRefSchema, ModelExecutionSourceSchema } from './model-execution-ref.js'

/**
 * Persisted runtime events. Every event has a per-thread `seq` so the
 * SSE stream can be replayed with `since_seq` after reconnects.
 */
export const RuntimeEventKind = z.enum([
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
])
export type RuntimeEventKind = z.infer<typeof RuntimeEventKind>

export const PipelineStage = z.enum([
  'setup',
  'pre_start',
  'post_start',
  'input_received',
  'input_cached',
  'input_routed',
  'input_compressed',
  'input_remembered',
  'pre_send',
  'post_send',
  'response_received',
  'provider_retrying',
  'provider_error',
  'empty_final_recovered',
  'subagent_running',
  'subagent_completed',
  'subagent_failed',
  'subagent_killed',
  'subagent_aborted',
  'subagent_interrupted'
])
export type PipelineStage = z.infer<typeof PipelineStage>

const RuntimeEventBase = z.object({
  seq: z.number().int().nonnegative(),
  timestamp: z.string(),
  threadId: z.string().min(1),
  turnId: z.string().optional(),
  itemId: z.string().optional(),
  child: z.object({
    parentThreadId: z.string().min(1),
    parentTurnId: z.string().min(1),
    parentToolCallId: z.string().min(1).optional(),
    childId: z.string().min(1),
    childRunId: z.string().min(1).optional(),
    childThreadId: z.string().min(1).optional(),
    childTurnId: z.string().min(1).optional(),
    childLabel: z.string().optional(),
    childName: z.string().optional(),
    childStatus: z.enum(['queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming', 'completed', 'failed', 'aborted', 'killed', 'interrupted']),
    childSeq: z.number().int().nonnegative().optional(),
    // Observability metrics carried alongside the child lifecycle event so
    // the GUI can show prefix reuse, tool fan-out, timing, and cost per
    // subagent without a separate diagnostics fetch.
    childModel: z.string().optional(),
    childProviderId: z.string().optional(),
    childEndpointFormat: z.string().optional(),
    childModelVariant: z.string().optional(),
    childModelSource: ModelExecutionSourceSchema.optional(),
    childModelExecution: ModelExecutionRefSchema.optional(),
    childEffort: ModelReasoningEffort.optional(),
    effort: ModelReasoningEffort.optional(),
    childProfile: z.string().optional(),
    childToolPolicy: SubagentToolPolicy.optional(),
    prefixReused: z.boolean().optional(),
    inheritedHistoryItems: z.number().int().nonnegative().optional(),
    toolInvocations: z.number().int().nonnegative().optional(),
    evidenceLedgered: z.boolean().optional(),
    durationMs: z.number().int().nonnegative().optional(),
    queuedMs: z.number().int().nonnegative().optional(),
    totalTokens: z.number().int().nonnegative().optional(),
    cachedTokens: z.number().int().nonnegative().optional(),
    cacheHitTokens: z.number().int().nonnegative().optional(),
    cacheMissTokens: z.number().int().nonnegative().optional(),
    cacheHitRate: z.number().min(0).max(1).nullable().optional(),
    cacheableTokenHitRate: z.number().min(0).max(1).nullable().optional(),
    totalInputTokenHitRate: z.number().min(0).max(1).nullable().optional(),
    cacheMissReasons: z.array(z.string()).optional(),
    cacheSuggestions: z.array(z.string()).optional(),
    cacheDiagnostics: z.record(z.string(), z.unknown()).optional(),
    costUsd: z.number().nonnegative().optional(),
    costCny: z.number().nonnegative().optional(),
    priceConfigured: z.boolean().optional(),
    cacheSavingsUsd: z.number().nonnegative().optional(),
    cacheSavingsCny: z.number().nonnegative().optional(),
    tokenEconomySavingsTokens: z.number().int().nonnegative().optional(),
    tokenEconomySavingsUsd: z.number().nonnegative().optional(),
    tokenEconomySavingsCny: z.number().nonnegative().optional(),
    background: z.boolean().optional(),
    parallelGroupId: z.string().min(1).optional(),
    parallelIndex: z.number().int().nonnegative().optional(),
    steerMessageId: z.string().min(1).optional(),
    steerStatus: z.string().min(1).optional(),
    pendingSteers: z.number().int().nonnegative().optional(),
    admittedSteers: z.number().int().nonnegative().optional(),
    steerCount: z.number().int().nonnegative().optional(),
    canAcceptSteer: z.boolean().optional(),
    lastSteerAt: z.string().optional(),
    lastSteerReason: z.string().optional(),
    pauseRequestId: z.string().min(1).optional(),
    pauseStatus: z.string().min(1).optional(),
    canPause: z.boolean().optional(),
    canResume: z.boolean().optional(),
    paused: z.boolean().optional(),
    pauseCount: z.number().int().nonnegative().optional(),
    pauseRequestedAt: z.string().optional(),
    lastPausedAt: z.string().optional(),
    lastResumedAt: z.string().optional(),
    lastPauseReason: z.string().optional(),
    resumeTokenIssuedAt: z.string().optional(),
    resumeTokenExpiresAt: z.string().optional()
  }).optional()
})

export const ItemEvent = RuntimeEventBase.extend({
  kind: z.enum([
    'item_created',
    'item_updated',
    'item_completed',
    'assistant_text_delta',
    'tool_call_started',
    'tool_call_finished'
  ]),
  item: TurnItem,
  acceptedFinalDigest: AcceptedFinalDigestSchema.optional(),
  publicationCommitId: AcceptedFinalDigestSchema.optional(),
  publicationEventId: AcceptedFinalDigestSchema.optional(),
  publicationSlot: z.string().min(1).optional(),
  publicationPayloadDigest: AcceptedFinalDigestSchema.optional()
}).superRefine((event, ctx) => {
  const publicationFields = [
    event.acceptedFinalDigest,
    event.publicationCommitId,
    event.publicationEventId,
    event.publicationSlot,
    event.publicationPayloadDigest
  ]
  const hasPublicationMetadata = publicationFields.some((value) => value !== undefined)
  if (hasPublicationMetadata) {
    ctx.addIssue({
      code: 'custom',
      path: ['acceptedFinalDigest'],
      message: 'accepted-final publication metadata is valid only inside a Batch V2 delivery group'
    })
  }
})
export type ItemEvent = z.infer<typeof ItemEvent>

/** Private/CAS audit event for verifying a full V5+V2 pair; never a public SSE contract. */
export const AcceptedFinalItemCompletedEventV1Schema = z.object({
  kind: z.literal('item_completed'),
  seq: z.number().int().nonnegative().safe(),
  timestamp: z.string().datetime({ offset: true }),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  itemId: z.string().min(1),
  item: PrivateAcceptedFinalAssistantTextTurnItem,
  acceptedFinalDigest: AcceptedFinalDigestSchema,
  publicationCommitId: AcceptedFinalDigestSchema,
  publicationEventId: AcceptedFinalDigestSchema,
  publicationSlot: z.literal('assistant-final'),
  publicationPayloadDigest: AcceptedFinalDigestSchema
}).strict().superRefine((event, ctx) => {
  const record = event.item.acceptedFinal
  if (!record || event.threadId !== record.threadId || event.turnId !== record.turnId ||
      event.itemId !== event.item.id || event.timestamp !== record.acceptedAt ||
      event.acceptedFinalDigest !== record.recordDigest || event.publicationCommitId !== record.recordDigest) {
    ctx.addIssue({ code: 'custom', path: ['item'], message: 'accepted-final public event identity is mismatched' })
  }
})
export type AcceptedFinalItemCompletedEventV1 = z.infer<typeof AcceptedFinalItemCompletedEventV1Schema>

/**
 * Closed V3 item-event shape used only inside AcceptedFinalDeliveryBatchV2.
 * Parsing this event alone does not grant generic display authority.
 */
export const AcceptedFinalItemCompletedEventV3Schema = z.object({
  kind: z.literal('item_completed'),
  seq: z.number().int().nonnegative().safe(),
  timestamp: z.string().datetime({ offset: true }),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  itemId: z.string().min(1),
  item: AcceptedFinalPublicAssistantTextTurnItemV3,
  acceptedFinalDigest: AcceptedFinalDigestSchema,
  publicationCommitId: AcceptedFinalDigestSchema,
  publicationEventId: AcceptedFinalDigestSchema,
  publicationSlot: z.literal('assistant-final'),
  publicationPayloadDigest: AcceptedFinalDigestSchema
}).strict().superRefine((event, ctx) => {
  const view = event.item.acceptedFinalView
  if (event.item.acceptedFinal !== undefined || !view || view.schemaVersion !== 3 ||
      event.threadId !== event.item.threadId || event.turnId !== event.item.turnId ||
      event.itemId !== event.item.id || event.timestamp !== view.acceptedAt ||
      event.acceptedFinalDigest !== view.acceptedFinalDigest ||
      event.publicationCommitId !== view.acceptedFinalDigest) {
    ctx.addIssue({ code: 'custom', path: ['item'], message: 'accepted-final generic public event identity is mismatched' })
  }
})
export type AcceptedFinalItemCompletedEventV3 = z.infer<typeof AcceptedFinalItemCompletedEventV3Schema>

const AcceptedFinalSseTraceV1Schema = z.object({
  sse_sent_at: z.number().finite().nonnegative(),
  sse_live_emitted_at: z.number().finite().nonnegative()
}).strict()

export const AcceptedFinalDeliverySealV1Schema = z.object({
  schemaVersion: z.literal('accepted-final-delivery-seal.v1'),
  purpose: z.literal('analytix.accepted-final-delivery-seal/v1'),
  sealId: AcceptedFinalDigestSchema,
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  publicationCommitId: AcceptedFinalDigestSchema,
  acceptedFinalDispositionDigest: AcceptedFinalDigestSchema,
  terminalDispositionId: AcceptedFinalDigestSchema,
  eventManifestDigest: AcceptedFinalDigestSchema,
  sequencedEventsDigest: AcceptedFinalDigestSchema,
  batchId: AcceptedFinalDigestSchema,
  firstSeq: z.number().int().positive().safe(),
  lastSeq: z.number().int().positive().safe(),
  timestamp: z.string().datetime({ offset: true }),
  authorityAlgorithm: z.literal('Ed25519'),
  authorityKeyId: AcceptedFinalDigestSchema,
  authorityPublicKey: z.string().regex(/^[A-Za-z0-9_-]{43}$/),
  authoritySignature: z.string().regex(/^[A-Za-z0-9_-]{86}$/)
}).strict()
export type AcceptedFinalDeliverySealV1 = z.infer<typeof AcceptedFinalDeliverySealV1Schema>

const AcceptedFinalPublicationMetadataV1 = {
  acceptedFinalDigest: AcceptedFinalDigestSchema,
  publicationCommitId: AcceptedFinalDigestSchema,
  publicationEventId: AcceptedFinalDigestSchema,
  publicationSlot: z.string().min(1),
  publicationPayloadDigest: AcceptedFinalDigestSchema
}

const AcceptedFinalCacheDiagnosticsV1Schema = z.lazy(
  () => GeneralTerminalCacheDiagnosticsV1Schema
)

const AcceptedFinalUsageEventV1Schema = z.object({
  kind: z.literal('usage'),
  seq: z.number().int().positive().safe(),
  timestamp: z.string().datetime({ offset: true }),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  model: z.string(),
  providerId: z.string().optional(),
  endpointFormat: z.string().optional(),
  effort: ModelReasoningEffort.optional(),
  usageSource: z.string().optional(),
  childRunId: z.string().optional(),
  usage: UsageSnapshotSchema.strict(),
  cacheDiagnostics: AcceptedFinalCacheDiagnosticsV1Schema,
  usageFinalStatus: z.enum(['completed', 'failed', 'aborted']),
  ...AcceptedFinalPublicationMetadataV1,
  publicationSlot: z.literal('usage')
}).strict()

export const AcceptedFinalTerminalErrorItemV1Schema = z.object({
  id: z.string().min(1),
  turnId: z.string().min(1),
  threadId: z.string().min(1),
  role: z.literal('system'),
  status: z.enum(['completed', 'failed', 'aborted']),
  createdAt: z.string().datetime({ offset: true }),
  finishedAt: z.string().datetime({ offset: true }),
  kind: z.literal('error'),
  code: z.string().min(1),
  message: z.string().min(1),
  severity: RuntimeErrorSeverity,
  acceptedFinalDigest: AcceptedFinalDigestSchema
}).strict().superRefine((item, ctx) => {
  const prefix = 'case_terminal_'
  const parsedReason = item.code.startsWith(prefix)
    ? AcceptedFinalTerminalReasonSchema.safeParse(item.code.slice(prefix.length))
    : null
  if (!parsedReason?.success || !acceptedFinalDeliveryRequiresErrorItem(parsedReason.data)) {
    ctx.addIssue({ code: 'custom', path: ['code'], message: 'accepted-final terminal error code is invalid' })
    return
  }
  const expectedStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[parsedReason.data]
  const expectedMessage = expectedStatus === 'completed'
    ? '案件分析已结束；仅发布通过宿主证据门的固定边界答复。'
    : expectedStatus === 'aborted'
      ? '案件分析已终止；未经核验的案件事实未发布。'
      : '案件分析未完成；未经核验的案件事实未发布。'
  const expectedSeverity = expectedStatus === 'failed' ? 'error' : 'warning'
  if (item.status !== expectedStatus || item.message !== expectedMessage ||
      item.severity !== expectedSeverity) {
    ctx.addIssue({
      code: 'custom',
      path: ['message'],
      message: 'accepted-final terminal error projection is not host canonical'
    })
  }
})

const AcceptedFinalTerminalErrorEventV1Schema = z.object({
  kind: z.literal('item_completed'),
  seq: z.number().int().positive().safe(),
  timestamp: z.string().datetime({ offset: true }),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  itemId: z.string().min(1),
  item: AcceptedFinalTerminalErrorItemV1Schema,
  ...AcceptedFinalPublicationMetadataV1,
  publicationSlot: z.literal('terminal-error-item')
}).strict()

const AcceptedFinalTerminalEventV1Schema = z.object({
  kind: z.enum(['turn_completed', 'turn_failed', 'turn_aborted']),
  seq: z.number().int().positive().safe(),
  timestamp: z.string().datetime({ offset: true }),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  status: z.enum(['completed', 'failed', 'aborted']),
  terminalReason: AcceptedFinalTerminalReasonSchema,
  error: z.string().optional(),
  message: z.string().optional(),
  code: z.string().optional(),
  itemId: z.string().optional(),
  discard: z.boolean().optional(),
  cancelled: z.boolean().optional(),
  cancelledPendingGates: z.number().int().nonnegative().safe().refine(
    (value) => !Object.is(value, -0),
    { message: 'accepted-final cancelled gate count is not canonical' }
  ).optional(),
  ...AcceptedFinalPublicationMetadataV1,
  publicationSlot: z.literal('terminal')
}).strict()

const AcceptedFinalDeliveryEventsV1Schema = z.union([
  z.tuple([
    AcceptedFinalItemCompletedEventV1Schema,
    AcceptedFinalUsageEventV1Schema,
    AcceptedFinalTerminalEventV1Schema
  ]),
  z.tuple([
    AcceptedFinalItemCompletedEventV1Schema,
    AcceptedFinalTerminalErrorEventV1Schema,
    AcceptedFinalUsageEventV1Schema,
    AcceptedFinalTerminalEventV1Schema
  ])
])

const AcceptedFinalDeliveryEventsV2Schema = z.union([
  z.tuple([
    AcceptedFinalItemCompletedEventV3Schema,
    AcceptedFinalUsageEventV1Schema,
    AcceptedFinalTerminalEventV1Schema
  ]),
  z.tuple([
    AcceptedFinalItemCompletedEventV3Schema,
    AcceptedFinalTerminalErrorEventV1Schema,
    AcceptedFinalUsageEventV1Schema,
    AcceptedFinalTerminalEventV1Schema
  ])
])

const AcceptedFinalDeliveryBatchV1BaseSchema = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal('analytix.accepted-final-delivery-batch/v1'),
  kind: z.literal('accepted_final_batch'),
  batchId: AcceptedFinalDigestSchema,
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  seq: z.number().int().positive().safe(),
  firstSeq: z.number().int().positive().safe(),
  lastSeq: z.number().int().positive().safe(),
  timestamp: z.string().datetime({ offset: true }),
  publicationCommitId: AcceptedFinalDigestSchema,
  eventManifestDigest: AcceptedFinalDigestSchema,
  publicationAuthority: AcceptedFinalDeliverySealV1Schema,
  events: AcceptedFinalDeliveryEventsV1Schema,
  trace: AcceptedFinalSseTraceV1Schema.optional()
}).strict()

const AcceptedFinalDeliveryBatchV2BaseSchema = z.object({
  schemaVersion: z.literal(2),
  purpose: z.literal('analytix.accepted-final-delivery-batch/v2'),
  kind: z.literal('accepted_final_batch'),
  batchId: AcceptedFinalDigestSchema,
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  seq: z.number().int().positive().safe(),
  firstSeq: z.number().int().positive().safe(),
  lastSeq: z.number().int().positive().safe(),
  timestamp: z.string().datetime({ offset: true }),
  publicationCommitId: AcceptedFinalDigestSchema,
  eventManifestDigest: AcceptedFinalDigestSchema,
  publicationAuthority: AcceptedFinalDeliverySealV1Schema,
  events: AcceptedFinalDeliveryEventsV2Schema,
  trace: AcceptedFinalSseTraceV1Schema.optional()
}).strict()

type AcceptedFinalDeliveryBatchWire =
  z.infer<typeof AcceptedFinalDeliveryBatchV1BaseSchema> |
  z.infer<typeof AcceptedFinalDeliveryBatchV2BaseSchema>

function refineAcceptedFinalDeliveryBatch(
  batch: AcceptedFinalDeliveryBatchWire,
  ctx: z.RefinementCtx
): void {
  if (batch.seq !== batch.lastSeq || batch.lastSeq - batch.firstSeq + 1 !== batch.events.length) {
    ctx.addIssue({ code: 'custom', path: ['seq'], message: 'accepted-final delivery range is not contiguous' })
  }
  batch.events.forEach((event, index) => {
    if (event.threadId !== batch.threadId || event.turnId !== batch.turnId ||
        event.seq !== batch.firstSeq + index || event.acceptedFinalDigest !== batch.publicationCommitId ||
        event.publicationCommitId !== batch.publicationCommitId || event.timestamp !== batch.timestamp) {
      ctx.addIssue({ code: 'custom', path: ['events', index], message: 'accepted-final delivery event binding is mismatched' })
    }
  })
  const assistant = batch.events[0]
  const usage = batch.events[batch.events.length - 2] as z.infer<typeof AcceptedFinalUsageEventV1Schema>
  const terminal = batch.events[batch.events.length - 1] as z.infer<typeof AcceptedFinalTerminalEventV1Schema>
  const terminalReason = terminal.terminalReason
  const expectedStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[terminalReason]
  const requiresErrorItem = acceptedFinalDeliveryRequiresErrorItem(terminalReason)
  const assistantTerminalReason = batch.schemaVersion === 1
    ? assistant.item.acceptedFinal?.terminalReason
    : assistant.item.acceptedFinalView?.terminalReason
  if (terminal.terminalReason !== assistantTerminalReason ||
      terminal.kind !== `turn_${expectedStatus}` || terminal.status !== expectedStatus ||
      usage.usageFinalStatus !== expectedStatus || (batch.events.length === 4) !== requiresErrorItem) {
    ctx.addIssue({
      code: 'custom',
      path: ['events'],
      message: 'accepted-final delivery terminal profile is mismatched'
    })
  }
  if (expectedStatus === 'aborted') {
    if (terminal.discard === undefined || terminal.cancelled === undefined ||
        terminal.cancelledPendingGates === undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['events', batch.events.length - 1, 'discard'],
        message: 'accepted-final aborted terminal disposition is incomplete'
      })
    } else if (terminal.cancelledPendingGates > 0 && terminal.cancelled === false) {
      ctx.addIssue({
        code: 'custom',
        path: ['events', batch.events.length - 1, 'cancelled'],
        message: 'accepted-final pending gate cancellation is inconsistent'
      })
    }
  } else if (terminal.discard !== undefined || terminal.cancelled !== undefined ||
      terminal.cancelledPendingGates !== undefined) {
    ctx.addIssue({
      code: 'custom',
      path: ['events', batch.events.length - 1, 'discard'],
      message: 'non-aborted accepted-final terminal carries abort disposition'
    })
  }
  if (expectedStatus !== 'failed' && (terminal.error !== undefined || terminal.message !== undefined)) {
    ctx.addIssue({
      code: 'custom',
      path: ['events', batch.events.length - 1, 'error'],
      message: 'non-failed accepted-final terminal carries failure text'
    })
  }
  if (batch.events.length === 3) {
    if (terminal.itemId !== undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['events', 2, 'itemId'],
        message: 'three-slot accepted-final terminal carries an error item identity'
      })
    }
  } else {
    const errorEvent = batch.events[1] as z.infer<typeof AcceptedFinalTerminalErrorEventV1Schema>
    const errorItem = errorEvent.item
    if (errorEvent.itemId !== errorItem.id || terminal.itemId !== errorItem.id ||
        errorItem.threadId !== batch.threadId || errorItem.turnId !== batch.turnId ||
        errorItem.status !== expectedStatus || errorItem.acceptedFinalDigest !== batch.publicationCommitId ||
        errorItem.createdAt !== batch.timestamp || errorItem.finishedAt !== batch.timestamp ||
        terminal.code === undefined || terminal.code !== errorItem.code ||
        errorItem.code !== `case_terminal_${terminalReason}`) {
      ctx.addIssue({
        code: 'custom',
        path: ['events', 1, 'item'],
        message: 'accepted-final terminal error item binding is mismatched'
      })
    }
    if (expectedStatus === 'failed') {
      if (terminal.error !== errorItem.message || terminal.message !== errorItem.message ||
          errorItem.severity !== 'error') {
        ctx.addIssue({
          code: 'custom',
          path: ['events', 1, 'item', 'message'],
          message: 'accepted-final failed terminal is detached from its error item'
        })
      }
    } else if (errorItem.severity !== 'warning') {
      ctx.addIssue({
        code: 'custom',
        path: ['events', 1, 'item', 'severity'],
        message: 'accepted-final boundary error item severity is invalid'
      })
    }
  }
  const authority = batch.publicationAuthority
  if (authority.threadId !== batch.threadId || authority.turnId !== batch.turnId ||
      authority.publicationCommitId !== batch.publicationCommitId || authority.batchId !== batch.batchId ||
      authority.eventManifestDigest !== batch.eventManifestDigest || authority.firstSeq !== batch.firstSeq ||
      authority.lastSeq !== batch.lastSeq || authority.timestamp !== batch.timestamp) {
    ctx.addIssue({ code: 'custom', path: ['publicationAuthority'], message: 'accepted-final delivery seal is detached' })
  }
}

/** Frozen private/audit wire. It is never a current generic HTTP/SSE batch. */
export const AcceptedFinalDeliveryBatchV1Schema =
  AcceptedFinalDeliveryBatchV1BaseSchema.superRefine(refineAcceptedFinalDeliveryBatch)
export type AcceptedFinalDeliveryBatchV1 = z.infer<typeof AcceptedFinalDeliveryBatchV1Schema>

export const ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1 = 256

/** Current closed generic delivery wire for AcceptedFinalPublicViewV3 events. */
export const AcceptedFinalDeliveryBatchV2Schema =
  AcceptedFinalDeliveryBatchV2BaseSchema.superRefine(refineAcceptedFinalDeliveryBatch)
export type AcceptedFinalDeliveryBatchV2 = z.infer<typeof AcceptedFinalDeliveryBatchV2Schema>

const GeneralTerminalDigestV1Schema = z.string().regex(/^[a-f0-9]{64}$/)
const GeneralTerminalPublicIDV1Schema = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/)

function isGeneralTerminalBoundarySpaceV1(codePoint: number): boolean {
  return (codePoint >= 0x09 && codePoint <= 0x0d) || codePoint === 0x20 ||
    codePoint === 0x85 || codePoint === 0xa0 || codePoint === 0x1680 ||
    (codePoint >= 0x2000 && codePoint <= 0x200a) || codePoint === 0x2028 ||
    codePoint === 0x2029 || codePoint === 0x202f || codePoint === 0x205f ||
    codePoint === 0x3000 || codePoint === 0xfeff
}

function isGeneralTerminalExactTextV1(value: string): boolean {
  if (value.length === 0) return false
  const codePoints = Array.from(value, (entry) => entry.codePointAt(0) as number)
  return !isGeneralTerminalBoundarySpaceV1(codePoints[0]) &&
    !isGeneralTerminalBoundarySpaceV1(codePoints[codePoints.length - 1])
}

const GeneralTerminalExactTextV1Schema = z.string().min(1).refine(
  isGeneralTerminalExactTextV1,
  { message: 'general terminal text is not canonical' }
)

const GeneralTerminalTimestampV1Schema = z.string().datetime({ offset: true }).refine(
  (value) => /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value),
  { message: 'general terminal timestamp exceeds RFC3339 nanosecond precision' }
)

function generalTerminalTimestampNanosV1(value: string): bigint | null {
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value)
  if (!match) return null
  const millis = Date.parse(`${match[1]}${match[3]}`)
  if (!Number.isSafeInteger(millis) || millis % 1_000 !== 0) return null
  const fraction = (match[2] ?? '').padEnd(9, '0')
  return BigInt(millis) * 1_000_000n + BigInt(fraction || '0')
}

const GeneralTerminalErrorDetailsV1Schema = z.object({
  status: generalTerminalCanonicalUnsignedV1Schema(599).optional(),
  retryAfterMs: generalTerminalCanonicalUnsignedV1Schema(900_000).optional(),
  attempt: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  maxAttempt: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  recoveryAttempt: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  maxRecoveryAttempt: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  maxRecoveryAttempts: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  maxModelSteps: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  stormCount: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  loopStep: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  advertisedToolCount: generalTerminalCanonicalUnsignedV1Schema(1_000_000).optional(),
  rejectedToolNormalizedNameSha256: GeneralTerminalDigestV1Schema.optional(),
  advertisedToolManifestHash: GeneralTerminalDigestV1Schema.optional(),
  advertisedNameSetSortedHash: GeneralTerminalDigestV1Schema.optional(),
  providerRequestToolManifestHash: GeneralTerminalDigestV1Schema.optional(),
  runToolStepManifestHash: GeneralTerminalDigestV1Schema.optional(),
  rejectedToolCategory: z.enum([
    'known_builtin_not_advertised', 'known_mcp_not_advertised',
    'known_alias_not_advertised', 'unknown_provider_name'
  ]).optional(),
  promptRoute: z.enum(['direct_answer', 'light_agent', 'tool_agent', 'subagent_agent']).optional(),
  endpointFormat: z.enum(['chat_completions', 'responses', 'messages', 'custom_endpoint']).optional(),
  kind: z.enum([
    'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
    'invalid_model', 'unknown'
  ]).optional(),
  authStatus: z.enum(['none', 'required']).optional(),
  hasApiKey: z.boolean().optional(),
  retryable: z.boolean().optional(),
  executed: z.boolean().optional(),
  partialToolStarted: z.boolean().optional(),
  visibleRecovery: z.boolean().optional(),
  recoveryExhausted: z.boolean().optional(),
  providerRequestRunToolStepManifestSame: z.boolean().optional()
}).strict().refine((details) => Object.keys(details).length > 0, {
  message: 'empty general terminal error details are not canonical'
})

const TOOL_NOT_ADVERTISED_DIAGNOSTIC_KEYS_V1 = [
  'rejectedToolNormalizedNameSha256', 'rejectedToolCategory', 'promptRoute', 'loopStep',
  'advertisedToolCount', 'advertisedToolManifestHash', 'advertisedNameSetSortedHash',
  'providerRequestToolManifestHash', 'runToolStepManifestHash',
  'providerRequestRunToolStepManifestSame'
] as const

export function isToolNotAdvertisedDiagnosticDetailsV1(details: unknown): boolean {
  const parsed = GeneralTerminalErrorDetailsV1Schema.safeParse(details)
  if (!parsed.success || Object.keys(parsed.data).length !== TOOL_NOT_ADVERTISED_DIAGNOSTIC_KEYS_V1.length ||
      !TOOL_NOT_ADVERTISED_DIAGNOSTIC_KEYS_V1.every((key) => parsed.data[key] !== undefined)) return false
  return parsed.data.providerRequestRunToolStepManifestSame ===
    (parsed.data.providerRequestToolManifestHash === parsed.data.runToolStepManifestHash)
}

const GENERAL_TERMINAL_FAILURE_CODES_V1 = new Set([
  'turn_failed', 'turn_cancelled', 'provider_error', 'provider_authentication_failed',
  'provider_rate_limited', 'provider_insufficient_balance', 'provider_endpoint_not_found',
  'provider_request_rejected', 'provider_unavailable', 'provider_network_unavailable',
  'provider_timeout', 'provider_stream_interrupted', 'provider_model_invalid',
  'provider_not_configured', 'provider_tool_arguments_invalid',
  'provider_reasoning_markup_invalid', 'provider_empty_final',
  'attachment_text_fallback_too_large', 'publication_receipt_required', 'validation_error',
  'runtime_restarted', 'turn_recovery_boundary', 'approval_denied', 'input_cancelled',
  'turn_step_limit_exceeded', 'turn_security_authority_unavailable',
  'turn_security_context_invalid', 'turn_security_workspace_mismatch',
  'turn_security_case_binding_mismatch', 'turn_security_dataset_snapshot_mismatch',
  'turn_security_risk_policy_mismatch', 'tool_call_identity_invalid', 'tool_not_advertised',
  'tool_schema_missing', 'tool_schema_invalid', 'tool_private_arguments',
  'tool_source_unavailable', 'tool_invalid_arguments_storm', 'tool_failure_storm',
  'execution_grant_rejected', 'source_probe_unavailable',
  'tool_pending_continuation_invalid', 'provider_stream_failed', 'context_window_hard_limit'
])

const GENERAL_TERMINAL_FAILURE_MESSAGES_V1: Readonly<Record<string, string>> = {
  turn_cancelled: 'The turn was cancelled before a verified response was available.',
  provider_authentication_failed: 'Provider authentication failed. Check the configured credential.',
  provider_rate_limited: 'The provider rate limit was reached. Retry after the bounded delay.',
  provider_insufficient_balance: 'The provider account balance or credit is insufficient.',
  provider_endpoint_not_found: 'The model endpoint was not found. Check Provider Base URL and Endpoint format.',
  provider_request_rejected: 'The provider rejected the model request. Response content was withheld.',
  provider_unavailable: 'The model provider is temporarily unavailable.',
  provider_network_unavailable: 'The model provider could not be reached.',
  provider_timeout: 'The provider request timed out before a verified response was available.',
  provider_stream_interrupted: 'The provider stream was interrupted before a verified response was available.',
  provider_model_invalid: 'The selected model is not configured for this provider.',
  provider_not_configured: 'The selected provider is not configured for this runtime.',
  provider_tool_arguments_invalid: 'The provider supplied incomplete or invalid tool arguments; execution was blocked.',
  provider_reasoning_markup_invalid: 'The provider returned invalid private-reasoning markup; the response was blocked.',
  provider_empty_final: 'The provider returned no final response after the bounded recovery attempt.',
  attachment_text_fallback_too_large: 'The attachment cannot be represented within the bounded text fallback.',
  runtime_restarted: 'The runtime restarted before this turn completed. The turn was marked aborted so the thread can continue.',
  turn_recovery_boundary: 'The bounded recovery path ended without a verified final response.',
  approval_denied: 'The requested operation was denied; no unverified final response was published.',
  input_cancelled: 'The requested input was cancelled; no unverified final response was published.',
  turn_step_limit_exceeded: 'The turn reached its configured model-step limit.',
  tool_not_advertised: 'The provider requested a tool that was not advertised for this turn.',
  tool_call_identity_invalid: 'The provider supplied an invalid or duplicate tool-call identity.',
  tool_private_arguments: 'The provider supplied private reasoning in tool arguments; execution was blocked.',
  tool_source_unavailable: 'The required source is not available for the current run.',
  source_probe_unavailable: 'The required source is not available for the current run.',
  publication_receipt_required: 'Formal artifact publication requires current host publication authority.',
  tool_invalid_arguments_storm: 'Repeated invalid tool arguments caused the host to stop the turn.',
  tool_failure_storm: 'Repeated tool failures caused the host to stop the turn.',
  validation_error: 'The request did not satisfy the current host contract.',
  context_window_hard_limit: 'The bounded request still exceeds the model context window after safe compaction; no provider request was sent.'
}

const GENERAL_TERMINAL_WARNING_CODES_V1 = new Set([
  'turn_cancelled', 'runtime_restarted', 'turn_recovery_boundary', 'approval_denied',
  'input_cancelled', 'source_probe_unavailable'
])

export function generalTerminalFailureMessageV1(code: string): string | null {
  if (!GENERAL_TERMINAL_FAILURE_CODES_V1.has(code)) return null
  const explicit = GENERAL_TERMINAL_FAILURE_MESSAGES_V1[code]
  if (explicit) return explicit
  if (code.startsWith('turn_security_') || code.startsWith('execution_grant_')) {
    return 'Current host security authority rejected the operation.'
  }
  if (code.startsWith('tool_schema_')) {
    return 'The tool schema is unavailable or invalid for the current turn.'
  }
  if (code === 'tool_pending_continuation_invalid') {
    return 'Pending tool continuation failed current host revalidation.'
  }
  if (code.startsWith('tool_')) return 'The host rejected or could not complete the tool operation.'
  if (code.startsWith('approval_')) return 'The operation did not receive current host approval.'
  if (code.startsWith('source_')) return 'The required source is unavailable or no longer current.'
  return 'The turn failed before a verified response was available.'
}

export function generalTerminalFailureSeverityV1(code: string): 'warning' | 'error' | null {
  if (!GENERAL_TERMINAL_FAILURE_CODES_V1.has(code)) return null
  return GENERAL_TERMINAL_WARNING_CODES_V1.has(code) ? 'warning' : 'error'
}

function isGeneralTerminalFailureProjectionV1(record: {
  code?: string
  message?: string
  error?: string
  severity?: string
}): boolean {
  if (record.code === undefined || record.message === undefined) return false
  const message = generalTerminalFailureMessageV1(record.code)
  const severity = generalTerminalFailureSeverityV1(record.code)
  return message !== null && severity !== null && record.message === message &&
    (record.error === undefined || record.error === message) &&
    (record.severity === undefined || record.severity === severity)
}

export const GENERAL_PROVIDER_FINAL_QUARANTINED_TEXT =
  '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。' as const

const GeneralTerminalAssistantTextItemBaseV1Schema = z.object({
  id: GeneralTerminalPublicIDV1Schema,
  turnId: GeneralTerminalPublicIDV1Schema,
  threadId: GeneralTerminalPublicIDV1Schema,
  role: z.literal('assistant'),
  status: z.literal('completed'),
  createdAt: GeneralTerminalTimestampV1Schema,
  finishedAt: GeneralTerminalTimestampV1Schema,
  kind: z.literal('assistant_text')
})

const LegacyGeneralTerminalAssistantTextItemV1Schema = GeneralTerminalAssistantTextItemBaseV1Schema.extend({
  text: z.literal(GENERAL_PROVIDER_FINAL_QUARANTINED_TEXT)
}).strict()

const TypedGeneralTerminalAssistantTextItemV1Schema = GeneralTerminalAssistantTextItemBaseV1Schema.extend({
  text: GeneralTerminalExactTextV1Schema,
  ordinaryResult: OrdinaryResultSlotV1Schema
}).strict().superRefine((item, ctx) => {
  if (item.text !== item.ordinaryResult.text) {
    ctx.addIssue({ code: 'custom', path: ['ordinaryResult', 'text'], message: 'ordinary result text is mismatched' })
  }
})

const GeneralTerminalAssistantTextItemV1Schema = z.union([
  LegacyGeneralTerminalAssistantTextItemV1Schema,
  TypedGeneralTerminalAssistantTextItemV1Schema
])

export const GeneralTerminalErrorItemV1Schema = z.object({
  id: GeneralTerminalPublicIDV1Schema,
  turnId: GeneralTerminalPublicIDV1Schema,
  threadId: GeneralTerminalPublicIDV1Schema,
  role: z.literal('system'),
  status: z.enum(['failed', 'aborted']),
  createdAt: GeneralTerminalTimestampV1Schema,
  finishedAt: GeneralTerminalTimestampV1Schema,
  kind: z.literal('error'),
  code: GeneralTerminalExactTextV1Schema,
  message: GeneralTerminalExactTextV1Schema,
  severity: RuntimeErrorSeverity,
  details: GeneralTerminalErrorDetailsV1Schema.optional()
}).strict().superRefine((item, ctx) => {
  if (!isGeneralTerminalFailureProjectionV1(item)) {
    ctx.addIssue({ code: 'custom', path: ['code'], message: 'general terminal failure projection is not host canonical' })
  }
  if (item.details !== undefined &&
      (item.code !== 'tool_not_advertised' || !isToolNotAdvertisedDiagnosticDetailsV1(item.details))) {
    ctx.addIssue({ code: 'custom', path: ['details'], message: 'general terminal failure diagnostics are not code-bound' })
  }
})

const GeneralTerminalItemCompletedEventV1Schema = z.object({
  kind: z.literal('item_completed'),
  seq: z.number().int().positive().safe(),
  timestamp: GeneralTerminalTimestampV1Schema,
  threadId: GeneralTerminalPublicIDV1Schema,
  turnId: GeneralTerminalPublicIDV1Schema,
  itemId: GeneralTerminalPublicIDV1Schema,
  item: z.union([
    GeneralTerminalAssistantTextItemV1Schema,
    GeneralTerminalErrorItemV1Schema
  ])
}).strict().superRefine((event, ctx) => {
  if (event.itemId !== event.item.id || event.threadId !== event.item.threadId ||
      event.turnId !== event.item.turnId || event.timestamp !== event.item.finishedAt) {
    ctx.addIssue({
      code: 'custom',
      path: ['item'],
      message: 'general terminal item identity is mismatched'
    })
  }
  const createdAt = generalTerminalTimestampNanosV1(event.item.createdAt)
  const finishedAt = generalTerminalTimestampNanosV1(event.item.finishedAt)
  if (createdAt === null || finishedAt === null || createdAt > finishedAt) {
    ctx.addIssue({
      code: 'custom',
      path: ['item', 'createdAt'],
      message: 'general terminal item timestamps are out of order'
    })
  }
})

function generalTerminalCanonicalUnsignedV1Schema(maximum = Number.MAX_SAFE_INTEGER) {
  return z.number().int().nonnegative().safe().max(maximum).refine(
    (value) => !Object.is(value, -0),
    { message: 'general terminal unsigned integer is not canonical' }
  )
}

const GeneralTerminalCanonicalRateV1Schema = z.number().min(0).max(1).refine(
  (value) => !Object.is(value, -0),
  { message: 'general terminal rate is not canonical' }
)

const GeneralTerminalTokenAggregateV1Schema = z.object({
  complete: z.boolean(),
  knownObservationCount: generalTerminalCanonicalUnsignedV1Schema(),
  observationCount: generalTerminalCanonicalUnsignedV1Schema(),
  value: generalTerminalCanonicalUnsignedV1Schema().optional()
}).strict().superRefine((aggregate, ctx) => {
  if (aggregate.knownObservationCount > aggregate.observationCount ||
      aggregate.complete !== (aggregate.value !== undefined) ||
      (aggregate.complete && aggregate.knownObservationCount !== aggregate.observationCount)) {
    ctx.addIssue({ code: 'custom', path: ['complete'], message: 'terminal token aggregate is inconsistent' })
  }
})

const GeneralTerminalAttemptStatusesV1Schema = z.object({
  succeeded: generalTerminalCanonicalUnsignedV1Schema(),
  failed: generalTerminalCanonicalUnsignedV1Schema(),
  cancelled: generalTerminalCanonicalUnsignedV1Schema(),
  timedOut: generalTerminalCanonicalUnsignedV1Schema(),
  streamAborted: generalTerminalCanonicalUnsignedV1Schema()
}).strict()

const GeneralTerminalAttemptRateV1Schema = z.object({
  known: z.boolean(),
  numerator: generalTerminalCanonicalUnsignedV1Schema(),
  denominator: generalTerminalCanonicalUnsignedV1Schema()
}).strict().superRefine((rate, ctx) => {
  if (rate.numerator > rate.denominator ||
      (rate.known ? rate.denominator === 0 : rate.numerator !== 0 || rate.denominator !== 0)) {
    ctx.addIssue({ code: 'custom', path: ['known'], message: 'terminal attempt cache rate is inconsistent' })
  }
})

const GeneralTerminalContextEpochImpactV1Schema = z.object({
  stablePrefix: z.boolean(),
  dynamicContext: z.boolean(),
  turnTail: z.boolean(),
  diagnosticsOnly: z.boolean()
}).strict()

const GENERAL_TERMINAL_PREFIX_REASONS = new Set([
  'system', 'tools', 'prefix', 'provider', 'provider_id', 'endpoint_format', 'model'
])
const GENERAL_TERMINAL_TOOL_SOURCE_REASONS = new Set(['tools', 'tool_sources'])
const GENERAL_TERMINAL_CONTEXT_REASONS = new Set([
  'source-added', 'source-removed', 'source-digest-changed', 'activation-changed', 'budget-changed',
  'prompt-boundary-changed', 'trust-state-changed', 'source-reference-changed', 'source-unavailable',
  'compaction-recovery', 'restart-reconcile', 'tool-schema-changed', 'security-context-changed'
])

function isCanonicalGeneralTerminalStringList(
  value: unknown,
  allowed?: ReadonlySet<string>
): value is string[] {
  if (!Array.isArray(value) || value.length > 512) return false
  const strings = value.filter((entry): entry is string => typeof entry === 'string')
  if (strings.length !== value.length || new Set(strings).size !== strings.length) return false
  const hasControlCharacter = (entry: string): boolean => Array.from(entry).some((character) => {
    const codePoint = character.codePointAt(0)
    return codePoint !== undefined && (codePoint <= 0x1f || codePoint === 0x7f)
  })
  if (strings.some((entry) => entry.length > 1024 || hasControlCharacter(entry) ||
      (allowed !== undefined && !allowed.has(entry)))) return false
  return strings.every((entry, index) => index === 0 || strings[index - 1] <= entry)
}

function isCanonicalGeneralTerminalDiagnostics(diagnostics: Record<string, unknown>): boolean {
  if (Object.keys(diagnostics).length > 64) return false
  const rejected = diagnostics.terminalCacheDiagnosticsSchema !== undefined ||
    diagnostics.terminalCacheDiagnosticsValid !== undefined ||
    diagnostics.terminalCacheDiagnosticsDisposition !== undefined
  if (rejected) {
    return Object.keys(diagnostics).length === 3 &&
      diagnostics.terminalCacheDiagnosticsSchema === 'terminal-cache-diagnostics.v1' &&
      diagnostics.terminalCacheDiagnosticsValid === false &&
      diagnostics.terminalCacheDiagnosticsDisposition === 'rejected'
  }

  if (diagnostics.prefixChangeReasons !== undefined &&
      !isCanonicalGeneralTerminalStringList(diagnostics.prefixChangeReasons, GENERAL_TERMINAL_PREFIX_REASONS)) return false
  if (diagnostics.toolSourceChangeReasons !== undefined &&
      !isCanonicalGeneralTerminalStringList(diagnostics.toolSourceChangeReasons, GENERAL_TERMINAL_TOOL_SOURCE_REASONS)) return false
  if (diagnostics.contextEpochChangeReasons !== undefined &&
      !isCanonicalGeneralTerminalStringList(diagnostics.contextEpochChangeReasons, GENERAL_TERMINAL_CONTEXT_REASONS)) return false

  const hit = diagnostics.cacheHitTokens as number | undefined
  const miss = diagnostics.cacheMissTokens as number | undefined
  const hitPresent = hit !== undefined
  if (hitPresent !== (miss !== undefined)) return false
  const observedRate = diagnostics.cacheHitRate as number | undefined
  if (observedRate !== undefined && !hitPresent) return false
  if (hitPresent && miss !== undefined) {
    const total = (hit as number) + miss
    if (total === 0 ? observedRate !== undefined : observedRate !== (hit as number) / total) return false
  }
  if (diagnostics.cacheTelemetrySource !== undefined && !hitPresent) return false
  const cacheDetailsPresent = hitPresent || observedRate !== undefined ||
    diagnostics.cacheTelemetrySource !== undefined
  if (cacheDetailsPresent && diagnostics.cacheTelemetryPresent === undefined) return false
  if (diagnostics.cacheTelemetryPresent !== undefined) {
    if (diagnostics.cacheTelemetryPresent !== hitPresent ||
        (diagnostics.cacheTelemetryPresent === false &&
          (observedRate !== undefined || diagnostics.cacheTelemetrySource !== undefined)) ||
        (diagnostics.cacheTelemetryPresent === true &&
          diagnostics.cacheTelemetrySource !== 'provider_usage')) return false
  }

  const attemptDetailKeys = [
    'providerAttemptStatuses', 'providerAttemptInputTokens', 'providerAttemptOutputTokens',
    'providerAttemptCacheHitTokens', 'providerAttemptCacheMissTokens', 'providerAttemptCacheRate',
    'providerLogicalCallCount', 'providerAttemptCount'
  ] as const
  const attemptDetailsPresent = attemptDetailKeys.some((key) => diagnostics[key] !== undefined)
  const attemptSchemaPresent = diagnostics.providerAttemptTelemetrySchema !== undefined
  const attemptValidPresent = diagnostics.providerAttemptTelemetryValid !== undefined
  if (attemptSchemaPresent !== attemptValidPresent) return false
  if (!attemptSchemaPresent) {
    if (diagnostics.providerAttemptTelemetryError !== undefined || attemptDetailsPresent) return false
  } else if (diagnostics.providerAttemptTelemetryValid === true) {
    if (diagnostics.providerAttemptTelemetryError !== undefined ||
        attemptDetailKeys.some((key) => diagnostics[key] === undefined)) return false
    const logical = diagnostics.providerLogicalCallCount as number
    const attempts = diagnostics.providerAttemptCount as number
    const statuses = diagnostics.providerAttemptStatuses as {
      succeeded: number
      failed: number
      cancelled: number
      timedOut: number
      streamAborted: number
    }
    if (logical > attempts || statuses.succeeded + statuses.failed + statuses.cancelled +
        statuses.timedOut + statuses.streamAborted !== attempts) return false
  } else if (diagnostics.providerAttemptTelemetryError !== 'invalid_or_incomplete' || attemptDetailsPresent) {
    return false
  }

  const costKeys = ['providerCostEstimateComplete', 'providerCostKnownAttemptCount',
    'providerKnownCostUsdNanos', 'providerKnownCostCnyNanos'] as const
  if (costKeys.some((key) => diagnostics[key] !== undefined)) {
    if (costKeys.some((key) => diagnostics[key] === undefined) || diagnostics.providerAttemptCount === undefined) return false
    const known = diagnostics.providerCostKnownAttemptCount as number
    const total = diagnostics.providerAttemptCount as number
    if (known > total || diagnostics.providerCostEstimateComplete !== (total > 0 && known === total) ||
        (known === 0 && (diagnostics.providerKnownCostUsdNanos !== 0 || diagnostics.providerKnownCostCnyNanos !== 0))) return false
  }

  if (diagnostics.cacheBaselineSchema !== undefined &&
      (typeof diagnostics.cacheContinuityDigest !== 'string' || diagnostics.cacheContinuityDigest.length === 0 ||
       typeof diagnostics.cacheProviderNamespaceDigest !== 'string' || diagnostics.cacheProviderNamespaceDigest.length === 0)) {
    return false
  }

  const contextDetailKeys = [
    'contextEpoch', 'contextEpochDigest', 'contextEpochRegistryDigest',
    'contextEpochChangeReasons', 'contextEpochImpact'
  ] as const
  const contextDetailsPresent = contextDetailKeys.some((key) => diagnostics[key] !== undefined)
  if (diagnostics.contextEpochStateValid === true) {
    if (contextDetailKeys.some((key) => diagnostics[key] === undefined) ||
        diagnostics.contextEpochDigest === '' || diagnostics.contextEpochRegistryDigest === '') return false
  } else if (diagnostics.contextEpochStateValid === false) {
    if (contextDetailsPresent) return false
  } else if (contextDetailsPresent) {
    return false
  }
  return true
}

const GeneralTerminalCacheDiagnosticsV1Schema = z.object({
  dynamicStateCheck: z.literal('not_checked').optional(),
  toolSchemaEstimator: z.literal('utf8_bytes_div4').optional(),
  responseModelObservation: z.enum(['not_reported', 'matches_resolved', 'differs_resolved']).optional(),
  modelInputFirstDifference: z.enum(['unavailable', 'none', 'system', 'tools', 'history', 'current', 'ordering']).optional(),
  modelInputComparable: z.boolean().optional(),
  modelInputComparablePrefixBytes: generalTerminalCanonicalUnsignedV1Schema().optional(),
  providerCostEstimateComplete: z.boolean().optional(),
  providerCostKnownAttemptCount: generalTerminalCanonicalUnsignedV1Schema().optional(),
  providerKnownCostUsdNanos: generalTerminalCanonicalUnsignedV1Schema().optional(),
  providerKnownCostCnyNanos: generalTerminalCanonicalUnsignedV1Schema().optional(),
  route: z.enum(['direct_answer', 'light_agent', 'tool_agent', 'subagent_agent']).optional(),
  prefixHash: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  systemHash: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  prefixItemsHash: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  toolsHash: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  toolSourcesHash: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  cacheContinuityDigest: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  cacheProviderNamespaceDigest: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  contextEpochDigest: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  contextEpochRegistryDigest: z.union([z.literal(''), GeneralTerminalDigestV1Schema]).optional(),
  prefixChanged: z.boolean().optional(),
  toolSourceChanged: z.boolean().optional(),
  cacheTelemetrySupported: z.boolean().optional(),
  cacheTelemetryPresent: z.boolean().optional(),
  providerNativeCacheTelemetry: z.boolean().optional(),
  cacheBaselineObserved: z.boolean().optional(),
  providerAttemptTelemetryValid: z.boolean().optional(),
  contextEpochStateValid: z.boolean().optional(),
  toolSchemaTokens: generalTerminalCanonicalUnsignedV1Schema().optional(),
  toolCount: generalTerminalCanonicalUnsignedV1Schema().optional(),
  firstTokenLatencyMs: generalTerminalCanonicalUnsignedV1Schema().optional(),
  durationMs: generalTerminalCanonicalUnsignedV1Schema().optional(),
  cacheHitTokens: generalTerminalCanonicalUnsignedV1Schema().optional(),
  cacheMissTokens: generalTerminalCanonicalUnsignedV1Schema().optional(),
  providerLogicalCallCount: generalTerminalCanonicalUnsignedV1Schema().optional(),
  providerAttemptCount: generalTerminalCanonicalUnsignedV1Schema().optional(),
  contextEpoch: generalTerminalCanonicalUnsignedV1Schema().optional(),
  cacheHitRate: GeneralTerminalCanonicalRateV1Schema.optional(),
  cacheTelemetrySource: z.literal('provider_usage').optional(),
  cacheBaselineSchema: z.literal('cache-prefix-baseline.v1').optional(),
  providerAttemptTelemetrySchema: z.literal('provider-attempt-telemetry.v1').optional(),
  providerAttemptTelemetryError: z.literal('invalid_or_incomplete').optional(),
  terminalCacheDiagnosticsSchema: z.literal('terminal-cache-diagnostics.v1').optional(),
  terminalCacheDiagnosticsValid: z.literal(false).optional(),
  terminalCacheDiagnosticsDisposition: z.literal('rejected').optional(),
  prefixChangeReasons: z.array(z.string()).optional(),
  toolSourceChangeReasons: z.array(z.string()).optional(),
  contextEpochChangeReasons: z.array(z.string()).optional(),
  providerAttemptStatuses: GeneralTerminalAttemptStatusesV1Schema.optional(),
  providerAttemptInputTokens: GeneralTerminalTokenAggregateV1Schema.optional(),
  providerAttemptOutputTokens: GeneralTerminalTokenAggregateV1Schema.optional(),
  providerAttemptCacheHitTokens: GeneralTerminalTokenAggregateV1Schema.optional(),
  providerAttemptCacheMissTokens: GeneralTerminalTokenAggregateV1Schema.optional(),
  providerAttemptCacheRate: GeneralTerminalAttemptRateV1Schema.optional(),
  contextEpochImpact: GeneralTerminalContextEpochImpactV1Schema.optional()
}).strict().superRefine((diagnostics, ctx) => {
  if (!isCanonicalGeneralTerminalDiagnostics(diagnostics)) {
    ctx.addIssue({ code: 'custom', path: [], message: 'terminal cache diagnostics are not canonical' })
  }
})

const GENERAL_TERMINAL_USAGE_REQUIRED_KEYS = [
  'promptTokens', 'completionTokens', 'reasoningTokens', 'totalTokens', 'cacheHitRate',
  'cacheableTokenHitRate', 'totalInputTokenHitRate', 'cacheMissReasons', 'cacheSuggestions',
  'costUsd', 'costCny', 'priceConfigured', 'cacheSavingsUsd', 'cacheSavingsCny',
  'tokenEconomySavingsTokens', 'turns'
] as const
const GENERAL_TERMINAL_USAGE_CACHE_KEYS = ['cachedTokens', 'cacheHitTokens', 'cacheMissTokens'] as const

function isCanonicalGeneralTerminalUsage(usage: Record<string, unknown>): boolean {
  const keys = Object.keys(usage)
  const hasCache = GENERAL_TERMINAL_USAGE_CACHE_KEYS.every((key) =>
    Object.prototype.hasOwnProperty.call(usage, key)
  )
  if (GENERAL_TERMINAL_USAGE_CACHE_KEYS.some((key) =>
    Object.prototype.hasOwnProperty.call(usage, key)
  ) !== hasCache) return false
  const expectedKeys = hasCache
    ? [...GENERAL_TERMINAL_USAGE_REQUIRED_KEYS, ...GENERAL_TERMINAL_USAGE_CACHE_KEYS]
    : [...GENERAL_TERMINAL_USAGE_REQUIRED_KEYS]
  if (keys.length !== expectedKeys.length || expectedKeys.some((key) => !keys.includes(key))) return false

  const prompt = usage.promptTokens as number
  const completion = usage.completionTokens as number
  const reasoning = usage.reasoningTokens as number
  const total = usage.totalTokens as number
  if (![prompt, completion, reasoning].every((value) =>
    Number.isSafeInteger(value) && value >= 0 && value <= 1_000_000_000 && !Object.is(value, -0)
  ) || !Number.isSafeInteger(total) || total < 0 || total > 2_000_000_000 || Object.is(total, -0) ||
      reasoning > completion || total !== prompt + completion || usage.turns !== 1 ||
      usage.tokenEconomySavingsTokens !== 0 || Object.is(usage.tokenEconomySavingsTokens, -0) ||
      !Array.isArray(usage.cacheMissReasons) || usage.cacheMissReasons.length !== 0 ||
      !Array.isArray(usage.cacheSuggestions) || usage.cacheSuggestions.length !== 0) return false

  for (const key of ['costUsd', 'costCny', 'cacheSavingsUsd', 'cacheSavingsCny'] as const) {
    const amount = usage[key]
    if (typeof amount !== 'number' || !Number.isFinite(amount) || amount < 0 || amount > 1e12) return false
  }
  if (typeof usage.priceConfigured !== 'boolean') return false

  let hit = 0
  let miss = 0
  if (hasCache) {
    hit = usage.cacheHitTokens as number
    miss = usage.cacheMissTokens as number
    if (![hit, miss, usage.cachedTokens].every((value) =>
      Number.isSafeInteger(value) && (value as number) >= 0 && (value as number) <= 1_000_000_000 &&
        !Object.is(value, -0)
    ) || usage.cachedTokens !== hit || hit + miss > prompt) return false
  }
  const cacheTotal = hit + miss
  const expectedCacheRate = hasCache && cacheTotal > 0 ? hit / cacheTotal : null
  const expectedInputRate = hasCache && prompt > 0 && cacheTotal > 0 ? hit / prompt : null
  const rates = [usage.cacheHitRate, usage.cacheableTokenHitRate, usage.totalInputTokenHitRate]
  return rates.every((rate) => rate === null || !Object.is(rate, -0)) &&
    usage.cacheHitRate === expectedCacheRate && usage.cacheableTokenHitRate === expectedCacheRate &&
    usage.totalInputTokenHitRate === expectedInputRate
}

const GeneralTerminalUsageEventV1Schema = z.object({
  kind: z.literal('usage'),
  seq: z.number().int().positive().safe(),
  timestamp: GeneralTerminalTimestampV1Schema,
  threadId: GeneralTerminalPublicIDV1Schema,
  turnId: GeneralTerminalPublicIDV1Schema,
  model: z.string(),
  usage: UsageSnapshotSchema.strict(),
  cacheDiagnostics: GeneralTerminalCacheDiagnosticsV1Schema,
  usageSource: GeneralTerminalExactTextV1Schema.optional(),
  childRunId: GeneralTerminalPublicIDV1Schema.optional(),
  usageFinalStatus: z.enum(['completed', 'failed', 'aborted']).optional()
}).strict().superRefine((event, ctx) => {
  const usage = event.usage
  if (!isCanonicalGeneralTerminalUsage(usage)) {
    ctx.addIssue({ code: 'custom', path: ['usage'], message: 'general terminal usage is not canonical' })
  }
  const telemetryPresent = event.cacheDiagnostics.cacheTelemetryPresent
  const hasCache = event.usage.cacheHitTokens !== undefined
  if (telemetryPresent !== undefined && (telemetryPresent !== hasCache ||
      (telemetryPresent && (event.cacheDiagnostics.cacheHitTokens !== event.usage.cacheHitTokens ||
        event.cacheDiagnostics.cacheMissTokens !== event.usage.cacheMissTokens ||
        event.cacheDiagnostics.cacheTelemetrySource !== 'provider_usage')))) {
    ctx.addIssue({ code: 'custom', path: ['cacheDiagnostics'], message: 'terminal cache usage binding is mismatched' })
  }
})

const GeneralTerminalLifecycleEventV1Schema = z.object({
  kind: z.enum(['turn_completed', 'turn_failed', 'turn_aborted']),
  seq: z.number().int().positive().safe(),
  timestamp: GeneralTerminalTimestampV1Schema,
  threadId: GeneralTerminalPublicIDV1Schema,
  turnId: GeneralTerminalPublicIDV1Schema,
  itemId: GeneralTerminalPublicIDV1Schema.optional(),
  status: z.enum(['completed', 'failed', 'aborted']),
  terminalReason: AcceptedFinalTerminalReasonSchema,
  message: GeneralTerminalExactTextV1Schema.optional(),
  error: GeneralTerminalExactTextV1Schema.optional(),
  code: GeneralTerminalExactTextV1Schema.optional(),
  details: GeneralTerminalErrorDetailsV1Schema.optional(),
  severity: RuntimeErrorSeverity.optional(),
  discard: z.boolean().optional(),
  cancelled: z.boolean().optional(),
  cancelledPendingGates: generalTerminalCanonicalUnsignedV1Schema().optional()
}).strict().superRefine((event, ctx) => {
  const expectedStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[event.terminalReason]
  if (event.kind !== `turn_${expectedStatus}` || event.status !== expectedStatus) {
    ctx.addIssue({ code: 'custom', path: ['status'], message: 'general terminal status is mismatched' })
  }
  const hasInterrupt = event.discard !== undefined || event.cancelled !== undefined ||
    event.cancelledPendingGates !== undefined
  if (event.terminalReason === 'cancel') {
    if (event.discard === undefined || event.cancelled === undefined || event.cancelledPendingGates === undefined) {
      ctx.addIssue({ code: 'custom', path: ['discard'], message: 'cancel terminal disposition is incomplete' })
    } else if (event.cancelledPendingGates > 0 && event.cancelled === false) {
      ctx.addIssue({ code: 'custom', path: ['cancelled'], message: 'pending gate cancellation is inconsistent' })
    }
  } else if (hasInterrupt) {
    ctx.addIssue({ code: 'custom', path: ['discard'], message: 'non-cancel terminal carries interrupt metadata' })
  }
  const hasFailureProjection = event.message !== undefined || event.error !== undefined ||
    event.code !== undefined || event.details !== undefined || event.severity !== undefined
  if (hasFailureProjection && !isGeneralTerminalFailureProjectionV1(event)) {
    ctx.addIssue({ code: 'custom', path: ['code'], message: 'general terminal lifecycle failure projection is not host canonical' })
  }
  if (event.details !== undefined &&
      (event.code !== 'tool_not_advertised' || !isToolNotAdvertisedDiagnosticDetailsV1(event.details))) {
    ctx.addIssue({ code: 'custom', path: ['details'], message: 'general terminal lifecycle diagnostics are not code-bound' })
  }
})

const GeneralTerminalDeliveryEventsV1Schema = z.union([
  z.tuple([
    GeneralTerminalUsageEventV1Schema,
    GeneralTerminalLifecycleEventV1Schema
  ]),
  z.tuple([
    GeneralTerminalItemCompletedEventV1Schema,
    GeneralTerminalUsageEventV1Schema,
    GeneralTerminalLifecycleEventV1Schema
  ])
])

const GeneralTerminalManifestEventV1Schema = z.object({
  slot: z.enum(['terminal-item', 'usage', 'terminal']),
  eventId: GeneralTerminalDigestV1Schema,
  payloadDigest: GeneralTerminalDigestV1Schema
}).strict()

/**
 * Atomic ordinary-terminal transport. Its CAS/hash fields bind delivery only:
 * they never confer evidence, citation, fact-answer, or publication authority.
 */
export const GeneralTerminalDeliveryBatchV1Schema = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal('analytix.general-terminal-delivery-batch/v1'),
  kind: z.literal('general_terminal_batch'),
  batchDigest: GeneralTerminalDigestV1Schema,
  threadId: GeneralTerminalPublicIDV1Schema,
  turnId: GeneralTerminalPublicIDV1Schema,
  seq: z.number().int().positive().safe(),
  firstSeq: z.number().int().positive().safe(),
  lastSeq: z.number().int().positive().safe(),
  timestamp: GeneralTerminalTimestampV1Schema,
  generalTerminalCommitId: GeneralTerminalDigestV1Schema,
  generalTerminalAuthorityKind: z.literal('general_terminal_cas'),
  generalTerminalAuthorityDigest: GeneralTerminalDigestV1Schema,
  eventManifestDigest: GeneralTerminalDigestV1Schema,
  projectedEventsDigest: GeneralTerminalDigestV1Schema,
  transportAuthority: z.literal('host_batch_digest_v1'),
  evidenceAuthority: z.literal(false),
  citationAuthority: z.literal(false),
  factAnswerAllowed: z.literal(false),
  events: GeneralTerminalDeliveryEventsV1Schema,
  eventManifest: z.array(GeneralTerminalManifestEventV1Schema).min(2).max(3)
}).strict().superRefine((batch, ctx) => {
  if (batch.seq !== batch.lastSeq || batch.lastSeq - batch.firstSeq + 1 !== batch.events.length ||
      batch.eventManifest.length !== batch.events.length) {
    ctx.addIssue({ code: 'custom', path: ['seq'], message: 'general terminal delivery range is not contiguous' })
  }
  const expectedSlots = batch.events.length === 2
    ? ['usage', 'terminal']
    : ['terminal-item', 'usage', 'terminal']
  const eventIds = new Set<string>()
  batch.events.forEach((event, index) => {
    if (event.threadId !== batch.threadId || event.turnId !== batch.turnId ||
        event.timestamp !== batch.timestamp || event.seq !== batch.firstSeq + index ||
        batch.eventManifest[index]?.slot !== expectedSlots[index] ||
        eventIds.has(batch.eventManifest[index]?.eventId ?? '')) {
      ctx.addIssue({ code: 'custom', path: ['events', index], message: 'general terminal delivery binding is mismatched' })
    }
    eventIds.add(batch.eventManifest[index]?.eventId ?? '')
  })
  const item = batch.events.length === 3
    ? batch.events[0] as z.infer<typeof GeneralTerminalItemCompletedEventV1Schema>
    : undefined
  const usage = batch.events[batch.events.length - 2] as z.infer<typeof GeneralTerminalUsageEventV1Schema>
  const terminal = batch.events[batch.events.length - 1] as z.infer<typeof GeneralTerminalLifecycleEventV1Schema>
  if (usage.kind !== 'usage' || !terminal.kind.startsWith('turn_') ||
      (usage.usageFinalStatus !== undefined && usage.usageFinalStatus !== terminal.status)) {
    ctx.addIssue({ code: 'custom', path: ['events'], message: 'general terminal delivery profile is mismatched' })
  }
  if (item?.kind === 'item_completed') {
    const terminalHasFailureProjection = terminal.code !== undefined || terminal.message !== undefined ||
      terminal.error !== undefined || terminal.severity !== undefined || terminal.details !== undefined
    const completedBoundaryFailure = new Set(['recovery', 'approval_denied', 'input_cancelled'])
      .has(terminal.terminalReason)
    const errorItemMismatch = item.item.kind === 'error' && (
      item.item.status !== terminal.status || terminal.itemId !== item.itemId ||
      terminal.code !== item.item.code || terminal.message !== item.item.message ||
      terminal.severity !== item.item.severity ||
      !sameGeneralTerminalFailureDetailsV1(item.item.details, terminal.details) ||
      (terminal.error !== undefined && terminal.error !== item.item.message)
    )
    if ((item.item.kind === 'assistant_text' &&
         (terminal.status !== 'completed' || terminalHasFailureProjection !== completedBoundaryFailure)) ||
        errorItemMismatch ||
        (terminal.itemId !== undefined && terminal.itemId !== item.itemId)) {
      ctx.addIssue({ code: 'custom', path: ['events', 0], message: 'general terminal item profile is mismatched' })
    }
  } else if (terminal.itemId !== undefined) {
    ctx.addIssue({ code: 'custom', path: ['events'], message: 'item-free general terminal refers to an item' })
  }
})
export type GeneralTerminalDeliveryBatchV1 = z.infer<typeof GeneralTerminalDeliveryBatchV1Schema>

function sameGeneralTerminalFailureDetailsV1(
  left: z.infer<typeof GeneralTerminalErrorDetailsV1Schema> | undefined,
  right: z.infer<typeof GeneralTerminalErrorDetailsV1Schema> | undefined
): boolean {
  if (left === undefined || right === undefined) return left === right
  const leftKeys = Object.keys(left).sort()
  const rightKeys = Object.keys(right).sort()
  return leftKeys.length === rightKeys.length && leftKeys.every((key, index) =>
    key === rightKeys[index] && left[key as keyof typeof left] === right[key as keyof typeof right]
  )
}

export const ThreadLifecycleEvent = RuntimeEventBase.extend({
  kind: z.enum(['thread_created', 'thread_updated']),
  title: z.string().optional(),
  status: z.string().optional()
})
export type ThreadLifecycleEvent = z.infer<typeof ThreadLifecycleEvent>

export const ThreadRewoundEvent = RuntimeEventBase.extend({
  kind: z.literal('thread_rewound'),
  rewindTurnId: z.string().min(1),
  removedTurns: z.number().int().nonnegative(),
  remainingTurns: z.number().int().nonnegative(),
  removedTurnIds: z.array(z.string().min(1)).default([])
})
export type ThreadRewoundEvent = z.infer<typeof ThreadRewoundEvent>

export const TurnLifecycleEvent = RuntimeEventBase.extend({
  kind: z.enum([
    'turn_started',
    'turn_completed',
    'turn_failed',
    'turn_aborted',
    'turn_steered'
  ]),
  status: z.string().optional(),
  text: z.string().optional(),
  clientUserMessageId: z.string().min(1).optional(),
  admittedSeq: z.number().int().nonnegative().optional(),
  message: z.string().optional(),
  code: z.string().optional(),
  details: z.unknown().optional(),
  severity: RuntimeErrorSeverity.optional(),
  acceptedFinalDigest: AcceptedFinalDigestSchema.optional(),
  terminalReason: AcceptedFinalTerminalReasonSchema.optional(),
  discard: z.boolean().optional(),
  cancelled: z.boolean().optional()
}).superRefine((event, ctx) => {
  const hasAcceptedTerminal = event.acceptedFinalDigest !== undefined || event.terminalReason !== undefined
  if (!hasAcceptedTerminal) return
  if (!event.acceptedFinalDigest || !event.terminalReason || !event.turnId) {
    ctx.addIssue({
      code: 'custom',
      path: ['acceptedFinalDigest'],
      message: 'accepted-final terminal requires digest, terminal reason, and turn identity'
    })
    return
  }
  const expectedStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[event.terminalReason]
  if (event.kind !== `turn_${expectedStatus}` || event.status !== expectedStatus) {
    ctx.addIssue({ code: 'custom', path: ['status'], message: 'accepted-final terminal status is mismatched' })
  }
  if (expectedStatus === 'aborted') {
    if (event.discard === undefined || event.cancelled === undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['discard'],
        message: 'accepted-final aborted terminal requires discard and cancelled disposition'
      })
    }
  } else if (event.discard !== undefined || event.cancelled !== undefined) {
    ctx.addIssue({
      code: 'custom',
      path: ['discard'],
      message: 'non-aborted accepted-final terminal must not carry abort disposition'
    })
  }
})
export type TurnLifecycleEvent = z.infer<typeof TurnLifecycleEvent>

export const ApprovalEvent = RuntimeEventBase.extend({
  kind: z.enum(['approval_requested', 'approval_resolved']),
  approvalId: z.string().min(1),
  toolName: z.string().min(1),
  status: z.enum(['pending', 'allowed', 'denied', 'expired']),
  approvalPolicy: ApprovalPolicySchema.optional(),
  sandboxMode: SandboxModeSchema.optional(),
  summary: z.string().optional()
})
export type ApprovalEvent = z.infer<typeof ApprovalEvent>

export const UserInputEvent = RuntimeEventBase.extend({
  kind: z.enum(['user_input_requested', 'user_input_resolved']),
  inputId: z.string().min(1),
  status: z.enum(['pending', 'submitted', 'cancelled']),
  prompt: z.string().optional(),
  questions: z.array(
    z.object({
      header: z.string().min(1),
      id: z.string().min(1),
      question: z.string().min(1),
      options: z.array(
        z.object({
          label: z.string().min(1),
          description: z.string()
        })
      )
    })
  ).optional()
})
export type UserInputEvent = z.infer<typeof UserInputEvent>

export const ToolCallReadyEvent = RuntimeEventBase.extend({
  kind: z.literal('tool_call_ready'),
  toolName: z.string().min(1),
  callId: z.string().min(1),
  readyCount: z.number().int().positive()
})
export type ToolCallReadyEvent = z.infer<typeof ToolCallReadyEvent>

export const ToolProgressEvent = RuntimeEventBase.extend({
  kind: z.literal('tool_progress'),
  toolName: z.string().min(1),
  callId: z.string().min(1),
  summary: z.string().optional(),
  status: z.string().min(1),
  message: z.string().optional(),
  details: z.record(z.string(), z.unknown()).optional()
})
export type ToolProgressEvent = z.infer<typeof ToolProgressEvent>

export const ToolUploadStatusEvent = RuntimeEventBase.extend({
  kind: z.literal('tool_result_upload_wait'),
  status: z.literal('waiting'),
  toolResultCount: z.number().int().nonnegative()
})
export type ToolUploadStatusEvent = z.infer<typeof ToolUploadStatusEvent>

export const ToolStormSuppressedEvent = RuntimeEventBase.extend({
  kind: z.literal('tool_storm_suppressed'),
  toolName: z.string().min(1),
  callId: z.string().min(1),
  message: z.string()
})
export type ToolStormSuppressedEvent = z.infer<typeof ToolStormSuppressedEvent>

export const ToolCatalogEvent = RuntimeEventBase.extend({
  kind: z.literal('tool_catalog_changed'),
  fingerprint: z.string().min(1),
  toolCount: z.number().int().nonnegative(),
  changeKind: z.enum(['additive', 'breaking']).optional(),
  toolNames: z.array(z.string().min(1)).optional(),
  message: z.string().optional()
})
export type ToolCatalogEvent = z.infer<typeof ToolCatalogEvent>

export const ChildSteerEvent = RuntimeEventBase.extend({
  kind: z.enum(['child_steer_queued', 'child_steer_admitted', 'child_steer_rejected']),
  status: z.enum(['queued', 'admitted', 'rejected', 'expired']).or(z.string().min(1)),
  jobId: z.string().min(1),
  childRunId: z.string().min(1),
  childThreadId: z.string().min(1).optional(),
  childTurnId: z.string().min(1).optional(),
  steerMessageId: z.string().min(1),
  parentThreadId: z.string().min(1),
  sourceTurnId: z.string().min(1).optional(),
  sourceToolCallId: z.string().min(1).optional(),
  createdAt: z.string().optional(),
  admittedAt: z.string().optional(),
  reason: z.string().optional()
})
export type ChildSteerEvent = z.infer<typeof ChildSteerEvent>

export const ChildPauseEvent = RuntimeEventBase.extend({
  kind: z.enum(['child_pause_requested', 'child_paused', 'child_resume_requested', 'child_resumed', 'child_pause_rejected']),
  status: z.enum(['requested', 'paused', 'rejected', 'expired', 'resumed', 'resume_requested']).or(z.string().min(1)),
  jobId: z.string().min(1),
  childRunId: z.string().min(1),
  childThreadId: z.string().min(1).optional(),
  childTurnId: z.string().min(1).optional(),
  pauseRequestId: z.string().min(1),
  parentThreadId: z.string().min(1),
  sourceTurnId: z.string().min(1).optional(),
  createdAt: z.string().optional(),
  pausedAt: z.string().optional(),
  resumedAt: z.string().optional(),
  reason: z.string().optional()
})
export type ChildPauseEvent = z.infer<typeof ChildPauseEvent>

export const MCPLifecycleAuditEvent = RuntimeEventBase.extend({
  kind: z.literal('mcp_lifecycle_audit'),
  schemaVersion: z.number().int().positive(),
  changeId: z.literal('mcp-lifecycle-contract'),
  runtimeContract: z.literal('analytix-go-runtime'),
  upstreamSource: z.literal('reasonix-absorbed'),
  serverId: z.string().min(1),
  transport: z.literal('live-local-jsonrpc'),
  protocolVersion: z.string().min(1),
  credentialed: z.boolean(),
  initialized: z.boolean(),
  notificationSent: z.boolean(),
  connected: z.boolean(),
  reconnect: z.boolean(),
  toolCount: z.number().int().nonnegative(),
  toolNames: z.array(z.string().min(1)),
  searchQuery: z.string(),
  searchMatches: z.array(z.string().min(1)),
  namespacePrefix: z.string().min(1),
  namespacedToolNames: z.boolean(),
  schemaOrderStable: z.boolean(),
  schemaHash: z.string().min(1),
  readOnlyHintMapped: z.boolean(),
  callRequiresApproval: z.boolean(),
  deniedCallExecuted: z.boolean(),
  approvedCallExecuted: z.boolean(),
  approvedCallOutput: z.string(),
  credentialRedaction: z.boolean(),
  diagnostic: z.string(),
  rawSecretPresent: z.boolean(),
  topLevelMcpIndexerExposed: z.boolean(),
  reasonixPublicProtocolUsed: z.boolean(),
  usesReasonixConfigRoot: z.boolean(),
  changesRendererContract: z.boolean(),
  changesProductIdentity: z.boolean()
})
export type MCPLifecycleAuditEvent = z.infer<typeof MCPLifecycleAuditEvent>

export const AutoResearchStateAuditEvent = RuntimeEventBase.extend({
  kind: z.literal('autoresearch_state_audit'),
  schemaVersion: z.number().int().positive(),
  changeId: z.literal('autoresearch-state-audit'),
  runtimeContract: z.literal('analytix-go-runtime'),
  upstreamSource: z.literal('reasonix-absorbed'),
  goalMode: z.literal('research'),
  stateRelativePath: z.string().regex(/^\.analytix\/autoresearch\/[^/]+$/),
  taskSpecPath: z.string().regex(/^\.analytix\/autoresearch\/[^/]+\/task_spec\.md$/),
  progressPath: z.string().regex(/^\.analytix\/autoresearch\/[^/]+\/progress\.json$/),
  findingsPath: z.string().regex(/^\.analytix\/autoresearch\/[^/]+\/findings\.jsonl$/),
  directionsTriedPath: z.string().regex(/^\.analytix\/autoresearch\/[^/]+\/directions_tried\.json$/),
  iterationLogPath: z.string().regex(/^\.analytix\/autoresearch\/[^/]+\/iteration_log\.jsonl$/),
  requiredFiles: z.array(z.enum([
    'task_spec.md',
    'progress.json',
    'findings.jsonl',
    'directions_tried.json',
    'iteration_log.jsonl'
  ])).length(5),
  fileCount: z.literal(5),
  requirementCount: z.number().int().nonnegative(),
  completedRequirementCount: z.number().int().nonnegative(),
  staleRequirementCount: z.number().int().nonnegative(),
  staleDirectionCount: z.number().int().nonnegative(),
  complete: z.boolean(),
  pivotRequired: z.boolean(),
  result: z.enum(['created', 'resumed', 'pivot_required', 'complete', 'errored']),
  unknownRequirementAccepted: z.literal(false),
  findingsWrittenForUnknownRequirement: z.literal(false),
  writesReasonixFile: z.literal(false),
  writesAgentsFile: z.literal(false),
  stablePrefixContainsState: z.literal(false),
  toolSchemaContainsState: z.literal(false),
  topLevelAutoResearchRouteExposed: z.literal(false),
  usesReasonixPublicProtocol: z.literal(false),
  usesReasonixConfigRoot: z.literal(false),
  changesRendererContract: z.literal(false),
  changesProductIdentity: z.literal(false)
})
export type AutoResearchStateAuditEvent = z.infer<typeof AutoResearchStateAuditEvent>

export const CacheDiagnosticsSchema = z.object({
  prefixHash: z.string().min(1),
  prefixChanged: z.boolean(),
  prefixChangeReasons: z.array(z.string()),
  toolSourceChanged: z.boolean().optional(),
  toolSourceChangeReasons: z.array(z.string()).optional(),
  systemHash: z.string().min(1),
  modeHash: z.string().min(1).optional(),
  prefixItemsHash: z.string().min(1),
  toolsHash: z.string().min(1),
  toolSchemaTokens: z.number().int().nonnegative(),
  toolCount: z.number().int().nonnegative().optional(),
  toolSourcesHash: z.string().min(1).optional(),
  toolSourceIds: z.array(z.string().min(1)).optional(),
  route: z.string().min(1).optional(),
  cacheTelemetrySupported: z.boolean(),
  firstTokenLatencyMs: z.number().int().nonnegative().optional(),
  durationMs: z.number().int().nonnegative().optional(),
  cacheHitTokens: z.number().int().nonnegative().optional(),
  cacheMissTokens: z.number().int().nonnegative().optional(),
  provider: z.string().optional(),
  providerId: z.string().optional(),
  endpointFormat: z.string().optional(),
  model: z.string().optional()
})
export type CacheDiagnostics = z.infer<typeof CacheDiagnosticsSchema>

export const CompactionEvent = RuntimeEventBase.extend({
  kind: z.enum(['compaction_started', 'compaction_completed']),
  summary: z.string().optional(),
  replacedTokens: z.number().int().nonnegative().optional(),
  auto: z.boolean().optional(),
  pinnedConstraints: z.array(z.string()).optional(),
  sourceDigest: z.string().min(1).optional(),
  digestMarker: z.string().min(1).optional(),
  sourceItemIds: z.array(z.string().min(1)).optional(),
  schemaVersion: z.literal(2).optional(),
  reasoningExcluded: z.literal(true).optional(),
  reasoningExclusionProof: z.string().regex(/^sha256:[a-f0-9]{64}$/).optional()
})
export type CompactionEvent = z.infer<typeof CompactionEvent>

export const GoalEvent = RuntimeEventBase.extend({
  kind: z.enum(['goal_updated', 'goal_cleared']),
  goal: ThreadGoalSchema.nullable().optional(),
  cleared: z.boolean().optional()
})
export type GoalEvent = z.infer<typeof GoalEvent>

export const GoalEvidenceAuditEvent = RuntimeEventBase.extend({
  kind: z.literal('goal_evidence_audit'),
  schemaVersion: z.number().int().positive(),
  changeId: z.literal('goal-evidence-audit'),
  runtimeContract: z.literal('analytix-go-runtime'),
  upstreamSource: z.literal('reasonix-absorbed'),
  goalId: z.string().min(1).optional(),
  result: z.enum(['allowed', 'blocked', 'errored']),
  recovered: z.boolean(),
  missingProjectChecks: z.number().int().nonnegative(),
  incompleteTodos: z.number().int().nonnegative(),
  commandMismatchMissing: z.number().int().nonnegative(),
  latestWriterReceiptIndex: z.number().int().nonnegative(),
  blockedStateKey: z.string().optional(),
  reason: z.string().optional(),
  missingCheckIds: z.array(z.string().min(1)).optional(),
  missingCommands: z.array(z.string().min(1)).optional(),
  usesReasonixPublicProtocol: z.boolean(),
  usesReasonixConfigRoot: z.boolean(),
  changesRendererContract: z.boolean(),
  changesProductIdentity: z.boolean()
})
export type GoalEvidenceAuditEvent = z.infer<typeof GoalEvidenceAuditEvent>

export const TodoEvent = RuntimeEventBase.extend({
  kind: z.enum(['todos_updated', 'todos_cleared']),
  todos: ThreadTodoListSchema.nullable().optional(),
  cleared: z.boolean().optional()
})
export type TodoEvent = z.infer<typeof TodoEvent>

export const CheckpointEvent = RuntimeEventBase.extend({
  kind: z.literal('checkpoint_captured'),
  checkpoint: CheckpointMetadataSchema
})
export type CheckpointEvent = z.infer<typeof CheckpointEvent>

export const CheckpointRewindRescueEvent = RuntimeEventBase.extend({
  kind: z.literal('checkpoint_rewind_rescue_created'),
  rescue: CheckpointRewindRescueRecordSchema
})
export type CheckpointRewindRescueEvent = z.infer<typeof CheckpointRewindRescueEvent>

export const CheckpointRewindAppliedEvent = RuntimeEventBase.extend({
  kind: z.literal('checkpoint_rewind_applied'),
  apply: CheckpointRewindApplyResultSchema
})
export type CheckpointRewindAppliedEvent = z.infer<typeof CheckpointRewindAppliedEvent>

export const UsageEvent = RuntimeEventBase.extend({
  kind: z.literal('usage'),
  model: z.string().optional(),
  providerId: z.string().optional(),
  endpointFormat: z.string().optional(),
  effort: ModelReasoningEffort.optional(),
  usageSource: z.string().optional(),
  childRunId: z.string().optional(),
  usage: UsageSnapshotSchema,
  cacheDiagnostics: CacheDiagnosticsSchema.optional()
})
export type UsageEvent = z.infer<typeof UsageEvent>

export const PipelineStageEvent = RuntimeEventBase.extend({
  kind: z.literal('pipeline_stage'),
  stage: PipelineStage,
  label: z.string().min(1).optional(),
  attempt: z.number().int().positive().optional(),
  maxAttempt: z.number().int().positive().optional(),
  details: z.record(z.string(), z.unknown()).optional()
})
export type PipelineStageEvent = z.infer<typeof PipelineStageEvent>

export const ErrorEvent = RuntimeEventBase.extend({
  kind: z.literal('error'),
  message: z.string(),
  code: z.string().optional(),
  details: z.unknown().optional(),
  severity: RuntimeErrorSeverity.optional(),
  terminal: z.boolean().optional(),
  fatal: z.boolean().optional(),
  recoverable: z.boolean().optional()
})
export type ErrorEvent = z.infer<typeof ErrorEvent>

export const HeartbeatEvent = RuntimeEventBase.extend({
  kind: z.literal('heartbeat')
})
export type HeartbeatEvent = z.infer<typeof HeartbeatEvent>

// Ephemeral SSE control frame. It is deliberately excluded from RuntimeEvent
// and carries no durable seq/timestamp, so clients cannot ACK it as history.
export const PublicProjectionRevokedEvent = z.object({
  schemaVersion: z.literal(1),
  kind: z.literal('public_projection_revoked'),
  threadId: z.string().min(1).max(256).refine((value) => value.trim() === value, {
    message: 'threadId must use its canonical form'
  }),
  historyAuthority: z.literal('case_boundary_only_v1'),
  code: z.enum(['case_public_authority_rejected', 'case_public_authority_unavailable']),
  action: z.literal('purge_case_projection'),
  terminal: z.literal(true)
}).strict()
export type PublicProjectionRevokedEvent = z.infer<typeof PublicProjectionRevokedEvent>

export const RuntimeEvent = z.discriminatedUnion('kind', [
  ItemEvent,
  ThreadLifecycleEvent,
  ThreadRewoundEvent,
  TurnLifecycleEvent,
  ApprovalEvent,
  UserInputEvent,
  ToolCallReadyEvent,
  ToolProgressEvent,
  ToolUploadStatusEvent,
  ToolStormSuppressedEvent,
  ToolCatalogEvent,
  ChildSteerEvent,
  ChildPauseEvent,
  MCPLifecycleAuditEvent,
  AutoResearchStateAuditEvent,
  CompactionEvent,
  GoalEvent,
  GoalEvidenceAuditEvent,
  TodoEvent,
  CheckpointEvent,
  CheckpointRewindRescueEvent,
  CheckpointRewindAppliedEvent,
  PipelineStageEvent,
  UsageEvent,
  ErrorEvent,
  HeartbeatEvent
])
export type RuntimeEvent = z.infer<typeof RuntimeEvent>

export const RuntimeEventList = z.array(RuntimeEvent)
export type RuntimeEventList = z.infer<typeof RuntimeEventList>
