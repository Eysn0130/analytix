import type { RuntimeEvent } from '../contracts/events.js'
import type { CheckpointMetadata } from '../contracts/checkpoints.js'
import type { TurnItem } from '../contracts/items.js'
import type { ModelExecutionRef, ModelExecutionSource } from '../contracts/model-execution-ref.js'
import type { ModelReasoningEffort } from '../contracts/capabilities.js'
import type { UsageSnapshot } from '../contracts/usage.js'
import { emptyUsageSnapshot } from '../contracts/usage.js'
import { addUsage } from './usage.js'

export type EventSourcedTurnStatus = 'unknown' | 'running' | 'completed' | 'failed' | 'aborted'

export type EventSourcedTurnProjection = {
  id: string
  threadId: string
  status: EventSourcedTurnStatus
  startedAt?: string
  finishedAt?: string
  steering: string[]
  itemIds: string[]
}

export type EventSourcedChildRunProjection = {
  childId: string
  parentThreadId: string
  parentTurnId: string
  parentToolCallId?: string
  childRunId?: string
  childThreadId?: string
  childTurnId?: string
  label?: string
  name?: string
  status: 'queued' | 'running' | 'pause_requested' | 'paused' | 'resume_requested' | 'resuming' | 'completed' | 'failed' | 'aborted' | 'killed' | 'interrupted'
  seq?: number
  updatedAt: string
  text?: string
  /** Observability metrics carried on the child lifecycle events. */
  model?: string
  providerId?: string
  endpointFormat?: string
  variant?: string
  modelSource?: ModelExecutionSource
  modelExecution?: ModelExecutionRef
  effort?: ModelReasoningEffort
  profile?: string
  toolPolicy?: 'readOnly' | 'inherit'
  prefixReused?: boolean
  inheritedHistoryItems?: number
  toolInvocations?: number
  evidenceLedgered?: boolean
  durationMs?: number
  queuedMs?: number
  totalTokens?: number
  cacheHitRate?: number | null
  costUsd?: number
  costCny?: number
  background?: boolean
  parallelGroupId?: string
  parallelIndex?: number
}

export type EventSourcedRuntimeProjection = {
  threadId: string
  lastSeq: number
  startedAt?: string
  updatedAt?: string
  title?: string
  threadStatus?: string
  turns: EventSourcedTurnProjection[]
  items: TurnItem[]
  usage: UsageSnapshot
  childRuns: EventSourcedChildRunProjection[]
  checkpoints: CheckpointMetadata[]
  compactions: Array<{
    itemId?: string
    turnId?: string
    summary?: string
    replacedTokens?: number
    pinnedConstraints?: string[]
    sourceDigest?: string
    digestMarker?: string
    sourceItemIds?: string[]
  }>
  toolCatalog?: {
    fingerprint: string
    toolCount: number
    changeKind?: 'additive' | 'breaking'
    toolNames?: string[]
    message?: string
  }
  errors: Array<{
    seq: number
    turnId?: string
    itemId?: string
    message: string
    code?: string
    details?: unknown
    severity?: 'info' | 'warning' | 'error'
  }>
}

type MutableProjection = EventSourcedRuntimeProjection & {
  turns: EventSourcedTurnProjection[]
  items: TurnItem[]
  childRuns: EventSourcedChildRunProjection[]
  checkpoints: CheckpointMetadata[]
  errors: EventSourcedRuntimeProjection['errors']
  compactions: EventSourcedRuntimeProjection['compactions']
}

export function createRuntimeEventProjection(threadId = ''): EventSourcedRuntimeProjection {
  return {
    threadId,
    lastSeq: 0,
    turns: [],
    items: [],
    usage: emptyUsageSnapshot(),
    childRuns: [],
    checkpoints: [],
    compactions: [],
    errors: []
  }
}

export function replayRuntimeEvents(
  events: readonly RuntimeEvent[],
  initial: EventSourcedRuntimeProjection = createRuntimeEventProjection(events[0]?.threadId ?? '')
): EventSourcedRuntimeProjection {
  return [...events]
    .sort((a, b) => a.seq - b.seq)
    .reduce((projection, event) => applyRuntimeEvent(projection, event), initial)
}

