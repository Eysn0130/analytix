import { createHash } from 'node:crypto'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'
import { SUBAGENT_READ_ONLY_TOOL_NAMES, SubagentToolPolicy } from '../contracts/capabilities.js'

export const TaskJobStatus = z.enum(['queued', 'running', 'completed', 'failed', 'interrupted', 'killed'])
export type TaskJobStatus = z.infer<typeof TaskJobStatus>

const TaskJobOutputChunk = z.object({
  offset: z.number().int().nonnegative(),
  text: z.string(),
  at: z.string()
}).strict()

export const TaskJobTranscriptRef = z.object({
  mode: z.enum(['new', 'continue', 'fork']),
  sourceId: z.string().min(1).optional(),
  targetId: z.string().min(1).optional(),
  identityHash: z.string().min(1)
}).strict()
export type TaskJobTranscriptRef = z.infer<typeof TaskJobTranscriptRef>

export const TaskJobRecord = z.object({
  id: z.string().min(1),
  kind: z.enum(['task', 'parallel_task', 'planner']),
  parentThreadId: z.string().min(1),
  parentTurnId: z.string().min(1),
  label: z.string().optional(),
  parallelIndex: z.number().int().positive().optional(),
  status: TaskJobStatus,
  dependencies: z.array(z.string().min(1)).default([]),
  permissionPolicy: SubagentToolPolicy.optional(),
  parentCallId: z.string().optional(),
  childRunId: z.string().optional(),
  output: z.array(TaskJobOutputChunk).default([]),
  result: z.string().optional(),
  error: z.string().optional(),
  transcript: TaskJobTranscriptRef.optional(),
  createdAt: z.string(),
  startedAt: z.string().optional(),
  finishedAt: z.string().optional(),
  updatedAt: z.string()
}).strict()
export type TaskJobRecord = z.infer<typeof TaskJobRecord>

export type TaskJobControlMetadata = Pick<TaskJobRecord,
  | 'id'
  | 'kind'
  | 'parentThreadId'
  | 'parentTurnId'
  | 'label'
  | 'parallelIndex'
  | 'status'
  | 'dependencies'
  | 'parentCallId'
  | 'childRunId'
  | 'createdAt'
  | 'startedAt'
  | 'finishedAt'
  | 'updatedAt'
>

export type TaskJobRunner = (context: {
  jobId: string
  signal: AbortSignal
  appendOutput: (text: string) => Promise<void>
  recordChildRun: (childRunId: string) => Promise<void>
}) => Promise<string | void>

export type TaskJobRehydrateResult = {
  restarted: TaskJobRecord[]
  failed: TaskJobRecord[]
}

export type TaskJobStartInput = {
  kind: TaskJobRecord['kind']
  parentThreadId: string
  parentTurnId: string
  label?: string
  parallelIndex?: number
  dependencies?: string[]
  permissionPolicy?: z.infer<typeof SubagentToolPolicy>
  parentCallId?: string
  childRunId?: string
  transcript?: TaskJobTranscriptRef
}

export type TaskJobOutputRead = {
  id: string
  status: TaskJobStatus
  output: string
  offset: number
  nextOffset: number
  outputBytes: number
  truncated: boolean
}

export type TaskJobOutputReadOptions = {
  offset?: number
  limit?: number
  parentThreadId?: string
}

export type TaskJobWaitOptions = {
  timeoutMs?: number
  parentThreadId?: string
}

export type TaskJobRunOptions = {
  parentSignal?: AbortSignal
}

export type TaskJobAccessResult<T> =
  | { ok: true; value: T }
  | { ok: false; reason: 'not_found' | 'forbidden' }

export type ParallelTaskPlanItem = {
  id: string
  dependsOn?: string[]
}

export type ParallelTaskContractItem = {
  id: string
  depends_on?: string[]
  dependsOn?: string[]
}

export type TranscriptIdentity = {
  modelId?: string
  model?: string
  providerId?: string
  endpointFormat?: string
  variant?: string
  modelSource?: string
  effort?: string
  profile?: string
  workspace?: string
  toolPolicy?: string
  toolNames: readonly string[]
  systemPromptHash?: string
  promptPreambleHash?: string
  toolSchemaHash?: string
  sandboxMode?: string
  approvalPolicy?: string
  capabilityFingerprint?: string
}

