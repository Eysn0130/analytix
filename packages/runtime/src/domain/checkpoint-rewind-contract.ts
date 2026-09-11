import { createHash } from 'node:crypto'
import { isAbsolute, relative, resolve, sep } from 'node:path'
import type { RuntimeEvent } from '../contracts/events.js'
import {
  ANALYTIX_CHECKPOINT_ID_PREFIX,
  ANALYTIX_CHECKPOINT_SCHEMA_VERSION,
  ANALYTIX_REWIND_PLAN_ID_PREFIX,
  ANALYTIX_REWIND_PLAN_SCHEMA_VERSION,
  CheckpointRewindPlanSchema,
  CheckpointMetadataSchema,
  type CheckpointChangeKind,
  type CheckpointChangedFile,
  type CheckpointMetadata,
  type CheckpointRewindPlan,
  type CheckpointRewindScope,
  type CheckpointRestoreFileAction,
  type CheckpointRestoreFilePlan,
  type CheckpointRestoreFileRisk,
  type CheckpointRestoreFileStatus,
  type CheckpointStatus
} from '../contracts/checkpoints.js'
import {
  replayRuntimeEvents,
  type EventSourcedRuntimeProjection
} from './runtime-event-reducer.js'

export type CheckpointChangedFileInput = {
  path?: string
  relativePath?: string
  changeKind?: CheckpointChangeKind
  beforeHash?: string | null
  afterHash?: string | null
}

// Raw tool output is a request-local host input. It is intentionally separate
// from the public TurnItem contract, whose tool_result output is metadata-only.
export type PrivateCheckpointToolResultInput = {
  kind: 'tool_result'
  toolKind: string
  isError: boolean
  output: unknown
}

export type CreateAnalytixCheckpointMetadataInput = {
  checkpointId?: string
  threadId: string
  turnId: string
  workspace: string
  createdAt?: string
  status?: CheckpointStatus
  changedFiles: readonly CheckpointChangedFileInput[]
}

export type ConversationOnlyRewindContract =
  | {
      ok: true
      scope: 'conversation'
      checkpoint: CheckpointMetadata
      retainedEvents: RuntimeEvent[]
      removedEvents: RuntimeEvent[]
      removedTurnIds: string[]
      projection: EventSourcedRuntimeProjection
    }
  | {
      ok: false
      scope: 'conversation'
      reason: 'checkpoint_not_found' | 'boundary_not_found'
      message: string
    }

export type CheckpointPathRisk = {
  exists?: boolean
  isSymlink?: boolean
}

export type AuditableCheckpointRewindPlanInput = {
  events: readonly RuntimeEvent[]
  checkpointId: string
  scope?: CheckpointRewindScope
  createdAt?: string
  pathRisks?: ReadonlyMap<string, CheckpointPathRisk>
}

export type AuditableCheckpointRewindPlanResult =
  | {
      ok: true
      plan: CheckpointRewindPlan
    }
  | {
      ok: false
      reason: 'checkpoint_not_found' | 'boundary_not_found'
      message: string
    }

export function createAnalytixCheckpointMetadata(
  input: CreateAnalytixCheckpointMetadataInput
): CheckpointMetadata {
  const createdAt = input.createdAt ?? new Date().toISOString()
  const changedFiles = normalizeChangedFiles(input.workspace, input.changedFiles)
  const checkpointId = input.checkpointId ?? createCheckpointId({
    threadId: input.threadId,
    turnId: input.turnId,
    workspace: input.workspace,
    createdAt,
    changedFiles
  })

  return CheckpointMetadataSchema.parse({
    schemaVersion: ANALYTIX_CHECKPOINT_SCHEMA_VERSION,
    checkpointId,
    threadId: input.threadId,
    turnId: input.turnId,
    workspace: input.workspace,
    createdAt,
    status: input.status ?? 'captured',
    changedFiles
  })
}

