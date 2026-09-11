import { execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstat, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path'
import { promisify } from 'node:util'
import type {
  ApplyCheckpointRewindPlanRequest,
  CheckpointApplySnapshotEvidence,
  CheckpointRewindApplyFileResult,
  CheckpointRewindApplyResult,
  CheckpointRewindPlan,
  CheckpointRewindRescueFile,
  CheckpointRewindRescueRecord,
  CheckpointRewindScope,
  CheckpointRestoreFilePlan
} from '../contracts/checkpoints.js'
import {
  ANALYTIX_REWIND_APPLY_ID_PREFIX,
  ANALYTIX_REWIND_APPLY_SCHEMA_VERSION,
  ANALYTIX_REWIND_RESCUE_ID_PREFIX,
  CheckpointRewindApplyResultSchema,
  CheckpointRewindRescueRecordSchema
} from '../contracts/checkpoints.js'
import {
  buildAuditableCheckpointRewindPlan,
  findCheckpointInEvents,
  type CheckpointPathRisk
} from '../domain/checkpoint-rewind-contract.js'
import type { RuntimeEvent } from '../contracts/events.js'
import type { SessionStore } from '../ports/session-store.js'
import { withFileMutationQueue } from '../tool-test-support/tool/file-mutation-queue.js'
import type { RuntimeEventRecorder } from './runtime-event-recorder.js'
import type { ThreadService } from './thread-service.js'

const execFileAsync = promisify(execFile)

export type CheckpointRewindServiceOptions = {
  threadService: ThreadService
  sessionStore: SessionStore
  events: RuntimeEventRecorder
  nowIso: () => string
}

export type CreateCheckpointRewindPlanOptions = {
  threadId: string
  checkpointId: string
  scope?: CheckpointRewindScope
}

export type CreateCheckpointRewindPlanResult =
  | { ok: true; plan: CheckpointRewindPlan }
  | {
      ok: false
      status: 404 | 409
      code: 'thread_not_found' | 'checkpoint_not_found' | 'rewind_boundary_not_found'
      message: string
    }

export type ApplyCheckpointRewindPlanOptions = {
  threadId: string
  checkpointId: string
  request: ApplyCheckpointRewindPlanRequest
  /** Test-support injection that represents host-private snapshot storage. */
  hostPrivateSnapshots?: readonly CheckpointApplySnapshotEvidence[]
}

export type ApplyCheckpointRewindPlanResult =
  | { ok: true; apply: CheckpointRewindApplyResult }
  | {
      ok: false
      status: 404 | 409
      code:
        | 'thread_not_found'
        | 'checkpoint_not_found'
        | 'rewind_boundary_not_found'
        | 'plan_mismatch'
        | 'apply_blocked'
        | 'apply_failed'
      message: string
      apply?: CheckpointRewindApplyResult
    }

type PlannedFilePreflight = {
  plan: CheckpointRestoreFilePlan
  workspace: string
  absolutePath: string
  status: 'apply' | 'noop' | 'manual_review' | 'blocked'
  reason: string
  currentHash?: string | null
  targetContent?: string
}

/**
 * Read-only bridge from persisted runtime events to an auditable restore plan.
 * It deliberately does not mutate the event log, turn items, git refs, or
 * workspace files; P4C owns destructive apply.
 */
export class CheckpointRewindService {
  private readonly threadService: ThreadService
  private readonly sessionStore: SessionStore
  private readonly events: RuntimeEventRecorder
  private readonly nowIso: () => string

  constructor(options: CheckpointRewindServiceOptions) {
    this.threadService = options.threadService
    this.sessionStore = options.sessionStore
    this.events = options.events
    this.nowIso = options.nowIso
  }

  async createPlan(options: CreateCheckpointRewindPlanOptions): Promise<CreateCheckpointRewindPlanResult> {
    const thread = await this.threadService.get(options.threadId)
    if (!thread) {
      return {
        ok: false,
        status: 404,
        code: 'thread_not_found',
        message: `thread not found: ${options.threadId}`
      }
    }

    const events = await this.sessionStore.loadEventsSince(options.threadId, 0)
    const checkpoint = findCheckpointInEvents(events, options.checkpointId)
    if (!checkpoint) {
      return {
        ok: false,
        status: 404,
        code: 'checkpoint_not_found',
        message: `checkpoint not found: ${options.checkpointId}`
      }
    }

    const pathRisks = await inspectCheckpointPathRisks(checkpoint.workspace, checkpoint.changedFiles)
    const result = buildAuditableCheckpointRewindPlan({
      events,
      checkpointId: options.checkpointId,
      scope: options.scope,
      createdAt: this.nowIso(),
      pathRisks
    })
    if (!result.ok) {
      return {
        ok: false,
        status: result.reason === 'checkpoint_not_found' ? 404 : 409,
        code: result.reason === 'checkpoint_not_found' ? 'checkpoint_not_found' : 'rewind_boundary_not_found',
        message: result.message
      }
    }

    return { ok: true, plan: result.plan }
  }

  async applyPlan(options: ApplyCheckpointRewindPlanOptions): Promise<ApplyCheckpointRewindPlanResult> {
    const thread = await this.threadService.get(options.threadId)
    if (!thread) {
      return {
        ok: false,
        status: 404,
        code: 'thread_not_found',
        message: `thread not found: ${options.threadId}`
      }
    }

    const plan = options.request.plan
    const mismatch = validateApplyPlanRoute(options.threadId, options.checkpointId, plan)
    if (mismatch) {
      return {
        ok: false,
        status: 409,
        code: 'plan_mismatch',
        message: mismatch
      }
    }

    const events = await this.sessionStore.loadEventsSince(options.threadId, 0)
    const existing = findExistingApply(events, plan.planId)
    if (existing) {
      return {
        ok: true,
        apply: CheckpointRewindApplyResultSchema.parse({
          ...existing,
          status: 'already_applied',
          conversation: {
            ...existing.conversation,
            status: existing.conversation.status === 'audit_recorded'
              ? 'already_applied'
              : existing.conversation.status
          }
        })
      }
    }

    const checkpoint = findCheckpointInEvents(events, options.checkpointId)
    if (!checkpoint) {
      return {
        ok: false,
        status: 404,
        code: 'checkpoint_not_found',
        message: `checkpoint not found: ${options.checkpointId}`
      }
    }

    const pathRisks = await inspectCheckpointPathRisks(checkpoint.workspace, checkpoint.changedFiles)
    const currentPlan = buildAuditableCheckpointRewindPlan({
      events,
      checkpointId: options.checkpointId,
      scope: plan.scope,
      createdAt: plan.createdAt,
      pathRisks
    })
    if (!currentPlan.ok) {
      return {
        ok: false,
        status: currentPlan.reason === 'checkpoint_not_found' ? 404 : 409,
        code: currentPlan.reason === 'checkpoint_not_found' ? 'checkpoint_not_found' : 'rewind_boundary_not_found',
        message: currentPlan.message
      }
    }
    const planMismatch = compareApplyPlan(plan, currentPlan.plan)
    if (planMismatch) {
      return {
        ok: false,
        status: 409,
        code: 'plan_mismatch',
        message: planMismatch
      }
    }

    // The TypeScript service is a conformance oracle only. Snapshot bytes may
    // enter through this host-private test seam, never through the public HTTP
    // request contract.
    const snapshotEvidence = snapshotEvidenceByPath(options.hostPrivateSnapshots ?? [])
    const preflight = await preflightPlanFiles(plan, snapshotEvidence)
    const blockedFiles = preflight.filter((file) => file.status === 'blocked' || file.status === 'manual_review')
    if (blockedFiles.length > 0 || plan.conversation?.status === 'blocked') {
      const apply = buildApplyResult({
        plan,
        createdAt: this.nowIso(),
        status: 'blocked',
        files: preflight.map(preflightToResult),
        conversation: plan.conversation?.status === 'blocked'
          ? {
              status: 'blocked',
              reason: plan.conversation.reason ?? 'checkpoint conversation boundary is blocked'
            }
          : conversationApplyResult(plan, 'not_requested'),
      })
      return {
        ok: false,
        status: 409,
        code: 'apply_blocked',
        message: blockedFiles[0]?.reason ?? plan.conversation?.reason ?? 'checkpoint rewind apply is blocked',
        apply
      }
    }

    const mutatingFiles = preflight.filter((file) => file.status === 'apply')
    let rescue: { record: CheckpointRewindRescueRecord; eventSeq: number } | undefined
    if (mutatingFiles.length > 0) {
      const record = await createRescueRecord(plan, mutatingFiles, this.nowIso())
      const event = await this.events.record({
        kind: 'checkpoint_rewind_rescue_created',
        threadId: plan.threadId,
        rescue: record
      })
      rescue = { record, eventSeq: event.seq }
    }

    let fileResults: CheckpointRewindApplyFileResult[] = []
    let activeFile: PlannedFilePreflight | undefined
    try {
      for (const file of preflight) {
        if (file.status !== 'apply') {
          fileResults.push(preflightToResult(file))
          continue
        }
        activeFile = file
        await applyPreflightFile(file)
        fileResults.push({
          relativePath: file.plan.relativePath,
          action: file.plan.action,
          status: 'applied',
          reason: applyReasonForAction(file.plan.action),
          ...(file.plan.beforeHash !== undefined ? { beforeHash: file.plan.beforeHash } : {}),
          ...(file.plan.afterHash !== undefined ? { afterHash: file.plan.afterHash } : {}),
            ...(file.currentHash !== undefined ? { currentHash: file.currentHash } : {})
          })
        activeFile = undefined
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      const apply = buildApplyResult({
        plan,
        createdAt: this.nowIso(),
        status: 'failed',
        rescue,
        files: fileResultsAfterMutationFailure(preflight, fileResults, message, activeFile),
        conversation: conversationApplyResult(plan, 'blocked', message)
      })
      await this.events.record({
        kind: 'checkpoint_rewind_applied',
        threadId: plan.threadId,
        apply
      }).catch(() => undefined)
      return {
        ok: false,
        status: 409,
        code: 'apply_failed',
        message,
        apply
      }
    }

    const conversation = conversationApplyResult(plan, plan.conversation ? 'audit_recorded' : 'not_requested')
    const applyWithoutSeq = buildApplyResult({
      plan,
      createdAt: this.nowIso(),
      status: 'applied',
      rescue,
      files: fileResults,
      conversation
    })
    const event = await this.events.record({
      kind: 'checkpoint_rewind_applied',
      threadId: plan.threadId,
      apply: applyWithoutSeq
    })
    const apply = CheckpointRewindApplyResultSchema.parse({
      ...applyWithoutSeq,
      auditEventSeq: event.seq,
      conversation: plan.conversation
        ? { ...applyWithoutSeq.conversation, auditEventSeq: event.seq }
        : applyWithoutSeq.conversation
    })

    return { ok: true, apply }
  }
}

function validateApplyPlanRoute(
  threadId: string,
  checkpointId: string,
  plan: CheckpointRewindPlan
): string | null {
  if (plan.threadId !== threadId) return `rewind plan thread mismatch: ${plan.threadId}`
  if (plan.checkpointId !== checkpointId) return `rewind plan checkpoint mismatch: ${plan.checkpointId}`
  if (plan.applyMode !== 'plan_only' || plan.destructive !== false) {
    return 'rewind apply must be based on a P4B plan_only CheckpointRewindPlan'
  }
  return null
}

function compareApplyPlan(requested: CheckpointRewindPlan, current: CheckpointRewindPlan): string | null {
  const comparableRequested = comparablePlan(requested)
  const comparableCurrent = comparablePlan(current)
  return JSON.stringify(comparableRequested) === JSON.stringify(comparableCurrent)
    ? null
    : 'rewind plan is stale or does not match the current checkpoint safety plan'
}

function comparablePlan(plan: CheckpointRewindPlan): unknown {
  return {
    schemaVersion: plan.schemaVersion,
    planId: plan.planId,
    checkpointId: plan.checkpointId,
    threadId: plan.threadId,
    planDigest: plan.planDigest,
    workspace: resolve(plan.workspace),
    createdAt: plan.createdAt,
    scope: plan.scope,
    applyMode: plan.applyMode,
    destructive: plan.destructive,
    checkpoint: plan.checkpoint,
    files: plan.files,
    conversation: plan.conversation,
    summary: plan.summary
  }
}

function findExistingApply(events: readonly RuntimeEvent[], planId: string): CheckpointRewindApplyResult | null {
  const event = [...events]
    .sort((a, b) => b.seq - a.seq)
    .find((candidate) => (
      candidate.kind === 'checkpoint_rewind_applied' &&
      candidate.apply.planId === planId &&
      candidate.apply.status === 'applied'
    ))
  return event?.kind === 'checkpoint_rewind_applied' ? event.apply : null
}

function snapshotEvidenceByPath(
  snapshots: readonly CheckpointApplySnapshotEvidence[]
): Map<string, CheckpointApplySnapshotEvidence> {
  return new Map(snapshots.map((snapshot) => [snapshot.relativePath, snapshot]))
}

async function preflightPlanFiles(
  plan: CheckpointRewindPlan,
  snapshots: ReadonlyMap<string, CheckpointApplySnapshotEvidence>
): Promise<PlannedFilePreflight[]> {
  if (plan.scope === 'conversation') return []
  const results: PlannedFilePreflight[] = []
  for (const file of plan.files) {
    results.push(await preflightPlanFile(plan.workspace, file, snapshots.get(file.relativePath)))
  }
  return results
}

async function preflightPlanFile(
  workspace: string,
  plan: CheckpointRestoreFilePlan,
  snapshot: CheckpointApplySnapshotEvidence | undefined
): Promise<PlannedFilePreflight> {
  const safePath = safeApplyPath(workspace, plan.relativePath)
  const base: Omit<PlannedFilePreflight, 'status' | 'reason'> = {
    plan,
    workspace,
    absolutePath: safePath.ok ? safePath.absolutePath : resolve(workspace, plan.relativePath)
  }
  if (!safePath.ok) return { ...base, status: 'blocked', reason: safePath.reason }
  const symlinkAncestor = await findSymlinkAncestor(workspace, safePath.relativePath)
  if (symlinkAncestor) {
    return { ...base, status: 'blocked', reason: `checkpoint apply path contains symlink ancestor: ${symlinkAncestor}` }
  }
  if (plan.status === 'blocked' || plan.action === 'blocked') {
    return { ...base, status: 'blocked', reason: plan.reason }
  }
  if (plan.status === 'manual_review' || plan.action === 'manual_review') {
    return { ...base, status: 'manual_review', reason: plan.reason }
  }
  const info = await lstat(safePath.absolutePath).catch(() => null)
  if (info?.isSymbolicLink()) {
    return { ...base, status: 'blocked', reason: 'current workspace path is a symlink' }
  }
  if (info?.isDirectory()) {
    return { ...base, status: 'blocked', reason: 'current workspace path is a directory' }
  }
  const git = await gitStatusForPath(workspace, plan.relativePath)
  if (git.staged) {
    return { ...base, status: 'blocked', reason: 'current workspace path has staged git changes' }
  }

  switch (plan.action) {
    case 'noop':
      return { ...base, status: 'noop', reason: 'checkpoint plan requires no file mutation' }
    case 'delete_created_file':
      return preflightDeleteCreatedFile(base, plan, info, git)
    case 'restore_previous_version':
      return preflightRestorePreviousVersion(base, plan, snapshot, info, git)
    case 'restore_deleted_file':
      return preflightRestoreDeletedFile(base, plan, snapshot, info, git)
    default:
      return { ...base, status: 'manual_review', reason: plan.reason }
  }
}

async function preflightDeleteCreatedFile(
  base: Omit<PlannedFilePreflight, 'status' | 'reason'>,
  plan: CheckpointRestoreFilePlan,
  info: Awaited<ReturnType<typeof lstat>> | null,
  git: GitPathStatus
): Promise<PlannedFilePreflight> {
  if (!info) return { ...base, status: 'noop', reason: 'created file is already absent' }
  if (!plan.afterHash) {
    return { ...base, status: 'blocked', reason: 'delete_created_file requires afterHash evidence' }
  }
  const current = await readUtf8Hash(base.absolutePath)
  if (current.hash !== plan.afterHash) {
    return {
      ...base,
      status: 'blocked',
      reason: git.untracked
        ? 'untracked file content does not match checkpoint-created hash'
        : 'current file hash does not match checkpoint-created hash',
      currentHash: current.hash
    }
  }
  return {
    ...base,
    status: 'apply',
    reason: git.untracked
      ? 'untracked file matches checkpoint-created hash and is safe to delete'
      : 'created file hash matches checkpoint-created hash',
    currentHash: current.hash
  }
}

async function preflightRestorePreviousVersion(
  base: Omit<PlannedFilePreflight, 'status' | 'reason'>,
  plan: CheckpointRestoreFilePlan,
  snapshot: CheckpointApplySnapshotEvidence | undefined,
  info: Awaited<ReturnType<typeof lstat>> | null,
  git: GitPathStatus
): Promise<PlannedFilePreflight> {
  if (!snapshot?.before) {
    return { ...base, status: 'blocked', reason: 'restore_previous_version requires before snapshot evidence' }
  }
  const snapshotError = validateSnapshotContent(snapshot.before, 'before')
  if (snapshotError) return { ...base, status: 'blocked', reason: snapshotError }
  if (plan.beforeHash && snapshot.before.hash !== plan.beforeHash) {
    return { ...base, status: 'blocked', reason: 'before snapshot hash does not match rewind plan beforeHash' }
  }
  if (!info) {
    return { ...base, status: 'manual_review', reason: 'current file is missing before modified restore' }
  }
  if (git.untracked) {
    return { ...base, status: 'blocked', reason: 'current workspace path is untracked in git' }
  }
  if (!plan.afterHash) {
    return { ...base, status: 'blocked', reason: 'restore_previous_version requires afterHash evidence' }
  }
  const current = await readUtf8Hash(base.absolutePath)
  if (current.hash === snapshot.before.hash) {
    return { ...base, status: 'noop', reason: 'current file already matches before snapshot', currentHash: current.hash }
  }
  if (current.hash !== plan.afterHash) {
    return {
      ...base,
      status: 'blocked',
      reason: 'current file hash differs from checkpoint afterHash',
      currentHash: current.hash
    }
  }
  return {
    ...base,
    status: 'apply',
    reason: 'current file matches checkpoint afterHash; restoring before snapshot',
    currentHash: current.hash,
    targetContent: snapshot.before.content
  }
}

async function preflightRestoreDeletedFile(
  base: Omit<PlannedFilePreflight, 'status' | 'reason'>,
  plan: CheckpointRestoreFilePlan,
  snapshot: CheckpointApplySnapshotEvidence | undefined,
  info: Awaited<ReturnType<typeof lstat>> | null,
  git: GitPathStatus
): Promise<PlannedFilePreflight> {
  if (!snapshot?.before) {
    return { ...base, status: 'blocked', reason: 'restore_deleted_file requires before snapshot evidence' }
  }
  const snapshotError = validateSnapshotContent(snapshot.before, 'before')
  if (snapshotError) return { ...base, status: 'blocked', reason: snapshotError }
  if (plan.beforeHash && snapshot.before.hash !== plan.beforeHash) {
    return { ...base, status: 'blocked', reason: 'before snapshot hash does not match rewind plan beforeHash' }
  }
  if (git.untracked) {
    return { ...base, status: 'blocked', reason: 'current workspace path is untracked in git' }
  }
  if (info) {
    const current = await readUtf8Hash(base.absolutePath)
    if (current.hash === snapshot.before.hash) {
      return { ...base, status: 'noop', reason: 'deleted file is already restored', currentHash: current.hash }
    }
    return {
      ...base,
      status: 'blocked',
      reason: 'current path exists with content that differs from before snapshot',
      currentHash: current.hash
    }
  }
  return {
    ...base,
    status: 'apply',
    reason: 'deleted file is absent; restoring before snapshot',
    currentHash: null,
    targetContent: snapshot.before.content
  }
}

type GitPathStatus = {
  staged: boolean
  untracked: boolean
}

async function gitStatusForPath(workspace: string, relativePath: string): Promise<GitPathStatus> {
  try {
    const { stdout } = await execFileAsync('git', [
      '-C',
      workspace,
      'status',
      '--porcelain=v1',
      '--',
      relativePath
    ], { encoding: 'utf8' })
    const lines = stdout.split(/\r?\n/).filter(Boolean)
    return {
      staged: lines.some((line) => line.length >= 2 && line[0] !== ' ' && line[0] !== '?'),
      untracked: lines.some((line) => line.startsWith('?? '))
    }
  } catch {
    return { staged: false, untracked: false }
  }
}

async function readUtf8Hash(path: string): Promise<{ hash: string; content: string }> {
  const content = await readFile(path, 'utf8')
  return { content, hash: hashUtf8(content) }
}

function hashUtf8(content: string): string {
  return `sha256:${createHash('sha256').update(content, 'utf8').digest('hex')}`
}

function validateSnapshotContent(
  snapshot: { hash: string; content: string },
  label: 'before' | 'after'
): string | null {
  const actual = hashUtf8(snapshot.content)
  return actual === snapshot.hash ? null : `${label} snapshot content hash does not match snapshot hash`
}

async function findSymlinkAncestor(workspace: string, relativePath: string): Promise<string | null> {
  const parts = relativePath.split('/').filter(Boolean).slice(0, -1)
  let current = resolve(workspace)
  const seen: string[] = []
  for (const part of parts) {
    seen.push(part)
    current = resolve(current, part)
    const info = await lstat(current).catch(() => null)
    if (info?.isSymbolicLink()) return seen.join('/')
  }
  return null
}

async function createRescueRecord(
  plan: CheckpointRewindPlan,
  files: readonly PlannedFilePreflight[],
  createdAt: string
): Promise<CheckpointRewindRescueRecord> {
  const rescueFiles: CheckpointRewindRescueFile[] = []
  for (const file of files) {
    const current = await readUtf8Hash(file.absolutePath).catch(() => null)
    rescueFiles.push({
      relativePath: file.plan.relativePath,
      existed: Boolean(current),
      ...(current ? { hash: current.hash, content: current.content, encoding: 'utf8' as const } : {})
    })
  }
  return CheckpointRewindRescueRecordSchema.parse({
    schemaVersion: ANALYTIX_REWIND_APPLY_SCHEMA_VERSION,
    rescueId: createRescueId(plan, createdAt, rescueFiles),
    planId: plan.planId,
    checkpointId: plan.checkpointId,
    threadId: plan.threadId,
    workspace: plan.workspace,
    createdAt,
    files: rescueFiles
  })
}

async function applyPreflightFile(file: PlannedFilePreflight): Promise<void> {
  await withFileMutationQueue(file.absolutePath, async () => {
    await assertPreflightStillSafe(file)
    if (file.plan.action === 'delete_created_file') {
      await rm(file.absolutePath, { force: true })
      return
    }
    if (
      file.plan.action === 'restore_previous_version' ||
      file.plan.action === 'restore_deleted_file'
    ) {
      if (file.targetContent === undefined) {
        throw new Error(`missing target content for ${file.plan.relativePath}`)
      }
      await mkdir(dirname(file.absolutePath), { recursive: true })
      await writeFile(file.absolutePath, file.targetContent, 'utf8')
      return
    }
    throw new Error(`unsupported checkpoint rewind file action: ${file.plan.action}`)
  })
}

async function assertPreflightStillSafe(file: PlannedFilePreflight): Promise<void> {
  const safePath = safeApplyPath(file.workspace, file.plan.relativePath)
  if (!safePath.ok) throw new Error(safePath.reason)
  const symlinkAncestor = await findSymlinkAncestor(file.workspace, safePath.relativePath)
  if (symlinkAncestor) throw new Error(`checkpoint apply path contains symlink ancestor: ${symlinkAncestor}`)
  const info = await lstat(file.absolutePath).catch(() => null)
  if (info?.isSymbolicLink()) throw new Error(`checkpoint apply target is a symlink: ${file.plan.relativePath}`)
  if (info?.isDirectory()) throw new Error(`checkpoint apply target is a directory: ${file.plan.relativePath}`)
  const git = await gitStatusForPath(file.workspace, file.plan.relativePath)
  if (git.staged) throw new Error(`checkpoint apply target has staged git changes: ${file.plan.relativePath}`)

  switch (file.plan.action) {
    case 'delete_created_file': {
      if (!info) return
      const current = await readUtf8Hash(file.absolutePath)
      if (file.currentHash && current.hash !== file.currentHash) {
        throw new Error(`checkpoint apply target changed after preflight: ${file.plan.relativePath}`)
      }
      return
    }
    case 'restore_previous_version': {
      if (!info) throw new Error(`checkpoint apply target disappeared after preflight: ${file.plan.relativePath}`)
      if (git.untracked) throw new Error(`checkpoint apply target became untracked: ${file.plan.relativePath}`)
      const current = await readUtf8Hash(file.absolutePath)
      if (file.currentHash && current.hash !== file.currentHash) {
        throw new Error(`checkpoint apply target changed after preflight: ${file.plan.relativePath}`)
      }
      return
    }
    case 'restore_deleted_file': {
      if (info) throw new Error(`checkpoint apply target appeared after preflight: ${file.plan.relativePath}`)
      if (git.untracked) throw new Error(`checkpoint apply target became untracked: ${file.plan.relativePath}`)
      return
    }
    default:
      throw new Error(`unsupported checkpoint rewind file action: ${file.plan.action}`)
  }
}

function preflightToResult(file: PlannedFilePreflight): CheckpointRewindApplyFileResult {
  return {
    relativePath: file.plan.relativePath,
    action: file.plan.action,
    status: file.status === 'apply' ? 'blocked' : file.status,
    reason: file.status === 'apply'
      ? 'not applied because checkpoint rewind preflight was blocked'
      : file.reason,
    ...(file.plan.beforeHash !== undefined ? { beforeHash: file.plan.beforeHash } : {}),
    ...(file.plan.afterHash !== undefined ? { afterHash: file.plan.afterHash } : {}),
    ...(file.currentHash !== undefined ? { currentHash: file.currentHash } : {})
  }
}

function fileResultsAfterMutationFailure(
  preflight: readonly PlannedFilePreflight[],
  completed: readonly CheckpointRewindApplyFileResult[],
  message: string,
  activeFile: PlannedFilePreflight | undefined
): CheckpointRewindApplyFileResult[] {
  const completedPaths = new Set(completed.map((file) => file.relativePath))
  return [
    ...completed,
    ...preflight
      .filter((file) => !completedPaths.has(file.plan.relativePath))
      .map((file) => file.status === 'apply'
        ? {
            relativePath: file.plan.relativePath,
            action: file.plan.action,
            status: 'failed' as const,
            reason: file === activeFile ? message : 'file mutation did not complete',
            ...(file.plan.beforeHash !== undefined ? { beforeHash: file.plan.beforeHash } : {}),
            ...(file.plan.afterHash !== undefined ? { afterHash: file.plan.afterHash } : {}),
            ...(file.currentHash !== undefined ? { currentHash: file.currentHash } : {})
          }
        : preflightToResult(file))
  ]
}

function conversationApplyResult(
  plan: CheckpointRewindPlan,
  status: CheckpointRewindApplyResult['conversation']['status'],
  reason?: string
): CheckpointRewindApplyResult['conversation'] {
  if (!plan.conversation) return { status: 'not_requested' }
  return {
    status,
    ...(reason ? { reason } : {}),
    ...(plan.conversation.boundaryTurnId ? { boundaryTurnId: plan.conversation.boundaryTurnId } : {}),
    retainedEventCount: plan.conversation.retainedEventCount,
    removedEventCount: plan.conversation.removedEventCount,
    removedTurnIds: plan.conversation.removedTurnIds
  }
}

function buildApplyResult(input: {
  plan: CheckpointRewindPlan
  createdAt: string
  status: CheckpointRewindApplyResult['status']
  rescue?: { record: CheckpointRewindRescueRecord; eventSeq: number }
  files: CheckpointRewindApplyFileResult[]
  conversation: CheckpointRewindApplyResult['conversation']
}): CheckpointRewindApplyResult {
  return CheckpointRewindApplyResultSchema.parse({
    schemaVersion: ANALYTIX_REWIND_APPLY_SCHEMA_VERSION,
    applyId: createApplyId(input.plan, input.createdAt),
    planId: input.plan.planId,
    checkpointId: input.plan.checkpointId,
    threadId: input.plan.threadId,
    workspace: input.plan.workspace,
    createdAt: input.createdAt,
    scope: input.plan.scope,
    status: input.status,
    destructive: true,
    ...(input.rescue
      ? {
          rescue: {
            rescueId: input.rescue.record.rescueId,
            eventSeq: input.rescue.eventSeq,
            fileCount: input.rescue.record.files.length
          }
        }
      : {}),
    files: input.files,
    conversation: input.conversation,
    summary: summarizeApplyFiles(input.files)
  })
}

function summarizeApplyFiles(files: readonly CheckpointRewindApplyFileResult[]): CheckpointRewindApplyResult['summary'] {
  return {
    fileAppliedCount: files.filter((file) => file.status === 'applied').length,
    fileNoopCount: files.filter((file) => file.status === 'noop').length,
    fileManualReviewCount: files.filter((file) => file.status === 'manual_review').length,
    fileBlockedCount: files.filter((file) => file.status === 'blocked').length,
    fileFailedCount: files.filter((file) => file.status === 'failed').length
  }
}

function createApplyId(plan: CheckpointRewindPlan, createdAt: string): string {
  const digest = createHash('sha256')
    .update(JSON.stringify({
      planId: plan.planId,
      checkpointId: plan.checkpointId,
      scope: plan.scope,
      createdAt
    }))
    .digest('hex')
    .slice(0, 20)
  return `${ANALYTIX_REWIND_APPLY_ID_PREFIX}${digest}`
}

function createRescueId(
  plan: CheckpointRewindPlan,
  createdAt: string,
  files: readonly CheckpointRewindRescueFile[]
): string {
  const digest = createHash('sha256')
    .update(JSON.stringify({
      planId: plan.planId,
      checkpointId: plan.checkpointId,
      createdAt,
      files: files.map((file) => ({
        relativePath: file.relativePath,
        existed: file.existed,
        hash: file.hash ?? null
      }))
    }))
    .digest('hex')
    .slice(0, 20)
  return `${ANALYTIX_REWIND_RESCUE_ID_PREFIX}${digest}`
}

function applyReasonForAction(action: CheckpointRestoreFilePlan['action']): string {
  switch (action) {
    case 'delete_created_file':
      return 'deleted checkpoint-created file after rescue checkpoint'
    case 'restore_previous_version':
      return 'restored previous file content after rescue checkpoint'
    case 'restore_deleted_file':
      return 'restored deleted file content after rescue checkpoint'
    default:
      return 'checkpoint rewind apply completed'
  }
}

function safeApplyPath(
  workspace: string,
  rawPath: string
): { ok: true; absolutePath: string; relativePath: string } | { ok: false; reason: string } {
  const trimmed = rawPath.trim()
  if (!trimmed) return { ok: false, reason: 'checkpoint apply path is required' }
  if (isAbsolute(trimmed)) return { ok: false, reason: 'checkpoint apply path must be workspace-relative' }
  const absolutePath = resolve(workspace, trimmed)
  const relativePath = relativePathFromWorkspace(workspace, absolutePath)
  if (!relativePath) return { ok: false, reason: `checkpoint apply path escapes workspace: ${rawPath}` }
  return { ok: true, absolutePath, relativePath }
}

async function inspectCheckpointPathRisks(
  workspace: string,
  files: readonly { relativePath: string }[]
): Promise<Map<string, CheckpointPathRisk>> {
  const risks = new Map<string, CheckpointPathRisk>()
  await Promise.all(files.map(async (file) => {
    if (isAbsolute(file.relativePath)) {
      risks.set(file.relativePath, { exists: false })
      return
    }
    const absolutePath = resolve(workspace, file.relativePath)
    const relativePath = relativePathFromWorkspace(workspace, absolutePath)
    if (!relativePath) {
      risks.set(file.relativePath, { exists: false })
      return
    }
    const info = await lstat(absolutePath).catch(() => null)
    risks.set(file.relativePath, {
      exists: Boolean(info),
      isSymlink: info?.isSymbolicLink() ?? false
    })
    if (relativePath !== file.relativePath) {
      risks.set(relativePath, risks.get(file.relativePath) ?? { exists: Boolean(info) })
    }
  }))
  return risks
}

function relativePathFromWorkspace(workspace: string, absolutePath: string): string | null {
  const root = resolve(workspace)
  const rel = relative(root, resolve(absolutePath))
  if (!rel || rel === '..' || rel.startsWith(`..${sep}`) || isAbsolute(rel)) return null
  return rel.split(sep).join('/')
}
