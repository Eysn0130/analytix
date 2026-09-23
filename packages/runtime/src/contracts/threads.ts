import { z } from 'zod'
import { TurnSchema } from './turns.js'
import {
  ApprovalPolicySchema,
  CURRENT_EXECUTION_POLICY_VERSION,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  SandboxModeSchema
} from './policy.js'
import { ModelExecutionSourceSchema } from './model-execution-ref.js'
import { MODEL_ENDPOINT_FORMATS } from './model-endpoint-format.js'
import { ModelReasoningEffort } from './capabilities.js'
import {
  TaskJobKindV1Schema,
  TaskJobStatusV1Schema,
  ThreadSummaryCommandTaskIdV1Schema,
  ThreadSummaryTaskJobIdV1Schema,
  ThreadSummaryTaskOutputResponseV1Schema
} from './task-job-output.js'

export const ThreadStatus = z.enum(['idle', 'running', 'archived', 'deleted'])
export type ThreadStatus = z.infer<typeof ThreadStatus>

export const PENDING_AUTO_THREAD_TITLE = '__analytix_pending_title__'

export const ThreadMode = z.enum(['agent', 'plan'])
export type ThreadMode = z.infer<typeof ThreadMode>

export const ThreadRuntimeStepLimitsSchema = z.object({
  /** Optional session-level override. `0` inherits the bounded runtime limit. */
  maxModelSteps: z.number().int().nonnegative().optional()
}).strict()
export type ThreadRuntimeStepLimits = z.infer<typeof ThreadRuntimeStepLimitsSchema>

/**
 * Discriminator describing how a thread relates to its origin.
 *
 * - `primary`: a top-level thread (the default).
 * - `fork`: a manual fork of another thread (switched-away clone).
 * - `side`: a "by-the-way" side conversation that inherits a one-time
 *   snapshot of its parent and runs in parallel. Excluded from the
 *   default thread listing.
 */
export const ThreadRelation = z.enum(['primary', 'fork', 'side'])
export type ThreadRelation = z.infer<typeof ThreadRelation>

export const ThreadGoalStatus = z.enum([
  'active',
  'paused',
  'blocked',
  'usageLimited',
  'budgetLimited',
  'complete'
])
export type ThreadGoalStatus = z.infer<typeof ThreadGoalStatus>

export const MAX_THREAD_GOAL_OBJECTIVE_CHARS = 4_000
export const MAX_THREAD_GOAL_EVIDENCE_STEP_CHARS = 1_000
export const MAX_THREAD_GOAL_EVIDENCE_SUMMARY_CHARS = 2_000
export const MAX_THREAD_GOAL_EVIDENCE_ITEM_CHARS = 2_000
export const MAX_THREAD_GOAL_EVIDENCE_ITEMS = 20
export const MAX_THREAD_GOAL_EVIDENCE_LEDGER_ENTRIES = 500
export const MAX_THREAD_GOAL_BLOCKED_REASON_CHARS = 1_000

export const ThreadGoalEvidenceEntrySchema = z.object({
  id: z.string().min(1),
  turnId: z.string().min(1).optional(),
  toolCallId: z.string().min(1).optional(),
  requirementId: z.string().min(1).optional(),
  step: z.string().trim().min(1).max(MAX_THREAD_GOAL_EVIDENCE_STEP_CHARS),
  evidence: z.array(
    z.string().trim().min(1).max(MAX_THREAD_GOAL_EVIDENCE_ITEM_CHARS)
  ).min(1).max(MAX_THREAD_GOAL_EVIDENCE_ITEMS),
  summary: z.string().trim().min(1).max(MAX_THREAD_GOAL_EVIDENCE_SUMMARY_CHARS).optional(),
  createdAt: z.string()
})
export type ThreadGoalEvidenceEntry = z.infer<typeof ThreadGoalEvidenceEntrySchema>

export const ThreadGoalResearchSchema = z.object({
  enabled: z.literal(true),
  stateRelativePath: z.string().min(1),
  taskSpecPath: z.string().min(1),
  progressPath: z.string().min(1),
  findingsPath: z.string().min(1),
  directionsTriedPath: z.string().min(1),
  iterationLogPath: z.string().min(1),
  requirementCount: z.number().int().nonnegative()
})
export type ThreadGoalResearch = z.infer<typeof ThreadGoalResearchSchema>