export function applyRuntimeEvent(
  projection: EventSourcedRuntimeProjection,
  event: RuntimeEvent
): EventSourcedRuntimeProjection {
  if (event.seq <= projection.lastSeq) return projection
  const next: MutableProjection = cloneProjection(projection)
  next.threadId ||= event.threadId
  next.lastSeq = event.seq
  next.updatedAt = event.timestamp
  if ((event as unknown as { kind?: unknown }).kind === 'assistant_reasoning_delta') return next

  switch (event.kind) {
    case 'thread_created':
    case 'thread_updated':
      if (event.title) next.title = event.title
      if (event.status) next.threadStatus = event.status
      if (event.kind === 'thread_created') next.startedAt = next.startedAt ?? event.timestamp
      break
    case 'turn_started':
    case 'turn_completed':
    case 'turn_failed':
    case 'turn_aborted':
    case 'turn_steered':
      applyTurnEvent(next, event)
      break
    case 'item_created':
    case 'item_updated':
    case 'item_completed':
    case 'tool_call_started':
    case 'tool_call_finished':
      {
        const item = itemFromEvent(event)
        if (item) upsertItem(next, item, 'replace')
      }
      break
    case 'assistant_text_delta':
      {
        const item = itemFromEvent(event)
        if (item) upsertItem(next, item, 'append-delta')
      }
      break
    case 'approval_requested':
    case 'approval_resolved':
      upsertApprovalFromEvent(next, event)
      break
    case 'user_input_requested':
    case 'user_input_resolved':
      upsertUserInputFromEvent(next, event)
      break
    case 'compaction_started':
    case 'compaction_completed':
      applyCompactionEvent(next, event)
      break
    case 'tool_call_ready':
    case 'tool_progress':
    case 'tool_result_upload_wait':
    case 'tool_storm_suppressed':
    case 'mcp_lifecycle_audit':
    case 'autoresearch_state_audit':
    case 'pipeline_stage':
    case 'heartbeat':
    case 'goal_updated':
    case 'goal_cleared':
    case 'goal_evidence_audit':
    case 'todos_updated':
    case 'todos_cleared':
      break
    case 'tool_catalog_changed':
      next.toolCatalog = {
        fingerprint: event.fingerprint,
        toolCount: event.toolCount,
        ...(event.changeKind ? { changeKind: event.changeKind } : {}),
        ...(event.toolNames ? { toolNames: event.toolNames } : {}),
        ...(event.message ? { message: event.message } : {})
      }
      break
    case 'checkpoint_captured':
      upsertCheckpoint(next, event.checkpoint)
      break
    case 'checkpoint_rewind_rescue_created':
    case 'checkpoint_rewind_applied':
      break
    case 'usage':
      next.usage = addUsage(next.usage, event.usage)
      break
    case 'error':
      next.errors.push({
        seq: event.seq,
        ...(event.turnId ? { turnId: event.turnId } : {}),
        ...(event.itemId ? { itemId: event.itemId } : {}),
        message: event.message,
        ...(event.code ? { code: event.code } : {}),
        ...(event.details !== undefined ? { details: event.details } : {}),
        ...(event.severity ? { severity: event.severity } : {})
      })
      if (event.itemId && event.turnId) {
        upsertItem(next, {
          id: event.itemId,
          turnId: event.turnId,
          threadId: event.threadId,
          role: 'system',
          status: 'failed',
          createdAt: event.timestamp,
          finishedAt: event.timestamp,
          kind: 'error',
          message: event.message,
          ...(event.code ? { code: event.code } : {}),
          ...(event.details !== undefined ? { details: event.details } : {}),
          ...(event.severity ? { severity: event.severity } : {})
        }, 'replace')
      }
      break
  }
  return freezeProjection(next)
}

function upsertCheckpoint(
  projection: MutableProjection,
  checkpoint: CheckpointMetadata
): void {
  const existingIndex = projection.checkpoints.findIndex((item) => item.checkpointId === checkpoint.checkpointId)
  if (existingIndex >= 0) projection.checkpoints[existingIndex] = checkpoint
  else projection.checkpoints.push(checkpoint)
}

