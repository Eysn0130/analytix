import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { z } from 'zod'
import { stableTranscriptHash } from './job-manager.js'
import {
  SubagentReasoningEffort,
  SubagentToolPolicy,
  type SubagentsCapabilityConfig
} from '../contracts/capabilities.js'
import type { RuntimeEventRecorder } from '../services-test-support/runtime-event-recorder.js'
import { CacheDiagnosticsSchema, type CacheDiagnostics } from '../contracts/events.js'
import {
  ModelExecutionRefSchema,
  type ModelExecutionRef,
  type ModelExecutionSource
} from '../contracts/model-execution-ref.js'
import type { UsageSnapshot } from '../contracts/usage.js'
import { TurnItem, type TurnItem as TurnItemType } from '../contracts/items.js'

const ChildRunUsage = z.object({
  promptTokens: z.number().int().nonnegative().default(0),
  completionTokens: z.number().int().nonnegative().default(0),
  reasoningTokens: z.number().int().nonnegative().optional(),
  totalTokens: z.number().int().nonnegative().default(0),
  cachedTokens: z.number().int().nonnegative().optional(),
  cacheHitTokens: z.number().int().nonnegative().optional(),
  cacheMissTokens: z.number().int().nonnegative().optional(),
  cacheHitRate: z.number().min(0).max(1).nullable().optional(),
  cacheableTokenHitRate: z.number().min(0).max(1).nullable().optional(),
  totalInputTokenHitRate: z.number().min(0).max(1).nullable().optional(),
  cacheMissReasons: z.array(z.string()).optional(),
  cacheSuggestions: z.array(z.string()).optional(),
  turns: z.number().int().nonnegative().optional(),
  priceConfigured: z.boolean().optional(),
  costUsd: z.number().nonnegative().optional(),
  costCny: z.number().nonnegative().optional(),
  cacheSavingsUsd: z.number().nonnegative().optional(),
  cacheSavingsCny: z.number().nonnegative().optional(),
  tokenEconomySavingsTokens: z.number().int().nonnegative().optional(),
  tokenEconomySavingsUsd: z.number().nonnegative().optional(),
  tokenEconomySavingsCny: z.number().nonnegative().optional()
})

export const ChildRunRecord = z.object({
  id: z.string().min(1),
  parentThreadId: z.string().min(1),
  parentTurnId: z.string().min(1),
  parentToolCallId: z.string().min(1).optional(),
  childThreadId: z.string().min(1).optional(),
  childTurnId: z.string().min(1).optional(),
  label: z.string().optional(),
  prompt: z.string().min(1),
  workspace: z.string().optional(),
  model: z.string().optional(),
  providerId: z.string().optional(),
  endpointFormat: z.string().optional(),
  variant: z.string().optional(),
  modelSource: z.enum(['thread', 'subagent-profile', 'explicit-input', 'session', 'runtime-default']).optional(),
  modelExecution: ModelExecutionRefSchema.optional(),
  effort: SubagentReasoningEffort.optional(),
  maxModelSteps: z.number().int().nonnegative().optional(),
  /** Resolved subagent profile name, when one was selected. */
  profile: z.string().optional(),
  /** Effective tool policy applied to the child (read-only vs inherited). */
  toolPolicy: SubagentToolPolicy.optional(),
  /** Effective explicit tool scope, when the profile or task narrowed it. */
  toolScope: z.array(z.string().min(1)).optional(),
  promptPreambleHash: z.string().optional(),
  approvalPolicy: z.string().optional(),
  sandboxMode: z.string().optional(),
  status: z.enum(['queued', 'running', 'completed', 'failed', 'aborted']),
  summary: z.string().optional(),
  error: z.string().optional(),
  usage: ChildRunUsage.default({ promptTokens: 0, completionTokens: 0, totalTokens: 0 }),
  /** True when the child reused the main agent's cached stable prefix. */
  prefixReused: z.boolean().optional(),
  /** Parent history items seeded into the child (0 = prefix-only). */
  inheritedHistoryItems: z.number().int().nonnegative().optional(),
  /** Tool calls the child executed during its run. */
  toolInvocations: z.number().int().nonnegative().optional(),
  /** Durable normalized child transcript items captured for continue/fork. */
  transcriptItems: z.array(TurnItem).optional(),
  /** True when the completed child was attached to the parent goal evidence ledger. */
  evidenceLedgered: z.boolean().optional(),
  /** Non-fatal error from attempting to attach the child to parent evidence. */
  evidenceLedgerError: z.string().optional(),
  /** Cache diagnostics from the child runtime usage event, when available. */
  cacheDiagnostics: CacheDiagnosticsSchema.optional(),
  /** Wall-clock spent running (after leaving the queue). */
  durationMs: z.number().int().nonnegative().optional(),
  /** Wall-clock spent waiting for a parallel slot before starting. */
  queuedMs: z.number().int().nonnegative().optional(),
  createdAt: z.string(),
  /** When the child left the queue and began running. */
  startedAt: z.string().optional(),
  updatedAt: z.string()
}).strict()
export type ChildRunRecord = z.infer<typeof ChildRunRecord>

