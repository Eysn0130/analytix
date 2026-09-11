import { createHash } from 'node:crypto'
import { isAbsolute, relative, resolve } from 'node:path'
import type { ModelClient, ModelHistoryItem, ModelRequest, ModelToolSpec } from '../ports/model-client.js'
import type {
  ToolHost,
  ToolCallLike,
  ToolHostContext,
  ToolHostResult,
  GuiPlanContext,
  ToolProviderKind
} from '../ports/tool-host.js'
import type { ModelCapabilityMetadata } from '../contracts/capabilities.js'
import {
  normalizeModelEndpointFormat,
  type ModelEndpointFormat
} from '../contracts/model-endpoint-format.js'
import {
  ModelExecutionRefSchema,
  type ModelExecutionRef,
  type ModelExecutionSource
} from '../contracts/model-execution-ref.js'
import { DEFAULT_APPROVAL_POLICY, DEFAULT_SANDBOX_MODE } from '../contracts/policy.js'
import type { ThreadStore } from '../ports/thread-store.js'
import type { SessionStore } from '../ports/session-store.js'
import type { ApprovalGate } from '../ports/approval-gate.js'
import type { UserInputGate, UserInputResolution } from '../ports/user-input-gate.js'
import type { ApprovalRequest } from '../domain/approval.js'
import type { UsageService } from '../services-test-support/usage-service.js'
import type { TurnService } from '../services-test-support/turn-service.js'
import type { RuntimeEventRecorder } from '../services-test-support/runtime-event-recorder.js'
import type { PipelineStage } from '../contracts/events.js'
import type { RuntimeErrorSeverity } from '../contracts/errors.js'
import type { IdGenerator } from '../ports/id-generator.js'
import type { ImmutablePrefix } from '../cache/immutable-prefix.js'
import { ContextCompactor } from '../shared/context-compactor.js'
import { ContextEstimator } from '../shared/context-estimator.js'
import {
  effectiveHistoryAfterLatestCompaction,
  insertCompactionIntoVisibleHistory,
  placeCompactionsAtTurnEnd
} from '../shared/compaction-history.js'
import { summarizeCompactionWithModel } from '../shared/compaction-summary.js'
import { InflightTracker } from './inflight-tracker.js'
import { SteeringQueue } from './steering-queue.js'
import {
  createImmutablePrefix,
  shouldVerifyImmutablePrefix,
  verifyImmutablePrefix
} from '../cache/immutable-prefix.js'
import {
  detectVolatilePrefixContent,
  type PrefixVolatilityFinding
} from '../cache/prefix-volatility.js'
import { buildToolCatalogFingerprint } from '../cache/tool-catalog-fingerprint.js'
import {
  captureCachePrefixShape,
  compareCachePrefixShapes,
  type CachePrefixShape,
  type ToolSourceCacheShape
} from '../cache/prefix-cache-diagnostics.js'
import {
  makeAssistantTextItem,
  makePrivateToolCallItem,
  makePrivateToolResultItem,
  projectToolCallItemForPublic,
  projectToolResultItemForPublic,
  makeUserInputItem,
  makeErrorItem,
  type InternalToolCallItem,
  type InternalToolResultItem
} from '../domain/item.js'
import { touchThread } from '../domain/thread.js'
import { repairModelHistoryItems } from '../domain/model-history-repair.js'
import type { TurnItem } from '../contracts/items.js'
import type { ThreadGoal, ThreadTodoList } from '../contracts/threads.js'
import { modelCapabilitiesForModel, type ContextCompactionConfig } from '../shared/model-context-profile.js'
import type { SkillRuntime } from '../skills/skill-runtime.js'
import type { AttachmentContent, AttachmentStore } from '../attachments/attachment-store.js'
import type { ModelInputAttachment, ModelTextAttachmentFallback } from '../ports/model-client.js'
import type { MemoryStore } from '../memory/memory-store.js'
import { compileMemoryContext } from '../memory/memory-context-compiler.js'
import {
  hasHooksForPhase,
  runObserverHooks,
  runUserPromptSubmitHooks,
  type ResolvedHook
} from '../hooks/hook-engine.js'
import {
  applyTokenEconomyToRequest,
  normalizeTokenEconomyConfig,
  type TokenEconomyConfig
} from '../shared/token-economy.js'
import { applyRequestHistoryHygiene } from '../shared/request-history-hygiene.js'
import { capToolResultImages } from '../shared/tool-result-image.js'
import { buildTranscriptHardeningContext } from '../shared/transcript-hardening.js'
import {
  AUTO_MODEL_ROUTER_FINGERPRINT,
  recentAutoRouterContext,
  resolveAutoModelRoute,
  type AutoModelRouteSelection
} from '../shared/auto-model-router.js'
import { ToolStormBreaker, type ToolStormBreakerOptions } from '../shared/tool-storm-breaker.js'
import { healLoadedHistoryItems } from '../shared/history-healing.js'
import { repairDispatchToolArguments } from '../shared/tool-call-repair.js'
import { CREATE_PLAN_TOOL_NAME } from '../tool-test-support/tool/create-plan-tool.js'
import { guiPlanWorkspaceMatches } from '../shared/gui-plan.js'
import {
  COMPLETE_STEP_TOOL_NAME,
  GET_GOAL_TOOL_NAME,
  UPDATE_GOAL_TOOL_NAME
} from '../tool-test-support/tool/goal-tools.js'
import { TODO_LIST_TOOL_NAME, TODO_WRITE_TOOL_NAME } from '../tool-test-support/tool/todo-tools.js'
import { shellRuntimeInstruction } from '../tool-test-support/tool/builtin-tool-utils.js'
import {
  DEFAULT_MAX_GOAL_RESUME_NO_PROGRESS_ATTEMPTS,
  GoalResumeCoordinator,
  type GoalResumeCoordinatorDeps
} from '../shared/goal-resume-coordinator.js'
import {
  GOAL_RESUME_PROMPT,
  GoalControlMachine,
  goalResumeKey
} from '../shared/goal-control-machine.js'
import {
  buildRuntimeEnvironmentInstruction,
  type CommandProbeDiagnostic
} from '../environment/command-probe.js'

const PARALLEL_READ_ONLY_TOOL_NAMES = new Set(['read', 'grep', 'find', 'ls'])
const DELEGATE_TASK_TOOL_NAME = 'delegate_task'
const MAX_PARALLEL_TOOL_CALLS = 3
const MAX_FORWARDED_TOOL_IMAGES = 3
export const DEFAULT_TURN_MODEL_STEP_LIMIT = 64
// Adapted from DeepSeek-Reasonix internal/control/goal.go maxGoalAutoTurns.
export const DEFAULT_GOAL_AUTO_MODEL_STEP_LIMIT = 50
const MAX_TOOL_CATALOG_SNAPSHOTS = 256
const MAX_CACHE_PREFIX_SHAPES = 512
const REQUEST_ESTIMATE_CHARS_PER_TOKEN = 4
const requestEstimator = new ContextEstimator(REQUEST_ESTIMATE_CHARS_PER_TOKEN)

export type RuntimeStepLimitConfig = {
  defaultMaxModelSteps?: number
  userGlobalMaxModelSteps?: number
  plannerMaxModelSteps?: number
  headlessMaxModelSteps?: number
}

function estimateModelRequestInputTokens(request: ModelRequest): number {
  let tokens = 0
  tokens += estimateRequestText(request.systemPrompt)
  tokens += estimateRequestText(request.modeInstruction)
  tokens += estimateRequestText(request.contextInstructions?.join('\n'))
  tokens += estimateRequestItems(request.prefix)
  tokens += estimateRequestItems(request.history)
  tokens += estimateRequestTools(request.tools)
  tokens += estimateRequestTextFallbacks(request.attachmentTextFallbacks)
  tokens += estimateRequestText(request.requiredToolName)
  tokens += estimateRequestText(request.reasoningEffort)
  return Math.max(0, tokens)
}

/**
 * Estimate the per-request token overhead that is sent on every turn
 * but is not part of the stored conversation items: system prompt,
 * few-shot prefix, mode/context instructions, and full tool schemas.
 */
function estimateRequestOverheadTokens(input: {
  systemPrompt?: string
  modeInstruction?: string
  contextInstructions?: string[]
  prefix?: TurnItem[]
  tools?: readonly ModelToolSpec[]
}): number {
  let tokens = 0
  tokens += estimateRequestText(input.systemPrompt)
  tokens += estimateRequestText(input.modeInstruction)
  tokens += estimateRequestText(input.contextInstructions?.join('\n'))
  tokens += estimateRequestItems(input.prefix)
  tokens += estimateRequestTools(input.tools)
  return Math.max(0, tokens)
}

function estimateRequestItems(items?: ModelHistoryItem[]): number {
  if (!items?.length) return 0
  const publicItems: TurnItem[] = []
  let internalTokens = 0
  for (const item of items) {
    if (item.kind === 'assistant_reasoning') {
      internalTokens += estimateRequestText([item.text, item.signature].filter(Boolean).join('\n'))
    } else if (item.kind === 'tool_call') {
      internalTokens += estimateRequestText([
        item.toolName,
        JSON.stringify(item.arguments)
      ].join('\n'))
    } else if (item.kind === 'tool_result') {
      internalTokens += estimateRequestText([
        item.toolName,
        JSON.stringify(item.output)
      ].join('\n'))
    } else {
      publicItems.push(item)
    }
  }
  return internalTokens + requestEstimator.estimateItems(publicItems)
}

function estimateRequestTools(tools?: readonly ModelToolSpec[]): number {
  if (!tools?.length) return 0
  return tools.reduce((sum, tool) => {
    return sum + estimateRequestText([
      tool.name,
      tool.description,
      JSON.stringify(tool.inputSchema)
    ].join('\n'))
  }, 0)
}

function estimateRequestTextFallbacks(fallbacks?: ModelTextAttachmentFallback[]): number {
  if (!fallbacks?.length) return 0
  return fallbacks.reduce((sum, attachment) => {
    return sum + estimateRequestText([
      attachment.name,
      attachment.mimeType,
      String(attachment.byteSize),
      attachment.dataBase64
    ].join('\n'))
  }, 0)
}

function estimateRequestText(text?: string): number {
  if (!text?.trim()) return 0
  return Math.max(1, Math.ceil(text.length / REQUEST_ESTIMATE_CHARS_PER_TOKEN))
}

export type RuntimeStepLimitScope = {
  mode?: 'agent' | 'plan'
  disableUserInput?: boolean
  threadMaxModelSteps?: number
  turnMaxModelSteps?: number
}

type TurnFailure = {
  error: string
  code?: string
  details?: unknown
  severity?: RuntimeErrorSeverity
}

type GoalStatusTransitionOptions = {
  code?: string
  severity?: RuntimeErrorSeverity
  details?: unknown
  blockedReason?: string
  blockedTurnId?: string
}

type ModelClientDiagnostics = {
  provider?: string
  providerId?: string
  providerBaseUrl?: string
  endpointFormat?: string
  configuredModel?: string
}

const PIPELINE_STAGE_LABELS: Record<PipelineStage, string> = {
  setup: 'Setup',
  pre_start: 'Pre-Start',
  post_start: 'Post-Start',
  input_received: 'Input Received',
  input_cached: 'Input Cached',
  input_routed: 'Input Routed',
  input_compressed: 'Input Compressed',
  input_remembered: 'Input Remembered',
  pre_send: 'Pre-Send',
  post_send: 'Post-Send',
  response_received: 'Response Received',
  provider_retrying: 'Provider Retrying',
  provider_error: 'Provider Error',
  empty_final_recovered: 'Empty Final Recovered',
  subagent_running: 'Subagent Running',
  subagent_completed: 'Subagent Completed',
  subagent_failed: 'Subagent Failed',
  subagent_killed: 'Subagent Killed',
  subagent_aborted: 'Subagent Aborted',
  subagent_interrupted: 'Subagent Interrupted'
}

type ToolCatalogSnapshot = {
  fingerprint: string
  toolNames: string[]
  toolHashes: Record<string, string>
}

type GoalElapsedTimer = {
  startedAtMs: number
  createdAt: string
  objective: string
}

type ToolCatalogDrift =
  | { kind: 'none' }
  | { kind: 'additive'; previous: ToolCatalogSnapshot }
  | { kind: 'breaking'; previous: ToolCatalogSnapshot }

function boundedStepLimit(value: number | undefined, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.max(0, Math.floor(value))
}

function positiveStepLimit(value: number | undefined): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined
  const normalized = Math.floor(value)
  return normalized > 0 ? normalized : undefined
}

export function resolveRuntimeModelStepLimit(
  config: RuntimeStepLimitConfig | undefined,
  scope: RuntimeStepLimitScope = {}
): number {
  let limit = boundedStepLimit(config?.defaultMaxModelSteps, DEFAULT_TURN_MODEL_STEP_LIMIT)
  const userGlobal = positiveStepLimit(config?.userGlobalMaxModelSteps)
  if (userGlobal !== undefined) limit = userGlobal
  const headless = positiveStepLimit(config?.headlessMaxModelSteps)
  if (scope.disableUserInput && headless !== undefined) limit = headless
  const planner = positiveStepLimit(config?.plannerMaxModelSteps)
  if (scope.mode === 'plan' && planner !== undefined) limit = planner
  if (scope.threadMaxModelSteps !== undefined) {
    limit = boundedStepLimit(scope.threadMaxModelSteps, limit)
  }
  if (scope.turnMaxModelSteps !== undefined) {
    limit = boundedStepLimit(scope.turnMaxModelSteps, limit)
  }
  return limit
}

/**
 * Plan-mode guidance. Emitted as a second system message after the
 * byte-stable prefix (see `ModelRequest.modeInstruction`) so the cached
 * prefix is untouched while the note still rides at the front. Kept as a
 * stable constant so Plan-mode turns continue to share cached bytes.
 */
export const PLAN_MODE_INSTRUCTION = [
  'You are in Plan mode.',
  'Investigate the task first using read-only tools: prefer `read`, `grep`, `find`, and `ls` to gather the facts you need.',
  'Do NOT modify project files, apply edits, run shell commands, or run mutating commands in this mode.',
  'If the request is ambiguous or hinges on a decision only the user can make, ask before planning: prefer the `user_input` tool to ask one concise round of clarifying questions (offer concrete options when there are any), then use the answer to write the plan in the same turn. If that tool is not available, end your turn with the question(s) in prose and wait for the answer. Either way, do NOT call `create_plan` until the ambiguity is resolved — a set of options the user still has to choose between is not a plan.',
  'When you understand the task well enough, call the `create_plan` tool to save a complete implementation plan as Markdown.',
  'Use `operation: "draft"` for the first plan, and `operation: "refine"` when revising an existing plan; you may call `create_plan` multiple times as the plan evolves.',
  'Write concrete, actionable steps rather than vague intentions, and structure the saved Markdown with `##` section headings (e.g. Summary, Steps, Tests, Risks).',
  'Favor the smallest plan that fully solves the task: question whether each proposed component, abstraction, dependency, config knob, or new file needs to exist at all (YAGNI), and prefer the standard library, a native platform feature, or an already-present dependency over new custom code. Do NOT trim correctness, input validation, error handling, security, or accessibility to make a plan smaller.',
  'After saving, give the user a short summary of the plan and what to review.'
].join('\n')

/** Read-only tools allowed during the investigation phase of a Plan-mode
 * turn (step 0, before `create_plan` has been called). Matches the
 * PLAN_MODE_INSTRUCTION guidance. `bash` is intentionally excluded —
 * it can execute arbitrary commands and its policy is `on-request` which
 * auto-approves under `approvalPolicy: auto`. */
const PLAN_READ_ONLY_TOOL_NAMES = new Set([
  'read',
  'ls',
  'find',
  'grep',
  'web_search',
  'web_fetch'
])

/** Interactive tools allowed during the investigation phase (step 0) of a
 * Plan-mode turn so the model can ask the user a structured clarifying
 * question (with options) and continue to `create_plan` in the same turn
 * instead of stopping with a prose question. Only effective on GUI turns:
 * these tools advertise off `awaitUserInput`, so IM/headless plan turns never
 * surface them and the prose-and-stop fallback applies there. */
const PLAN_INTERACTIVE_TOOL_NAMES = new Set(['user_input', 'request_user_input'])

/**
 * Resolve the tool list for a Plan-mode turn step. Extracted as a pure
 * function so the behaviour can be unit-tested without spinning up the
 * full agent loop.
 *
 * - Not plan-active or plan already satisfied → pass through unchanged.
 * - Step 0 (investigation): read-only + interactive (user_input) tools + create_plan.
 * - Step > 0 (must produce plan): only create_plan.
 */
