import { z } from 'zod'
import {
  ThreadSummaryResponse,
  ThreadSummaryTaskMutationResponse,
  ThreadSummaryTaskOutputResponse,
  type ThreadRecord,
  type ThreadSummaryBackgroundProcess,
  type ThreadSummaryItemState,
  type ThreadSummarySideChat,
  type ThreadSummarySubagent,
  type ThreadSummaryTask
} from '../../contracts/threads.js'
import {
  normalizeTaskJobKindV1,
  normalizeTaskJobStatusV1,
  taskJobSummaryV1
} from '../../contracts/task-job-output.js'
import type { TurnItem } from '../../contracts/items.js'
import { parseModelEndpointFormat } from '../../contracts/model-endpoint-format.js'
import { ModelExecutionSourceSchema } from '../../contracts/model-execution-ref.js'
import type { ChildRunRecord } from '../../delegation-test-support/delegation-runtime.js'
import {
  taskJobControlMetadata,
  type TaskJobControlMetadata
} from '../../delegation-test-support/job-manager.js'
import { replayRuntimeEvents, type EventSourcedChildRunProjection } from '../../domain/runtime-event-reducer.js'
import { jsonResponse, type JsonResponse } from '../response.js'
import { ERRORS } from './runtime-error.js'
import type { ServerRuntime } from './server-runtime.js'

const TaskOutputQuery = z.object({
  offset: z.preprocess((value) => {
    if (typeof value !== 'string' || value.trim() === '') return undefined
    return Number(value)
  }, z.number().int().nonnegative().optional()),
  limit: z.preprocess((value) => {
    if (typeof value !== 'string' || value.trim() === '') return undefined
    return Number(value)
  }, z.number().int().positive().max(2_000_000).optional())
})

const SUBAGENT_FALLBACK_NAMES = [
  'Lagrange',
  'Nash',
  'Noether',
  'Turing',
  'Euler',
  'Ada',
  'Kepler',
  'Curie',
  'Fermi',
  'Gauss',
  'Hopper',
  'Bohr'
]

type SummaryContext = {
  thread: ThreadRecord
  items: TurnItem[]
  latestSeq: number
  childRuns: EventSourcedChildRunProjection[]
  childRecords: ChildRunRecord[]
  taskJobs: TaskJobControlMetadata[]
  sideThreads: ThreadSummarySideChat[]
  childThreadMeta: Map<string, ChildThreadMeta>
}

type ChildThreadMeta = {
  title?: string
  prompt?: string
}

type CommandTaskGroup = {
  callId: string
  status?: string
  isError?: boolean
  hasResult?: boolean
}

type ThreadSummaryCommandTask = Extract<ThreadSummaryTask, { kind: 'command' }>

const CHILD_OUTPUT_WITHHOLDING_V1 = {
  schemaVersion: 1 as const,
  outputWithheld: true as const,
  outputTrustStatus: 'untrusted_child_output' as const,
  factAnswerAllowed: false as const,
  evidenceAuthority: false as const,
  canContinueParent: false as const,
  canReadOutput: false as const
}

export async function getThreadSummary(runtime: ServerRuntime, threadId: string): Promise<JsonResponse> {
  const context = await loadSummaryContext(runtime, threadId)
  if (!context) return ERRORS.notFound(`thread not found: ${threadId}`)
  return jsonResponse(ThreadSummaryResponse.parse({
    threadId,
    generatedAt: runtime.nowIso(),
    latestSeq: context.latestSeq,
    subagents: buildSubagents(context),
    tasks: buildTasks(context),
    outputs: [],
    sources: [],
    sideChats: context.sideThreads,
    backgroundProcesses: [] satisfies ThreadSummaryBackgroundProcess[]
  }))
}

