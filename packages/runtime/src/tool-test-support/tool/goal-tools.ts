import type { ThreadGoal } from '../../contracts/threads.js'
import type { ThreadService } from '../../services-test-support/thread-service.js'
import { LocalToolHost, type LocalTool } from './local-tool-host.js'

export const GET_GOAL_TOOL_NAME = 'get_goal'
export const CREATE_GOAL_TOOL_NAME = 'create_goal'
export const COMPLETE_STEP_TOOL_NAME = 'complete_step'
export const RECORD_RESEARCH_DIRECTION_TOOL_NAME = 'record_research_direction'
export const UPDATE_GOAL_TOOL_NAME = 'update_goal'
export const GOAL_TOOL_NAMES = [
  GET_GOAL_TOOL_NAME,
  CREATE_GOAL_TOOL_NAME,
  COMPLETE_STEP_TOOL_NAME,
  RECORD_RESEARCH_DIRECTION_TOOL_NAME,
  UPDATE_GOAL_TOOL_NAME
] as const

const BLOCKED_AUDIT_REQUIRED_COUNT = 3
const STRICT_COMPLETION_SELF_CHECK_INSTRUCTIONS = [
  'Strict goal completion self-check required:',
  '1. Verify changed files compile or parse correctly when applicable.',
  '2. Run the relevant tests or explain concrete evidence that covers the change.',
  '3. Confirm the original requirements, current todos, and evidence ledger are complete.',
  `Record the result with ${COMPLETE_STEP_TOOL_NAME} using self_check: true, then call ${UPDATE_GOAL_TOOL_NAME} status "complete" again.`
].join('\n')

type GoalBlockedAudit = {
  reason: string
  count: number
  requiredCount: number
  status: 'active' | 'blocked'
  message: string
}

type GoalCompletionAudit = {
  selfCheckRequired: boolean
  instructions: string
}

export function buildGoalLocalTools(threadService: ThreadService): LocalTool[] {
  return [
    createGetGoalTool(threadService),
    createCreateGoalTool(threadService),
    createCompleteStepTool(threadService),
    createRecordResearchDirectionTool(threadService),
    createUpdateGoalTool(threadService)
  ]
}

function createGetGoalTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: GET_GOAL_TOOL_NAME,
    description:
      'Get the current goal for this thread, including status, budgets, usage, and remaining token budget.',
    inputSchema: {
      type: 'object',
      properties: {},
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (_args, context) => {
      const goal = await threadService.getGoal(context.threadId)
      return { output: goalResponse(goal) }
    }
  })
}

function createCreateGoalTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: CREATE_GOAL_TOOL_NAME,
    description: [
      'Create a goal only when explicitly requested by the user or system/developer instructions;',
      'do not infer goals from ordinary tasks. Set token_budget only when an explicit token budget',
      `is requested. Fails if a goal exists; use ${UPDATE_GOAL_TOOL_NAME} only for status.`
    ].join(' '),
    inputSchema: {
      type: 'object',
      properties: {
        objective: {
          type: 'string',
          description:
            'Required. The concrete objective to start pursuing. This starts a new active goal only when no goal is currently defined.'
        },
        token_budget: {
          type: 'integer',
          description: 'Optional positive token budget for the new active goal.'
        },
        strict_completion: {
          type: 'boolean',
          description:
            'Optional. When true, completion requires a final self-check evidence entry after all todos and requirements are done.'
        }
      },
      required: ['objective'],
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (args, context) => {
      const objective = typeof args.objective === 'string' ? args.objective.trim() : ''
      const tokenBudget = normalizeTokenBudget(args.token_budget)
      const strictCompletion = args.strict_completion === true
      if (!objective) {
        return { output: { error: 'objective is required' }, isError: true }
      }
      if (tokenBudget === false) {
        return { output: { error: 'token_budget must be a positive integer' }, isError: true }
      }
      const existing = await threadService.getGoal(context.threadId)
      if (existing) {
        return {
          output: {
            error:
              'cannot create a new goal because this thread already has a goal; use update_goal only when the existing goal is complete'
          },
          isError: true
        }
      }
      const goal = await threadService.setGoal(context.threadId, {
        objective,
        status: 'active',
        ...(tokenBudget === undefined ? {} : { tokenBudget }),
        ...(strictCompletion ? { strictCompletion: true } : {})
      })
      return { output: goalResponse(goal) }
    }
  })
}

function createCompleteStepTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: COMPLETE_STEP_TOOL_NAME,
    description: [
      'Record a completed step for the active goal with concrete evidence.',
      `Call ${COMPLETE_STEP_TOOL_NAME} before ${UPDATE_GOAL_TOOL_NAME} status "complete";`,
      'the evidence ledger is required evidence that the goal is actually complete.'
    ].join(' '),
    inputSchema: {
      type: 'object',
      properties: {
        step: {
          type: 'string',
          description: 'Required. The completed requirement, step, or acceptance point.'
        },
        evidence: {
          type: 'array',
          minItems: 1,
          maxItems: 20,
          items: { type: 'string' },
          description:
            'Required. Concrete evidence such as tests, file paths, command names, output summaries, or reviewed state.'
        },
        requirement_id: {
          type: 'string',
          description:
            'Optional. For /goal --research goals, the requirement id from task_spec.md/progress.json that this evidence satisfies.'
        },
        summary: {
          type: 'string',
          description: 'Optional concise summary of what the evidence proves.'
        },
        self_check: {
          type: 'boolean',
          description:
            'Optional. Set true only when recording the final strict-completion self-check requested by update_goal.'
        }
      },
      required: ['step', 'evidence'],
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (args, context) => {
      const step = typeof args.step === 'string' ? args.step.trim() : ''
      const evidence = normalizeEvidence(args.evidence)
      const requirementId = typeof args.requirement_id === 'string'
        ? args.requirement_id.trim()
        : undefined
      const summary = typeof args.summary === 'string' ? args.summary.trim() : undefined
      const selfCheck = args.self_check === true
      if (!step) {
        return { output: { error: 'step is required' }, isError: true }
      }
      if (evidence === false) {
        return {
          output: { error: 'evidence must include at least one concrete non-empty item' },
          isError: true
        }
      }
      const existing = await threadService.getGoal(context.threadId)
      if (!existing) {
        return {
          output: { error: 'cannot record goal evidence because this thread does not have a goal' },
          isError: true
        }
      }
      if (existing.status !== 'active') {
        return {
          output: {
            error: `cannot record goal evidence because the current goal is ${existing.status}, not active`
          },
          isError: true
        }
      }
      if (selfCheck && (!existing.strictCompletion || !existing.selfCheckRequired)) {
        return {
          output: {
            error:
              'self_check evidence can only be recorded after update_goal requests a strict completion self-check'
          },
          isError: true
        }
      }
      let { goal, entry } = await threadService.appendGoalEvidence(context.threadId, {
        step,
        evidence,
        ...(summary ? { summary } : {}),
        ...(requirementId ? { requirementId } : {}),
        turnId: context.turnId,
        toolCallId: context.toolCallId
      })
      if (selfCheck) {
        goal = await threadService.setGoal(context.threadId, {
          selfCheckRequired: true,
          selfCheckCompleted: true,
          selfCheckTurnId: context.turnId
        })
      }
      return {
        output: {
          goal,
          entry,
          evidenceLedgerCount: goal.evidenceLedger?.length ?? 0,
          ...(selfCheck ? { selfCheckCompleted: true } : {})
        }
      }
    }
  })
}

function createRecordResearchDirectionTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: RECORD_RESEARCH_DIRECTION_TOOL_NAME,
    description: [
      'Record an AutoResearch direction tried for the active research goal.',
      'Use this for long-running /goal --research work so directions_tried.json and iteration_log.jsonl',
      'show what was attempted before final evidence is recorded.'
    ].join(' '),
    inputSchema: {
      type: 'object',
      properties: {
        direction: {
          type: 'string',
          description: 'Required. The research direction, question, source family, or hypothesis tried.'
        },
        outcome: {
          type: 'string',
          enum: ['tried', 'promising', 'dead_end'],
          description: 'Required. The observed outcome of this direction.'
        },
        summary: {
          type: 'string',
          description: 'Optional concise note about what this direction produced or ruled out.'
        }
      },
      required: ['direction', 'outcome'],
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (args, context) => {
      const direction = typeof args.direction === 'string' ? args.direction.trim() : ''
      const outcome = normalizeResearchDirectionOutcome(args.outcome)
      const summary = typeof args.summary === 'string' ? args.summary.trim() : undefined
      if (!direction) {
        return { output: { error: 'direction is required' }, isError: true }
      }
      if (!outcome) {
        return {
          output: { error: 'outcome must be one of tried, promising, or dead_end' },
          isError: true
        }
      }
      try {
        const result = await threadService.recordResearchDirection(context.threadId, {
          direction,
          outcome,
          ...(summary ? { summary } : {})
        })
        return { output: result }
      } catch (error) {
        return {
          output: { error: error instanceof Error ? error.message : String(error) },
          isError: true
        }
      }
    }
  })
}

function createUpdateGoalTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: UPDATE_GOAL_TOOL_NAME,
    description: [
      'Update the existing goal. Use this tool only to mark the goal achieved or blocked.',
      'Set status to complete only when the objective has actually been achieved and no required work remains.',
      `Before setting status complete, record at least one concrete ${COMPLETE_STEP_TOOL_NAME} evidence entry.`,
      'Set status to blocked only after the same blocking condition repeats for at least three consecutive goal turns; include reason every time.',
      'Do not mark a goal complete merely because the budget is nearly exhausted or because you are stopping work.',
      'You cannot use this tool to pause, resume, or budget-limit a goal; those status changes are controlled by the user or system.'
    ].join(' '),
    inputSchema: {
      type: 'object',
      properties: {
        status: {
          type: 'string',
          enum: ['complete', 'blocked'],
          description:
            'Required. Set to complete only when achieved; set to blocked only after repeated same-condition blocking.'
        },
        reason: {
          type: 'string',
          description:
            'Required when status is blocked. The specific external condition preventing progress; repeated same-condition reports are audited before the goal becomes blocked.'
        }
      },
      required: ['status'],
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (args, context) => {
      const status = args.status
      if (status !== 'complete' && status !== 'blocked') {
        return {
          output: {
            error:
              'update_goal can only mark the existing goal complete or blocked; pause, resume, budget-limited, and usage-limited status changes are controlled by the user or system'
          },
          isError: true
        }
      }
      const existing = await threadService.getGoal(context.threadId)
      if (!existing) {
        return {
          output: { error: 'cannot update goal because this thread does not have a goal' },
          isError: true
        }
      }
      if (status === 'blocked') {
        const reason = cleanBlockedReason(typeof args.reason === 'string' ? args.reason : '')
        if (!reason || !normalizeBlockedReason(reason)) {
          return {
            output: { error: 'reason is required when marking a goal blocked' },
            isError: true
          }
        }
        const sameReason = sameGoalBlock(existing.blockedReason, reason)
        const sameTurn = existing.blockedTurnId === context.turnId
        const priorCount = sameReason ? existing.blockedCount ?? 0 : 0
        const count = sameReason && sameTurn ? Math.max(1, priorCount) : priorCount + 1
        const nextStatus = count >= BLOCKED_AUDIT_REQUIRED_COUNT ? 'blocked' : 'active'
        const goal = await threadService.setGoal(context.threadId, {
          status: nextStatus,
          blockedReason: reason,
          blockedCount: count,
          blockedTurnId: context.turnId
        })
        return {
          output: goalResponse(goal, undefined, {
            reason,
            count,
            requiredCount: BLOCKED_AUDIT_REQUIRED_COUNT,
            status: nextStatus,
            message: nextStatus === 'blocked'
              ? 'Goal marked blocked after repeated same-condition reports.'
              : 'Goal remains active until the same blocking condition repeats across three goal turns.'
          })
        }
      }
      if (status === 'complete' && (existing.evidenceLedger?.length ?? 0) === 0) {
        return {
          output: {
            error: `cannot mark goal complete without evidence; call ${COMPLETE_STEP_TOOL_NAME} first`
          },
          isError: true
        }
      }
      if (status === 'complete' && existing.strictCompletion && !existing.selfCheckCompleted) {
        const readinessError = await completionReadinessError(threadService, context.threadId)
        if (readinessError && !/self-check/i.test(readinessError)) {
          return { output: { error: readinessError }, isError: true }
        }
        if (!existing.selfCheckRequired) {
          const goal = await threadService.setGoal(context.threadId, {
            selfCheckRequired: true,
            selfCheckTurnId: context.turnId
          })
          return {
            output: goalResponse(goal, undefined, undefined, {
              selfCheckRequired: true,
              instructions: STRICT_COMPLETION_SELF_CHECK_INSTRUCTIONS
            })
          }
        }
        return {
          output: {
            error:
              'cannot mark strict goal complete until a completion self-check has been recorded; call complete_step with self_check: true'
          },
          isError: true
        }
      }
      const goal = await threadService.setGoal(context.threadId, { status })
      return {
        output: goalResponse(
          goal,
          status === 'complete'
            ? 'Goal achieved. Report final usage from this tool result if relevant.'
            : undefined
        )
      }
    }
  })
}