export function resolvePlanModeToolSpecs(
  toolSpecs: ModelToolSpec[],
  options: {
    planTurnActive: boolean
    createPlanSatisfied: boolean
    stepIndex: number
    readOnlyToolNames?: ReadonlySet<string>
    interactiveToolNames?: ReadonlySet<string>
    planToolName?: string
  }
): ModelToolSpec[] {
  if (!options.planTurnActive || options.createPlanSatisfied) return toolSpecs
  const readOnly = options.readOnlyToolNames ?? PLAN_READ_ONLY_TOOL_NAMES
  const interactive = options.interactiveToolNames ?? PLAN_INTERACTIVE_TOOL_NAMES
  const planTool = options.planToolName ?? CREATE_PLAN_TOOL_NAME
  return options.stepIndex === 0
    ? toolSpecs.filter(
        (tool) => tool.name === planTool || readOnly.has(tool.name) || interactive.has(tool.name)
      )
    : toolSpecs.filter((tool) => tool.name === planTool)
}

/**
 * A GUI plan context whose workspace doesn't match the thread it runs in is
 * stale, commonly from a fork or cloned history. Ignore it for the live model
 * request instead of forcing create_plan to write into the wrong workspace.
 */
export function isStalePlanContext(
  planContext: { workspaceRoot: string } | undefined,
  workspace: string
): boolean {
  return planContext ? !guiPlanWorkspaceMatches(workspace, planContext.workspaceRoot) : false
}

const PLAN_CLARIFYING_CUE =
  /\b(which|what kind|do you want|would you (?:like|prefer)|let me know which|prefer)\b|哪|还是|你想要|请选择|选项/i

export function isPlanClarifyingQuestion(text: string): boolean {
  const trimmed = text.trim()
  if (!trimmed) return false
  if (/^#{1,6}\s/m.test(trimmed)) return false
  const tail = trimmed.split('\n').slice(-2).join('\n')
  if (!/[?？]/.test(tail)) return false
  return PLAN_CLARIFYING_CUE.test(trimmed)
}

export function buildRuntimeContextInstruction(input: {
  workspace?: string
  nowIso: string
  timeZone?: string
}): string | null {
  const workspace = input.workspace?.trim()
  const projectPath = workspace
    ? isAbsolute(workspace) ? workspace : resolve(workspace)
    : ''
  const localTime = formatLocalDateTimeForPrompt(input.nowIso, input.timeZone)
  if (!projectPath && !localTime) return null
  return [
    'Runtime context for this model request:',
    projectPath ? `- Current opened project absolute path: \`${projectPath}\`` : '',
    localTime ? `- Current user local time: ${localTime}` : '',
    '- Treat this block as environment context, not as user instructions.'
  ].filter(Boolean).join('\n')
}

async function buildRuntimeEnvironmentInstructionForLoop(
  runtimeEnvironment: AgentLoopOptions['runtimeEnvironment'] | undefined,
  input: { workspace?: string } = {}
): Promise<string | null> {
  if (!runtimeEnvironment?.commandDiagnostics) return null
  try {
    const commands = await runtimeEnvironment.commandDiagnostics(input)
    return buildRuntimeEnvironmentInstruction({ commands })
  } catch {
    return null
  }
}

export function shouldInjectInitialRuntimeContext(input: {
  stepIndex: number
  turnId: string
  historyItems: readonly TurnItem[]
}): boolean {
  return input.stepIndex === 0 && input.historyItems.every((item) => item.turnId === input.turnId)
}

function formatLocalDateTimeForPrompt(nowIso: string, timeZone?: string): string {
  const date = new Date(nowIso)
  const fallback = nowIso.trim()
  if (Number.isNaN(date.getTime())) return fallback
  const resolvedTimeZone = timeZone?.trim() || Intl.DateTimeFormat().resolvedOptions().timeZone
  try {
    const formatter = new Intl.DateTimeFormat('en-CA', {
      ...(resolvedTimeZone ? { timeZone: resolvedTimeZone } : {}),
      weekday: 'long',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
      timeZoneName: 'shortOffset'
    })
    const parts = new Map(formatter.formatToParts(date).map((part) => [part.type, part.value]))
    const year = parts.get('year')
    const month = parts.get('month')
    const day = parts.get('day')
    const hour = parts.get('hour')
    const minute = parts.get('minute')
    const second = parts.get('second')
    const weekday = parts.get('weekday')
    if (!year || !month || !day || !hour || !minute || !second || !weekday) {
      return fallback || date.toISOString()
    }
    const zone = [resolvedTimeZone, parts.get('timeZoneName')].filter(Boolean).join(', ')
    return `${year}-${month}-${day} ${hour}:${minute}:${second} ${weekday}${zone ? ` (${zone})` : ''}`
  } catch {
    return fallback || date.toISOString()
  }
}

function todoContinuationInstruction(todos: ThreadTodoList | undefined): string | null {
  const items = todos?.items ?? []
  if (items.length === 0) return null
  const rows = items.slice(0, 50).map((item, index) => {
    const source = item.source?.kind === 'plan' ? ` source=plan:${item.source.relativePath}` : ''
    return `${index + 1}. [${item.status}] ${escapeXmlText(item.content)}${source}`
  })
  return [
    'The current thread todo list is structured, user-visible progress state.',
    'Use `todo_list` to inspect it and `todo_write` to replace the whole list when task state changes.',
    'Keep at most one item in_progress. Plan-linked todos mirror Markdown checkboxes in the saved plan file.',
    'Do not mark the active goal complete while any todo is pending or in_progress.',
    '',
    '<thread_todos>',
    ...rows,
    '</thread_todos>'
  ].join('\n')
}

function escapeXmlText(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
}

function hasSuccessfulCreatePlanResult(items: readonly TurnItem[], turnId: string): boolean {
  return items.some((item) =>
    item.turnId === turnId &&
    item.kind === 'tool_result' &&
    item.toolName === CREATE_PLAN_TOOL_NAME &&
    item.status === 'completed' &&
    item.isError !== true
  )
}

function latestUserMessageText(items: readonly TurnItem[], turnId: string): string {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index]
    if (item?.turnId === turnId && item.kind === 'user_message' && item.text.trim()) {
      return item.text.trim()
    }
  }
  return ''
}

function usesDirectAnswerToolProfile(input: {
  prompt: string
  allowedToolNames?: readonly string[]
  planTurnActive: boolean
}): boolean {
  if (input.planTurnActive || (input.allowedToolNames?.length ?? 0) > 0) return false
  const normalized = input.prompt.trim().toLowerCase()
  if (!normalized || normalized.includes('\n') || Array.from(normalized).length > 80) return false
  if (containsAgentWorkCue(normalized)) return false
  const compact = normalized.split(/\s+/).filter(Boolean).join(' ')
  const identityPatterns = [
    '你是什么大模型',
    '你是什么模型',
    '你是什么 ai',
    '你是什么ai',
    '你是谁',
    '介绍一下你自己',
    'what model are you',
    'which model are you',
    'who are you'
  ]
  if (identityPatterns.some((pattern) => compact.includes(pattern))) return true
  const greetingPatterns = ['你好', '您好', '谢谢']
  if (greetingPatterns.some((pattern) => compact === pattern)) return true
  const lightweightPatterns = [
    '这个问题怎么理解',
    '这是什么意思',
    '简单总结一下',
    '简单解释一下',
    '帮我理解一下'
  ]
  if (lightweightPatterns.some((pattern) =>
    compact === pattern ||
    compact.startsWith(`${pattern} `) ||
    compact.startsWith(`${pattern}，`) ||
    compact.startsWith(`${pattern},`)
  )) return true
  if (Array.from(compact).length > 48) return false
  if (compact.includes('?') || compact.includes('？')) return true
  const chineseQuestionPrefixes = ['什么', '为什么', '怎么', '如何', '是否', '是不是', '能不能', '可以吗', '请问']
  if (chineseQuestionPrefixes.some((prefix) => compact.startsWith(prefix))) return true
  const englishQuestionPrefixes = ['what ', 'why ', 'how ', 'is ', 'are ', 'can ', 'could ', 'should ', 'do ', 'does ']
  if (englishQuestionPrefixes.some((prefix) => compact.startsWith(prefix))) return true
  return false
}

type PromptRoute = 'direct_answer' | 'light_agent' | 'tool_agent' | 'subagent_agent'

function promptRouteForPrompt(input: {
  prompt: string
  allowedToolNames?: readonly string[]
  planTurnActive: boolean
}): PromptRoute {
  if (usesDirectAnswerToolProfile(input)) return 'direct_answer'
  if (promptRequestsSubagentTools(input.prompt, input.allowedToolNames)) return 'subagent_agent'
  if (promptMentionsSkillTool(input.prompt, input.allowedToolNames)) return 'tool_agent'
  if (containsAgentWorkCue(input.prompt.trim().toLowerCase())) return 'tool_agent'
  return 'light_agent'
}

function filterProviderVisibleToolsForRoute<T extends {
  name: string
  toolKind?: string
  providerKind?: ToolProviderKind
}>(tools: readonly T[], route: PromptRoute, prompt: string, allowedToolNames?: readonly string[]): T[] {
  if (route === 'direct_answer') return []
  if (allowedToolNames && allowedToolNames.length > 0) return [...tools]
  const allowSubagent = route === 'subagent_agent' || promptRequestsSubagentTools(prompt, allowedToolNames)
  const allowSkill = promptMentionsSkillTool(prompt, allowedToolNames)
  return tools.filter((tool) => {
    if (tool.providerKind === 'delegation' || tool.toolKind === 'subagent') return allowSubagent
    if (tool.providerKind === 'skill') return allowSkill
    return true
  })
}

function routeForAdvertisedTools(route: PromptRoute, tools: ReadonlyArray<{
  name: string
  toolKind?: string
  providerKind?: ToolProviderKind
}>): PromptRoute {
  if (route === 'direct_answer') return route
  for (const tool of tools) {
    if (tool.providerKind === 'delegation' || tool.toolKind === 'subagent') return 'subagent_agent'
    if (tool.providerKind === 'mcp' || tool.providerKind === 'skill' || tool.providerKind === 'gui') return 'tool_agent'
    if (['bash', 'write', 'write_file', 'edit', 'edit_file', 'multi_edit', 'move_file', 'notebook_edit', 'delete_range', 'delete_symbol', 'web_fetch'].includes(tool.name)) {
      return 'tool_agent'
    }
  }
  return route
}

function promptRequestsSubagentTools(prompt: string, allowedToolNames?: readonly string[]): boolean {
  if (toolScopeIncludesAny(allowedToolNames, ['delegate_task', 'task', 'parallel_tasks', 'wait', 'bash_output', 'kill_shell'])) {
    return true
  }
  const compact = promptAliasText(prompt)
  const cues = [
    'subagent',
    'sub agent',
    'child agent',
    'child task',
    'delegate',
    'delegate task',
    'parallel task',
    'parallel tasks',
    'parallel_tasks',
    'background job',
    'background task',
    'background via tools',
    'inspect background',
    'background status',
    'background output',
    'bash output',
    'bash_output',
    'kill shell',
    'kill_shell'
  ]
  if (cues.some((cue) => compact.includes(cue))) return true
  const raw = prompt.trim().toLowerCase()
  return ['子代理', '子任务', '并行任务', '分解任务', '后台任务', '等待子任务', '等待后台']
    .some((cue) => raw.includes(cue))
}

function promptMentionsSkillTool(prompt: string, allowedToolNames?: readonly string[]): boolean {
  if (toolScopeIncludesAny(allowedToolNames, ['run_skill', 'load_skill'])) return true
  const compact = promptAliasText(prompt)
  const raw = prompt.trim().toLowerCase()
  return compact.includes('skill') ||
    compact.includes('run skill') ||
    compact.includes('load skill') ||
    raw.includes('技能')
}

function promptAliasText(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, ' ').split(/\s+/).filter(Boolean).join(' ')
}

function toolScopeIncludesAny(toolNames: readonly string[] | undefined, expected: readonly string[]): boolean {
  if (!toolNames?.length) return false
  const expectedSet = new Set(expected)
  return toolNames.some((name) => expectedSet.has(name.trim()))
}

function containsAgentWorkCue(normalizedPrompt: string): boolean {
  const cues = [
    'analytix',
    'reasonix',
    'opencode',
    'codex',
    'mcp',
    'plugin',
    'skill',
    'subagent',
    'agent',
    'provider',
    'workspace',
    'thread',
    'deeplink',
    'git',
    'npm',
    'go test',
    '源码',
    '代码',
    '工作区',
    '文件',
    '目录',
    '插件',
    '技能',
    '子代理',
    '工具',
    '线程',
    '深链',
    '读取',
    '打开',
    '搜索',
    '运行',
    '执行',
    '调用',
    '操作',
    '修改',
    '修复',
    '创建',
    '生成',
    '构建',
    '启动',
    '拉取',
    '提交',
    '测试',
    '排查',
    '研究',
    '复核',
    '审核',
    '报告',
    '文档',
    '案件',
    '资金',
    '数据库',
    '截图',
    '图片',
    '附件',
    '错误'
  ]
  return cues.some((cue) => normalizedPrompt.includes(cue))
}

/**
 * Injected when the turn runs without an interactive user (IM bridges,
 * headless runs). The user-input tools are also withheld from the tool
 * catalog; this line keeps the model from promising a GUI dialog that
 * nobody can answer.
 */
function userInputUnavailableInstruction(): string {
  return [
    'Interactive user input is unavailable for this turn: the user is on a remote channel (IM) and cannot answer GUI prompts.',
    'Do not ask for structured input or wait for confirmation. If information is missing, state your assumption and continue, or finish your reply with the question so the user can answer in their next message.'
  ].join(' ')
}

function allowedToolNamesWithGuiStateTools(
  allowedToolNames: readonly string[] | undefined,
  activeGoal: boolean
): readonly string[] | undefined {
  if (!allowedToolNames) return allowedToolNames
  const next = new Set(allowedToolNames)
  if (activeGoal) {
    next.add(GET_GOAL_TOOL_NAME)
    next.add(COMPLETE_STEP_TOOL_NAME)
    next.add(UPDATE_GOAL_TOOL_NAME)
  }
  next.add(TODO_LIST_TOOL_NAME)
  next.add(TODO_WRITE_TOOL_NAME)
  return [...next]
}

/**
 * Intersect an optional allow-list with a hard-forced allow-list. Used to
 * clamp a subagent loop to read-only tools: the forced list wins, but any
 * narrower skill-imposed list is preserved. Returns the forced list when no
 * base restriction exists, and leaves the base untouched when nothing is
 * forced (the main agent path).
 */
function intersectAllowedToolNames(
  base: readonly string[] | undefined,
  forced: readonly string[] | undefined
): readonly string[] | undefined {
  if (!forced) return base
  if (!base) return [...forced]
  const forcedSet = new Set(forced)
  return base.filter((name) => forcedSet.has(name))
}

function advertisedToolNamesForExecution(
  tools: ReadonlyArray<{ name: string }>
): readonly string[] {
  return [...new Set(tools.map((tool) => tool.name))].sort()
}

export type AgentLoopOptions = {
  threadStore: ThreadStore
  sessionStore: SessionStore
  approvalGate: ApprovalGate
  userInputGate: UserInputGate
  model: ModelClient
  toolHost: ToolHost
  usage: UsageService
  events: RuntimeEventRecorder
  turns: TurnService
  inflight: InflightTracker
  steering: SteeringQueue
  compactor: ContextCompactor
  prefix: ImmutablePrefix
  ids: IdGenerator
  nowIso: () => string
  nowMs?: () => number
  modelCapabilities?: (model: string) => ModelCapabilityMetadata
  skillRuntime?: SkillRuntime
  attachmentStore?: AttachmentStore
  memoryStore?: MemoryStore
  runtimeEnvironment?: {
    commandDiagnostics?: (
      input: { workspace?: string }
    ) => Promise<readonly CommandProbeDiagnostic[]> | readonly CommandProbeDiagnostic[]
  }
  tokenEconomy?: TokenEconomyConfig
  contextCompaction?: ContextCompactionConfig
  stepLimits?: RuntimeStepLimitConfig
  toolStorm?: ToolStormBreakerOptions & { enabled?: boolean }
  toolArgumentRepair?: {
    maxStringBytes?: number
  }
  goalResume?: Pick<
    GoalResumeCoordinatorDeps,
    'setTimer' | 'maxNoProgressAttempts' | 'baseDelayMs' | 'maxDelayMs' | 'log'
  >
  /**
   * Hard allow-list intersected into every tool context for this loop. Used
   * by read-only subagents to clamp the inherited tool host to investigation
   * tools — enforced at both the schema (listTools) and execute layers.
   */
  forcedAllowedToolNames?: readonly string[]
  /**
   * Lifecycle hooks (UserPromptSubmit, TurnStart, TurnEnd, PreCompact).
   * Tool phases are handled by the tool host; the loop ignores them.
   */
  hooks?: readonly ResolvedHook[]
  /**
   * Optional fallback GUI plan context for embedders that run the loop
   * without persisted turn metadata. Normal serve mode reads GUI plan
   * context from the active turn record.
   */
  activePlanContext?: GuiPlanContext
  /**
   * Optional callback to mutate the active plan context (e.g. when the
   * loop records a successful `create_plan` result). The default is a
   * no-op for callers that don't track plan state.
   */
  onActivePlanContextChange?: (context: GuiPlanContext | undefined) => void
  onPlanWritten?: (input: {
    threadId: string
    turnId: string
    planId: string
    relativePath: string
    markdown: string
  }) => Promise<void>
}