export async function getThreadSummaryTaskOutput(
  runtime: ServerRuntime,
  threadId: string,
  taskId: string,
  request: Request
): Promise<JsonResponse> {
  const context = await loadSummaryContext(runtime, threadId)
  if (!context) return ERRORS.notFound(`thread not found: ${threadId}`)
  const parsed = TaskOutputQuery.safeParse(Object.fromEntries(new URL(request.url).searchParams.entries()))
  if (!parsed.success) return ERRORS.validation('invalid task output query', parsed.error.flatten())
  if (taskId.startsWith('taskjob:')) {
    if (!runtime.taskJobs) return ERRORS.unavailable('task jobs are not available')
    const jobId = taskId.slice('taskjob:'.length)
    const output = await runtime.taskJobs.metadataForParent(jobId, {
      parentThreadId: threadId
    })
    if (!output.ok) return taskAccessError(output.reason, taskId)
    return jsonResponse(ThreadSummaryTaskOutputResponse.parse({
      schemaVersion: 1,
      availability: 'withheld',
      taskId,
      status: output.value.status,
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }))
  }

  const task = buildCommandTasks(context).find((candidate) => candidate.id === taskId)
  if (!task) return ERRORS.notFound(`summary task not found: ${taskId}`)
  return jsonResponse(ThreadSummaryTaskOutputResponse.parse({
    schemaVersion: 1,
    availability: 'withheld',
    taskId,
    status: task.status,
    reasonCode: 'tool_output_private',
    outputWithheld: true,
    outputTrustStatus: 'private_tool_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false
  }))
}

export async function killThreadSummaryTask(
  runtime: ServerRuntime,
  threadId: string,
  taskId: string
): Promise<JsonResponse> {
  const context = await loadSummaryContext(runtime, threadId)
  if (!context) return ERRORS.notFound(`thread not found: ${threadId}`)
  if (!taskId.startsWith('taskjob:')) {
    return ERRORS.conflict(`summary task can not be killed by the runtime route: ${taskId}`)
  }
  if (!runtime.taskJobs) return ERRORS.unavailable('task jobs are not available')
  const jobId = taskId.slice('taskjob:'.length)
  const result = await runtime.taskJobs.killMetadataForParent(jobId, {
    parentThreadId: threadId,
    reason: 'killed from thread summary'
  })
  if (!result.ok) return taskAccessError(result.reason, taskId)
  const task = taskFromJob(result.value, context)
  return jsonResponse(ThreadSummaryTaskMutationResponse.parse({ task }))
}

export async function restartThreadSummaryTask(
  runtime: ServerRuntime,
  threadId: string,
  taskId: string
): Promise<JsonResponse> {
  const context = await loadSummaryContext(runtime, threadId)
  if (!context) return ERRORS.notFound(`thread not found: ${threadId}`)
  return ERRORS.conflict(`summary task restart requires a new authorized tool call: ${taskId}`)
}

async function loadSummaryContext(runtime: ServerRuntime, threadId: string): Promise<SummaryContext | null> {
  const thread = await runtime.threadService.get(threadId)
  if (!thread) return null
  const [events, latestSeq, sessionItems, childDiagnostics, taskJobs, sideThreads] = await Promise.all([
    runtime.sessionStore.loadEventsSince(threadId, 0).catch(() => []),
    runtime.sessionStore.highestSeq(threadId).catch(() => 0),
    runtime.sessionStore.loadItems(threadId).catch(() => []),
    runtime.delegationRuntime?.diagnostics(threadId).catch(() => null) ?? Promise.resolve(null),
    runtime.taskJobs?.listMetadata(threadId).catch(() => []) ?? Promise.resolve([]),
    runtime.threadService.list({ includeSide: true, includeArchived: true, limit: 500 })
      .then((threads) => threads
        .filter((candidate) => candidate.parentThreadId === threadId && candidate.relation === 'side')
        .map((candidate): ThreadSummarySideChat => ({
          threadId: candidate.id,
          title: candidate.title,
          status: candidate.status,
          relation: 'side',
          parentThreadId: threadId,
          ...(candidate.messageCount !== undefined ? { messageCount: candidate.messageCount } : {}),
          ...(candidate.turnCount !== undefined ? { turnCount: candidate.turnCount } : {}),
          createdAt: candidate.createdAt,
          updatedAt: candidate.updatedAt
        })))
      .catch(() => [])
  ])
  const projection = replayRuntimeEvents(events)
  const childThreadIds = childThreadIdsForSummary(projection.childRuns, childDiagnostics?.childRuns ?? [])
  const childThreadMeta = await loadChildThreadMeta(runtime, childThreadIds, sideThreads)
  return {
    thread,
    items: mergeThreadItems(thread, sessionItems),
    latestSeq,
    childRuns: projection.childRuns,
    childRecords: childDiagnostics?.childRuns ?? [],
    taskJobs,
    sideThreads: sideThreads.filter((side) => !childThreadIds.has(side.threadId)),
    childThreadMeta
  }
}