export const PLANNER_READ_ONLY_TOOLSET = [...SUBAGENT_READ_ONLY_TOOL_NAMES]

export const TASK_JOB_ROUTE_CONTRACT = {
  wait: '/v1/runtime/task-jobs/wait',
  output: '/v1/runtime/task-jobs/output',
  kill: '/v1/runtime/task-jobs/kill'
} as const

export const TASK_TOOL_CONTRACT = {
  name: 'task',
  promptField: 'prompt',
  backgroundField: 'run_in_background',
  continueField: 'continue_from',
  forkField: 'fork_from',
  internalRuntimeOnly: true,
  requiresPermissionGate: true,
  mayAppendParentGoalEvidence: true
} as const

export const PARALLEL_TASKS_TOOL_CONTRACT = {
  name: 'parallel_tasks',
  tasksField: 'tasks',
  dependencyField: 'depends_on',
  internalRuntimeOnly: true,
  requiresDependencyValidation: true,
  requiresPlannerReadOnlyToolset: true
} as const

export const TASK_JOB_CONTROL_TOOL_CONTRACT = {
  wait: 'wait',
  output: 'bash_output',
  kill: 'kill_shell'
} as const

export class FileTaskJobStore {
  constructor(private readonly rootDir: string) {}

  async upsert(record: TaskJobRecord): Promise<void> {
    await mkdir(this.rootDir, { recursive: true })
    await writeFile(join(this.rootDir, `${record.id}.json`), JSON.stringify(record, null, 2), 'utf8')
  }

  async load(id: string): Promise<TaskJobRecord | undefined> {
    const path = join(this.rootDir, `${id}.json`)
    for (let attempt = 0; attempt < 3; attempt += 1) {
      try {
        return TaskJobRecord.parse(JSON.parse(await readFile(path, 'utf8')))
      } catch (error) {
        if (isMissingFileError(error) || attempt === 2) return undefined
        await new Promise((resolve) => setTimeout(resolve, 1))
      }
    }
    return undefined
  }

  async list(parentThreadId?: string): Promise<TaskJobRecord[]> {
    await mkdir(this.rootDir, { recursive: true })
    const entries = await readdir(this.rootDir).catch(() => [])
    const records = await Promise.all(entries
      .filter((entry) => entry.endsWith('.json'))
      .map((entry) => readFile(join(this.rootDir, entry), 'utf8')
        .then((text) => TaskJobRecord.parse(JSON.parse(text)))
        .catch(() => undefined)))
    return records
      .filter((record): record is TaskJobRecord => Boolean(record))
      .filter((record) => !parentThreadId || record.parentThreadId === parentThreadId)
      .sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id))
  }
}

export class DurableTaskJobManager {
  private readonly controllers = new Map<string, AbortController>()
  private readonly readOffsets = new Map<string, number>()
  private seq = 0

  constructor(private readonly options: {
    store: FileTaskJobStore
    nowIso?: () => string
    idGenerator?: (kind: TaskJobRecord['kind']) => string
  }) {}

  async startForeground(
    input: TaskJobStartInput,
    runner: TaskJobRunner,
    options: TaskJobRunOptions = {}
  ): Promise<TaskJobRecord> {
    const record = await this.createRunningRecord(input)
    return this.run(record, runner, options)
  }

  async startBackground(
    input: TaskJobStartInput,
    runner: TaskJobRunner,
    options: TaskJobRunOptions = {}
  ): Promise<TaskJobRecord> {
    const record = await this.createRunningRecord(input)
    void this.run(record, runner, options).catch(() => undefined)
    return record
  }

  async appendOutput(jobId: string, text: string): Promise<TaskJobRecord | undefined> {
    const record = await this.options.store.load(jobId)
    if (!record) return undefined
    const next = TaskJobRecord.parse({
      ...record,
      output: [
        ...record.output,
        { offset: taskJobOutputByteLength(record), text, at: this.now() }
      ],
      updatedAt: this.now()
    })
    await this.options.store.upsert(next)
    return next
  }