/**
 * Cache-first agent loop. The loop:
 * 1. Drains pending steering text and injects it as user messages.
 * 2. Calls the model client with the immutable prefix + compacted history.
 * 3. Streams text, reasoning, and tool-call deltas; emits runtime events.
 * 4. Executes tool calls through the tool host with approval gating.
 * 5. Folds usage/cache telemetry into the per-thread snapshot.
 * 6. Triggers compaction when the history exceeds the soft threshold.
 *
 * The loop is driven by `runTurn(threadId, turnId)` and is fully
 * cancellable through the AbortSignal returned by `getAbortController`.
 */
export class AgentLoop {
  private readonly opts: AgentLoopOptions
  private readonly autoModelRoutes = new Map<string, AutoModelRouteSelection>()
  private readonly promptTokenPressure = new Map<string, { model: string; promptTokens: number }>()
  /** Threads for which a one-time pressure hydration from persisted usage was already attempted. */
  private readonly hydratedPressureThreads = new Set<string>()
  private readonly toolStormBreakers = new Map<string, ToolStormBreaker>()
  private readonly toolCatalogSnapshots = new Map<string, ToolCatalogSnapshot>()
  private readonly cachePrefixShapes = new Map<string, CachePrefixShape>()
  private readonly goalControl = new GoalControlMachine()
  private readonly turnFailures = new Map<string, TurnFailure>()
  private readonly turnMadeProgress = new Set<string>()
  /**
   * The active provider continuation needs the exact tool protocol payload,
   * but durable/public history must never contain it. Entries live only for
   * one running turn and can restore a public pair only when all identities
   * match the item persisted by that same turn.
   */
  private readonly requestLocalToolHistory = new Map<
    string,
    Map<string, { call?: InternalToolCallItem; result?: InternalToolResultItem }>
  >()
  private readonly goalResume: GoalResumeCoordinator

  constructor(opts: AgentLoopOptions) {
    this.opts = opts
    this.goalResume = new GoalResumeCoordinator({
      launch: (threadId) => this.launchGoalResumeTurn(threadId),
      getActiveGoalKey: async (threadId) => {
        const goal = (await this.opts.threadStore.get(threadId))?.goal
        return goal && goal.status === 'active' ? goalResumeKey(threadId, goal) : null
      },
      isThreadBusy: async (threadId) =>
        (await this.opts.threadStore.get(threadId))?.status === 'running',
      ...this.opts.goalResume
    })
  }

  shutdownGoalResume(): void {
    this.goalResume.shutdown()
  }

  async resumeInterruptedGoals(threadIds: readonly string[]): Promise<number> {
    let resumed = 0
    for (const threadId of threadIds) {
      if (await this.goalResume.resumeInterrupted(threadId)) resumed += 1
    }
    return resumed
  }

  /**
   * Run a turn end-to-end. The loop returns the final turn status
   * (completed, failed, or aborted). All errors are caught and
   * surfaced through the `error` runtime event.
   */
  async runTurn(threadId: string, turnId: string): Promise<'completed' | 'failed' | 'aborted'> {
    const signal = this.opts.turns.getAbortController(turnId)
    if (!signal) {
      await this.failTurn(threadId, turnId, 'no abort controller for turn')
      return 'failed'
    }
    if (signal.aborted) {
      await this.opts.turns.finishTurn({ threadId, turnId, status: 'aborted' })
      return 'aborted'
    }
    let goalTimer: GoalElapsedTimer | null = null
    let finalStatus: 'completed' | 'failed' | 'aborted' | undefined
    let finalError: string | undefined
    try {
      goalTimer = await this.startGoalElapsedTimer(threadId)
      await this.recordPipelineStage(threadId, turnId, 'setup')
      if (this.opts.toolStorm?.enabled !== false) {
        this.toolStormBreakers.set(turnId, new ToolStormBreaker(this.opts.toolStorm))
      }
      await this.recordPipelineStage(threadId, turnId, 'pre_start')
      const denial = await this.runTurnStartLifecycleHooks(threadId, turnId)
      if (denial) {
        await this.opts.events.record({
          kind: 'error',
          threadId,
          turnId,
          message: denial,
          code: 'hook_denied',
          severity: 'error'
        })
        await this.opts.turns.applyItem(
          threadId,
          makeErrorItem({
            id: this.opts.ids.next('item_error'),
            turnId,
            threadId,
            message: denial,
            code: 'hook_denied',
            severity: 'error'
          })
        )
        await this.opts.turns.finishTurn({ threadId, turnId, status: 'failed', error: denial })
        finalStatus = 'failed'
        finalError = denial
        return 'failed'
      }
      await this.drainSteering(threadId, turnId, signal)
      await this.recordPipelineStage(threadId, turnId, 'post_start')
      const status = await this.loop(threadId, turnId, signal)
      const failure = status === 'failed' ? this.turnFailures.get(turnId) : undefined
      await this.opts.turns.finishTurn({
        threadId,
        turnId,
        status,
        ...(failure ?? {})
      })
      finalStatus = status
      finalError = failure?.error
      return status
    } catch (error) {
      const raw = error instanceof Error ? error.message : String(error)
      // Best-effort enrichment so the renderer can show "what failed where"
      // instead of the bare "Analytix turn failed" string. See issue #26.
      const modelInfo = this.opts.model && 'config' in this.opts.model
        ? (this.opts.model as { config: { model?: string; baseUrl?: string } }).config
        : undefined
      const modelName = modelInfo?.model ?? 'unknown'
      const provider = modelInfo?.baseUrl ?? 'unknown'
      const stack = error instanceof Error
        ? (error.stack?.split('\n').slice(0, 3).join(' | ') ?? '')
        : ''
      const message = [
        '[Analytix turn failed]',
        `turn=${turnId}`,
        `thread=${threadId}`,
        `model=${modelName}`,
        `provider=${provider}`,
        `error=${raw}`,
        stack ? `stack=${stack}` : ''
      ].filter(Boolean).join(' ')
      await this.failTurn(threadId, turnId, message)
      finalStatus = 'failed'
      finalError = message
      return 'failed'
    } finally {
      await this.finishGoalElapsedTimer(threadId, goalTimer)
      await this.evaluateGoalResume(threadId, turnId, finalStatus ?? 'failed')
      this.autoModelRoutes.delete(autoModelRouteKey(threadId, turnId, AUTO_MODEL_ROUTER_FINGERPRINT))
      this.toolStormBreakers.delete(turnId)
      this.goalControl.clearTurn(turnId)
      this.turnMadeProgress.delete(turnId)
      this.turnFailures.delete(turnId)
      this.requestLocalToolHistory.delete(turnId)
      await this.runTurnEndHooks(threadId, turnId, finalStatus ?? 'failed', finalError)
    }
  }

  private async evaluateGoalResume(
    threadId: string,
    turnId: string,
    finalStatus: 'completed' | 'failed' | 'aborted'
  ): Promise<void> {
    const thread = await this.opts.threadStore.get(threadId)
    const goal = thread?.goal
    if (!thread || !goal || goal.status !== 'active') {
      this.goalResume.clear(threadId)
      return
    }
    const turn = thread.turns.find((t) => t.id === turnId)
    const wasPlanTurn = turn?.mode === 'plan' || Boolean(turn?.guiPlan)
    if (finalStatus !== 'failed' || wasPlanTurn) {
      this.goalResume.clear(threadId)
      return
    }
    const outcome = this.goalResume.noteGoalTurnFailed({
      threadId,
      goalKey: goalResumeKey(threadId, goal),
      madeProgress: this.turnMadeProgress.has(turnId)
    })
    if (outcome === 'exhausted') {
      await this.transitionGoalStatus(
        threadId,
        turnId,
        'blocked',
        `Goal auto-resume stopped: ${DEFAULT_MAX_GOAL_RESUME_NO_PROGRESS_ATTEMPTS} consecutive attempts made no progress. Set the goal active again to retry.`
      )
    }
  }

  private async launchGoalResumeTurn(threadId: string): Promise<void> {
    const thread = await this.opts.threadStore.get(threadId)
    const goal = thread?.goal
    if (!thread || !goal || goal.status !== 'active') return
    const lastTurn = thread.turns[thread.turns.length - 1]
    const started = await this.opts.turns.startTurn({
      threadId,
      request: {
        prompt: GOAL_RESUME_PROMPT,
        mode: 'agent',
        ...(lastTurn?.disableUserInput ? { disableUserInput: true } : {})
      }
    })
    await this.opts.events.record({
      kind: 'error',
      threadId,
      turnId: started.turnId,
      message: 'Auto-resuming the active goal after an interrupted turn.',
      code: 'goal_auto_resume',
      severity: 'warning'
    })
    void this.runTurn(threadId, started.turnId)
  }

  private async transitionGoalStatus(
    threadId: string,
    turnId: string,
    status: ThreadGoal['status'],
    message?: string,
    options: GoalStatusTransitionOptions = {}
  ): Promise<boolean> {
    const current = await this.opts.threadStore.get(threadId)
    const goal = current?.goal
    if (!current || !goal || goal.status === status) return false
    const now = this.opts.nowIso()
    const next: ThreadGoal = {
      ...goal,
      status,
      updatedAt: now,
      ...(status === 'blocked' && options.blockedReason
        ? {
            blockedReason: options.blockedReason,
            blockedTurnId: options.blockedTurnId ?? turnId
          }
        : {})
    }
    await this.opts.threadStore.upsert(touchThread({ ...current, goal: next }, now))
    await this.opts.events.record({ kind: 'goal_updated', threadId, goal: next })
    if (message) {
      await this.opts.events.record({
        kind: 'error',
        threadId,
        turnId,
        message,
        code: options.code ?? 'goal_auto_resume_exhausted',
        ...(options.details !== undefined ? { details: options.details } : {}),
        severity: options.severity ?? 'warning'
      })
    }
    return true
  }

  /**
   * TurnStart (observe-only) then UserPromptSubmit hooks. Returns the
   * denial message when a UserPromptSubmit hook blocks the turn.
   * Accepted `additionalContext` is persisted as an extra user message
   * so replays and the prompt cache see a stable history.
   */
  private async runTurnStartLifecycleHooks(threadId: string, turnId: string): Promise<string | undefined> {
    const hooks = this.opts.hooks
    const hasStart = hasHooksForPhase(hooks, 'TurnStart')
    const hasSubmit = hasHooksForPhase(hooks, 'UserPromptSubmit')
    if (!hasStart && !hasSubmit) return undefined
    const turn = await this.opts.turns.getTurn(threadId, turnId)
    const thread = await this.opts.threadStore.get(threadId)
    const payload = {
      threadId,
      turnId,
      prompt: turn?.prompt ?? '',
      ...(thread?.workspace ? { workspace: thread.workspace } : {})
    }
    if (hasStart) {
      const started = await runObserverHooks(hooks, { phase: 'TurnStart', ...payload })
      await this.recordHookWarnings(threadId, turnId, started.warnings)
    }
    if (!hasSubmit) return undefined
    const submit = await runUserPromptSubmitHooks(hooks, payload)
    await this.recordHookWarnings(threadId, turnId, submit.warnings)
    if (submit.denied) return submit.denied
    if (submit.additionalContext.length > 0) {
      const now = this.opts.nowIso()
      const item: TurnItem = {
        id: this.opts.ids.next('item_hook'),
        turnId,
        threadId,
        role: 'user',
        status: 'completed',
        createdAt: now,
        finishedAt: now,
        kind: 'user_message',
        text: `<hook-context>\n${submit.additionalContext.join('\n\n')}\n</hook-context>`
      }
      await this.opts.turns.applyItem(threadId, item)
    }
    return undefined
  }

  /** Observe-only TurnEnd hooks; run after the turn is finalized and must never throw. */
  private async runTurnEndHooks(
    threadId: string,
    turnId: string,
    status: 'completed' | 'failed' | 'aborted',
    error?: string
  ): Promise<void> {
    if (!hasHooksForPhase(this.opts.hooks, 'TurnEnd')) return
    try {
      const outcome = await runObserverHooks(this.opts.hooks, {
        phase: 'TurnEnd',
        threadId,
        turnId,
        status,
        ...(error ? { error } : {})
      })
      await this.recordHookWarnings(threadId, turnId, outcome.warnings)
    } catch {
      // Observe-only: a TurnEnd hook must never break turn cleanup.
    }
  }

  private async recordHookWarnings(
    threadId: string,
    turnId: string,
    warnings: readonly string[]
  ): Promise<void> {
    for (const message of warnings) {
      await this.opts.events.record({
        kind: 'error',
        threadId,
        turnId,
        message,
        code: 'hook_warning',
        severity: 'warning'
      })
    }
  }

  private async failTurn(threadId: string, turnId: string, message: string): Promise<void> {
    await this.opts.turns.finishTurn({ threadId, turnId, status: 'failed', error: message })
  }

  private rememberTurnFailure(turnId: string, failure: TurnFailure): void {
    if (!failure.error.trim()) return
    this.turnFailures.set(turnId, failure)
  }

  private async modelClientDiagnostics(input: {
    threadId: string
    model: string
    providerId?: string
  }): Promise<ModelClientDiagnostics> {
    if (typeof this.opts.model.diagnosticsForRequest === 'function') {
      const diagnostics = await this.opts.model.diagnosticsForRequest(input)
      return {
        ...diagnostics,
        ...(diagnostics.providerBaseUrl
          ? { providerBaseUrl: sanitizeProviderBaseUrl(diagnostics.providerBaseUrl) }
          : {})
      }
    }
    const client = this.opts.model as ModelClient & {
      config?: {
        baseUrl?: string
        endpointFormat?: string
        model?: string
      }
    }
    return {
      provider: client.provider,
      ...(client.config?.baseUrl ? { providerBaseUrl: sanitizeProviderBaseUrl(client.config.baseUrl) } : {}),
      ...(client.config?.endpointFormat ? { endpointFormat: client.config.endpointFormat } : {}),
      ...(client.config?.model ? { configuredModel: client.config.model } : {})
    }
  }

  private nowMs(): number {
    return this.opts.nowMs?.() ?? Date.now()
  }

  private async startGoalElapsedTimer(threadId: string): Promise<GoalElapsedTimer | null> {
    const thread = await this.opts.threadStore.get(threadId)
    const goal = thread?.goal
    if (!goal || goal.status !== 'active') return null
    return {
      startedAtMs: this.nowMs(),
      createdAt: goal.createdAt,
      objective: goal.objective
    }
  }

  private async finishGoalElapsedTimer(
    threadId: string,
    timer: GoalElapsedTimer | null
  ): Promise<void> {
    if (!timer) return
    const elapsedSeconds = Math.floor(Math.max(0, this.nowMs() - timer.startedAtMs) / 1000)
    if (elapsedSeconds <= 0) return

    const current = await this.opts.threadStore.get(threadId)
    const currentGoal = current?.goal
    if (!current || !currentGoal) return
    if (currentGoal.createdAt !== timer.createdAt || currentGoal.objective !== timer.objective) {
      return
    }

    const now = this.opts.nowIso()
    const goal: ThreadGoal = {
      ...currentGoal,
      timeUsedSeconds: (currentGoal.timeUsedSeconds ?? 0) + elapsedSeconds,
      updatedAt: now
    }
    const updated = touchThread({ ...current, goal }, now)
    await this.opts.threadStore.upsert(updated)
    await this.opts.events.record({
      kind: 'goal_updated',
      threadId,
      goal
    })
  }

  private async drainSteering(threadId: string, turnId: string, signal: AbortSignal): Promise<void> {
    const pending = this.opts.steering.drain()
    if (pending.length === 0) return
    for (const entry of pending) {
      const itemId = entry.id?.trim() || entry.clientUserMessageId?.trim() || this.opts.ids.next('item_steered')
      const item: TurnItem = {
        id: itemId,
        turnId,
        threadId,
        role: 'user',
        status: 'completed',
        createdAt: this.opts.nowIso(),
        finishedAt: this.opts.nowIso(),
        kind: 'user_message',
        text: entry.text,
        delivery: 'steer',
        ...(entry.displayText ? { displayText: entry.displayText } : {}),
        ...(entry.clientUserMessageId ? { clientUserMessageId: entry.clientUserMessageId } : {}),
        ...(entry.attachmentIds?.length ? { attachmentIds: entry.attachmentIds } : {}),
        ...(entry.fileReferences?.length ? { fileReferences: entry.fileReferences } : {})
      }
      await this.opts.turns.applyItem(threadId, item)
    }
    void signal
  }