async function loadChildThreadMeta(
  runtime: ServerRuntime,
  childThreadIds: Set<string>,
  relatedThreads: ThreadSummarySideChat[]
): Promise<Map<string, ChildThreadMeta>> {
  const meta = new Map<string, ChildThreadMeta>()
  for (const thread of relatedThreads) {
    if (!childThreadIds.has(thread.threadId)) continue
    meta.set(thread.threadId, { title: thread.title })
  }
  await Promise.all([...childThreadIds].map(async (threadId) => {
    const thread = await runtime.threadService.get(threadId).catch(() => null)
    if (!thread) return
    const sessionItems = await runtime.sessionStore.loadItems(threadId).catch(() => [])
    const items = mergeThreadItems(thread, sessionItems)
    const current = meta.get(threadId) ?? {}
    meta.set(threadId, {
      ...current,
      title: thread.title || current.title,
      prompt: firstUserPrompt(items) ?? current.prompt
    })
  }))
  return meta
}

function mergeThreadItems(thread: ThreadRecord, sessionItems: TurnItem[]): TurnItem[] {
  const byId = new Map<string, TurnItem>()
  for (const turn of thread.turns) {
    for (const item of turn.items ?? []) byId.set(item.id, item)
  }
  for (const item of sessionItems) byId.set(item.id, item)
  return [...byId.values()].sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id))
}

function buildSubagents(context: SummaryContext): ThreadSummarySubagent[] {
  const byKey = new Map<string, ThreadSummarySubagent>()
  for (const run of context.childRuns) {
    upsertSubagent(byKey, subagentFromProjection(context, run))
  }
  for (const record of context.childRecords) {
    upsertSubagent(byKey, subagentFromRecord(context, record))
  }
  const childThreadIds = subagentChildThreadIds(byKey)
  for (const job of context.taskJobs) {
    const subagent = subagentFromChildTaskJob(context, job)
    if (!subagent) continue
    if (subagent.childThreadId && childThreadIds.has(subagent.childThreadId)) continue
    upsertSubagent(byKey, subagent)
    if (subagent.childThreadId) childThreadIds.add(subagent.childThreadId)
  }
  return [...byKey.values()].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt) || a.id.localeCompare(b.id))
}

function subagentChildThreadIds(subagents: Map<string, ThreadSummarySubagent>): Set<string> {
  const ids = new Set<string>()
  for (const subagent of subagents.values()) {
    if (subagent.childThreadId) ids.add(subagent.childThreadId)
  }
  return ids
}

function childThreadIdsForSummary(
  childRuns: EventSourcedChildRunProjection[],
  childRecords: ChildRunRecord[]
): Set<string> {
  const ids = new Set<string>()
  for (const run of childRuns) {
    if (run.childThreadId) ids.add(run.childThreadId)
  }
  for (const record of childRecords) {
    if (record.childThreadId) ids.add(record.childThreadId)
  }
  return ids
}