export function extractCheckpointChangedFilesFromItems(
  items: readonly PrivateCheckpointToolResultInput[],
  workspace: string
): CheckpointChangedFile[] {
  const changedFiles: CheckpointChangedFileInput[] = []
  for (const item of items) {
    if (item.kind !== 'tool_result' || item.toolKind !== 'file_change' || item.isError) continue
    const changedFile = changedFileInputFromToolResult(item.output)
    if (changedFile) changedFiles.push(changedFile)
  }
  return normalizeChangedFiles(workspace, changedFiles)
}

export function planConversationOnlyRewindFromEvents(
  events: readonly RuntimeEvent[],
  checkpointId: string
): ConversationOnlyRewindContract {
  const ordered = [...events].sort((a, b) => a.seq - b.seq)
  const checkpointEvent = ordered.find(
    (event) => event.kind === 'checkpoint_captured' && event.checkpoint.checkpointId === checkpointId
  )
  if (!checkpointEvent || checkpointEvent.kind !== 'checkpoint_captured') {
    return {
      ok: false,
      scope: 'conversation',
      reason: 'checkpoint_not_found',
      message: `checkpoint not found: ${checkpointId}`
    }
  }

  const boundary = ordered.find(
    (event) => event.kind === 'turn_started' && event.turnId === checkpointEvent.checkpoint.turnId
  )
  if (!boundary) {
    return {
      ok: false,
      scope: 'conversation',
      reason: 'boundary_not_found',
      message: `turn boundary not found for checkpoint: ${checkpointId}`
    }
  }

  const retainedEvents = ordered.filter((event) => event.seq < boundary.seq)
  const removedEvents = ordered.filter((event) => event.seq >= boundary.seq)
  const removedTurnIds = [...new Set(
    removedEvents
      .map((event) => event.turnId)
      .filter((turnId): turnId is string => typeof turnId === 'string' && turnId.length > 0)
  )]

  return {
    ok: true,
    scope: 'conversation',
    checkpoint: checkpointEvent.checkpoint,
    retainedEvents,
    removedEvents,
    removedTurnIds,
    projection: replayRuntimeEvents(retainedEvents)
  }
}

export function findCheckpointInEvents(
  events: readonly RuntimeEvent[],
  checkpointId: string
): CheckpointMetadata | null {
  const event = [...events]
    .sort((a, b) => b.seq - a.seq)
    .find((candidate) => (
      candidate.kind === 'checkpoint_captured' &&
      candidate.checkpoint.checkpointId === checkpointId
    ))
  return event?.kind === 'checkpoint_captured' ? event.checkpoint : null
}

export function buildAuditableCheckpointRewindPlan(
  input: AuditableCheckpointRewindPlanInput
): AuditableCheckpointRewindPlanResult {
  const scope = input.scope ?? 'combined'
  const checkpoint = findCheckpointInEvents(input.events, input.checkpointId)
  if (!checkpoint) {
    return {
      ok: false,
      reason: 'checkpoint_not_found',
      message: `checkpoint not found: ${input.checkpointId}`
    }
  }

  const conversation = scope === 'code'
    ? undefined
    : summarizeConversationRewind(input.events, input.checkpointId)
  if (conversation && conversation.status === 'blocked') {
    return {
      ok: false,
      reason: 'boundary_not_found',
      message: conversation.reason ?? `turn boundary not found for checkpoint: ${input.checkpointId}`
    }
  }

  const files = scope === 'conversation'
    ? []
    : checkpoint.changedFiles.map((file) => restoreFilePlanForCheckpointFile({
        workspace: checkpoint.workspace,
        file,
        risk: input.pathRisks?.get(file.relativePath)
      }))
  const summary = summarizeRewindPlanFiles(files, conversation)
  const createdAt = input.createdAt ?? new Date().toISOString()
  const planWithoutDigest = {
    schemaVersion: ANALYTIX_REWIND_PLAN_SCHEMA_VERSION,
    planId: createRewindPlanId({
      checkpointId: checkpoint.checkpointId,
      scope,
      createdAt,
      files
    }),
    checkpointId: checkpoint.checkpointId,
    threadId: checkpoint.threadId,
    workspace: checkpoint.workspace,
    createdAt,
    scope,
    applyMode: 'plan_only',
    destructive: false,
    checkpoint,
    files,
    ...(conversation ? { conversation } : {}),
    summary
  }
  const plan = CheckpointRewindPlanSchema.parse({
    ...planWithoutDigest,
    planDigest: createHash('sha256')
      .update(JSON.stringify(canonicalJsonValue(planWithoutDigest)))
      .digest('hex')
  })
  return { ok: true, plan }
}

function canonicalJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalJsonValue)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .filter(([, entry]) => entry !== undefined)
      .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
      .map(([key, entry]) => [key, canonicalJsonValue(entry)])
  )
}

function normalizeChangedFiles(
  workspace: string,
  files: readonly CheckpointChangedFileInput[]
): CheckpointChangedFile[] {
  const byRelativePath = new Map<string, CheckpointChangedFile>()
  for (const file of files) {
    const relativePath = normalizeRelativePath(workspace, file.relativePath ?? file.path ?? '')
    if (byRelativePath.has(relativePath)) continue
    byRelativePath.set(relativePath, {
      relativePath,
      changeKind: file.changeKind ?? 'unknown',
      ...(file.beforeHash !== undefined ? { beforeHash: file.beforeHash } : {}),
      ...(file.afterHash !== undefined ? { afterHash: file.afterHash } : {})
    })
  }
  return [...byRelativePath.values()]
}

function normalizeRelativePath(workspace: string, rawPath: string): string {
  const root = resolve(workspace)
  const trimmed = rawPath.trim()
  if (!trimmed) throw new Error('checkpoint changed file path is required')
  const absolutePath = isAbsolute(trimmed) ? resolve(trimmed) : resolve(root, trimmed)
  const relativePath = relative(root, absolutePath)
  if (
    !relativePath ||
    relativePath === '..' ||
    relativePath.startsWith(`..${sep}`) ||
    isAbsolute(relativePath)
  ) {
    throw new Error(`checkpoint changed file escapes workspace: ${rawPath}`)
  }
  return relativePath.split(sep).join('/')
}

function restoreFilePlanForCheckpointFile(input: {
  workspace: string
  file: CheckpointChangedFile
  risk?: CheckpointPathRisk
}): CheckpointRestoreFilePlan {
  const safePath = safeRestoreRelativePath(input.workspace, input.file.relativePath)
  if (!safePath.ok) {
    return filePlan({
      relativePath: input.file.relativePath,
      file: input.file,
      action: 'blocked',
      status: 'blocked',
      risk: 'high',
      reason: safePath.reason
    })
  }

  if (input.risk?.isSymlink) {
    return filePlan({
      relativePath: safePath.relativePath,
      file: input.file,
      action: 'blocked',
      status: 'blocked',
      risk: 'high',
      reason: 'current workspace path is a symlink; destructive restore requires explicit P4C review'
    })
  }

  if (input.risk?.exists === false && input.file.changeKind === 'modified') {
    return filePlan({
      relativePath: safePath.relativePath,
      file: input.file,
      action: 'manual_review',
      status: 'manual_review',
      risk: 'medium',
      reason: 'current workspace path is missing; snapshot-backed restore must be reviewed before apply'
    })
  }

  switch (input.file.changeKind) {
    case 'created':
      if (input.risk?.exists === false) {
        return filePlan({
          relativePath: safePath.relativePath,
          file: input.file,
          action: 'noop',
          status: 'ready',
          risk: 'low',
          reason: 'created file is already absent; no destructive file change is needed'
        })
      }
      return filePlan({
        relativePath: safePath.relativePath,
        file: input.file,
        action: 'delete_created_file',
        status: 'ready',
        risk: 'medium',
        reason: 'checkpoint says this file was created after the boundary; delete is only planned for review'
      })
    case 'modified':
      return filePlan({
        relativePath: safePath.relativePath,
        file: input.file,
        action: 'restore_previous_version',
        status: input.file.beforeHash ? 'ready' : 'manual_review',
        risk: input.file.beforeHash ? 'medium' : 'high',
        reason: input.file.beforeHash
          ? 'checkpoint has before/after metadata; content restore remains plan-only until P4C snapshot apply'
          : 'checkpoint lacks beforeHash; content restore requires manual review'
      })
    case 'deleted':
      return filePlan({
        relativePath: safePath.relativePath,
        file: input.file,
        action: 'restore_deleted_file',
        status: input.file.beforeHash ? 'ready' : 'manual_review',
        risk: input.file.beforeHash ? 'medium' : 'high',
        reason: input.file.beforeHash
          ? 'checkpoint has deleted-file metadata; content restore remains plan-only until P4C snapshot apply'
          : 'checkpoint lacks beforeHash for deleted file; restore requires manual review'
      })
    default:
      return filePlan({
        relativePath: safePath.relativePath,
        file: input.file,
        action: 'manual_review',
        status: 'manual_review',
        risk: 'high',
        reason: 'checkpoint change kind is unknown; destructive restore is blocked until reviewed'
      })
  }
}