  private async loop(
    threadId: string,
    turnId: string,
    signal: AbortSignal
  ): Promise<'completed' | 'failed' | 'aborted'> {
    const [thread, turn] = await Promise.all([
      this.opts.threadStore.get(threadId),
      this.opts.turns.getTurn(threadId, turnId)
    ])
    const modelStepLimit = resolveRuntimeModelStepLimit(this.opts.stepLimits, {
      mode: turn?.mode ?? thread?.mode,
      disableUserInput: turn?.disableUserInput === true,
      threadMaxModelSteps: thread?.runtimeStepLimits?.maxModelSteps,
      turnMaxModelSteps: turn?.maxModelSteps
    })
    for (let step = 0; ; step += 1) {
      if (signal.aborted) return 'aborted'
      if (modelStepLimit > 0 && step >= modelStepLimit) {
        const message =
          `Turn stopped after ${modelStepLimit} model steps without reaching a final response.`
        this.rememberTurnFailure(turnId, {
          error: message,
          code: 'turn_step_limit_exceeded',
          details: { maxModelSteps: modelStepLimit },
          severity: 'error'
        })
        await this.opts.events.record({
          kind: 'error',
          threadId,
          turnId,
          message,
          code: 'turn_step_limit_exceeded',
          details: { maxModelSteps: modelStepLimit },
          severity: 'error'
        })
        await this.opts.turns.applyItem(
          threadId,
          makeErrorItem({
            id: this.opts.ids.next('item_error'),
            turnId,
            threadId,
            message,
            code: 'turn_step_limit_exceeded',
            severity: 'error'
          })
        )
        return 'failed'
      }
      if (thread?.goal?.status === 'active' && step >= DEFAULT_GOAL_AUTO_MODEL_STEP_LIMIT) {
        const stopped = await this.stopActiveGoalAtAutoTurnLimit(threadId, turnId)
        if (stopped) return 'failed'
      }
      await this.drainSteering(threadId, turnId, signal)
      const stepResult = await this.modelStep(threadId, turnId, signal, step)
      if (stepResult === 'stop') return 'completed'
      if (stepResult === 'failed') return 'failed'
      if (stepResult === 'aborted') return 'aborted'
    }
  }

  private async stopActiveGoalAtAutoTurnLimit(threadId: string, turnId: string): Promise<boolean> {
    const current = await this.opts.threadStore.get(threadId)
    if (current?.goal?.status !== 'active') return false
    const details = { maxGoalModelSteps: DEFAULT_GOAL_AUTO_MODEL_STEP_LIMIT }
    const blockedReason = 'goal continuation limit reached'
    const message =
      `Goal auto-turn stopped: ${blockedReason} after ${DEFAULT_GOAL_AUTO_MODEL_STEP_LIMIT} model steps.`
    const transitioned = await this.transitionGoalStatus(threadId, turnId, 'blocked', message, {
      code: 'goal_auto_turn_limit_exceeded',
      details,
      severity: 'warning',
      blockedReason,
      blockedTurnId: turnId
    })
    if (!transitioned) return false
    this.rememberTurnFailure(turnId, {
      error: message,
      code: 'goal_auto_turn_limit_exceeded',
      details,
      severity: 'warning'
    })
    await this.opts.turns.applyItem(
      threadId,
      makeErrorItem({
        id: this.opts.ids.next('item_error'),
        turnId,
        threadId,
        message,
        code: 'goal_auto_turn_limit_exceeded',
        details,
        severity: 'warning'
      })
    )
    return true
  }

