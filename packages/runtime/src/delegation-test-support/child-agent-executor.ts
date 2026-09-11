import { InMemoryApprovalGate } from '../adapters/in-memory-approval-gate.js'
import { InMemoryEventBus } from '../adapters/in-memory-event-bus.js'
import { InMemorySessionStore } from '../adapters/in-memory-session-store.js'
import { InMemoryThreadStore } from '../adapters/in-memory-thread-store.js'
import { InMemoryUserInputGate } from '../adapters/in-memory-user-input-gate.js'
import type { ImmutablePrefix } from '../cache/immutable-prefix.js'
import { SUBAGENT_READ_ONLY_TOOL_NAMES, type ModelCapabilityMetadata } from '../contracts/capabilities.js'
import type { TurnItem } from '../contracts/items.js'
import {
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  type ApprovalPolicy,
  type SandboxMode
} from '../contracts/policy.js'
import type { RuntimeTuningConfig } from '../config/analytix-config.js'
import { AgentLoop } from '../loop-test-support/agent-loop.js'
import { modelCapabilitiesForModel, type ContextCompactionConfig, type ModelConfig } from '../shared/model-context-profile.js'
import { ContextCompactor } from '../shared/context-compactor.js'
import { InflightTracker } from '../loop-test-support/inflight-tracker.js'
import { SteeringQueue } from '../loop-test-support/steering-queue.js'
import type { TokenEconomyConfig } from '../shared/token-economy.js'
import type { MemoryStore } from '../memory/memory-store.js'
import type { ModelClient } from '../ports/model-client.js'
import { RandomIdGenerator, type IdGenerator } from '../ports/id-generator.js'
import type { SessionStore } from '../ports/session-store.js'
import type { ThreadStore } from '../ports/thread-store.js'
import type { ToolHost } from '../ports/tool-host.js'
import type { SkillRuntime } from '../skills/skill-runtime.js'
import { RuntimeEventRecorder } from '../services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../services-test-support/thread-service.js'
import { TurnService } from '../services-test-support/turn-service.js'
import { UsageService } from '../services-test-support/usage-service.js'
import { TASK_JOB_CONTROL_TOOL_CONTRACT, TASK_TOOL_CONTRACT, PARALLEL_TASKS_TOOL_CONTRACT } from './job-manager.js'
import { GOAL_TOOL_NAMES } from '../tool-test-support/tool/goal-tools.js'
import type { ChildRunExecutor } from './delegation-runtime.js'

export type ChildAgentExecutorOptions = {
  model: ModelClient
  toolHost: ToolHost
  prefix: ImmutablePrefix
  defaultModel: string
  durableState?: {
    threadStore: ThreadStore
    sessionStore: SessionStore
    events: RuntimeEventRecorder
    ids: IdGenerator
  }
  models?: ModelConfig
  contextCompaction?: ContextCompactionConfig
  approvalPolicy?: ApprovalPolicy
  sandboxMode?: SandboxMode
  tokenEconomy?: TokenEconomyConfig
  runtime?: RuntimeTuningConfig
  nowIso?: () => string
  modelCapabilities?: (model: string) => ModelCapabilityMetadata
  skillRuntime?: SkillRuntime
  memoryStore?: MemoryStore
}