function applyTurnEvent(
  projection: MutableProjection,
  event: Extract<RuntimeEvent, { kind: 'turn_started' | 'turn_completed' | 'turn_failed' | 'turn_aborted' | 'turn_steered' }>
): void {
  if (event.child) {
    upsertChildRun(projection, event)
    return
  }
  if (!event.turnId) return
  const turn = ensureTurn(projection, event.threadId, event.turnId)
  switch (event.kind) {
    case 'turn_started':
      turn.status = 'running'
      turn.startedAt = turn.startedAt ?? event.timestamp
      break
    case 'turn_completed':
      turn.status = 'completed'
      turn.finishedAt = event.timestamp
      break
    case 'turn_failed':
      turn.status = 'failed'
      turn.finishedAt = event.timestamp
      break
    case 'turn_aborted':
      turn.status = 'aborted'
      turn.finishedAt = event.timestamp
      break
    case 'turn_steered':
      if (event.text) turn.steering.push(event.text)
      break
  }
}

function upsertChildRun(
  projection: MutableProjection,
  event: Extract<RuntimeEvent, { kind: 'turn_started' | 'turn_completed' | 'turn_failed' | 'turn_aborted' | 'turn_steered' }>
): void {
  const child = event.child
  if (!child) return
  const key = childEventProjectionKey(child)
  const existingIndex = projection.childRuns.findIndex((run) => childRunProjectionMatches(run, child, key))
  const effort = child.childEffort ?? child.effort
  const next: EventSourcedChildRunProjection = {
    childId: child.childId,
    parentThreadId: child.parentThreadId,
    parentTurnId: child.parentTurnId,
    ...(child.parentToolCallId ? { parentToolCallId: child.parentToolCallId } : {}),
    ...(child.childRunId ? { childRunId: child.childRunId } : {}),
    ...(child.childThreadId ? { childThreadId: child.childThreadId } : {}),
    ...(child.childTurnId ? { childTurnId: child.childTurnId } : {}),
    ...(child.childLabel ? { label: child.childLabel } : {}),
    ...(child.childName ? { name: child.childName } : {}),
    status: child.childStatus,
    seq: child.childSeq,
    updatedAt: event.timestamp,
    ...(event.text ? { text: event.text } : {}),
    ...(child.childModel ? { model: child.childModel } : {}),
    ...(child.childProviderId ? { providerId: child.childProviderId } : {}),
    ...(child.childEndpointFormat ? { endpointFormat: child.childEndpointFormat } : {}),
    ...(child.childModelVariant ? { variant: child.childModelVariant } : {}),
    ...(child.childModelSource ? { modelSource: child.childModelSource } : {}),
    ...(child.childModelExecution ? { modelExecution: child.childModelExecution } : {}),
    ...(effort ? { effort } : {}),
    ...(child.childProfile ? { profile: child.childProfile } : {}),
    ...(child.childToolPolicy ? { toolPolicy: child.childToolPolicy } : {}),
    ...(child.prefixReused !== undefined ? { prefixReused: child.prefixReused } : {}),
    ...(child.inheritedHistoryItems !== undefined ? { inheritedHistoryItems: child.inheritedHistoryItems } : {}),
    ...(child.toolInvocations !== undefined ? { toolInvocations: child.toolInvocations } : {}),
    ...(child.evidenceLedgered !== undefined ? { evidenceLedgered: child.evidenceLedgered } : {}),
    ...(child.durationMs !== undefined ? { durationMs: child.durationMs } : {}),
    ...(child.queuedMs !== undefined ? { queuedMs: child.queuedMs } : {}),
    ...(child.totalTokens !== undefined ? { totalTokens: child.totalTokens } : {}),
    ...(child.cacheHitRate !== undefined ? { cacheHitRate: child.cacheHitRate } : {}),
    ...(child.costUsd !== undefined ? { costUsd: child.costUsd } : {}),
    ...(child.costCny !== undefined ? { costCny: child.costCny } : {}),
    ...(child.background !== undefined ? { background: child.background } : {}),
    ...(child.parallelGroupId ? { parallelGroupId: child.parallelGroupId } : {}),
    ...(child.parallelIndex !== undefined ? { parallelIndex: child.parallelIndex } : {})
  }
  if (existingIndex >= 0) projection.childRuns[existingIndex] = next
  else projection.childRuns.push(next)
}

function childEventProjectionKey(child: NonNullable<RuntimeEvent['child']>): string {
  if (child.childRunId) return `run:${child.childRunId}`
  if (child.childThreadId) return `thread:${child.childThreadId}`
  if (child.parentToolCallId) return `call:${child.parentToolCallId}`
  return `child:${child.childId}`
}