export const ThreadGoalSchema = z.object({
  /** Durable Go goal identity; legacy imported goals may not carry it. */
  id: z.string().min(1).optional(),
  threadId: z.string().min(1),
  objective: z.string().trim().min(1).max(MAX_THREAD_GOAL_OBJECTIVE_CHARS),
  status: ThreadGoalStatus,
  tokenBudget: z.number().int().positive().nullable().optional(),
  tokensUsed: z.number().int().nonnegative(),
  timeUsedSeconds: z.number().int().nonnegative(),
  evidenceLedger: z.array(ThreadGoalEvidenceEntrySchema).max(MAX_THREAD_GOAL_EVIDENCE_LEDGER_ENTRIES).optional(),
  research: ThreadGoalResearchSchema.optional(),
  blockedReason: z.string().trim().min(1).max(MAX_THREAD_GOAL_BLOCKED_REASON_CHARS).optional(),
  blockedCount: z.number().int().positive().optional(),
  blockedTurnId: z.string().min(1).optional(),
  strictCompletion: z.boolean().optional(),
  selfCheckRequired: z.boolean().optional(),
  selfCheckCompleted: z.boolean().optional(),
  selfCheckTurnId: z.string().min(1).optional(),
  createdAt: z.string(),
  updatedAt: z.string()
})
export type ThreadGoal = z.infer<typeof ThreadGoalSchema>

export const ThreadTodoStatus = z.enum(['pending', 'in_progress', 'completed', 'failed', 'canceled'])
export type ThreadTodoStatus = z.infer<typeof ThreadTodoStatus>

export const ThreadTodoStatusReasonCode = z.enum([
  'execution_failed',
  'dependency_failed',
  'verification_failed',
  'timeout',
  'tool_failed',
  'subagent_failed',
  'runtime_failed',
  'user_canceled',
  'parent_canceled',
  'runtime_canceled',
  'superseded'
])
export type ThreadTodoStatusReasonCode = z.infer<typeof ThreadTodoStatusReasonCode>

const THREAD_TODO_FAILURE_REASON_CODES = new Set<ThreadTodoStatusReasonCode>([
  'execution_failed', 'dependency_failed', 'verification_failed', 'timeout',
  'tool_failed', 'subagent_failed', 'runtime_failed'
])
const THREAD_TODO_CANCELLATION_REASON_CODES = new Set<ThreadTodoStatusReasonCode>([
  'user_canceled', 'parent_canceled', 'runtime_canceled', 'superseded'
])

export const ThreadTodoSourceSchema = z.discriminatedUnion('kind', [
  z.object({
    kind: z.literal('manual')
  }).strict(),
  z.object({
    kind: z.literal('plan'),
    planId: z.string().min(1).optional(),
    relativePath: z.string().min(1).optional(),
    ordinal: z.number().int().nonnegative().optional(),
    contentHash: z.string().min(1).optional()
  }).strict(),
  z.object({
    kind: z.literal('child'),
    parentThreadId: z.string().min(1).optional(),
    childThreadId: z.string().min(1).optional(),
    childRunId: z.string().min(1).optional(),
    jobId: z.string().min(1).optional(),
    projectionId: z.string().min(1).optional()
  }).strict()
])
export type ThreadTodoSource = z.infer<typeof ThreadTodoSourceSchema>

export const MAX_THREAD_TODO_CONTENT_CHARS = 1_000
export const MAX_THREAD_TODO_NOTE_CHARS = 2_000
export const MAX_THREAD_TODOS = 200

const ThreadTodoItemBaseSchema = z.object({
  id: z.string().min(1),
  content: z.string().trim().min(1).max(MAX_THREAD_TODO_CONTENT_CHARS),
  status: ThreadTodoStatus,
  statusReasonCode: ThreadTodoStatusReasonCode.optional(),
  source: ThreadTodoSourceSchema.optional(),
  note: z.string().trim().min(1).max(MAX_THREAD_TODO_NOTE_CHARS).optional(),
  createdAt: z.string(),
  updatedAt: z.string()
}).strict()