export function createChildAgentExecutor(options: ChildAgentExecutorOptions): ChildRunExecutor {
  return async (input) => {
    const nowIso = options.nowIso ?? (() => new Date().toISOString())
    const eventBus = options.durableState ? undefined : new InMemoryEventBus()
    const sessionStore = options.durableState?.sessionStore ?? new InMemorySessionStore()
    const threadStore = options.durableState?.threadStore ?? new InMemoryThreadStore()
    const usage = new UsageService()
    const ids = options.durableState?.ids ?? new RandomIdGenerator()
    const inflight = new InflightTracker()
    const steering = new SteeringQueue()
    const compactor = new ContextCompactor({
      contextCompaction: options.contextCompaction,
      models: options.models
    })
    const events = options.durableState?.events ?? new RuntimeEventRecorder({
      eventBus: eventBus!,
      sessionStore,
      allocateSeq: (threadId) => eventBus!.allocateSeq(threadId),
      nowIso
    })
    const turns = new TurnService({
      threadStore,
      sessionStore,
      events,
      inflight,
      steering,
      compactor,
      ids,
      nowIso
    })
    const threads = new ThreadService({
      threadStore,
      sessionStore,
      events,
      ids,
      nowIso
    })
    const requestedModel = input.model?.trim()
    const inheritedModel = input.modelExecution?.modelId?.trim()
    const providerId = input.modelExecution?.providerId?.trim()
    if (!input.modelExecution || !providerId) {
      throw new Error('provider_not_found: child executor requires an inherited providerId')
    }
    const model = requestedModel || inheritedModel
    if (!model) {
      throw new Error('model_not_found: child executor requires an explicit or inherited modelId')
    }
    const modelCapabilities = options.modelCapabilities?.(model) ?? modelCapabilitiesForModel(model)
    const title = childThreadTitle(input.childId, input.label)
    const existingThread = options.durableState && input.transcript?.mode === 'continue'
      ? await threads.get(input.childId)
      : null
    if (options.durableState && input.transcript?.mode === 'continue' && existingThread) {
      if ((existingThread.relation ?? 'primary') !== 'side' || existingThread.parentThreadId !== input.parentThreadId) {
        throw new Error(`child thread does not belong to parent thread: ${input.childId}`)
      }
      if (existingThread.status === 'running') {
        throw new Error(`child thread is already running: ${input.childId}`)
      }
    }
    let thread = existingThread ?? null
    if (!thread && options.durableState && input.transcript?.mode === 'fork' && input.transcript.sourceId?.trim()) {
      const sourceThread = await threads.get(input.transcript.sourceId.trim())
      if (sourceThread) {
        thread = await threads.fork(sourceThread.id, {
          id: input.childId,
          relation: 'side',
          title,
          parentThreadId: input.parentThreadId
        })
      }
    }
    thread ??= await threads.create({
      title,
      workspace: input.workspace?.trim() || '~',
      model,
      ...(providerId ? { providerId } : {}),
      mode: 'agent',
      approvalPolicy: options.approvalPolicy ?? DEFAULT_APPROVAL_POLICY,
      ...(options.sandboxMode ? { sandboxMode: options.sandboxMode } : {})
    }, {
      id: input.childId,
      title,
      ...(options.durableState
        ? { relation: 'side' as const, parentThreadId: input.parentThreadId }
        : {})
    })
    // A profile preamble rides in the prompt body (not the system prompt) so
    // the cached stable prefix stays byte-identical to the main agent's.
    const prompt = input.promptPreamble?.trim()
      ? `${input.promptPreamble.trim()}\n\n${input.prompt}`
      : input.prompt
    const started = await turns.startTurn({
      threadId: thread.id,
      request: {
        prompt,
        model,
        ...(providerId ? { providerId } : {}),
        ...(input.effort ? { reasoningEffort: input.effort } : {}),
        ...(input.maxModelSteps !== undefined ? { maxModelSteps: input.maxModelSteps } : {}),
        mode: 'agent',
        // Children have no GUI surface to answer structured input prompts.
        disableUserInput: true
      }
    })
    // Children advertise a narrowed catalog before their first model call.
    // Read-only children are pinned to investigation tools. Inherit-mode
    // children keep normal runtime tools, but recursive subagent/job/goal/user
    // input meta tools are filtered out so child runs cannot spawn control
    // loops or wait on parent-owned jobs.
    const forcedAllowedToolNames = await childAllowedToolNames({
      toolPolicy: input.toolPolicy,
      toolScope: input.toolScope,
      threadId: thread.id,
      turnId: started.turnId,
      workspace: thread.workspace,
      modelCapabilities,
      currentMaxModelSteps: input.maxModelSteps ?? 0,
      approvalPolicy: options.approvalPolicy ?? DEFAULT_APPROVAL_POLICY,
      sandboxMode: options.sandboxMode ?? DEFAULT_SANDBOX_MODE,
      signal: input.signal,
      toolHost: options.toolHost
    })
    const loop = new AgentLoop({
      threadStore,
      sessionStore,
      approvalGate: new InMemoryApprovalGate(),
      userInputGate: new InMemoryUserInputGate(),
      model: options.model,
      toolHost: options.toolHost,
      usage,
      events,
      turns,
      inflight,
      steering,
      compactor,
      prefix: options.prefix,
      ids,
      nowIso,
      ...(forcedAllowedToolNames ? { forcedAllowedToolNames } : {}),
      ...(options.modelCapabilities ? { modelCapabilities: options.modelCapabilities } : {}),
      ...(options.skillRuntime ? { skillRuntime: options.skillRuntime } : {}),
      ...(options.memoryStore ? { memoryStore: options.memoryStore } : {}),
      ...(options.contextCompaction ? { contextCompaction: options.contextCompaction } : {}),
      ...(options.tokenEconomy ? { tokenEconomy: options.tokenEconomy } : {}),
      ...(options.runtime?.stepLimits ? { stepLimits: options.runtime.stepLimits } : {}),
      ...(options.runtime?.toolStorm ? { toolStorm: options.runtime.toolStorm } : {}),
      ...(options.runtime?.toolArgumentRepair ? { toolArgumentRepair: options.runtime.toolArgumentRepair } : {})
    })
    const status = await loop.runTurn(thread.id, started.turnId)
    const runtimeError = (await sessionStore.loadEventsSince(thread.id, 0))
      .find((event) => event.kind === 'error' && event.turnId === started.turnId)
    if (runtimeError?.kind === 'error') {
      throw new Error(runtimeError.message)
    }
    const items = await sessionStore.loadItems(thread.id)
    const summary = summarizeChildTurn(items, started.turnId, status)
    const toolInvocations = items.filter(
      (item) => item.turnId === started.turnId && item.kind === 'tool_call'
    ).length
    if (status !== 'completed') {
      throw new Error(summary || `child agent ${status}`)
    }
    return {
      summary,
      usage: usage.forThread(thread.id),
      childThreadId: thread.id,
      childTurnId: started.turnId,
      toolInvocations,
      transcriptItems: childTranscriptItems(items, input.transcript?.mode === 'continue' ? undefined : started.turnId),
      // The child loop was constructed with the main agent's immutable
      // prefix; only the small delegation prompt is appended fresh.
      prefixReused: true,
      inheritedHistoryItems: 0
    }
  }
}