function safeRestoreRelativePath(
  workspace: string,
  rawPath: string
): { ok: true; relativePath: string } | { ok: false; reason: string } {
  const trimmed = rawPath.trim()
  if (!trimmed) return { ok: false, reason: 'checkpoint changed file path is required' }
  if (isAbsolute(trimmed)) {
    return { ok: false, reason: 'checkpoint changed file path must be workspace-relative for restore planning' }
  }
  try {
    return { ok: true, relativePath: normalizeRelativePath(workspace, trimmed) }
  } catch (error) {
    return {
      ok: false,
      reason: error instanceof Error ? error.message : 'checkpoint changed file escapes workspace'
    }
  }
}

function filePlan(input: {
  relativePath: string
  file: CheckpointChangedFile
  action: CheckpointRestoreFileAction
  status: CheckpointRestoreFileStatus
  risk: CheckpointRestoreFileRisk
  reason: string
}): CheckpointRestoreFilePlan {
  return {
    relativePath: input.relativePath,
    changeKind: input.file.changeKind,
    action: input.action,
    status: input.status,
    risk: input.risk,
    reason: input.reason,
    contentSource: 'checkpoint_metadata_only',
    ...(input.file.beforeHash !== undefined ? { beforeHash: input.file.beforeHash } : {}),
    ...(input.file.afterHash !== undefined ? { afterHash: input.file.afterHash } : {})
  }
}

function summarizeConversationRewind(
  events: readonly RuntimeEvent[],
  checkpointId: string
): CheckpointRewindPlan['conversation'] {
  const plan = planConversationOnlyRewindFromEvents(events, checkpointId)
  if (!plan.ok) {
    return {
      status: 'blocked',
      reason: plan.message,
      retainedEventCount: 0,
      removedEventCount: 0,
      removedTurnIds: [],
      projection: {
        latestSeq: 0,
        turnCount: 0,
        itemCount: 0,
        checkpointCount: 0
      }
    }
  }

  const ordered = [...events].sort((a, b) => a.seq - b.seq)
  const checkpointEvent = ordered.find((event) => (
    event.kind === 'checkpoint_captured' && event.checkpoint.checkpointId === checkpointId
  ))
  const boundary = ordered.find((event) => (
    event.kind === 'turn_started' && event.turnId === plan.checkpoint.turnId
  ))
  return {
    status: 'ready',
    boundaryTurnId: plan.checkpoint.turnId,
    ...(checkpointEvent ? { checkpointEventSeq: checkpointEvent.seq } : {}),
    ...(boundary ? { boundarySeq: boundary.seq } : {}),
    retainedEventCount: plan.retainedEvents.length,
    removedEventCount: plan.removedEvents.length,
    removedTurnIds: plan.removedTurnIds,
    projection: {
      latestSeq: plan.retainedEvents.reduce((max, event) => Math.max(max, event.seq), 0),
      turnCount: plan.projection.turns.length,
      itemCount: plan.projection.items.length,
      checkpointCount: plan.projection.checkpoints.length
    }
  }
}