function childRunProjectionKey(run: EventSourcedChildRunProjection): string {
  if (run.childRunId) return `run:${run.childRunId}`
  if (run.childThreadId) return `thread:${run.childThreadId}`
  if (run.parentToolCallId) return `call:${run.parentToolCallId}`
  return `child:${run.childId}`
}

function childRunProjectionMatches(
  run: EventSourcedChildRunProjection,
  child: NonNullable<RuntimeEvent['child']>,
  key: string
): boolean {
  if (childRunProjectionKey(run) === key) return true
  if (run.childId === child.childId) return true
  if (child.childRunId && run.childRunId === child.childRunId) return true
  if (child.childThreadId && run.childThreadId === child.childThreadId) return true
  const hasStableIdentity = Boolean(run.childRunId || run.childThreadId || child.childRunId || child.childThreadId)
  return Boolean(!hasStableIdentity && child.parentToolCallId && run.parentToolCallId === child.parentToolCallId)
}

function applyCompactionEvent(
  projection: MutableProjection,
  event: Extract<RuntimeEvent, { kind: 'compaction_started' | 'compaction_completed' }>
): void {
  projection.compactions.push({
    ...(event.itemId ? { itemId: event.itemId } : {}),
    ...(event.turnId ? { turnId: event.turnId } : {}),
    ...(event.summary ? { summary: event.summary } : {}),
    ...(event.replacedTokens !== undefined ? { replacedTokens: event.replacedTokens } : {}),
    ...(event.pinnedConstraints ? { pinnedConstraints: event.pinnedConstraints } : {}),
    ...(event.sourceDigest ? { sourceDigest: event.sourceDigest } : {}),
    ...(event.digestMarker ? { digestMarker: event.digestMarker } : {}),
    ...(event.sourceItemIds ? { sourceItemIds: event.sourceItemIds } : {})
  })
  if (event.kind !== 'compaction_completed' || !event.itemId || !event.turnId || !event.replacedTokens) return
  upsertItem(projection, {
    id: event.itemId,
    turnId: event.turnId,
    threadId: event.threadId,
    role: 'system',
    status: 'completed',
    createdAt: event.timestamp,
    finishedAt: event.timestamp,
    kind: 'compaction',
    summary: event.summary ?? '',
    replacedTokens: event.replacedTokens,
    pinnedConstraints: event.pinnedConstraints ?? [],
    ...(event.sourceDigest ? { sourceDigest: event.sourceDigest } : {}),
    ...(event.digestMarker ? { digestMarker: event.digestMarker } : {}),
    ...(event.sourceItemIds ? { sourceItemIds: event.sourceItemIds } : {})
  }, 'replace')
}

function upsertApprovalFromEvent(
  projection: MutableProjection,
  event: Extract<RuntimeEvent, { kind: 'approval_requested' | 'approval_resolved' }>
): void {
  if (!event.turnId) return
  const existing = projection.items.find((item) => item.kind === 'approval' && item.approvalId === event.approvalId)
  const status = event.status
  const item: TurnItem = existing?.kind === 'approval'
    ? { ...existing, status, ...(status !== 'pending' ? { finishedAt: event.timestamp } : {}) }
    : {
        id: event.itemId ?? `item_${event.approvalId}`,
        turnId: event.turnId,
        threadId: event.threadId,
        role: 'tool',
        createdAt: event.timestamp,
        kind: 'approval',
        approvalId: event.approvalId,
        toolName: event.toolName,
        summary: event.summary ?? '',
        status,
        ...(status !== 'pending' ? { finishedAt: event.timestamp } : {})
      }
  upsertItem(projection, item, 'replace')
}