  private async modelStep(
    threadId: string,
    turnId: string,
    signal: AbortSignal,
    stepIndex = 0
  ): Promise<'continue' | 'stop' | 'failed' | 'aborted'> {
    if (shouldVerifyImmutablePrefix()) {
      verifyImmutablePrefix(this.opts.prefix)
    }
    const [thread, turn] = await Promise.all([
      this.opts.threadStore.get(threadId),
      this.opts.turns.getTurn(threadId, turnId)
    ])
    await this.recordPipelineStage(threadId, turnId, 'input_received', { stepIndex })
    const candidatePlanContext = turn?.guiPlan
      ? { ...turn.guiPlan, turnId }
      : this.opts.activePlanContext
    const candidatePlanContextStale = isStalePlanContext(candidatePlanContext, thread?.workspace ?? '')
    const activePlanContext = candidatePlanContextStale
      ? undefined
      : candidatePlanContext
    const budgetGate = await this.checkBudgetGate(thread, threadId, turnId)
    if (budgetGate === 'blocked') return 'stop'
    const loadedItems = await this.opts.sessionStore.loadItems(threadId)
    // Heal (and possibly rewrite) on-disk history once per turn: within a
    // turn the loop only appends well-formed items, and healing's deep
    // change detection costs two full-history stringifies per call.
    let historyItems: TurnItem[] = loadedItems
    if (stepIndex === 0) {
      const healed = healLoadedHistoryItems(loadedItems)
      if (healed.changed) {
        await this.opts.sessionStore.rewriteItems(threadId, healed.items)
      }
      historyItems = healed.items
    }
    await this.recordPipelineStage(
      threadId,
      turnId,
      'input_cached',
      prefixVolatilityStageDetails(detectVolatilePrefixContent(this.opts.prefix))
    )
    if (stepIndex > 0) {
      const toolResultCount = historyItems.filter(
        (item) => item.turnId === turnId && item.kind === 'tool_result'
      ).length
      await this.opts.events.record({
        kind: 'tool_result_upload_wait',
        threadId,
        turnId,
        status: 'waiting',
        toolResultCount
      })
    }
    const items = repairModelHistoryItems(
      effectiveHistoryAfterLatestCompaction(historyItems)
    )
    const approvalPolicy = normalizeApprovalPolicy(thread?.approvalPolicy)
    const sandboxMode = normalizeSandboxMode(thread?.sandboxMode)
    // Per-turn mode overrides the thread mode so the GUI can toggle
    // Plan/agent (and run Build as agent) without recreating the thread.
    const effectiveMode = turn?.mode ?? thread?.mode
    const currentMaxModelSteps = resolveRuntimeModelStepLimit(this.opts.stepLimits, {
      mode: effectiveMode,
      disableUserInput: turn?.disableUserInput === true,
      threadMaxModelSteps: thread?.runtimeStepLimits?.maxModelSteps,
      turnMaxModelSteps: turn?.maxModelSteps
    })
    const modelRoute = await this.resolveTurnModel({
      threadId,
      turnId,
      latestRequest: turn?.prompt ?? '',
      items,
      signal,
      reasoningEffort: turn?.reasoningEffort,
      candidates: [turn?.model, thread?.model, this.opts.model.model]
    })
    await this.recordPipelineStage(threadId, turnId, 'input_routed', {
      model: modelRoute.model,
      ...(modelRoute.reasoningEffort ? { reasoningEffort: modelRoute.reasoningEffort } : {})
    })
    const model = modelRoute.model
    const modelCapabilities = this.opts.modelCapabilities?.(model) ?? modelCapabilitiesForModel(model)
    const modelClientDiagnostics = await this.modelClientDiagnostics({
      threadId,
      model,
      providerId: thread?.providerId
    })
    const attachments = await this.resolveAttachments({
      attachmentIds: turn?.attachmentIds ?? [],
      threadId,
      workspace: thread?.workspace ?? '',
      modelCapabilities
    })
    const skillResolution = this.opts.skillRuntime?.resolveTurn({
      prompt: turn?.prompt ?? '',
      workspace: thread?.workspace ?? ''
    }) ?? {
      activeSkillIds: [],
      activations: [],
      instructions: [],
      injectedBytes: 0
    }
    const memories = await this.retrieveMemories({
      prompt: turn?.prompt ?? '',
      workspace: thread?.workspace ?? ''
    })
    const compiledMemoryContext = compileMemoryContext(memories)
    this.opts.memoryStore?.setLastInjected(compiledMemoryContext.memoryIds)
    const staleGuiPlanTurn = Boolean(turn?.guiPlan) && candidatePlanContextStale
    const planTurnActive = !staleGuiPlanTurn && (effectiveMode === 'plan' || Boolean(activePlanContext))
    const toolContextMode = planTurnActive ? 'plan' : effectiveMode === 'plan' ? 'agent' : effectiveMode
    const activeGoalInstruction = planTurnActive
      ? null
      : this.goalControl.activeInstruction(thread?.goal)
    const activeTodoInstruction = planTurnActive
      ? null
      : todoContinuationInstruction(thread?.todos)
    const allowedToolNames = intersectAllowedToolNames(
      allowedToolNamesWithGuiStateTools(
        skillResolution.allowedToolNames,
        activeGoalInstruction !== null
      ),
      this.opts.forcedAllowedToolNames
    )
    // IM/headless turns run without the user-input gate; the tools key
    // their advertisement off `awaitUserInput`, so omitting it hides
    // `user_input`/`request_user_input` and rejects stray calls.
    const userInputDisabled = turn?.disableUserInput === true
    const modelExecutionSource: ModelExecutionSource = thread?.providerId ? 'thread' : 'runtime-default'
    const preliminaryModelExecution = buildLoopModelExecutionRef({
      modelId: model,
      providerId: modelClientDiagnostics.providerId ?? thread?.providerId,
      endpointFormat: modelClientDiagnostics.endpointFormat ?? modelCapabilities.endpointFormat,
      providerBaseUrl: modelClientDiagnostics.providerBaseUrl,
      modelCapabilities,
      source: modelExecutionSource,
      resolvedAt: this.opts.nowIso()
    })
    const toolContext: ToolHostContext = {
      threadId,
      turnId,
      workspace: thread?.workspace ?? '',
      threadMode: toolContextMode,
      ...(activePlanContext ? { guiPlan: activePlanContext } : {}),
      model: modelCapabilities,
      ...(preliminaryModelExecution ? { modelExecution: preliminaryModelExecution } : {}),
      activeSkillIds: skillResolution.activeSkillIds,
      memoryPolicy: { enabled: Boolean(this.opts.memoryStore) },
      delegationPolicy: { enabled: false },
      runtimeStepLimits: { currentMaxModelSteps },
      ...(allowedToolNames ? { allowedToolNames } : {}),
      approvalPolicy,
      sandboxMode,
      abortSignal: signal,
      awaitApproval: async () => 'allow',
      ...(userInputDisabled
        ? {}
        : { awaitUserInput: (input) => this.awaitUserInput(threadId, turnId, input, signal) })
    }
    const promptText = turn?.prompt ?? latestUserMessageText(historyItems, turnId)
    const preliminaryPromptRoute = promptRouteForPrompt({
      prompt: promptText,
      allowedToolNames,
      planTurnActive
    })
    const listedTools = preliminaryPromptRoute === 'direct_answer'
      ? []
      : await this.opts.toolHost.listTools(toolContext)
    const tools = filterProviderVisibleToolsForRoute(listedTools, preliminaryPromptRoute, promptText, allowedToolNames)
    const promptRoute = routeForAdvertisedTools(preliminaryPromptRoute, tools)
    const toolSpecs: ModelToolSpec[] = tools
    const toolProviderMetadata = new Map(
      tools.map((tool) => [tool.name, { providerId: tool.providerId, providerKind: tool.providerKind }])
    )
    const activeAllowedToolNames = advertisedToolNamesForExecution(tools)
    const toolCatalog = buildToolCatalogFingerprint(toolSpecs)
    const toolCatalogDrift = this.recordToolCatalogFingerprint({
      threadId,
      workspace: thread?.workspace ?? '',
      mode: toolContextMode ?? 'agent',
      route: promptRoute,
      model: modelCapabilities.id,
      activeSkillIds: skillResolution.activeSkillIds,
      allowedToolNames,
      userInputDisabled,
      fingerprint: toolCatalog.fingerprint,
      toolNames: toolCatalog.toolNames,
      toolHashes: toolCatalog.toolHashes
    })
    const toolCatalogDriftMessage = toolCatalogDrift.kind !== 'none'
      ? buildToolCatalogDriftMessage(toolCatalog, toolCatalogDrift.kind)
      : undefined
    if (toolCatalogDrift.kind !== 'none' && toolCatalogDriftMessage) {
      await this.recordToolCatalogDrift({
        threadId,
        turnId,
        fingerprint: toolCatalog.fingerprint,
        toolCount: toolCatalog.toolCount,
        toolNames: toolCatalog.toolNames,
        changeKind: toolCatalogDrift.kind,
        message: toolCatalogDriftMessage
      })
    }
    if (turn) {
      await this.opts.turns.updateTurnMetadata(threadId, turnId, {
        activeSkillIds: skillResolution.activeSkillIds,
        skillInjectionBytes: skillResolution.injectedBytes,
        injectedMemoryIds: compiledMemoryContext.memoryIds,
        toolCatalogFingerprint: toolCatalog.fingerprint,
        toolCatalogToolCount: toolCatalog.toolCount,
        toolCatalogDrift: toolCatalogDrift.kind !== 'none'
      })
    }
    if (toolCatalogDrift.kind === 'breaking') return 'stop'
    const toolKinds = new Map(toolSpecs.map((tool) => [tool.name, tool.toolKind]))
    const createPlanSatisfied = planTurnActive
      ? hasSuccessfulCreatePlanResult(historyItems, turnId)
      : false
    const requiredToolName =
      planTurnActive &&
      !createPlanSatisfied &&
      toolSpecs.some((tool) => tool.name === CREATE_PLAN_TOOL_NAME)
        ? CREATE_PLAN_TOOL_NAME
        : undefined
    const effectiveToolSpecs = resolvePlanModeToolSpecs(toolSpecs, {
      planTurnActive,
      createPlanSatisfied,
      stepIndex
    })
    const history = await this.compactIfNeeded(items, model, signal, {
      threadId,
      turnId,
      visibleItems: historyItems,
      toolSpecs: effectiveToolSpecs
    })
    const modelHistory = this.restoreRequestLocalToolHistory(history, turnId)
    const transcriptHardening = buildTranscriptHardeningContext(modelHistory)
    if (signal.aborted) return 'aborted'
    await this.recordPipelineStage(threadId, turnId, 'input_compressed', {
      historyItems: history.length
    })
    const runtimeContextInstruction = shouldInjectInitialRuntimeContext({
      stepIndex,
      turnId,
      historyItems
    })
      ? buildRuntimeContextInstruction({
          workspace: thread?.workspace,
          nowIso: this.opts.nowIso()
        })
      : null
    const runtimeEnvironmentInstruction = await buildRuntimeEnvironmentInstructionForLoop(
      this.opts.runtimeEnvironment,
      { workspace: thread?.workspace }
    )
    if (signal.aborted) return 'aborted'
    const contextInstructions = [
      ...skillResolution.instructions,
      ...(runtimeContextInstruction ? [runtimeContextInstruction] : []),
      ...(runtimeEnvironmentInstruction ? [runtimeEnvironmentInstruction] : []),
      ...transcriptHardening.instructions,
      ...(activeGoalInstruction ? [activeGoalInstruction] : []),
      ...this.goalControl.recoveryInstructions(turnId, activeGoalInstruction),
      ...(activeTodoInstruction ? [activeTodoInstruction] : []),
      ...compiledMemoryContext.instructions,
      ...(userInputDisabled ? [userInputUnavailableInstruction()] : []),
      ...(effectiveToolSpecs.some((tool) => tool.name === 'bash') ? [shellRuntimeInstruction()] : []),
      ...imageGenerationReferenceInstructions({
        imageAttachments: attachments.imageAttachments,
        textFallbacks: attachments.textFallbacks,
        workspace: thread?.workspace ?? '',
        tools: effectiveToolSpecs
      }),
      ...(toolCatalogDriftMessage ? [toolCatalogDriftMessage] : [])
    ]
    await this.recordPipelineStage(threadId, turnId, 'input_remembered', {
      memoryCount: compiledMemoryContext.retainedCount,
      memoryOriginalCount: compiledMemoryContext.originalCount,
      memoryOmittedCount: compiledMemoryContext.omittedMemoryIds.length,
      memoryDuplicateCount: compiledMemoryContext.duplicateCount,
      memoryContextChars: compiledMemoryContext.totalChars,
      memoryContextTruncated: compiledMemoryContext.truncated,
      transcriptHardening: transcriptHardening.findings.length > 0,
      transcriptHardeningFindings: transcriptHardening.findings.length,
      transcriptHardeningSources: transcriptHardening.sourceKinds,
      contextInstructionCount: contextInstructions.length
    })
    const tokenEconomy = normalizeTokenEconomyConfig(this.opts.tokenEconomy)
    const baseRequest: ModelRequest = {
      threadId,
      turnId,
      model,
      ...(thread?.providerId ? { providerId: thread.providerId } : {}),
      systemPrompt: this.opts.prefix.systemPrompt,
      ...(planTurnActive ? { modeInstruction: PLAN_MODE_INSTRUCTION } : {}),
      ...(contextInstructions.length ? { contextInstructions } : {}),
      prefix: this.opts.prefix.fewShots,
      history: capToolResultImages(modelHistory, MAX_FORWARDED_TOOL_IMAGES),
      ...(attachments.imageAttachments.length ? { attachments: attachments.imageAttachments } : {}),
      ...(attachments.textFallbacks.length ? { attachmentTextFallbacks: attachments.textFallbacks } : {}),
      tools: effectiveToolSpecs,
      ...(requiredToolName ? { requiredToolName } : {}),
      ...(modelRoute.reasoningEffort ? { reasoningEffort: modelRoute.reasoningEffort } : {}),
      abortSignal: signal
    }
    const rawInputTokens = tokenEconomy.enabled
      ? estimateModelRequestInputTokens(baseRequest)
      : 0
    const economyRequest = applyTokenEconomyToRequest(baseRequest, tokenEconomy)
    const toolSnipHints = new Map(
      effectiveToolSpecs
        .filter((tool) => tool.snipHint)
        .map((tool) => [tool.name, tool.snipHint!])
    )
    const request: ModelRequest = {
      ...economyRequest,
      // Model-bound only: keep persisted sessions intact while bounding stale
      // tool output across the full request history.
      history: applyRequestHistoryHygiene(economyRequest.history, tokenEconomy.historyHygiene, {
        ...(toolSnipHints.size ? { toolSnipHints } : {})
      })
    }
    if (tokenEconomy.enabled) {
      await this.recordTokenEconomySavings({
        threadId,
        turnId,
        model,
        rawInputTokens,
        sentInputTokens: estimateModelRequestInputTokens(request)
      })
    }
    const textAccumulator: { value: string } = { value: '' }
    let textItemId = ''
    const completedToolCalls: ToolCallLike[] = []
    const partialToolCallCards = new Set<string>()
    let stopReason: 'stop' | 'tool_calls' | 'length' | 'error' = 'stop'
    const modelExecution = buildLoopModelExecutionRef({
      modelId: request.model,
      providerId: modelClientDiagnostics.providerId ?? request.providerId ?? thread?.providerId,
      endpointFormat: modelClientDiagnostics.endpointFormat ?? modelCapabilities.endpointFormat,
      providerBaseUrl: modelClientDiagnostics.providerBaseUrl,
      modelCapabilities,
      toolCatalogFingerprint: toolCatalog.fingerprint,
      source: (request.providerId ?? thread?.providerId) ? 'thread' : 'runtime-default',
      resolvedAt: this.opts.nowIso()
    })
    const cachePrefixShape = captureCachePrefixShape({
      systemPrompt: request.systemPrompt,
      modeInstruction: request.modeInstruction,
      prefix: request.prefix,
      tools: request.tools,
      toolSources: advertisedToolSourcesForCacheDiagnostics(tools),
      toolCount: request.tools.length,
      route: promptRoute,
      model: request.model,
      provider: modelClientDiagnostics.provider,
      providerId: modelClientDiagnostics.providerId,
      endpointFormat: modelClientDiagnostics.endpointFormat
    })
    const previousCachePrefixShape = this.cachePrefixShapes.get(threadId) ?? cachePrefixShape
    let sawUsage = false
    let persistedText = false
    const providerStartedAtMs = this.nowMs()
    let firstTokenLatencyMs: number | undefined
    const markFirstToken = (): void => {
      if (firstTokenLatencyMs === undefined) {
        firstTokenLatencyMs = Math.max(0, this.nowMs() - providerStartedAtMs)
      }
    }
    const persistAccumulatedResponse = async (): Promise<void> => {
      if (!persistedText && textAccumulator.value) {
        persistedText = true
        const itemId = textItemId || this.opts.ids.next('item_text')
        await this.opts.turns.applyItem(
          threadId,
          makeAssistantTextItem({
            id: itemId,
            turnId,
            threadId,
            text: textAccumulator.value,
            status: 'completed'
          })
        )
      }
    }
    await this.recordPipelineStage(threadId, turnId, 'pre_send', {
      model: request.model,
      ...modelClientDiagnostics,
      route: promptRoute,
      historyItems: request.history.length,
      toolCount: request.tools.length,
      ...(request.requiredToolName ? { requiredToolName: request.requiredToolName } : {}),
      ...attachmentRequestPipelineDetails({
        attachmentIds: turn?.attachmentIds ?? [],
        imageAttachments: attachments.imageAttachments,
        textFallbacks: attachments.textFallbacks,
        modelCapabilities
      })
    })
    await this.recordPipelineStage(threadId, turnId, 'post_send', {
      model: request.model,
      ...modelClientDiagnostics
    })
    for await (const chunk of this.opts.model.stream(request)) {
      if (signal.aborted) {
        await persistAccumulatedResponse()
        return 'aborted'
      }
      switch (chunk.kind) {
        case 'assistant_text_delta':
          textItemId ||= this.opts.ids.next('item_text')
          textAccumulator.value += chunk.text
          if (chunk.text) markFirstToken()
          await this.opts.events.record({
            kind: 'assistant_text_delta',
            threadId,
            turnId,
            itemId: textItemId,
            item: makeAssistantTextItem({
              id: textItemId,
              turnId,
              threadId,
              text: chunk.text,
              status: 'running'
            })
          })
          break
        case 'assistant_reasoning_delta':
          if (chunk.text) markFirstToken()
          break
        case 'tool_call_delta':
          if (chunk.toolName && !partialToolCallCards.has(chunk.callId)) {
            markFirstToken()
            partialToolCallCards.add(chunk.callId)
            await this.opts.events.record({
              kind: 'tool_call_ready',
              threadId,
              turnId,
              callId: chunk.callId,
              toolName: chunk.toolName,
              readyCount: completedToolCalls.length + 1
            })
          }
          break
        case 'tool_call_complete': {
          markFirstToken()
          partialToolCallCards.add(chunk.callId)
          const provider = toolProviderMetadata.get(chunk.toolName)
          const toolKind = toolKinds.get(chunk.toolName)
          const repaired = repairDispatchToolArguments(chunk.arguments, {
            toolName: chunk.toolName,
            ...(toolKind ? { toolKind } : {}),
            ...(this.opts.toolArgumentRepair?.maxStringBytes !== undefined
              ? { maxStringBytes: this.opts.toolArgumentRepair.maxStringBytes }
              : {})
          })
          completedToolCalls.push({
            callId: chunk.callId,
            toolName: chunk.toolName,
            ...(provider?.providerId ? { providerId: provider.providerId } : {}),
            toolKind,
            arguments: repaired.arguments
          })
          const itemId = `item_tool_${turnId}_${chunk.callId}`
          const privateCallItem = makePrivateToolCallItem({
            id: itemId,
            turnId,
            threadId,
            callId: chunk.callId,
            toolName: chunk.toolName,
            toolKind,
            arguments: repaired.arguments,
            ...(repaired.notes.length
              ? { summary: `Repaired tool arguments: ${repaired.notes.join('; ')}` }
              : {})
          })
          this.rememberRequestLocalToolCall(privateCallItem)
          await this.opts.turns.applyItem(
            threadId,
            projectToolCallItemForPublic(privateCallItem)
          )
          await this.opts.events.record({
            kind: 'tool_call_ready',
            threadId,
            turnId,
            itemId,
            callId: chunk.callId,
            toolName: chunk.toolName,
            readyCount: completedToolCalls.length
          })
          break
        }
        case 'usage': {
          sawUsage = true
          const cacheDiagnostics = compareCachePrefixShapes(
            previousCachePrefixShape,
            cachePrefixShape,
            chunk.usage,
            {
              ...(firstTokenLatencyMs !== undefined ? { firstTokenLatencyMs } : {}),
              durationMs: Math.max(0, this.nowMs() - providerStartedAtMs)
            }
          )
          this.recordPromptPressure(threadId, request.model, chunk.usage.promptTokens)
          const usage = this.opts.usage.record(threadId, chunk.usage, cacheDiagnostics)
          await this.opts.events.record({
            kind: 'usage',
            threadId,
            turnId,
            model: request.model,
            ...(modelClientDiagnostics.providerId ? { providerId: modelClientDiagnostics.providerId } : {}),
            usage,
            cacheDiagnostics
          })
          break
        }
        case 'retrying':
          await this.opts.events.record({
            kind: 'pipeline_stage',
            threadId,
            turnId,
            stage: 'provider_retrying',
            label: PIPELINE_STAGE_LABELS.provider_retrying,
            attempt: chunk.attempt,
            maxAttempt: chunk.maxAttempt,
            details: {
              model: request.model,
              ...(modelClientDiagnostics.providerId ? { providerId: modelClientDiagnostics.providerId } : {}),
              ...(chunk.status !== undefined ? { status: chunk.status } : {}),
              message: chunk.message
            }
          })
          break
        case 'completed':
          if (stopReason !== 'error') stopReason = chunk.stopReason
          break
        case 'error':
          this.rememberTurnFailure(turnId, {
            error: chunk.message,
            ...(chunk.code ? { code: chunk.code } : {}),
            severity: 'error'
          })
          await this.opts.events.record({
            kind: 'error',
            threadId,
            turnId,
            message: chunk.message,
            code: chunk.code,
            severity: 'error'
          })
          stopReason = 'error'
          break
      }
    }
    if (sawUsage) {
      this.rememberCachePrefixShape(threadId, cachePrefixShape)
    }
    if (signal.aborted) {
      await persistAccumulatedResponse()
      return 'aborted'
    }
    await this.recordPipelineStage(threadId, turnId, 'response_received', {
      stopReason,
      toolCallCount: completedToolCalls.length
    })
    await persistAccumulatedResponse()
    if (stopReason === 'error') return 'failed'
    if (completedToolCalls.length === 0) {
      if (request.requiredToolName) {
        if (
          request.requiredToolName === CREATE_PLAN_TOOL_NAME &&
          textAccumulator.value.trim()
        ) {
          if (isPlanClarifyingQuestion(textAccumulator.value)) {
            return 'stop'
          }
          const callId = this.opts.ids.next('call_plan')
          const provider = toolProviderMetadata.get(CREATE_PLAN_TOOL_NAME)
          const toolKind = toolKinds.get(CREATE_PLAN_TOOL_NAME)
          const sourceRequest = activePlanContext?.sourceRequest ||
            latestUserMessageText(historyItems, turnId) ||
            turn?.prompt ||
            ''
          const argumentsForFallback: Record<string, unknown> = activePlanContext
            ? {
                markdown: textAccumulator.value.trim(),
                operation: activePlanContext.operation,
                plan_id: activePlanContext.planId,
                plan_relative_path: activePlanContext.relativePath,
                ...(sourceRequest ? { source_request: sourceRequest } : {}),
                ...(activePlanContext.title ? { title: activePlanContext.title } : {})
              }
            : {
                markdown: textAccumulator.value.trim(),
                operation: 'draft',
                ...(sourceRequest ? { source_request: sourceRequest } : {})
              }
          const call: ToolCallLike = {
            callId,
            toolName: CREATE_PLAN_TOOL_NAME,
            ...(provider?.providerId ? { providerId: provider.providerId } : {}),
            toolKind,
            arguments: argumentsForFallback
          }
          const itemId = `item_tool_${turnId}_${callId}`
          const privateCallItem = makePrivateToolCallItem({
            id: itemId,
            turnId,
            threadId,
            callId,
            toolName: CREATE_PLAN_TOOL_NAME,
            toolKind,
            arguments: argumentsForFallback,
            summary: 'Materialized assistant plan text into the required GUI plan.'
          })
          this.rememberRequestLocalToolCall(privateCallItem)
          await this.opts.turns.applyItem(
            threadId,
            projectToolCallItemForPublic(privateCallItem)
          )
          await this.opts.events.record({
            kind: 'tool_call_ready',
            threadId,
            turnId,
            itemId,
            callId,
            toolName: CREATE_PLAN_TOOL_NAME,
            readyCount: 1
          })
          const dispatched = await this.dispatchToolCalls({
            calls: [call],
            threadId,
            turnId,
            workspace: thread?.workspace ?? '',
            threadMode: effectiveMode,
            activePlanContext,
            modelCapabilities,
            activeSkillIds: skillResolution.activeSkillIds,
            allowedToolNames: activeAllowedToolNames,
            currentMaxModelSteps,
            toolProviderKinds: new Map(tools.map((tool) => [tool.name, tool.providerKind])),
            approvalPolicy,
            sandboxMode,
            signal
          })
          if (dispatched === 'aborted') return 'aborted'
          if (dispatched === 'all_suppressed') return 'stop'
          return 'continue'
        }
        const message = `Model did not call the required \`${request.requiredToolName}\` tool for this GUI plan turn.`
        await this.opts.events.record({
          kind: 'error',
          threadId,
          turnId,
          message,
          code: 'required_tool_missing'
        })
        await this.opts.turns.applyItem(
          threadId,
          makeErrorItem({
            id: this.opts.ids.next('item_error'),
            turnId,
            threadId,
            message,
            code: 'required_tool_missing'
          })
        )
        return 'failed'
      }
      const hasCurrentTurnFileChange = historyItems.some(
        (item) =>
          item.turnId === turnId &&
          item.kind === 'tool_call' &&
          item.toolKind === 'file_change' &&
          item.toolName !== CREATE_PLAN_TOOL_NAME
      )
      if (
        stopReason === 'stop' &&
        !textAccumulator.value.trim() &&
        hasCurrentTurnFileChange
      ) {
        const emptyPostTool = this.goalControl.noteEmptyPostFileChangeStop(turnId)
        if (emptyPostTool.action === 'recover') {
          return 'continue'
        }

        const { failure } = emptyPostTool
        const message = failure.message
        this.rememberTurnFailure(turnId, {
          error: message,
          code: failure.code,
          severity: failure.severity
        })
        await this.opts.events.record({
          kind: 'error',
          threadId,
          turnId,
          message,
          code: failure.code,
          severity: failure.severity
        })
        await this.opts.turns.applyItem(
          threadId,
          makeErrorItem({
            id: this.opts.ids.next('item_error'),
            turnId,
            threadId,
            message,
            code: failure.code,
            severity: failure.severity
          })
        )
        return 'failed'
      }
      if (this.opts.steering.peek().length > 0) {
        return 'continue'
      }
      if (stopReason === 'stop' && activeGoalInstruction) {
        const goalStop = this.goalControl.noteGoalNoToolStop(turnId, textAccumulator.value)
        if (goalStop.action === 'continue') {
          return 'continue'
        }
        const { failure } = goalStop
        const message = failure.message
        await this.opts.turns.applyItem(
          threadId,
          makeErrorItem({
            id: this.opts.ids.next('item_error'),
            turnId,
            threadId,
            message,
            code: failure.code,
            severity: failure.severity
          })
        )
        await this.opts.events.record({
          kind: 'error',
          threadId,
          turnId,
          message,
          code: failure.code,
          severity: failure.severity
        })
        return 'stop'
      }
      return 'stop'
    }
    // Tool calls mean the turn is making progress again; reset the no-tool
    // repetition window so unrelated later status texts are not compared.
    this.goalControl.noteToolCalls(turnId)
    const dispatched = await this.dispatchToolCalls({
      calls: completedToolCalls,
      threadId,
      turnId,
      workspace: thread?.workspace ?? '',
      threadMode: effectiveMode,
      activePlanContext,
      modelCapabilities,
      activeSkillIds: skillResolution.activeSkillIds,
      allowedToolNames: activeAllowedToolNames,
      userInputDisabled,
      currentMaxModelSteps,
      modelExecution,
      toolProviderKinds: new Map(tools.map((tool) => [tool.name, tool.providerKind])),
      approvalPolicy,
      sandboxMode,
      signal
    })
    if (dispatched === 'aborted') return 'aborted'
    if (dispatched === 'all_suppressed') return 'stop'
    return 'continue'
  }