function refineTodoStatusReason(
  value: { status: ThreadTodoStatus; statusReasonCode?: ThreadTodoStatusReasonCode },
  ctx: z.RefinementCtx
): void {
  if (value.status === 'failed') {
    if (!value.statusReasonCode || !THREAD_TODO_FAILURE_REASON_CODES.has(value.statusReasonCode)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['statusReasonCode'],
        message: 'failed todo requires a valid failure statusReasonCode'
      })
    }
    return
  }
  if (value.status === 'canceled') {
    if (!value.statusReasonCode || !THREAD_TODO_CANCELLATION_REASON_CODES.has(value.statusReasonCode)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['statusReasonCode'],
        message: 'canceled todo requires a valid cancellation statusReasonCode'
      })
    }
    return
  }
  if (value.statusReasonCode !== undefined) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['statusReasonCode'],
      message: `${value.status} todo prohibits statusReasonCode`
    })
  }
}

export const ThreadTodoItemSchema = ThreadTodoItemBaseSchema.superRefine(refineTodoStatusReason)
export type ThreadTodoItem = z.infer<typeof ThreadTodoItemSchema>

export const ThreadTodoListSchema = z.object({
  threadId: z.string().min(1),
  turnId: z.string().min(1).optional(),
  items: z.array(ThreadTodoItemSchema).max(MAX_THREAD_TODOS),
  updatedAt: z.string()
}).strict().superRefine((value, ctx) => {
  const inProgressCount = value.items.filter((item) => item.status === 'in_progress').length
  if (inProgressCount > 1) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['items'],
      message: 'at most one todo can be in_progress'
    })
  }
  const ids = new Set<string>()
  for (const item of value.items) {
    if (ids.has(item.id)) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['items'], message: 'todo ids must be unique' })
      break
    }
    ids.add(item.id)
  }
})
export type ThreadTodoList = z.infer<typeof ThreadTodoListSchema>

export const ThreadSchema = z.object({
  id: z.string().min(1),
  title: z.string(),
  autoTitle: z.boolean().optional(),
  workspace: z.string(),
  model: z.string(),
  providerId: z.string().trim().min(1).max(128).optional(),
  mode: ThreadMode,
  status: ThreadStatus,
  executionPolicyVersion: z.literal(CURRENT_EXECUTION_POLICY_VERSION).optional(),
  approvalPolicy: ApprovalPolicySchema.default(DEFAULT_APPROVAL_POLICY),
  sandboxMode: SandboxModeSchema.default(DEFAULT_SANDBOX_MODE),
  costBudgetUsd: z.number().positive().optional(),
  costBudgetWarningSent: z.boolean().optional(),
  relation: ThreadRelation.default('primary'),
  parentThreadId: z.string().optional(),
  forkedFromThreadId: z.string().optional(),
  forkedFromTitle: z.string().optional(),
  forkedAt: z.string().optional(),
  forkedFromMessageCount: z.number().int().nonnegative().optional(),
  forkedFromTurnCount: z.number().int().nonnegative().optional(),
  runtimeStepLimits: ThreadRuntimeStepLimitsSchema.optional(),
  goal: ThreadGoalSchema.optional(),
  todos: ThreadTodoListSchema.optional(),
  createdAt: z.string(),
  updatedAt: z.string(),
  turns: z.array(TurnSchema).default([])
})
export type ThreadRecord = z.infer<typeof ThreadSchema>

export const ThreadSummarySchema = ThreadSchema.pick({
  id: true,
  title: true,
  workspace: true,
  model: true,
  providerId: true,
  mode: true,
  status: true,
  executionPolicyVersion: true,
  approvalPolicy: true,
  sandboxMode: true,
  costBudgetUsd: true,
  costBudgetWarningSent: true,
  relation: true,
  parentThreadId: true,
  forkedFromThreadId: true,
  forkedFromTitle: true,
  forkedAt: true,
  forkedFromMessageCount: true,
  forkedFromTurnCount: true,
  runtimeStepLimits: true,
  goal: true,
  todos: true,
  createdAt: true,
  updatedAt: true
}).extend({
  // Case-bound public projections deliberately omit the workspace path. The
  // host-issued history authority marker is the signal that consumers must
  // stay on the boundary-only history path instead of treating the omission
  // as an ordinary thread record.
  workspace: z.string().optional(),
  preview: z.string().optional(),
  messageCount: z.number().int().nonnegative().optional(),
  turnCount: z.number().int().nonnegative().optional(),
  latestTurnId: z.string().min(1).optional(),
  historyAuthority: z.literal('case_boundary_only_v1').optional()
}).strict()
export type ThreadSummary = z.infer<typeof ThreadSummarySchema>

