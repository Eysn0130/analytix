import type {
  ThreadMode,
  ThreadRecord,
  ThreadGoal,
  ThreadTodoList,
  ThreadRuntimeStepLimits,
  ThreadRelation,
  ThreadStatus
} from '../contracts/threads.js'
import {
  CURRENT_EXECUTION_POLICY_VERSION,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  type ApprovalPolicy,
  type SandboxMode
} from '../contracts/policy.js'
import type { TurnItem } from '../contracts/items.js'

/**
 * Domain helper for thread records. The contract type is the source of
 * truth; this module only adds small factory/utility helpers so the
 * services and stores can stay free of date-string formatting.
 */
export type ThreadEntity = ThreadRecord

export function createThreadRecord(input: {
  id: string
  title: string
  workspace: string
  model: string
  providerId?: string
  mode?: ThreadMode
  status?: ThreadStatus
  approvalPolicy?: ApprovalPolicy
  sandboxMode?: SandboxMode
  costBudgetUsd?: number
  costBudgetWarningSent?: boolean
  relation?: ThreadRelation
  parentThreadId?: string
  forkedFromThreadId?: string
  forkedFromTitle?: string
  forkedAt?: string
  forkedFromMessageCount?: number
  forkedFromTurnCount?: number
  runtimeStepLimits?: ThreadRuntimeStepLimits
  goal?: ThreadGoal
  todos?: ThreadTodoList
  createdAt?: string
}): ThreadEntity {
  const now = input.createdAt ?? new Date().toISOString()
  return {
    id: input.id,
    title: input.title,
    workspace: input.workspace,
    model: input.model,
    ...(input.providerId?.trim() ? { providerId: input.providerId.trim() } : {}),
    mode: input.mode ?? 'agent',
    status: input.status ?? 'idle',
    executionPolicyVersion: CURRENT_EXECUTION_POLICY_VERSION,
    approvalPolicy: input.approvalPolicy ?? DEFAULT_APPROVAL_POLICY,
    sandboxMode: input.sandboxMode ?? DEFAULT_SANDBOX_MODE,
    ...(input.costBudgetUsd !== undefined ? { costBudgetUsd: input.costBudgetUsd } : {}),
    ...(input.costBudgetWarningSent !== undefined ? { costBudgetWarningSent: input.costBudgetWarningSent } : {}),
    relation: input.relation ?? 'primary',
    ...(input.parentThreadId ? { parentThreadId: input.parentThreadId } : {}),
    ...(input.forkedFromThreadId ? { forkedFromThreadId: input.forkedFromThreadId } : {}),
    ...(input.forkedFromTitle ? { forkedFromTitle: input.forkedFromTitle } : {}),
    ...(input.forkedAt ? { forkedAt: input.forkedAt } : {}),
    ...(input.forkedFromMessageCount !== undefined ? { forkedFromMessageCount: input.forkedFromMessageCount } : {}),
    ...(input.forkedFromTurnCount !== undefined ? { forkedFromTurnCount: input.forkedFromTurnCount } : {}),
    ...(input.runtimeStepLimits ? { runtimeStepLimits: input.runtimeStepLimits } : {}),
    ...(input.goal ? { goal: input.goal } : {}),
    ...(input.todos ? { todos: input.todos } : {}),
    createdAt: now,
    updatedAt: now,
    turns: []
  }
}

export function touchThread(thread: ThreadEntity, updatedAt?: string): ThreadEntity {
  return { ...thread, updatedAt: updatedAt ?? new Date().toISOString() }
}

export function toThreadSummary(
  thread: ThreadEntity
): Pick<
  ThreadEntity,
  'id' | 'title' | 'workspace' | 'model' | 'mode' | 'status' | 'executionPolicyVersion' | 'approvalPolicy' | 'sandboxMode' | 'createdAt' | 'updatedAt'
  | 'providerId' | 'costBudgetUsd' | 'costBudgetWarningSent'
  | 'relation' | 'parentThreadId'
  | 'forkedFromThreadId' | 'forkedFromTitle' | 'forkedAt' | 'forkedFromMessageCount' | 'forkedFromTurnCount'
  | 'runtimeStepLimits'
  | 'goal' | 'todos'
> & { preview?: string; messageCount: number; turnCount: number } {
  const items = thread.turns.flatMap((turn) => turn.items)
  const preview = previewFromItems(items)
  return {
    id: thread.id,
    title: thread.title,
    workspace: thread.workspace,
    model: thread.model,
    ...(thread.providerId ? { providerId: thread.providerId } : {}),
    mode: thread.mode,
    status: thread.status,
    ...(thread.executionPolicyVersion !== undefined ? { executionPolicyVersion: thread.executionPolicyVersion } : {}),
    approvalPolicy: thread.approvalPolicy,
    sandboxMode: thread.sandboxMode,
    ...(thread.costBudgetUsd !== undefined ? { costBudgetUsd: thread.costBudgetUsd } : {}),
    ...(thread.costBudgetWarningSent !== undefined ? { costBudgetWarningSent: thread.costBudgetWarningSent } : {}),
    relation: thread.relation ?? 'primary',
    ...(thread.parentThreadId ? { parentThreadId: thread.parentThreadId } : {}),
    ...(thread.forkedFromThreadId ? { forkedFromThreadId: thread.forkedFromThreadId } : {}),
    ...(thread.forkedFromTitle ? { forkedFromTitle: thread.forkedFromTitle } : {}),
    ...(thread.forkedAt ? { forkedAt: thread.forkedAt } : {}),
    ...(thread.forkedFromMessageCount !== undefined ? { forkedFromMessageCount: thread.forkedFromMessageCount } : {}),
    ...(thread.forkedFromTurnCount !== undefined ? { forkedFromTurnCount: thread.forkedFromTurnCount } : {}),
    ...(thread.runtimeStepLimits ? { runtimeStepLimits: thread.runtimeStepLimits } : {}),
    ...(thread.goal ? { goal: thread.goal } : {}),
    ...(thread.todos ? { todos: thread.todos } : {}),
    ...(preview ? { preview } : {}),
    messageCount: items.length,
    turnCount: thread.turns.length,
    createdAt: thread.createdAt,
    updatedAt: thread.updatedAt
  }
}

function previewFromItems(items: TurnItem[]): string {
  const firstUser = items.find((item): item is Extract<TurnItem, { kind: 'user_message' }> =>
    item.kind === 'user_message'
  )
  if (firstUser) return firstUser.text.slice(0, 500)
  const firstAssistant = items.find((item): item is Extract<TurnItem, { kind: 'assistant_text' }> =>
    item.kind === 'assistant_text'
  )
  if (firstAssistant) return firstAssistant.text.slice(0, 500)
  const firstError = items.find((item): item is Extract<TurnItem, { kind: 'error' }> =>
    item.kind === 'error'
  )
  if (firstError) return firstError.message.slice(0, 500)
  const firstTool = items.find((item): item is Extract<TurnItem, { kind: 'tool_call' }> =>
    item.kind === 'tool_call'
  )
  return firstTool ? firstTool.toolName.slice(0, 500) : ''
}
