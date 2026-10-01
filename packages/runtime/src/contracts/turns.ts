import { z } from 'zod'
import {
  TurnItem,
  UserFileReferenceSchema
} from './items.js'
import { isGuiPlanRelativePath } from '../shared/gui-plan.js'
import { ApprovalPolicySchema, SandboxModeSchema } from './policy.js'

/**
 * Mode enum, inlined here (instead of importing `ThreadMode` from
 * `threads.js`) to avoid a `threads <-> turns` module init cycle:
 * `threads.ts` already imports `TurnSchema` from this file. The two
 * literals must stay in sync with `ThreadMode` in `threads.ts`.
 */
const TurnModeSchema = z.enum(['agent', 'plan'])
export const TurnReasoningEffortSchema = z.enum(['auto', 'off', 'low', 'medium', 'high', 'max'])
export type TurnReasoningEffort = z.infer<typeof TurnReasoningEffortSchema>

/**
 * Plan operation kinds the renderer can advertise on a plan turn.
 * Mirrors the shared renderer contract so request metadata stays
 * stable across reconnects and replays.
 */
export const GuiPlanOperationSchema = z.enum(['draft', 'refine'])
export type GuiPlanOperationJson = z.infer<typeof GuiPlanOperationSchema>

/**
 * Plan context the renderer can attach to a `StartTurnRequest`. The
 * thread mode is carried on the thread record; this struct adds the
 * reserved path and source request needed to scope `create_plan`.
 */
export const GuiPlanContextSchema = z.object({
  operation: GuiPlanOperationSchema,
  workspaceRoot: z.string().min(1),
  relativePath: z
    .string()
    .min(1)
    .refine(isGuiPlanRelativePath, {
      message: 'relativePath must be a direct Markdown file under .analytixsdd/plan'
    }),
  planId: z.string().min(1),
  sourceRequest: z.string().optional(),
  title: z.string().optional()
}).strict()
export type GuiPlanContextJson = z.infer<typeof GuiPlanContextSchema>

export const TurnStatus = z.enum([
  'queued',
  'running',
  'completed',
  'failed',
  'aborted'
])
export type TurnStatus = z.infer<typeof TurnStatus>

export const SteeringEntrySchema = z.object({
  id: z.string().min(1).optional(),
  clientUserMessageId: z.string().min(1).optional(),
  text: z.string().min(1),
  displayText: z.string().optional(),
  attachmentIds: z.array(z.string().min(1)).default([]),
  fileReferences: z.array(UserFileReferenceSchema).default([]),
  admittedAt: z.string().optional(),
  status: z.enum(['pending', 'promoted', 'cancelled']).optional(),
  promotedAt: z.string().optional(),
  promotedItemId: z.string().min(1).optional(),
  cancelledAt: z.string().optional(),
  cancelReason: z.string().optional()
})
export type SteeringEntry = z.infer<typeof SteeringEntrySchema>

export const TurnSchema = z.object({
  id: z.string().min(1),
  threadId: z.string().min(1),
  status: TurnStatus,
  prompt: z.string(),
  model: z.string().optional(),
  reasoningEffort: TurnReasoningEffortSchema.optional(),
  /** Mid-turn user steering entries admitted while the turn is running. */
  steering: z.array(z.union([z.string(), SteeringEntrySchema])).default([]),
  createdAt: z.string(),
  startedAt: z.string().optional(),
  finishedAt: z.string().optional(),
  items: z.array(TurnItem).default([]),
  acceptedFinal: z.never().optional(),
  acceptedFinalView: z.never().optional(),
  attachmentIds: z.array(z.string().min(1)).default([]),
  activeSkillIds: z.array(z.string().min(1)).default([]),
  injectedMemoryIds: z.array(z.string().min(1)).default([]),
  skillInjectionBytes: z.number().int().nonnegative().optional(),
  workspaceCheckpointId: z.string().min(1).optional(),
  toolCatalogFingerprint: z.string().optional(),
  toolCatalogToolCount: z.number().int().nonnegative().optional(),
  toolCatalogDrift: z.boolean().optional(),
  /** Optional per-turn loop budget override. `0` means unlimited for this turn. */
  maxModelSteps: z.number().int().nonnegative().optional(),
  guiPlan: GuiPlanContextSchema.optional(),
  /**
   * Optional per-turn mode override. When set, it takes precedence over
   * the thread mode for this turn (e.g. a Plan-mode turn inside an
   * otherwise agent thread, or a Build turn that runs as agent).
   */
  mode: TurnModeSchema.optional(),
  /**
   * True when no interactive user is attached to this turn (IM bridges,
   * headless runs). Analytix hides `user_input`/`request_user_input` and
   * rejects calls to them instead of blocking on a GUI answer.
   */
  disableUserInput: z.boolean().optional(),
  error: z.string().optional()
})
export type Turn = z.infer<typeof TurnSchema>