export const CreateThreadRequest = z.object({
  title: z.string().optional(),
  autoTitle: z.boolean().optional(),
  workspace: z.string().min(1),
  model: z.string().min(1),
  providerId: z.string().trim().min(1).max(128).optional(),
  mode: ThreadMode.default('agent'),
  approvalPolicy: ApprovalPolicySchema.optional(),
  sandboxMode: SandboxModeSchema.optional(),
  costBudgetUsd: z.number().positive().optional(),
  runtimeStepLimits: ThreadRuntimeStepLimitsSchema.optional()
})
export type CreateThreadRequest = z.infer<typeof CreateThreadRequest>

/**
 * Optional body for `POST /v1/threads/{id}/fork`.
 *
 * `relation` defaults to `'fork'` to preserve the existing manual-fork
 * behavior when the body is absent. Passing `relation: 'side'` marks
 * the new thread as a side conversation (e.g. spawned by `/btw`).
 */
export const ForkThreadRequest = z
  .object({
    relation: ThreadRelation.default('fork'),
    title: z.string().optional(),
    turnId: z.string().trim().min(1).optional()
  })
  .optional()
export type ForkThreadRequest = z.infer<typeof ForkThreadRequest>

export const SetThreadGoalRequest = z
  .object({
    objective: z.string().trim().min(1).max(MAX_THREAD_GOAL_OBJECTIVE_CHARS).optional(),
    status: ThreadGoalStatus.optional(),
    tokenBudget: z.number().int().positive().nullable().optional(),
    blockedReason: z.string().trim().min(1).max(MAX_THREAD_GOAL_BLOCKED_REASON_CHARS).optional(),
    blockedCount: z.number().int().positive().optional(),
    blockedTurnId: z.string().trim().min(1).optional(),
    strictCompletion: z.boolean().optional(),
    selfCheckRequired: z.boolean().optional(),
    selfCheckCompleted: z.boolean().optional(),
    selfCheckTurnId: z.string().trim().min(1).optional(),
    research: z.object({
      enabled: z.literal(true),
      requirements: z.array(z.string().trim().min(1).max(1_000)).max(50).optional()
    }).optional()
  })
  .refine(
    (value) =>
      value.objective !== undefined ||
      value.status !== undefined ||
      value.tokenBudget !== undefined ||
      value.blockedReason !== undefined ||
      value.blockedCount !== undefined ||
      value.blockedTurnId !== undefined ||
      value.strictCompletion !== undefined ||
      value.selfCheckRequired !== undefined ||
      value.selfCheckCompleted !== undefined ||
      value.selfCheckTurnId !== undefined ||
      value.research !== undefined,
    { message: 'goal request must change at least one field' }
  )
export type SetThreadGoalRequest = z.infer<typeof SetThreadGoalRequest>

export const ThreadGoalResponse = z.object({
  goal: ThreadGoalSchema.nullable()
})
export type ThreadGoalResponse = z.infer<typeof ThreadGoalResponse>

export const ClearThreadGoalResponse = z.object({
  cleared: z.boolean()
})
export type ClearThreadGoalResponse = z.infer<typeof ClearThreadGoalResponse>

const SetThreadTodoItemSchema = z.object({
  id: z.string().min(1).optional(),
  content: z.string().trim().min(1).max(MAX_THREAD_TODO_CONTENT_CHARS),
  status: ThreadTodoStatus,
  statusReasonCode: ThreadTodoStatusReasonCode.optional(),
  source: ThreadTodoSourceSchema.optional(),
  note: z.string().trim().min(1).max(MAX_THREAD_TODO_NOTE_CHARS).optional()
}).strict().superRefine(refineTodoStatusReason)