function firstUserPrompt(items: TurnItem[]): string | undefined {
  for (const item of items) {
    if (item.kind !== 'user_message') continue
    const text = (item.displayText ?? item.text).trim()
    if (text) return text
  }
  return undefined
}

function subagentDisplayName(
  context: SummaryContext,
  childThreadId: string | undefined,
  input: {
    childName?: string
    label?: string
    profile?: string
    seeds: Array<string | undefined>
    disallowed?: Array<string | undefined>
  }
): string {
  const childName = normalizeExplicitSubagentDisplayLabel(input.childName, input.disallowed)
  if (childName) return childName

  for (const candidate of [input.profile, input.label]) {
    const label = normalizeExplicitSubagentDisplayLabel(candidate, input.disallowed)
    if (label) return label
  }

  if (childThreadId) {
    const meta = context.childThreadMeta.get(childThreadId)
    const title = normalizeChildThreadTitle(meta?.title, input.disallowed)
    if (title) return title
    const prompt = normalizeChildThreadPrompt(meta?.prompt, input.disallowed)
    if (prompt) return prompt
  }

  const profile = normalizeExplicitSubagentDisplayLabel(input.profile, input.disallowed)
  if (profile) return profile

  return generatedSubagentNickname([
    ...input.seeds,
    childThreadId,
    input.childName,
    input.profile,
    input.label
  ])
}

function normalizeExplicitSubagentDisplayLabel(
  value: string | undefined,
  disallowed: Array<string | undefined> = []
): string | undefined {
  let label = value?.trim().replace(/\s+/g, ' ')
  if (!label) return undefined
  if (label.toLowerCase().startsWith('child agent:')) label = label.slice('child agent:'.length).trim()
  if (label.endsWith(' fork')) label = label.slice(0, -' fork'.length).trim()
  if (disallowed.some((candidate) => sameNormalizedSubagentDisplayLabel(label, candidate))) return undefined
  if (!label || isGenericSubagentDisplayLabel(label)) return undefined
  return shortSubagentLabel(label)
}

function normalizeChildThreadTitle(
  value: string | undefined,
  disallowed: Array<string | undefined> = []
): string | undefined {
  let title = value?.trim()
  if (!title) return undefined
  if (title.toLowerCase().startsWith('child agent:')) return undefined
  if (title.endsWith(' fork')) title = title.slice(0, -' fork'.length).trim()
  return normalizeExplicitSubagentDisplayLabel(title, disallowed)
}

function normalizeChildThreadPrompt(
  value: string | undefined,
  disallowed: Array<string | undefined> = []
): string | undefined {
  const prompt = value?.trim().replace(/\s+/g, ' ')
  if (!prompt) return undefined
  if (disallowed.some((candidate) => sameNormalizedSubagentDisplayLabel(prompt, candidate))) return undefined
  if (isGenericSubagentDisplayLabel(prompt)) return undefined
  return shortSubagentLabel(prompt)
}

function sameNormalizedSubagentDisplayLabel(
  left: string | undefined,
  right: string | undefined
): boolean {
  const a = left?.trim().replace(/\s+/g, ' ').toLowerCase()
  const b = right?.trim().replace(/\s+/g, ' ').toLowerCase()
  return Boolean(a && b && a === b)
}

function shortSubagentLabel(value: string | undefined): string | undefined {
  const text = value?.trim().replace(/\s+/g, ' ')
  if (!text) return undefined
  return text.length > 48 ? `${text.slice(0, 48)}...` : text
}

function isGenericSubagentDisplayLabel(value: string): boolean {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/^child agent:\s*/, '')
  return new Set([
    '',
    'task',
    'delegate_task',
    'subagent',
    'child-run',
    'child run',
    'parallel-child-run'
  ]).has(normalized)
}