function upsertUserInputFromEvent(
  projection: MutableProjection,
  event: Extract<RuntimeEvent, { kind: 'user_input_requested' | 'user_input_resolved' }>
): void {
  if (!event.turnId) return
  const existing = projection.items.find((item) => item.kind === 'user_input' && item.inputId === event.inputId)
  const status = event.status
  const item: TurnItem = existing?.kind === 'user_input'
    ? { ...existing, status, ...(status !== 'pending' ? { finishedAt: event.timestamp } : {}) }
    : {
        id: event.itemId ?? `item_${event.inputId}`,
        turnId: event.turnId,
        threadId: event.threadId,
        role: 'tool',
        createdAt: event.timestamp,
        kind: 'user_input',
        inputId: event.inputId,
        prompt: event.prompt ?? '',
        questions: event.questions ?? [],
        status,
        ...(status !== 'pending' ? { finishedAt: event.timestamp } : {})
      }
  upsertItem(projection, item, 'replace')
}

function upsertItem(
  projection: MutableProjection,
  item: TurnItem,
  mode: 'replace' | 'append-delta'
): void {
  const index = projection.items.findIndex((candidate) => candidate.id === item.id)
  const nextItem = index >= 0 && mode === 'append-delta'
    ? appendDelta(projection.items[index]!, item)
    : item
  if (index >= 0) projection.items[index] = nextItem
  else projection.items.push(nextItem)
  const turn = ensureTurn(projection, item.threadId, item.turnId)
  if (!turn.itemIds.includes(item.id)) turn.itemIds.push(item.id)
}

function itemFromEvent(event: RuntimeEvent): TurnItem | undefined {
  const eventWithItem = event as RuntimeEvent & { item?: TurnItem; itemId?: string; text?: unknown; signature?: string }
  if (eventWithItem.item) {
    if ((eventWithItem.item as unknown as { kind?: unknown }).kind === 'assistant_reasoning') return undefined
    return eventWithItem.item
  }
  if (event.kind !== 'assistant_text_delta') return undefined
  const text = eventWithItem.text
  if (typeof text !== 'string') return undefined
  const turnId = (event as RuntimeEvent & { turnId?: string }).turnId
  if (!turnId) return undefined
  const id = eventWithItem.itemId ?? `item_${turnId}_assistant`
  return {
    id,
    turnId,
    threadId: event.threadId,
    role: 'assistant',
    status: 'running',
    createdAt: event.timestamp,
    kind: 'assistant_text',
    text
  } as TurnItem
}

function appendDelta(existing: TurnItem, delta: TurnItem): TurnItem {
  if (existing.kind === 'assistant_text' && delta.kind === 'assistant_text') {
    return {
      ...existing,
      text: `${existing.text}${delta.text}`,
      status: delta.status,
      finishedAt: delta.finishedAt ?? existing.finishedAt
    }
  }
  return delta
}

function ensureTurn(
  projection: MutableProjection,
  threadId: string,
  turnId: string
): EventSourcedTurnProjection {
  let turn = projection.turns.find((candidate) => candidate.id === turnId)
  if (!turn) {
    turn = {
      id: turnId,
      threadId,
      status: 'unknown',
      steering: [],
      itemIds: []
    }
    projection.turns.push(turn)
  }
  return turn
}

function cloneProjection(projection: EventSourcedRuntimeProjection): MutableProjection {
  return {
    ...projection,
    turns: projection.turns.map((turn) => ({
      ...turn,
      steering: [...turn.steering],
      itemIds: [...turn.itemIds]
    })),
    items: projection.items.map((item) => ({ ...item }) as TurnItem),
    usage: { ...projection.usage },
    childRuns: projection.childRuns.map((run) => ({ ...run })),
    checkpoints: projection.checkpoints.map((checkpoint) => ({
      ...checkpoint,
      changedFiles: checkpoint.changedFiles.map((file) => ({ ...file }))
    })),
    compactions: projection.compactions.map((compaction) => ({ ...compaction })),
    errors: projection.errors.map((error) => ({ ...error })),
    ...(projection.toolCatalog ? { toolCatalog: { ...projection.toolCatalog } } : {})
  }
}

function freezeProjection(projection: MutableProjection): EventSourcedRuntimeProjection {
  return {
    ...projection,
    turns: projection.turns.map((turn) => ({
      ...turn,
      steering: [...turn.steering],
      itemIds: [...turn.itemIds]
    })),
    items: [...projection.items],
    childRuns: [...projection.childRuns],
    checkpoints: projection.checkpoints.map((checkpoint) => ({
      ...checkpoint,
      changedFiles: checkpoint.changedFiles.map((file) => ({ ...file }))
    })),
    compactions: [...projection.compactions],
    errors: [...projection.errors]
  }
}