export const SetThreadTodosRequest = z.object({
  turnId: z.string().trim().min(1).optional(),
  todos: z.array(SetThreadTodoItemSchema).max(MAX_THREAD_TODOS)
}).strict().superRefine((value, ctx) => {
  const inProgressCount = value.todos.filter((item) => item.status === 'in_progress').length
  if (inProgressCount > 1) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['todos'],
      message: 'at most one todo can be in_progress'
    })
  }
  const ids = new Set<string>()
  for (const item of value.todos) {
    if (!item.id) continue
    if (ids.has(item.id)) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['todos'], message: 'todo ids must be unique' })
      break
    }
    ids.add(item.id)
  }
})
export type SetThreadTodosRequest = z.infer<typeof SetThreadTodosRequest>

export const ThreadTodosResponse = z.object({
  todos: ThreadTodoListSchema.nullable()
})
export type ThreadTodosResponse = z.infer<typeof ThreadTodosResponse>

export const ClearThreadTodosResponse = z.object({
  cleared: z.boolean()
})
export type ClearThreadTodosResponse = z.infer<typeof ClearThreadTodosResponse>

export const ResumeThreadResponse = z.object({
  thread_id: z.string().min(1),
  session_id: z.string().min(1),
  message_count: z.number().int().nonnegative(),
  summary: z.string()
}).strict()
export type ResumeThreadResponse = z.infer<typeof ResumeThreadResponse>

export const UpdateThreadRequest = z
  .object({
    title: z.string().optional(),
    workspace: z.string().min(1).optional(),
    providerId: z.string().trim().min(1).max(128).nullable().optional(),
    status: ThreadStatus.optional(),
    approvalPolicy: ApprovalPolicySchema.optional(),
    sandboxMode: SandboxModeSchema.optional(),
    costBudgetUsd: z.number().positive().nullable().optional(),
    costBudgetWarningSent: z.boolean().optional(),
    relation: ThreadRelation.optional(),
    runtimeStepLimits: ThreadRuntimeStepLimitsSchema.nullable().optional()
  })
  .refine(
    (value) =>
      value.title !== undefined ||
      value.workspace !== undefined ||
      value.providerId !== undefined ||
      value.status !== undefined ||
      value.approvalPolicy !== undefined ||
      value.sandboxMode !== undefined ||
      value.costBudgetUsd !== undefined ||
      value.costBudgetWarningSent !== undefined ||
      value.relation !== undefined ||
      value.runtimeStepLimits !== undefined,
    { message: 'update request must change at least one field' }
  )
export type UpdateThreadRequest = z.infer<typeof UpdateThreadRequest>

export const ListThreadsResponse = z.object({
  threads: z.array(ThreadSummarySchema)
})
export type ListThreadsResponse = z.infer<typeof ListThreadsResponse>

export const DeleteThreadResponse = z.object({
  id: z.string().min(1),
  deleted: z.literal(true)
})
export type DeleteThreadResponse = z.infer<typeof DeleteThreadResponse>

export const ThreadSummaryItemState = z.enum(['active', 'done', 'terminal', 'inactive'])
export type ThreadSummaryItemState = z.infer<typeof ThreadSummaryItemState>

export const ThreadSummaryPublicStatusV1Schema = z.enum([
  'queued',
  'running',
  'pause_requested',
  'paused',
  'resume_requested',
  'resuming',
  'completed',
  'failed',
  'aborted',
  'interrupted',
  'killed',
  'canceled',
  'timeout',
  'unknown',
  'starting',
  'done',
  'missing',
  'stopped',
  'archived',
  'idle',
  'pending',
  'error',
  'success'
])

export const ThreadSummaryJobDiagnosticsV1Schema = z.object({
  status: ThreadSummaryPublicStatusV1Schema,
  terminal: z.boolean(),
  background: z.boolean(),
  paused: z.boolean(),
  startedAt: z.string().datetime({ offset: true }).optional(),
  updatedAt: z.string().datetime({ offset: true }).optional(),
  finishedAt: z.string().datetime({ offset: true }).optional()
}).strict()