function generatedSubagentNickname(candidates: Array<string | undefined>): string {
  const seed = candidates
    .map((candidate) => candidate?.trim())
    .find((candidate): candidate is string => Boolean(candidate))
  if (!seed) return 'Subagent'
  const ordinal = seed.match(/(\d+)(?!.*\d)/)?.[1]
  if (ordinal) {
    const value = Number.parseInt(ordinal, 10)
    if (Number.isFinite(value) && value > 0) {
      return SUBAGENT_FALLBACK_NAMES[(value - 1) % SUBAGENT_FALLBACK_NAMES.length]
    }
  }
  return SUBAGENT_FALLBACK_NAMES[hashSubagentSeed(seed) % SUBAGENT_FALLBACK_NAMES.length]
}

function hashSubagentSeed(value: string): number {
  let hash = 2_166_136_261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16_777_619) >>> 0
  }
  return hash
}

function upsertSubagent(target: Map<string, ThreadSummarySubagent>, next: ThreadSummarySubagent): void {
  const existing = target.get(next.key)
  if (!existing || next.updatedAt >= existing.updatedAt) {
    target.set(next.key, { ...existing, ...next })
  }
}

function subagentFromProjection(context: SummaryContext, run: EventSourcedChildRunProjection): ThreadSummarySubagent {
  const key = childKey({
    childRunId: run.childRunId,
    childThreadId: run.childThreadId,
    childId: run.childId,
    parentToolCallId: run.parentToolCallId
  })
  const displayName = subagentDisplayName(context, run.childThreadId, {
    childName: run.name,
    label: run.label,
    profile: run.profile,
    seeds: [
      parallelIndexSeed(run.parallelIndex),
      run.childRunId,
      run.childId,
      run.parentToolCallId,
      distinctSubagentNameCandidate(run.name, run.label, run.profile)
    ]
  })
  const rawStatus = publicSummaryStatus(run.status)
  const endpointFormat = parseModelEndpointFormat(run.endpointFormat)
  const modelSource = ModelExecutionSourceSchema.safeParse(run.modelSource)
  return {
    ...CHILD_OUTPUT_WITHHOLDING_V1,
    id: key,
    key,
    parentThreadId: run.parentThreadId,
    parentTurnId: run.parentTurnId,
    ...(run.parentToolCallId ? { parentToolCallId: run.parentToolCallId } : {}),
    childId: run.childId,
    ...(run.childRunId ? { childRunId: run.childRunId } : {}),
    ...(run.childThreadId ? { childThreadId: run.childThreadId, canOpenThread: true } : { canOpenThread: false }),
    canKill: false,
    canRestart: false,
    ...(run.childTurnId ? { childTurnId: run.childTurnId } : {}),
    displayName,
    agentNickname: displayName,
    label: displayName,
    title: displayName,
    ...(run.model ? { model: run.model } : {}),
    ...(run.providerId ? { providerId: run.providerId } : {}),
    ...(endpointFormat ? { endpointFormat } : {}),
    ...(run.variant ? { variant: run.variant } : {}),
    ...(modelSource.success ? { modelSource: modelSource.data } : {}),
    ...(run.effort ? { effort: run.effort } : {}),
    ...(run.profile ? { profile: run.profile } : {}),
    ...(run.toolPolicy ? { toolPolicy: run.toolPolicy } : {}),
    status: normalizeActiveStatus(rawStatus),
    rawStatus,
    ...(run.background !== undefined ? { background: run.background } : {}),
    ...(run.parallelGroupId ? { parallelGroupId: run.parallelGroupId } : {}),
    ...(run.parallelIndex !== undefined ? { parallelIndex: run.parallelIndex } : {}),
    ...(run.toolInvocations !== undefined ? { toolInvocations: run.toolInvocations } : {}),
    ...(run.evidenceLedgered !== undefined ? { evidenceLedgered: run.evidenceLedgered } : {}),
    ...(run.durationMs !== undefined ? { durationMs: run.durationMs } : {}),
    ...(run.queuedMs !== undefined ? { queuedMs: run.queuedMs } : {}),
    ...(run.totalTokens !== undefined ? { totalTokens: run.totalTokens } : {}),
    ...(run.cacheHitRate !== undefined ? { cacheHitRate: run.cacheHitRate } : {}),
    ...(run.costUsd !== undefined ? { costUsd: run.costUsd } : {}),
    ...(run.costCny !== undefined ? { costCny: run.costCny } : {}),
    updatedAt: run.updatedAt
  }
}