  async recordChildRun(jobId: string, childRunId: string): Promise<TaskJobRecord | undefined> {
    const trimmed = childRunId.trim()
    if (!trimmed) return this.options.store.load(jobId)
    const record = await this.options.store.load(jobId)
    if (!record) return undefined
    if (record.childRunId === trimmed) return record
    const next = TaskJobRecord.parse({
      ...record,
      childRunId: trimmed,
      updatedAt: this.now()
    })
    await this.options.store.upsert(next)
    return next
  }

  async output(jobId: string, options: TaskJobOutputReadOptions = {}): Promise<TaskJobOutputRead | undefined> {
    const checked = await this.outputForParent(jobId, options)
    return checked.ok ? checked.value : undefined
  }

  async outputForParent(jobId: string, options: TaskJobOutputReadOptions = {}): Promise<TaskJobAccessResult<TaskJobOutputRead>> {
    const record = await this.options.store.load(jobId)
    if (!record) return { ok: false, reason: 'not_found' }
    if (!this.canAccess(record, options.parentThreadId)) return { ok: false, reason: 'forbidden' }
    const output = record.output.map((chunk) => chunk.text).join('')
    const outputBuffer = Buffer.from(output, 'utf8')
    const outputBytes = outputBuffer.length
    const offset = clampTaskJobOutputOffset(options.offset ?? this.readOffsets.get(jobId) ?? 0, outputBytes)
    const limit = options.limit !== undefined && options.limit > 0 ? options.limit : undefined
    const end = limit === undefined ? outputBytes : Math.min(outputBytes, offset + limit)
    this.readOffsets.set(jobId, end)
    return {
      ok: true,
      value: {
        id: jobId,
        status: record.status,
        output: outputBuffer.subarray(offset, end).toString('utf8'),
        offset,
        nextOffset: end,
        outputBytes,
        truncated: end < outputBytes
      }
    }
  }

  async metadataForParent(
    jobId: string,
    options: { parentThreadId?: string } = {}
  ): Promise<TaskJobAccessResult<TaskJobControlMetadata>> {
    const record = await this.options.store.load(jobId)
    if (!record) return { ok: false, reason: 'not_found' }
    if (!this.canAccess(record, options.parentThreadId)) return { ok: false, reason: 'forbidden' }
    return { ok: true, value: taskJobControlMetadata(record) }
  }

  async wait(jobIds: readonly string[], options: TaskJobWaitOptions = {}): Promise<TaskJobRecord[]> {
    const checked = await this.waitForParent(jobIds, options)
    return checked.ok ? checked.value : []
  }

  async waitForParent(jobIds: readonly string[], options: TaskJobWaitOptions = {}): Promise<TaskJobAccessResult<TaskJobRecord[]>> {
    const timeoutMs = options.timeoutMs ?? 0
    const deadline = Date.now() + timeoutMs
    const requestedJobIds = jobIds.length > 0
      ? [...jobIds]
      : await this.runningJobIdsForParent(options.parentThreadId)
    let records = await this.loadJobs(requestedJobIds)
    if (records.length !== requestedJobIds.length) return { ok: false, reason: 'not_found' }
    if (!records.every((record) => this.canAccess(record, options.parentThreadId))) {
      return { ok: false, reason: 'forbidden' }
    }
    while (
      timeoutMs > 0 &&
      records.some((record) => !isTerminalJobStatus(record.status)) &&
      Date.now() < deadline
    ) {
      await new Promise((resolve) => setTimeout(resolve, Math.min(25, Math.max(1, deadline - Date.now()))))
      records = await this.loadJobs(requestedJobIds)
      if (records.length !== requestedJobIds.length) return { ok: false, reason: 'not_found' }
      if (!records.every((record) => this.canAccess(record, options.parentThreadId))) {
        return { ok: false, reason: 'forbidden' }
      }
    }
    return { ok: true, value: records }
  }

  async waitMetadataForParent(
    jobIds: readonly string[],
    options: TaskJobWaitOptions = {}
  ): Promise<TaskJobAccessResult<TaskJobControlMetadata[]>> {
    const result = await this.waitForParent(jobIds, options)
    return result.ok
      ? { ok: true, value: result.value.map(taskJobControlMetadata) }
      : result
  }