  private async dispatchToolCalls(input: {
    calls: ToolCallLike[]
    threadId: string
    turnId: string
    workspace: string
    threadMode?: 'agent' | 'plan'
    activePlanContext?: GuiPlanContext
    modelCapabilities: ModelCapabilityMetadata
    activeSkillIds: readonly string[]
    allowedToolNames?: readonly string[]
    userInputDisabled?: boolean
    currentMaxModelSteps: number
    modelExecution?: ModelExecutionRef
    toolProviderKinds: ReadonlyMap<string, ToolProviderKind | undefined>
    approvalPolicy: ToolHostContext['approvalPolicy']
    sandboxMode: NonNullable<ToolHostContext['sandboxMode']>
    signal: AbortSignal
  }): Promise<'continue' | 'aborted' | 'all_suppressed'> {
    const context = this.createToolContext(input)
    let index = 0
    let executedAny = false
    const markProgress = (toolName: string): void => {
      if (this.goalControl.isProgressTool(toolName)) {
        this.turnMadeProgress.add(input.turnId)
      }
    }

    while (index < input.calls.length) {
      if (input.signal.aborted) {
        await this.persistCancelledToolCallResults({
          threadId: input.threadId,
          turnId: input.turnId,
          calls: input.calls.slice(index),
          reason: 'cancelled: turn aborted before tool execution'
        })
        return 'aborted'
      }

      const call = input.calls[index]
      if (!call) break

      const storm = this.toolStormBreakers.get(input.turnId)?.inspect(call)
      if (storm?.suppress) {
        await this.persistSuppressedToolCall({
          threadId: input.threadId,
          turnId: input.turnId,
          call,
          reason: storm.reason
        })
        index += 1
        continue
      }

      if (!this.isParallelSafeToolCall(call, input.approvalPolicy, input.toolProviderKinds)) {
        let result: ToolHostResult
        try {
          result = await this.executeToolCallSafely({
            threadId: input.threadId,
            turnId: input.turnId,
            call,
            context
          })
        } catch (error) {
          if (!input.signal.aborted) throw error
          await this.persistCancelledToolCallResults({
            threadId: input.threadId,
            turnId: input.turnId,
            calls: input.calls.slice(index),
            reason: 'cancelled: turn aborted during tool execution'
          })
          return 'aborted'
        }
        executedAny = true
        markProgress(call.toolName)
        await this.persistToolCallResult(input.threadId, input.turnId, call, result)
        index += 1
        if (input.signal.aborted) {
          await this.persistCancelledToolCallResults({
            threadId: input.threadId,
            turnId: input.turnId,
            calls: input.calls.slice(index),
            reason: 'cancelled: turn aborted after tool execution'
          })
          return 'aborted'
        }
        continue
      }

      // Keep batches homogeneous: delegation children fan out together (the
      // runtime semaphore bounds real concurrency), while built-in read-only
      // tools stay capped at MAX_PARALLEL_TOOL_CALLS.
      const headIsDelegation = this.isParallelDelegationCall(call, input.toolProviderKinds)
      const batchCap = headIsDelegation ? input.calls.length : MAX_PARALLEL_TOOL_CALLS
      const batch: ToolCallLike[] = [call]
      index += 1
      let suppressedAfterBatch: { call: ToolCallLike; reason?: string } | undefined

      while (batch.length < batchCap && index < input.calls.length) {
        const next = input.calls[index]
        if (!next) break
        if (!this.isParallelSafeToolCall(next, input.approvalPolicy, input.toolProviderKinds)) break
        if (this.isParallelDelegationCall(next, input.toolProviderKinds) !== headIsDelegation) break

        const nextStorm = this.toolStormBreakers.get(input.turnId)?.inspect(next)
        if (nextStorm?.suppress) {
          suppressedAfterBatch = { call: next, reason: nextStorm.reason }
          index += 1
          break
        }

        batch.push(next)
        index += 1
      }

      const settled = await Promise.allSettled(
        batch.map((entry) =>
          this.executeToolCallSafely({
            threadId: input.threadId,
            turnId: input.turnId,
            call: entry,
            context
          })
        )
      )
      executedAny = true
      let abortedDuringBatch: boolean = input.signal.aborted
      for (let batchIndex = 0; batchIndex < batch.length; batchIndex += 1) {
        const result = settled[batchIndex]
        const batchCall = batch[batchIndex]
        if (!result || !batchCall) continue
        if (result.status === 'rejected') {
          if (!input.signal.aborted) throw result.reason
          abortedDuringBatch = true
          await this.persistCancelledToolCallResult({
            threadId: input.threadId,
            turnId: input.turnId,
            call: batchCall,
            reason: 'cancelled: turn aborted during parallel tool execution'
          })
          continue
        }
        markProgress(batchCall.toolName)
        await this.persistToolCallResult(input.threadId, input.turnId, batchCall, result.value)
      }

      if (suppressedAfterBatch) {
        await this.persistSuppressedToolCall({
          threadId: input.threadId,
          turnId: input.turnId,
          call: suppressedAfterBatch.call,
          reason: suppressedAfterBatch.reason
        })
      }
      if (abortedDuringBatch) {
        await this.persistCancelledToolCallResults({
          threadId: input.threadId,
          turnId: input.turnId,
          calls: input.calls.slice(index),
          reason: 'cancelled: turn aborted before remaining tool execution'
        })
        return 'aborted'
      }
    }

    return executedAny ? 'continue' : 'all_suppressed'
  }

  private isParallelSafeToolCall(
    call: ToolCallLike,
    approvalPolicy: ToolHostContext['approvalPolicy'],
    toolProviderKinds: ReadonlyMap<string, ToolProviderKind | undefined>
  ): boolean {
    // always/untrusted/never prompt or block tool calls, so parallel fan-out is unsafe.
    if (approvalPolicy === 'always' || approvalPolicy === 'untrusted' || approvalPolicy === 'never') return false
    // Delegated children are isolated runs; multiple in one assistant message
    // are independent and safe to fan out. The delegation runtime caps real
    // concurrency at maxParallel and queues the overflow.
    if (this.isParallelDelegationCall(call, toolProviderKinds)) return true
    if (!PARALLEL_READ_ONLY_TOOL_NAMES.has(call.toolName)) return false
    if (call.toolKind && call.toolKind !== 'tool_call') return false
    return toolProviderKinds.get(call.toolName) === 'built-in'
  }

  private isParallelDelegationCall(
    call: ToolCallLike,
    toolProviderKinds: ReadonlyMap<string, ToolProviderKind | undefined>
  ): boolean {
    return (
      call.toolName === DELEGATE_TASK_TOOL_NAME &&
      toolProviderKinds.get(call.toolName) === 'delegation'
    )
  }

  private createToolContext(input: {
    threadId: string
    turnId: string
    workspace: string
    threadMode?: 'agent' | 'plan'
    activePlanContext?: GuiPlanContext
    modelCapabilities: ModelCapabilityMetadata
    activeSkillIds: readonly string[]
    allowedToolNames?: readonly string[]
    userInputDisabled?: boolean
    currentMaxModelSteps: number
    modelExecution?: ModelExecutionRef
    approvalPolicy: ToolHostContext['approvalPolicy']
    sandboxMode: NonNullable<ToolHostContext['sandboxMode']>
    signal: AbortSignal
  }): ToolHostContext {
    return {
      threadId: input.threadId,
      turnId: input.turnId,
      workspace: input.workspace,
      threadMode: input.threadMode,
      ...(input.activePlanContext ? { guiPlan: input.activePlanContext } : {}),
      model: input.modelCapabilities,
      ...(input.modelExecution ? { modelExecution: input.modelExecution } : {}),
      activeSkillIds: input.activeSkillIds,
      memoryPolicy: { enabled: Boolean(this.opts.memoryStore) },
      delegationPolicy: { enabled: false },
      runtimeStepLimits: { currentMaxModelSteps: input.currentMaxModelSteps },
      ...(input.allowedToolNames ? { allowedToolNames: input.allowedToolNames } : {}),
      approvalPolicy: input.approvalPolicy,
      sandboxMode: input.sandboxMode,
      abortSignal: input.signal,
      awaitApproval: (approval) =>
        this.awaitApproval(approval, {
          approvalPolicy: input.approvalPolicy,
          sandboxMode: input.sandboxMode,
          signal: input.signal
        }),
      ...(input.userInputDisabled
        ? {}
        : {
            awaitUserInput: (inputRequest) =>
              this.awaitUserInput(input.threadId, input.turnId, inputRequest, input.signal)
          })
    }
  }

  private async executeToolCall(input: {
    threadId: string
    turnId: string
    call: ToolCallLike
    context: ToolHostContext
  }): Promise<ToolHostResult> {
    return this.opts.inflight.run(
      {
        id: `inflight_${input.call.callId}`,
        kind: 'tool',
        threadId: input.threadId,
        turnId: input.turnId,
        callId: input.call.callId
      },
      async () => {
        try {
          return await this.opts.toolHost.execute(input.call, input.context, async (item) => {
            const publicItem = projectToolResultItemForPublic(item)
            const existing = await this.opts.turns.updateItem(input.threadId, item.id, {
              output: publicItem.output,
              isError: publicItem.isError,
              status: 'running'
            } as Partial<TurnItem>)
            if (existing) return
            await this.opts.turns.applyItem(input.threadId, publicItem)
          })
        } catch (error) {
          if (input.context.abortSignal.aborted || !this.isRecoverableToolDispatchError(error)) {
            throw error
          }
          const message = error instanceof Error ? error.message : String(error)
          await this.opts.events.record({
            kind: 'error',
            threadId: input.threadId,
            turnId: input.turnId,
            message: `Tool call ${input.call.toolName} was rejected: ${message}`,
            code: 'tool_dispatch_rejected',
            severity: 'warning'
          })
          return {
            item: makePrivateToolResultItem({
              id: `item_${input.call.callId}`,
              turnId: input.turnId,
              threadId: input.threadId,
              callId: input.call.callId,
              toolName: input.call.toolName,
              toolKind: input.call.toolKind ?? 'tool_call',
              output: {
                code: 'tool_dispatch_rejected',
                error: message,
                guidance: 'Use only tools advertised in the current turn context.'
              },
              isError: true
            }),
            approved: false
          }
        }
      }
    )
  }

  private async awaitApproval(
    approval: ApprovalRequest,
    context: {
      approvalPolicy: ToolHostContext['approvalPolicy']
      sandboxMode: NonNullable<ToolHostContext['sandboxMode']>
      signal: AbortSignal
    }
  ): Promise<'allow' | 'deny'> {
    await this.opts.events.record({
      kind: 'approval_requested',
      threadId: approval.threadId,
      turnId: approval.turnId,
      approvalId: approval.id,
      toolName: approval.toolName,
      status: 'pending',
      approvalPolicy: context.approvalPolicy,
      sandboxMode: context.sandboxMode,
      summary: approval.summary
    })
    return this.waitForApproval(approval, context.signal)
  }

  private async waitForApproval(
    approval: ApprovalRequest,
    signal: AbortSignal
  ): Promise<'allow' | 'deny'> {
    const pending = this.opts.approvalGate.request(approval)
    if (signal.aborted) {
      await this.expireApprovalForAbort(approval)
      throw new Error('cancelled while awaiting approval')
    }
    return new Promise<'allow' | 'deny'>((resolve, reject) => {
      let aborting = false
      const onAbort = (): void => {
        aborting = true
        signal.removeEventListener('abort', onAbort)
        void (async () => {
          try {
            await this.expireApprovalForAbort(approval)
          } finally {
            reject(new Error('cancelled while awaiting approval'))
          }
        })()
      }
      signal.addEventListener('abort', onAbort, { once: true })
      pending
        .then((decision) => {
          signal.removeEventListener('abort', onAbort)
          resolve(decision)
        })
        .catch((error) => {
          signal.removeEventListener('abort', onAbort)
          if (!aborting) reject(error)
        })
    })
  }

  private async expireApprovalForAbort(approval: ApprovalRequest): Promise<void> {
    const expired = this.opts.approvalGate.expire(approval.id, 'turn aborted while awaiting approval')
    if (!expired) return
    await this.opts.events.record({
      kind: 'approval_resolved',
      threadId: approval.threadId,
      turnId: approval.turnId,
      itemId: undefined,
      approvalId: approval.id,
      toolName: approval.toolName,
      status: 'expired',
      summary: approval.summary
    })
  }

  /**
   * A crashing tool handler must surface as an error tool_result the
   * model can react to, not kill the whole turn. Only turn aborts are
   * allowed to propagate.
   */
  private async executeToolCallSafely(input: {
    threadId: string
    turnId: string
    call: ToolCallLike
    context: ToolHostContext
  }): Promise<ToolHostResult> {
    try {
      return await this.executeToolCall(input)
    } catch (error) {
      if (input.context.abortSignal.aborted) throw error
      const message = error instanceof Error ? error.message : String(error)
      await this.opts.events.record({
        kind: 'error',
        threadId: input.threadId,
        turnId: input.turnId,
        message: `Tool call ${input.call.toolName} failed: ${message}`,
        code: 'tool_execution_failed',
        severity: 'warning'
      })
      return {
        item: makePrivateToolResultItem({
          id: `item_${input.call.callId}`,
          turnId: input.turnId,
          threadId: input.threadId,
          callId: input.call.callId,
          toolName: input.call.toolName,
          toolKind: input.call.toolKind ?? 'tool_call',
          output: {
            code: 'tool_execution_failed',
            error: message,
            guidance:
              'The tool crashed while executing. Adjust the arguments or take a different approach instead of retrying the identical call.'
          },
          isError: true
        }),
        approved: false
      }
    }
  }

  private isRecoverableToolDispatchError(error: unknown): boolean {
    const message = error instanceof Error ? error.message : String(error)
    return (
      message.startsWith('unknown tool:') ||
      message.includes(' is not provided by ') ||
      message.includes(' is not advertised') ||
      message.includes(' is disabled by policy')
    )
  }

  private async persistToolCallResult(
    threadId: string,
    turnId: string,
    call: ToolCallLike,
    result: ToolHostResult
  ): Promise<void> {
    const callStatus = result.item.kind === 'tool_result' && result.item.status === 'aborted'
      ? 'aborted'
      : result.item.kind === 'tool_result' && result.item.isError
        ? 'failed'
        : 'completed'
    await this.opts.turns.updateItem(threadId, `item_tool_${turnId}_${call.callId}`, {
      status: callStatus,
      finishedAt: this.opts.nowIso()
    } as Partial<TurnItem>)
    await this.opts.turns.applyItem(
      threadId,
      result.item.kind === 'tool_result'
        ? projectToolResultItemForPublic(result.item)
        : result.item
    )
    if (result.item.kind === 'tool_result') {
      this.rememberRequestLocalToolResult(result.item)
    }
    await this.afterToolResultPersisted(threadId, turnId, call, result)
  }

  private async persistCancelledToolCallResults(input: {
    threadId: string
    turnId: string
    calls: readonly ToolCallLike[]
    reason: string
  }): Promise<void> {
    for (const call of input.calls) {
      await this.persistCancelledToolCallResult({
        threadId: input.threadId,
        turnId: input.turnId,
        call,
        reason: input.reason
      })
    }
  }

  private async persistCancelledToolCallResult(input: {
    threadId: string
    turnId: string
    call: ToolCallLike
    reason: string
  }): Promise<void> {
    await this.persistToolCallResult(
      input.threadId,
      input.turnId,
      input.call,
      {
        item: makePrivateToolResultItem({
          id: `item_${input.call.callId}`,
          turnId: input.turnId,
          threadId: input.threadId,
          callId: input.call.callId,
          toolName: input.call.toolName,
          toolKind: input.call.toolKind ?? 'tool_call',
          output: {
            code: 'tool_call_cancelled',
            error: input.reason,
            cancelled: true
          },
          isError: true,
          status: 'aborted'
        }),
        approved: false
      }
    )
  }

  private async afterToolResultPersisted(
    threadId: string,
    turnId: string,
    call: ToolCallLike,
    result: ToolHostResult
  ): Promise<void> {
    if (call.toolName !== CREATE_PLAN_TOOL_NAME) return
    if (result.item.kind !== 'tool_result' || result.item.isError === true) return
    const output = result.item.output
    if (!output || typeof output !== 'object') return
    const record = output as Record<string, unknown>
    const planId = typeof record.plan_id === 'string' ? record.plan_id : ''
    const relativePath = typeof record.relative_path === 'string' ? record.relative_path : ''
    const markdown = typeof call.arguments.markdown === 'string' ? call.arguments.markdown : ''
    if (!planId || !relativePath || !markdown) return
    try {
      await this.opts.onPlanWritten?.({
        threadId,
        turnId,
        planId,
        relativePath,
        markdown
      })
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      await this.opts.events.record({
        kind: 'error',
        threadId,
        turnId,
        message: `Failed to sync plan checklist to thread todos: ${message}`,
        code: 'todo_plan_sync_failed',
        severity: 'warning'
      })
    }
  }

  private async persistSuppressedToolCall(input: {
    threadId: string
    turnId: string
    call: ToolCallLike
    reason?: string
  }): Promise<void> {
    const item = makePrivateToolResultItem({
      id: `item_${input.call.callId}_storm`,
      turnId: input.turnId,
      threadId: input.threadId,
      callId: input.call.callId,
      toolName: input.call.toolName,
      toolKind: input.call.toolKind ?? 'tool_call',
      output: { error: input.reason ?? 'duplicate tool call suppressed by repeat-loop guard' },
      isError: true
    })
    const message = input.reason ?? 'duplicate tool call suppressed by repeat-loop guard'
    await this.opts.turns.updateItem(input.threadId, `item_tool_${input.turnId}_${input.call.callId}`, {
      status: 'failed',
      finishedAt: this.opts.nowIso()
    } as Partial<TurnItem>)
    this.rememberRequestLocalToolResult(item)
    await this.opts.turns.applyItem(
      input.threadId,
      projectToolResultItemForPublic(item)
    )
    await this.opts.events.record({
      kind: 'tool_storm_suppressed',
      threadId: input.threadId,
      turnId: input.turnId,
      itemId: item.id,
      toolName: input.call.toolName,
      callId: input.call.callId,
      message
    })
  }