function subagentFromRecord(context: SummaryContext, record: ChildRunRecord): ThreadSummarySubagent {
  const key = childKey({
    childRunId: record.id,
    childThreadId: record.childThreadId,
    childId: record.id,
    parentToolCallId: record.parentToolCallId
  })
  const displayName = subagentDisplayName(context, record.childThreadId, {
    label: record.label,
    profile: record.profile,
    seeds: [
      parallelIndexSeed((record as ChildRunRecord & { parallelIndex?: number }).parallelIndex),
      record.id,
      record.parentToolCallId
    ]
  })
  const rawStatus = publicSummaryStatus(record.status)
  const endpointFormat = parseModelEndpointFormat(record.endpointFormat)
  const modelSource = ModelExecutionSourceSchema.safeParse(record.modelSource)
  return {
    ...CHILD_OUTPUT_WITHHOLDING_V1,
    id: key,
    key,
    parentThreadId: record.parentThreadId,
    parentTurnId: record.parentTurnId,
    ...(record.parentToolCallId ? { parentToolCallId: record.parentToolCallId } : {}),
    childId: record.id,
    childRunId: record.id,
    ...(record.childThreadId ? { childThreadId: record.childThreadId, canOpenThread: true } : { canOpenThread: false }),
    canKill: false,
    canRestart: false,
    ...(record.childTurnId ? { childTurnId: record.childTurnId } : {}),
    displayName,
    agentNickname: displayName,
    label: displayName,
    title: displayName,
    ...(record.model ? { model: record.model } : {}),
    ...(record.providerId ? { providerId: record.providerId } : {}),
    ...(endpointFormat ? { endpointFormat } : {}),
    ...(record.variant ? { variant: record.variant } : {}),
    ...(modelSource.success ? { modelSource: modelSource.data } : {}),
    ...(record.effort ? { effort: record.effort } : {}),
    ...(record.profile ? { profile: record.profile } : {}),
    ...(record.toolPolicy ? { toolPolicy: record.toolPolicy } : {}),
    status: normalizeActiveStatus(rawStatus),
    rawStatus,
    ...(record.toolInvocations !== undefined ? { toolInvocations: record.toolInvocations } : {}),
    ...(record.evidenceLedgered !== undefined ? { evidenceLedgered: record.evidenceLedgered } : {}),
    ...(record.durationMs !== undefined ? { durationMs: record.durationMs } : {}),
    ...(record.queuedMs !== undefined ? { queuedMs: record.queuedMs } : {}),
    ...(record.usage.totalTokens !== undefined ? { totalTokens: record.usage.totalTokens } : {}),
    ...(record.usage.cacheHitRate !== undefined ? { cacheHitRate: record.usage.cacheHitRate } : {}),
    ...(record.usage.costUsd !== undefined ? { costUsd: record.usage.costUsd } : {}),
    ...(record.usage.costCny !== undefined ? { costCny: record.usage.costCny } : {}),
    createdAt: record.createdAt,
    updatedAt: record.updatedAt
  }
}