function summarizeRewindPlanFiles(
  files: readonly CheckpointRestoreFilePlan[],
  conversation: CheckpointRewindPlan['conversation'] | undefined
): CheckpointRewindPlan['summary'] {
  const readyFileCount = files.filter((file) => file.status === 'ready').length
  const manualReviewFileCount = files.filter((file) => file.status === 'manual_review').length
  const blockedFileCount = files.filter((file) => file.status === 'blocked').length
  return {
    fileCount: files.length,
    readyFileCount,
    manualReviewFileCount,
    blockedFileCount,
    retainedEventCount: conversation?.retainedEventCount ?? 0,
    removedEventCount: conversation?.removedEventCount ?? 0,
    removedTurnCount: conversation?.removedTurnIds.length ?? 0,
    containsRawPrompt: false,
    containsFullFileContent: false,
    containsSecretValue: false
  }
}

function createRewindPlanId(input: {
  checkpointId: string
  scope: CheckpointRewindScope
  createdAt: string
  files: readonly CheckpointRestoreFilePlan[]
}): string {
  const digest = createHash('sha256')
    .update(JSON.stringify({
      checkpointId: input.checkpointId,
      scope: input.scope,
      createdAt: input.createdAt,
      files: input.files.map((file) => ({
        relativePath: file.relativePath,
        action: file.action,
        status: file.status,
        risk: file.risk,
        beforeHash: file.beforeHash ?? null,
        afterHash: file.afterHash ?? null
      }))
    }))
    .digest('hex')
    .slice(0, 20)
  return `${ANALYTIX_REWIND_PLAN_ID_PREFIX}${digest}`
}

function createCheckpointId(input: {
  threadId: string
  turnId: string
  workspace: string
  createdAt: string
  changedFiles: readonly CheckpointChangedFile[]
}): string {
  const digest = createHash('sha256')
    .update(JSON.stringify({
      threadId: input.threadId,
      turnId: input.turnId,
      workspace: resolve(input.workspace),
      createdAt: input.createdAt,
      changedFiles: input.changedFiles
    }))
    .digest('hex')
    .slice(0, 20)
  return `${ANALYTIX_CHECKPOINT_ID_PREFIX}${digest}`
}

function changedFileInputFromToolResult(output: unknown): CheckpointChangedFileInput | null {
  if (!output || typeof output !== 'object') return null
  const payload = output as Record<string, unknown>
  const path = stringField(payload, 'relative_path', 'relativePath', 'path', 'file_path', 'target_path')
  if (!path) return null
  return {
    path,
    changeKind: changeKindFromToolPayload(payload),
    beforeHash: stringField(payload, 'before_hash', 'beforeHash') ?? undefined,
    afterHash: stringField(payload, 'after_hash', 'afterHash') ?? undefined
  }
}

function changeKindFromToolPayload(payload: Record<string, unknown>): CheckpointChangeKind {
  const raw = stringField(payload, 'changeKind', 'change_kind', 'kind')
  if (raw === 'created' || raw === 'modified' || raw === 'deleted') return raw
  if (typeof payload.deleted === 'boolean' && payload.deleted) return 'deleted'
  if (typeof payload.created === 'boolean' && payload.created) return 'created'
  if (typeof payload.diff === 'string' || typeof payload.patch === 'string') return 'modified'
  return 'unknown'
}

function stringField(payload: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = payload[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return undefined
}