function threadSummaryStateForStatus(status: z.infer<typeof ThreadSummaryPublicStatusV1Schema>): z.infer<typeof ThreadSummaryItemState> {
  if (new Set([
    'queued', 'running', 'starting', 'pause_requested', 'paused', 'resume_requested', 'resuming'
  ]).has(status)) return 'active'
  if (status === 'completed' || status === 'done') return 'done'
  if (new Set(['failed', 'aborted', 'interrupted', 'killed', 'canceled', 'timeout']).has(status)) return 'terminal'
  return 'inactive'
}

const ThreadSummaryPublicIdV1Schema = z.string().regex(/^[A-Za-z0-9_.:-]{1,128}$/)

const ThreadSummaryWithheldOutputFieldsV1 = {
  schemaVersion: z.literal(1),
  outputWithheld: z.literal(true),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canContinueParent: z.literal(false),
  canReadOutput: z.literal(false)
} as const

export const ThreadSummarySubagentSchema = z.object({
  ...ThreadSummaryWithheldOutputFieldsV1,
  id: ThreadSummaryPublicIdV1Schema,
  key: ThreadSummaryPublicIdV1Schema,
  parentThreadId: ThreadSummaryPublicIdV1Schema,
  parentTurnId: ThreadSummaryPublicIdV1Schema.optional(),
  parentToolCallId: ThreadSummaryPublicIdV1Schema.optional(),
  childId: ThreadSummaryPublicIdV1Schema.optional(),
  childRunId: ThreadSummaryPublicIdV1Schema.optional(),
  taskJobId: ThreadSummaryPublicIdV1Schema.optional(),
  taskKind: TaskJobKindV1Schema.optional(),
  childThreadId: ThreadSummaryPublicIdV1Schema.optional(),
  childTurnId: ThreadSummaryPublicIdV1Schema.optional(),
  displayName: z.string().optional(),
  agentNickname: z.string().optional(),
  title: z.string().optional(),
  label: z.string().optional(),
  model: z.string().optional(),
  providerId: z.string().optional(),
  endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional(),
  variant: z.string().optional(),
  modelSource: ModelExecutionSourceSchema.optional(),
  effort: ModelReasoningEffort.optional(),
  profile: z.string().optional(),
  toolPolicy: z.string().optional(),
  maxModelSteps: z.number().int().nonnegative().optional(),
  timeBudgetMs: z.number().int().nonnegative().optional(),
  status: ThreadSummaryItemState,
  rawStatus: ThreadSummaryPublicStatusV1Schema,
  outputTrustStatus: z.literal('untrusted_child_output'),
  background: z.boolean().optional(),
  diagnostics: ThreadSummaryJobDiagnosticsV1Schema.optional(),
  parallelGroupId: ThreadSummaryPublicIdV1Schema.optional(),
  parallelIndex: z.number().int().nonnegative().optional(),
  toolInvocations: z.number().int().nonnegative().optional(),
  evidenceLedgered: z.boolean().optional(),
  durationMs: z.number().int().nonnegative().optional(),
  queuedMs: z.number().int().nonnegative().optional(),
  totalTokens: z.number().int().nonnegative().optional(),
  cacheHitRate: z.number().min(0).max(1).nullable().optional(),
  costUsd: z.number().nonnegative().optional(),
  costCny: z.number().nonnegative().optional(),
  canOpenThread: z.boolean(),
  canKill: z.literal(false),
  canRestart: z.literal(false),
  createdAt: z.string().datetime({ offset: true }).optional(),
  updatedAt: z.string().datetime({ offset: true })
}).strict().superRefine((value, context) => {
  if (value.id !== value.key) {
    context.addIssue({ code: 'custom', path: ['key'], message: 'thread summary subagent key mismatch' })
  }
  if (value.status !== threadSummaryStateForStatus(value.rawStatus)) {
    context.addIssue({ code: 'custom', path: ['status'], message: 'thread summary subagent lifecycle mismatch' })
  }
  if (value.canOpenThread !== Boolean(value.childThreadId)) {
    context.addIssue({ code: 'custom', path: ['canOpenThread'], message: 'thread summary child-thread capability mismatch' })
  }
  if (value.diagnostics) {
    const diagnostics = value.diagnostics
    const expectedTerminal = ['done', 'terminal'].includes(threadSummaryStateForStatus(diagnostics.status))
    const expectedPaused = diagnostics.status === 'paused' || diagnostics.status === 'pause_requested'
    if (diagnostics.status !== value.rawStatus || diagnostics.terminal !== expectedTerminal ||
        diagnostics.paused !== expectedPaused || diagnostics.background !== (value.background === true)) {
      context.addIssue({ code: 'custom', path: ['diagnostics'], message: 'thread summary diagnostics mismatch' })
    }
  }
})
export type ThreadSummarySubagent = z.infer<typeof ThreadSummarySubagentSchema>