  async kill(jobId: string, reason = 'killed by parent'): Promise<TaskJobRecord | undefined> {
    const checked = await this.killForParent(jobId, { reason })
    return checked.ok ? checked.value : undefined
  }

  async killForParent(
    jobId: string,
    options: { reason?: string; parentThreadId?: string } = {}
  ): Promise<TaskJobAccessResult<TaskJobRecord>> {
    const reason = options.reason ?? 'killed by parent'
    const record = await this.options.store.load(jobId)
    if (!record) return { ok: false, reason: 'not_found' }
    if (!this.canAccess(record, options.parentThreadId)) return { ok: false, reason: 'forbidden' }
    this.controllers.get(jobId)?.abort(reason)
    if (isTerminalJobStatus(record.status)) return { ok: true, value: record }
    const killed = TaskJobRecord.parse({
      ...record,
      status: 'killed',
      error: reason,
      finishedAt: this.now(),
      updatedAt: this.now()
    })
    await this.options.store.upsert(killed)
    return { ok: true, value: killed }
  }

  async killMetadataForParent(
    jobId: string,
    options: { reason?: string; parentThreadId?: string } = {}
  ): Promise<TaskJobAccessResult<TaskJobControlMetadata>> {
    const result = await this.killForParent(jobId, options)
    return result.ok
      ? { ok: true, value: taskJobControlMetadata(result.value) }
      : result
  }

  async list(parentThreadId?: string): Promise<TaskJobRecord[]> {
    return this.options.store.list(parentThreadId)
  }

  async listMetadata(parentThreadId?: string): Promise<TaskJobControlMetadata[]> {
    return (await this.options.store.list(parentThreadId)).map(taskJobControlMetadata)
  }

  async reconcileRunningJobs(reason = 'runtime restarted before task job finished'): Promise<TaskJobRecord[]> {
    const records = await this.options.store.list()
    const reconciled: TaskJobRecord[] = []
    for (const record of records) {
      if (record.status !== 'queued' && record.status !== 'running') continue
      const interrupted = TaskJobRecord.parse({
        ...record,
        status: 'interrupted',
        error: reason,
        finishedAt: record.finishedAt ?? this.now(),
        updatedAt: this.now()
      })
      await this.options.store.upsert(interrupted)
      reconciled.push(interrupted)
    }
    return reconciled
  }

  async rehydrateRunnableJobs(input: {
    runnerFor: (record: TaskJobRecord) => TaskJobRunner | undefined
    unavailableReason?: string
  }): Promise<TaskJobRehydrateResult> {
    const records = await this.options.store.list()
    const result: TaskJobRehydrateResult = { restarted: [], failed: [] }
    for (const record of records) {
      if (record.status !== 'queued' && record.status !== 'running') continue
      const runner = input.runnerFor(record)
      if (!runner) {
        const failed = await this.failJob(
          record,
          input.unavailableReason ?? 'runtime restart could not rehydrate task job runner'
        )
        result.failed.push(failed)
        continue
      }
      const running = TaskJobRecord.parse({
        ...record,
        status: 'running',
        startedAt: record.startedAt ?? this.now(),
        updatedAt: this.now()
      })
      await this.options.store.upsert(running)
      void this.run(running, runner).catch(() => undefined)
      result.restarted.push(running)
    }
    return result
  }

  private async createRunningRecord(input: TaskJobStartInput): Promise<TaskJobRecord> {
    const now = this.now()
    const record = TaskJobRecord.parse({
      id: this.nextId(input.kind),
      kind: input.kind,
      parentThreadId: input.parentThreadId,
      parentTurnId: input.parentTurnId,
      label: input.label,
      parallelIndex: input.parallelIndex,
      status: 'running',
      dependencies: input.dependencies ?? [],
      permissionPolicy: input.permissionPolicy,
      parentCallId: input.parentCallId,
      childRunId: input.childRunId,
      transcript: input.transcript,
      createdAt: now,
      startedAt: now,
      updatedAt: now
    })
    await this.options.store.upsert(record)
    return record
  }