const SUBAGENT_META_TOOL_NAMES = new Set<string>([
  'delegate_task',
  TASK_TOOL_CONTRACT.name,
  PARALLEL_TASKS_TOOL_CONTRACT.name,
  TASK_JOB_CONTROL_TOOL_CONTRACT.wait,
  TASK_JOB_CONTROL_TOOL_CONTRACT.output,
  TASK_JOB_CONTROL_TOOL_CONTRACT.kill,
  'user_input',
  'request_user_input',
  ...GOAL_TOOL_NAMES
])

async function childAllowedToolNames(input: {
  toolPolicy: 'readOnly' | 'inherit'
  toolScope: readonly string[] | undefined
  threadId: string
  turnId: string
  workspace: string
  modelCapabilities: ModelCapabilityMetadata
  currentMaxModelSteps: number
  approvalPolicy: ApprovalPolicy
  sandboxMode: SandboxMode
  signal: AbortSignal
  toolHost: ToolHost
}): Promise<string[]> {
  const scoped = normalizeToolScope(input.toolScope)
  if (input.toolPolicy === 'readOnly') {
    if (scoped.length === 0) return [...SUBAGENT_READ_ONLY_TOOL_NAMES]
    const readOnly: ReadonlySet<string> = new Set(SUBAGENT_READ_ONLY_TOOL_NAMES)
    return scoped.filter((name) => readOnly.has(name))
  }
  if (scoped.length > 0) return scoped.filter((name) => !SUBAGENT_META_TOOL_NAMES.has(name))
  const advertised = await input.toolHost.listTools({
    threadId: input.threadId,
    turnId: input.turnId,
    workspace: input.workspace,
    threadMode: 'agent',
    model: input.modelCapabilities,
    activeSkillIds: [],
    memoryPolicy: { enabled: false },
    delegationPolicy: { enabled: false },
    runtimeStepLimits: { currentMaxModelSteps: input.currentMaxModelSteps },
    approvalPolicy: input.approvalPolicy,
    sandboxMode: input.sandboxMode,
    abortSignal: input.signal,
    awaitApproval: async () => 'allow'
  })
  return advertised
    .map((tool) => tool.name)
    .filter((name) => !SUBAGENT_META_TOOL_NAMES.has(name))
    .sort()
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

function childThreadTitle(childId: string, label?: string): string {
  const suffix = childThreadTitleLabel(label) ?? childId
  return `Child agent: ${suffix}`
}

function childThreadTitleLabel(label: string | undefined): string | undefined {
  const text = label?.trim().replace(/\s+/g, ' ')
  if (!text || text.length > 32) return undefined
  if (text.split(/\s+/).filter(Boolean).length > 4) return undefined
  if (/[，。！？；：、,.!?;:]/.test(text) && text.length > 12) return undefined
  if (/^(please\s+)?(inspect|review|analy[sz]e|generate|summari[sz]e|search|read|create|build)\b/i.test(text) && (text.includes(' ') || text.length > 16)) return undefined
  if (/^(请|基于|根据|分析|整合|生成|查看|读取|审查|调研|搜索|检查)/.test(text) && text.length > 10) return undefined
  return text
}

function summarizeChildTurn(
  items: readonly TurnItem[],
  turnId: string,
  status: 'completed' | 'failed' | 'aborted'
): string {
  const turnItems = items.filter((item) => item.turnId === turnId)
  const assistantText = turnItems
    .filter((item): item is Extract<TurnItem, { kind: 'assistant_text' }> => item.kind === 'assistant_text')
    .map((item) => item.text.trim())
    .filter(Boolean)
    .join('\n\n')
    .trim()
  if (assistantText) return assistantText
  const errors = turnItems
    .filter((item): item is Extract<TurnItem, { kind: 'error' }> => item.kind === 'error')
    .map((item) => item.message.trim())
    .filter(Boolean)
    .join('\n')
    .trim()
  if (errors) return errors
  const toolResult = [...turnItems]
    .reverse()
    .find((item): item is Extract<TurnItem, { kind: 'tool_result' }> => item.kind === 'tool_result')
  if (toolResult) return stringifySummary(toolResult.output)
  return status === 'completed'
    ? 'Child agent completed without a text response.'
    : `Child agent ${status}.`
}

function childTranscriptItems(items: readonly TurnItem[], turnId?: string): TurnItem[] {
  return items
    .filter((item) => !turnId || item.turnId === turnId)
    .filter((item) => item.kind !== 'approval' && item.kind !== 'user_input')
    .map((item) => ({ ...item }))
}

function stringifySummary(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (value == null) return ''
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}