export type ChildRunExecutor = (input: {
  childId: string
  parentThreadId: string
  parentTurnId: string
  parentToolCallId?: string
  label?: string
  prompt: string
  workspace?: string
  model?: string
  modelExecution?: ModelExecutionRef
  effort?: SubagentReasoningEffort
  maxModelSteps?: number
  toolPolicy: SubagentToolPolicy
  toolScope?: string[]
  approvalPolicy?: string
  sandboxMode?: string
  promptPreamble?: string
  transcript?: {
    mode: 'continue' | 'fork'
    sourceId?: string
    targetId?: string
    identityHash: string
  }
  signal: AbortSignal
}) => Promise<{
  summary: string
  usage?: ChildRunRecord['usage']
  childThreadId?: string
  childTurnId?: string
  cacheDiagnostics?: CacheDiagnostics
  toolInvocations?: number
  transcriptItems?: TurnItemType[]
  prefixReused?: boolean
  inheritedHistoryItems?: number
}>

export type ChildRunAggregate = {
  key: string
  label?: string
  model?: string
  effort?: SubagentReasoningEffort
  runs: number
  completed: number
  failed: number
  aborted: number
  promptTokens: number
  completionTokens: number
  reasoningTokens: number
  totalTokens: number
  costUsd?: number
  costCny?: number
  averageTotalTokens: number
  averageCostUsd?: number
  averageCostCny?: number
}

export class FileDelegationStore {
  constructor(private readonly rootDir: string) {}

  async upsert(record: ChildRunRecord): Promise<void> {
    await mkdir(this.rootDir, { recursive: true })
    await writeFile(join(this.rootDir, `${record.id}.json`), JSON.stringify(record, null, 2), 'utf8')
  }

  async load(id: string): Promise<ChildRunRecord | undefined> {
    try {
      return ChildRunRecord.parse(JSON.parse(await readFile(join(this.rootDir, `${id}.json`), 'utf8')))
    } catch {
      return undefined
    }
  }

  async list(parentThreadId?: string): Promise<ChildRunRecord[]> {
    await mkdir(this.rootDir, { recursive: true })
    const entries = await readdir(this.rootDir).catch(() => [])
    const records = await Promise.all(entries
      .filter((entry) => entry.endsWith('.json'))
      .map((entry) => readFile(join(this.rootDir, entry), 'utf8')
        .then((text) => ChildRunRecord.parse(JSON.parse(text)))
        .catch(() => null)))
    return records
      .filter((record): record is ChildRunRecord => Boolean(record))
      .filter((record) => !parentThreadId || record.parentThreadId === parentThreadId)
      .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
  }
}

type SlotWaiter = {
  resolve: () => void
  reject: (error: unknown) => void
  signal: AbortSignal
  onAbort: () => void
}

export class DelegationRuntime {
  private active = 0
  private childSeq = 0
  /** Children waiting for a parallel slot, in FIFO order. */
  private readonly slotWaiters: SlotWaiter[] = []
  /** Per-thread child counts (persisted + in-flight) for the budget cap. */
  private readonly threadCounts = new Map<string, number>()
  /** Cached per-thread seed reads so concurrent first-spawns don't double-count. */
  private readonly threadSeeds = new Map<string, Promise<void>>()

  constructor(private readonly options: {
    config: SubagentsCapabilityConfig
    store: FileDelegationStore
    events?: RuntimeEventRecorder
    nowIso?: () => string
    idGenerator?: () => string
    executor?: ChildRunExecutor
    recordExternalUsage?: (
      threadId: string,
      usage: UsageSnapshot,
      attribution: { usageSource: 'subagent'; childRunId: string }
    ) => UsageSnapshot | void
    recordParentEvidence?: (record: ChildRunRecord) => Promise<boolean>
  }) {}