  private async loadJobs(jobIds: readonly string[]): Promise<TaskJobRecord[]> {
    const records = await Promise.all(jobIds.map((id) => this.options.store.load(id)))
    return records.filter((record): record is TaskJobRecord => Boolean(record))
  }

  private async runningJobIdsForParent(parentThreadId: string | undefined): Promise<string[]> {
    if (!parentThreadId) return []
    const records = await this.options.store.list(parentThreadId)
    return records
      .filter((record) => !isTerminalJobStatus(record.status))
      .map((record) => record.id)
  }

  private canAccess(record: TaskJobRecord, parentThreadId: string | undefined): boolean {
    return !parentThreadId || record.parentThreadId === parentThreadId
  }

  private async run(
    record: TaskJobRecord,
    runner: TaskJobRunner,
    options: TaskJobRunOptions = {}
  ): Promise<TaskJobRecord> {
    const controller = new AbortController()
    const parentSignal = options.parentSignal
    const abortFromParent = (): void => {
      controller.abort(parentSignal?.reason ?? 'cancelled by parent')
    }
    this.controllers.set(record.id, controller)
    if (parentSignal?.aborted) abortFromParent()
    else parentSignal?.addEventListener('abort', abortFromParent, { once: true })
    try {
      const result = await runner({
        jobId: record.id,
        signal: controller.signal,
        appendOutput: (text) => this.appendOutput(record.id, text).then(() => undefined),
        recordChildRun: (childRunId) => this.recordChildRun(record.id, childRunId).then(() => undefined)
      })
      const latest = await this.options.store.load(record.id)
      if (!latest || latest.status === 'killed') return latest ?? record
      const completed = TaskJobRecord.parse({
        ...latest,
        status: 'completed',
        ...(typeof result === 'string' && result ? { result } : {}),
        finishedAt: this.now(),
        updatedAt: this.now()
      })
      await this.options.store.upsert(completed)
      return completed
    } catch (error) {
      const latest = await this.options.store.load(record.id)
      if (!latest || latest.status === 'killed') return latest ?? record
      const failed = TaskJobRecord.parse({
        ...latest,
        status: controller.signal.aborted ? 'killed' : 'failed',
        error: errorMessage(error),
        finishedAt: this.now(),
        updatedAt: this.now()
      })
      await this.options.store.upsert(failed)
      return failed
    } finally {
      parentSignal?.removeEventListener('abort', abortFromParent)
      this.controllers.delete(record.id)
    }
  }

  private nextId(kind: TaskJobRecord['kind']): string {
    if (this.options.idGenerator) return this.options.idGenerator(kind)
    this.seq += 1
    return `${kind}_${this.seq}`
  }

  private now(): string {
    return this.options.nowIso?.() ?? new Date().toISOString()
  }

  private async failJob(record: TaskJobRecord, reason: string): Promise<TaskJobRecord> {
    const failed = TaskJobRecord.parse({
      ...record,
      status: 'failed',
      error: reason,
      finishedAt: record.finishedAt ?? this.now(),
      updatedAt: this.now()
    })
    await this.options.store.upsert(failed)
    return failed
  }
}

function taskJobOutputByteLength(record: TaskJobRecord): number {
  return record.output.reduce((total, chunk) => total + Buffer.byteLength(chunk.text, 'utf8'), 0)
}

export function taskJobControlMetadata(record: TaskJobRecord): TaskJobControlMetadata {
  return {
    id: record.id,
    kind: record.kind,
    parentThreadId: record.parentThreadId,
    parentTurnId: record.parentTurnId,
    ...(record.label !== undefined ? { label: record.label } : {}),
    ...(record.parallelIndex !== undefined ? { parallelIndex: record.parallelIndex } : {}),
    status: record.status,
    dependencies: [...record.dependencies],
    ...(record.parentCallId !== undefined ? { parentCallId: record.parentCallId } : {}),
    ...(record.childRunId !== undefined ? { childRunId: record.childRunId } : {}),
    createdAt: record.createdAt,
    ...(record.startedAt !== undefined ? { startedAt: record.startedAt } : {}),
    ...(record.finishedAt !== undefined ? { finishedAt: record.finishedAt } : {}),
    updatedAt: record.updatedAt
  }
}