const ThreadSummaryTaskJobSchema = z.object({
  schemaVersion: z.literal(1),
  id: ThreadSummaryTaskJobIdV1Schema.regex(/^taskjob:/),
  kind: TaskJobKindV1Schema,
  status: TaskJobStatusV1Schema,
  background: z.boolean(),
  active: z.boolean().optional(),
  terminal: z.boolean(),
  outputWithheld: z.literal(true),
  outputTrustStatus: z.literal('untrusted_child_output'),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canReadOutput: z.literal(false),
  canContinueParent: z.literal(false)
}).strict().superRefine((value, context) => {
  const active = new Set(['queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming']).has(value.status)
  const terminal = new Set(['completed', 'failed', 'aborted', 'interrupted', 'killed', 'canceled', 'timeout']).has(value.status)
  if (value.terminal !== terminal || (value.active !== undefined && value.active !== active)) {
    context.addIssue({ code: 'custom', path: ['status'], message: 'thread summary task-job lifecycle flags mismatch' })
  }
})

const ThreadSummaryCommandStatusV1Schema = z.enum([
  'running',
  'pending',
  'stopped',
  'failed',
  'aborted',
  'killed',
  'canceled',
  'timeout',
  'error',
  'completed',
  'done',
  'success',
  'unknown'
])

const ThreadSummaryCommandTaskSchema = z.object({
  schemaVersion: z.literal(1),
  id: ThreadSummaryCommandTaskIdV1Schema,
  kind: z.literal('command'),
  status: ThreadSummaryCommandStatusV1Schema,
  background: z.literal(false),
  active: z.boolean(),
  terminal: z.boolean(),
  outputWithheld: z.literal(true),
  outputTrustStatus: z.literal('private_tool_output'),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canReadOutput: z.literal(false),
  canContinueParent: z.literal(false)
}).strict().superRefine((value, context) => {
  const active = value.status === 'running' || value.status === 'pending'
  const terminal = new Set([
    'failed', 'aborted', 'killed', 'canceled', 'timeout', 'error', 'completed', 'done', 'success'
  ]).has(value.status)
  if (value.active !== active || value.terminal !== terminal) {
    context.addIssue({
      code: 'custom',
      path: ['status'],
      message: 'thread summary command lifecycle flags mismatch'
    })
  }
})

export const ThreadSummaryTaskSchema = z.union([
  ThreadSummaryTaskJobSchema,
  ThreadSummaryCommandTaskSchema
])
export type ThreadSummaryTask = z.infer<typeof ThreadSummaryTaskSchema>

export const ThreadSummaryOutputSchema = z.object({
  id: z.string().min(1),
  kind: z.enum(['generated_file', 'file', 'url', 'command_output', 'artifact']),
  label: z.string().min(1),
  path: z.string().optional(),
  relativePath: z.string().optional(),
  absolutePath: z.string().optional(),
  localFilePath: z.string().optional(),
  url: z.string().optional(),
  mimeType: z.string().optional(),
  byteSize: z.number().int().nonnegative().optional(),
  width: z.number().int().positive().optional(),
  height: z.number().int().positive().optional(),
  sourceItemId: z.string().optional(),
  turnId: z.string().optional(),
  createdAt: z.string().optional()
}).strict()
export type ThreadSummaryOutput = z.infer<typeof ThreadSummaryOutputSchema>

export const ThreadSummarySourceSchema = z.object({
  id: z.string().min(1),
  kind: z.enum(['web', 'tool', 'file', 'mcp', 'other']),
  label: z.string().min(1),
  title: z.string().optional(),
  url: z.string().optional(),
  toolName: z.string().optional(),
  serverName: z.string().optional(),
  sourceItemId: z.string().optional(),
  turnId: z.string().optional(),
  retrievedAt: z.string().optional()
}).strict()
export type ThreadSummarySource = z.infer<typeof ThreadSummarySourceSchema>