  private rememberRequestLocalToolCall(item: InternalToolCallItem): void {
    const byCall = this.requestLocalToolHistory.get(item.turnId) ?? new Map()
    const current = byCall.get(item.callId) ?? {}
    byCall.set(item.callId, { ...current, call: item })
    this.requestLocalToolHistory.set(item.turnId, byCall)
  }

  private rememberRequestLocalToolResult(item: InternalToolResultItem): void {
    const byCall = this.requestLocalToolHistory.get(item.turnId) ?? new Map()
    const current = byCall.get(item.callId) ?? {}
    byCall.set(item.callId, { ...current, result: item })
    this.requestLocalToolHistory.set(item.turnId, byCall)
  }

  private restoreRequestLocalToolHistory(
    items: readonly TurnItem[],
    turnId: string
  ): ModelHistoryItem[] {
    const byCall = this.requestLocalToolHistory.get(turnId)
    if (!byCall) return [...items]
    return items.map((item): ModelHistoryItem => {
      if (item.turnId !== turnId || (item.kind !== 'tool_call' && item.kind !== 'tool_result')) {
        return item
      }
      const privatePair = byCall.get(item.callId)
      const candidate = item.kind === 'tool_call' ? privatePair?.call : privatePair?.result
      if (
        !candidate ||
        candidate.id !== item.id ||
        candidate.threadId !== item.threadId ||
        candidate.toolName !== item.toolName
      ) {
        return item
      }
      return candidate
    })
  }

  private async awaitUserInput(
    threadId: string,
    turnId: string,
    input: {
      id: string
      itemId: string
      prompt: string
      questions: Array<{
        header: string
        id: string
        question: string
        options: Array<{ label: string; description: string }>
      }>
    },
    signal: AbortSignal
  ): Promise<UserInputResolution> {
    const item = makeUserInputItem({
      id: input.itemId,
      threadId,
      turnId,
      inputId: input.id,
      prompt: input.prompt,
      questions: input.questions
    })
    await this.opts.turns.applyItem(threadId, item)
    await this.opts.events.record({
      kind: 'user_input_requested',
      threadId,
      turnId,
      itemId: item.id,
      inputId: input.id,
      status: 'pending',
      prompt: input.prompt,
      questions: input.questions
    })

    let resolution: UserInputResolution
    try {
      resolution = await this.waitForUserInput(threadId, turnId, input, signal)
    } catch (error) {
      if (!signal.aborted) throw error
      resolution = { status: 'cancelled' }
      await this.finishUserInputRequest(threadId, turnId, item.id, input, resolution)
      throw error
    }
    await this.finishUserInputRequest(threadId, turnId, item.id, input, resolution)
    return resolution
  }

  private async finishUserInputRequest(
    threadId: string,
    turnId: string,
    itemId: string,
    input: {
      id: string
      prompt: string
      questions: Array<{
        header: string
        id: string
        question: string
        options: Array<{ label: string; description: string }>
      }>
    },
    resolution: UserInputResolution
  ): Promise<void> {
    await this.opts.turns.updateItem(threadId, itemId, {
      status: resolution.status,
      finishedAt: this.opts.nowIso()
    } as Partial<TurnItem>)
    await this.opts.events.record({
      kind: 'user_input_resolved',
      threadId,
      turnId,
      itemId,
      inputId: input.id,
      status: resolution.status,
      prompt: input.prompt,
      questions: input.questions
    })
  }

  private async waitForUserInput(
    threadId: string,
    turnId: string,
    input: {
      id: string
      itemId: string
      prompt: string
      questions: Array<{
        header: string
        id: string
        question: string
        options: Array<{ label: string; description: string }>
      }>
    },
    signal: AbortSignal
  ): Promise<UserInputResolution> {
    const pending = this.opts.userInputGate.request({
      id: input.id,
      threadId,
      turnId,
      itemId: input.itemId,
      prompt: input.prompt,
      questions: input.questions
    })
    if (!signal.aborted) {
      return new Promise<UserInputResolution>((resolve, reject) => {
        const onAbort = (): void => {
          this.opts.userInputGate.resolve(input.id, { status: 'cancelled' })
          signal.removeEventListener('abort', onAbort)
          reject(new Error('cancelled while awaiting user input'))
        }
        signal.addEventListener('abort', onAbort, { once: true })
        pending
          .then((resolution) => {
            signal.removeEventListener('abort', onAbort)
            resolve(resolution)
          })
          .catch((error) => {
            signal.removeEventListener('abort', onAbort)
            reject(error)
          })
      })
    }
    this.opts.userInputGate.resolve(input.id, { status: 'cancelled' })
    throw new Error('cancelled while awaiting user input')
  }

  private async compactIfNeeded(
    items: TurnItem[],
    model: string,
    signal: AbortSignal,
    context: {
      threadId: string
      turnId: string
      visibleItems: TurnItem[]
      toolSpecs?: readonly ModelToolSpec[]
    }
  ): Promise<TurnItem[]> {
    // Restore the accurate provider token count after a process restart,
    // when the in-memory pressure map is empty. Without this the next
    // line falls back to the item-only estimator, which under-counts and
    // can silently skip compaction until the context overruns the window.
    await this.hydratePromptPressureIfCold(context.threadId, model)
    const pressure = this.consumePromptPressure(context.threadId, model)
    const thresholdModel = pressure?.model || model
    const overheadTokens = estimateRequestOverheadTokens({
      systemPrompt: this.opts.prefix.systemPrompt,
      prefix: this.opts.prefix.fewShots,
      tools: context.toolSpecs
    })
    const plan = this.opts.compactor.planCompaction(items, {
      model: thresholdModel,
      promptTokens: pressure?.promptTokens,
      overheadTokens
    })
    if (!plan) return items
    const threadId = context.threadId
    const turnId = context.turnId
    if (hasHooksForPhase(this.opts.hooks, 'PreCompact')) {
      const observed = await runObserverHooks(this.opts.hooks, {
        phase: 'PreCompact',
        threadId,
        turnId,
        reason: String(plan.reason),
        mode: String(plan.mode)
      })
      await this.recordHookWarnings(threadId, turnId, observed.warnings)
    }
    let result = this.opts.compactor.compact({
      threadId,
      turnId,
      history: items,
      prefix: this.opts.prefix,
      reason: plan.reason,
      mode: plan.mode,
      keepRecent: plan.keepRecent
    })
    if (result.replacedTokens > 0 && this.opts.contextCompaction?.summaryMode === 'model') {
      const modelSummary = await summarizeCompactionWithModel({
        threadId,
        turnId,
        model,
        modelClient: this.opts.model,
        prefix: this.opts.prefix,
        contextCompaction: this.opts.contextCompaction,
        items,
        heuristicSummary: result.summaryItem.kind === 'compaction' ? result.summaryItem.summary : '',
        signal,
        recordUsage: async (usageSnapshot) => {
          const usage = this.opts.usage.record(threadId, usageSnapshot)
          await this.opts.events.record({
            kind: 'usage',
            threadId,
            turnId,
            model,
            usage
          })
        },
        recordFallback: async (message) => {
          await this.opts.events.record({
            kind: 'error',
            threadId,
            turnId,
            message,
            code: 'compaction_summary_fallback',
            severity: 'warning'
          })
        }
      })
      if (signal.aborted) return items
      if (modelSummary) {
        result = this.opts.compactor.compact({
          threadId,
          turnId,
          history: items,
          prefix: this.opts.prefix,
          reason: plan.reason,
          mode: plan.mode,
          keepRecent: plan.keepRecent,
          summaryOverride: modelSummary
        })
      }
    }
    if (result.replacedTokens > 0) {
      const visibleItems = insertCompactionIntoVisibleHistory({
        visibleItems: context.visibleItems,
        compactedItems: result.next,
        summaryItem: result.summaryItem
      })
      this.opts.toolHost.clearReadTracker?.(threadId)
      await this.opts.sessionStore.rewriteItems(threadId, visibleItems)
      await this.rewriteThreadItemsFromSession(threadId, visibleItems)
      await this.opts.events.record({
        kind: 'compaction_completed',
        threadId,
        turnId,
        itemId: result.summaryItem.id,
        summary: result.summaryItem.kind === 'compaction' ? result.summaryItem.summary : '',
        replacedTokens: result.replacedTokens,
        pinnedConstraints: this.opts.prefix.pinnedConstraints,
        ...(result.summaryItem.kind === 'compaction' && result.summaryItem.sourceDigest
          ? { sourceDigest: result.summaryItem.sourceDigest }
          : {}),
        ...(result.summaryItem.kind === 'compaction' && result.summaryItem.digestMarker
          ? { digestMarker: result.summaryItem.digestMarker }
          : {}),
        ...(result.summaryItem.kind === 'compaction' && result.summaryItem.sourceItemIds
          ? { sourceItemIds: result.summaryItem.sourceItemIds }
          : {})
      })
    }
    return result.next
  }

  private async rewriteThreadItemsFromSession(threadId: string, items: TurnItem[]): Promise<void> {
    if (items.length === 0) return
    const current = await this.opts.threadStore.get(threadId)
    if (!current) return
    const itemsByTurn = new Map<string, TurnItem[]>()
    for (const item of items) {
      const turnItems = itemsByTurn.get(item.turnId) ?? []
      turnItems.push(item)
      itemsByTurn.set(item.turnId, turnItems)
    }
    let changed = false
    const turns = current.turns.map((turn) => {
      const sessionItems = itemsByTurn.get(turn.id)
      if (!sessionItems) return turn
      changed = true
      return { ...turn, items: placeCompactionsAtTurnEnd(sessionItems) }
    })
    if (!changed) return
    await this.opts.threadStore.upsert(touchThread({ ...current, turns }, this.opts.nowIso()))
  }

  private async recordTokenEconomySavings(input: {
    threadId: string
    turnId: string
    model: string
    rawInputTokens: number
    sentInputTokens: number
  }): Promise<void> {
    const savedTokens = Math.max(0, Math.floor(input.rawInputTokens - input.sentInputTokens))
    if (savedTokens <= 0) return
    const usage = this.opts.usage.recordTokenEconomySavings(input.threadId, {
      tokenEconomySavingsTokens: savedTokens
    })
    await this.opts.events.record({
      kind: 'usage',
      threadId: input.threadId,
      turnId: input.turnId,
      model: input.model,
      usage
    })
  }

  private async recordPipelineStage(
    threadId: string,
    turnId: string,
    stage: PipelineStage,
    details?: Record<string, unknown>
  ): Promise<void> {
    await this.opts.events.record({
      kind: 'pipeline_stage',
      threadId,
      turnId,
      stage,
      label: PIPELINE_STAGE_LABELS[stage],
      ...(details && Object.keys(details).length > 0 ? { details } : {})
    })
  }

  private recordPromptPressure(threadId: string, model: string, promptTokens: number): void {
    if (!threadId || promptTokens <= 0) return
    const current = this.promptTokenPressure.get(threadId)
    if (current && current.promptTokens >= promptTokens) return
    this.promptTokenPressure.set(threadId, { model, promptTokens })
  }

  /**
   * Seed `promptTokenPressure` from persisted usage the first time a thread
   * is touched in this process. The pressure map is in-memory only, so after
   * a restart the compaction trigger would otherwise rely on the item-only
   * estimator (which omits the system prompt and tool schemas) and could
   * skip compaction for an already-oversized thread. `loadUsageRecords`
   * returns per-request deltas ordered oldest-first, so the last positive
   * entry is the most recent request's prompt size — the best available
   * proxy for the current context pressure. Best-effort: any failure leaves
   * the estimator (plus overhead floor) as the fallback.
   */
  private async hydratePromptPressureIfCold(threadId: string, fallbackModel: string): Promise<void> {
    if (!threadId) return
    if (this.promptTokenPressure.has(threadId)) return
    if (this.hydratedPressureThreads.has(threadId)) return
    this.hydratedPressureThreads.add(threadId)
    const loadUsageRecords = this.opts.sessionStore.loadUsageRecords
    if (typeof loadUsageRecords !== 'function') return
    try {
      const records = await loadUsageRecords.call(this.opts.sessionStore, { threadId })
      let restored: { model: string; promptTokens: number } | undefined
      for (const record of records) {
        if (record.threadId !== threadId) continue
        const promptTokens = Math.floor(record.usage?.promptTokens ?? 0)
        if (promptTokens > 0) {
          restored = { model: record.model || fallbackModel, promptTokens }
        }
      }
      if (restored && !this.promptTokenPressure.has(threadId)) {
        this.promptTokenPressure.set(threadId, restored)
      }
    } catch {
      // Best-effort restore; the estimator + overhead floor still applies.
    }
  }

  private async recordToolCatalogDrift(input: {
    threadId: string
    turnId: string
    fingerprint: string
    toolCount: number
    toolNames: string[]
    changeKind: 'additive' | 'breaking'
    message: string
  }): Promise<void> {
    await this.opts.turns.applyItem(input.threadId, makeErrorItem({
      id: `item_${input.turnId}_tool_catalog_changed_${input.fingerprint}`,
      threadId: input.threadId,
      turnId: input.turnId,
      message: input.message,
      code: 'tool_catalog_changed',
      severity: 'info'
    }))
    await this.opts.events.record({
      kind: 'tool_catalog_changed',
      threadId: input.threadId,
      turnId: input.turnId,
      fingerprint: input.fingerprint,
      toolCount: input.toolCount,
      changeKind: input.changeKind,
      toolNames: input.toolNames.slice(0, 50),
      message: input.message
    })
  }

  private recordToolCatalogFingerprint(input: {
    threadId: string
    workspace: string
    mode: string
    route: PromptRoute
    model: string
    activeSkillIds: readonly string[]
    allowedToolNames?: readonly string[]
    userInputDisabled?: boolean
    fingerprint: string
    toolNames: string[]
    toolHashes: Record<string, string>
  }): ToolCatalogDrift {
    const key = JSON.stringify({
      threadId: input.threadId,
      workspace: input.workspace,
      mode: input.mode,
      route: input.route,
      model: input.model,
      activeSkillIds: [...input.activeSkillIds].sort(),
      allowedToolNames: input.allowedToolNames ? [...input.allowedToolNames].sort() : [],
      userInputDisabled: input.userInputDisabled === true
    })
    const current: ToolCatalogSnapshot = {
      fingerprint: input.fingerprint,
      toolNames: input.toolNames,
      toolHashes: input.toolHashes
    }
    const previous = this.toolCatalogSnapshots.get(key)
    this.toolCatalogSnapshots.delete(key)
    this.toolCatalogSnapshots.set(key, current)
    if (this.toolCatalogSnapshots.size > MAX_TOOL_CATALOG_SNAPSHOTS) {
      const oldest = this.toolCatalogSnapshots.keys().next().value
      if (oldest !== undefined) this.toolCatalogSnapshots.delete(oldest)
    }
    if (!previous || previous.fingerprint === input.fingerprint) return { kind: 'none' }
    return isAdditiveToolCatalogChange(previous, current)
      ? { kind: 'additive', previous }
      : { kind: 'breaking', previous }
  }

  private rememberCachePrefixShape(threadId: string, shape: CachePrefixShape): void {
    this.cachePrefixShapes.delete(threadId)
    this.cachePrefixShapes.set(threadId, shape)
    if (this.cachePrefixShapes.size > MAX_CACHE_PREFIX_SHAPES) {
      const oldest = this.cachePrefixShapes.keys().next().value
      if (oldest !== undefined) this.cachePrefixShapes.delete(oldest)
    }
  }

  private async checkBudgetGate(
    thread: Awaited<ReturnType<ThreadStore['get']>>,
    threadId: string,
    turnId: string
  ): Promise<'allow' | 'blocked'> {
    if (!thread) return 'allow'
    const budget = thread.costBudgetUsd
    if (typeof budget !== 'number' || !Number.isFinite(budget) || budget <= 0) return 'allow'
    const spent = this.opts.usage.forThread(threadId).costUsd ?? 0
    if (spent >= budget) {
      const message = `Cost budget exhausted for this thread: $${spent.toFixed(4)} used of $${budget.toFixed(4)}.`
      await this.opts.turns.applyItem(threadId, makeErrorItem({
        id: `item_${turnId}_budget_limited`,
        threadId,
        turnId,
        message,
        code: 'budget_limited'
      }))
      await this.opts.events.record({
        kind: 'error',
        threadId,
        turnId,
        message,
        code: 'budget_limited'
      })
      return 'blocked'
    }
    if (spent >= budget * 0.8 && thread.costBudgetWarningSent !== true) {
      const message = `Cost budget warning: $${spent.toFixed(4)} used of $${budget.toFixed(4)}.`
      await this.opts.threadStore.upsert({
        ...thread,
        costBudgetWarningSent: true,
        updatedAt: this.opts.nowIso()
      })
      await this.opts.turns.applyItem(threadId, makeErrorItem({
        id: `item_${turnId}_budget_warning`,
        threadId,
        turnId,
        message,
        code: 'budget_warning',
        severity: 'warning'
      }))
      await this.opts.events.record({
        kind: 'error',
        threadId,
        turnId,
        message,
        code: 'budget_warning',
        severity: 'warning'
      })
    }
    return 'allow'
  }