function clampTaskJobOutputOffset(offset: number, outputBytes: number): number {
  if (!Number.isFinite(offset) || offset < 0) return 0
  if (offset > outputBytes) return outputBytes
  return Math.trunc(offset)
}

export function normalizeParallelTaskPlan(items: readonly ParallelTaskContractItem[]): ParallelTaskPlanItem[] {
  return items.map((item) => ({
    id: item.id,
    dependsOn: item.dependsOn ?? item.depends_on ?? []
  }))
}

export function validateParallelTaskPlan(items: readonly ParallelTaskPlanItem[]): string[] {
  if (items.length < 2) throw new Error('parallel task plan requires at least two tasks')
  const ids = new Set<string>()
  for (const item of items) {
    if (!item.id.trim()) throw new Error('parallel task ids must be non-empty')
    if (ids.has(item.id)) throw new Error(`duplicate parallel task id: ${item.id}`)
    ids.add(item.id)
  }
  for (const item of items) {
    for (const dependency of item.dependsOn ?? []) {
      if (dependency === item.id) throw new Error(`self-referencing parallel task dependency: ${item.id}`)
      if (!ids.has(dependency)) throw new Error(`unknown parallel task dependency: ${dependency}`)
    }
  }
  const visiting = new Set<string>()
  const visited = new Set<string>()
  const order: string[] = []
  const byId = new Map(items.map((item) => [item.id, item]))
  const visit = (id: string): void => {
    if (visited.has(id)) return
    if (visiting.has(id)) throw new Error(`parallel task dependency cycle includes: ${id}`)
    visiting.add(id)
    for (const dependency of byId.get(id)?.dependsOn ?? []) visit(dependency)
    visiting.delete(id)
    visited.add(id)
    order.push(id)
  }
  for (const item of items) visit(item.id)
  return order
}

export function resolveTranscriptOperation(input: {
  mode: 'continue' | 'fork'
  sourceId: string
  source: TranscriptIdentity
  requested: TranscriptIdentity
  newId?: string
}): TaskJobTranscriptRef {
  if (!sameTranscriptIdentity(input.source, input.requested)) {
    throw new Error('subagent transcript identity is incompatible')
  }
  const identityHash = transcriptIdentityHash(input.source)
  return {
    mode: input.mode,
    sourceId: input.sourceId,
    targetId: input.mode === 'fork' ? input.newId ?? `${input.sourceId}_fork` : input.sourceId,
    identityHash
  }
}

function sameTranscriptIdentity(a: TranscriptIdentity, b: TranscriptIdentity): boolean {
  return transcriptIdentityHash(a) === transcriptIdentityHash(b)
}

function transcriptIdentityHash(identity: TranscriptIdentity): string {
  return stableTranscriptHash({
    modelId: identity.modelId ?? identity.model ?? '',
    providerId: identity.providerId ?? '',
    endpointFormat: identity.endpointFormat ?? '',
    variant: identity.variant ?? '',
    modelSource: identity.modelSource ?? '',
    effort: identity.effort ?? '',
    profile: identity.profile ?? '',
    workspace: identity.workspace ?? '',
    toolPolicy: identity.toolPolicy ?? '',
    toolNames: [...identity.toolNames].sort(),
    systemPromptHash: identity.systemPromptHash ?? '',
    promptPreambleHash: identity.promptPreambleHash ?? '',
    toolSchemaHash: identity.toolSchemaHash ?? '',
    sandboxMode: identity.sandboxMode ?? '',
    approvalPolicy: identity.approvalPolicy ?? '',
    capabilityFingerprint: identity.capabilityFingerprint ?? ''
  })
}

export function stableTranscriptHash(value: unknown): string {
  return createHash('sha256')
    .update(JSON.stringify(canonicalTranscriptValue(value)))
    .digest('hex')
    .slice(0, 16)
}

function canonicalTranscriptValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalTranscriptValue)
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    out[key] = canonicalTranscriptValue((value as Record<string, unknown>)[key])
  }
  return out
}

function isTerminalJobStatus(status: TaskJobStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'interrupted' || status === 'killed'
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function isMissingFileError(error: unknown): boolean {
  return typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code?: unknown }).code === 'ENOENT'
}