function subagentFromChildTaskJob(context: SummaryContext, job: TaskJobControlMetadata): ThreadSummarySubagent | null {
  if (job.kind !== 'task' && job.kind !== 'parallel_task') return null
  const childThreadId = job.childRunId ? childThreadIdForRun(context, job.childRunId) : undefined
  if (!childThreadId) return null
  const childId = job.childRunId ?? job.id
  const key = childKey({
    childRunId: childId,
    childThreadId,
    childId,
    parentToolCallId: job.parentCallId
  })
  const displayName = subagentDisplayName(context, childThreadId, {
    label: job.label,
    seeds: [parallelIndexSeed(job.parallelIndex), childId, job.id, job.parentCallId]
  })
  const rawStatus = publicSummaryStatus(job.status)
  return {
    ...CHILD_OUTPUT_WITHHOLDING_V1,
    id: key,
    key,
    parentThreadId: job.parentThreadId,
    parentTurnId: job.parentTurnId,
    ...(job.parentCallId ? { parentToolCallId: job.parentCallId } : {}),
    childId,
    childRunId: childId,
    taskJobId: job.id,
    taskKind: normalizeTaskJobKindV1(job.kind),
    childThreadId,
    canOpenThread: true,
    canKill: false,
    canRestart: false,
    displayName,
    agentNickname: displayName,
    label: displayName,
    title: displayName,
    status: normalizeTaskStatus(rawStatus),
    rawStatus,
    ...(job.kind === 'parallel_task' ? { background: true } : {}),
    ...(job.parallelIndex !== undefined ? { parallelIndex: job.parallelIndex } : {}),
    createdAt: job.createdAt,
    updatedAt: job.updatedAt
  }
}

function distinctSubagentNameCandidate(
  name: string | undefined,
  label: string | undefined,
  profile: string | undefined
): string | undefined {
  const normalizedName = name?.trim().replace(/\s+/g, ' ')
  const normalizedLabel = label?.trim().replace(/\s+/g, ' ')
  const normalizedProfile = profile?.trim().replace(/\s+/g, ' ')
  if (!normalizedName) return undefined
  if (normalizedLabel && normalizedName === normalizedLabel) return undefined
  if (normalizedProfile && normalizedName === normalizedProfile) return undefined
  return normalizedName
}

function parallelIndexSeed(value: number | undefined): string | undefined {
  return value !== undefined && Number.isFinite(value) && value > 0 ? String(value) : undefined
}

function buildTasks(context: SummaryContext): ThreadSummaryTask[] {
  const byKey = new Map<string, ThreadSummaryTask>()
  for (const job of context.taskJobs) {
    const task = taskFromJob(job, context)
    byKey.set(task.id, task)
  }
  for (const task of buildCommandTasks(context)) {
    byKey.set(task.id, task)
  }
  return [...byKey.values()].sort((a, b) => a.id.localeCompare(b.id))
}

function taskFromJob(job: TaskJobControlMetadata, context: SummaryContext): ThreadSummaryTask {
  void context
  return taskJobSummaryV1({
    id: `taskjob:${job.id}`,
    kind: job.kind,
    status: job.status,
    background: job.kind === 'parallel_task'
  })
}

function childThreadIdForRun(context: SummaryContext, childRunId: string): string | undefined {
  const fromRecord = context.childRecords.find((record) => record.id === childRunId)?.childThreadId
  if (fromRecord) return fromRecord
  return context.childRuns.find((run) => run.childRunId === childRunId)?.childThreadId
}