  async runChild(input: {
    parentThreadId: string
    parentTurnId: string
    parentToolCallId?: string
    label?: string
    prompt: string
    workspace?: string
    model?: string
    modelExecution?: ModelExecutionRef
    effort?: string
    maxModelSteps?: number
    profile?: string
    tools?: string[]
    approvalPolicy?: string
    sandboxMode?: string
    transcript?: {
      mode: 'continue' | 'fork'
      sourceId?: string
      targetId?: string
      identityHash: string
    }
    onChildRunStarted?: (record: ChildRunRecord) => Promise<void> | void
    signal: AbortSignal
  }): Promise<ChildRunRecord> {
    const config = this.options.config
    if (!config.enabled) throw new Error('delegation is disabled by config')

    // Resolve the profile up front so model/preamble/tool-policy are
    // captured on the record even if the child later fails.
    const profileName = input.profile?.trim() || config.defaultProfile
    const profile = profileName ? config.profiles[profileName] : undefined
    if (profileName && !profile) {
      throw new Error(`unknown subagent profile: ${profileName}`)
    }
    const toolPolicy = profile?.toolPolicy ?? config.defaultToolPolicy
    const toolScope = normalizeToolScope(input.tools?.length ? input.tools : profile?.tools)
    const explicitModel = input.model?.trim()
    const profileProviderId = profile?.providerId?.trim()
    const profileModel = profile?.model?.trim()
    const profileVariant = profile?.variant?.trim()
    const profileEndpointFormat = profile?.endpointFormat
    const inheritedModel = input.modelExecution?.modelId?.trim()
    const inheritedProviderId = input.modelExecution?.providerId?.trim()
    const inheritedVariant = input.modelExecution?.variant?.trim()
    const inheritedEndpointFormat = input.modelExecution?.endpointFormat
    const resolvedModel = explicitModel || profileModel || inheritedModel
    const resolvedProviderId = profileProviderId || inheritedProviderId
    const resolvedVariant = profileVariant || inheritedVariant
    const resolvedEndpointFormat = profileEndpointFormat || inheritedEndpointFormat
    const modelSource = resolveModelSource({
      explicitModel,
      profileModel,
      profileProviderId,
      profileVariant,
      profileEndpointFormat,
      inherited: input.modelExecution
    })
    if (!resolvedProviderId) {
      throw new Error('provider_not_found: subagent requires an explicit, profile, or inherited providerId')
    }
    if (!resolvedModel) {
      throw new Error('model_not_found: subagent requires an explicit, profile, or inherited modelId')
    }
    const resolvedEffort = normalizeSubagentEffort(input.effort) ?? profile?.effort
    const promptPreamble = profile?.promptPreamble

    // Reserve against the per-thread budget before persisting anything.
    await this.ensureSeeded(input.parentThreadId)
    if (input.transcript?.mode !== 'continue' && !this.reserveChild(input.parentThreadId)) {
      throw new Error('delegation child-run budget exhausted')
    }

    const queuedAt = this.now()
    const inheritedModelExecution = input.modelExecution
      ? stripModelExecutionForResolution(input.modelExecution, Boolean(
          explicitModel ||
          profileModel ||
          profileProviderId ||
          profileVariant ||
          profileEndpointFormat
        ))
      : undefined
    const resolvedModelExecution = resolvedModel
      ? ModelExecutionRefSchema.parse({
          ...(inheritedModelExecution ?? {}),
          modelId: resolvedModel,
          ...(resolvedProviderId ? { providerId: resolvedProviderId } : {}),
          ...(resolvedVariant ? { variant: resolvedVariant } : {}),
          ...(resolvedEndpointFormat ? { endpointFormat: resolvedEndpointFormat } : {}),
          source: modelSource,
          resolvedAt: queuedAt
        })
      : undefined
    const id = input.transcript?.targetId?.trim() ||
      this.options.idGenerator?.() ||
      `child_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
    let record = ChildRunRecord.parse({
      id,
      parentThreadId: input.parentThreadId,
      parentTurnId: input.parentTurnId,
      ...(input.parentToolCallId ? { parentToolCallId: input.parentToolCallId } : {}),
      childThreadId: id,
      label: input.label,
      prompt: input.prompt,
      workspace: input.workspace,
      model: resolvedModel,
      ...(resolvedModelExecution?.providerId ? { providerId: resolvedModelExecution.providerId } : {}),
      ...(resolvedModelExecution?.endpointFormat ? { endpointFormat: resolvedModelExecution.endpointFormat } : {}),
      ...(resolvedModelExecution?.variant ? { variant: resolvedModelExecution.variant } : {}),
      ...(resolvedModelExecution?.source ? { modelSource: resolvedModelExecution.source } : {}),
      ...(resolvedModelExecution ? { modelExecution: resolvedModelExecution } : {}),
      effort: resolvedEffort,
      ...(input.maxModelSteps !== undefined ? { maxModelSteps: input.maxModelSteps } : {}),
      profile: profileName,
      toolPolicy,
      ...(toolScope.length > 0 ? { toolScope } : {}),
      ...(promptPreamble ? { promptPreambleHash: stableTranscriptHash(promptPreamble) } : {}),
      ...(input.approvalPolicy ? { approvalPolicy: input.approvalPolicy } : {}),
      ...(input.sandboxMode ? { sandboxMode: input.sandboxMode } : {}),
      status: 'queued',
      createdAt: queuedAt,
      updatedAt: queuedAt
    })
    await this.options.store.upsert(record)
    await this.recordChildEvent(record)
    await input.onChildRunStarted?.(record)

    try {
      await this.acquireSlot(input.signal)
    } catch (error) {
      // Aborted while still queued — never started, so no slot to release.
      record = ChildRunRecord.parse({
        ...record,
        status: 'aborted',
        error: errorMessage(error),
        updatedAt: this.now()
      })
      await this.options.store.upsert(record)
      await this.recordChildEvent(record)
      return record
    }

    const startedAt = this.now()
    const queuedMs = elapsedMs(queuedAt, startedAt)
    record = ChildRunRecord.parse({ ...record, status: 'running', startedAt, queuedMs, updatedAt: startedAt })
    await this.options.store.upsert(record)
    await this.recordChildEvent(record)
    try {
      const executor: ChildRunExecutor = this.options.executor ?? defaultExecutor
      const result = await executor({
        childId: id,
        parentThreadId: input.parentThreadId,
        parentTurnId: input.parentTurnId,
        ...(input.parentToolCallId ? { parentToolCallId: input.parentToolCallId } : {}),
        ...(input.label ? { label: input.label } : {}),
        prompt: input.prompt,
        workspace: input.workspace,
        model: resolvedModel,
        ...(resolvedModelExecution ? { modelExecution: resolvedModelExecution } : {}),
        ...(resolvedEffort ? { effort: resolvedEffort } : {}),
        ...(input.maxModelSteps !== undefined ? { maxModelSteps: input.maxModelSteps } : {}),
        toolPolicy,
        ...(toolScope.length > 0 ? { toolScope } : {}),
        ...(input.approvalPolicy ? { approvalPolicy: input.approvalPolicy } : {}),
        ...(input.sandboxMode ? { sandboxMode: input.sandboxMode } : {}),
        ...(promptPreamble ? { promptPreamble } : {}),
        ...(input.transcript ? { transcript: input.transcript } : {}),
        signal: input.signal
      })
      const finishedAt = this.now()
      const completedRecord = ChildRunRecord.parse({
        ...record,
        status: 'completed',
        summary: result.summary,
        usage: result.usage ?? record.usage,
        childThreadId: result.childThreadId ?? record.childThreadId,
        ...(result.childTurnId ? { childTurnId: result.childTurnId } : {}),
        ...(result.cacheDiagnostics ? { cacheDiagnostics: result.cacheDiagnostics } : {}),
        toolInvocations: result.toolInvocations,
        ...(result.transcriptItems?.length ? { transcriptItems: result.transcriptItems } : {}),
        prefixReused: result.prefixReused,
        inheritedHistoryItems: result.inheritedHistoryItems,
        durationMs: elapsedMs(startedAt, finishedAt),
        updatedAt: finishedAt
      })
      const evidence = await this.recordParentEvidence(completedRecord)
      record = ChildRunRecord.parse({ ...completedRecord, ...evidence })
      await this.options.store.upsert(record)
      await this.recordChildEvent(record)
      await this.recordExternalUsage(record)
      return record
    } catch (error) {
      const finishedAt = this.now()
      record = ChildRunRecord.parse({
        ...record,
        status: input.signal.aborted ? 'aborted' : 'failed',
        error: errorMessage(error),
        durationMs: elapsedMs(startedAt, finishedAt),
        updatedAt: finishedAt
      })
      await this.options.store.upsert(record)
      await this.recordChildEvent(record)
      return record
    } finally {
      this.releaseSlot()
    }
  }

  /** Concurrency ceiling; clamps to at least 1 so an enabled runtime never deadlocks. */
  private get parallelLimit(): number {
    return Math.max(1, this.options.config.maxParallel)
  }

  /** Acquire a parallel slot, queueing (FIFO) when the runtime is saturated. */
  private acquireSlot(signal: AbortSignal): Promise<void> {
    if (signal.aborted) return Promise.reject(new Error('aborted while queued'))
    if (this.active < this.parallelLimit) {
      this.active += 1
      return Promise.resolve()
    }
    return new Promise<void>((resolve, reject) => {
      const waiter: SlotWaiter = {
        resolve,
        reject,
        signal,
        onAbort: () => {
          const index = this.slotWaiters.indexOf(waiter)
          if (index >= 0) this.slotWaiters.splice(index, 1)
          reject(new Error('aborted while queued'))
        }
      }
      signal.addEventListener('abort', waiter.onAbort, { once: true })
      this.slotWaiters.push(waiter)
    })
  }

  /** Hand the freed slot to the next waiter, or shrink the active count. */
  private releaseSlot(): void {
    const next = this.slotWaiters.shift()
    if (next) {
      next.signal.removeEventListener('abort', next.onAbort)
      next.resolve() // slot is handed over directly; `active` stays the same
    } else {
      this.active = Math.max(0, this.active - 1)
    }
  }

  /** Seed the per-thread budget counter from persisted records exactly once. */
  private ensureSeeded(threadId: string): Promise<void> {
    let seed = this.threadSeeds.get(threadId)
    if (!seed) {
      seed = this.options.store
        .list(threadId)
        .then((runs) => {
          if (!this.threadCounts.has(threadId)) this.threadCounts.set(threadId, runs.length)
        })
        .catch(() => {
          if (!this.threadCounts.has(threadId)) this.threadCounts.set(threadId, 0)
        })
      this.threadSeeds.set(threadId, seed)
    }
    return seed
  }

  /** Atomically reserve a budget slot; returns false when the cap is reached. */
  private reserveChild(threadId: string): boolean {
    const used = this.threadCounts.get(threadId) ?? 0
    if (used >= this.options.config.maxChildRuns) return false
    this.threadCounts.set(threadId, used + 1)
    return true
  }

  /** Configured profiles, surfaced to the delegate_task tool schema/UI. */
  listProfiles(): {
    name: string
    toolPolicy: SubagentToolPolicy
    providerId?: string
    model?: string
    variant?: string
    endpointFormat?: string
    effort?: SubagentReasoningEffort
    promptPreambleHash?: string
    tools?: string[]
  }[] {
    return Object.entries(this.options.config.profiles).map(([name, profile]) => ({
      name,
      toolPolicy: profile.toolPolicy,
      ...(profile.providerId ? { providerId: profile.providerId } : {}),
      ...(profile.model ? { model: profile.model } : {}),
      ...(profile.variant ? { variant: profile.variant } : {}),
      ...(profile.endpointFormat ? { endpointFormat: profile.endpointFormat } : {}),
      ...(profile.effort ? { effort: profile.effort } : {}),
      ...(profile.promptPreamble ? { promptPreambleHash: stableTranscriptHash(profile.promptPreamble) } : {}),
      ...(profile.tools.length > 0 ? { tools: profile.tools } : {})
    }))
  }

  get defaultProfileName(): string | undefined {
    return this.options.config.defaultProfile
  }

  get defaultToolPolicy(): SubagentToolPolicy {
    return this.options.config.defaultToolPolicy
  }

  async loadChildRun(id: string, parentThreadId?: string): Promise<ChildRunRecord | undefined> {
    const record = await this.options.store.load(id)
    if (!record) return undefined
    if (parentThreadId && record.parentThreadId !== parentThreadId) return undefined
    return record
  }

  async reconcileRunningChildren(
    reason = 'runtime restarted before child run finished'
  ): Promise<ChildRunRecord[]> {
    const records = await this.options.store.list()
    const reconciled: ChildRunRecord[] = []
    for (const record of records) {
      if (record.status !== 'queued' && record.status !== 'running') continue
      const now = this.now()
      const aborted = ChildRunRecord.parse({
        ...record,
        status: 'aborted',
        error: reason,
        ...(record.status === 'queued' ? { queuedMs: record.queuedMs ?? elapsedMs(record.createdAt, now) } : {}),
        ...(record.status === 'running' && record.startedAt
          ? { durationMs: record.durationMs ?? elapsedMs(record.startedAt, now) }
          : {}),
        updatedAt: now
      })
      await this.options.store.upsert(aborted)
      await this.recordChildEvent(aborted)
      reconciled.push(aborted)
    }
    return reconciled
  }

  async diagnostics(parentThreadId?: string): Promise<{
    enabled: boolean
    active: number
    childRuns: ChildRunRecord[]
    aggregates: ChildRunAggregate[]
  }> {
    const childRuns = await this.options.store.list(parentThreadId)
    return {
      enabled: this.options.config.enabled,
      active: this.active,
      childRuns,
      aggregates: aggregateChildRuns(childRuns)
    }
  }

  private async recordChildEvent(record: ChildRunRecord): Promise<void> {
    const usage = record.usage
    await this.options.events?.record({
      kind: record.status === 'completed' ? 'turn_completed' : record.status === 'failed' ? 'turn_failed' : record.status === 'aborted' ? 'turn_aborted' : 'turn_started',
      threadId: record.parentThreadId,
      turnId: record.parentTurnId,
      status: record.status,
      text: record.summary ?? record.error,
      child: {
        parentThreadId: record.parentThreadId,
        parentTurnId: record.parentTurnId,
        ...(record.parentToolCallId ? { parentToolCallId: record.parentToolCallId } : {}),
        childId: record.id,
        childRunId: record.id,
        ...(record.childThreadId ? { childThreadId: record.childThreadId } : {}),
        ...(record.childTurnId ? { childTurnId: record.childTurnId } : {}),
        childLabel: record.label,
        childStatus: record.status,
        childSeq: ++this.childSeq,
        ...(record.model ? { childModel: record.model } : {}),
        ...(record.providerId ? { childProviderId: record.providerId } : {}),
        ...(record.endpointFormat ? { childEndpointFormat: record.endpointFormat } : {}),
        ...(record.variant ? { childModelVariant: record.variant } : {}),
        ...(record.modelSource ? { childModelSource: record.modelSource } : {}),
        ...(record.modelExecution ? { childModelExecution: record.modelExecution } : {}),
        ...(record.effort ? { childEffort: record.effort } : {}),
        ...(record.profile ? { childProfile: record.profile } : {}),
        ...(record.toolPolicy ? { childToolPolicy: record.toolPolicy } : {}),
        ...(record.prefixReused !== undefined ? { prefixReused: record.prefixReused } : {}),
        ...(record.inheritedHistoryItems !== undefined ? { inheritedHistoryItems: record.inheritedHistoryItems } : {}),
        ...(record.toolInvocations !== undefined ? { toolInvocations: record.toolInvocations } : {}),
        ...(record.evidenceLedgered !== undefined ? { evidenceLedgered: record.evidenceLedgered } : {}),
        ...(record.evidenceLedgerError ? { evidenceLedgerError: record.evidenceLedgerError } : {}),
        ...(record.durationMs !== undefined ? { durationMs: record.durationMs } : {}),
        ...(record.queuedMs !== undefined ? { queuedMs: record.queuedMs } : {}),
        ...(usage.totalTokens > 0 ? { totalTokens: usage.totalTokens } : {}),
        ...(usage.cachedTokens !== undefined ? { cachedTokens: usage.cachedTokens } : {}),
        ...(usage.cacheHitTokens !== undefined ? { cacheHitTokens: usage.cacheHitTokens } : {}),
        ...(usage.cacheMissTokens !== undefined ? { cacheMissTokens: usage.cacheMissTokens } : {}),
        ...(usage.cacheHitRate !== undefined && usage.cacheHitRate !== null ? { cacheHitRate: usage.cacheHitRate } : {}),
        ...(usage.cacheableTokenHitRate !== undefined && usage.cacheableTokenHitRate !== null ? { cacheableTokenHitRate: usage.cacheableTokenHitRate } : {}),
        ...(usage.totalInputTokenHitRate !== undefined && usage.totalInputTokenHitRate !== null ? { totalInputTokenHitRate: usage.totalInputTokenHitRate } : {}),
        ...(usage.cacheMissReasons?.length ? { cacheMissReasons: usage.cacheMissReasons } : {}),
        ...(usage.cacheSuggestions?.length ? { cacheSuggestions: usage.cacheSuggestions } : {}),
        ...(record.cacheDiagnostics ? { cacheDiagnostics: record.cacheDiagnostics } : {}),
        ...(usage.priceConfigured !== undefined ? { priceConfigured: usage.priceConfigured } : {}),
        ...(usage.costUsd !== undefined ? { costUsd: usage.costUsd } : {}),
        ...(usage.costCny !== undefined ? { costCny: usage.costCny } : {}),
        ...(usage.cacheSavingsUsd !== undefined ? { cacheSavingsUsd: usage.cacheSavingsUsd } : {}),
        ...(usage.cacheSavingsCny !== undefined ? { cacheSavingsCny: usage.cacheSavingsCny } : {}),
        ...(usage.tokenEconomySavingsTokens !== undefined ? { tokenEconomySavingsTokens: usage.tokenEconomySavingsTokens } : {}),
        ...(usage.tokenEconomySavingsUsd !== undefined ? { tokenEconomySavingsUsd: usage.tokenEconomySavingsUsd } : {}),
        ...(usage.tokenEconomySavingsCny !== undefined ? { tokenEconomySavingsCny: usage.tokenEconomySavingsCny } : {})
      }
    })
  }

  private async recordExternalUsage(record: ChildRunRecord): Promise<void> {
    if (record.status !== 'completed') return
    const usage = toUsageSnapshot(record.usage)
    if (usage.totalTokens <= 0 && usage.costUsd === undefined && usage.costCny === undefined) return
    const attribution = { usageSource: 'subagent' as const, childRunId: record.id }
    const recorded = this.options.recordExternalUsage?.(record.parentThreadId, usage, attribution) ?? usage
    await this.options.events?.record({
      kind: 'usage',
      threadId: record.parentThreadId,
      turnId: record.parentTurnId,
      ...(record.model ? { model: record.model } : {}),
      ...(record.providerId ? { providerId: record.providerId } : {}),
      ...(record.endpointFormat ? { endpointFormat: record.endpointFormat } : {}),
      ...(record.effort ? { effort: record.effort } : {}),
      ...attribution,
      usage: recorded,
      ...(record.cacheDiagnostics ? { cacheDiagnostics: record.cacheDiagnostics } : {})
    })
  }

  private async recordParentEvidence(record: ChildRunRecord): Promise<{
    evidenceLedgered?: boolean
    evidenceLedgerError?: string
  }> {
    if (!this.options.recordParentEvidence) return {}
    try {
      return { evidenceLedgered: await this.options.recordParentEvidence(record) }
    } catch (error) {
      return {
        evidenceLedgered: false,
        evidenceLedgerError: errorMessage(error)
      }
    }
  }

  private now(): string {
    return this.options.nowIso?.() ?? new Date().toISOString()
  }
}

function toUsageSnapshot(usage: ChildRunRecord['usage']): UsageSnapshot {
  return {
    promptTokens: usage.promptTokens,
    completionTokens: usage.completionTokens,
    reasoningTokens: usage.reasoningTokens,
    totalTokens: usage.totalTokens,
    cachedTokens: usage.cachedTokens,
    cacheHitTokens: usage.cacheHitTokens,
    cacheMissTokens: usage.cacheMissTokens,
    cacheHitRate: usage.cacheHitRate ?? null,
    cacheableTokenHitRate: usage.cacheableTokenHitRate,
    totalInputTokenHitRate: usage.totalInputTokenHitRate,
    cacheMissReasons: usage.cacheMissReasons,
    cacheSuggestions: usage.cacheSuggestions,
    turns: usage.turns ?? 0,
    priceConfigured: usage.priceConfigured,
    costUsd: usage.costUsd,
    costCny: usage.costCny,
    cacheSavingsUsd: usage.cacheSavingsUsd,
    cacheSavingsCny: usage.cacheSavingsCny,
    tokenEconomySavingsTokens: usage.tokenEconomySavingsTokens,
    tokenEconomySavingsUsd: usage.tokenEconomySavingsUsd,
    tokenEconomySavingsCny: usage.tokenEconomySavingsCny
  }
}

export function aggregateChildRuns(records: readonly ChildRunRecord[]): ChildRunAggregate[] {
  const buckets = new Map<string, ChildRunAggregate>()
  for (const record of records) {
    const label = record.label?.trim() || undefined
    const model = record.model?.trim() || undefined
    const effort = normalizeSubagentEffort(record.effort) ?? undefined
    const key = [label ?? 'unlabeled', model ?? 'default', effort].filter(Boolean).join(':')
    const bucket = buckets.get(key) ?? {
      key,
      ...(label ? { label } : {}),
      ...(model ? { model } : {}),
      ...(effort ? { effort } : {}),
      runs: 0,
      completed: 0,
      failed: 0,
      aborted: 0,
      promptTokens: 0,
      completionTokens: 0,
      reasoningTokens: 0,
      totalTokens: 0,
      averageTotalTokens: 0
    }
    bucket.runs += 1
    if (record.status === 'completed') bucket.completed += 1
    else if (record.status === 'failed') bucket.failed += 1
    else if (record.status === 'aborted') bucket.aborted += 1
    bucket.promptTokens += record.usage.promptTokens
    bucket.completionTokens += record.usage.completionTokens
    bucket.reasoningTokens += record.usage.reasoningTokens ?? 0
    bucket.totalTokens += record.usage.totalTokens
    if (record.usage.costUsd !== undefined) bucket.costUsd = (bucket.costUsd ?? 0) + record.usage.costUsd
    if (record.usage.costCny !== undefined) bucket.costCny = (bucket.costCny ?? 0) + record.usage.costCny
    bucket.averageTotalTokens = bucket.runs > 0 ? bucket.totalTokens / bucket.runs : 0
    bucket.averageCostUsd = bucket.costUsd !== undefined && bucket.runs > 0 ? bucket.costUsd / bucket.runs : undefined
    bucket.averageCostCny = bucket.costCny !== undefined && bucket.runs > 0 ? bucket.costCny / bucket.runs : undefined
    buckets.set(key, bucket)
  }
  return [...buckets.values()].sort((a, b) =>
    b.runs - a.runs ||
    b.totalTokens - a.totalTokens ||
    a.key.localeCompare(b.key)
  )
}

const defaultExecutor: ChildRunExecutor = async (input) => {
  return { summary: `Child result: ${input.prompt}` }
}

function normalizeSubagentEffort(effort: string | undefined): SubagentReasoningEffort | undefined {
  switch (effort?.trim()) {
    case 'off':
      return 'off'
    case 'low':
      return 'low'
    case 'medium':
      return 'medium'
    case 'high':
      return 'high'
    case 'max':
      return 'max'
    default:
      return undefined
  }
}

function resolveModelSource(input: {
  explicitModel?: string
  profileModel?: string
  profileProviderId?: string
  profileVariant?: string
  profileEndpointFormat?: string
  inherited?: ModelExecutionRef
}): ModelExecutionSource {
  if (input.explicitModel) return 'explicit-input'
  if (input.profileModel || input.profileProviderId || input.profileVariant || input.profileEndpointFormat) {
    return 'subagent-profile'
  }
  return input.inherited?.source ?? 'runtime-default'
}

function stripModelExecutionForResolution(
  ref: ModelExecutionRef,
  clearFingerprints: boolean
): ModelExecutionRef {
  const {
    baseUrlFingerprint: _baseUrlFingerprint,
    customFullEndpointFingerprint: _customFullEndpointFingerprint,
    capabilityFingerprint: _capabilityFingerprint,
    ...rest
  } = ref
  if (clearFingerprints) return rest
  return {
    ...rest,
    ...(ref.baseUrlFingerprint ? { baseUrlFingerprint: ref.baseUrlFingerprint } : {}),
    ...(ref.customFullEndpointFingerprint ? { customFullEndpointFingerprint: ref.customFullEndpointFingerprint } : {}),
    ...(ref.capabilityFingerprint ? { capabilityFingerprint: ref.capabilityFingerprint } : {})
  }
}

function normalizeToolScope(tools: readonly string[] | undefined): string[] {
  if (!Array.isArray(tools)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of tools) {
    const name = item.trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    out.push(name)
  }
  return out.sort()
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

/** Non-negative millisecond delta between two ISO timestamps (0 when unparseable). */
function elapsedMs(fromIso: string, toIso: string): number {
  const from = Date.parse(fromIso)
  const to = Date.parse(toIso)
  if (Number.isNaN(from) || Number.isNaN(to)) return 0
  return Math.max(0, to - from)
}