function normalizeTokenBudget(value: unknown): number | undefined | false {
  if (value === undefined || value === null) return undefined
  if (typeof value !== 'number' || !Number.isInteger(value) || value <= 0) return false
  return value
}

function normalizeResearchDirectionOutcome(
  value: unknown
): 'tried' | 'promising' | 'dead_end' | null {
  return value === 'tried' || value === 'promising' || value === 'dead_end' ? value : null
}

function normalizeEvidence(value: unknown): string[] | false {
  if (!Array.isArray(value)) return false
  const evidence = value
    .map((item) => (typeof item === 'string' ? item.trim() : ''))
    .filter(Boolean)
    .slice(0, 20)
  return evidence.length > 0 ? evidence : false
}

function cleanBlockedReason(reason: string): string {
  return reason.trim().replace(/^[\s:,.!?;_\-[\]()"']+|[\s:,.!?;_\-[\]()"']+$/g, '')
}

function normalizeBlockedReason(reason: string): string {
  return cleanBlockedReason(reason)
    .toLocaleLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim()
    .replace(/\s+/g, ' ')
}

function sameGoalBlock(left: string | undefined, right: string): boolean {
  if (!left) return false
  const normalizedLeft = normalizeBlockedReason(left)
  return Boolean(normalizedLeft) && normalizedLeft === normalizeBlockedReason(right)
}

async function completionReadinessError(
  threadService: ThreadService,
  threadId: string
): Promise<string | null> {
  try {
    await threadService.setGoal(threadId, { status: 'complete' })
    return null
  } catch (error) {
    return error instanceof Error ? error.message : String(error)
  }
}

function goalResponse(
  goal: ThreadGoal | null,
  completionBudgetReport?: string,
  blockedAudit?: GoalBlockedAudit,
  completionAudit?: GoalCompletionAudit
): {
  goal: ThreadGoal | null
  remainingTokens: number | null
  completionBudgetReport?: string
  blockedAudit?: GoalBlockedAudit
  completionAudit?: GoalCompletionAudit
} {
  const remainingTokens =
    goal?.tokenBudget == null ? null : Math.max(0, goal.tokenBudget - goal.tokensUsed)
  return {
    goal,
    remainingTokens,
    ...(completionBudgetReport && goal?.status === 'complete'
      ? { completionBudgetReport }
      : {}),
    ...(blockedAudit ? { blockedAudit } : {}),
    ...(completionAudit ? { completionAudit } : {})
  }
}