function buildCommandTasks(context: SummaryContext): ThreadSummaryTask[] {
  const groups = new Map<string, CommandTaskGroup>()
  for (const item of context.items) {
    if (item.kind !== 'tool_call' && item.kind !== 'tool_result') continue
    if (item.toolKind !== 'command_execution' && item.toolName !== 'bash') continue
    const callId = item.callId || item.id
    const existing = groups.get(callId) ?? { callId }
    if (item.kind === 'tool_result') {
      groups.set(callId, { ...existing, status: item.status, isError: item.isError, hasResult: true })
    } else if (!existing.hasResult) {
      groups.set(callId, { ...existing, status: item.status })
    }
  }
  return [...groups.values()].map((group): ThreadSummaryCommandTask => {
    let rawStatus: ThreadSummaryCommandTask['status'] = 'pending'
    if (group.hasResult) {
      rawStatus = group.isError
        ? 'error'
        : group.status?.trim()
          ? publicCommandStatus(group.status, false)
          : 'completed'
    } else {
      const current = publicCommandStatus(group.status, false)
      if (current === 'running' || current === 'pending') rawStatus = current
    }
    const id = `command:${group.callId}`
    return {
      schemaVersion: 1,
      id,
      kind: 'command',
      status: rawStatus,
      background: false,
      active: normalizeCommandStatus(rawStatus, group.isError) === 'active',
      terminal: ['done', 'terminal'].includes(normalizeCommandStatus(rawStatus, group.isError)),
      outputWithheld: true,
      outputTrustStatus: 'private_tool_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canContinueParent: false,
      canReadOutput: false
    }
  })
}

function childKey(input: {
  childRunId?: string
  childThreadId?: string
  childId?: string
  parentToolCallId?: string
}): string {
  return input.childRunId
    ? `run:${input.childRunId}`
    : input.childThreadId
      ? `thread:${input.childThreadId}`
      : input.childId
        ? `child:${input.childId}`
        : `call:${input.parentToolCallId ?? 'unknown'}`
}

function normalizeActiveStatus(status: string): ThreadSummaryItemState {
  if (childRunCanKill(status) || status === 'starting') return 'active'
  if (status === 'completed' || status === 'done') return 'done'
  if (status === 'failed' || status === 'aborted' || status === 'interrupted' || status === 'killed' || status === 'canceled' || status === 'timeout') return 'terminal'
  return 'inactive'
}

function childRunCanKill(status: string): boolean {
  return status === 'queued' ||
    status === 'running' ||
    status === 'pause_requested' ||
    status === 'paused' ||
    status === 'resume_requested' ||
    status === 'resuming'
}

function normalizeTaskStatus(status: string): ThreadSummaryItemState {
  return normalizeActiveStatus(status)
}

function normalizeCommandStatus(status: string, isError?: boolean): ThreadSummaryItemState {
  if (status === 'running' || status === 'pending') return 'active'
  if (status === 'stopped') return 'inactive'
  if (isError || status === 'failed' || status === 'aborted' || status === 'killed' || status === 'canceled' || status === 'timeout' || status === 'error') return 'terminal'
  if (status === 'completed' || status === 'done' || status === 'success') return 'done'
  return 'inactive'
}

function publicSummaryStatus(status: unknown): ThreadSummarySubagent['rawStatus'] {
  const normalized = normalizeTaskJobStatusV1(status)
  if (normalized !== 'unknown') return normalized
  if (typeof status !== 'string') return 'unknown'
  switch (status.trim()) {
    case 'starting':
    case 'done':
    case 'missing':
    case 'stopped':
    case 'archived':
    case 'idle':
      return status.trim() as ThreadSummarySubagent['rawStatus']
    default:
      return 'unknown'
  }
}

function publicCommandStatus(status: unknown, isError?: boolean): ThreadSummaryCommandTask['status'] {
  if (typeof status !== 'string' || !status.trim()) return isError ? 'error' : 'unknown'
  switch (status.trim()) {
    case 'running':
    case 'pending':
    case 'stopped':
    case 'failed':
    case 'aborted':
    case 'killed':
    case 'canceled':
    case 'timeout':
    case 'error':
    case 'completed':
    case 'done':
    case 'success':
      return status.trim() as ThreadSummaryCommandTask['status']
    default:
      return 'unknown'
  }
}

function taskAccessError(reason: 'not_found' | 'forbidden', id: string): JsonResponse {
  if (reason === 'forbidden') return ERRORS.forbidden(`summary task does not belong to this thread: ${id}`)
  return ERRORS.notFound(`summary task not found: ${id}`)
}