export const StartTurnRequest = z.object({
  prompt: z.string().min(1),
  displayText: z.string().optional(),
  /** Raise-only host risk hint. Callers cannot request a general-policy downgrade. */
  riskIntent: z.literal('case').optional(),
  async: z.boolean().optional(),
  model: z.string().optional(),
  providerId: z.string().trim().min(1).max(128).optional(),
  endpointFormat: z.string().optional(),
  reasoningEffort: TurnReasoningEffortSchema.optional(),
  /**
   * Optional per-turn loop budget. `0` and omission inherit the bounded
   * thread/runtime setting; callers cannot disable the host step guard.
   */
  maxModelSteps: z.number().int().nonnegative().max(10_000).optional(),
  approvalPolicy: ApprovalPolicySchema.optional(),
  sandboxMode: SandboxModeSchema.optional(),
  /**
   * Optional per-turn mode. Overrides the thread mode for this turn so
   * the GUI can toggle Plan/agent without recreating the thread. In Plan
   * mode Analytix advertises `create_plan` for the whole conversation.
   */
  mode: TurnModeSchema.optional(),
  attachmentIds: z.array(z.string().trim().min(1)).max(4096).default([]),
  fileReferences: z.array(UserFileReferenceSchema.strict()).max(4096).default([]),
  workspaceCheckpointId: z.string().min(1).optional(),
  /**
   * Optional GUI plan context. When set, Analytix advertises the
   * `create_plan` tool for the turn and writes only to the reserved
   * path advertised in the context.
   */
  guiPlan: GuiPlanContextSchema.optional(),
  /**
   * True when the caller cannot relay structured input prompts to a
   * user (IM bridges such as WeChat/Feishu, headless runs). The turn
   * runs without the `user_input`/`request_user_input` tools.
   */
  disableUserInput: z.boolean().optional()
}).strict()
export type StartTurnRequest = z.input<typeof StartTurnRequest>

export const StartTurnResponse = z.object({
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  userMessageItemId: z.string().min(1)
})
export type StartTurnResponse = z.infer<typeof StartTurnResponse>

export const SteerTurnRequest = z.object({
  text: z.string().trim().min(1),
  displayText: z.string().optional(),
  /** Raise-only host risk hint. Callers cannot request a general-policy downgrade. */
  riskIntent: z.literal('case').optional(),
  clientUserMessageId: z.string().min(1).optional(),
  expectedTurnId: z.string().min(1).optional(),
  attachmentIds: z.array(z.string().trim().min(1)).max(4096).default([]),
  fileReferences: z.array(UserFileReferenceSchema.strict()).max(4096).default([]),
  delivery: z.literal('steer').optional()
}).strict()
export type SteerTurnRequest = z.input<typeof SteerTurnRequest>

export const SteerTurnResponse = z.object({
  ok: z.literal(true),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  itemId: z.string().min(1).optional(),
  clientUserMessageId: z.string().min(1).optional(),
  admittedSeq: z.number().int().nonnegative().optional()
})
export type SteerTurnResponse = z.infer<typeof SteerTurnResponse>

export const InterruptTurnRequest = z.object({
  /**
   * When true, discard generated items from the interrupted turn while
   * preserving the user's prompt. Omitted/false keeps the aborted items
   * visible for inspection.
   */
  discard: z.boolean().optional()
}).strict()
export type InterruptTurnRequest = z.infer<typeof InterruptTurnRequest>

export const InterruptTurnResponse = z.object({
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  status: TurnStatus,
  discard: z.boolean().optional(),
  cancelled: z.boolean().optional()
})
export type InterruptTurnResponse = z.infer<typeof InterruptTurnResponse>

export const RewindThreadRequest = z.object({
  turnId: z.string().min(1)
})
export type RewindThreadRequest = z.infer<typeof RewindThreadRequest>

export const RewindThreadResponse = z.object({
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  removedTurns: z.number().int().nonnegative(),
  remainingTurns: z.number().int().nonnegative(),
  removedTurnIds: z.array(z.string().min(1)).default([])
})
export type RewindThreadResponse = z.infer<typeof RewindThreadResponse>

export const CompactRequest = z.object({
  reason: z.string().optional(),
  /** Optional explicit token budget. */
  budgetTokens: z.number().int().positive().optional()
})
export type CompactRequest = z.infer<typeof CompactRequest>

export const CompactResponse = z.object({
  threadId: z.string().min(1),
  replacedTokens: z.number().int().nonnegative(),
  summary: z.string(),
  pinnedConstraints: z.array(z.string()),
  sourceDigest: z.string().min(1).optional(),
  digestMarker: z.string().min(1).optional(),
  sourceItemIds: z.array(z.string().min(1)).optional()
})
export type CompactResponse = z.infer<typeof CompactResponse>