export const ThreadSummarySideChatSchema = z.object({
  threadId: ThreadSummaryPublicIdV1Schema,
  title: z.string(),
  status: ThreadStatus,
  relation: z.literal('side'),
  parentThreadId: ThreadSummaryPublicIdV1Schema,
  messageCount: z.number().int().nonnegative().optional(),
  turnCount: z.number().int().nonnegative().optional(),
  createdAt: z.string().datetime({ offset: true }),
  updatedAt: z.string().datetime({ offset: true })
}).strict()
export type ThreadSummarySideChat = z.infer<typeof ThreadSummarySideChatSchema>

export const ThreadSummaryBackgroundProcessSchema = z.object({
  id: z.string().min(1),
  threadId: z.string().min(1),
  turnId: z.string().optional(),
  itemId: z.string().optional(),
  command: z.string().optional(),
  cwd: z.string().optional(),
  shellSafety: z.object({
    readOnly: z.boolean(),
    reason: z.enum([
      'empty',
      'shell_syntax',
      'read_only_command',
      'read_only_prefix',
      'unknown_or_write_capable'
    ]),
    base: z.string().optional(),
    subcommand: z.string().optional()
  }).strict().optional(),
  pid: z.number().int().positive().nullable().optional(),
  processId: z.string().optional(),
  status: z.string().min(1),
  source: z.string().min(1),
  startedAt: z.string().datetime({ offset: true }).optional(),
  updatedAt: z.string().datetime({ offset: true }),
  finishedAt: z.string().datetime({ offset: true }).optional(),
  canKill: z.boolean().default(false),
  canRestart: z.boolean().default(false),
  canReadOutput: z.literal(false).default(false)
}).strict()
export type ThreadSummaryBackgroundProcess = z.infer<typeof ThreadSummaryBackgroundProcessSchema>

export const ThreadSummaryResponse = z.object({
  threadId: ThreadSummaryPublicIdV1Schema,
  generatedAt: z.string().datetime({ offset: true }),
  latestSeq: z.number().int().nonnegative(),
  subagents: z.array(ThreadSummarySubagentSchema),
  tasks: z.array(ThreadSummaryTaskSchema),
  outputs: z.array(ThreadSummaryOutputSchema).max(0),
  sources: z.array(ThreadSummarySourceSchema).max(0),
  sideChats: z.array(ThreadSummarySideChatSchema),
  backgroundProcesses: z.array(ThreadSummaryBackgroundProcessSchema).max(0),
  historyAuthority: z.literal('case_boundary_only_v1').optional()
}).strict().superRefine((value, context) => {
  for (const [index, subagent] of value.subagents.entries()) {
    if (subagent.parentThreadId !== value.threadId) {
      context.addIssue({ code: 'custom', path: ['subagents', index, 'parentThreadId'], message: 'thread summary parent identity mismatch' })
    }
  }
  for (const [index, sideChat] of value.sideChats.entries()) {
    if (sideChat.parentThreadId !== value.threadId || sideChat.threadId === value.threadId) {
      context.addIssue({ code: 'custom', path: ['sideChats', index], message: 'thread summary side-chat identity mismatch' })
    }
  }
  if (value.historyAuthority === 'case_boundary_only_v1' &&
      (value.tasks.length > 0 || value.sideChats.length > 0)) {
    context.addIssue({ code: 'custom', path: ['historyAuthority'], message: 'case-bound summary contains non-subagent metadata' })
  }
})
export type ThreadSummaryResponse = z.infer<typeof ThreadSummaryResponse>

export const ThreadSummaryTaskOutputResponse = ThreadSummaryTaskOutputResponseV1Schema
export type ThreadSummaryTaskOutputResponse = z.infer<typeof ThreadSummaryTaskOutputResponse>

export const ThreadSummaryTaskMutationResponse = z.object({
  task: ThreadSummaryTaskSchema
}).strict()
export type ThreadSummaryTaskMutationResponse = z.infer<typeof ThreadSummaryTaskMutationResponse>