  private consumePromptPressure(
    threadId: string,
    model: string
  ): { model: string; promptTokens: number } | undefined {
    if (!threadId) return undefined
    const pressure = this.promptTokenPressure.get(threadId)
    if (!pressure) return undefined
    this.promptTokenPressure.delete(threadId)
    return {
      model: pressure.model || model,
      promptTokens: pressure.promptTokens
    }
  }

  private async resolveTurnModel(input: {
    threadId: string
    turnId: string
    latestRequest: string
    items: readonly TurnItem[]
    signal: AbortSignal
    reasoningEffort?: string
    candidates: Array<string | undefined>
  }): Promise<{ model: string; reasoningEffort?: string }> {
    const requestedReasoningEffort = normalizeRequestedReasoningEffort(input.reasoningEffort)
    const resolved = resolveModelMode(...input.candidates)
    if (resolved.kind === 'fixed') {
      return {
        model: resolved.model,
        ...(requestedReasoningEffort ? { reasoningEffort: requestedReasoningEffort } : {})
      }
    }
    const key = autoModelRouteKey(input.threadId, input.turnId, AUTO_MODEL_ROUTER_FINGERPRINT)
    const cached = this.autoModelRoutes.get(key)
    if (cached) {
      return {
        model: cached.model,
        reasoningEffort: requestedReasoningEffort ?? cached.reasoningEffort
      }
    }
    const route = await resolveAutoModelRoute({
      modelClient: this.opts.model,
      threadId: input.threadId,
      turnId: input.turnId,
      latestRequest: input.latestRequest,
      recentContext: recentAutoRouterContext(input.items, input.turnId),
      selectedModelMode: 'auto',
      abortSignal: input.signal
    })
    this.autoModelRoutes.set(key, route)
    return {
      model: route.model,
      reasoningEffort: requestedReasoningEffort ?? route.reasoningEffort
    }
  }

  private async resolveAttachments(input: {
    attachmentIds: readonly string[]
    threadId: string
    workspace: string
    modelCapabilities: ModelCapabilityMetadata
  }): Promise<{ imageAttachments: ModelInputAttachment[]; textFallbacks: ModelTextAttachmentFallback[] }> {
    if (input.attachmentIds.length === 0) return { imageAttachments: [], textFallbacks: [] }
    if (!this.opts.attachmentStore) {
      throw new Error('attachment store is unavailable')
    }
    const supportsImageInput = input.modelCapabilities.inputModalities.includes('image')
    const textFallbackPolicy = this.opts.attachmentStore.textFallbackPolicy()
    const imageAttachments: ModelInputAttachment[] = []
    const textFallbacks: ModelTextAttachmentFallback[] = []
    for (const id of input.attachmentIds) {
      const attachment = await this.opts.attachmentStore.resolveContent(id, {
        threadId: input.threadId,
        workspace: input.workspace
      })
      if (supportsImageInput) {
        imageAttachments.push({
          id: attachment.id,
          name: attachment.name,
          mimeType: attachment.mimeType,
          dataBase64: attachment.data.toString('base64'),
          ...(attachment.width ? { width: attachment.width } : {}),
          ...(attachment.height ? { height: attachment.height } : {}),
          ...(attachment.localFilePath ? { localFilePath: attachment.localFilePath } : {})
        })
        continue
      }
      textFallbacks.push(buildTextAttachmentFallback(
        attachment,
        textFallbackPolicy.textFallbackMaxBase64Bytes
      ))
    }
    return { imageAttachments, textFallbacks }
  }

  private async retrieveMemories(input: {
    prompt: string
    workspace: string
  }) {
    void input
    return []
  }

  /** Convenience factory for tests: builds a loop with sensible defaults. */
  static defaultPrefix(): ImmutablePrefix {
    return createImmutablePrefix({
      systemPrompt: 'You are Analytix, a careful and helpful assistant.',
      pinnedConstraints: ['user: preserve recent turns', 'project: keep responses concise']
    })
  }
}

function buildTextAttachmentFallback(
  attachment: AttachmentContent,
  maxBase64Bytes: number
): ModelTextAttachmentFallback {
  const fallback = attachment.textFallback
  if (fallback) {
    const fallbackBase64Bytes = Buffer.byteLength(fallback.dataBase64, 'utf8')
    if (fallbackBase64Bytes > maxBase64Bytes) {
      throw new Error(`attachment ${attachment.id} text fallback exceeds ${maxBase64Bytes} base64 byte limit`)
    }
    return {
      id: attachment.id,
      name: attachment.name,
      mimeType: fallback.mimeType,
      dataBase64: fallback.dataBase64,
      byteSize: fallback.byteSize,
      ...(fallback.width ? { width: fallback.width } : {}),
      ...(fallback.height ? { height: fallback.height } : {}),
      ...(attachment.localFilePath ? { localFilePath: attachment.localFilePath } : {}),
      ...(fallback.wasCompressed !== undefined ? { wasCompressed: fallback.wasCompressed } : {})
    }
  }

  const originalBase64 = attachment.data.toString('base64')
  if (Buffer.byteLength(originalBase64, 'utf8') > maxBase64Bytes) {
    throw new Error(
      `attachment ${attachment.id} is missing a compressed text fallback and original base64 exceeds ${maxBase64Bytes} byte limit`
    )
  }
  return {
    id: attachment.id,
    name: attachment.name,
    mimeType: attachment.mimeType,
    dataBase64: originalBase64,
    byteSize: attachment.byteSize,
    ...(attachment.width ? { width: attachment.width } : {}),
    ...(attachment.height ? { height: attachment.height } : {}),
    ...(attachment.localFilePath ? { localFilePath: attachment.localFilePath } : {}),
    wasCompressed: false
  }
}

function attachmentRequestPipelineDetails(input: {
  attachmentIds: readonly string[]
  imageAttachments: readonly ModelInputAttachment[]
  textFallbacks: readonly ModelTextAttachmentFallback[]
  modelCapabilities: ModelCapabilityMetadata
}): Record<string, unknown> {
  if (
    input.attachmentIds.length === 0 &&
    input.imageAttachments.length === 0 &&
    input.textFallbacks.length === 0
  ) {
    return {}
  }
  return {
    attachmentIds: [...input.attachmentIds],
    modelInputModalities: [...input.modelCapabilities.inputModalities],
    modelMessageParts: [...input.modelCapabilities.messageParts],
    imageAttachmentCount: input.imageAttachments.length,
    imageAttachmentBase64Bytes: input.imageAttachments.reduce(
      (total, attachment) => total + Buffer.byteLength(attachment.dataBase64, 'base64'),
      0
    ),
    imageAttachmentMimeTypes: [...new Set(input.imageAttachments.map((attachment) => attachment.mimeType))],
    textFallbackCount: input.textFallbacks.length,
    textFallbackBase64Bytes: input.textFallbacks.reduce(
      (total, attachment) => total + Buffer.byteLength(attachment.dataBase64, 'utf8'),
      0
    ),
    textFallbackMimeTypes: [...new Set(input.textFallbacks.map((attachment) => attachment.mimeType))]
  }
}

function imageGenerationReferenceInstructions(input: {
  imageAttachments: readonly ModelInputAttachment[]
  textFallbacks: readonly ModelTextAttachmentFallback[]
  workspace: string
  tools: readonly Pick<ModelToolSpec, 'name'>[]
}): string[] {
  if (!input.tools.some((tool) => tool.name === 'generate_image')) return []

  const references = [...input.imageAttachments, ...input.textFallbacks]
    .filter((attachment) => attachment.mimeType.startsWith('image/'))
    .map((attachment) => ({
      name: attachment.name,
      path: workspaceRelativeAttachmentPath(attachment.localFilePath, input.workspace)
    }))
    .filter((attachment): attachment is { name: string; path: string } => Boolean(attachment.path))

  if (references.length === 0) return []
  return [[
    'Image-to-image reference images are available for this turn:',
    ...references.map((reference) => `- ${reference.name}: ${reference.path}`),
    'For image edits, restyles, redraws, or transformations, call `generate_image` with the matching workspace-relative path(s) in `reference_image_paths`.'
  ].join('\n')]
}

function workspaceRelativeAttachmentPath(
  localFilePath: string | undefined,
  workspace: string
): string | null {
  const workspaceRoot = workspace.trim()
  const rawPath = localFilePath?.trim()
  if (!workspaceRoot || !rawPath) return null

  const workspaceAbsolute = resolve(workspaceRoot)
  const fileAbsolute = isAbsolute(rawPath) ? resolve(rawPath) : resolve(workspaceAbsolute, rawPath)
  const relativePath = relative(workspaceAbsolute, fileAbsolute)
  if (!relativePath || relativePath.startsWith('..') || isAbsolute(relativePath)) return null
  return relativePath.replace(/\\/g, '/')
}

function normalizeApprovalPolicy(
  value: string | undefined
): ToolHostContext['approvalPolicy'] {
  switch (value) {
    case 'on-request':
    case 'always':
    case 'never':
    case 'auto':
    case 'suggest':
    case 'untrusted':
      return value
    default:
      return DEFAULT_APPROVAL_POLICY
  }
}

function normalizeSandboxMode(
  value: string | undefined
): NonNullable<ToolHostContext['sandboxMode']> {
  switch (value) {
    case 'read-only':
    case 'workspace-write':
    case 'danger-full-access':
    case 'external-sandbox':
      return value
    default:
      return DEFAULT_SANDBOX_MODE
  }
}

function isAdditiveToolCatalogChange(previous: ToolCatalogSnapshot, current: ToolCatalogSnapshot): boolean {
  let added = false
  for (const name of current.toolNames) {
    if (!previous.toolHashes[name]) added = true
  }
  if (!added) return false
  for (const name of previous.toolNames) {
    const previousHash = previous.toolHashes[name]
    const currentHash = current.toolHashes[name]
    if (!previousHash || !currentHash || previousHash !== currentHash) return false
  }
  return true
}

function advertisedToolSourcesForCacheDiagnostics(
  tools: ReadonlyArray<{ providerId?: string; providerKind?: ToolProviderKind }>
): ToolSourceCacheShape[] {
  const sources = new Map<string, ToolSourceCacheShape>()
  for (const tool of tools) {
    if (!tool.providerId) continue
    sources.set(tool.providerId, {
      id: tool.providerId,
      kind: tool.providerKind ?? 'unknown',
      status: 'advertised'
    })
  }
  return [...sources.values()].sort((a, b) => a.id.localeCompare(b.id))
}

function buildLoopModelExecutionRef(input: {
  modelId?: string
  providerId?: string
  variant?: string
  endpointFormat?: string
  providerBaseUrl?: string
  modelCapabilities?: ModelCapabilityMetadata
  toolCatalogFingerprint?: string
  source: ModelExecutionSource
  resolvedAt: string
}): ModelExecutionRef | undefined {
  const modelId = input.modelId?.trim()
  if (!modelId) return undefined
  const providerId = input.providerId?.trim()
  if (!providerId) return undefined
  const endpointFormat = normalizeOptionalEndpointFormat(input.endpointFormat)
  const variant = input.variant?.trim()
  const urlFingerprints = modelExecutionUrlFingerprints(input.providerBaseUrl, endpointFormat)
  return ModelExecutionRefSchema.parse({
    modelId,
    providerId,
    ...(variant ? { variant } : {}),
    ...(endpointFormat ? { endpointFormat } : {}),
    ...urlFingerprints,
    capabilityFingerprint: modelExecutionCapabilityFingerprint({
      endpointFormat,
      modelCapabilities: input.modelCapabilities,
      toolCatalogFingerprint: input.toolCatalogFingerprint
    }),
    source: input.source,
    resolvedAt: input.resolvedAt
  })
}

function normalizeOptionalEndpointFormat(value: string | undefined): ModelEndpointFormat | undefined {
  const raw = value?.trim()
  return raw ? normalizeModelEndpointFormat(raw) : undefined
}

function modelExecutionUrlFingerprints(
  providerBaseUrl: string | undefined,
  endpointFormat: ModelEndpointFormat | undefined
): Pick<ModelExecutionRef, 'baseUrlFingerprint' | 'customFullEndpointFingerprint'> {
  const value = providerBaseUrl?.trim()
  if (!value) return {}
  const fingerprint = shortFingerprint(value)
  return endpointFormat === 'custom_endpoint'
    ? { customFullEndpointFingerprint: fingerprint }
    : { baseUrlFingerprint: fingerprint }
}

function modelExecutionCapabilityFingerprint(input: {
  endpointFormat?: ModelEndpointFormat
  modelCapabilities?: ModelCapabilityMetadata
  toolCatalogFingerprint?: string
}): string {
  const capabilities = input.modelCapabilities
  return stableObjectFingerprint({
    endpointFormat: input.endpointFormat ?? capabilities?.endpointFormat ?? 'unknown',
    supportsToolCalling: capabilities?.supportsToolCalling ?? 'unknown',
    supportsImageInput: capabilities?.supportsImageInput ?? capabilities?.inputModalities.includes('image') ?? false,
    inputModalities: [...(capabilities?.inputModalities ?? [])].sort(),
    messageParts: [...(capabilities?.messageParts ?? [])].sort(),
    reasoningProtocol: capabilities?.reasoning?.requestProtocol ?? 'none',
    cacheTelemetry: cacheTelemetryFamily(input.endpointFormat ?? capabilities?.endpointFormat),
    toolCatalogFingerprint: input.toolCatalogFingerprint ?? 'unresolved'
  })
}

function cacheTelemetryFamily(endpointFormat: ModelEndpointFormat | undefined): string {
  switch (endpointFormat) {
    case 'responses':
      return 'openai-responses-usage'
    case 'messages':
      return 'anthropic-messages-usage'
    case 'custom_endpoint':
      return 'custom-endpoint-usage'
    case 'chat_completions':
      return 'openai-chat-completions-usage'
    default:
      return 'unknown'
  }
}

function shortFingerprint(value: string): string {
  return createHash('sha256').update(value).digest('hex').slice(0, 16)
}

function stableObjectFingerprint(value: unknown): string {
  return shortFingerprint(JSON.stringify(canonicalJsonValue(value)))
}

function canonicalJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalJsonValue)
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    out[key] = canonicalJsonValue((value as Record<string, unknown>)[key])
  }
  return out
}

function buildToolCatalogDriftMessage(toolCatalog: {
  fingerprint: string
  toolCount: number
  toolNames: string[]
}, changeKind: 'additive' | 'breaking'): string {
  const sample = toolCatalog.toolNames.slice(0, 12).join(', ')
  const suffix = toolCatalog.toolNames.length > 12 ? `, +${toolCatalog.toolNames.length - 12} more` : ''
  const policy = changeKind === 'additive'
    ? 'Only additive tool changes are allowed in-place; Analytix will continue with the refreshed tool list.'
    : 'Non-additive tool changes can invalidate prompt-cache assumptions; Analytix stopped this turn. Start a new thread after editing, removing, or reordering tool schemas.'
  return [
    `Tool catalog changed for this thread (${toolCatalog.toolCount} tools, fingerprint ${toolCatalog.fingerprint}).`,
    policy,
    sample ? `Current tools: ${sample}${suffix}.` : ''
  ].filter(Boolean).join(' ')
}

function resolveModelMode(...candidates: Array<string | undefined>): { kind: 'fixed'; model: string } | { kind: 'auto' } {
  for (const candidate of candidates) {
    const trimmed = candidate?.trim() ?? ''
    if (!trimmed) continue
    return trimmed.toLowerCase() === 'auto'
      ? { kind: 'auto' }
      : { kind: 'fixed', model: trimmed }
  }
  return { kind: 'fixed', model: '' }
}

function normalizeRequestedReasoningEffort(effort: string | undefined): string | undefined {
  const normalized = effort?.trim().toLowerCase()
  return normalized && normalized !== 'auto' ? normalized : undefined
}

function sanitizeProviderBaseUrl(baseUrl: string): string {
  try {
    const url = new URL(baseUrl)
    url.username = ''
    url.password = ''
    url.search = ''
    url.hash = ''
    return url.toString().replace(/\/$/, '')
  } catch {
    return baseUrl.replace(/[?#].*$/, '').replace(/\/+$/, '')
  }
}

function autoModelRouteKey(threadId: string, turnId: string, classifierFingerprint: string): string {
  return `${threadId}:${turnId}:${classifierFingerprint}`
}

function prefixVolatilityStageDetails(
  findings: PrefixVolatilityFinding[]
): Record<string, unknown> | undefined {
  if (findings.length === 0) return undefined
  const kinds = [...new Set(findings.map((finding) => finding.kind))].sort()
  const fields = [...new Set(findings.map((finding) => finding.field))].sort()
  return {
    prefixVolatileTokenCount: findings.length,
    prefixVolatileTokenKinds: kinds,
    prefixVolatileFields: fields,
    noRegexDetector: true
  }
}
